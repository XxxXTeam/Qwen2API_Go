package openai

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"qwen2api/internal/account"
	"qwen2api/internal/config"
	"qwen2api/internal/logging"
	"qwen2api/internal/metrics"
	"qwen2api/internal/qwen"
	"qwen2api/internal/toolcall"
)

var dataURIExpr = regexp.MustCompile(`^data:([^;]+);base64,(.*)$`)

type Handler struct {
	cfg      config.Config
	runtime  *config.Runtime
	qwen     *qwen.Client
	accounts *account.Service
	sessions *ConversationSessionService
	metrics  *metrics.DashboardStats
	logger   *logging.Logger
}

func NewHandler(cfg config.Config, runtime *config.Runtime, qwenClient *qwen.Client, accounts *account.Service, sessions *ConversationSessionService, stats *metrics.DashboardStats, logger *logging.Logger) *Handler {
	return &Handler{
		cfg:      cfg,
		runtime:  runtime,
		qwen:     qwenClient,
		accounts: accounts,
		sessions: sessions,
		metrics:  stats,
		logger:   logger,
	}
}

type upstreamMessage struct {
	Role          string `json:"role"`
	Content       any    `json:"content"`
	ChatType      string `json:"chat_type"`
	Extra         any    `json:"extra"`
	FeatureConfig any    `json:"feature_config"`
}

func mergeSystemMessages(messages []map[string]any) []map[string]any {
	systemTexts := make([]string, 0)
	result := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if fmt.Sprint(message["role"]) == "system" {
			if text := extractText(message["content"]); strings.TrimSpace(text) != "" {
				systemTexts = append(systemTexts, text)
			}
			continue
		}
		result = append(result, message)
	}
	if len(systemTexts) > 0 {
		result = append([]map[string]any{{
			"role":    "system",
			"content": strings.Join(systemTexts, "\n\n"),
		}}, result...)
	}
	return result
}

