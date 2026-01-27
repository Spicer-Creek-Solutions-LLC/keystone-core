package dns

import (
	"fmt"
	"testing"
	"time"
)

func TestNewMetrics(t *testing.T) {
	m := NewMetrics()

	if m == nil {
		t.Fatal("NewMetrics() returned nil")
	}
	if m.latencies == nil {
		t.Error("latencies map not initialized")
	}
	if m.providerMetrics == nil {
		t.Error("providerMetrics map not initialized")
	}
	if m.maxLatencies != 1000 {
		t.Errorf("maxLatencies = %d, want 1000", m.maxLatencies)
	}
}

func TestMetrics_RecordGetRecords(t *testing.T) {
	m := NewMetrics()

	// Record successful call
	m.RecordGetRecords("cloudflare", 50*time.Millisecond, nil)

	if m.getRecordsCalls.Load() != 1 {
		t.Errorf("getRecordsCalls = %d, want 1", m.getRecordsCalls.Load())
	}
	if m.getRecordsSuccess.Load() != 1 {
		t.Errorf("getRecordsSuccess = %d, want 1", m.getRecordsSuccess.Load())
	}
	if m.getRecordsFailure.Load() != 0 {
		t.Errorf("getRecordsFailure = %d, want 0", m.getRecordsFailure.Load())
	}

	// Record failed call
	m.RecordGetRecords("cloudflare", 100*time.Millisecond, fmt.Errorf("api error"))

	if m.getRecordsCalls.Load() != 2 {
		t.Errorf("getRecordsCalls = %d, want 2", m.getRecordsCalls.Load())
	}
	if m.getRecordsFailure.Load() != 1 {
		t.Errorf("getRecordsFailure = %d, want 1", m.getRecordsFailure.Load())
	}
}

func TestMetrics_RecordCreateRecord(t *testing.T) {
	m := NewMetrics()

	// Record successful create
	m.RecordCreateRecord("route53", 30*time.Millisecond, nil)

	if m.createRecordCalls.Load() != 1 {
		t.Errorf("createRecordCalls = %d, want 1", m.createRecordCalls.Load())
	}
	if m.createRecordSuccess.Load() != 1 {
		t.Errorf("createRecordSuccess = %d, want 1", m.createRecordSuccess.Load())
	}
	if m.recordsCreated.Load() != 1 {
		t.Errorf("recordsCreated = %d, want 1", m.recordsCreated.Load())
	}

	// Record failed create
	m.RecordCreateRecord("route53", 50*time.Millisecond, fmt.Errorf("rate limited"))

	if m.createRecordCalls.Load() != 2 {
		t.Errorf("createRecordCalls = %d, want 2", m.createRecordCalls.Load())
	}
	if m.createRecordFailure.Load() != 1 {
		t.Errorf("createRecordFailure = %d, want 1", m.createRecordFailure.Load())
	}
	// recordsCreated should not increment on failure
	if m.recordsCreated.Load() != 1 {
		t.Errorf("recordsCreated = %d, want 1 (should not increment on failure)", m.recordsCreated.Load())
	}
}

func TestMetrics_RecordUpdateRecord(t *testing.T) {
	m := NewMetrics()

	m.RecordUpdateRecord("gcp", 40*time.Millisecond, nil)

	if m.updateRecordCalls.Load() != 1 {
		t.Errorf("updateRecordCalls = %d, want 1", m.updateRecordCalls.Load())
	}
	if m.updateRecordSuccess.Load() != 1 {
		t.Errorf("updateRecordSuccess = %d, want 1", m.updateRecordSuccess.Load())
	}
	if m.recordsUpdated.Load() != 1 {
		t.Errorf("recordsUpdated = %d, want 1", m.recordsUpdated.Load())
	}

	m.RecordUpdateRecord("gcp", 60*time.Millisecond, fmt.Errorf("not found"))

	if m.updateRecordFailure.Load() != 1 {
		t.Errorf("updateRecordFailure = %d, want 1", m.updateRecordFailure.Load())
	}
}

