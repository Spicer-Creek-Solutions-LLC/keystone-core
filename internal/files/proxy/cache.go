// SPDX-License-Identifier: Apache-2.0

package proxy

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/internal/files/transport"
)

// Getter is the read-only seam Cache wraps. [transport.Client]
// satisfies it; tests can inject stubs without spinning up NATS.
type Getter interface {
	Get(ctx context.Context, path string, opts transport.GetOptions) (files.FileMetadata, []byte, error)
}

// Config controls a [Cache].
type Config struct {
	// Capacity is the maximum number of entries the cache holds.
	// 0 means "unbounded" (TTL alone bounds memory — only use when
	// the working set is genuinely small and the operator is
	// comfortable letting the cache grow).
	Capacity int

	// TTL is the per-entry staleness budget. 0 means "no TTL"
	// (rely on Capacity-driven eviction only).
	TTL time.Duration
}

// Cache wraps a Getter with an LRU + TTL cache for file bodies.
// One Cache per agent process; safe for concurrent use.
type Cache struct {
	source  Getter
	cfg     Config
	metrics *Metrics
	now     func() time.Time

	mu    sync.Mutex
	items map[string]*list.Element
	lru   *list.List
}

// entry is one cached body keyed by path.
type entry struct {
	path     string
	meta     files.FileMetadata
	body     []byte
	inserted time.Time
}

// New constructs a Cache. nil source is rejected; nil metrics is
// allowed and disables emission. nowFunc is the clock seam (nil
// → [time.Now]).
func New(source Getter, cfg Config, m *Metrics, nowFunc func() time.Time) (*Cache, error) {
	if source == nil {
		return nil, errors.New("proxy: source must not be nil")
	}
	if cfg.Capacity < 0 {
		return nil, fmt.Errorf("proxy: capacity must be >= 0, got %d", cfg.Capacity)
	}
	if cfg.TTL < 0 {
		return nil, fmt.Errorf("proxy: ttl must be >= 0, got %v", cfg.TTL)
	}
	if nowFunc == nil {
		nowFunc = time.Now
	}
	return &Cache{
		source:  source,
		cfg:     cfg,
		metrics: m,
		now:     nowFunc,
		items:   make(map[string]*list.Element),
		lru:     list.New(),
	}, nil
}

// Get satisfies [Getter]. On a cache miss the source is consulted
// outside the cache lock — concurrent misses for different paths
// do not serialise. The returned slice is the cache's own copy;
// the caller may mutate it freely without corrupting cached state.
func (c *Cache) Get(ctx context.Context, path string, opts transport.GetOptions) (files.FileMetadata, []byte, error) {
	// Partial-resume reads bypass the cache entirely — a body that
	// starts at chunk K is not a substitute for a full body.
	if opts.FromChunk > 0 {
		c.metrics.RecordMiss(ReasonBypass)
		meta, body, err := c.source.Get(ctx, path, opts)
		return meta, body, err
	}

	if meta, body, ok := c.lookup(path); ok {
		c.metrics.RecordHit()
		return meta, body, nil
	}

	meta, body, err := c.source.Get(ctx, path, opts)
	if err != nil {
		return meta, body, err
	}
	c.store(path, meta, body)
	return meta, copyBytes(body), nil
}

// Invalidate drops the cached entry for path. Callers that wrote
// to the source (via transport.Client.Put or .Delete) must call
// this so the cache does not serve a stale body. Returns true if
// an entry was actually removed.
func (c *Cache) Invalidate(path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[path]
	if !ok {
		return false
	}
	c.lru.Remove(elem)
	delete(c.items, path)
	return true
}

// Len returns the current entry count. Useful for tests and
// operational metrics tooling that scrapes the binary.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// lookup checks the cache, moves a hit to MRU position, and
// returns (meta, body, true) on a fresh hit. An expired entry is
// evicted in the same call and the function returns false with the
// expired-reason recorded so the caller knows whether to attribute
// the miss to cold-cache or to TTL.
func (c *Cache) lookup(path string) (files.FileMetadata, []byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[path]
	if !ok {
		c.metrics.RecordMiss(ReasonMiss)
		return files.FileMetadata{}, nil, false
	}
	e := elem.Value.(*entry)
	if c.expired(e) {
		c.lru.Remove(elem)
		delete(c.items, path)
		c.metrics.RecordMiss(ReasonExpired)
		return files.FileMetadata{}, nil, false
	}
	c.lru.MoveToFront(elem)
	return e.meta, copyBytes(e.body), true
}

// store inserts an entry, evicting the LRU tail when capacity is
// exceeded. Called only after a successful source fetch.
func (c *Cache) store(path string, meta files.FileMetadata, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Replace an existing entry rather than double-insert.
	if elem, ok := c.items[path]; ok {
		c.lru.MoveToFront(elem)
		e := elem.Value.(*entry)
		e.meta = meta
		e.body = copyBytes(body)
		e.inserted = c.now()
		return
	}
	e := &entry{
		path:     path,
		meta:     meta,
		body:     copyBytes(body),
		inserted: c.now(),
	}
	elem := c.lru.PushFront(e)
	c.items[path] = elem
	c.evictIfFull()
}

// evictIfFull drops LRU-tail entries until the entry count is
// within Capacity. Called with c.mu held.
func (c *Cache) evictIfFull() {
	if c.cfg.Capacity == 0 {
		return
	}
	for len(c.items) > c.cfg.Capacity {
		tail := c.lru.Back()
		if tail == nil {
			return
		}
		e := tail.Value.(*entry)
		c.lru.Remove(tail)
		delete(c.items, e.path)
	}
}

// expired returns true if the entry's age exceeds the configured
// TTL. Zero TTL means entries never expire by age.
func (c *Cache) expired(e *entry) bool {
	if c.cfg.TTL == 0 {
		return false
	}
	return c.now().Sub(e.inserted) > c.cfg.TTL
}

// copyBytes returns a defensive copy so callers can mutate the
// returned slice without corrupting cached state, and the cache
// is insulated from caller mutations of the body slice handed in.
func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
