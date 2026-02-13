package token

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockEtcdBackend is an in-memory mock of EtcdBackend.
type mockEtcdBackend struct {
	mu   sync.Mutex
	data map[string][]byte

	// casFailCount makes CompareAndSwap fail this many times before succeeding.
	casFailCount int
	casAttempts  int
}

func newMockEtcdBackend() *mockEtcdBackend {
	return &mockEtcdBackend{data: make(map[string][]byte)}
}

func (m *mockEtcdBackend) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return nil, nil
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (m *mockEtcdBackend) Put(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	m.data[key] = cp
	return nil
}

func (m *mockEtcdBackend) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mockEtcdBackend) List(_ context.Context, prefix string) (map[string][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string][]byte)
	for k, v := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			cp := make([]byte, len(v))
			copy(cp, v)
			result[k] = cp
		}
	}
	return result, nil
}

func (m *mockEtcdBackend) CompareAndSwap(_ context.Context, key string, expected, value []byte) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.casAttempts++
	if m.casFailCount > 0 {
		m.casFailCount--
		return false, nil
	}

	current, exists := m.data[key]

	if expected == nil {
		if exists {
			return false, nil
		}
	} else {
		if !exists || string(current) != string(expected) {
			return false, nil
		}
	}

	cp := make([]byte, len(value))
	copy(cp, value)
	m.data[key] = cp
	return true, nil
}

var _ EtcdBackend = (*mockEtcdBackend)(nil)

func createTestEtcdStore(t *testing.T) (*EtcdStore, *mockEtcdBackend) {
	t.Helper()
	backend := newMockEtcdBackend()
	store, err := NewEtcdStore(backend)
	if err != nil {
		t.Fatalf("NewEtcdStore() error: %v", err)
	}
	return store, backend
}

func createTestToken(t *testing.T, label string, ttl time.Duration, maxUses int) (*JoinToken, string) {
	t.Helper()
	tok, raw, err := NewJoinToken(label, "test-admin", ttl, maxUses)
	if err != nil {
		t.Fatalf("NewJoinToken() error: %v", err)
	}
	return tok, raw
}

func createExpiredTestToken(t *testing.T, label string) (*JoinToken, string) {
	t.Helper()
	raw, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error: %v", err)
	}
	id, err := GenerateID()
	if err != nil {
		t.Fatalf("GenerateID() error: %v", err)
	}
	now := time.Now()
	tok := &JoinToken{
		ID:        id,
		TokenHash: HashToken(raw, salt),
		Salt:      salt,
		Label:     label,
		CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-time.Hour),
		MaxUses:   0,
		CreatedBy: "test-admin",
	}
	return tok, raw
}

func TestNewEtcdStore_NilBackend(t *testing.T) {
	_, err := NewEtcdStore(nil)
	if err == nil {
		t.Fatal("expected error for nil backend")
	}
}

func TestEtcdStore_CreateAndGetByID(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "test", time.Hour, 0)
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
}

func TestEtcdStore_CreateDuplicate(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "test", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	err := store.Create(ctx, tok)
	if !errors.Is(err, ErrTokenDuplicate) {
		t.Errorf("expected ErrTokenDuplicate, got %v", err)
	}
}

