package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	reportstate "wxreport/internal/state"
)

type reportOptions struct {
	State    string
	Export   string
	Out      string
	ChatName string
	Top      int
}

func newReportCmd(env *Env) *cobra.Command {
	var opts reportOptions
	cmd := &cobra.Command{
		Use:   "report",
		Short: "从状态直接渲染并排序最终报告",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd.Context(), env, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.State, "state", "data/topics.json", "增量状态文件")
	flags.StringVar(&opts.Export, "export", "data/wxmsgs.json", "wxreport export 生成的 JSON")
	flags.StringVar(&opts.Out, "out", "", "输出报告 Markdown")
	flags.StringVar(&opts.ChatName, "chat-name", "", "群名（报告标题用）")
	flags.IntVar(&opts.Top, "top", 10, "保留的话题数")
	return cmd
}

func runReport(ctx context.Context, env *Env, opts reportOptions) error {
	if opts.Out == "" {
		return errors.New("用法: wxreport report --out data/reports/xxx.md [--state ... --export ...]")
	}
	statePath := env.abs(opts.State)
	exportPath := env.abs(opts.Export)
	out := env.abs(opts.Out)

	st, err := reportstate.Load(statePath)
	if err != nil {
		return err
	}
	export, err := loadExport(exportPath)
	if err != nil {
		return err
	}
	markdown, result, err := reportstate.Render(st, export, reportstate.RenderOptions{
		ChatName: opts.ChatName,
		Top:      opts.Top,
	})
	if err != nil {
		return err
	}
	if err := writeFileAtomic(out, markdown); err != nil {
		return err
	}
	printRanking(env.Stdout, result)
	fmt.Fprintf(env.Stdout, "已写入：%s\n", out)
	return nil
}
