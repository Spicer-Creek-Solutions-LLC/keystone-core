package nats

import (
	"strings"
	"testing"
	"time"
)

func TestNewMetricsCollector(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	if collector == nil {
		t.Fatal("expected non-nil collector")
	}
	if collector.config != config {
		t.Error("expected config to be set")
	}
}

func TestNATSMetricsCollector_RecordConnection(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordConnection("nats://localhost:4222", "primary", "success")
	collector.RecordConnection("nats://localhost:4222", "primary", "success")
	collector.RecordConnection("nats://localhost:4222", "primary", "failed")

	stats := collector.GetStats()
	if stats.TotalConnections != 3 {
		t.Errorf("expected 3 connections, got %d", stats.TotalConnections)
	}
}

func TestNATSMetricsCollector_RecordConnectionLatency(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordConnectionLatency("nats://localhost:4222", 10*time.Millisecond)
	collector.RecordConnectionLatency("nats://localhost:4222", 20*time.Millisecond)
	collector.RecordConnectionLatency("nats://localhost:4222", 30*time.Millisecond)

	stats := collector.GetStats()
	if len(stats.ConnectionLatency) == 0 {
		t.Error("expected latencies to be recorded")
	}
}

func TestNATSMetricsCollector_RecordConnectionError(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordConnectionError("nats://localhost:4222", "connection_refused")
	collector.RecordConnectionError("nats://localhost:4222", "connection_refused")
	collector.RecordConnectionError("nats://localhost:4222", "timeout")

	stats := collector.GetStats()
	if stats.TotalConnectionErrors != 3 {
		t.Errorf("expected 3 errors, got %d", stats.TotalConnectionErrors)
	}
}

func TestNATSMetricsCollector_RecordMessage(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordMessage("sent", "cmd", 1024)
	collector.RecordMessage("sent", "cmd", 2048)
	collector.RecordMessage("received", "event", 512)

	stats := collector.GetStats()
	if stats.TotalMessagesSent != 2 {
		t.Errorf("expected 2 sent messages, got %d", stats.TotalMessagesSent)
	}
	if stats.TotalMessagesReceived != 1 {
		t.Errorf("expected 1 received message, got %d", stats.TotalMessagesReceived)
	}
	if stats.TotalBytesSent != 3072 {
		t.Errorf("expected 3072 bytes sent, got %d", stats.TotalBytesSent)
	}
}

func TestNATSMetricsCollector_RecordReconnection(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordReconnection("nats://localhost:4222")
	collector.RecordReconnection("nats://localhost:4222")

	stats := collector.GetStats()
	if stats.TotalReconnections != 2 {
		t.Errorf("expected 2 reconnections, got %d", stats.TotalReconnections)
	}
}

func TestNATSMetricsCollector_RecordCircuitBreakerState(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordCircuitBreakerState("endpoint", CircuitStateClosed)
	stats := collector.GetStats()
	if stats.CircuitBreakerStates["endpoint"] != "closed" {
		t.Errorf("expected closed state, got %s", stats.CircuitBreakerStates["endpoint"])
	}

	collector.RecordCircuitBreakerState("endpoint", CircuitStateOpen)
	stats = collector.GetStats()
	if stats.CircuitBreakerStates["endpoint"] != "open" {
		t.Errorf("expected open state, got %s", stats.CircuitBreakerStates["endpoint"])
	}
}

func TestNATSMetricsCollector_RecordDegradationMode(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordDegradationMode("buffer", DegradationModeNormal)
	stats := collector.GetStats()
	if stats.DegradationModes["buffer"] != "normal" {
		t.Errorf("expected normal mode, got %s", stats.DegradationModes["buffer"])
	}

	collector.RecordDegradationMode("buffer", DegradationModeDegraded)
	stats = collector.GetStats()
	if stats.DegradationModes["buffer"] != "degraded" {
		t.Errorf("expected degraded mode, got %s", stats.DegradationModes["buffer"])
	}
}

func TestNATSMetricsCollector_RecordFailover(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordFailover("nats://primary:4222", "nats://backup:4222")
	collector.RecordFailover("nats://primary:4222", "nats://backup:4222")

	stats := collector.GetStats()
	if stats.TotalFailovers != 2 {
		t.Errorf("expected 2 failovers, got %d", stats.TotalFailovers)
	}
}

func TestNATSMetricsCollector_RecordBufferSize(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordBufferSize("outbound", 100)
	collector.RecordBufferSize("outbound", 200)

	stats := collector.GetStats()
	if stats.BufferSizes["outbound"] != 200 {
		t.Errorf("expected buffer size 200, got %d", stats.BufferSizes["outbound"])
	}
}

