package observability

import (
	"strings"
	"testing"
	"time"
)

func TestNewProxyMetrics(t *testing.T) {
	m := NewProxyMetrics()

	if m == nil {
		t.Fatal("NewProxyMetrics returned nil")
	}

	if m.devicesByProtocol == nil {
		t.Error("devicesByProtocol map is nil")
	}

	if m.connectionLatency == nil {
		t.Error("connectionLatency is nil")
	}

	if m.commandLatency == nil {
		t.Error("commandLatency is nil")
	}
}

func TestProxyMetrics_SetDeviceCounts(t *testing.T) {
	m := NewProxyMetrics()

	m.SetDeviceCounts(100, 75, 10, 5, 10)

	snapshot := m.Snapshot()
	if snapshot.DevicesTotal != 100 {
		t.Errorf("expected DevicesTotal 100, got %d", snapshot.DevicesTotal)
	}
	if snapshot.DevicesHealthy != 75 {
		t.Errorf("expected DevicesHealthy 75, got %d", snapshot.DevicesHealthy)
	}
	if snapshot.DevicesDegraded != 10 {
		t.Errorf("expected DevicesDegraded 10, got %d", snapshot.DevicesDegraded)
	}
	if snapshot.DevicesUnhealthy != 5 {
		t.Errorf("expected DevicesUnhealthy 5, got %d", snapshot.DevicesUnhealthy)
	}
	if snapshot.DevicesUnknown != 10 {
		t.Errorf("expected DevicesUnknown 10, got %d", snapshot.DevicesUnknown)
	}
}

func TestProxyMetrics_SetDevicesByProtocol(t *testing.T) {
	m := NewProxyMetrics()

	counts := map[string]int{
		"ssh":  50,
		"snmp": 30,
	}
	m.SetDevicesByProtocol(counts)

	snapshot := m.Snapshot()
	if snapshot.DevicesByProtocol["ssh"] != 50 {
		t.Errorf("expected ssh count 50, got %d", snapshot.DevicesByProtocol["ssh"])
	}
	if snapshot.DevicesByProtocol["snmp"] != 30 {
		t.Errorf("expected snmp count 30, got %d", snapshot.DevicesByProtocol["snmp"])
	}
}

func TestProxyMetrics_SetDevicesByVendor(t *testing.T) {
	m := NewProxyMetrics()

	counts := map[string]int{
		"Cisco":   40,
		"Juniper": 20,
	}
	m.SetDevicesByVendor(counts)

	snapshot := m.Snapshot()
	if snapshot.DevicesByVendor["Cisco"] != 40 {
		t.Errorf("expected Cisco count 40, got %d", snapshot.DevicesByVendor["Cisco"])
	}
	if snapshot.DevicesByVendor["Juniper"] != 20 {
		t.Errorf("expected Juniper count 20, got %d", snapshot.DevicesByVendor["Juniper"])
	}
}

func TestProxyMetrics_RecordConnection(t *testing.T) {
	m := NewProxyMetrics()

	// Record successful connection
	m.RecordConnection(true, 100*time.Millisecond)
	m.RecordConnection(true, 200*time.Millisecond)

	// Record failed connection
	m.RecordConnection(false, 0)

	snapshot := m.Snapshot()
	if snapshot.ConnectionsTotal != 3 {
		t.Errorf("expected ConnectionsTotal 3, got %d", snapshot.ConnectionsTotal)
	}
	if snapshot.ConnectionsActive != 2 {
		t.Errorf("expected ConnectionsActive 2, got %d", snapshot.ConnectionsActive)
	}
	if snapshot.ConnectionsFailed != 1 {
		t.Errorf("expected ConnectionsFailed 1, got %d", snapshot.ConnectionsFailed)
	}
}

func TestProxyMetrics_RecordDisconnection(t *testing.T) {
	m := NewProxyMetrics()

	m.RecordConnection(true, 100*time.Millisecond)
	m.RecordConnection(true, 100*time.Millisecond)
	m.RecordDisconnection()

	snapshot := m.Snapshot()
	if snapshot.ConnectionsActive != 1 {
		t.Errorf("expected ConnectionsActive 1, got %d", snapshot.ConnectionsActive)
	}

	// Verify it doesn't go below 0
	m.RecordDisconnection()
	m.RecordDisconnection()
	snapshot = m.Snapshot()
	if snapshot.ConnectionsActive < 0 {
		t.Error("ConnectionsActive went negative")
	}
}

