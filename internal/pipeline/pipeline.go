// Package pipeline 用固定顺序编排一次完整运行：刷新、导出、总结、合并、报告、长图。
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"wxreport/internal/compress"
	"wxreport/internal/fsutil"
	"wxreport/internal/oss"
	"wxreport/internal/rank"
	"wxreport/internal/report"
	"wxreport/internal/shot"
	"wxreport/internal/siwx"
	reportstate "wxreport/internal/state"
)

// Options 是一次运行的输入。
type Options struct {
	Root           string
	Account        string
	Chat           string
	ChatName       string
	Start          int64
	End            int64
	OSSPath        string
	NoUpload       bool
	NoParticipants bool
	Compress       bool
	Title          string
	Location       *time.Location
}

// Result 是一次运行的产物摘要。
type Result struct {
	ReportPath   string
	ImagePath    string
	PublicURL    string
	MessageTotal int
	Participants int
	Topics       int
	Skipped      bool
}

// Steps 是需要外部副作用的三步：SIWX 刷新、SIWX 导出、模型总结、截图。
type Steps interface {
	Refresh(ctx context.Context, account string) error
	Export(ctx context.Context, account, chat string, start, end int64, out string) (siwx.ExportStats, error)
	Summarize(ctx context.Context, plan *reportstate.Plan) (*reportstate.Update, error)
	Shot(ctx context.Context, opts shot.Options) (shot.Result, error)
}

// Run 按固定顺序执行完整流程；任一步失败立即返回。
func Run(ctx context.Context, opts Options, steps Steps, stdout io.Writer) (Result, error) {
	if opts.Root == "" {
		return Result{}, errors.New("root 不能为空")
	}
	if opts.Account == "" {
		return Result{}, errors.New("缺少账号：请设置 SIWX_ACCOUNT，或写入 .env")
	}
	if opts.Chat == "" {
		return Result{}, errors.New("缺少群：请设置 SIWX_CHAT，或传 --chat")
	}
	if steps == nil {
		return Result{}, errors.New("steps 不能为空")
	}

	end := opts.End
	if end == 0 {
		end = time.Now().Unix()
	}
	start := opts.Start
	if start == 0 {
		start = end - 24*3600
	}

	dataDir := filepath.Join(opts.Root, "data")
	exportPath := filepath.Join(dataDir, "wxmsgs.json")
	planPath := filepath.Join(dataDir, "wxstate-plan.json")
	updatePath := filepath.Join(dataDir, "wxstate-update.json")
	statePath := filepath.Join(dataDir, "topics.json")
	reportsDir := filepath.Join(dataDir, "reports")

	_, _ = fmt.Fprintln(stdout, "== 刷新 DB 缓存 ==")
	if err := steps.Refresh(ctx, opts.Account); err != nil {
		return Result{}, fmt.Errorf("refresh-db: %w", err)
	}

	_, _ = fmt.Fprintln(stdout, "== 导出消息 ==")
	stats, err := steps.Export(ctx, opts.Account, opts.Chat, start, end, exportPath)
	if err != nil {
		return Result{}, fmt.Errorf("export: %w", err)
	}
	if stats.Total == 0 {
		_, _ = fmt.Fprintln(stdout, "过去 24 小时无消息，跳过本次报告。")
		return Result{Skipped: true}, nil
	}

	export, err := loadExport(exportPath)
	if err != nil {
		return Result{}, err
	}

	_, _ = fmt.Fprintln(stdout, "== 生成增量计划 ==")
	st, err := reportstate.Load(statePath)
	if err != nil {
		return Result{}, err
	}
	plan, err := reportstate.BuildPlan(st, export, 6, opts.ChatName)
	if err != nil {
		return Result{}, fmt.Errorf("state-plan: %w", err)
	}
	if err := reportstate.WritePlan(planPath, plan); err != nil {
		return Result{}, fmt.Errorf("写计划: %w", err)
	}

	_, _ = fmt.Fprintln(stdout, "== 模型总结 ==")
	update, err := steps.Summarize(ctx, plan)
	if err != nil {
		return Result{}, fmt.Errorf("总结: %w", err)
	}
	if err := fsutil.WriteJSONAtomic(updatePath, update); err != nil {
		return Result{}, fmt.Errorf("写入模型输出: %w", err)
	}

	_, _ = fmt.Fprintln(stdout, "== 合并状态 ==")
	next, warnings, err := reportstate.Apply(st, plan, update, export)
	if err != nil {
		return Result{}, fmt.Errorf("state-apply: %w", err)
	}
	for _, warning := range warnings {
		_, _ = fmt.Fprintln(stdout, "警告:", warning)
	}
	if err := reportstate.Save(statePath, next); err != nil {
		return Result{}, fmt.Errorf("保存状态: %w", err)
	}

	_, _ = fmt.Fprintln(stdout, "== 生成报告 ==")
	markdown, rankResult, err := report.Render(next, export, report.Options{
		ChatName:       opts.ChatName,
		Top:            10,
		NoParticipants: opts.NoParticipants,
		Location:       opts.Location,
	})
	if err != nil {
		return Result{}, fmt.Errorf("report: %w", err)
	}
	endTime := time.Unix(end, 0)
	if opts.Location != nil {
		endTime = endTime.In(opts.Location)
	}
	slug := endTime.Format("20060102150405")
	reportPath := filepath.Join(reportsDir, slug+".md")
	if err := fsutil.WriteStringAtomic(reportPath, markdown, 0o644); err != nil {
		return Result{}, fmt.Errorf("写报告: %w", err)
	}
	printRanking(stdout, rankResult)

	_, _ = fmt.Fprintln(stdout, "== 生成长图 ==")
	htmlBytes, err := report.HTML(report.Report{Slug: slug, Path: reportPath}, opts.Title)
	if err != nil {
		return Result{}, fmt.Errorf("report html: %w", err)
	}
	shotResult, err := steps.Shot(ctx, shot.Options{
		HTML:  htmlBytes,
		Slug:  slug,
		Width: shot.DefaultWidth,
		Scale: shot.DefaultScale,
	})
	if err != nil {
		return Result{}, fmt.Errorf("shot: %w", err)
	}
	pngData := shotResult.PNG
	if opts.Compress {
		compressed, err := compress.PNG(pngData)
		switch {
		case err != nil:
			_, _ = fmt.Fprintf(stdout, "压缩失败，使用原图：%v\n", err)
		case len(compressed) >= len(pngData):
			_, _ = fmt.Fprintf(stdout, "压缩无收益（%d -> %d 字节），使用原图\n", len(pngData), len(compressed))
		default:
			_, _ = fmt.Fprintf(stdout, "压缩：%d -> %d 字节（-%.1f%%）\n",
				len(pngData), len(compressed), (1-float64(len(compressed))/float64(len(pngData)))*100)
			pngData = compressed
		}
	}

	imagePath := filepath.Join(dataDir, "latest-report.png")
	if err := fsutil.WriteFileAtomic(imagePath, pngData, 0o644); err != nil {
		return Result{}, fmt.Errorf("写入 PNG: %w", err)
	}
	_, _ = fmt.Fprintf(stdout, "已生成 %s（%d×%d，报告 %s，%d KB）\n",
		imagePath, shotResult.Width, shotResult.Height, shotResult.Slug, len(pngData)/1024)

	result := Result{
		ReportPath:   reportPath,
		ImagePath:    imagePath,
		MessageTotal: export.Total,
		Participants: export.Participants,
		Topics:       rankResult.Kept,
	}
	if opts.NoUpload {
		return result, nil
	}
	cfg, err := oss.LoadConfig(opts.Root)
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "未配置 OSS，跳过上传（本地图片已生成）：%v\n", err)
		return result, nil
	}
	objectPath := opts.OSSPath
	if objectPath == "" {
		objectPath = cfg.Path
	}
	if objectPath == "" {
		objectPath = "latest-report.png"
	}
	publicURL, err := oss.Upload(cfg, objectPath, pngData)
	if err != nil {
		return Result{}, err
	}
	result.PublicURL = publicURL
	_, _ = fmt.Fprintf(stdout, "已上传（覆盖）%s\n", publicURL)
	return result, nil
}

