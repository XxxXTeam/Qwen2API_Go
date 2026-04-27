package admin

import (
	"testing"

	"qwen2api/internal/metrics"
)

func TestMergeModelUsageAggregatesDistinctAliases(t *testing.T) {
	snapshot := map[string]metrics.ModelUsage{
		"Qwen3-Max": {
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
		"qwen3-max": {
			PromptTokens:     7,
			CompletionTokens: 3,
			TotalTokens:      10,
		},
	}

	usage := mergeModelUsage(snapshot, "Qwen3-Max", "qwen3-max", "Qwen3-Max")
	if usage.PromptTokens != 17 || usage.CompletionTokens != 8 || usage.TotalTokens != 25 {
		t.Fatalf("unexpected merged usage: %+v", usage)
	}
}
