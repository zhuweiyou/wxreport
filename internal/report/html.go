package report

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

//go:embed static/github-markdown-light.css
var githubMarkdownCSS string

//go:embed static/report.css
var reportCSS string

//go:embed static/report.html.tmpl
var reportTemplateSource string

//go:embed static/NotoSansLepcha-Regular.ttf
var lepchaFont []byte

//go:embed static/NotoSansTaiTham-Regular.ttf
var taiThamFont []byte

// 微信群昵称里常见 ᰔᩚ（Lepcha + Tai Tham）装饰字符，Windows/macOS 默认都没有对应字体，
// 不内嵌就会截成方框。字体只在截图 HTML 里使用，不影响 Markdown 报告本身。
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

var (
	markdownRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote))
	firstTitleRe     = regexp.MustCompile(`^#\s+(.*)$`)
	timestampRe      = regexp.MustCompile(`^\d{14}$`)
	reportTemplate   = template.Must(template.New("report").Parse(reportTemplateSource))
)

// Report 是一篇待截图的 Markdown 报告。
type Report struct {
	Slug string
	Path string
}

// Latest 返回报告目录（data/reports）里最新的一篇报告。文件名是 14 位时间戳时按文件名排序，
// 否则退回文件修改时间。
func Latest(dir string) (Report, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Report{}, fmt.Errorf("报告目录 %s 不存在，请先生成报告", dir)
		}
		return Report{}, fmt.Errorf("读取报告目录 %s: %w", dir, err)
	}

	type candidate struct {
		Report
		modTime time.Time
		timed   bool
	}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return Report{}, fmt.Errorf("读取报告 %s: %w", entry.Name(), err)
		}
		stem := strings.TrimSuffix(entry.Name(), ".md")
		candidates = append(candidates, candidate{
			Report:  Report{Slug: stem, Path: filepath.Join(dir, entry.Name())},
			modTime: info.ModTime(),
			timed:   timestampRe.MatchString(stem),
		})
	}
	if len(candidates) == 0 {
		return Report{}, fmt.Errorf("报告目录 %s 下没有 Markdown 报告", dir)
	}

	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.timed && b.timed {
			return a.Slug > b.Slug
		}
		if a.timed != b.timed {
			return a.timed
		}
		if !a.modTime.Equal(b.modTime) {
			return a.modTime.After(b.modTime)
		}
		return a.Slug > b.Slug
	})
	return candidates[0].Report, nil
}

// HTML 生成一份自包含的移动端报告页：Markdown 正文 + 内嵌 GitHub 样式，
// 不依赖本地 server、登录态或外部 CDN。title 为空时使用 Markdown 的 # 标题。
func HTML(rep Report, title string) ([]byte, error) {
	raw, err := os.ReadFile(rep.Path)
	if err != nil {
		return nil, fmt.Errorf("读取报告 %s: %w", filepath.Base(rep.Path), err)
	}
	source := string(raw)

	pageTitle := strings.TrimSpace(title)
	if pageTitle == "" {
		pageTitle = firstTitle(source)
	}
	if pageTitle == "" {
		pageTitle = "24小时话题TOP10"
	}

	var body bytes.Buffer
	if err := markdownRenderer.Convert([]byte(stripFirstTitle(source)), &body); err != nil {
		return nil, fmt.Errorf("渲染 Markdown %s: %w", filepath.Base(rep.Path), err)
	}

	page := reportPage{
		Title:       pageTitle,
		RareFontCSS: template.CSS(rareFontCSS),
		GitHubCSS:   template.CSS(githubMarkdownCSS),
		ReportCSS:   template.CSS(reportCSS),
		// goldmark 默认转义原始 HTML，这里注入的是受信任的渲染结果。
		Body: template.HTML(body.String()),
	}
	var out bytes.Buffer
	if err := reportTemplate.Execute(&out, page); err != nil {
		return nil, fmt.Errorf("渲染 HTML 模板: %w", err)
	}
	return out.Bytes(), nil
}

func firstTitle(source string) string {
	for _, line := range strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n") {
		if match := firstTitleRe.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			return strings.TrimSpace(match[1])
		}
	}
	return ""
}

type reportPage struct {
	Title       string
	RareFontCSS template.CSS
	GitHubCSS   template.CSS
	ReportCSS   template.CSS
	Body        template.HTML
}

// stripFirstTitle 去掉 Markdown 的一级标题，页面标题单独渲染，避免重复。
func stripFirstTitle(source string) string {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		if !removed {
			if firstTitleRe.MatchString(strings.TrimSpace(line)) {
				removed = true
				continue
			}
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
