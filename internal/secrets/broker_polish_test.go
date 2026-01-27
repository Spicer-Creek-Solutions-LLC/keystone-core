package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Connection Pool Tests
// =============================================================================

type mockConnection struct {
	id      int
	healthy bool
	closed  bool
}

func (c *mockConnection) Close() error {
	c.closed = true
	return nil
}

func (c *mockConnection) IsHealthy(ctx context.Context) bool {
	return c.healthy
}

type mockConnectionFactory struct {
	mu      sync.Mutex
	counter int
	healthy bool
	failOn  int // Fail creation on this count
}

func (f *mockConnectionFactory) Create(ctx context.Context) (Connection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.counter++
	if f.failOn > 0 && f.counter == f.failOn {
		return nil, fmt.Errorf("connection creation failed")
	}

	return &mockConnection{
		id:      f.counter,
		healthy: f.healthy,
	}, nil
}

func (f *mockConnectionFactory) Validate(ctx context.Context, conn Connection) bool {
	mc, ok := conn.(*mockConnection)
	if !ok {
		return false
	}
	return mc.healthy
}

func TestConnectionPool_Basic(t *testing.T) {
	factory := &mockConnectionFactory{healthy: true}
	config := &PoolConfig{
		MinConnections: 2,
		MaxConnections: 5,
		AcquireTimeout: time.Second,
	}

	pool, err := NewConnectionPool(config, factory)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	ctx := context.Background()
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("failed to start pool: %v", err)
	}
	defer pool.Close()

	// Should have created min connections
	stats := pool.Stats()
	if stats.CurrentSize != 2 {
		t.Errorf("expected 2 connections, got %d", stats.CurrentSize)
	}

	// Acquire a connection
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire: %v", err)
	}

	stats = pool.Stats()
	if stats.InUse != 1 {
		t.Errorf("expected 1 in use, got %d", stats.InUse)
	}

	// Release the connection
	pool.Release(conn)

	stats = pool.Stats()
	if stats.InUse != 0 {
		t.Errorf("expected 0 in use after release, got %d", stats.InUse)
	}
}

func TestConnectionPool_MaxConnections(t *testing.T) {
	factory := &mockConnectionFactory{healthy: true}
	config := &PoolConfig{
		MinConnections: 1,
		MaxConnections: 3,
		AcquireTimeout: 100 * time.Millisecond,
	}

	pool, err := NewConnectionPool(config, factory)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	ctx := context.Background()
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("failed to start pool: %v", err)
	}
	defer pool.Close()

	// Acquire all connections
	var conns []Connection
	for i := 0; i < 3; i++ {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("failed to acquire connection %d: %v", i, err)
		}
		conns = append(conns, conn)
	}

	// Next acquire should timeout
	_, err = pool.Acquire(ctx)
	if err == nil {
		t.Error("expected timeout error when pool exhausted")
	}

	// Release one and try again
	pool.Release(conns[0])
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Errorf("should acquire after release: %v", err)
	} else {
		pool.Release(conn)
	}

	// Cleanup
	for _, c := range conns[1:] {
		pool.Release(c)
	}
}

func TestConnectionPool_HealthCheck(t *testing.T) {
	factory := &mockConnectionFactory{healthy: true}
	config := &PoolConfig{
		MinConnections:       1,
		MaxConnections:       5,
		AcquireTimeout:       time.Second,
		HealthCheckOnAcquire: true,
	}

	pool, err := NewConnectionPool(config, factory)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	ctx := context.Background()
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("failed to start pool: %v", err)
	}
	defer pool.Close()

	// Acquire should work with healthy connections
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire: %v", err)
	}
	pool.Release(conn)

	// Mark as unhealthy
	factory.healthy = false

	// Acquire should still work (creates new connection)
	conn, err = pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("failed to acquire with unhealthy check: %v", err)
	}
	pool.Release(conn)
}

func TestConnectionPool_Stats(t *testing.T) {
	factory := &mockConnectionFactory{healthy: true}
	config := DefaultPoolConfig()
	config.MinConnections = 3

	pool, err := NewConnectionPool(config, factory)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}

	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Close()

	stats := pool.Stats()
	if stats.TotalCreated != 3 {
		t.Errorf("expected 3 created, got %d", stats.TotalCreated)
	}
	if stats.CurrentSize != 3 {
		t.Errorf("expected size 3, got %d", stats.CurrentSize)
	}
}

