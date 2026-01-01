package nats

import (
	"errors"
	"testing"
	"time"

	natsclient "github.com/nats-io/nats.go"
)

func TestNewConnectionMetrics(t *testing.T) {
	m := NewConnectionMetrics()
	if m == nil {
		t.Fatal("NewConnectionMetrics returned nil")
	}
	if m.endpointMetrics == nil {
		t.Error("endpointMetrics map is nil")
	}
	if m.startTime.IsZero() {
		t.Error("startTime not set")
	}
}

func TestConnectionMetrics_GetOrCreateEndpointMetrics(t *testing.T) {
	m := NewConnectionMetrics()

	// First call should create
	em1 := m.GetOrCreateEndpointMetrics("localhost:4222")
	if em1 == nil {
		t.Fatal("GetOrCreateEndpointMetrics returned nil")
	}
	if em1.Address != "localhost:4222" {
		t.Errorf("Address = %s, want localhost:4222", em1.Address)
	}

	// Second call should return same instance
	em2 := m.GetOrCreateEndpointMetrics("localhost:4222")
	if em1 != em2 {
		t.Error("GetOrCreateEndpointMetrics should return same instance")
	}
}

func TestConnectionMetrics_GetEndpointMetrics(t *testing.T) {
	m := NewConnectionMetrics()

	// Should return nil for non-existent
	if em := m.GetEndpointMetrics("localhost:4222"); em != nil {
		t.Error("GetEndpointMetrics should return nil for non-existent")
	}

	// Create and then get
	m.GetOrCreateEndpointMetrics("localhost:4222")
	em := m.GetEndpointMetrics("localhost:4222")
	if em == nil {
		t.Error("GetEndpointMetrics returned nil for existing endpoint")
	}
}

func TestConnectionMetrics_GetAllEndpointMetrics(t *testing.T) {
	m := NewConnectionMetrics()

	// Add multiple endpoints
	m.GetOrCreateEndpointMetrics("localhost:4222")
	m.GetOrCreateEndpointMetrics("localhost:4223")
	m.GetOrCreateEndpointMetrics("localhost:4224")

	all := m.GetAllEndpointMetrics()
	if len(all) != 3 {
		t.Errorf("GetAllEndpointMetrics returned %d, want 3", len(all))
	}
}

func TestConnectionMetrics_RecordConnectionAttempt(t *testing.T) {
	m := NewConnectionMetrics()

	m.RecordConnectionAttempt("localhost:4222")
	m.RecordConnectionAttempt("localhost:4222")
	m.RecordConnectionAttempt("localhost:4223")

	if m.ConnectionAttempts.Load() != 3 {
		t.Errorf("ConnectionAttempts = %d, want 3", m.ConnectionAttempts.Load())
	}

	em := m.GetEndpointMetrics("localhost:4222")
	if em.ConnectionAttempts.Load() != 2 {
		t.Errorf("endpoint ConnectionAttempts = %d, want 2", em.ConnectionAttempts.Load())
	}
}

func TestConnectionMetrics_RecordConnectionSuccess(t *testing.T) {
	m := NewConnectionMetrics()

	m.RecordConnectionSuccess("localhost:4222", 10*time.Millisecond)

	if m.ConnectionSuccesses.Load() != 1 {
		t.Errorf("ConnectionSuccesses = %d, want 1", m.ConnectionSuccesses.Load())
	}
	if m.ConnectedEndpoints.Load() != 1 {
		t.Errorf("ConnectedEndpoints = %d, want 1", m.ConnectedEndpoints.Load())
	}

	em := m.GetEndpointMetrics("localhost:4222")
	if em.ConnectionSuccesses.Load() != 1 {
		t.Errorf("endpoint ConnectionSuccesses = %d, want 1", em.ConnectionSuccesses.Load())
	}
	if em.LatencyCount.Load() != 1 {
		t.Errorf("LatencyCount = %d, want 1", em.LatencyCount.Load())
	}
}

func TestConnectionMetrics_RecordConnectionFailure(t *testing.T) {
	m := NewConnectionMetrics()

	testErr := errors.New("test error")
	m.RecordConnectionFailure("localhost:4222", testErr)

	if m.ConnectionFailures.Load() != 1 {
		t.Errorf("ConnectionFailures = %d, want 1", m.ConnectionFailures.Load())
	}

	em := m.GetEndpointMetrics("localhost:4222")
	if em.ConnectionFailures.Load() != 1 {
		t.Errorf("endpoint ConnectionFailures = %d, want 1", em.ConnectionFailures.Load())
	}
	if em.ErrorCount.Load() != 1 {
		t.Errorf("ErrorCount = %d, want 1", em.ErrorCount.Load())
	}
}

