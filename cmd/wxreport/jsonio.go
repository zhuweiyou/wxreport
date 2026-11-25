package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wxreport/internal/fsutil"
	"wxreport/internal/rank"
)

func readJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 %s: %w", filepath.Base(path), err)
	}
	text := strings.TrimSpace(string(raw))
	// 模型有时会把 JSON 包在 Markdown 代码块或说明文字里，这里做一次容错。
	if strings.HasPrefix(text, "```") {
		if newline := strings.Index(text, "\n"); newline >= 0 {
			text = strings.TrimSpace(text[newline+1:])
		}
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "```"))
	}
	if start := strings.Index(text, "{"); start >= 0 {
		if end := strings.LastIndex(text, "}"); end > start {
			text = text[start : end+1]
		}
	}
	if err := json.Unmarshal([]byte(text), target); err != nil {
		return fmt.Errorf("解析 %s: %w", filepath.Base(path), err)
	}
	return nil
}

func loadExport(path string) (*rank.Export, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取消息导出: %w", err)
	}
	var export rank.Export
	if err := json.Unmarshal(raw, &export); err != nil {
		return nil, fmt.Errorf("解析消息导出 %s: %w", filepath.Base(path), err)
	}
	return &export, nil
}

// writeFileAtomic 是给命令和测试共用的原子写入包装。
func writeFileAtomic(path, content string) error {
	return fsutil.WriteStringAtomic(path, content, 0o644)
}
