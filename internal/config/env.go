package config

import (
	"bufio"
	"os"
	"strings"
)

const defaultDotEnvTemplate = `# Qwen2API_Go default configuration
# First API key is treated as admin key by default.
API_KEY=sk-admin-change-me,sk-user-change-me

# Account source:
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
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

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
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}
