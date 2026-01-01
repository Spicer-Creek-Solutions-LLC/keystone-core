package nats

import (
	"sync"
	"sync/atomic"
	"time"

	natsclient "github.com/nats-io/nats.go"
)

// ConnectionMetrics tracks connection-related metrics
type ConnectionMetrics struct {
	// Connection counts
	ConnectionAttempts  atomic.Int64
	ConnectionSuccesses atomic.Int64
	ConnectionFailures  atomic.Int64
	Disconnections      atomic.Int64
	Reconnections       atomic.Int64

	// Active state
	ConnectedEndpoints atomic.Int64

	// Timing
	startTime          time.Time
	lastConnectionTime atomic.Int64 // Unix nano
	lastErrorTime      atomic.Int64 // Unix nano

	// Per-endpoint metrics
	endpointMetrics map[string]*EndpointMetrics
	mu              sync.RWMutex
}

// EndpointMetrics tracks metrics for a single endpoint
type EndpointMetrics struct {
	Address string

	// Connection counts
	ConnectionAttempts  atomic.Int64
	ConnectionSuccesses atomic.Int64
	ConnectionFailures  atomic.Int64
	Disconnections      atomic.Int64
	Reconnections       atomic.Int64

	// Circuit breaker
	CircuitOpenCount    atomic.Int64
	CircuitCloseCount   atomic.Int64
	CircuitCurrentlyOpen atomic.Bool

	// Health checks
	HealthCheckCount    atomic.Int64
	HealthCheckSuccesses atomic.Int64
	HealthCheckFailures atomic.Int64

	// Latency tracking
	TotalLatency         atomic.Int64 // nanoseconds
	LatencyCount         atomic.Int64
	MinLatency           atomic.Int64 // nanoseconds
	MaxLatency           atomic.Int64 // nanoseconds
	latencyBuckets       [8]atomic.Int64 // Histogram buckets

	// Messages
	MessagesPublished atomic.Int64
	MessagesReceived  atomic.Int64
	BytesSent         atomic.Int64
	BytesReceived     atomic.Int64

	// Errors
	LastError      error
	LastErrorTime  time.Time
	ErrorCount     atomic.Int64
	errorMu        sync.RWMutex
}

// LatencyBuckets defines the latency histogram bucket boundaries (in ms)
var LatencyBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500}

// NewConnectionMetrics creates a new connection metrics collector
func NewConnectionMetrics() *ConnectionMetrics {
	return &ConnectionMetrics{
		startTime:       time.Now(),
		endpointMetrics: make(map[string]*EndpointMetrics),
	}
}

// GetOrCreateEndpointMetrics returns metrics for an endpoint, creating if needed
func (m *ConnectionMetrics) GetOrCreateEndpointMetrics(address string) *EndpointMetrics {
	m.mu.RLock()
	em, ok := m.endpointMetrics[address]
	m.mu.RUnlock()

	if ok {
		return em
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if em, ok = m.endpointMetrics[address]; ok {
		return em
	}

	em = &EndpointMetrics{Address: address}
	em.MinLatency.Store(int64(time.Hour)) // Initialize to large value
	m.endpointMetrics[address] = em
	return em
}

// GetEndpointMetrics returns metrics for an endpoint, nil if not found
func (m *ConnectionMetrics) GetEndpointMetrics(address string) *EndpointMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.endpointMetrics[address]
}

// GetAllEndpointMetrics returns metrics for all endpoints
func (m *ConnectionMetrics) GetAllEndpointMetrics() []*EndpointMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*EndpointMetrics, 0, len(m.endpointMetrics))
	for _, em := range m.endpointMetrics {
		result = append(result, em)
	}
	return result
}

// RecordConnectionAttempt records a connection attempt
func (m *ConnectionMetrics) RecordConnectionAttempt(address string) {
	m.ConnectionAttempts.Add(1)
	em := m.GetOrCreateEndpointMetrics(address)
	em.ConnectionAttempts.Add(1)
}

// RecordConnectionSuccess records a successful connection
func (m *ConnectionMetrics) RecordConnectionSuccess(address string, latency time.Duration) {
	now := time.Now()
	m.ConnectionSuccesses.Add(1)
	m.ConnectedEndpoints.Add(1)
	m.lastConnectionTime.Store(now.UnixNano())

	em := m.GetOrCreateEndpointMetrics(address)
	em.ConnectionSuccesses.Add(1)
	em.RecordLatency(latency)
}

