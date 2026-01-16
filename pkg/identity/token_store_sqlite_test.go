package identity

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteTokenStore_NewSQLiteTokenStore(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		_, err := NewSQLiteTokenStore(nil)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("empty path", func(t *testing.T) {
		_, err := NewSQLiteTokenStore(&SQLiteTokenStoreConfig{})
		if err == nil {
			t.Error("expected error for empty path")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "tokens.db")

		store, err := NewSQLiteTokenStore(&SQLiteTokenStoreConfig{
			Path:    dbPath,
			WALMode: true,
		})
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}
		defer store.Close()

		// Verify file was created
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Error("database file was not created")
		}
	})
}

func TestSQLiteTokenStore_Create(t *testing.T) {
	store := createTestTokenStore(t)
	defer store.Close()
	ctx := context.Background()

	t.Run("create valid token", func(t *testing.T) {
		token := &JoinToken{
			Token:     "test-token-12345678",
			AgentID:   "agent-1",
			ExpiresAt: time.Now().Add(time.Hour),
		}

		err := store.Create(ctx, token)
		if err != nil {
			t.Fatalf("failed to create token: %v", err)
		}

		// Verify token was modified
		if token.TokenHash == "" {
			t.Error("token hash was not set")
		}
		if token.Salt == "" {
			t.Error("salt was not set")
		}
		if token.TokenPrefix == "" {
			t.Error("token prefix was not set")
		}
		if token.Token != "" {
			t.Error("plaintext token should be cleared after create")
		}
		if token.CreatedAt.IsZero() {
			t.Error("created_at was not set")
		}
	})

	t.Run("empty token value", func(t *testing.T) {
		token := &JoinToken{
			Token:     "",
			ExpiresAt: time.Now().Add(time.Hour),
		}

		err := store.Create(ctx, token)
		if err == nil {
			t.Error("expected error for empty token value")
		}
	})
}

func TestSQLiteTokenStore_Get(t *testing.T) {
	store := createTestTokenStore(t)
	defer store.Close()
	ctx := context.Background()

	// Create a token
	plainToken := "get-test-token-12345678"
	token := &JoinToken{
		Token:     plainToken,
		AgentID:   "agent-2",
		ExpiresAt: time.Now().Add(time.Hour),
		Metadata:  map[string]string{"key": "value"},
	}

	err := store.Create(ctx, token)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	t.Run("get existing token", func(t *testing.T) {
		retrieved, err := store.Get(ctx, plainToken)
		if err != nil {
			t.Fatalf("failed to get token: %v", err)
		}

		if retrieved.AgentID != "agent-2" {
			t.Errorf("expected agent_id 'agent-2', got %q", retrieved.AgentID)
		}
		if retrieved.Metadata["key"] != "value" {
			t.Error("metadata was not preserved")
		}
	})

	t.Run("get non-existent token", func(t *testing.T) {
		_, err := store.Get(ctx, "non-existent-token")
		if err == nil {
			t.Error("expected error for non-existent token")
		}
	})
}

