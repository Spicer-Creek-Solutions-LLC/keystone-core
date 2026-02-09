// Package ha provides high availability and scaling for the file distribution system.
package ha

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
)

// InstanceState represents the state of a file server instance.
type InstanceState string

// InstanceStateStarting constants define the possible states.
const (
	InstanceStateStarting  InstanceState = "starting"
	InstanceStateReady     InstanceState = "ready"
	InstanceStateDraining  InstanceState = "draining"
	InstanceStateStopped   InstanceState = "stopped"
	InstanceStateUnhealthy InstanceState = "unhealthy"
)

// InstanceInfo represents information about a file server instance.
type InstanceInfo struct {
	// ID is the unique identifier for this instance.
	ID string

	// Hostname is the hostname of this instance.
	Hostname string

	// Address is the address clients can use to reach this instance.
	Address string

	// State is the current state of the instance.
	State InstanceState

	// StartedAt is when this instance started.
	StartedAt time.Time

	// LastHeartbeat is the last time this instance reported health.
	LastHeartbeat time.Time

	// Metrics contains instance-level metrics.
	Metrics *InstanceMetrics
}

// InstanceMetrics contains metrics for an instance.
type InstanceMetrics struct {
	// TransfersActive is the number of active transfers.
	TransfersActive int64

	// TransfersTotal is the total number of transfers.
	TransfersTotal int64

	// BytesTransferred is the total bytes transferred.
	BytesTransferred int64

	// ErrorsTotal is the total number of errors.
	ErrorsTotal int64
}

// InstanceManager manages file server instances for high availability.
type InstanceManager struct {
	// info is information about this instance.
	info *InstanceInfo

	// nc is the NATS connection.
	nc *nats.Conn

	// instances is the map of known instances.
	instances map[string]*InstanceInfo

	// mu protects the instances map and info fields.
	mu sync.RWMutex

	// healthInterval is how often to report health.
	healthInterval time.Duration

	// healthSubject is the NATS subject for health reports.
	healthSubject string

	// healthSub is the subscription for health reports.
	healthSub *nats.Subscription

	// done signals shutdown.
	done chan struct{}

	// healthChecker provides health check functionality.
	healthChecker HealthChecker
}

// HealthChecker provides health check functionality.
type HealthChecker interface {
	// Check performs a health check and returns an error if unhealthy.
	Check(ctx context.Context) error
}

// InstanceManagerConfig configures the instance manager.
type InstanceManagerConfig struct {
	// ID is the unique identifier for this instance.
	ID string

	// Hostname is the hostname of this instance.
	Hostname string

	// Address is the address clients can use to reach this instance.
	Address string

	// NC is the NATS connection.
	NC *nats.Conn

	// HealthInterval is how often to report health.
	HealthInterval time.Duration

	// HealthSubject is the NATS subject for health reports.
	HealthSubject string

	// HealthChecker provides health check functionality.
	HealthChecker HealthChecker
}

// NewInstanceManager creates a new instance manager.
func NewInstanceManager(config *InstanceManagerConfig) *InstanceManager {
	if config.HealthInterval == 0 {
		config.HealthInterval = 10 * time.Second
	}
	if config.HealthSubject == "" {
		config.HealthSubject = "kscore.files.health"
	}

	return &InstanceManager{
		info: &InstanceInfo{
			ID:        config.ID,
			Hostname:  config.Hostname,
			Address:   config.Address,
			State:     InstanceStateStarting,
			StartedAt: time.Now(),
			Metrics:   &InstanceMetrics{},
		},
		nc:             config.NC,
		instances:      make(map[string]*InstanceInfo),
		healthInterval: config.HealthInterval,
		healthSubject:  config.HealthSubject,
		done:           make(chan struct{}),
		healthChecker:  config.HealthChecker,
	}
}

// Start starts the instance manager.
func (m *InstanceManager) Start(ctx context.Context) error {
	// Subscribe to health reports if NATS is available.
	if m.nc != nil {
		sub, err := m.nc.Subscribe(m.healthSubject, m.handleHealthReport)
		if err != nil {
			return err
		}
		m.healthSub = sub
	}

	// Mark instance as ready.
	m.info.State = InstanceStateReady
	m.info.LastHeartbeat = time.Now()

	// Start health reporting goroutine.
	go m.healthReportLoop() //nolint:contextcheck // background loop uses internal context

	return nil
}

// Stop stops the instance manager.
func (m *InstanceManager) Stop(ctx context.Context) error {
	// Check if already stopped.
	if m.info.State == InstanceStateStopped {
		return nil
	}

	// Mark instance as draining.
	m.info.State = InstanceStateDraining

	// Signal shutdown (only once).
	select {
	case <-m.done:
		// Already closed.
	default:
		close(m.done)
	}

	// Unsubscribe from health reports.
	if m.healthSub != nil {
		if err := m.healthSub.Unsubscribe(); err != nil {
			return err
		}
		m.healthSub = nil
	}

	// Mark instance as stopped.
	m.info.State = InstanceStateStopped

	return nil
}

// GetInfo returns information about this instance.
func (m *InstanceManager) GetInfo() *InstanceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Return a copy to prevent races
	infoCopy := *m.info
	return &infoCopy
}

