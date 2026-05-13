package identity

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/spiffe/go-spiffe/v2/bundle/jwtbundle"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
)

// Note: Ed25519 / EdDSA is intentionally NOT in the matrix below.
// The SPIFFE JWT-SVID spec restricts signing to the RS/PS/ES
// families; go-spiffe's verifier rejects EdDSA, so we reject it at
// Sign time too (see TestSignJWTSVID_RejectsEdDSAKey).

// ---- test helpers ------------------------------------------------

// signerFor mints a fresh signing key of the requested algorithm.
// Returns both the crypto.Signer (private side) and the matching
// crypto.PublicKey (for bundle registration).
func signerFor(t *testing.T, algo string) (crypto.Signer, crypto.PublicKey) {
	t.Helper()
	switch algo {
	case "ES256":
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("ecdsa P256: %v", err)
		}
		return k, &k.PublicKey
	case "ES384":
		k, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			t.Fatalf("ecdsa P384: %v", err)
		}
		return k, &k.PublicKey
	case "ES512":
		k, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
		if err != nil {
			t.Fatalf("ecdsa P521: %v", err)
		}
		return k, &k.PublicKey
	case "RS256":
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("rsa 2048: %v", err)
		}
		return k, &k.PublicKey
	}
	t.Fatalf("unknown algo %q", algo)
	return nil, nil
}

// bundleWith returns a jwtbundle.Source containing `pub` under `kid`
// for the given trust domain. Used by ParseJWTSVID tests.
func bundleWith(t *testing.T, td, kid string, pub crypto.PublicKey) jwtbundle.Source {
	t.Helper()
	domain, err := spiffeid.TrustDomainFromString(td)
	if err != nil {
		t.Fatalf("trust domain %q: %v", td, err)
	}
	b := jwtbundle.FromJWTAuthorities(domain, map[string]crypto.PublicKey{kid: pub})
	return jwtbundle.NewSet(b)
}

// mintSVID signs a default-shaped SVID for tests. lifetime defaults
// to 1h when zero; algo defaults to ES256; kid defaults to "kid-1".
func mintSVID(t *testing.T, opts ...func(*SignJWTSVIDRequest)) (JWTSVID, crypto.PublicKey, *SignJWTSVIDRequest) {
	t.Helper()
	signer, pub := signerFor(t, "ES256")
	id, err := AgentID(DefaultTrustDomain, "agent-jwt")
	if err != nil {
		t.Fatalf("AgentID: %v", err)
	}
	req := SignJWTSVIDRequest{
		ID:       id,
		Audience: []string{"kscore"},
		Lifetime: time.Hour,
		Key:      signer,
		KeyID:    "kid-1",
	}
	for _, opt := range opts {
		opt(&req)
	}
	svid, err := SignJWTSVID(req)
	if err != nil {
		t.Fatalf("SignJWTSVID: %v", err)
	}
	return svid, pub, &req
}

// ---- SignJWTSVID happy path --------------------------------------

