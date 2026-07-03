package qwen

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	defaultBrowserAuthTimeout = 45 * time.Second
	browserSessionTTL         = 30 * time.Minute
)

type browserAuthConfig struct {
	Enabled        bool
	Headless       bool
	ExecutablePath string
	Timeout        time.Duration
}

type BrowserSession struct {
	Headers     http.Header `json:"headers"`
	Cookie      string      `json:"cookie"`
	CapturedAt  time.Time   `json:"capturedAt"`
	Guest       bool        `json:"guest"`
	SourceURL   string      `json:"sourceUrl"`
	UserAgent   string      `json:"userAgent,omitempty"`
	HasCookie   bool        `json:"hasCookie"`
	CookieNames []string    `json:"cookieNames,omitempty"`
}

type browserSessionState struct {
	mu       sync.RWMutex
	guest    *BrowserSession
	accounts map[string]*BrowserSession
}

type browserSessionKind int

const (
	browserSessionGuest browserSessionKind = iota
	browserSessionAuth
)

type accountKeyContextKey struct{}

var browserHeaderAllowlist = map[string]struct{}{
	"Accept":                      {},
	"Accept-Encoding":             {},
	"Accept-Language":             {},
	"Bx-Ua":                       {},
	"Bx-Umidtoken":                {},
	"Bx-V":                        {},
	"Cache-Control":               {},
	"DNT":                         {},
	"If-None-Match":               {},
	"Origin":                      {},
	"Pragma":                      {},
	"Priority":                    {},
	"Referer":                     {},
	"Sec-Fetch-Dest":              {},
	"Sec-Fetch-Mode":              {},
	"Sec-Fetch-Site":              {},
	"Source":                      {},
	"Timezone":                    {},
	"User-Agent":                  {},
	"Version":                     {},
	"sec-ch-ua":                   {},
	"sec-ch-ua-arch":              {},
	"sec-ch-ua-bitness":           {},
	"sec-ch-ua-full-version":      {},
	"sec-ch-ua-full-version-list": {},
	"sec-ch-ua-mobile":            {},
	"sec-ch-ua-platform":          {},
	"sec-ch-ua-platform-version":  {},
	"sec-fetch-storage-access":    {},
	"sec-gpc":                     {},
}

func (c *Client) CaptureGuestBrowserSession(ctx context.Context) (*BrowserSession, error) {
	session, err := c.captureBrowserSession(ctx, "", true)
	if err != nil {
		return nil, err
	}
	c.setBrowserSession(browserSessionGuest, session)
	c.setGuestCookieHeader(session.Cookie)
	return session, nil
}

func (c *Client) CaptureBrowserSessionWithCookie(ctx context.Context, cookieHeader string) (*BrowserSession, error) {
	if strings.TrimSpace(cookieHeader) == "" {
		return nil, errors.New("cookie 不能为空")
	}
	session, err := c.captureBrowserSession(ctx, cookieHeader, false)
	if err != nil {
		return nil, err
	}
	c.setBrowserSessionForKey("auth", session)
	return session, nil
}

func (c *Client) CaptureBrowserSessionForAccount(ctx context.Context, accountKey string, cookieHeader string) (*BrowserSession, error) {
	accountKey = normalizeBrowserSessionKey(accountKey)
	if accountKey == "" || accountKey == "guest" {
		return nil, errors.New("账号标识不能为空")
	}
	if strings.TrimSpace(cookieHeader) == "" {
		return nil, errors.New("cookie 不能为空")
	}
	session, err := c.captureBrowserSession(ctx, cookieHeader, false)
	if err != nil {
		return nil, err
	}
	c.setBrowserSessionForKey(accountKey, session)
	return session, nil
}

func (c *Client) BrowserSessionSnapshot() map[string]any {
	c.browserSessions.mu.RLock()
	defer c.browserSessions.mu.RUnlock()

	accounts := make(map[string]any)
	for key, session := range c.browserSessions.accounts {
		accounts[key] = summarizeBrowserSession(session)
	}
	return map[string]any{
		"guest":    summarizeBrowserSession(c.browserSessions.guest),
		"auth":     summarizeBrowserSession(c.browserSessions.accounts["auth"]),
		"accounts": accounts,
	}
}

func (c *Client) BrowserSessions() map[string]BrowserSession {
	c.browserSessions.mu.RLock()
	defer c.browserSessions.mu.RUnlock()

	sessions := make(map[string]BrowserSession, 1+len(c.browserSessions.accounts))
	if c.browserSessions.guest != nil {
		copy := cloneBrowserSession(c.browserSessions.guest)
		sessions["guest"] = copy
	}
	for key, session := range c.browserSessions.accounts {
		if strings.TrimSpace(key) == "" || session == nil {
			continue
		}
		copy := cloneBrowserSession(session)
		sessions[key] = copy
	}
	return sessions
}

