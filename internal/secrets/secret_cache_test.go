package secrets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newCacheForTest(t *testing.T) (*SecretCache, *testCacheClock) {
	t.Helper()
	clock := &testCacheClock{now: time.Unix(1_700_000_000, 0).UTC()}
	c, err := NewSecretCache(SecretCacheConfig{
		DefaultTTL:    5 * time.Minute,
		MaxEntries:    1000,
		SweepInterval: time.Hour, // sweep never fires in tests unless we Start
		Clock:         clock.Now,
	})
	if err != nil {
		t.Fatalf("NewSecretCache: %v", err)
	}
	return c, clock
}

// ---- Construction ------------------------------------------------

func TestNewSecretCache_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     SecretCacheConfig
		wantSub string
	}{
		{"negative TTL", SecretCacheConfig{DefaultTTL: -time.Second}, "DefaultTTL"},
		{"negative MaxEntries", SecretCacheConfig{MaxEntries: -1}, "MaxEntries"},
		{"negative SweepInterval", SecretCacheConfig{SweepInterval: -time.Second}, "SweepInterval"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewSecretCache(tc.cfg)
			if err == nil {
				t.Fatalf("NewSecretCache = nil err")
			}
			if !errors.Is(err, ErrInvalidBackend) {
				t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestNewSecretCache_Defaults(t *testing.T) {
	t.Parallel()
	c, err := NewSecretCache(SecretCacheConfig{})
	if err != nil {
		t.Fatalf("NewSecretCache empty: %v", err)
	}
	if c.cfg.DefaultTTL != DefaultCacheTTL {
		t.Errorf("DefaultTTL = %v, want %v", c.cfg.DefaultTTL, DefaultCacheTTL)
	}
	if c.cfg.MaxEntries != DefaultCacheMaxEntries {
		t.Errorf("MaxEntries = %d, want %d", c.cfg.MaxEntries, DefaultCacheMaxEntries)
	}
	if c.cfg.SweepInterval != DefaultCacheSweepInterval {
		t.Errorf("SweepInterval = %v, want %v", c.cfg.SweepInterval, DefaultCacheSweepInterval)
	}
}

// ---- Round-trip --------------------------------------------------

func TestSecretCache_PutGet_RoundTrip(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)

	now := time.Unix(1_700_000_000, 0).UTC()
	in := &Secret{
		Path:          "kv/app/db",
		Data:          map[string]any{"password": "hunter2", "user": "alice"},
		Metadata:      map[string]string{"owner": "platform"},
		Version:       3,
		LeaseID:       "lease-x",
		LeaseDuration: 30 * time.Minute,
		Renewable:     true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	c.Put("kv/app/db", in)
	got, ok := c.Get("kv/app/db")
	if !ok {
		t.Fatalf("Get(kv/app/db) miss after Put")
	}
	if got.Path != in.Path {
		t.Errorf("Path = %q, want %q", got.Path, in.Path)
	}
	if got.Data["password"] != in.Data["password"] {
		t.Errorf("password = %v, want %v", got.Data["password"], in.Data["password"])
	}
	if got.Metadata["owner"] != in.Metadata["owner"] {
		t.Errorf("metadata mismatch: %#v", got.Metadata)
	}
	if got.Version != in.Version {
		t.Errorf("Version = %d, want %d", got.Version, in.Version)
	}
	if got.LeaseID != in.LeaseID || got.LeaseDuration != in.LeaseDuration {
		t.Errorf("lease fields lost: %#v", got)
	}
	if !got.CreatedAt.Equal(in.CreatedAt) {
		t.Errorf("CreatedAt drift: %v vs %v", got.CreatedAt, in.CreatedAt)
	}
}

func TestSecretCache_NonceUniqueness(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	c.Put("kv/x", &Secret{Path: "kv/x", Data: map[string]any{"k": "v"}})
	first := c.items["kv/x"].Value.(*cacheEntry)
	nonce1 := make([]byte, len(first.nonce))
	copy(nonce1, first.nonce)
	ct1 := make([]byte, len(first.ciphertext))
	copy(ct1, first.ciphertext)

	c.Put("kv/x", &Secret{Path: "kv/x", Data: map[string]any{"k": "v"}})
	second := c.items["kv/x"].Value.(*cacheEntry)

	if bytes.Equal(nonce1, second.nonce) {
		t.Errorf("nonce repeated across re-puts of same plaintext (catastrophic for AES-GCM)")
	}
	if bytes.Equal(ct1, second.ciphertext) {
		t.Errorf("ciphertext repeated across re-puts of same plaintext")
	}
}

func TestSecretCache_GetReturnsFreshCopy(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	in := &Secret{Path: "kv/x", Data: map[string]any{"password": "hunter2"}}
	c.Put("kv/x", in)

	got1, _ := c.Get("kv/x")
	got1.Data["password"] = "MUTATED"

	got2, _ := c.Get("kv/x")
	if got2.Data["password"] != "hunter2" {
		t.Errorf("mutation leaked into cached entry: %v", got2.Data["password"])
	}
}

func TestSecretCache_GetMiss(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	if _, ok := c.Get("never"); ok {
		t.Errorf("Get returned hit on cold cache")
	}
	if s := c.Stats(); s.Misses != 1 {
		t.Errorf("Misses = %d, want 1", s.Misses)
	}
}

// ---- TTL expiry --------------------------------------------------

func TestSecretCache_TTLExpiry(t *testing.T) {
	t.Parallel()
	c, clock := newCacheForTest(t)
	c.Put("kv/exp", &Secret{Path: "kv/exp"})

	// Advance just past the TTL.
	clock.Advance(5*time.Minute + time.Second)

	if _, ok := c.Get("kv/exp"); ok {
		t.Errorf("Get after TTL still hits")
	}
	stats := c.Stats()
	if stats.Expired != 1 {
		t.Errorf("Expired counter = %d, want 1", stats.Expired)
	}
	if stats.Entries != 0 {
		t.Errorf("Entries = %d, want 0 (expired entry should be reaped on Get)", stats.Entries)
	}
}

func TestSecretCache_TTL_Boundary(t *testing.T) {
	t.Parallel()
	c, clock := newCacheForTest(t)
	c.Put("kv/x", &Secret{Path: "kv/x"})
	// Exactly at the TTL: still alive (we compare with now.After,
	// not now.AtOrAfter).
	clock.Advance(5 * time.Minute)
	if _, ok := c.Get("kv/x"); !ok {
		t.Errorf("Get exactly at TTL = miss, want hit")
	}
}

// ---- LRU eviction ------------------------------------------------

func TestSecretCache_LRUBumpOnGet(t *testing.T) {
	t.Parallel()
	c, _ := NewSecretCache(SecretCacheConfig{
		DefaultTTL:    time.Hour,
		MaxEntries:    3,
		SweepInterval: time.Hour,
	})
	c.Put("a", &Secret{Path: "a"})
	c.Put("b", &Secret{Path: "b"})
	c.Put("c", &Secret{Path: "c"})

	// Touch "a" so it's most-recently-used.
	_, _ = c.Get("a")

	// Add "d" → "b" (the tail) is evicted, not "a".
	c.Put("d", &Secret{Path: "d"})

	if _, ok := c.Get("a"); !ok {
		t.Errorf("LRU evicted recently-touched entry")
	}
	if _, ok := c.Get("b"); ok {
		t.Errorf("LRU did not evict the oldest entry (b)")
	}
	if s := c.Stats(); s.Evictions != 1 {
		t.Errorf("Evictions = %d, want 1", s.Evictions)
	}
}

func TestSecretCache_LRUMaxEntries(t *testing.T) {
	t.Parallel()
	c, _ := NewSecretCache(SecretCacheConfig{
		DefaultTTL:    time.Hour,
		MaxEntries:    3,
		SweepInterval: time.Hour,
	})
	for i := 0; i < 5; i++ {
		c.Put(fmt.Sprintf("k-%d", i), &Secret{Path: fmt.Sprintf("k-%d", i)})
	}
	if s := c.Stats(); s.Entries != 3 {
		t.Errorf("Entries = %d, want 3", s.Entries)
	}
	if s := c.Stats(); s.Evictions != 2 {
		t.Errorf("Evictions = %d, want 2 (5 inserts past 3-cap)", s.Evictions)
	}
}

func TestSecretCache_PutReplaceSamePath(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	c.Put("kv/x", &Secret{Path: "kv/x", Data: map[string]any{"v": 1}})
	c.Put("kv/x", &Secret{Path: "kv/x", Data: map[string]any{"v": 2}})

	if s := c.Stats(); s.Entries != 1 {
		t.Errorf("Entries = %d, want 1 (same path replaces, not duplicates)", s.Entries)
	}
	got, _ := c.Get("kv/x")
	// JSON unmarshal into map[string]any decodes numeric to float64.
	if v, _ := got.Data["v"].(float64); v != 2 {
		t.Errorf("Data[v] = %v, want 2", got.Data["v"])
	}
}

// ---- Invalidate --------------------------------------------------

func TestSecretCache_InvalidatePath(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	c.Put("kv/a", &Secret{Path: "kv/a"})
	c.Put("kv/b", &Secret{Path: "kv/b"})

	c.InvalidatePath("kv/a")

	if _, ok := c.Get("kv/a"); ok {
		t.Errorf("InvalidatePath did not drop kv/a")
	}
	if _, ok := c.Get("kv/b"); !ok {
		t.Errorf("InvalidatePath dropped unrelated kv/b")
	}
}

func TestSecretCache_InvalidatePath_Missing(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	c.InvalidatePath("never-existed") // must not panic
}

func TestSecretCache_InvalidatePrefix(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	c.Put("kv/app/db", &Secret{Path: "kv/app/db"})
	c.Put("kv/app/cache", &Secret{Path: "kv/app/cache"})
	c.Put("kv/web/api", &Secret{Path: "kv/web/api"})
	c.Put("other/x", &Secret{Path: "other/x"})

	c.InvalidatePrefix("kv/app/")

	for _, dropped := range []string{"kv/app/db", "kv/app/cache"} {
		if _, ok := c.Get(dropped); ok {
			t.Errorf("InvalidatePrefix did not drop %q", dropped)
		}
	}
	for _, kept := range []string{"kv/web/api", "other/x"} {
		if _, ok := c.Get(kept); !ok {
			t.Errorf("InvalidatePrefix dropped %q (not under prefix)", kept)
		}
	}
}

func TestSecretCache_InvalidatePrefix_Empty(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	c.Put("kv/x", &Secret{Path: "kv/x"})
	c.InvalidatePrefix("")
	if _, ok := c.Get("kv/x"); !ok {
		t.Errorf("InvalidatePrefix(\"\") wiped the cache; want no-op")
	}
}

// ---- Clear -------------------------------------------------------

func TestSecretCache_Clear(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	c.Put("a", &Secret{Path: "a"})
	c.Put("b", &Secret{Path: "b"})
	_, _ = c.Get("a")
	_, _ = c.Get("missing")

	c.Clear()

	stats := c.Stats()
	if stats.Entries != 0 {
		t.Errorf("Entries after Clear = %d, want 0", stats.Entries)
	}
	if stats.Hits != 0 || stats.Misses != 0 || stats.Evictions != 0 || stats.Expired != 0 {
		t.Errorf("counters not reset: %#v", stats)
	}
	if stats.MemoryBytes != 0 {
		t.Errorf("MemoryBytes after Clear = %d, want 0", stats.MemoryBytes)
	}
}

// ---- Stats -------------------------------------------------------

func TestSecretCache_Stats(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	c.Put("a", &Secret{Path: "a"})
	c.Put("b", &Secret{Path: "b"})

	_, _ = c.Get("a")
	_, _ = c.Get("a")
	_, _ = c.Get("never")

	stats := c.Stats()
	if stats.Hits != 2 {
		t.Errorf("Hits = %d, want 2", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", stats.Misses)
	}
	if stats.Entries != 2 {
		t.Errorf("Entries = %d, want 2", stats.Entries)
	}
	if stats.MemoryBytes <= 0 {
		t.Errorf("MemoryBytes = %d, want > 0", stats.MemoryBytes)
	}
}

// ---- Sweep + Lifecycle -------------------------------------------

func TestSecretCache_BackgroundSweep(t *testing.T) {
	t.Parallel()
	clock := &testCacheClock{now: time.Unix(1_700_000_000, 0).UTC()}
	c, err := NewSecretCache(SecretCacheConfig{
		DefaultTTL:    100 * time.Millisecond,
		MaxEntries:    100,
		SweepInterval: 20 * time.Millisecond,
		Clock:         clock.Now,
	})
	if err != nil {
		t.Fatalf("NewSecretCache: %v", err)
	}

	c.Put("kv/x", &Secret{Path: "kv/x"})

	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = c.Stop(stopCtx)
	}()

	// Advance the clock past the TTL — sweep should reap on its
	// next tick.
	clock.Advance(200 * time.Millisecond)

	// Wait for the sweep to fire.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s := c.Stats(); s.Entries == 0 && s.Expired >= 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("background sweep did not reap expired entry: stats=%+v", c.Stats())
}

func TestSecretCache_Lifecycle(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	ctx := context.Background()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := c.Start(ctx); err == nil {
		t.Errorf("double Start = nil err")
	}
	stopCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := c.Stop(stopCtx); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if err := c.Stop(stopCtx); err != nil {
		t.Errorf("double Stop: %v", err)
	}
	if err := c.Start(ctx); err == nil {
		t.Errorf("Start after Stop = nil err")
	}
}

func TestSecretCache_UsableWithoutStart(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	// No Start.
	c.Put("kv/x", &Secret{Path: "kv/x"})
	if _, ok := c.Get("kv/x"); !ok {
		t.Errorf("Get without Start failed")
	}
}

// ---- Concurrency -------------------------------------------------

func TestSecretCache_Concurrent(t *testing.T) {
	t.Parallel()
	c, _ := newCacheForTest(t)
	const n = 50
	var wg sync.WaitGroup
	var hits, misses int64

	for i := 0; i < n; i++ {
		wg.Add(3)
		i := i
		go func() {
			defer wg.Done()
			c.Put(fmt.Sprintf("k-%d", i), &Secret{Path: fmt.Sprintf("k-%d", i)})
		}()
		go func() {
			defer wg.Done()
			if _, ok := c.Get(fmt.Sprintf("k-%d", i)); ok {
				atomic.AddInt64(&hits, 1)
			} else {
				atomic.AddInt64(&misses, 1)
			}
		}()
		go func() {
			defer wg.Done()
			c.InvalidatePath(fmt.Sprintf("k-%d", (i+25)%n))
		}()
	}
	wg.Wait()
	// No specific assertion about hit/miss counts — they race by
	// design. We're proving the data structure stays consistent
	// under -race.
}

// ---- Broker integration ------------------------------------------

func TestSecretCache_BrokerIntegration_HitRateOnSecondAccess(t *testing.T) {
	t.Parallel()

	cache, _ := newCacheForTest(t)
	auditor := &cacheTestAuditor{}

	be := &cacheTestBackend{
		name: "test",
		caps: []BackendCapability{CapKV},
	}
	router, err := NewRouter([]Route{{Prefix: "kv/", Backend: "test"}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	broker, err := NewBroker(BrokerConfig{
		Router:         router,
		Backends:       []SecretBackend{be},
		DefaultBackend: "test",
		Cache:          cache,
		Auditor:        auditor,
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}

	ctx := context.Background()

	// First Get — cache miss, backend dispatched.
	if _, err := broker.GetSecret(ctx, GetSecretRequest{Path: "kv/app/db"}); err != nil {
		t.Fatalf("first GetSecret: %v", err)
	}
	if be.getCount != 1 {
		t.Fatalf("backend dispatch count after first Get = %d, want 1", be.getCount)
	}

	// Subsequent Gets — should all hit the cache.
	const n = 9
	for i := 0; i < n; i++ {
		if _, err := broker.GetSecret(ctx, GetSecretRequest{Path: "kv/app/db"}); err != nil {
			t.Fatalf("Get[%d]: %v", i, err)
		}
	}
	if be.getCount != 1 {
		t.Errorf("backend called on cache-hit path: count=%d, want 1 over %d calls", be.getCount, n+1)
	}

	stats := cache.Stats()
	totalAccess := stats.Hits + stats.Misses
	if totalAccess == 0 {
		t.Fatalf("Stats counters all zero")
	}
	hitRate := float64(stats.Hits) / float64(totalAccess)
	if hitRate < 0.8 {
		t.Errorf("hit rate = %.2f, want >0.8 per epic-10 acceptance criterion", hitRate)
	}
}

// ---- Helpers -----------------------------------------------------

type testCacheClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testCacheClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testCacheClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// cacheTestBackend is a minimal SecretBackend for the broker
// integration test — every Get returns a canned secret + records the
// call count.
type cacheTestBackend struct {
	name     string
	caps     []BackendCapability
	getCount int
}

func (b *cacheTestBackend) Name() string                      { return b.name }
func (b *cacheTestBackend) Capabilities() []BackendCapability { return b.caps }
func (b *cacheTestBackend) Start(context.Context) error       { return nil }
func (b *cacheTestBackend) Stop(context.Context) error        { return nil }
func (b *cacheTestBackend) Health(context.Context) error      { return nil }
func (b *cacheTestBackend) GetSecret(_ context.Context, req GetSecretRequest) (*Secret, error) {
	b.getCount++
	return &Secret{Path: req.Path, Data: map[string]any{"k": "v"}}, nil
}
func (b *cacheTestBackend) WriteSecret(_ context.Context, req WriteSecretRequest) (*Secret, error) {
	return &Secret{Path: req.Path, Data: req.Data}, nil
}
func (b *cacheTestBackend) ListSecrets(context.Context, ListSecretsRequest) (*ListSecretsResponse, error) {
	return &ListSecretsResponse{}, nil
}
func (b *cacheTestBackend) DeleteSecret(context.Context, DeleteSecretRequest) error {
	return nil
}
func (b *cacheTestBackend) IssueDynamicSecret(context.Context, IssueDynamicSecretRequest) (*Secret, error) {
	return nil, ErrNotImplementedYet
}
func (b *cacheTestBackend) RenewLease(context.Context, RenewLeaseRequest) (*LeaseInfo, error) {
	return nil, ErrNotImplementedYet
}
func (b *cacheTestBackend) RevokeLease(context.Context, RevokeLeaseRequest) error {
	return ErrNotImplementedYet
}

type cacheTestAuditor struct{}

func (cacheTestAuditor) Emit(context.Context, SecretAccessEvent) {}
