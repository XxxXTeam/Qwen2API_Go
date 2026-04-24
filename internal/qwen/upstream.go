package qwen

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	maxStreamInspectBuffer = 4096
	defaultTimezoneHeader  = "Fri Apr 24 2026 12:00:00 GMT+0800"
)

var (
	toolPreludePatterns = []string{
		"<ml_tool_calls", "<ml_tool_call", "<tool_calls", "<tool_call",
		"<function_call", "<invoke", "<tool_use", "<antml:function_call",
	}
)

type RequestOptions struct {
	Accept      string
	ContentType string
	IncludeAuth bool
}

type UpstreamError struct {
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	Details    string `json:"details,omitempty"`
	ResponseID string `json:"response_id,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
	Retryable  bool   `json:"retryable,omitempty"`
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case strings.TrimSpace(e.Details) != "":
		return e.Details
	case strings.TrimSpace(e.Message) != "":
		return e.Message
	case strings.TrimSpace(e.Code) != "":
		return e.Code
	default:
		return "上游请求失败"
	}
}

type StreamInspectionResult struct {
	Stream        io.ReadCloser
	UpstreamError *UpstreamError
}

type streamInspectState struct {
	assistantContentProbe string
}

func InspectUpstreamStream(ctx context.Context, body io.ReadCloser) (*StreamInspectionResult, error) {
	defer func() {
		if ctx != nil {
			select {
			case <-ctx.Done():
				_ = body.Close()
			default:
			}
		}
	}()

	bufferedChunks := make([][]byte, 0, 8)
	var textBuffer strings.Builder
	inspectState := &streamInspectState{}
	reader := bufio.NewReader(body)

	for {
		chunk, err := reader.ReadBytes('\n')
		if len(chunk) > 0 {
			copied := append([]byte(nil), chunk...)
			bufferedChunks = append(bufferedChunks, copied)
			textBuffer.Write(copied)

			payloads, rest := parseSSEPayloads(textBuffer.String(), false)
			textBuffer.Reset()
			textBuffer.WriteString(rest)

			for _, payload := range payloads {
				decoded := parseJSONPayload(payload)
				if decoded == nil {
					continue
				}
				if upstreamErr := normalizeUpstreamError(decoded); upstreamErr != nil {
					_ = body.Close()
					return &StreamInspectionResult{UpstreamError: upstreamErr}, nil
				}
				if _, ok := decoded["response.created"]; ok {
					continue
				}
				if shouldReleaseBufferedStream(decoded, inspectState) {
					replay := io.NopCloser(io.MultiReader(bytes.NewReader(joinChunks(bufferedChunks)), reader))
					return &StreamInspectionResult{Stream: replay}, nil
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return &StreamInspectionResult{
					Stream: io.NopCloser(bytes.NewReader(joinChunks(bufferedChunks))),
				}, nil
			}
			_ = body.Close()
			return nil, err
		}
	}
}

func parseSSEPayloads(buffer string, flush bool) ([]string, string) {
	input := buffer
	if flush {
		input += "\n\n"
	}

	events := splitSSEEvents(input)
	if len(events) == 0 {
		return nil, buffer
	}

	payloads := make([]string, 0, len(events))
	rest := ""
	if !flush {
		rest = events[len(events)-1]
		events = events[:len(events)-1]
	}

	for _, event := range events {
		lines := strings.Split(event, "\n")
		dataLines := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
			if strings.HasPrefix(line, "data:") {
				value := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if value != "" && value != "[DONE]" {
					dataLines = append(dataLines, value)
				}
			}
		}
		if len(dataLines) > 0 {
			payloads = append(payloads, strings.Join(dataLines, "\n"))
		}
	}

	return payloads, rest
}

func splitSSEEvents(input string) []string {
	normalized := strings.ReplaceAll(input, "\r\n", "\n")
	return strings.Split(normalized, "\n\n")
}

func parseJSONPayload(payload string) map[string]any {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		return nil
	}
	return decoded
}

func normalizeUpstreamError(payload map[string]any) *UpstreamError {
	if payload == nil {
		return nil
	}

	var rawError map[string]any
	if direct, ok := payload["error"].(map[string]any); ok {
		rawError = direct
	}
	if rawError == nil {
		if data, ok := payload["data"].(map[string]any); ok {
			if nested, ok := data["error"].(map[string]any); ok {
				rawError = nested
			}
		}
	}

	if rawError == nil {
		if success, ok := payload["success"].(bool); ok && !success {
			if data, ok := payload["data"].(map[string]any); ok {
				if code := strings.TrimSpace(toString(data["code"])); code != "" {
					details := strings.TrimSpace(toString(data["details"]))
					status := http.StatusInternalServerError
					if code == "RateLimited" {
						status = http.StatusTooManyRequests
						waitHours := strings.TrimSpace(toString(data["num"]))
						if waitHours != "" {
							details = "当前账号的该功能使用次数已达上限，请等待约 " + waitHours + " 小时后再试"
						} else if details == "" {
							details = "当前账号的该功能使用次数已达上限，请稍后再试"
						}
					} else if strings.EqualFold(code, "Bad_Request") && strings.Contains(strings.ToLower(details), "internal error") {
						status = http.StatusBadGateway
					}
					return &UpstreamError{
						Code:       code,
						Details:    fallbackString(details, code),
						ResponseID: toString(payload["request_id"]),
						StatusCode: status,
						Retryable:  isRetryableUpstreamError(code, details),
					}
				}
			}
		}
		return nil
	}

	code := toString(rawError["code"])
	details := fallbackString(toString(rawError["details"]), toString(rawError["message"]), toString(payload["message"]))
	status := http.StatusBadGateway
	if isRetryableUpstreamError(code, details) && (strings.Contains(strings.ToLower(details), "quota") || strings.Contains(strings.ToLower(details), "token-limit")) {
		status = http.StatusTooManyRequests
	} else if strings.Contains(strings.ToLower(details), "overload") || strings.Contains(strings.ToLower(details), "temporarily unavailable") {
		status = http.StatusServiceUnavailable
	}
	return &UpstreamError{
		Code:       code,
		Message:    toString(rawError["message"]),
		Details:    details,
		ResponseID: fallbackString(toString(payload["response_id"]), toString(nestedValue(payload, "response", "id"))),
		StatusCode: status,
		Retryable:  isRetryableUpstreamError(code, details),
	}
}

func NormalizeUpstreamError(payload map[string]any) *UpstreamError {
	return normalizeUpstreamError(payload)
}

func shouldReleaseBufferedStream(payload map[string]any, state *streamInspectState) bool {
	choices, _ := payload["choices"].([]any)
	if len(choices) == 0 {
		return false
	}
	choice, _ := choices[0].(map[string]any)
	delta, _ := choice["delta"].(map[string]any)
	if delta == nil {
		return false
	}

	role := fallbackString(toString(delta["role"]), "assistant")
	if role != "assistant" {
		return false
	}

	if content, ok := delta["content"].(string); ok {
		state.assistantContentProbe = appendProbeText(state.assistantContentProbe, content)
		trimmed := strings.TrimSpace(state.assistantContentProbe)
		if trimmed == "" {
			return false
		}
		if isPendingToolPrelude(trimmed) {
			return false
		}
		return true
	}

	if functionCall, ok := delta["function_call"].(map[string]any); ok {
		return strings.TrimSpace(toString(functionCall["name"])) != "" || strings.TrimSpace(toString(functionCall["arguments"])) != ""
	}

	return false
}

func appendProbeText(current, chunk string) string {
	next := current + chunk
	if len(next) <= maxStreamInspectBuffer {
		return next
	}
	return next[len(next)-maxStreamInspectBuffer:]
}

func isPendingToolPrelude(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "<ml_tool_name") || strings.Contains(lower, "<ml_parameters") ||
		strings.Contains(lower, "<tool_name") || strings.Contains(lower, "<parameters") ||
		strings.Contains(lower, "<parameter") || strings.Contains(lower, "<antml:argument") {
		return false
	}
	for _, pattern := range toolPreludePatterns {
		if strings.HasPrefix(lower, pattern) && (strings.HasSuffix(lower, ">") || !strings.Contains(lower, ">")) {
			return true
		}
	}
	return false
}

func isRetryableUpstreamError(code, details string) bool {
	haystack := strings.ToLower(strings.TrimSpace(code + " " + details))
	return strings.Contains(haystack, "quota") ||
		strings.Contains(haystack, "allocated quota exceeded") ||
		strings.Contains(haystack, "token-limit") ||
		strings.Contains(haystack, "internal_error") ||
		(strings.Contains(strings.ToLower(code), "bad_request") && strings.Contains(haystack, "internal error"))
}

func joinChunks(chunks [][]byte) []byte {
	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
	}
	merged := make([]byte, 0, total)
	for _, chunk := range chunks {
		merged = append(merged, chunk...)
	}
	return merged
}

func toString(v any) string {
	switch vv := v.(type) {
	case string:
		return vv
	case json.Number:
		return vv.String()
	case float64:
		return strings.TrimSpace(strings.TrimRight(strings.TrimRight(strconvFormatFloat(vv), "0"), "."))
	case int:
		return strconvFormatInt(int64(vv))
	case int64:
		return strconvFormatInt(vv)
	case bool:
		if vv {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

func nestedValue(payload map[string]any, path ...string) any {
	current := any(payload)
	for _, part := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = obj[part]
	}
	return current
}

func fallbackString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func strconvFormatInt(v int64) string {
	return json.Number(strconvFormatFloat(float64(v))).String()
}

func strconvFormatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
