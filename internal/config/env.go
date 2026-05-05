package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

const DefaultEnvPath = ".env"

const defaultDotEnvTemplate = `# Qwen2API_Go default configuration
# First API key is treated as admin key by default.
API_KEY=sk-admin-change-me,sk-user-change-me

# Account source:
# guest = always use anonymous guest cookies
# none  = read ACCOUNTS only, no persistence
# file  = persist accounts to data/data.json
# redis = persist accounts to Redis via REDIS_URL
DATA_SAVE_MODE=file

# Optional preload accounts, format:
# email:password,email:password
ACCOUNTS=

# Service listen settings
LISTEN_ADDRESS=0.0.0.0
SERVICE_PORT=3000

# Upstream endpoint
QWEN_CHAT_PROXY_URL=https://chat.qwen.ai

# Optional outbound proxy
# PROXY_URL=http://127.0.0.1:7890
PROXY_URL=

# Redis URL, used only when DATA_SAVE_MODE=redis
# REDIS_URL=redis://127.0.0.1:6379/0
REDIS_URL=

# Runtime behavior
AUTO_REFRESH=true
AUTO_REFRESH_INTERVAL=21600
BATCH_LOGIN_CONCURRENCY=5
SIMPLE_MODEL_MAP=false
SEARCH_INFO_MODE=text
OUTPUT_THINK=false

# Logging
LOG_LEVEL=INFO
DEBUG_MODE=false
ENABLE_FILE_LOG=false
LOG_DIR=./logs
MAX_LOG_FILE_SIZE=10
MAX_LOG_FILES=5

# Chat cleanup mode
# 0 = do not delete chats
# 1 = delete only chats created by this program
# 2 = delete all chats older than 1 day
CHAT_CLEANUP_MODE=0

# Cache/runtime flags
CACHE_MODE=default
`

func EnsureDotEnv(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(defaultDotEnvTemplate), 0644)
}

func LoadDotEnv(path string) {
	values, err := ParseDotEnv(path)
	if err != nil {
		return
	}
	applyEnvMap(values, false)
}

func ReloadDotEnv(path string) error {
	values, err := ParseDotEnv(path)
	if err != nil {
		return err
	}
	applyEnvMap(values, true)
	return nil
}

func applyEnvMap(values map[string]string, override bool) {
	for key, value := range values {
		if !override {
			if _, exists := os.LookupEnv(key); exists {
				continue
			}
		}
		_ = os.Setenv(key, value)
	}
}

func RuntimeSnapshotFromConfig(cfg Config) RuntimeSnapshot {
	return RuntimeSnapshot{
		BatchLoginConcurrency: cfg.BatchLoginConcurrency,
		AutoRefresh:           cfg.AutoRefresh,
		AutoRefreshInterval:   cfg.AutoRefreshInterval,
		OutThink:              cfg.OutThink,
		SearchInfoMode:        cfg.SearchInfoMode,
		SimpleModelMap:        cfg.SimpleModelMap,
		ChatCleanupMode:       cfg.ChatCleanupMode,
	}
}

func ParseDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func SaveDotEnvValues(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(raw), "\n")
	seen := make(map[string]bool, len(updates))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value, exists := updates[key]
		if !exists {
			continue
		}
		lines[i] = key + "=" + value
		seen[key] = true
	}

	for key, value := range updates {
		if seen[key] {
			continue
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, key+"="+value)
	}

	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func RuntimeSnapshotToEnv(snapshot RuntimeSnapshot) map[string]string {
	return map[string]string{
		"AUTO_REFRESH":            strconv.FormatBool(snapshot.AutoRefresh),
		"AUTO_REFRESH_INTERVAL":   strconv.Itoa(snapshot.AutoRefreshInterval),
		"BATCH_LOGIN_CONCURRENCY": strconv.Itoa(snapshot.BatchLoginConcurrency),
		"SIMPLE_MODEL_MAP":        strconv.FormatBool(snapshot.SimpleModelMap),
		"SEARCH_INFO_MODE":        snapshot.SearchInfoMode,
		"OUTPUT_THINK":            strconv.FormatBool(snapshot.OutThink),
		"CHAT_CLEANUP_MODE":       strconv.Itoa(snapshot.ChatCleanupMode),
	}
}
