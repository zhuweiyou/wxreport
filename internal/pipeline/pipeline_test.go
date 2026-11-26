package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"wxreport/internal/rank"
	"wxreport/internal/shot"
	reportstate "wxreport/internal/state"
	"wxreport/internal/siwx"
)

type fakeSteps struct {
	export       rank.Export
	update       *reportstate.Update
	png          []byte
	refreshErr   error
	exportErr    error
	summarizeErr error
	gotPlan      *reportstate.Plan
}

func (f *fakeSteps) Refresh(ctx context.Context, account string) error {
	return f.refreshErr
}

func (f *fakeSteps) Export(ctx context.Context, account, chat string, start, end int64, out string) (siwx.ExportStats, error) {
	if f.exportErr != nil {
		return siwx.ExportStats{}, f.exportErr
	}
	f.export.Chat = chat
	f.export.Start = start
	f.export.End = end
	raw, err := json.Marshal(f.export)
	if err != nil {
		return siwx.ExportStats{}, err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return siwx.ExportStats{}, err
	}
	if err := os.WriteFile(out, raw, 0o644); err != nil {
		return siwx.ExportStats{}, err
	}
	return siwx.ExportStats{Total: f.export.Total, Participants: f.export.Participants, Output: out}, nil
}

func (f *fakeSteps) Summarize(ctx context.Context, plan *reportstate.Plan) (*reportstate.Update, error) {
	f.gotPlan = plan
	return f.update, f.summarizeErr
}

func (f *fakeSteps) Shot(ctx context.Context, opts shot.Options) (shot.Result, error) {
	return shot.Result{Slug: "test", PNG: f.png, Width: 2, Height: 2}, nil
}

func TestRunEndToEnd(t *testing.T) {
	root := t.TempDir()
	steps := &fakeSteps{
		export: rank.Export{
			Total:        2,
			Participants: 1,
			Messages: []rank.Message{
				{Index: 1, MID: "m1", TS: 1000, Sender: "甲", Text: "a", Kind: "text"},
				{Index: 2, MID: "m2", TS: 1060, Sender: "甲", Text: "b", Kind: "text"},
			},
		},
		update: &reportstate.Update{NewTopics: []reportstate.NewTopic{{
			TempID:     "n1",
			Title:      "话题",
			Commentary: "点评",
			Summary:    "`甲` 说了两句。",
			MessageIDs: []string{"m1", "m2"},
		}}},
		png: []byte("png"),
	}

	var out bytes.Buffer
	result, err := Run(context.Background(), Options{
		Root: root, Account: "wxid", Chat: "c@chatroom", ChatName: "测试群",
		Start: 1000, End: 2000, NoUpload: true, NoParticipants: true,
	}, steps, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Skipped {
		t.Fatal("unexpected skip")
	}
	if steps.gotPlan == nil || !steps.gotPlan.Bootstrap {
		t.Fatalf("plan = %+v, want bootstrap", steps.gotPlan)
	}

	report, err := os.ReadFile(result.ReportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	text := string(report)
	if strings.Contains(text, "参与者：") {
		t.Fatalf("participants should be omitted:\n%s", text)
	}
	windowRe := regexp.MustCompile(`统计窗口：\d{2}-\d{2} \d{2}:\d{2} ~ \d{2}-\d{2} \d{2}:\d{2}`)
	if !windowRe.MatchString(text) {
		t.Fatalf("window should omit year:\n%s", text)
	}

	for _, path := range []string{
		filepath.Join(root, "data", "topics.json"),
		filepath.Join(root, "data", "wxstate-plan.json"),
		filepath.Join(root, "data", "wxstate-update.json"),
		filepath.Join(root, "data", "latest-report.png"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
	png, err := os.ReadFile(result.ImagePath)
	if err != nil || string(png) != "png" {
		t.Errorf("image = %q, %v", png, err)
	}
}

func TestRunSkipsWhenNoMessages(t *testing.T) {
	root := t.TempDir()
	steps := &fakeSteps{export: rank.Export{Total: 0}}

	var out bytes.Buffer
	result, err := Run(context.Background(), Options{
		Root: root, Account: "wxid", Chat: "c@chatroom",
		Start: 1000, End: 2000, NoUpload: true,
	}, steps, &out)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Skipped {
		t.Fatal("expected skipped result")
	}
	if _, err := os.Stat(filepath.Join(root, "data", "reports")); !os.IsNotExist(err) {
		t.Fatalf("reports dir should not exist, err = %v", err)
	}
}

func TestRunStopsOnSummarizeError(t *testing.T) {
	root := t.TempDir()
	steps := &fakeSteps{
		export: rank.Export{
			Total: 1, Participants: 1,
			Messages: []rank.Message{{Index: 1, MID: "m1", TS: 1000, Sender: "甲", Text: "a", Kind: "text"}},
		},
		summarizeErr: errors.New("boom"),
	}

	var out bytes.Buffer
	_, err := Run(context.Background(), Options{
		Root: root, Account: "wxid", Chat: "c@chatroom",
		Start: 1000, End: 2000, NoUpload: true,
	}, steps, &out)
	if err == nil || !strings.Contains(err.Error(), "总结") {
		t.Fatalf("err = %v, want 总结 error", err)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "topics.json")); !os.IsNotExist(err) {
		t.Fatalf("topics.json should not be written, err = %v", err)
	}
}
