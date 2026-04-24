package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"qwen2api/internal/toolcall"
)

var anthropicVersionPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

type anthropicRequest struct {
	Model         string             `json:"model"`
	Messages      []anthropicMessage `json:"messages"`
	System        json.RawMessage    `json:"system"`
	MaxTokens     int                `json:"max_tokens"`
	Stream        bool               `json:"stream"`
	Tools         []anthropicTool    `json:"tools"`
	ToolChoice    json.RawMessage    `json:"tool_choice"`
	Metadata      map[string]any     `json:"metadata"`
	StopSequences []string           `json:"stop_sequences"`
	Temperature   *float64           `json:"temperature"`
	TopP          *float64           `json:"top_p"`
	TopK          *int               `json:"top_k"`
}

type anthropicCountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicContentBlock struct {
	Type      string                `json:"type"`
	Text      string                `json:"text,omitempty"`
	Source    *anthropicImageSource `json:"source,omitempty"`
	Name      string                `json:"name,omitempty"`
	ID        string                `json:"id,omitempty"`
	Input     map[string]any        `json:"input,omitempty"`
	Content   json.RawMessage       `json:"content,omitempty"`
	ToolUseID string                `json:"tool_use_id,omitempty"`
	IsError   bool                  `json:"is_error,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
	URL       string `json:"url"`
}

type anthropicResponseMessage struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Role         string           `json:"role"`
	Model        string           `json:"model"`
	Content      []map[string]any `json:"content"`
	StopReason   string           `json:"stop_reason"`
	StopSequence any              `json:"stop_sequence"`
	Usage        anthropicUsage   `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicStreamState struct {
	messageID      string
	messageStarted bool
	activeBlock    *anthropicActiveBlock
	nextIndex      int
}

type anthropicActiveBlock struct {
	Index  int
	Kind   string
	ToolID string
}

func (h *Handler) HandleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	if err := validateAnthropicVersion(r); err != nil {
		writeAnthropicStatusError(w, http.StatusBadRequest, err.Error())
		return
	}

	var payload anthropicRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "请求体格式错误")
		return
	}

	executedRequest, err := convertAnthropicRequest(payload)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	executed, status, err := h.executeChatRequest(r.Context(), executedRequest)
	if err != nil {
		writeAnthropicStatusError(w, status, err.Error())
		return
	}
	defer executed.Stream.Close()

	if payload.Stream {
		h.handleAnthropicStream(w, executed.Stream, executed.Model, executed.ToolNames)
		return
	}
	h.handleAnthropicNonStream(w, executed.Stream, executed.Model, executed.ToolNames)
}

func (h *Handler) HandleAnthropicCountTokens(w http.ResponseWriter, r *http.Request) {
	if err := validateAnthropicVersion(r); err != nil {
		writeAnthropicStatusError(w, http.StatusBadRequest, err.Error())
		return
	}

	var payload anthropicRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "请求体格式错误")
		return
	}

	tokens, err := estimateAnthropicInputTokens(payload)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, anthropicCountTokensResponse{InputTokens: tokens})
}

func validateAnthropicVersion(r *http.Request) error {
	if r == nil {
		return nil
	}
	version := strings.TrimSpace(r.Header.Get("anthropic-version"))
	if version == "" {
		return nil
	}
	if anthropicVersionPattern.MatchString(version) {
		return nil
	}
	return fmt.Errorf("anthropic-version 格式不支持")
}

func convertAnthropicRequest(payload anthropicRequest) (executedChatRequest, error) {
	messages := make([]map[string]any, 0, len(payload.Messages)+1)

	systemText, err := normalizeAnthropicSystem(payload.System)
	if err != nil {
		return executedChatRequest{}, err
	}
	if systemText != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": systemText,
		})
	}

	for _, message := range payload.Messages {
		normalized, err := normalizeAnthropicMessage(message)
		if err != nil {
			return executedChatRequest{}, err
		}
		messages = append(messages, normalized...)
	}

	return executedChatRequest{
		Model:      payload.Model,
		Messages:   messages,
		Tools:      convertAnthropicTools(payload.Tools),
		ToolChoice: convertAnthropicToolChoice(payload.ToolChoice),
	}, nil
}

func normalizeAnthropicSystem(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text), nil
	}

	blocks, err := normalizeAnthropicContent(raw)
	if err != nil {
		return "", fmt.Errorf("system 字段格式不支持")
	}

	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, strings.TrimSpace(block.Text))
		}
	}
	return strings.Join(parts, "\n"), nil
}

