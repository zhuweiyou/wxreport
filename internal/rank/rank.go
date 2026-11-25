// Package rank orders report topics by a deterministic engagement score.
//
// 有完整消息数据时使用「话轮模型」：
//
//	N_eff = 1 / Σ(p_i²)              p_i 是第 i 个参与者的话轮占比
//	θ     = 带文字话轮 / 总话轮
//	μ     = 密集话轮 / 总话轮（相邻话轮间隔 ≤ 5 分钟）
//	分数  = 0.45 × (N_eff / ΣN_eff) + 0.40 × θ × (话轮数 / Σ话轮数) + 0.15 × μ
//
// 没有消息数据时退化为按参与者消息数的粗略分数：
//
//	分数 = 有效参与人数 × log2(2 + 消息数)
package rank

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	turnGapSeconds  = 20 * 60 // 同一人连续消息超过 20 分钟算新话轮
	denseGapSeconds = 5 * 60  // 相邻话轮间隔不超过 5 分钟算密集讨论
)

var (
	topicHeadingRe     = regexp.MustCompile(`^##\s+(\d+)[.、]\s*(.*)$`)
	resourceHeadingRe  = regexp.MustCompile(`^##\s+资源列表\s*$`)
	participantLineRe  = regexp.MustCompile(`^参与者[：:]`)
	participantValueRe = regexp.MustCompile(`^参与者[：:]\s*(.*)$`)
	participantRe      = regexp.MustCompile(`^(.*?)[（(](\d+)[）)]\s*$`)
	footnoteDefRe      = regexp.MustCompile(`^\s*\[\^([^\]]+)\]:`)
	footnoteRefRe      = regexp.MustCompile(`\[\^([^\]\s]+)\]`)
	msgsCommentRe      = regexp.MustCompile(`<!--\s*msgs:\s*([0-9,\s-]+)\s*-->`)
	headerMessagesRe   = regexp.MustCompile(`(?m)^([-*]\s*消息总数：).*$`)
	headerPeopleRe     = regexp.MustCompile(`(?m)^([-*]\s*参与人数：).*$`)
	headerWindowRe     = regexp.MustCompile(`^[-*]\s*统计窗口：`)
)

