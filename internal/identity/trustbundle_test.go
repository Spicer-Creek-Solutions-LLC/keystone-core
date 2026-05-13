package identity

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
)

// ---- helpers ----------------------------------------------------

// mintCA returns a self-signed CA cert + its signing key. The cert
// is templated as a CA (BasicConstraintsValid=true, IsCA=true) so
// x509svid.Verify accepts it as a trust anchor.
func mintCA(t *testing.T, subject string) (*x509.Certificate, crypto.Signer) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: subject},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
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

// mintLeafSignedBy issues a leaf carrying id as its URI SAN, signed
// by ca / caKey. Returns the leaf chain [leaf, ca] + the leaf's
// private key.
func mintLeafSignedBy(t *testing.T, id SPIFFEID, ca *x509.Certificate, caKey crypto.Signer) ([]*x509.Certificate, crypto.Signer) {
	t.Helper()
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: id.String()},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		URIs:         []*url.URL{id.URI()},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return []*x509.Certificate{leaf, ca}, leafKey
}

// ---- NewTrustBundle ----------------------------------------------

func TestNewTrustBundle_Empty(t *testing.T) {
	t.Parallel()
	b, err := NewTrustBundle(DefaultTrustDomain)
	if err != nil {
		t.Fatalf("NewTrustBundle: %v", err)
	}
	if b.TrustDomain() != DefaultTrustDomain {
		t.Errorf("TrustDomain = %q, want %q", b.TrustDomain(), DefaultTrustDomain)
	}
	if !b.IsEmpty() {
		t.Error("fresh bundle is not IsEmpty")
	}
	if got := b.X509Authorities(); got != nil {
		t.Errorf("X509Authorities = %v, want nil", got)
	}
	if got := b.JWTAuthorities(); got != nil {
		t.Errorf("JWTAuthorities = %v, want nil", got)
	}
}

func TestNewTrustBundle_RejectsBadTrustDomain(t *testing.T) {
	t.Parallel()
	for _, td := range []string{"", "UPPER", "with space"} {
		_, err := NewTrustBundle(td)
		if err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
			t.Errorf("td=%q err=%v; want ErrInvalidTrustBundle", td, err)
		}
	}
}

// ---- TrustBundleFromAuthorities ----------------------------------

func TestTrustBundleFromAuthorities_HappyPath(t *testing.T) {
	t.Parallel()
	ca, _ := mintCA(t, "ca-1")
	_, pub := signerFor(t, "ES256")

	b, err := TrustBundleFromAuthorities(
		DefaultTrustDomain,
		[]*x509.Certificate{ca},
		map[string]crypto.PublicKey{"kid-1": pub},
	)
	if err != nil {
		t.Fatalf("TrustBundleFromAuthorities: %v", err)
	}
	if b.IsEmpty() {
		t.Error("populated bundle reports IsEmpty")
	}
	if got := len(b.X509Authorities()); got != 1 {
		t.Errorf("X509Authorities count = %d, want 1", got)
	}
	if got := len(b.JWTAuthorities()); got != 1 {
		t.Errorf("JWTAuthorities count = %d, want 1", got)
	}
}

func TestTrustBundleFromAuthorities_NilSlicesOK(t *testing.T) {
	t.Parallel()
	b, err := TrustBundleFromAuthorities(DefaultTrustDomain, nil, nil)
	if err != nil {
		t.Fatalf("TrustBundleFromAuthorities: %v", err)
	}
	if !b.IsEmpty() {
		t.Error("nil-seeded bundle should be IsEmpty")
	}
}

