// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"container/list"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// SecretCache defaults — the §4.11 reference numbers.
const (
	// DefaultCacheTTL is the per-entry expiry when [Put] doesn't
	// specify a custom one. 5 minutes balances hit rate against
	// staleness for typical secret-access patterns.
	DefaultCacheTTL = 5 * time.Minute

	// DefaultCacheMaxEntries is the bounded-LRU cap. 10000 is the
	// §4.11 config example; covers trial-scale deployments without
	// runaway memory.
	DefaultCacheMaxEntries = 10000

	// DefaultCacheSweepInterval is the cadence the background
	// reaper walks the LRU dropping expired entries. Without a
	// sweep, expired-but-never-re-accessed entries stay resident
	// until LRU eviction; 30s reclaims them promptly.
	DefaultCacheSweepInterval = 30 * time.Second
)

// SecretCacheConfig drives [NewSecretCache].
type SecretCacheConfig struct {
	// DefaultTTL is the expiry stamped on every [Put]. Must be > 0.
	// 0 → [DefaultCacheTTL].
	DefaultTTL time.Duration

	// MaxEntries caps the live entry count. Must be > 0. 0 →
	// [DefaultCacheMaxEntries]. Past the cap, [Put] evicts the
	// least-recently-used entry.
	MaxEntries int

	// SweepInterval is the cadence the background reaper runs.
	// Must be > 0. 0 → [DefaultCacheSweepInterval].
	SweepInterval time.Duration

	// Clock injects testable now-time. nil → time.Now().UTC().
	Clock func() time.Time

	// Logger drives the sweep / lifecycle log lines. nil →
	// slog.Default.
	Logger *slog.Logger
}

// cacheEntry is one row in the LRU. Stored in [list.Element.Value].
type cacheEntry struct {
	path       string
	nonce      []byte
	ciphertext []byte
	expiresAt  time.Time
}

// approxBytes is a rough memory-footprint estimate for the entry.
// Counts the path string + nonce + ciphertext + a fixed per-entry
// overhead (list element header + map bucket slot). Not precise —
// Go's runtime overhead is not directly reportable.
func (e *cacheEntry) approxBytes() int64 {
	const perEntryOverhead = 96 // list node header + map slot + struct fields
	return int64(len(e.path)+len(e.nonce)+len(e.ciphertext)) + perEntryOverhead
}

// SecretCache is the in-memory AES-GCM-encrypted-at-rest [Cache] per
// PROJECT-DETAILS §4.11. Bounded LRU + TTL eviction + prefix-delete
// + a background sweep reaper.
//
// Concurrency: a single [sync.Mutex] guards the map + list + counters.
// Cache ops are fast enough that lock contention isn't a v0.x concern
// at trial scale.
//
// The at-rest encryption key is generated via [crypto/rand] at
// construction and never leaves the cache instance. Defense-in-depth
// against process-memory inspection (debugger / core dump / container
// snapshot); not a confidentiality boundary against in-process callers.
// Master-key rotation on the upstream secrets backend is operator-
// driven via [SecretCache.Clear] — see the gate-v1.0 ROADMAP entry
// "Encrypted-file backend master-key rotation tooling" for the
// dual-key-window follow-up.
type SecretCache struct {
	cfg    SecretCacheConfig
	aead   cipher.AEAD
	clock  func() time.Time
	logger *slog.Logger

	mu    sync.Mutex
	items map[string]*list.Element
	lru   *list.List
	hits  uint64
	miss  uint64
	evict uint64
	expr  uint64
	bytes int64

	startMu sync.Mutex
	started bool
	stopped bool
	loopCtx context.Context
	cancel  context.CancelFunc
	doneCh  chan struct{}
}

