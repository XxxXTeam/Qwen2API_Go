package qwen

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"qwen2api/internal/config"
	"qwen2api/internal/logging"
)

func TestNewRequestSetsAuthHeadersAndCookie(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	client.guestAuth = guestAuthState{
		cookieHeader: "cna=guest-cna; qwen-locale=zh-CN; atpsida=seeded",
		refreshedAt:  time.Now(),
	}

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
	for _, key := range []string{"cna=guest-cna", "qwen-locale=zh-CN", "atpsida=seeded", "_bl_uid=", "tfstk=", "isg="} {
		if !strings.Contains(cookie, key) {
			t.Fatalf("expected auth cookie to include %q, got %q", key, cookie)
		}
	}
	if !strings.Contains(cookie, "ssxmod_itna=") || !strings.Contains(cookie, "ssxmod_itna2=") {
		t.Fatalf("expected ssxmod cookies, got %q", cookie)
	}
	for _, header := range []string{
		"sec-ch-ua-full-version",
		"sec-ch-ua-full-version-list",
		"sec-ch-ua-platform-version",
		"sec-ch-ua-arch",
		"sec-ch-ua-bitness",
		"Priority",
		"DNT",
		"sec-gpc",
		"X-Request-Id",
	} {
		if req.Header.Get(header) == "" {
			t.Fatalf("expected %s to be set", header)
		}
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

func TestNewRequestUsesGuestCookieWithoutAuthorization(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	client.guestAuth = guestAuthState{
		cookieHeader: "cna=guest-cna; qwen-locale=zh-CN",
		refreshedAt:  time.Now(),
	}

	req, err := client.newRequest(context.Background(), http.MethodGet, "/api/v2/models/", "", nil)
	if err != nil {
		t.Fatalf("newRequest() error = %v", err)
	}

	if got := req.Header.Get("Authorization"); got != "" {
		t.Fatalf("Authorization = %q, want empty", got)
	}
	cookie := req.Header.Get("Cookie")
	if !strings.Contains(cookie, "cna=guest-cna") {
		t.Fatalf("expected guest cookie, got %q", cookie)
	}
	if strings.Contains(cookie, "token=") {
		t.Fatalf("did not expect token cookie in guest request, got %q", cookie)
	}
	if got := req.Header.Get("Version"); got != qwenWebVersion {
		t.Fatalf("Version = %q, want %q", got, qwenWebVersion)
	}
	if got := req.Header.Get("bx-v"); got != "2.5.36" {
		t.Fatalf("bx-v = %q, want %q", got, "2.5.36")
	}
	if got := req.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("X-Accel-Buffering = %q, want %q", got, "no")
	}
}

func TestNewRequestPrefersCapturedBrowserSessionHeaders(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	client.browserSessions.guest = &BrowserSession{
		Headers: http.Header{
			"User-Agent":               []string{"Mozilla/5.0 Browser Capture"},
			"Accept-Language":          []string{"en-US,en;q=0.9"},
			"Bx-Ua":                    []string{"captured-bx-ua"},
			"Sec-Fetch-Site":           []string{"same-origin"},
			"sec-ch-ua":                []string{`"Chromium";v="147", "Not.A/Brand";v="8"`},
			"sec-fetch-storage-access": []string{"active"},
			"If-None-Match":            []string{`W/"captured-etag"`},
			"sec-gpc":                  []string{"1"},
			"Referer":                  []string{"https://chat.qwen.ai/c/captured"},
		},
		Cookie:     "cna=captured-cna; sca=captured-sca",
		CapturedAt: time.Now(),
		Guest:      true,
	}
	client.guestAuth = guestAuthState{
		cookieHeader: "cna=guest-cna; qwen-locale=zh-CN",
		refreshedAt:  time.Now(),
	}

	req, err := client.newRequest(context.Background(), http.MethodGet, "/api/v2/models/", "", nil)
	if err != nil {
		t.Fatalf("newRequest() error = %v", err)
	}

	if got := req.Header.Get("User-Agent"); got != "Mozilla/5.0 Browser Capture" {
		t.Fatalf("User-Agent = %q, want browser captured value", got)
	}
	if got := req.Header.Get("Accept-Language"); got != "en-US,en;q=0.9" {
		t.Fatalf("Accept-Language = %q, want browser captured value", got)
	}
	if got := req.Header.Get("Referer"); got != "https://chat.qwen.ai/c/captured" {
		t.Fatalf("Referer = %q, want browser captured value", got)
	}
	if got := req.Header.Get("Bx-Ua"); got != "captured-bx-ua" {
		t.Fatalf("Bx-Ua = %q, want browser captured value", got)
	}
	if got := req.Header.Get("Sec-Fetch-Site"); got != "same-origin" {
		t.Fatalf("Sec-Fetch-Site = %q, want browser captured value", got)
	}
	if got := req.Header.Get("sec-fetch-storage-access"); got != "active" {
		t.Fatalf("sec-fetch-storage-access = %q, want browser captured value", got)
	}
	if got := req.Header.Get("If-None-Match"); got != `W/"captured-etag"` {
		t.Fatalf("If-None-Match = %q, want browser captured value", got)
	}
	cookie := req.Header.Get("Cookie")
	for _, part := range []string{"cna=captured-cna", "sca=captured-sca"} {
		if !strings.Contains(cookie, part) {
			t.Fatalf("Cookie = %q, expected %q", cookie, part)
		}
	}
}

func TestRestoreBrowserSessionsFeedsProtocolRequests(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	client.RestoreBrowserSessions(map[string]BrowserSession{
		"guest": {
			Headers: http.Header{
				"User-Agent":      []string{"Persisted Browser"},
				"Accept-Language": []string{"en-US,en;q=0.9"},
			},
			Cookie:     "cna=persisted-cna; sca=persisted-sca",
			CapturedAt: time.Now(),
			Guest:      true,
			HasCookie:  true,
		},
	})

	req, err := client.newRequest(context.Background(), http.MethodGet, "/api/v2/models/", "", nil)
	if err != nil {
		t.Fatalf("newRequest() error = %v", err)
	}
	if got := req.Header.Get("User-Agent"); got != "Persisted Browser" {
		t.Fatalf("User-Agent = %q, want persisted browser header", got)
	}
	cookie := req.Header.Get("Cookie")
	for _, part := range []string{"cna=persisted-cna", "sca=persisted-sca"} {
		if !strings.Contains(cookie, part) {
			t.Fatalf("Cookie = %q, expected %q", cookie, part)
		}
	}
}

func TestNewRequestUsesAccountScopedBrowserSessions(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	client.RestoreBrowserSessions(map[string]BrowserSession{
		"user-a@example.com": {
			Headers: http.Header{
				"User-Agent": []string{"Browser A"},
				"Bx-Ua":      []string{"bx-a"},
			},
			Cookie:     "cna=cookie-a; sca=sca-a",
			CapturedAt: time.Now(),
			HasCookie:  true,
		},
		"user-b@example.com": {
			Headers: http.Header{
				"User-Agent": []string{"Browser B"},
				"Bx-Ua":      []string{"bx-b"},
			},
			Cookie:     "cna=cookie-b; sca=sca-b",
			CapturedAt: time.Now(),
			HasCookie:  true,
		},
	})

	reqA, err := client.newRequest(WithAccountKey(context.Background(), "user-a@example.com"), http.MethodGet, "/api/v2/models/", "token-a", nil)
	if err != nil {
		t.Fatalf("newRequest(account a) error = %v", err)
	}
	reqB, err := client.newRequest(WithAccountKey(context.Background(), "user-b@example.com"), http.MethodGet, "/api/v2/models/", "token-b", nil)
	if err != nil {
		t.Fatalf("newRequest(account b) error = %v", err)
	}

	if got := reqA.Header.Get("User-Agent"); got != "Browser A" {
		t.Fatalf("account a User-Agent = %q, want Browser A", got)
	}
	if got := reqB.Header.Get("User-Agent"); got != "Browser B" {
		t.Fatalf("account b User-Agent = %q, want Browser B", got)
	}
	if got := reqA.Header.Get("Bx-Ua"); got != "bx-a" {
		t.Fatalf("account a Bx-Ua = %q, want bx-a", got)
	}
	if got := reqB.Header.Get("Bx-Ua"); got != "bx-b" {
		t.Fatalf("account b Bx-Ua = %q, want bx-b", got)
	}
	if cookie := reqA.Header.Get("Cookie"); !strings.Contains(cookie, "cna=cookie-a") || strings.Contains(cookie, "cna=cookie-b") {
		t.Fatalf("account a Cookie = %q, want only account a browser cookie", cookie)
	}
	if cookie := reqB.Header.Get("Cookie"); !strings.Contains(cookie, "cna=cookie-b") || strings.Contains(cookie, "cna=cookie-a") {
		t.Fatalf("account b Cookie = %q, want only account b browser cookie", cookie)
	}
}

func TestDoRetriesAnonymousRequestAfterRefreshingGuestCookie(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	client.guestAuth = guestAuthState{
		cookieHeader: "cna=stale-cna",
		refreshedAt:  time.Now(),
	}

	targetCalls := 0
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/":
				return &http.Response{
					StatusCode: 200,
					Header: http.Header{
						"Set-Cookie": []string{"cna=fresh-cna; Path=/"},
					},
					Body: io.NopCloser(strings.NewReader("ok")),
				}, nil
			case "/api/v2/configs/":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("{}")),
					Header:     make(http.Header),
				}, nil
			case "/api/v2/configs/setting-config":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("{}")),
					Header:     make(http.Header),
				}, nil
			case "/api/v2/tts/config":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("{}")),
					Header:     make(http.Header),
				}, nil
			case "/api/v2/users/status":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
					Header:     make(http.Header),
				}, nil
			case "/api/v1/auths/":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader("{}")),
					Header:     make(http.Header),
				}, nil
			case "/api/v2/models/":
				targetCalls++
				cookie := req.Header.Get("Cookie")
				if targetCalls == 1 {
					if !strings.Contains(cookie, "cna=stale-cna") {
						t.Fatalf("first request cookie = %q, want stale guest cookie", cookie)
					}
					return &http.Response{
						StatusCode: 401,
						Body:       io.NopCloser(strings.NewReader("unauthorized")),
						Header:     make(http.Header),
					}, nil
				}
				if strings.Contains(cookie, "cna=stale-cna") || !strings.Contains(cookie, "cna=") {
					t.Fatalf("second request cookie = %q, want refreshed guest cookie", cookie)
				}
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"success":true,"data":{"data":[]}}`)),
					Header:     make(http.Header),
				}, nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	req, err := client.newRequest(context.Background(), http.MethodGet, "/api/v2/models/", "", nil)
	if err != nil {
		t.Fatalf("newRequest() error = %v", err)
	}

	resp, err := client.do(req)
	if err != nil {
		t.Fatalf("do() error = %v", err)
	}
	defer resp.Body.Close()

	if targetCalls != 2 {
		t.Fatalf("targetCalls = %d, want 2", targetCalls)
	}
}

func TestRefreshModelsBypassesCachedList(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	seedGuestAuth(client)
	calls := 0
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/v2/models/" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			calls++
			body := `{"success":true,"data":{"data":[{"id":"first","name":"First"}]}}`
			if calls == 2 {
				body = `{"success":true,"data":{"data":[{"id":"second","name":"Second"}]}}`
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	models, err := client.ListModels(context.Background(), "token-123")
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 1 || models[0].ID != "first" {
		t.Fatalf("first models = %#v, want first", models)
	}

	cached, err := client.ListModels(context.Background(), "token-123")
	if err != nil {
		t.Fatalf("cached ListModels() error = %v", err)
	}
	if len(cached) != 1 || cached[0].ID != "first" || calls != 1 {
		t.Fatalf("cached models = %#v calls=%d, want first with one call", cached, calls)
	}

	refreshed, err := client.RefreshModels(context.Background(), "token-123")
	if err != nil {
		t.Fatalf("RefreshModels() error = %v", err)
	}
	if len(refreshed) != 1 || refreshed[0].ID != "second" || calls != 2 {
		t.Fatalf("refreshed models = %#v calls=%d, want second with two calls", refreshed, calls)
	}
}

func TestEnsureGuestCookieHeaderUsesAuthPyBootstrapSequence(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))

	var calls []string
	statusContentTypes := make([]string, 0, 2)
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls = append(calls, req.Method+" "+req.URL.RequestURI())
			switch req.URL.Path {
			case "/":
				if got := req.Header.Get("Accept"); !strings.Contains(got, "text/html") {
					t.Fatalf("homepage Accept = %q, want browser-style html accept", got)
				}
			case "/api/v2/configs/", "/api/v2/configs/setting-config", "/api/v2/tts/config", "/api/v1/auths/":
				if got := req.Header.Get("Accept"); got != "application/json, text/plain, */*" {
					t.Fatalf("%s Accept = %q", req.URL.Path, got)
				}
				if got := req.Header.Get("Version"); got != qwenWebVersion {
					t.Fatalf("%s Version = %q, want %s", req.URL.Path, got, qwenWebVersion)
				}
				if got := req.Header.Get("Source"); got != "web" {
					t.Fatalf("%s Source = %q, want web", req.URL.Path, got)
				}
				if got := req.Header.Get("Timezone"); got == "" {
					t.Fatalf("%s Timezone should not be empty", req.URL.Path)
				}
			case "/api/v2/users/status":
				if req.Method != http.MethodPost {
					t.Fatalf("users/status method = %s, want POST", req.Method)
				}
				statusContentTypes = append(statusContentTypes, req.Header.Get("Content-Type"))
				raw, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("ReadAll(users/status) error = %v", err)
				}
				var payload map[string]any
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("Unmarshal(users/status) error = %v", err)
				}
				typarms, _ := payload["typarms"].(map[string]any)
				if typarms == nil {
					t.Fatal("expected typarms payload for users/status")
				}
				if strings.Contains(req.Header.Get("Content-Type"), "text/plain") {
					if got := strings.TrimSpace(anyString(typarms["requestType"])); got != "sendBeacon" {
						t.Fatalf("beacon requestType = %q, want sendBeacon", got)
					}
				} else {
					if got := strings.TrimSpace(anyString(typarms["project_id"])); got != "" {
						t.Fatalf("project_id = %q, want empty string", got)
					}
					if got := strings.TrimSpace(anyString(typarms["cdn_version"])); got != qwenWebVersion {
						t.Fatalf("cdn_version = %q, want %q", got, qwenWebVersion)
					}
				}
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}

			return &http.Response{
				StatusCode: 200,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("{}")),
			}, nil
		}),
	}

	cookieHeader, err := client.EnsureGuestCookieHeader(context.Background())
	if err != nil {
		t.Fatalf("EnsureGuestCookieHeader() error = %v", err)
	}

	wantCalls := []string{
		"GET /",
		"GET /api/v2/configs/",
		"GET /api/v2/configs/setting-config",
		"POST /api/v2/users/status",
		"GET /api/v2/configs/setting-config",
		"GET /api/v2/tts/config?omni_speakers=v1&audio_tts_speakers=v1&omni_language=v1&audio_tts_language=v1",
		"GET /api/v1/auths/",
		"POST /api/v2/users/status",
	}
	if len(calls) != len(wantCalls) {
		t.Fatalf("bootstrap call count = %d, want %d (%v)", len(calls), len(wantCalls), calls)
	}
	for i, want := range wantCalls {
		if calls[i] != want {
			t.Fatalf("bootstrap call[%d] = %q, want %q", i, calls[i], want)
		}
	}
	if len(statusContentTypes) != 2 {
		t.Fatalf("users/status count = %d, want 2", len(statusContentTypes))
	}
	if got := statusContentTypes[0]; got != "text/plain;charset=UTF-8" {
		t.Fatalf("first users/status content-type = %q, want text/plain;charset=UTF-8", got)
	}
	if got := statusContentTypes[1]; got != "application/json" {
		t.Fatalf("second users/status content-type = %q, want application/json", got)
	}
	if !strings.Contains(cookieHeader, "cna=") {
		t.Fatalf("expected generated cna in cookie header, got %q", cookieHeader)
	}
	if !strings.Contains(cookieHeader, "ssxmod_itna=") || !strings.Contains(cookieHeader, "ssxmod_itna2=") {
		t.Fatalf("expected ssxmod cookies in guest cookie header, got %q", cookieHeader)
	}
}

func TestEnsureGuestCookieHeaderFallsBackWhenBootstrapFails(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		}),
		Timeout: 50 * time.Millisecond,
	}

	cookieHeader, err := client.EnsureGuestCookieHeader(context.Background())
	if err != nil {
		t.Fatalf("EnsureGuestCookieHeader() error = %v", err)
	}
	for _, key := range []string{"cna=", "_bl_uid=", "atpsida=", "x-ap=", "sca=", "tfstk=", "isg=", "ssxmod_itna=", "ssxmod_itna2="} {
		if !strings.Contains(cookieHeader, key) {
			t.Fatalf("expected fallback cookie %q in %q", key, cookieHeader)
		}
	}
}

func TestNewChatSendsRequestedChatType(t *testing.T) {
	var capturedBody map[string]any
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	seedGuestAuth(client)
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/v2/chats/new" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			raw, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if err := json.Unmarshal(raw, &capturedBody); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"data":{"id":"chat-123"}}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	chatID, err := client.NewChat(context.Background(), "token-123", "qwen3", "t2t")
	if err != nil {
		t.Fatalf("NewChat() error = %v", err)
	}
	if chatID != "chat-123" {
		t.Fatalf("chatID = %q, want %q", chatID, "chat-123")
	}
	if got := strings.TrimSpace(anyString(capturedBody["chat_type"])); got != "t2t" {
		t.Fatalf("chat_type = %q, want %q", got, "t2t")
	}
	if got := strings.TrimSpace(anyString(capturedBody["chat_mode"])); got != "normal" {
		t.Fatalf("chat_mode = %q, want %q", got, "normal")
	}
	if got := strings.TrimSpace(anyString(capturedBody["project_id"])); got != "" {
		t.Fatalf("project_id = %q, want empty string", got)
	}
	if got := strings.TrimSpace(anyString(capturedBody["title"])); got != "新建对话" {
		t.Fatalf("title = %q, want 新建对话", got)
	}
}

func TestNewChatParsesAlternateChatIDShape(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	seedGuestAuth(client)
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"data":{"chat":{"chat_id":"chat-alt"}}}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	chatID, err := client.NewChat(context.Background(), "token-123", "qwen3", "t2t")
	if err != nil {
		t.Fatalf("NewChat() error = %v", err)
	}
	if chatID != "chat-alt" {
		t.Fatalf("chatID = %q, want %q", chatID, "chat-alt")
	}
}

func TestNewChatMapsAlibabaHumanVerificationHTMLTo429(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	seedGuestAuth(client)
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`<html><title>Access Verification</title><script>var aliyun_waf_aa = true; var aliyunCaptcha = true;</script></html>`)),
				Header: http.Header{
					"Content-Type": []string{"text/html; charset=utf-8"},
				},
			}, nil
		}),
	}

	_, err := client.NewChat(context.Background(), "token-123", "qwen3", "t2t")
	if err == nil {
		t.Fatal("expected error")
	}
	upstreamErr, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("error type = %T, want *UpstreamError", err)
	}
	if upstreamErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", upstreamErr.StatusCode)
	}
}

func TestNewChatUsesGuestModeWithoutToken(t *testing.T) {
	var capturedBody map[string]any
	var capturedRequest *http.Request
	var calls []string
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	client.guestAuth = guestAuthState{
		cookieHeader: "cna=guest-cna",
		refreshedAt:  time.Now(),
	}
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls = append(calls, req.Method+" "+req.URL.RequestURI())
			switch req.URL.Path {
			case "/api/v2/users/status":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
					Header:     make(http.Header),
				}, nil
			case "/api/v2/configs/setting-config":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
					Header:     make(http.Header),
				}, nil
			case "/api/v2/chats/new":
				capturedRequest = req.Clone(req.Context())
				raw, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("ReadAll() error = %v", err)
				}
				if err := json.Unmarshal(raw, &capturedBody); err != nil {
					t.Fatalf("Unmarshal() error = %v", err)
				}
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"data":{"id":"chat-guest"}}`)),
					Header:     make(http.Header),
				}, nil
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	chatID, err := client.NewChat(context.Background(), "", "qwen3", "t2t")
	if err != nil {
		t.Fatalf("NewChat() error = %v", err)
	}
	if chatID != "chat-guest" {
		t.Fatalf("chatID = %q, want %q", chatID, "chat-guest")
	}
	if got := strings.TrimSpace(anyString(capturedBody["chat_mode"])); got != "guest" {
		t.Fatalf("chat_mode = %q, want %q", got, "guest")
	}
	if got := strings.TrimSpace(anyString(capturedBody["title"])); got != "新建对话" {
		t.Fatalf("title = %q, want %q", got, "新建对话")
	}
	if got := strings.TrimSpace(anyString(capturedBody["project_id"])); got != "" {
		t.Fatalf("project_id = %q, want empty string", got)
	}
	if capturedRequest == nil {
		t.Fatal("expected request to be captured")
	}
	if got := capturedRequest.Header.Get("Accept"); got != "application/json, text/plain, */*" {
		t.Fatalf("Accept = %q, want %q", got, "application/json, text/plain, */*")
	}
	if got := capturedRequest.Header.Get("Referer"); got != "https://chat.qwen.ai/c/new-chat" {
		t.Fatalf("Referer = %q, want %q", got, "https://chat.qwen.ai/c/new-chat")
	}
	if got := capturedRequest.Header.Get("X-Request-Id"); got == "" {
		t.Fatal("expected X-Request-Id to be set")
	}
	if got := capturedRequest.Header.Get("Priority"); got != "u=0" {
		t.Fatalf("Priority = %q, want %q", got, "u=0")
	}
	wantCalls := []string{
		"POST /api/v2/users/status",
		"POST /api/v2/chats/new",
		"GET /api/v2/configs/setting-config",
		"POST /api/v2/users/status",
	}
	if len(calls) != len(wantCalls) {
		t.Fatalf("call count = %d, want %d (%v)", len(calls), len(wantCalls), calls)
	}
	for i, want := range wantCalls {
		if calls[i] != want {
			t.Fatalf("call[%d] = %q, want %q", i, calls[i], want)
		}
	}
}

