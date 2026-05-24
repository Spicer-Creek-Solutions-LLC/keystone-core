// SPDX-License-Identifier: Apache-2.0

// Package cache adds the §4.18 size/age/readonly lifecycle policy
// on top of the task-5 content-addressed store (Epic 14 task 7).
//
// cas.Store is the raw CAS primitive; Cache wraps it and enforces
// CacheConfig: a MaxSize byte cap (LRU eviction), a MaxAge staleness
// bound, and an optional Readonly (immutable shared cache) mode.
//
// Recency is tracked by mtime — atime is unreliable under `noatime`
// mounts; Get explicitly touches mtime so it doubles as the
// last-use signal (LRU for MaxSize, "time since last use" for
// MaxAge). The loader's load-time caching wiring is task 10.
//
// Pure standard library; no new dependency.
package cache

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"go.keystone-core.io/keystone-core/pkg/module/cas"
)

var (
	// ErrReadOnly — a mutating op was attempted on a Readonly cache.
	ErrReadOnly = errors.New("cache: read-only")
	// ErrInvalidConfig — CacheConfig failed validation.
	ErrInvalidConfig = errors.New("cache: invalid config")
)

// CacheConfig configures the module cache (PROJECT-DETAILS §4.18).
// A zero MaxSize or MaxAge disables that limit (the
// internal/config retention convention).
type CacheConfig struct {
	Dir      string        // cache root; "" → cas.DefaultRoot()
	MaxSize  int64         // max total bytes; 0 = unbounded
	MaxAge   time.Duration // evict entries unused longer than this; 0 = no age limit
	Readonly bool          // immutable shared cache (no Put/Delete/evict)
}

func (c CacheConfig) validate() error {
	if c.MaxSize < 0 {
		return fmt.Errorf("%w: MaxSize must be >= 0", ErrInvalidConfig)
	}
	if c.MaxAge < 0 {
		return fmt.Errorf("%w: MaxAge must be >= 0", ErrInvalidConfig)
	}
	return nil
}

// Cache is a cas.Store with the CacheConfig policy applied.
type Cache struct {
	cfg   CacheConfig
	store *cas.Store
	now   func() time.Time // internal clock seam (tests drive recency via file mtimes)
}

// New validates cfg and opens the backing store.
func New(cfg CacheConfig) (*Cache, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	st, err := cas.New(cfg.Dir)
	if err != nil {
		return nil, err
	}
	return &Cache{cfg: cfg, store: st, now: time.Now}, nil
}

// Put stores r and then enforces MaxSize. ErrReadOnly on a
// read-only cache.
func (c *Cache) Put(r io.Reader) (string, error) {
	if c.cfg.Readonly {
		return "", ErrReadOnly
	}
	h, err := c.store.Put(r)
	if err != nil {
		return "", err
	}
	c.enforce(h)
	return h, nil
}

// PutExpected is Put with the cas integrity gate.
func (c *Cache) PutExpected(r io.Reader, want string) (string, error) {
	if c.cfg.Readonly {
		return "", ErrReadOnly
	}
	h, err := c.store.PutExpected(r, want)
	if err != nil {
		return "", err
	}
	c.enforce(h)
	return h, nil
}

// Get returns the content for hash. A MaxAge-stale entry is a miss
// (and evicted, unless read-only): cas.ErrNotFound. A served entry
// has its mtime touched so it stays MRU.
func (c *Cache) Get(hash string) (io.ReadCloser, error) {
	p, err := c.store.Path(hash)
	if err != nil {
		return nil, err
	}
	fi, statErr := os.Stat(p)
	if statErr == nil && c.stale(fi.ModTime()) {
		if !c.cfg.Readonly {
			_ = c.store.Delete(hash)
			return nil, fmt.Errorf("%w: %s (stale)", cas.ErrNotFound, hash)
		}
		// Read-only: can't evict; serve it anyway.
	}
	rc, err := c.store.Get(hash)
	if err != nil {
		return nil, err
	}
	if !c.cfg.Readonly {
		t := c.now()
		_ = os.Chtimes(p, t, t) // best-effort LRU touch
	}
	return rc, nil
}

// Has reports presence (ignores staleness).
func (c *Cache) Has(hash string) bool { return c.store.Has(hash) }

// Verify re-hashes stored content (corruption check).
func (c *Cache) Verify(hash string) error { return c.store.Verify(hash) }

// Path returns the on-disk path for hash.
func (c *Cache) Path(hash string) (string, error) { return c.store.Path(hash) }

// Delete removes an entry. ErrReadOnly on a read-only cache.
func (c *Cache) Delete(hash string) error {
	if c.cfg.Readonly {
		return ErrReadOnly
	}
	return c.store.Delete(hash)
}

// Size reports the current total bytes + entry count.
func (c *Cache) Size() (bytes int64, count int, err error) {
	es, err := c.store.Entries()
	if err != nil {
		return 0, 0, err
	}
	for _, e := range es {
		bytes += e.Size
	}
	return bytes, len(es), nil
}

// Prune drops MaxAge-stale entries then LRU-evicts until MaxSize is
// satisfied. No-op (0, nil) on a read-only cache.
func (c *Cache) Prune() (evicted int, err error) {
	if c.cfg.Readonly {
		return 0, nil
	}
	return c.sweep("")
}

func (c *Cache) stale(mod time.Time) bool {
	return c.cfg.MaxAge > 0 && c.now().Sub(mod) > c.cfg.MaxAge
}

// enforce keeps the cache within MaxSize after a Put, never
// evicting keep (the just-stored hash). Best-effort: a sweep error
// must not fail the Put.
func (c *Cache) enforce(keep string) {
	_, _ = c.sweep(keep)
}

// sweep evicts every stale entry, and — oldest-first — enough more
// to bring the total under MaxSize. keep (the just-stored hash) is
// never evicted.
func (c *Cache) sweep(keep string) (int, error) {
	es, err := c.store.Entries()
	if err != nil {
		return 0, err
	}
	sort.Slice(es, func(i, j int) bool { return es[i].ModTime.Before(es[j].ModTime) }) // LRU first

	var total int64
	for _, e := range es {
		total += e.Size
	}
	evicted := 0
	for _, e := range es {
		if e.Hash == keep {
			continue
		}
		over := c.cfg.MaxSize > 0 && total > c.cfg.MaxSize
		if !c.stale(e.ModTime) && !over {
			continue // keep: fresh and we're within the size cap
		}
		if err := c.store.Delete(e.Hash); err != nil {
			continue
		}
		total -= e.Size
		evicted++
	}
	return evicted, nil
}