func extractText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0)
		for _, item := range v {
			if obj, ok := item.(map[string]any); ok && fmt.Sprint(obj["type"]) == "text" {
				parts = append(parts, fmt.Sprint(obj["text"]))
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func chatTypeForModel(model string) string {
	switch {
	case strings.Contains(model, "-search"):
		return "search"
	case strings.Contains(model, "-image-edit"):
		return "image_edit"
	case strings.Contains(model, "-image"):
		return "t2i"
	case strings.Contains(model, "-video"):
		return "t2v"
	default:
		return "t2t"
	}
}

func isThinkingEnabled(model string, raw any) bool {
	if strings.Contains(model, "-thinking") {
		return true
	}
	if strings.Contains(model, "-fast") {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

func splitModelSuffix(model string) string {
	suffixes := []string{
		"-thinking-search",
		"-fast-search",
		"-image-edit",
		"-thinking",
		"-fast",
		"-search",
		"-video",
		"-image",
	}
	for _, suffix := range suffixes {
		if strings.HasSuffix(model, suffix) {
			return strings.TrimSuffix(model, suffix)
		}
	}
	return model
}

func modelSupports(meta map[string]any, feature string) bool {
	info, ok := meta["meta"].(map[string]any)
	if !ok {
		return false
	}

	if feature == "thinking" {
		abilities, ok := info["abilities"].(map[string]any)
		if !ok {
			return false
		}
		if enabled, ok := abilities["thinking"].(bool); ok {
			return enabled
		}
		return false
	}

	chatTypes, ok := info["chat_type"].([]any)
	if !ok {
		return false
	}
	for _, item := range chatTypes {
		if fmt.Sprint(item) == feature {
			return true
		}
	}
	return false
}

func (h *Handler) ResolveModel(ctx context.Context, requested string, chatType string) (string, error) {
	base := splitModelSuffix(strings.TrimSpace(requested))
	if base == "" {
		base = "qwen3-235b-a22b"
	}

	session, err := h.accounts.GetAccountSession()
	if err != nil {
		return base, nil
	}

	models, err := h.qwen.ListModels(ctx, session.Token)
	if err != nil {
		return base, nil
	}

	normalized := strings.ToLower(base)
	for _, model := range models {
		aliases := []string{
			strings.ToLower(model.ID),
			strings.ToLower(model.Name),
		}
		for _, alias := range aliases {
			if alias == normalized && strings.TrimSpace(model.ID) != "" {
				return model.ID, nil
			}
		}
	}

	if chatType == "t2i" || chatType == "t2v" || chatType == "image_edit" {
		for _, model := range models {
			if modelSupports(model.Info, chatType) {
				return model.ID, nil
			}
		}
	}

	return base, nil
}

func buildFeatureConfig(thinkingEnabled bool) map[string]any {
	config := map[string]any{
		"thinking_enabled": thinkingEnabled,
		"output_schema":    "phase",
		"research_mode":    "normal",
		"auto_thinking":    false,
		"auto_search":      true,
	}
	if thinkingEnabled {
		config["thinking_mode"] = "Thinking"
		config["thinking_format"] = "summary"
	} else {
		config["thinking_mode"] = "Fast"
	}
	return config
}

func extractThinkingSummary(extra map[string]any) string {
	if extra == nil {
		return ""
	}
	joinContent := func(value any) string {
		block, _ := value.(map[string]any)
		if block == nil {
			return ""
		}
		content, _ := block["content"].([]any)
		if len(content) == 0 {
			return ""
		}
		parts := make([]string, 0, len(content))
		for _, item := range content {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}

	title := joinContent(extra["summary_title"])
	thought := joinContent(extra["summary_thought"])
	switch {
	case title != "" && thought != "":
		return title + "\n" + thought
	case thought != "":
		return thought
	default:
		return title
	}
}

func extractDeltaContent(delta map[string]any) string {
	if delta == nil {
		return ""
	}
	content := fmt.Sprint(delta["content"])
	if strings.TrimSpace(content) != "" {
		return content
	}
	extra, _ := delta["extra"].(map[string]any)
	return extractThinkingSummary(extra)
}

func normalizeMessages(messages []map[string]any, chatType string, thinkingEnabled bool) []map[string]any {
	messages = mergeSystemMessages(messages)
	if len(messages) == 0 {
		return []map[string]any{{
			"role":    "user",
			"content": "user:",
		}}
	}

	systemText := ""
	conversation := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		if fmt.Sprint(message["role"]) == "system" {
			systemText = extractText(message["content"])
			continue
		}
		conversation = append(conversation, message)
	}

	if len(conversation) == 0 {
		conversation = []map[string]any{{"role": "user", "content": "user:"}}
	}

	if len(conversation) == 1 {
		normalized := convertContent(conversation[0]["content"])
		text := extractText(normalized)
		if systemText != "" {
			text = "system:" + systemText + "\n\n" + text
			switch vv := normalized.(type) {
			case string:
				normalized = strings.TrimSpace(text)
			case []map[string]any:
				normalized = append([]map[string]any{{"type": "text", "text": strings.TrimSpace(text)}}, vv...)
			}
		}
		return []map[string]any{{
			"role":           "user",
			"content":        normalized,
			"chat_type":      chatType,
			"extra":          map[string]any{},
			"feature_config": buildFeatureConfig(thinkingEnabled),
		}}
	}

	history := make([]string, 0, len(conversation)-1)
	for _, message := range conversation[:len(conversation)-1] {
		history = append(history, fmt.Sprintf("%s:%s", fmt.Sprint(message["role"]), extractText(message["content"])))
	}
	last := conversation[len(conversation)-1]
	lastContent := convertContent(last["content"])
	currentText := extractText(lastContent)
	if systemText != "" {
		history = append([]string{"system:" + systemText}, history...)
	}
	combined := strings.Join(history, ";")
	if combined != "" {
		combined += ";"
	}
	combined += fmt.Sprintf("%s:%s", fmt.Sprint(last["role"]), currentText)

	switch vv := lastContent.(type) {
	case string:
		lastContent = combined
	case []map[string]any:
		lastContent = append([]map[string]any{{"type": "text", "text": combined}}, vv...)
	}

	return []map[string]any{{
		"role":           "user",
		"content":        lastContent,
		"chat_type":      chatType,
		"extra":          map[string]any{},
		"feature_config": buildFeatureConfig(thinkingEnabled),
	}}
}

func cloneMap(input map[string]any) map[string]any {
	output := make(map[string]any, len(input))
	for k, v := range input {
		output[k] = v
	}
	return output
}

func cloneMessageList(messages []map[string]any) []map[string]any {
	cloned := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		cloned = append(cloned, cloneMap(message))
	}
	return cloned
}

func detectMediaURL(item map[string]any) (field string, url string) {
	itemType := fmt.Sprint(item["type"])
	switch itemType {
	case "image":
		return "image", strings.TrimSpace(fmt.Sprint(item["image"]))
	case "video":
		return "video", strings.TrimSpace(fmt.Sprint(item["video"]))
	case "image_url":
		if imageURL, ok := item["image_url"].(map[string]any); ok {
			return "image_url", strings.TrimSpace(fmt.Sprint(imageURL["url"]))
		}
	case "video_url", "input_video":
		if videoURL, ok := item["video_url"].(map[string]any); ok {
			return "video_url", strings.TrimSpace(fmt.Sprint(videoURL["url"]))
		}
	}
	return "", ""
}

func applyMediaURL(item map[string]any, field string, url string) {
	switch field {
	case "image":
		item["image"] = url
	case "video":
		item["video"] = url
	case "image_url":
		imageURL, _ := item["image_url"].(map[string]any)
		if imageURL == nil {
			imageURL = map[string]any{}
		}
		imageURL["url"] = url
		item["image_url"] = imageURL
	case "video_url":
		videoURL, _ := item["video_url"].(map[string]any)
		if videoURL == nil {
			videoURL = map[string]any{}
		}
		videoURL["url"] = url
		item["video_url"] = videoURL
	}
}

func (h *Handler) uploadInlineMedia(ctx context.Context, token string, messages []map[string]any) ([]map[string]any, error) {
	cloned := cloneMessageList(messages)
	for _, message := range cloned {
		switch content := message["content"].(type) {
		case []map[string]any:
			for i := range content {
				field, url := detectMediaURL(content[i])
				if field == "" || !dataURIExpr.MatchString(url) {
					continue
				}
				uploaded, err := h.uploadDataURI(ctx, token, url, field)
				if err != nil {
					return nil, err
				}
				applyMediaURL(content[i], field, uploaded)
			}
			message["content"] = content
		case []any:
			normalized := make([]map[string]any, 0, len(content))
			for _, raw := range content {
				item, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				item = cloneMap(item)
				field, url := detectMediaURL(item)
				if field != "" && dataURIExpr.MatchString(url) {
					uploaded, err := h.uploadDataURI(ctx, token, url, field)
					if err != nil {
						return nil, err
					}
					applyMediaURL(item, field, uploaded)
				}
				normalized = append(normalized, item)
			}
			message["content"] = normalized
		}
	}
	return cloned, nil
}

func (h *Handler) uploadDataURI(ctx context.Context, token string, raw string, field string) (string, error) {
	matches := dataURIExpr.FindStringSubmatch(strings.TrimSpace(raw))
	if len(matches) != 3 {
		return "", errors.New("无效的 data URI")
	}

	contentType := strings.TrimSpace(matches[1])
	content, err := base64.StdEncoding.DecodeString(matches[2])
	if err != nil {
		return "", err
	}

	exts, _ := mime.ExtensionsByType(contentType)
	ext := ".bin"
	if len(exts) > 0 {
		ext = exts[0]
	}
	if ext == "" {
		ext = ".bin"
	}
	prefix := "image"
	if strings.Contains(field, "video") {
		prefix = "video"
	}
	filename := prefix + "_" + fmt.Sprintf("%d", time.Now().UnixNano()) + filepath.Ext("placeholder"+ext)
	if filepath.Ext(filename) == "" {
		filename += ext
	}

	fileURL, _, err := h.qwen.UploadFile(ctx, token, filename, content, contentType)
	if err != nil {
		return "", err
	}
	return fileURL, nil
}

func convertContent(content any) any {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		items := make([]map[string]any, 0)
		for _, raw := range v {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			itemType := fmt.Sprint(item["type"])
			switch itemType {
			case "text":
				items = append(items, map[string]any{"type": "text", "text": fmt.Sprint(item["text"])})
			case "image_url":
				if imageURL, ok := item["image_url"].(map[string]any); ok {
					items = append(items, map[string]any{"type": "image", "image": fmt.Sprint(imageURL["url"])})
				}
			case "input_image", "image":
				items = append(items, map[string]any{"type": "image", "image": fmt.Sprint(item["image"])})
			case "video_url", "input_video", "video":
				urlValue := fmt.Sprint(item["video"])
				if urlValue == "" {
					if videoURL, ok := item["video_url"].(map[string]any); ok {
						urlValue = fmt.Sprint(videoURL["url"])
					}
				}
				items = append(items, map[string]any{"type": "video", "video": urlValue})
			}
		}
		if len(items) == 0 {
			return ""
		}
		return items
	default:
		return ""
	}
}

type chatRequest struct {
	Model          string           `json:"model"`
	Messages       []map[string]any `json:"messages"`
	Stream         bool             `json:"stream"`
	EnableThinking any              `json:"enable_thinking"`
	ThinkingBudget any              `json:"thinking_budget"`
	Tools          any              `json:"tools"`
	ToolChoice     any              `json:"tool_choice"`
	Size           string           `json:"size"`
}

func (h *Handler) HandleModels(w http.ResponseWriter, r *http.Request) {
	session, err := h.accounts.GetAccountSession()
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	models, err := h.qwen.ListModels(r.Context(), session.Token)
	if err != nil {
		h.accounts.RecordFailure(session.Email)
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	h.accounts.ResetFailure(session.Email)

	simple := h.runtime.Snapshot().SimpleModelMap
	result := make([]map[string]any, 0)
	for _, model := range models {
		result = append(result, buildModelVariant(model, ""))
		result = append(result, buildModelVariant(model, "-fast"))
		result = append(result, buildModelVariant(model, "-thinking"))
		if simple {
			continue
		}
		if modelSupports(model.Info, "search") {
			result = append(result, buildModelVariant(model, "-search"))
			result = append(result, buildModelVariant(model, "-fast-search"))
			result = append(result, buildModelVariant(model, "-thinking-search"))
		}
		if modelSupports(model.Info, "t2i") {
			result = append(result, buildModelVariant(model, "-image"))
		}
		if modelSupports(model.Info, "t2v") {
			result = append(result, buildModelVariant(model, "-video"))
		}
		if modelSupports(model.Info, "image_edit") {
			result = append(result, buildModelVariant(model, "-image-edit"))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   result,
	})
}

func buildModelVariant(model qwen.Model, suffix string) map[string]any {
	displayName := model.Name
	if displayName == "" {
		displayName = model.ID
	}
	variantDisplayName := displayName + suffix
	return map[string]any{
		"id":           variantDisplayName,
		"name":         model.ID + suffix,
		"upstream_id":  model.ID,
		"display_name": variantDisplayName,
	}
}

func (h *Handler) HandleChatCompletion(w http.ResponseWriter, r *http.Request) {
	var payload chatRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	if shouldRedirectYuanshen(payload) {
		http.Redirect(w, r, "https://www.yuanshen.com", http.StatusFound)
		return
	}
	executed, status, err := h.executeChatRequest(r.Context(), executedChatRequest{
		Model:          payload.Model,
		Messages:       payload.Messages,
		EnableThinking: payload.EnableThinking,
		Tools:          payload.Tools,
		ToolChoice:     payload.ToolChoice,
		Size:           payload.Size,
	})
	if err != nil {
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	defer executed.Stream.Close()

	if payload.Stream {
		h.handleStream(w, executed.Stream, executed.Model, executed.ToolNames)
		return
	}
	h.handleNonStream(w, executed.Stream, executed.Model, executed.ToolNames)
}

func shouldRedirectYuanshen(payload chatRequest) bool {
	for i := len(payload.Messages) - 1; i >= 0; i-- {
		message := payload.Messages[i]
		if fmt.Sprint(message["role"]) != "user" {
			continue
		}
		return strings.EqualFold(strings.TrimSpace(extractText(message["content"])), "hi")
	}
	return false
}

func (h *Handler) handleStream(w http.ResponseWriter, body io.Reader, model string, toolNames []string) {
	setSSEHeaders(w)
	flusher, _ := w.(http.Flusher)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	messageID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	thinkingStarted := false
	thinkingEnded := false
	promptTokens, completionTokens, totalTokens := 0, 0, 0
	var contentBuilder strings.Builder
	toolCallsSent := false
	streamState := toolcall.NewStreamState()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		h.logger.DebugModule("OPENAI", "stream raw line model=%s line=%q", model, line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		h.logger.DebugModule("OPENAI", "stream raw payload model=%s payload=%q", model, payload)
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			continue
		}
		promptTokens, completionTokens, totalTokens = extractUsage(raw, promptTokens, completionTokens, totalTokens)

		choices, _ := raw["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			continue
		}

		role := fmt.Sprint(delta["role"])
		if role != "" && role != "assistant" {
			continue
		}

		content := extractDeltaContent(delta)
		phase := fmt.Sprint(delta["phase"])
		if content == "" {
			continue
		}
		if (phase == "think" || phase == "thinking_summary") && !thinkingStarted {
			thinkingStarted = true
			content = "<think>\n\n" + content
		}
		if phase == "answer" && thinkingStarted && !thinkingEnded {
			thinkingEnded = true
			content = "\n\n</think>\n" + content
		}
		h.logger.DebugModule("OPENAI", "stream delta model=%s phase=%s content=%q", model, phase, content)
		if len(toolNames) > 0 {
			chunkResult := toolcall.ProcessStreamChunk(streamState, content)
			h.logger.DebugModule("OPENAI", "stream tool sieve model=%s input=%q raw_visible=%q tool_calls=%s", model, content, chunkResult.Content, debugJSON(chunkResult.ToolCalls))
			if len(chunkResult.ToolCalls) > 0 {
				toolCallsSent = true
				h.logger.DebugModule("OPENAI", "stream emit tool calls model=%s tool_calls=%s", model, debugJSON(toolcall.FormatOpenAIToolCalls(chunkResult.ToolCalls)))
				writeSSE(w, map[string]any{
					"id":      messageID,
					"object":  "chat.completion.chunk",
					"created": time.Now().Unix(),
					"model":   model,
					"choices": []map[string]any{{
						"index": 0,
						"delta": map[string]any{
							"tool_calls": toolcall.FormatOpenAIToolCalls(chunkResult.ToolCalls),
						},
						"finish_reason": nil,
					}},
				})
			}
			content = toolcall.CleanVisibleChunk(chunkResult.Content)
			h.logger.DebugModule("OPENAI", "stream cleaned visible model=%s cleaned=%q", model, content)
		}
		contentBuilder.WriteString(content)

		if content != "" {
			h.logger.DebugModule("OPENAI", "stream emit content model=%s content=%q", model, content)
			writeSSE(w, map[string]any{
				"id":      messageID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   model,
				"choices": []map[string]any{{
					"index":         0,
					"delta":         map[string]any{"content": content},
					"finish_reason": nil,
				}},
			})
			if flusher != nil {
				flusher.Flush()
			}
		}
	}

	if len(toolNames) > 0 {
		finalResult := toolcall.FinalizeStream(streamState)
		h.logger.DebugModule("OPENAI", "stream final sieve model=%s raw_visible=%q tool_calls=%s", model, finalResult.Content, debugJSON(finalResult.ToolCalls))
		finalContent := toolcall.CleanVisibleChunk(finalResult.Content)
		h.logger.DebugModule("OPENAI", "stream final cleaned model=%s cleaned=%q", model, finalContent)
		if strings.TrimSpace(finalContent) != "" {
			contentBuilder.WriteString(finalContent)
			h.logger.DebugModule("OPENAI", "stream emit final content model=%s content=%q", model, finalContent)
			writeSSE(w, map[string]any{
				"id":      messageID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   model,
				"choices": []map[string]any{{
					"index":         0,
					"delta":         map[string]any{"content": finalContent},
					"finish_reason": nil,
				}},
			})
			if flusher != nil {
				flusher.Flush()
			}
		}
		if len(finalResult.ToolCalls) > 0 {
			toolCallsSent = true
			h.logger.DebugModule("OPENAI", "stream emit final tool calls model=%s tool_calls=%s", model, debugJSON(toolcall.FormatOpenAIToolCalls(finalResult.ToolCalls)))
			writeSSE(w, map[string]any{
				"id":      messageID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   model,
				"choices": []map[string]any{{
					"index": 0,
					"delta": map[string]any{
						"tool_calls": toolcall.FormatOpenAIToolCalls(finalResult.ToolCalls),
					},
					"finish_reason": nil,
				}},
			})
			if flusher != nil {
				flusher.Flush()
			}
		}
	}

	writeSSE(w, map[string]any{
		"id":      messageID,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index": 0,
			"delta": map[string]any{},
			"finish_reason": func() string {
				if toolCallsSent {
					return "tool_calls"
				}
				return "stop"
			}(),
		}},
	})
	writeSSE(w, map[string]any{
		"id":      messageID,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"choices": []any{},
		"usage": map[string]any{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      totalTokens,
		},
	})
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	h.metrics.RecordUsage(promptTokens, completionTokens, totalTokens)
	h.logger.DebugModule("OPENAI", "stream completed model=%s final_content=%q finish_reason=%s usage=%s", model, contentBuilder.String(), func() string {
		if toolCallsSent {
			return "tool_calls"
		}
		return "stop"
	}(), debugJSON(map[string]any{
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      totalTokens,
	}))
}

func (h *Handler) handleNonStream(w http.ResponseWriter, body io.Reader, model string, toolNames []string) {
	result, upstreamErr, err := h.readCompletedChat(body, model, toolNames)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "读取上游响应失败"})
		return
	}
	if upstreamErr != nil {
		status := upstreamErr.StatusCode
		if status <= 0 {
			status = http.StatusBadGateway
		}
		writeJSON(w, status, map[string]any{"error": upstreamErr.Error()})
		return
	}

	message := map[string]any{
		"role":    "assistant",
		"content": result.Content,
	}
	if len(result.ToolCalls) > 0 {
		if result.Content == "" {
			message["content"] = nil
		}
		message["tool_calls"] = toolcall.FormatOpenAIToolCalls(result.ToolCalls)
	}
	h.logger.DebugModule("OPENAI", "non-stream response model=%s content=%q tool_calls=%s finish_reason=%s usage=%s", model, result.Content, debugJSON(result.ToolCalls), result.FinishReason, debugJSON(map[string]any{
		"prompt_tokens":     result.PromptTokens,
		"completion_tokens": result.CompletionTokens,
		"total_tokens":      result.TotalTokens,
	}))
	h.metrics.RecordUsage(result.PromptTokens, result.CompletionTokens, result.TotalTokens)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       message,
			"finish_reason": result.FinishReason,
		}},
		"usage": map[string]any{
			"prompt_tokens":     result.PromptTokens,
			"completion_tokens": result.CompletionTokens,
			"total_tokens":      result.TotalTokens,
		},
	})
}

