// Package proxy implements proxy agent support for managing devices that cannot
// run native Keystone Core agents.
package proxy

import (
	"context"
	"sync"
	"time"
)

// HealthCheckerConfig configures the health checker.
type HealthCheckerConfig struct {
	// Registry is the device registry.
	Registry DeviceRegistry

	// Executor is the proxied executor for performing health checks.
	Executor ProxiedExecutor

	// CheckInterval is how often to check device health.
	CheckInterval time.Duration

	// CheckTimeout is the timeout for individual health checks.
	CheckTimeout time.Duration

	// MaxConcurrent limits the number of concurrent health checks.
	MaxConcurrent int

	// StaleThreshold is how long before a device is considered stale.
	StaleThreshold time.Duration

	// OnHealthChanged is called when a device's health status changes.
	OnHealthChanged func(deviceID string, oldStatus, newStatus DeviceStatus)

	// OnDeviceStale is called when a device is considered stale.
	OnDeviceStale func(deviceID string)
}

// HealthChecker performs periodic health checks on registered devices.
type HealthChecker struct {
	config *HealthCheckerConfig

	// Semaphore for limiting concurrent checks
	sem chan struct{}

	// Track last check times
	lastCheck   map[string]time.Time
	lastCheckMu sync.RWMutex
}

// NewHealthChecker creates a new health checker.
func NewHealthChecker(config *HealthCheckerConfig) *HealthChecker {
	if config.CheckInterval == 0 {
		config.CheckInterval = 30 * time.Second
	}
	if config.CheckTimeout == 0 {
		config.CheckTimeout = 10 * time.Second
	}
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 10
	}
	if config.StaleThreshold == 0 {
		config.StaleThreshold = 5 * time.Minute
	}

	return &HealthChecker{
		config:    config,
		sem:       make(chan struct{}, config.MaxConcurrent),
		lastCheck: make(map[string]time.Time),
	}
}

// Run starts the health checker loop.
func (h *HealthChecker) Run(ctx context.Context) {
	ticker := time.NewTicker(h.config.CheckInterval)
	defer ticker.Stop()

	// Perform initial check
	h.checkAllDevices(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.checkAllDevices(ctx)
		}
	}
}

// checkAllDevices checks all registered devices.
func (h *HealthChecker) checkAllDevices(ctx context.Context) {
	devices, err := h.config.Registry.List(ctx, nil)
	if err != nil {
		return
	}

	var wg sync.WaitGroup

	for _, device := range devices {
		// Check if device is stale
		if h.config.OnDeviceStale != nil {
			h.lastCheckMu.RLock()
			lastCheck, exists := h.lastCheck[device.ID]
			h.lastCheckMu.RUnlock()

			if exists && time.Since(lastCheck) > h.config.StaleThreshold {
				h.config.OnDeviceStale(device.ID)
			}
		}

		wg.Add(1)
		go func(d *ProxiedDevice) {
			defer wg.Done()
			h.checkDevice(ctx, d)
		}(device)
	}

	wg.Wait()
}

// checkDevice performs a health check on a single device.
func (h *HealthChecker) checkDevice(ctx context.Context, device *ProxiedDevice) {
	// Acquire semaphore
	select {
	case h.sem <- struct{}{}:
		defer func() { <-h.sem }()
	case <-ctx.Done():
		return
	}

	// Create timeout context
	checkCtx, cancel := context.WithTimeout(ctx, h.config.CheckTimeout)
	defer cancel()

	// Perform health check
	result, err := h.config.Executor.Check(checkCtx, device.ID)

	// Record check time
	h.lastCheckMu.Lock()
	h.lastCheck[device.ID] = time.Now()
	h.lastCheckMu.Unlock()

	// Determine new status
	var newStatus DeviceStatus
	var message string

	if err != nil {
		newStatus = DeviceStatusUnreachable
		message = err.Error()
	} else {
		newStatus = result.Status
		message = result.Message
	}

	// Update status if changed
	oldStatus := device.Status
	if oldStatus != newStatus {
		if err := h.config.Registry.UpdateStatus(ctx, device.ID, newStatus, message); err == nil {
			if h.config.OnHealthChanged != nil {
				h.config.OnHealthChanged(device.ID, oldStatus, newStatus)
			}
		}
	} else {
		// Update last seen even if status unchanged
		_ = h.config.Registry.UpdateStatus(ctx, device.ID, newStatus, message)
	}
}