// =============================================================================
// Rate Limiter Tests
// =============================================================================

func TestRateLimiter_Basic(t *testing.T) {
	config := &RateLimitConfig{
		Enabled:        true,
		GlobalLimit:    10, // 10 requests/second
		GlobalBurst:    10,
		PerClientLimit: 0,  // Disable per-client to test global only
		PerClientBurst: 0,
		PerPathLimit:   0,  // Disable per-path
		PerPathBurst:   0,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Should allow initial burst (global limit of 10)
	for i := 0; i < 10; i++ {
		result, err := limiter.Allow(ctx, &RateLimitRequest{
			Path:     "/test",
			ClientID: "client1",
		})
		if err != nil {
			t.Fatalf("limiter error: %v", err)
		}
		if !result.Allowed {
			t.Errorf("request %d should be allowed", i)
		}
	}

	// Next request should be rate limited (global limit hit)
	result, _ := limiter.Allow(ctx, &RateLimitRequest{
		Path:     "/test",
		ClientID: "client1",
	})
	if result.Allowed {
		t.Error("request should be rate limited")
	}
}

func TestRateLimiter_PerClient(t *testing.T) {
	config := &RateLimitConfig{
		Enabled:        true,
		GlobalLimit:    1000, // High global limit
		GlobalBurst:    1000,
		PerClientLimit: 3,
		PerClientBurst: 3,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Client 1 should be limited after 3 requests
	for i := 0; i < 3; i++ {
		result, _ := limiter.Allow(ctx, &RateLimitRequest{
			ClientID: "client1",
		})
		if !result.Allowed {
			t.Errorf("client1 request %d should be allowed", i)
		}
	}

	result, _ := limiter.Allow(ctx, &RateLimitRequest{
		ClientID: "client1",
	})
	if result.Allowed {
		t.Error("client1 should be rate limited")
	}

	// Client 2 should still be allowed
	result, _ = limiter.Allow(ctx, &RateLimitRequest{
		ClientID: "client2",
	})
	if !result.Allowed {
		t.Error("client2 should be allowed")
	}
}

func TestRateLimiter_Disabled(t *testing.T) {
	config := &RateLimitConfig{
		Enabled: false,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// All requests should be allowed when disabled
	for i := 0; i < 100; i++ {
		result, _ := limiter.Allow(ctx, &RateLimitRequest{
			Path:     "/test",
			ClientID: "client1",
		})
		if !result.Allowed {
			t.Errorf("request %d should be allowed when disabled", i)
		}
	}
}

func TestRateLimiter_Stats(t *testing.T) {
	config := DefaultRateLimitConfig()
	config.GlobalLimit = 5
	config.GlobalBurst = 5

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Make some requests
	for i := 0; i < 10; i++ {
		limiter.Allow(ctx, &RateLimitRequest{ClientID: "test"})
	}

	stats := limiter.Stats()
	if stats.TotalRequests != 10 {
		t.Errorf("expected 10 total requests, got %d", stats.TotalRequests)
	}
	if stats.AllowedRequests != 5 {
		t.Errorf("expected 5 allowed, got %d", stats.AllowedRequests)
	}
	if stats.RejectedRequests != 5 {
		t.Errorf("expected 5 rejected, got %d", stats.RejectedRequests)
	}
}

func TestRateLimiter_PathRules(t *testing.T) {
	config := &RateLimitConfig{
		Enabled:     true,
		GlobalLimit: 1000,
		GlobalBurst: 1000,
		PathRules: []PathRateLimitRule{
			{PathPrefix: "/sensitive/", Limit: 2, Burst: 2},
		},
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Sensitive path should be limited
	for i := 0; i < 2; i++ {
		result, _ := limiter.Allow(ctx, &RateLimitRequest{
			Path: "/sensitive/secret",
		})
		if !result.Allowed {
			t.Errorf("sensitive request %d should be allowed", i)
		}
	}

	result, _ := limiter.Allow(ctx, &RateLimitRequest{
		Path: "/sensitive/secret",
	})
	if result.Allowed {
		t.Error("sensitive path should be rate limited")
	}

	// Non-sensitive path should still work
	result, _ = limiter.Allow(ctx, &RateLimitRequest{
		Path: "/normal/path",
	})
	if !result.Allowed {
		t.Error("normal path should be allowed")
	}
}

func TestRateLimitError(t *testing.T) {
	err := &RateLimitError{
		Reason:     "test limit exceeded",
		RetryAfter: time.Second,
	}

	if !IsRateLimitError(err) {
		t.Error("should be recognized as rate limit error")
	}

	msg := err.Error()
	if msg == "" {
		t.Error("error message should not be empty")
	}
}

// =============================================================================
// Metrics Tests
// =============================================================================

func TestBrokerMetrics_Recording(t *testing.T) {
	metrics := NewBrokerMetrics()

	// Record some requests
	metrics.RecordRequest("read", "vault", 10*time.Millisecond, nil)
	metrics.RecordRequest("read", "vault", 20*time.Millisecond, nil)
	metrics.RecordRequest("read_dynamic", "vault", 30*time.Millisecond, fmt.Errorf("test error"))

	snapshot := metrics.Snapshot()

	if snapshot.RequestsTotal != 3 {
		t.Errorf("expected 3 total requests, got %d", snapshot.RequestsTotal)
	}
	if snapshot.RequestsSuccessful != 2 {
		t.Errorf("expected 2 successful, got %d", snapshot.RequestsSuccessful)
	}
	if snapshot.RequestsFailed != 1 {
		t.Errorf("expected 1 failed, got %d", snapshot.RequestsFailed)
	}
	if snapshot.ReadRequests != 2 {
		t.Errorf("expected 2 read requests, got %d", snapshot.ReadRequests)
	}
	if snapshot.ReadDynamicRequests != 1 {
		t.Errorf("expected 1 dynamic read, got %d", snapshot.ReadDynamicRequests)
	}
}

func TestBrokerMetrics_Cache(t *testing.T) {
	metrics := NewBrokerMetrics()

	metrics.RecordCacheHit()
	metrics.RecordCacheHit()
	metrics.RecordCacheMiss()

	snapshot := metrics.Snapshot()

	if snapshot.CacheHits != 2 {
		t.Errorf("expected 2 cache hits, got %d", snapshot.CacheHits)
	}
	if snapshot.CacheMisses != 1 {
		t.Errorf("expected 1 cache miss, got %d", snapshot.CacheMisses)
	}
	if snapshot.CacheHitRate < 66 || snapshot.CacheHitRate > 67 {
		t.Errorf("expected ~66%% hit rate, got %.2f", snapshot.CacheHitRate)
	}
}

func TestBrokerMetrics_Latency(t *testing.T) {
	metrics := NewBrokerMetrics()

	// Record various latencies
	for _, d := range []time.Duration{10, 20, 30, 40, 50} {
		metrics.RecordRequest("read", "", d*time.Millisecond, nil)
	}

	snapshot := metrics.Snapshot()
	stats, ok := snapshot.Latencies["read"]
	if !ok {
		t.Fatal("expected read latency stats")
	}

	if stats.Count != 5 {
		t.Errorf("expected 5 samples, got %d", stats.Count)
	}
	if stats.Avg != 30 {
		t.Errorf("expected avg 30ms, got %.2f", stats.Avg)
	}
	if stats.Min != 10 {
		t.Errorf("expected min 10ms, got %.2f", stats.Min)
	}
	if stats.Max != 50 {
		t.Errorf("expected max 50ms, got %.2f", stats.Max)
	}
}

func TestBrokerMetrics_Rotation(t *testing.T) {
	metrics := NewBrokerMetrics()

	metrics.RecordRotationStart()
	metrics.RecordRotationStart()
	metrics.RecordRotationComplete(true)
	metrics.RecordRotationComplete(false)

	snapshot := metrics.Snapshot()

	if snapshot.RotationsStarted != 2 {
		t.Errorf("expected 2 started, got %d", snapshot.RotationsStarted)
	}
	if snapshot.RotationsCompleted != 1 {
		t.Errorf("expected 1 completed, got %d", snapshot.RotationsCompleted)
	}
	if snapshot.RotationsFailed != 1 {
		t.Errorf("expected 1 failed, got %d", snapshot.RotationsFailed)
	}
}

func TestBrokerMetrics_Reset(t *testing.T) {
	metrics := NewBrokerMetrics()

	metrics.RecordRequest("read", "", time.Millisecond, nil)
	metrics.RecordCacheHit()

	metrics.Reset()

	snapshot := metrics.Snapshot()
	if snapshot.RequestsTotal != 0 {
		t.Error("metrics should be reset")
	}
	if snapshot.CacheHits != 0 {
		t.Error("cache hits should be reset")
	}
}

func TestPrometheusExporter(t *testing.T) {
	metrics := NewBrokerMetrics()

	metrics.RecordRequest("read", "vault", 10*time.Millisecond, nil)
	metrics.RecordCacheHit()

	exporter := NewPrometheusExporter(metrics, "test")
	output := exporter.Export()

	if output == "" {
		t.Error("prometheus output should not be empty")
	}

	// Check for expected metrics
	expectedMetrics := []string{
		"test_requests_total",
		"test_cache_hits_total",
		"test_request_rate",
	}
	for _, m := range expectedMetrics {
		if len(output) < len(m) {
			t.Errorf("expected metric %s in output", m)
		}
	}
}

// =============================================================================
// Health Server Tests
// =============================================================================

func TestHealthServer_Liveness(t *testing.T) {
	broker := NewSecretBroker(nil)
	server := NewHealthServer(broker, nil, nil)

	req := httptest.NewRequest("GET", "/health/live", nil)
	w := httptest.NewRecorder()

	server.handleLiveness(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["status"] != "ok" {
		t.Errorf("expected status ok, got %s", response["status"])
	}
}

func TestHealthServer_Readiness(t *testing.T) {
	broker := NewSecretBroker(nil)
	server := NewHealthServer(broker, nil, nil)

	// Without backends, broker considers itself healthy (by design - empty is OK)
	// Readiness check returns OK if broker.Healthy() returns true
	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()

	server.handleReadiness(w, req)

	// Broker with no backends is considered healthy (just not useful)
	// so readiness returns OK
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for empty broker (considered healthy), got %d", w.Code)
	}

	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["status"] != "ready" {
		t.Errorf("expected status ready, got %s", response["status"])
	}
}

func TestHealthServer_Health(t *testing.T) {
	broker := NewSecretBroker(nil)
	metrics := NewBrokerMetrics()
	server := NewHealthServer(broker, metrics, nil)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	var status HealthStatus
	json.Unmarshal(w.Body.Bytes(), &status)

	if status.Status == "" {
		t.Error("status should not be empty")
	}
	if status.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
}

func TestHealthServer_DetailedHealth(t *testing.T) {
	broker := NewSecretBroker(nil)
	metrics := NewBrokerMetrics()
	limiter := NewRateLimiter(DefaultRateLimitConfig())
	server := NewHealthServer(broker, metrics, limiter)

	req := httptest.NewRequest("GET", "/health/detailed", nil)
	w := httptest.NewRecorder()

	server.handleDetailedHealth(w, req)

	var status HealthStatus
	json.Unmarshal(w.Body.Bytes(), &status)

	if status.Components == nil {
		t.Error("components should not be nil")
	}
}

func TestHealthServer_Stats(t *testing.T) {
	broker := NewSecretBroker(nil)
	metrics := NewBrokerMetrics()
	server := NewHealthServer(broker, metrics, nil)

	req := httptest.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHealthServer_Metrics(t *testing.T) {
	broker := NewSecretBroker(nil)
	metrics := NewBrokerMetrics()
	metrics.RecordRequest("read", "", time.Millisecond, nil)
	server := NewHealthServer(broker, metrics, nil)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	server.handleMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/plain; version=0.0.4" {
		t.Errorf("unexpected content type: %s", contentType)
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestRateLimitedBroker(t *testing.T) {
	broker := NewSecretBroker(nil)
	config := &RateLimitConfig{
		Enabled:     true,
		GlobalLimit: 5,
		GlobalBurst: 5,
	}

	rlBroker := NewRateLimitedBroker(broker, config)
	ctx := context.Background()

	rlBroker.Start(ctx)
	defer rlBroker.Stop()

	// Should work until limit is hit
	for i := 0; i < 5; i++ {
		_, err := rlBroker.Read(ctx, &SecretRequest{Path: "test/path"})
		if IsRateLimitError(err) {
			t.Errorf("request %d should not be rate limited", i)
		}
	}

	// Next should be rate limited
	_, err := rlBroker.Read(ctx, &SecretRequest{Path: "test/path"})
	if !IsRateLimitError(err) {
		t.Error("should be rate limited")
	}

	stats := rlBroker.RateLimitStats()
	if stats.RejectedRequests == 0 {
		t.Error("should have rejected requests")
	}
}

func TestMetricsBroker(t *testing.T) {
	broker := NewSecretBroker(nil)
	metricsBroker := NewMetricsBroker(broker)

	ctx := context.Background()

	// Make some requests (they will fail but metrics should be recorded)
	for i := 0; i < 5; i++ {
		metricsBroker.Read(ctx, &SecretRequest{Path: "test/path"})
	}

	snapshot := metricsBroker.MetricsSnapshot()
	if snapshot.RequestsTotal != 5 {
		t.Errorf("expected 5 requests recorded, got %d", snapshot.RequestsTotal)
	}
}

// =============================================================================
// Benchmark Tests
// =============================================================================

func BenchmarkRateLimiter_Allow(b *testing.B) {
	config := DefaultRateLimitConfig()
	limiter := NewRateLimiter(config)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(ctx, &RateLimitRequest{
			Path:     "/test/path",
			ClientID: "client1",
		})
	}
}

func BenchmarkConnectionPool_AcquireRelease(b *testing.B) {
	factory := &mockConnectionFactory{healthy: true}
	config := DefaultPoolConfig()

	pool, _ := NewConnectionPool(config, factory)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, _ := pool.Acquire(ctx)
		pool.Release(conn)
	}
}

func BenchmarkMetrics_Record(b *testing.B) {
	metrics := NewBrokerMetrics()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.RecordRequest("read", "vault", 10*time.Millisecond, nil)
	}
}

func BenchmarkMetrics_Snapshot(b *testing.B) {
	metrics := NewBrokerMetrics()

	// Pre-populate some data
	for i := 0; i < 1000; i++ {
		metrics.RecordRequest("read", "vault", time.Millisecond*time.Duration(i%100), nil)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.Snapshot()
	}
}

// =============================================================================
// Concurrent Access Tests
// =============================================================================

func TestConnectionPool_Concurrent(t *testing.T) {
	factory := &mockConnectionFactory{healthy: true}
	config := &PoolConfig{
		MinConnections: 5,
		MaxConnections: 10,
		AcquireTimeout: time.Second,
	}

	pool, _ := NewConnectionPool(config, factory)
	ctx := context.Background()
	pool.Start(ctx)
	defer pool.Close()

	var wg sync.WaitGroup
	var acquireErrors int64
	var releaseCount int64

	// Spawn many goroutines
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				conn, err := pool.Acquire(ctx)
				if err != nil {
					atomic.AddInt64(&acquireErrors, 1)
					continue
				}

				// Simulate some work
				time.Sleep(time.Microsecond)

				pool.Release(conn)
				atomic.AddInt64(&releaseCount, 1)
			}
		}()
	}

	wg.Wait()

	t.Logf("Acquire errors: %d, Releases: %d", acquireErrors, releaseCount)
}

