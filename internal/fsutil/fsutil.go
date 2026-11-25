// Package fsutil 提供文件原子写入的小工具。
package fsutil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic 先写同目录临时文件再重命名，避免进程中断留下半截文件。
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建目录: %w", err)
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("写入 %s: %w", filepath.Base(path), err)
	}
	return nil
}

// WriteStringAtomic 是 WriteFileAtomic 的字符串版本。
func WriteStringAtomic(path, content string, perm os.FileMode) error {
	return WriteFileAtomic(path, []byte(content), perm)
}

// WriteJSONAtomic 以两空格缩进序列化 value 并原子写入，末尾保留换行。
func WriteJSONAtomic(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(raw, '\n'), 0o644)
}
