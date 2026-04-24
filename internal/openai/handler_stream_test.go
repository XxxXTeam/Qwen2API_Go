package openai

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"qwen2api/internal/config"
	"qwen2api/internal/logging"
	"qwen2api/internal/metrics"
)

func TestHandleStreamDoesNotFragmentWhenNoTools(t *testing.T) {
	handler := &Handler{
		cfg:     config.Config{},
		metrics: metrics.NewDashboardStats(),
		logger:  logging.New(false),
	}

	upstream := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant","content":"你好！有什么我可以"}}]}`,
		"",
		`data: {"choices":[{"delta":{"role":"assistant","content":"帮你的吗？"}}]}`,
		"",
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	recorder := httptest.NewRecorder()
	handler.handleStream(recorder, strings.NewReader(upstream), "qwen3.6-plus", nil)

	body := recorder.Body.String()
	lines := strings.Split(body, "\n\n")
	contentPieces := make([]string, 0)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
			continue
		}
		choices, _ := decoded["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			continue
		}
		if content := strings.TrimSpace(stringValue(delta["content"])); content != "" {
			contentPieces = append(contentPieces, content)
		}
	}

	if len(contentPieces) != 2 {
		t.Fatalf("contentPieces len = %d, want 2, pieces=%#v", len(contentPieces), contentPieces)
	}
	if contentPieces[0] != "你好！有什么我可以" {
		t.Fatalf("first piece = %q", contentPieces[0])
	}
	if contentPieces[1] != "帮你的吗？" {
		t.Fatalf("second piece = %q", contentPieces[1])
	}
}
