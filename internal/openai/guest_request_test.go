package openai

import (
	"testing"

	"qwen2api/internal/storage"
)

func TestBuildChatRequestBodyForGuestMatchesGuestProtocolShape(t *testing.T) {
	session := storage.Account{Email: "guest", Source: storage.AccountSourceGuest}
	body := buildChatRequestBody(session, "qwen3.6-plus", "chat-123", "t2t", []map[string]any{
		{
			"role":           "user",
			"content":        "你好",
			"chat_type":      "t2t",
			"feature_config": map[string]any{"thinking_enabled": true},
			"extra":          map[string]any{},
		},
	})

	if got := body["chat_mode"]; got != "guest" {
		t.Fatalf("chat_mode = %v, want guest", got)
	}
	if got := body["version"]; got != "2.1" {
		t.Fatalf("version = %v, want 2.1", got)
	}
	if _, exists := body["session_id"]; exists {
		t.Fatalf("did not expect session_id in guest body")
	}
	if _, exists := body["id"]; exists {
		t.Fatalf("did not expect id in guest body")
	}

	messages, ok := body["messages"].([]map[string]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("messages = %#v", body["messages"])
	}
	message := messages[0]
	if message["user_action"] != "chat" {
		t.Fatalf("user_action = %v, want chat", message["user_action"])
	}
	if message["sub_chat_type"] != "t2t" {
		t.Fatalf("sub_chat_type = %v, want t2t", message["sub_chat_type"])
	}
	featureConfig, _ := message["feature_config"].(map[string]any)
	if featureConfig == nil {
		t.Fatal("expected feature_config")
	}
	if featureConfig["thinking_enabled"] != true {
		t.Fatalf("thinking_enabled = %v, want true", featureConfig["thinking_enabled"])
	}
	if featureConfig["auto_thinking"] != true {
		t.Fatalf("auto_thinking = %v, want true", featureConfig["auto_thinking"])
	}
	if featureConfig["thinking_mode"] != "Auto" {
		t.Fatalf("thinking_mode = %v, want Auto", featureConfig["thinking_mode"])
	}
	if featureConfig["thinking_format"] != "summary" {
		t.Fatalf("thinking_format = %v, want summary", featureConfig["thinking_format"])
	}
	extra, _ := message["extra"].(map[string]any)
	meta, _ := extra["meta"].(map[string]any)
	if meta["subChatType"] != "t2t" {
		t.Fatalf("subChatType = %v, want t2t", meta["subChatType"])
	}
}

func TestBuildChatRequestBodyForRegularAccountKeepsLegacyProtocol(t *testing.T) {
	session := storage.Account{Email: "user@example.com", Token: "token-123"}
	body := buildChatRequestBody(session, "qwen3.6-plus", "chat-123", "t2t", []map[string]any{
		{"role": "user", "content": "你好"},
	})

	if got := body["chat_mode"]; got != "normal" {
		t.Fatalf("chat_mode = %v, want normal", got)
	}
	if _, exists := body["version"]; exists {
		t.Fatalf("did not expect version in regular body")
	}
	if _, exists := body["session_id"]; !exists {
		t.Fatalf("expected session_id in regular body")
	}
	if _, exists := body["id"]; !exists {
		t.Fatalf("expected id in regular body")
	}
}
