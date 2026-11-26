package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wxreport/internal/rank"
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

func TestBuildPlanNewAndTailMessages(t *testing.T) {
	st := &State{
		Version:   Version,
		WindowEnd: 5000,
		Topics: []Topic{
			{ID: "t1", Title: "话题一", Summary: "摘要", MessageIDs: []string{"m1", "m2", "m3"}},
		},
	}
	export := testExport(
		testMessage(1, "m1", 1000, "甲", "a"),
		testMessage(2, "m2", 4000, "乙", "b"),
		testMessage(3, "m4", 4500, "丙", "tail"),
		testMessage(4, "m5", 6000, "丁", "new1"),
		testMessage(5, "m6", 7000, "戊", "new2"),
	)

	plan, err := BuildPlan(st, export, 6, "测试群")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Bootstrap {
		t.Error("should be incremental")
	}
	if len(plan.Topics) != 1 || plan.Topics[0].Active != 2 || plan.Topics[0].Expired != 1 {
		t.Errorf("topics = %+v", plan.Topics)
	}
	if len(plan.NewMessages) != 2 || plan.NewMessages[0].MID != "m5" || plan.NewMessages[1].MID != "m6" {
		t.Errorf("new messages = %+v", plan.NewMessages)
	}
	if len(plan.TailMessages) != 1 || plan.TailMessages[0].MID != "m4" {
		t.Errorf("tail messages = %+v", plan.TailMessages)
	}
	if len(plan.ActiveMessages) != 2 || plan.ActiveMessages[0].TopicID != "t1" || plan.ActiveMessages[1].MID != "m2" {
		t.Errorf("active messages = %+v", plan.ActiveMessages)
	}
	if plan.Topics[0].ActiveTruncated {
		t.Error("active messages should not be truncated")
	}
}

func TestBuildPlanBootstrap(t *testing.T) {
	export := testExport(
		testMessage(1, "m1", 1000, "甲", "a"),
		testMessage(2, "m2", 2000, "乙", "b"),
	)
	plan, err := BuildPlan(&State{Version: Version}, export, 6, "")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if !plan.Bootstrap || len(plan.NewMessages) != 2 || len(plan.TailMessages) != 0 {
		t.Errorf("plan = %+v", plan)
	}
}

func TestApplyUpdatesNewTopicsAndExpiry(t *testing.T) {
	st := &State{
		Version:   Version,
		Chat:      "test@chatroom",
		WindowEnd: 5000,
		Topics: []Topic{
			{ID: "t1", Title: "旧标题", Summary: "旧摘要", MessageIDs: []string{"m1", "m2", "gone"}},
			{ID: "t2", Title: "要删除", Summary: "删除", MessageIDs: []string{"m3"}},
		},
	}
	export := testExport(
		testMessage(1, "m1", 1000, "甲", "a"),
		testMessage(2, "m2", 2000, "乙", "b"),
		testMessage(3, "m3", 3000, "丙", "c"),
		testMessage(4, "m4", 4500, "丁", "tail"),
		testMessage(5, "m5", 6000, "戊", "new1"),
		testMessage(6, "m6", 7000, "己", "new2"),
	)
	plan, err := BuildPlan(st, export, 6, "测试群")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	update := &Update{
		Updates: []TopicUpdate{{
			ID:         "t1",
			Title:      "新标题",
			Commentary: "点评",
			Summary:    "新摘要 [[res:1]]",
			Resources:  []Resource{{Title: "资源", URL: "https://example.com/a"}},
		}},
		NewTopics: []NewTopic{{
			TempID:     "n1",
			Title:      "新话题",
			Commentary: "新点评",
			Summary:    "新话题摘要",
			MessageIDs: []string{"m5", "m6"},
		}},
		Assignments: []Assignment{{MID: "m4", TopicID: "t1"}},
		Drop:        []string{"t2"},
	}

	next, warnings, err := Apply(st, plan, update, export)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	if len(next.Topics) != 2 {
		t.Fatalf("topics = %+v", next.Topics)
	}
	t1 := next.Topics[0]
	if t1.ID != "t1" || t1.Title != "新标题" || len(t1.MessageIDs) != 3 {
		t.Errorf("t1 = %+v", t1)
	}
	gotIDs := strings.Join(t1.MessageIDs, ",")
	if gotIDs != "m1,m2,m4" {
		t.Errorf("t1 message ids = %s", gotIDs)
	}
	newTopic := next.Topics[1]
	if newTopic.Title != "新话题" || len(newTopic.MessageIDs) != 2 || newTopic.ID != "t3" {
		t.Errorf("new topic = %+v", newTopic)
	}
	if next.WindowEnd != plan.WindowEnd || next.ChatName != "测试群" {
		t.Errorf("state metadata = %+v", next)
	}
}

