package toolcall

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

type ToolSchema struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type ToolChoicePolicy struct {
	Enabled      bool
	Mode         string
	RequiredTool string
}

type ToolCall struct {
	Name  string
	Input map[string]any
}

type InjectionResult struct {
	Messages  []map[string]any
	ToolNames []string
	Policy    ToolChoicePolicy
}

var (
	xmlToolCallsBlock          = regexp.MustCompile(`(?is)<(?:ml_tool_calls|tool_calls)[^>]*>(.*?)</(?:ml_tool_calls|tool_calls)>`)
	xmlToolCallBlock           = regexp.MustCompile(`(?is)<(?:ml_tool_call|tool_call)[^>]*>(.*?)</(?:ml_tool_call|tool_call)>`)
	xmlToolNameBlock           = regexp.MustCompile(`(?is)<(?:ml_tool_name|tool_name)>(.*?)</(?:ml_tool_name|tool_name)>`)
	xmlParameters              = regexp.MustCompile(`(?is)<(?:ml_parameters|parameters)>(.*?)</(?:ml_parameters|parameters)>`)
	xmlParameterItem           = regexp.MustCompile(`(?is)<([a-zA-Z_][\w.:-]*)>(.*?)</([a-zA-Z_][\w.:-]*)>`)
	xmlCDATA                   = regexp.MustCompile(`(?is)<!\[CDATA\[(.*?)\]\]>`)
	xmlNoiseBlock              = regexp.MustCompile(`(?is)<(?:ml_tool_calls|ml_tool_call|ml_tool_result|tool_calls|tool_call|tool_result|function_call|invoke|tool_use)[^>]*>.*?</(?:ml_tool_calls|ml_tool_call|ml_tool_result|tool_calls|tool_call|tool_result|function_call|invoke|tool_use)>`)
	xmlResidualTag             = regexp.MustCompile(`(?is)</?(?:ml_tool_calls|ml_tool_call|ml_tool_result|ml_tool_name|ml_tool_call_id|ml_parameters|tool_calls|tool_call|tool_result|tool_name|parameters|function_call|invoke|tool_use)\b[^>]*>`)
	malformedToolPreludePrefix = regexp.MustCompile(`(?is)^(?:\s*<(?:ml_tool_calls|ml_tool_call|tool_calls|tool_call|function_call|invoke|tool_use|antml:function_call)\s*>\s*)+`)
	leadingToolTagLines        = regexp.MustCompile(`(?is)^(?:\s*</?(?:ml_tool_calls|ml_tool_call|ml_tool_result|ml_tool_name|ml_tool_call_id|ml_parameters|tool_calls|tool_call|tool_result|tool_name|parameters|function_call|invoke|tool_use)\b[^>]*>\s*)+`)
	startMarkers               = []string{"<ml_tool_calls", "<ml_tool_call", "<tool_calls", "<tool_call"}
	toolTagPairs               = []struct {
		open  string
		close string
	}{
		{open: "<ml_tool_calls", close: "</ml_tool_calls>"},
		{open: "<ml_tool_call", close: "</ml_tool_call>"},
		{open: "<tool_calls", close: "</tool_calls>"},
		{open: "<tool_call", close: "</tool_call>"},
		{open: "<function_call", close: "</function_call>"},
		{open: "<invoke", close: "</invoke>"},
		{open: "<tool_use", close: "</tool_use>"},
		{open: "<antml:function_call", close: "</antml:function_call>"},
	}
)

type StreamState struct {
	pending     string
	capturing   bool
	captureBuff string
}

type StreamChunkResult struct {
	Content   string
	ToolCalls []ToolCall
}

