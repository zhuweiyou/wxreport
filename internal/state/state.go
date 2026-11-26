// Package state 持久化小时报告的增量话题状态。
//
// 每个话题保存稳定的 message_id（platformMessageId）、摘要和资源；
// 下一次运行只让模型处理新消息和上一小时未被归属的尾部消息，
// 然后把更新合并回原有话题，避免每小时重读完整的 24 小时消息。
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"wxreport/internal/fsutil"
	"wxreport/internal/rank"
)

const (
	Version                   = 1
	maxTopics                 = 30
	maxTailMessages           = 500
	maxActiveMessagesPerTopic = 120
)

// Resource 是话题引用的一个资源。
type Resource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Topic 是跨小时持久化的一个话题。
type Topic struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Commentary string     `json:"commentary"`
	Summary    string     `json:"summary"`
	MessageIDs []string   `json:"message_ids"`
	Resources  []Resource `json:"resources,omitempty"`
}

// State 是 data/topics.json 的完整内容。
type State struct {
	Version   int     `json:"version"`
	Chat      string  `json:"chat"`
	ChatName  string  `json:"chat_name,omitempty"`
	WindowEnd int64   `json:"window_end"`
	Topics    []Topic `json:"topics"`
}

// Load 读取状态；文件不存在时返回一个空状态。
func Load(path string) (*State, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &State{Version: Version}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取状态文件: %w", err)
	}
	var st State
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("解析状态文件 %s: %w", filepath.Base(path), err)
	}
	if st.Version == 0 {
		st.Version = Version
	}
	return &st, nil
}

// Save 原子写入状态文件。
func Save(path string, st *State) error {
	if st.Version == 0 {
		st.Version = Version
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, append(raw, '\n'), 0o644)
}

// PlanMessage 是给模型看的新消息/尾部消息。
type PlanMessage struct {
	MID       string `json:"mid"`
	TS        int64  `json:"ts"`
	Sender    string `json:"sender"`
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"`
	Link      string `json:"link,omitempty"`
	LinkTitle string `json:"link_title,omitempty"`
}

// PlanTopic 是上一轮话题的摘要信息。
type PlanTopic struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Commentary      string     `json:"commentary"`
	Summary         string     `json:"summary"`
	Resources       []Resource `json:"resources,omitempty"`
	Active          int        `json:"active"`
	Expired         int        `json:"expired"`
	Participants    int        `json:"participants"`
	ActiveTruncated bool       `json:"active_truncated,omitempty"`
}

// PlanActiveMessage 是带话题归属的当前窗口消息，供模型在淘汰后重写摘要。
type PlanActiveMessage struct {
	TopicID string `json:"topic_id"`
	PlanMessage
}

// Plan 是下一次增量总结的完整输入。
type Plan struct {
	Chat              string              `json:"chat"`
	ChatName          string              `json:"chat_name,omitempty"`
	WindowStart       int64               `json:"window_start"`
	WindowEnd         int64               `json:"window_end"`
	PreviousWindowEnd int64               `json:"previous_window_end"`
	MessageTotal      int                 `json:"message_total"`
	ParticipantTotal  int                 `json:"participant_total"`
	Bootstrap         bool                `json:"bootstrap"`
	Topics            []PlanTopic         `json:"topics"`
	ActiveMessages    []PlanActiveMessage `json:"active_messages"`
	NewMessages       []PlanMessage       `json:"new_messages"`
	TailMessages      []PlanMessage       `json:"tail_messages"`
}

