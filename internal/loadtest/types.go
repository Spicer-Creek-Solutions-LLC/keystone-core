// Package loadtest provides load testing infrastructure for Keystone Core.
// It allows simulating configurable numbers of agents to test control plane
// capacity, throughput, and latency under various load conditions.
//
// Usage:
//
//	# Run via go test
//	KSCORE_LOAD_TEST=1 KSCORE_AGENT_COUNT=100 go test -v ./internal/loadtest/...
//
//	# Run specific scenarios
//	KSCORE_LOAD_TEST=1 KSCORE_AGENT_COUNT=50 go test -v -run TestLoad_Registration ./internal/loadtest/...
package loadtest

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// Config holds load test configuration.
type Config struct {
	// AgentCount is the number of simulated agents.
	AgentCount int

	// HeartbeatInterval is how often agents send heartbeats.
	HeartbeatInterval time.Duration

	// CommandTimeout is the timeout for command execution.
	CommandTimeout time.Duration

	// TestDuration is how long to run sustained load tests.
	TestDuration time.Duration

	// RampUpDuration is how long to take to reach full agent count.
	RampUpDuration time.Duration

	// CommandsPerAgent is the number of commands per agent in command tests.
	CommandsPerAgent int

	// ConcurrentCommands is the max concurrent commands to send.
	ConcurrentCommands int

	// NATSPort is the port for embedded NATS.
	NATSPort int

	// ReportDir is where to save test reports.
	ReportDir string
}

// DefaultConfig returns default load test configuration.
func DefaultConfig() *Config {
	return &Config{
		AgentCount:         10,
		HeartbeatInterval:  5 * time.Second,
		CommandTimeout:     30 * time.Second,
		TestDuration:       60 * time.Second,
		RampUpDuration:     10 * time.Second,
		CommandsPerAgent:   10,
		ConcurrentCommands: 50,
		NATSPort:           14222,
		ReportDir:          "reports/loadtest",
	}
}

// ConfigFromEnv creates config from environment variables.
func ConfigFromEnv() *Config {
	cfg := DefaultConfig()

	if v := os.Getenv("KSCORE_AGENT_COUNT"); v != "" {
		var count int
		if _, err := fmt.Sscanf(v, "%d", &count); err == nil && count > 0 {
			cfg.AgentCount = count
		}
	}

	if v := os.Getenv("KSCORE_TEST_DURATION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.TestDuration = d
		}
	}

	if v := os.Getenv("KSCORE_RAMP_UP"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.RampUpDuration = d
		}
	}

	if v := os.Getenv("KSCORE_HEARTBEAT_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.HeartbeatInterval = d
		}
	}

	if v := os.Getenv("KSCORE_COMMANDS_PER_AGENT"); v != "" {
		var count int
		if _, err := fmt.Sscanf(v, "%d", &count); err == nil && count > 0 {
			cfg.CommandsPerAgent = count
		}
	}

	if v := os.Getenv("KSCORE_CONCURRENT_COMMANDS"); v != "" {
		var count int
		if _, err := fmt.Sscanf(v, "%d", &count); err == nil && count > 0 {
			cfg.ConcurrentCommands = count
		}
	}

	if v := os.Getenv("KSCORE_REPORT_DIR"); v != "" {
		cfg.ReportDir = v
	}

	return cfg
}

// Result holds the results of a load test.
type Result struct {
	TestName     string        `json:"test_name"`
	Config       ResultConfig  `json:"config"`
	Metrics      Metrics       `json:"metrics"`
	AgentMetrics []AgentMetric `json:"agent_metrics,omitempty"`
	Errors       []string      `json:"errors,omitempty"`
	StartTime    time.Time     `json:"start_time"`
	EndTime      time.Time     `json:"end_time"`
	Duration     time.Duration `json:"duration_ns"`
	Success      bool          `json:"success"`
}

// ResultConfig is a subset of Config for result storage.
type ResultConfig struct {
	AgentCount         int           `json:"agent_count"`
	HeartbeatInterval  time.Duration `json:"heartbeat_interval_ns"`
	TestDuration       time.Duration `json:"test_duration_ns"`
	CommandsPerAgent   int           `json:"commands_per_agent,omitempty"`
	ConcurrentCommands int           `json:"concurrent_commands,omitempty"`
}

