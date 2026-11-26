// wxreport 生成微信群 24h 话题报告。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"

	"wxreport/internal/pipeline"
	"wxreport/internal/shot"
	"wxreport/internal/siwx"
	reportstate "wxreport/internal/state"
	"wxreport/internal/summarize"
)

const usageText = `wxreport

用法:
  wxreport          常驻运行，按 WXREPORT_CRON 定时执行
  wxreport --once   立即执行一次后退出

配置:
  当前目录的 .env（同名环境变量优先）`

func main() {
	os.Exit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

func runMain(args []string, stdout, stderr io.Writer) int {
	root, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "错误:", err)
		return 1
	}
	_ = godotenv.Load()

	flags := flag.NewFlagSet("wxreport", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprintln(stdout, usageText)
	}
	once := flags.Bool("once", false, "立即执行一次后退出")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "未知参数：%s\n", strings.Join(flags.Args(), " "))
		return 2
	}

	ctx := context.Background()
	if *once {
		if err := runOnce(ctx, root, stdout); err != nil {
			_, _ = fmt.Fprintln(stderr, "错误:", err)
			return 1
		}
		return 0
	}
	if err := runDaemon(ctx, root, stdout, stderr); err != nil {
		_, _ = fmt.Fprintln(stderr, "错误:", err)
		return 1
	}
	return 0
}

// runOnce 从配置读取全部参数，执行一次完整流程。
func runOnce(ctx context.Context, root string, stdout io.Writer) error {
	cfg, err := summarize.ConfigFromEnv(os.Getenv)
	if err != nil {
		return err
	}
	_, loc, err := loadLocation()
	if err != nil {
		return err
	}

	result, err := pipeline.Run(ctx, pipeline.Options{
		Root:           root,
		Account:        os.Getenv("SIWX_ACCOUNT"),
		Chat:           os.Getenv("SIWX_CHAT"),
		ChatName:       os.Getenv("SIWX_CHAT_NAME"),
		OSSPath:        os.Getenv("OSS_PATH"),
		NoUpload:       !envBool("WXREPORT_UPLOAD", false),
		NoParticipants: !envBool("WXREPORT_PARTICIPANTS", true),
		Compress:       envBool("WXREPORT_COMPRESS", true),
		Title:          os.Getenv("WXREPORT_TITLE"),
		Location:       loc,
	}, liveSteps{summarizer: summarize.New(cfg)}, stdout)
	if err != nil {
		return err
	}
	if result.Skipped {
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "完成：%s\n", result.ReportPath)
	return nil
}

// runDaemon 常驻运行，按 WXREPORT_CRON 定时执行 runOnce。
func runDaemon(ctx context.Context, root string, stdout, stderr io.Writer) error {
	schedule := os.Getenv("WXREPORT_CRON")
	if schedule == "" {
		schedule = "0 0 0,4,8,12,16,20 * * *"
	}
	timezone, loc, err := loadLocation()
	if err != nil {
		return err
	}

	logger := cron.PrintfLogger(log.New(stderr, "", log.LstdFlags))
	scheduler := newScheduler(loc, logger)
	if _, err := scheduler.AddFunc(schedule, func() {
		runCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := runOnce(runCtx, root, stdout); err != nil {
			_, _ = fmt.Fprintf(stderr, "执行失败：%v\n", err)
		}
	}); err != nil {
		return fmt.Errorf("cron 表达式: %w", err)
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	scheduler.Start()
	_, _ = fmt.Fprintf(stdout, "daemon 已启动：时区 %s，计划 %s\n", timezone, schedule)

	<-ctx.Done()
	scheduler.Stop()
	_, _ = fmt.Fprintln(stdout, "daemon 已停止。")
	return nil
}

// newScheduler 使用含秒的 6 段 cron 表达式（秒 分 时 日 月 周）。
func newScheduler(loc *time.Location, logger cron.Logger) *cron.Cron {
	return cron.New(
		cron.WithSeconds(),
		cron.WithLocation(loc),
		cron.WithChain(cron.SkipIfStillRunning(logger)),
		cron.WithLogger(logger),
	)
}

func loadLocation() (string, *time.Location, error) {
	name := os.Getenv("WXREPORT_TIMEZONE")
	if name == "" {
		name = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return "", nil, fmt.Errorf("时区 %s: %w", name, err)
	}
	return name, loc, nil
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

// liveSteps 是 pipeline.Steps 的生产实现。
type liveSteps struct {
	summarizer summarize.Summarizer
}

func (s liveSteps) Refresh(ctx context.Context, account string) error {
	return siwx.Refresh(account)
}

func (s liveSteps) Export(ctx context.Context, account, chat string, start, end int64, out string) (siwx.ExportStats, error) {
	return siwx.Export(account, chat, start, end, out)
}

func (s liveSteps) Summarize(ctx context.Context, plan *reportstate.Plan) (*reportstate.Update, error) {
	return s.summarizer.Summarize(ctx, plan)
}

func (s liveSteps) Shot(ctx context.Context, opts shot.Options) (shot.Result, error) {
	return shot.Capture(ctx, opts)
}