func TestProxyMetrics_RecordCommand(t *testing.T) {
	m := NewProxyMetrics()

	// Record successful commands
	m.RecordCommand(true, 50*time.Millisecond, "ssh")
	m.RecordCommand(true, 100*time.Millisecond, "snmp")

	// Record failed command
	m.RecordCommand(false, 0, "ssh")

	snapshot := m.Snapshot()
	if snapshot.CommandsTotal != 3 {
		t.Errorf("expected CommandsTotal 3, got %d", snapshot.CommandsTotal)
	}
	if snapshot.CommandsSucceeded != 2 {
		t.Errorf("expected CommandsSucceeded 2, got %d", snapshot.CommandsSucceeded)
	}
	if snapshot.CommandsFailed != 1 {
		t.Errorf("expected CommandsFailed 1, got %d", snapshot.CommandsFailed)
	}
	if snapshot.SSHCommands != 2 {
		t.Errorf("expected SSHCommands 2, got %d", snapshot.SSHCommands)
	}
	if snapshot.SNMPRequests != 1 {
		t.Errorf("expected SNMPRequests 1, got %d", snapshot.SNMPRequests)
	}
}

func TestProxyMetrics_RecordState(t *testing.T) {
	m := NewProxyMetrics()

	// Success without change
	m.RecordState(true, false, 100*time.Millisecond)

	// Success with change
	m.RecordState(true, true, 200*time.Millisecond)

	// Failure
	m.RecordState(false, false, 50*time.Millisecond)

	snapshot := m.Snapshot()
	if snapshot.StatesApplied != 3 {
		t.Errorf("expected StatesApplied 3, got %d", snapshot.StatesApplied)
	}
	if snapshot.StatesSucceeded != 2 {
		t.Errorf("expected StatesSucceeded 2, got %d", snapshot.StatesSucceeded)
	}
	if snapshot.StatesFailed != 1 {
		t.Errorf("expected StatesFailed 1, got %d", snapshot.StatesFailed)
	}
	if snapshot.StatesChanged != 1 {
		t.Errorf("expected StatesChanged 1, got %d", snapshot.StatesChanged)
	}
}

func TestProxyMetrics_RecordDrift(t *testing.T) {
	m := NewProxyMetrics()

	m.RecordDrift(true, "high")
	m.RecordDrift(true, "high")
	m.RecordDrift(true, "low")
	m.RecordDrift(false, "")

	snapshot := m.Snapshot()
	if snapshot.DriftChecks != 4 {
		t.Errorf("expected DriftChecks 4, got %d", snapshot.DriftChecks)
	}
	if snapshot.DriftDetected != 3 {
		t.Errorf("expected DriftDetected 3, got %d", snapshot.DriftDetected)
	}
	if snapshot.DriftSeverity["high"] != 2 {
		t.Errorf("expected high severity 2, got %d", snapshot.DriftSeverity["high"])
	}
	if snapshot.DriftSeverity["low"] != 1 {
		t.Errorf("expected low severity 1, got %d", snapshot.DriftSeverity["low"])
	}
}

func TestProxyMetrics_RecordDiscovery(t *testing.T) {
	m := NewProxyMetrics()

	m.RecordDiscovery(1, 10, 5, 2)
	m.RecordDiscovery(1, 5, 3, 1)

	snapshot := m.Snapshot()
	if snapshot.DiscoveryScans != 2 {
		t.Errorf("expected DiscoveryScans 2, got %d", snapshot.DiscoveryScans)
	}
	if snapshot.DiscoveredDevices != 15 {
		t.Errorf("expected DiscoveredDevices 15, got %d", snapshot.DiscoveredDevices)
	}
	if snapshot.ApprovedDevices != 8 {
		t.Errorf("expected ApprovedDevices 8, got %d", snapshot.ApprovedDevices)
	}
	if snapshot.RejectedDevices != 3 {
		t.Errorf("expected RejectedDevices 3, got %d", snapshot.RejectedDevices)
	}
}

func TestProxyMetrics_RecordError(t *testing.T) {
	m := NewProxyMetrics()

	m.RecordError("connection_timeout")
	m.RecordError("connection_timeout")
	m.RecordError("auth_failure")

	snapshot := m.Snapshot()
	if snapshot.ErrorsByType["connection_timeout"] != 2 {
		t.Errorf("expected connection_timeout errors 2, got %d", snapshot.ErrorsByType["connection_timeout"])
	}
	if snapshot.ErrorsByType["auth_failure"] != 1 {
		t.Errorf("expected auth_failure errors 1, got %d", snapshot.ErrorsByType["auth_failure"])
	}
}

func TestNewLatencyStats(t *testing.T) {
	stats := NewLatencyStats()

	if stats == nil {
		t.Fatal("NewLatencyStats returned nil")
	}

	count, avg, minVal, maxVal := stats.Stats()
	if count != 0 || avg != 0 || minVal != 0 || maxVal != 0 {
		t.Error("expected all stats to be 0 for new LatencyStats")
	}
}

