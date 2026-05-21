package proxy

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/internal/files/transport"
)

// --- stub Getter -------------------------------------------------------------

type stubGetter struct {
	mu       sync.Mutex
	calls    int32
	results  map[string]stubResult
	getErr   error
	captured []capturedCall
}

type stubResult struct {
	meta files.FileMetadata
	body []byte
}

type capturedCall struct {
	path string
	opts transport.GetOptions
}

func (s *stubGetter) Get(_ context.Context, path string, opts transport.GetOptions) (files.FileMetadata, []byte, error) {
	atomic.AddInt32(&s.calls, 1)
	s.mu.Lock()
	s.captured = append(s.captured, capturedCall{path: path, opts: opts})
	s.mu.Unlock()
	if s.getErr != nil {
		return files.FileMetadata{}, nil, s.getErr
	}
	r, ok := s.results[path]
	if !ok {
		return files.FileMetadata{}, nil, errors.New("not found")
	}
	// Hand back a copy so a caller mutating the body doesn't
	// bleed into subsequent stub invocations.
	out := make([]byte, len(r.body))
	copy(out, r.body)
	return r.meta, out, nil
}

func newStub() *stubGetter {
	return &stubGetter{results: make(map[string]stubResult)}
}

func (s *stubGetter) callCount() int32 { return atomic.LoadInt32(&s.calls) }

// --- tests -------------------------------------------------------------------

func TestNew_Validation(t *testing.T) {
	cases := []struct {
		name string
		src  Getter
		cfg  Config
	}{
		{"nil source", nil, Config{Capacity: 10}},
		{"negative capacity", newStub(), Config{Capacity: -1}},
		{"negative ttl", newStub(), Config{Capacity: 1, TTL: -time.Second}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.src, tc.cfg, nil, nil); err == nil {
				t.Fatal("want error")
			}
		})
	}
}

func TestCache_HitOnSecondGet(t *testing.T) {
	src := newStub()
	src.results["p"] = stubResult{
		meta: files.FileMetadata{Path: "p", Size: 5},
		body: []byte("hello"),
	}
	c, err := New(src, Config{Capacity: 10}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		meta, body, err := c.Get(context.Background(), "p", transport.GetOptions{})
		if err != nil {
			t.Fatalf("Get #%d: %v", i, err)
		}
		if meta.Path != "p" || !bytes.Equal(body, []byte("hello")) {
			t.Errorf("Get #%d returned %+v / %q", i, meta, body)
		}
	}
	if got := src.callCount(); got != 1 {
		t.Errorf("source calls = %d, want 1 (subsequent reads should hit cache)", got)
	}
}

func TestCache_MissOnUnknownPath(t *testing.T) {
	src := newStub()
	c, _ := New(src, Config{Capacity: 10}, nil, nil)
	_, _, err := c.Get(context.Background(), "missing", transport.GetOptions{})
	if err == nil {
		t.Fatal("want error from source")
	}
	if src.callCount() != 1 {
		t.Errorf("source called %d times, want 1", src.callCount())
	}
}

func TestCache_LRU_EvictsOldest(t *testing.T) {
	src := newStub()
	for _, p := range []string{"a", "b", "c", "d"} {
		src.results[p] = stubResult{
			meta: files.FileMetadata{Path: p},
			body: []byte(p),
		}
	}
	c, _ := New(src, Config{Capacity: 2}, nil, nil)
	ctx := context.Background()

	mustGet(t, c, ctx, "a")
	mustGet(t, c, ctx, "b") // [b, a]
	mustGet(t, c, ctx, "c") // [c, b], a evicted
	if c.Len() != 2 {
		t.Errorf("len = %d, want 2", c.Len())
	}

	// "a" should miss now (was evicted)
	src.calls = 0
	mustGet(t, c, ctx, "a") // [a, c], b evicted
	if src.callCount() != 1 {
		t.Errorf("a re-fetch source calls = %d, want 1", src.callCount())
	}
	// "b" was evicted too
	src.calls = 0
	mustGet(t, c, ctx, "b")
	if src.callCount() != 1 {
		t.Errorf("b re-fetch source calls = %d, want 1", src.callCount())
	}
}

