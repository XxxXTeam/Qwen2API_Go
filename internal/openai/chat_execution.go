package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"qwen2api/internal/qwen"
	"qwen2api/internal/toolcall"
)

type executedChatRequest struct {
	Model          string
	Messages       []map[string]any
	EnableThinking any
	Tools          any
	ToolChoice     any
	Size           string
}

type executedChat struct {
	Model     string
	ToolNames []string
	Stream    io.ReadCloser
}

type completedChat struct {
	Content          string
	ToolCalls        []toolcall.ToolCall
	FinishReason     string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func (h *Handler) executeChatRequest(ctx context.Context, payload executedChatRequest) (*executedChat, int, error) {
	chatType := chatTypeForModel(payload.Model)
	model, _ := h.ResolveModel(ctx, payload.Model, chatType)
	thinkingEnabled := isThinkingEnabled(payload.Model, payload.EnableThinking)
	injection := toolcall.InjectPrompt(payload.Messages, payload.Tools, payload.ToolChoice)
	upstreamMessages := normalizeMessages(injection.Messages, chatType, thinkingEnabled)

	session, err := h.accounts.GetAccountSession()
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	upstreamMessages, err = h.uploadInlineMedia(ctx, session.Token, upstreamMessages)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}

	chatID, err := h.qwen.NewChat(ctx, session.Token, model)
	if err != nil {
		h.accounts.RecordFailure(session.Email)
		return nil, http.StatusBadGateway, err
	}

	body := map[string]any{
		"stream":             true,
		"incremental_output": true,
		"chat_id":            chatID,
		"chat_type":          chatType,
		"model":              model,
		"messages":           upstreamMessages,
		"session_id":         fmt.Sprintf("%d", time.Now().UnixNano()),
		"id":                 fmt.Sprintf("%d", time.Now().UnixNano()),
		"sub_chat_type":      chatType,
		"chat_mode":          "normal",
	}
	if payload.Size != "" {
		body["size"] = payload.Size
	}

	resp, err := h.qwen.ChatCompletions(ctx, session.Token, chatID, body)
	if err != nil {
		h.accounts.RecordFailure(session.Email)
		return nil, http.StatusBadGateway, err
	}
	inspected, err := qwen.InspectUpstreamStream(ctx, resp.Body)
	if err != nil {
		h.accounts.RecordFailure(session.Email)
		return nil, http.StatusBadGateway, err
	}
	if inspected.UpstreamError != nil {
		h.accounts.RecordFailure(session.Email)
		status := inspected.UpstreamError.StatusCode
		if status <= 0 {
			status = http.StatusBadGateway
		}
		return nil, status, inspected.UpstreamError
	}

	h.accounts.ResetFailure(session.Email)
	return &executedChat{
		Model:     model,
		ToolNames: injection.ToolNames,
		Stream:    inspected.Stream,
	}, http.StatusOK, nil
}

func (h *Handler) readCompletedChat(body io.Reader, model string, toolNames []string) (completedChat, *qwen.UpstreamError, error) {
	rawBody, err := io.ReadAll(body)
	if err != nil {
		return completedChat{}, nil, err
	}
	h.logger.DebugModule("OPENAI", "non-stream upstream raw response model=%s body=%s", model, string(rawBody))
	if upstreamErr := parseAssetError(rawBody); upstreamErr != nil {
		return completedChat{}, upstreamErr, nil
	}

	fullContent, promptTokens, completionTokens, totalTokens := parseChatCompletionContent(rawBody)
	parsedCalls := []toolcall.ToolCall(nil)
	normalizedContent := fullContent
	if len(toolNames) > 0 {
		parsedCalls = toolcall.ParseCalls(fullContent)
		if len(parsedCalls) > 0 {
			normalizedContent = cleanupToolMarkup(toolcall.RemoveMarkup(fullContent))
		}
	}

	finishReason := "stop"
	if len(parsedCalls) > 0 {
		finishReason = "tool_calls"
	}

	return completedChat{
		Content:          strings.TrimSpace(normalizedContent),
		ToolCalls:        parsedCalls,
		FinishReason:     finishReason,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}, nil, nil
}

func cleanupToolMarkup(text string) string {
	replacer := strings.NewReplacer(
		"</tool_calls>", "",
		"<tool_calls>", "",
		"</ml_tool_calls>", "",
		"<ml_tool_calls>", "",
		"</tool_call>", "",
		"<tool_call>", "",
		"</ml_tool_call>", "",
		"<ml_tool_call>", "",
	)
	return strings.TrimSpace(replacer.Replace(text))
}
