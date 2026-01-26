package nats

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Phase 9: Observability - T9.1 Connection Metrics
// ============================================================================

// MetricLabels defines common label sets for metrics
type MetricLabels struct {
	Endpoint      string
	Strategy      string
	Status        string
	Direction     string
	SubjectPrefix string
	Error         string
	Type          string
	From          string
	To            string
	Cluster       string
	AgentID       string
	ServerID      string
	Hop           string
}

// ConnectionMetricType defines the type of connection metric
type ConnectionMetricType string

const (
	MetricConnectionsTotal       ConnectionMetricType = "connections_total"
	MetricConnectionLatency      ConnectionMetricType = "connection_latency_seconds"
	MetricConnectionErrors       ConnectionMetricType = "connection_errors_total"
	MetricMessagesTotal          ConnectionMetricType = "messages_total"
	MetricMessageBytes           ConnectionMetricType = "message_bytes_total"
	MetricBufferSize             ConnectionMetricType = "buffer_size"
	MetricBufferOverflow         ConnectionMetricType = "buffer_overflow_total"
	MetricReconnections          ConnectionMetricType = "reconnections_total"
	MetricFailovers              ConnectionMetricType = "failovers_total"
	MetricLeafNodes              ConnectionMetricType = "leaf_nodes_total"
	MetricGatewayConnections     ConnectionMetricType = "gateway_connections_total"
	MetricGatewayLatency         ConnectionMetricType = "gateway_latency_seconds"
	MetricCircuitBreakerState    ConnectionMetricType = "circuit_breaker_state"
	MetricDegradationMode        ConnectionMetricType = "degradation_mode"
	MetricDeliveryPending        ConnectionMetricType = "delivery_pending"
	MetricDeliveryAcked          ConnectionMetricType = "delivery_acked_total"
	MetricDeliveryFailed         ConnectionMetricType = "delivery_failed_total"
	MetricDuplicatesDetected     ConnectionMetricType = "duplicates_detected_total"
	MetricBootstrapRequests      ConnectionMetricType = "bootstrap_requests_total"
	MetricCredentialsIssued      ConnectionMetricType = "credentials_issued_total"
	MetricCoordinationRPCs       ConnectionMetricType = "coordination_rpcs_total"
	MetricCoordinationLatency    ConnectionMetricType = "coordination_latency_seconds"
)

// MetricValue represents a metric value with labels
type MetricValue struct {
	Value     float64
	Labels    MetricLabels
	Timestamp time.Time
}

// ObservabilityConfig holds observability configuration
type ObservabilityConfig struct {
	// Enabled enables observability collection
	Enabled bool

	// MetricsPrefix is the prefix for all metrics
	MetricsPrefix string

	// HistogramBuckets for latency metrics
	HistogramBuckets []float64

	// CollectionInterval for periodic metrics
	CollectionInterval time.Duration

	// RetentionDuration for historical data
	RetentionDuration time.Duration

	// MaxTimeSeriesPerMetric limits cardinality
	MaxTimeSeriesPerMetric int

	// EnableDetailedLatency enables per-hop latency
	EnableDetailedLatency bool

	// EnableMessageTracing enables message flow tracing
	EnableMessageTracing bool
}

// DefaultObservabilityConfig returns default configuration
func DefaultObservabilityConfig() *ObservabilityConfig {
	return &ObservabilityConfig{
		Enabled:                true,
		MetricsPrefix:          "kscore_nats",
		HistogramBuckets:       []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		CollectionInterval:     10 * time.Second,
		RetentionDuration:      time.Hour,
		MaxTimeSeriesPerMetric: 1000,
		EnableDetailedLatency:  true,
		EnableMessageTracing:   false,
	}
}

