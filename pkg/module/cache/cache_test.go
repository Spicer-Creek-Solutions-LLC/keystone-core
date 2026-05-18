package cache_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/module/cache"
	"go.keystone-core.io/keystone-core/pkg/module/cas"
)

func newCache(t *testing.T, cfg cache.CacheConfig) *cache.Cache {
	t.Helper()
	if cfg.Dir == "" {
		cfg.Dir = t.TempDir()
	}
	c, err := cache.New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func put(t *testing.T, c *cache.Cache, s string) string {
	t.Helper()
	h, err := c.Put(strings.NewReader(s))
	if err != nil {
		t.Fatalf("Put(%q): %v", s, err)
	}
	return h
}

// backdate sets an entry's mtime so the cache sees it as old/LRU.
func backdate(t *testing.T, c *cache.Cache, hash string, age time.Duration) {
	t.Helper()
	p, err := c.Path(hash)
	if err != nil {
		t.Fatal(err)
	}
	tm := time.Now().Add(-age)
	if err := os.Chtimes(p, tm, tm); err != nil {
		t.Fatal(err)
	}
}

func TestPutGetRoundTrip(t *testing.T) {
	c := newCache(t, cache.CacheConfig{})
	h := put(t, c, "hello module")
	rc, err := c.Get(h)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "hello module" {
		t.Fatalf("Get = %q", got)
	}
	b, n, err := c.Size()
	if err != nil || n != 1 || b != int64(len("hello module")) {
		t.Fatalf("Size = %d,%d,%v", b, n, err)
	}
}

func TestMaxSizeEvictsLRUNotJustAdded(t *testing.T) {
	c := newCache(t, cache.CacheConfig{MaxSize: 20})
	a := put(t, c, strings.Repeat("a", 8))
	b := put(t, c, strings.Repeat("b", 8))
	// Make `a` the LRU (oldest).
	backdate(t, c, a, time.Hour)
	backdate(t, c, b, 30*time.Minute)
	// Adding a third 8-byte blob → total 24 > 20 → evict LRU (`a`),
	// never the just-added one.
	d := put(t, c, strings.Repeat("d", 8))
	if c.Has(a) {
		t.Fatal("LRU entry a should have been evicted")
	}
	if !c.Has(b) || !c.Has(d) {
		t.Fatalf("b/d wrongly evicted: b=%v d=%v", c.Has(b), c.Has(d))
	}
	if bytes, _, _ := c.Size(); bytes > 20 {
		t.Fatalf("size %d still over MaxSize 20", bytes)
	}
}

func TestGetTouchesMTimeForLRU(t *testing.T) {
	c := newCache(t, cache.CacheConfig{MaxSize: 20})
	a := put(t, c, strings.Repeat("a", 8))
	b := put(t, c, strings.Repeat("b", 8))
	backdate(t, c, a, time.Hour)
	backdate(t, c, b, 30*time.Minute)
	// Touch `a` via Get → it becomes MRU; now `b` is the LRU.
	rc, err := c.Get(a)
	if err != nil {
		t.Fatalf("Get(a): %v", err)
	}
	_ = rc.Close()
	put(t, c, strings.Repeat("d", 8)) // total 24 > 20 → evict LRU (now b)
	if c.Has(b) {
		t.Fatal("b should be evicted (it became LRU after a was touched)")
	}
	if !c.Has(a) {
		t.Fatal("a should survive (touched via Get)")
	}
}

func TestMaxAgeStaleIsMissAndEvicted(t *testing.T) {
	c := newCache(t, cache.CacheConfig{MaxAge: time.Minute})
	h := put(t, c, "perishable")
	backdate(t, c, h, 2*time.Minute) // older than MaxAge
	if _, err := c.Get(h); !errors.Is(err, cas.ErrNotFound) {
		t.Fatalf("stale Get = %v, want cas.ErrNotFound", err)
	}
	if c.Has(h) {
		t.Fatal("stale entry must be evicted on access")
	}
}

// Prune's distinct role is age-based eviction (MaxSize is kept
// continuously by enforce-on-Put — covered by the LRU test). Use
// MaxSize:0 so put-time eviction doesn't interfere with seeding.
func TestPruneDropsStaleEntries(t *testing.T) {
	c := newCache(t, cache.CacheConfig{MaxAge: time.Minute})
	old := put(t, c, strings.Repeat("o", 8))
	mid := put(t, c, strings.Repeat("m", 8))
	fresh := put(t, c, strings.Repeat("f", 8))
	backdate(t, c, old, 5*time.Minute)   // stale by age
	backdate(t, c, mid, 2*time.Second)   // fresh
	backdate(t, c, fresh, 1*time.Second) // fresh
	n, err := c.Prune()
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if n != 1 || c.Has(old) {
		t.Fatalf("Prune evicted=%d, old still present=%v (want only old gone)", n, c.Has(old))
	}
	if !c.Has(mid) || !c.Has(fresh) {
		t.Fatalf("fresh entries wrongly pruned: mid=%v fresh=%v", c.Has(mid), c.Has(fresh))
	}
}

func TestReadonly(t *testing.T) {
	dir := t.TempDir()
	rw := newCache(t, cache.CacheConfig{Dir: dir})
	h := put(t, rw, "seeded content")

	ro := newCache(t, cache.CacheConfig{Dir: dir, Readonly: true, MaxAge: time.Nanosecond})
	if _, err := ro.Put(strings.NewReader("x")); !errors.Is(err, cache.ErrReadOnly) {
		t.Fatalf("ro.Put = %v, want ErrReadOnly", err)
	}
	if err := ro.Delete(h); !errors.Is(err, cache.ErrReadOnly) {
		t.Fatalf("ro.Delete = %v, want ErrReadOnly", err)
	}
	if n, err := ro.Prune(); n != 0 || err != nil {
		t.Fatalf("ro.Prune = %d,%v, want 0,nil", n, err)
	}
	// Even though MaxAge is tiny, a read-only cache serves (can't evict).
	backdate(t, rw, h, time.Hour)
	rc, err := ro.Get(h)
	if err != nil {
		t.Fatalf("ro.Get stale = %v, want served", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "seeded content" {
		t.Fatalf("ro.Get content = %q", got)
	}
}

func TestPutExpectedAndOversizeSingleBlobKept(t *testing.T) {
	c := newCache(t, cache.CacheConfig{MaxSize: 4})
	big := strings.Repeat("z", 100) // single blob > MaxSize
	h := cas.HashBytes([]byte(big))
	got, err := c.PutExpected(bytes.NewReader([]byte(big)), h)
	if err != nil || got != h {
		t.Fatalf("PutExpected = %q,%v", got, err)
	}
	if !c.Has(h) {
		t.Fatal("an oversize single blob must still be stored (CAS correctness wins)")
	}
	if _, err := c.PutExpected(strings.NewReader("nope"), h); !errors.Is(err, cas.ErrHashMismatch) {
		t.Fatalf("PutExpected mismatch = %v, want cas.ErrHashMismatch", err)
	}
}

func TestValidateAndDefaults(t *testing.T) {
	if _, err := cache.New(cache.CacheConfig{MaxSize: -1}); !errors.Is(err, cache.ErrInvalidConfig) {
		t.Fatalf("MaxSize<0 = %v, want ErrInvalidConfig", err)
	}
	if _, err := cache.New(cache.CacheConfig{MaxAge: -time.Second}); !errors.Is(err, cache.ErrInvalidConfig) {
		t.Fatalf("MaxAge<0 = %v, want ErrInvalidConfig", err)
	}
	// MaxSize/MaxAge zero ⇒ unbounded (no eviction).
	c := newCache(t, cache.CacheConfig{})
	for i := 0; i < 5; i++ {
		put(t, c, strings.Repeat("x", 50))
	}
	if _, n, _ := c.Size(); n != 1 { // identical content → 1 entry (CAS dedup)
		t.Fatalf("dedup: count = %d, want 1", n)
	}
}

func TestPassThroughs(t *testing.T) {
	c := newCache(t, cache.CacheConfig{})
	h := put(t, c, "verify me")
	if err := c.Verify(h); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if p, err := c.Path(h); err != nil || p == "" {
		t.Fatalf("Path = %q,%v", p, err)
	}
	if err := c.Delete(h); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if c.Has(h) {
		t.Fatal("Has after Delete")
	}
	if _, err := c.Get(h); !errors.Is(err, cas.ErrNotFound) {
		t.Fatalf("Get after Delete = %v, want cas.ErrNotFound", err)
	}
}
