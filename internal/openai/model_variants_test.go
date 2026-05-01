package openai

import (
	"encoding/json"
	"testing"
)

func TestSplitModelSuffixSupportsFastAndThinkingVariants(t *testing.T) {
	cases := map[string]string{
		"qwen3.6-plus-fast":            "qwen3.6-plus",
		"qwen3.6-plus-thinking":        "qwen3.6-plus",
		"qwen3.6-plus-fast-search":     "qwen3.6-plus",
		"qwen3.6-plus-thinking-search": "qwen3.6-plus",
		"qwen3.6-plus-search":          "qwen3.6-plus",
	}

	for input, want := range cases {
		if got := splitModelSuffix(input); got != want {
			t.Fatalf("splitModelSuffix(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildFeatureConfigForFastAndThinking(t *testing.T) {
	fast := buildFeatureConfig(thinkingModeFast)
	if fast["thinking_enabled"] != false {
		t.Fatalf("fast thinking_enabled = %v, want false", fast["thinking_enabled"])
	}
	if fast["thinking_mode"] != "Fast" {
		t.Fatalf("fast thinking_mode = %v, want Fast", fast["thinking_mode"])
	}

	thinking := buildFeatureConfig(thinkingModeThinking)
	if thinking["thinking_enabled"] != true {
		t.Fatalf("thinking thinking_enabled = %v, want true", thinking["thinking_enabled"])
	}
	if thinking["thinking_mode"] != "Thinking" {
		t.Fatalf("thinking thinking_mode = %v, want Thinking", thinking["thinking_mode"])
	}
	if thinking["thinking_format"] != "summary" {
		t.Fatalf("thinking thinking_format = %v, want summary", thinking["thinking_format"])
	}
}

func TestIsThinkingEnabledRecognizesModelVariant(t *testing.T) {
	if !isThinkingEnabled("qwen3.6-plus-thinking", nil) {
		t.Fatal("expected -thinking model variant to enable thinking")
	}
	if isThinkingEnabled("qwen3.6-plus-fast", true) {
		t.Fatal("expected -fast model variant to force fast mode")
	}
}

func TestResolveThinkingModeMapsReasoningEffort(t *testing.T) {
	if got := resolveThinkingMode("qwen3.6-plus", "low", nil, nil); got != thinkingModeFast {
		t.Fatalf("resolveThinkingMode(low) = %q, want %q", got, thinkingModeFast)
	}
	if got := resolveThinkingMode("qwen3.6-plus", "high", nil, nil); got != thinkingModeThinking {
		t.Fatalf("resolveThinkingMode(high) = %q, want %q", got, thinkingModeThinking)
	}
}

func TestResolveThinkingModeSupportsNestedReasoningEffort(t *testing.T) {
	if got := resolveThinkingMode("qwen3.6-plus", nil, "high", nil); got != thinkingModeThinking {
		t.Fatalf("resolveThinkingMode(nested high) = %q, want %q", got, thinkingModeThinking)
	}
}

func TestResolveThinkingModePrefersTopLevelReasoningEffort(t *testing.T) {
	if got := resolveThinkingMode("qwen3.6-plus", "low", "high", nil); got != thinkingModeFast {
		t.Fatalf("resolveThinkingMode(top-level low nested high) = %q, want %q", got, thinkingModeFast)
	}
}

func TestResolveThinkingModePrioritizesModelSuffix(t *testing.T) {
	if got := resolveThinkingMode("qwen3.6-plus-fast", "high", nil, true); got != thinkingModeFast {
		t.Fatalf("resolveThinkingMode(-fast) = %q, want %q", got, thinkingModeFast)
	}
	if got := resolveThinkingMode("qwen3.6-plus-thinking", "none", nil, false); got != thinkingModeThinking {
		t.Fatalf("resolveThinkingMode(-thinking) = %q, want %q", got, thinkingModeThinking)
	}
}

func TestResolveThinkingModeFallsBackToEnableThinking(t *testing.T) {
	if got := resolveThinkingMode("qwen3.6-plus", nil, nil, true); got != thinkingModeThinking {
		t.Fatalf("resolveThinkingMode(enable_thinking=true) = %q, want %q", got, thinkingModeThinking)
	}
	if got := resolveThinkingMode("qwen3.6-plus", nil, nil, nil); got != thinkingModeFast {
		t.Fatalf("resolveThinkingMode(default) = %q, want %q", got, thinkingModeFast)
	}
}

func TestChatRequestSupportsReasoningEffortAlias(t *testing.T) {
	var payload chatRequest
	raw := []byte(`{
		"model": "qwen3.6-plus",
		"reasoning": {
			"effort": "high"
		}
	}`)

	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Reasoning == nil {
		t.Fatal("expected reasoning to be decoded")
	}
	if got := resolveThinkingMode(payload.Model, payload.ReasoningEffort, payload.Reasoning.Effort, payload.EnableThinking); got != thinkingModeThinking {
		t.Fatalf("resolveThinkingMode(decoded alias) = %q, want %q", got, thinkingModeThinking)
	}
}
