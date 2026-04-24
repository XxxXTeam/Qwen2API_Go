package qwen

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"

	"qwen2api/internal/config"
	"qwen2api/internal/logging"
	"qwen2api/internal/ssxmod"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *logging.Logger
	modelsMu   sync.RWMutex
	models     []Model
	modelsAt   time.Time
	ssxmod     *ssxmod.Manager
}

func NewClient(cfg config.Config, logger *logging.Logger) *Client {
	transport := &http.Transport{}
	if strings.TrimSpace(cfg.ProxyURL) != "" {
		if proxyURL, err := url.Parse(cfg.ProxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &Client{
		baseURL: strings.TrimRight(cfg.QwenChatProxyURL, "/"),
		httpClient: &http.Client{
			Timeout:   5 * time.Minute,
			Transport: transport,
		},
		logger: logger,
		ssxmod: ssxmod.NewManager(),
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string, token string, body any) (*http.Request, error) {
	return c.newRequestWithOptions(ctx, method, path, token, body, RequestOptions{
		Accept:      "application/json",
		ContentType: "application/json",
		IncludeAuth: true,
	})
}

func (c *Client) newRequestWithOptions(ctx context.Context, method, path string, token string, body any, options RequestOptions) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}

	if options.Accept == "" {
		options.Accept = "application/json"
	}
	if options.ContentType == "" && body != nil {
		options.ContentType = "application/json"
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Accept", options.Accept)
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7")
	if options.ContentType != "" {
		req.Header.Set("Content-Type", options.ContentType)
	}
	req.Header.Set("Timezone", defaultTimezoneHeader)
	req.Header.Set("sec-ch-ua", `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`)
	req.Header.Set("Origin", c.baseURL)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Referer", c.baseURL+"/c/guest")
	req.Header.Set("source", "web")
	req.Header.Set("Version", "0.1.13")
	req.Header.Set("bx-v", "2.5.31")
	req.Header.Set("X-Request-Timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))

	rawToken := strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(rawToken), "bearer ") {
		rawToken = strings.TrimSpace(strings.TrimPrefix(rawToken, "Bearer "))
	}
	if options.IncludeAuth && rawToken != "" {
		req.Header.Set("Authorization", "Bearer "+rawToken)
	}
	req.Header.Set("Cookie", c.buildCookieHeader(rawToken))

	return req, nil
}

func (c *Client) buildCookieHeader(token string) string {
	ssxmodITNA, ssxmodITNA2 := c.ssxmod.Get()
	parts := make([]string, 0, 3)
	if strings.TrimSpace(token) != "" {
		parts = append(parts, "token="+strings.TrimSpace(token))
	}
	if strings.TrimSpace(ssxmodITNA) != "" {
		parts = append(parts, "ssxmod_itna="+ssxmodITNA)
	}
	if strings.TrimSpace(ssxmodITNA2) != "" {
		parts = append(parts, "ssxmod_itna2="+ssxmodITNA2)
	}
	return strings.Join(parts, "; ")
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	start := time.Now()
	bodyPreview := requestBodyPreview(req)
	if bodyPreview != "" {
		c.logger.DebugModule("UPSTREAM", "upstream request method=%s url=%s accept=%s content_type=%s auth=%t body=%s", req.Method, req.URL.String(), req.Header.Get("Accept"), req.Header.Get("Content-Type"), strings.TrimSpace(req.Header.Get("Authorization")) != "", bodyPreview)
	} else {
		c.logger.DebugModule("UPSTREAM", "upstream request method=%s url=%s accept=%s content_type=%s auth=%t", req.Method, req.URL.String(), req.Header.Get("Accept"), req.Header.Get("Content-Type"), strings.TrimSpace(req.Header.Get("Authorization")) != "")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WarnModule("UPSTREAM", "upstream request failed method=%s url=%s duration=%s err=%v", req.Method, req.URL.String(), time.Since(start), err)
		return nil, err
	}
	if err := decodeCompressedBody(resp); err != nil {
		resp.Body.Close()
		c.logger.WarnModule("UPSTREAM", "upstream decode failed method=%s url=%s duration=%s err=%v", req.Method, req.URL.String(), time.Since(start), err)
		return nil, err
	}
	c.logger.DebugModule("UPSTREAM", "upstream response method=%s url=%s status=%d duration=%s content_type=%s", req.Method, req.URL.String(), resp.StatusCode, time.Since(start), resp.Header.Get("Content-Type"))
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		trimmedBody := strings.TrimSpace(string(body))
		c.logger.WarnModule("UPSTREAM", "upstream non-success method=%s url=%s status=%d duration=%s body=%q", req.Method, req.URL.String(), resp.StatusCode, time.Since(start), trimmedBody)
		return nil, fmt.Errorf("上游请求失败 (%d): %s", resp.StatusCode, trimmedBody)
	}
	return resp, nil
}

func requestBodyPreview(req *http.Request) string {
	if req == nil || req.GetBody == nil {
		return ""
	}
	body, err := req.GetBody()
	if err != nil {
		return ""
	}
	defer body.Close()
	raw, err := io.ReadAll(body)
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(raw))
	if len(text) > 1200 {
		return text[:1200] + "...(truncated)"
	}
	return text
}

func decodeCompressedBody(resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return nil
	}

	encoding := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	switch encoding {
	case "", "identity":
		return nil
	case "gzip":
		originalBody := resp.Body
		reader, err := gzip.NewReader(originalBody)
		if err != nil {
			return fmt.Errorf("解析 gzip 响应失败: %w", err)
		}
		resp.Body = &decodedReadCloser{
			Reader: reader,
			closeFn: func() error {
				closeErr := reader.Close()
				bodyErr := originalBody.Close()
				if closeErr != nil {
					return closeErr
				}
				return bodyErr
			},
		}
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.ContentLength = -1
		return nil
	case "deflate":
		originalBody := resp.Body
		reader := flate.NewReader(originalBody)
		resp.Body = &decodedReadCloser{
			Reader: reader,
			closeFn: func() error {
				closeErr := reader.Close()
				bodyErr := originalBody.Close()
				if closeErr != nil {
					return closeErr
				}
				return bodyErr
			},
		}
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.ContentLength = -1
		return nil
	default:
		return nil
	}
}

