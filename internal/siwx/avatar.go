package siwx

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BaseURL 返回 SIWX 本地接口地址（可用 SIWX_BASE_URL 覆盖）。
func BaseURL() string {
	return strings.TrimRight(envOr("SIWX_BASE_URL", defaultBaseURL), "/")
}

// FetchAvatar 通过本地 SIWX 接口获取成员头像原始字节（JPEG/PNG）。
func FetchAvatar(account, wxid string) ([]byte, error) {
	if wxid == "" {
		return nil, fmt.Errorf("wxid 为空")
	}
	baseURL := BaseURL()
	query := url.Values{}
	query.Set("account", account)
	query.Set("username", wxid)

	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Get(baseURL + "/api/chat/avatar?" + query.Encode())
	if err != nil {
		return nil, fmt.Errorf("请求头像: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("头像接口 HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("读取头像: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("头像为空")
	}
	return data, nil
}
