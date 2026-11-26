package siwx

import (
	"errors"
	"os"
	"strings"
)

// resolveOutputRoot 优先使用 SIWX API 返回的目录，其次 SIWX_OUTPUT_ROOT。
func resolveOutputRoot(apiValue string) (string, error) {
	if root := strings.TrimSpace(apiValue); root != "" {
		return root, nil
	}
	if root := strings.TrimSpace(os.Getenv("SIWX_OUTPUT_ROOT")); root != "" {
		return root, nil
	}
	return "", errors.New("SIWX 未返回解密输出目录，请设置 SIWX_OUTPUT_ROOT")
}
