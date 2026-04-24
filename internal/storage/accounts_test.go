package storage

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestFileAndRedisStoresHaveConsistentSemantics(t *testing.T) {
	file := &fileStore{path: filepath.Join(t.TempDir(), "data.json")}

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run() error = %v", err)
	}
	defer mr.Close()

	redisStore := &redisStore{
		client: redis.NewClient(&redis.Options{Addr: mr.Addr()}),
	}

	stores := map[string]AccountStore{
		"file":  file,
		"redis": redisStore,
	}

	initial := []Account{
		{Email: "a@example.com", Password: "p1", Token: "t1", Expires: 1710000000},
		{Email: "b@example.com", Password: "p2", Token: "t2", Expires: 1720000000},
	}
	updated := Account{Email: "a@example.com", Password: "p1x", Token: "t1x", Expires: 1730000000}

	for name, store := range stores {
		t.Run(name, func(t *testing.T) {
			if err := store.SaveAllAccounts(initial); err != nil {
				t.Fatalf("SaveAllAccounts() error = %v", err)
			}

			got, err := store.LoadAccounts()
			if err != nil {
				t.Fatalf("LoadAccounts() error = %v", err)
			}
			assertAccountsEqual(t, got, initial)

			if err := store.SaveAccount(updated); err != nil {
				t.Fatalf("SaveAccount() error = %v", err)
			}
			got, err = store.LoadAccounts()
			if err != nil {
				t.Fatalf("LoadAccounts() error = %v", err)
			}
			assertAccountsEqual(t, got, []Account{
				updated,
				initial[1],
			})

			if err := store.DeleteAccount("b@example.com"); err != nil {
				t.Fatalf("DeleteAccount() error = %v", err)
			}
			got, err = store.LoadAccounts()
			if err != nil {
				t.Fatalf("LoadAccounts() error = %v", err)
			}
			assertAccountsEqual(t, got, []Account{updated})

			if err := store.SaveAllAccounts(nil); err != nil {
				t.Fatalf("SaveAllAccounts(nil) error = %v", err)
			}
			got, err = store.LoadAccounts()
			if err != nil {
				t.Fatalf("LoadAccounts() error = %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("expected empty accounts after overwrite, got %#v", got)
			}
		})
	}
}

func assertAccountsEqual(t *testing.T, got, want []Account) {
	t.Helper()
	gotCopy := append([]Account(nil), got...)
	wantCopy := append([]Account(nil), want...)
	slices.SortFunc(gotCopy, func(a, b Account) int {
		switch {
		case a.Email < b.Email:
			return -1
		case a.Email > b.Email:
			return 1
		default:
			return 0
		}
	})
	slices.SortFunc(wantCopy, func(a, b Account) int {
		switch {
		case a.Email < b.Email:
			return -1
		case a.Email > b.Email:
			return 1
		default:
			return 0
		}
	})
	if len(gotCopy) != len(wantCopy) {
		t.Fatalf("account len = %d, want %d", len(gotCopy), len(wantCopy))
	}
	for i := range wantCopy {
		if gotCopy[i] != wantCopy[i] {
			t.Fatalf("account[%d] = %#v, want %#v", i, gotCopy[i], wantCopy[i])
		}
	}
}
