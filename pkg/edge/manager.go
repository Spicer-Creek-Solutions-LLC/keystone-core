package edge

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
)

// DefaultManager implements the Manager interface
type DefaultManager struct {
	config    *Config
	mode      OperationMode
	connected bool
	cache     Cache
	mu        sync.RWMutex

	lastConnected     time.Time
	reconnectAttempts int
	startTime         time.Time

	// CPU tracking
	lastCPUPercent float64
	lastCPUUpdate  time.Time
	cpuMu          sync.RWMutex
}

// NewManager creates a new edge manager
func NewManager(config *Config) (Manager, error) {
	if config == nil {
		config = DefaultEdgeConfig()
	}

	// Create cache
	cache, err := NewFileCache(config.LocalCachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	// Determine initial mode
	mode := ModeOnline
	if config.EnableLightweightMode {
		mode = ModeLightweight
	}

	return &DefaultManager{
		config:        config,
		mode:          mode,
		connected:     true,
		cache:         cache,
		lastConnected: time.Now(),
		startTime:     time.Now(),
	}, nil
}

// GetMode returns current operation mode
func (m *DefaultManager) GetMode() OperationMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mode
}

// SetMode sets the operation mode
func (m *DefaultManager) SetMode(mode OperationMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.mode = mode
	return nil
}

// getCPUUsage returns the current CPU usage percentage.
// If the cached value is recent (within 5 seconds), it returns the cached value.
// Otherwise, it performs a quick non-blocking check using gopsutil.
func (m *DefaultManager) getCPUUsage() float64 {
	m.cpuMu.RLock()
	// If we have a recent CPU measurement, use it
	if time.Since(m.lastCPUUpdate) < 5*time.Second && m.lastCPUPercent > 0 {
		cpu := m.lastCPUPercent
		m.cpuMu.RUnlock()
		return cpu
	}
	m.cpuMu.RUnlock()

	// Get CPU usage with a short sampling interval (100ms)
	// This blocks briefly but provides accurate data
	percents, err := cpu.Percent(100*time.Millisecond, false)
	if err != nil || len(percents) == 0 {
		return 0
	}

	cpuPercent := percents[0]

	// Cache the result
	m.cpuMu.Lock()
	m.lastCPUPercent = cpuPercent
	m.lastCPUUpdate = time.Now()
	m.cpuMu.Unlock()

	return cpuPercent
}

// GetStatus returns current edge status
func (m *DefaultManager) GetStatus() (*Status, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get cache size
	cacheSize, err := m.cache.GetSize()
	if err != nil {
		cacheSize = 0
	}

	// Get cache stats
	stats, err := m.cache.GetStats()
	if err != nil {
		stats = &CacheStats{}
	}

	// Get memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memoryUsageMB := int(memStats.Alloc / 1024 / 1024)

	// Get CPU usage
	cpuPercent := m.getCPUUsage()

	// Check if resource constrained
	resourceConstrained := false
	if m.config.EnableLightweightMode {
		if memoryUsageMB > m.config.MaxMemoryMB {
			resourceConstrained = true
		}
		// Also consider CPU constraint if CPU is very high
		if cpuPercent > 90 {
			resourceConstrained = true
		}
	}

	status := &Status{
		Mode:                m.mode,
		Connected:           m.connected,
		LastConnected:       m.lastConnected,
		ReconnectAttempts:   m.reconnectAttempts,
		CacheSize:           cacheSize,
		CachedStatesCount:   stats.TotalEntries,
		CachedCommandsCount: 0,
		MemoryUsageMB:       memoryUsageMB,
		CPUUsagePercent:     int(cpuPercent),
		UptimeSeconds:       int64(time.Since(m.startTime).Seconds()),
		ResourceConstrained: resourceConstrained,
	}

	return status, nil
}

// IsConnected checks if connected to control plane
func (m *DefaultManager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

// SetConnected updates connection status
func (m *DefaultManager) SetConnected(connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	previousState := m.connected
	m.connected = connected

	if connected {
		m.lastConnected = time.Now()
		m.reconnectAttempts = 0

		// Transition to online mode if we were offline
		if !previousState && m.config.EnableOfflineMode {
			m.mode = ModeOnline
		}
	} else {
		// Transition to offline mode if enabled
		if m.config.EnableOfflineMode {
			m.mode = ModeOffline
		}
	}
}

// GetCache returns the local cache
func (m *DefaultManager) GetCache() Cache {
	return m.cache
}

// CheckResourceConstraints checks if resources are constrained
func (m *DefaultManager) CheckResourceConstraints() (bool, error) {
	if !m.config.EnableLightweightMode {
		return false, nil
	}

	// Get memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	memoryUsageMB := int(memStats.Alloc / 1024 / 1024)

	// Check memory constraint
	if memoryUsageMB > m.config.MaxMemoryMB {
		return true, nil
	}

	// Check CPU constraint - consider constrained if CPU is above 90%
	cpuPercent := m.getCPUUsage()
	if cpuPercent > 90 {
		return true, nil
	}

	return false, nil
}

// HandleDisconnect handles disconnection from control plane
func (m *DefaultManager) HandleDisconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.config.EnableOfflineMode {
		return fmt.Errorf("offline mode not enabled")
	}

	// Transition to offline mode
	m.mode = ModeOffline
	m.connected = false

	// Prune expired cache entries to free up space
	if err := m.cache.Prune(); err != nil {
		// Log error but continue
		fmt.Printf("Warning: failed to prune cache: %v\n", err)
	}

	return nil
}

// HandleReconnect handles reconnection to control plane
func (m *DefaultManager) HandleReconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.reconnectAttempts++

	// Check if we've exceeded max attempts
	if m.config.MaxReconnectAttempts > 0 && m.reconnectAttempts > m.config.MaxReconnectAttempts {
		return fmt.Errorf("exceeded maximum reconnect attempts")
	}

	// If reconnect succeeds, this will be handled by SetConnected
	return nil
}

// Global default manager instance
var defaultManager Manager

// InitDefaultManager initializes the default edge manager
func InitDefaultManager(config *Config) error {
	mgr, err := NewManager(config)
	if err != nil {
		return err
	}
	defaultManager = mgr
	return nil
}

// GetDefaultManager returns the default edge manager
func GetDefaultManager() Manager {
	return defaultManager
}