// Metrics holds aggregated performance metrics.
type Metrics struct {
	TotalOps      int64         `json:"total_ops"`
	SuccessfulOps int64         `json:"successful_ops"`
	FailedOps     int64         `json:"failed_ops"`
	OpsPerSecond  float64       `json:"ops_per_second"`
	AvgLatency    time.Duration `json:"avg_latency_ns"`
	MinLatency    time.Duration `json:"min_latency_ns"`
	MaxLatency    time.Duration `json:"max_latency_ns"`
	P50Latency    time.Duration `json:"p50_latency_ns"`
	P95Latency    time.Duration `json:"p95_latency_ns"`
	P99Latency    time.Duration `json:"p99_latency_ns"`
	ErrorRate     float64       `json:"error_rate_percent"`

	// Registration-specific
	RegistrationTime time.Duration `json:"registration_time_ns,omitempty"`
	AgentsRegistered int           `json:"agents_registered,omitempty"`

	// Heartbeat-specific
	HeartbeatsSent     int64 `json:"heartbeats_sent,omitempty"`
	HeartbeatsReceived int64 `json:"heartbeats_received,omitempty"`
	HeartbeatsMissed   int64 `json:"heartbeats_missed,omitempty"`

	// Command-specific
	CommandsSent      int64 `json:"commands_sent,omitempty"`
	CommandsCompleted int64 `json:"commands_completed,omitempty"`
	CommandsFailed    int64 `json:"commands_failed,omitempty"`
	CommandsTimedOut  int64 `json:"commands_timed_out,omitempty"`
}

// AgentMetric holds per-agent metrics.
type AgentMetric struct {
	AgentID          string        `json:"agent_id"`
	RegistrationTime time.Duration `json:"registration_time_ns"`
	HeartbeatsSent   int64         `json:"heartbeats_sent"`
	CommandsReceived int64         `json:"commands_received"`
	CommandsExecuted int64         `json:"commands_executed"`
	AvgCommandTime   time.Duration `json:"avg_command_time_ns"`
	Errors           int64         `json:"errors"`
}

// LatencyCollector collects and calculates latency statistics.
type LatencyCollector struct {
	mu        sync.Mutex
	latencies []time.Duration
}

// NewLatencyCollector creates a new latency collector.
func NewLatencyCollector() *LatencyCollector {
	return &LatencyCollector{
		latencies: make([]time.Duration, 0, 1000),
	}
}

// Add adds a latency measurement.
func (c *LatencyCollector) Add(d time.Duration) {
	c.mu.Lock()
	c.latencies = append(c.latencies, d)
	c.mu.Unlock()
}

// AddBatch adds multiple latency measurements.
func (c *LatencyCollector) AddBatch(ds []time.Duration) {
	c.mu.Lock()
	c.latencies = append(c.latencies, ds...)
	c.mu.Unlock()
}

// Count returns the number of measurements.
func (c *LatencyCollector) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.latencies)
}

// Calculate calculates latency statistics.
//
//nolint:gocritic // tooManyResultsChecker: 6 results needed for latency stats (min, max, avg, p50, p95, p99)
func (c *LatencyCollector) Calculate() (minVal, maxVal, avg, p50, p95, p99 time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.latencies) == 0 {
		return
	}

	// Sort for percentile calculation
	sorted := make([]time.Duration, len(c.latencies))
	copy(sorted, c.latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	minVal = sorted[0]
	maxVal = sorted[len(sorted)-1]

	var sum time.Duration
	for _, l := range sorted {
		sum += l
	}
	avg = sum / time.Duration(len(sorted))

	p50 = sorted[len(sorted)*50/100]
	p95 = sorted[len(sorted)*95/100]

	if len(sorted) >= 100 {
		p99 = sorted[len(sorted)*99/100]
	} else {
		p99 = maxVal
	}

	return
}

// Reset clears all measurements.
func (c *LatencyCollector) Reset() {
	c.mu.Lock()
	c.latencies = c.latencies[:0]
	c.mu.Unlock()
}

// Latencies returns a copy of all latency measurements.
func (c *LatencyCollector) Latencies() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]time.Duration, len(c.latencies))
	copy(result, c.latencies)
	return result
}

// Percentile returns the given percentile value.
func (c *LatencyCollector) Percentile(p int) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.latencies) == 0 {
		return 0
	}

	sorted := make([]time.Duration, len(c.latencies))
	copy(sorted, c.latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	idx := len(sorted) * p / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// Counter is a thread-safe counter.
type Counter struct {
	mu    sync.Mutex
	value int64
}

// Inc increments the counter.
func (c *Counter) Inc() {
	c.mu.Lock()
	c.value++
	c.mu.Unlock()
}

// Add adds n to the counter.
func (c *Counter) Add(n int64) {
	c.mu.Lock()
	c.value += n
	c.mu.Unlock()
}

// Value returns the current value.
func (c *Counter) Value() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

// SaveResult saves a load test result to a JSON file.
func SaveResult(result *Result, dir string) error {
	//nolint:gosec // G301: report directory needs to be accessible by users
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}

	filename := fmt.Sprintf("%s/%s_%d.json", dir, result.TestName, result.StartTime.Unix())
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	//nolint:gosec // G306: load test results need to be readable for analysis
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return fmt.Errorf("failed to write result file: %w", err)
	}

	return nil
}

// LoadResult loads a load test result from a JSON file.
func LoadResult(filename string) (*Result, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read result file: %w", err)
	}

	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return &result, nil
}
