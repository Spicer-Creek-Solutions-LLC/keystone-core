// SPDX-License-Identifier: Apache-2.0

package ratelimit

import (
	"container/list"
	"sync"
	"time"
)

// DefaultCapacity is the [Registry] entry-count cap when
// [RegistryConfig.Capacity] is unset. 10 000 keyed buckets fit
// in ~320 KB — enough headroom for normal traffic while keeping
// a botnet-driven extractor from leaking memory.
const DefaultCapacity = 10_000

// RegistryConfig configures a [Registry].
type RegistryConfig struct {
	// Default is the [Config] applied to every newly-created
	// bucket. All keys share the same limit by design — per-
	// route or per-tenant overrides are a v1.x feature.
	Default Config

	// Capacity bounds the number of live keyed buckets. 0 uses
	// [DefaultCapacity]. Excess keys evict in LRU order on
	// insertion.
	Capacity int
}

// Registry caches one [Bucket] per key. Safe for concurrent use.
type Registry struct {
	cfg RegistryConfig

	mu    sync.Mutex
	items map[string]*list.Element
	lru   *list.List // front = most recently used
}

type entry struct {
	key    string
	bucket *Bucket
}

// NewRegistry returns a Registry with the given config.
func NewRegistry(cfg RegistryConfig) *Registry {
	if cfg.Capacity <= 0 {
		cfg.Capacity = DefaultCapacity
	}
	return &Registry{
		cfg:   cfg,
		items: make(map[string]*list.Element),
		lru:   list.New(),
	}
}

// Allow returns whether the key's bucket permits one more event.
// Creates the bucket on first use; promotes it to MRU position
// on every call (so hot keys do not get evicted).
func (r *Registry) Allow(key string) bool {
	b := r.bucket(key, time.Now())
	return b.Allow()
}

// AllowOrRetryAfter returns whether the key's bucket permits one
// more event and, if not, how long until the next token will be
// available. The middleware feeds the duration into the HTTP
// Retry-After header (Task 19).
func (r *Registry) AllowOrRetryAfter(key string) (bool, time.Duration) {
	b := r.bucket(key, time.Now())
	return b.AllowOrRetryAfter()
}

// Len returns the current number of keyed buckets.
func (r *Registry) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.items)
}

// bucket returns the [Bucket] for key, creating it on first use.
// The now parameter is a test seam — the only behavior it affects
// today is the not-yet-implemented per-entry idle eviction
// (which v1.0 does not ship; LRU on insertion is sufficient).
func (r *Registry) bucket(key string, _ time.Time) *Bucket {
	r.mu.Lock()
	defer r.mu.Unlock()
	if elem, ok := r.items[key]; ok {
		r.lru.MoveToFront(elem)
		return elem.Value.(*entry).bucket
	}
	e := &entry{key: key, bucket: New(r.cfg.Default)}
	elem := r.lru.PushFront(e)
	r.items[key] = elem
	r.evictIfFull()
	return e.bucket
}

// evictIfFull drops LRU-tail entries until the registry size is
// within capacity. Called with r.mu held.
func (r *Registry) evictIfFull() {
	for len(r.items) > r.cfg.Capacity {
		tail := r.lru.Back()
		if tail == nil {
			return
		}
		e := tail.Value.(*entry)
		r.lru.Remove(tail)
		delete(r.items, e.key)
	}
}
