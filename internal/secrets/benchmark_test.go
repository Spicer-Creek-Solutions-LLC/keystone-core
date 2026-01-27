package secrets

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// BenchmarkSecretRead benchmarks secret read operations.
func BenchmarkSecretRead(b *testing.B) {
	backend := NewMockBackend("vault", BackendTypeVault)
	backend.AddSecret("myapp/secret", map[string]interface{}{
		"username": "admin",
		"password": "secretpassword123",
	})

	broker := NewSecretBroker(&BrokerConfig{
		DefaultBackend: "vault",
	})
	_ = broker.RegisterBackend("vault", backend)

	ctx := context.Background()
	req := &SecretRequest{Path: "vault/myapp/secret"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := broker.Read(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSecretReadWithCache benchmarks secret read with caching.
func BenchmarkSecretReadWithCache(b *testing.B) {
	backend := NewMockBackend("vault", BackendTypeVault)
	backend.AddSecret("myapp/secret", map[string]interface{}{
		"username": "admin",
		"password": "secretpassword123",
	})

	broker := NewSecretBroker(&BrokerConfig{
		DefaultBackend: "vault",
		Cache: &CacheConfig{
			Enabled:    true,
			MaxEntries: 1000,
			DefaultTTL: 5 * time.Minute,
		},
	})
	_ = broker.RegisterBackend("vault", backend)

	// Create a simple in-memory cache for benchmarking
	cache := newBenchmarkCache()
	broker.SetCache(cache)

	ctx := context.Background()
	req := &SecretRequest{Path: "vault/myapp/secret"}

	// Warm up the cache
	_, _ = broker.Read(ctx, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := broker.Read(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSecretReadParallel benchmarks parallel secret reads.
func BenchmarkSecretReadParallel(b *testing.B) {
	backend := NewMockBackend("vault", BackendTypeVault)
	for i := 0; i < 100; i++ {
		backend.AddSecret(fmt.Sprintf("myapp/secret%d", i), map[string]interface{}{
			"value": fmt.Sprintf("secret%d", i),
		})
	}

	broker := NewSecretBroker(&BrokerConfig{
		DefaultBackend: "vault",
	})
	_ = broker.RegisterBackend("vault", backend)

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			req := &SecretRequest{Path: fmt.Sprintf("vault/myapp/secret%d", i%100)}
			_, err := broker.Read(ctx, req)
			if err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

// BenchmarkSecretReadCacheParallel benchmarks parallel cached reads.
func BenchmarkSecretReadCacheParallel(b *testing.B) {
	backend := NewMockBackend("vault", BackendTypeVault)
	for i := 0; i < 100; i++ {
		backend.AddSecret(fmt.Sprintf("myapp/secret%d", i), map[string]interface{}{
			"value": fmt.Sprintf("secret%d", i),
		})
	}

	broker := NewSecretBroker(&BrokerConfig{
		DefaultBackend: "vault",
		Cache: &CacheConfig{
			Enabled:    true,
			MaxEntries: 1000,
			DefaultTTL: 5 * time.Minute,
		},
	})
	_ = broker.RegisterBackend("vault", backend)

	cache := newBenchmarkCache()
	broker.SetCache(cache)

	ctx := context.Background()

	// Warm up the cache
	for i := 0; i < 100; i++ {
		req := &SecretRequest{Path: fmt.Sprintf("vault/myapp/secret%d", i)}
		_, _ = broker.Read(ctx, req)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			req := &SecretRequest{Path: fmt.Sprintf("vault/myapp/secret%d", i%100)}
			_, err := broker.Read(ctx, req)
			if err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}

// BenchmarkCircuitBreaker benchmarks circuit breaker operations.
func BenchmarkCircuitBreaker(b *testing.B) {
	cb := NewCircuitBreaker(nil)

	b.Run("AllowRequest", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = cb.AllowRequest()
		}
	})

	b.Run("RecordSuccess", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			cb.RecordSuccess()
		}
	})

	b.Run("RecordFailure", func(b *testing.B) {
		cb2 := NewCircuitBreaker(&CircuitBreakerConfig{
			FailureThreshold: 1000000, // High threshold so it doesn't open
		})
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cb2.RecordFailure()
		}
	})
}

// BenchmarkRetry benchmarks retry operations.
func BenchmarkRetry(b *testing.B) {
	r := NewRetryer(&RetryConfig{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		Multiplier:   2,
	})

	ctx := context.Background()

	b.Run("ImmediateSuccess", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = r.Execute(ctx, func() error {
				return nil
			})
		}
	})
}

// BenchmarkErrorTranslation benchmarks error translation.
func BenchmarkErrorTranslation(b *testing.B) {
	backends := []BackendType{
		BackendTypeVault,
		BackendTypeAWS,
		BackendTypeAzure,
		BackendTypeGCP,
	}

	errors := []error{
		fmt.Errorf("permission denied"),
		fmt.Errorf("ResourceNotFoundException"),
		fmt.Errorf("rate limit exceeded"),
		fmt.Errorf("connection refused"),
	}

	for _, bt := range backends {
		b.Run(string(bt), func(b *testing.B) {
			translator := NewErrorTranslator(bt)
			for i := 0; i < b.N; i++ {
				err := errors[i%len(errors)]
				_ = translator.Translate(err, "test/path")
			}
		})
	}
}

// BenchmarkMultiBackendRouting benchmarks routing to multiple backends.
func BenchmarkMultiBackendRouting(b *testing.B) {
	broker := NewSecretBroker(&BrokerConfig{
		DefaultBackend: "vault",
		Routing: []RoutingRule{
			{Prefix: "aws/", Backend: "aws"},
			{Prefix: "azure/", Backend: "azure"},
			{Prefix: "gcp/", Backend: "gcp"},
		},
	})

	backends := []struct {
		name string
		bt   BackendType
	}{
		{"vault", BackendTypeVault},
		{"aws", BackendTypeAWS},
		{"azure", BackendTypeAzure},
		{"gcp", BackendTypeGCP},
	}

	for _, be := range backends {
		mock := NewMockBackend(be.name, be.bt)
		mock.AddSecret("test/secret", map[string]interface{}{"value": "test"})
		_ = broker.RegisterBackend(be.name, mock)
	}

	paths := []string{
		"vault/test/secret",
		"aws/test/secret",
		"azure/test/secret",
		"gcp/test/secret",
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := paths[i%len(paths)]
		_, err := broker.Read(ctx, &SecretRequest{Path: path})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// benchmarkCache is a simple in-memory cache for benchmarking.
type benchmarkCache struct {
	mu      sync.RWMutex
	entries map[string]*benchCacheEntry
	stats   CacheStats
}

type benchCacheEntry struct {
	cachedSecret *Secret
	expiresAt    time.Time
}

func newBenchmarkCache() *benchmarkCache {
	return &benchmarkCache{
		entries: make(map[string]*benchCacheEntry),
	}
}

func (c *benchmarkCache) Get(ctx context.Context, path string) (*Secret, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[path]
	if !ok {
		c.stats.Misses++
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		c.stats.Misses++
		return nil, false
	}

	c.stats.Hits++
	return entry.cachedSecret, true
}

func (c *benchmarkCache) Put(ctx context.Context, secret *Secret, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[secret.Path] = &benchCacheEntry{
		cachedSecret: secret,
		expiresAt:    time.Now().Add(ttl),
	}
	c.stats.Entries = len(c.entries)
	return nil
}

func (c *benchmarkCache) Delete(ctx context.Context, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, path)
	c.stats.Entries = len(c.entries)
	return nil
}

func (c *benchmarkCache) DeleteByPrefix(ctx context.Context, prefix string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for path := range c.entries {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			delete(c.entries, path)
			count++
		}
	}
	c.stats.Entries = len(c.entries)
	return count, nil
}

func (c *benchmarkCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*benchCacheEntry)
	c.stats.Entries = 0
	return nil
}

