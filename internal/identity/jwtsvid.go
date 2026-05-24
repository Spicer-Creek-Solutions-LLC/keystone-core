// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"errors"
	"fmt"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/cryptosigner"
	josejwt "github.com/go-jose/go-jose/v4/jwt"
	"github.com/spiffe/go-spiffe/v2/bundle/jwtbundle"
	"github.com/spiffe/go-spiffe/v2/svid/jwtsvid"
)

// ErrInvalidJWTSVID wraps every rejection [SignJWTSVID],
// [ParseJWTSVID], and [ParseJWTSVIDInsecure] return. Callers branch
// with [errors.Is]; mirrors [ErrInvalidSPIFFEID] +
// [ErrInvalidX509SVID].
var ErrInvalidJWTSVID = errors.New("identity: invalid JWTSVID")

// reservedJWTClaims are the names the SPIFFE JWT-SVID spec assigns
// fixed meaning to. [SignJWTSVID] populates them itself and refuses
// to let [SignJWTSVIDRequest.ExtraClaims] shadow them — a silent
// shadow would let an operator override Subject / Audience / Expiry
// and produce a malformed (or worse, mis-attributed) SVID.
var reservedJWTClaims = map[string]struct{}{
	"sub": {},
	"aud": {},
	"exp": {},
	"iat": {},
	"iss": {},
	"nbf": {},
	"jti": {},
}

// JWTSVID is one SPIFFE JWT-SVID: a signed JWT whose `sub` claim is
// a SPIFFE ID, `aud` carries one-or-more audiences, and `exp` is the
// expiry. Mirrors [X509SVID] in shape — accessors return defensive
// copies, the type is immutable after the constructor returns, and
// [JWTSVID.Expired] / [JWTSVID.ShouldRotate] take an explicit `now`
// so the rotation loop (task 6) can inject a clock.
//
// Tasks 5-7 (CAManager, EmbeddedProvider) produce JWTSVIDs;
// tasks 13-14 (pkg/api/auth.JWTAuthenticator's SPIFFE branch)
// consume them via [ParseJWTSVID].
type JWTSVID struct {
	id        SPIFFEID
	audience  []string
	issuedAt  time.Time      // from `iat` claim; zero when absent
	expiresAt time.Time      // from `exp` claim; always present
	claims    map[string]any // parsed claims (includes the standard set)
	hint      string         // operator disambiguator; not on-wire
	token     string         // the serialized JWT
}

// SignJWTSVIDRequest is the issuance input. Caller-supplied
// [SignJWTSVIDRequest.Now] is honoured when non-zero so tests
// (and time-warp scenarios) get deterministic `iat` / `exp`.
type SignJWTSVIDRequest struct {
	ID          SPIFFEID       // required; populates `sub`
	Audience    []string       // required, ≥ 1 non-empty
	Lifetime    time.Duration  // required, > 0; `exp = Now + Lifetime`
	Key         crypto.Signer  // required; ECDSA / RSA / Ed25519
	KeyID       string         // required; populates the JWT `kid` header
	Issuer      string         // optional; populates `iss`
	Hint        string         // optional; retained on the value, not on the wire
	ExtraClaims map[string]any // optional; rejected if it shadows a reserved claim
	Now         time.Time      // optional; zero → time.Now()
}

