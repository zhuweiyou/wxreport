package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wxreport/internal/rank"
)

func TestRunMainHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runMain([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("--help 退出码 = %d，期望 0（stderr: %s）", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "wxreport") {
		t.Fatalf("帮助输出缺少命令名: %q", stdout.String())
	}
}

func TestRunMainUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runMain([]string{"nope"}, &stdout, &stderr); code == 0 {
		t.Fatal("未知命令应返回非 0 退出码")
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("stderr 未包含 unknown command: %q", stderr.String())
	}
}

func TestRunMainMissingArg(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runMain([]string{"export"}, &stdout, &stderr); code != 1 {
		t.Fatalf("export 缺参数退出码 = %d，期望 1", code)
	}
	if !strings.Contains(stderr.String(), "用法: wxreport export") {
		t.Fatalf("stderr 未包含用法提示: %q", stderr.String())
	}
}

func TestRankFileWithMessages(t *testing.T) {
	dir := t.TempDir()
	exportPath := filepath.Join(dir, "wxmsgs.json")
	reportPath := filepath.Join(dir, "report.md")

	export := rank.Export{
		Total:        4,
		Participants: 2,
		Messages: []rank.Message{
			{Index: 1, TS: 0, Sender: "甲", Text: "a"},
			{Index: 2, TS: 60, Sender: "甲", Text: "b"},
			{Index: 3, TS: 120, Sender: "乙", Text: "c"},
			{Index: 4, TS: 180, Sender: "乙", Text: "d"},
		},
	}
	raw, err := json.Marshal(export)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exportPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	report := `# 报告

- 统计窗口：2026-11-22 23:00 ~ 2026-11-23 23:00
- 消息总数：0
- 参与人数：0

## 1. 话题二
<!-- msgs: 3,4 -->

正文 B

参与者：

## 2. 话题一
<!-- msgs: 1,2 -->

正文 A

参与者：
`
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := rankFile(reportPath, exportPath, 10, false); err != nil {
		t.Fatalf("rankFile: %v", err)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"- 消息总数：4",
		"- 参与人数：2",
		"参与者：乙(2)",
		"参与者：甲(2)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "<!-- msgs") {
		t.Errorf("comment not removed:\n%s", text)
	}
}

func TestRankFileFallbackWithoutMessages(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.md")
	report := `# 报告

- 统计窗口：2026-11-22 23:00 ~ 2026-11-23 23:00

## 1. 刷屏
正文 A
参与者：甲(10)、乙(2)

## 2. 广泛
正文 B
参与者：甲(2)、乙(2)、丙(2)、丁(2)、戊(2)
`
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rankFile(reportPath, "", 10, false); err != nil {
		t.Fatalf("rankFile: %v", err)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "## 1. 广泛") {
		t.Errorf("fallback ranking did not reorder:\n%s", text)
	}
}

func TestLoadExportRejectsInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadExport(path); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestReadJSONStripsCodeFence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	content := "```json\n{\"mid\": \"m1\"}\n```\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var value struct {
		MID string `json:"mid"`
	}
	if err := readJSON(path, &value); err != nil {
		t.Fatalf("readJSON: %v", err)
	}
	if value.MID != "m1" {
		t.Errorf("mid = %q, want m1", value.MID)
	}
}

func TestWriteFileAtomicCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reports", "nested", "x.md")
	if err := writeFileAtomic(path, "# x\n"); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# x\n" {
		t.Errorf("content = %q", data)
	}
}
