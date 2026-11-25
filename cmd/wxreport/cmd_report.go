package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	reportstate "wxreport/internal/state"
)

func runReport(ctx context.Context, env *Env, args []string) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	statePath := fs.String("state", "data/topics.json", "增量状态文件")
	exportPath := fs.String("export", "data/wxmsgs.json", "wxreport export 生成的 JSON")
	out := fs.String("out", "", "输出报告 Markdown")
	chatName := fs.String("chat-name", "", "群名（报告标题用）")
	top := fs.Int("top", 10, "保留的话题数")
	if err := parseFlags(env, fs, args); err != nil {
		return err
	}

	if *out == "" {
		return errors.New("用法: wxreport report --out data/reports/xxx.md [--state ... --export ...]")
	}
	*statePath = env.abs(*statePath)
	*exportPath = env.abs(*exportPath)
	*out = env.abs(*out)

	st, err := reportstate.Load(*statePath)
	if err != nil {
		return err
	}
	export, err := loadExport(*exportPath)
	if err != nil {
		return err
	}
	markdown, result, err := reportstate.Render(st, export, reportstate.RenderOptions{
		ChatName: *chatName,
		Top:      *top,
	})
	if err != nil {
		return err
	}
	if err := writeFileAtomic(*out, markdown); err != nil {
		return err
	}
	printRanking(env.Stdout, result)
	fmt.Fprintf(env.Stdout, "已写入：%s\n", *out)
	return nil
}
