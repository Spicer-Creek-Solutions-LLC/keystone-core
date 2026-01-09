package mirror

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewMirrorMetrics(t *testing.T) {
	metrics := NewMirrorMetrics()
	if metrics == nil {
		t.Fatal("NewMirrorMetrics returned nil")
	}

	// Check that all maps are initialized
	if metrics.mirrorHealth == nil {
		t.Error("mirrorHealth map not initialized")
	}
	if metrics.readOperations == nil {
		t.Error("readOperations map not initialized")
	}
	if metrics.writeOperations == nil {
		t.Error("writeOperations map not initialized")
	}
	if metrics.syncOperationsTotal == nil {
		t.Error("syncOperationsTotal map not initialized")
	}
	if len(metrics.latencyBuckets) == 0 {
		t.Error("latencyBuckets not initialized")
	}
}

func TestMirrorMetrics_SetGroupCount(t *testing.T) {
	metrics := NewMirrorMetrics()

	metrics.SetGroupCount(5)
	if metrics.groupCount != 5 {
		t.Errorf("expected groupCount 5, got %d", metrics.groupCount)
	}

	metrics.SetGroupCount(10)
	if metrics.groupCount != 10 {
		t.Errorf("expected groupCount 10, got %d", metrics.groupCount)
	}
}

func TestMirrorMetrics_SetMirrorHealth(t *testing.T) {
	metrics := NewMirrorMetrics()

	metrics.SetMirrorHealth("group1", "mirror1", MirrorStateHealthy)
	metrics.SetMirrorHealth("group1", "mirror2", MirrorStateDegraded)
	metrics.SetMirrorHealth("group2", "mirror1", MirrorStateUnhealthy)

	// Check group1
	if metrics.mirrorHealth["group1"]["mirror1"] != MirrorStateHealthy {
		t.Error("mirror1 state not healthy")
	}
	if metrics.mirrorHealth["group1"]["mirror2"] != MirrorStateDegraded {
		t.Error("mirror2 state not degraded")
	}

	// Check group2
	if metrics.mirrorHealth["group2"]["mirror1"] != MirrorStateUnhealthy {
		t.Error("group2 mirror1 state not unhealthy")
	}
}

func TestMirrorMetrics_RecordRead(t *testing.T) {
	metrics := NewMirrorMetrics()

	// Record successful read
	metrics.RecordRead("group1", 1024, 10*time.Millisecond, nil)

	if metrics.readOperations["group1"] != 1 {
		t.Errorf("expected 1 read operation, got %d", metrics.readOperations["group1"])
	}
	if metrics.readBytes["group1"] != 1024 {
		t.Errorf("expected 1024 bytes, got %d", metrics.readBytes["group1"])
	}
	if metrics.readErrors["group1"] != 0 {
		t.Errorf("expected 0 errors, got %d", metrics.readErrors["group1"])
	}

	// Record failed read
	metrics.RecordRead("group1", 0, 5*time.Millisecond, errors.New("test error"))

	if metrics.readOperations["group1"] != 2 {
		t.Errorf("expected 2 read operations, got %d", metrics.readOperations["group1"])
	}
	if metrics.readErrors["group1"] != 1 {
		t.Errorf("expected 1 error, got %d", metrics.readErrors["group1"])
	}
}

func TestMirrorMetrics_RecordWrite(t *testing.T) {
	metrics := NewMirrorMetrics()

	// Record successful write
	metrics.RecordWrite("group1", 2048, 20*time.Millisecond, nil)

	if metrics.writeOperations["group1"] != 1 {
		t.Errorf("expected 1 write operation, got %d", metrics.writeOperations["group1"])
	}
	if metrics.writeBytes["group1"] != 2048 {
		t.Errorf("expected 2048 bytes, got %d", metrics.writeBytes["group1"])
	}
	if metrics.writeErrors["group1"] != 0 {
		t.Errorf("expected 0 errors, got %d", metrics.writeErrors["group1"])
	}

	// Record failed write
	metrics.RecordWrite("group1", 0, 10*time.Millisecond, errors.New("test error"))

	if metrics.writeOperations["group1"] != 2 {
		t.Errorf("expected 2 write operations, got %d", metrics.writeOperations["group1"])
	}
	if metrics.writeErrors["group1"] != 1 {
		t.Errorf("expected 1 error, got %d", metrics.writeErrors["group1"])
	}
}