// SignJWTSVID issues a new JWT-SVID. The JWT is signed with the
// algorithm derived from Key's type (ECDSA → ES256/384/512 by curve;
// RSA → RS256; Ed25519 → EdDSA). Returns a fully-populated
// [JWTSVID] whose [JWTSVID.Token] is the serialized JWT ready to put
// on the wire.
func SignJWTSVID(req SignJWTSVIDRequest) (JWTSVID, error) {
	if req.ID.IsZero() {
		return JWTSVID{}, fmt.Errorf("%w: SPIFFE ID is required", ErrInvalidJWTSVID)
	}
	audience, err := normalizeAudience(req.Audience)
	if err != nil {
		return JWTSVID{}, err
	}
	if req.Lifetime <= 0 {
		return JWTSVID{}, fmt.Errorf("%w: lifetime must be > 0", ErrInvalidJWTSVID)
	}
	if req.Key == nil {
		return JWTSVID{}, fmt.Errorf("%w: signing key is required", ErrInvalidJWTSVID)
	}
	if req.KeyID == "" {
		return JWTSVID{}, fmt.Errorf("%w: KeyID is required (SPIFFE JWT-SVIDs MUST carry `kid` for verification routing)", ErrInvalidJWTSVID)
	}
	for k := range req.ExtraClaims {
		if _, reserved := reservedJWTClaims[k]; reserved {
			return JWTSVID{}, fmt.Errorf("%w: ExtraClaims may not contain reserved claim %q", ErrInvalidJWTSVID, k)
		}
	}

	alg, err := joseAlgorithm(req.Key)
	if err != nil {
		return JWTSVID{}, err
	}

	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	expiresAt := now.Add(req.Lifetime)

	standard := josejwt.Claims{
		Subject:  req.ID.String(),
		Audience: josejwt.Audience(audience),
		IssuedAt: josejwt.NewNumericDate(now),
		Expiry:   josejwt.NewNumericDate(expiresAt),
	}
	if req.Issuer != "" {
		standard.Issuer = req.Issuer
	}

	signer, err := jose.NewSigner(
		jose.SigningKey{
			Algorithm: alg,
			Key: jose.JSONWebKey{
				Key:   cryptosigner.Opaque(req.Key),
				KeyID: req.KeyID,
			},
		},
		new(jose.SignerOptions).WithType("JWT"),
	)
	if err != nil {
		return JWTSVID{}, fmt.Errorf("%w: jose signer: %v", ErrInvalidJWTSVID, err)
	}

	builder := josejwt.Signed(signer).Claims(standard)
	if len(req.ExtraClaims) > 0 {
		builder = builder.Claims(req.ExtraClaims)
	}
	token, err := builder.Serialize()
	if err != nil {
		return JWTSVID{}, fmt.Errorf("%w: serialize jwt: %v", ErrInvalidJWTSVID, err)
	}

	// Build the claims map directly from the inputs. Avoids a
	// round-trip through [jwtsvid.ParseInsecure] which would reject
	// tokens whose `iat` is in the future (deliberate clock-skew
	// tests) or whose `exp` is in the past (deliberately-expired
	// tokens for verification tests).
	claims := map[string]any{
		"sub": req.ID.String(),
		"aud": audienceClaim(audience),
		"iat": float64(now.Unix()),
		"exp": float64(expiresAt.Unix()),
	}
	if req.Issuer != "" {
		claims["iss"] = req.Issuer
	}
	for k, v := range req.ExtraClaims {
		claims[k] = v
	}

	return JWTSVID{
		id:        req.ID,
		audience:  cloneStrings(audience),
		issuedAt:  now,
		expiresAt: expiresAt,
		claims:    claims,
		hint:      req.Hint,
		token:     token,
	}, nil
}

// audienceClaim mirrors how a JWT `aud` claim parses out of JSON:
// a single-string audience deserializes to a string, multi-audience
// to a []any. Matches what [jwtsvid.ParseInsecure] produces so the
// Sign-and-Parse paths populate the claims map identically for
// downstream consumers.
func audienceClaim(in []string) any {
	if len(in) == 1 {
		return in[0]
	}
	out := make([]any, len(in))
	for i, a := range in {
		out[i] = a
	}
	return out
}

// ParseJWTSVID verifies the JWT against bundles (the JWT-SVID
// verification surface — task 4's [TrustBundle] implements
// [jwtbundle.Source]), checks the audience overlap, and confirms the
// token is not expired. All failures wrap [ErrInvalidJWTSVID].
//
// Mirrors [jwtsvid.ParseAndValidate]'s contract — the audience slice
// must overlap (non-empty intersection) with the JWT's `aud` array.
func ParseJWTSVID(token string, audience []string, bundles jwtbundle.Source) (JWTSVID, error) {
	if bundles == nil {
		return JWTSVID{}, fmt.Errorf("%w: bundle source is required (use ParseJWTSVIDInsecure for diagnostics)", ErrInvalidJWTSVID)
	}
	if _, err := normalizeAudience(audience); err != nil {
		return JWTSVID{}, err
	}
	svid, err := jwtsvid.ParseAndValidate(token, bundles, audience)
	if err != nil {
		return JWTSVID{}, fmt.Errorf("%w: %v", ErrInvalidJWTSVID, err)
	}
	return fromUpstreamSVID(svid, token)
}

// ParseJWTSVIDInsecure parses the JWT without verifying the
// signature. Audience overlap and expiry are still enforced. Useful
// for diagnostics / audit-log post-mortems / the agent bootstrap
// flow before the trust bundle is available.
//
// Production code paths MUST call [ParseJWTSVID] — a token whose
// signature has not been verified is not an identity proof.
func ParseJWTSVIDInsecure(token string, audience []string) (JWTSVID, error) {
	if _, err := normalizeAudience(audience); err != nil {
		return JWTSVID{}, err
	}
	svid, err := jwtsvid.ParseInsecure(token, audience)
	if err != nil {
		return JWTSVID{}, fmt.Errorf("%w: %v", ErrInvalidJWTSVID, err)
	}
	return fromUpstreamSVID(svid, token)
}

