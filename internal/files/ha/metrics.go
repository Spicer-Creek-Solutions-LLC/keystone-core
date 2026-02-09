// Package ha provides high availability and scaling for the file distribution system.
package ha

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector collects metrics for the file distribution system.
type MetricsCollector struct {
	// Transfer metrics
	transfersTotal   int64
	transfersActive  int64
	transfersFailed  int64
	bytesTransferred int64
	bytesUploaded    int64
	bytesDownloaded  int64

	// Cache metrics
	cacheHits      int64
	cacheMisses    int64
	cacheSize      int64
	cacheEntries   int64
	cacheEvictions int64

	// Backend metrics
	backendRequests     map[string]int64
	backendErrors       map[string]int64
	backendLatencySum   map[string]int64
	backendLatencyCount map[string]int64
	backendMu           sync.RWMutex

	// Rate limiting metrics
	rateLimitedRequests int64

	// Queue metrics
	queuedTransfers int64

	// Latency histograms (simplified - just tracking count per bucket)
	transferLatencyBuckets map[string]int64
	latencyMu              sync.Mutex

	// Labels
	instanceID string
	hostname   string
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector(instanceID, hostname string) *MetricsCollector {
	return &MetricsCollector{
		instanceID:             instanceID,
		hostname:               hostname,
		backendRequests:        make(map[string]int64),
		backendErrors:          make(map[string]int64),
		backendLatencySum:      make(map[string]int64),
		backendLatencyCount:    make(map[string]int64),
		transferLatencyBuckets: make(map[string]int64),
	}
}

// RecordTransfer records a completed transfer.
func (mc *MetricsCollector) RecordTransfer(bytes int64, isUpload bool, duration time.Duration) {
	atomic.AddInt64(&mc.transfersTotal, 1)
	atomic.AddInt64(&mc.bytesTransferred, bytes)

	if isUpload {
		atomic.AddInt64(&mc.bytesUploaded, bytes)
	} else {
		atomic.AddInt64(&mc.bytesDownloaded, bytes)
	}

	// Record latency bucket.
	bucket := mc.latencyBucket(duration)
	mc.latencyMu.Lock()
	mc.transferLatencyBuckets[bucket]++
	mc.latencyMu.Unlock()
}

// RecordTransferError records a failed transfer.
func (mc *MetricsCollector) RecordTransferError() {
	atomic.AddInt64(&mc.transfersFailed, 1)
}

// IncrementActiveTransfers increments active transfer count.
func (mc *MetricsCollector) IncrementActiveTransfers() {
	atomic.AddInt64(&mc.transfersActive, 1)
}

// DecrementActiveTransfers decrements active transfer count.
func (mc *MetricsCollector) DecrementActiveTransfers() {
	atomic.AddInt64(&mc.transfersActive, -1)
}

// RecordCacheHit records a cache hit.
func (mc *MetricsCollector) RecordCacheHit() {
	atomic.AddInt64(&mc.cacheHits, 1)
}

// RecordCacheMiss records a cache miss.
func (mc *MetricsCollector) RecordCacheMiss() {
	atomic.AddInt64(&mc.cacheMisses, 1)
}

// SetCacheSize sets the current cache size.
func (mc *MetricsCollector) SetCacheSize(size int64) {
	atomic.StoreInt64(&mc.cacheSize, size)
}

// SetCacheEntries sets the current number of cache entries.
func (mc *MetricsCollector) SetCacheEntries(count int64) {
	atomic.StoreInt64(&mc.cacheEntries, count)
}

// RecordCacheEviction records a cache eviction.
func (mc *MetricsCollector) RecordCacheEviction() {
	atomic.AddInt64(&mc.cacheEvictions, 1)
}

// RecordBackendRequest records a backend request.
func (mc *MetricsCollector) RecordBackendRequest(backend string, duration time.Duration, err error) {
	mc.backendMu.Lock()
	defer mc.backendMu.Unlock()

	mc.backendRequests[backend]++
	mc.backendLatencySum[backend] += duration.Nanoseconds()
	mc.backendLatencyCount[backend]++

	if err != nil {
		mc.backendErrors[backend]++
	}
}

// RecordRateLimited records a rate-limited request.
func (mc *MetricsCollector) RecordRateLimited() {
	atomic.AddInt64(&mc.rateLimitedRequests, 1)
}

// SetQueuedTransfers sets the current number of queued transfers.
func (mc *MetricsCollector) SetQueuedTransfers(count int64) {
	atomic.StoreInt64(&mc.queuedTransfers, count)
}

// latencyBucket returns the bucket name for a latency value.
func (mc *MetricsCollector) latencyBucket(d time.Duration) string {
	ms := d.Milliseconds()
	switch {
	case ms <= 10:
		return "le_10ms"
	case ms <= 50:
		return "le_50ms"
	case ms <= 100:
		return "le_100ms"
	case ms <= 500:
		return "le_500ms"
	case ms <= 1000:
		return "le_1s"
	case ms <= 5000:
		return "le_5s"
	case ms <= 10000:
		return "le_10s"
	default:
		return "le_inf"
	}
}

// GetMetrics returns all metrics as a map.
func (mc *MetricsCollector) GetMetrics() map[string]interface{} {
	mc.backendMu.RLock()
	backends := make(map[string]map[string]int64)
	for backend, requests := range mc.backendRequests {
		backends[backend] = map[string]int64{
			"requests": requests,
			"errors":   mc.backendErrors[backend],
		}
		if count := mc.backendLatencyCount[backend]; count > 0 {
			backends[backend]["latency_avg_ns"] = mc.backendLatencySum[backend] / count
		}
	}
	mc.backendMu.RUnlock()

	mc.latencyMu.Lock()
	latencyBuckets := make(map[string]int64)
	for k, v := range mc.transferLatencyBuckets {
		latencyBuckets[k] = v
	}
	mc.latencyMu.Unlock()

	return map[string]interface{}{
		"instance_id": mc.instanceID,
		"hostname":    mc.hostname,
		"transfers": map[string]int64{
			"total":      atomic.LoadInt64(&mc.transfersTotal),
			"active":     atomic.LoadInt64(&mc.transfersActive),
			"failed":     atomic.LoadInt64(&mc.transfersFailed),
			"bytes":      atomic.LoadInt64(&mc.bytesTransferred),
			"uploaded":   atomic.LoadInt64(&mc.bytesUploaded),
			"downloaded": atomic.LoadInt64(&mc.bytesDownloaded),
		},
		"cache": map[string]int64{
			"hits":      atomic.LoadInt64(&mc.cacheHits),
			"misses":    atomic.LoadInt64(&mc.cacheMisses),
			"size":      atomic.LoadInt64(&mc.cacheSize),
			"entries":   atomic.LoadInt64(&mc.cacheEntries),
			"evictions": atomic.LoadInt64(&mc.cacheEvictions),
		},
		"backends":        backends,
		"rate_limited":    atomic.LoadInt64(&mc.rateLimitedRequests),
		"queued":          atomic.LoadInt64(&mc.queuedTransfers),
		"latency_buckets": latencyBuckets,
	}
}

// PrometheusExporter exports metrics in Prometheus format.
type PrometheusExporter struct {
	collector *MetricsCollector
	prefix    string
}

// NewPrometheusExporter creates a new Prometheus exporter.
func NewPrometheusExporter(collector *MetricsCollector, prefix string) *PrometheusExporter {
	if prefix == "" {
		prefix = "kscore_files"
	}
	return &PrometheusExporter{
		collector: collector,
		prefix:    prefix,
	}
}

// Export exports metrics in Prometheus text format.
func (pe *PrometheusExporter) Export() string {
	var sb strings.Builder

	mc := pe.collector
	labels := fmt.Sprintf(`instance=%q,hostname=%q`, mc.instanceID, mc.hostname)

	// Transfer metrics
	sb.WriteString(fmt.Sprintf("# HELP %s_transfers_total Total number of file transfers\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_transfers_total counter\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_transfers_total{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.transfersTotal)))

	sb.WriteString(fmt.Sprintf("# HELP %s_transfers_active Current number of active transfers\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_transfers_active gauge\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_transfers_active{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.transfersActive)))

	sb.WriteString(fmt.Sprintf("# HELP %s_transfers_failed_total Total number of failed transfers\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_transfers_failed_total counter\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_transfers_failed_total{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.transfersFailed)))

	sb.WriteString(fmt.Sprintf("# HELP %s_bytes_transferred_total Total bytes transferred\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_bytes_transferred_total counter\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_bytes_transferred_total{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.bytesTransferred)))

	sb.WriteString(fmt.Sprintf("# HELP %s_bytes_uploaded_total Total bytes uploaded\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_bytes_uploaded_total counter\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_bytes_uploaded_total{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.bytesUploaded)))

	sb.WriteString(fmt.Sprintf("# HELP %s_bytes_downloaded_total Total bytes downloaded\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_bytes_downloaded_total counter\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_bytes_downloaded_total{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.bytesDownloaded)))

	// Cache metrics
	sb.WriteString(fmt.Sprintf("# HELP %s_cache_hits_total Total cache hits\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_cache_hits_total counter\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_cache_hits_total{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.cacheHits)))

	sb.WriteString(fmt.Sprintf("# HELP %s_cache_misses_total Total cache misses\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_cache_misses_total counter\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_cache_misses_total{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.cacheMisses)))

	sb.WriteString(fmt.Sprintf("# HELP %s_cache_size_bytes Current cache size in bytes\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_cache_size_bytes gauge\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_cache_size_bytes{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.cacheSize)))

	sb.WriteString(fmt.Sprintf("# HELP %s_cache_entries Current number of cache entries\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_cache_entries gauge\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_cache_entries{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.cacheEntries)))

	sb.WriteString(fmt.Sprintf("# HELP %s_cache_evictions_total Total cache evictions\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_cache_evictions_total counter\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_cache_evictions_total{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.cacheEvictions)))

	// Rate limiting metrics
	sb.WriteString(fmt.Sprintf("# HELP %s_rate_limited_total Total rate-limited requests\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_rate_limited_total counter\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_rate_limited_total{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.rateLimitedRequests)))

	// Queue metrics
	sb.WriteString(fmt.Sprintf("# HELP %s_queued_transfers Current queued transfers\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_queued_transfers gauge\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("%s_queued_transfers{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.queuedTransfers)))

	// Backend metrics
	mc.backendMu.RLock()
	sb.WriteString(fmt.Sprintf("# HELP %s_backend_requests_total Total backend requests\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_backend_requests_total counter\n", pe.prefix))
	for backend, requests := range mc.backendRequests {
		sb.WriteString(fmt.Sprintf("%s_backend_requests_total{%s,backend=%q} %d\n", pe.prefix, labels, backend, requests))
	}

	sb.WriteString(fmt.Sprintf("# HELP %s_backend_errors_total Total backend errors\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_backend_errors_total counter\n", pe.prefix))
	for backend, errors := range mc.backendErrors {
		sb.WriteString(fmt.Sprintf("%s_backend_errors_total{%s,backend=%q} %d\n", pe.prefix, labels, backend, errors))
	}

	sb.WriteString(fmt.Sprintf("# HELP %s_backend_latency_avg_seconds Average backend latency\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_backend_latency_avg_seconds gauge\n", pe.prefix))
	for backend, sum := range mc.backendLatencySum {
		count := mc.backendLatencyCount[backend]
		if count > 0 {
			avgSeconds := float64(sum) / float64(count) / 1e9
			sb.WriteString(fmt.Sprintf("%s_backend_latency_avg_seconds{%s,backend=%q} %.6f\n", pe.prefix, labels, backend, avgSeconds))
		}
	}
	mc.backendMu.RUnlock()

	// Transfer latency histogram
	mc.latencyMu.Lock()
	sb.WriteString(fmt.Sprintf("# HELP %s_transfer_latency_seconds Transfer latency distribution\n", pe.prefix))
	sb.WriteString(fmt.Sprintf("# TYPE %s_transfer_latency_seconds histogram\n", pe.prefix))
	var cumulative int64
	buckets := []struct {
		key   string
		bound string
	}{
		{"le_10ms", "0.01"},
		{"le_50ms", "0.05"},
		{"le_100ms", "0.1"},
		{"le_500ms", "0.5"},
		{"le_1s", "1"},
		{"le_5s", "5"},
		{"le_10s", "10"},
		{"le_inf", "+Inf"},
	}
	for _, b := range buckets {
		cumulative += mc.transferLatencyBuckets[b.key]
		sb.WriteString(fmt.Sprintf("%s_transfer_latency_seconds_bucket{%s,le=%q} %d\n", pe.prefix, labels, b.bound, cumulative))
	}
	sb.WriteString(fmt.Sprintf("%s_transfer_latency_seconds_count{%s} %d\n", pe.prefix, labels, atomic.LoadInt64(&mc.transfersTotal)))
	mc.latencyMu.Unlock()

	return sb.String()
}

// CacheHitRate returns the cache hit rate (0.0 to 1.0).
func (mc *MetricsCollector) CacheHitRate() float64 {
	hits := atomic.LoadInt64(&mc.cacheHits)
	misses := atomic.LoadInt64(&mc.cacheMisses)
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// TransferErrorRate returns the transfer error rate (0.0 to 1.0).
func (mc *MetricsCollector) TransferErrorRate() float64 {
	total := atomic.LoadInt64(&mc.transfersTotal)
	failed := atomic.LoadInt64(&mc.transfersFailed)
	if total == 0 {
		return 0
	}
	return float64(failed) / float64(total)
}
