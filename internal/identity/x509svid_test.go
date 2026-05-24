// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net/url"
	"testing"
	"time"
)

// ---- test cert minting -------------------------------------------

// mintLeaf generates an ECDSA-P256 key + a self-signed leaf
// certificate carrying id as its sole URI SAN. The leaf is its
// own issuer — fine for these unit tests, which never verify the
// chain against a trust bundle (that's tasks 4-5).
func mintLeaf(t *testing.T, id SPIFFEID, notBefore, notAfter time.Time) (*x509.Certificate, crypto.Signer) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	uri := id.URI()
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: id.String()},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		URIs:         []*url.URL{uri},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return cert, key
}

// mintLeafURIs is mintLeaf with caller-controlled URI SAN slice
// (for "no URI SAN" / "multiple URI SAN" rejection cases).
func mintLeafURIs(t *testing.T, uris []*url.URL, notBefore, notAfter time.Time) (*x509.Certificate, crypto.Signer) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-leaf"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		URIs:         uris,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return cert, key
}

func mustAgentID(t *testing.T, name string) SPIFFEID {
	t.Helper()
	id, err := AgentID(DefaultTrustDomain, name)
	if err != nil {
		t.Fatalf("AgentID(%q): %v", name, err)
	}
	return id
}

// ---- NewX509SVID happy path --------------------------------------

func TestNewX509SVID_HappyPath(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-1")
	nb := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	na := nb.Add(2 * time.Hour)
	leaf, key := mintLeaf(t, id, nb, na)

	svid, err := NewX509SVID(id, []*x509.Certificate{leaf}, key, "v1")
	if err != nil {
		t.Fatalf("NewX509SVID: %v", err)
	}
	if !svid.SPIFFEID().Equal(id) {
		t.Errorf("SPIFFEID = %q, want %q", svid.SPIFFEID(), id)
	}
	if svid.Leaf() != leaf {
		t.Error("Leaf != input leaf")
	}
	if got := svid.Chain(); len(got) != 1 || got[0] != leaf {
		t.Errorf("Chain = %v, want single-leaf", got)
	}
	if svid.PrivateKey() != key {
		t.Error("PrivateKey != input key")
	}
	if !svid.IssuedAt().Equal(nb) {
		t.Errorf("IssuedAt = %s, want %s", svid.IssuedAt(), nb)
	}
	if !svid.ExpiresAt().Equal(na) {
		t.Errorf("ExpiresAt = %s, want %s", svid.ExpiresAt(), na)
	}
	if got, want := svid.Lifetime(), 2*time.Hour; got != want {
		t.Errorf("Lifetime = %s, want %s", got, want)
	}
	if got := svid.Hint(); got != "v1" {
		t.Errorf("Hint = %q, want v1", got)
	}
	if svid.IsZero() {
		t.Error("non-zero SVID reports IsZero")
	}
}

func TestNewX509SVID_AcceptsEmptyHint(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-2")
	leaf, key := mintLeaf(t, id, time.Now(), time.Now().Add(time.Hour))
	svid, err := NewX509SVID(id, []*x509.Certificate{leaf}, key, "")
	if err != nil {
		t.Fatalf("NewX509SVID: %v", err)
	}
	if svid.Hint() != "" {
		t.Errorf("Hint = %q, want empty", svid.Hint())
	}
}

func TestNewX509SVID_StoresChainWithIntermediates(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-3")
	now := time.Now()
	leaf, key := mintLeaf(t, id, now, now.Add(time.Hour))
	// Fake "intermediate": another self-signed cert. The validation
	// surface only inspects the leaf, so the intermediate's content
	// doesn't matter for this test.
	intermediate, _ := mintLeaf(t, mustAgentID(t, "ca-intermediate"), now, now.Add(2*time.Hour))

	chain := []*x509.Certificate{leaf, intermediate}
	svid, err := NewX509SVID(id, chain, key, "")
	if err != nil {
		t.Fatalf("NewX509SVID: %v", err)
	}
	if got := svid.Chain(); len(got) != 2 || got[0] != leaf || got[1] != intermediate {
		t.Errorf("Chain = %v, want [leaf, intermediate]", got)
	}
}

