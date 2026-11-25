package siwx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
)

// 本地接口单页最多返回 300 条；用 before 向前翻页拿到完整窗口。
const chatPageSize = 300

const maxChatPages = 200

type chatMessage struct {
	ID                json.RawMessage `json:"id"`
	PlatformMessageID string          `json:"platformMessageId"`
	TS                int64           `json:"ts"`
	SenderName        string          `json:"sender_name"`
	SenderWxid        string          `json:"sender_wxid"`
	Text              string          `json:"text"`
	Kind              string          `json:"kind"`
	Link              json.RawMessage `json:"link"`
	Channels          json.RawMessage `json:"channels"`
}

type chatResponse struct {
	Messages []chatMessage `json:"messages"`
}

// fetchChatSince 拉取 since 之后的消息（自动分页），按时间升序返回。
func fetchChatSince(client *http.Client, baseURL, account, chat string, since int64) ([]chatMessage, error) {
	var all []chatMessage
	seen := make(map[string]bool)
	prevOldest := int64(0)
	var before int64

	for page := 0; page < maxChatPages; page++ {
		messages, err := fetchChatPage(client, baseURL, account, chat, before)
		if err != nil {
			return nil, err
		}
		if len(messages) == 0 {
			break
		}

		added := 0
		for _, message := range messages {
			key := chatMessageKey(message)
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, message)
			added++
		}

		oldest := messages[0].TS
		for _, message := range messages {
			if message.TS < oldest {
				oldest = message.TS
			}
		}
		if len(messages) < chatPageSize {
			break
		}
		if since > 0 && oldest <= since {
			break
		}
		// 没有新消息或翻页没有前进时停止，避免死循环。
		if added == 0 || (before != 0 && oldest == prevOldest) {
			break
		}
		prevOldest = oldest
		// before 是开区间；+1 把同一秒的消息也带上，重复项由 seen 去重。
		before = oldest + 1
	}

	sort.SliceStable(all, func(i, j int) bool {
		if all[i].TS == all[j].TS {
			return chatMessageKey(all[i]) < chatMessageKey(all[j])
		}
		return all[i].TS < all[j].TS
	})
	return all, nil
}

func fetchChatPage(client *http.Client, baseURL, account, chat string, before int64) ([]chatMessage, error) {
	query := url.Values{}
	query.Set("account", account)
	query.Set("chat", chat)
	query.Set("limit", strconv.Itoa(chatPageSize))
	if before > 0 {
		query.Set("before", strconv.FormatInt(before, 10))
	}

	var data chatResponse
	if err := getJSON(client, baseURL+"/api/chat/messages?"+query.Encode(), &data); err != nil {
		return nil, fmt.Errorf("读取聊天消息失败: %w", err)
	}
	return data.Messages, nil
}

func chatMessageKey(message chatMessage) string {
	if message.PlatformMessageID != "" {
		return "p:" + message.PlatformMessageID
	}
	if len(message.ID) > 0 && string(message.ID) != "null" {
		return "i:" + string(message.ID)
	}
	return fmt.Sprintf("t:%d|%s|%s", message.TS, message.SenderWxid, message.Text)
}
