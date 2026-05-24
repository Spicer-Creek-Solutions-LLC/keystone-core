// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/metadata"

	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

const hsSecret = "test-secret-do-not-use-in-prod"

func hsKeyFunc(_ *jwt.Token) (any, error) {
	return []byte(hsSecret), nil
}

func makeHSToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(hsSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func ctxWithJWT(token string) context.Context {
	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewIncomingContext(context.Background(), md)
}

func newJWTForTest(t *testing.T, cfg auth.JWTConfig) *auth.JWTAuthenticator {
	t.Helper()
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = hsKeyFunc
	}
	a, err := auth.NewJWTAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator: %v", err)
	}
	return a
}

func TestJWTAuthenticator_HS256_Success(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tok := makeHSToken(t, jwt.MapClaims{
		"sub":  "user-1",
		"name": "Alice",
		"role": "operator",
		"iat":  now.Unix(),
		"exp":  now.Add(time.Hour).Unix(),
	})

	a := newJWTForTest(t, auth.JWTConfig{})
	a.SetClock(func() time.Time { return now })

	got, err := a.Authenticate(ctxWithJWT(tok))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.ID != "user-1" || got.Name != "Alice" || got.Role != auth.RoleOperator {
		t.Errorf("principal: %+v", got)
	}
	if got.AuthMethod != auth.AuthMethodJWT {
		t.Errorf("AuthMethod = %v, want JWT", got.AuthMethod)
	}
}

func TestJWTAuthenticator_RS256_Success(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	claims := jwt.MapClaims{
		"sub":  "user-2",
		"role": "admin",
		"iat":  now.Unix(),
		"exp":  now.Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	a := newJWTForTest(t, auth.JWTConfig{
		KeyFunc: func(_ *jwt.Token) (any, error) { return &priv.PublicKey, nil },
	})
	a.SetClock(func() time.Time { return now })

	got, err := a.Authenticate(ctxWithJWT(signed))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.Role != auth.RoleAdmin {
		t.Errorf("Role = %v, want admin", got.Role)
	}
}

func TestJWTAuthenticator_NoBearer(t *testing.T) {
	a := newJWTForTest(t, auth.JWTConfig{})
	_, err := a.Authenticate(context.Background())
	if !errors.Is(err, auth.ErrCredentialsNotFound) {
		t.Errorf("err = %v, want ErrCredentialsNotFound", err)
	}
}

func TestJWTAuthenticator_NonJWTSkipped(t *testing.T) {
	// Token without two dots looks like an API key.
	a := newJWTForTest(t, auth.JWTConfig{})
	_, err := a.Authenticate(ctxWithJWT("just-a-key"))
	if !errors.Is(err, auth.ErrCredentialsNotFound) {
		t.Errorf("err = %v, want ErrCredentialsNotFound", err)
	}
}

func TestJWTAuthenticator_Expired(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tok := makeHSToken(t, jwt.MapClaims{
		"sub":  "user-1",
		"role": "operator",
		"exp":  now.Add(-time.Hour).Unix(),
	})

	a := newJWTForTest(t, auth.JWTConfig{})
	a.SetClock(func() time.Time { return now })

	_, err := a.Authenticate(ctxWithJWT(tok))
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestJWTAuthenticator_BadSignature(t *testing.T) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "user-1",
		"role": "operator",
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	signed, err := tok.SignedString([]byte("wrong-secret"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	a := newJWTForTest(t, auth.JWTConfig{})
	_, err = a.Authenticate(ctxWithJWT(signed))
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestJWTAuthenticator_MissingRoleClaim_Strict(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tok := makeHSToken(t, jwt.MapClaims{
		"sub": "user-1",
		"exp": now.Add(time.Hour).Unix(),
	})

	a := newJWTForTest(t, auth.JWTConfig{}) // AllowReadonlyOnNoRoleClaim = false
	a.SetClock(func() time.Time { return now })

	_, err := a.Authenticate(ctxWithJWT(tok))
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestJWTAuthenticator_MissingRoleClaim_Fallback(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tok := makeHSToken(t, jwt.MapClaims{
		"sub": "user-1",
		"exp": now.Add(time.Hour).Unix(),
	})

	a := newJWTForTest(t, auth.JWTConfig{AllowReadonlyOnNoRoleClaim: true})
	a.SetClock(func() time.Time { return now })

	got, err := a.Authenticate(ctxWithJWT(tok))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.Role != auth.RoleReadonly {
		t.Errorf("Role = %v, want readonly fallback", got.Role)
	}
}

func TestJWTAuthenticator_InvalidRoleClaim(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tok := makeHSToken(t, jwt.MapClaims{
		"sub":  "user-1",
		"role": "superuser", // not a known role
		"exp":  now.Add(time.Hour).Unix(),
	})

	a := newJWTForTest(t, auth.JWTConfig{AllowReadonlyOnNoRoleClaim: true})
	a.SetClock(func() time.Time { return now })

	// Per PROJECT-DETAILS §4.10 gotcha: invalid string -> reject (don't default).
	_, err := a.Authenticate(ctxWithJWT(tok))
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials (invalid role must reject)", err)
	}
}

func TestJWTAuthenticator_MissingSub(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tok := makeHSToken(t, jwt.MapClaims{
		"role": "operator",
		"exp":  now.Add(time.Hour).Unix(),
	})

	a := newJWTForTest(t, auth.JWTConfig{})
	a.SetClock(func() time.Time { return now })

	_, err := a.Authenticate(ctxWithJWT(tok))
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestJWTAuthenticator_AudienceMismatch(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tok := makeHSToken(t, jwt.MapClaims{
		"sub":  "user-1",
		"role": "operator",
		"exp":  now.Add(time.Hour).Unix(),
		"aud":  []string{"other-service"},
	})

	a := newJWTForTest(t, auth.JWTConfig{ExpectedAudiences: []string{"kscore"}})
	a.SetClock(func() time.Time { return now })

	_, err := a.Authenticate(ctxWithJWT(tok))
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestJWTAuthenticator_AudienceMatch(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tok := makeHSToken(t, jwt.MapClaims{
		"sub":  "user-1",
		"role": "operator",
		"exp":  now.Add(time.Hour).Unix(),
		"aud":  []string{"kscore", "other"},
	})

	a := newJWTForTest(t, auth.JWTConfig{ExpectedAudiences: []string{"kscore"}})
	a.SetClock(func() time.Time { return now })

	if _, err := a.Authenticate(ctxWithJWT(tok)); err != nil {
		t.Errorf("err = %v", err)
	}
}

func TestJWTAuthenticator_ExpectedIssuer(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tok := makeHSToken(t, jwt.MapClaims{
		"sub":  "user-1",
		"role": "operator",
		"iss":  "wrong-issuer",
		"exp":  now.Add(time.Hour).Unix(),
	})

	a := newJWTForTest(t, auth.JWTConfig{ExpectedIssuer: "expected-issuer"})
	a.SetClock(func() time.Time { return now })

	if _, err := a.Authenticate(ctxWithJWT(tok)); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestNewJWTAuthenticator_RequiresKeyFunc(t *testing.T) {
	if _, err := auth.NewJWTAuthenticator(auth.JWTConfig{}); err == nil {
		t.Error("expected error for nil KeyFunc")
	}
}
