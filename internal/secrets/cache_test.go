package secrets

import "testing"

func TestNoopCache_AlwaysMisses(t *testing.T) {
	t.Parallel()
	c := NoopCache{}
	c.Put("kv/foo", &Secret{Path: "kv/foo"})
	if _, ok := c.Get("kv/foo"); ok {
		t.Errorf("NoopCache.Get returned hit after Put")
	}
	// Invalidate paths must be safe to call.
	c.InvalidatePath("kv/foo")
	c.InvalidatePrefix("kv/")
	if stats := c.Stats(); stats != (CacheStats{}) {
		t.Errorf("NoopCache.Stats() = %#v, want zero value", stats)
	}
}

func TestDefaultCache_IsNoop(t *testing.T) {
	t.Parallel()
	c := DefaultCache()
	if _, ok := c.Get("anything"); ok {
		t.Errorf("DefaultCache.Get returned hit on cold lookup")
	}
}
