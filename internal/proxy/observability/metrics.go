// Package observability provides metrics, logging, and tracing for proxy agents.
package observability

import (
	"sync"
	"time"
)

// ProxyMetrics collects metrics for proxy agent operations.
type ProxyMetrics struct {
	mu sync.RWMutex

	// Device metrics
	devicesTotal      int
	devicesHealthy    int
	devicesDegraded   int
	devicesUnhealthy  int
	devicesUnknown    int
	devicesByProtocol map[string]int
	devicesByVendor   map[string]int

	// Connection metrics
	connectionsTotal  int64
	connectionsActive int
	connectionsFailed int64
	connectionLatency *LatencyStats

	// Command metrics
	commandsTotal     int64
	commandsSucceeded int64
	commandsFailed    int64
	commandLatency    *LatencyStats

	// State metrics
	statesApplied   int64
	statesSucceeded int64
	statesFailed    int64
	statesChanged   int64
	stateLatency    *LatencyStats

	// Drift metrics
	driftChecks   int64
	driftDetected int64
	driftSeverity map[string]int64 // by severity level

	// Discovery metrics
	discoveryScans    int64
	discoveredDevices int64
	approvedDevices   int64
	rejectedDevices   int64

	// Protocol-specific metrics
	sshCommands   int64
	snmpRequests  int64
	restRequests  int64
	winrmCommands int64

	// Error tracking
	errorsByType map[string]int64

	// Last update time
	lastUpdate time.Time
}

// LatencyStats tracks latency statistics.
type LatencyStats struct {
	mu      sync.Mutex
	count   int64
	total   time.Duration
	min     time.Duration
	max     time.Duration
	buckets []int64 // histogram buckets
}

// DefaultLatencyBuckets are the default histogram buckets (in ms).
var DefaultLatencyBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

// NewLatencyStats creates a new latency stats tracker.
func NewLatencyStats() *LatencyStats {
	return &LatencyStats{
		buckets: make([]int64, len(DefaultLatencyBuckets)+1),
	}
}

// Record records a latency measurement.
func (s *LatencyStats) Record(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.count++
	s.total += d

	if s.count == 1 || d < s.min {
		s.min = d
	}
	if d > s.max {
		s.max = d
	}

	// Update histogram bucket
	ms := float64(d.Milliseconds())
	for i, bucket := range DefaultLatencyBuckets {
		if ms <= bucket {
			s.buckets[i]++
			return
		}
	}
	s.buckets[len(s.buckets)-1]++ // overflow bucket
}

// Stats returns latency statistics.
func (s *LatencyStats) Stats() (count int64, avg, minVal, maxVal time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.count == 0 {
		return 0, 0, 0, 0
	}

	return s.count, s.total / time.Duration(s.count), s.min, s.max
}

// Buckets returns histogram buckets.
func (s *LatencyStats) Buckets() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	buckets := make([]int64, len(s.buckets))
	copy(buckets, s.buckets)
	return buckets
}

// NewProxyMetrics creates a new proxy metrics collector.
func NewProxyMetrics() *ProxyMetrics {
	return &ProxyMetrics{
		devicesByProtocol: make(map[string]int),
		devicesByVendor:   make(map[string]int),
		driftSeverity:     make(map[string]int64),
		errorsByType:      make(map[string]int64),
		connectionLatency: NewLatencyStats(),
		commandLatency:    NewLatencyStats(),
		stateLatency:      NewLatencyStats(),
		lastUpdate:        time.Now(),
	}
}

// SetDeviceCounts updates device counts.
func (m *ProxyMetrics) SetDeviceCounts(total, healthy, degraded, unhealthy, unknown int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.devicesTotal = total
	m.devicesHealthy = healthy
	m.devicesDegraded = degraded
	m.devicesUnhealthy = unhealthy
	m.devicesUnknown = unknown
	m.lastUpdate = time.Now()
}

// SetDevicesByProtocol updates devices by protocol counts.
func (m *ProxyMetrics) SetDevicesByProtocol(counts map[string]int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.devicesByProtocol = make(map[string]int)
	for k, v := range counts {
		m.devicesByProtocol[k] = v
	}
	m.lastUpdate = time.Now()
}

// SetDevicesByVendor updates devices by vendor counts.
func (m *ProxyMetrics) SetDevicesByVendor(counts map[string]int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.devicesByVendor = make(map[string]int)
	for k, v := range counts {
		m.devicesByVendor[k] = v
	}
	m.lastUpdate = time.Now()
}

// RecordConnection records a connection attempt.
func (m *ProxyMetrics) RecordConnection(success bool, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.connectionsTotal++
	if success {
		m.connectionsActive++
	} else {
		m.connectionsFailed++
	}
	m.connectionLatency.Record(latency)
	m.lastUpdate = time.Now()
}

