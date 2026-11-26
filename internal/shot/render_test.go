package shot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLatestReportPrefersTimestamp(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "20261123230000.md")
	newer := filepath.Join(dir, "20261124000000.md")
	if err := os.WriteFile(older, []byte("# 旧\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("# 新\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 时间戳更大的文件反而 mtime 更旧，仍应被选中。
	now := time.Now()
	if err := os.Chtimes(older, now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	rep, err := latestReport(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Slug != "20261124000000" {
		t.Fatalf("slug = %s, want 20261124000000", rep.Slug)
	}
}

func TestLatestReportMissingDir(t *testing.T) {
	_, err := latestReport(filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("err = %v, want 目录不存在", err)
	}
}

func TestRenderHTMLUsesFixedTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20261124000000.md")
	content := "# 原标题\n\n## 1. 话题\n\n正文 `甲`。\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	htmlBytes, err := renderHTML(report{Slug: "20261124000000", Title: "原标题", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	text := string(htmlBytes)
	for _, want := range []string{fixedTitle, "话题", "正文", "report-title", "main.markdown-body {"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(text, "原标题") {
		t.Errorf("original # title should be replaced by fixed title")
	}
}
