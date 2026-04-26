package toolcall

import (
	"strings"
	"testing"
)

func TestProcessStreamChunkKeepsUTF8Boundary(t *testing.T) {
	state := NewStreamState()

	first := ProcessStreamChunk(state, "你好，世界")
	second := FinalizeStream(state)

	got := first.Content + second.Content
	want := "你好，世界"
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestFinalizeStreamRemovesResidualClosingToolTags(t *testing.T) {
	state := NewStreamState()

	ProcessStreamChunk(state, "<ml_tool_calls>")
	result := FinalizeStream(state)

	if result.Content != "" {
		t.Fatalf("content = %q, want empty", result.Content)
	}
	if len(result.ToolCalls) != 0 {
		t.Fatalf("tool calls len = %d, want 0", len(result.ToolCalls))
	}
}

func TestRemoveMarkupRemovesResidualToolTagsFromMixedContent(t *testing.T) {
	input := "</ml_tool_calls>\n\n这是正常回答内容"
	got := RemoveMarkup(input)
	want := "这是正常回答内容"
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestProcessStreamChunkFallsBackWhenMalformedToolPreludeFollowedByAnswer(t *testing.T) {
	state := NewStreamState()

	first := ProcessStreamChunk(state, "<")
	if first.Content != "" || len(first.ToolCalls) != 0 {
		t.Fatalf("first = %+v, want empty", first)
	}

	second := ProcessStreamChunk(state, "ml_tool_calls>\n  ")
	if second.Content != "" || len(second.ToolCalls) != 0 {
		t.Fatalf("second = %+v, want empty", second)
	}

	third := ProcessStreamChunk(state, "你好，继续正常回答")
	if third.Content != "你好，继续正常回答" {
		t.Fatalf("third content = %q, want %q", third.Content, "你好，继续正常回答")
	}
	if len(third.ToolCalls) != 0 {
		t.Fatalf("third tool calls len = %d, want 0", len(third.ToolCalls))
	}

	final := FinalizeStream(state)
	if final.Content != "" || len(final.ToolCalls) != 0 {
		t.Fatalf("final = %+v, want empty", final)
	}
}

func TestFinalizeStreamStripsToolPromptLeakageNoise(t *testing.T) {
	state := NewStreamState()
	state.pending = "你好！既然你希望我随便调用一下工具，那我就用 Python 执行一个简单的计算吧。\n\n真正答案"

	final := FinalizeStream(state)
	if final.Content != "真正答案" {
		t.Fatalf("content = %q, want %q", final.Content, "真正答案")
	}
}

func TestCleanVisibleTextRemovesResidualClosingTags(t *testing.T) {
	input := "</ml_tool_calls>\n\n以下是查询结果"
	got := CleanVisibleText(input)
	want := "以下是查询结果"
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestCleanVisibleTextRemovesExactLeakedPrefixFromRealCase(t *testing.T) {
	input := "</ml_tool_calls>\n\n访问https://opendata.baidu.com/api.php?query=1.1.1.1&co=&resource_id=6006&oe=utf8 的结果如下："
	got := CleanVisibleText(input)
	want := "访问https://opendata.baidu.com/api.php?query=1.1.1.1&co=&resource_id=6006&oe=utf8 的结果如下："
	if got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestProcessStreamChunkDoesNotLeakSplitClosingWrapperAfterValidToolCall(t *testing.T) {
	state := NewStreamState()

	chunks := []string{
		"<ml_tool_calls>\n  <ml_tool_call>\n    <ml_tool_name>mcp__CherryFetch__fetchJson</ml_tool_name>\n    <ml_parameters>\n      <url><![CDATA[https://opendata.baidu.com/api.php?query=1.1.1.1&co=&resource_id=6006&oe=utf8]]></url>\n    </ml_parameters>\n  </ml_tool_call>\n</ml",
		"_tool_calls>",
	}

	var combinedContent strings.Builder
	var combinedCalls []ToolCall
	for _, chunk := range chunks {
		result := ProcessStreamChunk(state, chunk)
		combinedContent.WriteString(result.Content)
		combinedCalls = append(combinedCalls, result.ToolCalls...)
	}
	final := FinalizeStream(state)
	combinedContent.WriteString(final.Content)
	combinedCalls = append(combinedCalls, final.ToolCalls...)

	if combinedContent.String() != "" {
		t.Fatalf("content = %q, want empty", combinedContent.String())
	}
	if len(combinedCalls) != 1 {
		t.Fatalf("tool calls len = %d, want 1", len(combinedCalls))
	}
	if combinedCalls[0].Name != "mcp__CherryFetch__fetchJson" {
		t.Fatalf("tool name = %q", combinedCalls[0].Name)
	}
}

func TestProcessStreamChunkDoesNotFragmentPlainTextWhenToolsEnabled(t *testing.T) {
	state := NewStreamState()

	first := ProcessStreamChunk(state, "查询")
	if first.Content != "查询" {
		t.Fatalf("first content = %q, want %q", first.Content, "查询")
	}
	if len(first.ToolCalls) != 0 {
		t.Fatalf("first tool calls len = %d, want 0", len(first.ToolCalls))
	}

	second := ProcessStreamChunk(state, "结果如下：\n\n")
	if second.Content != "结果如下：\n\n" {
		t.Fatalf("second content = %q, want %q", second.Content, "结果如下：\n\n")
	}
	if len(second.ToolCalls) != 0 {
		t.Fatalf("second tool calls len = %d, want 0", len(second.ToolCalls))
	}

	final := FinalizeStream(state)
	if final.Content != "" {
		t.Fatalf("final content = %q, want empty", final.Content)
	}
	if len(final.ToolCalls) != 0 {
		t.Fatalf("final tool calls len = %d, want 0", len(final.ToolCalls))
	}
}

func TestProcessStreamChunkStillCapturesSplitOpeningToolMarker(t *testing.T) {
	state := NewStreamState()

	first := ProcessStreamChunk(state, "<")
	if first.Content != "" || len(first.ToolCalls) != 0 {
		t.Fatalf("first = %+v, want empty", first)
	}

	second := ProcessStreamChunk(state, "ml_tool_calls>")
	if second.Content != "" || len(second.ToolCalls) != 0 {
		t.Fatalf("second = %+v, want empty", second)
	}

	if !state.capturing {
		t.Fatal("expected capturing=true after split marker")
	}
}

func TestBuildInstructionsMatchesStrictJSGuardrails(t *testing.T) {
	text := buildInstructions([]string{"fetch_json"}, ToolChoicePolicy{Enabled: true, Mode: "auto"})
	for _, snippet := range []string{
		"Never output the legacy tags <tool_calls>, <tool_call>, <tool_name>, <parameters>, or any other non-ml tag.",
		"Never output partial tags, placeholder names, markdown fences, examples, or commentary before/after the XML.",
		"If you are not calling a tool, do not mention XML or tools. Answer normally.",
		"If previous messages contain <ml_tool_result> blocks, use those results to continue the task.",
	} {
		if !strings.Contains(text, snippet) {
			t.Fatalf("instructions missing %q\n%s", snippet, text)
		}
	}
}

func TestInjectPromptAppendsReminderToLatestMessage(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "你是一个助手"},
		{"role": "user", "content": "第一轮问题"},
		{"role": "assistant", "content": "第一轮回答"},
		{"role": "user", "content": "请继续处理"},
	}
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "fetch_json",
				"description": "fetch data",
				"parameters": map[string]any{
					"type": "object",
				},
			},
		},
	}

	result := InjectPrompt(messages, tools, "auto")
	if len(result.Messages) != 4 {
		t.Fatalf("messages len = %d, want 4", len(result.Messages))
	}

	lastContent := normalizeMessageTextContent(result.Messages[len(result.Messages)-1]["content"])
	for _, snippet := range []string{
		"[ml_tool reminder]",
		"Allowed ml_tool names: fetch_json.",
		"Ignore built-in/native/platform tools.",
	} {
		if !strings.Contains(lastContent, snippet) {
			t.Fatalf("latest message missing %q\n%s", snippet, lastContent)
		}
	}
}

func TestCleanVisibleChunkPreservesIndentedJSONLine(t *testing.T) {
	input := "\n      \"t"
	got := CleanVisibleChunk(input)
	if got != input {
		t.Fatalf("content = %q, want %q", got, input)
	}
}

func TestCleanVisibleChunkPreservesCodeFenceAndBracketWhitespace(t *testing.T) {
	input := "}\n```\n\n"
	got := CleanVisibleChunk(input)
	if got != input {
		t.Fatalf("content = %q, want %q", got, input)
	}

	input2 := "\n  ]\n"
	got2 := CleanVisibleChunk(input2)
	if got2 != input2 {
		t.Fatalf("content = %q, want %q", got2, input2)
	}
}
