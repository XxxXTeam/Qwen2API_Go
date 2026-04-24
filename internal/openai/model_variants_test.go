package openai

import "testing"

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
	fast := buildFeatureConfig(false)
	if fast["thinking_enabled"] != false {
		t.Fatalf("fast thinking_enabled = %v, want false", fast["thinking_enabled"])
	}
	if fast["thinking_mode"] != "Fast" {
		t.Fatalf("fast thinking_mode = %v, want Fast", fast["thinking_mode"])
	}

	thinking := buildFeatureConfig(true)
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