// Message 是 wxreport export 输出的一条消息。
type Message struct {
	Index     int    `json:"i"`
	MID       string `json:"mid"`
	TS        int64  `json:"ts"`
	Sender    string `json:"sender"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Link      string `json:"link"`
	LinkTitle string `json:"link_title"`
}

// Export 是 wxreport export 的完整 JSON。
type Export struct {
	Chat         string    `json:"chat"`
	Start        int64     `json:"start"`
	End          int64     `json:"end"`
	Total        int       `json:"total"`
	Participants int       `json:"participants"`
	Messages     []Message `json:"messages"`
}

// Participant 是「参与者：」行里的一个昵称。
type Participant struct {
	Name  string
	Count int // 消息数（用于展示）
	Turns int // 话轮数（用于计算有效参与人数）
}

// Topic 是报告里的一个话题章节。
type Topic struct {
	Number       int
	Title        string
	Participants []Participant
	Messages     int
	Turns        int
	TextTurns    int
	DenseTurns   int
	Effective    float64
	Score        float64
	Kept         bool

	refs  []int
	block string
}

// Result 是一次排序的完整结果（按分数从高到低）。
type Result struct {
	Topics   []Topic
	Kept     int
	Total    int
	Mode     string // "turns" 或 "counts"
	Warnings []string
}

// Process 在没有消息明细时按参与者消息数排序（兼容旧报告）。
func Process(markdown string, top int) (string, Result, error) {
	return process(markdown, top, nil)
}

// ProcessWithMessages 用导出的消息明细计算话轮、参与广度和密集度后排序。
func ProcessWithMessages(markdown string, top int, export *Export) (string, Result, error) {
	if export == nil || len(export.Messages) == 0 {
		return "", Result{}, errors.New("消息导出为空，无法按话轮排序")
	}
	return process(markdown, top, export)
}

func process(markdown string, top int, export *Export) (string, Result, error) {
	doc, err := parse(markdown)
	if err != nil {
		return "", Result{}, err
	}

	result := Result{Mode: "counts"}
	if export != nil {
		result.Mode = "turns"
		warnings, err := scoreWithMessages(doc.topics, export)
		if err != nil {
			return "", Result{}, err
		}
		result.Warnings = warnings
		doc.header = rewriteHeader(doc.header, export.Total, export.Participants)
	} else if err := scoreWithCounts(doc.topics); err != nil {
		return "", Result{}, err
	}

	topics := doc.topics
	sort.SliceStable(topics, func(i, j int) bool {
		a, b := topics[i], topics[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Effective != b.Effective {
			return a.Effective > b.Effective
		}
		if a.Turns != b.Turns {
			return a.Turns > b.Turns
		}
		if a.Messages != b.Messages {
			return a.Messages > b.Messages
		}
		return a.Title < b.Title
	})

	keep := len(topics)
	if top > 0 && keep > top {
		keep = top
	}
	kept := make([]Topic, 0, keep)
	for i := range topics {
		topics[i].Kept = i < keep
		if topics[i].Kept {
			kept = append(kept, topics[i])
		}
	}

	result.Topics = topics
	result.Kept = keep
	result.Total = len(topics)
	return render(doc.header, kept, doc.footer), result, nil
}

type document struct {
	header string
	topics []Topic
	footer string
}

func parse(markdown string) (document, error) {
	text := strings.ReplaceAll(markdown, "\r\n", "\n")
	lines := strings.Split(text, "\n")

	var headingLines []int
	footerLine := -1
	for i, line := range lines {
		if resourceHeadingRe.MatchString(line) {
			footerLine = i
			break
		}
		if topicHeadingRe.MatchString(line) {
			headingLines = append(headingLines, i)
		}
	}
	if len(headingLines) == 0 {
		return document{}, errors.New("没有找到话题标题（形如 ## 1. 标题）")
	}

	topicsEnd := len(lines)
	if footerLine >= 0 {
		topicsEnd = footerLine
	}

	doc := document{
		header: strings.TrimRight(strings.Join(lines[:headingLines[0]], "\n"), "\n"),
	}
	for i, start := range headingLines {
		end := topicsEnd
		if i+1 < len(headingLines) {
			end = headingLines[i+1]
		}
		block := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
		block, refs, err := extractRefs(block)
		if err != nil {
			return document{}, err
		}

		match := topicHeadingRe.FindStringSubmatch(lines[start])
		number, _ := strconv.Atoi(match[1])
		title := strings.TrimSpace(match[2])
		// 行内 <!-- msgs --> 已从 block 中删除，标题以清理后的首行为准。
		if cleaned := strings.Split(block, "\n"); len(cleaned) > 0 {
			if heading := topicHeadingRe.FindStringSubmatch(cleaned[0]); heading != nil {
				title = strings.TrimSpace(heading[2])
			}
		}
		doc.topics = append(doc.topics, Topic{
			Number: number,
			Title:  title,
			refs:   refs,
			block:  trimSeparator(block),
		})
	}
	if footerLine >= 0 {
		doc.footer = strings.TrimSpace(strings.Join(lines[footerLine:], "\n"))
	}
	return doc, nil
}

// extractRefs 取出话题块里的 <!-- msgs: 1,2,3 --> 并删除注释本身。
// 注释可以单独一行，也可以跟在标题行后面。
func extractRefs(block string) (string, []int, error) {
	lines := strings.Split(block, "\n")
	kept := make([]string, 0, len(lines))
	seen := make(map[int]bool)
	var refs []int
	for _, line := range lines {
		match := msgsCommentRe.FindStringSubmatch(line)
		if match == nil {
			kept = append(kept, line)
			continue
		}
		raw := strings.ReplaceAll(match[1], "，", ",")
		for _, part := range strings.Split(raw, ",") {
			numbers, err := parseRefPart(part)
			if err != nil {
				return "", nil, err
			}
			for _, number := range numbers {
				if seen[number] {
					continue
				}
				seen[number] = true
				refs = append(refs, number)
			}
		}
		if cleaned := strings.TrimSpace(msgsCommentRe.ReplaceAllString(line, "")); cleaned != "" {
			kept = append(kept, cleaned)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n")), refs, nil
}

// parseRefPart 支持单个编号（12）和闭区间（12-20）。
func parseRefPart(part string) ([]int, error) {
	part = strings.TrimSpace(part)
	if part == "" {
		return nil, nil
	}
	if strings.Contains(part, "-") {
		bounds := strings.SplitN(part, "-", 2)
		start, errStart := strconv.Atoi(strings.TrimSpace(bounds[0]))
		end, errEnd := strconv.Atoi(strings.TrimSpace(bounds[1]))
		if errStart != nil || errEnd != nil || start <= 0 || end < start || end-start > 10000 {
			return nil, fmt.Errorf("无法解析消息编号 %q", part)
		}
		numbers := make([]int, 0, end-start+1)
		for number := start; number <= end; number++ {
			numbers = append(numbers, number)
		}
		return numbers, nil
	}
	number, err := strconv.Atoi(part)
	if err != nil || number <= 0 {
		return nil, fmt.Errorf("无法解析消息编号 %q", part)
	}
	return []int{number}, nil
}

func scoreWithCounts(topics []Topic) error {
	for i := range topics {
		participants, err := parseParticipants(topics[i].block)
		if err != nil {
			return fmt.Errorf("话题「%s」%w", topics[i].Title, err)
		}
		total := 0
		for _, participant := range participants {
			total += participant.Count
		}
		sumSquares := 0.0
		for _, participant := range participants {
			share := float64(participant.Count) / float64(total)
			sumSquares += share * share
		}
		topics[i].Participants = participants
		topics[i].Messages = total
		topics[i].Effective = 1 / sumSquares
		topics[i].Score = topics[i].Effective * math.Log2(2+float64(total))
	}
	return nil
}

func scoreWithMessages(topics []Topic, export *Export) ([]string, error) {
	byIndex := make(map[int]Message, len(export.Messages))
	for _, message := range export.Messages {
		if message.Index <= 0 {
			return nil, fmt.Errorf("消息导出里有无效编号 %d", message.Index)
		}
		byIndex[message.Index] = message
	}

	assigned := make(map[int]int)
	var warnings []string
	topicMessages := make([][]Message, len(topics))
	for i := range topics {
		if len(topics[i].refs) == 0 {
			return nil, fmt.Errorf("话题「%s」缺少 <!-- msgs: ... --> 消息编号", topics[i].Title)
		}
		messages := make([]Message, 0, len(topics[i].refs))
		for _, ref := range topics[i].refs {
			message, ok := byIndex[ref]
			if !ok {
				warnings = append(warnings, fmt.Sprintf("话题「%s」引用了不存在的消息 %d，已忽略", topics[i].Title, ref))
				continue
			}
			if owner, ok := assigned[ref]; ok {
				warnings = append(warnings, fmt.Sprintf("消息 %d 同时出现在话题「%s」和「%s」，按前者计算", ref, topics[owner].Title, topics[i].Title))
				continue
			}
			assigned[ref] = i
			messages = append(messages, message)
		}
		if len(messages) == 0 {
			return nil, fmt.Errorf("话题「%s」没有可用的消息", topics[i].Title)
		}
		topicMessages[i] = messages
	}

	// 话轮按全局时间线计算：同一人连续发言、且没有切换到别的话题，
	// 才算同一个话轮；跨话题插话会打断话轮。
	metricsList := computeTopicMetrics(topicMessages)
	for i := range topics {
		metrics := metricsList[i]
		topics[i].Participants = metrics.participants
		topics[i].Messages = len(topicMessages[i])
		topics[i].Turns = metrics.turns
		topics[i].TextTurns = metrics.textTurns
		topics[i].DenseTurns = metrics.denseTurns
		topics[i].Effective = metrics.effective
		topics[i].block = setParticipantLine(topics[i].block, participantLine(metrics.participants))
	}

	var sumEffective, sumTurns float64
	for _, topic := range topics {
		sumEffective += topic.Effective
		sumTurns += float64(topic.Turns)
	}
	for i := range topics {
		if sumEffective <= 0 || sumTurns <= 0 {
			topics[i].Score = 0
			continue
		}
		textRatio := 0.0
		denseRatio := 0.0
		if topics[i].Turns > 0 {
			textRatio = float64(topics[i].TextTurns) / float64(topics[i].Turns)
			denseRatio = float64(topics[i].DenseTurns) / float64(topics[i].Turns)
		}
		topics[i].Score = 0.45*(topics[i].Effective/sumEffective) +
			0.40*textRatio*(float64(topics[i].Turns)/sumTurns) +
			0.15*denseRatio
	}
	return warnings, nil
}

type metrics struct {
	participants []Participant
	turns        int
	textTurns    int
	denseTurns   int
	effective    float64
}

func computeTopicMetrics(topics [][]Message) []metrics {
	type item struct {
		message Message
		topic   int
	}
	type turn struct {
		topic   int
		sender  string
		startTS int64
		endTS   int64
		hasText bool
	}

	var all []item
	for topic, messages := range topics {
		for _, message := range messages {
			all = append(all, item{message: message, topic: topic})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].message.TS == all[j].message.TS {
			return all[i].message.Index < all[j].message.Index
		}
		return all[i].message.TS < all[j].message.TS
	})

	turns := make([]turn, 0, len(all))
	for _, entry := range all {
		hasText := strings.TrimSpace(entry.message.Text) != "" || strings.TrimSpace(entry.message.LinkTitle) != ""
		last := len(turns) - 1
		if last >= 0 &&
			turns[last].topic == entry.topic &&
			turns[last].sender == entry.message.Sender &&
			entry.message.TS-turns[last].endTS <= turnGapSeconds {
			turns[last].endTS = entry.message.TS
			turns[last].hasText = turns[last].hasText || hasText
			continue
		}
		turns = append(turns, turn{
			topic:   entry.topic,
			sender:  entry.message.Sender,
			startTS: entry.message.TS,
			endTS:   entry.message.TS,
			hasText: hasText,
		})
	}

	messageCounts := make([]map[string]int, len(topics))
	turnCounts := make([]map[string]int, len(topics))
	textTurns := make([]int, len(topics))
	denseTurns := make([]int, len(topics))
	lastTurnEnd := make([]int64, len(topics))
	hasTurn := make([]bool, len(topics))
	for i := range topics {
		messageCounts[i] = make(map[string]int)
		turnCounts[i] = make(map[string]int)
		for _, message := range topics[i] {
			messageCounts[i][message.Sender]++
		}
	}
	for _, item := range turns {
		turnCounts[item.topic][item.sender]++
		if item.hasText {
			textTurns[item.topic]++
		}
		if hasTurn[item.topic] && item.startTS-lastTurnEnd[item.topic] <= denseGapSeconds {
			denseTurns[item.topic]++
		}
		lastTurnEnd[item.topic] = item.endTS
		hasTurn[item.topic] = true
	}

	result := make([]metrics, len(topics))
	for i := range topics {
		participants := make([]Participant, 0, len(messageCounts[i]))
		for name, count := range messageCounts[i] {
			participants = append(participants, Participant{Name: name, Count: count, Turns: turnCounts[i][name]})
		}
		sort.Slice(participants, func(a, b int) bool {
			if participants[a].Count != participants[b].Count {
				return participants[a].Count > participants[b].Count
			}
			return participants[a].Name < participants[b].Name
		})

		totalTurns := 0
		for _, count := range turnCounts[i] {
			totalTurns += count
		}
		effective := 0.0
		if totalTurns > 0 {
			sumSquares := 0.0
			for _, count := range turnCounts[i] {
				share := float64(count) / float64(totalTurns)
				sumSquares += share * share
			}
			if sumSquares > 0 {
				effective = 1 / sumSquares
			}
		}

		result[i] = metrics{
			participants: participants,
			turns:        totalTurns,
			textTurns:    textTurns[i],
			denseTurns:   denseTurns[i],
			effective:    effective,
		}
	}
	return result
}

func parseParticipants(block string) ([]Participant, error) {
	for _, line := range strings.Split(block, "\n") {
		if !participantLineRe.MatchString(strings.TrimSpace(line)) {
			continue
		}
		match := participantValueRe.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		var participants []Participant
		for _, segment := range strings.Split(match[1], "、") {
			segment = strings.TrimSpace(segment)
			if segment == "" {
				continue
			}
			parsed := participantRe.FindStringSubmatch(segment)
			if parsed == nil {
				return nil, fmt.Errorf("无法解析参与者 %q", segment)
			}
			count, err := strconv.Atoi(parsed[2])
			if err != nil || count <= 0 {
				return nil, fmt.Errorf("参与者 %q 的消息数无效", segment)
			}
			name := strings.TrimSpace(parsed[1])
			if name == "" {
				return nil, fmt.Errorf("参与者 %q 缺少昵称", segment)
			}
			participants = append(participants, Participant{Name: name, Count: count})
		}
		if len(participants) == 0 {
			return nil, errors.New("参与者行为空（补充消息明细后可由 rank 自动生成）")
		}
		return participants, nil
	}
	return nil, errors.New("缺少「参与者：」行")
}

func participantLine(participants []Participant) string {
	parts := make([]string, 0, len(participants))
	for _, participant := range participants {
		parts = append(parts, fmt.Sprintf("%s(%d)", participant.Name, participant.Count))
	}
	return "参与者：" + strings.Join(parts, "、")
}

func setParticipantLine(block, line string) string {
	lines := strings.Split(block, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if participantLineRe.MatchString(strings.TrimSpace(lines[i])) {
			lines[i] = line
			return strings.Join(lines, "\n")
		}
	}
	block = strings.TrimRight(block, "\n")
	if block != "" {
		block += "\n\n"
	}
	return block + line
}

// trimSeparator 去掉话题块末尾 AI 可能加的分隔线，避免重排后分隔线错位。
func trimSeparator(block string) string {
	block = strings.TrimSpace(block)
	for strings.HasSuffix(block, "---") {
		block = strings.TrimSpace(strings.TrimSuffix(block, "---"))
	}
	return block
}

func rewriteHeader(header string, total, participants int) string {
	if total <= 0 && participants <= 0 {
		return header
	}
	replacedMessages := headerMessagesRe.MatchString(header)
	replacedPeople := headerPeopleRe.MatchString(header)
	if replacedMessages {
		header = headerMessagesRe.ReplaceAllString(header, fmt.Sprintf("${1}%d", total))
	}
	if replacedPeople {
		header = headerPeopleRe.ReplaceAllString(header, fmt.Sprintf("${1}%d", participants))
	}
	if replacedMessages && replacedPeople {
		return header
	}

	lines := strings.Split(header, "\n")
	inserted := false
	out := make([]string, 0, len(lines)+2)
	for _, line := range lines {
		out = append(out, line)
		if !inserted && headerWindowRe.MatchString(line) {
			if !replacedMessages {
				out = append(out, fmt.Sprintf("- 消息总数：%d", total))
			}
			if !replacedPeople {
				out = append(out, fmt.Sprintf("- 参与人数：%d", participants))
			}
			inserted = true
		}
	}
	if !inserted {
		if !replacedMessages {
			out = append(out, fmt.Sprintf("- 消息总数：%d", total))
		}
		if !replacedPeople {
			out = append(out, fmt.Sprintf("- 参与人数：%d", participants))
		}
	}
	return strings.Join(out, "\n")
}

func render(header string, kept []Topic, footer string) string {
	var builder strings.Builder
	header = strings.TrimSpace(header)
	if header != "" {
		builder.WriteString(header)
		builder.WriteString("\n\n")
	}
	for i, topic := range kept {
		builder.WriteString(renumber(topic.block, i+1))
		builder.WriteString("\n\n")
	}
	if footer != "" {
		builder.WriteString(stripUnusedFootnotes(footer, builder.String()))
		builder.WriteString("\n")
		return builder.String()
	}
	return strings.TrimRight(builder.String(), "\n") + "\n"
}

func renumber(block string, number int) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if match := topicHeadingRe.FindStringSubmatch(line); match != nil {
			lines[i] = fmt.Sprintf("## %d. %s", number, strings.TrimSpace(match[2]))
			break
		}
	}
	return strings.Join(lines, "\n")
}

// stripUnusedFootnotes 删除保留话题里不再引用的脚注定义。
// goldmark 渲染时会自动忽略未引用脚注，这里只是让 Markdown 源文件保持干净。
func stripUnusedFootnotes(footer, body string) string {
	referenced := make(map[string]bool)
	for _, line := range strings.Split(body, "\n") {
		if footnoteDefRe.MatchString(line) {
			continue
		}
		for _, match := range footnoteRefRe.FindAllStringSubmatch(line, -1) {
			referenced[match[1]] = true
		}
	}

	lines := strings.Split(footer, "\n")
	kept := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if match := footnoteDefRe.FindStringSubmatch(line); match != nil && !referenced[match[1]] {
			// 连同缩进的续行一起删掉。
			j := i + 1
			for j < len(lines) {
				next := lines[j]
				if strings.TrimSpace(next) == "" || !(strings.HasPrefix(next, " ") || strings.HasPrefix(next, "\t")) {
					break
				}
				j++
			}
			i = j - 1
			continue
		}
		kept = append(kept, line)
	}

	result := strings.Join(kept, "\n")
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}
