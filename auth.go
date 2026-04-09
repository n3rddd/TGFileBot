package main

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"

	handleUrl "net/url"
)

// buildSets 根据配置重建 AdminMap 和 WhiteMap 以支持 O(1) 权限查询
func (infos *Infos) buildSets() {
	infos.AdminMap = make(map[int64]struct{}, len(infos.Conf.AdminIDs))
	for _, id := range infos.Conf.AdminIDs {
		infos.AdminMap[id] = struct{}{}
	}
	infos.WhiteMap = make(map[int64]struct{}, len(infos.Conf.WhiteIDs))
	for _, id := range infos.Conf.WhiteIDs {
		infos.WhiteMap[id] = struct{}{}
	}
}

func (infos *Infos) isAdmin(id int64) bool {
	if id == infos.Conf.UserID {
		return true
	}
	_, ok := infos.AdminMap[id]
	return ok
}

func (infos *Infos) isWhite(id int64) bool {
	if id == infos.BotID {
		return true
	}
	if infos.isAdmin(id) {
		return true
	}
	_, ok := infos.WhiteMap[id]
	return ok
}

// calculateHash 为指定用户 ID 生成 6 位 MD5 哈希，用于鉴权
func (infos *Infos) calculateHash(userID int64) string {
	if infos.Conf.Password == "" {
		return ""
	}
	res := fmt.Sprintf("%d%s", userID, infos.Conf.Password)
	src := md5.Sum([]byte(res))
	return hex.EncodeToString(src[:])[:6]
}

// checkHash 根据哈希值查找对应的用户 ID，返回 0 表示未找到
func (infos *Infos) checkHash(hash string) int64 {
	if hash == "" {
		return 0
	}
	if value, ok := infos.IDs[infos.Conf.UserID]; ok && value != "" {
		if value == hash {
			return infos.Conf.UserID
		}
	} else {
		infos.IDs[infos.Conf.UserID] = infos.calculateHash(infos.Conf.UserID)
	}

	for _, id := range infos.Conf.AdminIDs {
		if value, ok := infos.IDs[id]; ok && value != "" {
			if value == hash {
				return id
			}
		} else {
			infos.IDs[id] = infos.calculateHash(id)
		}
	}

	for _, id := range infos.Conf.WhiteIDs {
		if value, ok := infos.IDs[id]; ok && value != "" {
			if value == hash {
				return id
			}
		} else {
			infos.IDs[id] = infos.calculateHash(id)
		}
	}
	return 0
}

// checkPass 验证 HTTP 请求中的访问密码或哈希
func checkPass(params handleUrl.Values) error {
	if infos.Conf.Password != "" {
		hash := params.Get("hash") // 基于用户 ID 的哈希校验
		password := params.Get("key")
		switch {
		case password != "":
			if password != infos.Conf.Password {
				return errors.New("无效的密码")
			}
		case hash != "":
			value := params.Get("uid")
			uid, err := strconv.ParseInt(value, 10, 64)
			if err == nil && uid != 0 {
				if hash != infos.calculateHash(uid) {
					return errors.New("无效的哈希密码")
				}
			} else {
				log.Printf("UID无效: %s", value)
				return errors.New("无效的UID")
			}
		default:
			return errors.New("您没有权限访问此链接")
		}
	}
	return nil
}