func InjectPrompt(messages []map[string]any, toolsRaw any, toolChoice any) InjectionResult {
	normalizedMessages := normalizeToolMessages(messages)
	toolSchemas := normalizeToolSchemas(toolsRaw)
	if len(toolSchemas) == 0 {
		return InjectionResult{Messages: normalizedMessages}
	}

	toolNames := make([]string, 0, len(toolSchemas))
	for _, schema := range toolSchemas {
		toolNames = append(toolNames, schema.Name)
	}

	policy := parseToolChoicePolicy(toolChoice, toolNames)
	if !policy.Enabled {
		return InjectionResult{
			Messages:  normalizedMessages,
			ToolNames: toolNames,
			Policy:    policy,
		}
	}

	sections := []string{"You have access to these tools:", ""}
	for _, schema := range toolSchemas {
		rawParams, _ := json.Marshal(schema.Parameters)
		sections = append(sections,
			fmt.Sprintf("Tool: %s", schema.Name),
			fmt.Sprintf("Description: %s", fallbackText(schema.Description, "(no description provided)")),
			fmt.Sprintf("Parameters: %s", string(rawParams)),
			"",
		)
	}
	sections = append(sections, buildInstructions(toolNames, policy))
	toolPrompt := strings.TrimSpace(strings.Join(sections, "\n"))

	for i, message := range normalizedMessages {
		if strings.EqualFold(fmt.Sprint(message["role"]), "system") {
			current := normalizeMessageTextContent(message["content"])
			if strings.TrimSpace(current) == "" {
				normalizedMessages[i]["content"] = toolPrompt
			} else {
				normalizedMessages[i]["content"] = strings.TrimSpace(current) + "\n\n" + toolPrompt
			}
			return InjectionResult{
				Messages:  appendToolReminder(normalizedMessages, toolNames, policy),
				ToolNames: toolNames,
				Policy:    policy,
			}
		}
	}

	return InjectionResult{
		Messages:  appendToolReminder(append([]map[string]any{{"role": "system", "content": toolPrompt}}, normalizedMessages...), toolNames, policy),
		ToolNames: toolNames,
		Policy:    policy,
	}
}

func appendToolReminder(messages []map[string]any, toolNames []string, policy ToolChoicePolicy) []map[string]any {
	if !policy.Enabled || len(messages) == 0 || len(toolNames) == 0 {
		return messages
	}

	reminder := buildReminder(toolNames, policy)
	if strings.TrimSpace(reminder) == "" {
		return messages
	}

	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] == nil {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(fmt.Sprint(messages[i]["role"])))
		if role == "system" {
			continue
		}
		messages[i]["content"] = appendReminderToContent(messages[i]["content"], reminder)
		return messages
	}
	return messages
}

func buildReminder(toolNames []string, policy ToolChoicePolicy) string {
	if len(toolNames) == 0 {
		return ""
	}

	modeLine := "If a tool is needed, use only the listed ml_tool names."
	if policy.Mode == "required" {
		modeLine = "You must call one listed ml_tool before the final answer."
	}
	if policy.Mode == "specific" && policy.RequiredTool != "" {
		modeLine = fmt.Sprintf("You must call ml_tool %q before the final answer.", policy.RequiredTool)
	}

	return strings.Join([]string{
		"[ml_tool reminder]",
		"Ignore built-in/native/platform tools.",
		fmt.Sprintf("Allowed ml_tool names: %s.", strings.Join(toolNames, ", ")),
		modeLine,
		"If calling a tool, output only complete <ml_tool_calls> XML with <ml_tool_name> and <ml_parameters>.",
	}, "\n")
}

func appendReminderToContent(content any, reminder string) any {
	if strings.TrimSpace(reminder) == "" {
		return content
	}

	switch value := content.(type) {
	case string:
		if strings.Contains(value, "[ml_tool reminder]") {
			return value
		}
		if strings.TrimSpace(value) == "" {
			return reminder
		}
		return strings.TrimSpace(value) + "\n\n" + reminder
	case []any:
		hasReminder := false
		items := make([]any, 0, len(value)+1)
		for _, raw := range value {
			item, ok := raw.(map[string]any)
			if ok && strings.EqualFold(fmt.Sprint(item["type"]), "text") && strings.Contains(fmt.Sprint(item["text"]), "[ml_tool reminder]") {
				hasReminder = true
			}
			items = append(items, raw)
		}
		if hasReminder {
			return value
		}
		items = append(items, map[string]any{"type": "text", "text": reminder})
		return items
	default:
		return reminder
	}
}