// NATSMetricsCollector collects NATS mesh metrics
type NATSMetricsCollector struct {
	config *ObservabilityConfig

	// Connection metrics
	connectionsTotal   map[string]*atomic.Int64 // endpoint:strategy:status -> count
	connectionLatency  map[string]*LatencyHistogram
	connectionErrors   map[string]*atomic.Int64 // endpoint:error -> count
	reconnections      map[string]*atomic.Int64 // endpoint -> count
	failovers          map[string]*atomic.Int64 // from:to -> count

	// Message metrics
	messagesTotal    map[string]*atomic.Int64 // direction:subject_prefix -> count
	messageBytesTotal map[string]*atomic.Int64 // direction -> bytes

	// Buffer metrics
	bufferSize     map[string]*atomic.Int64 // type -> size
	bufferOverflow *atomic.Int64

	// Topology metrics
	leafNodesTotal       map[string]*atomic.Int64 // hub -> count
	gatewayConnections   map[string]*atomic.Int64 // local:remote -> count
	gatewayLatency       map[string]*LatencyHistogram

	// Reliability metrics
	circuitBreakerStates map[string]CircuitState
	degradationModes     map[string]DegradationMode
	deliveryPending      *atomic.Int64
	deliveryAcked        *atomic.Int64
	deliveryFailed       *atomic.Int64
	duplicatesDetected   *atomic.Int64

	// Bootstrap metrics
	bootstrapRequests   map[string]*atomic.Int64 // status -> count
	credentialsIssued   map[string]*atomic.Int64 // type -> count

	// Coordination metrics
	coordinationRPCs     map[string]*atomic.Int64 // method:status -> count
	coordinationLatency  map[string]*LatencyHistogram

	mu sync.RWMutex
}

// LatencyHistogram tracks latency distribution
type LatencyHistogram struct {
	buckets   []float64
	counts    []int64
	sum       float64
	count     int64
	min       float64
	max       float64
	mu        sync.Mutex
}

// NewLatencyHistogram creates a new histogram
func NewLatencyHistogram(buckets []float64) *LatencyHistogram {
	return &LatencyHistogram{
		buckets: buckets,
		counts:  make([]int64, len(buckets)+1), // +1 for overflow bucket
		min:     -1,
		max:     -1,
	}
}

// Observe records a latency observation
func (h *LatencyHistogram) Observe(seconds float64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.sum += seconds
	h.count++

	if h.min < 0 || seconds < h.min {
		h.min = seconds
	}
	if seconds > h.max {
		h.max = seconds
	}

	// Find bucket
	for i, bound := range h.buckets {
		if seconds <= bound {
			h.counts[i]++
			return
		}
	}
	h.counts[len(h.buckets)]++ // overflow bucket
}

// GetStats returns histogram statistics
func (h *LatencyHistogram) GetStats() LatencyStats {
	h.mu.Lock()
	defer h.mu.Unlock()

	stats := LatencyStats{
		Count: h.count,
		Sum:   h.sum,
		Min:   h.min,
		Max:   h.max,
	}
	if h.count > 0 {
		stats.Avg = h.sum / float64(h.count)
	}

	// Calculate percentiles
	if h.count > 0 {
		stats.P50 = h.percentile(0.50)
		stats.P95 = h.percentile(0.95)
		stats.P99 = h.percentile(0.99)
	}

	return stats
}

func (h *LatencyHistogram) percentile(p float64) float64 {
	target := int64(float64(h.count) * p)
	var cumulative int64
	for i, count := range h.counts {
		cumulative += count
		if cumulative >= target {
			if i < len(h.buckets) {
				return h.buckets[i]
			}
			return h.max
		}
	}
	return h.max
}

// LatencyStats holds latency statistics
type LatencyStats struct {
	Count int64
	Sum   float64
	Avg   float64
	Min   float64
	Max   float64
	P50   float64
	P95   float64
	P99   float64
}