// WritePlan 输出计划 JSON：topic 和消息各占一行，方便模型分段阅读。
func WritePlan(path string, plan *Plan) error {
	var builder strings.Builder
	builder.WriteString("{\n")
	writePlanField := func(key string, value any, comma bool) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		builder.WriteString("  ")
		keyJSON, _ := json.Marshal(key)
		builder.Write(keyJSON)
		builder.WriteString(": ")
		builder.Write(raw)
		if comma {
			builder.WriteString(",")
		}
		builder.WriteString("\n")
		return nil
	}

	if err := writePlanField("chat", plan.Chat, true); err != nil {
		return err
	}
	if err := writePlanField("chat_name", plan.ChatName, true); err != nil {
		return err
	}
	if err := writePlanField("window_start", plan.WindowStart, true); err != nil {
		return err
	}
	if err := writePlanField("window_end", plan.WindowEnd, true); err != nil {
		return err
	}
	if err := writePlanField("previous_window_end", plan.PreviousWindowEnd, true); err != nil {
		return err
	}
	if err := writePlanField("message_total", plan.MessageTotal, true); err != nil {
		return err
	}
	if err := writePlanField("participant_total", plan.ParticipantTotal, true); err != nil {
		return err
	}
	if err := writePlanField("bootstrap", plan.Bootstrap, true); err != nil {
		return err
	}

	writeList := func(name string, count int, writeItem func(int) error, comma bool) error {
		builder.WriteString("  ")
		keyJSON, _ := json.Marshal(name)
		builder.Write(keyJSON)
		builder.WriteString(": [\n")
		for i := 0; i < count; i++ {
			builder.WriteString("    ")
			if err := writeItem(i); err != nil {
				return err
			}
			if i+1 < count {
				builder.WriteString(",")
			}
			builder.WriteString("\n")
		}
		builder.WriteString("  ]")
		if comma {
			builder.WriteString(",")
		}
		builder.WriteString("\n")
		return nil
	}

	if err := writeList("topics", len(plan.Topics), func(i int) error {
		raw, err := json.Marshal(plan.Topics[i])
		if err != nil {
			return err
		}
		_, err = builder.Write(raw)
		return err
	}, true); err != nil {
		return err
	}
	if err := writeList("active_messages", len(plan.ActiveMessages), func(i int) error {
		raw, err := json.Marshal(plan.ActiveMessages[i])
		if err != nil {
			return err
		}
		_, err = builder.Write(raw)
		return err
	}, true); err != nil {
		return err
	}
	if err := writeList("new_messages", len(plan.NewMessages), func(i int) error {
		raw, err := json.Marshal(plan.NewMessages[i])
		if err != nil {
			return err
		}
		_, err = builder.Write(raw)
		return err
	}, true); err != nil {
		return err
	}
	if err := writeList("tail_messages", len(plan.TailMessages), func(i int) error {
		raw, err := json.Marshal(plan.TailMessages[i])
		if err != nil {
			return err
		}
		_, err = builder.Write(raw)
		return err
	}, false); err != nil {
		return err
	}
	builder.WriteString("}\n")

	return fsutil.WriteStringAtomic(path, builder.String(), 0o644)
}

