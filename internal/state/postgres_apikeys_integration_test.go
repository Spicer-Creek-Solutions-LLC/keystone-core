// SPDX-License-Identifier: Apache-2.0

//go:build integration

package state

import (
	"errors"
	"testing"
	"time"
)

func TestPg_APIKeyCRUD(t *testing.T) {
	s := newPgStoreForTest(t)
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
	if got.ID != k.ID || got.KeyHash != k.KeyHash || got.Role != k.Role {
		t.Errorf("scalars: %+v", got)
	}
	if !got.CreatedAt.Equal(k.CreatedAt) {
		t.Errorf("CreatedAt: %v vs %v", got.CreatedAt, k.CreatedAt)
	}

	byHash, err := s.GetAPIKeyByHash(ctx, "deadbeef")
	if err != nil {
		t.Fatalf("GetAPIKeyByHash: %v", err)
	}
	if byHash.ID != "k-1" {
		t.Errorf("byHash: %+v", byHash)
	}

	t0 := time.Date(2026, 5, 7, 14, 0, 0, 0, time.UTC)
	if err := s.UpdateAPIKeyLastUsed(ctx, "k-1", t0); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetAPIKey(ctx, "k-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastUsed.Equal(t0) {
		t.Errorf("LastUsed: %v vs %v", got.LastUsed, t0)
	}

	if err := s.DeleteAPIKey(ctx, "k-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAPIKey(ctx, "k-1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("post-delete: %v", err)
	}
}

func TestPg_APIKey_NotFound(t *testing.T) {
	s := newPgStoreForTest(t)
	if _, err := s.GetAPIKey(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAPIKey: %v", err)
	}
	if _, err := s.GetAPIKeyByHash(t.Context(), "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAPIKeyByHash: %v", err)
	}
}

func TestPg_APIKey_HashUniqueConstraint(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	if err := s.CreateAPIKey(ctx, sampleAPIKey("k-1", "samehash")); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAPIKey(ctx, sampleAPIKey("k-2", "samehash")); err == nil {
		t.Error("expected UNIQUE violation")
	}
}

func TestPg_ListAPIKeys_Filters(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	for i, role := range []string{"admin", "operator", "operator", "readonly"} {
		k := sampleAPIKey(string(rune('a'+i)), string(rune('h'+i)))
		k.Role = role
		if err := s.CreateAPIKey(ctx, k); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListAPIKeys(ctx, APIKeyFilter{Role: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("operator keys: %d, want 2", len(got))
	}

	if _, err := s.ListAPIKeys(ctx, APIKeyFilter{SortColumn: "evil"}); err == nil {
		t.Error("expected sort allowlist error")
	}
}
