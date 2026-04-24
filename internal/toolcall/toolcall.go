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
	xmlToolCallsBlock = regexp.MustCompile(`(?is)<(?:ml_tool_calls|tool_calls)[^>]*>(.*?)</(?:ml_tool_calls|tool_calls)>`)
	xmlToolCallBlock  = regexp.MustCompile(`(?is)<(?:ml_tool_call|tool_call)[^>]*>(.*?)</(?:ml_tool_call|tool_call)>`)
	xmlToolNameBlock  = regexp.MustCompile(`(?is)<(?:ml_tool_name|tool_name)>(.*?)</(?:ml_tool_name|tool_name)>`)
	xmlParameters     = regexp.MustCompile(`(?is)<(?:ml_parameters|parameters)>(.*?)</(?:ml_parameters|parameters)>`)
	xmlParameterItem  = regexp.MustCompile(`(?is)<([a-zA-Z_][\w.:-]*)>(.*?)</([a-zA-Z_][\w.:-]*)>`)
	xmlCDATA          = regexp.MustCompile(`(?is)<!\[CDATA\[(.*?)\]\]>`)
	xmlNoiseBlock     = regexp.MustCompile(`(?is)<(?:ml_tool_calls|ml_tool_call|ml_tool_result|tool_calls|tool_call|tool_result|function_call|invoke|tool_use)[^>]*>.*?</(?:ml_tool_calls|ml_tool_call|ml_tool_result|tool_calls|tool_call|tool_result|function_call|invoke|tool_use)>`)
	startMarkers      = []string{"<ml_tool_calls", "<ml_tool_call", "<tool_calls", "<tool_call"}
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
				Messages:  normalizedMessages,
				ToolNames: toolNames,
				Policy:    policy,
			}
		}
	}

	return InjectionResult{
		Messages:  append([]map[string]any{{"role": "system", "content": toolPrompt}}, normalizedMessages...),
		ToolNames: toolNames,
		Policy:    policy,
	}
}

func buildInstructions(toolNames []string, policy ToolChoicePolicy) string {
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
		"Never output role=\"function\" or function_call JSON.",
		"When you decide to use a tool, respond with XML only and no extra prose.",
		"",
		"Available tool names:",
		strings.Join(toolNames, ", "),
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
	return strings.TrimSpace(cleaned)
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
		return StreamChunkResult{}
	}

	if idx := firstMarkerIndex(state.pending); idx >= 0 {
		safe := state.pending[:idx]
		state.capturing = true
		state.captureBuff = state.pending[idx:]
		state.pending = ""
		return StreamChunkResult{Content: safe}
	}

	maxMarkerLen := 0
	for _, marker := range startMarkers {
		if len(marker) > maxMarkerLen {
			maxMarkerLen = len(marker)
		}
	}
	if len(state.pending) > maxMarkerLen {
		splitIndex := safeUTF8SplitIndex(state.pending, len(state.pending)-maxMarkerLen)
		safe := state.pending[:splitIndex]
		state.pending = state.pending[splitIndex:]
		return StreamChunkResult{Content: safe}
	}
	return StreamChunkResult{}
}

func FinalizeStream(state *StreamState) StreamChunkResult {
	if state.capturing {
		if ready, content, calls := tryConsumeCapture(state.captureBuff + state.pending); ready {
			return StreamChunkResult{Content: content, ToolCalls: calls}
		}
		return StreamChunkResult{Content: RemoveMarkup(state.captureBuff + state.pending)}
	}

	calls := ParseCalls(state.pending)
	if len(calls) > 0 {
		return StreamChunkResult{
			Content:   RemoveMarkup(state.pending),
			ToolCalls: calls,
		}
	}
	return StreamChunkResult{Content: state.pending}
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
	calls := ParseCalls(captured)
	if len(calls) == 0 {
		lower := strings.ToLower(captured)
		if strings.Contains(lower, "</ml_tool_calls>") || strings.Contains(lower, "</tool_calls>") || strings.Contains(lower, "</ml_tool_call>") || strings.Contains(lower, "</tool_call>") {
			return true, RemoveMarkup(captured), nil
		}
		return false, "", nil
	}
	return true, RemoveMarkup(captured), calls
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
