package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"time"

	"wxreport/internal/siwx"
)

func runRefreshDB(ctx context.Context, env *Env, args []string) error {
	account := env.Getenv("SIWX_ACCOUNT")
	if account == "" {
		return errors.New("缺少账号：请设置 SIWX_ACCOUNT，或写入 config/local.env")
	}
	return siwx.Refresh(account)
}

func runExport(ctx context.Context, env *Env, args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	chat := fs.String("chat", "", "会话 username")
	account := fs.String("account", env.Getenv("SIWX_ACCOUNT"), "账号 wxid（默认取 SIWX_ACCOUNT）")
	start := fs.Int64("start", time.Now().Add(-24*time.Hour).Unix(), "开始时间（Unix 秒）")
	end := fs.Int64("end", time.Now().Unix(), "结束时间（Unix 秒）")
	out := fs.String("out", "data/wxmsgs.json", "输出 JSON 文件")
	if err := parseFlags(env, fs, args); err != nil {
		return err
	}

	*out = env.abs(*out)
	if *chat == "" {
		return errors.New("用法: wxreport export --chat xxx@chatroom --start UNIX --end UNIX --out data/wxmsgs.json")
	}
	if *account == "" {
		return errors.New("缺少账号：请设置 SIWX_ACCOUNT、写入 config/local.env，或传 --account")
	}
	stats, err := siwx.Export(*account, *chat, *start, *end, *out)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "消息总数：%d\n参与人数：%d\n已写入：%s\n", stats.Total, stats.Participants, stats.Output)
	return nil
}

func runNames(ctx context.Context, env *Env, args []string) error {
	fs := flag.NewFlagSet("names", flag.ContinueOnError)
	chat := fs.String("chat", "", "会话 username")
	account := fs.String("account", env.Getenv("SIWX_ACCOUNT"), "账号 wxid（默认取 SIWX_ACCOUNT）")
	if err := parseFlags(env, fs, args); err != nil {
		return err
	}

	if *chat == "" {
		return errors.New("用法: wxreport names --chat xxx@chatroom")
	}
	if *account == "" {
		return errors.New("缺少账号：请设置 SIWX_ACCOUNT、写入 config/local.env，或传 --account")
	}
	return siwx.Names(*account, *chat)
}

func runResources(ctx context.Context, env *Env, args []string) error {
	fs := flag.NewFlagSet("resources", flag.ContinueOnError)
	chat := fs.String("chat", "", "会话 username")
	account := fs.String("account", env.Getenv("SIWX_ACCOUNT"), "账号 wxid（默认取 SIWX_ACCOUNT）")
	if err := parseFlags(env, fs, args); err != nil {
		return err
	}

	if *chat == "" {
		return errors.New("用法: wxreport resources --chat xxx@chatroom")
	}
	if *account == "" {
		return errors.New("缺少账号：请设置 SIWX_ACCOUNT、写入 config/local.env，或传 --account")
	}
	return siwx.Resources(*account, *chat)
}
