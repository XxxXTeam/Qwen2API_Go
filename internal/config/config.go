package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DataSaveMode          string
	APIKeys               []string
	AdminKey              string
	BatchLoginConcurrency int
	SimpleModelMap        bool
	ListenAddress         string
	ListenPort            int
	SearchInfoMode        string
	OutThink              bool
	RedisURL              string
	AutoRefresh           bool
	AutoRefreshInterval   int
	CacheMode             string
	LogLevel              string
	DebugMode             bool
	EnableFileLog         bool
	LogDir                string
	MaxLogFileSize        int
	MaxLogFiles           int
	QwenChatProxyURL      string
	ProxyURL              string
}

func Load() Config {
	apiKeys := parseAPIKeys(os.Getenv("API_KEY"))
	adminKey := ""
	if len(apiKeys) > 0 {
		adminKey = apiKeys[0]
	}

	return Config{
		DataSaveMode:          getEnv("DATA_SAVE_MODE", "none"),
		APIKeys:               apiKeys,
		AdminKey:              adminKey,
		BatchLoginConcurrency: getEnvInt("BATCH_LOGIN_CONCURRENCY", 5),
		SimpleModelMap:        getEnvBool("SIMPLE_MODEL_MAP", false),
		ListenAddress:         os.Getenv("LISTEN_ADDRESS"),
		ListenPort:            getEnvInt("SERVICE_PORT", 3000),
		SearchInfoMode:        parseSearchInfoMode(os.Getenv("SEARCH_INFO_MODE")),
		OutThink:              getEnvBool("OUTPUT_THINK", false),
		RedisURL:              os.Getenv("REDIS_URL"),
		AutoRefresh:           getEnvBool("AUTO_REFRESH", true),
		AutoRefreshInterval:   getEnvInt("AUTO_REFRESH_INTERVAL", 6*60*60),
		CacheMode:             getEnv("CACHE_MODE", "default"),
		LogLevel:              getEnv("LOG_LEVEL", "INFO"),
		DebugMode:             getEnvBool("DEBUG_MODE", false),
		EnableFileLog:         getEnvBool("ENABLE_FILE_LOG", false),
		LogDir:                getEnv("LOG_DIR", "./logs"),
		MaxLogFileSize:        getEnvInt("MAX_LOG_FILE_SIZE", 10),
		MaxLogFiles:           getEnvInt("MAX_LOG_FILES", 5),
		QwenChatProxyURL:      getEnv("QWEN_CHAT_PROXY_URL", "https://chat.qwen.ai"),
		ProxyURL:              os.Getenv("PROXY_URL"),
	}
}

func (c Config) ListenAddressOrDefault() string {
	if strings.TrimSpace(c.ListenAddress) == "" {
		return "0.0.0.0"
	}
	return c.ListenAddress
}

func parseAPIKeys(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	keys := make([]string, 0, len(parts))
	for _, part := range parts {
		key := strings.TrimSpace(part)
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func parseSearchInfoMode(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "table") {
		return "table"
	}
	return "text"
}

func getEnv(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
