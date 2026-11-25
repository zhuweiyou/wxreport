package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	reportstate "wxreport/internal/state"
)

func runStatePlan(ctx context.Context, env *Env, args []string) error {
	fs := flag.NewFlagSet("state-plan", flag.ContinueOnError)
	exportPath := fs.String("export", "data/wxmsgs.json", "wxreport export 生成的 JSON")
	statePath := fs.String("state", "data/topics.json", "增量状态文件")
	out := fs.String("out", "data/wxstate-plan.json", "输出给模型的计划 JSON")
	chatName := fs.String("chat-name", "", "群名（报告标题用）")
	tailHours := fs.Int("tail-hours", 6, "尾部未归属消息回看小时数")
	if err := parseFlags(env, fs, args); err != nil {
		return err
	}

	*exportPath = env.abs(*exportPath)
	*statePath = env.abs(*statePath)
	*out = env.abs(*out)

	st, err := reportstate.Load(*statePath)
	if err != nil {
		return err
	}
	export, err := loadExport(*exportPath)
	if err != nil {
		return err
	}
	plan, err := reportstate.BuildPlan(st, export, *tailHours, *chatName)
	if err != nil {
		return err
	}
	if err := reportstate.WritePlan(*out, plan); err != nil {
		return err
	}

	mode := "增量"
	if plan.Bootstrap {
		mode = "bootstrap（首次全量）"
	}
	fmt.Fprintf(env.Stdout, "模式：%s\n", mode)
	fmt.Fprintf(env.Stdout, "上一轮话题：%d（活跃 %d）\n", len(st.Topics), len(plan.Topics))
	fmt.Fprintf(env.Stdout, "新消息：%d，尾部未归属：%d\n", len(plan.NewMessages), len(plan.TailMessages))
	fmt.Fprintf(env.Stdout, "已写入：%s\n", *out)
	return nil
}

func runStateApply(ctx context.Context, env *Env, args []string) error {
	fs := flag.NewFlagSet("state-apply", flag.ContinueOnError)
	statePath := fs.String("state", "data/topics.json", "增量状态文件")
	planPath := fs.String("plan", "data/wxstate-plan.json", "state-plan 生成的计划")
	updatePath := fs.String("update", "data/wxstate-update.json", "模型输出的差异 JSON")
	exportPath := fs.String("export", "data/wxmsgs.json", "wxreport export 生成的 JSON")
	if err := parseFlags(env, fs, args); err != nil {
		return err
	}

	*statePath = env.abs(*statePath)
	*planPath = env.abs(*planPath)
	*updatePath = env.abs(*updatePath)
	*exportPath = env.abs(*exportPath)

	st, err := reportstate.Load(*statePath)
	if err != nil {
		return err
	}
	var plan reportstate.Plan
	if err := readJSON(*planPath, &plan); err != nil {
		return err
	}
	var update reportstate.Update
	if err := readJSON(*updatePath, &update); err != nil {
		return err
	}
	export, err := loadExport(*exportPath)
	if err != nil {
		return err
	}
	next, warnings, err := reportstate.Apply(st, &plan, &update, export)
	if err != nil {
		return err
	}
	if err := reportstate.Save(*statePath, next); err != nil {
		return err
	}
	for _, warning := range warnings {
		fmt.Fprintln(env.Stdout, "警告:", warning)
	}
	fmt.Fprintf(env.Stdout, "状态已更新：%d 个话题，窗口结束 %s\n",
		len(next.Topics), time.Unix(next.WindowEnd, 0).Format("2006-01-02 15:04"))
	return nil
}
