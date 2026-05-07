package auth_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// stubVerifier is an in-memory KeyVerifier for tests.
type stubVerifier struct {
	keys      map[string]*auth.VerifiedKey
	calls     atomic.Int32
	returnErr error
}

func (s *stubVerifier) VerifyKey(_ context.Context, cleartext string) (*auth.VerifiedKey, error) {
	s.calls.Add(1)
	if s.returnErr != nil {
		return nil, s.returnErr
	}
	vk, ok := s.keys[cleartext]
	if !ok {
		return nil, auth.ErrInvalidCredentials
	}
	return vk, nil
}

func ctxWithBearer(token string) context.Context {
	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestAPIKeyAuthenticator_Success(t *testing.T) {
	verifier := &stubVerifier{
		keys: map[string]*auth.VerifiedKey{
			"key-abc123": {ID: "k-1", Name: "ops", Role: auth.RoleOperator},
		},
	}
	a := auth.NewAPIKeyAuthenticator(verifier)

	got, err := a.Authenticate(ctxWithBearer("key-abc123"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.ID != "k-1" || got.Name != "ops" || got.Role != auth.RoleOperator {
		t.Errorf("principal: %+v", got)
	}
	if got.AuthMethod != auth.AuthMethodAPIKey {
		t.Errorf("AuthMethod = %v, want AuthMethodAPIKey", got.AuthMethod)
	}
	if got.AuthenticatedAt.IsZero() {
		t.Error("AuthenticatedAt should be set")
	}
}

func TestAPIKeyAuthenticator_NoMetadata(t *testing.T) {
	a := auth.NewAPIKeyAuthenticator(&stubVerifier{})
	_, err := a.Authenticate(context.Background())
	if !errors.Is(err, auth.ErrCredentialsNotFound) {
		t.Errorf("err = %v, want ErrCredentialsNotFound", err)
	}
}

func TestAPIKeyAuthenticator_NoAuthHeader(t *testing.T) {
	md := metadata.Pairs("user-agent", "test")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	a := auth.NewAPIKeyAuthenticator(&stubVerifier{})
	_, err := a.Authenticate(ctx)
	if !errors.Is(err, auth.ErrCredentialsNotFound) {
		t.Errorf("err = %v, want ErrCredentialsNotFound", err)
	}
}

func TestAPIKeyAuthenticator_JWTShapedSkipped(t *testing.T) {
	// Three dot-segments looks like a JWT — should be skipped so the
	// JWTAuthenticator next in the chain can handle it.
	a := auth.NewAPIKeyAuthenticator(&stubVerifier{})
	ctx := ctxWithBearer("eyJh.eyJh.sig")
	_, err := a.Authenticate(ctx)
	if !errors.Is(err, auth.ErrCredentialsNotFound) {
		t.Errorf("err = %v, want ErrCredentialsNotFound", err)
	}
}

func TestAPIKeyAuthenticator_Invalid(t *testing.T) {
	a := auth.NewAPIKeyAuthenticator(&stubVerifier{})
	_, err := a.Authenticate(ctxWithBearer("nope"))
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestAPIKeyAuthenticator_Expired(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	verifier := &stubVerifier{
		keys: map[string]*auth.VerifiedKey{
			"key-old": {ID: "k-1", Role: auth.RoleAdmin, ExpiresAt: now.Add(-time.Hour)},
		},
	}
	a := auth.NewAPIKeyAuthenticator(verifier)
	a.SetClock(func() time.Time { return now })

	_, err := a.Authenticate(ctxWithBearer("key-old"))
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestAPIKeyAuthenticator_BareTokenAccepted(t *testing.T) {
	// Header without "Bearer " prefix should still resolve.
	verifier := &stubVerifier{
		keys: map[string]*auth.VerifiedKey{
			"key-bare": {ID: "k-1", Role: auth.RoleReadonly},
		},
	}
	a := auth.NewAPIKeyAuthenticator(verifier)

	md := metadata.Pairs("authorization", "key-bare")
	ctx := metadata.NewIncomingContext(context.Background(), md)
	got, err := a.Authenticate(ctx)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.ID != "k-1" {
		t.Errorf("principal: %+v", got)
	}
}

func TestAPIKeyAuthenticator_EmptyBearer(t *testing.T) {
	a := auth.NewAPIKeyAuthenticator(&stubVerifier{})
	md := metadata.Pairs("authorization", "Bearer ")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := a.Authenticate(ctx)
	if !errors.Is(err, auth.ErrCredentialsNotFound) {
		t.Errorf("err = %v, want ErrCredentialsNotFound", err)
	}
}

func TestHashAPIKey_Stable(t *testing.T) {
	got1 := auth.HashAPIKey("the-key")
	got2 := auth.HashAPIKey("the-key")
	if got1 != got2 {
		t.Errorf("hash should be stable; %q vs %q", got1, got2)
	}
	if got1 == auth.HashAPIKey("different") {
		t.Error("different inputs must hash differently")
	}
	if len(got1) != 64 {
		t.Errorf("SHA-256 hex should be 64 chars; got %d", len(got1))
	}
}

func TestCompareKeyHash(t *testing.T) {
	cleartext := "the-key"
	hash := auth.HashAPIKey(cleartext)

	if !auth.CompareKeyHash(cleartext, hash) {
		t.Error("matching cleartext+hash should compare equal")
	}
	if auth.CompareKeyHash("wrong", hash) {
		t.Error("non-matching cleartext should compare unequal")
	}
	if auth.CompareKeyHash(cleartext, "wronghash") {
		t.Error("non-matching hash should compare unequal")
	}
}
