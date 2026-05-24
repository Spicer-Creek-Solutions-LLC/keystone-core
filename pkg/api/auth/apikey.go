// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/metadata"
)

// KeyVerifier is the contract APIKeyAuthenticator depends on for
// looking up + comparing API keys. Task 5 (pkg/api/apikeys) provides
// the concrete in-memory + SQLite store; this interface lets
// pkg/api/auth stay backend-agnostic and self-testable.
type KeyVerifier interface {
	// VerifyKey takes the cleartext key presented by the client and
	// returns the matching VerifiedKey or ErrInvalidCredentials if
	// no enabled key matches.
	//
	// Implementations MUST use a constant-time hash comparison
	// (subtle.ConstantTimeCompare) to defeat timing-side-channels.
	VerifyKey(ctx context.Context, cleartext string) (*VerifiedKey, error)
}

// VerifiedKey is what a KeyVerifier returns on a successful match.
// Excludes the cleartext value (KV is verifier-internal).
type VerifiedKey struct {
	ID        string
	Name      string
	Role      Role
	ExpiresAt time.Time // zero = never expires
}

// APIKeyAuthenticator pulls a Bearer token from the request metadata,
// looks it up via the KeyVerifier, and returns the corresponding
// Principal.
//
// Recognises both "Bearer <key>" and (for compatibility with
// kscorectl scripts that put the key directly) bare "<key>" header
// values. Empty / non-bearer auth headers => ErrCredentialsNotFound.
type APIKeyAuthenticator struct {
	verifier KeyVerifier
	now      func() time.Time
}

// NewAPIKeyAuthenticator returns an Authenticator backed by verifier.
func NewAPIKeyAuthenticator(verifier KeyVerifier) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{verifier: verifier, now: time.Now}
}

// SetClock overrides the clock used for expiry checks. Tests only.
func (a *APIKeyAuthenticator) SetClock(now func() time.Time) {
	a.now = now
}

// Authenticate extracts the API key from the gRPC `authorization`
// metadata or the HTTP-equivalent header (set by the HTTP middleware
// before reaching this layer) and returns the corresponding Principal.
func (a *APIKeyAuthenticator) Authenticate(ctx context.Context) (*Principal, error) {
	cleartext, ok := extractBearerToken(ctx)
	if !ok {
		return nil, ErrCredentialsNotFound
	}
	if !looksLikeAPIKey(cleartext) {
		// The chain may have a JWTAuthenticator behind us; cleartext
		// that looks JWT-shaped is not our credential.
		return nil, ErrCredentialsNotFound
	}

	vk, err := a.verifier.VerifyKey(ctx, cleartext)
	if err != nil {
		return nil, err
	}
	if !vk.ExpiresAt.IsZero() && a.now().After(vk.ExpiresAt) {
		return nil, fmt.Errorf("%w: api key expired", ErrInvalidCredentials)
	}

	return &Principal{
		ID:              vk.ID,
		Name:            vk.Name,
		Role:            vk.Role,
		AuthMethod:      AuthMethodAPIKey,
		AuthenticatedAt: a.now().UTC(),
	}, nil
}

// HashAPIKey returns a hex-encoded SHA-256 hash of cleartext, suitable
// for persistence in an APIKeyStore. Storage holds only the hash;
// verification re-hashes the inbound cleartext and compares with
// constant-time semantics.
func HashAPIKey(cleartext string) string {
	sum := sha256.Sum256([]byte(cleartext))
	return hex.EncodeToString(sum[:])
}

// CompareKeyHash reports whether the cleartext hashes to the same
// value as expectedHash, in constant time.
func CompareKeyHash(cleartext, expectedHash string) bool {
	got := HashAPIKey(cleartext)
	return subtle.ConstantTimeCompare([]byte(got), []byte(expectedHash)) == 1
}

// extractBearerToken pulls the bearer value from the inbound
// authorization metadata. Returns ("", false) if absent or malformed.
func extractBearerToken(ctx context.Context) (string, bool) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", false
	}
	values := md.Get("authorization")
	if len(values) == 0 {
		return "", false
	}
	header := values[0]
	if header == "" {
		return "", false
	}
	const bearerPrefix = "Bearer "
	if strings.HasPrefix(header, bearerPrefix) {
		token := strings.TrimSpace(header[len(bearerPrefix):])
		if token == "" {
			return "", false
		}
		return token, true
	}
	// Tolerate bare-token headers for backwards compatibility with
	// kscorectl scripts that historically set just the key value.
	return strings.TrimSpace(header), true
}

// looksLikeAPIKey discriminates API keys from JWTs by shape: JWTs are
// three base64url segments separated by dots. Anything else routes to
// the API-key authenticator. Edge case: an API key that contains two
// dots will be misrouted to JWT — generated keys (epic 03 task 5)
// use base62, which excludes the dot character, so this is safe.
func looksLikeAPIKey(token string) bool {
	return strings.Count(token, ".") != 2
}
