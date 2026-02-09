package secrets

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// BrokerMetrics collects metrics for the secret broker.
type BrokerMetrics struct {
	// Request metrics
	requestsTotal      int64
	requestsSuccessful int64
	requestsFailed     int64

	// Request types
	readRequests        int64
	readDynamicRequests int64
	listRequests        int64
	renewRequests       int64
	revokeRequests      int64

	// Cache metrics
	cacheHits   int64
	cacheMisses int64

	// Latency tracking
	mu             sync.RWMutex
	latencyBuckets map[string]*latencyBucket

	// Backend metrics
	backendMetrics map[string]*BackendMetrics

	// Rate limit metrics
	rateLimitRejections int64

	// Rotation metrics
	rotationsStarted   int64
	rotationsCompleted int64
	rotationsFailed    int64

	// Error tracking
	errorCounts map[string]int64

	// Start time
	startTime time.Time
}

// BackendMetrics contains metrics for a specific backend.
type BackendMetrics struct {
	Name string

	RequestsTotal      int64
	RequestsSuccessful int64
	RequestsFailed     int64

	AvgLatencyMs float64
	P50LatencyMs float64
	P95LatencyMs float64
	P99LatencyMs float64

	LastSuccessTime time.Time
	LastErrorTime   time.Time
	LastError       string

	ConnectionPoolSize      int
	ConnectionPoolAvailable int
	ConnectionPoolInUse     int
}

// latencyBucket tracks latency measurements.
type latencyBucket struct {
	mu         sync.Mutex
	count      int64
	sum        float64
	min        float64
	max        float64
	samples    []float64
	maxSamples int
}

// NewBrokerMetrics creates a new broker metrics collector.
func NewBrokerMetrics() *BrokerMetrics {
	return &BrokerMetrics{
		latencyBuckets: make(map[string]*latencyBucket),
		backendMetrics: make(map[string]*BackendMetrics),
		errorCounts:    make(map[string]int64),
		startTime:      time.Now(),
	}
}

// RecordRequest records a secret request.
func (m *BrokerMetrics) RecordRequest(operation, backend string, duration time.Duration, err error) {
	atomic.AddInt64(&m.requestsTotal, 1)

	// Track operation type
	switch operation {
	case "read":
		atomic.AddInt64(&m.readRequests, 1)
	case "read_dynamic":
		atomic.AddInt64(&m.readDynamicRequests, 1)
	case "list":
		atomic.AddInt64(&m.listRequests, 1)
	case "renew":
		atomic.AddInt64(&m.renewRequests, 1)
	case "revoke":
		atomic.AddInt64(&m.revokeRequests, 1)
	}

	// Track success/failure
	if err == nil {
		atomic.AddInt64(&m.requestsSuccessful, 1)
	} else {
		atomic.AddInt64(&m.requestsFailed, 1)
		m.recordError(err)
	}

	// Track latency
	m.recordLatency(operation, duration)
	if backend != "" {
		m.recordBackendLatency(backend, duration, err)
	}
}

// RecordCacheHit records a cache hit.
func (m *BrokerMetrics) RecordCacheHit() {
	atomic.AddInt64(&m.cacheHits, 1)
}

// RecordCacheMiss records a cache miss.
func (m *BrokerMetrics) RecordCacheMiss() {
	atomic.AddInt64(&m.cacheMisses, 1)
}

// RecordRateLimitRejection records a rate limit rejection.
func (m *BrokerMetrics) RecordRateLimitRejection() {
	atomic.AddInt64(&m.rateLimitRejections, 1)
}

// RecordRotationStart records the start of a rotation.
func (m *BrokerMetrics) RecordRotationStart() {
	atomic.AddInt64(&m.rotationsStarted, 1)
}

// RecordRotationComplete records the completion of a rotation.
func (m *BrokerMetrics) RecordRotationComplete(success bool) {
	if success {
		atomic.AddInt64(&m.rotationsCompleted, 1)
	} else {
		atomic.AddInt64(&m.rotationsFailed, 1)
	}
}

