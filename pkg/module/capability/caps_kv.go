// SPDX-License-Identifier: Apache-2.0

package capability

import (
	"fmt"
	"sync"

	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

// KV is the in-process key-value capability (PROJECT-DETAILS §4.18
// "in-process key-value"). State is per-instance (one loaded
// module); an optional MaxFileSize in the manifest caps the number
// of keys (a coarse memory guard — the only scoping knob kv has).
type KV struct {
	mu      sync.RWMutex
	data    map[string]string
	maxKeys int64
}

// NewKV builds the capability. cfg.MaxFileSize, when set, is
// interpreted as a max key count (kv has no path/byte scope).
func NewKV(cfg manifest.CapabilityConfig) (*KV, error) {
	var maxKeys int64
	if cfg.MaxFileSize != "" {
		n, err := manifest.ParseSize(cfg.MaxFileSize)
		if err != nil {
			return nil, fmt.Errorf("kv max_keys: %w", err)
		}
		maxKeys = n
	}
	return &KV{data: make(map[string]string), maxKeys: maxKeys}, nil
}

// Get returns the value and whether it was present.
func (k *KV) Get(key string) (string, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	v, ok := k.data[key]
	return v, ok
}

// Set stores key=value, enforcing the optional key-count cap on
// inserts of new keys.
func (k *KV) Set(key, value string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, exists := k.data[key]; !exists && k.maxKeys > 0 &&
		int64(len(k.data)) >= k.maxKeys {
		return fmt.Errorf("%w: kv key count %d", ErrSizeExceeded, k.maxKeys)
	}
	k.data[key] = value
	return nil
}

// Delete removes key (no-op if absent).
func (k *KV) Delete(key string) {
	k.mu.Lock()
	delete(k.data, key)
	k.mu.Unlock()
}

// Len returns the current key count.
func (k *KV) Len() int {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return len(k.data)
}
