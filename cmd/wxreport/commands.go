package main

import (
	"github.com/spf13/cobra"
)

func newRootCmd(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wxreport",
		Short: "生成微信群 24h 话题报告所需的辅助命令",
		Long: `wxreport 生成微信群 24h 话题报告所需的辅助命令。

典型流程：refresh-db -> export -> state-plan ->（模型总结）-> state-apply -> report -> shot`,
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.SetOut(env.Stdout)
	cmd.SetErr(env.Stderr)

	cmd.AddCommand(
		newRefreshDBCmd(env),
		newExportCmd(env),
		newNamesCmd(env),
		newResourcesCmd(env),
		newStatePlanCmd(env),
		newStateApplyCmd(env),
		newReportCmd(env),
		newRankCmd(env),
		newShotCmd(env),
	)
	return cmd
}