// CheckDevice performs an immediate health check on a specific device.
func (h *HealthChecker) CheckDevice(ctx context.Context, deviceID string) (*DeviceHealthResult, error) {
	device, err := h.config.Registry.Get(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	// Create timeout context
	checkCtx, cancel := context.WithTimeout(ctx, h.config.CheckTimeout)
	defer cancel()

	// Perform health check
	result, err := h.config.Executor.Check(checkCtx, device.ID)
	if err != nil {
		return &DeviceHealthResult{
			DeviceID:  deviceID,
			Status:    DeviceStatusUnreachable,
			Message:   err.Error(),
			CheckTime: time.Now(),
		}, nil
	}

	// Record check time
	h.lastCheckMu.Lock()
	h.lastCheck[deviceID] = time.Now()
	h.lastCheckMu.Unlock()

	// Update registry
	oldStatus := device.Status
	if oldStatus != result.Status {
		if err := h.config.Registry.UpdateStatus(ctx, deviceID, result.Status, result.Message); err == nil {
			if h.config.OnHealthChanged != nil {
				h.config.OnHealthChanged(deviceID, oldStatus, result.Status)
			}
		}
	}

	return result, nil
}

// GetLastCheckTime returns the last time a device was checked.
func (h *HealthChecker) GetLastCheckTime(deviceID string) (time.Time, bool) {
	h.lastCheckMu.RLock()
	defer h.lastCheckMu.RUnlock()
	t, ok := h.lastCheck[deviceID]
	return t, ok
}

// ClearCheckHistory clears the check history for a device.
func (h *HealthChecker) ClearCheckHistory(deviceID string) {
	h.lastCheckMu.Lock()
	defer h.lastCheckMu.Unlock()
	delete(h.lastCheck, deviceID)
}

// HealthStats contains health check statistics.
type HealthStats struct {
	// TotalChecks is the total number of health checks performed.
	TotalChecks int64

	// SuccessfulChecks is the number of successful checks.
	SuccessfulChecks int64

	// FailedChecks is the number of failed checks.
	FailedChecks int64

	// AverageLatency is the average check latency.
	AverageLatency time.Duration

	// LastCheckTime is when the last check cycle completed.
	LastCheckTime time.Time
}

// HealthStatusSummary summarizes device health across the registry.
type HealthStatusSummary struct {
	// Total is the total number of devices.
	Total int

	// Online is the number of online devices.
	Online int

	// Offline is the number of offline devices.
	Offline int

	// Degraded is the number of degraded devices.
	Degraded int

	// Unreachable is the number of unreachable devices.
	Unreachable int

	// Unknown is the number of devices with unknown status.
	Unknown int

	// AuthFailed is the number of devices with auth failures.
	AuthFailed int

	// ByType groups counts by device type.
	ByType map[DeviceType]int

	// ByProtocol groups counts by protocol.
	ByProtocol map[ProtocolType]int
}

// GetHealthSummary returns a summary of device health.
func (h *HealthChecker) GetHealthSummary(ctx context.Context) (*HealthStatusSummary, error) {
	devices, err := h.config.Registry.List(ctx, nil)
	if err != nil {
		return nil, err
	}

	summary := &HealthStatusSummary{
		Total:      len(devices),
		ByType:     make(map[DeviceType]int),
		ByProtocol: make(map[ProtocolType]int),
	}

	for _, device := range devices {
		switch device.Status {
		case DeviceStatusOnline:
			summary.Online++
		case DeviceStatusOffline:
			summary.Offline++
		case DeviceStatusDegraded:
			summary.Degraded++
		case DeviceStatusUnreachable:
			summary.Unreachable++
		case DeviceStatusAuthFailed:
			summary.AuthFailed++
		default:
			summary.Unknown++
		}

		summary.ByType[device.Type]++
		summary.ByProtocol[device.Protocol]++
	}

	return summary, nil
}