func TestMetrics_RecordDeleteRecord(t *testing.T) {
	m := NewMetrics()

	m.RecordDeleteRecord("azure", 25*time.Millisecond, nil)

	if m.deleteRecordCalls.Load() != 1 {
		t.Errorf("deleteRecordCalls = %d, want 1", m.deleteRecordCalls.Load())
	}
	if m.deleteRecordSuccess.Load() != 1 {
		t.Errorf("deleteRecordSuccess = %d, want 1", m.deleteRecordSuccess.Load())
	}
	if m.recordsDeleted.Load() != 1 {
		t.Errorf("recordsDeleted = %d, want 1", m.recordsDeleted.Load())
	}

	m.RecordDeleteRecord("azure", 35*time.Millisecond, fmt.Errorf("permission denied"))

	if m.deleteRecordFailure.Load() != 1 {
		t.Errorf("deleteRecordFailure = %d, want 1", m.deleteRecordFailure.Load())
	}
}

func TestMetrics_RecordSync(t *testing.T) {
	m := NewMetrics()

	// Successful sync with changes
	result := &SyncResult{
		Created: 3,
		Updated: 2,
		Deleted: 1,
		Errors:  nil,
	}
	m.RecordSync("cloudflare", 500*time.Millisecond, result)

	if m.syncCalls.Load() != 1 {
		t.Errorf("syncCalls = %d, want 1", m.syncCalls.Load())
	}
	if m.syncSuccess.Load() != 1 {
		t.Errorf("syncSuccess = %d, want 1", m.syncSuccess.Load())
	}
	if m.recordsCreated.Load() != 3 {
		t.Errorf("recordsCreated = %d, want 3", m.recordsCreated.Load())
	}
	if m.recordsUpdated.Load() != 2 {
		t.Errorf("recordsUpdated = %d, want 2", m.recordsUpdated.Load())
	}
	if m.recordsDeleted.Load() != 1 {
		t.Errorf("recordsDeleted = %d, want 1", m.recordsDeleted.Load())
	}

	// Sync with errors
	resultWithErrors := &SyncResult{
		Errors: []error{fmt.Errorf("error1")},
	}
	m.RecordSync("cloudflare", 200*time.Millisecond, resultWithErrors)

	if m.syncCalls.Load() != 2 {
		t.Errorf("syncCalls = %d, want 2", m.syncCalls.Load())
	}
	if m.syncFailure.Load() != 1 {
		t.Errorf("syncFailure = %d, want 1", m.syncFailure.Load())
	}

	// Sync with nil result
	m.RecordSync("cloudflare", 100*time.Millisecond, nil)
	if m.syncCalls.Load() != 3 {
		t.Errorf("syncCalls = %d, want 3", m.syncCalls.Load())
	}
	if m.syncSuccess.Load() != 2 {
		t.Errorf("syncSuccess = %d, want 2", m.syncSuccess.Load())
	}
}

func TestMetrics_ProviderMetrics(t *testing.T) {
	m := NewMetrics()

	// Record calls for multiple providers
	m.RecordGetRecords("cloudflare", 50*time.Millisecond, nil)
	m.RecordGetRecords("cloudflare", 60*time.Millisecond, nil)
	m.RecordGetRecords("route53", 100*time.Millisecond, fmt.Errorf("error"))

	snap := m.Snapshot()

	if len(snap.Providers) != 2 {
		t.Errorf("Providers count = %d, want 2", len(snap.Providers))
	}

	cf := snap.Providers["cloudflare"]
	if cf.Calls != 2 {
		t.Errorf("cloudflare calls = %d, want 2", cf.Calls)
	}
	if cf.Errors != 0 {
		t.Errorf("cloudflare errors = %d, want 0", cf.Errors)
	}
	// Average should be (50+60)/2 = 55ms
	if cf.AvgLatencyMs < 54 || cf.AvgLatencyMs > 56 {
		t.Errorf("cloudflare avgLatency = %f, want ~55", cf.AvgLatencyMs)
	}

	r53 := snap.Providers["route53"]
	if r53.Calls != 1 {
		t.Errorf("route53 calls = %d, want 1", r53.Calls)
	}
	if r53.Errors != 1 {
		t.Errorf("route53 errors = %d, want 1", r53.Errors)
	}
	if r53.ErrorRate != 1.0 {
		t.Errorf("route53 errorRate = %f, want 1.0", r53.ErrorRate)
	}
}

