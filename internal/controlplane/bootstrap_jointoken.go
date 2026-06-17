// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/identity"
)

// ErrJoinTokenAgentMismatch is returned when the cleartext token's
// AgentID doesn't match the agent's claimed identity in the
// bootstrap request. The mismatch suggests the operator handed
// the token to the wrong agent — a configuration error, not a
// protocol failure.
var ErrJoinTokenAgentMismatch = errors.New("controlplane: bootstrap: token agent_id mismatch")

// JoinTokenBootstrapValidator implements [BootstrapValidator] by
// running the cleartext token through an
// [identity.JoinTokenAttestor]. Compared to [PSKValidator]:
//
//   - Storage is shared with the operator-facing
//     `kscore-identity` CLI: the same token an operator minted
//     via `kscore-identity token create` is the one the agent
//     presents here.
//   - Atomic MaxUses consumption: the attestor's MarkUsed runs
//     under the store's lock; concurrent agents racing on the
//     same token see exactly MaxUses wins.
//   - SPIFFE-aware: rejects when the token's AgentID doesn't
//     match the request's claimed agent_id (defense-in-depth
//     against misused tokens).
//
// Task 14 wires this into [BootstrapHandlerConfig.Validator] when
// `cfg.Identity.Enabled`; [PSKValidator] remains the fallback when
// identity is off.
type JoinTokenBootstrapValidator struct {
	attestor    *identity.JoinTokenAttestor
	trustDomain string
}

// NewJoinTokenBootstrapValidator wires an attestor + the expected
// trust domain. The trust domain is used to construct the SPIFFE
// ID the validator extracts the attested agent ID from.
func NewJoinTokenBootstrapValidator(attestor *identity.JoinTokenAttestor, trustDomain string) (*JoinTokenBootstrapValidator, error) {
	if attestor == nil {
		return nil, errors.New("controlplane: NewJoinTokenBootstrapValidator: attestor is required")
	}
	if trustDomain == "" {
		return nil, errors.New("controlplane: NewJoinTokenBootstrapValidator: trustDomain is required")
	}
	return &JoinTokenBootstrapValidator{
		attestor:    attestor,
		trustDomain: trustDomain,
	}, nil
}

// Validate satisfies [BootstrapValidator]. `proof` MUST be the
// cleartext token bytes; `claimedID` MUST match the token's
// AgentID. Errors wrap either [identity.ErrAttestation] (token
// rejected by the attestor) or [ErrJoinTokenAgentMismatch]
// (token valid, but for a different agent).
func (v *JoinTokenBootstrapValidator) Validate(ctx context.Context, claimedID string, proof []byte) error {
	if claimedID == "" {
		return errors.New("controlplane: bootstrap: agent_id is required")
	}
	if len(proof) == 0 {
		return errors.New("controlplane: bootstrap: proof is required")
	}

	result, err := v.attestor.Attest(ctx, proof)
	if err != nil {
		return fmt.Errorf("controlplane: bootstrap attest: %w", err)
	}
	// Extract /agent/<id> from the attested SPIFFE path.
	segs := result.ID.Segments()
	if len(segs) != 2 || segs[0] != "agent" {
		return fmt.Errorf("controlplane: bootstrap: attested ID %q is not an agent identity", result.ID)
	}
	attestedID := segs[1]
	// Constant-time comparison defeats the timing side channel
	// an attacker could exploit by spamming bootstrap requests
	// with different claimedID values.
	if subtle.ConstantTimeCompare([]byte(attestedID), []byte(claimedID)) != 1 {
		return fmt.Errorf("%w: token bound to %q, request claimed %q",
			ErrJoinTokenAgentMismatch, attestedID, claimedID)
	}
	return nil
}

// SVIDBootstrapIssuer wraps an existing [CredentialIssuer] (the
// v0.1 [APIKeyIssuer] in production) and augments the returned
// [AgentCredentials] with an [identity.X509SVID] + the trust
// bundle, PEM-encoded for the wire.
//
// The base issuer is preserved so v0.1 agents that don't yet
// speak mTLS still receive an API key for HTTP fallback. SVID-
// aware agents check `creds.CertChainPEM != ""` and decode.
type SVIDBootstrapIssuer struct {
	provider *identity.EmbeddedProvider
	base     CredentialIssuer
	ttl      time.Duration
}

// NewSVIDBootstrapIssuer constructs the augmented issuer. ttl=0
// falls back to the provider's CAConfig.MaxSVIDTTL. The base
// issuer is required — task 14 keeps the API-key path as a
// fallback so v0.1 agents continue to authenticate over HTTP
// without an immediate mTLS lift.
func NewSVIDBootstrapIssuer(provider *identity.EmbeddedProvider, base CredentialIssuer, ttl time.Duration) (*SVIDBootstrapIssuer, error) {
	if provider == nil {
		return nil, errors.New("controlplane: NewSVIDBootstrapIssuer: provider is required")
	}
	if base == nil {
		return nil, errors.New("controlplane: NewSVIDBootstrapIssuer: base CredentialIssuer is required")
	}
	if ttl < 0 {
		return nil, errors.New("controlplane: NewSVIDBootstrapIssuer: ttl must be >= 0")
	}
	return &SVIDBootstrapIssuer{
		provider: provider,
		base:     base,
		ttl:      ttl,
	}, nil
}

