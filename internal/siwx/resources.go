package siwx

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

var urlPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

type linkCard struct {
	Title string `json:"title"`
	Desc  string `json:"desc"`
	URL   string `json:"url"`
}

type resourceRow struct {
	TS    int64
	Title string
	URL   string
}

// Resources 输出最近 24 小时内消息中出现的网址/卡片资源。
// 输出 TSV：n、ts、title、url；不包含发送者，避免昵称/备注泄露。
func Resources(account, chat string) error {
	baseURL := strings.TrimRight(envOr("SIWX_BASE_URL", defaultBaseURL), "/")
	client := &http.Client{Timeout: 30 * time.Second}

	since := time.Now().Add(-24 * time.Hour).Unix()
	messages, err := fetchChatSince(client, baseURL, account, chat, since)
	if err != nil {
		return err
	}

	seen := make(map[string]bool)
	rows := make([]resourceRow, 0)
	for _, message := range messages {
		if message.TS < since {
			continue
		}
		card := decodeLink(message.Link)
		title := resourceTitle(message, card)
		for _, raw := range messageURLs(message, card) {
			link := cleanURL(raw)
			if link == "" || seen[link] {
				continue
			}
			seen[link] = true
			rows = append(rows, resourceRow{TS: message.TS, Title: title, URL: link})
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].TS == rows[j].TS {
			return rows[i].URL < rows[j].URL
		}
		return rows[i].TS < rows[j].TS
	})

	fmt.Println("n\tts\ttitle\turl")
	for index, row := range rows {
		fmt.Printf("%d\t%d\t%s\t%s\n", index+1, row.TS, row.Title, row.URL)
	}
	return nil
}

func decodeLink(raw json.RawMessage) linkCard {
	var card linkCard
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &card)
	}
	return card
}

func resourceTitle(message chatMessage, card linkCard) string {
	title := strings.TrimSpace(card.Title)
	if title == "" {
		title = strings.TrimSpace(strings.TrimPrefix(message.Text, "[链接]"))
	}
	if title == "" && len(message.Channels) > 0 && string(message.Channels) != "null" {
		var value any
		if json.Unmarshal(message.Channels, &value) == nil {
			title = firstString(value, "title", "desc", "nickname")
		}
	}
	if title == "" {
		for _, raw := range messageURLs(message, card) {
			if host := urlHost(raw); host != "" {
				title = host
				break
			}
		}
	}
	if title == "" {
		title = "资源"
	}
	return strings.Join(strings.Fields(title), " ")
}

func messageURLs(message chatMessage, card linkCard) []string {
	candidates := make([]string, 0)
	if card.URL != "" {
		candidates = append(candidates, card.URL)
	} else if card.Desc != "" {
		candidates = append(candidates, extractURLs(card.Desc)...)
	}
	candidates = append(candidates, extractURLs(message.Text)...)
	if len(message.Channels) > 0 && string(message.Channels) != "null" {
		var value any
		if json.Unmarshal(message.Channels, &value) == nil {
			candidates = append(candidates, collectURLs(value)...)
		}
	}
	return candidates
}

func extractURLs(text string) []string {
	return urlPattern.FindAllString(html.UnescapeString(text), -1)
}

func collectURLs(value any) []string {
	switch typed := value.(type) {
	case string:
		return extractURLs(typed)
	case []any:
		var urls []string
		for _, item := range typed {
			urls = append(urls, collectURLs(item)...)
		}
		return urls
	case map[string]any:
		var urls []string
		for _, item := range typed {
			urls = append(urls, collectURLs(item)...)
		}
		return urls
	default:
		return nil
	}
}

func firstString(value any, keys ...string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range keys {
			if text, ok := typed[key].(string); ok && strings.TrimSpace(text) != "" {
				return text
			}
		}
		for _, item := range typed {
			if text := firstString(item, keys...); text != "" {
				return text
			}
		}
	case []any:
		for _, item := range typed {
			if text := firstString(item, keys...); text != "" {
				return text
			}
		}
	}
	return ""
}

func cleanURL(raw string) string {
	value := strings.TrimSpace(html.UnescapeString(raw))
	value = strings.TrimRight(value, ".,;，。、）)]}】》\"'")
	if !strings.HasPrefix(value, "http") {
		return ""
	}
	return value
}

func urlHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Host
}
