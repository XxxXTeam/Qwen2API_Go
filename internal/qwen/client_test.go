package qwen

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"qwen2api/internal/config"
	"qwen2api/internal/logging"
)

func TestNewRequestSetsAuthHeadersAndCookie(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))

	req, err := client.newRequest(context.Background(), http.MethodGet, "/api/models", "token-123", nil)
	if err != nil {
		t.Fatalf("newRequest() error = %v", err)
	}

	if got := req.Header.Get("Authorization"); got != "Bearer token-123" {
		t.Fatalf("Authorization = %q, want %q", got, "Bearer token-123")
	}
	cookie := req.Header.Get("Cookie")
	if !strings.Contains(cookie, "token=token-123") {
		t.Fatalf("expected token cookie, got %q", cookie)
	}
	if !strings.Contains(cookie, "ssxmod_itna=") || !strings.Contains(cookie, "ssxmod_itna2=") {
		t.Fatalf("expected ssxmod cookies, got %q", cookie)
	}
}

func TestSignInRequestOmitsAuthorizationButKeepsCookie(t *testing.T) {
	var captured *http.Request
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			captured = req.Clone(req.Context())
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"token":"ok-token"}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	token, err := client.SignIn(context.Background(), "test@example.com", "hashed")
	if err != nil {
		t.Fatalf("SignIn() error = %v", err)
	}
	if token != "ok-token" {
		t.Fatalf("token = %q, want %q", token, "ok-token")
	}
	if captured == nil {
		t.Fatal("expected request to be captured")
	}
	if got := captured.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization = %q, want empty", got)
	}
	cookie := captured.Header.Get("Cookie")
	if cookie == "" {
		t.Fatal("expected Cookie header to be set")
	}
	if strings.Contains(cookie, "token=") {
		t.Fatalf("did not expect token cookie in sign-in request, got %q", cookie)
	}
	if !strings.Contains(cookie, "ssxmod_itna=") || !strings.Contains(cookie, "ssxmod_itna2=") {
		t.Fatalf("expected ssxmod cookies, got %q", cookie)
	}
}

func TestSignInHandlesGzipJSONResponse(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write([]byte(`{"token":"gzip-token"}`)); err != nil {
		t.Fatalf("gzip write error = %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close error = %v", err)
	}

	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Header: http.Header{
					"Content-Encoding": []string{"gzip"},
				},
				Body: io.NopCloser(bytes.NewReader(compressed.Bytes())),
			}, nil
		}),
	}

	token, err := client.SignIn(context.Background(), "test@example.com", "hashed")
	if err != nil {
		t.Fatalf("SignIn() error = %v", err)
	}
	if token != "gzip-token" {
		t.Fatalf("token = %q, want %q", token, "gzip-token")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
