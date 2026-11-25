// wxreport 生成微信群 24h 话题报告所需的辅助命令。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wxreport/internal/rank"
	"wxreport/internal/shot"
	"wxreport/internal/siwx"
	reportstate "wxreport/internal/state"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	root, err := findRoot()
	if err != nil {
		fatal(err)
	}
	loadLocalEnv(root)

	switch os.Args[1] {
	case "refresh-db":
		account := os.Getenv("SIWX_ACCOUNT")
		if account == "" {
			fatal(errors.New("缺少账号：请设置 SIWX_ACCOUNT，或写入 config/local.env"))
		}
		if err := siwx.Refresh(account); err != nil {
			fatal(err)
		}
	case "export":
		flags := flag.NewFlagSet("export", flag.ExitOnError)
		chat := flags.String("chat", "", "会话 username")
		account := flags.String("account", os.Getenv("SIWX_ACCOUNT"), "账号 wxid（默认取 SIWX_ACCOUNT）")
		start := flags.Int64("start", time.Now().Add(-24*time.Hour).Unix(), "开始时间（Unix 秒）")
		end := flags.Int64("end", time.Now().Unix(), "结束时间（Unix 秒）")
		out := flags.String("out", "/tmp/wxmsgs.json", "输出 JSON 文件")
		_ = flags.Parse(os.Args[2:])
		if *chat == "" {
			fatal(errors.New("用法: wxreport export --chat xxx@chatroom --start UNIX --end UNIX --out /tmp/wxmsgs.json"))
		}
		if *account == "" {
			fatal(errors.New("缺少账号：请设置 SIWX_ACCOUNT、写入 config/local.env，或传 --account"))
		}
		stats, err := siwx.Export(*account, *chat, *start, *end, *out)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("消息总数：%d\n参与人数：%d\n已写入：%s\n", stats.Total, stats.Participants, stats.Output)
	case "names":
		flags := flag.NewFlagSet("names", flag.ExitOnError)
		chat := flags.String("chat", "", "会话 username")
		account := flags.String("account", os.Getenv("SIWX_ACCOUNT"), "账号 wxid（默认取 SIWX_ACCOUNT）")
		_ = flags.Parse(os.Args[2:])
		if *chat == "" {
			fatal(errors.New("用法: wxreport names --chat xxx@chatroom"))
		}
		if *account == "" {
			fatal(errors.New("缺少账号：请设置 SIWX_ACCOUNT、写入 config/local.env，或传 --account"))
		}
		if err := siwx.Names(*account, *chat); err != nil {
			fatal(err)
		}
	case "resources":
		flags := flag.NewFlagSet("resources", flag.ExitOnError)
		chat := flags.String("chat", "", "会话 username")
		account := flags.String("account", os.Getenv("SIWX_ACCOUNT"), "账号 wxid（默认取 SIWX_ACCOUNT）")
		_ = flags.Parse(os.Args[2:])
		if *chat == "" {
			fatal(errors.New("用法: wxreport resources --chat xxx@chatroom"))
		}
		if *account == "" {
			fatal(errors.New("缺少账号：请设置 SIWX_ACCOUNT、写入 config/local.env，或传 --account"))
		}
		if err := siwx.Resources(*account, *chat); err != nil {
			fatal(err)
		}
	case "state-plan":
		flags := flag.NewFlagSet("state-plan", flag.ExitOnError)
		exportPath := flags.String("export", "/tmp/wxmsgs.json", "wxreport export 生成的 JSON")
		statePath := flags.String("state", "state/topics.json", "增量状态文件")
		out := flags.String("out", "/tmp/wxstate-plan.json", "输出给模型的计划 JSON")
		chatName := flags.String("chat-name", "", "群名（报告标题用）")
		tailHours := flags.Int("tail-hours", 6, "尾部未归属消息回看小时数")
		_ = flags.Parse(os.Args[2:])
		resolveRootPath(root, exportPath)
		resolveRootPath(root, statePath)
		resolveRootPath(root, out)
		st, err := reportstate.Load(*statePath)
		if err != nil {
			fatal(err)
		}
		export, err := loadExport(*exportPath)
		if err != nil {
			fatal(err)
		}
		plan, err := reportstate.BuildPlan(st, export, *tailHours, *chatName)
		if err != nil {
			fatal(err)
		}
		if err := reportstate.WritePlan(*out, plan); err != nil {
			fatal(err)
		}
		mode := "增量"
		if plan.Bootstrap {
			mode = "bootstrap（首次全量）"
		}
		fmt.Printf("模式：%s\n", mode)
		fmt.Printf("上一轮话题：%d（活跃 %d）\n", len(st.Topics), len(plan.Topics))
		fmt.Printf("新消息：%d，尾部未归属：%d\n", len(plan.NewMessages), len(plan.TailMessages))
		fmt.Printf("已写入：%s\n", *out)
	case "state-apply":
		flags := flag.NewFlagSet("state-apply", flag.ExitOnError)
		statePath := flags.String("state", "state/topics.json", "增量状态文件")
		planPath := flags.String("plan", "/tmp/wxstate-plan.json", "state-plan 生成的计划")
		updatePath := flags.String("update", "/tmp/wxstate-update.json", "模型输出的差异 JSON")
		exportPath := flags.String("export", "/tmp/wxmsgs.json", "wxreport export 生成的 JSON")
		_ = flags.Parse(os.Args[2:])
		resolveRootPath(root, statePath)
		resolveRootPath(root, planPath)
		resolveRootPath(root, updatePath)
		resolveRootPath(root, exportPath)
		st, err := reportstate.Load(*statePath)
		if err != nil {
			fatal(err)
		}
		var plan reportstate.Plan
		if err := readJSON(*planPath, &plan); err != nil {
			fatal(err)
		}
		var update reportstate.Update
		if err := readJSON(*updatePath, &update); err != nil {
			fatal(err)
		}
		export, err := loadExport(*exportPath)
		if err != nil {
			fatal(err)
		}
		next, warnings, err := reportstate.Apply(st, &plan, &update, export)
		if err != nil {
			fatal(err)
		}
		if err := reportstate.Save(*statePath, next); err != nil {
			fatal(err)
		}
		for _, warning := range warnings {
			fmt.Println("警告:", warning)
		}
		fmt.Printf("状态已更新：%d 个话题，窗口结束 %s\n",
			len(next.Topics), time.Unix(next.WindowEnd, 0).Format("2006-01-02 15:04"))
	case "report":
		flags := flag.NewFlagSet("report", flag.ExitOnError)
		statePath := flags.String("state", "state/topics.json", "增量状态文件")
		exportPath := flags.String("export", "/tmp/wxmsgs.json", "wxreport export 生成的 JSON")
		out := flags.String("out", "", "输出报告 Markdown")
		chatName := flags.String("chat-name", "", "群名（报告标题用）")
		top := flags.Int("top", 10, "保留的话题数")
		_ = flags.Parse(os.Args[2:])
		if *out == "" {
			fatal(errors.New("用法: wxreport report --out reports/xxx.md [--state ... --export ...]"))
		}
		resolveRootPath(root, statePath)
		resolveRootPath(root, exportPath)
		resolveRootPath(root, out)
		st, err := reportstate.Load(*statePath)
		if err != nil {
			fatal(err)
		}
		export, err := loadExport(*exportPath)
		if err != nil {
			fatal(err)
		}
		markdown, result, err := reportstate.Render(st, export, reportstate.RenderOptions{
			ChatName: *chatName,
			Top:      *top,
		})
		if err != nil {
			fatal(err)
		}
		if err := writeFileAtomic(*out, markdown); err != nil {
			fatal(err)
		}
		printRanking(result)
		fmt.Printf("已写入：%s\n", *out)
	case "rank":
		flags := flag.NewFlagSet("rank", flag.ExitOnError)
		top := flags.Int("top", 10, "保留的话题数（<=0 表示全部保留）")
		dryRun := flags.Bool("dry-run", false, "只打印排序结果，不写回文件")
		messagesPath := flags.String("messages", "", "wxreport export 生成的 JSON（提供后按话轮模型排序）")
		_ = flags.Parse(os.Args[2:])
		if flags.NArg() != 1 {
			fatal(errors.New("用法: wxreport rank [--messages /tmp/wxmsgs.json] <report.md> [--top 10] [--dry-run]"))
		}
		path := flags.Arg(0)
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		if *messagesPath != "" && !filepath.IsAbs(*messagesPath) {
			*messagesPath = filepath.Join(root, *messagesPath)
		}
		if err := rankFile(path, *messagesPath, *top, *dryRun); err != nil {
			fatal(err)
		}
	case "shot":
		flags := flag.NewFlagSet("shot", flag.ExitOnError)
		reportID := flags.String("report", "", "报告 slug/文件名，默认最新一篇")
		width := flags.Int("width", shot.DefaultWidth, "CSS 视口宽度")
		scale := flags.Int("scale", shot.DefaultScale, "图片倍率（2 或 3）")
		out := flags.String("out", "", "本地 PNG 路径，默认 runs/latest.png")
		key := flags.String("key", os.Getenv("OSS_KEY"), "OSS 对象 key，默认 wxreport/latest.png（可用 OSS_KEY 覆盖）")
		noUpload := flags.Bool("no-upload", false, "只生成本地图片，不上传 OSS")
		chromePath := flags.String("chrome", "", "Chrome/Edge 可执行文件路径，默认自动查找")
		_ = flags.Parse(os.Args[2:])

		if strings.TrimSpace(*key) == "" {
			*key = "wxreport/latest.png"
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		result, err := shot.Capture(ctx, shot.Options{
			ReportsDir: filepath.Join(root, "reports"),
			Report:     *reportID,
			Width:      *width,
			Scale:      *scale,
			ChromePath: *chromePath,
		})
		if err != nil {
			fatal(err)
		}

		outPath := *out
		if outPath == "" {
			outPath = filepath.Join(root, "runs", "latest.png")
		} else if !filepath.IsAbs(outPath) {
			outPath = filepath.Join(root, outPath)
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			fatal(fmt.Errorf("创建输出目录: %w", err))
		}
		if err := os.WriteFile(outPath, result.PNG, 0o644); err != nil {
			fatal(fmt.Errorf("写入 PNG: %w", err))
		}
		fmt.Printf("已生成 %s（%d×%d，报告 %s，%d KB）\n",
			outPath, result.Width, result.Height, result.Slug, len(result.PNG)/1024)

		if !*noUpload {
			cfg, err := shot.LoadOSSConfig(root)
			if err != nil {
				// OSS 是可选的自用步骤：未配置时只保留本地 PNG，不影响本次执行。
				fmt.Printf("未配置 OSS，跳过上传（本地图片已生成）：%v\n", err)
			} else {
				publicURL, err := shot.Upload(cfg, *key, result.PNG)
				if err != nil {
					fatal(err)
				}
				fmt.Printf("已上传（覆盖）%s\n", publicURL)
			}
		}
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
}

// rankFile 按话轮模型（或消息数回退模型）重排话题并保留 top 个。
func rankFile(path, messagesPath string, top int, dryRun bool) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取报告: %w", err)
	}

	var output string
	var result rank.Result
	if messagesPath != "" {
		export, err := loadExport(messagesPath)
		if err != nil {
			return err
		}
		output, result, err = rank.ProcessWithMessages(string(raw), top, export)
		if err != nil {
			return fmt.Errorf("排序 %s: %w", filepath.Base(path), err)
		}
	} else {
		output, result, err = rank.Process(string(raw), top)
		if err != nil {
			return fmt.Errorf("排序 %s: %w", filepath.Base(path), err)
		}
	}

	printRanking(result)

	if dryRun {
		fmt.Println("dry-run：未写回文件。")
		return nil
	}
	if output == string(raw) {
		fmt.Println("顺序无需调整。")
		return nil
	}

	if err := writeFileAtomic(path, output); err != nil {
		return err
	}
	fmt.Printf("已写回 %s。\n", filepath.Base(path))
	return nil
}