type decodedReadCloser struct {
	io.Reader
	closeFn func() error
}

func (d *decodedReadCloser) Close() error {
	if d.closeFn == nil {
		return nil
	}
	return d.closeFn()
}

func (c *Client) SignIn(ctx context.Context, email, hashedPassword string) (string, error) {
	req, err := c.newRequestWithOptions(ctx, http.MethodPost, "/api/v1/auths/signin", "", map[string]any{
		"email":    email,
		"password": hashedPassword,
	}, RequestOptions{
		Accept:      "application/json",
		ContentType: "application/json",
		IncludeAuth: false,
	})
	if err != nil {
		return "", err
	}
	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var payload struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.Token) == "" {
		return "", errors.New("登录响应缺少令牌")
	}
	return payload.Token, nil
}

type Model struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Info map[string]any `json:"info"`
}

type stsTokenResponse struct {
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	SecurityToken   string `json:"security_token"`
	FileURL         string `json:"file_url"`
	FilePath        string `json:"file_path"`
	FileID          string `json:"file_id"`
	BucketName      string `json:"bucketname"`
	Region          string `json:"region"`
}

func (c *Client) ListModels(ctx context.Context, token string) ([]Model, error) {
	c.modelsMu.RLock()
	if len(c.models) > 0 && time.Since(c.modelsAt) < 5*time.Minute {
		cached := append([]Model(nil), c.models...)
		c.modelsMu.RUnlock()
		return cached, nil
	}
	c.modelsMu.RUnlock()

	req, err := c.newRequest(ctx, http.MethodGet, "/api/models", token, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload struct {
		Data []Model `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	c.modelsMu.Lock()
	c.models = append([]Model(nil), payload.Data...)
	c.modelsAt = time.Now()
	c.modelsMu.Unlock()

	return append([]Model(nil), payload.Data...), nil
}

func (c *Client) NewChat(ctx context.Context, token, model string) (string, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v2/chats/new", token, map[string]any{
		"title":     "New Chat",
		"models":    []string{model},
		"chat_mode": "local",
		"chat_type": "t2i",
		"timestamp": time.Now().UnixMilli(),
	})
	if err != nil {
		return "", err
	}
	resp, err := c.do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var payload struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.Data.ID) == "" {
		return "", errors.New("生成 chat_id 失败")
	}
	return payload.Data.ID, nil
}

func (c *Client) ChatCompletions(ctx context.Context, token, chatID string, body map[string]any) (*http.Response, error) {
	req, err := c.newRequestWithOptions(ctx, http.MethodPost, "/api/v2/chat/completions?chat_id="+url.QueryEscape(chatID), token, body, RequestOptions{
		Accept:      "text/event-stream",
		ContentType: "application/json",
		IncludeAuth: true,
	})
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *Client) GetChatDetail(ctx context.Context, token, chatID string) (map[string]any, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v2/chats/"+url.PathEscape(chatID), token, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) GetVideoTaskStatus(ctx context.Context, token, taskID string) (map[string]any, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v1/tasks/status/"+url.PathEscape(taskID), token, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) UploadFile(ctx context.Context, token, filename string, content []byte, contentType string) (string, string, error) {
	if len(content) == 0 {
		return "", "", errors.New("上传内容不能为空")
	}

	if strings.TrimSpace(contentType) == "" {
		contentType = mime.TypeByExtension(filepath.Ext(filename))
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}

	fileType := simpleFileType(contentType)
	sts, err := c.getSTSToken(ctx, token, filename, len(content), fileType)
	if err != nil {
		return "", "", err
	}

	endpoint := sts.Region + ".aliyuncs.com"
	client, err := oss.New(
		endpoint,
		sts.AccessKeyID,
		sts.AccessKeySecret,
		oss.SecurityToken(sts.SecurityToken),
		oss.UseCname(false),
	)
	if err != nil {
		return "", "", err
	}

	bucket, err := client.Bucket(sts.BucketName)
	if err != nil {
		return "", "", err
	}

	err = bucket.PutObject(
		sts.FilePath,
		bytes.NewReader(content),
		oss.ContentType(contentType),
	)
	if err != nil {
		return "", "", err
	}

	return sts.FileURL, sts.FileID, nil
}

func (c *Client) getSTSToken(ctx context.Context, token, filename string, filesize int, fileType string) (*stsTokenResponse, error) {
	req, err := c.newRequest(ctx, http.MethodPost, "/api/v1/files/getstsToken", token, map[string]any{
		"filename": filename,
		"filesize": filesize,
		"filetype": fileType,
	})
	if err != nil {
		return nil, err
	}

	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload stsTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.AccessKeyID == "" || payload.AccessKeySecret == "" || payload.SecurityToken == "" {
		return nil, errors.New("STS 响应缺少凭证字段")
	}
	if payload.FileURL == "" || payload.FilePath == "" || payload.BucketName == "" || payload.Region == "" {
		return nil, errors.New("STS 响应缺少文件字段")
	}
	return &payload, nil
}

func simpleFileType(contentType string) string {
	mainType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, "/")[0]))
	switch mainType {
	case "image", "video", "audio", "text", "application":
		if mainType == "application" {
			return "document"
		}
		return mainType
	default:
		return "file"
	}
}
