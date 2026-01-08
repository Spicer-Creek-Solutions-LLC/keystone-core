// Package observability provides metrics, logging, and tracing for proxy agents.
package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HealthStatus represents the health status of a component.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// HealthCheck represents a single health check.
type HealthCheck struct {
	Name        string            `json:"name"`
	Status      HealthStatus      `json:"status"`
	Message     string            `json:"message,omitempty"`
	Duration    time.Duration     `json:"duration_ms"`
	LastChecked time.Time         `json:"last_checked"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// HealthChecker is the interface for health check implementations.
type HealthChecker interface {
	Name() string
	Check(ctx context.Context) *HealthCheck
}

// HealthMonitor monitors the health of proxy agent components.
type HealthMonitor struct {
	mu sync.RWMutex

	// checkers are registered health checkers
	checkers []HealthChecker

	// results holds the latest health check results
	results map[string]*HealthCheck

	// checkInterval is how often to run health checks
	checkInterval time.Duration

	// timeout is the timeout for individual health checks
	timeout time.Duration

	// stopCh is used to stop background checking
	stopCh chan struct{}

	// running indicates if background checking is running
	running bool

	// onStatusChange is called when overall status changes
	onStatusChange func(old, new HealthStatus)
}

// HealthMonitorConfig configures the health monitor.
type HealthMonitorConfig struct {
	CheckInterval time.Duration
	Timeout       time.Duration
}

// DefaultHealthMonitorConfig returns default configuration.
func DefaultHealthMonitorConfig() HealthMonitorConfig {
	return HealthMonitorConfig{
		CheckInterval: 30 * time.Second,
		Timeout:       10 * time.Second,
	}
}

// NewHealthMonitor creates a new health monitor.
func NewHealthMonitor(config HealthMonitorConfig) *HealthMonitor {
	if config.CheckInterval == 0 {
		config.CheckInterval = 30 * time.Second
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	return &HealthMonitor{
		checkers:      make([]HealthChecker, 0),
		results:       make(map[string]*HealthCheck),
		checkInterval: config.CheckInterval,
		timeout:       config.Timeout,
		stopCh:        make(chan struct{}),
	}
}

// RegisterChecker registers a health checker.
func (m *HealthMonitor) RegisterChecker(checker HealthChecker) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.checkers = append(m.checkers, checker)
}

// SetStatusChangeCallback sets the callback for status changes.
func (m *HealthMonitor) SetStatusChangeCallback(callback func(old, new HealthStatus)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.onStatusChange = callback
}

// Start starts background health checking.
func (m *HealthMonitor) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("health monitor already running")
	}
	m.running = true
	m.mu.Unlock()

	// Run initial check
	m.RunChecks()

	go func() {
		ticker := time.NewTicker(m.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.RunChecks()
			}
		}
	}()

	return nil
}

// Stop stops background health checking.
func (m *HealthMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		close(m.stopCh)
		m.running = false
		m.stopCh = make(chan struct{})
	}
}

// RunChecks runs all health checks.
func (m *HealthMonitor) RunChecks() {
	m.mu.RLock()
	checkers := make([]HealthChecker, len(m.checkers))
	copy(checkers, m.checkers)
	oldStatus := m.overallStatus()
	m.mu.RUnlock()

	// Run checks concurrently
	var wg sync.WaitGroup
	resultsCh := make(chan *HealthCheck, len(checkers))

	for _, checker := range checkers {
		wg.Add(1)
		go func(c HealthChecker) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
			defer cancel()

			result := c.Check(ctx)
			resultsCh <- result
		}(checker)
	}

	// Wait for all checks to complete
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	// Collect results
	newResults := make(map[string]*HealthCheck)
	for result := range resultsCh {
		newResults[result.Name] = result
	}

	// Update results
	m.mu.Lock()
	m.results = newResults
	newStatus := m.overallStatusLocked()
	callback := m.onStatusChange
	m.mu.Unlock()

	// Notify status change
	if callback != nil && oldStatus != newStatus {
		callback(oldStatus, newStatus)
	}
}

// GetStatus returns the overall health status.
func (m *HealthMonitor) GetStatus() HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.overallStatusLocked()
}

// overallStatus returns overall status (must hold read lock).
func (m *HealthMonitor) overallStatus() HealthStatus {
	return m.overallStatusLocked()
}

// overallStatusLocked returns overall status (assumes lock held).
func (m *HealthMonitor) overallStatusLocked() HealthStatus {
	if len(m.results) == 0 {
		return HealthStatusUnknown
	}

	hasUnhealthy := false
	hasDegraded := false

	for _, result := range m.results {
		switch result.Status {
		case HealthStatusUnhealthy:
			hasUnhealthy = true
		case HealthStatusDegraded:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return HealthStatusUnhealthy
	}
	if hasDegraded {
		return HealthStatusDegraded
	}
	return HealthStatusHealthy
}

// GetResults returns all health check results.
func (m *HealthMonitor) GetResults() map[string]*HealthCheck {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]*HealthCheck)
	for k, v := range m.results {
		results[k] = v
	}
	return results
}

// HealthResponse is the response for health endpoints.
type HealthResponse struct {
	Status    HealthStatus             `json:"status"`
	Checks    map[string]*HealthCheck  `json:"checks,omitempty"`
	Timestamp time.Time                `json:"timestamp"`
}

// HTTPHandler returns an HTTP handler for health endpoints.
func (m *HealthMonitor) HTTPHandler() http.Handler {
	mux := http.NewServeMux()

	// Liveness probe - always returns 200 if process is running
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "alive",
		})
	})

	// Readiness probe - returns 200 if healthy or degraded
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		status := m.GetStatus()

		w.Header().Set("Content-Type", "application/json")
		if status == HealthStatusHealthy || status == HealthStatusDegraded {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(HealthResponse{
			Status:    status,
			Timestamp: time.Now(),
		})
	})

	// Detailed health status
	mux.HandleFunc("/health/status", func(w http.ResponseWriter, r *http.Request) {
		status := m.GetStatus()
		results := m.GetResults()

		w.Header().Set("Content-Type", "application/json")
		if status == HealthStatusHealthy {
			w.WriteHeader(http.StatusOK)
		} else if status == HealthStatusDegraded {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(HealthResponse{
			Status:    status,
			Checks:    results,
			Timestamp: time.Now(),
		})
	})

	return mux
}

// DeviceHealthChecker checks the health of proxied devices.
type DeviceHealthChecker struct {
	name          string
	getDevices    func() []DeviceStatus
	minHealthy    int
	minPercentage float64
}

// DeviceStatus represents a device's health status.
type DeviceStatus struct {
	ID       string
	Healthy  bool
	LastSeen time.Time
}

// NewDeviceHealthChecker creates a new device health checker.
func NewDeviceHealthChecker(name string, getDevices func() []DeviceStatus, minHealthy int, minPercentage float64) *DeviceHealthChecker {
	return &DeviceHealthChecker{
		name:          name,
		getDevices:    getDevices,
		minHealthy:    minHealthy,
		minPercentage: minPercentage,
	}
}

// Name returns the checker name.
func (c *DeviceHealthChecker) Name() string {
	return c.name
}

// Check performs the health check.
func (c *DeviceHealthChecker) Check(ctx context.Context) *HealthCheck {
	start := time.Now()

	devices := c.getDevices()
	total := len(devices)
	healthy := 0

	for _, d := range devices {
		if d.Healthy {
			healthy++
		}
	}

	result := &HealthCheck{
		Name:        c.name,
		LastChecked: time.Now(),
		Duration:    time.Since(start),
		Metadata: map[string]string{
			"total_devices":   fmt.Sprintf("%d", total),
			"healthy_devices": fmt.Sprintf("%d", healthy),
		},
	}

	if total == 0 {
		result.Status = HealthStatusUnknown
		result.Message = "No devices registered"
		return result
	}

	percentage := float64(healthy) / float64(total) * 100

	if healthy >= c.minHealthy && percentage >= c.minPercentage {
		result.Status = HealthStatusHealthy
		result.Message = fmt.Sprintf("%d/%d devices healthy (%.1f%%)", healthy, total, percentage)
	} else if healthy > 0 {
		result.Status = HealthStatusDegraded
		result.Message = fmt.Sprintf("Only %d/%d devices healthy (%.1f%%), minimum: %d or %.1f%%",
			healthy, total, percentage, c.minHealthy, c.minPercentage)
	} else {
		result.Status = HealthStatusUnhealthy
		result.Message = "No healthy devices"
	}

	return result
}

// ConnectionHealthChecker checks the health of protocol connections.
type ConnectionHealthChecker struct {
	name          string
	getStats      func() ConnectionStats
	maxFailRate   float64
	maxLatencyMs  float64
}

// ConnectionStats holds connection statistics.
type ConnectionStats struct {
	Total    int64
	Active   int
	Failed   int64
	AvgLatency time.Duration
}

// NewConnectionHealthChecker creates a new connection health checker.
func NewConnectionHealthChecker(name string, getStats func() ConnectionStats, maxFailRate, maxLatencyMs float64) *ConnectionHealthChecker {
	return &ConnectionHealthChecker{
		name:         name,
		getStats:     getStats,
		maxFailRate:  maxFailRate,
		maxLatencyMs: maxLatencyMs,
	}
}

// Name returns the checker name.
func (c *ConnectionHealthChecker) Name() string {
	return c.name
}

// Check performs the health check.
func (c *ConnectionHealthChecker) Check(ctx context.Context) *HealthCheck {
	start := time.Now()
	stats := c.getStats()

	result := &HealthCheck{
		Name:        c.name,
		LastChecked: time.Now(),
		Duration:    time.Since(start),
		Metadata: map[string]string{
			"total_connections":  fmt.Sprintf("%d", stats.Total),
			"active_connections": fmt.Sprintf("%d", stats.Active),
			"failed_connections": fmt.Sprintf("%d", stats.Failed),
			"avg_latency_ms":     fmt.Sprintf("%.2f", float64(stats.AvgLatency.Milliseconds())),
		},
	}

	if stats.Total == 0 {
		result.Status = HealthStatusHealthy
		result.Message = "No connections yet"
		return result
	}

	failRate := float64(stats.Failed) / float64(stats.Total) * 100
	latencyMs := float64(stats.AvgLatency.Milliseconds())

	if failRate > c.maxFailRate {
		result.Status = HealthStatusUnhealthy
		result.Message = fmt.Sprintf("High failure rate: %.2f%% (max: %.2f%%)", failRate, c.maxFailRate)
	} else if latencyMs > c.maxLatencyMs {
		result.Status = HealthStatusDegraded
		result.Message = fmt.Sprintf("High latency: %.2fms (max: %.2fms)", latencyMs, c.maxLatencyMs)
	} else {
		result.Status = HealthStatusHealthy
		result.Message = fmt.Sprintf("Active: %d, Fail rate: %.2f%%, Latency: %.2fms", stats.Active, failRate, latencyMs)
	}

	return result
}

// ProtocolHealthChecker checks the health of a specific protocol adapter.
type ProtocolHealthChecker struct {
	name     string
	protocol string
	check    func(ctx context.Context) error
}

// NewProtocolHealthChecker creates a new protocol health checker.
func NewProtocolHealthChecker(name, protocol string, check func(ctx context.Context) error) *ProtocolHealthChecker {
	return &ProtocolHealthChecker{
		name:     name,
		protocol: protocol,
		check:    check,
	}
}

// Name returns the checker name.
func (c *ProtocolHealthChecker) Name() string {
	return c.name
}

// Check performs the health check.
func (c *ProtocolHealthChecker) Check(ctx context.Context) *HealthCheck {
	start := time.Now()

	result := &HealthCheck{
		Name:        c.name,
		LastChecked: time.Now(),
		Metadata: map[string]string{
			"protocol": c.protocol,
		},
	}

	err := c.check(ctx)
	result.Duration = time.Since(start)

	if err != nil {
		result.Status = HealthStatusUnhealthy
		result.Message = err.Error()
	} else {
		result.Status = HealthStatusHealthy
		result.Message = "Protocol adapter operational"
	}

	return result
}

// FunctionHealthChecker wraps a function as a health checker.
type FunctionHealthChecker struct {
	name  string
	check func(ctx context.Context) (HealthStatus, string, map[string]string)
}

// NewFunctionHealthChecker creates a new function health checker.
func NewFunctionHealthChecker(name string, check func(ctx context.Context) (HealthStatus, string, map[string]string)) *FunctionHealthChecker {
	return &FunctionHealthChecker{
		name:  name,
		check: check,
	}
}

// Name returns the checker name.
func (c *FunctionHealthChecker) Name() string {
	return c.name
}

// Check performs the health check.
func (c *FunctionHealthChecker) Check(ctx context.Context) *HealthCheck {
	start := time.Now()

	status, message, metadata := c.check(ctx)

	return &HealthCheck{
		Name:        c.name,
		Status:      status,
		Message:     message,
		Duration:    time.Since(start),
		LastChecked: time.Now(),
		Metadata:    metadata,
	}
}