// fromUpstreamSVID translates a [jwtsvid.SVID] into our wrapper.
// Pulls `iat` out of the claims map (the upstream type doesn't
// surface it directly) so [JWTSVID.IssuedAt] is populated. `iat`
// absent → IssuedAt stays zero; [JWTSVID.ShouldRotate] treats that
// defensively.
func fromUpstreamSVID(upstream *jwtsvid.SVID, token string) (JWTSVID, error) {
	id, err := ParseSPIFFEID(upstream.ID.String())
	if err != nil {
		return JWTSVID{}, fmt.Errorf("%w: subject claim: %v", ErrInvalidJWTSVID, err)
	}
	var issuedAt time.Time
	if raw, ok := upstream.Claims["iat"]; ok {
		switch v := raw.(type) {
		case float64:
			issuedAt = time.Unix(int64(v), 0)
		case int64:
			issuedAt = time.Unix(v, 0)
		}
	}
	return JWTSVID{
		id:        id,
		audience:  cloneStrings(upstream.Audience),
		issuedAt:  issuedAt,
		expiresAt: upstream.Expiry,
		claims:    cloneClaims(upstream.Claims),
		hint:      upstream.Hint,
		token:     token,
	}, nil
}

// ---- accessors ---------------------------------------------------

// SPIFFEID returns the identifier the `sub` claim encoded.
func (s JWTSVID) SPIFFEID() SPIFFEID { return s.id }

// Audience returns a defensive copy of the `aud` array.
func (s JWTSVID) Audience() []string { return cloneStrings(s.audience) }

// IssuedAt returns the `iat` claim's time. Zero when `iat` was
// absent (it is OPTIONAL per the SPIFFE JWT-SVID spec).
func (s JWTSVID) IssuedAt() time.Time { return s.issuedAt }

// ExpiresAt returns the `exp` claim's time. The SPIFFE JWT-SVID spec
// makes `exp` REQUIRED, so this is always set for a well-formed SVID.
func (s JWTSVID) ExpiresAt() time.Time { return s.expiresAt }

// Lifetime is ExpiresAt − IssuedAt; 0 when `iat` was absent or the
// claims coincide.
func (s JWTSVID) Lifetime() time.Duration {
	if s.issuedAt.IsZero() {
		return 0
	}
	return s.expiresAt.Sub(s.issuedAt)
}

// Claims returns a defensive copy of the parsed claims map
// (including the standard set: sub/aud/exp/iat/iss/nbf/jti).
func (s JWTSVID) Claims() map[string]any { return cloneClaims(s.claims) }

// Hint returns the operator-supplied disambiguator (or "").
func (s JWTSVID) Hint() string { return s.hint }

// Token returns the serialized JWT — the on-wire form.
func (s JWTSVID) Token() string { return s.token }

// IsZero reports whether the receiver is the uninitialised value.
func (s JWTSVID) IsZero() bool { return s.token == "" && s.id.IsZero() }

// Equal compares two SVIDs by Token equality. JWT signatures embed
// a timestamp + serialized claims, so token equality implies semantic
// equality.
func (s JWTSVID) Equal(other JWTSVID) bool { return s.token == other.token }

// ---- predicates --------------------------------------------------

// Expired reports whether now is at or past `exp`. Boundary-inclusive
// — mirrors [X509SVID.Expired].
func (s JWTSVID) Expired(now time.Time) bool {
	return !now.Before(s.expiresAt)
}

// ShouldRotate mirrors [X509SVID.ShouldRotate]. Edge cases:
//   - Lifetime() <= 0 (missing `iat` or degenerate) → true; force
//     rotation rather than spin on a sentinel.
//   - now before IssuedAt (clock skew at boot) → false; the loop
//     should wait, not rotate.
//   - else elapsed*2 >= Lifetime.
func (s JWTSVID) ShouldRotate(now time.Time) bool {
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

// ---- helpers -----------------------------------------------------

// normalizeAudience trims empty entries, rejects an audience that
// ends up empty, and returns the cleaned slice. Same logic on the
// Sign and Parse paths so a "       " audience doesn't slip through
// the cracks.
func normalizeAudience(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, a := range in {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: audience must contain at least one non-empty entry", ErrInvalidJWTSVID)
	}
	return out, nil
}

// joseAlgorithm picks the SPIFFE-permitted JWS algorithm for a
// signing key. The SPIFFE JWT-SVID spec restricts signing to the
// RS / PS / ES algorithm families (per the canonical
// allow-list in `go-spiffe/v2/svid/jwtsvid`); Ed25519 / EdDSA is
// NOT permitted by the spec today, so it's rejected here even
// though go-jose itself supports it.
func joseAlgorithm(key crypto.Signer) (jose.SignatureAlgorithm, error) {
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		switch k.Curve {
		case elliptic.P256():
			return jose.ES256, nil
		case elliptic.P384():
			return jose.ES384, nil
		case elliptic.P521():
			return jose.ES512, nil
		default:
			return "", fmt.Errorf("%w: unsupported ECDSA curve %q", ErrInvalidJWTSVID, k.Curve.Params().Name)
		}
	case *rsa.PrivateKey:
		return jose.RS256, nil
	default:
		return "", fmt.Errorf("%w: unsupported signing key type %T (want ECDSA-P256/P384/P521 or RSA)", ErrInvalidJWTSVID, key)
	}
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneClaims(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