func resolveRootPath(root string, value *string) {
	if *value == "" || filepath.IsAbs(*value) {
		return
	}
	*value = filepath.Join(root, *value)
}

func readJSON(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 %s: %w", filepath.Base(path), err)
	}
	text := strings.TrimSpace(string(raw))
	// 模型有时会把 JSON 包在 Markdown 代码块或说明文字里，这里做一次容错。
	if strings.HasPrefix(text, "```") {
		if newline := strings.Index(text, "\n"); newline >= 0 {
			text = strings.TrimSpace(text[newline+1:])
		}
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "```"))
	}
	if start := strings.Index(text, "{"); start >= 0 {
		if end := strings.LastIndex(text, "}"); end > start {
			text = text[start : end+1]
		}
	}
	if err := json.Unmarshal([]byte(text), target); err != nil {
		return fmt.Errorf("解析 %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录: %w", err)
	}
	return writeFileAtomic(path, string(append(raw, '\n')))
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

func printRanking(result rank.Result) {
	for _, warning := range result.Warnings {
		fmt.Println("警告:", warning)
	}
	if result.Mode == "turns" {
		fmt.Println("话题排序（话轮模型：0.45×有效参与人数 + 0.40×文字话轮 + 0.15×密集度）：")
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
			fmt.Printf("%2d. %.4f  话轮 %3d  有效 %5.2f  人数 %3d  消息 %4d  文字 %.2f  密集 %.2f  [%s] %s\n",
				i+1, topic.Score, topic.Turns, topic.Effective, len(topic.Participants), topic.Messages,
				textRatio, denseRatio, status, topic.Title)
		}
	} else {
		fmt.Println("话题排序（有效参与人数 × log2(2 + 消息数)）：")
		for i, topic := range result.Topics {
			status := "保留"
			if !topic.Kept {
				status = "截断"
			}
			fmt.Printf("%2d. %6.2f  消息 %4d  人数 %3d  有效 %5.2f  [%s] %s\n",
				i+1, topic.Score, topic.Messages, len(topic.Participants), topic.Effective, status, topic.Title)
		}
	}
	fmt.Printf("共 %d 个话题，保留 %d 个。\n", result.Total, result.Kept)
}

func writeFileAtomic(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("写回报告: %w", err)
	}
	return nil
}

// loadLocalEnv 读取项目根目录的 config/local.env（KEY=VALUE），用于本机私有配置。
// 已存在的环境变量优先；该文件只存在于本机，已被 .gitignore 忽略。
func loadLocalEnv(root string) {
	raw, err := os.ReadFile(filepath.Join(root, "config", "local.env"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("找不到 go.mod，请在项目目录内运行")
		}
		dir = parent
	}
}

func usage() {
	fmt.Println(`wxreport

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
  wxreport report --state FILE --export FILE --out reports/x.md
                            从状态直接渲染并排序最终报告
  wxreport rank <report.md> 按有效参与人数重排话题并保留 TOP10（--dry-run 预览）
  wxreport shot             生成长图（配置了 OSS 则覆盖上传，未配置自动跳过；--no-upload 强制只生成本地图）

示例:
  go run ./cmd/wxreport rank --dry-run reports/20261123230000.md
  go run ./cmd/wxreport shot --no-upload
  go run ./cmd/wxreport shot --scale 3 --key latest.png`)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "错误:", err)
	os.Exit(1)
}
