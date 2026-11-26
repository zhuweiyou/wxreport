package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLatestPrefersTimestamp(t *testing.T) {
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

	rep, err := Latest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Slug != "20261124000000" {
		t.Fatalf("slug = %s, want 20261124000000", rep.Slug)
	}
}

func TestLatestMissingDir(t *testing.T) {
	_, err := Latest(filepath.Join(t.TempDir(), "nope"))
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("err = %v, want 目录不存在", err)
	}
}

func TestHTMLUsesReportTitle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "20261124000000.md")
	content := "# 原标题\n\n## 1. 话题\n\n正文 `甲`。\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	htmlBytes, err := HTML(Report{Slug: "20261124000000", Path: path}, "")
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

	htmlBytes, err := HTML(Report{Slug: "20261124000000", Path: path}, "自定义标题")
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
