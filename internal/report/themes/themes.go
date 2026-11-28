package themes

import (
	"fmt"
	"sort"
	"strings"

	"wxreport/internal/report/theme"
	minimaltheme "wxreport/internal/report/themes/minimal"
	momentstheme "wxreport/internal/report/themes/moments"
)

// Default 是没有配置 WXREPORT_THEME 时使用的主题。
const Default = "moments"

var registry = map[string]theme.Theme{}

func init() {
	register(momentstheme.New())
	register(minimaltheme.New())
}

func register(t theme.Theme) {
	if t.Name == "" {
		panic("主题名不能为空")
	}
	if _, exists := registry[t.Name]; exists {
		panic("主题重复注册: " + t.Name)
	}
	registry[t.Name] = t
}

// Get 按名称返回主题；空名称使用默认主题。
func Get(name string) (theme.Theme, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = Default
	}
	t, ok := registry[name]
	if !ok {
		return theme.Theme{}, fmt.Errorf("未知主题 %q，可用主题: %s", name, strings.Join(Names(), ", "))
	}
	return t, nil
}

// Names 返回已注册主题名（按字典序）。
func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