func TestNATSMetricsCollector_RecordBufferOverflow(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordBufferOverflow()
	collector.RecordBufferOverflow()

	stats := collector.GetStats()
	if stats.TotalBufferOverflow != 2 {
		t.Errorf("expected 2 overflows, got %d", stats.TotalBufferOverflow)
	}
}

func TestNATSMetricsCollector_RecordDelivery(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordDeliveryAcked()
	collector.RecordDeliveryAcked()
	collector.RecordDeliveryFailed()
	collector.RecordDeliveryPending(5)

	stats := collector.GetStats()
	if stats.DeliveryAcked != 2 {
		t.Errorf("expected 2 acked, got %d", stats.DeliveryAcked)
	}
	if stats.DeliveryFailed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.DeliveryFailed)
	}
	if stats.DeliveryPending != 5 {
		t.Errorf("expected 5 pending, got %d", stats.DeliveryPending)
	}
}

func TestNATSMetricsCollector_RecordDuplicate(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordDuplicateDetected()
	collector.RecordDuplicateDetected()
	collector.RecordDuplicateDetected()

	stats := collector.GetStats()
	if stats.DuplicatesDetected != 3 {
		t.Errorf("expected 3 duplicates, got %d", stats.DuplicatesDetected)
	}
}

func TestNATSMetricsCollector_RecordLeafNode(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordLeafNode("hub1", 1)
	collector.RecordLeafNode("hub1", 1)
	collector.RecordLeafNode("hub1", -1)

	stats := collector.GetStats()
	if stats.LeafNodeCounts["hub1"] != 1 {
		t.Errorf("expected 1 leaf node, got %d", stats.LeafNodeCounts["hub1"])
	}
}

func TestNATSMetricsCollector_RecordGateway(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordGatewayConnection("cluster1", "cluster2", 1)
	collector.RecordGatewayConnection("cluster1", "cluster2", -1)

	stats := collector.GetStats()
	key := "cluster1_cluster2"
	if stats.GatewayConnections[key] != 0 {
		t.Errorf("expected 0 gateway connections, got %d", stats.GatewayConnections[key])
	}
}

func TestNATSMetricsCollector_RecordGatewayLatency(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordGatewayLatency("cluster1", "cluster2", 50*time.Millisecond)
	collector.RecordGatewayLatency("cluster1", "cluster2", 100*time.Millisecond)

	stats := collector.GetStats()
	if len(stats.GatewayLatency) == 0 {
		t.Error("expected gateway latencies to be recorded")
	}
}

func TestNATSMetricsCollector_RecordBootstrapRequest(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordBootstrapRequest("approved")
	collector.RecordBootstrapRequest("approved")
	collector.RecordBootstrapRequest("rejected")
	collector.RecordBootstrapRequest("expired")

	stats := collector.GetStats()
	total := stats.BootstrapRequests["approved"] + stats.BootstrapRequests["rejected"] + stats.BootstrapRequests["expired"]
	if total != 4 {
		t.Errorf("expected 4 bootstrap requests, got %d", total)
	}
}

func TestNATSMetricsCollector_RecordCoordinationRPC(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordCoordinationRPC("GetLeader", "success")
	collector.RecordCoordinationRPC("GetLeader", "success")
	collector.RecordCoordinationRPC("GetLeader", "error")

	stats := collector.GetStats()
	// Key format is "method:status"
	total := stats.CoordinationRPCs["GetLeader:success"] + stats.CoordinationRPCs["GetLeader:error"]
	if total != 3 {
		t.Errorf("expected 3 coordination RPCs, got %d", total)
	}
}

func TestNATSMetricsCollector_RecordCoordinationLatency(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	collector.RecordCoordinationLatency("GetLeader", 10*time.Millisecond)
	collector.RecordCoordinationLatency("GetLeader", 20*time.Millisecond)

	stats := collector.GetStats()
	if len(stats.CoordinationLatency) == 0 {
		t.Error("expected coordination latencies to be recorded")
	}
}

func TestLatencyHistogram(t *testing.T) {
	buckets := []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0}
	hist := NewLatencyHistogram(buckets)

	// Add samples (in seconds)
	for i := 0; i < 100; i++ {
		hist.Observe(float64(i+1) / 1000.0) // 1-100ms in seconds
	}

	stats := hist.GetStats()

	if stats.Count != 100 {
		t.Errorf("expected 100 samples, got %d", stats.Count)
	}
	// Min should be ~0.001 (1ms in seconds)
	if stats.Min < 0.0009 || stats.Min > 0.0011 {
		t.Errorf("expected min ~0.001, got %v", stats.Min)
	}
	// Max should be ~0.1 (100ms in seconds)
	if stats.Max < 0.099 || stats.Max > 0.101 {
		t.Errorf("expected max ~0.1, got %v", stats.Max)
	}
}

