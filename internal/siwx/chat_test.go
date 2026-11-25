package siwx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestFetchChatSincePaginates(t *testing.T) {
	const total = 650
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = chatPageSize
		}

		var page []chatMessage
		for ts := int64(1); ts <= total; ts++ {
			if before > 0 && ts >= before {
				continue
			}
			page = append(page, chatMessage{
				TS:                ts,
				PlatformMessageID: fmt.Sprintf("m%d", ts),
				SenderWxid:        "wxid_member",
				SenderName:        "member",
				Kind:              "text",
				Text:              "x",
			})
		}
		if len(page) > limit {
			page = page[len(page)-limit:]
		}
		_ = json.NewEncoder(w).Encode(chatResponse{Messages: page})
	}))
	defer server.Close()

	messages, err := fetchChatSince(http.DefaultClient, server.URL, "account", "chat", 100)
	if err != nil {
		t.Fatalf("fetchChatSince: %v", err)
	}
	// 第一页 351..650，第二页 52..351（重复 351 去重），到 52 后停止。
	if len(messages) != 599 {
		t.Fatalf("len = %d, want 599", len(messages))
	}
	if messages[0].TS != 52 || messages[len(messages)-1].TS != 650 {
		t.Errorf("range = %d..%d, want 52..650", messages[0].TS, messages[len(messages)-1].TS)
	}
	for i := 1; i < len(messages); i++ {
		if messages[i-1].TS > messages[i].TS {
			t.Fatalf("messages are not sorted at %d", i)
		}
	}
}

func TestPreferredNameNeverUsesRemark(t *testing.T) {
	message := chatMessage{
		SenderName: "备注名",
		SenderWxid: "wxid_abc",
	}
	if name := preferredName(message, nil, nil); name != "wxid_abc" {
		t.Errorf("fallback name = %q, want wxid_abc", name)
	}
	if name := preferredName(message, nil, map[string]string{"wxid_abc": "微信昵称"}); name != "微信昵称" {
		t.Errorf("nick_name = %q, want 微信昵称", name)
	}
	if name := preferredName(message, nil, map[string]string{"wxid_abc": "ㅤ"}); name != "wxid_abc" {
		t.Errorf("invisible nick_name = %q, want wxid_abc", name)
	}
	if name := preferredName(message, map[string]string{"wxid_abc": "群昵称"}, map[string]string{"wxid_abc": "微信昵称"}); name != "群昵称" {
		t.Errorf("group nickname = %q, want 群昵称", name)
	}
	if name := preferredName(message, map[string]string{"wxid_abc": "ㅤ"}, map[string]string{"wxid_abc": "微信昵称"}); name != "微信昵称" {
		t.Errorf("invisible group nickname = %q, want 微信昵称", name)
	}
}

func TestMarshalExportOneMessagePerLine(t *testing.T) {
	content, err := marshalExport("chat", 1, 2, []ExportMessage{
		{Index: 1, TS: 1, Sender: "甲", Kind: "text", Text: "第一行\n第二行"},
		{Index: 2, TS: 2, Sender: "乙", Kind: "image"},
	}, 2)
	if err != nil {
		t.Fatalf("marshalExport: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, `"total": 2`) || !strings.Contains(text, `"participants": 2`) {
		t.Errorf("missing stats:\n%s", text)
	}
	if got := strings.Count(text, "\n    {\"i\":"); got != 2 {
		t.Errorf("message lines = %d, want 2:\n%s", got, text)
	}
	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, text)
	}
	messages := parsed["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages))
	}
}

func TestChatMessageKeyPreference(t *testing.T) {
	if got := chatMessageKey(chatMessage{PlatformMessageID: "p1", ID: json.RawMessage("123")}); got != "p:p1" {
		t.Errorf("platform id key = %q", got)
	}
	if got := chatMessageKey(chatMessage{ID: json.RawMessage("123")}); got != "i:123" {
		t.Errorf("db id key = %q", got)
	}
	if got := chatMessageKey(chatMessage{TS: 1, SenderWxid: "wxid_x", Text: "hello"}); got != "t:1|wxid_x|hello" {
		t.Errorf("fallback key = %q", got)
	}
}
