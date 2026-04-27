package openai

import "testing"

func TestStatsModelNamePrefersRequestedModel(t *testing.T) {
	if got := statsModelName("qwen3-max-thinking", "qwen3-max"); got != "qwen3-max-thinking" {
		t.Fatalf("statsModelName = %q, want requested model", got)
	}
}

func TestStatsModelNameFallsBackToResolvedModel(t *testing.T) {
	if got := statsModelName("", "qwen3-max"); got != "qwen3-max" {
		t.Fatalf("statsModelName = %q, want resolved model", got)
	}
}
