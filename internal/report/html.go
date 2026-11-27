package report

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"strings"
)

//go:embed static/report.css
var reportCSS string

//go:embed static/report.html.tmpl
var reportTemplateSource string

//go:embed static/NotoSansLepcha-Regular.ttf
var lepchaFont []byte

//go:embed static/NotoSansTaiTham-Regular.ttf
var taiThamFont []byte

// 微信群昵称里常见 ᰔᩚ（Lepcha + Tai Tham）装饰字符，Windows/macOS 默认都没有对应字体，
// 不内嵌就会截成方框。字体只用于截图 HTML。
var rareFontCSS = func() string {
	var builder strings.Builder
	builder.WriteString(`@font-face{font-family:"Shot Lepcha";src:url(data:font/ttf;base64,`)
	builder.WriteString(base64.StdEncoding.EncodeToString(lepchaFont))
	builder.WriteString(`) format("truetype");}`)
	builder.WriteString(`@font-face{font-family:"Shot Tai Tham";src:url(data:font/ttf;base64,`)
	builder.WriteString(base64.StdEncoding.EncodeToString(taiThamFont))
	builder.WriteString(`) format("truetype");}`)
	return builder.String()
}()

var reportTemplate = template.Must(template.New("report").Parse(reportTemplateSource))

type reportPage struct {
	Report
	PageTitle string
	FontCSS   template.CSS
	CSS       template.CSS
}

// HTML 用结构化报告生成一份自包含的移动端页面，供 headless Chrome 截图。
func HTML(rep Report, title string) ([]byte, error) {
	pageTitle := strings.TrimSpace(title)
	if pageTitle == "" {
		pageTitle = rep.Title
	}
	if pageTitle == "" {
		pageTitle = "24小时话题TOP10"
	}
	page := reportPage{
		Report:    rep,
		PageTitle: pageTitle,
		FontCSS:   template.CSS(rareFontCSS),
		CSS:       template.CSS(reportCSS),
	}
	var out bytes.Buffer
	if err := reportTemplate.Execute(&out, page); err != nil {
		return nil, fmt.Errorf("渲染 HTML 模板: %w", err)
	}
	return out.Bytes(), nil
}
