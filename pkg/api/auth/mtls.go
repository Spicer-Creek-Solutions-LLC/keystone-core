// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// MTLSAuthenticator extracts the peer's verified TLS certificate from
// the gRPC peer info (or the equivalent injected by the HTTP
// middleware) and constructs a Principal from its SPIFFE URI SAN.
//
// v1.0 expects SPIFFE-shaped certs: `spiffe://<trust-domain>/<path>`
// in URI SAN. Trust-domain matching + path patterns are honored; the
// path determines the Principal Role via a callback (RoleResolver).
//
// Glob/regex CN/SAN matching from PROJECT-DETAILS §4.10 is a future
// extension; v1.0 does SPIFFE-only.
type MTLSAuthenticator struct {
	trustDomain  string
	roleResolver RoleResolver
	now          func() time.Time
}

// RoleResolver maps a SPIFFE path (everything after `spiffe://<td>/`)
// to a Role. Returns RoleNone + error to reject the principal.
type RoleResolver func(spiffePath string) (Role, error)

// MTLSConfig configures an MTLSAuthenticator.
type MTLSConfig struct {
	// TrustDomain is the expected SPIFFE trust domain, e.g.,
	// "kscore.local". Required; certs from other trust domains
	// produce ErrInvalidCredentials.
	TrustDomain string

	// RoleResolver maps the SPIFFE path to a Role. Required.
	// PROJECT-DETAILS §4.10 defaults: server/control-plane -> admin,
	// agent/* -> operator, service/* -> operator. Specific mapping
	// is the caller's choice.
	RoleResolver RoleResolver
}

// NewMTLSAuthenticator returns an Authenticator backed by config.
func NewMTLSAuthenticator(cfg MTLSConfig) (*MTLSAuthenticator, error) {
	if cfg.TrustDomain == "" {
		return nil, fmt.Errorf("auth: MTLSConfig.TrustDomain is required")
	}
	if cfg.RoleResolver == nil {
		return nil, fmt.Errorf("auth: MTLSConfig.RoleResolver is required")
	}
	return &MTLSAuthenticator{
		trustDomain:  cfg.TrustDomain,
		roleResolver: cfg.RoleResolver,
		now:          time.Now,
	}, nil
}

// SetClock overrides the clock used for AuthenticatedAt timestamping.
// Tests only.
func (a *MTLSAuthenticator) SetClock(now func() time.Time) {
	a.now = now
}

// Authenticate extracts the verified peer cert and constructs a
// Principal from its SPIFFE URI SAN.
func (a *MTLSAuthenticator) Authenticate(ctx context.Context) (*Principal, error) {
	cert, ok := peerCertFromContext(ctx)
	if !ok {
		return nil, ErrCredentialsNotFound
	}

	spiffeURI, err := spiffeURIFromCert(cert)
	if err != nil {
		return nil, err
	}

	if spiffeURI.Host != a.trustDomain {
		return nil, fmt.Errorf("%w: trust domain %q != expected %q",
			ErrInvalidCredentials, spiffeURI.Host, a.trustDomain)
	}
	path := strings.TrimPrefix(spiffeURI.Path, "/")
	if path == "" {
		return nil, fmt.Errorf("%w: empty SPIFFE path", ErrInvalidCredentials)
	}

	role, err := a.roleResolver(path)
	if err != nil {
		return nil, fmt.Errorf("%w: role resolution: %w", ErrInvalidCredentials, err)
	}
	if role == RoleNone {
		return nil, fmt.Errorf("%w: SPIFFE path %q resolved to RoleNone",
			ErrInvalidCredentials, path)
	}

	return &Principal{
		ID:              spiffeURI.String(),
		Name:            path,
		Role:            role,
		AuthMethod:      AuthMethodMTLS,
		AuthenticatedAt: a.now().UTC(),
	}, nil
}

// peerCertFromContext returns the first verified peer certificate,
// or (nil, false) if the context has no TLS peer info.
func peerCertFromContext(ctx context.Context) (*x509.Certificate, bool) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, false
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, false
	}
	if len(tlsInfo.State.VerifiedChains) == 0 {
		return nil, false
	}
	chain := tlsInfo.State.VerifiedChains[0]
	if len(chain) == 0 {
		return nil, false
	}
	return chain[0], true
}

// spiffeURIFromCert returns the first spiffe:// URI in cert's URI SANs.
// Returns ErrInvalidCredentials if none is present or if the URI fails
// to parse.
func spiffeURIFromCert(cert *x509.Certificate) (*url.URL, error) {
	for _, u := range cert.URIs {
		if u.Scheme == "spiffe" {
			return u, nil
		}
	}
	return nil, fmt.Errorf("%w: no spiffe:// URI in cert SANs", ErrInvalidCredentials)
}