func TestEtcdStore_GetByID_NotFound(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	_, err := store.GetByID(ctx, "nonexistent")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestEtcdStore_Lookup(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	tok, raw := createTestToken(t, "lookup-test", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	found, err := store.Lookup(ctx, raw)
	if err != nil {
		t.Fatalf("Lookup() error: %v", err)
	}
	if found.ID != tok.ID {
		t.Errorf("Lookup returned wrong token: got ID %q, want %q", found.ID, tok.ID)
	}
}

func TestEtcdStore_Lookup_NotFound(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	_, err := store.Lookup(ctx, "kscore-join-nonexistent")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestEtcdStore_List(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		tok, _ := createTestToken(t, fmt.Sprintf("tok-%d", i), time.Hour, 0)
		if err := store.Create(ctx, tok); err != nil {
			t.Fatalf("Create() error: %v", err)
		}
	}

	tokens, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(tokens) != 3 {
		t.Errorf("List() returned %d tokens, want 3", len(tokens))
	}
}

func TestEtcdStore_Revoke(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "revoke-test", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := store.Revoke(ctx, tok.ID); err != nil {
		t.Fatalf("Revoke() error: %v", err)
	}

	got, err := store.GetByID(ctx, tok.ID)
	if err != nil {
		t.Fatalf("GetByID() error: %v", err)
	}
	if !got.Revoked {
		t.Error("token should be revoked")
	}
}

func TestEtcdStore_Revoke_NotFound(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	err := store.Revoke(ctx, "nonexistent")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestEtcdStore_IncrementUses(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "inc-test", time.Hour, 5)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := store.IncrementUses(ctx, tok.ID); err != nil {
		t.Fatalf("IncrementUses() error: %v", err)
	}

	got, _ := store.GetByID(ctx, tok.ID)
	if got.UsedCount != 1 {
		t.Errorf("UsedCount = %d, want 1", got.UsedCount)
	}
}

func TestEtcdStore_IncrementUses_NotFound(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	err := store.IncrementUses(ctx, "nonexistent")
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("expected ErrTokenNotFound, got %v", err)
	}
}

func TestEtcdStore_IncrementUses_Revoked(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "revoked", time.Hour, 0)
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

func TestEtcdStore_IncrementUses_Expired(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	tok, _ := createExpiredTestToken(t, "expired")
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	err := store.IncrementUses(ctx, tok.ID)
	if !errors.Is(err, ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestEtcdStore_IncrementUses_Exhausted(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "exhausted", time.Hour, 1)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := store.IncrementUses(ctx, tok.ID); err != nil {
		t.Fatalf("first IncrementUses() error: %v", err)
	}

	err := store.IncrementUses(ctx, tok.ID)
	if !errors.Is(err, ErrTokenExhausted) {
		t.Errorf("expected ErrTokenExhausted, got %v", err)
	}
}

func TestEtcdStore_IncrementUses_CASRetry(t *testing.T) {
	store, backend := createTestEtcdStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "cas-retry", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// Make CAS fail twice before succeeding.
	backend.mu.Lock()
	backend.casFailCount = 2
	backend.casAttempts = 0
	backend.mu.Unlock()

	if err := store.IncrementUses(ctx, tok.ID); err != nil {
		t.Fatalf("IncrementUses() with CAS retries error: %v", err)
	}

	backend.mu.Lock()
	attempts := backend.casAttempts
	backend.mu.Unlock()
	if attempts < 3 {
		t.Errorf("expected at least 3 CAS attempts, got %d", attempts)
	}
}

func TestEtcdStore_IncrementUses_CASExhausted(t *testing.T) {
	store, backend := createTestEtcdStore(t)
	ctx := context.Background()

	tok, _ := createTestToken(t, "cas-exhaust", time.Hour, 0)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	backend.mu.Lock()
	backend.casFailCount = casMaxRetries + 10
	backend.mu.Unlock()

	err := store.IncrementUses(ctx, tok.ID)
	if !errors.Is(err, ErrCASMaxRetries) {
		t.Errorf("expected ErrCASMaxRetries, got %v", err)
	}
}

func TestEtcdStore_DeleteExpired(t *testing.T) {
	store, _ := createTestEtcdStore(t)
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
	if tokens[0].ID != tok1.ID {
		t.Errorf("remaining token ID = %q, want %q", tokens[0].ID, tok1.ID)
	}
}

func TestEtcdStore_InterfaceCompliance(t *testing.T) {
	var _ Store = (*EtcdStore)(nil)
}

// createTestToken with negative TTL creates an already-expired token.
func TestEtcdStore_CreateExpiredToken(t *testing.T) {
	store, _ := createTestEtcdStore(t)
	ctx := context.Background()

	now := time.Now()
	tok := &JoinToken{
		ID:        "expired-manual",
		TokenHash: HashToken("raw", "salt"),
		Salt:      "salt",
		CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-time.Hour),
		MaxUses:   0,
	}
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	got, err := store.GetByID(ctx, tok.ID)
	if err != nil {
		t.Fatalf("GetByID() error: %v", err)
	}
	if !got.IsExpired() {
		t.Error("token should be expired")
	}
}

// Verify the mock backend stores actual JSON correctly.
func TestEtcdStore_StoredFormat(t *testing.T) {
	_, backend := createTestEtcdStore(t)
	store, _ := NewEtcdStore(backend)
	ctx := context.Background()

	tok, _ := createTestToken(t, "format-test", time.Hour, 3)
	if err := store.Create(ctx, tok); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	data, _ := backend.Get(ctx, etcdKeyPrefix+tok.ID)
	var stored JoinToken
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("stored data is not valid JSON: %v", err)
	}
	if stored.ID != tok.ID {
		t.Errorf("stored ID = %q, want %q", stored.ID, tok.ID)
	}
}