// Snapshot returns a snapshot of the current metrics.
func (m *BrokerMetrics) Snapshot() *MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := &MetricsSnapshot{
		Timestamp: time.Now(),
		Uptime:    time.Since(m.startTime),

		RequestsTotal:      atomic.LoadInt64(&m.requestsTotal),
		RequestsSuccessful: atomic.LoadInt64(&m.requestsSuccessful),
		RequestsFailed:     atomic.LoadInt64(&m.requestsFailed),

		ReadRequests:        atomic.LoadInt64(&m.readRequests),
		ReadDynamicRequests: atomic.LoadInt64(&m.readDynamicRequests),
		ListRequests:        atomic.LoadInt64(&m.listRequests),
		RenewRequests:       atomic.LoadInt64(&m.renewRequests),
		RevokeRequests:      atomic.LoadInt64(&m.revokeRequests),

		CacheHits:   atomic.LoadInt64(&m.cacheHits),
		CacheMisses: atomic.LoadInt64(&m.cacheMisses),

		RateLimitRejections: atomic.LoadInt64(&m.rateLimitRejections),

		RotationsStarted:   atomic.LoadInt64(&m.rotationsStarted),
		RotationsCompleted: atomic.LoadInt64(&m.rotationsCompleted),
		RotationsFailed:    atomic.LoadInt64(&m.rotationsFailed),

		Latencies: make(map[string]LatencyStats),
		Backends:  make(map[string]*BackendMetrics),
		Errors:    make(map[string]int64),
	}

	// Copy latency stats
	for op, bucket := range m.latencyBuckets {
		snapshot.Latencies[op] = bucket.stats()
	}

	// Copy backend metrics
	for name, bm := range m.backendMetrics {
		snapshot.Backends[name] = &BackendMetrics{
			Name:               bm.Name,
			RequestsTotal:      bm.RequestsTotal,
			RequestsSuccessful: bm.RequestsSuccessful,
			RequestsFailed:     bm.RequestsFailed,
			AvgLatencyMs:       bm.AvgLatencyMs,
			P50LatencyMs:       bm.P50LatencyMs,
			P95LatencyMs:       bm.P95LatencyMs,
			P99LatencyMs:       bm.P99LatencyMs,
			LastSuccessTime:    bm.LastSuccessTime,
			LastErrorTime:      bm.LastErrorTime,
			LastError:          bm.LastError,
		}
	}

	// Copy error counts
	for errType, count := range m.errorCounts {
		snapshot.Errors[errType] = count
	}

	// Calculate rates
	seconds := snapshot.Uptime.Seconds()
	if seconds > 0 {
		snapshot.RequestRate = float64(snapshot.RequestsTotal) / seconds
		snapshot.ErrorRate = float64(snapshot.RequestsFailed) / seconds
	}

	// Calculate cache hit rate
	totalCache := snapshot.CacheHits + snapshot.CacheMisses
	if totalCache > 0 {
		snapshot.CacheHitRate = float64(snapshot.CacheHits) / float64(totalCache) * 100
	}

	return snapshot
}

// Reset resets all metrics.
func (m *BrokerMetrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	atomic.StoreInt64(&m.requestsTotal, 0)
	atomic.StoreInt64(&m.requestsSuccessful, 0)
	atomic.StoreInt64(&m.requestsFailed, 0)
	atomic.StoreInt64(&m.readRequests, 0)
	atomic.StoreInt64(&m.readDynamicRequests, 0)
	atomic.StoreInt64(&m.listRequests, 0)
	atomic.StoreInt64(&m.renewRequests, 0)
	atomic.StoreInt64(&m.revokeRequests, 0)
	atomic.StoreInt64(&m.cacheHits, 0)
	atomic.StoreInt64(&m.cacheMisses, 0)
	atomic.StoreInt64(&m.rateLimitRejections, 0)
	atomic.StoreInt64(&m.rotationsStarted, 0)
	atomic.StoreInt64(&m.rotationsCompleted, 0)
	atomic.StoreInt64(&m.rotationsFailed, 0)

	m.latencyBuckets = make(map[string]*latencyBucket)
	m.backendMetrics = make(map[string]*BackendMetrics)
	m.errorCounts = make(map[string]int64)
	m.startTime = time.Now()
}

func (m *BrokerMetrics) recordLatency(operation string, d time.Duration) {
	m.mu.Lock()
	bucket, ok := m.latencyBuckets[operation]
	if !ok {
		bucket = newLatencyBucket(1000) // Keep last 1000 samples
		m.latencyBuckets[operation] = bucket
	}
	m.mu.Unlock()

	bucket.record(float64(d.Milliseconds()))
}