func TestApplyWarnsOnDuplicateAssignment(t *testing.T) {
	st := &State{
		Version:   Version,
		WindowEnd: 1000,
		Topics:    []Topic{{ID: "t1", Title: "话题", Summary: "摘要", MessageIDs: []string{"m1"}}},
	}
	export := testExport(
		testMessage(1, "m1", 500, "甲", "a"),
		testMessage(2, "m2", 600, "乙", "b"),
	)
	plan, err := BuildPlan(st, export, 6, "")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	update := &Update{
		Assignments: []Assignment{
			{MID: "m1", TopicID: "t1"},
			{MID: "m2", TopicID: "t1"},
		},
	}
	next, warnings, err := Apply(st, plan, update, export)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "已属于") {
		t.Errorf("warnings = %v", warnings)
	}
	if len(next.Topics[0].MessageIDs) != 2 {
		t.Errorf("message ids = %v", next.Topics[0].MessageIDs)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "topics.json")
	st := &State{
		Version:   Version,
		Chat:      "test@chatroom",
		ChatName:  "测试群",
		WindowEnd: 1234,
		Topics: []Topic{{
			ID:         "t1",
			Title:      "标题",
			Commentary: "点评",
			Summary:    "摘要",
			MessageIDs: []string{"m1"},
			Resources:  []Resource{{Title: "资源", URL: "https://example.com"}},
		}},
	}
	if err := Save(path, st); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ChatName != st.ChatName || loaded.WindowEnd != st.WindowEnd || len(loaded.Topics) != 1 {
		t.Errorf("loaded = %+v", loaded)
	}
	if fmt.Sprint(loaded.Topics[0].MessageIDs) != "[m1]" {
		t.Errorf("message ids = %v", loaded.Topics[0].MessageIDs)
	}
}

func TestApplyMergesResourcesByURL(t *testing.T) {
	st := &State{
		Version:   Version,
		WindowEnd: 1000,
		Topics: []Topic{{
			ID:         "t1",
			Title:      "标题",
			Summary:    "摘要",
			MessageIDs: []string{"m1"},
			Resources: []Resource{
				{Title: "旧A", URL: "https://example.com/a"},
				{Title: "旧B", URL: "https://example.com/b"},
			},
		}},
	}
	export := testExport(testMessage(1, "m1", 500, "甲", "a"))
	plan, err := BuildPlan(st, export, 6, "")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	update := &Update{
		Updates: []TopicUpdate{{
			ID:         "t1",
			Title:      "标题",
			Commentary: "点评",
			Summary:    "摘要[[res:1]][[res:2]]",
			Resources: []Resource{
				{Title: "新B", URL: "https://example.com/b"},
				{Title: "新C", URL: "https://example.com/c"},
			},
		}},
	}
	next, _, err := Apply(st, plan, update, export)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	resources := next.Topics[0].Resources
	if len(resources) != 3 {
		t.Fatalf("resources = %+v", resources)
	}
	if resources[0].Title != "新B" || resources[1].Title != "新C" || resources[2].Title != "旧A" {
		t.Errorf("resources order = %+v", resources)
	}
	if resources[0].URL != "https://example.com/b" {
		t.Errorf("dedupe failed: %+v", resources)
	}
}