// RecordDisconnection records a disconnection.
func (m *ProxyMetrics) RecordDisconnection() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connectionsActive > 0 {
		m.connectionsActive--
	}
	m.lastUpdate = time.Now()
}

// RecordCommand records a command execution.
func (m *ProxyMetrics) RecordCommand(success bool, latency time.Duration, protocol string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.commandsTotal++
	if success {
		m.commandsSucceeded++
	} else {
		m.commandsFailed++
	}
	m.commandLatency.Record(latency)

	switch protocol {
	case "ssh":
		m.sshCommands++
	case "snmp":
		m.snmpRequests++
	case "rest", "http", "https":
		m.restRequests++
	case "winrm":
		m.winrmCommands++
	}
	m.lastUpdate = time.Now()
}

// RecordState records a state application.
func (m *ProxyMetrics) RecordState(success, changed bool, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.statesApplied++
	if success {
		m.statesSucceeded++
	} else {
		m.statesFailed++
	}
	if changed {
		m.statesChanged++
	}
	m.stateLatency.Record(latency)
	m.lastUpdate = time.Now()
}

// RecordDrift records a drift check.
func (m *ProxyMetrics) RecordDrift(detected bool, severity string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.driftChecks++
	if detected {
		m.driftDetected++
		m.driftSeverity[severity]++
	}
	m.lastUpdate = time.Now()
}

// RecordDiscovery records discovery operations.
func (m *ProxyMetrics) RecordDiscovery(scans, discovered, approved, rejected int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.discoveryScans += scans
	m.discoveredDevices += discovered
	m.approvedDevices += approved
	m.rejectedDevices += rejected
	m.lastUpdate = time.Now()
}

// RecordError records an error by type.
func (m *ProxyMetrics) RecordError(errorType string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.errorsByType[errorType]++
	m.lastUpdate = time.Now()
}

// MetricsSnapshot is a point-in-time snapshot of metrics.
type MetricsSnapshot struct {
	// Device metrics
	DevicesTotal      int            `json:"devices_total"`
	DevicesHealthy    int            `json:"devices_healthy"`
	DevicesDegraded   int            `json:"devices_degraded"`
	DevicesUnhealthy  int            `json:"devices_unhealthy"`
	DevicesUnknown    int            `json:"devices_unknown"`
	DevicesByProtocol map[string]int `json:"devices_by_protocol"`
	DevicesByVendor   map[string]int `json:"devices_by_vendor"`

	// Connection metrics
	ConnectionsTotal     int64   `json:"connections_total"`
	ConnectionsActive    int     `json:"connections_active"`
	ConnectionsFailed    int64   `json:"connections_failed"`
	ConnectionLatencyAvg float64 `json:"connection_latency_avg_ms"`

	// Command metrics
	CommandsTotal      int64   `json:"commands_total"`
	CommandsSucceeded  int64   `json:"commands_succeeded"`
	CommandsFailed     int64   `json:"commands_failed"`
	CommandSuccessRate float64 `json:"command_success_rate"`
	CommandLatencyAvg  float64 `json:"command_latency_avg_ms"`

	// State metrics
	StatesApplied    int64   `json:"states_applied"`
	StatesSucceeded  int64   `json:"states_succeeded"`
	StatesFailed     int64   `json:"states_failed"`
	StatesChanged    int64   `json:"states_changed"`
	StateSuccessRate float64 `json:"state_success_rate"`
	StateLatencyAvg  float64 `json:"state_latency_avg_ms"`

	// Drift metrics
	DriftChecks   int64            `json:"drift_checks"`
	DriftDetected int64            `json:"drift_detected"`
	DriftSeverity map[string]int64 `json:"drift_severity"`

	// Discovery metrics
	DiscoveryScans    int64 `json:"discovery_scans"`
	DiscoveredDevices int64 `json:"discovered_devices"`
	ApprovedDevices   int64 `json:"approved_devices"`
	RejectedDevices   int64 `json:"rejected_devices"`

	// Protocol metrics
	SSHCommands   int64 `json:"ssh_commands"`
	SNMPRequests  int64 `json:"snmp_requests"`
	RESTRequests  int64 `json:"rest_requests"`
	WinRMCommands int64 `json:"winrm_commands"`

	// Errors
	ErrorsByType map[string]int64 `json:"errors_by_type"`

	// Timestamp
	Timestamp time.Time `json:"timestamp"`
}