// Issue mints the agent's X509SVID, PEM-encodes the chain + key,
// pulls the current trust bundle, and merges with the base
// issuer's API-key result. The base issuer's `AgentID` +
// `IssuedAt` win; we don't second-guess them.
func (i *SVIDBootstrapIssuer) Issue(ctx context.Context, agentID string) (AgentCredentials, error) {
	if agentID == "" {
		return AgentCredentials{}, errors.New("controlplane: SVIDBootstrapIssuer: agent_id is required")
	}

	// Base credentials (API key) first — preserves the v0.1
	// fallback path even if SVID issuance fails below. If the
	// operator wants a strict "SVID or nothing" policy they can
	// run the SVID issuer alone (no base) once that path is
	// wired in a follow-up.
	base, err := i.base.Issue(ctx, agentID)
	if err != nil {
		return AgentCredentials{}, fmt.Errorf("controlplane: SVIDBootstrapIssuer base: %w", err)
	}

	id, err := identity.AgentID(i.provider.TrustDomain(), agentID)
	if err != nil {
		return AgentCredentials{}, fmt.Errorf("controlplane: SVIDBootstrapIssuer: invalid agent id: %w", err)
	}
	svid, err := i.provider.IssueX509SVID(ctx, identity.IssueX509SVIDRequest{
		ID:  id,
		TTL: i.ttl, // 0 → provider's DefaultSVIDTTL
	})
	if err != nil {
		return AgentCredentials{}, fmt.Errorf("controlplane: SVIDBootstrapIssuer issue svid: %w", err)
	}

	chainPEM, err := encodeChainPEM(svid.Chain())
	if err != nil {
		return AgentCredentials{}, fmt.Errorf("controlplane: SVIDBootstrapIssuer encode chain: %w", err)
	}
	keyPEM, err := encodePrivateKeyPEM(svid.PrivateKey())
	if err != nil {
		return AgentCredentials{}, fmt.Errorf("controlplane: SVIDBootstrapIssuer encode key: %w", err)
	}
	bundle, err := i.provider.GetTrustBundle(ctx)
	if err != nil {
		return AgentCredentials{}, fmt.Errorf("controlplane: SVIDBootstrapIssuer trust bundle: %w", err)
	}
	bundlePEM, err := encodeChainPEM(bundle.X509Authorities())
	if err != nil {
		return AgentCredentials{}, fmt.Errorf("controlplane: SVIDBootstrapIssuer encode bundle: %w", err)
	}

	base.CertChainPEM = chainPEM
	base.PrivateKeyPEM = keyPEM
	base.TrustBundlePEM = bundlePEM
	return base, nil
}

// agentCertMeta extracts metadata from an issued chain PEM (the leaf is
// the first CERTIFICATE block): hex SHA-256 fingerprint of the leaf, its
// NotAfter, and the spiffe:// URI SAN (empty if none). Best-effort —
// callers treat a parse error as "metadata unavailable", not fatal.
func agentCertMeta(chainPEM string) (fingerprint string, notAfter time.Time, spiffeID string, err error) {
	block, _ := pem.Decode([]byte(chainPEM))
	if block == nil {
		return "", time.Time{}, "", errors.New("no PEM CERTIFICATE block")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", time.Time{}, "", fmt.Errorf("parse leaf: %w", err)
	}
	sum := sha256.Sum256(leaf.Raw)
	for _, u := range leaf.URIs {
		if u != nil && u.Scheme == "spiffe" {
			spiffeID = u.String()
			break
		}
	}
	return hex.EncodeToString(sum[:]), leaf.NotAfter, spiffeID, nil
}

// encodeChainPEM concatenates PEM CERTIFICATE blocks for every
// non-nil cert in `certs`. Nil entries are skipped (defensive —
// the SVID + bundle invariants reject them upstream).
func encodeChainPEM(certs []*x509.Certificate) (string, error) {
	if len(certs) == 0 {
		return "", errors.New("empty cert slice")
	}
	var buf bytes.Buffer
	for i, c := range certs {
		if c == nil {
			continue
		}
		if err := pem.Encode(&buf, &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: c.Raw,
		}); err != nil {
			return "", fmt.Errorf("pem encode cert[%d]: %w", i, err)
		}
	}
	out := buf.String()
	if !strings.HasPrefix(out, "-----BEGIN CERTIFICATE-----") {
		return "", errors.New("encoded chain has no CERTIFICATE blocks")
	}
	return out, nil
}

// encodePrivateKeyPEM PKCS#8-encodes the key inside a PEM PRIVATE
// KEY block. Works for every key type the identity package
// generates (ECDSA / RSA).
func encodePrivateKeyPEM(key any) (string, error) {
	if key == nil {
		return "", errors.New("nil private key")
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("marshal pkcs8: %w", err)
	}
	var buf bytes.Buffer
	if err := pem.Encode(&buf, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	}); err != nil {
		return "", fmt.Errorf("pem encode key: %w", err)
	}
	return buf.String(), nil
}