func TestApplyEvictsTopicsWithNoMessagesInWindow(t *testing.T) {
	st := &State{
		Version:   Version,
		WindowEnd: 5000,
		Topics: []Topic{
			{ID: "old", Title: "全部过期", Summary: "旧", MessageIDs: []string{"gone1", "gone2"}},
			{ID: "keep", Title: "仍然活跃", Summary: "新", MessageIDs: []string{"m1"}},
		},
	}
	export := testExport(testMessage(1, "m1", 6000, "甲", "a"))
	plan, err := BuildPlan(st, export, 6, "")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Topics) != 1 || plan.Topics[0].ID != "keep" {
		t.Errorf("plan topics should omit fully expired topic: %+v", plan.Topics)
	}
	next, _, err := Apply(st, plan, &Update{}, export)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(next.Topics) != 1 || next.Topics[0].ID != "keep" {
		t.Fatalf("fully expired topic was not evicted: %+v", next.Topics)
	}
}

func TestWritePlanSerializesAllSections(t *testing.T) {
	plan := &Plan{
		Chat:              "test@chatroom",
		ChatName:          "测试群",
		WindowStart:       100,
		WindowEnd:         200,
		PreviousWindowEnd: 150,
		MessageTotal:      3,
		ParticipantTotal:  2,
		Topics: []PlanTopic{{
			ID: "t1", Title: "标题", Summary: "摘要", Active: 1, Expired: 1,
		}},
		ActiveMessages: []PlanActiveMessage{{
			TopicID:     "t1",
			PlanMessage: PlanMessage{MID: "m1", TS: 120, Sender: "甲", Kind: "text", Text: "a"},
		}},
		NewMessages:  []PlanMessage{{MID: "m2", TS: 180, Sender: "乙", Kind: "text", Text: "b"}},
		TailMessages: []PlanMessage{{MID: "m3", TS: 110, Sender: "丙", Kind: "text", Text: "c"}},
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := WritePlan(path, plan); err != nil {
		t.Fatalf("WritePlan: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("plan is not valid JSON: %v\n%s", err, raw)
	}
	for _, key := range []string{"chat", "topics", "active_messages", "new_messages", "tail_messages"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("missing key %q:\n%s", key, raw)
		}
	}
	if !strings.Contains(string(raw), `"topic_id":"t1"`) {
		t.Errorf("active message missing topic_id:\n%s", raw)
	}
}

func TestBuildPlanTruncatesActiveMessages(t *testing.T) {
	state := &State{Version: Version, WindowEnd: 10000}
	topic := Topic{ID: "t1", Title: "长话题", Summary: "摘要"}
	var exportMessages []rank.Message
	for i := 0; i < 130; i++ {
		mid := fmt.Sprintf("m%d", i)
		topic.MessageIDs = append(topic.MessageIDs, mid)
		exportMessages = append(exportMessages, testMessage(i+1, mid, int64(1000+i), "甲", "x"))
	}
	topic.MessageIDs = append(topic.MessageIDs, "gone")
	state.Topics = []Topic{topic}

	plan, err := BuildPlan(state, testExport(exportMessages...), 6, "")
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.ActiveMessages) != 120 {
		t.Fatalf("active messages = %d, want 120", len(plan.ActiveMessages))
	}
	if !plan.Topics[0].ActiveTruncated {
		t.Error("ActiveTruncated should be true")
	}
	if plan.ActiveMessages[0].MID != "m10" {
		t.Errorf("first active message = %s, want m10 (latest 120)", plan.ActiveMessages[0].MID)
	}
}

func TestApplyRejectsUnknownTopicUpdate(t *testing.T) {
	st := &State{Version: Version, WindowEnd: 1000, Topics: []Topic{{ID: "t1", MessageIDs: []string{"m1"}, Title: "a", Summary: "b"}}}
	export := testExport(testMessage(1, "m1", 500, "甲", "a"))
	plan, err := BuildPlan(st, export, 6, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = Apply(st, plan, &Update{Updates: []TopicUpdate{{ID: "missing", Title: "x", Summary: "y"}}}, export)
	if err == nil {
		t.Fatal("expected error for unknown topic update")
	}
}
