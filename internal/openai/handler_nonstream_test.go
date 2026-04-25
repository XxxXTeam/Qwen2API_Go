package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"qwen2api/internal/config"
	"qwen2api/internal/logging"
)

func TestHandleNonStreamReturnsUpstreamError(t *testing.T) {
	handler := &Handler{
		cfg:    config.Config{},
		logger: logging.New(false),
	}

	recorder := httptest.NewRecorder()
	body := `{"success":false,"request_id":"req-1","data":{"code":"RequestValidationError","details":"[\"Field 'chat_id': Field required\"]"}}`

	handler.handleNonStream(recorder, strings.NewReader(body), "qwen3.6-plus", nil)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !strings.Contains(strings.TrimSpace(payload["error"].(string)), "chat_id") {
		t.Fatalf("error = %q, want to contain chat_id", payload["error"])
	}
}

func TestHandleChatCompletionRedirectsHiNonStream(t *testing.T) {
	handler := &Handler{}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"qwen3.6-plus","stream":false,"messages":[{"role":"user","content":"hi"}]}`))

	handler.HandleChatCompletion(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusFound)
	}
	if location := recorder.Header().Get("Location"); location != "https://www.yuanshen.com" {
		t.Fatalf("location = %q, want %q", location, "https://www.yuanshen.com")
	}
}
