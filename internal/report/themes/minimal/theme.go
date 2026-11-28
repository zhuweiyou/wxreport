package minimal

import (
	_ "embed"
	"html/template"

	"wxreport/internal/report/theme"
)

//go:embed report.html.tmpl
var templateSource string

//go:embed report.css
var cssSource string

// New 返回「简约」主题。
func New() theme.Theme {
	return theme.Theme{
		Name:     "minimal",
		Title:    "简约",
		Width:    390,
		Template: template.Must(template.New("minimal").Parse(templateSource)),
		CSS:      template.CSS(cssSource),
	}
}