// BuildPlan 计算新消息、尾部未归属消息，以及上一轮话题的活跃/过期情况。
func BuildPlan(st *State, export *rank.Export, tailHours int, chatName string) (*Plan, error) {
	if st == nil {
		st = &State{Version: Version}
	}
	if export == nil {
		return nil, errors.New("消息导出为空")
	}
	if tailHours <= 0 {
		tailHours = 6
	}

	byMID := make(map[string]rank.Message, len(export.Messages))
	for _, message := range export.Messages {
		if message.MID == "" {
			return nil, fmt.Errorf("消息缺少稳定 mid（index=%d）", message.Index)
		}
		byMID[message.MID] = message
	}

	assigned := make(map[string]bool)
	for _, topic := range st.Topics {
		for _, mid := range topic.MessageIDs {
			assigned[mid] = true
		}
	}

	bootstrap := st.WindowEnd == 0
	plan := &Plan{
		Chat:              st.Chat,
		ChatName:          chatName,
		WindowStart:       export.Start,
		WindowEnd:         export.End,
		PreviousWindowEnd: st.WindowEnd,
		MessageTotal:      export.Total,
		ParticipantTotal:  export.Participants,
		Bootstrap:         bootstrap,
	}
	if plan.Chat == "" {
		plan.Chat = export.Chat
	}
	if plan.ChatName == "" {
		plan.ChatName = st.ChatName
	}
	if plan.ChatName == "" {
		plan.ChatName = plan.Chat
	}

	tailStart := export.End - int64(tailHours)*3600
	for _, topic := range st.Topics {
		active := 0
		expired := 0
		senders := make(map[string]bool)
		var activeMessages []rank.Message
		for _, mid := range topic.MessageIDs {
			message, ok := byMID[mid]
			if !ok {
				expired++
				continue
			}
			active++
			senders[message.Sender] = true
			activeMessages = append(activeMessages, message)
		}
		if active == 0 {
			continue
		}

		truncated := false
		if expired > 0 {
			sort.SliceStable(activeMessages, func(i, j int) bool {
				if activeMessages[i].TS == activeMessages[j].TS {
					return activeMessages[i].Index < activeMessages[j].Index
				}
				return activeMessages[i].TS < activeMessages[j].TS
			})
			if len(activeMessages) > maxActiveMessagesPerTopic {
				activeMessages = activeMessages[len(activeMessages)-maxActiveMessagesPerTopic:]
				truncated = true
			}
			for _, message := range activeMessages {
				plan.ActiveMessages = append(plan.ActiveMessages, PlanActiveMessage{
					TopicID:     topic.ID,
					PlanMessage: planMessage(message),
				})
			}
		}

		plan.Topics = append(plan.Topics, PlanTopic{
			ID:              topic.ID,
			Title:           topic.Title,
			Commentary:      topic.Commentary,
			Summary:         topic.Summary,
			Resources:       topic.Resources,
			Active:          active,
			Expired:         expired,
			Participants:    len(senders),
			ActiveTruncated: truncated,
		})
	}

	for _, message := range export.Messages {
		item := PlanMessage{
			MID:       message.MID,
			TS:        message.TS,
			Sender:    message.Sender,
			Kind:      message.Kind,
			Text:      message.Text,
			Link:      message.Link,
			LinkTitle: message.LinkTitle,
		}
		if bootstrap || message.TS > st.WindowEnd {
			plan.NewMessages = append(plan.NewMessages, item)
			continue
		}
		if !assigned[message.MID] && message.TS >= tailStart {
			plan.TailMessages = append(plan.TailMessages, item)
		}
	}

	sort.SliceStable(plan.NewMessages, func(i, j int) bool {
		return plan.NewMessages[i].TS < plan.NewMessages[j].TS
	})
	sort.SliceStable(plan.TailMessages, func(i, j int) bool {
		return plan.TailMessages[i].TS < plan.TailMessages[j].TS
	})
	if len(plan.TailMessages) > maxTailMessages {
		plan.TailMessages = plan.TailMessages[len(plan.TailMessages)-maxTailMessages:]
	}
	return plan, nil
}

// Assignment 把一条新消息/尾部消息归入已有话题。
type Assignment struct {
	MID     string `json:"mid"`
	TopicID string `json:"topic_id"`
}

// TopicUpdate 是已有话题的完整更新。
type TopicUpdate struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Commentary string     `json:"commentary"`
	Summary    string     `json:"summary"`
	Resources  []Resource `json:"resources"`
}

// NewTopic 是模型新建的话题；TempID 只在本次更新文件内使用。
type NewTopic struct {
	TempID     string     `json:"temp_id"`
	Title      string     `json:"title"`
	Commentary string     `json:"commentary"`
	Summary    string     `json:"summary"`
	MessageIDs []string   `json:"message_ids"`
	Resources  []Resource `json:"resources"`
}

// Update 是模型输出的差异。
type Update struct {
	Assignments []Assignment  `json:"assignments"`
	Updates     []TopicUpdate `json:"updates"`
	NewTopics   []NewTopic    `json:"new_topics"`
	Drop        []string      `json:"drop"`
}

