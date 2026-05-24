// SPDX-License-Identifier: Apache-2.0

package auth_test

import (
	"context"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// stubAuthenticator returns the configured (principal, err) on every call.
type stubAuthenticator struct {
	principal *auth.Principal
	err       error
}

func (s *stubAuthenticator) Authenticate(_ context.Context) (*auth.Principal, error) {
	return s.principal, s.err
}

func TestChain_FirstSuccessWins(t *testing.T) {
	want := &auth.Principal{ID: "via-second"}
	chain := auth.NewChain(
		&stubAuthenticator{err: auth.ErrCredentialsNotFound},
		&stubAuthenticator{principal: want},
		&stubAuthenticator{err: auth.ErrInvalidCredentials}, // never reached
	)
	got, err := chain.Authenticate(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestChain_InvalidShortCircuits(t *testing.T) {
	// Mid-chain ErrInvalidCredentials must surface immediately —
	// don't fall through to a later authenticator that might succeed.
	chain := auth.NewChain(
		&stubAuthenticator{err: auth.ErrCredentialsNotFound},
		&stubAuthenticator{err: auth.ErrInvalidCredentials},
		&stubAuthenticator{principal: &auth.Principal{ID: "should-not-reach"}},
	)
	_, err := chain.Authenticate(context.Background())
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestChain_AllSkippedReturnsUnauthenticated(t *testing.T) {
	chain := auth.NewChain(
		&stubAuthenticator{err: auth.ErrCredentialsNotFound},
		&stubAuthenticator{err: auth.ErrCredentialsNotFound},
	)
	_, err := chain.Authenticate(context.Background())
	if !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("err = %v, want ErrUnauthenticated", err)
	}
}

func TestChain_EmptyReturnsUnauthenticated(t *testing.T) {
	chain := auth.NewChain()
	_, err := chain.Authenticate(context.Background())
	if !errors.Is(err, auth.ErrUnauthenticated) {
		t.Errorf("err = %v, want ErrUnauthenticated", err)
	}
}

func TestChain_OtherErrorShortCircuits(t *testing.T) {
	// Errors that aren't ErrCredentialsNotFound (and aren't
	// ErrInvalidCredentials specifically) should surface unchanged.
	custom := errors.New("custom: token store unavailable")
	chain := auth.NewChain(
		&stubAuthenticator{err: custom},
		&stubAuthenticator{principal: &auth.Principal{ID: "later"}},
	)
	_, err := chain.Authenticate(context.Background())
	if err != custom {
		t.Errorf("err = %v, want %v (verbatim)", err, custom)
	}
}