func loadExport(path string) (*rank.Export, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取消息导出: %w", err)
	}
	var export rank.Export
	if err := json.Unmarshal(raw, &export); err != nil {
		return nil, fmt.Errorf("解析消息导出 %s: %w", filepath.Base(path), err)
	}
	return &export, nil
}

func printRanking(w io.Writer, result rank.Result) {
	for _, warning := range result.Warnings {
		_, _ = fmt.Fprintln(w, "警告:", warning)
	}
	if result.Mode == "turns" {
		_, _ = fmt.Fprintln(w, "话题排序（话轮模型：0.45×有效参与人数 + 0.40×文字话轮 + 0.15×密集度）：")
		for i, topic := range result.Topics {
			status := "保留"
			if !topic.Kept {
				status = "截断"
			}
			textRatio := 0.0
			denseRatio := 0.0
			if topic.Turns > 0 {
				textRatio = float64(topic.TextTurns) / float64(topic.Turns)
				denseRatio = float64(topic.DenseTurns) / float64(topic.Turns)
			}
			_, _ = fmt.Fprintf(w, "%2d. %.4f  话轮 %3d  有效 %5.2f  人数 %3d  消息 %4d  文字 %.2f  密集 %.2f  [%s] %s\n",
				i+1, topic.Score, topic.Turns, topic.Effective, len(topic.Participants), topic.Messages,
				textRatio, denseRatio, status, topic.Title)
		}
	} else {
		_, _ = fmt.Fprintln(w, "话题排序（有效参与人数 × log2(2 + 消息数)）：")
		for i, topic := range result.Topics {
			status := "保留"
			if !topic.Kept {
				status = "截断"
			}
			_, _ = fmt.Fprintf(w, "%2d. %6.2f  消息 %4d  人数 %3d  有效 %5.2f  [%s] %s\n",
				i+1, topic.Score, topic.Messages, len(topic.Participants), topic.Effective, status, topic.Title)
		}
	}
	_, _ = fmt.Fprintf(w, "共 %d 个话题，保留 %d 个。\n", result.Total, result.Kept)
}