func (c *Client) RestoreBrowserSessions(sessions map[string]BrowserSession) {
	if len(sessions) == 0 {
		return
	}

	c.browserSessions.mu.Lock()
	defer c.browserSessions.mu.Unlock()

	if guest, ok := sessions["guest"]; ok && strings.TrimSpace(guest.Cookie) != "" {
		copy := cloneBrowserSession(&guest)
		c.browserSessions.guest = &copy
		c.guestMu.Lock()
		c.guestAuth = guestAuthState{
			cookieHeader: strings.TrimSpace(guest.Cookie),
			refreshedAt:  guest.CapturedAt,
		}
		c.guestMu.Unlock()
	}
	if c.browserSessions.accounts == nil {
		c.browserSessions.accounts = map[string]*BrowserSession{}
	}
	for key, session := range sessions {
		key = normalizeBrowserSessionKey(key)
		if key == "" || key == "guest" || strings.TrimSpace(session.Cookie) == "" {
			continue
		}
		copy := cloneBrowserSession(&session)
		c.browserSessions.accounts[key] = &copy
	}
}

func (c *Client) captureBrowserSession(ctx context.Context, cookieHeader string, guest bool) (*BrowserSession, error) {
	timeout := c.browserAuth.Timeout
	if timeout <= 0 {
		timeout = defaultBrowserAuthTimeout
	}

	captureCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", c.browserAuth.Headless),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	if path := c.browserExecutablePath(); path != "" {
		opts = append(opts, chromedp.ExecPath(path))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(captureCtx, opts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	headerCh := make(chan http.Header, 1)
	sourceURLCh := make(chan string, 1)
	var captureMu sync.Mutex
	var capturedHeaders http.Header
	var capturedSourceURL string
	var capturedPreferred bool
	chromedp.ListenTarget(browserCtx, func(ev any) {
		event, ok := ev.(*network.EventRequestWillBeSent)
		if !ok || event.Request == nil {
			return
		}
		requestURL := strings.TrimSpace(event.Request.URL)
		if !strings.Contains(requestURL, "/api/") {
			return
		}
		headers := browserHeadersFromNetwork(event.Request.Headers)
		if len(headers) == 0 {
			return
		}
		preferred := strings.Contains(requestURL, "/api/v2/chats/new") || strings.Contains(requestURL, "/api/v2/chat/completions")
		captureMu.Lock()
		if capturedHeaders == nil || preferred || !capturedPreferred {
			capturedHeaders = headers
			capturedSourceURL = requestURL
			capturedPreferred = preferred
		}
		captureMu.Unlock()
		select {
		case headerCh <- headers:
			sourceURLCh <- requestURL
		default:
		}
	})

	var cookie string
	var names []string
	if err := chromedp.Run(browserCtx,
		network.Enable(),
		c.injectBrowserCookies(cookieHeader),
		chromedp.Navigate(c.baseURL+"/"),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(`fetch("/api/v2/configs/", { credentials: "include" }).catch(() => null)`, nil),
		chromedp.Navigate(c.baseURL+"/c/new-chat"),
		chromedp.Sleep(2*time.Second),
		chromedp.Evaluate(browserProbeScript(guest), nil),
		chromedp.Sleep(2*time.Second),
		chromedp.Sleep(2*time.Second),
		browserCookieHeader(c.baseURL, &cookie, &names),
	); err != nil {
		return nil, err
	}

	headers := http.Header{}
	sourceURL := ""
	captureMu.Lock()
	if capturedHeaders != nil {
		headers = capturedHeaders.Clone()
		sourceURL = capturedSourceURL
	}
	captureMu.Unlock()
	if len(headers) > 0 {
		goto captured
	}
	select {
	case headers = <-headerCh:
		select {
		case sourceURL = <-sourceURLCh:
		default:
		}
	default:
	}

captured:
	if strings.TrimSpace(cookie) == "" {
		cookie = strings.TrimSpace(cookieHeader)
		names = cookieNames(cookie)
	}
	if strings.TrimSpace(cookie) == "" {
		return nil, errors.New("浏览器未采集到 cookie")
	}
	if sourceURL == "" {
		sourceURL = c.baseURL + "/"
	}
	if headers.Get("Origin") == "" {
		headers.Set("Origin", c.baseURL)
	}
	if headers.Get("Referer") == "" {
		headers.Set("Referer", c.baseURL+"/")
	}
	if headers.Get("Cookie") != "" {
		headers.Del("Cookie")
	}
	return &BrowserSession{
		Headers:     headers,
		Cookie:      cookie,
		CapturedAt:  time.Now(),
		Guest:       guest,
		SourceURL:   sourceURL,
		UserAgent:   headers.Get("User-Agent"),
		HasCookie:   strings.TrimSpace(cookie) != "",
		CookieNames: names,
	}, nil
}

func browserProbeScript(guest bool) string {
	chatMode := "normal"
	if guest {
		chatMode = "guest"
	}
	return fmt.Sprintf(`Promise.resolve()
  .then(() => fetch("/api/v2/users/status", {
    method: "POST",
    credentials: "include",
    headers: {"content-type": "application/json"},
    body: JSON.stringify({typarms: {
      typarm1: "web", typarm2: "", typarm3: "prod", typarm4: "qwen_chat", typarm5: "product", typarm6: "",
      orgid: "tongyi", share_id: "", project_id: "", channel_type: "", community_type: "", from_id: "",
      cdn_version: "0.2.68", spmId: "a2ty_o01.29997170", aemPageId: "//chat.qwen.ai/c/new-chat", domain: "chat.qwen.ai"
    }})
  }).catch(() => null))
  .then(() => fetch("/api/v2/chats/new", {
    method: "POST",
    credentials: "include",
    headers: {"content-type": "application/json"},
    body: JSON.stringify({
      title: "新建对话",
      models: ["qwen3.7-plus"],
      chat_mode: %q,
      chat_type: "t2t",
      timestamp: Date.now(),
      project_id: ""
    })
  }).catch(() => null));`, chatMode)
}

func (c *Client) browserExecutablePath() string {
	if path := strings.TrimSpace(c.browserAuth.ExecutablePath); path != "" {
		return path
	}
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("LocalAppData"), "Microsoft", "Edge", "Application", "msedge.exe"),
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func (c *Client) injectBrowserCookies(cookieHeader string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		cookies := map[string]string{}
		mergeCookieHeader(cookies, cookieHeader)
		if len(cookies) == 0 {
			return nil
		}
		for name, value := range cookies {
			if strings.TrimSpace(name) == "" || strings.TrimSpace(value) == "" {
				continue
			}
			err := network.SetCookie(name, value).
				WithURL(c.baseURL + "/").
				WithPath("/").
				WithSecure(strings.HasPrefix(strings.ToLower(c.baseURL), "https://")).
				Do(ctx)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func browserHeadersFromNetwork(headers network.Headers) http.Header {
	result := http.Header{}
	for key, raw := range headers {
		key = canonicalBrowserHeaderKey(key)
		if _, ok := browserHeaderAllowlist[key]; !ok {
			continue
		}
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value == "" {
			continue
		}
		result.Set(key, value)
	}
	return result
}

func browserCookieHeader(rawURL string, out *string, names *[]string) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		cookies, err := network.GetCookies().WithURLs([]string{rawURL}).Do(ctx)
		if err != nil {
			return err
		}
		values := make(map[string]string, len(cookies))
		for _, cookie := range cookies {
			if cookie == nil || strings.TrimSpace(cookie.Name) == "" || strings.TrimSpace(cookie.Value) == "" {
				continue
			}
			values[cookie.Name] = cookie.Value
		}
		header := formatCookieMap(values)
		*out = header
		*names = cookieNames(header)
		return nil
	})
}