func TestMirrorMetrics_RecordSyncOperation(t *testing.T) {
	metrics := NewMirrorMetrics()

	// Record successful sync
	metrics.RecordSyncOperation("group1", true, 10240, 5, 100*time.Millisecond)

	if metrics.syncOperationsTotal["group1"] != 1 {
		t.Errorf("expected 1 sync operation, got %d", metrics.syncOperationsTotal["group1"])
	}
	if metrics.syncOperationsSucceeded["group1"] != 1 {
		t.Errorf("expected 1 succeeded, got %d", metrics.syncOperationsSucceeded["group1"])
	}
	if metrics.syncOperationsFailed["group1"] != 0 {
		t.Errorf("expected 0 failed, got %d", metrics.syncOperationsFailed["group1"])
	}
	if metrics.syncBytesTotal["group1"] != 10240 {
		t.Errorf("expected 10240 bytes, got %d", metrics.syncBytesTotal["group1"])
	}
	if metrics.syncFilesTotal["group1"] != 5 {
		t.Errorf("expected 5 files, got %d", metrics.syncFilesTotal["group1"])
	}

	// Record failed sync
	metrics.RecordSyncOperation("group1", false, 0, 0, 50*time.Millisecond)

	if metrics.syncOperationsTotal["group1"] != 2 {
		t.Errorf("expected 2 sync operations, got %d", metrics.syncOperationsTotal["group1"])
	}
	if metrics.syncOperationsFailed["group1"] != 1 {
		t.Errorf("expected 1 failed, got %d", metrics.syncOperationsFailed["group1"])
	}
}

func TestMirrorMetrics_SetActiveSyncOperations(t *testing.T) {
	metrics := NewMirrorMetrics()

	metrics.SetActiveSyncOperations("group1", 3)
	if metrics.syncOperationsActive["group1"] != 3 {
		t.Errorf("expected 3 active, got %d", metrics.syncOperationsActive["group1"])
	}

	metrics.SetActiveSyncOperations("group1", 0)
	if metrics.syncOperationsActive["group1"] != 0 {
		t.Errorf("expected 0 active, got %d", metrics.syncOperationsActive["group1"])
	}
}

func TestMirrorMetrics_RecordConflict(t *testing.T) {
	metrics := NewMirrorMetrics()

	metrics.RecordConflict("group1")
	metrics.RecordConflict("group1")
	metrics.RecordConflict("group2")

	if metrics.syncConflicts["group1"] != 2 {
		t.Errorf("expected 2 conflicts for group1, got %d", metrics.syncConflicts["group1"])
	}
	if metrics.syncConflicts["group2"] != 1 {
		t.Errorf("expected 1 conflict for group2, got %d", metrics.syncConflicts["group2"])
	}
}

func TestMirrorMetrics_Export(t *testing.T) {
	metrics := NewMirrorMetrics()

	// Set up some data
	metrics.SetGroupCount(2)
	metrics.SetMirrorHealth("group1", "mirror1", MirrorStateHealthy)
	metrics.RecordRead("group1", 1024, 10*time.Millisecond, nil)
	metrics.RecordWrite("group1", 2048, 20*time.Millisecond, nil)
	metrics.RecordSyncOperation("group1", true, 4096, 2, 50*time.Millisecond)
	metrics.RecordConflict("group1")

	output := metrics.Export()

	// Check for expected metric names
	expectedMetrics := []string{
		"kscore_mirror_groups_total",
		"kscore_mirror_health",
		"kscore_mirror_read_operations_total",
		"kscore_mirror_write_operations_total",
		"kscore_mirror_read_bytes_total",
		"kscore_mirror_write_bytes_total",
		"kscore_mirror_sync_operations_total",
		"kscore_mirror_sync_bytes_total",
		"kscore_mirror_sync_files_total",
		"kscore_mirror_sync_conflicts_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(output, metric) {
			t.Errorf("expected metric %s in output", metric)
		}
	}

	// Check for group label
	if !strings.Contains(output, `group="group1"`) {
		t.Error("expected group label in output")
	}

	// Check for mirror label in health metric
	if !strings.Contains(output, `mirror="mirror1"`) {
		t.Error("expected mirror label in output")
	}
}

func TestMirrorMetrics_Export_Empty(t *testing.T) {
	metrics := NewMirrorMetrics()
	output := metrics.Export()

	// Should still have group count header
	if !strings.Contains(output, "kscore_mirror_groups_total") {
		t.Error("expected group count metric even when empty")
	}

	// Should have zero value
	if !strings.Contains(output, "kscore_mirror_groups_total 0") {
		t.Error("expected zero group count")
	}
}

func TestMirrorMetrics_RecordLatencyBucket(t *testing.T) {
	metrics := NewMirrorMetrics()

	// Record reads with different latencies
	metrics.RecordRead("group1", 100, 1*time.Millisecond, nil)   // bucket 0 (<=1ms)
	metrics.RecordRead("group1", 100, 50*time.Millisecond, nil)  // bucket 4 (<=50ms)
	metrics.RecordRead("group1", 100, 100*time.Millisecond, nil) // bucket 5 (<=100ms)
	metrics.RecordRead("group1", 100, 20*time.Second, nil)       // +Inf bucket

	// Check that buckets have values
	buckets := metrics.readLatencyBuckets["group1"]
	if buckets == nil {
		t.Fatal("readLatencyBuckets not recorded")
	}

	// At least some buckets should have counts
	totalCount := int64(0)
	for _, count := range buckets {
		totalCount += count
	}
	if totalCount != 4 {
		t.Errorf("expected 4 latency records, got %d", totalCount)
	}
}