// RecordConnectionFailure records a failed connection
func (m *ConnectionMetrics) RecordConnectionFailure(address string, err error) {
	now := time.Now()
	m.ConnectionFailures.Add(1)
	m.lastErrorTime.Store(now.UnixNano())

	em := m.GetOrCreateEndpointMetrics(address)
	em.ConnectionFailures.Add(1)
	em.RecordError(err)
}

// RecordDisconnection records a disconnection
func (m *ConnectionMetrics) RecordDisconnection(address string, err error) {
	m.Disconnections.Add(1)
	m.ConnectedEndpoints.Add(-1)

	em := m.GetOrCreateEndpointMetrics(address)
	em.Disconnections.Add(1)
	if err != nil {
		em.RecordError(err)
	}
}

// RecordReconnection records a reconnection
func (m *ConnectionMetrics) RecordReconnection(address string) {
	now := time.Now()
	m.Reconnections.Add(1)
	m.lastConnectionTime.Store(now.UnixNano())

	em := m.GetOrCreateEndpointMetrics(address)
	em.Reconnections.Add(1)
}

// RecordCircuitOpen records a circuit breaker opening
func (m *ConnectionMetrics) RecordCircuitOpen(address string) {
	em := m.GetOrCreateEndpointMetrics(address)
	em.CircuitOpenCount.Add(1)
	em.CircuitCurrentlyOpen.Store(true)
}

// RecordCircuitClose records a circuit breaker closing
func (m *ConnectionMetrics) RecordCircuitClose(address string) {
	em := m.GetOrCreateEndpointMetrics(address)
	em.CircuitCloseCount.Add(1)
	em.CircuitCurrentlyOpen.Store(false)
}

// RecordHealthCheck records a health check result
func (m *ConnectionMetrics) RecordHealthCheck(address string, success bool, latency time.Duration) {
	em := m.GetOrCreateEndpointMetrics(address)
	em.HealthCheckCount.Add(1)
	if success {
		em.HealthCheckSuccesses.Add(1)
		em.RecordLatency(latency)
	} else {
		em.HealthCheckFailures.Add(1)
	}
}

// Uptime returns the duration since metrics collection started
func (m *ConnectionMetrics) Uptime() time.Duration {
	return time.Since(m.startTime)
}

// LastConnectionTime returns the time of the last successful connection
func (m *ConnectionMetrics) LastConnectionTime() time.Time {
	nano := m.lastConnectionTime.Load()
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}

// LastErrorTime returns the time of the last error
func (m *ConnectionMetrics) LastErrorTime() time.Time {
	nano := m.lastErrorTime.Load()
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}

// Summary returns a summary of connection metrics
func (m *ConnectionMetrics) Summary() ConnectionMetricsSummary {
	return ConnectionMetricsSummary{
		Uptime:              m.Uptime(),
		ConnectionAttempts:  m.ConnectionAttempts.Load(),
		ConnectionSuccesses: m.ConnectionSuccesses.Load(),
		ConnectionFailures:  m.ConnectionFailures.Load(),
		Disconnections:      m.Disconnections.Load(),
		Reconnections:       m.Reconnections.Load(),
		ConnectedEndpoints:  m.ConnectedEndpoints.Load(),
		LastConnectionTime:  m.LastConnectionTime(),
		LastErrorTime:       m.LastErrorTime(),
	}
}

// ConnectionMetricsSummary is a snapshot of connection metrics
type ConnectionMetricsSummary struct {
	Uptime              time.Duration
	ConnectionAttempts  int64
	ConnectionSuccesses int64
	ConnectionFailures  int64
	Disconnections      int64
	Reconnections       int64
	ConnectedEndpoints  int64
	LastConnectionTime  time.Time
	LastErrorTime       time.Time
}

// SuccessRate returns the connection success rate (0.0 to 1.0)
func (s *ConnectionMetricsSummary) SuccessRate() float64 {
	total := s.ConnectionSuccesses + s.ConnectionFailures
	if total == 0 {
		return 1.0
	}
	return float64(s.ConnectionSuccesses) / float64(total)
}