func buildInstructions(toolNames []string, policy ToolChoicePolicy) string {
	toolList := "(none)"
	if len(toolNames) > 0 {
		toolList = strings.Join(toolNames, ", ")
	}
	modeLine := "Call a tool only when it is necessary."
	if policy.Mode == "required" {
		modeLine = "You must call one of the provided tools before giving a final answer."
	}
	if policy.Mode == "specific" && policy.RequiredTool != "" {
		modeLine = fmt.Sprintf("You must call the tool %q before giving a final answer.", policy.RequiredTool)
	}

	return strings.Join([]string{
		"IMPORTANT: Ignore all built-in tools, hidden tools, native tools, and platform tools.",
		"The ONLY tools you may use are the explicit tool names listed below.",
		"Never say that tool resources are exhausted. Never say you will directly chat instead. Never mention built-in tool failures.",
		"Never output role=\"function\" or function_call JSON.",
		"Never output {\"name\":...,\"arguments\":...}, \"Tool does not exists.\", or any prose about tool execution availability.",
		"",
		"When you decide to use a tool, respond with XML only and no extra prose.",
		"Use ONLY the exact XML schema below.",
		"Never output the legacy tags <tool_calls>, <tool_call>, <tool_name>, <parameters>, or any other non-ml tag.",
		"Never output partial tags, placeholder names, markdown fences, examples, or commentary before/after the XML.",
		"Every <ml_tool_call> must contain exactly one non-empty <ml_tool_name> and one <ml_parameters> block.",
		"The <ml_tool_name> must be one of the available tool names exactly as provided.",
		"Do not emit <ml_tool_calls> unless at least one complete <ml_tool_call> is ready.",
		"If you are not calling a tool, do not mention XML or tools. Answer normally.",
		"",
		"Available tool names:",
		toolList,
		modeLine,
		"",
		"Use this exact structure:",
		"<ml_tool_calls>",
		"  <ml_tool_call>",
		"    <ml_tool_name>TOOL_NAME_HERE</ml_tool_name>",
		"    <ml_parameters>",
		"      <ARG_NAME><![CDATA[ARG_VALUE]]></ARG_NAME>",
		"    </ml_parameters>",
		"  </ml_tool_call>",
		"</ml_tool_calls>",
		"",
		"Bad example: <tool_calls> or <tool_call> or <function_call>",
		"Bad example: <ml_tool_calls> without a complete nested <ml_tool_call>",
		"Bad example: ```xml ...``` or {\"tool_calls\":[...]}",
		"Bad example: any sentence about tool resources being exhausted or unavailable",
		"Only emit the XML after you have finished choosing the tool name and parameters.",
		"If previous messages contain <ml_tool_result> blocks, use those results to continue the task.",
	}, "\n")
}

func ParseCalls(text string) []ToolCall {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	calls := make([]ToolCall, 0)
	matches := xmlToolCallsBlock.FindAllStringSubmatch(text, -1)
	for _, wrapper := range matches {
		for _, block := range xmlToolCallBlock.FindAllStringSubmatch(wrapper[1], -1) {
			call := parseToolCallBlock(block[1])
			if call.Name != "" {
				calls = append(calls, call)
			}
		}
	}

	if len(calls) == 0 {
		for _, block := range xmlToolCallBlock.FindAllStringSubmatch(text, -1) {
			call := parseToolCallBlock(block[1])
			if call.Name != "" {
				calls = append(calls, call)
			}
		}
	}

	return dedupe(calls)
}

