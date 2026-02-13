package token

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateToken(t *testing.T) {
	tok, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	if !strings.HasPrefix(tok, TokenPrefix) {
		t.Errorf("token %q does not have prefix %q", tok, TokenPrefix)
	}

	randomPart := strings.TrimPrefix(tok, TokenPrefix)
	if len(randomPart) != TokenLength {
		t.Errorf("random part length = %d, want %d", len(randomPart), TokenLength)
	}

	for _, c := range randomPart {
		if !strings.ContainsRune(base62Chars, c) {
			t.Errorf("token contains non-base62 character: %c", c)
		}
	}
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		tok, err := GenerateToken()
		if err != nil {
			t.Fatalf("GenerateToken() error: %v", err)
		}
		if seen[tok] {
			t.Fatalf("duplicate token generated: %s", tok)
		}
		seen[tok] = true
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	raw := "kscore-join-abc123"
	salt := "deadbeef"

	h1 := HashToken(raw, salt)
	h2 := HashToken(raw, salt)
	if h1 != h2 {
		t.Errorf("HashToken not deterministic: %s != %s", h1, h2)
	}

	if len(h1) != 64 {
		t.Errorf("hash length = %d, want 64 (SHA-256 hex)", len(h1))
	}
}

func TestHashToken_SaltChangesHash(t *testing.T) {
	raw := "kscore-join-abc123"
	h1 := HashToken(raw, "salt1")
	h2 := HashToken(raw, "salt2")
	if h1 == h2 {
		t.Error("different salts produced identical hashes")
	}
}

func TestHashToken_DifferentTokens(t *testing.T) {
	salt := "same-salt"
	h1 := HashToken("token-a", salt)
	h2 := HashToken("token-b", salt)
	if h1 == h2 {
		t.Error("different tokens produced identical hashes")
	}
}

func TestGenerateSalt(t *testing.T) {
	s1, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error: %v", err)
	}
	if len(s1) != 32 {
		t.Errorf("salt length = %d, want 32 (16 bytes hex)", len(s1))
	}

	s2, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt() error: %v", err)
	}
	if s1 == s2 {
		t.Error("two salts are identical")
	}
}

func TestGenerateID(t *testing.T) {
	id1, err := GenerateID()
	if err != nil {
		t.Fatalf("GenerateID() error: %v", err)
	}
	if len(id1) != 32 {
		t.Errorf("ID length = %d, want 32 (16 bytes hex)", len(id1))
	}

	id2, err := GenerateID()
	if err != nil {
		t.Fatalf("GenerateID() error: %v", err)
	}
	if id1 == id2 {
		t.Error("two IDs are identical")
	}
}

func TestNewJoinToken(t *testing.T) {
	tok, raw, err := NewJoinToken("test-label", "admin", time.Hour, 5)
	if err != nil {
		t.Fatalf("NewJoinToken() error: %v", err)
	}

	if tok.ID == "" {
		t.Error("token ID is empty")
	}
	if tok.Label != "test-label" {
		t.Errorf("Label = %q, want %q", tok.Label, "test-label")
	}
	if tok.CreatedBy != "admin" {
		t.Errorf("CreatedBy = %q, want %q", tok.CreatedBy, "admin")
	}
	if tok.MaxUses != 5 {
		t.Errorf("MaxUses = %d, want 5", tok.MaxUses)
	}
	if tok.UsedCount != 0 {
		t.Errorf("UsedCount = %d, want 0", tok.UsedCount)
	}
	if tok.Revoked {
		t.Error("token should not be revoked")
	}
	if tok.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if tok.ExpiresAt.IsZero() {
		t.Error("ExpiresAt is zero")
	}
	if tok.ExpiresAt.Before(tok.CreatedAt) {
		t.Error("ExpiresAt before CreatedAt")
	}

	if !strings.HasPrefix(raw, TokenPrefix) {
		t.Errorf("raw token %q missing prefix", raw)
	}

	if HashToken(raw, tok.Salt) != tok.TokenHash {
		t.Error("token hash does not match hash of raw token with stored salt")
	}
}

func TestNewJoinToken_DefaultTTL(t *testing.T) {
	tok, _, err := NewJoinToken("", "", 0, 0)
	if err != nil {
		t.Fatalf("NewJoinToken() error: %v", err)
	}

	expectedDuration := DefaultTTL
	actual := tok.ExpiresAt.Sub(tok.CreatedAt)
	diff := actual - expectedDuration
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("TTL = %v, want ~%v", actual, expectedDuration)
	}
}

func TestRandomBase62(t *testing.T) {
	s, err := randomBase62(100)
	if err != nil {
		t.Fatalf("randomBase62() error: %v", err)
	}
	if len(s) != 100 {
		t.Errorf("length = %d, want 100", len(s))
	}
	for _, c := range s {
		if !strings.ContainsRune(base62Chars, c) {
			t.Errorf("non-base62 character: %c", c)
		}
	}
}