// NewNATSMetricsCollector creates a new metrics collector
func NewNATSMetricsCollector(config *ObservabilityConfig) *NATSMetricsCollector {
	if config == nil {
		config = DefaultObservabilityConfig()
	}

	return &NATSMetricsCollector{
		config:               config,
		connectionsTotal:     make(map[string]*atomic.Int64),
		connectionLatency:    make(map[string]*LatencyHistogram),
		connectionErrors:     make(map[string]*atomic.Int64),
		reconnections:        make(map[string]*atomic.Int64),
		failovers:            make(map[string]*atomic.Int64),
		messagesTotal:        make(map[string]*atomic.Int64),
		messageBytesTotal:    make(map[string]*atomic.Int64),
		bufferSize:           make(map[string]*atomic.Int64),
		bufferOverflow:       &atomic.Int64{},
		leafNodesTotal:       make(map[string]*atomic.Int64),
		gatewayConnections:   make(map[string]*atomic.Int64),
		gatewayLatency:       make(map[string]*LatencyHistogram),
		circuitBreakerStates: make(map[string]CircuitState),
		degradationModes:     make(map[string]DegradationMode),
		deliveryPending:      &atomic.Int64{},
		deliveryAcked:        &atomic.Int64{},
		deliveryFailed:       &atomic.Int64{},
		duplicatesDetected:   &atomic.Int64{},
		bootstrapRequests:    make(map[string]*atomic.Int64),
		credentialsIssued:    make(map[string]*atomic.Int64),
		coordinationRPCs:     make(map[string]*atomic.Int64),
		coordinationLatency:  make(map[string]*LatencyHistogram),
	}
}

// RecordConnection records a connection event
func (c *NATSMetricsCollector) RecordConnection(endpoint, strategy, status string) {
	key := endpoint + ":" + strategy + ":" + status
	c.mu.Lock()
	counter, exists := c.connectionsTotal[key]
	if !exists {
		counter = &atomic.Int64{}
		c.connectionsTotal[key] = counter
	}
	c.mu.Unlock()
	counter.Add(1)
}

// RecordConnectionLatency records connection latency
func (c *NATSMetricsCollector) RecordConnectionLatency(endpoint string, latency time.Duration) {
	c.mu.Lock()
	hist, exists := c.connectionLatency[endpoint]
	if !exists {
		hist = NewLatencyHistogram(c.config.HistogramBuckets)
		c.connectionLatency[endpoint] = hist
	}
	c.mu.Unlock()
	hist.Observe(latency.Seconds())
}

// RecordConnectionError records a connection error
func (c *NATSMetricsCollector) RecordConnectionError(endpoint, errorType string) {
	key := endpoint + ":" + errorType
	c.mu.Lock()
	counter, exists := c.connectionErrors[key]
	if !exists {
		counter = &atomic.Int64{}
		c.connectionErrors[key] = counter
	}
	c.mu.Unlock()
	counter.Add(1)
}

// RecordReconnection records a reconnection event
func (c *NATSMetricsCollector) RecordReconnection(endpoint string) {
	c.mu.Lock()
	counter, exists := c.reconnections[endpoint]
	if !exists {
		counter = &atomic.Int64{}
		c.reconnections[endpoint] = counter
	}
	c.mu.Unlock()
	counter.Add(1)
}

// RecordFailover records a failover event
func (c *NATSMetricsCollector) RecordFailover(from, to string) {
	key := from + ":" + to
	c.mu.Lock()
	counter, exists := c.failovers[key]
	if !exists {
		counter = &atomic.Int64{}
		c.failovers[key] = counter
	}
	c.mu.Unlock()
	counter.Add(1)
}

// RecordMessage records a message event
func (c *NATSMetricsCollector) RecordMessage(direction, subjectPrefix string, bytes int64) {
	key := direction + ":" + subjectPrefix
	c.mu.Lock()
	counter, exists := c.messagesTotal[key]
	if !exists {
		counter = &atomic.Int64{}
		c.messagesTotal[key] = counter
	}
	bytesCounter, bytesExists := c.messageBytesTotal[direction]
	if !bytesExists {
		bytesCounter = &atomic.Int64{}
		c.messageBytesTotal[direction] = bytesCounter
	}
	c.mu.Unlock()
	counter.Add(1)
	bytesCounter.Add(bytes)
}

// RecordBufferSize records buffer size
func (c *NATSMetricsCollector) RecordBufferSize(bufferType string, size int64) {
	c.mu.Lock()
	counter, exists := c.bufferSize[bufferType]
	if !exists {
		counter = &atomic.Int64{}
		c.bufferSize[bufferType] = counter
	}
	c.mu.Unlock()
	counter.Store(size)
}

// RecordBufferOverflow records a buffer overflow event
func (c *NATSMetricsCollector) RecordBufferOverflow() {
	c.bufferOverflow.Add(1)
}

