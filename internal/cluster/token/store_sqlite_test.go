package token

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func createTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tokens.db")
	store, err := NewSQLiteStore(&SQLiteStoreConfig{
		Path:    dbPath,
		WALMode: true,
	})
	if err != nil {
		t.Fatalf("NewSQLiteStore() error: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestNewSQLiteStore_NilConfig(t *testing.T) {
	_, err := NewSQLiteStore(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewSQLiteStore_EmptyPath(t *testing.T) {
	_, err := NewSQLiteStore(&SQLiteStoreConfig{})
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestSQLiteStore_CreateAndGetByID(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "sqlite-test", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	got, err := store.GetByID(ctx, tok.ID)
	if err != nil {
		t.Fatalf("GetByID() error: %v", err)
	}
	if got.ID != tok.ID {
		t.Errorf("ID = %q, want %q", got.ID, tok.ID)
	}
	if got.Label != tok.Label {
		t.Errorf("Label = %q, want %q", got.Label, tok.Label)
	}
	if got.TokenHash != tok.TokenHash {
		t.Errorf("TokenHash mismatch")
	}
	if got.MaxUses != tok.MaxUses {
		t.Errorf("MaxUses = %d, want %d", got.MaxUses, tok.MaxUses)
	}
}

func TestSQLiteStore_GetByID_NotFound(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	_, err := store.GetByID(ctx, "nonexistent")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestSQLiteStore_Lookup(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	tok, raw := createTestToken(t, "lookup", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	found, err := store.Lookup(ctx, raw)
	if err != nil {
		t.Fatalf("Lookup() error: %v", err)
	}
	if found.ID != tok.ID {
		t.Errorf("Lookup returned wrong token: got %q, want %q", found.ID, tok.ID)
	}
}

func TestSQLiteStore_Lookup_NotFound(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	_, err := store.Lookup(ctx, "kscore-join-bogus")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestSQLiteStore_Lookup_SkipsExpiredAndRevoked(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	// Expired token.
	tok1, raw1 := createExpiredTestToken(t, "expired")
	if err := store.Create(ctx, tok1); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Revoked token.
	tok2, raw2 := createTestToken(t, "revoked", time.Hour, 0)
	if err := store.Create(ctx, tok2); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := store.Revoke(ctx, tok2.ID); err != nil {
		t.Fatalf("Revoke() error: %v", err)
	}

	_, err := store.Lookup(ctx, raw1)
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("expired token Lookup: expected ErrTokenNotFound, got %v", err)
	}

	_, err = store.Lookup(ctx, raw2)
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("revoked token Lookup: expected ErrTokenNotFound, got %v", err)
	}
}

func TestSQLiteStore_List(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		tok, _ := createTestToken(t, fmt.Sprintf("tok-%d", i), time.Hour, 0)
		if err := store.Create(ctx, tok); err != nil {
			t.Fatalf("Create() error: %v", err)
		}
	}

	tokens, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(tokens) != 5 {
		t.Errorf("List() returned %d tokens, want 5", len(tokens))
	}
}

func TestSQLiteStore_Revoke(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "revoke", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := store.Revoke(ctx, tok.ID); err != nil {
		t.Fatalf("Revoke() error: %v", err)
	}

	got, _ := store.GetByID(ctx, tok.ID)
	if !got.Revoked {
		t.Error("token should be revoked")
	}
}

func TestSQLiteStore_Revoke_NotFound(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	err := store.Revoke(ctx, "nonexistent")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestSQLiteStore_IncrementUses(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "inc", time.Hour, 5)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := store.IncrementUses(ctx, tok.ID); err != nil {
			t.Fatalf("IncrementUses() iteration %d error: %v", i, err)
		}
	}

	got, _ := store.GetByID(ctx, tok.ID)
	if got.UsedCount != 3 {
		t.Errorf("UsedCount = %d, want 3", got.UsedCount)
	}
}

func TestSQLiteStore_IncrementUses_NotFound(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	err := store.IncrementUses(ctx, "nonexistent")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestSQLiteStore_IncrementUses_Revoked(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "revoked-inc", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := store.Revoke(ctx, tok.ID); err != nil {
		t.Fatalf("Revoke() error: %v", err)
	}

	err := store.IncrementUses(ctx, tok.ID)
	if !errors.Is(err, ErrTokenRevoked) {
		t.Errorf("expected ErrTokenRevoked, got %v", err)
	}
}

func TestSQLiteStore_IncrementUses_Expired(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	tok, _ := createExpiredTestToken(t, "expired-inc")
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	err := store.IncrementUses(ctx, tok.ID)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestSQLiteStore_IncrementUses_Exhausted(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "exhausted-inc", time.Hour, 2)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Use up all allowed uses.
	for i := 0; i < 2; i++ {
		if err := store.IncrementUses(ctx, tok.ID); err != nil {
			t.Fatalf("IncrementUses() iteration %d error: %v", i, err)
		}
	}

	err := store.IncrementUses(ctx, tok.ID)
	if !errors.Is(err, ErrTokenExhausted) {
		t.Errorf("expected ErrTokenExhausted, got %v", err)
	}
}

func TestSQLiteStore_IncrementUses_Unlimited(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "unlimited", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	for i := 0; i < 20; i++ {
		if err := store.IncrementUses(ctx, tok.ID); err != nil {
			t.Fatalf("IncrementUses() iteration %d error: %v", i, err)
		}
	}

	got, _ := store.GetByID(ctx, tok.ID)
	if got.UsedCount != 20 {
		t.Errorf("UsedCount = %d, want 20", got.UsedCount)
	}
}

func TestSQLiteStore_IncrementUses_Concurrent(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "concurrent", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := store.IncrementUses(ctx, tok.ID); err != nil {
				t.Errorf("IncrementUses() error: %v", err)
			}
		}()
	}
	wg.Wait()

	got, _ := store.GetByID(ctx, tok.ID)
	if got.UsedCount != goroutines {
		t.Errorf("UsedCount = %d, want %d", got.UsedCount, goroutines)
	}
}

func TestSQLiteStore_DeleteExpired(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	// Valid token.
	tok1, _ := createTestToken(t, "valid", time.Hour, 0)
	if err := store.Create(ctx, tok1); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Expired token.
	tok2, _ := createExpiredTestToken(t, "expired")
	if err := store.Create(ctx, tok2); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Revoked token.
	tok3, _ := createTestToken(t, "revoked", time.Hour, 0)
	if err := store.Create(ctx, tok3); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := store.Revoke(ctx, tok3.ID); err != nil {
		t.Fatalf("Revoke() error: %v", err)
	}

	deleted, err := store.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired() error: %v", err)
	}
	if deleted != 2 {
		t.Errorf("DeleteExpired() = %d, want 2", deleted)
	}

	tokens, _ := store.List(ctx)
	if len(tokens) != 1 {
		t.Errorf("remaining tokens = %d, want 1", len(tokens))
	}
}

func TestSQLiteStore_DeleteExpired_NoneToDelete(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "valid", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	deleted, err := store.DeleteExpired(ctx)
	if err != nil {
		t.Fatalf("DeleteExpired() error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("DeleteExpired() = %d, want 0", deleted)
	}
}

func TestSQLiteStore_EmptyLabel(t *testing.T) {
	store := createTestSQLiteStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	got, err := store.GetByID(ctx, tok.ID)
	if err != nil {
		t.Fatalf("GetByID() error: %v", err)
	}
	if got.Label != "" {
		t.Errorf("Label = %q, want empty", got.Label)
	}
}

func TestSQLiteStore_InterfaceCompliance(t *testing.T) {
	var _ Store = (*SQLiteStore)(nil)
}
