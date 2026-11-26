package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTMLUsesReportTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20261124000000.md")
	content := "# 原标题\n\n## 1. 话题\n\n正文 `甲`。\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	htmlBytes, err := HTML(Report{Path: path}, "")
	if err != nil {
		t.Fatal(err)
	}
	text := string(htmlBytes)
	for _, want := range []string{"原标题", "话题", "正文", "report-title", "main.markdown-body {"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestHTMLUsesTitleOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20261124000000.md")
	content := "# 原标题\n\n## 1. 话题\n\n正文 `甲`。\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	htmlBytes, err := HTML(Report{Path: path}, "自定义标题")
	if err != nil {
		t.Fatal(err)
	}
	text := string(htmlBytes)
	if !strings.Contains(text, "自定义标题") {
		t.Error("title override missing")
	}
	if strings.Contains(text, "原标题") {
		t.Error("original title should be replaced by override")
	}
}