// RecordLatency records a latency measurement
func (em *EndpointMetrics) RecordLatency(latency time.Duration) {
	nanos := int64(latency)
	em.TotalLatency.Add(nanos)
	em.LatencyCount.Add(1)

	// Update min/max using CAS loops
	for {
		old := em.MinLatency.Load()
		if nanos >= old {
			break
		}
		if em.MinLatency.CompareAndSwap(old, nanos) {
			break
		}
	}

	for {
		old := em.MaxLatency.Load()
		if nanos <= old {
			break
		}
		if em.MaxLatency.CompareAndSwap(old, nanos) {
			break
		}
	}

	// Update histogram bucket
	millis := latency.Milliseconds()
	bucket := 7 // Last bucket
	for i, threshold := range LatencyBuckets {
		if float64(millis) <= threshold {
			bucket = i
			break
		}
	}
	em.latencyBuckets[bucket].Add(1)
}

// RecordError records an error
func (em *EndpointMetrics) RecordError(err error) {
	em.ErrorCount.Add(1)
	em.errorMu.Lock()
	em.LastError = err
	em.LastErrorTime = time.Now()
	em.errorMu.Unlock()
}

// AverageLatency returns the average connection latency
func (em *EndpointMetrics) AverageLatency() time.Duration {
	count := em.LatencyCount.Load()
	if count == 0 {
		return 0
	}
	return time.Duration(em.TotalLatency.Load() / count)
}

// GetMinLatency returns the minimum latency observed
func (em *EndpointMetrics) GetMinLatency() time.Duration {
	val := em.MinLatency.Load()
	if val == int64(time.Hour) {
		return 0 // No samples yet
	}
	return time.Duration(val)
}

// GetMaxLatency returns the maximum latency observed
func (em *EndpointMetrics) GetMaxLatency() time.Duration {
	return time.Duration(em.MaxLatency.Load())
}

// LatencyHistogram returns the latency histogram bucket counts
func (em *EndpointMetrics) LatencyHistogram() []int64 {
	result := make([]int64, len(em.latencyBuckets))
	for i := range em.latencyBuckets {
		result[i] = em.latencyBuckets[i].Load()
	}
	return result
}

// GetLastError returns the last error and when it occurred
func (em *EndpointMetrics) GetLastError() (error, time.Time) {
	em.errorMu.RLock()
	defer em.errorMu.RUnlock()
	return em.LastError, em.LastErrorTime
}

// SuccessRate returns the connection success rate
func (em *EndpointMetrics) SuccessRate() float64 {
	total := em.ConnectionSuccesses.Load() + em.ConnectionFailures.Load()
	if total == 0 {
		return 1.0
	}
	return float64(em.ConnectionSuccesses.Load()) / float64(total)
}

// HealthCheckSuccessRate returns the health check success rate
func (em *EndpointMetrics) HealthCheckSuccessRate() float64 {
	total := em.HealthCheckSuccesses.Load() + em.HealthCheckFailures.Load()
	if total == 0 {
		return 1.0
	}
	return float64(em.HealthCheckSuccesses.Load()) / float64(total)
}

// Summary returns a summary of endpoint metrics
func (em *EndpointMetrics) Summary() EndpointMetricsSummary {
	lastErr, lastErrTime := em.GetLastError()
	return EndpointMetricsSummary{
		Address:              em.Address,
		ConnectionAttempts:   em.ConnectionAttempts.Load(),
		ConnectionSuccesses:  em.ConnectionSuccesses.Load(),
		ConnectionFailures:   em.ConnectionFailures.Load(),
		Disconnections:       em.Disconnections.Load(),
		Reconnections:        em.Reconnections.Load(),
		CircuitOpenCount:     em.CircuitOpenCount.Load(),
		CircuitCloseCount:    em.CircuitCloseCount.Load(),
		CircuitCurrentlyOpen: em.CircuitCurrentlyOpen.Load(),
		HealthCheckCount:     em.HealthCheckCount.Load(),
		HealthCheckSuccesses: em.HealthCheckSuccesses.Load(),
		HealthCheckFailures:  em.HealthCheckFailures.Load(),
		AverageLatency:       em.AverageLatency(),
		MinLatency:           em.GetMinLatency(),
		MaxLatency:           em.GetMaxLatency(),
		MessagesPublished:    em.MessagesPublished.Load(),
		MessagesReceived:     em.MessagesReceived.Load(),
		BytesSent:            em.BytesSent.Load(),
		BytesReceived:        em.BytesReceived.Load(),
		ErrorCount:           em.ErrorCount.Load(),
		LastError:            lastErr,
		LastErrorTime:        lastErrTime,
		SuccessRate:          em.SuccessRate(),
	}
}