func parseToolCallBlock(block string) ToolCall {
	nameMatch := xmlToolNameBlock.FindStringSubmatch(block)
	if len(nameMatch) < 2 {
		return ToolCall{}
	}

	params := map[string]any{}
	paramMatch := xmlParameters.FindStringSubmatch(block)
	if len(paramMatch) >= 2 {
		for _, item := range xmlParameterItem.FindAllStringSubmatch(paramMatch[1], -1) {
			if len(item) < 4 || item[1] != item[3] {
				continue
			}
			params[item[1]] = decodeXMLText(item[2])
		}
	}

	return ToolCall{
		Name:  decodeXMLText(nameMatch[1]),
		Input: params,
	}
}

func RemoveMarkup(text string) string {
	cleaned := xmlNoiseBlock.ReplaceAllString(text, "")
	cleaned = xmlResidualTag.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

func removeMarkupPreserveWhitespace(text string) string {
	cleaned := xmlNoiseBlock.ReplaceAllString(text, "")
	cleaned = xmlResidualTag.ReplaceAllString(cleaned, "")
	return cleaned
}

func FormatOpenAIToolCalls(calls []ToolCall) []map[string]any {
	result := make([]map[string]any, 0, len(calls))
	for index, call := range calls {
		rawArgs, _ := json.Marshal(call.Input)
		result = append(result, map[string]any{
			"index": index,
			"id":    "call_" + randomHex(8),
			"type":  "function",
			"function": map[string]any{
				"name":      call.Name,
				"arguments": string(rawArgs),
			},
		})
	}
	return result
}

func NewStreamState() *StreamState {
	return &StreamState{}
}

func ProcessStreamChunk(state *StreamState, chunk string) StreamChunkResult {
	state.pending += chunk

	if state.capturing {
		state.captureBuff += state.pending
		state.pending = ""
		if ready, content, calls := tryConsumeCapture(state.captureBuff); ready {
			state.capturing = false
			state.captureBuff = ""
			return StreamChunkResult{Content: content, ToolCalls: calls}
		}
		if shouldFallbackMalformedCapture(state.captureBuff) {
			fallback := stripMalformedToolPreludePrefix(state.captureBuff)
			state.capturing = false
			state.captureBuff = ""
			return StreamChunkResult{Content: fallback}
		}
		return StreamChunkResult{}
	}

	if idx := firstMarkerIndex(state.pending); idx >= 0 {
		safe := state.pending[:idx]
		state.capturing = true
		state.captureBuff = state.pending[idx:]
		state.pending = ""
		return StreamChunkResult{Content: safe}
	}

	suffixLen := longestMarkerPrefixSuffix(state.pending)
	if suffixLen == 0 {
		safe := state.pending
		state.pending = ""
		return StreamChunkResult{Content: safe}
	}
	if len(state.pending) > suffixLen {
		splitIndex := safeUTF8SplitIndex(state.pending, len(state.pending)-suffixLen)
		safe := state.pending[:splitIndex]
		state.pending = state.pending[splitIndex:]
		return StreamChunkResult{Content: safe}
	}
	return StreamChunkResult{}
}

func FinalizeStream(state *StreamState) StreamChunkResult {
	toolSource := state.pending
	if state.capturing {
		toolSource = state.captureBuff + state.pending
	}

	if state.capturing {
		if ready, content, calls := tryConsumeCapture(toolSource); ready {
			return StreamChunkResult{Content: content, ToolCalls: calls}
		}
		return StreamChunkResult{Content: stripToolPromptLeakage(stripMalformedToolPreludePrefix(RemoveMarkup(toolSource)))}
	}

	calls := ParseCalls(toolSource)
	if len(calls) > 0 {
		return StreamChunkResult{
			Content:   stripToolPromptLeakage(RemoveMarkup(toolSource)),
			ToolCalls: calls,
		}
	}
	return StreamChunkResult{Content: stripToolPromptLeakage(stripMalformedToolPreludePrefix(toolSource))}
}

func normalizeToolSchemas(toolsRaw any) []ToolSchema {
	items, ok := toolsRaw.([]any)
	if !ok {
		return nil
	}
	result := make([]ToolSchema, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		target := item
		if strings.EqualFold(fmt.Sprint(item["type"]), "function") {
			if fn, ok := item["function"].(map[string]any); ok {
				target = fn
			}
		}
		name := strings.TrimSpace(fmt.Sprint(target["name"]))
		if name == "" {
			continue
		}
		parameters, _ := target["parameters"].(map[string]any)
		result = append(result, ToolSchema{
			Name:        name,
			Description: strings.TrimSpace(fmt.Sprint(target["description"])),
			Parameters:  parameters,
		})
	}
	return result
}

func parseToolChoicePolicy(raw any, toolNames []string) ToolChoicePolicy {
	switch value := raw.(type) {
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "none":
			return ToolChoicePolicy{Enabled: false, Mode: "none"}
		case "required":
			return ToolChoicePolicy{Enabled: true, Mode: "required"}
		default:
			return ToolChoicePolicy{Enabled: len(toolNames) > 0, Mode: "auto"}
		}
	case map[string]any:
		required := ""
		if fn, ok := value["function"].(map[string]any); ok {
			required = strings.TrimSpace(fmt.Sprint(fn["name"]))
		}
		if required == "" {
			required = strings.TrimSpace(fmt.Sprint(value["name"]))
		}
		if required != "" {
			return ToolChoicePolicy{
				Enabled:      true,
				Mode:         "specific",
				RequiredTool: required,
			}
		}
	}
	return ToolChoicePolicy{Enabled: len(toolNames) > 0, Mode: "auto"}
}

