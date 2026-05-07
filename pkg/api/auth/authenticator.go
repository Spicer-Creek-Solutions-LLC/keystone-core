package auth

import (
	"context"
	"errors"
)

// Authenticator extracts a Principal from the inbound RPC context.
//
// Implementations must distinguish two failure modes via the sentinel
// errors below:
//
//   - ErrCredentialsNotFound: this authenticator's credential type is
//     not present in the context. The Chain proceeds to the next
//     authenticator.
//
//   - ErrInvalidCredentials: the credential type is present but
//     fails validation (bad signature, expired, malformed, …). The
//     Chain stops immediately and surfaces the error.
//
// Any other error is treated as ErrInvalidCredentials by the Chain.
type Authenticator interface {
	Authenticate(ctx context.Context) (*Principal, error)
}

// Sentinel errors returned by Authenticator implementations.
var (
	// ErrCredentialsNotFound signals "not my credential type"; the
	// chain continues to the next authenticator.
	ErrCredentialsNotFound = errors.New("auth: credentials not found")

	// ErrInvalidCredentials signals "my credential type, but it's
	// invalid"; the chain stops and surfaces this error.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")

	// ErrUnauthenticated is returned by Chain when no authenticator
	// produced a Principal.
	ErrUnauthenticated = errors.New("auth: unauthenticated")
)

// Chain composes multiple Authenticators. On Authenticate, it tries
// each in order: the first to return a Principal wins; the first to
// return a non-CredentialsNotFound error short-circuits.
type Chain struct {
	authenticators []Authenticator
}

// NewChain returns a Chain over authenticators. Order matters — put
// cheap-to-check, high-confidence methods first (e.g., mTLS, which
// has already happened at the TLS handshake by the time the request
// reaches the auth layer).
func NewChain(authenticators ...Authenticator) *Chain {
	return &Chain{authenticators: authenticators}
}

// Authenticate iterates the chain. Returns the first successful
// Principal; or, on validation failure, the first
// non-CredentialsNotFound error; or ErrUnauthenticated if every
// authenticator skipped.
func (c *Chain) Authenticate(ctx context.Context) (*Principal, error) {
	for _, a := range c.authenticators {
		p, err := a.Authenticate(ctx)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, ErrCredentialsNotFound) {
			return nil, err
		}
	}
	return nil, ErrUnauthenticated
}
