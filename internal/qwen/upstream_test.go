package qwen

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestInspectUpstreamStreamReleasesValidSSE(t *testing.T) {
	input := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"hello"}}]}`,
		"",
		`data: [DONE]`,
		"",
	}, "\n")

	result, err := InspectUpstreamStream(context.Background(), io.NopCloser(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("InspectUpstreamStream() error = %v", err)
	}
	if result.UpstreamError != nil {
		t.Fatalf("expected no upstream error, got %+v", result.UpstreamError)
	}

	raw, err := io.ReadAll(result.Stream)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(raw) != input {
		t.Fatalf("expected replayed stream to equal input\nwant: %q\ngot:  %q", input, string(raw))
	}
}

func TestInspectUpstreamStreamInterceptsBusinessError(t *testing.T) {
	input := `data: {"success":false,"data":{"code":"RateLimited","num":2,"details":"quota exceeded"}}` + "\n\n"

	result, err := InspectUpstreamStream(context.Background(), io.NopCloser(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("InspectUpstreamStream() error = %v", err)
	}
	if result.UpstreamError == nil {
		t.Fatal("expected upstream error, got nil")
	}
	if result.UpstreamError.StatusCode != 429 {
		t.Fatalf("expected status 429, got %d", result.UpstreamError.StatusCode)
	}
	if !strings.Contains(result.UpstreamError.Error(), "等待约 2 小时") {
		t.Fatalf("unexpected error message: %q", result.UpstreamError.Error())
	}
}

func TestInspectUpstreamStreamKeepsToolPreludeBuffered(t *testing.T) {
	input := strings.Join([]string{
		`data: {"choices":[{"delta":{"role":"assistant"}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"<tool_calls>"}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"<tool_name>search</tool_name>"}}]}`,
		"",
	}, "\n")

	result, err := InspectUpstreamStream(context.Background(), io.NopCloser(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("InspectUpstreamStream() error = %v", err)
	}
	if result.UpstreamError != nil {
		t.Fatalf("expected no upstream error, got %+v", result.UpstreamError)
	}
	raw, err := io.ReadAll(result.Stream)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(raw) != input {
		t.Fatalf("expected full buffered stream, got %q", string(raw))
	}
}

func TestNormalizeUpstreamErrorMapsDirectQuotaErrorTo429(t *testing.T) {
	payload := map[string]any{
		"error": map[string]any{
			"message": "Allocated quota exceeded, please increase your quota limit. For details, see: https://www.alibabacloud.com/help/en/model-studio/error-code#token-limit",
		},
	}

	result := NormalizeUpstreamError(payload)
	if result == nil {
		t.Fatal("expected upstream error, got nil")
	}
	if result.StatusCode != 429 {
		t.Fatalf("expected status 429, got %d", result.StatusCode)
	}
	if !result.Retryable {
		t.Fatal("expected retryable=true")
	}
}

func TestNormalizeUpstreamErrorMapsAlibabaHumanVerificationTo429(t *testing.T) {
	payload := map[string]any{
		"error": map[string]any{
			"message": "阿里云安全验证，请完成验证后继续访问",
		},
	}

	result := NormalizeUpstreamError(payload)
	if result == nil {
		t.Fatal("expected upstream error, got nil")
	}
	if result.StatusCode != 429 {
		t.Fatalf("expected status 429, got %d", result.StatusCode)
	}
	if result.Retryable {
		t.Fatal("expected retryable=false")
	}
	if !strings.Contains(result.Error(), "人机验证") {
		t.Fatalf("unexpected error message: %q", result.Error())
	}
}

func TestInspectUpstreamStreamInterceptsAlibabaHumanVerificationHTML(t *testing.T) {
	input := `<html><title>安全验证</title><script src="//g.alicdn.com/AWSC/AWSC"></script>请完成验证</html>`

	result, err := InspectUpstreamStream(context.Background(), io.NopCloser(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("InspectUpstreamStream() error = %v", err)
	}
	if result.UpstreamError == nil {
		t.Fatal("expected upstream error, got nil")
	}
	if result.UpstreamError.StatusCode != 429 {
		t.Fatalf("expected status 429, got %d", result.UpstreamError.StatusCode)
	}
}

func TestInspectUpstreamStreamIgnoresNonJSONPrelude(t *testing.T) {
	input := "event: ping\n\n" +
		`data: {"choices":[{"delta":{"content":"hello"}}]}` + "\n\n"

	result, err := InspectUpstreamStream(context.Background(), io.NopCloser(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("InspectUpstreamStream() error = %v", err)
	}
	if result.UpstreamError != nil {
		t.Fatalf("expected no upstream error, got %+v", result.UpstreamError)
	}
	raw, err := io.ReadAll(result.Stream)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(raw) != input {
		t.Fatalf("expected original stream, got %q", string(raw))
	}
}
