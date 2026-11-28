// Package theme 定义报告主题契约。主题包只依赖本包，不依赖 report，
// 避免 report -> themes -> report 的循环依赖。
package theme

import "html/template"

// Theme 是一套报告主题：HTML 模板、CSS，以及可选的主题专属字体 CSS。
type Theme struct {
	Name         string
	Title        string
	Width        int
	Template     *template.Template
	CSS          template.CSS
	ExtraFontCSS template.CSS
}
