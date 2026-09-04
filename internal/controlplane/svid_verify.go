// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/agent"
)

// Errors from SVIDVerifier.Verify. Callers distinguish them so a
// rejection can say which check failed without leaking the
// certificate contents into an operator-facing message.
var (
	// ErrSVIDUntrusted means the chain does not build a path to a
	// configured CA.
	ErrSVIDUntrusted = errors.New("controlplane: svid: certificate is not trusted")
	// ErrSVIDIdentityMismatch means the certificate names a different
	// agent than the request claims -- the impersonation case.
	ErrSVIDIdentityMismatch = errors.New("controlplane: svid: claimed agent id does not match the certificate")
	// ErrSVIDStale means the request is outside the freshness window.
	ErrSVIDStale = errors.New("controlplane: svid: request is outside the freshness window")
	// ErrSVIDSignature means the signature does not verify.
	ErrSVIDSignature = errors.New("controlplane: svid: signature does not verify")
)

// Defaults for the freshness window. A captured request stays usable
// for at most MaxAge; a small amount of clock skew ahead of the server
// is tolerated because agents are separate machines.
const (
	defaultSVIDMaxAge = 60 * time.Second
	defaultSVIDSkew   = 30 * time.Second
)

// SVIDVerifier establishes WHICH agent sent a request.
//
// The whole point is that it does not consult any per-agent state: the
// agent presents the certificate the control plane issued it, the
// verifier builds a path to the CA, and the identity comes out of the
// leaf. That means an agent can be authenticated on a server process
// that has never seen it before, and an agent cannot promote itself by
// editing a field.
//
// It replaces nothing yet. Everything on the agent NATS path is still
// HMAC-authenticated with the fleet-wide key; this is the primitive
// that makes per-agent authorization possible, landing ahead of its
// first consumer so the signature checking is reviewable on its own
// rather than buried inside a feature.
type SVIDVerifier struct {
	// Roots are the CA certificates a leaf must chain to.
	Roots *x509.CertPool
	// MaxAge bounds how long a captured request stays usable.
	// Zero means defaultSVIDMaxAge.
	MaxAge time.Duration
	// Skew tolerates an agent clock running ahead of the server's.
	// Zero means defaultSVIDSkew.
	Skew time.Duration
	// Now is the clock, injectable for tests. Nil means time.Now.
	Now func() time.Time
}

func (v *SVIDVerifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

func (v *SVIDVerifier) maxAge() time.Duration {
	if v.MaxAge > 0 {
		return v.MaxAge
	}
	return defaultSVIDMaxAge
}

func (v *SVIDVerifier) skew() time.Duration {
	if v.Skew > 0 {
		return v.Skew
	}
	return defaultSVIDSkew
}

// Verify authenticates req and returns the agent id the certificate
// attests to, along with the signed payload.
//
// The returned id is the one from the certificate. Callers must use it
// and not req.AgentID -- they are checked to be equal here, but taking
// the verified one keeps that guarantee from depending on a caller
// remembering which field was authoritative.
//
// Freshness is a window, not replay protection: a request captured and
// re-sent inside MaxAge verifies again. For a read that returns the
// same value to the same agent on its own subject that is acceptable;
// the ROADMAP entry "Replay protection on agent commands" covers the
// nonce-tracking side for paths where it is not.
func (v *SVIDVerifier) Verify(req *agent.SignedRequest) (string, []byte, error) {
	if req == nil {
		return "", nil, errors.New("controlplane: svid: nil request")
	}
	if v.Roots == nil {
		return "", nil, errors.New("controlplane: svid: no trust roots configured")
	}

	leaf, intermediates, err := parseChainPEM(req.CertChainPEM)
	if err != nil {
		return "", nil, err
	}

	// Verify the chain before anything else touches the certificate's
	// contents: an unverified leaf's URI SAN is attacker-controlled
	// text, and reading an identity out of it first would be trusting
	// exactly what has not been established yet.
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         v.Roots,
		Intermediates: intermediates,
		CurrentTime:   v.now(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrSVIDUntrusted, err)
	}

	certID, err := agent.AgentIDFromCert(leaf)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrSVIDUntrusted, err)
	}
	if req.AgentID != "" && req.AgentID != certID {
		return "", nil, fmt.Errorf("%w: claimed %q, certificate says %q",
			ErrSVIDIdentityMismatch, req.AgentID, certID)
	}

	now := v.now()
	if req.IssuedAt.IsZero() {
		return "", nil, fmt.Errorf("%w: no issued_at", ErrSVIDStale)
	}
	if age := now.Sub(req.IssuedAt); age > v.maxAge() {
		return "", nil, fmt.Errorf("%w: issued %s ago, limit %s", ErrSVIDStale, age, v.maxAge())
	}
	if ahead := req.IssuedAt.Sub(now); ahead > v.skew() {
		return "", nil, fmt.Errorf("%w: issued %s in the future, skew allowance %s",
			ErrSVIDStale, ahead, v.skew())
	}

	if err := agent.VerifySignature(leaf.PublicKey, req); err != nil {
		return "", nil, fmt.Errorf("%w: %v", ErrSVIDSignature, err)
	}
	return certID, req.Payload, nil
}

// parseChainPEM splits a PEM chain into its leaf and the rest.
func parseChainPEM(chainPEM string) (*x509.Certificate, *x509.CertPool, error) {
	if chainPEM == "" {
		return nil, nil, fmt.Errorf("%w: empty certificate chain", ErrSVIDUntrusted)
	}
	rest := []byte(chainPEM)
	var leaf *x509.Certificate
	intermediates := x509.NewCertPool()
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: parse: %v", ErrSVIDUntrusted, err)
		}
		if leaf == nil {
			leaf = cert
			continue
		}
		intermediates.AddCert(cert)
	}
	if leaf == nil {
		return nil, nil, fmt.Errorf("%w: chain carries no certificate", ErrSVIDUntrusted)
	}
	return leaf, intermediates, nil
}

// SVIDRootsFromCerts builds a trust pool from parsed CA certificates —
// what identity.TrustBundle.X509Authorities returns.
func SVIDRootsFromCerts(authorities []*x509.Certificate) (*x509.CertPool, error) {
	if len(authorities) == 0 {
		return nil, errors.New("controlplane: svid: trust bundle carries no authorities")
	}
	pool := x509.NewCertPool()
	for _, c := range authorities {
		if c == nil {
			continue
		}
		pool.AddCert(c)
	}
	return pool, nil
}

// SVIDRootsFromPEM builds a trust pool from PEM authorities — what an
// agent stores as TrustBundlePEM and what the server holds as its own
// CA bundle.
func SVIDRootsFromPEM(bundlePEM string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(bundlePEM)) {
		return nil, errors.New("controlplane: svid: trust bundle carries no usable certificate")
	}
	return pool, nil
}