func TestChatCompletionsUsesGuestHeadersWithoutToken(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	client.guestAuth = guestAuthState{
		cookieHeader: "cna=guest-cna",
		refreshedAt:  time.Now(),
	}

	var captured *http.Request
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			captured = req.Clone(req.Context())
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("data: {}\n\n")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	resp, err := client.ChatCompletions(context.Background(), "", "chat-123", map[string]any{"stream": true})
	if err != nil {
		t.Fatalf("ChatCompletions() error = %v", err)
	}
	defer resp.Body.Close()

	if captured == nil {
		t.Fatal("expected request to be captured")
	}
	if got := captured.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q, want %q", got, "application/json")
	}
	if got := captured.Header.Get("Referer"); got != "https://chat.qwen.ai/c/guest" {
		t.Fatalf("Referer = %q, want %q", got, "https://chat.qwen.ai/c/guest")
	}
	if got := captured.Header.Get("Version"); got != qwenWebVersion {
		t.Fatalf("Version = %q, want %q", got, qwenWebVersion)
	}
	if got := captured.Header.Get("bx-v"); got != qwenBxVersion {
		t.Fatalf("bx-v = %q, want %q", got, qwenBxVersion)
	}
	if got := captured.Header.Get("X-Request-Id"); got == "" {
		t.Fatal("expected X-Request-Id to be set")
	}
	if got := captured.Header.Get("Accept-Language"); got != "zh-CN,zh;q=0.9" {
		t.Fatalf("Accept-Language = %q, want %q", got, "zh-CN,zh;q=0.9")
	}
	if got := captured.Header.Get("Timezone"); !strings.Contains(got, "GMT") {
		t.Fatalf("Timezone = %q, want GMT-formatted current timezone", got)
	}
}

