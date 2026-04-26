package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"qwen2api/internal/auth"
	"qwen2api/internal/config"
	"qwen2api/internal/logging"
	"qwen2api/internal/metrics"
)

func TestConvertAnthropicRequestSupportsToolResultMetadata(t *testing.T) {
	payload := anthropicRequest{
		Model:  "qwen3-235b-a22b",
		System: json.RawMessage(`[{"type":"text","text":"system prompt"}]`),
		Messages: []anthropicMessage{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"text","text":"hello"},
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"YWJj"}}
				]`),
			},
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"tool_result","tool_use_id":"toolu_123","is_error":true,"content":[{"type":"text","text":"weather: sunny"}]}
				]`),
			},
		},
		Tools: []anthropicTool{
			{
				Name:        "search",
				Description: "Search docs",
				InputSchema: map[string]any{"type": "object"},
			},
		},
		ToolChoice:  json.RawMessage(`{"type":"tool","name":"search"}`),
		Metadata:    map[string]any{"client": "sdk"},
		Temperature: ptrFloat(0.1),
		TopP:        ptrFloat(0.9),
	}

	result, err := convertAnthropicRequest(payload)
	if err != nil {
		t.Fatalf("convertAnthropicRequest() error = %v", err)
	}
	if len(result.Messages) != 3 {
		t.Fatalf("messages len = %d, want 3", len(result.Messages))
	}
	if got := result.Messages[0]["role"]; got != "system" {
		t.Fatalf("system role = %v", got)
	}
	userContent, _ := result.Messages[1]["content"].([]map[string]any)
	if len(userContent) != 2 {
		t.Fatalf("user content len = %d, want 2", len(userContent))
	}
	imageURL := userContent[1]["image_url"].(map[string]any)["url"]
	if imageURL != "data:image/png;base64,YWJj" {
		t.Fatalf("image url = %v", imageURL)
	}
	if result.Messages[2]["role"] != "tool" {
		t.Fatalf("tool result role = %v", result.Messages[2]["role"])
	}
	if result.Messages[2]["tool_call_id"] != "toolu_123" {
		t.Fatalf("tool_call_id = %v", result.Messages[2]["tool_call_id"])
	}
	if result.Messages[2]["content"] != "ERROR: weather: sunny" {
		t.Fatalf("tool result content = %v", result.Messages[2]["content"])
	}
	toolChoice, _ := result.ToolChoice.(map[string]any)
	function, _ := toolChoice["function"].(map[string]any)
	if function["name"] != "search" {
		t.Fatalf("tool choice name = %v", function["name"])
	}
}

func TestHandleAnthropicNonStreamMapsStableToolUseAndStopReason(t *testing.T) {
	handler := &Handler{
		cfg:     config.Config{},
		metrics: metrics.NewDashboardStats(),
		logger:  logging.New(false),
	}

	recorder := httptest.NewRecorder()
	body := `{
		"choices":[
			{
				"message":{
					"role":"assistant",
					"content":"<tool_calls><tool_call><tool_name>search</tool_name><parameters><query><![CDATA[golang]]></query></parameters></tool_call></tool_calls>"
				},
				"finish_reason":"tool_calls"
			}
		],
		"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}
	}`

	handler.handleAnthropicNonStream(recorder, strings.NewReader(body), "qwen3.6-plus", []string{"search"}, 1)

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["type"] != "message" {
		t.Fatalf("type = %v, want message", payload["type"])
	}
	if payload["stop_reason"] != "tool_use" {
		t.Fatalf("stop_reason = %v, want tool_use", payload["stop_reason"])
	}
	content, _ := payload["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content len = %d, want 1, payload=%s", len(content), recorder.Body.String())
	}
	block := content[0].(map[string]any)
	if block["type"] != "tool_use" {
		t.Fatalf("content type = %v, want tool_use", block["type"])
	}
	if !strings.HasPrefix(block["id"].(string), "toolu_") {
		t.Fatalf("tool id = %v", block["id"])
	}
	input := block["input"].(map[string]any)
	if input["query"] != "golang" {
		t.Fatalf("tool input query = %v", input["query"])
	}
}

