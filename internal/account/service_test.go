package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"qwen2api/internal/config"
	"qwen2api/internal/logging"
	"qwen2api/internal/qwen"
	"qwen2api/internal/storage"
)

type stubAccountStore struct {
	accounts []storage.Account
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
