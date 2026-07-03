package qwen

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"time"

	"qwen2api/internal/ssxmod"
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
	body    any
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

	if c.browserAuth.Enabled {
		session, err := c.captureBrowserSession(ctx, "", true)
		if err == nil && strings.TrimSpace(session.Cookie) != "" {
			c.setBrowserSession(browserSessionGuest, session)
			c.guestAuth = guestAuthState{
				cookieHeader: session.Cookie,
				refreshedAt:  time.Now(),
			}
			return session.Cookie, nil
		}
		if err != nil && c.logger != nil {
			c.logger.WarnModule("UPSTREAM", "browser guest cookie capture failed, falling back to HTTP bootstrap err=%v", err)
		}
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
	fp := guestRequestFingerprint()

	browserHeaders := http.Header{
		"User-Agent":                []string{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36"},
		"Accept":                    []string{"text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8"},
		"Accept-Language":           []string{"zh-CN,zh;q=0.9,en;q=0.8"},
		"Accept-Encoding":           []string{"gzip, deflate, br"},
		"Connection":                []string{"keep-alive"},
		"Upgrade-Insecure-Requests": []string{"1"},
	}
	apiHeaders := http.Header{
		"User-Agent":                  []string{fp.UserAgent},
		"Accept":                      []string{"application/json, text/plain, */*"},
		"Accept-Language":             []string{fp.AcceptLanguage},
		"Accept-Encoding":             []string{fp.AcceptEncoding},
		"Connection":                  []string{"keep-alive"},
		"Version":                     []string{qwenWebVersion},
		"bx-v":                        []string{qwenBxVersion},
		"Source":                      []string{"web"},
		"Timezone":                    []string{fp.Timezone},
		"sec-ch-ua":                   []string{fp.SecChUA},
		"sec-ch-ua-full-version":      []string{fp.SecChUAFullVersion},
		"sec-ch-ua-full-version-list": []string{fp.SecChUAFullVersionList},
		"sec-ch-ua-platform":          []string{fp.SecChUAPlatform},
		"sec-ch-ua-platform-version":  []string{fp.SecChUAPlatformVersion},
		"sec-ch-ua-mobile":            []string{fp.SecChUAMobile},
		"sec-ch-ua-arch":              []string{fp.SecChUAArch},
		"sec-ch-ua-bitness":           []string{fp.SecChUABitness},
		"Cache-Control":               []string{fp.CacheControl},
		"Pragma":                      []string{fp.Pragma},
		"Priority":                    []string{fp.Priority},
		"DNT":                         []string{fp.DNT},
		"Content-Type":                []string{"application/json"},
		"X-Request-Id":                []string{newRequestID()},
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
	if strings.TrimSpace(cookies["ssxmod_itna"]) == "" || strings.TrimSpace(cookies["ssxmod_itna2"]) == "" {
		manager := ssxmod.NewManager()
		itna, itna2 := manager.Get()
		if strings.TrimSpace(cookies["ssxmod_itna"]) == "" {
			cookies["ssxmod_itna"] = itna
		}
		if strings.TrimSpace(cookies["ssxmod_itna2"]) == "" {
			cookies["ssxmod_itna2"] = itna2
		}
	}
	if len(bootstrapErrs) > 0 && c.logger != nil {
		c.logger.WarnModule("UPSTREAM", "guest cookie bootstrap degraded, using synthetic defaults where needed err=%v", errors.Join(bootstrapErrs...))
	}
	return formatCookieMap(cookies), nil
}

func (c *Client) guestBootstrapRequests(browserHeaders, apiHeaders http.Header) []guestBootstrapRequest {
	domain := ""
	if parsed, err := url.Parse(c.baseURL); err == nil {
		domain = parsed.Host
	}
	beaconHeaders := apiHeaders.Clone()
	beaconHeaders.Del("Version")
	beaconHeaders.Del("Source")
	beaconHeaders.Del("Timezone")
	beaconHeaders.Set("Content-Type", "text/plain;charset=UTF-8")
	beaconHeaders.Set("Priority", "u=6")

	return []guestBootstrapRequest{
		{method: http.MethodGet, url: c.baseURL + "/", headers: browserHeaders, timeout: guestBootstrapHomeTimeout},
		{method: http.MethodGet, url: c.baseURL + "/api/v2/configs/", headers: apiHeaders, timeout: guestBootstrapAPITimeout},
		{method: http.MethodGet, url: c.baseURL + "/api/v2/configs/setting-config", headers: apiHeaders, timeout: guestBootstrapAPITimeout},
		{method: http.MethodPost, url: c.baseURL + "/api/v2/users/status", headers: beaconHeaders, body: guestBeaconStatusPayload(domain), timeout: guestBootstrapAPITimeout},
		{method: http.MethodGet, url: c.baseURL + "/api/v2/configs/setting-config", headers: apiHeaders, timeout: guestBootstrapAPITimeout},
		{method: http.MethodGet, url: c.baseURL + "/api/v2/tts/config?omni_speakers=v1&audio_tts_speakers=v1&omni_language=v1&audio_tts_language=v1", headers: apiHeaders, timeout: guestBootstrapAPITimeout},
		{method: http.MethodGet, url: c.baseURL + "/api/v1/auths/", headers: apiHeaders, timeout: guestBootstrapAPITimeout},
		{method: http.MethodPost, url: c.baseURL + "/api/v2/users/status", headers: apiHeaders, body: guestStatusPayload(domain, "/", "a2ty_o01.29997169"), timeout: guestBootstrapAPITimeout},
	}
}

func (c *Client) warmGuestNewChat(ctx context.Context) {
	c.warmGuestRoute(ctx, "/c/new-chat", "a2ty_o01.29997170", true)
}

func (c *Client) warmGuestAfterNewChat(ctx context.Context) {
	c.warmGuestSettingConfig(ctx, "/c/new-chat")
	c.warmGuestRoute(ctx, "/c/guest", "a2ty_o01.29997172", false)
}

func (c *Client) warmGuestSettingConfig(ctx context.Context, refererPath string) {
	headers := c.guestAPIHeaders(refererPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2/configs/setting-config", nil)
	if err != nil {
		return
	}
	req.Header = headers
	if cookieHeader, err := c.buildCookieHeader(ctx, "", cookieOptions{includeGuestBootstrap: true}); err == nil {
		req.Header.Set("Cookie", cookieHeader)
	}
	resp, err := c.do(req)
	if err != nil {
		if c.logger != nil {
			c.logger.DebugModule("UPSTREAM", "guest setting-config warmup skipped referer=%s err=%v", refererPath, err)
		}
		return
	}
	resp.Body.Close()
}

func (c *Client) warmGuestRoute(ctx context.Context, refererPath, spmID string, withPriority bool) {
	domain := ""
	if parsed, err := url.Parse(c.baseURL); err == nil {
		domain = parsed.Host
	}
	headers := c.guestAPIHeaders(refererPath)
	if withPriority {
		headers.Set("Priority", "u=0")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2/users/status", bytes.NewReader(mustJSON(guestStatusPayload(domain, refererPath, spmID))))
	if err != nil {
		return
	}
	req.Header = headers
	if cookieHeader, err := c.buildCookieHeader(ctx, "", cookieOptions{includeGuestBootstrap: true}); err == nil {
		req.Header.Set("Cookie", cookieHeader)
	}
	resp, err := c.do(req)
	if err != nil {
		if c.logger != nil {
			c.logger.DebugModule("UPSTREAM", "guest route warmup skipped referer=%s err=%v", refererPath, err)
		}
		return
	}
	resp.Body.Close()
}

func (c *Client) guestAPIHeaders(refererPath string) http.Header {
	fp := guestRequestFingerprint()
	headers := http.Header{
		"User-Agent":                  []string{fp.UserAgent},
		"Accept":                      []string{"application/json, text/plain, */*"},
		"Accept-Language":             []string{fp.AcceptLanguage},
		"Accept-Encoding":             []string{fp.AcceptEncoding},
		"Connection":                  []string{"keep-alive"},
		"Version":                     []string{qwenWebVersion},
		"bx-v":                        []string{qwenBxVersion},
		"Source":                      []string{"web"},
		"Timezone":                    []string{fp.Timezone},
		"sec-ch-ua":                   []string{fp.SecChUA},
		"sec-ch-ua-full-version":      []string{fp.SecChUAFullVersion},
		"sec-ch-ua-full-version-list": []string{fp.SecChUAFullVersionList},
		"sec-ch-ua-platform":          []string{fp.SecChUAPlatform},
		"sec-ch-ua-platform-version":  []string{fp.SecChUAPlatformVersion},
		"sec-ch-ua-mobile":            []string{fp.SecChUAMobile},
		"sec-ch-ua-arch":              []string{fp.SecChUAArch},
		"sec-ch-ua-bitness":           []string{fp.SecChUABitness},
		"Origin":                      []string{c.baseURL},
		"Referer":                     []string{c.baseURL + refererPath},
		"Sec-Fetch-Site":              []string{"same-origin"},
		"Sec-Fetch-Mode":              []string{"cors"},
		"Sec-Fetch-Dest":              []string{"empty"},
		"Cache-Control":               []string{fp.CacheControl},
		"Pragma":                      []string{fp.Pragma},
		"Content-Type":                []string{"application/json"},
		"X-Request-Id":                []string{newRequestID()},
		"DNT":                         []string{fp.DNT},
	}
	return headers
}

func guestBeaconStatusPayload(domain string) map[string]any {
	return map[string]any{
		"typarms": map[string]any{
			"logId":       randomHex(40),
			"timestamp":   time.Now().UnixMilli(),
			"domain":      domain,
			"testTag":     "compareLogService",
			"testVersion": "5.0.0",
			"serviceName": "tongyiLogService",
			"requestType": "sendBeacon",
		},
	}
}

func guestStatusPayload(domain, pagePath, spmID string) map[string]any {
	if strings.TrimSpace(pagePath) == "" {
		pagePath = "/"
	}
	aemPageID := "//" + domain + pagePath
	if pagePath == "/" {
		aemPageID = "//" + domain + "/"
	}
	return map[string]any{
		"typarms": map[string]any{
			"typarm1":        "web",
			"typarm2":        "",
			"typarm3":        "prod",
			"typarm4":        "qwen_chat",
			"typarm5":        "product",
			"typarm6":        "",
			"orgid":          "tongyi",
			"share_id":       "",
			"project_id":     "",
			"channel_type":   "",
			"community_type": "",
			"from_id":        "",
			"cdn_version":    qwenWebVersion,
			"spmId":          spmID,
			"aemPageId":      aemPageID,
			"domain":         domain,
		},
	}
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
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

	var bodyReader io.Reader
	if request.body != nil {
		rawBody, err := json.Marshal(request.body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(rawBody)
	}
	req, err := http.NewRequestWithContext(requestCtx, request.method, request.url, bodyReader)
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
	if region := strings.TrimSpace(resp.Header.Get("ga-ap")); region != "" {
		cookies["x-ap"] = region
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
	return randomAlphaNumeric(10) + "ICAUB2nKgBJlzs"
}

func formatCookieMap(cookies map[string]string) string {
	if len(cookies) == 0 {
		return ""
	}
	preferred := []string{
		"token",
		"cna",
		"xlly_s",
		"sca",
		"x-ap",
		"qwen-theme",
		"qwen-locale",
		"aui",
		"cnaui",
		"acw_tc",
		"atpsida",
		"_bl_uid",
		"isg",
		"tfstk",
		"ssxmod_itna",
		"ssxmod_itna2",
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
	seen := make(map[string]struct{}, len(preferred))
	for _, key := range preferred {
		value := strings.TrimSpace(cookies[key])
		if strings.TrimSpace(key) == "" || value == "" {
			continue
		}
		parts = append(parts, key+"="+value)
		seen[key] = struct{}{}
	}
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		value := strings.TrimSpace(cookies[key])
		if strings.TrimSpace(key) == "" || value == "" {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, "; ")
}

func (c *Client) rememberAnonymousResponse(req *http.Request, resp *http.Response) {
	if req == nil || resp == nil || !isAnonymousRequest(req) {
		return
	}
	cookies := map[string]string{}

	c.guestMu.RLock()
	mergeCookieHeader(cookies, c.guestAuth.cookieHeader)
	c.guestMu.RUnlock()

	for _, cookie := range resp.Cookies() {
		if cookie == nil {
			continue
		}
		name := strings.TrimSpace(cookie.Name)
		value := strings.TrimSpace(cookie.Value)
		if name == "" || value == "" {
			continue
		}
		cookies[name] = value
	}
	if region := strings.TrimSpace(resp.Header.Get("ga-ap")); region != "" {
		cookies["x-ap"] = region
	}
	if len(cookies) == 0 {
		return
	}
	fillGuestCookieDefaults(cookies)
	header := formatCookieMap(cookies)
	if header == "" {
		return
	}
	c.setGuestCookieHeader(header)
	c.setGuestBrowserSessionCookie(header)
}

func mergeCookieHeader(cookies map[string]string, header string) {
	if cookies == nil {
		return
	}
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || value == "" {
			continue
		}
		cookies[name] = value
	}
}

func (c *Client) setGuestBrowserSessionCookie(cookieHeader string) {
	if strings.TrimSpace(cookieHeader) == "" {
		return
	}
	c.browserSessions.mu.Lock()
	defer c.browserSessions.mu.Unlock()
	if c.browserSessions.guest == nil {
		return
	}
	copy := cloneBrowserSession(c.browserSessions.guest)
	copy.Cookie = cookieHeader
	copy.CookieNames = cookieNames(cookieHeader)
	copy.HasCookie = true
	copy.CapturedAt = time.Now()
	c.browserSessions.guest = &copy
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