func TestChatCompletionsUsesJSONAcceptWhenStreamFalse(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	seedGuestAuth(client)

	var captured *http.Request
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			captured = req.Clone(req.Context())
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	resp, err := client.ChatCompletions(context.Background(), "token-123", "chat-123", map[string]any{"stream": false})
	if err != nil {
		t.Fatalf("ChatCompletions() error = %v", err)
	}
	defer resp.Body.Close()

	if captured == nil {
		t.Fatal("expected request to be captured")
	}
	if got := captured.Header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got)
	}
	if got := captured.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("X-Accel-Buffering = %q, want no", got)
	}
}

func TestChatCompletionsMapsAlibabaHumanVerificationTo429(t *testing.T) {
	client := NewClient(config.Config{QwenChatProxyURL: "https://chat.qwen.ai"}, logging.New(false))
	seedGuestAuth(client)
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Body:       io.NopCloser(strings.NewReader(`<html><title>安全验证</title>请完成验证<script src="awsc.js"></script></html>`)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	_, err := client.ChatCompletions(context.Background(), "token-123", "chat-123", map[string]any{"stream": false})
	if err == nil {
		t.Fatal("expected error")
	}
	upstreamErr, ok := err.(*UpstreamError)
	if !ok {
		t.Fatalf("error type = %T, want *UpstreamError", err)
	}
	if upstreamErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", upstreamErr.StatusCode)
	}
	if !strings.Contains(upstreamErr.Error(), "人机验证") {
		t.Fatalf("unexpected error message: %q", upstreamErr.Error())
	}
}

func TestBrowserProbeScriptUsesGuestAndNormalChatMode(t *testing.T) {
	guestScript := browserProbeScript(true)
	if !strings.Contains(guestScript, `chat_mode: "guest"`) {
		t.Fatalf("guest script = %q, want guest chat mode", guestScript)
	}
	if !strings.Contains(guestScript, `aemPageId: "//chat.qwen.ai/c/new-chat"`) {
		t.Fatalf("guest script = %q, want new-chat page id", guestScript)
	}

	authScript := browserProbeScript(false)
	if !strings.Contains(authScript, `chat_mode: "normal"`) {
		t.Fatalf("auth script = %q, want normal chat mode", authScript)
	}
	if strings.Contains(authScript, `chat_mode: "guest"`) {
		t.Fatalf("auth script = %q, should not contain guest chat mode", authScript)
	}
}

func anyString(value any) string {
	text, _ := value.(string)
	return text
}

func seedGuestAuth(client *Client) {
	client.guestAuth = guestAuthState{
		cookieHeader: "cna=guest-cna; qwen-locale=zh-CN; atpsida=seeded",
		refreshedAt:  time.Now(),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