func TestMetrics_LatencyPercentiles(t *testing.T) {
	m := NewMetrics()

	// Record 100 samples with increasing latencies
	for i := 1; i <= 100; i++ {
		m.RecordGetRecords("test", time.Duration(i)*time.Millisecond, nil)
	}

	snap := m.Snapshot()

	p50 := snap.LatencyP50["get_records"]
	p95 := snap.LatencyP95["get_records"]
	p99 := snap.LatencyP99["get_records"]

	// P50 should be around 50ms
	if p50 < 49 || p50 > 51 {
		t.Errorf("P50 = %f, want ~50", p50)
	}

	// P95 should be around 95ms
	if p95 < 94 || p95 > 96 {
		t.Errorf("P95 = %f, want ~95", p95)
	}

	// P99 should be around 99ms
	if p99 < 98 || p99 > 100 {
		t.Errorf("P99 = %f, want ~99", p99)
	}
}

func TestMetrics_LatencySampleLimit(t *testing.T) {
	m := NewMetrics()
	m.maxLatencies = 10 // Small limit for testing

	// Record more samples than the limit
	for i := 0; i < 20; i++ {
		m.RecordGetRecords("test", time.Duration(i)*time.Millisecond, nil)
	}

	m.latenciesMu.RLock()
	samples := len(m.latencies["get_records"])
	m.latenciesMu.RUnlock()

	if samples != 10 {
		t.Errorf("samples = %d, want 10 (max)", samples)
	}
}

func TestMetrics_Snapshot(t *testing.T) {
	m := NewMetrics()

	// Record various operations
	m.RecordGetRecords("cloudflare", 50*time.Millisecond, nil)
	m.RecordCreateRecord("cloudflare", 30*time.Millisecond, nil)
	m.RecordUpdateRecord("cloudflare", 40*time.Millisecond, nil)
	m.RecordDeleteRecord("cloudflare", 20*time.Millisecond, nil)
	m.RecordSync("cloudflare", 200*time.Millisecond, &SyncResult{Created: 1})

	snap := m.Snapshot()

	if snap.GetRecordsCalls != 1 {
		t.Errorf("GetRecordsCalls = %d, want 1", snap.GetRecordsCalls)
	}
	if snap.CreateRecordCalls != 1 {
		t.Errorf("CreateRecordCalls = %d, want 1", snap.CreateRecordCalls)
	}
	if snap.UpdateRecordCalls != 1 {
		t.Errorf("UpdateRecordCalls = %d, want 1", snap.UpdateRecordCalls)
	}
	if snap.DeleteRecordCalls != 1 {
		t.Errorf("DeleteRecordCalls = %d, want 1", snap.DeleteRecordCalls)
	}
	if snap.SyncCalls != 1 {
		t.Errorf("SyncCalls = %d, want 1", snap.SyncCalls)
	}

	if snap.GetRecordsSuccess != 1 {
		t.Errorf("GetRecordsSuccess = %d, want 1", snap.GetRecordsSuccess)
	}
	if snap.CreateRecordSuccess != 1 {
		t.Errorf("CreateRecordSuccess = %d, want 1", snap.CreateRecordSuccess)
	}
	if snap.SyncSuccess != 1 {
		t.Errorf("SyncSuccess = %d, want 1", snap.SyncSuccess)
	}

	// Check record change counts
	if snap.RecordsCreated != 2 { // 1 from CreateRecord + 1 from Sync
		t.Errorf("RecordsCreated = %d, want 2", snap.RecordsCreated)
	}
	if snap.RecordsUpdated != 1 {
		t.Errorf("RecordsUpdated = %d, want 1", snap.RecordsUpdated)
	}
	if snap.RecordsDeleted != 1 {
		t.Errorf("RecordsDeleted = %d, want 1", snap.RecordsDeleted)
	}

	// Check maps are initialized
	if snap.LatencyP50 == nil {
		t.Error("LatencyP50 map not initialized")
	}
	if snap.Providers == nil {
		t.Error("Providers map not initialized")
	}
}