// EndpointMetricsSummary is a snapshot of endpoint metrics
type EndpointMetricsSummary struct {
	Address              string
	ConnectionAttempts   int64
	ConnectionSuccesses  int64
	ConnectionFailures   int64
	Disconnections       int64
	Reconnections        int64
	CircuitOpenCount     int64
	CircuitCloseCount    int64
	CircuitCurrentlyOpen bool
	HealthCheckCount     int64
	HealthCheckSuccesses int64
	HealthCheckFailures  int64
	AverageLatency       time.Duration
	MinLatency           time.Duration
	MaxLatency           time.Duration
	MessagesPublished    int64
	MessagesReceived     int64
	BytesSent            int64
	BytesReceived        int64
	ErrorCount           int64
	LastError            error
	LastErrorTime        time.Time
	SuccessRate          float64
}

// MetricsCollectorCallbacks returns callbacks that record metrics
func MetricsCollectorCallbacks(metrics *ConnectionMetrics) ConnectionCallbacks {
	return ConnectionCallbacks{
		OnConnect: func(endpoint *Endpoint) {
			// Note: latency is recorded separately in RecordConnectionSuccess
		},
		OnDisconnect: func(endpoint *Endpoint, err error) {
			metrics.RecordDisconnection(endpoint.Address(), err)
		},
		OnReconnect: func(endpoint *Endpoint) {
			metrics.RecordReconnection(endpoint.Address())
		},
		OnError: func(endpoint *Endpoint, err error) {
			em := metrics.GetOrCreateEndpointMetrics(endpoint.Address())
			em.RecordError(err)
		},
		OnCircuitOpen: func(endpoint *Endpoint) {
			metrics.RecordCircuitOpen(endpoint.Address())
		},
		OnCircuitClose: func(endpoint *Endpoint) {
			metrics.RecordCircuitClose(endpoint.Address())
		},
	}
}

// NATSStatsCollector collects metrics from NATS connection stats
type NATSStatsCollector struct {
	metrics   *ConnectionMetrics
	conn      ConnectionProvider
	address   string
	mu        sync.Mutex
	lastStats NATSStats
}

// ConnectionProvider provides access to a NATS connection
type ConnectionProvider interface {
	Connection() *natsclient.Conn
	ActiveEndpoint() *Endpoint
}

// NATSStats holds a snapshot of NATS statistics
type NATSStats struct {
	InMsgs     uint64
	OutMsgs    uint64
	InBytes    uint64
	OutBytes   uint64
	Reconnects uint64
}

// NewNATSStatsCollector creates a collector that updates metrics from NATS stats
func NewNATSStatsCollector(metrics *ConnectionMetrics, conn ConnectionProvider, address string) *NATSStatsCollector {
	return &NATSStatsCollector{
		metrics: metrics,
		conn:    conn,
		address: address,
	}
}

// Collect collects current NATS statistics
func (c *NATSStatsCollector) Collect() {
	conn := c.conn.Connection()
	if conn == nil {
		return
	}

	stats := conn.Stats()
	current := NATSStats{
		InMsgs:     stats.InMsgs,
		OutMsgs:    stats.OutMsgs,
		InBytes:    stats.InBytes,
		OutBytes:   stats.OutBytes,
		Reconnects: stats.Reconnects,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Calculate deltas
	address := c.address
	if endpoint := c.conn.ActiveEndpoint(); endpoint != nil {
		address = endpoint.Address()
	}

	em := c.metrics.GetOrCreateEndpointMetrics(address)

	// Add deltas to endpoint metrics
	if current.InMsgs >= c.lastStats.InMsgs {
		em.MessagesReceived.Add(int64(current.InMsgs - c.lastStats.InMsgs))
	}
	if current.OutMsgs >= c.lastStats.OutMsgs {
		em.MessagesPublished.Add(int64(current.OutMsgs - c.lastStats.OutMsgs))
	}
	if current.InBytes >= c.lastStats.InBytes {
		em.BytesReceived.Add(int64(current.InBytes - c.lastStats.InBytes))
	}
	if current.OutBytes >= c.lastStats.OutBytes {
		em.BytesSent.Add(int64(current.OutBytes - c.lastStats.OutBytes))
	}

	c.lastStats = current
}

// Reset resets the last stats baseline
func (c *NATSStatsCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastStats = NATSStats{}
}