// GetInstances returns all known instances.
func (m *InstanceManager) GetInstances() []*InstanceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	instances := make([]*InstanceInfo, 0, len(m.instances))
	for _, inst := range m.instances {
		instances = append(instances, inst)
	}
	return instances
}

// GetHealthyInstances returns all healthy instances.
func (m *InstanceManager) GetHealthyInstances() []*InstanceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	instances := make([]*InstanceInfo, 0)
	for _, inst := range m.instances {
		if inst.State == InstanceStateReady {
			instances = append(instances, inst)
		}
	}
	return instances
}

// healthReportLoop periodically reports health.
func (m *InstanceManager) healthReportLoop() {
	ticker := time.NewTicker(m.healthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.reportHealth()
		}
	}
}

// reportHealth reports the health of this instance.
func (m *InstanceManager) reportHealth() {
	// Check health if a checker is configured.
	if m.healthChecker != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := m.healthChecker.Check(ctx)

		m.mu.Lock()
		if err != nil {
			m.info.State = InstanceStateUnhealthy
		} else if m.info.State == InstanceStateUnhealthy {
			m.info.State = InstanceStateReady
		}
		m.mu.Unlock()
	}

	// Update last heartbeat.
	m.mu.Lock()
	m.info.LastHeartbeat = time.Now()
	m.mu.Unlock()

	// Publish health report if NATS is available.
	if m.nc != nil {
		// In a real implementation, we would serialize and publish the info.
		m.mu.RLock()
		infoID := m.info.ID
		m.mu.RUnlock()
		_ = m.nc.Publish(m.healthSubject, []byte(infoID))
	}
}

// handleHealthReport handles a health report from another instance.
func (m *InstanceManager) handleHealthReport(msg *nats.Msg) {
	// In a real implementation, we would deserialize the health report.
	instanceID := string(msg.Data)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.instances[instanceID]; !ok {
		m.instances[instanceID] = &InstanceInfo{
			ID:      instanceID,
			State:   InstanceStateReady,
			Metrics: &InstanceMetrics{},
		}
	}

	m.instances[instanceID].LastHeartbeat = time.Now()
	m.instances[instanceID].State = InstanceStateReady
}

// RecordTransfer records a transfer.
func (m *InstanceManager) RecordTransfer(bytes int64) {
	atomic.AddInt64(&m.info.Metrics.TransfersTotal, 1)
	atomic.AddInt64(&m.info.Metrics.BytesTransferred, bytes)
}

// RecordError records an error.
func (m *InstanceManager) RecordError() {
	atomic.AddInt64(&m.info.Metrics.ErrorsTotal, 1)
}

// IncrementActiveTransfers increments the active transfer count.
func (m *InstanceManager) IncrementActiveTransfers() {
	atomic.AddInt64(&m.info.Metrics.TransfersActive, 1)
}

// DecrementActiveTransfers decrements the active transfer count.
func (m *InstanceManager) DecrementActiveTransfers() {
	atomic.AddInt64(&m.info.Metrics.TransfersActive, -1)
}

// HealthCheck represents a health check result.
type HealthCheck struct {
	// Healthy indicates if the instance is healthy.
	Healthy bool

	// Checks contains individual check results.
	Checks map[string]CheckResult
}

// CheckResult represents the result of a single health check.
type CheckResult struct {
	// Healthy indicates if this check passed.
	Healthy bool

	// Message provides details about the check.
	Message string

	// Duration is how long the check took.
	Duration time.Duration
}

// HealthHandler provides HTTP health check endpoints.
type HealthHandler struct {
	manager *InstanceManager
	checks  []NamedHealthChecker
	mu      sync.RWMutex
}

// NamedHealthChecker is a health checker with a name.
type NamedHealthChecker struct {
	Name    string
	Checker HealthChecker
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(manager *InstanceManager) *HealthHandler {
	return &HealthHandler{
		manager: manager,
		checks:  make([]NamedHealthChecker, 0),
	}
}

// AddCheck adds a health check.
func (h *HealthHandler) AddCheck(name string, checker HealthChecker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks = append(h.checks, NamedHealthChecker{Name: name, Checker: checker})
}

// Check performs all health checks and returns the result.
func (h *HealthHandler) Check(ctx context.Context) *HealthCheck {
	h.mu.RLock()
	checks := make([]NamedHealthChecker, len(h.checks))
	copy(checks, h.checks)
	h.mu.RUnlock()

	result := &HealthCheck{
		Healthy: true,
		Checks:  make(map[string]CheckResult),
	}

	for _, check := range checks {
		start := time.Now()
		err := check.Checker.Check(ctx)
		duration := time.Since(start)

		checkResult := CheckResult{
			Healthy:  err == nil,
			Duration: duration,
		}
		if err != nil {
			checkResult.Message = err.Error()
			result.Healthy = false
		} else {
			checkResult.Message = "ok"
		}

		result.Checks[check.Name] = checkResult
	}

	return result
}

// LivenessCheck returns true if the instance is alive.
func (h *HealthHandler) LivenessCheck() bool {
	return h.manager.info.State != InstanceStateStopped
}

// ReadinessCheck returns true if the instance is ready to serve requests.
func (h *HealthHandler) ReadinessCheck() bool {
	return h.manager.info.State == InstanceStateReady
}