// Apply 把模型的差异合并进状态，并清理过期消息。
func Apply(st *State, plan *Plan, update *Update, export *rank.Export) (*State, []string, error) {
	if st == nil || plan == nil || update == nil || export == nil {
		return nil, nil, errors.New("state/plan/update/export 不能为空")
	}
	byMID := make(map[string]rank.Message, len(export.Messages))
	for _, message := range export.Messages {
		byMID[message.MID] = message
	}

	next := cloneState(st)
	maxID := maxTopicID(next.Topics)
	topicIndex := make(map[string]int, len(next.Topics))
	for i := range next.Topics {
		topicIndex[next.Topics[i].ID] = i
	}

	var warnings []string
	for _, updateTopic := range update.Updates {
		index, ok := topicIndex[updateTopic.ID]
		if !ok {
			return nil, nil, fmt.Errorf("更新不存在的话题 %s", updateTopic.ID)
		}
		if strings.TrimSpace(updateTopic.Title) == "" || strings.TrimSpace(updateTopic.Summary) == "" {
			return nil, nil, fmt.Errorf("话题 %s 的标题或摘要为空", updateTopic.ID)
		}
		next.Topics[index].Title = updateTopic.Title
		next.Topics[index].Commentary = updateTopic.Commentary
		next.Topics[index].Summary = updateTopic.Summary
		// 更新里的资源排在最前，保证摘要里的 [[res:n]] 序号不变；
		// 上一轮已有、但这次没重复列出的资源追加在后面，避免资源丢失。
		next.Topics[index].Resources = mergeResources(updateTopic.Resources, next.Topics[index].Resources)
	}

	for _, id := range update.Drop {
		if _, ok := topicIndex[id]; !ok {
			warnings = append(warnings, fmt.Sprintf("忽略不存在的 drop 话题 %s", id))
		}
	}
	if len(update.Drop) > 0 {
		dropped := make(map[string]bool, len(update.Drop))
		for _, id := range update.Drop {
			dropped[id] = true
		}
		kept := next.Topics[:0]
		for _, topic := range next.Topics {
			if dropped[topic.ID] {
				continue
			}
			kept = append(kept, topic)
		}
		next.Topics = kept
		topicIndex = make(map[string]int, len(next.Topics))
		for i := range next.Topics {
			topicIndex[next.Topics[i].ID] = i
		}
	}

	assigned := make(map[string]string)
	for _, topic := range next.Topics {
		for _, mid := range topic.MessageIDs {
			assigned[mid] = topic.ID
		}
	}

	tempToID := make(map[string]string, len(update.NewTopics))
	for _, newTopic := range update.NewTopics {
		if strings.TrimSpace(newTopic.Title) == "" || strings.TrimSpace(newTopic.Summary) == "" {
			return nil, nil, errors.New("新话题的标题或摘要为空")
		}
		if newTopic.TempID != "" {
			if _, exists := topicIndex[newTopic.TempID]; exists {
				return nil, nil, fmt.Errorf("新话题临时 ID %s 与已有话题冲突", newTopic.TempID)
			}
			if _, exists := tempToID[newTopic.TempID]; exists {
				return nil, nil, fmt.Errorf("新话题临时 ID %s 重复", newTopic.TempID)
			}
		}
		var mids []string
		for _, mid := range newTopic.MessageIDs {
			if _, ok := byMID[mid]; !ok {
				warnings = append(warnings, fmt.Sprintf("新话题「%s」引用了不存在的消息 %s，已忽略", newTopic.Title, mid))
				continue
			}
			if owner, ok := assigned[mid]; ok {
				warnings = append(warnings, fmt.Sprintf("消息 %s 已属于话题 %s，新话题「%s」重复引用已忽略", mid, owner, newTopic.Title))
				continue
			}
			assigned[mid] = newTopic.TempID
			mids = append(mids, mid)
		}
		if len(mids) == 0 {
			warnings = append(warnings, fmt.Sprintf("新话题「%s」没有有效消息，已丢弃", newTopic.Title))
			continue
		}
		maxID++
		id := fmt.Sprintf("t%d", maxID)
		if newTopic.TempID != "" {
			tempToID[newTopic.TempID] = id
		}
		next.Topics = append(next.Topics, Topic{
			ID:         id,
			Title:      newTopic.Title,
			Commentary: newTopic.Commentary,
			Summary:    newTopic.Summary,
			MessageIDs: mids,
			Resources:  mergeResources(newTopic.Resources, nil),
		})
		topicIndex[id] = len(next.Topics) - 1
		// assigned 里先把临时 ID 改成正式 ID
		for _, mid := range mids {
			assigned[mid] = id
		}
	}

	for _, assignment := range update.Assignments {
		if _, ok := byMID[assignment.MID]; !ok {
			warnings = append(warnings, fmt.Sprintf("归属消息 %s 不在当前导出里，已忽略", assignment.MID))
			continue
		}
		topicID := assignment.TopicID
		if mapped, ok := tempToID[topicID]; ok {
			topicID = mapped
		}
		index, ok := topicIndex[topicID]
		if !ok {
			return nil, nil, fmt.Errorf("归属到不存在的话题 %s", assignment.TopicID)
		}
		if owner, ok := assigned[assignment.MID]; ok {
			warnings = append(warnings, fmt.Sprintf("消息 %s 已属于话题 %s，重复归属到 %s 已忽略", assignment.MID, owner, topicID))
			continue
		}
		next.Topics[index].MessageIDs = append(next.Topics[index].MessageIDs, assignment.MID)
		assigned[assignment.MID] = topicID
	}

	for i := range next.Topics {
		seen := make(map[string]bool)
		kept := next.Topics[i].MessageIDs[:0]
		for _, mid := range next.Topics[i].MessageIDs {
			if _, ok := byMID[mid]; !ok {
				continue
			}
			if seen[mid] {
				continue
			}
			seen[mid] = true
			kept = append(kept, mid)
		}
		next.Topics[i].MessageIDs = kept
	}

	keptTopics := next.Topics[:0]
	for _, topic := range next.Topics {
		if len(topic.MessageIDs) > 0 {
			keptTopics = append(keptTopics, topic)
		}
	}
	next.Topics = keptTopics

	if len(next.Topics) > maxTopics {
		sort.SliceStable(next.Topics, func(i, j int) bool {
			return len(next.Topics[i].MessageIDs) > len(next.Topics[j].MessageIDs)
		})
		warnings = append(warnings, fmt.Sprintf("活跃话题超过 %d 个，保留消息最多的 %d 个", maxTopics, maxTopics))
		next.Topics = next.Topics[:maxTopics]
	}

	next.Version = Version
	next.Chat = plan.Chat
	if next.Chat == "" {
		next.Chat = export.Chat
	}
	if plan.ChatName != "" {
		next.ChatName = plan.ChatName
	}
	next.WindowEnd = plan.WindowEnd
	return next, warnings, nil
}

