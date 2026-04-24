package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"qwen2api/internal/qwen"
)

var (
	markdownURLPattern = regexp.MustCompile(`!\[[^\]]*\]\((https?://[^\s)]+)\)`)
	downloadURLPattern = regexp.MustCompile(`\[Download [^\]]+\]\((https?://[^\s)]+)\)`)
	httpURLPattern     = regexp.MustCompile(`https?://[^\s<>"')\]]+`)
	responseIDPattern  = regexp.MustCompile(`"response(?:_id|Id)"\s*:\s*"([^"]+)"`)
	taskIDPatterns     = []*regexp.Regexp{
		regexp.MustCompile(`"task_id"\s*:\s*"([^"]+)"`),
		regexp.MustCompile(`"taskId"\s*:\s*"([^"]+)"`),
		regexp.MustCompile(`task_id\s*[:=]\s*["']?([a-zA-Z0-9._-]+)["']?`),
		regexp.MustCompile(`taskId\s*[:=]\s*["']?([a-zA-Z0-9._-]+)["']?`),
		regexp.MustCompile(`"id"\s*:\s*"([^"]+)"[\s\S]{0,120}"task_status"`),
	}
)

type assetResult struct {
	ContentURL         string
	TaskID             string
	TaskCandidates     []string
	ResponseIDs        []string
	UpstreamError      *qwen.UpstreamError
	RawPreview         string
	RawBody            []byte
	ChatDetailFallback bool
}

func parseAssetResult(raw []byte) assetResult {
	text := strings.TrimSpace(string(raw))
	result := assetResult{
		RawBody:    append([]byte(nil), raw...),
		RawPreview: previewText(text),
	}
	if text == "" {
		return result
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err == nil {
		applyAssetPayload(&result, payload)
	}

	payloads, _ := parseSSEPayloads(text, true)
	for _, payloadText := range payloads {
		var payload any
		if err := json.Unmarshal([]byte(payloadText), &payload); err == nil {
			applyAssetPayload(&result, payload)
			continue
		}
		applyAssetPayload(&result, payloadText)
	}

	if result.UpstreamError == nil {
		result.UpstreamError = parseAssetError(text)
	}
	if result.ContentURL == "" {
		result.ContentURL = extractResourceURL(text)
	}
	for _, responseID := range extractResponseIDs(text) {
		pushUnique(&result.ResponseIDs, responseID)
	}
	for _, taskID := range extractTaskIDs(text) {
		pushUnique(&result.TaskCandidates, taskID)
	}
	if result.TaskID == "" && len(result.TaskCandidates) > 0 {
		result.TaskID = result.TaskCandidates[0]
	}
	return result
}

func readAssetResult(body io.Reader) (assetResult, error) {
	raw, err := io.ReadAll(body)
	if err != nil {
		return assetResult{}, err
	}
	return parseAssetResult(raw), nil
}

func applyAssetPayload(result *assetResult, payload any) {
	if result == nil || payload == nil {
		return
	}
	if result.UpstreamError == nil {
		if obj, ok := parseJSONLikeMap(payload); ok {
			result.UpstreamError = qwen.NormalizeUpstreamError(obj)
		}
	}
	if result.ContentURL == "" {
		result.ContentURL = extractResourceURLFromPayload(payload)
	}
	for _, responseID := range extractResponseIDsFromPayload(payload) {
		pushUnique(&result.ResponseIDs, responseID)
	}
	for _, taskID := range extractTaskIDsFromPayload(payload) {
		pushUnique(&result.TaskCandidates, taskID)
	}
	if result.TaskID == "" && len(result.TaskCandidates) > 0 {
		result.TaskID = result.TaskCandidates[0]
	}
}

func parseAssetError(value any) *qwen.UpstreamError {
	switch v := value.(type) {
	case string:
		var decoded map[string]any
		if err := json.Unmarshal([]byte(v), &decoded); err == nil {
			return qwen.NormalizeUpstreamError(decoded)
		}
		return nil
	default:
		obj, ok := parseJSONLikeMap(value)
		if !ok {
			return nil
		}
		return qwen.NormalizeUpstreamError(obj)
	}
}

func parseJSONLikeMap(value any) (map[string]any, bool) {
	switch v := value.(type) {
	case map[string]any:
		return v, true
	case []byte:
		var decoded map[string]any
		if err := json.Unmarshal(v, &decoded); err != nil {
			return nil, false
		}
		return decoded, true
	case string:
		var decoded map[string]any
		if err := json.Unmarshal([]byte(v), &decoded); err != nil {
			return nil, false
		}
		return decoded, true
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, false
		}
		return decoded, true
	}
}

