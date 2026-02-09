package verify

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// DefaultTrustPolicy implements TrustPolicy
type DefaultTrustPolicy struct {
	// trustedKeys maps identity to public key
	trustedKeys map[string][]byte

	// trustedKeyIDs is a set of trusted key IDs/fingerprints
	trustedKeyIDs map[string]bool

	mu sync.RWMutex
}

// NewTrustPolicy creates a new trust policy
func NewTrustPolicy() *DefaultTrustPolicy {
	return &DefaultTrustPolicy{
		trustedKeys:   make(map[string][]byte),
		trustedKeyIDs: make(map[string]bool),
	}
}

// IsTrusted checks if a key ID or identity is trusted
func (p *DefaultTrustPolicy) IsTrusted(identity string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Check if identity is directly trusted
	if _, exists := p.trustedKeys[identity]; exists {
		return true
	}

	// Check if it's a trusted key ID
	if p.trustedKeyIDs[identity] {
		return true
	}

	return false
}

// AddTrustedKey adds a trusted key
func (p *DefaultTrustPolicy) AddTrustedKey(identity string, publicKey []byte) error {
	if identity == "" {
		return fmt.Errorf("identity cannot be empty")
	}

	if len(publicKey) == 0 {
		return fmt.Errorf("public key cannot be empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Store the key
	p.trustedKeys[identity] = publicKey

	// Also store the key fingerprint
	fingerprint := computeKeyFingerprint(publicKey)
	p.trustedKeyIDs[fingerprint] = true

	return nil
}

// RemoveTrustedKey removes a trusted key
func (p *DefaultTrustPolicy) RemoveTrustedKey(identity string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Get the key to remove its fingerprint
	if publicKey, exists := p.trustedKeys[identity]; exists {
		fingerprint := computeKeyFingerprint(publicKey)
		delete(p.trustedKeyIDs, fingerprint)
	}

	delete(p.trustedKeys, identity)
	return nil
}

// ListTrustedKeys returns all trusted key identities
func (p *DefaultTrustPolicy) ListTrustedKeys() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	keys := make([]string, 0, len(p.trustedKeys))
	for identity := range p.trustedKeys {
		keys = append(keys, identity)
	}

	return keys
}

// GetPublicKey retrieves a public key by identity
func (p *DefaultTrustPolicy) GetPublicKey(identity string) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	publicKey, exists := p.trustedKeys[identity]
	if !exists {
		return nil, fmt.Errorf("key not found for identity: %s", identity)
	}

	return publicKey, nil
}

// AddTrustedKeyID adds a trusted key ID without the full public key
func (p *DefaultTrustPolicy) AddTrustedKeyID(keyID string) error {
	if keyID == "" {
		return fmt.Errorf("key ID cannot be empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.trustedKeyIDs[keyID] = true
	return nil
}

// computeKeyFingerprint computes a fingerprint for a public key
func computeKeyFingerprint(publicKey []byte) string {
	hash := sha256.Sum256(publicKey)
	return hex.EncodeToString(hash[:])
}

// CompositeTrustPolicy combines multiple trust policies.
type CompositeTrustPolicy struct {
	policies []TrustPolicy
	mu       sync.RWMutex
}

// NewCompositeTrustPolicy creates a composite trust policy
func NewCompositeTrustPolicy(policies ...TrustPolicy) *CompositeTrustPolicy {
	return &CompositeTrustPolicy{
		policies: policies,
	}
}

// IsTrusted checks if any of the policies trust the identity
func (p *CompositeTrustPolicy) IsTrusted(identity string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, policy := range p.policies {
		if policy.IsTrusted(identity) {
			return true
		}
	}

	return false
}

// AddTrustedKey adds a trusted key to all policies
func (p *CompositeTrustPolicy) AddTrustedKey(identity string, publicKey []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, policy := range p.policies {
		if err := policy.AddTrustedKey(identity, publicKey); err != nil {
			return err
		}
	}

	return nil
}

// RemoveTrustedKey removes a trusted key from all policies
func (p *CompositeTrustPolicy) RemoveTrustedKey(identity string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, policy := range p.policies {
		if err := policy.RemoveTrustedKey(identity); err != nil {
			return err
		}
	}

	return nil
}

// ListTrustedKeys returns all trusted keys from all policies
func (p *CompositeTrustPolicy) ListTrustedKeys() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	keysMap := make(map[string]bool)

	for _, policy := range p.policies {
		for _, key := range policy.ListTrustedKeys() {
			keysMap[key] = true
		}
	}

	keys := make([]string, 0, len(keysMap))
	for key := range keysMap {
		keys = append(keys, key)
	}

	return keys
}

// AddPolicy adds a new policy to the composite
func (p *CompositeTrustPolicy) AddPolicy(policy TrustPolicy) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.policies = append(p.policies, policy)
}
