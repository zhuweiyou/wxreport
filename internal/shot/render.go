package shot

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html"
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

// fixedTitle 是分享长图的固定大标题，不跟随 Markdown 里的 # 标题变化。
const fixedTitle = "3群24小时话题TOP10"

var (
	markdownRenderer = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote))
	firstTitleRe     = regexp.MustCompile(`^#\s+(.*)$`)
	timestampRe      = regexp.MustCompile(`^\d{14}$`)
)

const reportCSS = `
html, body { margin: 0; padding: 0; background: #f6f8fa; }
main.markdown-body {
  box-sizing: border-box;
  min-width: 200px;
  max-width: 980px;
  margin: 24px auto;
  padding: 45px;
  background: #ffffff;
  border: 1px solid #d0d7de;
  border-radius: 6px;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC",
    "Hiragino Sans GB", "Microsoft YaHei", "Shot Lepcha", "Shot Tai Tham",
    "Microsoft Himalaya", "Kailasa", "Apple Color Emoji", "Segoe UI Emoji",
    "Noto Color Emoji", sans-serif;
}
@media (max-width: 767px) {
  main.markdown-body {
    margin: 0;
    padding: 15px;
    border: 0;
    border-radius: 0;
  }
}
main.markdown-body .report-title {
  margin: 0 0 20px;
  padding-bottom: 14px;
  border-bottom: 1px solid #d8dee4;
  font-size: 26px;
  line-height: 1.25;
  font-weight: 700;
  text-align: center;
  color: #1f2328;
}
main.markdown-body h2 { font-size: 18px; }
main.markdown-body sup a { text-decoration: none; }
main.markdown-body sup a::before { content: "["; }
main.markdown-body sup a::after { content: "]"; }
main.markdown-body .footnotes { font-size: 1em; color: #1f2328; margin-top: 8px; border-top: 0; }
main.markdown-body .footnotes > hr { display: none; }
main.markdown-body .footnotes ol { padding-left: 1.5em; }
main.markdown-body .footnotes li { margin: 8px 0; }
main.markdown-body .footnote-backref { display: none; }
main.markdown-body .footnotes li > p > a[href^="http"] { color: #0969da; word-break: break-all; }
`

// report 是一篇待截图的 Markdown 报告。
type report struct {
	Slug  string
	Title string
	Path  string
}

// latestReport 返回报告目录（data/reports）里最新的一篇报告。文件名是 14 位时间戳时按文件名排序，
// 否则退回文件修改时间。
func latestReport(dir string) (report, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return report{}, fmt.Errorf("报告目录 %s 不存在，请先运行 wxreport report 生成报告", dir)
		}
		return report{}, fmt.Errorf("读取报告目录 %s: %w", dir, err)
	}

	type candidate struct {
		report
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
			return report{}, fmt.Errorf("读取报告 %s: %w", entry.Name(), err)
		}
		stem := strings.TrimSuffix(entry.Name(), ".md")
		candidates = append(candidates, candidate{
			report:  report{Slug: stem, Path: filepath.Join(dir, entry.Name())},
			modTime: info.ModTime(),
			timed:   timestampRe.MatchString(stem),
		})
	}
	if len(candidates) == 0 {
		return report{}, fmt.Errorf("报告目录 %s 下没有 Markdown 报告", dir)
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

	latest := candidates[0]
	title, err := reportTitle(latest.Path)
	if err != nil {
		return report{}, err
	}
	latest.Title = title
	return latest.report, nil
}

func reportTitle(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取报告 %s: %w", filepath.Base(path), err)
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if match := firstTitleRe.FindStringSubmatch(strings.TrimSpace(line)); match != nil {
			return match[1], nil
		}
	}
	return strings.TrimSuffix(filepath.Base(path), ".md"), nil
}

// renderHTML 生成一份自包含的移动端报告页：Markdown 正文 + 内嵌 GitHub 样式，
// 不依赖本地 server、登录态或外部 CDN。
func renderHTML(rep report) ([]byte, error) {
	raw, err := os.ReadFile(rep.Path)
	if err != nil {
		return nil, fmt.Errorf("读取报告 %s: %w", filepath.Base(rep.Path), err)
	}
	title := fixedTitle

	var body bytes.Buffer
	if err := markdownRenderer.Convert([]byte(stripFirstTitle(string(raw))), &body); err != nil {
		return nil, fmt.Errorf("渲染 Markdown %s: %w", filepath.Base(rep.Path), err)
	}

	var page strings.Builder
	page.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">`)
	page.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	page.WriteString(`<title>`)
	page.WriteString(html.EscapeString(title))
	page.WriteString(`</title><style>`)
	page.WriteString(rareFontCSS)
	page.WriteString(githubMarkdownCSS)
	page.WriteString(reportCSS)
	page.WriteString(`</style></head><body><main class="markdown-body"><div class="report-title">`)
	page.WriteString(html.EscapeString(title))
	page.WriteString(`</div>`)
	page.WriteString(body.String())
	page.WriteString(`</main></body></html>`)
	return []byte(page.String()), nil
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
