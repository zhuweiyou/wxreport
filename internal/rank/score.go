package rank

import (
	"errors"
	"fmt"
	"sort"
)

// InputTopic 是结构化排序的输入：话题 ID、标题和消息编号。
type InputTopic struct {
	ID             string
	Title          string
	MessageIndices []int
}

// ScoredTopic 是排序后的单个话题指标。
type ScoredTopic struct {
	ID           string
	Title        string
	Participants []Participant
	Messages     int
	Turns        int
	TextTurns    int
	DenseTurns   int
	Effective    float64
	Score        float64
}

// Score 用消息明细计算话题指标，并按分数从高到低排序。
// 同一消息只归属第一个引用它的话题，重复引用会产生警告。
func Score(inputs []InputTopic, export *Export) ([]ScoredTopic, []string, error) {
	if export == nil || len(export.Messages) == 0 {
		return nil, nil, errors.New("消息导出为空，无法按话轮排序")
	}

	byIndex := make(map[int]Message, len(export.Messages))
	for _, message := range export.Messages {
		if message.Index <= 0 {
			return nil, nil, fmt.Errorf("消息导出里有无效编号 %d", message.Index)
		}
		byIndex[message.Index] = message
	}

	assigned := make(map[int]int)
	var warnings []string
	topicMessages := make([][]Message, len(inputs))
	for i, input := range inputs {
		if len(input.MessageIndices) == 0 {
			return nil, nil, fmt.Errorf("话题「%s」没有消息", input.Title)
		}
		messages := make([]Message, 0, len(input.MessageIndices))
		for _, ref := range input.MessageIndices {
			message, ok := byIndex[ref]
			if !ok {
				warnings = append(warnings,
					fmt.Sprintf("话题「%s」引用了不存在的消息 %d，已忽略", input.Title, ref))
				continue
			}
			if owner, ok := assigned[ref]; ok {
				warnings = append(warnings,
					fmt.Sprintf("消息 %d 同时出现在话题「%s」和「%s」，按前者计算", ref, inputs[owner].Title, input.Title))
				continue
			}
			assigned[ref] = i
			messages = append(messages, message)
		}
		if len(messages) == 0 {
			return nil, nil, fmt.Errorf("话题「%s」没有可用的消息", input.Title)
		}
		topicMessages[i] = messages
	}

	metricsList := computeTopicMetrics(topicMessages)
	scored := make([]ScoredTopic, len(inputs))
	var sumEffective, sumTurns float64
	for i, input := range inputs {
		metrics := metricsList[i]
		scored[i] = ScoredTopic{
			ID:           input.ID,
			Title:        input.Title,
			Participants: metrics.participants,
			Messages:     len(topicMessages[i]),
			Turns:        metrics.turns,
			TextTurns:    metrics.textTurns,
			DenseTurns:   metrics.denseTurns,
			Effective:    metrics.effective,
		}
		sumEffective += metrics.effective
		sumTurns += float64(metrics.turns)
	}
	for i := range scored {
		if sumEffective <= 0 || sumTurns <= 0 {
			scored[i].Score = 0
			continue
		}
		textRatio := 0.0
		denseRatio := 0.0
		if scored[i].Turns > 0 {
			textRatio = float64(scored[i].TextTurns) / float64(scored[i].Turns)
			denseRatio = float64(scored[i].DenseTurns) / float64(scored[i].Turns)
		}
		scored[i].Score = 0.45*(scored[i].Effective/sumEffective) +
			0.40*textRatio*(float64(scored[i].Turns)/sumTurns) +
			0.15*denseRatio
	}

	sort.SliceStable(scored, func(i, j int) bool {
		a, b := scored[i], scored[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Effective != b.Effective {
			return a.Effective > b.Effective
		}
		if a.Turns != b.Turns {
			return a.Turns > b.Turns
		}
		if a.Messages != b.Messages {
			return a.Messages > b.Messages
		}
		return a.Title < b.Title
	})
	return scored, warnings, nil
}
