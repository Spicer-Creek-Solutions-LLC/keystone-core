package ha

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMetricsCollector_NewMetricsCollector(t *testing.T) {
	mc := NewMetricsCollector("instance-1", "host1.example.com")

	if mc.instanceID != "instance-1" {
		t.Errorf("expected instanceID 'instance-1', got '%s'", mc.instanceID)
	}

	if mc.hostname != "host1.example.com" {
		t.Errorf("expected hostname 'host1.example.com', got '%s'", mc.hostname)
	}
}

func TestMetricsCollector_RecordTransfer(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	// Record a download.
	mc.RecordTransfer(1024, false, 100*time.Millisecond)

	if mc.transfersTotal != 1 {
		t.Errorf("expected 1 total transfer, got %d", mc.transfersTotal)
	}

	if mc.bytesTransferred != 1024 {
		t.Errorf("expected 1024 bytes transferred, got %d", mc.bytesTransferred)
	}

	if mc.bytesDownloaded != 1024 {
		t.Errorf("expected 1024 bytes downloaded, got %d", mc.bytesDownloaded)
	}

	if mc.bytesUploaded != 0 {
		t.Errorf("expected 0 bytes uploaded, got %d", mc.bytesUploaded)
	}

	// Record an upload.
	mc.RecordTransfer(2048, true, 50*time.Millisecond)

	if mc.transfersTotal != 2 {
		t.Errorf("expected 2 total transfers, got %d", mc.transfersTotal)
	}

	if mc.bytesUploaded != 2048 {
		t.Errorf("expected 2048 bytes uploaded, got %d", mc.bytesUploaded)
	}
}

func TestMetricsCollector_RecordTransferError(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	mc.RecordTransferError()
	mc.RecordTransferError()

	if mc.transfersFailed != 2 {
		t.Errorf("expected 2 failed transfers, got %d", mc.transfersFailed)
	}
}

func TestMetricsCollector_ActiveTransfers(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	mc.IncrementActiveTransfers()
	mc.IncrementActiveTransfers()

	if mc.transfersActive != 2 {
		t.Errorf("expected 2 active transfers, got %d", mc.transfersActive)
	}

	mc.DecrementActiveTransfers()

	if mc.transfersActive != 1 {
		t.Errorf("expected 1 active transfer, got %d", mc.transfersActive)
	}
}

func TestMetricsCollector_CacheMetrics(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	mc.RecordCacheHit()
	mc.RecordCacheHit()
	mc.RecordCacheMiss()

	if mc.cacheHits != 2 {
		t.Errorf("expected 2 cache hits, got %d", mc.cacheHits)
	}

	if mc.cacheMisses != 1 {
		t.Errorf("expected 1 cache miss, got %d", mc.cacheMisses)
	}

	mc.SetCacheSize(1024 * 1024)
	if mc.cacheSize != 1024*1024 {
		t.Errorf("expected cache size %d, got %d", 1024*1024, mc.cacheSize)
	}

	mc.SetCacheEntries(100)
	if mc.cacheEntries != 100 {
		t.Errorf("expected 100 cache entries, got %d", mc.cacheEntries)
	}

	mc.RecordCacheEviction()
	if mc.cacheEvictions != 1 {
		t.Errorf("expected 1 cache eviction, got %d", mc.cacheEvictions)
	}
}

func TestMetricsCollector_BackendMetrics(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	// Record successful request.
	mc.RecordBackendRequest("s3", 100*time.Millisecond, nil)

	// Record failed request.
	mc.RecordBackendRequest("s3", 200*time.Millisecond, errors.New("timeout"))

	if mc.backendRequests["s3"] != 2 {
		t.Errorf("expected 2 s3 requests, got %d", mc.backendRequests["s3"])
	}

	if mc.backendErrors["s3"] != 1 {
		t.Errorf("expected 1 s3 error, got %d", mc.backendErrors["s3"])
	}

	// Check average latency.
	count := mc.backendLatencyCount["s3"]
	sum := mc.backendLatencySum["s3"]
	if count != 2 {
		t.Errorf("expected 2 latency samples, got %d", count)
	}

	expectedSum := (100 + 200) * int64(time.Millisecond)
	if sum != expectedSum {
		t.Errorf("expected latency sum %d, got %d", expectedSum, sum)
	}
}

func TestMetricsCollector_RateLimiting(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	mc.RecordRateLimited()
	mc.RecordRateLimited()

	if mc.rateLimitedRequests != 2 {
		t.Errorf("expected 2 rate limited requests, got %d", mc.rateLimitedRequests)
	}
}

