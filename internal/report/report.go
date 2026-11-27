// Package report 把增量状态和消息导出渲染成结构化报告。
package report

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"wxreport/internal/rank"
	reportstate "wxreport/internal/state"
)

// Version 是报告 JSON 存档的结构版本。
const Version = 1

// Report 是一篇完整报告，也是 data/reports/<slug>.json 的内容。
type Report struct {
	Version          int        `json:"version"`
	Title            string     `json:"title"`
	ChatName         string     `json:"chat_name"`
	WindowStart      string     `json:"window_start"`
	WindowEnd        string     `json:"window_end"`
	MessageTotal     int        `json:"message_total"`
	ParticipantTotal int        `json:"participant_total"`
	TopicTotal       int        `json:"topic_total"`
	Topics           []Topic    `json:"topics"`
	Resources        []Resource `json:"resources"`
}

// Topic 是报告里的一个话题。
type Topic struct {
	Rank         int           `json:"rank"`
	Title        string        `json:"title"`
	Commentary   string        `json:"commentary,omitempty"`
	Segments     []Segment     `json:"segments"`
	Participants []Participant `json:"participants,omitempty"`
	Resources    []Resource    `json:"resources,omitempty"`
	MessageCount int           `json:"message_count"`
	Score        float64       `json:"score"`
	// Initial 是话题消息最多者昵称的首字（大写），作为头像的底层占位。
	Initial string `json:"initial,omitempty"`
	// AvatarURL 是话题中消息最多者的头像地址（SIWX 本地接口）。
	AvatarURL string `json:"avatar_url,omitempty"`
	// AvatarWxid 是头像对应的成员 wxid，仅用于本地拉取，不写入报告 JSON。
	AvatarWxid string `json:"-"`
	// AvatarData 是头像的 data URI，仅用于截图 HTML，不写入报告 JSON。
	AvatarData string `json:"-"`
}

// Segment 是摘要里的一段内容：普通文本、人名或资源引用。
type Segment struct {
	Kind  string `json:"kind"` // text / person / resource
	Text  string `json:"text,omitempty"`
	Index int    `json:"index,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Participant 是话题参与者及其消息数。
type Participant struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	// Initial 是昵称首字，头像缺失时作为占位。
	Initial string `json:"initial,omitempty"`
	// AvatarURL 是该参与者的头像地址。
	AvatarURL string `json:"avatar_url,omitempty"`
	// AvatarWxid 仅用于本地拉取头像，不写入报告 JSON。
	AvatarWxid string `json:"-"`
	// AvatarData 是头像 data URI，仅用于截图 HTML。
	AvatarData string `json:"-"`
}

// Resource 是全局去重编号后的资源。
type Resource struct {
	Index int    `json:"index"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Options 控制报告生成。
type Options struct {
	ChatName       string
	Top            int
	NoParticipants bool
	Location       *time.Location
	// AvatarBaseURL 是 SIWX 本地接口地址；为空时不生成头像地址。
	AvatarBaseURL string
	// Account 是头像接口需要的账号 wxid。
	Account string
}

var summaryTokenRe = regexp.MustCompile("`([^`]+)`|\\[\\[res:(\\d+)\\]\\]")

// Build 把状态和当前 24h 导出渲染成结构化报告。
func Build(st *reportstate.State, export *rank.Export, opts Options) (Report, []string, error) {
	if st == nil || export == nil {
		return Report{}, nil, errors.New("state/export 不能为空")
	}
	if opts.Top <= 0 {
		opts.Top = 10
	}

	byMID := make(map[string]rank.Message, len(export.Messages))
	for _, message := range export.Messages {
		byMID[message.MID] = message
	}

	chatName := opts.ChatName
	if chatName == "" {
		chatName = st.ChatName
	}
	if chatName == "" {
		chatName = st.Chat
	}

	topicByID := make(map[string]*reportstate.Topic, len(st.Topics))
	inputs := make([]rank.InputTopic, 0, len(st.Topics))
	for i := range st.Topics {
		topic := &st.Topics[i]
		indices := make([]int, 0, len(topic.MessageIDs))
		for _, mid := range topic.MessageIDs {
			message, ok := byMID[mid]
			if !ok {
				continue
			}
			indices = append(indices, message.Index)
		}
		if len(indices) == 0 {
			continue
		}
		topicByID[topic.ID] = topic
		inputs = append(inputs, rank.InputTopic{
			ID:             topic.ID,
			Title:          topic.Title,
			MessageIndices: indices,
		})
	}
	if len(inputs) == 0 {
		return Report{}, nil, errors.New("状态里没有可渲染的话题")
	}

	scored, warnings, err := rank.Score(inputs, export)
	if err != nil {
		return Report{}, nil, err
	}

	resourceNumbers := make(map[string]int)
	resourceOrder := make([]Resource, 0)
	number := func(resource reportstate.Resource) int {
		if index, ok := resourceNumbers[resource.URL]; ok {
			return index
		}
		resourceOrder = append(resourceOrder, Resource{
			Index: len(resourceOrder) + 1,
			Title: resource.Title,
			URL:   resource.URL,
		})
		index := len(resourceOrder)
		resourceNumbers[resource.URL] = index
		return index
	}

	rep := Report{
		Version:          Version,
		Title:            fmt.Sprintf("「%s」24 小时话题 TOP10", chatName),
		ChatName:         chatName,
		WindowStart:      formatTime(export.Start, opts.Location),
		WindowEnd:        formatTime(export.End, opts.Location),
		MessageTotal:     export.Total,
		ParticipantTotal: export.Participants,
	}

	for i, item := range scored {
		if i >= opts.Top {
			break
		}
		source := topicByID[item.ID]
		if source == nil {
			continue
		}
		segments, segWarnings := parseSummary(source.Summary, source.Resources, number)
		warnings = append(warnings, segWarnings...)

		topic := Topic{
			Rank:         i + 1,
			Title:        source.Title,
			Commentary:   source.Commentary,
			Segments:     segments,
			MessageCount: item.Messages,
			Score:        item.Score,
		}
		topName := ""
		if len(item.Participants) > 0 {
			topName = item.Participants[0].Name
		}
		topic.Initial = "?"
		if topName != "" {
			topic.Initial = avatarInitial(topName)
		}
		if opts.AvatarBaseURL != "" && topName != "" {
			if wxid := topWxid(topName, source.MessageIDs, byMID); wxid != "" {
				topic.AvatarWxid = wxid
				topic.AvatarURL = avatarURL(opts.AvatarBaseURL, opts.Account, wxid)
			}
		}
		if !opts.NoParticipants {
			for _, participant := range item.Participants {
				entry := Participant{
					Name:    participant.Name,
					Count:   participant.Count,
					Initial: avatarInitial(participant.Name),
				}
				if opts.AvatarBaseURL != "" {
					entry.AvatarWxid = topWxid(participant.Name, source.MessageIDs, byMID)
					entry.AvatarURL = avatarURL(opts.AvatarBaseURL, opts.Account, entry.AvatarWxid)
				}
				topic.Participants = append(topic.Participants, entry)
			}
		}
		rep.Topics = append(rep.Topics, topic)
	}
	rep.TopicTotal = len(rep.Topics)
	rep.Resources = resourceOrder
	return rep, warnings, nil
}

// avatarInitial 取昵称的第一个字（字素簇）并转大写。
// 组合符号、变体选择符、肤色、ZWJ 序列和国旗修饰会一并保留，避免把 emoji 截坏。
func avatarInitial(name string) string {
	runes := []rune(strings.TrimSpace(name))
	i := 0
	for i < len(runes) && (unicode.IsSpace(runes[i]) || unicode.Is(unicode.Cf, runes[i]) || isBlankFiller(runes[i])) {
		i++
	}
	if i >= len(runes) {
		return "?"
	}
	end := i + 1
	for end < len(runes) {
		prev := runes[end-1]
		cur := runes[end]
		switch {
		case prev == 0x200D || cur == 0x200D: // ZWJ 序列
			end++
		case isCombiningRune(cur) || cur == 0xFE0E || cur == 0xFE0F || cur == 0x20E3:
			end++
		case cur >= 0x1F3FB && cur <= 0x1F3FF: // 肤色修饰
			end++
		case cur >= 0x1F1E6 && cur <= 0x1F1FF: // 国旗的第二个区域指示符
			end++
		case cur >= 0xE0020 && cur <= 0xE007F: // 标签序列
			end++
		default:
			return strings.ToUpper(string(runes[i:end]))
		}
	}
	return strings.ToUpper(string(runes[i:end]))
}

func isCombiningRune(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Mc, r)
}