func TestSignJWTSVID_RoundTrip_AllAlgorithms(t *testing.T) {
	t.Parallel()
	for _, algo := range []string{"ES256", "ES384", "ES512", "RS256"} {
		algo := algo
		t.Run(algo, func(t *testing.T) {
			t.Parallel()
			signer, pub := signerFor(t, algo)
			id, _ := AgentID(DefaultTrustDomain, "agent-rt")
			now := time.Now().Truncate(time.Second)
			req := SignJWTSVIDRequest{
				ID:       id,
				Audience: []string{"kscore"},
				Lifetime: time.Hour,
				Key:      signer,
				KeyID:    "kid-rt",
				Now:      now,
			}
			svid, err := SignJWTSVID(req)
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			if svid.Token() == "" {
				t.Fatal("Token() empty")
			}
			if !svid.SPIFFEID().Equal(id) {
				t.Errorf("SPIFFEID = %q, want %q", svid.SPIFFEID(), id)
			}
			if got := svid.Audience(); len(got) != 1 || got[0] != "kscore" {
				t.Errorf("Audience = %v", got)
			}
			if !svid.IssuedAt().Equal(now) {
				t.Errorf("IssuedAt = %s, want %s", svid.IssuedAt(), now)
			}
			if !svid.ExpiresAt().Equal(now.Add(time.Hour)) {
				t.Errorf("ExpiresAt = %s, want %s", svid.ExpiresAt(), now.Add(time.Hour))
			}
			if svid.IsZero() {
				t.Error("freshly-signed SVID IsZero")
			}

			// Round-trip through ParseJWTSVID.
			bundles := bundleWith(t, DefaultTrustDomain, "kid-rt", pub)
			parsed, err := ParseJWTSVID(svid.Token(), []string{"kscore"}, bundles)
			if err != nil {
				t.Fatalf("ParseJWTSVID: %v", err)
			}
			if !parsed.Equal(svid) {
				t.Errorf("round-trip Token mismatch")
			}
			if !parsed.SPIFFEID().Equal(id) {
				t.Errorf("parsed SPIFFEID = %q, want %q", parsed.SPIFFEID(), id)
			}
		})
	}
}

func TestSignJWTSVID_DefaultsNowToWallClock(t *testing.T) {
	t.Parallel()
	before := time.Now()
	svid, _, _ := mintSVID(t, func(r *SignJWTSVIDRequest) { r.Now = time.Time{} })
	after := time.Now()
	if svid.IssuedAt().Before(before.Add(-time.Second)) || svid.IssuedAt().After(after.Add(time.Second)) {
		t.Errorf("IssuedAt = %s, want between %s and %s", svid.IssuedAt(), before, after)
	}
}

func TestSignJWTSVID_IssuerClaim(t *testing.T) {
	t.Parallel()
	svid, _, _ := mintSVID(t, func(r *SignJWTSVIDRequest) { r.Issuer = "kscore-issuer" })
	if got, ok := svid.Claims()["iss"].(string); !ok || got != "kscore-issuer" {
		t.Errorf("iss claim = %v (%T), want \"kscore-issuer\"", svid.Claims()["iss"], svid.Claims()["iss"])
	}
}

func TestSignJWTSVID_HintRoundTrips(t *testing.T) {
	t.Parallel()
	svid, _, _ := mintSVID(t, func(r *SignJWTSVIDRequest) { r.Hint = "rotating-new" })
	if svid.Hint() != "rotating-new" {
		t.Errorf("Hint = %q, want \"rotating-new\"", svid.Hint())
	}
}

func TestSignJWTSVID_ExtraClaims(t *testing.T) {
	t.Parallel()
	svid, _, _ := mintSVID(t, func(r *SignJWTSVIDRequest) {
		r.ExtraClaims = map[string]any{
			"role":     "operator",
			"clusters": []string{"east", "west"},
		}
	})
	cl := svid.Claims()
	if cl["role"] != "operator" {
		t.Errorf("custom claim role = %v, want operator", cl["role"])
	}
	// Standard claim still present + uncorrupted.
	if cl["sub"] == nil {
		t.Error("standard claim sub missing")
	}
}

func TestSignJWTSVID_MultipleAudiences(t *testing.T) {
	t.Parallel()
	svid, _, _ := mintSVID(t, func(r *SignJWTSVIDRequest) {
		r.Audience = []string{"a", "b", "c"}
	})
	if got := svid.Audience(); len(got) != 3 {
		t.Errorf("Audience = %v, want 3 entries", got)
	}
}

func TestSignJWTSVID_TrimsEmptyAudience(t *testing.T) {
	t.Parallel()
	svid, _, _ := mintSVID(t, func(r *SignJWTSVIDRequest) {
		r.Audience = []string{"", "real", "  ", "second"}
	})
	got := svid.Audience()
	if len(got) != 2 || got[0] != "real" || got[1] != "second" {
		t.Errorf("Audience = %v, want [real second]", got)
	}
}

