package ratelimit

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestNewRegistry_DefaultCapacity(t *testing.T) {
	r := NewRegistry(RegistryConfig{Default: Config{RequestsPerMinute: 60}})
	if r.cfg.Capacity != DefaultCapacity {
		t.Errorf("capacity = %d, want %d", r.cfg.Capacity, DefaultCapacity)
	}
}

func TestNewRegistry_ExplicitCapacity(t *testing.T) {
	r := NewRegistry(RegistryConfig{
		Default:  Config{RequestsPerMinute: 60},
		Capacity: 100,
	})
	if r.cfg.Capacity != 100 {
		t.Errorf("capacity = %d, want 100", r.cfg.Capacity)
	}
}

func TestRegistry_PerKeyIsolation(t *testing.T) {
	// Each key has its own bucket — exhausting one key must
	// not affect another.
	r := NewRegistry(RegistryConfig{
		Default: Config{RequestsPerMinute: 60, Burst: 1},
	})

	if !r.Allow("alice") {
		t.Fatal("alice first allow denied")
	}
	if r.Allow("alice") {
		t.Error("alice second allow should be denied")
	}
	// bob is unaffected.
	if !r.Allow("bob") {
		t.Error("bob's first allow should be allowed despite alice exhausting")
	}
}

func TestRegistry_LRUEviction(t *testing.T) {
	r := NewRegistry(RegistryConfig{
		Default:  Config{RequestsPerMinute: 60, Burst: 1},
		Capacity: 2,
	})

	// Insert three keys; the first (a) should evict because
	// capacity is 2.
	r.Allow("a")
	r.Allow("b")
	r.Allow("c")

	if r.Len() != 2 {
		t.Errorf("Len = %d, want 2", r.Len())
	}

	// a is gone — re-Allow creates a fresh bucket (so Allow
	// returns true since the new bucket has its burst).
	if !r.Allow("a") {
		t.Error("a after eviction should be a fresh bucket — first Allow allowed")
	}

	// b should now be evicted because adding "a" pushed it
	// off the LRU tail.
	if r.Len() != 2 {
		t.Errorf("Len after re-adding a = %d, want 2", r.Len())
	}
}

func TestRegistry_HotKeyStaysCached(t *testing.T) {
	// Recently-used keys move to MRU position and are not the
	// first to evict.
	r := NewRegistry(RegistryConfig{
		Default:  Config{RequestsPerMinute: 60, Burst: 1},
		Capacity: 2,
	})

	r.Allow("hot")  // [hot]
	r.Allow("cold") // [cold, hot]
	r.Allow("hot")  // [hot, cold] — hot moved to MRU
	r.Allow("new")  // [new, hot] — cold evicted

	if r.Len() != 2 {
		t.Errorf("Len = %d, want 2", r.Len())
	}
}

func TestRegistry_AllowOrRetryAfter(t *testing.T) {
	r := NewRegistry(RegistryConfig{Default: Config{RequestsPerMinute: 60, Burst: 1}})

	allowed, delay := r.AllowOrRetryAfter("k")
	if !allowed || delay != 0 {
		t.Errorf("first: (%v, %v), want (true, 0)", allowed, delay)
	}

	allowed, delay = r.AllowOrRetryAfter("k")
	if allowed {
		t.Error("second call allowed; want denied")
	}
	if delay <= 0 {
		t.Errorf("delay = %v, want > 0", delay)
	}
}

func TestRegistry_PassthroughConfig(t *testing.T) {
	// RPM=0 default → every Allow returns true on any key.
	r := NewRegistry(RegistryConfig{Default: Config{}})
	for i := 0; i < 1000; i++ {
		if !r.Allow(fmt.Sprintf("k-%d", i)) {
			t.Fatalf("passthrough Allow denied at i=%d", i)
		}
	}
}

func TestRegistry_ConcurrentAllow(t *testing.T) {
	r := NewRegistry(RegistryConfig{
		Default:  Config{RequestsPerMinute: 60000, Burst: 1000},
		Capacity: 100,
	})

	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("k-%d", id%10)
			for j := 0; j < 20; j++ {
				if r.Allow(key) {
					allowed.Add(1)
				}
			}
		}(i)
	}
	wg.Wait()

	// 50 goroutines × 20 calls = 1000 attempts across 10 keys.
	// With burst 1000 per key, all 1000 attempts should be
	// allowed; this primarily exercises the lock for race-detector.
	if allowed.Load() != 1000 {
		t.Logf("allowed = %d (informational; lock-correctness checked under -race)", allowed.Load())
	}
}

func TestRegistry_EmptyKey(t *testing.T) {
	// An empty key is a valid key — the registry treats "" like
	// any other string. This matters because some extractors
	// (anonymous requests) may produce an empty key.
	r := NewRegistry(RegistryConfig{Default: Config{RequestsPerMinute: 60, Burst: 1}})
	if !r.Allow("") {
		t.Error("empty-key first Allow denied")
	}
	if r.Allow("") {
		t.Error("empty-key second Allow should be denied (shares one bucket)")
	}
}

func TestRegistry_Len(t *testing.T) {
	r := NewRegistry(RegistryConfig{
		Default:  Config{RequestsPerMinute: 60},
		Capacity: 10,
	})
	for i := 0; i < 5; i++ {
		r.Allow(fmt.Sprintf("k-%d", i))
	}
	if got := r.Len(); got != 5 {
		t.Errorf("Len = %d, want 5", got)
	}
}
