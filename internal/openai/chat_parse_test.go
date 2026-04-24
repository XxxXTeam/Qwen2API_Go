package openai

import "testing"

func TestParseChatCompletionContentSupportsDirectJSONMessage(t *testing.T) {
	raw := []byte(`{
		"choices": [
			{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "hello from json"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 12,
			"completion_tokens": 4,
			"total_tokens": 16
		}
	}`)

	content, prompt, completion, total := parseChatCompletionContent(raw)
	if content != "hello from json" {
		t.Fatalf("content = %q, want %q", content, "hello from json")
	}
	if prompt != 12 || completion != 4 || total != 16 {
		t.Fatalf("usage = (%d,%d,%d), want (12,4,16)", prompt, completion, total)
	}
}

func TestParseChatCompletionContentSupportsSSEDelta(t *testing.T) {
	raw := []byte("data: {\"choices\":[{\"delta\":{\"phase\":\"think\",\"content\":\"first\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"phase\":\"answer\",\"content\":\"second\"}}]}\n\n")

	content, _, _, _ := parseChatCompletionContent(raw)
	want := "<think>\n\nfirst\n\n</think>\nsecond"
	if content != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}

func TestParseChatCompletionContentSupportsNestedJSONContent(t *testing.T) {
	raw := []byte(`{
		"data": {
			"message": {
				"content": [
					{"text": "hello from nested payload"}
				]
			}
		}
	}`)

	content, _, _, _ := parseChatCompletionContent(raw)
	if content != "hello from nested payload" {
		t.Fatalf("content = %q, want %q", content, "hello from nested payload")
	}
}

func TestParseChatCompletionContentSupportsThinkingSummaryPhase(t *testing.T) {
	raw := []byte("data: {\"choices\":[{\"delta\":{\"phase\":\"thinking_summary\",\"content\":\"\",\"extra\":{\"summary_title\":{\"content\":[\"回应用户的问候并主动提供帮助\"]},\"summary_thought\":{\"content\":[\"我感知到用户重复发送了简单的问候。\",\"我希望能为用户提供更有价值的协助。\"]}}}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"phase\":\"answer\",\"content\":\"你好\"}}]}\n\n")

	content, _, _, _ := parseChatCompletionContent(raw)
	want := "<think>\n\n回应用户的问候并主动提供帮助\n我感知到用户重复发送了简单的问候。\n我希望能为用户提供更有价值的协助。\n\n</think>\n你好"
	if content != want {
		t.Fatalf("content = %q, want %q", content, want)
	}
}
