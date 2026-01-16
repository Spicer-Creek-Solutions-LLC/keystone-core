// Package proxy implements proxy agent support for managing devices that cannot
// run native Keystone Core agents.
package proxy

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ManagerConfig configures the proxy agent manager.
type ManagerConfig struct {
	// AgentID is the unique identifier for this proxy agent.
	AgentID string

	// ClusterName is the Keystone Core cluster name.
	ClusterName string

	// HealthCheckInterval is how often to check device health.
	HealthCheckInterval time.Duration

	// HealthCheckTimeout is the timeout for health checks.
	HealthCheckTimeout time.Duration

	// MaxConcurrentHealthChecks limits parallel health checks.
	MaxConcurrentHealthChecks int

	// StaleDeviceThreshold is how long before a device is considered stale.
	StaleDeviceThreshold time.Duration

	// DeviceConfigPath is the path to device configuration file.
	DeviceConfigPath string

	// Registry is an optional custom device registry.
	// If nil, an InMemoryDeviceRegistry will be used.
	//
	// For production deployments with persistence requirements, use SQLiteDeviceRegistry:
	//   registry, _ := NewSQLiteDeviceRegistry(&SQLiteDeviceRegistryConfig{Path: "/data/devices.db"})
	//   config.Registry = registry
	//
	// Limitations of InMemoryDeviceRegistry (default):
	//   - All data is lost on proxy agent restart
	//   - Cannot be shared across multiple proxy agent instances
	//   - Suitable for: development, testing, single-instance deployments <100 devices
	//
	// Limitations of SQLiteDeviceRegistry:
	//   - Slower for high-frequency updates due to disk I/O
	//   - Requires filesystem access
	//   - Suitable for: production, persistence required, >100 devices
	Registry DeviceRegistry
}

// DefaultManagerConfig returns a ManagerConfig with sensible defaults.
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		HealthCheckInterval:       30 * time.Second,
		HealthCheckTimeout:        10 * time.Second,
		MaxConcurrentHealthChecks: 10,
		StaleDeviceThreshold:      5 * time.Minute,
	}
}

// Manager implements ProxyAgentManager for coordinating proxy operations.
type Manager struct {
	config   *ManagerConfig
	registry DeviceRegistry
	executor ProxiedExecutor

	state     atomic.Value // ProxyAgentState
	startTime time.Time

	// Statistics
	commandsExecuted  atomic.Int64
	commandsSucceeded atomic.Int64
	commandsFailed    atomic.Int64
	healthChecksTotal atomic.Int64

	// Health checker
	healthChecker *HealthChecker

	// Lifecycle management
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	startMu    sync.Mutex
	shutdownCh chan struct{}
}

// NewManager creates a new proxy agent manager.
//
// By default, an InMemoryDeviceRegistry is used which loses all data on restart.
// For production deployments requiring persistence, provide a SQLiteDeviceRegistry
// via config.Registry:
//
//	sqliteRegistry, err := NewSQLiteDeviceRegistry(&SQLiteDeviceRegistryConfig{
//	    Path:    "/data/proxy-devices.db",
//	    WALMode: true,
//	})
//	if err != nil {
//	    return nil, err
//	}
//	config := DefaultManagerConfig()
//	config.Registry = sqliteRegistry
//	manager, err := NewManager(config)
func NewManager(config *ManagerConfig) (*Manager, error) {
	if config == nil {
		config = DefaultManagerConfig()
	}

	if config.AgentID == "" {
		return nil, fmt.Errorf("agent ID is required")
	}

	// Use provided registry or default to in-memory
	registry := config.Registry
	if registry == nil {
		registry = NewInMemoryDeviceRegistry()
	}

	m := &Manager{
		config:     config,
		registry:   registry,
		shutdownCh: make(chan struct{}),
	}

	m.state.Store(ProxyAgentStateStopped)

	return m, nil
}

// SetExecutor sets the proxied executor.
func (m *Manager) SetExecutor(executor ProxiedExecutor) {
	m.executor = executor
}

