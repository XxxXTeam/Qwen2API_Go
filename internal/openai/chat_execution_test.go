package openai

import (
	"strings"
	"testing"

	"qwen2api/internal/toolcall"
)

func TestNormalizeMessagesKeepsToolReminderNearLatestTurn(t *testing.T) {
	injected := toolcall.InjectPrompt([]map[string]any{
		{"role": "system", "content": "你是一个助手"},
		{"role": "user", "content": strings.Repeat("历史问题;", 200)},
		{"role": "assistant", "content": strings.Repeat("历史回答;", 200)},
		{"role": "user", "content": "现在请查询天气"},
	}, []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "weather_lookup",
				"description": "query weather",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		},
	}, "auto")
	normalized := normalizeMessages(cloneMessageList(injected.Messages), "t2t", false)
	if len(normalized) != 1 {
		t.Fatalf("normalized len = %d, want 1", len(normalized))
	}

	content := extractText(normalized[0]["content"])
	for _, snippet := range []string{
		"[ml_tool reminder]",
		"Allowed ml_tool names: weather_lookup.",
		"Ignore built-in/native/platform tools.",
	} {
		if !strings.Contains(content, snippet) {
			t.Fatalf("upstream content missing %q", snippet)
		}
	}

	if !strings.Contains(content, "现在请查询天气") {
		t.Fatalf("upstream content missing latest user turn")
	}
}