func parseChatCompletionContent(rawBody []byte) (string, int, int, int) {
	payloads := make([]map[string]any, 0)
	trimmed := strings.TrimSpace(string(rawBody))

	if trimmed != "" {
		var direct map[string]any
		if json.Unmarshal([]byte(trimmed), &direct) == nil {
			payloads = append(payloads, direct)
		} else {
			events, _ := parseSSEPayloads(trimmed, true)
			for _, event := range events {
				var decoded map[string]any
				if json.Unmarshal([]byte(event), &decoded) == nil {
					payloads = append(payloads, decoded)
				}
			}
		}
	}

	var builder strings.Builder
	thinkingStarted := false
	thinkingEnded := false
	promptTokens, completionTokens, totalTokens := 0, 0, 0

	for _, raw := range payloads {
		promptTokens, completionTokens, totalTokens = extractUsage(raw, promptTokens, completionTokens, totalTokens)
		for _, choice := range collectChatChoices(raw) {
			content := extractChoiceContent(choice)
			if content == "" {
				continue
			}
			phase := extractChoicePhase(choice)
			if (phase == "think" || phase == "thinking_summary") && !thinkingStarted {
				thinkingStarted = true
				content = "<think>\n\n" + content
			}
			if phase == "answer" && thinkingStarted && !thinkingEnded {
				thinkingEnded = true
				content = "\n\n</think>\n" + content
			}
			builder.WriteString(content)
		}
	}

	if strings.TrimSpace(builder.String()) == "" {
		for _, raw := range payloads {
			if fallback := strings.TrimSpace(extractChatTextFromPayload(raw)); fallback != "" {
				builder.WriteString(fallback)
				break
			}
		}
	}

	return strings.TrimSpace(builder.String()), promptTokens, completionTokens, totalTokens
}

