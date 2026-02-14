package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const trustFileName = "trust.json"

// TrustConfig configures trust management.
type TrustConfig struct {
	// TrustDir is the directory storing trust root data.
	TrustDir string

	// RequireSignatures rejects unsigned modules on import/verify.
	RequireSignatures bool
}

// TrustRoot represents a trusted signing key.
type TrustRoot struct {
	Name      string     `json:"name"`
	PublicKey []byte     `json:"public_key"`
	Algorithm string     `json:"algorithm"` // ed25519, ecdsa, rsa
	AddedAt   time.Time  `json:"added_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// TrustStore manages trusted signing keys for the offline registry.
type TrustStore struct {
	config TrustConfig
	roots  []TrustRoot
}

// NewTrustStore creates a new trust store. Call Init() to create the directory,
// or LoadRoots() to load existing trust data.
func NewTrustStore(cfg TrustConfig) (*TrustStore, error) {
	if cfg.TrustDir == "" {
		return nil, fmt.Errorf("trust directory is required")
	}
	ts := &TrustStore{config: cfg}
	// Try to load existing roots
	_ = ts.LoadRoots()
	return ts, nil
}

// Init creates the trust directory and an empty trust file.
func (ts *TrustStore) Init() error {
	//nolint:gosec // G301: trust directory needs to be accessible
	if err := os.MkdirAll(ts.config.TrustDir, 0o755); err != nil {
		return fmt.Errorf("create trust dir: %w", err)
	}
	ts.roots = nil
	return ts.SaveRoots()
}

// AddRoot adds a trusted signing key.
func (ts *TrustStore) AddRoot(root TrustRoot) error {
	if root.Name == "" {
		return fmt.Errorf("trust root name is required")
	}
	if len(root.PublicKey) == 0 {
		return fmt.Errorf("public key is required")
	}

	// Check for duplicate name
	for _, r := range ts.roots {
		if r.Name == root.Name {
			return fmt.Errorf("trust root %q already exists", root.Name)
		}
	}

	if root.AddedAt.IsZero() {
		root.AddedAt = time.Now().UTC()
	}

	ts.roots = append(ts.roots, root)
	return ts.SaveRoots()
}

// RemoveRoot removes a trusted signing key by name.
func (ts *TrustStore) RemoveRoot(name string) error {
	for i, r := range ts.roots {
		if r.Name == name {
			ts.roots = append(ts.roots[:i], ts.roots[i+1:]...)
			return ts.SaveRoots()
		}
	}
	return fmt.Errorf("trust root %q not found", name)
}

// ListRoots returns all trusted signing keys.
func (ts *TrustStore) ListRoots() []TrustRoot {
	result := make([]TrustRoot, len(ts.roots))
	copy(result, ts.roots)
	return result
}

// RequireSignatures returns whether unsigned modules should be rejected.
func (ts *TrustStore) RequireSignatures() bool {
	return ts.config.RequireSignatures
}

// LoadRoots reads trust roots from the trust file.
func (ts *TrustStore) LoadRoots() error {
	path := filepath.Join(ts.config.TrustDir, trustFileName)
	//nolint:gosec // G304: path is constructed from trusted config directory
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			ts.roots = nil
			return nil
		}
		return fmt.Errorf("read trust file: %w", err)
	}

	var roots []TrustRoot
	if err := json.Unmarshal(data, &roots); err != nil {
		return fmt.Errorf("parse trust file: %w", err)
	}

	ts.roots = roots
	return nil
}

// SaveRoots writes trust roots to the trust file.
func (ts *TrustStore) SaveRoots() error {
	data, err := json.MarshalIndent(ts.roots, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal trust roots: %w", err)
	}

	path := filepath.Join(ts.config.TrustDir, trustFileName)
	//nolint:gosec // G306: trust file needs to be readable
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write trust file: %w", err)
	}
	return nil
}

// ActiveRoots returns trust roots that are not expired.
func (ts *TrustStore) ActiveRoots() []TrustRoot {
	now := time.Now()
	var active []TrustRoot
	for _, r := range ts.roots {
		if r.ExpiresAt != nil && r.ExpiresAt.Before(now) {
			continue
		}
		active = append(active, r)
	}
	return active
}