func (c *Client) setBrowserSession(kind browserSessionKind, session *BrowserSession) {
	if session == nil {
		return
	}
	c.browserSessions.mu.Lock()
	defer c.browserSessions.mu.Unlock()
	if kind == browserSessionGuest {
		copy := cloneBrowserSession(session)
		c.browserSessions.guest = &copy
		return
	}
	if c.browserSessions.accounts == nil {
		c.browserSessions.accounts = map[string]*BrowserSession{}
	}
	copy := cloneBrowserSession(session)
	c.browserSessions.accounts["auth"] = &copy
}

func (c *Client) setBrowserSessionForKey(key string, session *BrowserSession) {
	if session == nil {
		return
	}
	key = normalizeBrowserSessionKey(key)
	if key == "" {
		return
	}
	if key == "guest" {
		c.setBrowserSession(browserSessionGuest, session)
		return
	}
	c.browserSessions.mu.Lock()
	defer c.browserSessions.mu.Unlock()
	if c.browserSessions.accounts == nil {
		c.browserSessions.accounts = map[string]*BrowserSession{}
	}
	copy := cloneBrowserSession(session)
	c.browserSessions.accounts[key] = &copy
}

func (c *Client) browserSessionForRequest(ctx context.Context, token string) *BrowserSession {
	c.browserSessions.mu.RLock()
	defer c.browserSessions.mu.RUnlock()

	var session *BrowserSession
	if strings.TrimSpace(token) == "" {
		session = c.browserSessions.guest
	} else {
		key := accountKeyFromContext(ctx)
		if key != "" {
			session = c.browserSessions.accounts[key]
		}
		if session == nil {
			session = c.browserSessions.accounts["auth"]
		}
	}
	if session == nil || strings.TrimSpace(session.Cookie) == "" || time.Since(session.CapturedAt) > browserSessionTTL {
		return nil
	}
	copy := cloneBrowserSession(session)
	return &copy
}

