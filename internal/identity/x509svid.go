package identity

import (
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidX509SVID wraps every rejection [NewX509SVID] returns.
// Callers branch with [errors.Is]; the wrapped underlying error
// describes the specific shape problem (chain empty, key
// mismatched, URI SAN missing, …).
var ErrInvalidX509SVID = errors.New("identity: invalid X509SVID")

// X509SVID is one issued x509 SPIFFE Verifiable Identity: a leaf
// certificate whose URI SAN carries the SPIFFE ID, any intermediate
// CAs needed to chain it to a trust bundle, and the leaf's private
// key. Hint is the SPIFFE-Workload-API disambiguator for multiple
// concurrent SVIDs at the same ID (e.g. "rotating-old" /
// "rotating-new"); empty Hint is the common case.
//
// This is the in-process surface tasks 5-7 produce — [CAManager]
// (task 5) issues the chain, [EmbeddedProvider] (task 7) wraps it
// in an X509SVID. Tasks 13-14 consume it: the mTLS peer extractor
// reads the URI SAN; the gRPC server uses [X509SVID.Leaf] and
// [X509SVID.PrivateKey] for its TLS cert.
//
// Fields are unexported; the only path in is [NewX509SVID], which
// validates the shape. The type is comparable on the SPIFFEID +
// IssuedAt + ExpiresAt + Hint tuple via [X509SVID.Equal] — `==`
// is unsafe because the chain slice contains pointers.
type X509SVID struct {
	id        SPIFFEID
	chain     []*x509.Certificate // [leaf, intermediate...]
	privKey   crypto.Signer
	issuedAt  time.Time
	expiresAt time.Time
	hint      string
}

// NewX509SVID validates the chain + key against the expected
// SPIFFE id, and returns an immutable [X509SVID]. id MUST match
// the leaf's single URI SAN; the leaf's NotBefore / NotAfter set
// IssuedAt / ExpiresAt. hint is operator-supplied and may be "".
//
// Validation checks:
//   - chain has at least one certificate; every entry is non-nil
//   - leaf carries exactly one URI SAN
//   - the URI SAN parses as a SPIFFE ID and Equals `id`
//   - key is non-nil and its public half matches the leaf's
//   - leaf NotBefore ≤ NotAfter
func NewX509SVID(id SPIFFEID, chain []*x509.Certificate, key crypto.Signer, hint string) (X509SVID, error) {
	if id.IsZero() {
		return X509SVID{}, fmt.Errorf("%w: SPIFFE ID is required", ErrInvalidX509SVID)
	}
	if len(chain) == 0 {
		return X509SVID{}, fmt.Errorf("%w: chain is empty", ErrInvalidX509SVID)
	}
	for i, c := range chain {
		if c == nil {
			return X509SVID{}, fmt.Errorf("%w: chain[%d] is nil", ErrInvalidX509SVID, i)
		}
	}
	leaf := chain[0]
	if len(leaf.URIs) != 1 {
		return X509SVID{}, fmt.Errorf("%w: leaf must carry exactly one URI SAN, got %d", ErrInvalidX509SVID, len(leaf.URIs))
	}
	leafID, err := ParseSPIFFEID(leaf.URIs[0].String())
	if err != nil {
		return X509SVID{}, fmt.Errorf("%w: leaf URI SAN %q: %v", ErrInvalidX509SVID, leaf.URIs[0], err)
	}
	if !leafID.Equal(id) {
		return X509SVID{}, fmt.Errorf("%w: leaf URI SAN %q does not match id %q", ErrInvalidX509SVID, leafID, id)
	}
	if key == nil {
		return X509SVID{}, fmt.Errorf("%w: private key is required", ErrInvalidX509SVID)
	}
	if !publicKeysEqual(key.Public(), leaf.PublicKey) {
		return X509SVID{}, fmt.Errorf("%w: private key's public half does not match leaf certificate", ErrInvalidX509SVID)
	}
	if leaf.NotAfter.Before(leaf.NotBefore) {
		return X509SVID{}, fmt.Errorf("%w: leaf NotAfter %s is before NotBefore %s", ErrInvalidX509SVID, leaf.NotAfter, leaf.NotBefore)
	}
	return X509SVID{
		id:        id,
		chain:     append([]*x509.Certificate(nil), chain...), // defensive copy
		privKey:   key,
		issuedAt:  leaf.NotBefore,
		expiresAt: leaf.NotAfter,
		hint:      hint,
	}, nil
}

// SPIFFEID returns the ID the leaf URI SAN encoded. Zero value for
// a zero X509SVID.
func (s X509SVID) SPIFFEID() SPIFFEID { return s.id }

// Leaf returns the end-entity certificate (chain[0]). nil for the
// zero value.
func (s X509SVID) Leaf() *x509.Certificate {
	if len(s.chain) == 0 {
		return nil
	}
	return s.chain[0]
}

// Chain returns a defensive copy of the full chain [leaf,
// intermediate...]. Callers may mutate the returned slice freely.
// nil (not an empty slice) for the zero value.
func (s X509SVID) Chain() []*x509.Certificate {
	if len(s.chain) == 0 {
		return nil
	}
	out := make([]*x509.Certificate, len(s.chain))
	copy(out, s.chain)
	return out
}

// PrivateKey returns the leaf's private key as a [crypto.Signer]
// (the algorithm-agnostic interface). nil for the zero value.
func (s X509SVID) PrivateKey() crypto.Signer { return s.privKey }

// IssuedAt returns the leaf's NotBefore.
func (s X509SVID) IssuedAt() time.Time { return s.issuedAt }

// ExpiresAt returns the leaf's NotAfter.
func (s X509SVID) ExpiresAt() time.Time { return s.expiresAt }

// Lifetime is ExpiresAt − IssuedAt. Zero for the zero value or any
// SVID whose timestamps coincide.
func (s X509SVID) Lifetime() time.Duration { return s.expiresAt.Sub(s.issuedAt) }

// Hint returns the operator-supplied disambiguator; "" is the
// common case.
func (s X509SVID) Hint() string { return s.hint }

// IsZero reports whether the receiver is the uninitialised value.
// A successful [NewX509SVID] never returns a zero value.
func (s X509SVID) IsZero() bool { return s.id.IsZero() && len(s.chain) == 0 }

// Equal reports whether two SVIDs name the same ID, were issued
// over the same window, and carry the same Hint. Cheaper than
// comparing full chains; the (id, IssuedAt, ExpiresAt, Hint)
// tuple uniquely identifies an SVID in practice.
func (s X509SVID) Equal(other X509SVID) bool {
	return s.id.Equal(other.id) &&
		s.issuedAt.Equal(other.issuedAt) &&
		s.expiresAt.Equal(other.expiresAt) &&
		s.hint == other.hint
}

// Expired reports whether now is at or past the leaf's NotAfter.
// `now` is explicit so the auto-rotation loop (task 6) can inject
// a test clock; production callers pass time.Now().
func (s X509SVID) Expired(now time.Time) bool {
	return !now.Before(s.expiresAt)
}

// ShouldRotate reports whether at least 50% of [X509SVID.Lifetime]
// has elapsed by now. The auto-rotation loop (task 6) calls this
// hourly and triggers a re-issue when it flips true. Per
// PROJECT-DETAILS §4.10: "Cert auto-rotation at ~50% lifetime."
//
// Edge cases:
//   - now before IssuedAt (clock skew at boot) → false; the loop
//     keeps waiting.
//   - Lifetime == 0 (malformed / degenerate) → true; the loop
//     forces an immediate rotation rather than spinning.
func (s X509SVID) ShouldRotate(now time.Time) bool {
	lifetime := s.Lifetime()
	if lifetime <= 0 {
		return true
	}
	elapsed := now.Sub(s.issuedAt)
	if elapsed < 0 {
		return false
	}
	return elapsed*2 >= lifetime
}

// publicKeysEqual compares two crypto public keys by their
// algorithmic equality (ECDSA / RSA / Ed25519). go's crypto
// public-key interfaces all implement Equal(crypto.PublicKey)
// since Go 1.15; a nil-aware shim keeps the call sites readable.
func publicKeysEqual(a, b crypto.PublicKey) bool {
	if a == nil || b == nil {
		return false
	}
	type equaler interface {
		Equal(crypto.PublicKey) bool
	}
	if eq, ok := a.(equaler); ok {
		return eq.Equal(b)
	}
	return false
}
