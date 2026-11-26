package report

import (
	"fmt"
	"strings"
	"testing"

	"wxreport/internal/rank"
	reportstate "wxreport/internal/state"
)

func testMessage(index int, mid string, ts int64, sender, text string) rank.Message {
	return rank.Message{Index: index, MID: mid, TS: ts, Sender: sender, Kind: "text", Text: text}
}

func testExport(messages ...rank.Message) *rank.Export {
	senders := make(map[string]bool)
	for _, message := range messages {
		senders[message.Sender] = true
	}
	return &rank.Export{
		Chat:         "test@chatroom",
		Start:        0,
		End:          10000,
		Total:        len(messages),
		Participants: len(senders),
		Messages:     messages,
	}
}

func TestRenderReportFromState(t *testing.T) {
	st := &reportstate.State{
		Version:  reportstate.Version,
		ChatName: "测试群",
		Topics: []reportstate.Topic{
			{
				ID:         "t1",
				Title:      "话题一",
				Commentary: "一句话点评",
				Summary:    "`甲` 和 `乙` 讨论了资源 [[res:1]]。",
				MessageIDs: []string{"m1", "m2"},
				Resources:  []reportstate.Resource{{Title: "资源一", URL: "https://example.com/a"}},
			},
			{
				ID:         "t2",
				Title:      "话题二",
				Commentary: "另一句点评",
				Summary:    "`丙` 说了几句。",
				MessageIDs: []string{"m3"},
			},
		},
	}
	export := testExport(
		testMessage(1, "m1", 1000, "甲", "a"),
		testMessage(2, "m2", 1060, "乙", "b"),
		testMessage(3, "m3", 2000, "丙", "c"),
	)

	markdown, result, err := Render(st, export, Options{ChatName: "测试群", Top: 10})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{
		"# 「测试群」24 小时话题 TOP10",
		"- 消息总数：3",
		"- 参与人数：3",
		"## 1.",
		"[^1]: 资源一 — [https://example.com/a](https://example.com/a)",
		"参与者：",
	} {
		if !strings.Contains(markdown, want) {
			t.Errorf("missing %q:\n%s", want, markdown)
		}
	}
	if strings.Contains(markdown, "[[res:") || strings.Contains(markdown, "<!-- msgs") {
		t.Errorf("internal markers left in report:\n%s", markdown)
	}
	if result.Kept != 2 || len(result.Topics) != 2 {
		t.Errorf("rank result = %+v", result)
	}
}

func TestRenderNoParticipants(t *testing.T) {
	st := &reportstate.State{
		Version:  reportstate.Version,
		Chat:     "test@chatroom",
		ChatName: "测试群",
		Topics: []reportstate.Topic{{
			ID:         "t1",
			Title:      "话题一",
			Summary:    "`甲` 说了几句。",
			MessageIDs: []string{"m1"},
		}},
	}
	export := testExport(testMessage(1, "m1", 1000, "甲", "a"))

	markdown, _, err := Render(st, export, Options{
		ChatName:       "测试群",
		Top:            10,
		NoParticipants: true,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(markdown, "参与者：") {
		t.Fatalf("participants should be omitted:\n%s", markdown)
	}
}

func TestRenderDeduplicatesResourcesAndHasNoLimit(t *testing.T) {
	resources := make([]reportstate.Resource, 0, 12)
	markers := make([]string, 0, 12)
	for i := 1; i <= 12; i++ {
		resources = append(resources, reportstate.Resource{
			Title: fmt.Sprintf("资源%d", i),
			URL:   fmt.Sprintf("https://example.com/%d", i),
		})
		markers = append(markers, fmt.Sprintf("[[res:%d]]", i))
	}
	st := &reportstate.State{
		Version:  reportstate.Version,
		ChatName: "测试群",
		Topics: []reportstate.Topic{
			{
				ID:         "t1",
				Title:      "资源很多的话题",
				Commentary: "点评",
				Summary:    "摘要 " + strings.Join(markers, " "),
				MessageIDs: []string{"m1"},
				Resources:  resources,
			},
			{
				ID:         "t2",
				Title:      "重复资源",
				Commentary: "点评",
				Summary:    "重复引用 [[res:1]]",
				MessageIDs: []string{"m2"},
				Resources:  []reportstate.Resource{{Title: "资源1", URL: "https://example.com/1"}},
			},
		},
	}
	export := testExport(
		testMessage(1, "m1", 1000, "甲", "a"),
		testMessage(2, "m2", 1060, "乙", "b"),
	)
	markdown, _, err := Render(st, export, Options{ChatName: "测试群", Top: 10})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(markdown, "[[res:") {
		t.Errorf("resource markers left:\n%s", markdown)
	}
	if got := strings.Count(markdown, "[^"); got < 24 {
		t.Errorf("footnote refs+defs = %d, want >= 24", got)
	}
	defs := 0
	for _, line := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(line, "[^") && strings.Contains(line, "]:") {
			defs++
		}
	}
	if defs != 12 {
		t.Errorf("resource definitions = %d, want 12 (dedupe + no 10 cap)\n%s", defs, markdown)
	}
}

func TestRenderRejectsEmptyState(t *testing.T) {
	export := testExport(testMessage(1, "m1", 1000, "甲", "a"))
	if _, _, err := Render(&reportstate.State{Version: reportstate.Version}, export, Options{}); err == nil {
		t.Fatal("expected error for empty state")
	}
}
