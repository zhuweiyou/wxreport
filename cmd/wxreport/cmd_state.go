package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	reportstate "wxreport/internal/state"
)

type statePlanOptions struct {
	Export    string
	State     string
	Out       string
	ChatName  string
	TailHours int
}

func newStatePlanCmd(env *Env) *cobra.Command {
	var opts statePlanOptions
	cmd := &cobra.Command{
		Use:   "state-plan",
		Short: "生成增量总结输入（上一轮话题 + 新消息 + 尾部消息）",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatePlan(cmd.Context(), env, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.Export, "export", "data/wxmsgs.json", "wxreport export 生成的 JSON")
	flags.StringVar(&opts.State, "state", "data/topics.json", "增量状态文件")
	flags.StringVar(&opts.Out, "out", "data/wxstate-plan.json", "输出给模型的计划 JSON")
	flags.StringVar(&opts.ChatName, "chat-name", "", "群名（报告标题用）")
	flags.IntVar(&opts.TailHours, "tail-hours", 6, "尾部未归属消息回看小时数")
	return cmd
}

func runStatePlan(ctx context.Context, env *Env, opts statePlanOptions) error {
	exportPath := env.abs(opts.Export)
	statePath := env.abs(opts.State)
	out := env.abs(opts.Out)

	st, err := reportstate.Load(statePath)
	if err != nil {
		return err
	}
	export, err := loadExport(exportPath)
	if err != nil {
		return err
	}
	plan, err := reportstate.BuildPlan(st, export, opts.TailHours, opts.ChatName)
	if err != nil {
		return err
	}
	if err := reportstate.WritePlan(out, plan); err != nil {
		return err
	}

	mode := "增量"
	if plan.Bootstrap {
		mode = "bootstrap（首次全量）"
	}
	fmt.Fprintf(env.Stdout, "模式：%s\n", mode)
	fmt.Fprintf(env.Stdout, "上一轮话题：%d（活跃 %d）\n", len(st.Topics), len(plan.Topics))
	fmt.Fprintf(env.Stdout, "新消息：%d，尾部未归属：%d\n", len(plan.NewMessages), len(plan.TailMessages))
	fmt.Fprintf(env.Stdout, "已写入：%s\n", out)
	return nil
}

type stateApplyOptions struct {
	State  string
	Plan   string
	Update string
	Export string
}

func newStateApplyCmd(env *Env) *cobra.Command {
	var opts stateApplyOptions
	cmd := &cobra.Command{
		Use:   "state-apply",
		Short: "合并模型输出的增量状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStateApply(cmd.Context(), env, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.State, "state", "data/topics.json", "增量状态文件")
	flags.StringVar(&opts.Plan, "plan", "data/wxstate-plan.json", "state-plan 生成的计划")
	flags.StringVar(&opts.Update, "update", "data/wxstate-update.json", "模型输出的差异 JSON")
	flags.StringVar(&opts.Export, "export", "data/wxmsgs.json", "wxreport export 生成的 JSON")
	return cmd
}

func runStateApply(ctx context.Context, env *Env, opts stateApplyOptions) error {
	statePath := env.abs(opts.State)
	planPath := env.abs(opts.Plan)
	updatePath := env.abs(opts.Update)
	exportPath := env.abs(opts.Export)

	st, err := reportstate.Load(statePath)
	if err != nil {
		return err
	}
	var plan reportstate.Plan
	if err := readJSON(planPath, &plan); err != nil {
		return err
	}
	var update reportstate.Update
	if err := readJSON(updatePath, &update); err != nil {
		return err
	}
	export, err := loadExport(exportPath)
	if err != nil {
		return err
	}
	next, warnings, err := reportstate.Apply(st, &plan, &update, export)
	if err != nil {
		return err
	}
	if err := reportstate.Save(statePath, next); err != nil {
		return err
	}
	for _, warning := range warnings {
		fmt.Fprintln(env.Stdout, "警告:", warning)
	}
	fmt.Fprintf(env.Stdout, "状态已更新：%d 个话题，窗口结束 %s\n",
		len(next.Topics), time.Unix(next.WindowEnd, 0).Format("2006-01-02 15:04"))
	return nil
}
