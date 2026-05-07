package apikeys_test

import (
	"context"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/apikeys"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

func TestEnsureDevKey_FirstRunGenerates(t *testing.T) {
	store := &fakeAPIKeyStore{}
	cleartext, generated, err := apikeys.EnsureDevKey(context.Background(), store)
	if err != nil {
		t.Fatalf("EnsureDevKey: %v", err)
	}
	if !generated {
		t.Errorf("generated = false; want true on first run")
	}
	if cleartext == "" {
		t.Errorf("cleartext is empty")
	}
	if len(cleartext) < apikeys.MinCleartextLength {
		t.Errorf("cleartext too short: %d < %d", len(cleartext), apikeys.MinCleartextLength)
	}

	// Persisted record exists with the expected name + role.
	rows, err := store.ListAPIKeys(context.Background(), state.APIKeyFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Name != apikeys.DevKeyName {
		t.Errorf("Name = %q, want %q", rows[0].Name, apikeys.DevKeyName)
	}
	if rows[0].Role != "admin" {
		t.Errorf("Role = %q, want admin", rows[0].Role)
	}
	if rows[0].KeyHash == "" {
		t.Errorf("KeyHash empty")
	}
	if !rows[0].ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero (never expires)", rows[0].ExpiresAt)
	}
}

func TestEnsureDevKey_SecondCallIsNoop(t *testing.T) {
	store := &fakeAPIKeyStore{}
	if _, _, err := apikeys.EnsureDevKey(context.Background(), store); err != nil {
		t.Fatalf("first EnsureDevKey: %v", err)
	}

	cleartext, generated, err := apikeys.EnsureDevKey(context.Background(), store)
	if err != nil {
		t.Fatalf("second EnsureDevKey: %v", err)
	}
	if generated {
		t.Errorf("generated = true on second call")
	}
	if cleartext != "" {
		t.Errorf("cleartext = %q on second call; want empty", cleartext)
	}

	rows, _ := store.ListAPIKeys(context.Background(), state.APIKeyFilter{})
	if len(rows) != 1 {
		t.Errorf("rows = %d after 2 calls; want 1 (idempotent)", len(rows))
	}
}

func TestEnsureDevKey_PreservesUnrelatedKeys(t *testing.T) {
	store := &fakeAPIKeyStore{}
	other := &state.APIKeyRecord{
		ID:        "ops-1",
		Name:      "ops-team",
		KeyHash:   "deadbeef",
		Role:      "operator",
		CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateAPIKey(context.Background(), other); err != nil {
		t.Fatal(err)
	}

	if _, generated, err := apikeys.EnsureDevKey(context.Background(), store); err != nil {
		t.Fatalf("EnsureDevKey: %v", err)
	} else if !generated {
		t.Errorf("generated = false; should have generated dev key alongside ops-team")
	}

	rows, _ := store.ListAPIKeys(context.Background(), state.APIKeyFilter{})
	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2 (ops-team + dev-default)", len(rows))
	}

	// Both names present
	names := make(map[string]bool)
	for _, r := range rows {
		names[r.Name] = true
	}
	if !names["ops-team"] || !names[apikeys.DevKeyName] {
		t.Errorf("missing names: %v", names)
	}
}

func TestEnsureDevKey_GeneratedKeyVerifiesEndToEnd(t *testing.T) {
	// Round-trip: generate dev key → look it up via the same
	// StoreVerifier the auth chain uses → assert role + name match.
	store := &fakeAPIKeyStore{}
	cleartext, _, err := apikeys.EnsureDevKey(context.Background(), store)
	if err != nil {
		t.Fatalf("EnsureDevKey: %v", err)
	}

	v := apikeys.NewStoreVerifier(store)
	verified, err := v.VerifyKey(context.Background(), cleartext)
	if err != nil {
		t.Fatalf("VerifyKey: %v", err)
	}
	if verified.Name != apikeys.DevKeyName {
		t.Errorf("Name = %q, want %q", verified.Name, apikeys.DevKeyName)
	}
	if verified.Role != auth.RoleAdmin {
		t.Errorf("Role = %v, want admin", verified.Role)
	}
}
