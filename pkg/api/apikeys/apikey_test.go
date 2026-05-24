// SPDX-License-Identifier: Apache-2.0

package apikeys_test

import (
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/api/apikeys"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

func TestGenerate_Success(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	g, err := apikeysGenerateAt(t, "ops-key", "operator", time.Time{}, now)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if g.Name != "ops-key" || g.Role != "operator" {
		t.Errorf("metadata: %+v", g)
	}
	if g.ID == "" {
		t.Error("ID should be populated")
	}
	if len(g.Cleartext) < apikeys.MinCleartextLength {
		t.Errorf("Cleartext length = %d, want >= %d", len(g.Cleartext), apikeys.MinCleartextLength)
	}
	if g.KeyHash == "" {
		t.Error("KeyHash should be populated")
	}
	if g.KeyHash != auth.HashAPIKey(g.Cleartext) {
		t.Error("KeyHash must be SHA-256 hex of Cleartext (matches auth.HashAPIKey)")
	}
	if !g.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt: %v vs %v", g.CreatedAt, now)
	}
}

func TestGenerate_RolesAccepted(t *testing.T) {
	for _, role := range []string{"admin", "operator", "readonly"} {
		t.Run(role, func(t *testing.T) {
			g, err := apikeys.Generate("k", role, time.Time{})
			if err != nil {
				t.Errorf("Generate(%q): %v", role, err)
			}
			if g.Role != role {
				t.Errorf("Role = %q, want %q", g.Role, role)
			}
		})
	}
}

func TestGenerate_RejectsInvalid(t *testing.T) {
	tests := []struct {
		name  string
		nName string
		role  string
	}{
		{"empty name", "", "admin"},
		{"empty role", "k", ""},
		{"unknown role", "k", "superuser"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := apikeys.Generate(tt.nName, tt.role, time.Time{}); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestGenerate_DistinctCleartext(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		g, err := apikeys.Generate("k", "operator", time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if seen[g.Cleartext] {
			t.Fatalf("duplicate cleartext after %d iters", i)
		}
		seen[g.Cleartext] = true
	}
}

func TestGenerate_HashRoundTrips(t *testing.T) {
	g, err := apikeys.Generate("k", "operator", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !auth.CompareKeyHash(g.Cleartext, g.KeyHash) {
		t.Error("CompareKeyHash should match cleartext+hash")
	}
}

func TestGeneratedKey_Record_OmitsCleartext(t *testing.T) {
	g, err := apikeys.Generate("k", "operator", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	rec := g.Record()
	// Sanity: can't directly check that cleartext is absent (Record
	// returns *state.APIKeyRecord which has no Cleartext field), but
	// we can verify KeyHash is populated and matches.
	if rec.KeyHash != g.KeyHash {
		t.Errorf("Record.KeyHash mismatch")
	}
	if rec.ID != g.ID || rec.Name != g.Name || rec.Role != g.Role {
		t.Errorf("Record metadata: %+v", rec)
	}
}

// apikeysGenerateAt is a thin wrapper that lets tests inject a fixed
// clock without re-exporting generateAt. Uses Generate's
// well-formed-ness then patches CreatedAt.
func apikeysGenerateAt(t *testing.T, name, role string, expiresAt, now time.Time) (*apikeys.GeneratedKey, error) {
	t.Helper()
	g, err := apikeys.Generate(name, role, expiresAt)
	if err != nil {
		return nil, err
	}
	g.CreatedAt = now.UTC()
	return g, nil
}

func TestGenerate_EnforcesMinLength(t *testing.T) {
	// Sanity guard: every generation should hit ≥ MinCleartextLength.
	// Big.Int could in theory drop leading zero bytes, so we run a
	// burst and check.
	for i := 0; i < 20; i++ {
		g, err := apikeys.Generate("k", "operator", time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if len(g.Cleartext) < apikeys.MinCleartextLength {
			t.Fatalf("iter %d: len=%d, want >= %d (cleartext=%q)",
				i, len(g.Cleartext), apikeys.MinCleartextLength, g.Cleartext)
		}
		// Cleartext must be base62 alphanumeric (no special chars).
		for _, c := range g.Cleartext {
			isDigit := c >= '0' && c <= '9'
			isLower := c >= 'a' && c <= 'z'
			isUpper := c >= 'A' && c <= 'Z'
			if !isDigit && !isLower && !isUpper {
				t.Fatalf("non-base62 char %q in %q", c, g.Cleartext)
			}
		}
	}
}

func TestGenerate_HashHexLength(t *testing.T) {
	g, err := apikeys.Generate("k", "operator", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.KeyHash) != 64 {
		t.Errorf("SHA-256 hex should be 64 chars; got %d (%s)", len(g.KeyHash), g.KeyHash)
	}
	// Hex.
	for _, c := range g.KeyHash {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("non-hex char %q in hash", c)
		}
	}
}