func TestX509SVID_Chain_IsDefensiveCopy(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-defensive")
	leaf, key := mintLeaf(t, id, time.Now(), time.Now().Add(time.Hour))
	svid, _ := NewX509SVID(id, []*x509.Certificate{leaf}, key, "")

	got := svid.Chain()
	got[0] = nil
	// Re-fetch — the internal state must be untouched.
	if svid.Leaf() == nil {
		t.Error("mutating returned Chain corrupted internal state")
	}
	if again := svid.Chain(); again[0] == nil {
		t.Error("second Chain() call returns mutated copy")
	}
}

// ---- NewX509SVID rejections --------------------------------------

func TestNewX509SVID_RejectsZeroID(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-x")
	leaf, key := mintLeaf(t, id, time.Now(), time.Now().Add(time.Hour))
	_, err := NewX509SVID(SPIFFEID{}, []*x509.Certificate{leaf}, key, "")
	if err == nil || !errors.Is(err, ErrInvalidX509SVID) {
		t.Fatalf("err = %v; want ErrInvalidX509SVID", err)
	}
}

func TestNewX509SVID_RejectsEmptyChain(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-x")
	_, key := mintLeaf(t, id, time.Now(), time.Now().Add(time.Hour))
	_, err := NewX509SVID(id, nil, key, "")
	if err == nil || !errors.Is(err, ErrInvalidX509SVID) {
		t.Fatalf("err = %v; want ErrInvalidX509SVID", err)
	}
}

func TestNewX509SVID_RejectsNilChainEntry(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-x")
	leaf, key := mintLeaf(t, id, time.Now(), time.Now().Add(time.Hour))
	_, err := NewX509SVID(id, []*x509.Certificate{leaf, nil}, key, "")
	if err == nil || !errors.Is(err, ErrInvalidX509SVID) {
		t.Fatalf("err = %v; want ErrInvalidX509SVID", err)
	}
}

func TestNewX509SVID_RejectsLeafWithNoURISAN(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-x")
	leaf, key := mintLeafURIs(t, nil, time.Now(), time.Now().Add(time.Hour))
	_, err := NewX509SVID(id, []*x509.Certificate{leaf}, key, "")
	if err == nil || !errors.Is(err, ErrInvalidX509SVID) {
		t.Fatalf("err = %v; want ErrInvalidX509SVID", err)
	}
}

func TestNewX509SVID_RejectsLeafWithMultipleURISAN(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-x")
	other := mustAgentID(t, "agent-y")
	leaf, key := mintLeafURIs(t, []*url.URL{id.URI(), other.URI()}, time.Now(), time.Now().Add(time.Hour))
	_, err := NewX509SVID(id, []*x509.Certificate{leaf}, key, "")
	if err == nil || !errors.Is(err, ErrInvalidX509SVID) {
		t.Fatalf("err = %v; want ErrInvalidX509SVID", err)
	}
}

func TestNewX509SVID_RejectsLeafURINotSPIFFE(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-x")
	bad, _ := url.Parse("https://example.org/something")
	leaf, key := mintLeafURIs(t, []*url.URL{bad}, time.Now(), time.Now().Add(time.Hour))
	_, err := NewX509SVID(id, []*x509.Certificate{leaf}, key, "")
	if err == nil || !errors.Is(err, ErrInvalidX509SVID) {
		t.Fatalf("err = %v; want ErrInvalidX509SVID", err)
	}
}

func TestNewX509SVID_RejectsURIMismatch(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-x")
	other := mustAgentID(t, "agent-y")
	leaf, key := mintLeaf(t, other, time.Now(), time.Now().Add(time.Hour))
	_, err := NewX509SVID(id, []*x509.Certificate{leaf}, key, "")
	if err == nil || !errors.Is(err, ErrInvalidX509SVID) {
		t.Fatalf("err = %v; want ErrInvalidX509SVID", err)
	}
}