func (c *benchmarkCache) Stats() *CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return &c.stats
}

func (c *benchmarkCache) Close() error {
	return nil
}

// Ensure benchmarkCache implements SecretCache.
var _ SecretCache = (*benchmarkCache)(nil)

// TestCachedReadLatency verifies the <50ms latency target.
func TestCachedReadLatency(t *testing.T) {
	backend := NewMockBackend("vault", BackendTypeVault)
	backend.AddSecret("myapp/secret", map[string]interface{}{
		"username": "admin",
		"password": "secretpassword123",
	})

	broker := NewSecretBroker(&BrokerConfig{
		DefaultBackend: "vault",
		Cache: &CacheConfig{
			Enabled:    true,
			MaxEntries: 1000,
			DefaultTTL: 5 * time.Minute,
		},
	})
	_ = broker.RegisterBackend("vault", backend)

	cache := newBenchmarkCache()
	broker.SetCache(cache)

	ctx := context.Background()
	req := &SecretRequest{Path: "vault/myapp/secret"}

	// Warm up the cache
	_, err := broker.Read(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	// Measure cached read latency
	iterations := 1000
	var totalDuration time.Duration

	for i := 0; i < iterations; i++ {
		start := time.Now()
		_, err := broker.Read(ctx, req)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		totalDuration += elapsed
	}

	avgLatency := totalDuration / time.Duration(iterations)

	// Log the average latency
	t.Logf("Average cached read latency: %v", avgLatency)

	// Verify latency is under 50ms (target)
	if avgLatency > 50*time.Millisecond {
		t.Errorf("Average cached read latency %v exceeds 50ms target", avgLatency)
	}

	// In practice, cached reads should be sub-millisecond
	if avgLatency > 1*time.Millisecond {
		t.Logf("Warning: cached read latency %v is higher than expected (should be sub-millisecond)", avgLatency)
	}
}

// TestThroughput tests secret retrieval throughput.
func TestThroughput(t *testing.T) {
	backend := NewMockBackend("vault", BackendTypeVault)
	for i := 0; i < 100; i++ {
		backend.AddSecret(fmt.Sprintf("myapp/secret%d", i), map[string]interface{}{
			"value": fmt.Sprintf("secret%d", i),
		})
	}

	broker := NewSecretBroker(&BrokerConfig{
		DefaultBackend: "vault",
		Cache: &CacheConfig{
			Enabled:    true,
			MaxEntries: 1000,
			DefaultTTL: 5 * time.Minute,
		},
	})
	_ = broker.RegisterBackend("vault", backend)

	cache := newBenchmarkCache()
	broker.SetCache(cache)

	ctx := context.Background()

	// Warm up the cache
	for i := 0; i < 100; i++ {
		req := &SecretRequest{Path: fmt.Sprintf("vault/myapp/secret%d", i)}
		_, _ = broker.Read(ctx, req)
	}

	// Measure throughput
	duration := 1 * time.Second
	done := make(chan struct{})
	var ops int64

	go func() {
		time.Sleep(duration)
		close(done)
	}()

	var wg sync.WaitGroup
	numWorkers := 10

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			i := 0
			for {
				select {
				case <-done:
					return
				default:
					req := &SecretRequest{Path: fmt.Sprintf("vault/myapp/secret%d", i%100)}
					_, err := broker.Read(ctx, req)
					if err != nil {
						t.Errorf("worker %d: read error: %v", workerID, err)
						return
					}
					ops++
					i++
				}
			}
		}(w)
	}

	wg.Wait()

	opsPerSecond := float64(ops) / duration.Seconds()
	t.Logf("Throughput: %.0f ops/sec with %d workers", opsPerSecond, numWorkers)

	// Expect at least 10,000 ops/sec for cached reads
	if opsPerSecond < 10000 {
		t.Logf("Warning: throughput %.0f ops/sec is lower than expected (target: 10,000+ ops/sec)", opsPerSecond)
	}
}