func TestConnectionMetrics_RecordDisconnection(t *testing.T) {
	m := NewConnectionMetrics()
	m.ConnectedEndpoints.Store(1)

	m.RecordDisconnection("localhost:4222", nil)

	if m.Disconnections.Load() != 1 {
		t.Errorf("Disconnections = %d, want 1", m.Disconnections.Load())
	}
	if m.ConnectedEndpoints.Load() != 0 {
		t.Errorf("ConnectedEndpoints = %d, want 0", m.ConnectedEndpoints.Load())
	}
}

func TestConnectionMetrics_RecordReconnection(t *testing.T) {
	m := NewConnectionMetrics()

	m.RecordReconnection("localhost:4222")

	if m.Reconnections.Load() != 1 {
		t.Errorf("Reconnections = %d, want 1", m.Reconnections.Load())
	}

	em := m.GetEndpointMetrics("localhost:4222")
	if em.Reconnections.Load() != 1 {
		t.Errorf("endpoint Reconnections = %d, want 1", em.Reconnections.Load())
	}
}

func TestConnectionMetrics_RecordCircuitOpen(t *testing.T) {
	m := NewConnectionMetrics()

	m.RecordCircuitOpen("localhost:4222")

	em := m.GetEndpointMetrics("localhost:4222")
	if em.CircuitOpenCount.Load() != 1 {
		t.Errorf("CircuitOpenCount = %d, want 1", em.CircuitOpenCount.Load())
	}
	if !em.CircuitCurrentlyOpen.Load() {
		t.Error("CircuitCurrentlyOpen should be true")
	}
}

func TestConnectionMetrics_RecordCircuitClose(t *testing.T) {
	m := NewConnectionMetrics()
	m.RecordCircuitOpen("localhost:4222")

	m.RecordCircuitClose("localhost:4222")

	em := m.GetEndpointMetrics("localhost:4222")
	if em.CircuitCloseCount.Load() != 1 {
		t.Errorf("CircuitCloseCount = %d, want 1", em.CircuitCloseCount.Load())
	}
	if em.CircuitCurrentlyOpen.Load() {
		t.Error("CircuitCurrentlyOpen should be false")
	}
}

func TestConnectionMetrics_RecordHealthCheck(t *testing.T) {
	m := NewConnectionMetrics()

	m.RecordHealthCheck("localhost:4222", true, 5*time.Millisecond)
	m.RecordHealthCheck("localhost:4222", false, 0)

	em := m.GetEndpointMetrics("localhost:4222")
	if em.HealthCheckCount.Load() != 2 {
		t.Errorf("HealthCheckCount = %d, want 2", em.HealthCheckCount.Load())
	}
	if em.HealthCheckSuccesses.Load() != 1 {
		t.Errorf("HealthCheckSuccesses = %d, want 1", em.HealthCheckSuccesses.Load())
	}
	if em.HealthCheckFailures.Load() != 1 {
		t.Errorf("HealthCheckFailures = %d, want 1", em.HealthCheckFailures.Load())
	}
}

func TestConnectionMetrics_Uptime(t *testing.T) {
	m := NewConnectionMetrics()

	time.Sleep(10 * time.Millisecond)

	uptime := m.Uptime()
	if uptime < 10*time.Millisecond {
		t.Errorf("Uptime = %v, want >= 10ms", uptime)
	}
}

func TestConnectionMetrics_LastConnectionTime(t *testing.T) {
	m := NewConnectionMetrics()

	// Should be zero initially
	if !m.LastConnectionTime().IsZero() {
		t.Error("LastConnectionTime should be zero initially")
	}

	m.RecordConnectionSuccess("localhost:4222", time.Millisecond)

	if m.LastConnectionTime().IsZero() {
		t.Error("LastConnectionTime should be set after connection")
	}
}

func TestConnectionMetrics_LastErrorTime(t *testing.T) {
	m := NewConnectionMetrics()

	// Should be zero initially
	if !m.LastErrorTime().IsZero() {
		t.Error("LastErrorTime should be zero initially")
	}

	m.RecordConnectionFailure("localhost:4222", errors.New("test"))

	if m.LastErrorTime().IsZero() {
		t.Error("LastErrorTime should be set after error")
	}
}

