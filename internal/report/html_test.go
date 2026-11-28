package report

import (
	"strings"
	"testing"
)

func testReport() Report {
	return Report{
		Version:          Version,
		Title:            "「测试群」24 小时话题 TOP10",
		ChatName:         "测试群",
		WindowStart:      "11-25 20:00",
		WindowEnd:        "11-26 20:00",
		MessageTotal:     100,
		ParticipantTotal: 10,
		TopicTotal:       1,
		Topics: []Topic{{
			Rank:       1,
			Title:      "话题一",
			Commentary: "一句话点评",
			Segments: []Segment{
				{Kind: "text", Text: "正文 "},
				{Kind: "person", Text: "甲"},
				{Kind: "text", Text: " 提到了 "},
				{Kind: "resource", Index: 1, URL: "https://example.com/a"},
			},
			Participants: []Participant{{Name: "甲", Count: 3}},
			MessageCount: 3,
		}},
		Resources: []Resource{{Index: 1, Title: "资源一", URL: "https://example.com/a"}},
	}
}

func TestHTMLRendersReport(t *testing.T) {
	result, err := HTML(testReport(), HTMLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text := string(result.HTML)
	for _, want := range []string{
		"<title>「测试群」24 小时话题 TOP10</title>",
		"测试群",
		"11-25 20:00",
		"话题一",
		"一句话点评",
		"甲",
		"https://example.com/a",
		"moment__avatar",
		"moment__comments",
		"moment__person-avatar",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestHTMLUsesTitleOverride(t *testing.T) {
	result, err := HTML(testReport(), HTMLOptions{Title: "自定义标题"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(result.HTML)
	if !strings.Contains(text, "<title>自定义标题</title>") {
		t.Error("title override missing")
	}
}

func TestHTMLMinimalTheme(t *testing.T) {
	result, err := HTML(testReport(), HTMLOptions{Theme: "minimal"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(result.HTML)
	if !strings.Contains(text, "masthead") || !strings.Contains(text, "topic__rank") {
		t.Error("minimal theme markup missing")
	}
	if result.Width != 390 {
		t.Errorf("width = %d, want 390", result.Width)
	}
}

func TestHTMLUnknownTheme(t *testing.T) {
	_, err := HTML(testReport(), HTMLOptions{Theme: "not-exist"})
	if err == nil || !strings.Contains(err.Error(), "未知主题") {
		t.Fatalf("err = %v, want unknown theme error", err)
	}
}