func TestTrustBundleFromAuthorities_RejectsBadDomain(t *testing.T) {
	t.Parallel()
	_, err := TrustBundleFromAuthorities("UPPER", nil, nil)
	if err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

func TestTrustBundleFromAuthorities_RejectsNilCert(t *testing.T) {
	t.Parallel()
	ca, _ := mintCA(t, "ca-x")
	_, err := TrustBundleFromAuthorities(
		DefaultTrustDomain,
		[]*x509.Certificate{ca, nil},
		nil,
	)
	if err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

func TestTrustBundleFromAuthorities_RejectsNilJWTKey(t *testing.T) {
	t.Parallel()
	_, err := TrustBundleFromAuthorities(
		DefaultTrustDomain,
		nil,
		map[string]crypto.PublicKey{"kid-x": nil},
	)
	if err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

func TestTrustBundleFromAuthorities_RejectsEmptyKID(t *testing.T) {
	t.Parallel()
	_, pub := signerFor(t, "ES256")
	_, err := TrustBundleFromAuthorities(
		DefaultTrustDomain,
		nil,
		map[string]crypto.PublicKey{"": pub},
	)
	if err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

// ---- X509 authorities lifecycle ----------------------------------

func TestTrustBundle_X509AuthorityLifecycle(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	ca, _ := mintCA(t, "ca-life")

	if b.HasX509Authority(ca) {
		t.Error("fresh bundle should not have any authority")
	}
	if err := b.AddX509Authority(ca); err != nil {
		t.Fatalf("AddX509Authority: %v", err)
	}
	if !b.HasX509Authority(ca) {
		t.Error("Has not reflecting Add")
	}
	if b.IsEmpty() {
		t.Error("IsEmpty after Add")
	}
	b.RemoveX509Authority(ca)
	if b.HasX509Authority(ca) {
		t.Error("Has after Remove")
	}
	if !b.IsEmpty() {
		t.Error("not IsEmpty after Remove")
	}
}

func TestTrustBundle_AddX509AuthorityNil(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	if err := b.AddX509Authority(nil); err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

func TestTrustBundle_RemoveX509Authority_NilSafe(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	b.RemoveX509Authority(nil) // must not panic
}

func TestTrustBundle_HasX509Authority_Nil(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	if b.HasX509Authority(nil) {
		t.Error("HasX509Authority(nil) returned true")
	}
}

func TestTrustBundle_SetX509Authorities(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	a, _ := mintCA(t, "ca-a")
	c, _ := mintCA(t, "ca-c")
	if err := b.SetX509Authorities([]*x509.Certificate{a, c}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := len(b.X509Authorities()); got != 2 {
		t.Errorf("count = %d, want 2", got)
	}
}

func TestTrustBundle_SetX509Authorities_RejectsNilEntry(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	ca, _ := mintCA(t, "ca-q")
	err := b.SetX509Authorities([]*x509.Certificate{ca, nil})
	if err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "[1]") {
		t.Errorf("err = %v; want index in message", err)
	}
}

func TestTrustBundle_X509Authorities_DefensiveCopy(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	ca, _ := mintCA(t, "ca-d")
	_ = b.AddX509Authority(ca)
	got := b.X509Authorities()
	got[0] = nil
	if again := b.X509Authorities(); again[0] == nil {
		t.Error("X509Authorities not defensive: mutating returned slice corrupted internal state")
	}
}

// ---- JWT authorities lifecycle -----------------------------------

func TestTrustBundle_JWTAuthorityLifecycle(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	_, pub := signerFor(t, "ES256")

	if b.HasJWTAuthority("k") {
		t.Error("fresh bundle reports Has(k)")
	}
	if err := b.AddJWTAuthority("k", pub); err != nil {
		t.Fatalf("AddJWTAuthority: %v", err)
	}
	if !b.HasJWTAuthority("k") {
		t.Error("Has not reflecting Add")
	}
	if got, ok := b.FindJWTAuthority("k"); !ok || got == nil {
		t.Error("FindJWTAuthority should return the key")
	}
	if _, ok := b.FindJWTAuthority("missing"); ok {
		t.Error("FindJWTAuthority(missing) returned ok=true")
	}
	b.RemoveJWTAuthority("k")
	if b.HasJWTAuthority("k") {
		t.Error("Has after Remove")
	}
}

func TestTrustBundle_AddJWTAuthority_EmptyKID(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	_, pub := signerFor(t, "ES256")
	if err := b.AddJWTAuthority("", pub); err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

func TestTrustBundle_AddJWTAuthority_NilKey(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	if err := b.AddJWTAuthority("k", nil); err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

func TestTrustBundle_RemoveJWTAuthority_EmptyKIDNoop(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	b.RemoveJWTAuthority("") // must not panic; no-op
}

func TestTrustBundle_HasJWTAuthority_EmptyKID(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	if b.HasJWTAuthority("") {
		t.Error("HasJWTAuthority(\"\") returned true")
	}
}

func TestTrustBundle_SetJWTAuthorities(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	_, p1 := signerFor(t, "ES256")
	_, p2 := signerFor(t, "ES256")
	err := b.SetJWTAuthorities(map[string]crypto.PublicKey{
		"k1": p1,
		"k2": p2,
	})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := len(b.JWTAuthorities()); got != 2 {
		t.Errorf("count = %d, want 2", got)
	}
}

func TestTrustBundle_SetJWTAuthorities_RejectsEmptyKID(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	_, pub := signerFor(t, "ES256")
	err := b.SetJWTAuthorities(map[string]crypto.PublicKey{
		"":  pub,
		"k": pub,
	})
	if err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

func TestTrustBundle_SetJWTAuthorities_RejectsNilKey(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	err := b.SetJWTAuthorities(map[string]crypto.PublicKey{"k": nil})
	if err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

func TestTrustBundle_JWTAuthorities_DefensiveCopy(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	_, pub := signerFor(t, "ES256")
	_ = b.AddJWTAuthority("k", pub)
	got := b.JWTAuthorities()
	got["k"] = nil
	if again := b.JWTAuthorities(); again["k"] == nil {
		t.Error("JWTAuthorities not defensive")
	}
}

// ---- RefreshHint + SequenceNumber --------------------------------

func TestTrustBundle_RefreshHint(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	if _, ok := b.RefreshHint(); ok {
		t.Error("fresh bundle has RefreshHint set")
	}
	b.SetRefreshHint(5 * time.Minute)
	if got, ok := b.RefreshHint(); !ok || got != 5*time.Minute {
		t.Errorf("RefreshHint = %s ok=%v, want 5m true", got, ok)
	}
	b.ClearRefreshHint()
	if _, ok := b.RefreshHint(); ok {
		t.Error("ClearRefreshHint did not clear")
	}
}

func TestTrustBundle_SequenceNumber(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	if _, ok := b.SequenceNumber(); ok {
		t.Error("fresh bundle has SequenceNumber set")
	}
	b.SetSequenceNumber(42)
	if got, ok := b.SequenceNumber(); !ok || got != 42 {
		t.Errorf("SequenceNumber = %d ok=%v, want 42 true", got, ok)
	}
	b.ClearSequenceNumber()
	if _, ok := b.SequenceNumber(); ok {
		t.Error("ClearSequenceNumber did not clear")
	}
}

// ---- Source interface --------------------------------------------

func TestTrustBundle_GetX509BundleForTrustDomain(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	ca, _ := mintCA(t, "ca-src")
	_ = b.AddX509Authority(ca)

	td, _ := spiffeid.TrustDomainFromString(DefaultTrustDomain)
	x, err := b.GetX509BundleForTrustDomain(td)
	if err != nil {
		t.Fatalf("GetX509BundleForTrustDomain: %v", err)
	}
	if len(x.X509Authorities()) != 1 {
		t.Errorf("authorities = %d, want 1", len(x.X509Authorities()))
	}
}

func TestTrustBundle_GetX509BundleForTrustDomain_ForeignTD(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	other, _ := spiffeid.TrustDomainFromString("other.org")
	_, err := b.GetX509BundleForTrustDomain(other)
	if err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

func TestTrustBundle_GetJWTBundleForTrustDomain(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	_, pub := signerFor(t, "ES256")
	_ = b.AddJWTAuthority("k", pub)

	td, _ := spiffeid.TrustDomainFromString(DefaultTrustDomain)
	j, err := b.GetJWTBundleForTrustDomain(td)
	if err != nil {
		t.Fatalf("GetJWTBundleForTrustDomain: %v", err)
	}
	if len(j.JWTAuthorities()) != 1 {
		t.Errorf("authorities = %d, want 1", len(j.JWTAuthorities()))
	}
}

func TestTrustBundle_GetJWTBundleForTrustDomain_ForeignTD(t *testing.T) {
	t.Parallel()
	b, _ := NewTrustBundle(DefaultTrustDomain)
	other, _ := spiffeid.TrustDomainFromString("other.org")
	_, err := b.GetJWTBundleForTrustDomain(other)
	if err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

// ---- Marshal / Parse ---------------------------------------------

func TestTrustBundle_MarshalParse_RoundTrip(t *testing.T) {
	t.Parallel()
	ca, _ := mintCA(t, "ca-rt")
	_, pub := signerFor(t, "ES256")
	orig, _ := TrustBundleFromAuthorities(
		DefaultTrustDomain,
		[]*x509.Certificate{ca},
		map[string]crypto.PublicKey{"kid-rt": pub},
	)
	orig.SetRefreshHint(7 * time.Minute)
	orig.SetSequenceNumber(13)

	data, err := orig.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	back, err := ParseTrustBundle(DefaultTrustDomain, data)
	if err != nil {
		t.Fatalf("ParseTrustBundle: %v", err)
	}
	if !orig.Equal(back) {
		t.Error("round-trip bundles not Equal")
	}
	if got, ok := back.RefreshHint(); !ok || got != 7*time.Minute {
		t.Errorf("RefreshHint after round-trip = %s ok=%v", got, ok)
	}
	if got, ok := back.SequenceNumber(); !ok || got != 13 {
		t.Errorf("SequenceNumber after round-trip = %d ok=%v", got, ok)
	}
}

func TestTrustBundle_Parse_RejectsGarbage(t *testing.T) {
	t.Parallel()
	_, err := ParseTrustBundle(DefaultTrustDomain, []byte("not-json"))
	if err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

func TestTrustBundle_Parse_RejectsBadTrustDomain(t *testing.T) {
	t.Parallel()
	_, err := ParseTrustBundle("UPPER", []byte("{}"))
	if err == nil || !errors.Is(err, ErrInvalidTrustBundle) {
		t.Errorf("err = %v", err)
	}
}

// ---- Clone / Equal -----------------------------------------------

func TestTrustBundle_Clone_DeepCopy(t *testing.T) {
	t.Parallel()
	ca, _ := mintCA(t, "ca-clone")
	orig, _ := TrustBundleFromAuthorities(
		DefaultTrustDomain,
		[]*x509.Certificate{ca},
		nil,
	)
	clone := orig.Clone()
	if !orig.Equal(clone) {
		t.Error("fresh clone not Equal to original")
	}
	// Mutate the clone — original must be untouched.
	other, _ := mintCA(t, "ca-other")
	_ = clone.AddX509Authority(other)
	if orig.Equal(clone) {
		t.Error("mutating clone affected original (Equal still true)")
	}
	if len(orig.X509Authorities()) != 1 {
		t.Errorf("original mutated; X509Authorities = %d, want 1", len(orig.X509Authorities()))
	}
}

func TestTrustBundle_Equal_NilHandling(t *testing.T) {
	t.Parallel()
	var nilA, nilB *TrustBundle
	if !nilA.Equal(nilB) {
		t.Error("nil.Equal(nil) should be true")
	}
	b, _ := NewTrustBundle(DefaultTrustDomain)
	if b.Equal(nilA) {
		t.Error("non-nil.Equal(nil) should be false")
	}
	if nilA.Equal(b) {
		t.Error("nil.Equal(non-nil) should be false")
	}
}

func TestTrustBundle_Equal_DifferentAuthorities(t *testing.T) {
	t.Parallel()
	a, _ := NewTrustBundle(DefaultTrustDomain)
	b, _ := NewTrustBundle(DefaultTrustDomain)
	ca, _ := mintCA(t, "ca-eq")
	_ = a.AddX509Authority(ca)
	if a.Equal(b) {
		t.Error("bundles with different authorities should not Equal")
	}
}

// ---- Integration with task 3 (ParseJWTSVID) ---------------------

func TestTrustBundle_AsJWTSource_VerifyJWTSVID(t *testing.T) {
	t.Parallel()
	signer, pub := signerFor(t, "ES256")
	id, _ := AgentID(DefaultTrustDomain, "agent-int")
	svid, err := SignJWTSVID(SignJWTSVIDRequest{
		ID: id, Audience: []string{"kscore"}, Lifetime: time.Hour,
		Key: signer, KeyID: "kid-int",
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	b, _ := TrustBundleFromAuthorities(
		DefaultTrustDomain, nil,
		map[string]crypto.PublicKey{"kid-int": pub},
	)
	// *TrustBundle directly satisfies jwtbundle.Source.
	parsed, err := ParseJWTSVID(svid.Token(), []string{"kscore"}, b)
	if err != nil {
		t.Fatalf("ParseJWTSVID against TrustBundle: %v", err)
	}
	if !parsed.SPIFFEID().Equal(id) {
		t.Errorf("parsed SPIFFEID = %q, want %q", parsed.SPIFFEID(), id)
	}
}

// ---- Integration with x509svid.Verify ---------------------------

func TestTrustBundle_AsX509Source_VerifySVID(t *testing.T) {
	t.Parallel()
	ca, caKey := mintCA(t, "ca-verify")
	id, _ := AgentID(DefaultTrustDomain, "agent-verify")
	chain, _ := mintLeafSignedBy(t, id, ca, caKey)

	b, _ := TrustBundleFromAuthorities(
		DefaultTrustDomain,
		[]*x509.Certificate{ca},
		nil,
	)
	// x509svid.Verify takes an x509bundle.Source — *TrustBundle
	// satisfies it directly.
	gotID, chains, err := x509svid.Verify(chain, b)
	if err != nil {
		t.Fatalf("x509svid.Verify: %v", err)
	}
	if gotID.String() != id.String() {
		t.Errorf("verified id = %q, want %q", gotID, id)
	}
	if len(chains) == 0 {
		t.Error("no verified chains returned")
	}
}