func TestConnectionMetrics_Summary(t *testing.T) {
	m := NewConnectionMetrics()

	m.RecordConnectionAttempt("localhost:4222")
	m.RecordConnectionSuccess("localhost:4222", time.Millisecond)
	m.RecordConnectionAttempt("localhost:4222")
	m.RecordConnectionFailure("localhost:4222", errors.New("test"))

	summary := m.Summary()

	if summary.ConnectionAttempts != 2 {
		t.Errorf("ConnectionAttempts = %d, want 2", summary.ConnectionAttempts)
	}
	if summary.ConnectionSuccesses != 1 {
		t.Errorf("ConnectionSuccesses = %d, want 1", summary.ConnectionSuccesses)
	}
	if summary.ConnectionFailures != 1 {
		t.Errorf("ConnectionFailures = %d, want 1", summary.ConnectionFailures)
	}
	if summary.ConnectedEndpoints != 1 {
		t.Errorf("ConnectedEndpoints = %d, want 1", summary.ConnectedEndpoints)
	}
}

func TestConnectionMetricsSummary_SuccessRate(t *testing.T) {
	tests := []struct {
		name     string
		success  int64
		failures int64
		want     float64
	}{
		{"no attempts", 0, 0, 1.0},
		{"all success", 10, 0, 1.0},
		{"all failure", 0, 10, 0.0},
		{"50/50", 5, 5, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := ConnectionMetricsSummary{
				ConnectionSuccesses: tt.success,
				ConnectionFailures:  tt.failures,
			}
			if got := summary.SuccessRate(); got != tt.want {
				t.Errorf("SuccessRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEndpointMetrics_RecordLatency(t *testing.T) {
	em := &EndpointMetrics{}
	em.MinLatency.Store(int64(time.Hour))

	em.RecordLatency(10 * time.Millisecond)
	em.RecordLatency(5 * time.Millisecond)
	em.RecordLatency(15 * time.Millisecond)

	if em.LatencyCount.Load() != 3 {
		t.Errorf("LatencyCount = %d, want 3", em.LatencyCount.Load())
	}
	if em.GetMinLatency() != 5*time.Millisecond {
		t.Errorf("MinLatency = %v, want 5ms", em.GetMinLatency())
	}
	if em.GetMaxLatency() != 15*time.Millisecond {
		t.Errorf("MaxLatency = %v, want 15ms", em.GetMaxLatency())
	}
}

func TestEndpointMetrics_AverageLatency(t *testing.T) {
	em := &EndpointMetrics{}
	em.MinLatency.Store(int64(time.Hour))

	// No samples
	if em.AverageLatency() != 0 {
		t.Errorf("AverageLatency with no samples = %v, want 0", em.AverageLatency())
	}

	// With samples
	em.RecordLatency(10 * time.Millisecond)
	em.RecordLatency(20 * time.Millisecond)

	avg := em.AverageLatency()
	if avg != 15*time.Millisecond {
		t.Errorf("AverageLatency = %v, want 15ms", avg)
	}
}

func TestEndpointMetrics_LatencyHistogram(t *testing.T) {
	em := &EndpointMetrics{}
	em.MinLatency.Store(int64(time.Hour))

	// Record latencies in different buckets
	em.RecordLatency(500 * time.Microsecond) // < 1ms, bucket 0
	em.RecordLatency(3 * time.Millisecond)   // < 5ms, bucket 1
	em.RecordLatency(200 * time.Millisecond) // < 250ms, bucket 6
	em.RecordLatency(1 * time.Second)        // > 500ms, bucket 7

	hist := em.LatencyHistogram()
	if len(hist) != 8 {
		t.Fatalf("LatencyHistogram length = %d, want 8", len(hist))
	}

	// Check some buckets
	if hist[0] != 1 {
		t.Errorf("bucket[0] = %d, want 1", hist[0])
	}
	if hist[7] != 1 {
		t.Errorf("bucket[7] = %d, want 1", hist[7])
	}
}

func TestEndpointMetrics_RecordError(t *testing.T) {
	em := &EndpointMetrics{}

	testErr := errors.New("test error")
	em.RecordError(testErr)

	if em.ErrorCount.Load() != 1 {
		t.Errorf("ErrorCount = %d, want 1", em.ErrorCount.Load())
	}

	err, errTime := em.GetLastError()
	if err != testErr {
		t.Error("LastError mismatch")
	}
	if errTime.IsZero() {
		t.Error("LastErrorTime should be set")
	}
}

func TestEndpointMetrics_SuccessRate(t *testing.T) {
	em := &EndpointMetrics{}

	// No attempts
	if em.SuccessRate() != 1.0 {
		t.Errorf("SuccessRate with no attempts = %v, want 1.0", em.SuccessRate())
	}

	// Some attempts
	em.ConnectionSuccesses.Store(7)
	em.ConnectionFailures.Store(3)

	if em.SuccessRate() != 0.7 {
		t.Errorf("SuccessRate = %v, want 0.7", em.SuccessRate())
	}
}

func TestEndpointMetrics_HealthCheckSuccessRate(t *testing.T) {
	em := &EndpointMetrics{}

	// No checks
	if em.HealthCheckSuccessRate() != 1.0 {
		t.Errorf("HealthCheckSuccessRate with no checks = %v, want 1.0", em.HealthCheckSuccessRate())
	}

	// Some checks
	em.HealthCheckSuccesses.Store(8)
	em.HealthCheckFailures.Store(2)

	if em.HealthCheckSuccessRate() != 0.8 {
		t.Errorf("HealthCheckSuccessRate = %v, want 0.8", em.HealthCheckSuccessRate())
	}
}

func TestEndpointMetrics_Summary(t *testing.T) {
	em := &EndpointMetrics{Address: "localhost:4222"}
	em.MinLatency.Store(int64(time.Hour))

	em.ConnectionSuccesses.Store(10)
	em.ConnectionFailures.Store(2)
	em.RecordLatency(10 * time.Millisecond)

	summary := em.Summary()

	if summary.Address != "localhost:4222" {
		t.Errorf("Address = %s, want localhost:4222", summary.Address)
	}
	if summary.ConnectionSuccesses != 10 {
		t.Errorf("ConnectionSuccesses = %d, want 10", summary.ConnectionSuccesses)
	}
	if summary.ConnectionFailures != 2 {
		t.Errorf("ConnectionFailures = %d, want 2", summary.ConnectionFailures)
	}
}

func TestMetricsCollectorCallbacks(t *testing.T) {
	metrics := NewConnectionMetrics()
	callbacks := MetricsCollectorCallbacks(metrics)

	endpoint := &Endpoint{
		Scheme: SchemeNATS,
		Host:   "localhost",
		Port:   4222,
	}

	// Test OnDisconnect
	callbacks.OnDisconnect(endpoint, nil)
	if metrics.Disconnections.Load() != 1 {
		t.Errorf("Disconnections = %d, want 1", metrics.Disconnections.Load())
	}

	// Test OnReconnect
	callbacks.OnReconnect(endpoint)
	if metrics.Reconnections.Load() != 1 {
		t.Errorf("Reconnections = %d, want 1", metrics.Reconnections.Load())
	}

	// Test OnError
	callbacks.OnError(endpoint, errors.New("test"))
	em := metrics.GetEndpointMetrics(endpoint.Address())
	if em.ErrorCount.Load() != 1 {
		t.Errorf("ErrorCount = %d, want 1", em.ErrorCount.Load())
	}

	// Test OnCircuitOpen
	callbacks.OnCircuitOpen(endpoint)
	if !em.CircuitCurrentlyOpen.Load() {
		t.Error("CircuitCurrentlyOpen should be true")
	}

	// Test OnCircuitClose
	callbacks.OnCircuitClose(endpoint)
	if em.CircuitCurrentlyOpen.Load() {
		t.Error("CircuitCurrentlyOpen should be false")
	}
}

// Mock connection provider for testing
type mockConnectionProvider struct {
	conn     *natsclient.Conn
	endpoint *Endpoint
}

func (m *mockConnectionProvider) Connection() *natsclient.Conn {
	return m.conn
}

func (m *mockConnectionProvider) ActiveEndpoint() *Endpoint {
	return m.endpoint
}

func TestNewNATSStatsCollector(t *testing.T) {
	metrics := NewConnectionMetrics()
	provider := &mockConnectionProvider{
		endpoint: &Endpoint{Host: "localhost", Port: 4222},
	}

	collector := NewNATSStatsCollector(metrics, provider, "localhost:4222")
	if collector == nil {
		t.Fatal("NewNATSStatsCollector returned nil")
	}
}

func TestNATSStatsCollector_Collect_NoConnection(t *testing.T) {
	metrics := NewConnectionMetrics()
	provider := &mockConnectionProvider{} // nil connection

	collector := NewNATSStatsCollector(metrics, provider, "localhost:4222")

	// Should not panic with nil connection
	collector.Collect()
}

func TestNATSStatsCollector_Reset(t *testing.T) {
	metrics := NewConnectionMetrics()
	provider := &mockConnectionProvider{}

	collector := NewNATSStatsCollector(metrics, provider, "localhost:4222")

	// Set some last stats
	collector.lastStats = NATSStats{
		InMsgs:  100,
		OutMsgs: 200,
	}

	collector.Reset()

	if collector.lastStats.InMsgs != 0 {
		t.Error("Reset should clear lastStats")
	}
}

func TestEndpointMetrics_GetMinLatency_NoSamples(t *testing.T) {
	em := &EndpointMetrics{}
	em.MinLatency.Store(int64(time.Hour))

	if em.GetMinLatency() != 0 {
		t.Errorf("GetMinLatency with no samples = %v, want 0", em.GetMinLatency())
	}
}
