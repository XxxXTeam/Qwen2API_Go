package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"qwen2api/internal/config"
)

type Account struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Token    string `json:"token"`
	Expires  int64  `json:"expires"`
}

type FileData struct {
	DefaultHeaders       any                   `json:"defaultHeaders"`
	DefaultCookie        any                   `json:"defaultCookie"`
	Accounts             []Account             `json:"accounts"`
	ConversationSessions []ConversationSession `json:"conversationSessions,omitempty"`
}

type AccountStore interface {
	LoadAccounts() ([]Account, error)
	SaveAccount(account Account) error
	DeleteAccount(email string) error
	SaveAllAccounts(accounts []Account) error
}

type fileStore struct {
	path string
	mu   sync.Mutex
}

type envStore struct {
	accounts []Account
}

type redisStore struct {
	client *redis.Client
}

func NewAccountStore(cfg config.Config) (AccountStore, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.DataSaveMode)) {
	case "", "none":
		return newEnvStoreFromCurrentEnv(), nil
	case "file":
		return &fileStore{path: filepath.Join("data", "data.json")}, nil
	case "redis":
		if strings.TrimSpace(cfg.RedisURL) == "" {
			return nil, errors.New("DATA_SAVE_MODE=redis 时必须提供 REDIS_URL")
		}
		opts, err := redis.ParseURL(cfg.RedisURL)
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
		return &redisStore{
			client: redis.NewClient(opts),
		}, nil
	default:
		return nil, errors.New("不支持的数据保存模式: " + cfg.DataSaveMode)
	}
}

func newEnvStoreFromCurrentEnv() *envStore {
	raw := strings.TrimSpace(os.Getenv("ACCOUNTS"))
	accounts := make([]Account, 0)
	if raw == "" {
		return &envStore{accounts: accounts}
	}

	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		email, password, ok := strings.Cut(item, ":")
		if !ok {
			continue
		}
		email = strings.TrimSpace(email)
		password = strings.TrimSpace(password)
		if email == "" || password == "" {
			continue
		}
		accounts = append(accounts, Account{
			Email:    email,
			Password: password,
		})
	}
	return &envStore{accounts: accounts}
}

func (s *envStore) LoadAccounts() ([]Account, error) {
	return append([]Account(nil), s.accounts...), nil
}

func (s *envStore) SaveAccount(Account) error {
	return errors.New("环境变量模式不支持保存账户数据")
}

func (s *envStore) DeleteAccount(string) error {
	return errors.New("环境变量模式不支持删除账户数据")
}

func (s *envStore) SaveAllAccounts([]Account) error {
	return errors.New("环境变量模式不支持批量保存账户数据")
}

func (s *fileStore) LoadAccounts() ([]Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return nil, err
	}
	return append([]Account(nil), data.Accounts...), nil
}

func (s *fileStore) SaveAccount(account Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return err
	}
	updated := false
	for i := range data.Accounts {
		if strings.EqualFold(data.Accounts[i].Email, account.Email) {
			data.Accounts[i] = account
			updated = true
			break
		}
	}
	if !updated {
		data.Accounts = append(data.Accounts, account)
	}
	return s.write(data)
}

func (s *fileStore) DeleteAccount(email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return err
	}
	filtered := make([]Account, 0, len(data.Accounts))
	for _, account := range data.Accounts {
		if !strings.EqualFold(account.Email, email) {
			filtered = append(filtered, account)
		}
	}
	data.Accounts = filtered
	return s.write(data)
}

func (s *fileStore) SaveAllAccounts(accounts []Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return err
	}
	data.Accounts = append([]Account(nil), accounts...)
	return s.write(data)
}

func (s *fileStore) read() (FileData, error) {
	if err := s.ensure(); err != nil {
		return FileData{}, err
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return FileData{}, err
	}
	var data FileData
	if err := json.Unmarshal(raw, &data); err != nil {
		return FileData{}, err
	}
	return data, nil
}

func (s *fileStore) write(data FileData) error {
	if err := s.ensure(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0644)
}

func (s *fileStore) ensure() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(s.path); err == nil {
		return nil
	}
	defaultData := FileData{
		Accounts: []Account{},
	}
	raw, err := json.MarshalIndent(defaultData, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, raw, 0644)
}

func (s *redisStore) LoadAccounts() ([]Account, error) {
	ctx, cancel := redisContext()
	defer cancel()

	keys, err := s.scanUserKeys(ctx)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return []Account{}, nil
	}

	pipe := s.client.Pipeline()
	cmds := make([]*redis.MapStringStringCmd, 0, len(keys))
	for _, key := range keys {
		cmds = append(cmds, pipe.HGetAll(ctx, key))
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	accounts := make([]Account, 0, len(keys))
	for i, cmd := range cmds {
		values, err := cmd.Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}
		email := strings.TrimPrefix(keys[i], "user:")
		account := Account{
			Email:    email,
			Password: values["password"],
			Token:    values["token"],
		}
		if values["expires"] != "" {
			if parsed, parseErr := time.Parse(time.RFC3339Nano, values["expires"]); parseErr == nil {
				account.Expires = parsed.Unix()
			}
		}
		if account.Expires == 0 && values["expires_unix"] != "" {
			if unixValue, parseErr := strconv.ParseInt(values["expires_unix"], 10, 64); parseErr == nil {
				account.Expires = unixValue
			}
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

func (s *redisStore) SaveAccount(account Account) error {
	ctx, cancel := redisContext()
	defer cancel()

	values := map[string]any{
		"password":     account.Password,
		"token":        account.Token,
		"expires_unix": account.Expires,
	}
	if account.Expires > 0 {
		values["expires"] = time.Unix(account.Expires, 0).UTC().Format(time.RFC3339Nano)
	} else {
		values["expires"] = ""
	}
	return s.client.HSet(ctx, "user:"+account.Email, values).Err()
}

func (s *redisStore) DeleteAccount(email string) error {
	ctx, cancel := redisContext()
	defer cancel()
	return s.client.Del(ctx, "user:"+email).Err()
}

func (s *redisStore) SaveAllAccounts(accounts []Account) error {
	ctx, cancel := redisContext()
	defer cancel()

	keys, err := s.scanUserKeys(ctx)
	if err != nil {
		return err
	}

	pipe := s.client.TxPipeline()
	for _, key := range keys {
		pipe.Del(ctx, key)
	}
	for _, account := range accounts {
		values := map[string]any{
			"password":     account.Password,
			"token":        account.Token,
			"expires_unix": account.Expires,
		}
		if account.Expires > 0 {
			values["expires"] = time.Unix(account.Expires, 0).UTC().Format(time.RFC3339Nano)
		} else {
			values["expires"] = ""
		}
		pipe.HSet(ctx, "user:"+account.Email, values)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (s *redisStore) scanUserKeys(ctx context.Context) ([]string, error) {
	var cursor uint64
	keys := make([]string, 0)
	for {
		batch, nextCursor, err := s.client.Scan(ctx, cursor, "user:*", 100).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return keys, nil
}

func redisContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 20*time.Second)
}
