package rank

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestProcessReordersByEffectiveParticipants(t *testing.T) {
	markdown := `# 测试报告

- 统计窗口：2026-11-22 23:00 ~ 2026-11-23 23:00
- 消息总数：34

## 1. 刷屏话题

正文 A [^1]

参与者：甲(15)、乙(5)

## 2. 广泛话题

正文 B [^2]

参与者：甲(2)、乙(2)、丙(2)、丁(2)、戊(2)

## 3. 小话题

正文 C

参与者：甲(1)、乙(1)、丙(1)、丁(1)

## 资源列表

[^1]: 资源一 — [https://example.com/a](https://example.com/a)
[^2]: 资源二 — [https://example.com/b](https://example.com/b)
`

	output, result, err := Process(markdown, 10)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Total != 3 || result.Kept != 3 {
		t.Fatalf("result = %+v", result)
	}

	wantOrder := []string{"广泛话题", "小话题", "刷屏话题"}
	for i, want := range wantOrder {
		if result.Topics[i].Title != want {
			t.Errorf("topics[%d] = %q, want %q", i, result.Topics[i].Title, want)
		}
	}
	for i, want := range wantOrder {
		heading := fmt.Sprintf("## %d. %s", i+1, want)
		if !strings.Contains(output, heading) {
			t.Errorf("output missing %q:\n%s", heading, output)
		}
	}
	if strings.Index(output, "正文 B") > strings.Index(output, "正文 C") {
		t.Error("topic bodies did not move with their headings")
	}
	if !strings.Contains(output, "参与者：甲(15)、乙(5)") {
		t.Error("participant line was altered")
	}
	if got := result.Topics[0].Effective; got < 4.99 || got > 5.01 {
		t.Errorf("effective participants = %.2f, want 5", got)
	}
}

func TestProcessKeepsTopNAndDropsUnusedFootnotes(t *testing.T) {
	var sections []string
	for i := 1; i <= 11; i++ {
		sections = append(sections, fmt.Sprintf(
			"## %d. 话题%d\n\n正文 [^%d]\n\n参与者：甲(%d)、乙(%d)",
			i, i, i, i, i,
		))
	}
	markdown := "# 测试报告\n\n- 统计窗口：2026-11-22 23:00 ~ 2026-11-23 23:00\n\n" +
		strings.Join(sections, "\n\n") +
		"\n\n## 资源列表\n\n[^1]: 资源一\n[^2]: 资源二\n"

	output, result, err := Process(markdown, 10)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Total != 11 || result.Kept != 10 {
		t.Fatalf("total/kept = %d/%d, want 11/10", result.Total, result.Kept)
	}
	if result.Topics[10].Title != "话题1" || result.Topics[10].Kept {
		t.Errorf("lowest topic should be dropped, got %+v", result.Topics[10])
	}
	if strings.Contains(output, "## 1. 话题1\n") {
		t.Error("dropped topic still present")
	}
	if strings.Contains(output, "[^1]:") {
		t.Error("unused footnote definition was not removed")
	}
	if !strings.Contains(output, "[^2]: 资源二") {
		t.Error("referenced footnote definition was removed")
	}
	if got := strings.Count(output, "## "); got != 11 { // 10 topics + 资源列表
		t.Errorf("heading count = %d, want 11:\n%s", got, output)
	}

	again, _, err := Process(output, 10)
	if err != nil {
		t.Fatalf("second Process: %v", err)
	}
	if again != output {
		t.Errorf("Process is not idempotent:\n--- first ---\n%s\n--- second ---\n%s", output, again)
	}
}

