package main

import (
	"fmt"
	"io"

	"wxreport/internal/rank"
)

func printRanking(w io.Writer, result rank.Result) {
	for _, warning := range result.Warnings {
		fmt.Fprintln(w, "警告:", warning)
	}
	if result.Mode == "turns" {
		fmt.Fprintln(w, "话题排序（话轮模型：0.45×有效参与人数 + 0.40×文字话轮 + 0.15×密集度）：")
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
			fmt.Fprintf(w, "%2d. %.4f  话轮 %3d  有效 %5.2f  人数 %3d  消息 %4d  文字 %.2f  密集 %.2f  [%s] %s\n",
				i+1, topic.Score, topic.Turns, topic.Effective, len(topic.Participants), topic.Messages,
				textRatio, denseRatio, status, topic.Title)
		}
	} else {
		fmt.Fprintln(w, "话题排序（有效参与人数 × log2(2 + 消息数)）：")
		for i, topic := range result.Topics {
			status := "保留"
			if !topic.Kept {
				status = "截断"
			}
			fmt.Fprintf(w, "%2d. %6.2f  消息 %4d  人数 %3d  有效 %5.2f  [%s] %s\n",
				i+1, topic.Score, topic.Messages, len(topic.Participants), topic.Effective, status, topic.Title)
		}
	}
	fmt.Fprintf(w, "共 %d 个话题，保留 %d 个。\n", result.Total, result.Kept)
}
