package openai

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"qwen2api/internal/logging"
	"qwen2api/internal/storage"
)

const conversationSessionTTL = 24 * time.Hour

type ConversationSessionService struct {
	store  storage.ConversationStore
	logger *logging.Logger
}

type preparedChatRequest struct {
	Model                string
	ChatType             string
	ThinkingEnabled      bool
	ExpandedMessages     []map[string]any
	FullUpstreamMessages []map[string]any
	LastUpstreamMessages []map[string]any
	ContextHash          string
	ToolNames            []string
}

func NewConversationSessionService(store storage.ConversationStore, logger *logging.Logger) *ConversationSessionService {
	return &ConversationSessionService{store: store, logger: logger}
}

func (s *ConversationSessionService) Get(contextHash string) (storage.ConversationSession, bool) {
	if s == nil || s.store == nil || strings.TrimSpace(contextHash) == "" {
		return storage.ConversationSession{}, false
	}
	s.cleanupExpired()
	session, ok, err := s.store.GetConversationSession(contextHash)
	if err != nil {
		if s.logger != nil {
			s.logger.WarnModule("OPENAI", "load conversation session failed hash=%s err=%v", contextHash, err)
		}
		return storage.ConversationSession{}, false
	}
	if !ok {
		return storage.ConversationSession{}, false
	}
	if session.UpdatedAt > 0 && time.Since(time.UnixMilli(session.UpdatedAt)) > conversationSessionTTL {
		_ = s.store.DeleteConversationSession(contextHash)
		return storage.ConversationSession{}, false
	}
	return session, true
}

func (s *ConversationSessionService) Save(contextHash, accountEmail, chatID, model, chatType string) {
	if s == nil || s.store == nil || strings.TrimSpace(contextHash) == "" || strings.TrimSpace(accountEmail) == "" || strings.TrimSpace(chatID) == "" {
		return
	}
	session := storage.ConversationSession{
		ContextHash:  contextHash,
		AccountEmail: accountEmail,
		ChatID:       chatID,
		Model:        model,
		ChatType:     chatType,
		UpdatedAt:    time.Now().UnixMilli(),
	}
	if err := s.store.SaveConversationSession(session); err != nil && s.logger != nil {
		s.logger.WarnModule("OPENAI", "save conversation session failed hash=%s account=%s chat_id=%s err=%v", contextHash, accountEmail, chatID, err)
	}
}

func (s *ConversationSessionService) Delete(contextHash string) {
	if s == nil || s.store == nil || strings.TrimSpace(contextHash) == "" {
		return
	}
	if err := s.store.DeleteConversationSession(contextHash); err != nil && s.logger != nil {
		s.logger.WarnModule("OPENAI", "delete conversation session failed hash=%s err=%v", contextHash, err)
	}
}

func (s *ConversationSessionService) cleanupExpired() {
	if s == nil || s.store == nil {
		return
	}
	sessions, err := s.store.ListConversationSessions()
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-conversationSessionTTL).UnixMilli()
	for _, session := range sessions {
		if session.UpdatedAt > 0 && session.UpdatedAt < cutoff {
			_ = s.store.DeleteConversationSession(session.ContextHash)
		}
	}
}

func computeContextHash(model, chatType string, toolNames []string, expandedMessages []map[string]any) string {
	if len(expandedMessages) <= 1 {
		return ""
	}
	payload := map[string]any{
		"model":      strings.TrimSpace(model),
		"chat_type":  strings.TrimSpace(chatType),
		"tool_names": append([]string(nil), toolNames...),
		"messages":   expandedMessages[:len(expandedMessages)-1],
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
