package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"qwen2api/internal/qwen"
	"qwen2api/internal/storage"
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
	Model          string
	RequestedModel string
	ToolNames      []string
	Stream         io.ReadCloser
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
	prepared := h.prepareChatRequest(ctx, payload)
	maxAttempts := len(h.accounts.Accounts())
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	attempted := map[string]struct{}{}

	if prepared.ContextHash != "" {
		if mapped, ok := h.sessions.Get(prepared.ContextHash); ok && mapped.Model == prepared.Model && mapped.ChatType == prepared.ChatType {
			if session, err := h.accounts.GetAccountSessionByEmail(mapped.AccountEmail); err == nil {
				h.logger.DebugModule("OPENAI", "reuse mapped chat model=%s hash=%s account=%s chat_id=%s", prepared.Model, prepared.ContextHash, mapped.AccountEmail, mapped.ChatID)
				executed, status, err := h.sendChatWithSession(ctx, prepared, session, mapped.ChatID, true)
				if err == nil {
					h.sessions.Save(prepared.ContextHash, session.Email, mapped.ChatID, prepared.Model, prepared.ChatType)
					return executed, status, nil
				}
				if upstreamErr, ok := err.(*qwen.UpstreamError); ok {
					if shouldInvalidateConversationMapping(upstreamErr) {
						h.sessions.Delete(prepared.ContextHash)
						h.logger.WarnModule("OPENAI", "invalidate mapped chat model=%s hash=%s account=%s chat_id=%s err=%v", prepared.Model, prepared.ContextHash, session.Email, mapped.ChatID, upstreamErr)
					} else if upstreamErr.Retryable {
						h.accounts.RecordFailure(session.Email)
						attempted[session.Email] = struct{}{}
						h.logger.WarnModule("OPENAI", "mapped chat retryable error model=%s hash=%s account=%s chat_id=%s status=%d err=%v", prepared.Model, prepared.ContextHash, session.Email, mapped.ChatID, upstreamErr.StatusCode, upstreamErr)
					} else {
						return nil, normalizeUpstreamStatus(upstreamErr.StatusCode), upstreamErr
					}
				} else {
					h.accounts.RecordFailure(session.Email)
					attempted[session.Email] = struct{}{}
				}
			}
		}
	}

	var lastErr error
	lastStatus := http.StatusBadGateway
	for len(attempted) < maxAttempts {
		session, err := h.accounts.GetAccountSessionExcluding(attempted)
		if err != nil {
			break
		}
		attempted[session.Email] = struct{}{}

		executed, status, err := h.sendChatWithSession(ctx, prepared, session, "", false)
		if err == nil {
			if prepared.ContextHash != "" {
				if chatID := chatIDFromStream(executed.Stream); chatID != "" {
					h.sessions.Save(prepared.ContextHash, session.Email, chatID, prepared.Model, prepared.ChatType)
				}
			}
			return executed, status, nil
		}

		lastErr = err
		lastStatus = status
		if upstreamErr, ok := err.(*qwen.UpstreamError); ok {
			h.accounts.RecordFailure(session.Email)
			if shouldInvalidateConversationMapping(upstreamErr) && prepared.ContextHash != "" {
				h.sessions.Delete(prepared.ContextHash)
			}
			if upstreamErr.Retryable {
				continue
			}
		}
		return nil, status, err
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("上游聊天请求失败")
	}
	return nil, lastStatus, lastErr
}

func (h *Handler) prepareChatRequest(ctx context.Context, payload executedChatRequest) preparedChatRequest {
	chatType := chatTypeForModel(payload.Model)
	model, _ := h.ResolveModel(ctx, payload.Model, chatType)
	thinkingEnabled := isThinkingEnabled(payload.Model, payload.EnableThinking)
	injection := toolcall.InjectPrompt(payload.Messages, payload.Tools, payload.ToolChoice)
	expandedMessages := cloneMessageList(injection.Messages)
	fullUpstreamMessages := normalizeMessages(cloneMessageList(expandedMessages), chatType, thinkingEnabled)

	lastUpstreamMessages := fullUpstreamMessages
	if len(payload.Messages) > 0 && len(expandedMessages) > 1 {
		lastRaw := cloneMessageList(payload.Messages[len(payload.Messages)-1:])
		lastExpanded := toolcall.NormalizeToolMessagesForExecution(lastRaw)
		lastUpstreamMessages = normalizeMessages(lastExpanded, chatType, thinkingEnabled)
	}

	return preparedChatRequest{
		RequestedModel:       strings.TrimSpace(payload.Model),
		Model:                model,
		ChatType:             chatType,
		ThinkingEnabled:      thinkingEnabled,
		ExpandedMessages:     expandedMessages,
		FullUpstreamMessages: fullUpstreamMessages,
		LastUpstreamMessages: lastUpstreamMessages,
		ContextHash:          computeContextHash(model, chatType, injection.ToolNames, expandedMessages),
		ToolNames:            injection.ToolNames,
	}
}

