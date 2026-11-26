package report

import (
	"fmt"
	"strings"
	"testing"
	"time"

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

func TestBuildReportFromState(t *testing.T) {
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

	rep, warnings, err := Build(st, export, Options{ChatName: "测试群", Top: 10})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
	if rep.Title != "「测试群」24 小时话题 TOP10" {
		t.Errorf("title = %q", rep.Title)
	}
	if rep.MessageTotal != 3 || rep.ParticipantTotal != 3 {
		t.Errorf("stats = %d/%d", rep.MessageTotal, rep.ParticipantTotal)
	}
	if rep.TopicTotal != 2 || len(rep.Topics) != 2 {
		t.Fatalf("topics = %d/%d", rep.TopicTotal, len(rep.Topics))
	}
	// 两个话题都有话轮，话题一有 2 条消息，应排第一。
	if rep.Topics[0].Title != "话题一" {
		t.Errorf("first topic = %q", rep.Topics[0].Title)
	}
	if len(rep.Resources) != 1 || rep.Resources[0].Index != 1 {
		t.Fatalf("resources = %+v", rep.Resources)
	}

	first := rep.Topics[0]
	if len(first.Segments) == 0 {
		t.Fatal("missing summary segments")
	}
	var persons, resourceRefs int
	for _, segment := range first.Segments {
		switch segment.Kind {
		case "person":
			persons++
		case "resource":
			resourceRefs++
			if segment.Index != 1 || segment.URL != "https://example.com/a" {
				t.Errorf("resource segment = %+v", segment)
			}
		}
	}
	if persons != 2 || resourceRefs != 1 {
		t.Errorf("segments: persons=%d resourceRefs=%d", persons, resourceRefs)
	}
	if len(first.Participants) != 2 {
		t.Errorf("participants = %+v", first.Participants)
	}
}

func TestBuildNoParticipants(t *testing.T) {
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

	rep, _, err := Build(st, export, Options{
		ChatName:       "测试群",
		Top:            10,
		NoParticipants: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(rep.Topics) != 1 || len(rep.Topics[0].Participants) != 0 {
		t.Fatalf("participants should be omitted: %+v", rep.Topics)
	}
}

func TestBuildDeduplicatesResourcesAndHasNoLimit(t *testing.T) {
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

	rep, _, err := Build(st, export, Options{ChatName: "测试群", Top: 10})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(rep.Resources) != 12 {
		t.Fatalf("resources = %d, want 12", len(rep.Resources))
	}
	for i, resource := range rep.Resources {
		if resource.Index != i+1 {
			t.Errorf("resource[%d].Index = %d", i, resource.Index)
		}
	}
}

func TestBuildUsesLocation(t *testing.T) {
	st := &reportstate.State{
		Version:  reportstate.Version,
		ChatName: "测试群",
		Topics: []reportstate.Topic{{
			ID:         "t1",
			Title:      "话题",
			Summary:    "`甲` 说。",
			MessageIDs: []string{"m1"},
		}},
	}
	export := testExport(testMessage(1, "m1", 1000, "甲", "a"))
	loc := time.FixedZone("UTC+8", 8*3600)

	rep, _, err := Build(st, export, Options{Location: loc})
	if err != nil {
		t.Fatal(err)
	}
	if rep.WindowStart != "01-01 08:00" || rep.WindowEnd != "01-01 10:46" {
		t.Fatalf("window = %q ~ %q", rep.WindowStart, rep.WindowEnd)
	}
}

func TestBuildRejectsEmptyState(t *testing.T) {
	export := testExport(testMessage(1, "m1", 1000, "甲", "a"))
	if _, _, err := Build(&reportstate.State{Version: reportstate.Version}, export, Options{}); err == nil {
		t.Fatal("expected error for empty state")
	}
}