// Start starts the proxy agent.
func (m *Manager) Start(ctx context.Context) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()

	currentState := m.state.Load().(ProxyAgentState)
	if currentState == ProxyAgentStateRunning {
		return nil
	}

	m.state.Store(ProxyAgentStateStarting)

	// Create cancellable context
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.startTime = time.Now()

	// Initialize health checker
	if m.executor != nil {
		m.healthChecker = NewHealthChecker(&HealthCheckerConfig{
			Registry:          m.registry,
			Executor:          m.executor,
			CheckInterval:     m.config.HealthCheckInterval,
			CheckTimeout:      m.config.HealthCheckTimeout,
			MaxConcurrent:     m.config.MaxConcurrentHealthChecks,
			StaleThreshold:    m.config.StaleDeviceThreshold,
			OnHealthChanged:   m.onDeviceHealthChanged,
			OnDeviceStale:     m.onDeviceStale,
		})

		// Start health checker
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			m.healthChecker.Run(m.ctx)
		}()
	}

	m.state.Store(ProxyAgentStateRunning)
	return nil
}

// Stop stops the proxy agent.
func (m *Manager) Stop(ctx context.Context) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()

	currentState := m.state.Load().(ProxyAgentState)
	if currentState == ProxyAgentStateStopped {
		return nil
	}

	m.state.Store(ProxyAgentStateStopping)

	// Cancel context to stop all goroutines
	if m.cancel != nil {
		m.cancel()
	}

	// Wait for goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines finished
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Second):
		return fmt.Errorf("timeout waiting for proxy agent to stop")
	}

	// Close all device connections
	if m.executor != nil {
		devices, err := m.registry.GetOnlineDevices(ctx)
		if err == nil {
			for _, device := range devices {
				_ = m.executor.Close(ctx, device.ID)
			}
		}
	}

	m.state.Store(ProxyAgentStateStopped)
	close(m.shutdownCh)

	return nil
}

// State returns the current state.
func (m *Manager) State() ProxyAgentState {
	return m.state.Load().(ProxyAgentState)
}

// Stats returns current statistics.
func (m *Manager) Stats() *ProxyAgentStats {
	registryStats, _ := m.registry.GetStats(context.Background())

	stats := &ProxyAgentStats{
		DevicesTotal:      registryStats.TotalDevices,
		DevicesOnline:     registryStats.OnlineDevices,
		DevicesOffline:    registryStats.OfflineDevices,
		DevicesDegraded:   registryStats.DegradedDevices,
		CommandsExecuted:  m.commandsExecuted.Load(),
		CommandsSucceeded: m.commandsSucceeded.Load(),
		CommandsFailed:    m.commandsFailed.Load(),
		HealthChecksTotal: m.healthChecksTotal.Load(),
		StartTime:         m.startTime,
	}

	if !m.startTime.IsZero() {
		stats.Uptime = time.Since(m.startTime)
	}

	return stats
}

// Registry returns the device registry.
func (m *Manager) Registry() DeviceRegistry {
	return m.registry
}

// Executor returns the proxied executor.
func (m *Manager) Executor() ProxiedExecutor {
	return m.executor
}

// RefreshDevice triggers an immediate health check for a device.
func (m *Manager) RefreshDevice(ctx context.Context, deviceID string) error {
	if m.state.Load().(ProxyAgentState) != ProxyAgentStateRunning {
		return ErrProxyAgentNotRunning
	}

	device, err := m.registry.Get(ctx, deviceID)
	if err != nil {
		return err
	}

	if m.executor == nil {
		return fmt.Errorf("no executor configured")
	}

	// Perform health check
	result, err := m.executor.Check(ctx, device.ID)
	if err != nil {
		return err
	}

	// Update status based on result
	return m.registry.UpdateStatus(ctx, deviceID, result.Status, result.Message)
}

// ReloadConfig reloads the proxy agent configuration.
func (m *Manager) ReloadConfig(ctx context.Context) error {
	if m.state.Load().(ProxyAgentState) != ProxyAgentStateRunning {
		return ErrProxyAgentNotRunning
	}

	// For now, just return nil. In a full implementation, this would:
	// 1. Re-read device configuration file
	// 2. Register new devices
	// 3. Update existing devices
	// 4. Unregister removed devices
	return nil
}

