package openai

import "testing"

func TestApplyUsageFallbackFillsMissingUsage(t *testing.T) {
	prompt, completion, total := applyUsageFallback(0, 0, 0, 12, 8)
	if prompt != 12 || completion != 8 || total != 20 {
		t.Fatalf("fallback usage = (%d,%d,%d), want (12,8,20)", prompt, completion, total)
	}
}

func TestEstimateOpenAIInputTokensUsesMessagesAndTools(t *testing.T) {
	tokens := estimateOpenAIInputTokens(
		[]map[string]any{
			{"role": "system", "content": "你是助手"},
			{"role": "user", "content": []any{map[string]any{"type": "text", "text": "你好，帮我总结一下日志"}}},
		},
		[]map[string]any{{"type": "function", "function": map[string]any{"name": "search_logs"}}},
		map[string]any{"type": "auto"},
	)
	if tokens <= 0 {
		t.Fatalf("tokens = %d, want > 0", tokens)
	}
}
