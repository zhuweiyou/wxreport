package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestSchedulerAcceptsSixFields(t *testing.T) {
	scheduler := newScheduler(time.UTC, cron.DefaultLogger)
	if _, err := scheduler.AddFunc("0 0 0,4,8,12,16,20 * * *", func() {}); err != nil {
		t.Fatalf("6 段 cron 解析失败: %v", err)
	}
}

func TestLoadLocationFallback(t *testing.T) {
	t.Setenv("WXREPORT_TIMEZONE", "")
	name, loc, err := loadLocation()
	if err != nil {
		t.Fatal(err)
	}
	if name != "UTC+8" {
		t.Fatalf("name = %q", name)
	}
	_, offset := time.Now().In(loc).Zone()
	if offset != 8*3600 {
		t.Fatalf("offset = %d, want %d", offset, 8*3600)
	}
}

func TestEnvBool(t *testing.T) {
	if envBool("WXREPORT_TEST_UNSET", false) {
		t.Fatal("未设置时应返回默认值 false")
	}
	t.Setenv("WXREPORT_TEST", "true")
	if !envBool("WXREPORT_TEST", false) {
		t.Fatal("true 应解析为 true")
	}
}

func TestRunMainHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runMain([]string{"--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("--help 退出码 = %d，期望 0（stderr: %s）", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--once") {
		t.Fatalf("帮助输出缺少 --once: %q", stdout.String())
	}
}

func TestRunMainUnknownArg(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runMain([]string{"nope"}, &stdout, &stderr); code != 2 {
		t.Fatalf("未知参数退出码 = %d，期望 2", code)
	}
	if !strings.Contains(stderr.String(), "未知参数") {
		t.Fatalf("stderr 未包含未知参数提示: %q", stderr.String())
	}
}

func TestRunMainUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runMain([]string{"--nope"}, &stdout, &stderr); code == 0 {
		t.Fatal("未知 flag 应返回非 0 退出码")
	}
}