// NewSecretCache validates the config and returns a ready-to-use
// cache. The cache is usable immediately; [SecretCache.Start] only
// spawns the background sweep goroutine.
func NewSecretCache(cfg SecretCacheConfig) (*SecretCache, error) {
	if cfg.DefaultTTL < 0 {
		return nil, fmt.Errorf("%w: SecretCache: DefaultTTL must be non-negative", ErrInvalidBackend)
	}
	if cfg.MaxEntries < 0 {
		return nil, fmt.Errorf("%w: SecretCache: MaxEntries must be non-negative", ErrInvalidBackend)
	}
	if cfg.SweepInterval < 0 {
		return nil, fmt.Errorf("%w: SecretCache: SweepInterval must be non-negative", ErrInvalidBackend)
	}
	if cfg.DefaultTTL == 0 {
		cfg.DefaultTTL = DefaultCacheTTL
	}
	if cfg.MaxEntries == 0 {
		cfg.MaxEntries = DefaultCacheMaxEntries
	}
	if cfg.SweepInterval == 0 {
		cfg.SweepInterval = DefaultCacheSweepInterval
	}
	clock := cfg.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var keyBytes [32]byte
	if _, err := io.ReadFull(rand.Reader, keyBytes[:]); err != nil {
		return nil, fmt.Errorf("%w: SecretCache: generate at-rest key: %v", ErrInvalidBackend, err)
	}
	block, err := aes.NewCipher(keyBytes[:])
	if err != nil {
		return nil, fmt.Errorf("%w: SecretCache: aes cipher: %v", ErrInvalidBackend, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: SecretCache: aes-gcm: %v", ErrInvalidBackend, err)
	}

	return &SecretCache{
		cfg:    cfg,
		aead:   aead,
		clock:  clock,
		logger: logger,
		items:  make(map[string]*list.Element, cfg.MaxEntries),
		lru:    list.New(),
	}, nil
}

// Get returns the cached secret for path. On miss (or expiry), the
// returned bool is false. Hit + non-expired path: bumps recency.
func (c *SecretCache) Get(path string) (*Secret, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[path]
	if !ok {
		c.miss++
		return nil, false
	}
	entry := elem.Value.(*cacheEntry)
	now := c.clock()
	if now.After(entry.expiresAt) {
		// Expired: drop + count as a miss + expired tick.
		c.removeElementLocked(elem)
		c.miss++
		c.expr++
		return nil, false
	}

	c.lru.MoveToFront(elem)
	secret, err := c.decryptLocked(entry.nonce, entry.ciphertext)
	if err != nil {
		// At-rest decryption failure is a serious internal bug —
		// drop the entry + log + return miss so the caller retries
		// the backend. Don't bubble to the broker as an error
		// because [Cache.Get] is a (value, bool) contract.
		c.logger.LogAttrs(context.Background(), slog.LevelError,
			"secret cache: at-rest decryption failed",
			slog.String("path", path),
			slog.String("err", err.Error()),
		)
		c.removeElementLocked(elem)
		c.miss++
		return nil, false
	}
	c.hits++
	return secret, true
}

// Put inserts or replaces the entry for path. The TTL stamped is
// [SecretCacheConfig.DefaultTTL]. If [MaxEntries] is reached, the
// least-recently-used entry is evicted.
func (c *SecretCache) Put(path string, secret *Secret) {
	if secret == nil || path == "" {
		return
	}
	nonce, ciphertext, err := c.encrypt(secret)
	if err != nil {
		c.logger.LogAttrs(context.Background(), slog.LevelError,
			"secret cache: at-rest encryption failed (skipping put)",
			slog.String("path", path),
			slog.String("err", err.Error()),
		)
		return
	}
	entry := &cacheEntry{
		path:       path,
		nonce:      nonce,
		ciphertext: ciphertext,
		expiresAt:  c.clock().Add(c.cfg.DefaultTTL),
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[path]; ok {
		c.bytes -= elem.Value.(*cacheEntry).approxBytes()
		elem.Value = entry
		c.lru.MoveToFront(elem)
		c.bytes += entry.approxBytes()
		return
	}
	elem := c.lru.PushFront(entry)
	c.items[path] = elem
	c.bytes += entry.approxBytes()
	for c.lru.Len() > c.cfg.MaxEntries {
		tail := c.lru.Back()
		if tail == nil {
			break
		}
		c.removeElementLocked(tail)
		c.evict++
	}
}

// InvalidatePath drops a single entry (if present).
func (c *SecretCache) InvalidatePath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[path]; ok {
		c.removeElementLocked(elem)
	}
}

// InvalidatePrefix drops every entry whose path starts with prefix.
// Used on bulk operations (KV-v2 destroy on a directory; lease
// revocation when the credential covers many paths).
func (c *SecretCache) InvalidatePrefix(prefix string) {
	if prefix == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// Walk a snapshot of keys so removal-during-iteration is safe.
	matching := make([]*list.Element, 0)
	for path, elem := range c.items {
		if strings.HasPrefix(path, prefix) {
			matching = append(matching, elem)
		}
	}
	for _, elem := range matching {
		c.removeElementLocked(elem)
	}
}

// Stats returns a snapshot of cache counters.
func (c *SecretCache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{
		Hits:        c.hits,
		Misses:      c.miss,
		Evictions:   c.evict,
		Expired:     c.expr,
		Entries:     c.lru.Len(),
		MemoryBytes: c.bytes,
	}
}

// Clear drops every entry and resets counters. Operator-facing
// (the future `kscore-secrets cache clear` CLI subcommand). Useful
// after a secrets-system master-key rotation to discard now-stale
// at-rest plaintexts referring to keys that no longer apply.
func (c *SecretCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element, c.cfg.MaxEntries)
	c.lru.Init()
	c.hits = 0
	c.miss = 0
	c.evict = 0
	c.expr = 0
	c.bytes = 0
}