func TestMirrorMetrics_CollectFromRegistry(t *testing.T) {
	registry := NewRegistry()
	metrics := NewMirrorMetrics()

	// Add a group with mirrors
	group, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID:   "test-group",
		Name: "Test Group",
		Mirrors: []*Mirror{
			{ID: "mirror-1", ClusterID: "cluster-1", Enabled: true},
			{ID: "mirror-2", ClusterID: "cluster-2", Enabled: true},
		},
		ReadStrategy: ReadStrategyRoundRobin,
		WritePolicy:  WritePolicyAll,
	})
	registry.Register(group)
	group.UpdateHealth("mirror-1", MirrorStateHealthy, 10*time.Millisecond, nil)
	group.UpdateHealth("mirror-2", MirrorStateDegraded, 50*time.Millisecond, nil)

	// Collect
	metrics.CollectFromRegistry(registry)

	if metrics.groupCount != 1 {
		t.Errorf("expected 1 group, got %d", metrics.groupCount)
	}

	if metrics.mirrorHealth["test-group"]["mirror-1"] != MirrorStateHealthy {
		t.Error("mirror-1 health not collected correctly")
	}
	if metrics.mirrorHealth["test-group"]["mirror-2"] != MirrorStateDegraded {
		t.Error("mirror-2 health not collected correctly")
	}
}

func TestMirrorMetrics_CollectFromSyncEngine(t *testing.T) {
	registry := NewRegistry()
	syncEngine := NewSyncEngine(registry, DefaultSyncConfig())
	metrics := NewMirrorMetrics()

	// Add a group
	group, _ := NewMirrorGroup(&MirrorGroupConfig{
		ID:   "test-group",
		Name: "Test Group",
		Mirrors: []*Mirror{
			{ID: "mirror-1", ClusterID: "cluster-1", Enabled: true, IsPrimary: true},
			{ID: "mirror-2", ClusterID: "cluster-2", Enabled: true},
		},
		ReadStrategy: ReadStrategyRoundRobin,
		WritePolicy:  WritePolicyAll,
	})
	registry.Register(group)

	// Trigger a sync (won't actually complete without backend)
	syncEngine.TriggerSync("test-group", "mirror-1", "mirror-2", 0)

	// Collect
	metrics.CollectFromSyncEngine(syncEngine)

	// Should have recorded active operations
	// (Exact count depends on whether operation is still active)
	// Just verify no panic and method completes
}

func TestMirrorMetrics_MetricsHandler(t *testing.T) {
	metrics := NewMirrorMetrics()
	metrics.SetGroupCount(1)

	handler := metrics.MetricsHandler()
	if handler == nil {
		t.Fatal("MetricsHandler returned nil")
	}

	// Handler is a function, just verify it's callable
	// Full HTTP testing would require httptest
}

func TestMirrorMetrics_WriteHistogram(t *testing.T) {
	metrics := NewMirrorMetrics()

	// Record some latencies
	metrics.RecordRead("group1", 100, 5*time.Millisecond, nil)
	metrics.RecordRead("group1", 100, 25*time.Millisecond, nil)
	metrics.RecordRead("group1", 100, 100*time.Millisecond, nil)

	output := metrics.Export()

	// Should have histogram buckets
	if !strings.Contains(output, "kscore_mirror_read_latency_seconds_bucket") {
		t.Error("expected histogram bucket in output")
	}

	// Should have le labels
	if !strings.Contains(output, `le="`) {
		t.Error("expected le label in histogram")
	}

	// Should have +Inf bucket
	if !strings.Contains(output, `le="+Inf"`) {
		t.Error("expected +Inf bucket in histogram")
	}
}

func TestMirrorMetrics_ConcurrentAccess(t *testing.T) {
	metrics := NewMirrorMetrics()

	// Run concurrent operations to test thread safety
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(n int) {
			groupID := "group1"
			for j := 0; j < 100; j++ {
				metrics.RecordRead(groupID, 100, time.Duration(n)*time.Millisecond, nil)
				metrics.RecordWrite(groupID, 100, time.Duration(n)*time.Millisecond, nil)
				metrics.SetMirrorHealth(groupID, "mirror1", MirrorStateHealthy)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify operations were recorded
	if metrics.readOperations["group1"] != 1000 {
		t.Errorf("expected 1000 reads, got %d", metrics.readOperations["group1"])
	}
	if metrics.writeOperations["group1"] != 1000 {
		t.Errorf("expected 1000 writes, got %d", metrics.writeOperations["group1"])
	}
}