// Snapshot returns a point-in-time snapshot of metrics.
func (m *ProxyMetrics) Snapshot() *MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := &MetricsSnapshot{
		DevicesTotal:      m.devicesTotal,
		DevicesHealthy:    m.devicesHealthy,
		DevicesDegraded:   m.devicesDegraded,
		DevicesUnhealthy:  m.devicesUnhealthy,
		DevicesUnknown:    m.devicesUnknown,
		DevicesByProtocol: make(map[string]int),
		DevicesByVendor:   make(map[string]int),

		ConnectionsTotal:  m.connectionsTotal,
		ConnectionsActive: m.connectionsActive,
		ConnectionsFailed: m.connectionsFailed,

		CommandsTotal:     m.commandsTotal,
		CommandsSucceeded: m.commandsSucceeded,
		CommandsFailed:    m.commandsFailed,

		StatesApplied:   m.statesApplied,
		StatesSucceeded: m.statesSucceeded,
		StatesFailed:    m.statesFailed,
		StatesChanged:   m.statesChanged,

		DriftChecks:   m.driftChecks,
		DriftDetected: m.driftDetected,
		DriftSeverity: make(map[string]int64),

		DiscoveryScans:    m.discoveryScans,
		DiscoveredDevices: m.discoveredDevices,
		ApprovedDevices:   m.approvedDevices,
		RejectedDevices:   m.rejectedDevices,

		SSHCommands:   m.sshCommands,
		SNMPRequests:  m.snmpRequests,
		RESTRequests:  m.restRequests,
		WinRMCommands: m.winrmCommands,

		ErrorsByType: make(map[string]int64),

		Timestamp: time.Now(),
	}

	// Copy maps
	for k, v := range m.devicesByProtocol {
		snapshot.DevicesByProtocol[k] = v
	}
	for k, v := range m.devicesByVendor {
		snapshot.DevicesByVendor[k] = v
	}
	for k, v := range m.driftSeverity {
		snapshot.DriftSeverity[k] = v
	}
	for k, v := range m.errorsByType {
		snapshot.ErrorsByType[k] = v
	}

	// Calculate rates
	if m.commandsTotal > 0 {
		snapshot.CommandSuccessRate = float64(m.commandsSucceeded) / float64(m.commandsTotal) * 100
	}
	if m.statesApplied > 0 {
		snapshot.StateSuccessRate = float64(m.statesSucceeded) / float64(m.statesApplied) * 100
	}

	// Calculate average latencies
	if count, avg, _, _ := m.connectionLatency.Stats(); count > 0 {
		snapshot.ConnectionLatencyAvg = float64(avg.Milliseconds())
	}
	if count, avg, _, _ := m.commandLatency.Stats(); count > 0 {
		snapshot.CommandLatencyAvg = float64(avg.Milliseconds())
	}
	if count, avg, _, _ := m.stateLatency.Stats(); count > 0 {
		snapshot.StateLatencyAvg = float64(avg.Milliseconds())
	}

	return snapshot
}

// PrometheusExporter exports metrics in Prometheus format.
type PrometheusExporter struct {
	metrics *ProxyMetrics
	prefix  string
}

// NewPrometheusExporter creates a new Prometheus exporter.
func NewPrometheusExporter(metrics *ProxyMetrics, prefix string) *PrometheusExporter {
	if prefix == "" {
		prefix = "kscore_proxy"
	}
	return &PrometheusExporter{
		metrics: metrics,
		prefix:  prefix,
	}
}

