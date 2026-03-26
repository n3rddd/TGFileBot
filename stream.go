package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mtproto "github.com/amarnathcjd/gogram"
	"github.com/amarnathcjd/gogram/telegram"
)

type Reader struct {
	Ctx           context.Context
	Cancel        context.CancelFunc
	Client        *telegram.Client
	Location      telegram.InputFileLocation
	DC            int32
	Start         int64
	End           int64
	ChunkSize     int64
	ContentLength int64
	ChannelID     int64
	MessageID     int32
	Cate          string
	Buffers       chan []byte
	Errs          chan error
	CurrBuffer    []byte
	Pos           int
	ReadBytes     int64
	Refreshing    bool
	Cond          *sync.Cond
	Version       atomic.Int64
	Once          sync.Once
	Mutex         sync.Mutex
	LastRefresh   time.Time // 记录上次刷新时间，避免刷新过于频繁
}

func (reader *Reader) Close() error {
	if reader.Cancel != nil {
		reader.Cancel()
	}
	return nil
}

func newReader(
	ctx context.Context,
	client *telegram.Client,
	location telegram.InputFileLocation,
	dc int32,
	start int64,
	end int64,
	contentLength int64,
	channelID int64,
	messageID int32,
	cate string,
) io.ReadCloser {
	ctx, cancel := context.WithCancel(ctx)
	reader := &Reader{
		Ctx:           ctx,
		Cancel:        cancel,
		Client:        client,
		Location:      location,
		DC:            dc,
		Start:         start,
		End:           end,
		ChunkSize:     int64(1024 * 1024),
		ContentLength: contentLength,
		ChannelID:     channelID,
		MessageID:     messageID,
		Cate:          cate,
		Buffers:       make(chan []byte, 8), // Buffer up to 8MB
		Errs:          make(chan error, 1),
		Cond:          sync.NewCond(new(sync.Mutex)),
	}
	return reader
}

func (reader *Reader) startFetching() {
	go func() {
		defer close(reader.Buffers)

		workers := infos.Conf.Workers
		if workers == 0 {
			workers = 1
		}
		type task struct {
			index  int
			offset int64
		}
		type result struct {
			content []byte
			index   int
			err     error
		}

		tasks := make(chan task, workers)
		results := make(chan result, workers)

		// Start workers
		for count := 0; count < workers; count++ {
			go func() {
				for t := range tasks {
					content, err := reader.fetchChunk(t.offset)
					select {
					case results <- result{index: t.index, content: content, err: err}:
						// 成功发送结果
					case <-reader.Ctx.Done():
						return
					}
				}
			}()
		}

		totalChunks := int((reader.End - (reader.Start - (reader.Start % reader.ChunkSize)) + reader.ChunkSize) / reader.ChunkSize)
		if reader.End < reader.Start {
			totalChunks = 0
		}

		go func() {
			defer close(tasks)
			startOffset := reader.Start - (reader.Start % reader.ChunkSize)
			for count := 0; count < totalChunks; count++ {
				select {
				case tasks <- task{index: count, offset: startOffset + int64(count)*reader.ChunkSize}:
					// 成功发送任务
				case <-reader.Ctx.Done():
					return
				}
			}
		}()

		// Collector
		contents := make(map[int][]byte)
		nextIndex := 0
		for nextIndex < totalChunks {
			select {
			case res := <-results:
				if res.err != nil {
					select {
					case reader.Errs <- res.err:
						// 成功发送错误
					default:
					}
					return
				}
				contents[res.index] = res.content
				for {
					content, ok := contents[nextIndex]
					if !ok {
						break
					}

					// Handle cuts for first and last chunks
					if totalChunks == 1 {
						firstCut := reader.Start % reader.ChunkSize
						lastCut := (reader.End % reader.ChunkSize) + 1
						if int64(len(content)) > lastCut {
							content = content[:lastCut]
						}
						if int64(len(content)) > firstCut {
							content = content[firstCut:]
						} else {
							content = []byte{}
						}
					} else if nextIndex == 0 {
						firstCut := reader.Start % reader.ChunkSize
						if int64(len(content)) > firstCut {
							content = content[firstCut:]
						} else {
							content = []byte{}
						}
					} else if nextIndex == totalChunks-1 {
						lastCut := (reader.End % reader.ChunkSize) + 1
						if int64(len(content)) > lastCut {
							content = content[:lastCut]
						}
					}

					if len(content) > 0 {
						select {
						case reader.Buffers <- content:
							// 成功发送数据
						case <-reader.Ctx.Done():
							return
						}
					}
					delete(contents, nextIndex)
					nextIndex++
				}
			case <-reader.Ctx.Done():
				return
			}
		}
	}()
}