func extractResourceURL(text string) string {
	if match := markdownURLPattern.FindStringSubmatch(text); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	if match := downloadURLPattern.FindStringSubmatch(text); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return extractFirstURL(text)
}

func extractFirstURL(text string) string {
	return httpURLPattern.FindString(text)
}

func parseSSEPayloads(raw string, flush bool) ([]string, string) {
	input := raw
	if flush {
		input += "\n\n"
	}
	input = strings.ReplaceAll(input, "\r\n", "\n")
	events := strings.Split(input, "\n\n")
	payloads := make([]string, 0, len(events))
	rest := ""
	if !flush && len(events) > 0 {
		rest = events[len(events)-1]
		events = events[:len(events)-1]
	}
	for _, event := range events {
		lines := strings.Split(event, "\n")
		dataLines := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
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

func extractResourceURLFromPayload(payload any) string {
	switch v := payload.(type) {
	case nil:
		return ""
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return ""
		}
		var nested any
		if (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) && json.Unmarshal([]byte(trimmed), &nested) == nil {
			if url := extractResourceURLFromPayload(nested); url != "" {
				return url
			}
		}
		return extractResourceURL(trimmed)
	case []any:
		for _, item := range v {
			if url := extractResourceURLFromPayload(item); url != "" {
				return url
			}
		}
	case map[string]any:
		directCandidates := []any{
			v["content"], v["url"], v["image"], v["video"], v["video_url"], v["videoUrl"],
			v["download_url"], v["downloadUrl"], v["file_url"], v["resource_url"], v["resourceUrl"],
			v["output_url"], v["result_url"], v["final_url"], v["finalUrl"], v["uri"],
		}
		for _, candidate := range directCandidates {
			if url := extractResourceURLFromPayload(candidate); url != "" {
				return url
			}
		}
		nestedCandidates := []any{
			v["data"], v["message"], v["delta"], v["extra"], v["choices"], v["messages"],
			v["output"], v["result"], v["results"], v["urls"], v["files"], v["image_list"], v["video_list"],
			v["response"],
		}
		for _, candidate := range nestedCandidates {
			if url := extractResourceURLFromPayload(candidate); url != "" {
				return url
			}
		}
		for _, candidate := range collectNestedValues(v) {
			if url := extractResourceURLFromPayload(candidate); url != "" {
				return url
			}
		}
	}
	return ""
}

func extractResponseIDs(text string) []string {
	matches := responseIDPattern.FindAllStringSubmatch(text, -1)
	results := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			pushUnique(&results, match[1])
		}
	}
	return results
}

func extractResponseIDsFromPayload(payload any) []string {
	switch v := payload.(type) {
	case nil:
		return nil
	case []any:
		var results []string
		for _, item := range v {
			for _, responseID := range extractResponseIDsFromPayload(item) {
				pushUnique(&results, responseID)
			}
		}
		return results
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		var nested any
		if json.Unmarshal([]byte(trimmed), &nested) == nil {
			return extractResponseIDsFromPayload(nested)
		}
		return extractResponseIDs(trimmed)
	case map[string]any:
		var results []string
		pushUnique(&results, stringValue(v["response_id"]))
		pushUnique(&results, stringValue(v["responseId"]))
		if response, ok := v["response"].(map[string]any); ok {
			if created, ok := response["created"].(map[string]any); ok {
				pushUnique(&results, stringValue(created["response_id"]))
				pushUnique(&results, stringValue(created["responseId"]))
			}
			pushUnique(&results, stringValue(response["id"]))
		}
		for _, candidate := range collectNestedValues(v) {
			for _, responseID := range extractResponseIDsFromPayload(candidate) {
				pushUnique(&results, responseID)
			}
		}
		return results
	default:
		return nil
	}
}

func extractTaskIDs(text string) []string {
	results := make([]string, 0)
	for _, pattern := range taskIDPatterns {
		for _, match := range pattern.FindAllStringSubmatch(text, -1) {
			if len(match) > 1 {
				pushUnique(&results, match[1])
			}
		}
	}
	return results
}