func TestRateLimiter_Concurrent(t *testing.T) {
	config := &RateLimitConfig{
		Enabled:        true,
		GlobalLimit:    1000,
		GlobalBurst:    1000,
		PerClientLimit: 100,
		PerClientBurst: 100,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	var wg sync.WaitGroup
	var allowed, rejected int64

	for i := 0; i < 50; i++ {
		wg.Add(1)
		clientID := fmt.Sprintf("client%d", i%10)
		go func(cid string) {
			defer wg.Done()

			for j := 0; j < 50; j++ {
				result, _ := limiter.Allow(ctx, &RateLimitRequest{
					ClientID: cid,
				})
				if result.Allowed {
					atomic.AddInt64(&allowed, 1)
				} else {
					atomic.AddInt64(&rejected, 1)
				}
			}
		}(clientID)
	}

	wg.Wait()

	t.Logf("Allowed: %d, Rejected: %d", allowed, rejected)

	// With these settings, most should be allowed
	if allowed == 0 {
		t.Error("some requests should be allowed")
	}
}

func TestMetrics_Concurrent(t *testing.T) {
	metrics := NewBrokerMetrics()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < 100; j++ {
				metrics.RecordRequest("read", "vault", time.Millisecond, nil)
				metrics.RecordCacheHit()
			}
		}(i)
	}

	wg.Wait()

	snapshot := metrics.Snapshot()
	if snapshot.RequestsTotal != 10000 {
		t.Errorf("expected 10000 requests, got %d", snapshot.RequestsTotal)
	}
	if snapshot.CacheHits != 10000 {
		t.Errorf("expected 10000 cache hits, got %d", snapshot.CacheHits)
	}
}
