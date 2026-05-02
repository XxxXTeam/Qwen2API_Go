package storage

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"qwen2api/internal/config"
)

type ConversationSession struct {
	ContextHash  string `json:"context_hash"`
	AccountEmail string `json:"account_email"`
	ChatID       string `json:"chat_id"`
	Model        string `json:"model"`
	ChatType     string `json:"chat_type"`
	UpdatedAt    int64  `json:"updated_at"`
}

type ConversationStore interface {
	GetConversationSession(contextHash string) (ConversationSession, bool, error)
	SaveConversationSession(session ConversationSession) error
	DeleteConversationSession(contextHash string) error
	ListConversationSessions() ([]ConversationSession, error)
}

type memoryConversationStore struct {
	mu       sync.RWMutex
	sessions map[string]ConversationSession
}

func NewConversationStore(cfg config.Config) (ConversationStore, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.DataSaveMode)) {
	case "", "none", "guest":
		return &memoryConversationStore{sessions: map[string]ConversationSession{}}, nil
	case "file":
		return &fileStore{path: filepathForData(cfg)}, nil
	case "redis":
		if strings.TrimSpace(cfg.RedisURL) == "" {
			return nil, errors.New("DATA_SAVE_MODE=redis 时必须提供 REDIS_URL")
		}
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return nil, err
		}
		opts.MaxRetries = 3
		opts.MinRetryBackoff = 200 * time.Millisecond
		opts.MaxRetryBackoff = 3 * time.Second
		opts.DialTimeout = 10 * time.Second
		opts.ReadTimeout = 15 * time.Second
		opts.WriteTimeout = 15 * time.Second
		opts.ConnMaxIdleTime = 45 * time.Second
		return &redisStore{client: redis.NewClient(opts)}, nil
	default:
		return nil, errors.New("不支持的数据保存模式: " + cfg.DataSaveMode)
	}
}

func filepathForData(cfg config.Config) string {
	return "data/data.json"
}

func (s *memoryConversationStore) GetConversationSession(contextHash string) (ConversationSession, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[strings.TrimSpace(contextHash)]
	return session, ok, nil
}

func (s *memoryConversationStore) SaveConversationSession(session ConversationSession) error {
	if strings.TrimSpace(session.ContextHash) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ContextHash] = session
	return nil
}

func (s *memoryConversationStore) DeleteConversationSession(contextHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, strings.TrimSpace(contextHash))
	return nil
}

func (s *memoryConversationStore) ListConversationSessions() ([]ConversationSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ConversationSession, 0, len(s.sessions))
	for _, session := range s.sessions {
		result = append(result, session)
	}
	return result, nil
}

func (s *fileStore) GetConversationSession(contextHash string) (ConversationSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return ConversationSession{}, false, err
	}
	for _, session := range data.ConversationSessions {
		if session.ContextHash == strings.TrimSpace(contextHash) {
			return session, true, nil
		}
	}
	return ConversationSession{}, false, nil
}

func (s *fileStore) SaveConversationSession(session ConversationSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return err
	}
	updated := false
	for i := range data.ConversationSessions {
		if data.ConversationSessions[i].ContextHash == session.ContextHash {
			data.ConversationSessions[i] = session
			updated = true
			break
		}
	}
	if !updated {
		data.ConversationSessions = append(data.ConversationSessions, session)
	}
	return s.write(data)
}

func (s *fileStore) DeleteConversationSession(contextHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return err
	}
	filtered := make([]ConversationSession, 0, len(data.ConversationSessions))
	for _, session := range data.ConversationSessions {
		if session.ContextHash != strings.TrimSpace(contextHash) {
			filtered = append(filtered, session)
		}
	}
	data.ConversationSessions = filtered
	return s.write(data)
}

func (s *fileStore) ListConversationSessions() ([]ConversationSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.read()
	if err != nil {
		return nil, err
	}
	return append([]ConversationSession(nil), data.ConversationSessions...), nil
}

func (s *redisStore) GetConversationSession(contextHash string) (ConversationSession, bool, error) {
	ctx, cancel := redisContext()
	defer cancel()

	raw, err := s.client.Get(ctx, "chat_session:"+strings.TrimSpace(contextHash)).Result()
	if errors.Is(err, redis.Nil) {
		return ConversationSession{}, false, nil
	}
	if err != nil {
		return ConversationSession{}, false, err
	}
	var session ConversationSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return ConversationSession{}, false, err
	}
	return session, true, nil
}

func (s *redisStore) SaveConversationSession(session ConversationSession) error {
	ctx, cancel := redisContext()
	defer cancel()
	raw, err := json.Marshal(session)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, "chat_session:"+session.ContextHash, raw, 0).Err()
}

func (s *redisStore) DeleteConversationSession(contextHash string) error {
	ctx, cancel := redisContext()
	defer cancel()
	return s.client.Del(ctx, "chat_session:"+strings.TrimSpace(contextHash)).Err()
}

func (s *redisStore) ListConversationSessions() ([]ConversationSession, error) {
	ctx, cancel := redisContext()
	defer cancel()

	var cursor uint64
	result := make([]ConversationSession, 0)
	for {
		keys, next, err := s.client.Scan(ctx, cursor, "chat_session:*", 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			raw, err := s.client.Get(ctx, key).Result()
			if err != nil {
				continue
			}
			var session ConversationSession
			if json.Unmarshal([]byte(raw), &session) == nil {
				result = append(result, session)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return result, nil
}
