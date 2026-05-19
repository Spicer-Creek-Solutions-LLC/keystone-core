package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
)

// ErrUnauthenticated is returned by [Authenticator.Authenticate] when a
// request fails its source's authentication. The receiver maps it to
// HTTP 401. The message is deliberately coarse — callers must not leak
// whether the header was missing vs. the signature mismatched.
var ErrUnauthenticated = errors.New("gitops/webhook: request authentication failed")

// AuthMethod is the closed set of v1.0 webhook auth schemes.
type AuthMethod string

const (
	// AuthNone disables authentication for a source (explicit opt-out).
	AuthNone AuthMethod = "none"
	// AuthHMAC verifies an HMAC-SHA256 of the raw body.
	AuthHMAC AuthMethod = "hmac"
	// AuthBearer verifies a shared secret carried in a header.
	AuthBearer AuthMethod = "bearer"
)

// Authenticator verifies an inbound webhook before it is parsed.
// Implementations must be safe for concurrent use (the receiver shares
// one per source across requests) and must not mutate r.
//
// Replay protection in v1.0 is HMAC-only: a captured, correctly-signed
// request can be replayed. Timestamp-window + nonce dedup are
// post-v1.0 (PROJECT-DETAILS §4.13 gotchas).
type Authenticator interface {
	// Method reports the scheme, for logging and production warnings.
	Method() AuthMethod
	// Authenticate returns nil if the request is authentic, else
	// [ErrUnauthenticated].
	Authenticate(r *http.Request, body []byte) error
}

// NoneAuthenticator accepts every request. It is the default for a
// source with no configured auth; production deployments are warned
// (see config.ProductionWarnings).
type NoneAuthenticator struct{}

// Method implements [Authenticator].
func (NoneAuthenticator) Method() AuthMethod { return AuthNone }

// Authenticate implements [Authenticator]; always nil.
func (NoneAuthenticator) Authenticate(*http.Request, []byte) error { return nil }

// HMACAuthenticator verifies an HMAC-SHA256 of the raw request body,
// hex-encoded, carried in SignatureHeader with an optional Prefix
// (e.g. GitHub's "X-Hub-Signature-256: sha256=<hex>").
type HMACAuthenticator struct {
	// Secret is the shared HMAC key. Non-empty (enforced at build).
	Secret []byte
	// SignatureHeader carries the hex digest, e.g. "X-Hub-Signature-256".
	SignatureHeader string
	// Prefix is stripped from the header value before decoding, e.g.
	// "sha256=". Empty means the header is a bare hex digest.
	Prefix string
}

// Method implements [Authenticator].
func (HMACAuthenticator) Method() AuthMethod { return AuthHMAC }

// Authenticate implements [Authenticator]. The comparison is
// constant-time ([hmac.Equal]); a missing/malformed header is
// indistinguishable from a wrong signature in the returned error.
func (a HMACAuthenticator) Authenticate(r *http.Request, body []byte) error {
	got := r.Header.Get(a.SignatureHeader)
	if got == "" {
		return ErrUnauthenticated
	}
	got = strings.TrimPrefix(got, a.Prefix)
	want := hmac.New(sha256.New, a.Secret)
	want.Write(body)
	wantHex := hex.EncodeToString(want.Sum(nil))
	if !hmac.Equal([]byte(got), []byte(wantHex)) {
		return ErrUnauthenticated
	}
	return nil
}

// BearerAuthenticator verifies a shared secret carried verbatim in a
// header. With RequireScheme it expects "Bearer <token>" (the
// Authorization convention); without it the header value is the raw
// token (GitLab's "X-Gitlab-Token").
type BearerAuthenticator struct {
	// Header carries the token, e.g. "Authorization" or "X-Gitlab-Token".
	Header string
	// Token is the expected secret. Non-empty (enforced at build).
	Token []byte
	// RequireScheme strips a leading "Bearer " before comparing.
	RequireScheme bool
}

// Method implements [Authenticator].
func (BearerAuthenticator) Method() AuthMethod { return AuthBearer }

// Authenticate implements [Authenticator]. The comparison is
// constant-time ([subtle.ConstantTimeCompare]).
func (a BearerAuthenticator) Authenticate(r *http.Request, _ []byte) error {
	got := r.Header.Get(a.Header)
	if got == "" {
		return ErrUnauthenticated
	}
	if a.RequireScheme {
		const scheme = "Bearer "
		if !strings.HasPrefix(got, scheme) {
			return ErrUnauthenticated
		}
		got = got[len(scheme):]
	}
	if subtle.ConstantTimeCompare([]byte(got), a.Token) != 1 {
		return ErrUnauthenticated
	}
	return nil
}
