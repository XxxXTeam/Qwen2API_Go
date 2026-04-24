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
