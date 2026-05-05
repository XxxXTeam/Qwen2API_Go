package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ChatTracker records which chats were created/used by the program per account.
// Used for cleanup mode 1 (delete only program-created chats).
type ChatTracker interface {
	RecordChatUsage(accountEmail, chatID string) error
	ListChatUsages() ([]ChatUsage, error)
	DeleteChatUsage(accountEmail, chatID string) error
}

type ChatUsage struct {
	AccountEmail string `json:"account_email"`
	ChatID       string `json:"chat_id"`
	UpdatedAt    int64  `json:"updated_at"`
}

// NewChatTracker creates a ChatTracker. If redisURL is provided, uses Redis;
// otherwise falls back to an in-memory map.
func NewChatTracker(redisURL string) (ChatTracker, error) {
	if strings.TrimSpace(redisURL) != "" {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			return nil, fmt.Errorf("解析 REDIS_URL 失败: %w", err)
		}
		opts.MaxRetries = 3
		opts.MinRetryBackoff = 200 * time.Millisecond
		opts.MaxRetryBackoff = 3 * time.Second
		opts.DialTimeout = 10 * time.Second
		opts.ReadTimeout = 15 * time.Second
		opts.WriteTimeout = 15 * time.Second
		opts.ConnMaxIdleTime = 45 * time.Second
		return &redisChatTracker{client: redis.NewClient(opts)}, nil
	}
	return &memoryChatTracker{usages: map[string]int64{}}, nil
}

type memoryChatTracker struct {
	mu     sync.RWMutex
	usages map[string]int64 // key: "accountEmail|chatID" -> updatedAt unix seconds
}

func (t *memoryChatTracker) RecordChatUsage(accountEmail, chatID string) error {
	key := chatTrackerKey(accountEmail, chatID)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.usages[key] = time.Now().Unix()
	return nil
}

func (t *memoryChatTracker) ListChatUsages() ([]ChatUsage, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]ChatUsage, 0, len(t.usages))
	for key, updatedAt := range t.usages {
		accountEmail, chatID, ok := strings.Cut(key, "|")
		if !ok {
			continue
		}
		result = append(result, ChatUsage{
			AccountEmail: accountEmail,
			ChatID:       chatID,
			UpdatedAt:    updatedAt,
		})
	}
	return result, nil
}

func (t *memoryChatTracker) DeleteChatUsage(accountEmail, chatID string) error {
	key := chatTrackerKey(accountEmail, chatID)
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.usages, key)
	return nil
}

type redisChatTracker struct {
	client *redis.Client
}

func (t *redisChatTracker) RecordChatUsage(accountEmail, chatID string) error {
	ctx, cancel := redisContext()
	defer cancel()
	key := redisChatTrackerKey(accountEmail, chatID)
	value := map[string]any{
		"account_email": accountEmail,
		"chat_id":       chatID,
		"updated_at":    time.Now().Unix(),
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return t.client.Set(ctx, key, raw, 48*time.Hour).Err()
}

func (t *redisChatTracker) ListChatUsages() ([]ChatUsage, error) {
	ctx, cancel := redisContext()
	defer cancel()

	var cursor uint64
	result := make([]ChatUsage, 0)
	for {
		keys, next, err := t.client.Scan(ctx, cursor, "qwen2api:chat_usage:*", 100).Result()
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			raw, err := t.client.Get(ctx, key).Result()
			if errors.Is(err, redis.Nil) {
				continue
			}
			if err != nil {
				continue
			}
			var usage ChatUsage
			if json.Unmarshal([]byte(raw), &usage) == nil {
				result = append(result, usage)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return result, nil
}

func (t *redisChatTracker) DeleteChatUsage(accountEmail, chatID string) error {
	ctx, cancel := redisContext()
	defer cancel()
	return t.client.Del(ctx, redisChatTrackerKey(accountEmail, chatID)).Err()
}

func chatTrackerKey(accountEmail, chatID string) string {
	return accountEmail + "|" + chatID
}

func redisChatTrackerKey(accountEmail, chatID string) string {
	return "qwen2api:chat_usage:" + accountEmail + ":" + chatID
}