// Start spawns the background sweep goroutine. One-shot — double
// Start rejects with [ErrInvalidBackend]. The cache is usable
// without Start; Start only enables the background expired-entry
// reaper.
func (c *SecretCache) Start(ctx context.Context) error {
	c.startMu.Lock()
	defer c.startMu.Unlock()
	if c.stopped {
		return fmt.Errorf("%w: SecretCache: cannot Start after Stop", ErrInvalidBackend)
	}
	if c.started {
		return fmt.Errorf("%w: SecretCache: already started", ErrInvalidBackend)
	}
	c.started = true
	loopCtx, cancel := context.WithCancel(ctx)
	c.loopCtx = loopCtx
	c.cancel = cancel
	c.doneCh = make(chan struct{})
	go c.runSweep(loopCtx)
	return nil
}

// Stop cancels the sweep goroutine. Idempotent.
func (c *SecretCache) Stop(ctx context.Context) error {
	c.startMu.Lock()
	if c.stopped || !c.started {
		c.stopped = true
		c.startMu.Unlock()
		return nil
	}
	c.stopped = true
	cancel := c.cancel
	doneCh := c.doneCh
	c.startMu.Unlock()

	cancel()
	select {
	case <-doneCh:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: SecretCache: stop deadline: %v", ErrInvalidBackend, ctx.Err())
	}
}

// runSweep is the background reaper loop. Walks the LRU on each
// tick and drops every entry whose expiry has passed.
func (c *SecretCache) runSweep(ctx context.Context) {
	defer close(c.doneCh)
	ticker := time.NewTicker(c.cfg.SweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.sweepOnce()
		}
	}
}

// sweepOnce reaps every expired entry. Holds the lock for the full
// pass — at trial scale (≤ 10k entries) the walk is bounded.
func (c *SecretCache) sweepOnce() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	// Walk a snapshot so we can remove while iterating.
	expired := make([]*list.Element, 0)
	for _, elem := range c.items {
		if now.After(elem.Value.(*cacheEntry).expiresAt) {
			expired = append(expired, elem)
		}
	}
	for _, elem := range expired {
		c.removeElementLocked(elem)
		c.expr++
	}
}

// removeElementLocked drops elem from both the map and the list,
// decrements the byte counter. Caller MUST hold the mutex.
func (c *SecretCache) removeElementLocked(elem *list.Element) {
	entry := elem.Value.(*cacheEntry)
	delete(c.items, entry.path)
	c.lru.Remove(elem)
	c.bytes -= entry.approxBytes()
	if c.bytes < 0 {
		c.bytes = 0
	}
}

// encrypt serialises secret to JSON, encrypts with a fresh nonce,
// returns (nonce, ciphertext). Lock-free — callers can encrypt
// before taking the cache mutex to keep the critical section tiny.
func (c *SecretCache) encrypt(secret *Secret) ([]byte, []byte, error) {
	plain, err := json.Marshal(secret)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal: %w", err)
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("nonce: %w", err)
	}
	ciphertext := c.aead.Seal(nil, nonce, plain, nil)
	return nonce, ciphertext, nil
}

// decryptLocked recovers a *Secret from stored (nonce, ciphertext).
// Caller MUST hold the mutex (decryption is racy against `aead` only
// in pathological scenarios — the AEAD is safe for concurrent use,
// but pairing it with map/list lookups means we just keep the
// critical section unified).
func (c *SecretCache) decryptLocked(nonce, ciphertext []byte) (*Secret, error) {
	plain, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("aead: %w", err)
	}
	var secret Secret
	if err := json.Unmarshal(plain, &secret); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &secret, nil
}

// Compile-time interface assertion.
var _ Cache = (*SecretCache)(nil)
