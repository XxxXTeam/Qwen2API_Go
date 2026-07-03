package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"qwen2api/internal/config"
	"qwen2api/internal/logging"
	"qwen2api/internal/qwen"
	"qwen2api/internal/storage"
)

type stubAccountStore struct {
	accounts        []storage.Account
	browserSessions map[string]storage.BrowserSession
}

func (s *stubAccountStore) LoadAccounts() ([]storage.Account, error) {
	return append([]storage.Account(nil), s.accounts...), nil
}

func (s *stubAccountStore) SaveAccount(account storage.Account) error {
	s.accounts = upsertAccount(s.accounts, account)
	return nil
}

func (s *stubAccountStore) DeleteAccount(email string) error {
	filtered := s.accounts[:0]
	for _, account := range s.accounts {
		if strings.EqualFold(account.Email, email) {
			continue
		}
		filtered = append(filtered, account)
	}
	s.accounts = filtered
	return nil
}

func (s *stubAccountStore) SaveAllAccounts(accounts []storage.Account) error {
	s.accounts = append([]storage.Account(nil), accounts...)
	return nil
}

func (s *stubAccountStore) LoadBrowserSessions() (map[string]storage.BrowserSession, error) {
	sessions := make(map[string]storage.BrowserSession, len(s.browserSessions))
	for kind, session := range s.browserSessions {
		sessions[kind] = session
	}
	return sessions, nil
}

func (s *stubAccountStore) SaveBrowserSessions(sessions map[string]storage.BrowserSession) error {
	s.browserSessions = make(map[string]storage.BrowserSession, len(sessions))
	for kind, session := range sessions {
		s.browserSessions[kind] = session
	}
	return nil
}

func upsertAccount(accounts []storage.Account, next storage.Account) []storage.Account {
	for i, account := range accounts {
		if strings.EqualFold(account.Email, next.Email) {
			accounts[i] = next
			return accounts
		}
	}
	return append(accounts, next)
}

func newGuestBootstrapServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: "cna", Value: "guest-cna", Path: "/"})
			_, _ = w.Write([]byte("ok"))
		case "/api/v2/configs/":
			_, _ = w.Write([]byte("{}"))
		case "/api/v2/users/status":
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestInitializeFallsBackToGuestWhenNoAccounts(t *testing.T) {
	server := newGuestBootstrapServer()
	defer server.Close()

	client := qwen.NewClient(config.Config{QwenChatProxyURL: server.URL}, logging.New(false))
	service := NewService(config.Config{DataSaveMode: "none"}, config.NewRuntime(config.Config{}), &stubAccountStore{}, client, logging.New(false))

	if err := service.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	accounts := service.Accounts()
	if len(accounts) != 1 {
		t.Fatalf("accounts len = %d, want 1", len(accounts))
	}
	if !accounts[0].IsGuest() {
		t.Fatalf("expected guest fallback account, got %#v", accounts[0])
	}
}

func TestGuestFallbackYieldsToRealAccountsAndReturnsAfterDelete(t *testing.T) {
	server := newGuestBootstrapServer()
	defer server.Close()

	store := &stubAccountStore{}
	client := qwen.NewClient(config.Config{QwenChatProxyURL: server.URL}, logging.New(false))
	service := NewService(config.Config{DataSaveMode: "none"}, config.NewRuntime(config.Config{}), store, client, logging.New(false))

	if err := service.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if err := service.AddAccountWithToken("user@example.com", "secret", "jwt-token", 4102444800); err != nil {
		t.Fatalf("AddAccountWithToken() error = %v", err)
	}

	accounts := service.Accounts()
	if len(accounts) != 1 || accounts[0].IsGuest() {
		t.Fatalf("expected only real account after add, got %#v", accounts)
	}

	if err := service.DeleteAccount("user@example.com"); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	accounts = service.Accounts()
	if len(accounts) != 1 || !accounts[0].IsGuest() {
		t.Fatalf("expected guest fallback after delete, got %#v", accounts)
	}
}

func TestInitializeRestoresPersistedBrowserSession(t *testing.T) {
	server := newGuestBootstrapServer()
	defer server.Close()

	store := &stubAccountStore{
		browserSessions: map[string]storage.BrowserSession{
			"guest": {
				Headers: map[string][]string{
					"User-Agent":      {"Persisted Browser"},
					"Accept-Language": {"en-US,en;q=0.9"},
				},
				Cookie:     "cna=persisted-cna; sca=persisted-sca",
				CapturedAt: time.Now(),
				Guest:      true,
				HasCookie:  true,
			},
		},
	}
	client := qwen.NewClient(config.Config{QwenChatProxyURL: server.URL}, logging.New(false))
	service := NewService(config.Config{DataSaveMode: "file"}, config.NewRuntime(config.Config{}), store, client, logging.New(false))

	if err := service.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	snapshot := service.BrowserSessionSnapshot()
	guest, _ := snapshot["guest"].(map[string]any)
	if captured, _ := guest["captured"].(bool); !captured {
		t.Fatalf("expected restored guest browser session, got %#v", snapshot)
	}
	if hasCookie, _ := guest["hasCookie"].(bool); !hasCookie {
		t.Fatalf("expected restored guest cookie state, got %#v", guest)
	}
}

func TestInitializePreservesStoredAccountsWhenValidationFails(t *testing.T) {
	server := newGuestBootstrapServer()
	defer server.Close()

	original := storage.Account{
		Email:   "stale@example.com",
		Token:   "invalid-token",
		Cookie:  "token=invalid-token",
		Expires: 0,
	}
	store := &stubAccountStore{
		accounts: []storage.Account{original},
	}
	client := qwen.NewClient(config.Config{
		QwenChatProxyURL:   server.URL,
		BrowserAuthEnabled: false,
	}, logging.New(false))
	service := NewService(config.Config{DataSaveMode: "file"}, config.NewRuntime(config.Config{}), store, client, logging.New(false))

	if err := service.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	if len(store.accounts) != 1 {
		t.Fatalf("stored accounts len = %d, want 1", len(store.accounts))
	}
	if store.accounts[0].Email != original.Email {
		t.Fatalf("stored account email = %q, want %q", store.accounts[0].Email, original.Email)
	}

	accounts := service.Accounts()
	if len(accounts) != 1 || !accounts[0].IsGuest() {
		t.Fatalf("expected runtime guest fallback after failed validation, got %#v", accounts)
	}
}
