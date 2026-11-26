// Package summarize 调用 OpenAI 兼容的 Chat Completions 接口，把增量计划归纳成状态差异。
package summarize

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	reportstate "wxreport/internal/state"
)

//go:embed prompt.md
var systemPrompt string

const maxAttempts = 3

// Config 是 OpenAI 兼容接口的配置。
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

// ConfigFromEnv 读取 OPENAI_BASE_URL / OPENAI_API_KEY / OPENAI_MODEL。
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		BaseURL: strings.TrimSpace(getenv("OPENAI_BASE_URL")),
		APIKey:  strings.TrimSpace(getenv("OPENAI_API_KEY")),
		Model:   strings.TrimSpace(getenv("OPENAI_MODEL")),
	}
	var missing []string
	if cfg.APIKey == "" {
		missing = append(missing, "OPENAI_API_KEY")
	}
	if cfg.Model == "" {
		missing = append(missing, "OPENAI_MODEL")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("缺少 %s；请设置环境变量或写入 .env", strings.Join(missing, " / "))
	}
	return cfg, nil
}

// Summarizer 把增量计划归纳成状态差异。
type Summarizer interface {
	Summarize(ctx context.Context, plan *reportstate.Plan) (*reportstate.Update, error)
}

// Client 是 OpenAI 兼容接口的 Summarizer 实现。
type Client struct {
	api   openai.Client
	model string
}

// New 创建客户端；BaseURL 为空时使用 OpenAI 官方地址。
func New(cfg Config) *Client {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &Client{api: openai.NewClient(opts...), model: cfg.Model}
}

// Summarize 调用模型生成差异 JSON；解析失败时把错误反馈给模型重试。
func (c *Client) Summarize(ctx context.Context, plan *reportstate.Plan) (*reportstate.Update, error) {
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("序列化计划: %w", err)
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
		openai.UserMessage("增量计划：\n" + string(planJSON)),
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resp, err := c.api.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Model:    openai.ChatModel(c.model),
			Messages: messages,
		})
		if err != nil {
			return nil, fmt.Errorf("调用模型: %w", err)
		}
		if len(resp.Choices) == 0 {
			lastErr = errors.New("模型没有返回内容")
			continue
		}

		content := resp.Choices[0].Message.Content
		update, parseErr := parseUpdate(content)
		if parseErr == nil {
			return update, nil
		}
		lastErr = parseErr
		messages = append(messages,
			openai.AssistantMessage(content),
			openai.UserMessage(fmt.Sprintf("上面的输出无法解析：%v。请只输出合法 JSON，不要 Markdown 代码块或解释。", parseErr)),
		)
	}
	return nil, fmt.Errorf("模型输出解析失败（已尝试 %d 次）: %w", maxAttempts, lastErr)
}

// parseUpdate 容忍 Markdown 代码块和前后说明文字，只提取 JSON 对象。
func parseUpdate(content string) (*reportstate.Update, error) {
	text := strings.TrimSpace(content)
	if strings.HasPrefix(text, "```") {
		if newline := strings.Index(text, "\n"); newline >= 0 {
			text = text[newline+1:]
		}
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "```"))
	}
	if start := strings.Index(text, "{"); start >= 0 {
		if end := strings.LastIndex(text, "}"); end > start {
			text = text[start : end+1]
		}
	}

	var update reportstate.Update
	if err := json.Unmarshal([]byte(text), &update); err != nil {
		snippet := content
		if len(snippet) > 200 {
			snippet = snippet[:200] + "…"
		}
		return nil, fmt.Errorf("%w（输出开头：%s）", err, snippet)
	}
	return &update, nil
}
