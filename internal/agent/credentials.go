// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Credentials is what the control plane hands this agent once its
// bootstrap proof is accepted: an API key, and — when the server runs
// with identity enabled — a per-agent X509 SVID plus the trust bundle
// to verify the server with.
//
// The JSON tags mirror controlplane.AgentCredentials field for field.
// They are deliberately duplicated rather than imported: internal/
// controlplane imports internal/agent, so importing back would close a
// cycle. controlplane's own test pins the two shapes together, which
// is the right direction for the check to run in — the wire format
// belongs to the protocol, not to either end of it.
//
// The private key is the reason this type exists. Before this, the
// server minted a per-agent SVID at bootstrap and the agent dropped it
// on the floor, so every agent's identity on the NATS path came down
// to a fleet-wide shared HMAC key — which proves membership, not
// which member.
type Credentials struct {
	APIKey   string    `json:"api_key"` //nolint:gosec // the point of the bootstrap protocol is that the agent receives this cleartext exactly once
	AgentID  string    `json:"agent_id"`
	IssuedAt time.Time `json:"issued_at"`

	// CertChainPEM is [leaf, signing CA] as PEM CERTIFICATE blocks.
	// Empty when the server issues API keys only (identity disabled).
	CertChainPEM string `json:"cert_chain_pem,omitempty"`
	// PrivateKeyPEM is the leaf's PKCS#8 key. Never leaves this host.
	PrivateKeyPEM string `json:"private_key_pem,omitempty"` //nolint:gosec // legitimate cleartext key, persisted 0600
	// TrustBundlePEM is the X509 authorities to verify the server.
	TrustBundlePEM string `json:"trust_bundle_pem,omitempty"`
}

// HasSVID reports whether these credentials carry a usable per-agent
// certificate. False for an API-key-only issuer, which is the shape a
// server with identity disabled returns.
func (c *Credentials) HasSVID() bool {
	return c != nil && c.CertChainPEM != "" && c.PrivateKeyPEM != ""
}

// LeafNotAfter returns the expiry of the leaf certificate — the first
// CERTIFICATE block in the chain. Returns the zero time when there is
// no SVID, and an error when the chain is present but unparseable,
// which is a corrupt credential file rather than an absent one.
func (c *Credentials) LeafNotAfter() (time.Time, error) {
	if !c.HasSVID() {
		return time.Time{}, nil
	}
	block, _ := pem.Decode([]byte(c.CertChainPEM))
	if block == nil {
		return time.Time{}, errors.New("agent: credentials: cert chain is not PEM")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, fmt.Errorf("agent: credentials: parse leaf: %w", err)
	}
	return leaf.NotAfter, nil
}

// Valid reports whether these credentials are usable at now. An
// API-key-only credential is valid whenever it carries a key: there is
// nothing in it that expires. An SVID credential is valid until its
// leaf does.
//
// A credential that fails to parse is NOT valid — the agent should
// re-bootstrap rather than carry on with something it cannot read.
func (c *Credentials) Valid(now time.Time) bool {
	if c == nil || c.APIKey == "" {
		return false
	}
	if !c.HasSVID() {
		return true
	}
	notAfter, err := c.LeafNotAfter()
	if err != nil {
		return false
	}
	return now.Before(notAfter)
}

// ErrNoCredentials is returned by Load when nothing has been stored
// yet. Distinguished from a read failure so a first boot is not
// reported as an error.
var ErrNoCredentials = errors.New("agent: no stored credentials")

// CredentialStore persists Credentials to a single JSON file.
//
// One file, not a directory of PEMs: the credential is issued
// atomically and is only meaningful whole — a cert chain without its
// key, or either without the trust bundle that verifies the issuer, is
// not a usable identity. Writing it as one object makes a torn write
// impossible to mistake for a partial-but-usable state.
type CredentialStore struct {
	// Path is the file to read and write. Its parent directory is
	// created on Save with 0700.
	Path string
}

// Load reads the stored credentials. Returns ErrNoCredentials when the
// file does not exist.
func (s *CredentialStore) Load() (*Credentials, error) {
	if s == nil || s.Path == "" {
		return nil, ErrNoCredentials
	}
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("agent: credentials read %q: %w", s.Path, err)
	}
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("agent: credentials decode %q: %w", s.Path, err)
	}
	return &c, nil
}

// Save writes the credentials 0600, replacing any previous file.
//
// Write-temp-then-rename within the same directory: rename(2) is
// atomic there, so a crash mid-write leaves either the old credential
// or the new one, never a truncated file the agent would then refuse
// to parse and re-bootstrap against a consumed PSK.
func (s *CredentialStore) Save(c *Credentials) error {
	if s == nil || s.Path == "" {
		return errors.New("agent: credentials: no path configured")
	}
	if c == nil {
		return errors.New("agent: credentials: nothing to save")
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("agent: credentials dir %q: %w", dir, err)
	}
	b, err := json.Marshal(c) //nolint:gosec // marshaling the credential is the point; the file is written 0600
	if err != nil {
		return fmt.Errorf("agent: credentials encode: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".credentials-*.tmp")
	if err != nil {
		return fmt.Errorf("agent: credentials temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once renamed

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: credentials chmod: %w", err)
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: credentials write: %w", err)
	}
	// Sync before rename: rename only orders the directory entry, not
	// the file's own data. Without this a power loss can leave the new
	// name pointing at zero bytes.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: credentials sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("agent: credentials close: %w", err)
	}
	if err := os.Rename(tmpName, s.Path); err != nil {
		return fmt.Errorf("agent: credentials rename to %q: %w", s.Path, err)
	}
	return nil
}