func TestMetricsCollector_QueueMetrics(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	mc.SetQueuedTransfers(10)

	if mc.queuedTransfers != 10 {
		t.Errorf("expected 10 queued transfers, got %d", mc.queuedTransfers)
	}
}

func TestMetricsCollector_LatencyBuckets(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	tests := []struct {
		duration       time.Duration
		expectedBucket string
	}{
		{5 * time.Millisecond, "le_10ms"},
		{10 * time.Millisecond, "le_10ms"},
		{30 * time.Millisecond, "le_50ms"},
		{75 * time.Millisecond, "le_100ms"},
		{300 * time.Millisecond, "le_500ms"},
		{750 * time.Millisecond, "le_1s"},
		{3 * time.Second, "le_5s"},
		{7 * time.Second, "le_10s"},
		{15 * time.Second, "le_inf"},
	}

	for _, tt := range tests {
		bucket := mc.latencyBucket(tt.duration)
		if bucket != tt.expectedBucket {
			t.Errorf("latencyBucket(%v) = %s, want %s", tt.duration, bucket, tt.expectedBucket)
		}
	}
}

func TestMetricsCollector_GetMetrics(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	// Record some metrics.
	mc.RecordTransfer(1024, false, 50*time.Millisecond)
	mc.RecordCacheHit()
	mc.RecordBackendRequest("s3", 100*time.Millisecond, nil)

	metrics := mc.GetMetrics()

	if metrics["instance_id"] != "test" {
		t.Errorf("expected instance_id 'test', got '%s'", metrics["instance_id"])
	}

	transfers := metrics["transfers"].(map[string]int64)
	if transfers["total"] != 1 {
		t.Errorf("expected 1 total transfer, got %d", transfers["total"])
	}

	cache := metrics["cache"].(map[string]int64)
	if cache["hits"] != 1 {
		t.Errorf("expected 1 cache hit, got %d", cache["hits"])
	}

	backends := metrics["backends"].(map[string]map[string]int64)
	if backends["s3"]["requests"] != 1 {
		t.Errorf("expected 1 s3 request, got %d", backends["s3"]["requests"])
	}
}

func TestMetricsCollector_CacheHitRate(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	// No accesses yet.
	if mc.CacheHitRate() != 0 {
		t.Errorf("expected cache hit rate 0 with no accesses, got %f", mc.CacheHitRate())
	}

	// 2 hits, 2 misses = 50% hit rate.
	mc.RecordCacheHit()
	mc.RecordCacheHit()
	mc.RecordCacheMiss()
	mc.RecordCacheMiss()

	rate := mc.CacheHitRate()
	if rate != 0.5 {
		t.Errorf("expected cache hit rate 0.5, got %f", rate)
	}
}

func TestMetricsCollector_TransferErrorRate(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	// No transfers yet.
	if mc.TransferErrorRate() != 0 {
		t.Errorf("expected transfer error rate 0 with no transfers, got %f", mc.TransferErrorRate())
	}

	// 4 transfers, 1 failed = 25% error rate.
	mc.RecordTransfer(1024, false, 50*time.Millisecond)
	mc.RecordTransfer(1024, false, 50*time.Millisecond)
	mc.RecordTransfer(1024, false, 50*time.Millisecond)
	mc.RecordTransfer(1024, false, 50*time.Millisecond)
	mc.RecordTransferError()

	rate := mc.TransferErrorRate()
	if rate != 0.25 {
		t.Errorf("expected transfer error rate 0.25, got %f", rate)
	}
}

func TestPrometheusExporter_NewPrometheusExporter(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")
	pe := NewPrometheusExporter(mc, "")

	if pe.prefix != "kscore_files" {
		t.Errorf("expected default prefix 'kscore_files', got '%s'", pe.prefix)
	}

	pe2 := NewPrometheusExporter(mc, "custom_prefix")
	if pe2.prefix != "custom_prefix" {
		t.Errorf("expected prefix 'custom_prefix', got '%s'", pe2.prefix)
	}
}

