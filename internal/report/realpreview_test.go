package report

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wxreport/internal/rank"
	"wxreport/internal/shot"
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

	rep, warnings, err := Build(&st, &export, Options{
		Top:            10,
		Location:       time.FixedZone("UTC+8", 8*3600),
		NoParticipants: os.Getenv("REAL_PREVIEW_PARTICIPANTS") != "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range warnings {
		t.Log("警告:", warning)
	}

	html, err := HTML(rep, "")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "data", "report-preview.html")
	if err := os.WriteFile(out, html, 0o644); err != nil {
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

	rep, _, err := Build(&st, &export, Options{
		Top:            10,
		Location:       time.FixedZone("UTC+8", 8*3600),
		NoParticipants: os.Getenv("REAL_PREVIEW_PARTICIPANTS") == "false",
	})
	if err != nil {
		t.Fatal(err)
	}
	html, err := HTML(rep, "")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result, err := shot.Capture(ctx, shot.Options{
		HTML:  html,
		Slug:  "preview",
		Width: shot.DefaultWidth,
		Scale: shot.DefaultScale,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "data", "latest-report.png")
	if err := os.WriteFile(out, result.PNG, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("已生成 %s（%d×%d，%d KB）", out, result.Width, result.Height, len(result.PNG)/1024)
}