func TestCache_LRU_MoveToFrontOnHit(t *testing.T) {
	src := newStub()
	for _, p := range []string{"a", "b", "c"} {
		src.results[p] = stubResult{meta: files.FileMetadata{Path: p}, body: []byte(p)}
	}
	c, _ := New(src, Config{Capacity: 2}, nil, nil)
	ctx := context.Background()

	mustGet(t, c, ctx, "a") // [a]
	mustGet(t, c, ctx, "b") // [b, a]
	mustGet(t, c, ctx, "a") // [a, b] — a is MRU
	mustGet(t, c, ctx, "c") // [c, a] — b should be evicted (LRU)

	// "b" should miss (LRU-evicted), "a" should hit (was MRU)
	src.calls = 0
	mustGet(t, c, ctx, "a")
	if src.callCount() != 0 {
		t.Errorf("a should still be cached, got %d source calls", src.callCount())
	}
	src.calls = 0
	mustGet(t, c, ctx, "b")
	if src.callCount() != 1 {
		t.Errorf("b should have been evicted, got %d source calls", src.callCount())
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	src := newStub()
	src.results["p"] = stubResult{meta: files.FileMetadata{Path: "p"}, body: []byte("v")}

	var now time.Time
	clock := func() time.Time { return now }
	c, _ := New(src, Config{Capacity: 10, TTL: 100 * time.Millisecond}, nil, clock)
	ctx := context.Background()

	now = time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	mustGet(t, c, ctx, "p")
	if src.callCount() != 1 {
		t.Fatalf("first Get source calls = %d", src.callCount())
	}

	// Within TTL window — cache hit.
	now = now.Add(50 * time.Millisecond)
	mustGet(t, c, ctx, "p")
	if src.callCount() != 1 {
		t.Errorf("within TTL source calls = %d, want 1", src.callCount())
	}

	// Past TTL — should refetch.
	now = now.Add(200 * time.Millisecond)
	mustGet(t, c, ctx, "p")
	if src.callCount() != 2 {
		t.Errorf("past TTL source calls = %d, want 2", src.callCount())
	}
}

func TestCache_TTLZero_NeverExpires(t *testing.T) {
	src := newStub()
	src.results["p"] = stubResult{meta: files.FileMetadata{Path: "p"}, body: []byte("v")}
	c, _ := New(src, Config{Capacity: 10, TTL: 0}, nil, nil)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		mustGet(t, c, ctx, "p")
	}
	if src.callCount() != 1 {
		t.Errorf("source calls = %d, want 1 (TTL=0 means no expiry)", src.callCount())
	}
}

func TestCache_FromChunkBypass(t *testing.T) {
	src := newStub()
	src.results["p"] = stubResult{meta: files.FileMetadata{Path: "p"}, body: []byte("v")}
	c, _ := New(src, Config{Capacity: 10}, nil, nil)
	ctx := context.Background()

	// First Get with FromChunk=1 should bypass cache.
	_, _, _ = c.Get(ctx, "p", transport.GetOptions{FromChunk: 1})
	if c.Len() != 0 {
		t.Errorf("bypass should not populate cache, got len=%d", c.Len())
	}
	if src.callCount() != 1 {
		t.Errorf("source calls = %d", src.callCount())
	}

	// Second Get with FromChunk=1 also bypasses — calls source again.
	_, _, _ = c.Get(ctx, "p", transport.GetOptions{FromChunk: 1})
	if src.callCount() != 2 {
		t.Errorf("second bypass source calls = %d, want 2", src.callCount())
	}

	// Without FromChunk, normal cache populate happens.
	mustGet(t, c, ctx, "p")
	if c.Len() != 1 {
		t.Errorf("len = %d, want 1", c.Len())
	}
}

func TestCache_Invalidate(t *testing.T) {
	src := newStub()
	src.results["p"] = stubResult{meta: files.FileMetadata{Path: "p"}, body: []byte("v1")}
	c, _ := New(src, Config{Capacity: 10}, nil, nil)
	ctx := context.Background()

	mustGet(t, c, ctx, "p")
	if !c.Invalidate("p") {
		t.Error("Invalidate should return true for existing entry")
	}
	if c.Invalidate("p") {
		t.Error("Invalidate should return false on second call (already gone)")
	}
	if c.Invalidate("never-there") {
		t.Error("Invalidate should return false for unknown path")
	}

	// After invalidate, source is called again.
	src.calls = 0
	mustGet(t, c, ctx, "p")
	if src.callCount() != 1 {
		t.Errorf("after invalidate, source calls = %d, want 1", src.callCount())
	}
}

