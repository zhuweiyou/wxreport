package siwx

import (
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"strings"
)

var urlPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

type linkCard struct {
	Title string `json:"title"`
	Desc  string `json:"desc"`
	URL   string `json:"url"`
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
