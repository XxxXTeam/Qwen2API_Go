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
