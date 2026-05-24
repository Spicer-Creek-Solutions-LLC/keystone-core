// SPDX-License-Identifier: Apache-2.0

package state

import (
	"errors"
	"testing"
	"time"
)

func sampleAPIKey(id, hash string) *APIKeyRecord {
	return &APIKeyRecord{
		ID:        id,
		Name:      "key-" + id,
		KeyHash:   hash,
		Role:      "operator",
		CreatedAt: time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
	}
}

func TestSQLiteStore_APIKeyCRUD(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	k := sampleAPIKey("k-1", "deadbeef")
	k.ExpiresAt = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := s.CreateAPIKey(ctx, k); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	got, err := s.GetAPIKey(ctx, "k-1")
	if err != nil {
		t.Fatalf("GetAPIKey: %v", err)
	}
	if got.ID != k.ID || got.Name != k.Name || got.KeyHash != k.KeyHash || got.Role != k.Role {
		t.Errorf("scalars lost: %+v", got)
	}
	if !got.CreatedAt.Equal(k.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, k.CreatedAt)
	}
	if !got.ExpiresAt.Equal(k.ExpiresAt) {
		t.Errorf("ExpiresAt: got %v, want %v", got.ExpiresAt, k.ExpiresAt)
	}
	if !got.LastUsed.IsZero() {
		t.Errorf("LastUsed should be zero on a fresh key; got %v", got.LastUsed)
	}

	// Lookup by hash powers the auth verifier.
	byHash, err := s.GetAPIKeyByHash(ctx, "deadbeef")
	if err != nil {
		t.Fatalf("GetAPIKeyByHash: %v", err)
	}
	if byHash.ID != "k-1" {
		t.Errorf("byHash: %+v", byHash)
	}

	// Update last-used.
	t0 := time.Date(2026, 5, 7, 14, 0, 0, 0, time.UTC)
	if err := s.UpdateAPIKeyLastUsed(ctx, "k-1", t0); err != nil {
		t.Fatalf("UpdateAPIKeyLastUsed: %v", err)
	}
	got, err = s.GetAPIKey(ctx, "k-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastUsed.Equal(t0) {
		t.Errorf("LastUsed: got %v, want %v", got.LastUsed, t0)
	}

	// Delete.
	if err := s.DeleteAPIKey(ctx, "k-1"); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}
	if _, err := s.GetAPIKey(ctx, "k-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("post-delete Get: %v, want ErrNotFound", err)
	}
}

func TestSQLiteStore_APIKey_NotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	if _, err := s.GetAPIKey(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAPIKey: %v", err)
	}
	if _, err := s.GetAPIKeyByHash(ctx, "deadbeef"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAPIKeyByHash: %v", err)
	}
	if err := s.UpdateAPIKeyLastUsed(ctx, "missing", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateAPIKeyLastUsed: %v", err)
	}
	if err := s.DeleteAPIKey(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteAPIKey: %v", err)
	}
}

func TestSQLiteStore_APIKey_NilRecord(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	if err := s.CreateAPIKey(t.Context(), nil); err == nil {
		t.Error("CreateAPIKey(nil): expected error")
	}
}

func TestSQLiteStore_APIKey_RequiresKeyHash(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	k := sampleAPIKey("k-empty", "")
	if err := s.CreateAPIKey(t.Context(), k); err == nil {
		t.Error("CreateAPIKey with empty KeyHash: expected error")
	}
}

func TestSQLiteStore_APIKey_HashUniqueConstraint(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	if err := s.CreateAPIKey(ctx, sampleAPIKey("k-1", "samehash")); err != nil {
		t.Fatal(err)
	}
	// Second key with the same hash must fail (UNIQUE constraint on
	// key_hash). Different IDs aren't enough.
	if err := s.CreateAPIKey(ctx, sampleAPIKey("k-2", "samehash")); err == nil {
		t.Error("expected UNIQUE violation on duplicate key_hash")
	}
}

func TestSQLiteStore_ListAPIKeys_Filters(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	for i, role := range []string{"admin", "operator", "operator", "readonly"} {
		k := sampleAPIKey(string(rune('a'+i)), string(rune('h'+i)))
		k.Role = role
		if err := s.CreateAPIKey(ctx, k); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("all", func(t *testing.T) {
		got, err := s.ListAPIKeys(ctx, APIKeyFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 4 {
			t.Errorf("got %d, want 4", len(got))
		}
	})
	t.Run("by role", func(t *testing.T) {
		got, err := s.ListAPIKeys(ctx, APIKeyFilter{Role: "operator"})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("got %d operator keys, want 2", len(got))
		}
	})
	t.Run("limit", func(t *testing.T) {
		got, err := s.ListAPIKeys(ctx, APIKeyFilter{Limit: 2})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Errorf("got %d, want 2", len(got))
		}
	})
	t.Run("invalid sort rejected", func(t *testing.T) {
		_, err := s.ListAPIKeys(ctx, APIKeyFilter{SortColumn: "evil"})
		if err == nil {
			t.Error("expected error for non-allowlisted sort column")
		}
	})
}