func extractTaskIDsFromPayload(payload any) []string {
	switch v := payload.(type) {
	case nil:
		return nil
	case []any:
		var results []string
		for _, item := range v {
			for _, taskID := range extractTaskIDsFromPayload(item) {
				pushUnique(&results, taskID)
			}
		}
		return results
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		var nested any
		if json.Unmarshal([]byte(trimmed), &nested) == nil {
			return extractTaskIDsFromPayload(nested)
		}
		return extractTaskIDs(trimmed)
	case map[string]any:
		var results []string
		candidates := []any{
			v["task_id"], v["taskId"],
		}
		for _, key := range []string{"wanx", "output", "result", "results"} {
			if obj, ok := v[key].(map[string]any); ok {
				candidates = append(candidates, obj["task_id"], obj["taskId"], obj["id"])
			}
		}
		for _, candidate := range candidates {
			pushUnique(&results, stringValue(candidate))
		}
		if status := strings.ToLower(stringValue(v["status"])); status == "pending" || status == "running" {
			pushUnique(&results, stringValue(v["id"]))
		}
		if taskStatus := stringValue(v["task_status"]); taskStatus != "" {
			pushUnique(&results, stringValue(v["id"]))
		}
		for _, candidate := range collectNestedValues(v) {
			for _, taskID := range extractTaskIDsFromPayload(candidate) {
				pushUnique(&results, taskID)
			}
		}
		return results
	default:
		return nil
	}
}

func collectNestedValues(payload any) []any {
	switch v := payload.(type) {
	case nil:
		return nil
	case []any:
		results := make([]any, 0, len(v))
		for _, item := range v {
			results = append(results, item)
			results = append(results, collectNestedValues(item)...)
		}
		return results
	case map[string]any:
		results := make([]any, 0, len(v))
		for _, item := range v {
			results = append(results, item)
			results = append(results, collectNestedValues(item)...)
		}
		return results
	default:
		return nil
	}
}

func previewText(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= 400 {
		return trimmed
	}
	return trimmed[:400]
}

func pushUnique(items *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, item := range *items {
		if item == value {
			return
		}
	}
	*items = append(*items, value)
}

func stringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	default:
		return ""
	}
}

func resolveAssetURL(result assetResult, fallbackChatID string) (string, error) {
	if result.UpstreamError != nil {
		return "", result.UpstreamError
	}
	if result.ContentURL != "" {
		return result.ContentURL, nil
	}
	if fallbackChatID == "" {
		return "", errors.New("未能从上游响应中解析资源链接")
	}
	return "", errors.New("未能从上游响应或聊天详情中解析资源链接")
}

func extractAssetFromChatDetail(chatDetail map[string]any, responseIDs []string) string {
	for _, message := range matchingChatMessages(chatDetail, responseIDs) {
		if url := extractResourceURLFromPayload(message); url != "" {
			return url
		}
	}
	return ""
}

func extractVideoTasksFromChatDetail(chatDetail map[string]any, responseIDs []string) []string {
	responseIDSet := make(map[string]struct{}, len(responseIDs))
	for _, responseID := range responseIDs {
		if trimmed := strings.TrimSpace(responseID); trimmed != "" {
			responseIDSet[trimmed] = struct{}{}
		}
	}
	var tasks []string
	for _, message := range matchingChatMessages(chatDetail, responseIDs) {
		for _, taskID := range extractTaskIDsFromPayload(message) {
			if _, exists := responseIDSet[taskID]; exists {
				continue
			}
			pushUnique(&tasks, taskID)
		}
	}
	return tasks
}

func matchingChatMessages(chatDetail map[string]any, responseIDs []string) []any {
	if chatDetail == nil {
		return nil
	}
	idSet := make(map[string]struct{}, len(responseIDs))
	for _, responseID := range responseIDs {
		if strings.TrimSpace(responseID) != "" {
			idSet[strings.TrimSpace(responseID)] = struct{}{}
		}
	}

	var messages []any
	if data, ok := chatDetail["data"].(map[string]any); ok {
		if chat, ok := data["chat"].(map[string]any); ok {
			if history, ok := chat["history"].(map[string]any); ok {
				if messageMap, ok := history["messages"].(map[string]any); ok {
					for _, message := range messageMap {
						messages = append(messages, message)
					}
				}
			}
		}
	}
	if len(messages) == 0 {
		messages = append(messages, collectNestedValues(chatDetail)...)
	}
	if len(idSet) == 0 {
		return messages
	}

	filtered := make([]any, 0, len(messages))
	for _, message := range messages {
		obj, ok := message.(map[string]any)
		if !ok {
			continue
		}
		responseID := stringValue(obj["response_id"])
		if responseID == "" {
			responseID = stringValue(obj["responseId"])
		}
		if responseID == "" {
			responseID = stringValue(obj["id"])
		}
		if _, ok := idSet[responseID]; ok {
			filtered = append(filtered, obj)
		}
	}
	return filtered
}
