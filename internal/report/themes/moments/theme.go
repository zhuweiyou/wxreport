package moments

import (
	_ "embed"
	"html/template"

	"wxreport/internal/report/theme"
)

//go:embed report.html.tmpl
var templateSource string

//go:embed report.css
var cssSource string

// New 返回当前「朋友圈」主题。
func New() theme.Theme {
	return theme.Theme{
		Name:     "moments",
		Title:    "朋友圈",
		Width:    390,
		Template: template.Must(template.New("moments").Parse(templateSource)),
		CSS:      template.CSS(cssSource),
	}
}
