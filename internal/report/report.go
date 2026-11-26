// Package report 把增量状态和消息导出渲染成 Markdown 报告。
package report

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"wxreport/internal/rank"
	reportstate "wxreport/internal/state"
)

var (
	leftoverResRe       = regexp.MustCompile(`\[\[res:\d+\]\]`)
	participantOutputRe = regexp.MustCompile(`(?m)^参与者[：:].*(?:\r?\n|$)`)
)

// Options 控制报告渲染。
type Options struct {
	ChatName       string
	Top            int
	NoParticipants bool
	Location       *time.Location
}

// Render 把状态和当前 24h 导出渲染成最终报告；排序、参与者由 rank 完成。
func Render(st *reportstate.State, export *rank.Export, opts Options) (string, rank.Result, error) {
	if st == nil || export == nil {
		return "", rank.Result{}, errors.New("state/export 不能为空")
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

	resourceNumbers := make(map[string]int)
	var resourceOrder []reportstate.Resource
	var warnings []string
	resourceNumber := func(resource reportstate.Resource) int {
		if number, ok := resourceNumbers[resource.URL]; ok {
			return number
		}
		resourceOrder = append(resourceOrder, resource)
		number := len(resourceOrder)
		resourceNumbers[resource.URL] = number
		return number
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "# 「%s」24 小时话题 TOP10\n\n", chatName)
	fmt.Fprintf(&builder, "- 统计窗口：%s ~ %s\n", formatTime(export.Start, opts.Location), formatTime(export.End, opts.Location))
	fmt.Fprintf(&builder, "- 消息总数：%d\n", export.Total)
	fmt.Fprintf(&builder, "- 参与人数：%d\n\n", export.Participants)

	topicCount := 0
	for _, topic := range st.Topics {
		var indices []int
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
		topicCount++
		fmt.Fprintf(&builder, "## %d. %s\n", topicCount, topic.Title)
		fmt.Fprintf(&builder, "<!-- msgs: %s -->\n\n", joinIndices(indices))
		if strings.TrimSpace(topic.Commentary) != "" {
			fmt.Fprintf(&builder, "> %s\n\n", topic.Commentary)
		}
		summary := topic.Summary
		for i, resource := range topic.Resources {
			marker := fmt.Sprintf("[[res:%d]]", i+1)
			if !strings.Contains(summary, marker) {
				continue
			}
			summary = strings.ReplaceAll(summary, marker, fmt.Sprintf("[^%d]", resourceNumber(resource)))
		}
		if leftoverResRe.MatchString(summary) {
			warnings = append(warnings, fmt.Sprintf("话题「%s」存在无效的资源标记，已忽略", topic.Title))
			summary = leftoverResRe.ReplaceAllString(summary, "")
		}
		builder.WriteString(strings.TrimSpace(summary))
		builder.WriteString("\n\n参与者：\n\n")
	}
	if topicCount == 0 {
		return "", rank.Result{}, errors.New("状态里没有可渲染的话题")
	}

	if len(resourceOrder) > 0 {
		builder.WriteString("## 资源列表\n\n")
		for i, resource := range resourceOrder {
			fmt.Fprintf(&builder, "[^%d]: %s — [%s](%s)\n", i+1, resource.Title, resource.URL, resource.URL)
		}
		builder.WriteString("\n")
	}

	output, result, err := rank.ProcessWithMessages(builder.String(), opts.Top, export)
	if err != nil {
		return "", rank.Result{}, err
	}
	if opts.NoParticipants {
		output = participantOutputRe.ReplaceAllString(output, "")
	}
	result.Warnings = append(result.Warnings, warnings...)
	return output, result, nil
}

func formatTime(ts int64, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	return time.Unix(ts, 0).In(loc).Format("01-02 15:04")
}

func joinIndices(indices []int) string {
	parts := make([]string, len(indices))
	for i, index := range indices {
		parts[i] = strconv.Itoa(index)
	}
	return strings.Join(parts, ",")
}