func collectChatChoices(payload map[string]any) []map[string]any {
	if payload == nil {
		return nil
	}
	var result []map[string]any
	if choices, ok := payload["choices"].([]any); ok {
		for _, item := range choices {
			if choice, ok := item.(map[string]any); ok {
				result = append(result, choice)
			}
		}
	}
	for _, key := range []string{"data", "message", "response", "result", "output"} {
		if nested, ok := payload[key].(map[string]any); ok {
			result = append(result, collectChatChoices(nested)...)
		}
	}
	return result
}

func extractChoiceContent(choice map[string]any) string {
	if choice == nil {
		return ""
	}
	if message, ok := choice["message"].(map[string]any); ok {
		if content := extractStructuredContent(message["content"]); content != "" {
			return content
		}
		if extra, ok := message["extra"].(map[string]any); ok {
			if summary := extractThinkingSummary(extra); summary != "" {
				return summary
			}
		}
	}
	if delta, ok := choice["delta"].(map[string]any); ok {
		if content := extractStructuredContent(delta["content"]); content != "" {
			return content
		}
		if extra, ok := delta["extra"].(map[string]any); ok {
			if summary := extractThinkingSummary(extra); summary != "" {
				return summary
			}
		}
	}
	return ""
}

func extractChoicePhase(choice map[string]any) string {
	if choice == nil {
		return ""
	}
	if delta, ok := choice["delta"].(map[string]any); ok {
		return strings.TrimSpace(fmt.Sprint(delta["phase"]))
	}
	return ""
}

