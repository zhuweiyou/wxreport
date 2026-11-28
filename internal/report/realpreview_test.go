package report

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"wxreport/internal/rank"
	"wxreport/internal/shot"
	"wxreport/internal/siwx"
	reportstate "wxreport/internal/state"
)

// 临时调试：用真实 data/ 里的状态和导出渲染 HTML 预览。
// REAL_PREVIEW=1 go test ./internal/report -run TestRealPreview
func TestRealPreview(t *testing.T) {
	if os.Getenv("REAL_PREVIEW") == "" {
		t.Skip("设置 REAL_PREVIEW=1 生成真实数据预览")
	}
	root := filepath.Join("..", "..")

	rawState, err := os.ReadFile(filepath.Join(root, "data", "topics.json"))
	if err != nil {
		t.Fatal(err)
	}
	var st reportstate.State
	if err := json.Unmarshal(rawState, &st); err != nil {
		t.Fatal(err)
	}

	rawExport, err := os.ReadFile(filepath.Join(root, "data", "wxmsgs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var export rank.Export
	if err := json.Unmarshal(rawExport, &export); err != nil {
		t.Fatal(err)
	}

	avatarBaseURL := previewValue("SIWX_BASE_URL")
	if avatarBaseURL == "" {
		avatarBaseURL = "http://127.0.0.1:8787"
	}
	account := previewValue("SIWX_ACCOUNT")
	rep, warnings, err := Build(&st, &export, Options{
		Top:            10,
		Location:       time.FixedZone("UTC+8", 8*3600),
		NoParticipants: os.Getenv("REAL_PREVIEW_PARTICIPANTS") != "true",
		AvatarBaseURL:  avatarBaseURL,
		Account:        account,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range warnings {
		t.Log("警告:", warning)
	}
	attachPreviewAvatars(&rep, account)

	themeName := previewTheme()
	htmlResult, err := HTML(rep, HTMLOptions{Theme: themeName})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "data", fmt.Sprintf("report-preview-%s.html", themeName))
	if err := os.WriteFile(out, htmlResult.HTML, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("已生成 %s（%d 个话题，%d 个片段）", out, len(rep.Topics), len(rep.Topics[0].Segments))
}

// 临时调试：用真实数据直接生成长图，跳过模型总结。
// REAL_SHOT=1 go test ./internal/report -run TestRealShot
func TestRealShot(t *testing.T) {
	if os.Getenv("REAL_SHOT") == "" {
		t.Skip("设置 REAL_SHOT=1 生成长图")
	}
	root := filepath.Join("..", "..")

	rawState, err := os.ReadFile(filepath.Join(root, "data", "topics.json"))
	if err != nil {
		t.Fatal(err)
	}
	var st reportstate.State
	if err := json.Unmarshal(rawState, &st); err != nil {
		t.Fatal(err)
	}
	rawExport, err := os.ReadFile(filepath.Join(root, "data", "wxmsgs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var export rank.Export
	if err := json.Unmarshal(rawExport, &export); err != nil {
		t.Fatal(err)
	}

	avatarBaseURL := previewValue("SIWX_BASE_URL")
	if avatarBaseURL == "" {
		avatarBaseURL = "http://127.0.0.1:8787"
	}
	account := previewValue("SIWX_ACCOUNT")
	rep, _, err := Build(&st, &export, Options{
		Top:            10,
		Location:       time.FixedZone("UTC+8", 8*3600),
		NoParticipants: os.Getenv("REAL_PREVIEW_PARTICIPANTS") == "false",
		AvatarBaseURL:  avatarBaseURL,
		Account:        account,
	})
	if err != nil {
		t.Fatal(err)
	}
	attachPreviewAvatars(&rep, account)
	themeName := previewTheme()
	htmlResult, err := HTML(rep, HTMLOptions{Theme: themeName})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result, err := shot.Capture(ctx, shot.Options{
		HTML:  htmlResult.HTML,
		Slug:  "preview",
		Width: htmlResult.Width,
		Scale: shot.DefaultScale,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "data", fmt.Sprintf("theme-preview-%s.png", themeName))
	if err := os.WriteFile(out, result.PNG, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("已生成 %s（%d×%d，%d KB）", out, result.Width, result.Height, len(result.PNG)/1024)
}

func previewTheme() string {
	name := strings.TrimSpace(os.Getenv("REAL_PREVIEW_THEME"))
	if name == "" {
		return "moments"
	}
	return name
}

// previewValue 优先读环境变量，其次读项目根目录的 .env。
func previewValue(key string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	values, err := godotenv.Read(filepath.Join("..", "..", ".env"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(values[key])
}

// attachPreviewAvatars 模仿 pipeline 的头像内嵌步骤，让调试截图使用真实头像。
func attachPreviewAvatars(rep *Report, account string) {
	if strings.TrimSpace(account) == "" {
		return
	}
	cache := make(map[string]string)
	fetch := func(wxid string) string {
		if wxid == "" {
			return ""
		}
		if uri, ok := cache[wxid]; ok {
			return uri
		}
		data, err := siwx.FetchAvatar(account, wxid)
		if err != nil || len(data) == 0 {
			cache[wxid] = ""
			return ""
		}
		uri := "data:" + previewImageMIME(data) + ";base64," + base64.StdEncoding.EncodeToString(data)
		cache[wxid] = uri
		return uri
	}
	for i := range rep.Topics {
		topic := &rep.Topics[i]
		topic.AvatarData = fetch(topic.AvatarWxid)
		for j := range topic.Participants {
			participant := &topic.Participants[j]
			participant.AvatarData = fetch(participant.AvatarWxid)
		}
	}
}

func previewImageMIME(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		return "image/gif"
	default:
		return "image/jpeg"
	}
}
