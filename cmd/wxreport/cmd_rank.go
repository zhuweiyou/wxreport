package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"wxreport/internal/rank"
)

type rankOptions struct {
	Top      int
	DryRun   bool
	Messages string
}

func newRankCmd(env *Env) *cobra.Command {
	var opts rankOptions
	cmd := &cobra.Command{
		Use:   "rank <report.md>",
		Short: "按有效参与人数重排话题并保留 TOP10",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("用法: wxreport rank [--messages data/wxmsgs.json] <report.md> [--top 10] [--dry-run]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRank(cmd.Context(), env, args[0], opts)
		},
	}
	flags := cmd.Flags()
	flags.IntVar(&opts.Top, "top", 10, "保留的话题数（<=0 表示全部保留）")
	flags.BoolVar(&opts.DryRun, "dry-run", false, "只打印排序结果，不写回文件")
	flags.StringVar(&opts.Messages, "messages", "", "wxreport export 生成的 JSON（提供后按话轮模型排序）")
	return cmd
}

func runRank(ctx context.Context, env *Env, reportPath string, opts rankOptions) error {
	path := env.abs(reportPath)
	messagesPath := opts.Messages
	if messagesPath != "" {
		messagesPath = env.abs(messagesPath)
	}
	return rankFileTo(env.Stdout, path, messagesPath, opts.Top, opts.DryRun)
}

// rankFile 按话轮模型（或消息数回退模型）重排话题并保留 top 个。
func rankFile(path, messagesPath string, top int, dryRun bool) error {
	return rankFileTo(os.Stdout, path, messagesPath, top, dryRun)
}

func rankFileTo(w io.Writer, path, messagesPath string, top int, dryRun bool) error {
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

	printRanking(w, result)

	if dryRun {
		fmt.Fprintln(w, "dry-run：未写回文件。")
		return nil
	}
	if output == string(raw) {
		fmt.Fprintln(w, "顺序无需调整。")
		return nil
	}

	if err := writeFileAtomic(path, output); err != nil {
		return err
	}
	fmt.Fprintf(w, "已写回 %s。\n", filepath.Base(path))
	return nil
}
