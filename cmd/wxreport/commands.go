package main

import (
	"context"
	"fmt"
	"io"
)

// command 描述一个子命令。run 返回 error，由 runMain 统一转成退出码。
type command struct {
	name string
	run  func(ctx context.Context, env *Env, args []string) error
}

// commandList 保持 usage 中的展示顺序。
var commandList = []command{
	{name: "refresh-db", run: runRefreshDB},
	{name: "export", run: runExport},
	{name: "names", run: runNames},
	{name: "resources", run: runResources},
	{name: "state-plan", run: runStatePlan},
	{name: "state-apply", run: runStateApply},
	{name: "report", run: runReport},
	{name: "rank", run: runRank},
	{name: "shot", run: runShot},
}

var commandByName = func() map[string]command {
	m := make(map[string]command, len(commandList))
	for _, cmd := range commandList {
		m[cmd.name] = cmd
	}
	return m
}()

func usage(w io.Writer) {
	fmt.Fprint(w, `wxreport

用法:
  wxreport refresh-db       刷新 stories-in-wx 的 DB 解密缓存（查询前先跑）
  wxreport export --chat ID --start UNIX --end UNIX --out FILE
                            导出窗口内消息（含昵称、时间、类型）供话题聚类
  wxreport names --chat ID  输出 sender -> 群内昵称/微信昵称 映射（排除备注）
  wxreport resources --chat ID 输出最近 24h 的网址/卡片资源
  wxreport state-plan --export FILE --state FILE --out FILE
                            生成增量总结输入（上一轮话题 + 新消息 + 尾部消息）
  wxreport state-apply --state FILE --plan FILE --update FILE --export FILE
                            合并模型输出的增量状态
  wxreport report --state FILE --export FILE --out data/reports/x.md
                            从状态直接渲染并排序最终报告
  wxreport rank <report.md> 按有效参与人数重排话题并保留 TOP10（--dry-run 预览）
  wxreport shot             生成长图（配置了 OSS 则覆盖上传，未配置自动跳过；--no-upload 强制只生成本地图）

示例:
  go run ./cmd/wxreport rank --dry-run data/reports/20261123230000.md
  go run ./cmd/wxreport shot --no-upload
  go run ./cmd/wxreport shot --scale 3 --oss-path latest-report.png
`)
}
