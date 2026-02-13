package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"
)

const base62Chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// GenerateToken produces a token string of the form "kscore-join-<32 base62 chars>".
func GenerateToken() (string, error) {
	random, err := randomBase62(TokenLength)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	return TokenPrefix + random, nil
}

// HashToken computes a salted SHA-256 hash of the token, returned as hex.
func HashToken(rawToken, salt string) string {
	h := sha256.New()
	h.Write([]byte(salt))
	h.Write([]byte(rawToken))
	return hex.EncodeToString(h.Sum(nil))
}

// GenerateSalt produces 16 random bytes, hex-encoded.
func GenerateSalt() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateID produces a 16-byte random hex-encoded identifier.
func GenerateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate ID: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// NewJoinToken creates a JoinToken and returns both the struct and the raw
// plaintext token. The raw token is only available at creation time.
func NewJoinToken(label, createdBy string, ttl time.Duration, maxUses int) (*JoinToken, string, error) {
	rawToken, err := GenerateToken()
	if err != nil {
		return nil, "", err
	}

	salt, err := GenerateSalt()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate salt: %w", err)
	}

	id, err := GenerateID()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate ID: %w", err)
	}

	if ttl <= 0 {
		ttl = DefaultTTL
	}

	now := time.Now()
	t := &JoinToken{
		ID:        id,
		TokenHash: HashToken(rawToken, salt),
		Salt:      salt,
		Label:     label,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
		MaxUses:   maxUses,
		UsedCount: 0,
		CreatedBy: createdBy,
		Revoked:   false,
	}

	return t, rawToken, nil
}

// randomBase62 generates n unbiased random base62 characters using rejection
// sampling via crypto/rand.
func randomBase62(n int) (string, error) {
	charset := big.NewInt(int64(len(base62Chars)))
	result := make([]byte, n)
	for i := 0; i < n; i++ {
		idx, err := rand.Int(rand.Reader, charset)
		if err != nil {
			return "", err
		}
		result[i] = base62Chars[idx.Int64()]
	}
	return string(result), nil
}