// isBlankFiller 是昵称里常见的不可见占位字符（盲文空白、韩文填充符等）。
func isBlankFiller(r rune) bool {
	switch r {
	case 0x2800, 0x3164, 0xFFA0, 0x115F, 0x1160:
		return true
	}
	return false
}

// avatarURL 拼接 SIWX 本地头像接口地址。
func avatarURL(base, account, wxid string) string {
	if base == "" || wxid == "" {
		return ""
	}
	query := url.Values{}
	query.Set("account", account)
	query.Set("username", wxid)
	return strings.TrimRight(base, "/") + "/api/chat/avatar?" + query.Encode()
}

// topWxid 返回话题内指定昵称（参与者列表第一个）对应的 wxid。
// 同名可能对应多个 wxid，取消息最多的那个；相同条数按 wxid 字典序取小，保证稳定。
func topWxid(name string, messageIDs []string, byMID map[string]rank.Message) string {
	counts := make(map[string]int, len(messageIDs))
	for _, mid := range messageIDs {
		message, ok := byMID[mid]
		if !ok || message.Wxid == "" || message.Sender != name {
			continue
		}
		counts[message.Wxid]++
	}
	best := ""
	bestCount := 0
	for wxid, count := range counts {
		if count > bestCount || (count == bestCount && wxid < best) {
			best, bestCount = wxid, count
		}
	}
	return best
}

// parseSummary 把带 `人名` 和 [[res:n]] 标记的摘要拆成结构化片段。
func parseSummary(summary string, resources []reportstate.Resource, number func(reportstate.Resource) int) ([]Segment, []string) {
	var segments []Segment
	var warnings []string
	last := 0
	for _, match := range summaryTokenRe.FindAllStringSubmatchIndex(summary, -1) {
		if match[0] > last {
			segments = append(segments, Segment{Kind: "text", Text: summary[last:match[0]]})
		}
		switch {
		case match[2] >= 0:
			segments = append(segments, Segment{Kind: "person", Text: summary[match[2]:match[3]]})
		default:
			n, _ := strconv.Atoi(summary[match[4]:match[5]])
			if n < 1 || n > len(resources) {
				warnings = append(warnings, fmt.Sprintf("摘要引用了不存在的资源 [[res:%d]]，已忽略", n))
				break
			}
			resource := resources[n-1]
			segments = append(segments, Segment{
				Kind:  "resource",
				Index: number(resource),
				URL:   resource.URL,
			})
		}
		last = match[1]
	}
	if last < len(summary) {
		segments = append(segments, Segment{Kind: "text", Text: summary[last:]})
	}
	return segments, warnings
}

func formatTime(ts int64, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	return time.Unix(ts, 0).In(loc).Format("01-02 15:04")
}