func TestSQLiteTokenStore_MarkUsed(t *testing.T) {
	store := createTestTokenStore(t)
	defer store.Close()
	ctx := context.Background()

	// Create a token
	plainToken := "mark-used-test-token-12345678"
	token := &JoinToken{
		Token:     plainToken,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	err := store.Create(ctx, token)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	t.Run("mark token as used", func(t *testing.T) {
		err := store.MarkUsed(ctx, plainToken, "agent-using-token")
		if err != nil {
			t.Fatalf("failed to mark token as used: %v", err)
		}

		// Verify token is marked as used
		retrieved, err := store.Get(ctx, plainToken)
		if err != nil {
			t.Fatalf("failed to get token: %v", err)
		}

		if !retrieved.Used {
			t.Error("token should be marked as used")
		}
		if retrieved.UsedBy != "agent-using-token" {
			t.Errorf("expected used_by 'agent-using-token', got %q", retrieved.UsedBy)
		}
		if retrieved.UsedAt.IsZero() {
			t.Error("used_at should be set")
		}
	})

	t.Run("mark non-existent token", func(t *testing.T) {
		err := store.MarkUsed(ctx, "non-existent-token", "agent")
		if err == nil {
			t.Error("expected error for non-existent token")
		}
	})
}

func TestSQLiteTokenStore_Delete(t *testing.T) {
	store := createTestTokenStore(t)
	defer store.Close()
	ctx := context.Background()

	// Create a token
	plainToken := "delete-test-token-12345678"
	token := &JoinToken{
		Token:     plainToken,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	err := store.Create(ctx, token)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	t.Run("delete existing token", func(t *testing.T) {
		err := store.Delete(ctx, plainToken)
		if err != nil {
			t.Fatalf("failed to delete token: %v", err)
		}

		// Verify token is deleted
		_, err = store.Get(ctx, plainToken)
		if err == nil {
			t.Error("expected error when getting deleted token")
		}
	})

	t.Run("delete non-existent token", func(t *testing.T) {
		err := store.Delete(ctx, "non-existent-token")
		if err == nil {
			t.Error("expected error for non-existent token")
		}
	})
}

func TestSQLiteTokenStore_List(t *testing.T) {
	store := createTestTokenStore(t)
	defer store.Close()
	ctx := context.Background()

	// Create multiple tokens
	for i := 0; i < 3; i++ {
		token := &JoinToken{
			Token:     generateTestToken(t),
			AgentID:   "agent-list",
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if err := store.Create(ctx, token); err != nil {
			t.Fatalf("failed to create token %d: %v", i, err)
		}
	}

	tokens, err := store.List(ctx)
	if err != nil {
		t.Fatalf("failed to list tokens: %v", err)
	}

	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d", len(tokens))
	}

	// Verify tokens don't have plaintext values
	for _, token := range tokens {
		if token.Token != "" {
			t.Error("listed tokens should not have plaintext value")
		}
	}
}

func TestSQLiteTokenStore_Cleanup(t *testing.T) {
	store := createTestTokenStore(t)
	defer store.Close()
	ctx := context.Background()

	// Create an expired token
	expiredToken := &JoinToken{
		Token:     generateTestToken(t),
		ExpiresAt: time.Now().Add(-time.Hour), // Already expired
	}
	if err := store.Create(ctx, expiredToken); err != nil {
		t.Fatalf("failed to create expired token: %v", err)
	}

	// Create a used token
	usedPlainToken := generateTestToken(t)
	usedToken := &JoinToken{
		Token:     usedPlainToken,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.Create(ctx, usedToken); err != nil {
		t.Fatalf("failed to create used token: %v", err)
	}
	if err := store.MarkUsed(ctx, usedPlainToken, "agent"); err != nil {
		t.Fatalf("failed to mark token as used: %v", err)
	}

	// Create a valid token
	validToken := &JoinToken{
		Token:     generateTestToken(t),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.Create(ctx, validToken); err != nil {
		t.Fatalf("failed to create valid token: %v", err)
	}

	// Run cleanup
	count, err := store.Cleanup(ctx)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 tokens cleaned up, got %d", count)
	}

	// Verify only valid token remains
	remaining, err := store.List(ctx)
	if err != nil {
		t.Fatalf("failed to list tokens: %v", err)
	}

	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining token, got %d", len(remaining))
	}
}

func TestSQLiteTokenStore_Count(t *testing.T) {
	store := createTestTokenStore(t)
	defer store.Close()
	ctx := context.Background()

	// Initially should be 0
	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 tokens, got %d", count)
	}

	// Create tokens
	for i := 0; i < 5; i++ {
		token := &JoinToken{
			Token:     generateTestToken(t),
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if err := store.Create(ctx, token); err != nil {
			t.Fatalf("failed to create token: %v", err)
		}
	}

	count, err = store.Count(ctx)
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5 tokens, got %d", count)
	}
}

func TestSQLiteTokenStore_GetStats(t *testing.T) {
	store := createTestTokenStore(t)
	defer store.Close()
	ctx := context.Background()

	// Create expired token
	expiredToken := &JoinToken{
		Token:     generateTestToken(t),
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	if err := store.Create(ctx, expiredToken); err != nil {
		t.Fatalf("failed to create expired token: %v", err)
	}

	// Create used token
	usedPlainToken := generateTestToken(t)
	usedToken := &JoinToken{
		Token:     usedPlainToken,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := store.Create(ctx, usedToken); err != nil {
		t.Fatalf("failed to create used token: %v", err)
	}
	if err := store.MarkUsed(ctx, usedPlainToken, "agent"); err != nil {
		t.Fatalf("failed to mark token as used: %v", err)
	}

	// Create valid tokens
	for i := 0; i < 3; i++ {
		token := &JoinToken{
			Token:     generateTestToken(t),
			ExpiresAt: time.Now().Add(time.Hour),
		}
		if err := store.Create(ctx, token); err != nil {
			t.Fatalf("failed to create valid token: %v", err)
		}
	}

	stats, err := store.GetStats(ctx)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.Total != 5 {
		t.Errorf("expected total 5, got %d", stats.Total)
	}
	if stats.Used != 1 {
		t.Errorf("expected used 1, got %d", stats.Used)
	}
	if stats.Expired != 1 {
		t.Errorf("expected expired 1, got %d", stats.Expired)
	}
	if stats.Valid != 3 {
		t.Errorf("expected valid 3, got %d", stats.Valid)
	}
}

func TestSQLiteTokenStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "tokens.db")
	ctx := context.Background()

	// Create a token with first store instance
	plainToken := generateTestToken(t)
	{
		store, err := NewSQLiteTokenStore(&SQLiteTokenStoreConfig{
			Path:    dbPath,
			WALMode: true,
		})
		if err != nil {
			t.Fatalf("failed to create store: %v", err)
		}

		token := &JoinToken{
			Token:     plainToken,
			AgentID:   "persistent-agent",
			ExpiresAt: time.Now().Add(time.Hour),
			Metadata:  map[string]string{"persistent": "true"},
		}

		if err := store.Create(ctx, token); err != nil {
			t.Fatalf("failed to create token: %v", err)
		}

		store.Close()
	}

	// Reopen and verify token exists
	{
		store, err := NewSQLiteTokenStore(&SQLiteTokenStoreConfig{
			Path:    dbPath,
			WALMode: true,
		})
		if err != nil {
			t.Fatalf("failed to reopen store: %v", err)
		}
		defer store.Close()

		retrieved, err := store.Get(ctx, plainToken)
		if err != nil {
			t.Fatalf("failed to get persistent token: %v", err)
		}

		if retrieved.AgentID != "persistent-agent" {
			t.Errorf("expected agent_id 'persistent-agent', got %q", retrieved.AgentID)
		}
		if retrieved.Metadata["persistent"] != "true" {
			t.Error("metadata was not persisted")
		}
	}
}

// Helper functions

func createTestTokenStore(t *testing.T) *SQLiteTokenStore {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_tokens.db")

	store, err := NewSQLiteTokenStore(&SQLiteTokenStoreConfig{
		Path:    dbPath,
		WALMode: true,
	})
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	return store
}

func generateTestToken(t *testing.T) string {
	t.Helper()
	// Generate a unique token for testing
	salt, err := generateSalt()
	if err != nil {
		t.Fatalf("failed to generate salt: %v", err)
	}
	return "test-token-" + salt[:16]
}
