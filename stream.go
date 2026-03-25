package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	mtproto "github.com/amarnathcjd/gogram"
	"github.com/amarnathcjd/gogram/telegram"
	"golang.org/x/sync/singleflight"
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
	Once          sync.Once
	Mutex         sync.Mutex
	Refreshes     singleflight.Group
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
	}
	return reader
}

func (reader *Reader) startFetching() {
	go func() {
		defer close(reader.Buffers)

		workers := 4
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
		reader.Mutex.Lock()
		loc := reader.Location
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
			infos.Mutex.Unlock()

			if ok {
				res, err = sender.MakeRequest(params)
			} else {
				// 尝试创建一个导出授权的 Sender
				newSender, serr := reader.Client.CreateExportedSender(targetDC, false)
				if serr == nil {
					infos.Mutex.Lock()
					infos.Senders[targetDC] = newSender
					infos.Mutex.Unlock()
					log.Printf("成功创建 DC %d 的导出 Sender", targetDC)
					res, err = newSender.MakeRequest(params)
				} else {
					log.Printf("创建 DC %d 的导出 Sender 失败: %v, 将回退到主客户端", targetDC, serr)
					res, err = reader.Client.UploadGetFile(params)
				}
			}
		} else {
			res, err = reader.Client.UploadGetFile(params)
		}

		if err != nil {
			// 如果是文件引用过期, 通过 singleflight 合并并发刷新请求
			if (strings.Contains(err.Error(), "FILE_REFERENCE") || strings.Contains(err.Error(), "EXPIRED")) &&
				reader.ChannelID != 0 && reader.MessageID != 0 {

				log.Printf("获取分片失败提示引用过期 (%d/3), 尝试刷新消息: %v", count+1, err)
				if _, err, _ := reader.Refreshes.Do("refresh", reader.refresh); err != nil {
					log.Printf("刷新文件引用失败: %v", err)
				} else {
					time.Sleep(time.Second) // 稍作等待后重试
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

func (reader *Reader) refresh() (interface{}, error) {
	/*
		if _, err := reader.Client.GetDialogs(&telegram.DialogOptions{Limit: 100}); err != nil {
			return nil, fmt.Errorf("刷新对话列表失败: %w", err)
		}
	*/

	ms, err := reader.Client.GetMessages(reader.ChannelID, &telegram.SearchOption{IDs: []int32{reader.MessageID}})
	if err != nil {
		return nil, fmt.Errorf("刷新消息位置失败: %w", err)
	}
	if len(ms) == 0 {
		return nil, fmt.Errorf("刷新消息位置失败: 未找到消息或消息列表为空")
	}

	src := ms[0]
	if !src.IsMedia() {
		return nil, fmt.Errorf("获取到的消息不包含媒体内容, 无法刷新文件引用")
	}

	newLoc, newDC, _, _, err := telegram.GetFileLocation(src.Media(), telegram.FileLocationOptions{})
	if err != nil {
		return nil, fmt.Errorf("从媒体刷新文件位置失败: %w", err)
	}

	reader.Mutex.Lock()
	reader.Location = newLoc
	reader.DC = newDC
	reader.Mutex.Unlock()
	log.Printf("成功刷新文件引用, DC: %d, 新位置: %+v", newDC, newLoc)
	return nil, nil
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
