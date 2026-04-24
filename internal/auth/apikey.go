package auth

import (
	"net/http"
	"strings"
	"sync"
)

type ValidationResult struct {
	IsValid bool
	IsAdmin bool
	Key     string
}

type KeyringSnapshot struct {
	AdminKey    string
	APIKeys     []string
	RegularKeys []string
}

type Keyring struct {
	mu       sync.RWMutex
	adminKey string
	apiKeys  []string
}

func NewKeyring(apiKeys []string, adminKey string) *Keyring {
	keys := make([]string, 0, len(apiKeys))
	for _, key := range apiKeys {
		key = strings.TrimSpace(key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	if adminKey == "" && len(keys) > 0 {
		adminKey = keys[0]
	}
	return &Keyring{
		adminKey: adminKey,
		apiKeys:  keys,
	}
}

func ExtractAPIKey(r *http.Request) string {
	if key := r.Header.Get("Authorization"); key != "" {
		return key
	}
	if key := r.Header.Get("x-api-key"); key != "" {
		return key
	}
	return ""
}

func normalizeKey(raw string) string {
	clean := strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(clean), "bearer ") {
		clean = strings.TrimSpace(clean[7:])
	}
	return clean
}

func (k *Keyring) Validate(provided string) ValidationResult {
	clean := normalizeKey(provided)
	if clean == "" {
		return ValidationResult{}
	}

	k.mu.RLock()
	defer k.mu.RUnlock()
	for _, candidate := range k.apiKeys {
		if clean == candidate {
			return ValidationResult{
				IsValid: true,
				IsAdmin: clean == k.adminKey,
				Key:     clean,
			}
		}
	}
	return ValidationResult{}
}

func (k *Keyring) Snapshot() KeyringSnapshot {
	k.mu.RLock()
	defer k.mu.RUnlock()

	apiKeys := append([]string(nil), k.apiKeys...)
	regular := make([]string, 0, len(apiKeys))
	for _, key := range apiKeys {
		if key != k.adminKey {
			regular = append(regular, key)
		}
	}

	return KeyringSnapshot{
		AdminKey:    k.adminKey,
		APIKeys:     apiKeys,
		RegularKeys: regular,
	}
}

func (k *Keyring) AddRegularKey(key string) error {
	clean := normalizeKey(key)
	if clean == "" {
		return ErrInvalidAPIKey
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	for _, existing := range k.apiKeys {
		if existing == clean {
			return ErrAPIKeyExists
		}
	}
	k.apiKeys = append(k.apiKeys, clean)
	return nil
}

func (k *Keyring) DeleteRegularKey(key string) error {
	clean := normalizeKey(key)
	if clean == "" {
		return ErrInvalidAPIKey
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	if clean == k.adminKey {
		return ErrDeleteAdminKey
	}
	for i, existing := range k.apiKeys {
		if existing == clean {
			k.apiKeys = append(k.apiKeys[:i], k.apiKeys[i+1:]...)
			return nil
		}
	}
	return ErrAPIKeyNotFound
}