func extractStructuredContent(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := extractStructuredContent(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]any:
		if text := strings.TrimSpace(fmt.Sprint(v["text"])); text != "" {
			return text
		}
		for _, key := range []string{"content", "value", "output_text", "answer"} {
			if text := extractStructuredContent(v[key]); text != "" {
				return text
			}
		}
	}
	return ""
}

func extractChatTextFromPayload(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return ""
		}
		if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
			var nested any
			if json.Unmarshal([]byte(text), &nested) == nil {
				return extractChatTextFromPayload(nested)
			}
		}
		return text
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if text := extractChatTextFromPayload(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]any:
		for _, key := range []string{
			"content", "text", "message", "delta", "answer", "output_text",
			"result", "output", "response", "data",
		} {
			if text := extractChatTextFromPayload(v[key]); text != "" {
				return text
			}
		}
		for _, item := range v {
			if text := extractChatTextFromPayload(item); text != "" {
				return text
			}
		}
	}
	return ""
}

func extractUsage(raw map[string]any, currentPrompt, currentCompletion, currentTotal int) (int, int, int) {
	usage, _ := raw["usage"].(map[string]any)
	if usage == nil {
		return currentPrompt, currentCompletion, currentTotal
	}
	prompt := int(numberValue(usage["prompt_tokens"]))
	if prompt == 0 {
		prompt = int(numberValue(usage["input_tokens"]))
	}
	completion := int(numberValue(usage["completion_tokens"]))
	if completion == 0 {
		completion = int(numberValue(usage["output_tokens"]))
	}
	total := int(numberValue(usage["total_tokens"]))
	if total == 0 {
		total = prompt + completion
	}
	if prompt > 0 {
		currentPrompt = prompt
	}
	if completion > 0 {
		currentCompletion = completion
	}
	if total > 0 {
		currentTotal = total
	}
	return currentPrompt, currentCompletion, currentTotal
}

func numberValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func setSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

func writeSSE(w http.ResponseWriter, payload any) {
	raw, _ := json.Marshal(payload)
	_, _ = io.WriteString(w, "data: "+string(raw)+"\n\n")
}

func debugJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(raw)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func normalizeSize(size string) string {
	switch size {
	case "1024x1024":
		return "1:1"
	case "1536x1024":
		return "4:3"
	case "1024x1536":
		return "3:4"
	case "1792x1024":
		return "16:9"
	case "1024x1792":
		return "9:16"
	default:
		return size
	}
}

func (h *Handler) HandleImagesGeneration(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Model          string `json:"model"`
		Prompt         string `json:"prompt"`
		Size           string `json:"size"`
		ResponseFormat string `json:"response_format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "请求体格式错误"}})
		return
	}
	if strings.TrimSpace(payload.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "prompt 是必填参数"}})
		return
	}

	url, err := h.generateAsset(r.Context(), payload.Model, "t2i", normalizeSize(payload.Size), []map[string]any{{
		"role":    "user",
		"content": payload.Prompt,
	}})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}

	item := map[string]any{"url": url}
	if payload.ResponseFormat == "b64_json" {
		if encoded, err := h.downloadAsBase64(r.Context(), url); err == nil {
			item = map[string]any{"b64_json": encoded}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"created": time.Now().Unix(),
		"data":    []map[string]any{item},
	})
}

func (h *Handler) HandleImagesEdit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "无法解析 multipart 请求"}})
		return
	}
	prompt := r.FormValue("prompt")
	if strings.TrimSpace(prompt) == "" {
		prompt = "请基于上传图片完成编辑"
	}
	model := r.FormValue("model")
	size := normalizeSize(r.FormValue("size"))
	responseFormat := r.FormValue("response_format")

	parts := []map[string]any{{"type": "text", "text": prompt}}
	files := collectMultipartFiles(r.MultipartForm)
	if len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "image 是必填参数"}})
		return
	}
	for _, file := range files {
		parts = append(parts, map[string]any{"type": "image", "image": file})
	}

	url, err := h.generateAsset(r.Context(), model, "image_edit", size, []map[string]any{{
		"role":    "user",
		"content": parts,
	}})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}
	item := map[string]any{"url": url}
	if responseFormat == "b64_json" {
		if encoded, err := h.downloadAsBase64(r.Context(), url); err == nil {
			item = map[string]any{"b64_json": encoded}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"created": time.Now().Unix(),
		"data":    []map[string]any{item},
	})
}

func collectMultipartFiles(form *multipart.Form) []string {
	result := make([]string, 0)
	if form == nil {
		return result
	}
	appendFiles := func(key string) {
		for _, header := range form.File[key] {
			file, err := header.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(file)
			_ = file.Close()
			if err != nil {
				continue
			}
			mimeType := header.Header.Get("Content-Type")
			if mimeType == "" {
				mimeType = "image/png"
			}
			result = append(result, "data:"+mimeType+";base64,"+base64.StdEncoding.EncodeToString(data))
		}
	}
	appendFiles("image")
	appendFiles("images")
	return result
}

func (h *Handler) HandleVideos(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Size   string `json:"size"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "请求体格式错误"}})
		return
	}
	if strings.TrimSpace(payload.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": "prompt 是必填参数"}})
		return
	}

	url, err := h.generateAsset(r.Context(), payload.Model, "t2v", normalizeSize(payload.Size), []map[string]any{{
		"role":    "user",
		"content": payload.Prompt,
	}})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": err.Error()}})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":      fmt.Sprintf("video_%d", time.Now().UnixNano()),
		"object":  "video",
		"created": time.Now().Unix(),
		"status":  "completed",
		"data":    []map[string]any{{"url": url}},
	})
}

