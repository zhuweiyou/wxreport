package siwx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"wxreport/internal/fsutil"
)

// ExportMessage 是导出文件里的一条消息。Index 从 1 开始，供话题聚类引用。
type ExportMessage struct {
	Index     int    `json:"i"`
	MID       string `json:"mid"`
	TS        int64  `json:"ts"`
	Sender    string `json:"sender"`
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"`
	Link      string `json:"link,omitempty"`
	LinkTitle string `json:"link_title,omitempty"`
}

// ExportStats 是导出后的汇总信息。
type ExportStats struct {
	Total        int
	Participants int
	Output       string
}

// 这些类型的消息不是成员发言，不进入话题统计。
var ignoredKinds = map[string]bool{
	"system": true,
	"recall": true,
	"revoke": true,
	"join":   true,
	"leave":  true,
}

// Export 导出 [start, end] 窗口内的消息为 JSON 文件。
// 昵称优先群昵称，其次 contact.nick_name（微信昵称），绝不使用备注名。
func Export(account, chat string, start, end int64, out string) (ExportStats, error) {
	baseURL := strings.TrimRight(envOr("SIWX_BASE_URL", defaultBaseURL), "/")
	client := &http.Client{Timeout: 60 * time.Second}

	messages, err := fetchChatSince(client, baseURL, account, chat, start)
	if err != nil {
		return ExportStats{}, err
	}

	var overview overviewResponse
	if err := getJSON(client, baseURL+"/api/settings/overview", &overview); err != nil {
		return ExportStats{}, fmt.Errorf("读取解密输出目录失败: %w", err)
	}
	outputRoot := overview.OutputRoot
	if outputRoot == "" {
		outputRoot = "/Applications/stories-in-wx.app/Contents/MacOS/output"
	}
	contactDB := filepath.Join(outputRoot, account, "contact", "contact.db")
	nickNames, err := loadNickNames(contactDB)
	if err != nil {
		return ExportStats{}, err
	}
	groupNames, err := loadGroupNickNames(contactDB, chat)
	if err != nil {
		return ExportStats{}, err
	}

	exported := make([]ExportMessage, 0, len(messages))
	senders := make(map[string]bool)
	for _, message := range messages {
		if message.TS < start || message.TS > end {
			continue
		}
		if message.SenderWxid == "" || ignoredKinds[strings.ToLower(message.Kind)] {
			continue
		}
		sender := preferredName(message, groupNames, nickNames)
		card := decodeLink(message.Link)
		link := ""
		for _, raw := range messageURLs(message, card) {
			if cleaned := cleanURL(raw); cleaned != "" {
				link = cleaned
				break
			}
		}
		item := ExportMessage{
			MID:    chatMessageKey(message),
			TS:     message.TS,
			Sender: sender,
			Kind:   message.Kind,
			Text:   strings.TrimSpace(message.Text),
			Link:   link,
		}
		if item.Text == "" && link != "" {
			item.LinkTitle = resourceTitle(message, card)
		}
		exported = append(exported, item)
		senders[sender] = true
	}
	for i := range exported {
		exported[i].Index = i + 1
	}

	content, err := marshalExport(chat, start, end, exported, len(senders))
	if err != nil {
		return ExportStats{}, err
	}
	if err := fsutil.WriteFileAtomic(out, content, 0o644); err != nil {
		return ExportStats{}, err
	}

	return ExportStats{Total: len(exported), Participants: len(senders), Output: out}, nil
}

// preferredName 优先群昵称，其次微信昵称，最后回退到 wxid；绝不使用备注。
func preferredName(message chatMessage, groupNames, nickNames map[string]string) string {
	for _, name := range []string{groupNames[message.SenderWxid], nickNames[message.SenderWxid]} {
		if meaningfulName(name) {
			return strings.TrimSpace(name)
		}
	}
	name := message.SenderWxid
	if !meaningfulName(name) {
		name = "未知成员"
	}
	return strings.TrimSpace(name)
}

// marshalExport 输出「一行一条消息」的 JSON，方便人和模型逐行阅读。
func marshalExport(chat string, start, end int64, messages []ExportMessage, participants int) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteString("{\n")
	writeJSONField(&buffer, "chat", chat, true)
	buffer.WriteString("  \"start\": " + fmt.Sprint(start) + ",\n")
	buffer.WriteString("  \"end\": " + fmt.Sprint(end) + ",\n")
	buffer.WriteString("  \"total\": " + fmt.Sprint(len(messages)) + ",\n")
	buffer.WriteString("  \"participants\": " + fmt.Sprint(participants) + ",\n")
	buffer.WriteString("  \"messages\": [\n")
	for i, message := range messages {
		line, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}
		buffer.WriteString("    ")
		buffer.Write(line)
		if i+1 < len(messages) {
			buffer.WriteString(",")
		}
		buffer.WriteString("\n")
	}
	buffer.WriteString("  ]\n}\n")
	return buffer.Bytes(), nil
}

func writeJSONField(buffer *bytes.Buffer, key, value string, trailingComma bool) {
	encoded, _ := json.Marshal(value)
	buffer.WriteString("  ")
	keyJSON, _ := json.Marshal(key)
	buffer.Write(keyJSON)
	buffer.WriteString(": ")
	buffer.Write(encoded)
	if trailingComma {
		buffer.WriteString(",")
	}
	buffer.WriteString("\n")
}
