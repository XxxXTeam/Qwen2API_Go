package openai

import (
	"encoding/json"
	"net/http"
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

func TestHandleStreamIncludesThinkingSummaryContent(t *testing.T) {
	handler := &Handler{
		cfg:     config.Config{},
		metrics: metrics.NewDashboardStats(),
		logger:  logging.New(false),
	}

	upstream := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant","content":"","phase":"thinking_summary","extra":{"summary_title":{"content":["回应用户的问候并主动提供帮助"]},"summary_thought":{"content":["我感知到用户重复发送了简单的问候。"]}}}}]}`,
		"",
		`data: {"choices":[{"delta":{"role":"assistant","content":"你好","phase":"answer"}}]}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	recorder := httptest.NewRecorder()
	handler.handleStream(recorder, strings.NewReader(upstream), "qwen3.6-plus", nil)

	body := recorder.Body.String()
	if !strings.Contains(body, "回应用户的问候并主动提供帮助") || !strings.Contains(body, "我感知到用户重复发送了简单的问候。") {
		t.Fatalf("stream body missing thinking summary: %s", body)
	}
	if !strings.Contains(body, "\\u003c/think\\u003e\\n你好") {
		t.Fatalf("stream body missing answer after think: %s", body)
	}
}

func TestHandleChatCompletionRedirectsHiStream(t *testing.T) {
	handler := &Handler{}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen3.6-plus","stream":true,"messages":[{"role":"user","content":"hi"}]}`))

	handler.HandleChatCompletion(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusFound)
	}
	if location := recorder.Header().Get("Location"); location != "https://www.yuanshen.com" {
		t.Fatalf("location = %q, want %q", location, "https://www.yuanshen.com")
	}
}