func (h *Handler) sendChatWithSession(ctx context.Context, prepared preparedChatRequest, session storage.Account, existingChatID string, incremental bool) (*executedChat, int, error) {
	chatID := strings.TrimSpace(existingChatID)
	if chatID == "" {
		var err error
		chatID, err = h.qwen.NewChat(ctx, session.Token, prepared.Model)
		if err != nil {
			return nil, http.StatusBadGateway, err
		}
	}

	baseMessages := prepared.FullUpstreamMessages
	if incremental && len(prepared.LastUpstreamMessages) > 0 {
		baseMessages = prepared.LastUpstreamMessages
	}

	upstreamMessages, err := h.uploadInlineMedia(ctx, session.Token, cloneMessageList(baseMessages))
	if err != nil {
		return nil, http.StatusBadGateway, err
	}

	body := map[string]any{
		"stream":             true,
		"incremental_output": true,
		"chat_id":            chatID,
		"chat_type":          prepared.ChatType,
		"model":              prepared.Model,
		"messages":           upstreamMessages,
		"session_id":         fmt.Sprintf("%d", time.Now().UnixNano()),
		"id":                 fmt.Sprintf("%d", time.Now().UnixNano()),
		"sub_chat_type":      prepared.ChatType,
		"chat_mode":          "normal",
	}

	resp, err := h.qwen.ChatCompletions(ctx, session.Token, chatID, body)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	inspected, err := qwen.InspectUpstreamStream(ctx, resp.Body)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	if inspected.UpstreamError != nil {
		return nil, normalizeUpstreamStatus(inspected.UpstreamError.StatusCode), inspected.UpstreamError
	}
	h.accounts.ResetFailure(session.Email)
	stream := withChatID(inspected.Stream, chatID)
	return &executedChat{
		Model:          prepared.Model,
		RequestedModel: prepared.RequestedModel,
		ToolNames:      prepared.ToolNames,
		Stream:         stream,
	}, http.StatusOK, nil
}

func normalizeUpstreamStatus(status int) int {
	if status <= 0 {
		return http.StatusBadGateway
	}
	return status
}

func shouldInvalidateConversationMapping(err *qwen.UpstreamError) bool {
	if err == nil {
		return false
	}
	haystack := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(haystack, "chat_id") ||
		strings.Contains(haystack, "not found") ||
		strings.Contains(haystack, "permission") ||
		strings.Contains(haystack, "unauthorized")
}

type streamWithChatID struct {
	io.ReadCloser
	chatID string
}

func withChatID(stream io.ReadCloser, chatID string) io.ReadCloser {
	return &streamWithChatID{ReadCloser: stream, chatID: chatID}
}

func chatIDFromStream(stream io.ReadCloser) string {
	if wrapped, ok := stream.(*streamWithChatID); ok {
		return wrapped.chatID
	}
	return ""
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
	h.logger.DebugModule("OPENAI", "non-stream parsed model=%s full_content=%q usage=%s", model, fullContent, debugJSON(map[string]any{
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      totalTokens,
	}))
	parsedCalls := []toolcall.ToolCall(nil)
	normalizedContent := fullContent
	if len(toolNames) > 0 {
		parsedCalls = toolcall.ParseCalls(fullContent)
		if len(parsedCalls) > 0 {
			normalizedContent = toolcall.CleanVisibleText(fullContent)
		}
	}
	normalizedContent = toolcall.CleanVisibleText(normalizedContent)
	h.logger.DebugModule("OPENAI", "non-stream normalized model=%s normalized_content=%q parsed_tool_calls=%s", model, normalizedContent, debugJSON(parsedCalls))

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
