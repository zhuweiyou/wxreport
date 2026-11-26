package summarize

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	reportstate "wxreport/internal/state"
)

func TestConfigFromEnv(t *testing.T) {
	getenv := func(key string) string {
		return map[string]string{
			"OPENAI_BASE_URL": "https://example.com/v1",
			"OPENAI_API_KEY":  "key",
			"OPENAI_MODEL":    "model",
		}[key]
	}
	cfg, err := ConfigFromEnv(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://example.com/v1" || cfg.APIKey != "key" || cfg.Model != "model" {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	if _, err := ConfigFromEnv(func(string) string { return "" }); err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestSummarizeRetriesInvalidJSON(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		content := "这不是 JSON"
		if calls > 1 {
			content = "```json\n{\"assignments\":[],\"updates\":[],\"new_topics\":[],\"drop\":[]}\n```"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 0,
			"model":   "test",
			"choices": []map[string]any{{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": content},
				"finish_reason": "stop",
			}},
		})
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL + "/v1", APIKey: "test", Model: "test"})
	update, err := client.Summarize(context.Background(), &reportstate.Plan{})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if update == nil || len(update.NewTopics) != 0 {
		t.Fatalf("unexpected update: %+v", update)
	}
}