func TestLatencyHistogram_MultipleObservations(t *testing.T) {
	buckets := []float64{0.001, 0.005, 0.01, 0.05, 0.1, 1.0}
	hist := NewLatencyHistogram(buckets)

	hist.Observe(0.001) // 1ms
	hist.Observe(0.005) // 5ms
	hist.Observe(0.05)  // 50ms
	hist.Observe(0.5)   // 500ms
	hist.Observe(5.0)   // 5s

	stats := hist.GetStats()

	// Should have recorded all 5 observations
	if stats.Count != 5 {
		t.Errorf("expected count 5, got %d", stats.Count)
	}
	// Min should be 1ms
	if stats.Min != 0.001 {
		t.Errorf("expected min 0.001, got %f", stats.Min)
	}
	// Max should be 5s
	if stats.Max != 5.0 {
		t.Errorf("expected max 5.0, got %f", stats.Max)
	}
}

func TestPrometheusExporter(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	// Record some metrics
	collector.RecordConnection("nats://localhost:4222", "primary", "success")
	collector.RecordConnectionLatency("nats://localhost:4222", 10*time.Millisecond)
	collector.RecordMessage("sent", "cmd", 1024)
	collector.RecordBufferSize("outbound", 50)

	exporter := NewPrometheusExporter(collector)
	output := exporter.Export()

	// Check output contains expected metrics
	expectedMetrics := []string{
		"kscore_nats_connections_total",
		"kscore_nats_messages_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(output, metric) {
			t.Errorf("expected output to contain %s", metric)
		}
	}

	// Should have HELP and TYPE lines
	if !strings.Contains(output, "# HELP") {
		t.Error("expected HELP lines in output")
	}
	if !strings.Contains(output, "# TYPE") {
		t.Error("expected TYPE lines in output")
	}
}

func TestPrometheusExporter_AllMetricTypes(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	// Record various metric types
	collector.RecordConnection("endpoint", "primary", "success")
	collector.RecordConnectionError("endpoint", "timeout")
	collector.RecordReconnection("endpoint")
	collector.RecordCircuitBreakerState("endpoint", CircuitStateOpen)
	collector.RecordDegradationMode("test", DegradationModeDegraded)
	collector.RecordFailover("from", "to")
	collector.RecordBufferOverflow()
	collector.RecordDeliveryAcked()
	collector.RecordDuplicateDetected()
	collector.RecordLeafNode("hub", 1)
	collector.RecordGatewayConnection("c1", "c2", 1)
	collector.RecordBootstrapRequest("approved")
	collector.RecordCoordinationRPC("method", "success")

	exporter := NewPrometheusExporter(collector)
	output := exporter.Export()

	// Should export all metric families
	expectedFamilies := []string{
		"kscore_nats_connections_total",
		"kscore_nats_connection_errors_total",
		"kscore_nats_reconnections_total",
	}

	for _, family := range expectedFamilies {
		if !strings.Contains(output, family) {
			t.Errorf("expected output to contain %s", family)
		}
	}
}

func TestObservabilityConfig_Defaults(t *testing.T) {
	config := DefaultObservabilityConfig()

	if config.Enabled != true {
		t.Error("expected enabled by default")
	}
	if config.MetricsPrefix != "kscore_nats" {
		t.Errorf("expected prefix 'kscore_nats', got %s", config.MetricsPrefix)
	}
	if len(config.HistogramBuckets) == 0 {
		t.Error("expected histogram buckets to be configured")
	}
}

func TestNATSMetricsCollector_GetStats(t *testing.T) {
	config := DefaultObservabilityConfig()
	collector := NewMetricsCollector(config)

	// Record some data
	collector.RecordConnection("ep1", "primary", "success")
	collector.RecordMessage("sent", "cmd", 1024)
	collector.RecordBufferSize("outbound", 100)

	stats := collector.GetStats()

	// Verify stats contains data
	if stats.TotalConnections != 1 {
		t.Errorf("expected 1 connection, got %d", stats.TotalConnections)
	}
	if stats.TotalMessagesSent != 1 {
		t.Errorf("expected 1 message, got %d", stats.TotalMessagesSent)
	}
}

func TestCircuitState_Values(t *testing.T) {
	// Verify circuit states are distinct
	states := []CircuitState{CircuitStateClosed, CircuitStateHalfOpen, CircuitStateOpen}
	seen := make(map[CircuitState]bool)
	for _, s := range states {
		if seen[s] {
			t.Errorf("duplicate circuit state: %v", s)
		}
		seen[s] = true
	}
}

func TestDegradationMode_Values(t *testing.T) {
	// Verify degradation modes are distinct
	modes := []DegradationMode{DegradationModeNormal, DegradationModeDegraded, DegradationModeLimited, DegradationModeOffline}
	seen := make(map[DegradationMode]bool)
	for _, m := range modes {
		if seen[m] {
			t.Errorf("duplicate degradation mode: %v", m)
		}
		seen[m] = true
	}
}