func normalizeToolMessages(messages []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	systemParts := make([]string, 0)

	for _, message := range messages {
		if message == nil {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(fmt.Sprint(message["role"])))
		switch role {
		case "system":
			content := normalizeMessageTextContent(message["content"])
			if strings.TrimSpace(content) != "" {
				systemParts = append(systemParts, content)
			}
		case "assistant":
			if toolCalls, ok := message["tool_calls"].([]any); ok && len(toolCalls) > 0 {
				content := normalizeMessageTextContent(message["content"])
				toolMarkup := formatAssistantToolCalls(toolCalls)
				if strings.TrimSpace(content) != "" {
					content += "\n\n" + toolMarkup
				} else {
					content = toolMarkup
				}
				result = append(result, map[string]any{"role": "assistant", "content": content})
				continue
			}
			result = append(result, message)
		case "tool":
			content := formatToolResult(message)
			result = append(result, map[string]any{"role": "user", "content": content})
		default:
			result = append(result, message)
		}
	}

	if len(systemParts) > 0 {
		result = append([]map[string]any{{"role": "system", "content": strings.Join(systemParts, "\n\n")}}, result...)
	}

	return result
}

func NormalizeToolMessagesForExecution(messages []map[string]any) []map[string]any {
	return normalizeToolMessages(messages)
}

func formatAssistantToolCalls(toolCalls []any) string {
	blocks := make([]string, 0, len(toolCalls))
	for _, raw := range toolCalls {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		function, _ := item["function"].(map[string]any)
		name := strings.TrimSpace(fmt.Sprint(function["name"]))
		if name == "" {
			name = strings.TrimSpace(fmt.Sprint(item["name"]))
		}
		if name == "" {
			continue
		}

		args := map[string]any{}
		rawArgs := function["arguments"]
		if rawArgs == nil {
			rawArgs = item["arguments"]
		}
		switch value := rawArgs.(type) {
		case string:
			_ = json.Unmarshal([]byte(value), &args)
		case map[string]any:
			args = value
		}

		parameters := make([]string, 0, len(args))
		for key, value := range args {
			parameters = append(parameters, fmt.Sprintf("      <%s><![CDATA[%v]]></%s>", sanitizeTagName(key), value, sanitizeTagName(key)))
		}
		blocks = append(blocks, strings.Join([]string{
			"  <ml_tool_call>",
			fmt.Sprintf("    <ml_tool_name>%s</ml_tool_name>", escapeXML(name)),
			"    <ml_parameters>",
			strings.Join(parameters, "\n"),
			"    </ml_parameters>",
			"  </ml_tool_call>",
		}, "\n"))
	}

	if len(blocks) == 0 {
		return ""
	}
	return "<ml_tool_calls>\n" + strings.Join(blocks, "\n") + "\n</ml_tool_calls>"
}