// ---- SignJWTSVID rejection paths ---------------------------------

func TestSignJWTSVID_RejectsZeroID(t *testing.T) {
	t.Parallel()
	signer, _ := signerFor(t, "ES256")
	_, err := SignJWTSVID(SignJWTSVIDRequest{
		Audience: []string{"a"},
		Lifetime: time.Hour,
		Key:      signer,
		KeyID:    "k",
	})
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestSignJWTSVID_RejectsEmptyAudience(t *testing.T) {
	t.Parallel()
	signer, _ := signerFor(t, "ES256")
	id, _ := AgentID(DefaultTrustDomain, "a")
	_, err := SignJWTSVID(SignJWTSVIDRequest{
		ID: id, Audience: nil, Lifetime: time.Hour, Key: signer, KeyID: "k",
	})
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestSignJWTSVID_RejectsAllEmptyStrings(t *testing.T) {
	t.Parallel()
	signer, _ := signerFor(t, "ES256")
	id, _ := AgentID(DefaultTrustDomain, "a")
	_, err := SignJWTSVID(SignJWTSVIDRequest{
		ID: id, Audience: []string{"", "  ", ""}, Lifetime: time.Hour, Key: signer, KeyID: "k",
	})
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestSignJWTSVID_RejectsBadLifetime(t *testing.T) {
	t.Parallel()
	signer, _ := signerFor(t, "ES256")
	id, _ := AgentID(DefaultTrustDomain, "a")
	for _, lt := range []time.Duration{0, -time.Second} {
		_, err := SignJWTSVID(SignJWTSVIDRequest{
			ID: id, Audience: []string{"a"}, Lifetime: lt, Key: signer, KeyID: "k",
		})
		if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
			t.Errorf("lt=%s err=%v", lt, err)
		}
	}
}

func TestSignJWTSVID_RejectsNilKey(t *testing.T) {
	t.Parallel()
	id, _ := AgentID(DefaultTrustDomain, "a")
	_, err := SignJWTSVID(SignJWTSVIDRequest{
		ID: id, Audience: []string{"a"}, Lifetime: time.Hour, KeyID: "k",
	})
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestSignJWTSVID_RejectsEmptyKeyID(t *testing.T) {
	t.Parallel()
	signer, _ := signerFor(t, "ES256")
	id, _ := AgentID(DefaultTrustDomain, "a")
	_, err := SignJWTSVID(SignJWTSVIDRequest{
		ID: id, Audience: []string{"a"}, Lifetime: time.Hour, Key: signer,
	})
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestSignJWTSVID_RejectsShadowingExtraClaim(t *testing.T) {
	t.Parallel()
	signer, _ := signerFor(t, "ES256")
	id, _ := AgentID(DefaultTrustDomain, "a")
	for _, k := range []string{"sub", "aud", "exp", "iat", "iss", "nbf", "jti"} {
		_, err := SignJWTSVID(SignJWTSVIDRequest{
			ID: id, Audience: []string{"a"}, Lifetime: time.Hour, Key: signer, KeyID: "k",
			ExtraClaims: map[string]any{k: "shadow"},
		})
		if err == nil || !errors.Is(err, ErrInvalidJWTSVID) || !strings.Contains(err.Error(), "reserved") {
			t.Errorf("key %q: err = %v; want \"reserved\" rejection", k, err)
		}
	}
}

// unsupportedSigner is a crypto.Signer whose key type isn't ECDSA /
// RSA / Ed25519. Used to exercise joseAlgorithm's default branch.
type unsupportedSigner struct{}

func (unsupportedSigner) Public() crypto.PublicKey                                { return nil }
func (unsupportedSigner) Sign(io.Reader, []byte, crypto.SignerOpts) ([]byte, error) { return nil, nil }

func TestSignJWTSVID_RejectsUnsupportedKeyType(t *testing.T) {
	t.Parallel()
	id, _ := AgentID(DefaultTrustDomain, "a")
	_, err := SignJWTSVID(SignJWTSVIDRequest{
		ID: id, Audience: []string{"a"}, Lifetime: time.Hour,
		Key: unsupportedSigner{}, KeyID: "k",
	})
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestSignJWTSVID_RejectsEdDSAKey(t *testing.T) {
	t.Parallel()
	id, _ := AgentID(DefaultTrustDomain, "a")
	// Ed25519 keys (EdDSA in JWS terms) are NOT permitted by the
	// SPIFFE JWT-SVID spec — go-spiffe's verifier would reject the
	// resulting token. Sign rejects up-front rather than producing
	// an unverifiable artifact.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519: %v", err)
	}
	_, err = SignJWTSVID(SignJWTSVIDRequest{
		ID: id, Audience: []string{"a"}, Lifetime: time.Hour,
		Key: priv, KeyID: "k",
	})
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestSignJWTSVID_RejectsBadECDSACurve(t *testing.T) {
	t.Parallel()
	id, _ := AgentID(DefaultTrustDomain, "a")
	// P224 is a valid Go curve but not allowed by the SPIFFE JWT-SVID
	// spec (no ES224 in JWS). joseAlgorithm rejects it.
	k, err := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	if err != nil {
		t.Fatalf("p224: %v", err)
	}
	_, err = SignJWTSVID(SignJWTSVIDRequest{
		ID: id, Audience: []string{"a"}, Lifetime: time.Hour,
		Key: k, KeyID: "k",
	})
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

// ---- ParseJWTSVID ------------------------------------------------

func TestParseJWTSVID_RejectsNilBundle(t *testing.T) {
	t.Parallel()
	svid, _, _ := mintSVID(t)
	_, err := ParseJWTSVID(svid.Token(), []string{"kscore"}, nil)
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseJWTSVID_RejectsBadAudienceArg(t *testing.T) {
	t.Parallel()
	svid, pub, _ := mintSVID(t)
	bundles := bundleWith(t, DefaultTrustDomain, "kid-1", pub)
	_, err := ParseJWTSVID(svid.Token(), nil, bundles)
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseJWTSVID_RejectsWrongAudience(t *testing.T) {
	t.Parallel()
	svid, pub, _ := mintSVID(t)
	bundles := bundleWith(t, DefaultTrustDomain, "kid-1", pub)
	_, err := ParseJWTSVID(svid.Token(), []string{"different"}, bundles)
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseJWTSVID_RejectsExpiredToken(t *testing.T) {
	t.Parallel()
	signer, pub := signerFor(t, "ES256")
	id, _ := AgentID(DefaultTrustDomain, "agent-exp")
	// Sign with Now in the past so the token is already expired.
	svid, err := SignJWTSVID(SignJWTSVIDRequest{
		ID:       id,
		Audience: []string{"kscore"},
		Lifetime: time.Second,
		Key:      signer,
		KeyID:    "kid-1",
		Now:      time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	bundles := bundleWith(t, DefaultTrustDomain, "kid-1", pub)
	_, err = ParseJWTSVID(svid.Token(), []string{"kscore"}, bundles)
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseJWTSVID_RejectsUnknownKid(t *testing.T) {
	t.Parallel()
	svid, _, _ := mintSVID(t)
	// Bundle has a different key under a different kid → signature
	// verification fails.
	_, otherPub := signerFor(t, "ES256")
	bundles := bundleWith(t, DefaultTrustDomain, "other-kid", otherPub)
	_, err := ParseJWTSVID(svid.Token(), []string{"kscore"}, bundles)
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseJWTSVID_RejectsTamperedSignature(t *testing.T) {
	t.Parallel()
	svid, pub, _ := mintSVID(t)
	bundles := bundleWith(t, DefaultTrustDomain, "kid-1", pub)

	// Replace the signature segment wholesale with a fresh ECDSA
	// signature computed over different data (a different mint).
	// Single-byte flips on the last base64url char are not reliable
	// — base64url has 64 symbols, the modified char might decode to
	// bits that flip a byte the verifier doesn't depend on, or the
	// JOSE library may tolerate one-char anomalies in certain
	// alignments. A whole-signature swap is unambiguous.
	token := svid.Token()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token format unexpected: %q", token)
	}

	// Mint a second token with a different signing key and steal
	// its signature segment.
	otherSigner, _ := signerFor(t, "ES256")
	otherID, _ := AgentID(DefaultTrustDomain, "agent-other")
	other, err := SignJWTSVID(SignJWTSVIDRequest{
		ID: otherID, Audience: []string{"kscore"}, Lifetime: time.Hour,
		Key: otherSigner, KeyID: "kid-other",
	})
	if err != nil {
		t.Fatalf("Sign other: %v", err)
	}
	otherParts := strings.Split(other.Token(), ".")
	if len(otherParts) != 3 {
		t.Fatalf("other token format unexpected: %q", other.Token())
	}
	tampered := parts[0] + "." + parts[1] + "." + otherParts[2]

	_, err = ParseJWTSVID(tampered, []string{"kscore"}, bundles)
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

// ---- ParseJWTSVIDInsecure ----------------------------------------

func TestParseJWTSVIDInsecure_ParsesValidToken(t *testing.T) {
	t.Parallel()
	svid, _, req := mintSVID(t)
	got, err := ParseJWTSVIDInsecure(svid.Token(), req.Audience)
	if err != nil {
		t.Fatalf("ParseJWTSVIDInsecure: %v", err)
	}
	if !got.SPIFFEID().Equal(svid.SPIFFEID()) {
		t.Errorf("SPIFFEID = %q, want %q", got.SPIFFEID(), svid.SPIFFEID())
	}
}

func TestParseJWTSVIDInsecure_RejectsBadAudience(t *testing.T) {
	t.Parallel()
	svid, _, _ := mintSVID(t)
	_, err := ParseJWTSVIDInsecure(svid.Token(), nil)
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseJWTSVIDInsecure_RejectsWrongAudience(t *testing.T) {
	t.Parallel()
	svid, _, _ := mintSVID(t)
	_, err := ParseJWTSVIDInsecure(svid.Token(), []string{"different"})
	if err == nil || !errors.Is(err, ErrInvalidJWTSVID) {
		t.Fatalf("err = %v", err)
	}
}

// ---- accessors / defensive copies --------------------------------

func TestJWTSVID_DefensiveCopies(t *testing.T) {
	t.Parallel()
	svid, _, _ := mintSVID(t, func(r *SignJWTSVIDRequest) {
		r.ExtraClaims = map[string]any{"role": "operator"}
		r.Audience = []string{"a", "b"}
	})
	// Mutate accessor results — internal state must be untouched.
	aud := svid.Audience()
	aud[0] = "tampered"
	if again := svid.Audience(); again[0] != "a" {
		t.Errorf("Audience() not defensive: got %v", again)
	}
	cl := svid.Claims()
	cl["role"] = "tampered"
	if again := svid.Claims(); again["role"] != "operator" {
		t.Errorf("Claims() not defensive: got %v", again["role"])
	}
}

func TestJWTSVID_IsZero(t *testing.T) {
	t.Parallel()
	var zero JWTSVID
	if !zero.IsZero() {
		t.Error("zero not IsZero")
	}
	if zero.Audience() != nil {
		t.Errorf("zero Audience = %v", zero.Audience())
	}
	if zero.Claims() != nil {
		t.Errorf("zero Claims = %v", zero.Claims())
	}
	if zero.Token() != "" {
		t.Errorf("zero Token = %q", zero.Token())
	}
	if zero.Lifetime() != 0 {
		t.Errorf("zero Lifetime = %s", zero.Lifetime())
	}
}

func TestJWTSVID_Equal(t *testing.T) {
	t.Parallel()
	a, _, _ := mintSVID(t)
	// Same constructor inputs but signed twice = different `iat`s
	// (and thus different tokens) only when Now differs. Force a
	// distinct Now so they don't accidentally match.
	b, _, _ := mintSVID(t, func(r *SignJWTSVIDRequest) { r.Now = time.Now().Add(time.Hour) })
	if a.Equal(b) {
		t.Error("distinct SVIDs reported Equal")
	}
	// Re-parse a's token — must Equal the original.
	got, err := ParseJWTSVIDInsecure(a.Token(), []string{"kscore"})
	if err != nil {
		t.Fatalf("ParseJWTSVIDInsecure: %v", err)
	}
	if !got.Equal(a) {
		t.Error("reparse not Equal")
	}
}

// ---- predicates --------------------------------------------------

func TestJWTSVID_Expired(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	svid, _, _ := mintSVID(t, func(r *SignJWTSVIDRequest) {
		r.Now = now
		r.Lifetime = time.Hour
	})
	if svid.Expired(now) {
		t.Error("Expired at IssuedAt: want false")
	}
	if svid.Expired(now.Add(30 * time.Minute)) {
		t.Error("Expired mid-life: want false")
	}
	if !svid.Expired(now.Add(time.Hour)) {
		t.Error("Expired at ExpiresAt: want true (boundary inclusive)")
	}
	if !svid.Expired(now.Add(2 * time.Hour)) {
		t.Error("Expired past ExpiresAt: want true")
	}
}

func TestJWTSVID_ShouldRotate(t *testing.T) {
	t.Parallel()
	now := time.Now().Truncate(time.Second)
	svid, _, _ := mintSVID(t, func(r *SignJWTSVIDRequest) {
		r.Now = now
		r.Lifetime = time.Hour
	})
	if svid.ShouldRotate(now) {
		t.Error("ShouldRotate at IssuedAt: want false")
	}
	if svid.ShouldRotate(now.Add(15 * time.Minute)) {
		t.Error("ShouldRotate at 25%: want false")
	}
	if !svid.ShouldRotate(now.Add(30 * time.Minute)) {
		t.Error("ShouldRotate at 50%: want true (boundary inclusive)")
	}
	if !svid.ShouldRotate(now.Add(45 * time.Minute)) {
		t.Error("ShouldRotate at 75%: want true")
	}
}

func TestJWTSVID_ShouldRotate_ClockSkew(t *testing.T) {
	t.Parallel()
	// IssuedAt in the future → `now` (real clock) is before it →
	// ShouldRotate must return false.
	svid, _, _ := mintSVID(t, func(r *SignJWTSVIDRequest) {
		r.Now = time.Now().Add(time.Hour)
		r.Lifetime = time.Hour
	})
	if svid.ShouldRotate(time.Now()) {
		t.Error("ShouldRotate with now < IssuedAt: want false")
	}
}

func TestJWTSVID_ShouldRotate_ZeroLifetime(t *testing.T) {
	t.Parallel()
	// Manually-built JWTSVID with `iat` absent (Lifetime == 0).
	// Defensive return: ShouldRotate=true to force re-issue.
	svid := JWTSVID{
		id:        MustParseSPIFFEID("spiffe://kscore.local/agent/zero"),
		audience:  []string{"a"},
		expiresAt: time.Now().Add(time.Hour),
		// issuedAt intentionally zero.
		token: "synthetic",
	}
	if svid.Lifetime() != 0 {
		t.Fatalf("Lifetime = %s, want 0", svid.Lifetime())
	}
	if !svid.ShouldRotate(time.Now()) {
		t.Error("zero-Lifetime ShouldRotate: want true (defensive)")
	}
}
