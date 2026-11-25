package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"wxreport/internal/siwx"
)

func newRefreshDBCmd(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "refresh-db",
		Short: "刷新 stories-in-wx 的 DB 解密缓存（查询前先跑）",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRefreshDB(cmd.Context(), env)
		},
	}
}

func runRefreshDB(ctx context.Context, env *Env) error {
	account := env.Getenv("SIWX_ACCOUNT")
	if account == "" {
		return errors.New("缺少账号：请设置 SIWX_ACCOUNT，或写入 config/local.env")
	}
	return siwx.Refresh(account)
}

type exportOptions struct {
	Chat    string
	Account string
	Start   int64
	End     int64
	Out     string
}

func newExportCmd(env *Env) *cobra.Command {
	var opts exportOptions
	cmd := &cobra.Command{
		Use:   "export",
		Short: "导出窗口内消息（含昵称、时间、类型）",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd.Context(), env, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.Chat, "chat", "", "会话 username")
	flags.StringVar(&opts.Account, "account", env.Getenv("SIWX_ACCOUNT"), "账号 wxid（默认取 SIWX_ACCOUNT）")
	flags.Int64Var(&opts.Start, "start", time.Now().Add(-24*time.Hour).Unix(), "开始时间（Unix 秒）")
	flags.Int64Var(&opts.End, "end", time.Now().Unix(), "结束时间（Unix 秒）")
	flags.StringVar(&opts.Out, "out", "data/wxmsgs.json", "输出 JSON 文件")
	return cmd
}

func runExport(ctx context.Context, env *Env, opts exportOptions) error {
	out := env.abs(opts.Out)
	if opts.Chat == "" {
		return errors.New("用法: wxreport export --chat xxx@chatroom --start UNIX --end UNIX --out data/wxmsgs.json")
	}
	if opts.Account == "" {
		return errors.New("缺少账号：请设置 SIWX_ACCOUNT、写入 config/local.env，或传 --account")
	}
	stats, err := siwx.Export(opts.Account, opts.Chat, opts.Start, opts.End, out)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "消息总数：%d\n参与人数：%d\n已写入：%s\n", stats.Total, stats.Participants, stats.Output)
	return nil
}

func newNamesCmd(env *Env) *cobra.Command {
	var chat, account string
	cmd := &cobra.Command{
		Use:   "names",
		Short: "输出 sender -> 群内昵称/微信昵称 映射（排除备注）",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNames(cmd.Context(), env, chat, account)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&chat, "chat", "", "会话 username")
	flags.StringVar(&account, "account", env.Getenv("SIWX_ACCOUNT"), "账号 wxid（默认取 SIWX_ACCOUNT）")
	return cmd
}

func runNames(ctx context.Context, env *Env, chat, account string) error {
	if chat == "" {
		return errors.New("用法: wxreport names --chat xxx@chatroom")
	}
	if account == "" {
		return errors.New("缺少账号：请设置 SIWX_ACCOUNT、写入 config/local.env，或传 --account")
	}
	return siwx.Names(account, chat)
}

func newResourcesCmd(env *Env) *cobra.Command {
	var chat, account string
	cmd := &cobra.Command{
		Use:   "resources",
		Short: "输出最近 24h 的网址/卡片资源",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResources(cmd.Context(), env, chat, account)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&chat, "chat", "", "会话 username")
	flags.StringVar(&account, "account", env.Getenv("SIWX_ACCOUNT"), "账号 wxid（默认取 SIWX_ACCOUNT）")
	return cmd
}

func runResources(ctx context.Context, env *Env, chat, account string) error {
	if chat == "" {
		return errors.New("用法: wxreport resources --chat xxx@chatroom")
	}
	if account == "" {
		return errors.New("缺少账号：请设置 SIWX_ACCOUNT、写入 config/local.env，或传 --account")
	}
	return siwx.Resources(account, chat)
}
