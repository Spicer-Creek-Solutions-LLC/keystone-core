// SPDX-License-Identifier: Apache-2.0

// Package apikeys generates, stores, and verifies API keys for the
// Keystone Core HTTP/gRPC API surface. See PROJECT-DETAILS §4.10.
//
// The package layers on top of internal/state's APIKeyStore (storage)
// and pkg/api/auth's KeyVerifier (auth chain). Cleartext values are
// returned to operators exactly once on creation and never persisted;
// storage holds only the SHA-256 hash.
package apikeys

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"

	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// MinCleartextLength is the floor on generated key length per
// PROJECT-DETAILS §4.10 ("random base62 ≥ 32 chars"). 32 bytes of
// entropy encodes to roughly 43 base62 chars, well above the floor.
const MinCleartextLength = 32

// GeneratedKey is the result of Generate. Cleartext is the only place
// the unhashed key appears anywhere in the codebase — caller MUST
// surface it to the operator immediately and discard the field.
// KeyHash is what gets persisted via APIKeyStore.CreateAPIKey.
type GeneratedKey struct {
	ID        string
	Name      string
	Role      string
	Cleartext string // !! sensitive: present only on creation, never persisted !!
	KeyHash   string
	CreatedAt time.Time
	ExpiresAt time.Time // zero = never expires
}

// Record returns the persistence shape of g — same fields as the
// GeneratedKey minus Cleartext, ready to pass to CreateAPIKey.
func (g *GeneratedKey) Record() *state.APIKeyRecord {
	return &state.APIKeyRecord{
		ID:        g.ID,
		Name:      g.Name,
		KeyHash:   g.KeyHash,
		Role:      g.Role,
		CreatedAt: g.CreatedAt,
		ExpiresAt: g.ExpiresAt,
	}
}

// Generate produces a fresh API key for the given name + role.
// expiresAt may be zero (never expires).
//
// The cleartext is base62-encoded random with ≥32 chars. The hash is
// SHA-256 hex via auth.HashAPIKey. Caller is responsible for
// persisting the record (Record()) and surfacing the cleartext to the
// operator exactly once.
func Generate(name, role string, expiresAt time.Time) (*GeneratedKey, error) {
	return generateAt(name, role, expiresAt, time.Now)
}

// generateAt is the test-friendly shape; production callers use Generate.
func generateAt(name, role string, expiresAt time.Time, now func() time.Time) (*GeneratedKey, error) {
	if name == "" {
		return nil, errors.New("apikeys: Name is required")
	}
	r, err := auth.ParseRole(role)
	if err != nil {
		return nil, fmt.Errorf("apikeys: %w", err)
	}
	if r == auth.RoleNone {
		return nil, errors.New("apikeys: Role is required")
	}

	cleartext, err := randomBase62(32)
	if err != nil {
		return nil, fmt.Errorf("apikeys: random: %w", err)
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("apikeys: id: %w", err)
	}

	return &GeneratedKey{
		ID:        id.String(),
		Name:      name,
		Role:      r.String(),
		Cleartext: cleartext,
		KeyHash:   auth.HashAPIKey(cleartext),
		CreatedAt: now().UTC(),
		ExpiresAt: expiresAt,
	}, nil
}

// randomBase62 returns a base62-encoded random string from byteLength
// bytes of crypto/rand entropy. Output length is ceil(byteLength * 8 /
// log2(62)) ≈ byteLength * 1.343 chars. For byteLength=32 that's
// typically 43 chars (≥ MinCleartextLength).
//
// Uses big.Int.Text(62) for unbiased encoding (alphabet 0-9a-zA-Z,
// in that order). Big.Int strips leading zero bytes — extremely
// unlikely for 32 random bytes, but pad to MinCleartextLength as a
// belt-and-suspenders guarantee.
func randomBase62(byteLength int) (string, error) {
	b := make([]byte, byteLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := new(big.Int).SetBytes(b).Text(62)
	for len(s) < MinCleartextLength {
		s = "0" + s
	}
	return s, nil
}
