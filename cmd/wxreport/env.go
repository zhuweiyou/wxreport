package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Env 承载命令运行时的依赖，避免散落的全局状态。
type Env struct {
	Root   string
	Stdout io.Writer
	Stderr io.Writer
	Getenv func(string) string
}

// abs 把相对路径解释为相对项目根目录；空值和绝对路径原样返回。
func (e *Env) abs(value string) string {
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(e.Root, value)
}

// loadLocalEnv 读取项目根目录的 config/local.env（KEY=VALUE），用于本机私有配置。
// 已存在的环境变量优先；该文件只存在于本机，已被 .gitignore 忽略。
func loadLocalEnv(root string) {
	raw, err := os.ReadFile(filepath.Join(root, "config", "local.env"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("找不到 go.mod，请在项目目录内运行")
		}
		dir = parent
	}
}