func TestNewX509SVID_RejectsNilKey(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-x")
	leaf, _ := mintLeaf(t, id, time.Now(), time.Now().Add(time.Hour))
	_, err := NewX509SVID(id, []*x509.Certificate{leaf}, nil, "")
	if err == nil || !errors.Is(err, ErrInvalidX509SVID) {
		t.Fatalf("err = %v; want ErrInvalidX509SVID", err)
	}
}

func TestNewX509SVID_RejectsKeyMismatch(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-x")
	leaf, _ := mintLeaf(t, id, time.Now(), time.Now().Add(time.Hour))
	// Different key — same algorithm, distinct material.
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	_, err = NewX509SVID(id, []*x509.Certificate{leaf}, other, "")
	if err == nil || !errors.Is(err, ErrInvalidX509SVID) {
		t.Fatalf("err = %v; want ErrInvalidX509SVID", err)
	}
}

func TestNewX509SVID_RejectsKeyMismatch_DifferentAlgo(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-x")
	leaf, _ := mintLeaf(t, id, time.Now(), time.Now().Add(time.Hour))
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	_, err = NewX509SVID(id, []*x509.Certificate{leaf}, rsaKey, "")
	if err == nil || !errors.Is(err, ErrInvalidX509SVID) {
		t.Fatalf("err = %v; want ErrInvalidX509SVID", err)
	}
}

func TestNewX509SVID_RejectsInvertedValidity(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-x")
	// x509.CreateCertificate happily accepts NotAfter < NotBefore;
	// our wrapper rejects it.
	now := time.Now()
	leaf, key := mintLeaf(t, id, now.Add(time.Hour), now) // inverted
	_, err := NewX509SVID(id, []*x509.Certificate{leaf}, key, "")
	if err == nil || !errors.Is(err, ErrInvalidX509SVID) {
		t.Fatalf("err = %v; want ErrInvalidX509SVID", err)
	}
}

// ---- IsZero / Equal ----------------------------------------------

func TestX509SVID_IsZero(t *testing.T) {
	t.Parallel()
	var zero X509SVID
	if !zero.IsZero() {
		t.Error("zero value not IsZero")
	}
	if zero.Leaf() != nil {
		t.Error("zero Leaf() not nil")
	}
	if zero.Chain() != nil {
		t.Error("zero Chain() not nil")
	}
	if zero.PrivateKey() != nil {
		t.Error("zero PrivateKey() not nil")
	}
	if got := zero.Lifetime(); got != 0 {
		t.Errorf("zero Lifetime = %s, want 0", got)
	}
}

func TestX509SVID_Equal(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-eq")
	nb := time.Now().Truncate(time.Second)
	na := nb.Add(time.Hour)
	leaf, key := mintLeaf(t, id, nb, na)
	a, _ := NewX509SVID(id, []*x509.Certificate{leaf}, key, "h")
	b, _ := NewX509SVID(id, []*x509.Certificate{leaf}, key, "h")
	if !a.Equal(b) {
		t.Error("SVIDs with same id+window+hint not Equal")
	}

	// Different hint — not Equal.
	c, _ := NewX509SVID(id, []*x509.Certificate{leaf}, key, "different")
	if a.Equal(c) {
		t.Error("different Hint should not Equal")
	}

	// Different ID — not Equal.
	other := mustAgentID(t, "agent-other")
	leaf2, key2 := mintLeaf(t, other, nb, na)
	d, _ := NewX509SVID(other, []*x509.Certificate{leaf2}, key2, "h")
	if a.Equal(d) {
		t.Error("different IDs should not Equal")
	}
}

// ---- Expired -----------------------------------------------------