func (reader *Reader) fetchChunk(offset int64) (content []byte, err error) {
	for count := 0; count < 3; count++ {
		select {
		case <-reader.Ctx.Done():
			return nil, fmt.Errorf("canceled")
		default:
		}
		reader.Mutex.Lock()
		loc := reader.Location
		version := reader.Version.Load()
		targetDC := int(reader.DC)
		reader.Mutex.Unlock()

		params := &telegram.UploadGetFileParams{
			Location: loc,
			Offset:   offset,
			Limit:    int32(reader.ChunkSize),
		}

		var res any

		if targetDC != 0 {
			infos.Mutex.Lock()
			if infos.Senders == nil {
				infos.Senders = make(map[int]*mtproto.MTProto)
			}
			sender, ok := infos.Senders[targetDC]

			if ok {
				res, err = sender.MakeRequest(params)
				infos.Mutex.Unlock()
			} else {
				// 尝试创建一个导出授权的 Sender
				newSender, serr := reader.Client.CreateExportedSender(targetDC, false)
				if serr == nil {
					infos.Senders[targetDC] = newSender
					log.Printf("成功创建 DC %d 的导出 Sender", targetDC)
					res, err = newSender.MakeRequest(params)
				} else {
					log.Printf("创建 DC %d 的导出 Sender 失败: %v, 将回退到主客户端", targetDC, serr)
					res, err = reader.Client.UploadGetFile(params)
				}
				infos.Mutex.Unlock()
			}
		} else {
			res, err = reader.Client.UploadGetFile(params)
		}

		if err != nil {
			// 如果是文件引用过期, 通过版本号合并并发刷新请求
			if (strings.Contains(err.Error(), "FILE_REFERENCE") || strings.Contains(err.Error(), "EXPIRED")) &&
				reader.ChannelID != 0 && reader.MessageID != 0 {
				log.Printf("获取分片失败提示引用过期 (%d/3), 尝试刷新消息: %v", count+1, err)
				if refreshed, wait := reader.refresh(version); refreshed {
					// 刷新成功或有其他 worker 刷新，等待指定时间后重试
					if wait > 0 {
						time.Sleep(wait)
					}
					continue
				}
			}

			// 如果是其它错误或刷新失败, 也给一次重试机会（网络抖动等）
			time.Sleep(time.Duration(count+1) * time.Second)
			continue
		}

		if obj, ok := res.(*telegram.UploadFileObj); ok {
			return obj.Bytes, nil
		}
		return nil, fmt.Errorf("未知的响应类型: %T", res)
	}
	return nil, err
}

func (reader *Reader) refresh(version int64) (refreshed bool, waitDuration time.Duration) {
	reader.Mutex.Lock()

	// 如果别人正在刷新 → 等
	for reader.Refreshing {
		reader.Cond.Wait()
	}

	const cooldown = 8 * time.Second

	currentVersion := reader.Version.Load()
	if currentVersion > version {
		// 其他 worker 已刷新过；计算需要等待多久才能让新引用在 Telegram 服务端生效
		elapsed := time.Since(reader.LastRefresh)
		if elapsed < cooldown {
			remaining := cooldown - elapsed
			log.Printf("其他 worker 刚刷新过文件引用 (版本 %d -> %d, %v前), 等待 %v 后复用", version, currentVersion, elapsed.Round(time.Millisecond), remaining.Round(time.Millisecond))
			reader.Mutex.Unlock()
			return true, remaining
		}
		log.Printf("其他 worker 已刷新文件引用 (版本 %d -> %d), 直接复用", version, currentVersion)
		reader.Mutex.Unlock()
		return true, 0
	}

	// 距上次刷新不足冷却时间，等剩余时间后再重试（避免拿到相同的过期引用）
	if !reader.LastRefresh.IsZero() {
		elapsed := time.Since(reader.LastRefresh)
		if elapsed < cooldown {
			remaining := cooldown - elapsed
			log.Printf("距离上次刷新仅 %v, 等待 %v 后重试以避免获取到相同的过期引用", elapsed.Round(time.Millisecond), remaining.Round(time.Millisecond))
			reader.Mutex.Unlock()
			return true, remaining
		}
	}
		
	reader.Refreshing = true
	reader.Mutex.Unlock()

	ms, err := reader.Client.GetMessages(reader.ChannelID, &telegram.SearchOption{IDs: []int32{reader.MessageID}})
	if err != nil {
		log.Printf("刷新消息位置失败: %v", err)
		return false, 0
	}

	if len(ms) == 0 {
		log.Printf("刷新消息位置失败: 未找到消息或消息列表为空")
		return false, 0
	}

	log.Printf("成功获取消息进行刷新, 消息数量: %d", len(ms))
	src := ms[0]
	if !src.IsMedia() {
		log.Printf("获取到的消息不包含媒体内容, 无法刷新文件引用")
		return false, 0
	}

	newLoc, newDC, _, _, err := telegram.GetFileLocation(src.Media(), telegram.FileLocationOptions{})
	if err != nil {
		log.Printf("从媒体刷新文件位置失败: %v", err)
		return false, 0
	}

	reader.Mutex.Lock()
	reader.Refreshing = false
	reader.Location = newLoc
	reader.DC = newDC
	reader.Version.Add(1)
	reader.LastRefresh = time.Now()
	reader.Cond.Broadcast()
	reader.Mutex.Unlock()
	
	log.Printf("成功刷新文件引用, DC: %d, 新位置: %+v", newDC, newLoc)
	// 等待冷却时间让新引用在 Telegram 服务端生效
	return true, cooldown
}

func (reader *Reader) Read(content []byte) (num int, err error) {
	reader.Once.Do(reader.startFetching)

	if reader.ReadBytes >= reader.ContentLength {
		return 0, io.EOF
	}

	if reader.Pos >= len(reader.CurrBuffer) {
		select {
		case data, ok := <-reader.Buffers:
			if !ok {
				select {
				case err := <-reader.Errs:
					return 0, err
				default:
					return 0, io.EOF
				}
			}
			reader.CurrBuffer = data
			reader.Pos = 0
		case err := <-reader.Errs:
			return 0, err
		case <-reader.Ctx.Done():
			return 0, reader.Ctx.Err()
		}
	}

	num = copy(content, reader.CurrBuffer[reader.Pos:])
	reader.Pos += num
	reader.ReadBytes += int64(num)
	return num, nil
}