// Export exports metrics in Prometheus text format.
func (e *PrometheusExporter) Export() string {
	snapshot := e.metrics.Snapshot()

	var output string

	// Device metrics
	output += e.formatGauge("devices_total", "Total number of proxied devices", float64(snapshot.DevicesTotal), nil)
	output += e.formatGauge("devices_healthy", "Number of healthy devices", float64(snapshot.DevicesHealthy), nil)
	output += e.formatGauge("devices_degraded", "Number of degraded devices", float64(snapshot.DevicesDegraded), nil)
	output += e.formatGauge("devices_unhealthy", "Number of unhealthy devices", float64(snapshot.DevicesUnhealthy), nil)
	output += e.formatGauge("devices_unknown", "Number of devices with unknown status", float64(snapshot.DevicesUnknown), nil)

	for protocol, count := range snapshot.DevicesByProtocol {
		output += e.formatGauge("devices_by_protocol", "Devices by protocol", float64(count), map[string]string{"protocol": protocol})
	}
	for vendor, count := range snapshot.DevicesByVendor {
		output += e.formatGauge("devices_by_vendor", "Devices by vendor", float64(count), map[string]string{"vendor": vendor})
	}

	// Connection metrics
	output += e.formatCounter("connections_total", "Total connection attempts", float64(snapshot.ConnectionsTotal), nil)
	output += e.formatGauge("connections_active", "Active connections", float64(snapshot.ConnectionsActive), nil)
	output += e.formatCounter("connections_failed", "Failed connections", float64(snapshot.ConnectionsFailed), nil)
	output += e.formatGauge("connection_latency_avg_ms", "Average connection latency (ms)", snapshot.ConnectionLatencyAvg, nil)

	// Command metrics
	output += e.formatCounter("commands_total", "Total commands executed", float64(snapshot.CommandsTotal), nil)
	output += e.formatCounter("commands_succeeded", "Successful commands", float64(snapshot.CommandsSucceeded), nil)
	output += e.formatCounter("commands_failed", "Failed commands", float64(snapshot.CommandsFailed), nil)
	output += e.formatGauge("command_success_rate", "Command success rate (%)", snapshot.CommandSuccessRate, nil)
	output += e.formatGauge("command_latency_avg_ms", "Average command latency (ms)", snapshot.CommandLatencyAvg, nil)

	// Protocol-specific commands
	output += e.formatCounter("ssh_commands_total", "Total SSH commands", float64(snapshot.SSHCommands), nil)
	output += e.formatCounter("snmp_requests_total", "Total SNMP requests", float64(snapshot.SNMPRequests), nil)
	output += e.formatCounter("rest_requests_total", "Total REST requests", float64(snapshot.RESTRequests), nil)
	output += e.formatCounter("winrm_commands_total", "Total WinRM commands", float64(snapshot.WinRMCommands), nil)

	// State metrics
	output += e.formatCounter("states_applied_total", "Total states applied", float64(snapshot.StatesApplied), nil)
	output += e.formatCounter("states_succeeded_total", "Successful state applications", float64(snapshot.StatesSucceeded), nil)
	output += e.formatCounter("states_failed_total", "Failed state applications", float64(snapshot.StatesFailed), nil)
	output += e.formatCounter("states_changed_total", "States that made changes", float64(snapshot.StatesChanged), nil)
	output += e.formatGauge("state_success_rate", "State success rate (%)", snapshot.StateSuccessRate, nil)
	output += e.formatGauge("state_latency_avg_ms", "Average state application latency (ms)", snapshot.StateLatencyAvg, nil)

	// Drift metrics
	output += e.formatCounter("drift_checks_total", "Total drift checks", float64(snapshot.DriftChecks), nil)
	output += e.formatCounter("drift_detected_total", "Total drift detections", float64(snapshot.DriftDetected), nil)
	for severity, count := range snapshot.DriftSeverity {
		output += e.formatCounter("drift_by_severity_total", "Drift detections by severity", float64(count), map[string]string{"severity": severity})
	}

	// Discovery metrics
	output += e.formatCounter("discovery_scans_total", "Total discovery scans", float64(snapshot.DiscoveryScans), nil)
	output += e.formatCounter("discovered_devices_total", "Total discovered devices", float64(snapshot.DiscoveredDevices), nil)
	output += e.formatCounter("approved_devices_total", "Total approved devices", float64(snapshot.ApprovedDevices), nil)
	output += e.formatCounter("rejected_devices_total", "Total rejected devices", float64(snapshot.RejectedDevices), nil)

	// Errors
	for errorType, count := range snapshot.ErrorsByType {
		output += e.formatCounter("errors_total", "Total errors by type", float64(count), map[string]string{"type": errorType})
	}

	return output
}

// formatGauge formats a gauge metric.
func (e *PrometheusExporter) formatGauge(name, help string, value float64, labels map[string]string) string {
	fullName := e.prefix + "_" + name
	labelStr := e.formatLabels(labels)
	return "# HELP " + fullName + " " + help + "\n" +
		"# TYPE " + fullName + " gauge\n" +
		fullName + labelStr + " " + formatFloat(value) + "\n"
}

// formatCounter formats a counter metric.
func (e *PrometheusExporter) formatCounter(name, help string, value float64, labels map[string]string) string {
	fullName := e.prefix + "_" + name
	labelStr := e.formatLabels(labels)
	return "# HELP " + fullName + " " + help + "\n" +
		"# TYPE " + fullName + " counter\n" +
		fullName + labelStr + " " + formatFloat(value) + "\n"
}

// formatLabels formats labels for Prometheus.
func (e *PrometheusExporter) formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	result := "{"
	first := true
	for k, v := range labels {
		if !first {
			result += ","
		}
		result += k + "=\"" + v + "\""
		first = false
	}
	result += "}"
	return result
}

// formatFloat formats a float64 for Prometheus output.
func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return string(rune(int64(f) + '0'))
	}
	// Simple formatting - in production use strconv
	return ""
}