func normalizeAnthropicMessage(message anthropicMessage) ([]map[string]any, error) {
	role := strings.TrimSpace(message.Role)
	if role == "" {
		return nil, fmt.Errorf("messages.role 是必填参数")
	}

	content, err := normalizeAnthropicContent(message.Content)
	if err != nil {
		return nil, err
	}

	result := make([]map[string]any, 0, len(content)+1)
	contentParts := make([]map[string]any, 0)
	for _, block := range content {
		switch block.Type {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				contentParts = append(contentParts, map[string]any{
					"type": "text",
					"text": block.Text,
				})
			}
		case "image", "image_url":
			imageURL := normalizeAnthropicImage(block)
			if imageURL != "" {
				contentParts = append(contentParts, map[string]any{
					"type":      "image_url",
					"image_url": map[string]any{"url": imageURL},
				})
			}
		case "tool_result":
			result = append(result, normalizeAnthropicToolResultMessage(block))
		}
	}

	if len(contentParts) > 0 || len(result) == 0 {
		result = append([]map[string]any{{
			"role":    role,
			"content": contentParts,
		}}, result...)
	}
	return result, nil
}

func normalizeAnthropicContent(raw json.RawMessage) ([]anthropicContentBlock, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []anthropicContentBlock{{Type: "text", Text: text}}, nil
	}

	var blocks []anthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("messages.content 格式不支持")
	}
	return blocks, nil
}

func normalizeAnthropicImage(block anthropicContentBlock) string {
	if block.Source == nil {
		return ""
	}
	if strings.EqualFold(block.Source.Type, "base64") && block.Source.MediaType != "" && block.Source.Data != "" {
		return "data:" + block.Source.MediaType + ";base64," + block.Source.Data
	}
	if strings.TrimSpace(block.Source.URL) != "" {
		return strings.TrimSpace(block.Source.URL)
	}
	return ""
}

func normalizeAnthropicToolResultMessage(block anthropicContentBlock) map[string]any {
	content := extractAnthropicToolResultText(block)
	if block.IsError && strings.TrimSpace(content) != "" {
		content = "ERROR: " + content
	}
	return map[string]any{
		"role":         "tool",
		"name":         "tool",
		"tool_call_id": strings.TrimSpace(block.ToolUseID),
		"content":      content,
	}
}

func extractAnthropicToolResultText(block anthropicContentBlock) string {
	content := strings.TrimSpace(string(block.Content))
	if content == "" {
		return ""
	}

	var text string
	if err := json.Unmarshal(block.Content, &text); err == nil {
		return strings.TrimSpace(text)
	}

	var nestedBlocks []anthropicContentBlock
	if err := json.Unmarshal(block.Content, &nestedBlocks); err == nil {
		parts := make([]string, 0, len(nestedBlocks))
		for _, nested := range nestedBlocks {
			if nested.Type == "text" && strings.TrimSpace(nested.Text) != "" {
				parts = append(parts, strings.TrimSpace(nested.Text))
			}
		}
		return strings.Join(parts, "\n")
	}

	return content
}

func convertAnthropicTools(tools []anthropicTool) any {
	if len(tools) == 0 {
		return nil
	}
	result := make([]any, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			continue
		}
		result = append(result, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.InputSchema,
			},
		})
	}
	return result
}

func convertAnthropicToolChoice(raw json.RawMessage) any {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}

	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct
	}

	var payload struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(payload.Type)) {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "tool":
		if strings.TrimSpace(payload.Name) != "" {
			return map[string]any{
				"type": "function",
				"function": map[string]any{
					"name": payload.Name,
				},
			}
		}
	}
	return nil
}