func TestProcessKeepsAllWhenFewerThanTop(t *testing.T) {
	markdown := "# 报告\n\n## 1. 只有一个话题\n\n正文\n\n参与者：甲(3)\n"
	output, result, err := Process(markdown, 10)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Total != 1 || result.Kept != 1 {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(output, "## 1. 只有一个话题") {
		t.Errorf("output = %q", output)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Error("output should end with a newline")
	}
}

func TestProcessSupportsFullWidthParentheses(t *testing.T) {
	markdown := "# 报告\n\n## 1. 话题\n\n正文\n\n参与者：甲（2）、乙（1）\n"
	_, result, err := Process(markdown, 10)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Topics[0].Messages != 3 {
		t.Errorf("messages = %d, want 3", result.Topics[0].Messages)
	}
}

func TestProcessRejectsMissingParticipants(t *testing.T) {
	markdown := "# 报告\n\n## 1. 话题\n\n正文没有参与者行\n"
	if _, _, err := Process(markdown, 10); err == nil {
		t.Fatal("expected error for missing participant line")
	}
}

func TestProcessRejectsNoTopics(t *testing.T) {
	if _, _, err := Process("# 空报告\n\n没有话题。\n", 10); err == nil {
		t.Fatal("expected error for report without topics")
	}
}

func testExport() *Export {
	var messages []Message
	add := func(sender string, startTS int64, count int, gap int64) {
		for i := 0; i < count; i++ {
			messages = append(messages, Message{
				Index:  len(messages) + 1,
				TS:     startTS + int64(i)*gap,
				Sender: sender,
				Kind:   "text",
				Text:   "内容",
			})
		}
	}

	// 刷屏话题：甲连发 10 条，乙接 2 条。
	add("甲", 0, 10, 60)
	add("乙", 600, 2, 60)

	// 广泛话题：5 个人各连发 3 条。
	ts := int64(1000)
	for _, name := range []string{"甲", "乙", "丙", "丁", "戊"} {
		add(name, ts, 3, 60)
		ts += 180
	}

	// 小话题：3 个人各 1 条。
	ts = 3000
	for _, name := range []string{"甲", "乙", "丙"} {
		add(name, ts, 1, 0)
		ts += 60
	}

	return &Export{
		Chat:         "test@chatroom",
		Start:        0,
		End:          4000,
		Total:        len(messages),
		Participants: 5,
		Messages:     messages,
	}
}

func TestProcessWithMessagesUsesTurns(t *testing.T) {
	markdown := `# 测试报告

- 统计窗口：2026-11-22 23:00 ~ 2026-11-23 23:00
- 消息总数：0
- 参与人数：0

## 1. 刷屏话题
<!-- msgs: 1,2,3,4,5,6,7,8,9,10,11,12 -->

正文 A

参与者：

## 2. 广泛话题
<!-- msgs: 13,14,15,16,17,18,19,20,21,22,23,24,25,26,27 -->

正文 B [^1]

参与者：

## 3. 小话题
<!-- msgs: 28,29,30 -->

正文 C

参与者：

## 资源列表

[^1]: 资源一 — [https://example.com/a](https://example.com/a)
`

	output, result, err := ProcessWithMessages(markdown, 10, testExport())
	if err != nil {
		t.Fatalf("ProcessWithMessages: %v", err)
	}
	if result.Mode != "turns" || result.Total != 3 || result.Kept != 3 {
		t.Fatalf("result = %+v", result)
	}

	wantOrder := []string{"广泛话题", "小话题", "刷屏话题"}
	for i, want := range wantOrder {
		if result.Topics[i].Title != want {
			t.Errorf("topics[%d] = %q, want %q (score %.4f)", i, result.Topics[i].Title, want, result.Topics[i].Score)
		}
		if !strings.Contains(output, fmt.Sprintf("## %d. %s", i+1, want)) {
			t.Errorf("output missing renumbered heading %q", want)
		}
	}

	if strings.Contains(output, "<!-- msgs") {
		t.Error("message metadata comment was not removed")
	}
	if !strings.Contains(output, "参与者：丁(3)、丙(3)、乙(3)、戊(3)、甲(3)") {
		t.Errorf("broad topic participant line not generated:\n%s", output)
	}
	if !strings.Contains(output, "参与者：甲(10)、乙(2)") {
		t.Errorf("spam topic participant line not generated:\n%s", output)
	}
	if !strings.Contains(output, "- 消息总数：30") || !strings.Contains(output, "- 参与人数：5") {
		t.Errorf("header stats not rewritten:\n%s", output)
	}
	if !strings.Contains(output, "[^1]: 资源一") {
		t.Error("referenced footnote was removed")
	}

	broad := result.Topics[0]
	if broad.Turns != 5 || math.Abs(broad.Effective-5) > 1e-9 || broad.Messages != 15 {
		t.Errorf("broad metrics = turns %d effective %.4f messages %d", broad.Turns, broad.Effective, broad.Messages)
	}
	spam := result.Topics[2]
	if spam.Turns != 2 || math.Abs(spam.Effective-2) > 1e-9 {
		t.Errorf("spam metrics = turns %d effective %.4f, want 2/2", spam.Turns, spam.Effective)
	}
}

func TestProcessWithMessagesWarnsOnDuplicateAndUnknownRefs(t *testing.T) {
	export := &Export{
		Total:        2,
		Participants: 2,
		Messages: []Message{
			{Index: 1, TS: 0, Sender: "甲", Text: "a"},
			{Index: 2, TS: 60, Sender: "乙", Text: "b"},
		},
	}
	markdown := `# 报告

## 1. 话题一
<!-- msgs: 1 -->

正文

参与者：

## 2. 话题二
<!-- msgs: 1,2,99 -->

正文

参与者：
`
	_, result, err := ProcessWithMessages(markdown, 10, export)
	if err != nil {
		t.Fatalf("ProcessWithMessages: %v", err)
	}
	if len(result.Warnings) != 2 {
		t.Errorf("warnings = %v, want 2", result.Warnings)
	}
	for _, topic := range result.Topics {
		if topic.Title == "话题二" && topic.Messages != 1 {
			t.Errorf("话题二 messages = %d, want 1", topic.Messages)
		}
	}
}

func TestProcessWithMessagesRequiresMsgComment(t *testing.T) {
	export := &Export{
		Total:    1,
		Messages: []Message{{Index: 1, TS: 0, Sender: "甲", Text: "a"}},
	}
	markdown := "# 报告\n\n## 1. 话题\n\n正文\n\n参与者：\n"
	if _, _, err := ProcessWithMessages(markdown, 10, export); err == nil {
		t.Fatal("expected error for topic without msgs comment")
	}
}

func TestProcessWithMessagesInsertsMissingHeaderStats(t *testing.T) {
	export := &Export{
		Total:        1,
		Participants: 1,
		Messages:     []Message{{Index: 1, TS: 0, Sender: "甲", Text: "a"}},
	}
	markdown := "# 报告\n\n- 统计窗口：2026-11-22 23:00 ~ 2026-11-23 23:00\n\n## 1. 话题\n<!-- msgs: 1 -->\n\n正文\n\n参与者：\n"
	output, _, err := ProcessWithMessages(markdown, 10, export)
	if err != nil {
		t.Fatalf("ProcessWithMessages: %v", err)
	}
	window := strings.Index(output, "- 统计窗口")
	messages := strings.Index(output, "- 消息总数：1")
	people := strings.Index(output, "- 参与人数：1")
	if window < 0 || messages < window || people < messages {
		t.Errorf("header stats not inserted in order:\n%s", output)
	}
}

func TestComputeMetricsTurns(t *testing.T) {
	messages := []Message{
		{Index: 1, TS: 0, Sender: "甲", Text: "a"},
		{Index: 2, TS: 1200, Sender: "甲", Text: "b"}, // 恰好 20 分钟，仍算同一话轮
		{Index: 3, TS: 2401, Sender: "甲", Text: ""},  // 间隔 1201 秒，新话轮，无文字
		{Index: 4, TS: 2500, Sender: "乙", Text: "c"}, // 新话轮，距上一话轮 99 秒
	}
	m := computeTopicMetrics([][]Message{messages})[0]
	if m.turns != 3 {
		t.Errorf("turns = %d, want 3", m.turns)
	}
	if m.textTurns != 2 {
		t.Errorf("textTurns = %d, want 2", m.textTurns)
	}
	if m.denseTurns != 1 {
		t.Errorf("denseTurns = %d, want 1", m.denseTurns)
	}
	if math.Abs(m.effective-1.8) > 1e-9 {
		t.Errorf("effective = %.4f, want 1.8", m.effective)
	}
	if len(m.participants) != 2 || m.participants[0].Name != "甲" || m.participants[0].Count != 3 {
		t.Errorf("participants = %+v", m.participants)
	}
}

func TestComputeTopicMetricsBreaksTurnsAcrossTopics(t *testing.T) {
	topicA := []Message{
		{Index: 1, TS: 0, Sender: "甲", Text: "a1"},
		{Index: 3, TS: 120, Sender: "甲", Text: "a2"},
	}
	topicB := []Message{
		{Index: 2, TS: 60, Sender: "乙", Text: "b1"},
	}
	metrics := computeTopicMetrics([][]Message{topicA, topicB})
	if metrics[0].turns != 2 {
		t.Errorf("topic A turns = %d, want 2 (topic B interrupted)", metrics[0].turns)
	}
	if metrics[1].turns != 1 {
		t.Errorf("topic B turns = %d, want 1", metrics[1].turns)
	}
}

func TestProcessWithMessagesSupportsInlineComment(t *testing.T) {
	export := &Export{
		Total:        1,
		Participants: 1,
		Messages:     []Message{{Index: 1, TS: 0, Sender: "甲", Text: "a"}},
	}
	markdown := "# 报告\n\n## 1. 话题 <!-- msgs: 1 -->\n\n正文\n\n参与者：\n"
	output, result, err := ProcessWithMessages(markdown, 10, export)
	if err != nil {
		t.Fatalf("ProcessWithMessages: %v", err)
	}
	if result.Topics[0].Title != "话题" {
		t.Errorf("title = %q, want 话题", result.Topics[0].Title)
	}
	if strings.Contains(output, "<!--") {
		t.Errorf("inline comment not removed:\n%s", output)
	}
	if !strings.Contains(output, "## 1. 话题") {
		t.Errorf("heading missing:\n%s", output)
	}
	if !strings.Contains(output, "参与者：甲(1)") {
		t.Errorf("participant line missing:\n%s", output)
	}
}

func TestProcessWithMessagesSupportsRanges(t *testing.T) {
	export := &Export{
		Total:        3,
		Participants: 1,
		Messages: []Message{
			{Index: 1, TS: 0, Sender: "甲", Text: "a"},
			{Index: 2, TS: 60, Sender: "甲", Text: "b"},
			{Index: 3, TS: 120, Sender: "甲", Text: "c"},
		},
	}
	markdown := "# 报告\n\n## 1. 话题\n<!-- msgs: 1-3 -->\n\n正文\n\n参与者：\n"
	output, result, err := ProcessWithMessages(markdown, 10, export)
	if err != nil {
		t.Fatalf("ProcessWithMessages: %v", err)
	}
	if result.Topics[0].Messages != 3 || result.Topics[0].Turns != 1 {
		t.Errorf("messages/turns = %d/%d, want 3/1", result.Topics[0].Messages, result.Topics[0].Turns)
	}
	if !strings.Contains(output, "参与者：甲(3)") {
		t.Errorf("participant line = %s", output)
	}
}