func (c *Client) applyBrowserSessionHeaders(req *http.Request, session *BrowserSession, options RequestOptions) {
	if req == nil || session == nil {
		return
	}
	for key, values := range session.Headers {
		key = canonicalBrowserHeaderKey(key)
		if key == "Cookie" {
			continue
		}
		if _, ok := browserHeaderAllowlist[key]; !ok {
			continue
		}
		req.Header.Del(key)
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				req.Header.Add(key, value)
			}
		}
	}
	req.Header.Set("Origin", c.baseURL)
	if strings.TrimSpace(options.Referer) != "" {
		req.Header.Set("Referer", options.Referer)
	}
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", c.baseURL+"/")
	}
}

func (c *Client) maybeCaptureGuestBrowserSession(ctx context.Context) (*BrowserSession, error) {
	if !c.browserAuth.Enabled {
		return nil, errors.New("browser auth disabled")
	}
	return c.CaptureGuestBrowserSession(ctx)
}

func (c *Client) BrowserAuthEnabled() bool {
	return c.browserAuth.Enabled
}

func (c *Client) setGuestCookieHeader(cookieHeader string) {
	if strings.TrimSpace(cookieHeader) == "" {
		return
	}
	c.guestMu.Lock()
	defer c.guestMu.Unlock()
	c.guestAuth = guestAuthState{
		cookieHeader: strings.TrimSpace(cookieHeader),
		refreshedAt:  time.Now(),
	}
}

func cloneBrowserSession(session *BrowserSession) BrowserSession {
	if session == nil {
		return BrowserSession{}
	}
	copy := *session
	copy.Headers = session.Headers.Clone()
	copy.CookieNames = append([]string(nil), session.CookieNames...)
	return copy
}

func summarizeBrowserSession(session *BrowserSession) map[string]any {
	if session == nil {
		return map[string]any{
			"captured": false,
		}
	}
	return map[string]any{
		"captured":    true,
		"capturedAt":  session.CapturedAt.UTC().Format(time.RFC3339),
		"guest":       session.Guest,
		"sourceUrl":   session.SourceURL,
		"userAgent":   session.UserAgent,
		"hasCookie":   session.HasCookie,
		"cookieNames": append([]string(nil), session.CookieNames...),
		"headers":     headerNames(session.Headers),
	}
}

func canonicalBrowserHeaderKey(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "sec-ch-ua":
		return "sec-ch-ua"
	case "sec-ch-ua-arch":
		return "sec-ch-ua-arch"
	case "sec-ch-ua-bitness":
		return "sec-ch-ua-bitness"
	case "sec-ch-ua-full-version":
		return "sec-ch-ua-full-version"
	case "sec-ch-ua-full-version-list":
		return "sec-ch-ua-full-version-list"
	case "sec-ch-ua-mobile":
		return "sec-ch-ua-mobile"
	case "sec-ch-ua-platform":
		return "sec-ch-ua-platform"
	case "sec-ch-ua-platform-version":
		return "sec-ch-ua-platform-version"
	case "sec-fetch-storage-access":
		return "sec-fetch-storage-access"
	case "sec-gpc":
		return "sec-gpc"
	default:
		return http.CanonicalHeaderKey(key)
	}
}

func cookieNames(cookieHeader string) []string {
	names := make([]string, 0)
	seen := map[string]struct{}{}
	for _, part := range strings.Split(cookieHeader, ";") {
		name, _, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func headerNames(headers http.Header) []string {
	names := make([]string, 0, len(headers))
	for key := range headers {
		names = append(names, key)
	}
	return names
}

func WithAccountKey(ctx context.Context, accountKey string) context.Context {
	accountKey = normalizeBrowserSessionKey(accountKey)
	if accountKey == "" {
		return ctx
	}
	return context.WithValue(ctx, accountKeyContextKey{}, accountKey)
}

func accountKeyFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	key, _ := ctx.Value(accountKeyContextKey{}).(string)
	return normalizeBrowserSessionKey(key)
}

func normalizeBrowserSessionKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}