// RecordLeafNode records a leaf node connection
func (c *NATSMetricsCollector) RecordLeafNode(hub string, delta int64) {
	c.mu.Lock()
	counter, exists := c.leafNodesTotal[hub]
	if !exists {
		counter = &atomic.Int64{}
		c.leafNodesTotal[hub] = counter
	}
	c.mu.Unlock()
	counter.Add(delta)
}

// RecordGatewayConnection records a gateway connection
func (c *NATSMetricsCollector) RecordGatewayConnection(localCluster, remoteCluster string, delta int64) {
	key := localCluster + ":" + remoteCluster
	c.mu.Lock()
	counter, exists := c.gatewayConnections[key]
	if !exists {
		counter = &atomic.Int64{}
		c.gatewayConnections[key] = counter
	}
	c.mu.Unlock()
	counter.Add(delta)
}

// RecordGatewayLatency records gateway latency
func (c *NATSMetricsCollector) RecordGatewayLatency(localCluster, remoteCluster string, latency time.Duration) {
	key := localCluster + ":" + remoteCluster
	c.mu.Lock()
	hist, exists := c.gatewayLatency[key]
	if !exists {
		hist = NewLatencyHistogram(c.config.HistogramBuckets)
		c.gatewayLatency[key] = hist
	}
	c.mu.Unlock()
	hist.Observe(latency.Seconds())
}

// RecordCircuitBreakerState records circuit breaker state
func (c *NATSMetricsCollector) RecordCircuitBreakerState(name string, state CircuitState) {
	c.mu.Lock()
	c.circuitBreakerStates[name] = state
	c.mu.Unlock()
}

// RecordDegradationMode records degradation mode
func (c *NATSMetricsCollector) RecordDegradationMode(component string, mode DegradationMode) {
	c.mu.Lock()
	c.degradationModes[component] = mode
	c.mu.Unlock()
}

// RecordDeliveryPending records pending delivery count
func (c *NATSMetricsCollector) RecordDeliveryPending(count int64) {
	c.deliveryPending.Store(count)
}

// RecordDeliveryAcked records a delivery acknowledgment
func (c *NATSMetricsCollector) RecordDeliveryAcked() {
	c.deliveryAcked.Add(1)
}

// RecordDeliveryFailed records a delivery failure
func (c *NATSMetricsCollector) RecordDeliveryFailed() {
	c.deliveryFailed.Add(1)
}

// RecordDuplicateDetected records a duplicate detection
func (c *NATSMetricsCollector) RecordDuplicateDetected() {
	c.duplicatesDetected.Add(1)
}

// RecordBootstrapRequest records a bootstrap request
func (c *NATSMetricsCollector) RecordBootstrapRequest(status string) {
	c.mu.Lock()
	counter, exists := c.bootstrapRequests[status]
	if !exists {
		counter = &atomic.Int64{}
		c.bootstrapRequests[status] = counter
	}
	c.mu.Unlock()
	counter.Add(1)
}

// RecordCredentialIssued records a credential issuance
func (c *NATSMetricsCollector) RecordCredentialIssued(credType string) {
	c.mu.Lock()
	counter, exists := c.credentialsIssued[credType]
	if !exists {
		counter = &atomic.Int64{}
		c.credentialsIssued[credType] = counter
	}
	c.mu.Unlock()
	counter.Add(1)
}

// RecordCoordinationRPC records a coordination RPC
func (c *NATSMetricsCollector) RecordCoordinationRPC(method, status string) {
	key := method + ":" + status
	c.mu.Lock()
	counter, exists := c.coordinationRPCs[key]
	if !exists {
		counter = &atomic.Int64{}
		c.coordinationRPCs[key] = counter
	}
	c.mu.Unlock()
	counter.Add(1)
}

// RecordCoordinationLatency records coordination RPC latency
func (c *NATSMetricsCollector) RecordCoordinationLatency(method string, latency time.Duration) {
	c.mu.Lock()
	hist, exists := c.coordinationLatency[method]
	if !exists {
		hist = NewLatencyHistogram(c.config.HistogramBuckets)
		c.coordinationLatency[method] = hist
	}
	c.mu.Unlock()
	hist.Observe(latency.Seconds())
}

