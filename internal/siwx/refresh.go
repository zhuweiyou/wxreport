// Package siwx 调用 stories-in-wx 的本地 HTTP API，刷新微信 DB 解密缓存。
package siwx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultBaseURL = "http://127.0.0.1:8787"

type statusResponse struct {
	Accounts []struct {
		Wxid  string `json:"wxid"`
		DBDir string `json:"db_dir"`
	} `json:"accounts"`
}

type jobStatus struct {
	Done  bool                `json:"done"`
	Error *string             `json:"error"`
	Logs  [][]json.RawMessage `json:"logs"`
}

// Refresh 触发一次 auto 解密任务，并等待完成。
// 该服务不会自动解密最新写入的 DB，所以每次查询前都要调用。
func Refresh(account string) error {
	baseURL := strings.TrimRight(envOr("SIWX_BASE_URL", defaultBaseURL), "/")
	client := &http.Client{Timeout: 30 * time.Second}

	var status statusResponse
	if err := getJSON(client, baseURL+"/api/status", &status); err != nil {
		return fmt.Errorf("读取 stories-in-wx 状态失败（服务是否在运行？）: %w", err)
	}

	dbDir := ""
	for _, item := range status.Accounts {
		if item.Wxid == account {
			dbDir = item.DBDir
			break
		}
	}
	if dbDir == "" {
		return fmt.Errorf("stories-in-wx 中找不到账号 %s 的 db_dir", account)
	}

	payload, err := json.Marshal(map[string]string{"mode": "auto", "db_dir": dbDir})
	if err != nil {
		return err
	}
	response, err := client.Post(baseURL+"/api/run", "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("启动刷新任务失败: %w", err)
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	response.Body.Close()
	if response.StatusCode == http.StatusConflict {
		fmt.Println("已有刷新任务在运行，等待它完成…")
	} else if response.StatusCode >= 300 {
		return fmt.Errorf("启动刷新任务失败: HTTP %d %s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	deadline := time.Now().Add(3 * time.Minute)
	printed := 0
	lastLog := ""
	for {
		var job jobStatus
		if err := getJSON(client, baseURL+"/api/job", &job); err != nil {
			return fmt.Errorf("读取刷新任务状态失败: %w", err)
		}
		for ; printed < len(job.Logs); printed++ {
			if len(job.Logs[printed]) < 2 {
				continue
			}
			var line string
			if err := json.Unmarshal(job.Logs[printed][1], &line); err == nil {
				lastLog = line
			}
		}
		if job.Error != nil && *job.Error != "" {
			return fmt.Errorf("刷新失败: %s", *job.Error)
		}
		if job.Done {
			if lastLog != "" {
				fmt.Println(lastLog)
			}
			fmt.Println("DB 缓存已刷新。")
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("刷新超时（3 分钟）")
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func getJSON(client *http.Client, url string, target any) error {
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("HTTP %d %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(response.Body).Decode(target)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
