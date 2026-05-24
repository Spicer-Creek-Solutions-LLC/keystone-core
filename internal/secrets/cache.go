// SPDX-License-Identifier: Apache-2.0

package secrets

// CacheStats is the operator-visible view of cache effectiveness.
// Surfaced by `/api/status` (task 9). Task 8 fills in the real
// counters; task 3's [NoopCache] leaves them all zero.
type CacheStats struct {
	Hits        uint64 `json:"hits"`
	Misses      uint64 `json:"misses"`
	Evictions   uint64 `json:"evictions"`
	Expired     uint64 `json:"expired"`
	Entries     int    `json:"entries"`
	MemoryBytes int64  `json:"memory_bytes"`
}

// Cache is the seam the [Broker] consults on every read and updates
// on every write / delete / revoke. Task 8's encrypted in-memory cache
// (AES-GCM, TTL, bounded LRU, prefix-delete) satisfies it; task 3's
// default is [NoopCache] so the broker stays usable before task 8
// lands.
//
// Get returns (value, true) on hit and (nil, false) on miss. The
// returned [Secret] is owned by the cache — callers MUST treat it as
// read-only (the broker hands out [Secret.Clone] copies to gRPC
// handlers; task 8 may also defensively encrypt-at-rest).
//
// Put accepts the secret to cache. Implementations decide TTL,
// eviction, and at-rest encryption; the broker just hands over what
// the backend returned.
//
// InvalidatePath drops a single path. InvalidatePrefix drops every
// entry whose path begins with the supplied string — used on
// directory-style deletes (KV v2 destroy) and on `RevokeLease` so a
// revoked credential never reads from a stale cache hit.
//
// Stats is a snapshot; concurrent updates are allowed and the values
// may move under your feet.
type Cache interface {
	Get(path string) (*Secret, bool)
	Put(path string, secret *Secret)
	InvalidatePath(path string)
	InvalidatePrefix(prefix string)
	Stats() CacheStats
}

// NoopCache is the broker default — every Get is a miss, every other
// method discards. Concurrent-safe (no state). Task 8 swaps in the
// real AES-GCM-at-rest cache.
type NoopCache struct{}

// Get returns (nil, false).
func (NoopCache) Get(string) (*Secret, bool) { return nil, false }

// Put discards.
func (NoopCache) Put(string, *Secret) {}

// InvalidatePath discards.
func (NoopCache) InvalidatePath(string) {}

// InvalidatePrefix discards.
func (NoopCache) InvalidatePrefix(string) {}

// Stats returns the zero value.
func (NoopCache) Stats() CacheStats { return CacheStats{} }

// Compile-time assertion that NoopCache implements [Cache]. Mirrors
// the same pattern across the rest of the package.
var _ Cache = NoopCache{}

// DefaultCache returns the canonical [NoopCache]. Provided so
// [BrokerConfig] zero values resolve to a known no-op without a
// per-broker allocation.
func DefaultCache() Cache { return NoopCache{} }
