package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"qwen2api/internal/toolcall"
)

func estimateOpenAIInputTokens(messages []map[string]any, tools any, toolChoice any) int {
	total := 0
	for _, message := range messages {
		total += estimateTextTokens(fmt.Sprint(message["role"]))
		total += estimatePayloadTokens(message["content"])
		if name := strings.TrimSpace(fmt.Sprint(message["name"])); name != "" {
			total += estimateTextTokens(name)
		}
		if toolCallID := strings.TrimSpace(fmt.Sprint(message["tool_call_id"])); toolCallID != "" {
			total += estimateTextTokens(toolCallID)
		}
		if toolCalls, ok := message["tool_calls"]; ok {
			total += estimatePayloadTokens(toolCalls)
		}
	}
	total += estimatePayloadTokens(tools)
	total += estimatePayloadTokens(toolChoice)
	if total <= 0 {
		return 1
	}
	return total
}

func estimateOpenAIOutputTokens(content string, toolCalls []toolcall.ToolCall) int {
	total := estimateTextTokens(content)
	for _, call := range toolCalls {
		total += estimateTextTokens(call.Name)
		total += estimatePayloadTokens(call.Input)
	}
	if total <= 0 {
		return 1
	}
	return total
}

func applyUsageFallback(promptTokens, completionTokens, totalTokens, estimatedPrompt, estimatedCompletion int) (int, int, int) {
	if promptTokens <= 0 {
		promptTokens = estimatedPrompt
	}
	if completionTokens <= 0 {
		completionTokens = estimatedCompletion
	}
	if totalTokens <= 0 {
		totalTokens = promptTokens + completionTokens
	}
	return promptTokens, completionTokens, totalTokens
}

func estimatePayloadTokens(value any) int {
	switch v := value.(type) {
	case nil:
		return 0
	case string:
		return estimateTextTokens(v)
	case []byte:
		return estimateTextTokens(string(v))
	case json.RawMessage:
		return estimateTextTokens(string(v))
	case []any:
		total := 0
		for _, item := range v {
			total += estimatePayloadTokens(item)
		}
		return total
	case []map[string]any:
		total := 0
		for _, item := range v {
			total += estimatePayloadTokens(item)
		}
		return total
	case map[string]any:
		total := 0
		for key, item := range v {
			total += estimateTextTokens(key)
			total += estimatePayloadTokens(item)
		}
		return total
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return estimateTextTokens(fmt.Sprint(v))
		}
		return estimateTextTokens(string(raw))
	}
}
