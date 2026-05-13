package identity

import (
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	"github.com/spiffe/go-spiffe/v2/bundle/jwtbundle"
	"github.com/spiffe/go-spiffe/v2/bundle/spiffebundle"
	"github.com/spiffe/go-spiffe/v2/bundle/x509bundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
)

// ErrInvalidTrustBundle wraps every rejection from this file.
// Mirrors the ErrInvalid* sentinels on the other identity types.
var ErrInvalidTrustBundle = errors.New("identity: invalid TrustBundle")

// TrustBundle is the public material a workload in a single trust
// domain needs to verify peer SVIDs:
//
//   - X509Authorities — the CA roots used to verify X.509-SVIDs.
//   - JWTAuthorities — JWT verification keys keyed by `kid`.
//   - RefreshHint — operator hint at how often to refresh the bundle.
//   - SequenceNumber — bundle revision; lets caches detect updates.
//
// Wraps [github.com/spiffe/go-spiffe/v2/bundle/spiffebundle.Bundle]
// — that type already implements the full SPIFFE trust-bundle spec
// (JWKS marshal/parse, refresh hint, sequence number, both Source
// interfaces). This wrapper adds the [ErrInvalidTrustBundle]
// rejection surface (nil arguments are silently accepted by the
// upstream), a string-trust-domain public surface that matches our
// [SPIFFEID] API, and a Go-comparable pointer-equality story.
//
// TrustBundle is mutable and goroutine-safe — the upstream bundle
// holds its own lock. Tasks 5-7 produce one inside the CA manager
// and EmbeddedProvider; task 6's rotation loop updates its JWT keys;
// task 13's mTLS authenticator consumes it.
//
// *TrustBundle satisfies both [x509bundle.Source] and
// [jwtbundle.Source] so it plugs directly into [ParseJWTSVID] and
// into [github.com/spiffe/go-spiffe/v2/svid/x509svid.Verify].
type TrustBundle struct {
	inner *spiffebundle.Bundle
}

// NewTrustBundle returns an empty TrustBundle for trustDomain.
// Empty until populated via Set / Add.
func NewTrustBundle(trustDomain string) (*TrustBundle, error) {
	td, err := spiffeid.TrustDomainFromString(trustDomain)
	if err != nil {
		return nil, fmt.Errorf("%w: trust domain: %v", ErrInvalidTrustBundle, err)
	}
	return &TrustBundle{inner: spiffebundle.New(td)}, nil
}