func TestLatencyStats_Record(t *testing.T) {
	stats := NewLatencyStats()

	stats.Record(100 * time.Millisecond)
	stats.Record(200 * time.Millisecond)
	stats.Record(150 * time.Millisecond)

	count, avg, minVal, maxVal := stats.Stats()

	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}

	if minVal != 100*time.Millisecond {
		t.Errorf("expected min 100ms, got %v", minVal)
	}

	if maxVal != 200*time.Millisecond {
		t.Errorf("expected max 200ms, got %v", maxVal)
	}

	expectedAvg := 150 * time.Millisecond
	if avg != expectedAvg {
		t.Errorf("expected avg %v, got %v", expectedAvg, avg)
	}
}

func TestLatencyStats_Buckets(t *testing.T) {
	stats := NewLatencyStats()

	// Record values in different buckets
	stats.Record(2 * time.Millisecond)   // bucket 5ms
	stats.Record(50 * time.Millisecond)  // bucket 50ms
	stats.Record(500 * time.Millisecond) // bucket 500ms

	buckets := stats.Buckets()
	if len(buckets) == 0 {
		t.Error("expected non-empty buckets")
	}
}

func TestNewPrometheusExporter(t *testing.T) {
	metrics := NewProxyMetrics()
	exporter := NewPrometheusExporter(metrics, "kscore_proxy")

	if exporter == nil {
		t.Fatal("NewPrometheusExporter returned nil")
	}

	if exporter.prefix != "kscore_proxy" {
		t.Errorf("expected prefix 'kscore_proxy', got '%s'", exporter.prefix)
	}
}

func TestNewPrometheusExporter_DefaultPrefix(t *testing.T) {
	metrics := NewProxyMetrics()
	exporter := NewPrometheusExporter(metrics, "")

	if exporter.prefix != "kscore_proxy" {
		t.Errorf("expected default prefix 'kscore_proxy', got '%s'", exporter.prefix)
	}
}

func TestPrometheusExporter_Export(t *testing.T) {
	metrics := NewProxyMetrics()

	// Set some metrics
	metrics.SetDeviceCounts(10, 8, 1, 1, 0)
	metrics.SetDevicesByProtocol(map[string]int{"ssh": 5, "snmp": 5})
	metrics.RecordCommand(true, 100*time.Millisecond, "ssh")
	metrics.RecordConnection(true, 50*time.Millisecond)

	exporter := NewPrometheusExporter(metrics, "kscore_proxy")

	output := exporter.Export()

	// Check that output contains expected metric names
	expectedMetrics := []string{
		"kscore_proxy_devices_total",
		"kscore_proxy_devices_healthy",
		"kscore_proxy_commands_total",
		"kscore_proxy_connections_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(output, metric) {
			t.Errorf("expected metric '%s' in output", metric)
		}
	}
}

func TestPrometheusExporter_ExportWithBuckets(t *testing.T) {
	metrics := NewProxyMetrics()

	// Record some connections to populate latency
	metrics.RecordConnection(true, 50*time.Millisecond)
	metrics.RecordConnection(true, 100*time.Millisecond)
	metrics.RecordConnection(true, 500*time.Millisecond)

	exporter := NewPrometheusExporter(metrics, "kscore_proxy")

	output := exporter.Export()

	// Check that connection latency metric is present
	if !strings.Contains(output, "connection_latency") {
		t.Error("expected connection_latency metric in output")
	}
}

func TestMetricsSnapshot(t *testing.T) {
	m := NewProxyMetrics()

	m.SetDeviceCounts(10, 8, 1, 1, 0)
	m.RecordCommand(true, 100*time.Millisecond, "ssh")
	m.RecordConnection(true, 50*time.Millisecond)

	snapshot := m.Snapshot()

	if snapshot.DevicesTotal != 10 {
		t.Errorf("expected DevicesTotal 10, got %d", snapshot.DevicesTotal)
	}

	if snapshot.DevicesHealthy != 8 {
		t.Errorf("expected DevicesHealthy 8, got %d", snapshot.DevicesHealthy)
	}

	if snapshot.CommandsTotal != 1 {
		t.Errorf("expected CommandsTotal 1, got %d", snapshot.CommandsTotal)
	}

	if snapshot.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}
}

func TestMetricsSnapshot_SuccessRates(t *testing.T) {
	m := NewProxyMetrics()

	// 8 successful, 2 failed commands
	for i := 0; i < 8; i++ {
		m.RecordCommand(true, 100*time.Millisecond, "ssh")
	}
	for i := 0; i < 2; i++ {
		m.RecordCommand(false, 0, "ssh")
	}

	// 9 successful, 1 failed states
	for i := 0; i < 9; i++ {
		m.RecordState(true, false, 100*time.Millisecond)
	}
	m.RecordState(false, false, 0)

	snapshot := m.Snapshot()

	if snapshot.CommandSuccessRate != 80 {
		t.Errorf("expected CommandSuccessRate 80, got %f", snapshot.CommandSuccessRate)
	}

	if snapshot.StateSuccessRate != 90 {
		t.Errorf("expected StateSuccessRate 90, got %f", snapshot.StateSuccessRate)
	}
}