// ExecuteCommand executes a command on a proxied device.
func (m *Manager) ExecuteCommand(ctx context.Context, req *ProxiedExecuteRequest) (*ProxiedExecuteResult, error) {
	if m.state.Load().(ProxyAgentState) != ProxyAgentStateRunning {
		return nil, ErrProxyAgentNotRunning
	}

	if m.executor == nil {
		return nil, fmt.Errorf("no executor configured")
	}

	// Verify device exists and is available
	device, err := m.registry.Get(ctx, req.DeviceID)
	if err != nil {
		return nil, err
	}

	if !device.Status.IsAvailable() {
		return nil, fmt.Errorf("device %s is not available (status: %s)", device.ID, device.Status)
	}

	// Execute command
	m.commandsExecuted.Add(1)
	result, err := m.executor.Execute(ctx, req)
	if err != nil {
		m.commandsFailed.Add(1)
		return nil, err
	}

	if result.Success() {
		m.commandsSucceeded.Add(1)
	} else {
		m.commandsFailed.Add(1)
	}

	return result, nil
}

// ExecuteCommandWithOutput executes a command with streaming output.
func (m *Manager) ExecuteCommandWithOutput(ctx context.Context, req *ProxiedExecuteRequest, handler OutputHandler) (*ProxiedExecuteResult, error) {
	if m.state.Load().(ProxyAgentState) != ProxyAgentStateRunning {
		return nil, ErrProxyAgentNotRunning
	}

	if m.executor == nil {
		return nil, fmt.Errorf("no executor configured")
	}

	// Verify device exists and is available
	device, err := m.registry.Get(ctx, req.DeviceID)
	if err != nil {
		return nil, err
	}

	if !device.Status.IsAvailable() {
		return nil, fmt.Errorf("device %s is not available (status: %s)", device.ID, device.Status)
	}

	// Execute command with streaming
	m.commandsExecuted.Add(1)
	result, err := m.executor.ExecuteWithOutput(ctx, req, handler)
	if err != nil {
		m.commandsFailed.Add(1)
		return nil, err
	}

	if result.Success() {
		m.commandsSucceeded.Add(1)
	} else {
		m.commandsFailed.Add(1)
	}

	return result, nil
}

// RegisterDevice registers a new device with the proxy agent.
func (m *Manager) RegisterDevice(ctx context.Context, device *ProxiedDevice) error {
	// Set proxy agent ID
	device.ProxyAgentID = m.config.AgentID

	// Register in registry
	if err := m.registry.Register(ctx, device); err != nil {
		return err
	}

	// Perform initial health check if running
	if m.state.Load().(ProxyAgentState) == ProxyAgentStateRunning && m.executor != nil {
		go func() {
			checkCtx, cancel := context.WithTimeout(context.Background(), m.config.HealthCheckTimeout)
			defer cancel()
			_ = m.RefreshDevice(checkCtx, device.ID)
		}()
	}

	return nil
}

// UnregisterDevice removes a device from the proxy agent.
func (m *Manager) UnregisterDevice(ctx context.Context, deviceID string) error {
	// Close connection if executor is available
	if m.executor != nil {
		_ = m.executor.Close(ctx, deviceID)
	}

	return m.registry.Unregister(ctx, deviceID)
}

// GetAgentID returns the proxy agent ID.
func (m *Manager) GetAgentID() string {
	return m.config.AgentID
}

// GetClusterName returns the cluster name.
func (m *Manager) GetClusterName() string {
	return m.config.ClusterName
}

// WaitForShutdown waits for the manager to shut down.
func (m *Manager) WaitForShutdown() <-chan struct{} {
	return m.shutdownCh
}

// onDeviceHealthChanged is called when a device's health status changes.
func (m *Manager) onDeviceHealthChanged(deviceID string, oldStatus, newStatus DeviceStatus) {
	// Log or emit event here
	// In a full implementation, this would:
	// 1. Emit an event to the event bus
	// 2. Update metrics
	// 3. Trigger any configured reactors
}

// onDeviceStale is called when a device is considered stale.
func (m *Manager) onDeviceStale(deviceID string) {
	// Mark device as unreachable
	ctx := context.Background()
	_ = m.registry.UpdateStatus(ctx, deviceID, DeviceStatusUnreachable, "device is stale")
}

// Ensure Manager implements ProxyAgentManager.
var _ ProxyAgentManager = (*Manager)(nil)
