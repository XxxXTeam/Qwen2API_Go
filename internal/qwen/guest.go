package qwen

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	guestCookieTTL            = 30 * time.Minute
	guestBootstrapHomeTimeout = 30 * time.Second
	guestBootstrapAPITimeout  = 10 * time.Second
)

type guestAuthState struct {
	cookieHeader string
	refreshedAt  time.Time
}

type guestBootstrapRequest struct {
	method  string
	url     string
	headers http.Header
	body    io.Reader
	timeout time.Duration
}

func (c *Client) EnsureGuestCookieHeader(ctx context.Context) (string, error) {
	return c.ensureGuestCookieHeader(ctx, false)
}

func (c *Client) RefreshGuestCookieHeader(ctx context.Context) (string, error) {
	return c.ensureGuestCookieHeader(ctx, true)
}

func (c *Client) ensureGuestCookieHeader(ctx context.Context, force bool) (string, error) {
	c.guestMu.RLock()
	cached := c.guestAuth
	c.guestMu.RUnlock()

	if !force && strings.TrimSpace(cached.cookieHeader) != "" && time.Since(cached.refreshedAt) < guestCookieTTL {
		return cached.cookieHeader, nil
	}

	c.guestMu.Lock()
	defer c.guestMu.Unlock()

	if !force && strings.TrimSpace(c.guestAuth.cookieHeader) != "" && time.Since(c.guestAuth.refreshedAt) < guestCookieTTL {
		return c.guestAuth.cookieHeader, nil
	}

	cookieHeader, err := c.fetchAnonymousCookieHeader(ctx)
	if err != nil {
		return "", err
	}
	c.guestAuth = guestAuthState{
		cookieHeader: cookieHeader,
		refreshedAt:  time.Now(),
	}
	return cookieHeader, nil
}

func (c *Client) fetchAnonymousCookieHeader(ctx context.Context) (string, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return "", err
	}

	bootstrapClient := c.bootstrapHTTPClient(jar)
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return "", err
	}

	browserHeaders := http.Header{
		"User-Agent":                []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"},
		"Accept":                    []string{"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"},
		"Accept-Language":           []string{"zh-CN,zh;q=0.9,en;q=0.8"},
		"Accept-Encoding":           []string{"gzip, deflate, br"},
		"Connection":                []string{"keep-alive"},
		"Upgrade-Insecure-Requests": []string{"1"},
	}
	apiHeaders := http.Header{
		"User-Agent":      []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"},
		"Accept":          []string{"application/json, text/plain, */*"},
		"Accept-Language": []string{"zh-CN,zh;q=0.9,en;q=0.8"},
		"Accept-Encoding": []string{"gzip, deflate, br"},
		"Connection":      []string{"keep-alive"},
		"Version":         []string{"0.2.45"},
		"Source":          []string{"web"},
		"Timezone":        []string{time.Now().Format("Mon Jan 02 2006 15:04:05 GMT-0700")},
	}

	cookies := map[string]string{
		"cna": generateCNA(),
	}

	var bootstrapErrs []error
	for _, request := range c.guestBootstrapRequests(browserHeaders, apiHeaders) {
		if err := c.bootstrapGuestRequest(ctx, bootstrapClient, base, cookies, request); err != nil {
			bootstrapErrs = append(bootstrapErrs, err)
		}
	}

	for _, cookie := range jar.Cookies(base) {
		if strings.TrimSpace(cookie.Name) == "" || strings.TrimSpace(cookie.Value) == "" {
			continue
		}
		cookies[cookie.Name] = cookie.Value
	}
	if strings.TrimSpace(cookies["cna"]) == "" || len(strings.TrimSpace(cookies["cna"])) < 20 {
		cookies["cna"] = generateCNA()
	}
	fillGuestCookieDefaults(cookies)
	if len(bootstrapErrs) > 0 && c.logger != nil {
		c.logger.WarnModule("UPSTREAM", "guest cookie bootstrap degraded, using synthetic defaults where needed err=%v", errors.Join(bootstrapErrs...))
	}
	return formatCookieMap(cookies), nil
}

func (c *Client) guestBootstrapRequests(browserHeaders, apiHeaders http.Header) []guestBootstrapRequest {
	return []guestBootstrapRequest{
		{method: http.MethodGet, url: c.baseURL + "/", headers: browserHeaders, timeout: guestBootstrapHomeTimeout},
		{method: http.MethodGet, url: c.baseURL + "/api/v2/configs/", headers: apiHeaders, timeout: guestBootstrapAPITimeout},
		{method: http.MethodGet, url: c.baseURL + "/api/v2/configs/setting-config", headers: apiHeaders, timeout: guestBootstrapAPITimeout},
		{method: http.MethodGet, url: c.baseURL + "/api/v2/tts/config?omni_speakers=v1&audio_tts_speakers=v1&omni_language=v1&audio_tts_language=v1", headers: apiHeaders, timeout: guestBootstrapAPITimeout},
		{method: http.MethodPost, url: c.baseURL + "/api/v2/users/status", headers: apiHeaders, body: http.NoBody, timeout: guestBootstrapAPITimeout},
		{method: http.MethodGet, url: c.baseURL + "/api/v1/auths/", headers: apiHeaders, timeout: guestBootstrapAPITimeout},
	}
}