func formatToolResult(message map[string]any) string {
	name := strings.TrimSpace(fmt.Sprint(message["name"]))
	if name == "" {
		name = "tool"
	}
	callID := strings.TrimSpace(fmt.Sprint(message["tool_call_id"]))
	content := normalizeMessageTextContent(message["content"])
	lines := []string{
		"<ml_tool_result>",
		fmt.Sprintf("  <ml_tool_name>%s</ml_tool_name>", escapeXML(name)),
	}
	if callID != "" {
		lines = append(lines, fmt.Sprintf("  <ml_tool_call_id>%s</ml_tool_call_id>", escapeXML(callID)))
	}
	lines = append(lines, fmt.Sprintf("  <content><![CDATA[%s]]></content>", content))
	lines = append(lines, "</ml_tool_result>")
	return strings.Join(lines, "\n")
}

func normalizeMessageTextContent(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0)
		for _, raw := range value {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if strings.EqualFold(fmt.Sprint(item["type"]), "text") {
				parts = append(parts, fmt.Sprint(item["text"]))
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func decodeXMLText(value string) string {
	value = xmlCDATA.ReplaceAllString(value, "$1")
	replacer := strings.NewReplacer(
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&apos;", "'",
		"&amp;", "&",
	)
	return strings.TrimSpace(replacer.Replace(value))
}

func escapeXML(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}

func sanitizeTagName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "value"
	}
	var builder strings.Builder
	for i, r := range name {
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == ':'
		if !valid {
			r = '_'
		}
		if i == 0 && !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_') {
			builder.WriteByte('_')
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func dedupe(calls []ToolCall) []ToolCall {
	seen := map[string]bool{}
	result := make([]ToolCall, 0, len(calls))
	for _, call := range calls {
		rawInput, _ := json.Marshal(call.Input)
		key := call.Name + ":" + string(rawInput)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, call)
	}
	return result
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "fallback"
	}
	return hex.EncodeToString(buf)
}

func firstMarkerIndex(text string) int {
	result := -1
	lower := strings.ToLower(text)
	for _, marker := range startMarkers {
		idx := strings.Index(lower, strings.ToLower(marker))
		if idx >= 0 && (result == -1 || idx < result) {
			result = idx
		}
	}
	return result
}

func tryConsumeCapture(captured string) (bool, string, []ToolCall) {
	source := captured
	lower := strings.ToLower(source)

	for _, pair := range toolTagPairs {
		openIndex := strings.Index(lower, strings.ToLower(pair.open))
		if openIndex == -1 {
			continue
		}

		closeIndex := strings.LastIndex(lower, strings.ToLower(pair.close))
		if closeIndex < openIndex {
			return false, "", nil
		}

		closeEnd := closeIndex + len(pair.close)
		xmlBlock := source[openIndex:closeEnd]
		prefix := source[:openIndex]
		suffix := source[closeEnd:]
		calls := ParseCalls(xmlBlock)
		if len(calls) > 0 {
			return true, stripToolPromptLeakage(RemoveMarkup(prefix + xmlBlock + suffix)), calls
		}
		return true, stripToolPromptLeakage(stripMalformedToolPreludePrefix(prefix + xmlBlock + suffix)), nil
	}

	if strings.Contains(lower, "</ml_tool_calls>") || strings.Contains(lower, "</tool_calls>") || strings.Contains(lower, "</ml_tool_call>") || strings.Contains(lower, "</tool_call>") {
		return true, stripToolPromptLeakage(stripMalformedToolPreludePrefix(RemoveMarkup(source))), nil
	}

	return false, "", nil
}