// mergeResources 按 URL 去重并累加资源；primary 的顺序保持不变，
// 这样模型摘要里的 [[res:n]] 不会被旧资源挤乱。
func mergeResources(primary, existing []Resource) []Resource {
	seen := make(map[string]bool)
	merged := make([]Resource, 0, len(primary)+len(existing))
	add := func(resource Resource) {
		url := strings.TrimSpace(resource.URL)
		if url == "" {
			return
		}
		if seen[url] {
			return
		}
		seen[url] = true
		merged = append(merged, resource)
	}
	for _, resource := range primary {
		add(resource)
	}
	for _, resource := range existing {
		add(resource)
	}
	return merged
}

func planMessage(message rank.Message) PlanMessage {
	return PlanMessage{
		MID:       message.MID,
		TS:        message.TS,
		Sender:    message.Sender,
		Kind:      message.Kind,
		Text:      message.Text,
		Link:      message.Link,
		LinkTitle: message.LinkTitle,
	}
}

func cloneState(st *State) *State {
	next := *st
	next.Topics = make([]Topic, len(st.Topics))
	for i, topic := range st.Topics {
		next.Topics[i] = topic
		next.Topics[i].MessageIDs = append([]string(nil), topic.MessageIDs...)
		next.Topics[i].Resources = append([]Resource(nil), topic.Resources...)
	}
	return &next
}

func maxTopicID(topics []Topic) int {
	max := 0
	for _, topic := range topics {
		if strings.HasPrefix(topic.ID, "t") {
			if number, err := strconv.Atoi(strings.TrimPrefix(topic.ID, "t")); err == nil && number > max {
				max = number
			}
		}
	}
	return max
}