func (m *BrokerMetrics) recordBackendLatency(backend string, d time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	bm, ok := m.backendMetrics[backend]
	if !ok {
		bm = &BackendMetrics{Name: backend}
		m.backendMetrics[backend] = bm
	}

	bm.RequestsTotal++
	if err == nil {
		bm.RequestsSuccessful++
		bm.LastSuccessTime = time.Now()
	} else {
		bm.RequestsFailed++
		bm.LastErrorTime = time.Now()
		bm.LastError = err.Error()
	}

	// Update latency (simple moving average)
	ms := float64(d.Milliseconds())
	if bm.AvgLatencyMs == 0 {
		bm.AvgLatencyMs = ms
	} else {
		bm.AvgLatencyMs = bm.AvgLatencyMs*0.9 + ms*0.1
	}
}

func (m *BrokerMetrics) recordError(err error) {
	errType := classifyError(err)

	m.mu.Lock()
	m.errorCounts[errType]++
	m.mu.Unlock()
}

func classifyError(err error) string {
	if err == nil {
		return "none"
	}

	switch {
	case errors.Is(err, ErrSecretNotFound):
		return "not_found"
	case errors.Is(err, ErrBackendNotFound):
		return "backend_not_found"
	case errors.Is(err, ErrBackendUnavailable):
		return "backend_unavailable"
	case errors.Is(err, ErrAccessDenied):
		return "access_denied"
	case errors.Is(err, ErrLeaseExpired):
		return "lease_expired"
	case IsRateLimitError(err):
		return "rate_limited"
	default:
		return "unknown"
	}
}

func newLatencyBucket(maxSamples int) *latencyBucket {
	return &latencyBucket{
		samples:    make([]float64, 0, maxSamples),
		maxSamples: maxSamples,
		min:        -1,
	}
}

func (b *latencyBucket) record(ms float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.count++
	b.sum += ms

	if b.min < 0 || ms < b.min {
		b.min = ms
	}
	if ms > b.max {
		b.max = ms
	}

	// Keep samples for percentile calculation
	if len(b.samples) >= b.maxSamples {
		// Remove oldest sample
		copy(b.samples, b.samples[1:])
		b.samples = b.samples[:len(b.samples)-1]
	}
	b.samples = append(b.samples, ms)
}

func (b *latencyBucket) stats() LatencyStats {
	b.mu.Lock()
	defer b.mu.Unlock()

	stats := LatencyStats{
		Count: b.count,
		Min:   b.min,
		Max:   b.max,
	}

	if b.count > 0 {
		stats.Avg = b.sum / float64(b.count)
	}

	// Calculate percentiles from samples
	if len(b.samples) > 0 {
		sorted := make([]float64, len(b.samples))
		copy(sorted, b.samples)
		sortFloat64s(sorted)

		stats.P50 = percentile(sorted, 0.50)
		stats.P90 = percentile(sorted, 0.90)
		stats.P95 = percentile(sorted, 0.95)
		stats.P99 = percentile(sorted, 0.99)
	}

	return stats
}