// ObservabilityStats holds all observability statistics
type ObservabilityStats struct {
	// Connection stats
	TotalConnections      int64
	TotalConnectionErrors int64
	TotalReconnections    int64
	TotalFailovers        int64
	ConnectionLatency     map[string]LatencyStats

	// Message stats
	TotalMessagesSent     int64
	TotalMessagesReceived int64
	TotalBytesSent        int64
	TotalBytesReceived    int64

	// Buffer stats
	BufferSizes       map[string]int64
	TotalBufferOverflow int64

	// Topology stats
	LeafNodeCounts     map[string]int64
	GatewayConnections map[string]int64
	GatewayLatency     map[string]LatencyStats

	// Reliability stats
	CircuitBreakerStates map[string]string
	DegradationModes     map[string]string
	DeliveryPending      int64
	DeliveryAcked        int64
	DeliveryFailed       int64
	DuplicatesDetected   int64

	// Bootstrap stats
	BootstrapRequests map[string]int64
	CredentialsIssued map[string]int64

	// Coordination stats
	CoordinationRPCs    map[string]int64
	CoordinationLatency map[string]LatencyStats
}

// GetStats returns all observability statistics
func (c *NATSMetricsCollector) GetStats() ObservabilityStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := ObservabilityStats{
		ConnectionLatency:    make(map[string]LatencyStats),
		BufferSizes:          make(map[string]int64),
		LeafNodeCounts:       make(map[string]int64),
		GatewayConnections:   make(map[string]int64),
		GatewayLatency:       make(map[string]LatencyStats),
		CircuitBreakerStates: make(map[string]string),
		DegradationModes:     make(map[string]string),
		BootstrapRequests:    make(map[string]int64),
		CredentialsIssued:    make(map[string]int64),
		CoordinationRPCs:     make(map[string]int64),
		CoordinationLatency:  make(map[string]LatencyStats),
	}

	// Connection stats
	for _, counter := range c.connectionsTotal {
		stats.TotalConnections += counter.Load()
	}
	for _, counter := range c.connectionErrors {
		stats.TotalConnectionErrors += counter.Load()
	}
	for _, counter := range c.reconnections {
		stats.TotalReconnections += counter.Load()
	}
	for _, counter := range c.failovers {
		stats.TotalFailovers += counter.Load()
	}
	for endpoint, hist := range c.connectionLatency {
		stats.ConnectionLatency[endpoint] = hist.GetStats()
	}

	// Message stats
	for key, counter := range c.messagesTotal {
		if len(key) > 5 && key[:5] == "sent:" {
			stats.TotalMessagesSent += counter.Load()
		} else if len(key) > 9 && key[:9] == "received:" {
			stats.TotalMessagesReceived += counter.Load()
		}
	}
	for direction, counter := range c.messageBytesTotal {
		if direction == "sent" {
			stats.TotalBytesSent = counter.Load()
		} else if direction == "received" {
			stats.TotalBytesReceived = counter.Load()
		}
	}

	// Buffer stats
	for bufType, counter := range c.bufferSize {
		stats.BufferSizes[bufType] = counter.Load()
	}
	stats.TotalBufferOverflow = c.bufferOverflow.Load()

	// Topology stats
	for hub, counter := range c.leafNodesTotal {
		stats.LeafNodeCounts[hub] = counter.Load()
	}
	for key, counter := range c.gatewayConnections {
		stats.GatewayConnections[key] = counter.Load()
	}
	for key, hist := range c.gatewayLatency {
		stats.GatewayLatency[key] = hist.GetStats()
	}

	// Reliability stats
	for name, state := range c.circuitBreakerStates {
		stats.CircuitBreakerStates[name] = string(state)
	}
	for component, mode := range c.degradationModes {
		stats.DegradationModes[component] = string(mode)
	}
	stats.DeliveryPending = c.deliveryPending.Load()
	stats.DeliveryAcked = c.deliveryAcked.Load()
	stats.DeliveryFailed = c.deliveryFailed.Load()
	stats.DuplicatesDetected = c.duplicatesDetected.Load()

	// Bootstrap stats
	for status, counter := range c.bootstrapRequests {
		stats.BootstrapRequests[status] = counter.Load()
	}
	for credType, counter := range c.credentialsIssued {
		stats.CredentialsIssued[credType] = counter.Load()
	}

	// Coordination stats
	for key, counter := range c.coordinationRPCs {
		stats.CoordinationRPCs[key] = counter.Load()
	}
	for method, hist := range c.coordinationLatency {
		stats.CoordinationLatency[method] = hist.GetStats()
	}

	return stats
}