func (h *Handler) generateAsset(ctx context.Context, requestedModel, chatType, size string, messages []map[string]any) (string, error) {
	model, _ := h.ResolveModel(ctx, requestedModel, chatType)
	session, err := h.accounts.GetAccountSession()
	if err != nil {
		return "", err
	}
	normalizedMessages := normalizeMessages(messages, chatType, false)
	normalizedMessages, err = h.uploadInlineMedia(ctx, session.Token, normalizedMessages)
	if err != nil {
		return "", err
	}
	chatID, err := h.qwen.NewChat(ctx, session.Token, model)
	if err != nil {
		return "", err
	}

	body := map[string]any{
		"stream":             true,
		"incremental_output": true,
		"chat_id":            chatID,
		"chat_type":          chatType,
		"model":              model,
		"messages":           normalizedMessages,
		"session_id":         fmt.Sprintf("%d", time.Now().UnixNano()),
		"id":                 fmt.Sprintf("%d", time.Now().UnixNano()),
		"sub_chat_type":      chatType,
		"chat_mode":          "normal",
	}
	if size != "" {
		body["size"] = size
	}
	resp, err := h.qwen.ChatCompletions(ctx, session.Token, chatID, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	result, err := readAssetResult(resp.Body)
	if err != nil {
		return "", err
	}
	if result.UpstreamError != nil && result.UpstreamError.Retryable {
		time.Sleep(800 * time.Millisecond)
		resp, err = h.qwen.ChatCompletions(ctx, session.Token, chatID, body)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		result, err = readAssetResult(resp.Body)
		if err != nil {
			return "", err
		}
	}
	if result.UpstreamError != nil {
		return "", result.UpstreamError
	}

	if result.ContentURL == "" && chatID != "" {
		if fallbackURL := h.resolveAssetFromChatDetail(ctx, session.Token, chatID, result.ResponseIDs); fallbackURL != "" {
			result.ContentURL = fallbackURL
			result.ChatDetailFallback = true
		}
		if chatType == "t2v" && len(result.TaskCandidates) == 0 {
			result.TaskCandidates = h.resolveVideoTasksFromChatDetail(ctx, session.Token, chatID, result.ResponseIDs)
			if result.TaskID == "" && len(result.TaskCandidates) > 0 {
				result.TaskID = result.TaskCandidates[0]
			}
		}
	}

	if result.ContentURL != "" {
		return result.ContentURL, nil
	}
	if chatType == "t2v" && len(result.TaskCandidates) > 0 {
		return h.pollVideo(ctx, session.Token, result.TaskCandidates)
	}
	return "", errors.New("未能从上游响应中解析资源链接")
}

func (h *Handler) pollVideo(ctx context.Context, token string, taskCandidates []string) (string, error) {
	deadline := time.Now().Add(2 * time.Minute)
	for _, taskID := range uniqueStrings(taskCandidates) {
		for time.Now().Before(deadline) {
			payload, err := h.qwen.GetVideoTaskStatus(ctx, token, taskID)
			if err == nil {
				if status := strings.ToLower(fmt.Sprint(payload["task_status"])); status == "failed" {
					break
				}
				if url := extractResourceURLFromPayload(payload); url != "" {
					return url, nil
				}
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
	}
	return "", errors.New("视频生成超时，请稍后再试")
}

func (h *Handler) resolveAssetFromChatDetail(ctx context.Context, token, chatID string, responseIDs []string) string {
	for attempt := 0; attempt < 5; attempt++ {
		detail, err := h.qwen.GetChatDetail(ctx, token, chatID)
		if err == nil {
			if url := extractAssetFromChatDetail(detail, responseIDs); url != "" {
				return url
			}
		}
		select {
		case <-ctx.Done():
			return ""
		case <-time.After(800 * time.Millisecond):
		}
	}
	return ""
}

func (h *Handler) resolveVideoTasksFromChatDetail(ctx context.Context, token, chatID string, responseIDs []string) []string {
	for attempt := 0; attempt < 5; attempt++ {
		detail, err := h.qwen.GetChatDetail(ctx, token, chatID)
		if err == nil {
			tasks := extractVideoTasksFromChatDetail(detail, responseIDs)
			if len(tasks) > 0 {
				return tasks
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(1200 * time.Millisecond):
		}
	}
	return nil
}

func (h *Handler) downloadAsBase64(ctx context.Context, assetURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(body), nil
}
