// SPDX-License-Identifier: Apache-2.0

package verify

import (
	"crypto"
	"fmt"
	"sort"
	"sync"
)

// TrustPolicy is the set of public keys a module signature may be
// verified against, indexed by [KeyID]. v1.0 trust = a key in this
// policy AND the module fetched over a TLS-trusted registry (the
// transport half is the registry client's responsibility, tasks
// 8/9). Rotation = add the new key (a distinct KeyID); both the old
// and new key can be trusted during the overlap.
type TrustPolicy struct {
	mu   sync.RWMutex
	keys map[string]crypto.PublicKey
}

// NewTrustPolicy returns an empty policy.
func NewTrustPolicy() *TrustPolicy {
	return &TrustPolicy{keys: make(map[string]crypto.PublicKey)}
}

// LoadTrustPolicy builds a policy from one or more PKIX PEM public
// keys.
func LoadTrustPolicy(pems ...[]byte) (*TrustPolicy, error) {
	tp := NewTrustPolicy()
	for i, p := range pems {
		if err := tp.AddKeyPEM(p); err != nil {
			return nil, fmt.Errorf("trust key #%d: %w", i, err)
		}
	}
	return tp, nil
}

// AddKey trusts pub, returning its [KeyID]. Idempotent (re-adding
// the same key is a no-op returning the same ID).
func (tp *TrustPolicy) AddKey(pub crypto.PublicKey) (string, error) {
	if _, err := algorithmFor(pub); err != nil {
		return "", err
	}
	id, err := KeyID(pub)
	if err != nil {
		return "", err
	}
	tp.mu.Lock()
	tp.keys[id] = pub
	tp.mu.Unlock()
	return id, nil
}

// AddKeyPEM parses a PKIX PEM public key and trusts it.
func (tp *TrustPolicy) AddKeyPEM(pemBytes []byte) error {
	pub, err := ParsePublicKeyPEM(pemBytes)
	if err != nil {
		return err
	}
	_, err = tp.AddKey(pub)
	return err
}

// lookup returns the trusted key for id.
func (tp *TrustPolicy) lookup(id string) (crypto.PublicKey, bool) {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	pub, ok := tp.keys[id]
	return pub, ok
}

// KeyIDs returns the trusted key IDs, sorted (deterministic).
func (tp *TrustPolicy) KeyIDs() []string {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	out := make([]string, 0, len(tp.keys))
	for id := range tp.keys {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