func TestCache_BodyIsolation(t *testing.T) {
	// Mutating the returned body must not corrupt cached state.
	src := newStub()
	src.results["p"] = stubResult{meta: files.FileMetadata{Path: "p"}, body: []byte("abc")}
	c, _ := New(src, Config{Capacity: 10}, nil, nil)
	ctx := context.Background()

	_, body1, _ := c.Get(ctx, "p", transport.GetOptions{})
	body1[0] = 'Z'

	_, body2, _ := c.Get(ctx, "p", transport.GetOptions{})
	if string(body2) != "abc" {
		t.Errorf("post-mutation body = %q, want abc", body2)
	}
}

func TestCache_ReplaceOnRePut(t *testing.T) {
	// If the source returns updated metadata on a subsequent miss
	// after a stale entry was evicted, the cache stores the new
	// version. We exercise this by invalidating + changing the
	// stub's result.
	src := newStub()
	src.results["p"] = stubResult{meta: files.FileMetadata{Path: "p", Version: 1}, body: []byte("v1")}
	c, _ := New(src, Config{Capacity: 10}, nil, nil)
	ctx := context.Background()

	mustGet(t, c, ctx, "p")

	src.results["p"] = stubResult{meta: files.FileMetadata{Path: "p", Version: 2}, body: []byte("v2")}
	c.Invalidate("p")

	meta, body, _ := c.Get(ctx, "p", transport.GetOptions{})
	if meta.Version != 2 || string(body) != "v2" {
		t.Errorf("after invalidate-refetch got version=%d body=%q", meta.Version, body)
	}
}

func TestCache_SourceError_NoCachePopulation(t *testing.T) {
	src := newStub()
	src.getErr = errors.New("transport boom")
	c, _ := New(src, Config{Capacity: 10}, nil, nil)
	_, _, err := c.Get(context.Background(), "p", transport.GetOptions{})
	if err == nil {
		t.Fatal("want error")
	}
	if c.Len() != 0 {
		t.Errorf("len = %d, want 0 (error responses must not populate cache)", c.Len())
	}
}

func TestCache_UnboundedCapacity(t *testing.T) {
	src := newStub()
	for i := 0; i < 100; i++ {
		p := pathOf(i)
		src.results[p] = stubResult{meta: files.FileMetadata{Path: p}, body: []byte("x")}
	}
	c, _ := New(src, Config{Capacity: 0}, nil, nil)
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		mustGet(t, c, ctx, pathOf(i))
	}
	if c.Len() != 100 {
		t.Errorf("len = %d, want 100 (Capacity=0 means unbounded)", c.Len())
	}
}

func TestCache_ConcurrentReads(t *testing.T) {
	src := newStub()
	src.results["p"] = stubResult{meta: files.FileMetadata{Path: "p"}, body: []byte("v")}
	c, _ := New(src, Config{Capacity: 10}, nil, nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_, _, _ = c.Get(context.Background(), "p", transport.GetOptions{})
			}
		}()
	}
	wg.Wait()
	// At least one source call must have happened; cache hits cover the rest.
	if src.callCount() < 1 {
		t.Errorf("source never called, got %d", src.callCount())
	}
	if c.Len() != 1 {
		t.Errorf("len = %d, want 1", c.Len())
	}
}

func TestNewMetrics_Nil(t *testing.T) {
	m, err := NewMetrics(nil)
	if err != nil {
		t.Fatalf("nil registry: err = %v", err)
	}
	if m != nil {
		t.Errorf("want nil emitter, got %+v", m)
	}
	// Nil-safety: calls on nil should not panic.
	m.RecordHit()
	m.RecordMiss(ReasonMiss)
}

// --- helpers -----------------------------------------------------------------

func mustGet(t *testing.T, c *Cache, ctx context.Context, path string) {
	t.Helper()
	if _, _, err := c.Get(ctx, path, transport.GetOptions{}); err != nil {
		t.Fatalf("Get(%s): %v", path, err)
	}
}

func pathOf(i int) string {
	const letters = "abcdefghij"
	if i < len(letters) {
		return string(letters[i])
	}
	return "p" + string('0'+rune(i/10)) + string('0'+rune(i%10))
}
