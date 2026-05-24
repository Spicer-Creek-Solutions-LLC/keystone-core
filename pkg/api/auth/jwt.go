// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTAuthenticator validates a Bearer JWT and constructs a Principal
// from its claims.
//
// Supports HS256, RS256, and ES256 via a caller-supplied jwt.Keyfunc.
// The Keyfunc receives the parsed token and returns the verifying
// key — same shape as the upstream library so existing key-loaders
// (PEM, JWKS, etc.) drop in directly.
type JWTAuthenticator struct {
	keyFunc            jwt.Keyfunc
	expectedIssuer     string // empty = don't enforce iss claim
	expectedAudiences  []string
	allowReadonlyOnNoRoleClaim bool // PROJECT-DETAILS §4.10 gotcha
	now                func() time.Time
}

// JWTConfig configures a JWTAuthenticator at construction.
type JWTConfig struct {
	// KeyFunc returns the verifying key for the token. Required.
	KeyFunc jwt.Keyfunc

	// ExpectedIssuer, if non-empty, must match the token's iss claim.
	ExpectedIssuer string

	// ExpectedAudiences, if non-empty, must contain at least one of
	// the token's aud values.
	ExpectedAudiences []string

	// AllowReadonlyOnNoRoleClaim controls the missing-role-claim
	// fallback. PROJECT-DETAILS §4.10 gotcha: missing role -> readonly
	// fallback (with caller-side warning); invalid role -> reject
	// outright. Default false (reject when role is missing) for
	// strictness; set true for the legacy permissive behavior.
	AllowReadonlyOnNoRoleClaim bool
}

// NewJWTAuthenticator returns an Authenticator backed by config.
func NewJWTAuthenticator(cfg JWTConfig) (*JWTAuthenticator, error) {
	if cfg.KeyFunc == nil {
		return nil, fmt.Errorf("auth: JWTConfig.KeyFunc is required")
	}
	return &JWTAuthenticator{
		keyFunc:                    cfg.KeyFunc,
		expectedIssuer:             cfg.ExpectedIssuer,
		expectedAudiences:          cfg.ExpectedAudiences,
		allowReadonlyOnNoRoleClaim: cfg.AllowReadonlyOnNoRoleClaim,
		now:                        time.Now,
	}, nil
}

// SetClock overrides the clock used for expiry checks. Tests only.
func (a *JWTAuthenticator) SetClock(now func() time.Time) {
	a.now = now
}

// Authenticate extracts the JWT from the request and parses it.
func (a *JWTAuthenticator) Authenticate(ctx context.Context) (*Principal, error) {
	tokenStr, ok := extractBearerToken(ctx)
	if !ok {
		return nil, ErrCredentialsNotFound
	}
	if strings.Count(tokenStr, ".") != 2 {
		// Looks like an API key, not a JWT — let the chain try the
		// next authenticator.
		return nil, ErrCredentialsNotFound
	}

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"HS256", "RS256", "ES256"}),
		jwt.WithTimeFunc(a.now),
	)
	if a.expectedIssuer != "" {
		parser = jwt.NewParser(
			jwt.WithValidMethods([]string{"HS256", "RS256", "ES256"}),
			jwt.WithTimeFunc(a.now),
			jwt.WithIssuer(a.expectedIssuer),
		)
	}

	claims := jwt.MapClaims{}
	if _, err := parser.ParseWithClaims(tokenStr, claims, a.keyFunc); err != nil {
		return nil, fmt.Errorf("%w: jwt parse: %w", ErrInvalidCredentials, err)
	}

	if len(a.expectedAudiences) > 0 {
		if !audienceMatches(claims, a.expectedAudiences) {
			return nil, fmt.Errorf("%w: jwt audience mismatch", ErrInvalidCredentials)
		}
	}

	role, err := roleFromClaims(claims, a.allowReadonlyOnNoRoleClaim)
	if err != nil {
		return nil, err
	}

	id := stringClaim(claims, "sub")
	if id == "" {
		return nil, fmt.Errorf("%w: jwt missing sub claim", ErrInvalidCredentials)
	}

	return &Principal{
		ID:              id,
		Name:            stringClaim(claims, "name"),
		Role:            role,
		AuthMethod:      AuthMethodJWT,
		AuthenticatedAt: a.now().UTC(),
	}, nil
}

// audienceMatches reports whether at least one token audience appears
// in the expected list.
func audienceMatches(claims jwt.MapClaims, expected []string) bool {
	got, _ := claims.GetAudience()
	want := make(map[string]bool, len(expected))
	for _, a := range expected {
		want[a] = true
	}
	for _, a := range got {
		if want[a] {
			return true
		}
	}
	return false
}

// roleFromClaims maps the "role" claim to a Role per PROJECT-DETAILS
// §4.10. Missing claim -> readonly fallback if AllowReadonlyOnNoRoleClaim
// is set, else reject. Invalid string -> reject outright.
func roleFromClaims(claims jwt.MapClaims, allowReadonlyFallback bool) (Role, error) {
	raw, exists := claims["role"]
	if !exists {
		if allowReadonlyFallback {
			return RoleReadonly, nil
		}
		return RoleNone, fmt.Errorf("%w: jwt missing role claim", ErrInvalidCredentials)
	}
	s, ok := raw.(string)
	if !ok {
		return RoleNone, fmt.Errorf("%w: jwt role claim is not a string", ErrInvalidCredentials)
	}
	role, err := ParseRole(s)
	if err != nil {
		return RoleNone, fmt.Errorf("%w: %w", ErrInvalidCredentials, err)
	}
	if role == RoleNone {
		// Empty string parsed as RoleNone is also a reject.
		return RoleNone, fmt.Errorf("%w: jwt role claim is empty", ErrInvalidCredentials)
	}
	return role, nil
}

// stringClaim returns the claim as a string, or "" if missing/wrong type.
func stringClaim(claims jwt.MapClaims, key string) string {
	v, ok := claims[key].(string)
	if !ok {
		return ""
	}
	return v
}