func TestX509SVID_Expired(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-exp")
	nb := time.Now().Truncate(time.Second)
	na := nb.Add(time.Hour)
	leaf, key := mintLeaf(t, id, nb, na)
	svid, _ := NewX509SVID(id, []*x509.Certificate{leaf}, key, "")

	if svid.Expired(nb) {
		t.Error("Expired at IssuedAt: want false")
	}
	if svid.Expired(nb.Add(30 * time.Minute)) {
		t.Error("Expired mid-life: want false")
	}
	if !svid.Expired(na) {
		t.Error("Expired at ExpiresAt: want true (boundary inclusive)")
	}
	if !svid.Expired(na.Add(time.Minute)) {
		t.Error("Expired past ExpiresAt: want true")
	}
}

// ---- ShouldRotate ------------------------------------------------

func TestX509SVID_ShouldRotate(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-rot")
	nb := time.Now().Truncate(time.Second)
	na := nb.Add(time.Hour)
	leaf, key := mintLeaf(t, id, nb, na)
	svid, _ := NewX509SVID(id, []*x509.Certificate{leaf}, key, "")

	if svid.ShouldRotate(nb) {
		t.Error("ShouldRotate at IssuedAt: want false")
	}
	if svid.ShouldRotate(nb.Add(15 * time.Minute)) {
		t.Error("ShouldRotate at 25%: want false")
	}
	if !svid.ShouldRotate(nb.Add(30 * time.Minute)) {
		t.Error("ShouldRotate at 50%: want true (boundary inclusive)")
	}
	if !svid.ShouldRotate(nb.Add(45 * time.Minute)) {
		t.Error("ShouldRotate at 75%: want true")
	}
}

func TestX509SVID_ShouldRotate_NowBeforeIssued(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-skew")
	nb := time.Now().Add(time.Hour).Truncate(time.Second) // future
	na := nb.Add(time.Hour)
	leaf, key := mintLeaf(t, id, nb, na)
	svid, _ := NewX509SVID(id, []*x509.Certificate{leaf}, key, "")

	// `now` (real wall-clock now) is before IssuedAt — clock skew at
	// boot. The loop should wait, not rotate.
	if svid.ShouldRotate(time.Now()) {
		t.Error("ShouldRotate with now < IssuedAt: want false")
	}
}

func TestX509SVID_ShouldRotate_ZeroLifetime(t *testing.T) {
	t.Parallel()
	id := mustAgentID(t, "agent-zero")
	nb := time.Now().Truncate(time.Second)
	leaf, key := mintLeaf(t, id, nb, nb) // NotBefore == NotAfter
	svid, _ := NewX509SVID(id, []*x509.Certificate{leaf}, key, "")

	if !svid.ShouldRotate(nb) {
		t.Error("ShouldRotate on zero-Lifetime SVID: want true (defensive)")
	}
}

// ---- publicKeysEqual edge cases (via the rejection paths) --------

func TestPublicKeysEqual_NilArgs(t *testing.T) {
	t.Parallel()
	// Both directly through the unexported helper to confirm the
	// nil-guard is honoured. The mismatch test above only exercises
	// the false-from-Equal path; this one exercises the nil arms.
	if publicKeysEqual(nil, nil) {
		t.Error("publicKeysEqual(nil, nil) = true, want false")
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if publicKeysEqual(nil, &key.PublicKey) {
		t.Error("publicKeysEqual(nil, key) = true, want false")
	}
	if publicKeysEqual(&key.PublicKey, nil) {
		t.Error("publicKeysEqual(key, nil) = true, want false")
	}
}

// noEqualKey is a crypto.PublicKey that does NOT implement the
// Equal(crypto.PublicKey) bool method. ecdsa / rsa / ed25519 all
// implement Equal since Go 1.15, so we synthesize a non-conforming
// type to exercise the type-assertion-fail return path in
// publicKeysEqual.
type noEqualKey struct{}

func TestPublicKeysEqual_TypeWithoutEqualMethod(t *testing.T) {
	t.Parallel()
	a := noEqualKey{}
	b := noEqualKey{}
	if publicKeysEqual(a, b) {
		t.Error("publicKeysEqual on type lacking Equal: want false")
	}
}