// TrustBundleFromAuthorities seeds the bundle with X.509 + JWT
// authorities in one call. Either map / slice may be nil or empty;
// nil entries within them are rejected.
func TrustBundleFromAuthorities(trustDomain string, x509Authorities []*x509.Certificate, jwtAuthorities map[string]crypto.PublicKey) (*TrustBundle, error) {
	b, err := NewTrustBundle(trustDomain)
	if err != nil {
		return nil, err
	}
	if x509Authorities != nil {
		if err := b.SetX509Authorities(x509Authorities); err != nil {
			return nil, err
		}
	}
	if jwtAuthorities != nil {
		if err := b.SetJWTAuthorities(jwtAuthorities); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// ParseTrustBundle parses the SPIFFE JWKS-extended bundle form.
// Wraps [spiffebundle.Parse] so the operator-facing error names
// the SPIFFE bundle spec consistently.
func ParseTrustBundle(trustDomain string, jwksBytes []byte) (*TrustBundle, error) {
	td, err := spiffeid.TrustDomainFromString(trustDomain)
	if err != nil {
		return nil, fmt.Errorf("%w: trust domain: %v", ErrInvalidTrustBundle, err)
	}
	inner, err := spiffebundle.Parse(td, jwksBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTrustBundle, err)
	}
	return &TrustBundle{inner: inner}, nil
}

// ---- identity ----------------------------------------------------

// TrustDomain returns the bundle's trust-domain name.
func (b *TrustBundle) TrustDomain() string { return b.inner.TrustDomain().Name() }

// IsEmpty reports whether the bundle contains zero authorities of
// either kind. A bundle with RefreshHint / SequenceNumber metadata
// only is still IsEmpty=true — those alone don't make a usable
// verification surface.
func (b *TrustBundle) IsEmpty() bool {
	return len(b.inner.X509Authorities()) == 0 && len(b.inner.JWTAuthorities()) == 0
}

// ---- X509 authorities --------------------------------------------

// X509Authorities returns a defensive copy of the CA cert list.
// Callers may mutate the returned slice freely.
func (b *TrustBundle) X509Authorities() []*x509.Certificate {
	src := b.inner.X509Authorities()
	if len(src) == 0 {
		return nil
	}
	out := make([]*x509.Certificate, len(src))
	copy(out, src)
	return out
}

// AddX509Authority installs cert as a CA root. Rejects nil.
func (b *TrustBundle) AddX509Authority(cert *x509.Certificate) error {
	if cert == nil {
		return fmt.Errorf("%w: x509 authority is nil", ErrInvalidTrustBundle)
	}
	b.inner.AddX509Authority(cert)
	return nil
}

// RemoveX509Authority removes cert if present; no-op if absent.
func (b *TrustBundle) RemoveX509Authority(cert *x509.Certificate) {
	if cert == nil {
		return
	}
	b.inner.RemoveX509Authority(cert)
}

// SetX509Authorities replaces the CA cert list. Rejects a slice
// containing any nil entry; an empty slice clears the set.
func (b *TrustBundle) SetX509Authorities(certs []*x509.Certificate) error {
	for i, c := range certs {
		if c == nil {
			return fmt.Errorf("%w: x509 authority [%d] is nil", ErrInvalidTrustBundle, i)
		}
	}
	b.inner.SetX509Authorities(certs)
	return nil
}

// HasX509Authority reports whether cert is registered.
func (b *TrustBundle) HasX509Authority(cert *x509.Certificate) bool {
	if cert == nil {
		return false
	}
	return b.inner.HasX509Authority(cert)
}

// ---- JWT authorities ---------------------------------------------

// JWTAuthorities returns a defensive copy of the kid → public-key
// map.
func (b *TrustBundle) JWTAuthorities() map[string]crypto.PublicKey {
	src := b.inner.JWTAuthorities()
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]crypto.PublicKey, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// FindJWTAuthority returns (key, true) when kid is registered.
func (b *TrustBundle) FindJWTAuthority(kid string) (crypto.PublicKey, bool) {
	return b.inner.FindJWTAuthority(kid)
}

// AddJWTAuthority registers key under kid. Rejects empty kid or
// nil key.
func (b *TrustBundle) AddJWTAuthority(kid string, key crypto.PublicKey) error {
	if kid == "" {
		return fmt.Errorf("%w: kid is required", ErrInvalidTrustBundle)
	}
	if key == nil {
		return fmt.Errorf("%w: jwt authority key is nil", ErrInvalidTrustBundle)
	}
	if err := b.inner.AddJWTAuthority(kid, key); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTrustBundle, err)
	}
	return nil
}

// RemoveJWTAuthority drops the entry under kid; no-op if absent.
func (b *TrustBundle) RemoveJWTAuthority(kid string) {
	if kid == "" {
		return
	}
	b.inner.RemoveJWTAuthority(kid)
}

// SetJWTAuthorities replaces the whole keyset. Rejects an empty
// kid or nil key anywhere in the input.
func (b *TrustBundle) SetJWTAuthorities(keys map[string]crypto.PublicKey) error {
	for kid, key := range keys {
		if kid == "" {
			return fmt.Errorf("%w: empty kid in jwt authority map", ErrInvalidTrustBundle)
		}
		if key == nil {
			return fmt.Errorf("%w: jwt authority %q is nil", ErrInvalidTrustBundle, kid)
		}
	}
	b.inner.SetJWTAuthorities(keys)
	return nil
}

