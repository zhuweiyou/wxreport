package report

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"

	"wxreport/internal/report/theme"
	"wxreport/internal/report/themes"
)

// HTMLOptions 控制报告 HTML 渲染。
type HTMLOptions struct {
	Title string
	Theme string
}

// HTMLResult 是渲染后的自包含 HTML 与主题声明的截图宽度。
type HTMLResult struct {
	HTML  []byte
	Width int
}

type reportPage struct {
	Report
	PageTitle string
	FontCSS   template.CSS
	CSS       template.CSS
	Topics    []htmlTopic
}

// htmlTopic 在 Topic 基础上提供可安全输出的头像 src。
// html/template 默认把 data: URI 判为不安全并替换成 #ZgotmplZ，
// 这里用 template.URL 显式声明这是可信的内嵌图片。
type htmlTopic struct {
	Topic
	AvatarSrc    template.URL
	Participants []htmlParticipant
}

type htmlParticipant struct {
	Participant
	AvatarSrc template.URL
}

// HTML 用结构化报告生成一份自包含的移动端页面，供 headless Chrome 截图。
func HTML(rep Report, opts HTMLOptions) (HTMLResult, error) {
	selected, err := themes.Get(opts.Theme)
	if err != nil {
		return HTMLResult{}, err
	}

	pageTitle := strings.TrimSpace(opts.Title)
	if pageTitle == "" {
		pageTitle = rep.Title
	}
	if pageTitle == "" {
		pageTitle = "24小时话题TOP10"
	}
	topics := make([]htmlTopic, len(rep.Topics))
	for i, topic := range rep.Topics {
		var src template.URL
		if topic.AvatarData != "" {
			src = template.URL(topic.AvatarData)
		}
		participants := make([]htmlParticipant, len(topic.Participants))
		for j, participant := range topic.Participants {
			var participantSrc template.URL
			if participant.AvatarData != "" {
				participantSrc = template.URL(participant.AvatarData)
			}
			participants[j] = htmlParticipant{Participant: participant, AvatarSrc: participantSrc}
		}
		topics[i] = htmlTopic{Topic: topic, AvatarSrc: src, Participants: participants}
	}

	fontCSS := theme.SharedFontCSS()
	if selected.ExtraFontCSS != "" {
		fontCSS = template.CSS(string(fontCSS) + string(selected.ExtraFontCSS))
	}
	page := reportPage{
		Report:    rep,
		PageTitle: pageTitle,
		FontCSS:   fontCSS,
		CSS:       selected.CSS,
		Topics:    topics,
	}
	var out bytes.Buffer
	if err := selected.Template.Execute(&out, page); err != nil {
		return HTMLResult{}, fmt.Errorf("渲染 HTML 模板: %w", err)
	}
	return HTMLResult{HTML: out.Bytes(), Width: selected.Width}, nil
}