func (h *Handler) handleAnthropicNonStream(w http.ResponseWriter, body io.Reader, model string, toolNames []string) {
	result, upstreamErr, err := h.readCompletedChat(body, model, toolNames)
	if err != nil {
		writeAnthropicStatusError(w, http.StatusBadGateway, "读取上游响应失败")
		return
	}
	if upstreamErr != nil {
		status := upstreamErr.StatusCode
		if status <= 0 {
			status = http.StatusBadGateway
		}
		writeAnthropicStatusError(w, status, upstreamErr.Error())
		return
	}

	messageID := anthropicMessageID()
	h.metrics.RecordUsage(result.PromptTokens, result.CompletionTokens, result.TotalTokens)
	response := anthropicResponseMessage{
		ID:           messageID,
		Type:         "message",
		Role:         "assistant",
		Model:        model,
		Content:      anthropicContentFromResult(messageID, result),
		StopReason:   anthropicStopReason(result.FinishReason),
		StopSequence: nil,
		Usage: anthropicUsage{
			InputTokens:  result.PromptTokens,
			OutputTokens: result.CompletionTokens,
		},
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) handleAnthropicStream(w http.ResponseWriter, body io.Reader, model string, toolNames []string) {
	setSSEHeaders(w)
	flusher, _ := w.(http.Flusher)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	promptTokens, completionTokens, totalTokens := 0, 0, 0
	toolCallsSent := false
	streamState := toolcall.NewStreamState()
	anthropicState := anthropicStreamState{messageID: anthropicMessageID()}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}

		var raw map[string]any
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			continue
		}
		promptTokens, completionTokens, totalTokens = extractUsage(raw, promptTokens, completionTokens, totalTokens)
		ensureAnthropicMessageStart(w, &anthropicState, model, promptTokens)

		choices, _ := raw["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		choice, _ := choices[0].(map[string]any)
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			continue
		}

		content := fmt.Sprint(delta["content"])
		if content == "" {
			continue
		}

		chunkResult := toolcall.ProcessStreamChunk(streamState, content)
		if chunkResult.Content != "" && !shouldSkipAnthropicTextChunk(chunkResult.Content) {
			emitAnthropicTextDelta(w, &anthropicState, chunkResult.Content)
		}
		if len(chunkResult.ToolCalls) > 0 {
			toolCallsSent = true
			closeAnthropicActiveBlock(w, &anthropicState)
			for _, call := range chunkResult.ToolCalls {
				emitAnthropicToolUse(w, &anthropicState, call)
			}
		}

		if flusher != nil {
			flusher.Flush()
		}
	}

	finalResult := toolcall.FinalizeStream(streamState)
	if finalResult.Content != "" && !shouldSkipAnthropicTextChunk(finalResult.Content) {
		ensureAnthropicMessageStart(w, &anthropicState, model, promptTokens)
		emitAnthropicTextDelta(w, &anthropicState, finalResult.Content)
	}
	if len(finalResult.ToolCalls) > 0 {
		ensureAnthropicMessageStart(w, &anthropicState, model, promptTokens)
		toolCallsSent = true
		closeAnthropicActiveBlock(w, &anthropicState)
		for _, call := range finalResult.ToolCalls {
			emitAnthropicToolUse(w, &anthropicState, call)
		}
	}

	ensureAnthropicMessageStart(w, &anthropicState, model, promptTokens)
	closeAnthropicActiveBlock(w, &anthropicState)
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}
	writeAnthropicSSE(w, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   anthropicStopReasonFromToolCalls(toolCallsSent),
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"output_tokens": completionTokens,
		},
	})
	writeAnthropicSSE(w, "message_stop", map[string]any{
		"type": "message_stop",
	})
	h.metrics.RecordUsage(promptTokens, completionTokens, totalTokens)
}

func ensureAnthropicMessageStart(w io.Writer, state *anthropicStreamState, model string, promptTokens int) {
	if state.messageStarted {
		return
	}
	state.messageStarted = true
	writeAnthropicSSE(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            state.messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":  promptTokens,
				"output_tokens": 0,
			},
		},
	})
}

func emitAnthropicTextDelta(w io.Writer, state *anthropicStreamState, text string) {
	if shouldSkipAnthropicTextChunk(text) {
		return
	}
	if strings.TrimSpace(text) == "" && text == "" {
		return
	}
	if state.activeBlock == nil || state.activeBlock.Kind != "text" {
		closeAnthropicActiveBlock(w, state)
		state.activeBlock = &anthropicActiveBlock{
			Index: state.nextIndex,
			Kind:  "text",
		}
		state.nextIndex++
		writeAnthropicSSE(w, "content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": state.activeBlock.Index,
			"content_block": map[string]any{
				"type": "text",
				"text": "",
			},
		})
	}
	writeAnthropicSSE(w, "content_block_delta", map[string]any{
		"type":  "content_block_delta",
		"index": state.activeBlock.Index,
		"delta": map[string]any{
			"type": "text_delta",
			"text": text,
		},
	})
}

func emitAnthropicToolUse(w io.Writer, state *anthropicStreamState, call toolcall.ToolCall) {
	closeAnthropicActiveBlock(w, state)
	toolID := anthropicToolUseID(state.messageID, state.nextIndex)
	index := state.nextIndex
	state.nextIndex++
	writeAnthropicSSE(w, "content_block_start", map[string]any{
		"type":  "content_block_start",
		"index": index,
		"content_block": map[string]any{
			"type":  "tool_use",
			"id":    toolID,
			"name":  call.Name,
			"input": map[string]any{},
		},
	})
	rawInput, _ := json.Marshal(call.Input)
	writeAnthropicSSE(w, "content_block_delta", map[string]any{
		"type":  "content_block_delta",
		"index": index,
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": string(rawInput),
		},
	})
	writeAnthropicSSE(w, "content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": index,
	})
}

