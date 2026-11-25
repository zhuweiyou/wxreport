package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"wxreport/internal/fsutil"
	"wxreport/internal/shot"
)

type shotOptions struct {
	Report     string
	Width      int
	Scale      int
	Out        string
	OSSPath    string
	NoUpload   bool
	ChromePath string
}

func newShotCmd(env *Env) *cobra.Command {
	var opts shotOptions
	cmd := &cobra.Command{
		Use:   "shot",
		Short: "生成长图（配置了 OSS 则覆盖上传）",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShot(cmd.Context(), env, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.Report, "report", "", "报告 slug/文件名，默认最新一篇")
	flags.IntVar(&opts.Width, "width", shot.DefaultWidth, "CSS 视口宽度")
	flags.IntVar(&opts.Scale, "scale", shot.DefaultScale, "图片倍率（2 或 3）")
	flags.StringVar(&opts.Out, "out", "", "本地 PNG 路径，默认 data/latest-report.png")
	flags.StringVar(&opts.OSSPath, "oss-path", env.Getenv("OSS_PATH"), "OSS 上传路径，默认 latest-report.png（可用 --oss-path 或 OSS_PATH 覆盖）")
	flags.BoolVar(&opts.NoUpload, "no-upload", false, "只生成本地图片，不上传 OSS")
	flags.StringVar(&opts.ChromePath, "chrome", "", "Chrome/Edge 可执行文件路径，默认自动查找")
	return cmd
}

func runShot(ctx context.Context, env *Env, opts shotOptions) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	result, err := shot.Capture(ctx, shot.Options{
		ReportsDir: filepath.Join(env.Root, "data", "reports"),
		Report:     opts.Report,
		Width:      opts.Width,
		Scale:      opts.Scale,
		ChromePath: opts.ChromePath,
	})
	if err != nil {
		return err
	}

	outPath := env.abs(opts.Out)
	if outPath == "" {
		outPath = filepath.Join(env.Root, "data", "latest-report.png")
	}
	if err := fsutil.WriteFileAtomic(outPath, result.PNG, 0o644); err != nil {
		return fmt.Errorf("写入 PNG: %w", err)
	}
	fmt.Fprintf(env.Stdout, "已生成 %s（%d×%d，报告 %s，%d KB）\n",
		outPath, result.Width, result.Height, result.Slug, len(result.PNG)/1024)

	if opts.NoUpload {
		return nil
	}
	cfg, err := shot.LoadOSSConfig(env.Root)
	if err != nil {
		// OSS 是可选的自用步骤：未配置时只保留本地 PNG，不影响本次执行。
		fmt.Fprintf(env.Stdout, "未配置 OSS，跳过上传（本地图片已生成）：%v\n", err)
		return nil
	}
	objectPath := strings.TrimSpace(opts.OSSPath)
	if objectPath == "" {
		objectPath = cfg.Path
	}
	if objectPath == "" {
		objectPath = "latest-report.png"
	}
	publicURL, err := shot.Upload(cfg, objectPath, result.PNG)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "已上传（覆盖）%s\n", publicURL)
	return nil
}