func TestPrometheusExporter_Export(t *testing.T) {
	mc := NewMetricsCollector("test-instance", "test-host")

	// Record some metrics.
	mc.RecordTransfer(1024, false, 50*time.Millisecond)
	mc.RecordTransferError()
	mc.IncrementActiveTransfers()
	mc.RecordCacheHit()
	mc.RecordCacheMiss()
	mc.SetCacheSize(1024)
	mc.SetCacheEntries(10)
	mc.RecordCacheEviction()
	mc.RecordBackendRequest("s3", 100*time.Millisecond, nil)
	mc.RecordRateLimited()
	mc.SetQueuedTransfers(5)

	pe := NewPrometheusExporter(mc, "kscore_files")
	output := pe.Export()

	// Check that all expected metrics are present.
	expectedMetrics := []string{
		"kscore_files_transfers_total",
		"kscore_files_transfers_active",
		"kscore_files_transfers_failed_total",
		"kscore_files_bytes_transferred_total",
		"kscore_files_bytes_uploaded_total",
		"kscore_files_bytes_downloaded_total",
		"kscore_files_cache_hits_total",
		"kscore_files_cache_misses_total",
		"kscore_files_cache_size_bytes",
		"kscore_files_cache_entries",
		"kscore_files_cache_evictions_total",
		"kscore_files_rate_limited_total",
		"kscore_files_queued_transfers",
		"kscore_files_backend_requests_total",
		"kscore_files_transfer_latency_seconds_bucket",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(output, metric) {
			t.Errorf("expected metric '%s' in output", metric)
		}
	}

	// Check labels are present.
	if !strings.Contains(output, `instance="test-instance"`) {
		t.Error("expected instance label in output")
	}

	if !strings.Contains(output, `hostname="test-host"`) {
		t.Error("expected hostname label in output")
	}

	// Check HELP and TYPE comments.
	if !strings.Contains(output, "# HELP") {
		t.Error("expected HELP comments in output")
	}

	if !strings.Contains(output, "# TYPE") {
		t.Error("expected TYPE comments in output")
	}
}

func TestPrometheusExporter_ExportHistogram(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	// Record transfers with different latencies.
	mc.RecordTransfer(1024, false, 5*time.Millisecond)   // le_10ms
	mc.RecordTransfer(1024, false, 30*time.Millisecond)  // le_50ms
	mc.RecordTransfer(1024, false, 300*time.Millisecond) // le_500ms

	pe := NewPrometheusExporter(mc, "kscore_files")
	output := pe.Export()

	// Check histogram buckets.
	expectedBuckets := []string{
		`le="0.01"`,
		`le="0.05"`,
		`le="0.1"`,
		`le="0.5"`,
		`le="1"`,
		`le="5"`,
		`le="10"`,
		`le="+Inf"`,
	}

	for _, bucket := range expectedBuckets {
		if !strings.Contains(output, bucket) {
			t.Errorf("expected bucket '%s' in output", bucket)
		}
	}
}

func TestPrometheusExporter_MultipleBackends(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	mc.RecordBackendRequest("s3", 100*time.Millisecond, nil)
	mc.RecordBackendRequest("gcs", 150*time.Millisecond, nil)
	mc.RecordBackendRequest("local", 10*time.Millisecond, nil)

	pe := NewPrometheusExporter(mc, "kscore_files")
	output := pe.Export()

	// Check all backends are present.
	if !strings.Contains(output, `backend="s3"`) {
		t.Error("expected s3 backend in output")
	}
	if !strings.Contains(output, `backend="gcs"`) {
		t.Error("expected gcs backend in output")
	}
	if !strings.Contains(output, `backend="local"`) {
		t.Error("expected local backend in output")
	}
}

func TestMetricsCollector_ConcurrentAccess(t *testing.T) {
	mc := NewMetricsCollector("test", "localhost")

	// Run multiple goroutines recording metrics concurrently.
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				mc.RecordTransfer(1024, j%2 == 0, time.Duration(j)*time.Millisecond)
				mc.RecordCacheHit()
				mc.RecordCacheMiss()
				mc.RecordBackendRequest("s3", 100*time.Millisecond, nil)
				mc.IncrementActiveTransfers()
				mc.DecrementActiveTransfers()
			}
			done <- true
		}()
	}

	// Wait for all goroutines.
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify counts.
	if mc.transfersTotal != 1000 {
		t.Errorf("expected 1000 total transfers, got %d", mc.transfersTotal)
	}

	if mc.cacheHits != 1000 {
		t.Errorf("expected 1000 cache hits, got %d", mc.cacheHits)
	}

	if mc.cacheMisses != 1000 {
		t.Errorf("expected 1000 cache misses, got %d", mc.cacheMisses)
	}

	if mc.backendRequests["s3"] != 1000 {
		t.Errorf("expected 1000 s3 requests, got %d", mc.backendRequests["s3"])
	}
}