func closeAnthropicActiveBlock(w io.Writer, state *anthropicStreamState) {
	if state.activeBlock == nil {
		return
	}
	writeAnthropicSSE(w, "content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": state.activeBlock.Index,
	})
	state.activeBlock = nil
}

func anthropicContentFromResult(messageID string, result completedChat) []map[string]any {
	content := make([]map[string]any, 0, 1+len(result.ToolCalls))
	if result.Content != "" || len(result.ToolCalls) == 0 {
		content = append(content, map[string]any{
			"type": "text",
			"text": result.Content,
		})
	}
	for i, call := range result.ToolCalls {
		content = append(content, map[string]any{
			"type":  "tool_use",
			"id":    anthropicToolUseID(messageID, i),
			"name":  call.Name,
			"input": call.Input,
		})
	}
	return content
}

func anthropicStopReason(finishReason string) string {
	if finishReason == "tool_calls" {
		return "tool_use"
	}
	return "end_turn"
}

func anthropicStopReasonFromToolCalls(toolCallsSent bool) string {
	if toolCallsSent {
		return "tool_use"
	}
	return "end_turn"
}

func writeAnthropicError(w http.ResponseWriter, status int, errorType string, message string) {
	writeJSON(w, status, map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    errorType,
			"message": message,
		},
	})
}

func writeAnthropicStatusError(w http.ResponseWriter, status int, message string) {
	writeAnthropicError(w, status, anthropicErrorType(status, message), message)
}

func anthropicErrorType(status int, message string) string {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusServiceUnavailable:
		return "overloaded_error"
	}
	if strings.Contains(lower, "overload") || strings.Contains(lower, "temporarily unavailable") {
		return "overloaded_error"
	}
	return "api_error"
}

func writeAnthropicSSE(w io.Writer, event string, payload any) {
	raw, _ := json.Marshal(payload)
	_, _ = io.WriteString(w, "event: "+event+"\n")
	_, _ = io.WriteString(w, "data: "+string(raw)+"\n\n")
}

func anthropicMessageID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

func anthropicToolUseID(messageID string, index int) string {
	return fmt.Sprintf("toolu_%s_%d", strings.TrimPrefix(messageID, "msg_"), index)
}

func estimateAnthropicInputTokens(payload anthropicRequest) (int, error) {
	total := 0

	systemText, err := normalizeAnthropicSystem(payload.System)
	if err != nil {
		return 0, err
	}
	total += estimateTextTokens(systemText)

	for _, tool := range payload.Tools {
		total += estimateTextTokens(tool.Name)
		total += estimateTextTokens(tool.Description)
		if len(tool.InputSchema) > 0 {
			raw, _ := json.Marshal(tool.InputSchema)
			total += estimateTextTokens(string(raw))
		}
	}

	for _, message := range payload.Messages {
		total += estimateTextTokens(message.Role)
		blocks, err := normalizeAnthropicContent(message.Content)
		if err != nil {
			return 0, err
		}
		for _, block := range blocks {
			switch block.Type {
			case "text":
				total += estimateTextTokens(block.Text)
			case "tool_result":
				total += estimateTextTokens(block.ToolUseID)
				total += estimateTextTokens(extractAnthropicToolResultText(block))
				if block.IsError {
					total += 2
				}
			case "image", "image_url":
				total += 256
				total += estimateTextTokens(normalizeAnthropicImage(block))
			case "tool_use":
				total += estimateTextTokens(block.Name)
				if len(block.Input) > 0 {
					raw, _ := json.Marshal(block.Input)
					total += estimateTextTokens(string(raw))
				}
			default:
				total += estimateTextTokens(block.Text)
			}
		}
	}

	if total <= 0 {
		total = 1
	}
	return total, nil
}

func estimateTextTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	runes := utf8.RuneCountInString(text)
	return int(math.Ceil(float64(runes) / 4.0))
}

func shouldSkipAnthropicTextChunk(text string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.HasPrefix(trimmed, "<tool_calls"),
		strings.HasPrefix(trimmed, "</tool_calls"),
		strings.HasPrefix(trimmed, "<ml_tool_calls"),
		strings.HasPrefix(trimmed, "</ml_tool_calls"),
		strings.HasPrefix(trimmed, "<tool_call"),
		strings.HasPrefix(trimmed, "</tool_call"),
		strings.HasPrefix(trimmed, "<ml_tool_call"),
		strings.HasPrefix(trimmed, "</ml_tool_call"):
		return true
	default:
		return false
	}
}