func TestHandleAnthropicStreamFormatsUsageAndToolOnlyEvents(t *testing.T) {
	handler := &Handler{
		cfg:     config.Config{},
		metrics: metrics.NewDashboardStats(),
		logger:  logging.New(false),
	}

	upstream := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant","content":"<tool_calls>"}}],"usage":{"prompt_tokens":2,"completion_tokens":4,"total_tokens":6}}`,
		"",
		`data: {"choices":[{"delta":{"content":"<tool_call><tool_name>search</tool_name><parameters><query><![CDATA[golang]]></query></parameters></tool_call></tool_calls>"}}]}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	recorder := httptest.NewRecorder()
	handler.handleAnthropicStream(recorder, strings.NewReader(upstream), "qwen3.6-plus", []string{"search"}, 1)

	body := recorder.Body.String()
	for _, marker := range []string{
		"event: message_start",
		"event: content_block_start",
		"event: content_block_delta",
		"event: content_block_stop",
		"event: message_delta",
		"event: message_stop",
		`"input_tokens":2`,
		`"output_tokens":4`,
		`"type":"input_json_delta"`,
		`"stop_reason":"tool_use"`,
	} {
		if !strings.Contains(body, marker) {
			t.Fatalf("stream body missing %q\n%s", marker, body)
		}
	}
	if strings.Contains(body, `"type":"text_delta"`) {
		t.Fatalf("tool-only stream emitted text delta: %s", body)
	}
	if strings.Contains(body, `"object":"chat.completion.chunk"`) || strings.Contains(body, "[DONE]") {
		t.Fatalf("anthropic stream leaked openai format: %s", body)
	}
}

func TestValidateAnthropicVersion(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req.Header.Set("anthropic-version", "2023-06-01")
	if err := validateAnthropicVersion(req); err != nil {
		t.Fatalf("validateAnthropicVersion() error = %v", err)
	}

	badReq := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	badReq.Header.Set("anthropic-version", "bad-version")
	if err := validateAnthropicVersion(badReq); err == nil {
		t.Fatal("expected invalid version error, got nil")
	}
}

func TestHandleAnthropicCountTokens(t *testing.T) {
	handler := &Handler{
		cfg:     config.Config{},
		metrics: metrics.NewDashboardStats(),
		logger:  logging.New(false),
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(`{
		"model":"qwen3-235b-a22b",
		"system":[{"type":"text","text":"system"}],
		"messages":[
			{"role":"user","content":[{"type":"text","text":"hello world"}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"done"}]},
			{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"YWJj"}}]}
		]
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	recorder := httptest.NewRecorder()

	handler.HandleAnthropicCountTokens(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if int(payload["input_tokens"].(float64)) <= 0 {
		t.Fatalf("input_tokens = %v, want > 0", payload["input_tokens"])
	}
}

func TestAnthropicErrorType(t *testing.T) {
	cases := []struct {
		status int
		msg    string
		want   string
	}{
		{http.StatusBadRequest, "bad", "invalid_request_error"},
		{http.StatusUnauthorized, "bad", "authentication_error"},
		{http.StatusForbidden, "bad", "permission_error"},
		{http.StatusTooManyRequests, "bad", "rate_limit_error"},
		{http.StatusServiceUnavailable, "bad", "overloaded_error"},
		{http.StatusBadGateway, "temporary overload", "overloaded_error"},
	}
	for _, tc := range cases {
		if got := anthropicErrorType(tc.status, tc.msg); got != tc.want {
			t.Fatalf("anthropicErrorType(%d,%q)=%q want %q", tc.status, tc.msg, got, tc.want)
		}
	}
}

func TestExtractAPIKeyPrefersXAPIKey(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/messages", nil)
	req.Header.Set("Authorization", "Bearer should-not-win")
	req.Header.Set("x-api-key", "sk-anthropic")

	if got := auth.ExtractAPIKey(req); got != "sk-anthropic" {
		t.Fatalf("ExtractAPIKey() = %q, want %q", got, "sk-anthropic")
	}
}

func ptrFloat(v float64) *float64 {
	return &v
}