func sortFloat64s(s []float64) {
	// Simple bubble sort for small samples (usually ~1000)
	for i := 0; i < len(s)-1; i++ {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

// MetricsSnapshot represents a point-in-time snapshot of broker metrics.
type MetricsSnapshot struct {
	Timestamp time.Time     `json:"timestamp"`
	Uptime    time.Duration `json:"uptime"`

	// Request counts
	RequestsTotal      int64 `json:"requests_total"`
	RequestsSuccessful int64 `json:"requests_successful"`
	RequestsFailed     int64 `json:"requests_failed"`

	// Request types
	ReadRequests        int64 `json:"read_requests"`
	ReadDynamicRequests int64 `json:"read_dynamic_requests"`
	ListRequests        int64 `json:"list_requests"`
	RenewRequests       int64 `json:"renew_requests"`
	RevokeRequests      int64 `json:"revoke_requests"`

	// Cache metrics
	CacheHits    int64   `json:"cache_hits"`
	CacheMisses  int64   `json:"cache_misses"`
	CacheHitRate float64 `json:"cache_hit_rate"`

	// Rate limiting
	RateLimitRejections int64 `json:"rate_limit_rejections"`

	// Rotation metrics
	RotationsStarted   int64 `json:"rotations_started"`
	RotationsCompleted int64 `json:"rotations_completed"`
	RotationsFailed    int64 `json:"rotations_failed"`

	// Rates
	RequestRate float64 `json:"request_rate_per_sec"`
	ErrorRate   float64 `json:"error_rate_per_sec"`

	// Latency stats by operation
	Latencies map[string]LatencyStats `json:"latencies"`

	// Backend metrics
	Backends map[string]*BackendMetrics `json:"backends"`

	// Error counts by type
	Errors map[string]int64 `json:"errors"`
}

// LatencyStats contains latency statistics.
type LatencyStats struct {
	Count int64   `json:"count"`
	Avg   float64 `json:"avg_ms"`
	Min   float64 `json:"min_ms"`
	Max   float64 `json:"max_ms"`
	P50   float64 `json:"p50_ms"`
	P90   float64 `json:"p90_ms"`
	P95   float64 `json:"p95_ms"`
	P99   float64 `json:"p99_ms"`
}

// =============================================================================
// Prometheus Metrics Exporter
// =============================================================================

// PrometheusExporter exports broker metrics in Prometheus format.
type PrometheusExporter struct {
	metrics *BrokerMetrics
	prefix  string
}

// NewPrometheusExporter creates a new Prometheus exporter.
func NewPrometheusExporter(metrics *BrokerMetrics, prefix string) *PrometheusExporter {
	if prefix == "" {
		prefix = "keystone_secrets"
	}
	return &PrometheusExporter{
		metrics: metrics,
		prefix:  prefix,
	}
}

// Export exports metrics in Prometheus text format.
func (e *PrometheusExporter) Export() string {
	snapshot := e.metrics.Snapshot()
	var out string

	// Request metrics
	out += e.formatCounter("requests_total", "Total secret requests", snapshot.RequestsTotal)
	out += e.formatCounter("requests_successful_total", "Successful secret requests", snapshot.RequestsSuccessful)
	out += e.formatCounter("requests_failed_total", "Failed secret requests", snapshot.RequestsFailed)

	// Request types
	out += e.formatCounter("read_requests_total", "Read requests", snapshot.ReadRequests)
	out += e.formatCounter("read_dynamic_requests_total", "Dynamic read requests", snapshot.ReadDynamicRequests)
	out += e.formatCounter("list_requests_total", "List requests", snapshot.ListRequests)
	out += e.formatCounter("renew_requests_total", "Lease renew requests", snapshot.RenewRequests)
	out += e.formatCounter("revoke_requests_total", "Lease revoke requests", snapshot.RevokeRequests)

	// Cache metrics
	out += e.formatCounter("cache_hits_total", "Cache hits", snapshot.CacheHits)
	out += e.formatCounter("cache_misses_total", "Cache misses", snapshot.CacheMisses)
	out += e.formatGauge("cache_hit_rate", "Cache hit rate percentage", snapshot.CacheHitRate)

	// Rate limiting
	out += e.formatCounter("rate_limit_rejections_total", "Rate limit rejections", snapshot.RateLimitRejections)

	// Rotation metrics
	out += e.formatCounter("rotations_started_total", "Rotations started", snapshot.RotationsStarted)
	out += e.formatCounter("rotations_completed_total", "Rotations completed", snapshot.RotationsCompleted)
	out += e.formatCounter("rotations_failed_total", "Rotations failed", snapshot.RotationsFailed)

	// Rates
	out += e.formatGauge("request_rate", "Requests per second", snapshot.RequestRate)
	out += e.formatGauge("error_rate", "Errors per second", snapshot.ErrorRate)

	// Latency metrics
	for op, stats := range snapshot.Latencies {
		out += e.formatGaugeLabels("request_latency_avg_ms", "Average request latency in ms", stats.Avg, "operation", op)
		out += e.formatGaugeLabels("request_latency_p50_ms", "P50 request latency in ms", stats.P50, "operation", op)
		out += e.formatGaugeLabels("request_latency_p95_ms", "P95 request latency in ms", stats.P95, "operation", op)
		out += e.formatGaugeLabels("request_latency_p99_ms", "P99 request latency in ms", stats.P99, "operation", op)
	}

	// Backend metrics
	for name, bm := range snapshot.Backends {
		out += e.formatGaugeLabels("backend_requests_total", "Backend requests", float64(bm.RequestsTotal), "backend", name)
		out += e.formatGaugeLabels("backend_requests_failed_total", "Backend failed requests", float64(bm.RequestsFailed), "backend", name)
		out += e.formatGaugeLabels("backend_latency_avg_ms", "Backend average latency", bm.AvgLatencyMs, "backend", name)
	}

	// Error counts
	for errType, count := range snapshot.Errors {
		out += e.formatGaugeLabels("errors_total", "Errors by type", float64(count), "type", errType)
	}

	return out
}

func (e *PrometheusExporter) formatCounter(name, help string, value int64) string {
	return fmt.Sprintf("# HELP %s_%s %s\n# TYPE %s_%s counter\n%s_%s %d\n",
		e.prefix, name, help, e.prefix, name, e.prefix, name, value)
}

func (e *PrometheusExporter) formatGauge(name, help string, value float64) string {
	return fmt.Sprintf("# HELP %s_%s %s\n# TYPE %s_%s gauge\n%s_%s %.6f\n",
		e.prefix, name, help, e.prefix, name, e.prefix, name, value)
}

func (e *PrometheusExporter) formatGaugeLabels(name, help string, value float64, labelName, labelValue string) string {
	return fmt.Sprintf("# HELP %s_%s %s\n# TYPE %s_%s gauge\n%s_%s{%s=%q} %.6f\n",
		e.prefix, name, help, e.prefix, name, e.prefix, name, labelName, labelValue, value)
}

// =============================================================================
// Metrics-Enabled Broker Wrapper
// =============================================================================

// MetricsBroker wraps a SecretBroker with metrics collection.
type MetricsBroker struct {
	broker  *SecretBroker
	metrics *BrokerMetrics
}

// NewMetricsBroker creates a new metrics-enabled broker.
func NewMetricsBroker(broker *SecretBroker) *MetricsBroker {
	return &MetricsBroker{
		broker:  broker,
		metrics: NewBrokerMetrics(),
	}
}

// Read reads a secret with metrics collection.
func (m *MetricsBroker) Read(ctx context.Context, req *SecretRequest) (*Secret, error) {
	start := time.Now()
	secret, err := m.broker.Read(ctx, req)
	m.metrics.RecordRequest("read", "", time.Since(start), err)
	return secret, err
}

// ReadDynamic reads a dynamic secret with metrics collection.
func (m *MetricsBroker) ReadDynamic(ctx context.Context, req *SecretRequest) (*Secret, error) {
	start := time.Now()
	secret, err := m.broker.ReadDynamic(ctx, req)
	m.metrics.RecordRequest("read_dynamic", "", time.Since(start), err)
	return secret, err
}

// List lists secrets with metrics collection.
func (m *MetricsBroker) List(ctx context.Context, path string) ([]string, error) {
	start := time.Now()
	list, err := m.broker.List(ctx, path)
	m.metrics.RecordRequest("list", "", time.Since(start), err)
	return list, err
}

// RenewLease renews a lease with metrics collection.
func (m *MetricsBroker) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*Lease, error) {
	start := time.Now()
	lease, err := m.broker.RenewLease(ctx, leaseID, increment)
	m.metrics.RecordRequest("renew", "", time.Since(start), err)
	return lease, err
}

// RevokeLease revokes a lease with metrics collection.
func (m *MetricsBroker) RevokeLease(ctx context.Context, leaseID string) error {
	start := time.Now()
	err := m.broker.RevokeLease(ctx, leaseID)
	m.metrics.RecordRequest("revoke", "", time.Since(start), err)
	return err
}

// Metrics returns the metrics collector.
func (m *MetricsBroker) Metrics() *BrokerMetrics {
	return m.metrics
}

// Broker returns the underlying broker.
func (m *MetricsBroker) Broker() *SecretBroker {
	return m.broker
}

// MetricsSnapshot returns a snapshot of the current metrics.
func (m *MetricsBroker) MetricsSnapshot() *MetricsSnapshot {
	return m.metrics.Snapshot()
}