func safeUTF8SplitIndex(text string, idx int) int {
	if idx <= 0 {
		return 0
	}
	if idx >= len(text) {
		return len(text)
	}
	for idx > 0 && !utf8.ValidString(text[:idx]) {
		idx--
	}
	return idx
}

func longestMarkerPrefixSuffix(text string) int {
	if text == "" {
		return 0
	}
	lower := strings.ToLower(text)
	best := 0
	for _, marker := range startMarkers {
		markerLower := strings.ToLower(marker)
		maxLen := len(markerLower)
		if len(lower) < maxLen {
			maxLen = len(lower)
		}
		for l := maxLen; l > best; l-- {
			if strings.HasSuffix(lower, markerLower[:l]) {
				best = l
				break
			}
		}
	}
	return best
}

func stripMalformedToolPreludePrefix(text string) string {
	return strings.TrimLeft(malformedToolPreludePrefix.ReplaceAllString(text, ""), " \t\r\n")
}

func startsWithContainerMarkup(text string) bool {
	trimmed := strings.ToLower(strings.TrimLeft(text, " \t\r\n"))
	return strings.HasPrefix(trimmed, "<tool_calls") ||
		strings.HasPrefix(trimmed, "<ml_tool_calls") ||
		strings.HasPrefix(trimmed, "<tool_call") ||
		strings.HasPrefix(trimmed, "<ml_tool_call") ||
		strings.HasPrefix(trimmed, "<tool_use") ||
		strings.HasPrefix(trimmed, "<invoke")
}

func shouldFallbackMalformedCapture(captured string) bool {
	source := strings.TrimLeft(captured, " \t\r\n")
	if !startsWithContainerMarkup(source) {
		return false
	}
	firstTagEnd := strings.Index(source, ">")
	if firstTagEnd == -1 {
		return false
	}
	body := strings.TrimLeft(source[firstTagEnd+1:], " \t\r\n")
	if body == "" {
		return false
	}
	return !strings.HasPrefix(body, "<")
}

func stripToolPromptLeakage(text string) string {
	cleaned := strings.TrimSpace(text)
	cleaned = stripToolPromptLeakagePreserveWhitespace(cleaned)
	cleaned = regexp.MustCompile(`\n{3,}`).ReplaceAllString(cleaned, "\n\n")
	return strings.TrimSpace(cleaned)
}

func stripToolPromptLeakagePreserveWhitespace(text string) string {
	cleaned := text
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)既然你希望我随便调用一下工具[^。！？]*[。！？]*`),
		regexp.MustCompile(`(?i)好的，既然工具调用资源已用完[^。！？]*[。！？]*`),
		regexp.MustCompile(`(?i)工具调用资源已用完[^。！？]*[。！？]*`),
		regexp.MustCompile(`(?i)那我们就直接聊天吧[^。！？]*[。！？]*`),
		regexp.MustCompile(`(?i)由于工具调用资源已用完[^。！？]*[。！？]*`),
		regexp.MustCompile(`(?i)我无法直接访问该链接获取实时数据[^。！？]*[。！？]*`),
		regexp.MustCompile(`(?i)你好！今天有什么我可以帮你的吗？或者你想聊点什么话题？`),
		regexp.MustCompile(`(?im)^你好！\s*$`),
	}
	for _, pattern := range patterns {
		cleaned = pattern.ReplaceAllString(cleaned, "")
	}
	return cleaned
}

func CleanVisibleText(text string) string {
	cleaned := RemoveMarkup(text)
	cleaned = leadingToolTagLines.ReplaceAllString(cleaned, "")
	return stripToolPromptLeakage(strings.TrimSpace(cleaned))
}

func CleanVisibleChunk(text string) string {
	cleaned := removeMarkupPreserveWhitespace(text)
	cleaned = leadingToolTagLines.ReplaceAllString(cleaned, "")
	cleaned = stripToolPromptLeakagePreserveWhitespace(cleaned)
	return cleaned
}