func (c *Client) bootstrapHTTPClient(jar http.CookieJar) *http.Client {
	timeout := guestBootstrapHomeTimeout
	if c.httpClient != nil && c.httpClient.Timeout > 0 && c.httpClient.Timeout < timeout {
		timeout = c.httpClient.Timeout
	}

	bootstrap := &http.Client{
		Timeout: timeout,
		Jar:     jar,
	}
	if c.httpClient == nil {
		return bootstrap
	}
	bootstrap.Transport = c.httpClient.Transport
	bootstrap.CheckRedirect = c.httpClient.CheckRedirect
	return bootstrap
}

func (c *Client) bootstrapGuestRequest(ctx context.Context, client *http.Client, base *url.URL, cookies map[string]string, request guestBootstrapRequest) error {
	requestCtx := ctx
	cancel := func() {}
	if request.timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, request.timeout)
	}
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, request.method, request.url, request.body)
	if err != nil {
		return err
	}
	req.Header = request.headers.Clone()
	if base != nil && len(cookies) > 0 {
		req.Header.Set("Cookie", formatCookieMap(cookies))
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if base != nil {
		for _, cookie := range client.Jar.Cookies(base) {
			if strings.TrimSpace(cookie.Name) == "" || strings.TrimSpace(cookie.Value) == "" {
				continue
			}
			cookies[cookie.Name] = cookie.Value
		}
	}
	return nil
}

func fillGuestCookieDefaults(cookies map[string]string) {
	if cookies == nil {
		return
	}
	if _, ok := cookies["cna"]; !ok {
		cookies["cna"] = generateCNA()
	}
	if _, ok := cookies["_bl_uid"]; !ok {
		cookies["_bl_uid"] = randomFromAlphabet(28, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	}
	if _, ok := cookies["atpsida"]; !ok {
		cookies["atpsida"] = randomHex(24) + fmt.Sprintf("_%d_1", time.Now().Unix())
	}
	if _, ok := cookies["x-ap"]; !ok {
		cookies["x-ap"] = "eu-central-1"
	}
	if _, ok := cookies["sca"]; !ok {
		cookies["sca"] = randomHex(8)
	}
	if _, ok := cookies["xlly_s"]; !ok {
		cookies["xlly_s"] = "1"
	}
	if _, ok := cookies["qwen-theme"]; !ok {
		cookies["qwen-theme"] = "light"
	}
	if _, ok := cookies["qwen-locale"]; !ok {
		cookies["qwen-locale"] = "zh-CN"
	}
	if _, ok := cookies["isg"]; !ok {
		prefix := "BG"
		if randomInt(2) == 1 {
			prefix = "BA"
		}
		cookies["isg"] = prefix + randomFromAlphabet(50, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_")
	}
	if _, ok := cookies["tfstk"]; !ok {
		cookies["tfstk"] = randomFromAlphabet(500, "abcdefghijklmnopqrstuvwxyz0123456789_-")
	}
}

func generateCNA() string {
	return randomAlphaNumeric(24) + "ICAUB2nKgBJlzs"
}

func formatCookieMap(cookies map[string]string) string {
	if len(cookies) == 0 {
		return ""
	}
	keys := make([]string, 0, len(cookies))
	for key, value := range cookies {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+cookies[key])
	}
	return strings.Join(parts, "; ")
}

func randomAlphaNumeric(length int) string {
	return randomFromAlphabet(length, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
}

func randomHex(length int) string {
	return randomFromAlphabet(length, "0123456789abcdef")
}

func randomFromAlphabet(length int, alphabet string) string {
	if length <= 0 || alphabet == "" {
		return ""
	}
	buf := make([]byte, length)
	random := make([]byte, length)
	if _, err := rand.Read(random); err != nil {
		for i := range buf {
			buf[i] = alphabet[i%len(alphabet)]
		}
		return string(buf)
	}
	for i := range buf {
		buf[i] = alphabet[int(random[i])%len(alphabet)]
	}
	return string(buf)
}

func randomInt(max int) int {
	if max <= 1 {
		return 0
	}
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0
	}
	return int(b[0]) % max
}