func TestMetrics_Reset(t *testing.T) {
	m := NewMetrics()

	// Record some operations
	m.RecordGetRecords("cloudflare", 50*time.Millisecond, nil)
	m.RecordCreateRecord("cloudflare", 30*time.Millisecond, nil)
	m.RecordSync("cloudflare", 200*time.Millisecond, &SyncResult{Created: 5})

	// Verify counters are non-zero
	if m.getRecordsCalls.Load() == 0 {
		t.Fatal("getRecordsCalls should be non-zero before reset")
	}

	// Reset
	m.Reset()

	// Verify all counters are zero
	if m.getRecordsCalls.Load() != 0 {
		t.Errorf("getRecordsCalls = %d after reset, want 0", m.getRecordsCalls.Load())
	}
	if m.createRecordCalls.Load() != 0 {
		t.Errorf("createRecordCalls = %d after reset, want 0", m.createRecordCalls.Load())
	}
	if m.syncCalls.Load() != 0 {
		t.Errorf("syncCalls = %d after reset, want 0", m.syncCalls.Load())
	}
	if m.recordsCreated.Load() != 0 {
		t.Errorf("recordsCreated = %d after reset, want 0", m.recordsCreated.Load())
	}

	// Verify maps are cleared
	m.latenciesMu.RLock()
	latencyCount := len(m.latencies)
	m.latenciesMu.RUnlock()
	if latencyCount != 0 {
		t.Errorf("latencies map has %d entries after reset, want 0", latencyCount)
	}

	m.mu.RLock()
	providerCount := len(m.providerMetrics)
	m.mu.RUnlock()
	if providerCount != 0 {
		t.Errorf("providerMetrics map has %d entries after reset, want 0", providerCount)
	}
}

func TestMetrics_ConcurrentAccess(t *testing.T) {
	m := NewMetrics()
	done := make(chan bool)

	// Spawn multiple goroutines recording metrics concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				provider := fmt.Sprintf("provider-%d", id%3)
				m.RecordGetRecords(provider, time.Duration(j)*time.Microsecond, nil)
				m.RecordCreateRecord(provider, time.Duration(j)*time.Microsecond, nil)
				_ = m.Snapshot()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify counts (10 goroutines * 100 iterations each)
	if m.getRecordsCalls.Load() != 1000 {
		t.Errorf("getRecordsCalls = %d, want 1000", m.getRecordsCalls.Load())
	}
	if m.createRecordCalls.Load() != 1000 {
		t.Errorf("createRecordCalls = %d, want 1000", m.createRecordCalls.Load())
	}
}

func TestPercentile_EdgeCases(t *testing.T) {
	// Empty samples
	result := percentile([]time.Duration{}, 50)
	if result != 0 {
		t.Errorf("percentile of empty slice = %f, want 0", result)
	}

	// Single sample
	result = percentile([]time.Duration{100 * time.Millisecond}, 50)
	if result != 100 {
		t.Errorf("percentile of single sample = %f, want 100", result)
	}

	// Two samples
	result = percentile([]time.Duration{100 * time.Millisecond, 200 * time.Millisecond}, 50)
	if result != 100 {
		t.Errorf("P50 of [100, 200] = %f, want 100", result)
	}
}

func TestSortDurations(t *testing.T) {
	durations := []time.Duration{
		300 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		50 * time.Millisecond,
	}

	sortDurations(durations)

	expected := []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
	}

	for i, d := range durations {
		if d != expected[i] {
			t.Errorf("sorted[%d] = %v, want %v", i, d, expected[i])
		}
	}
}

func TestDefaultMetrics(t *testing.T) {
	if DefaultMetrics == nil {
		t.Error("DefaultMetrics is nil")
	}

	// Verify it's usable
	DefaultMetrics.RecordGetRecords("test", time.Millisecond, nil)
	snap := DefaultMetrics.Snapshot()
	if snap.GetRecordsCalls == 0 {
		t.Error("DefaultMetrics should track calls")
	}

	// Clean up for other tests
	DefaultMetrics.Reset()
}