// HasJWTAuthority reports whether kid is registered.
func (b *TrustBundle) HasJWTAuthority(kid string) bool {
	if kid == "" {
		return false
	}
	return b.inner.HasJWTAuthority(kid)
}

// ---- SPIFFE bundle metadata --------------------------------------

// RefreshHint returns the bundle's operator-supplied refresh
// interval. The boolean reports whether the hint is set — both
// RefreshHint and SequenceNumber are OPTIONAL per the SPIFFE spec.
func (b *TrustBundle) RefreshHint() (time.Duration, bool) { return b.inner.RefreshHint() }

// SetRefreshHint sets the refresh-hint metadata.
func (b *TrustBundle) SetRefreshHint(d time.Duration) { b.inner.SetRefreshHint(d) }

// ClearRefreshHint removes the metadata. Subsequent
// [TrustBundle.RefreshHint] calls report (0, false).
func (b *TrustBundle) ClearRefreshHint() { b.inner.ClearRefreshHint() }

// SequenceNumber returns the bundle's revision number. The boolean
// reports whether the metadata is set.
func (b *TrustBundle) SequenceNumber() (uint64, bool) { return b.inner.SequenceNumber() }

// SetSequenceNumber sets the revision number.
func (b *TrustBundle) SetSequenceNumber(n uint64) { b.inner.SetSequenceNumber(n) }

// ClearSequenceNumber removes the metadata.
func (b *TrustBundle) ClearSequenceNumber() { b.inner.ClearSequenceNumber() }

// ---- Source interface implementations ----------------------------

// GetX509BundleForTrustDomain makes [*TrustBundle] satisfy
// [x509bundle.Source] so it plugs directly into
// [github.com/spiffe/go-spiffe/v2/svid/x509svid.Verify]. Returns an
// error wrapping [ErrInvalidTrustBundle] when td is foreign — this
// is a single-domain bundle (federation is v1.1 — see the epic's
// out-of-scope list).
func (b *TrustBundle) GetX509BundleForTrustDomain(td spiffeid.TrustDomain) (*x509bundle.Bundle, error) {
	if td.Compare(b.inner.TrustDomain()) != 0 {
		return nil, fmt.Errorf("%w: trust domain %q not in this bundle (own=%q)", ErrInvalidTrustBundle, td.Name(), b.inner.TrustDomain().Name())
	}
	return b.inner.X509Bundle(), nil
}

// GetJWTBundleForTrustDomain makes [*TrustBundle] satisfy
// [jwtbundle.Source] so it plugs directly into [ParseJWTSVID] and
// [github.com/spiffe/go-spiffe/v2/svid/jwtsvid.ParseAndValidate].
// Foreign trust domain → error.
func (b *TrustBundle) GetJWTBundleForTrustDomain(td spiffeid.TrustDomain) (*jwtbundle.Bundle, error) {
	if td.Compare(b.inner.TrustDomain()) != 0 {
		return nil, fmt.Errorf("%w: trust domain %q not in this bundle (own=%q)", ErrInvalidTrustBundle, td.Name(), b.inner.TrustDomain().Name())
	}
	return b.inner.JWTBundle(), nil
}

// ---- serialization -----------------------------------------------

// Marshal emits the bundle in the SPIFFE Trust Domain and Bundle
// JWKS format. Round-trips with [ParseTrustBundle].
func (b *TrustBundle) Marshal() ([]byte, error) {
	out, err := b.inner.Marshal()
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %v", ErrInvalidTrustBundle, err)
	}
	return out, nil
}

// Clone returns a deep copy. Mutating the clone does not affect
// the receiver and vice-versa.
func (b *TrustBundle) Clone() *TrustBundle {
	return &TrustBundle{inner: b.inner.Clone()}
}

// Equal reports whether two bundles carry the same authorities +
// metadata for the same trust domain. nil compares unequal to a
// non-nil bundle.
func (b *TrustBundle) Equal(other *TrustBundle) bool {
	if b == nil || other == nil {
		return b == nil && other == nil
	}
	return b.inner.Equal(other.inner)
}