// PrometheusExporter exports metrics in Prometheus format
type PrometheusExporter struct {
	collector *NATSMetricsCollector
	prefix    string
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter(collector *NATSMetricsCollector) *PrometheusExporter {
	prefix := "kscore_nats"
	if collector.config != nil && collector.config.MetricsPrefix != "" {
		prefix = collector.config.MetricsPrefix
	}
	return &PrometheusExporter{
		collector: collector,
		prefix:    prefix,
	}
}

// Export exports metrics in Prometheus text format
func (e *PrometheusExporter) Export() string {
	var output string

	e.collector.mu.RLock()
	defer e.collector.mu.RUnlock()

	// Connection metrics
	output += "# HELP " + e.prefix + "_connections_total Total number of connections\n"
	output += "# TYPE " + e.prefix + "_connections_total counter\n"
	for key, counter := range e.collector.connectionsTotal {
		parts := splitKey(key)
		if len(parts) >= 3 {
			output += e.prefix + "_connections_total{endpoint=\"" + parts[0] + "\",strategy=\"" + parts[1] + "\",status=\"" + parts[2] + "\"} " + formatInt64(counter.Load()) + "\n"
		}
	}

	output += "# HELP " + e.prefix + "_connection_errors_total Total number of connection errors\n"
	output += "# TYPE " + e.prefix + "_connection_errors_total counter\n"
	for key, counter := range e.collector.connectionErrors {
		parts := splitKey(key)
		if len(parts) >= 2 {
			output += e.prefix + "_connection_errors_total{endpoint=\"" + parts[0] + "\",error=\"" + parts[1] + "\"} " + formatInt64(counter.Load()) + "\n"
		}
	}

	output += "# HELP " + e.prefix + "_reconnections_total Total number of reconnections\n"
	output += "# TYPE " + e.prefix + "_reconnections_total counter\n"
	for endpoint, counter := range e.collector.reconnections {
		output += e.prefix + "_reconnections_total{endpoint=\"" + endpoint + "\"} " + formatInt64(counter.Load()) + "\n"
	}

	output += "# HELP " + e.prefix + "_failovers_total Total number of failovers\n"
	output += "# TYPE " + e.prefix + "_failovers_total counter\n"
	for key, counter := range e.collector.failovers {
		parts := splitKey(key)
		if len(parts) >= 2 {
			output += e.prefix + "_failovers_total{from=\"" + parts[0] + "\",to=\"" + parts[1] + "\"} " + formatInt64(counter.Load()) + "\n"
		}
	}

	// Message metrics
	output += "# HELP " + e.prefix + "_messages_total Total number of messages\n"
	output += "# TYPE " + e.prefix + "_messages_total counter\n"
	for key, counter := range e.collector.messagesTotal {
		parts := splitKey(key)
		if len(parts) >= 2 {
			output += e.prefix + "_messages_total{direction=\"" + parts[0] + "\",subject_prefix=\"" + parts[1] + "\"} " + formatInt64(counter.Load()) + "\n"
		}
	}

	output += "# HELP " + e.prefix + "_message_bytes_total Total bytes of messages\n"
	output += "# TYPE " + e.prefix + "_message_bytes_total counter\n"
	for direction, counter := range e.collector.messageBytesTotal {
		output += e.prefix + "_message_bytes_total{direction=\"" + direction + "\"} " + formatInt64(counter.Load()) + "\n"
	}

	// Buffer metrics
	output += "# HELP " + e.prefix + "_buffer_size Current buffer size\n"
	output += "# TYPE " + e.prefix + "_buffer_size gauge\n"
	for bufType, counter := range e.collector.bufferSize {
		output += e.prefix + "_buffer_size{type=\"" + bufType + "\"} " + formatInt64(counter.Load()) + "\n"
	}

	output += "# HELP " + e.prefix + "_buffer_overflow_total Total buffer overflows\n"
	output += "# TYPE " + e.prefix + "_buffer_overflow_total counter\n"
	output += e.prefix + "_buffer_overflow_total " + formatInt64(e.collector.bufferOverflow.Load()) + "\n"

	// Topology metrics
	output += "# HELP " + e.prefix + "_leaf_nodes_total Total leaf nodes\n"
	output += "# TYPE " + e.prefix + "_leaf_nodes_total gauge\n"
	for hub, counter := range e.collector.leafNodesTotal {
		output += e.prefix + "_leaf_nodes_total{hub=\"" + hub + "\"} " + formatInt64(counter.Load()) + "\n"
	}

	output += "# HELP " + e.prefix + "_gateway_connections_total Total gateway connections\n"
	output += "# TYPE " + e.prefix + "_gateway_connections_total gauge\n"
	for key, counter := range e.collector.gatewayConnections {
		parts := splitKey(key)
		if len(parts) >= 2 {
			output += e.prefix + "_gateway_connections_total{local_cluster=\"" + parts[0] + "\",remote_cluster=\"" + parts[1] + "\"} " + formatInt64(counter.Load()) + "\n"
		}
	}

	// Reliability metrics
	output += "# HELP " + e.prefix + "_delivery_pending Current pending deliveries\n"
	output += "# TYPE " + e.prefix + "_delivery_pending gauge\n"
	output += e.prefix + "_delivery_pending " + formatInt64(e.collector.deliveryPending.Load()) + "\n"

	output += "# HELP " + e.prefix + "_delivery_acked_total Total acknowledged deliveries\n"
	output += "# TYPE " + e.prefix + "_delivery_acked_total counter\n"
	output += e.prefix + "_delivery_acked_total " + formatInt64(e.collector.deliveryAcked.Load()) + "\n"

	output += "# HELP " + e.prefix + "_delivery_failed_total Total failed deliveries\n"
	output += "# TYPE " + e.prefix + "_delivery_failed_total counter\n"
	output += e.prefix + "_delivery_failed_total " + formatInt64(e.collector.deliveryFailed.Load()) + "\n"

	output += "# HELP " + e.prefix + "_duplicates_detected_total Total duplicates detected\n"
	output += "# TYPE " + e.prefix + "_duplicates_detected_total counter\n"
	output += e.prefix + "_duplicates_detected_total " + formatInt64(e.collector.duplicatesDetected.Load()) + "\n"

	// Bootstrap metrics
	output += "# HELP " + e.prefix + "_bootstrap_requests_total Total bootstrap requests\n"
	output += "# TYPE " + e.prefix + "_bootstrap_requests_total counter\n"
	for status, counter := range e.collector.bootstrapRequests {
		output += e.prefix + "_bootstrap_requests_total{status=\"" + status + "\"} " + formatInt64(counter.Load()) + "\n"
	}

	output += "# HELP " + e.prefix + "_credentials_issued_total Total credentials issued\n"
	output += "# TYPE " + e.prefix + "_credentials_issued_total counter\n"
	for credType, counter := range e.collector.credentialsIssued {
		output += e.prefix + "_credentials_issued_total{type=\"" + credType + "\"} " + formatInt64(counter.Load()) + "\n"
	}

	// Coordination metrics
	output += "# HELP " + e.prefix + "_coordination_rpcs_total Total coordination RPCs\n"
	output += "# TYPE " + e.prefix + "_coordination_rpcs_total counter\n"
	for key, counter := range e.collector.coordinationRPCs {
		parts := splitKey(key)
		if len(parts) >= 2 {
			output += e.prefix + "_coordination_rpcs_total{method=\"" + parts[0] + "\",status=\"" + parts[1] + "\"} " + formatInt64(counter.Load()) + "\n"
		}
	}

	return output
}

// Helper functions
func splitKey(key string) []string {
	var result []string
	var current string
	for _, c := range key {
		if c == ':' {
			result = append(result, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	result = append(result, current)
	return result
}

func formatInt64(v int64) string {
	return fmt.Sprintf("%d", v)
}

func formatFloat64(v float64) string {
	return fmt.Sprintf("%.6f", v)
}
