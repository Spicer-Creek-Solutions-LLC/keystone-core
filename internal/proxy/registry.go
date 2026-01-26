// Package proxy implements proxy agent support for managing devices that cannot
// run native Keystone Core agents.
package proxy

import (
	"context"
	"sort"
	"sync"
	"time"
)

// InMemoryDeviceRegistry implements DeviceRegistry with in-memory storage.
// It provides thread-safe CRUD operations for proxied devices.
type InMemoryDeviceRegistry struct {
	mu      sync.RWMutex
	devices map[string]*ProxiedDevice

	// observers are notified of device changes
	observers []DeviceObserver
	obsMu     sync.RWMutex
}

// DeviceObserver is notified of device registry changes.
type DeviceObserver interface {
	// OnDeviceRegistered is called when a device is registered.
	OnDeviceRegistered(device *ProxiedDevice)
	// OnDeviceUnregistered is called when a device is unregistered.
	OnDeviceUnregistered(deviceID string)
	// OnDeviceUpdated is called when a device is updated.
	OnDeviceUpdated(device *ProxiedDevice)
	// OnDeviceStatusChanged is called when a device status changes.
	OnDeviceStatusChanged(deviceID string, oldStatus, newStatus DeviceStatus)
}

// NewInMemoryDeviceRegistry creates a new in-memory device registry.
func NewInMemoryDeviceRegistry() *InMemoryDeviceRegistry {
	return &InMemoryDeviceRegistry{
		devices:   make(map[string]*ProxiedDevice),
		observers: make([]DeviceObserver, 0),
	}
}

// AddObserver adds an observer to be notified of device changes.
func (r *InMemoryDeviceRegistry) AddObserver(observer DeviceObserver) {
	r.obsMu.Lock()
	defer r.obsMu.Unlock()
	r.observers = append(r.observers, observer)
}

// RemoveObserver removes an observer.
func (r *InMemoryDeviceRegistry) RemoveObserver(observer DeviceObserver) {
	r.obsMu.Lock()
	defer r.obsMu.Unlock()
	for i, obs := range r.observers {
		if obs == observer {
			r.observers = append(r.observers[:i], r.observers[i+1:]...)
			return
		}
	}
}

// notifyRegistered notifies observers of a device registration.
func (r *InMemoryDeviceRegistry) notifyRegistered(device *ProxiedDevice) {
	r.obsMu.RLock()
	defer r.obsMu.RUnlock()
	for _, obs := range r.observers {
		obs.OnDeviceRegistered(device)
	}
}

// notifyUnregistered notifies observers of a device unregistration.
func (r *InMemoryDeviceRegistry) notifyUnregistered(deviceID string) {
	r.obsMu.RLock()
	defer r.obsMu.RUnlock()
	for _, obs := range r.observers {
		obs.OnDeviceUnregistered(deviceID)
	}
}

// notifyUpdated notifies observers of a device update.
func (r *InMemoryDeviceRegistry) notifyUpdated(device *ProxiedDevice) {
	r.obsMu.RLock()
	defer r.obsMu.RUnlock()
	for _, obs := range r.observers {
		obs.OnDeviceUpdated(device)
	}
}

// notifyStatusChanged notifies observers of a device status change.
func (r *InMemoryDeviceRegistry) notifyStatusChanged(deviceID string, oldStatus, newStatus DeviceStatus) {
	r.obsMu.RLock()
	defer r.obsMu.RUnlock()
	for _, obs := range r.observers {
		obs.OnDeviceStatusChanged(deviceID, oldStatus, newStatus)
	}
}

// Register registers a new proxied device.
func (r *InMemoryDeviceRegistry) Register(ctx context.Context, device *ProxiedDevice) error {
	if err := device.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.devices[device.ID]; exists {
		return ErrDeviceAlreadyExists
	}

	// Set registration time
	now := time.Now()
	clone := device.Clone()
	clone.RegisteredAt = now
	clone.UpdatedAt = now
	if clone.Status == "" {
		clone.Status = DeviceStatusUnknown
	}

	r.devices[device.ID] = clone

	// Notify observers (outside lock would be better, but simple for now)
	go r.notifyRegistered(clone.Clone())

	return nil
}

// Unregister removes a device from the registry.
func (r *InMemoryDeviceRegistry) Unregister(ctx context.Context, deviceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.devices[deviceID]; !exists {
		return ErrDeviceNotFound
	}

	delete(r.devices, deviceID)

	go r.notifyUnregistered(deviceID)

	return nil
}

// Get retrieves a device by ID.
func (r *InMemoryDeviceRegistry) Get(ctx context.Context, deviceID string) (*ProxiedDevice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	device, exists := r.devices[deviceID]
	if !exists {
		return nil, ErrDeviceNotFound
	}

	return device.Clone(), nil
}

// List lists devices with optional filtering.
func (r *InMemoryDeviceRegistry) List(ctx context.Context, filter *DeviceFilter) ([]*ProxiedDevice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect matching devices
	var results []*ProxiedDevice
	for _, device := range r.devices {
		if matchesFilter(device, filter) {
			results = append(results, device.Clone())
		}
	}

	// Sort by ID for consistent ordering
	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})

	// Apply offset and limit
	if filter != nil {
		if filter.Offset > 0 {
			if filter.Offset >= len(results) {
				return []*ProxiedDevice{}, nil
			}
			results = results[filter.Offset:]
		}
		if filter.Limit > 0 && filter.Limit < len(results) {
			results = results[:filter.Limit]
		}
	}

	return results, nil
}

// matchesFilter returns true if the device matches the filter criteria.
func matchesFilter(device *ProxiedDevice, filter *DeviceFilter) bool {
	if filter == nil {
		return true
	}

	// Filter by IDs
	if len(filter.IDs) > 0 {
		found := false
		for _, id := range filter.IDs {
			if device.ID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by proxy agent ID
	if filter.ProxyAgentID != "" && device.ProxyAgentID != filter.ProxyAgentID {
		return false
	}

	// Filter by types
	if len(filter.Types) > 0 {
		found := false
		for _, t := range filter.Types {
			if device.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by vendors
	if len(filter.Vendors) > 0 {
		found := false
		for _, v := range filter.Vendors {
			if device.Vendor == v {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by protocols
	if len(filter.Protocols) > 0 {
		found := false
		for _, p := range filter.Protocols {
			if device.Protocol == p {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by statuses
	if len(filter.Statuses) > 0 {
		found := false
		for _, s := range filter.Statuses {
			if device.Status == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by labels (all must match)
	if len(filter.Labels) > 0 {
		for key, value := range filter.Labels {
			if deviceValue, exists := device.Labels[key]; !exists || deviceValue != value {
				return false
			}
		}
	}

	return true
}

// Update updates device metadata, status, or configuration.
func (r *InMemoryDeviceRegistry) Update(ctx context.Context, device *ProxiedDevice) error {
	if err := device.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.devices[device.ID]
	if !exists {
		return ErrDeviceNotFound
	}

	// Preserve registration time
	clone := device.Clone()
	clone.RegisteredAt = existing.RegisteredAt
	clone.UpdatedAt = time.Now()

	r.devices[device.ID] = clone

	go r.notifyUpdated(clone.Clone())

	return nil
}

// GetByProxyAgent returns all devices for a specific proxy agent.
func (r *InMemoryDeviceRegistry) GetByProxyAgent(ctx context.Context, proxyAgentID string) ([]*ProxiedDevice, error) {
	return r.List(ctx, &DeviceFilter{ProxyAgentID: proxyAgentID})
}

// UpdateStatus updates only the device status.
func (r *InMemoryDeviceRegistry) UpdateStatus(ctx context.Context, deviceID string, status DeviceStatus, message string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	device, exists := r.devices[deviceID]
	if !exists {
		return ErrDeviceNotFound
	}

	oldStatus := device.Status
	device.Status = status
	device.StatusMessage = message
	device.UpdatedAt = time.Now()
	if status == DeviceStatusOnline {
		device.LastSeen = time.Now()
	}

	if oldStatus != status {
		go r.notifyStatusChanged(deviceID, oldStatus, status)
	}

	return nil
}

// Count returns the total number of registered devices.
func (r *InMemoryDeviceRegistry) Count(ctx context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.devices), nil
}

// GetDevicesByStatus returns devices grouped by status.
func (r *InMemoryDeviceRegistry) GetDevicesByStatus(ctx context.Context) (map[DeviceStatus][]*ProxiedDevice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[DeviceStatus][]*ProxiedDevice)
	for _, device := range r.devices {
		result[device.Status] = append(result[device.Status], device.Clone())
	}

	return result, nil
}

// GetOnlineDevices returns all devices that are currently online.
func (r *InMemoryDeviceRegistry) GetOnlineDevices(ctx context.Context) ([]*ProxiedDevice, error) {
	return r.List(ctx, &DeviceFilter{Statuses: []DeviceStatus{DeviceStatusOnline}})
}

// GetOfflineDevices returns all devices that are currently offline or unreachable.
func (r *InMemoryDeviceRegistry) GetOfflineDevices(ctx context.Context) ([]*ProxiedDevice, error) {
	return r.List(ctx, &DeviceFilter{
		Statuses: []DeviceStatus{DeviceStatusOffline, DeviceStatusUnreachable},
	})
}

// GetStaleDevices returns devices that haven't been seen for longer than the threshold.
func (r *InMemoryDeviceRegistry) GetStaleDevices(ctx context.Context, threshold time.Duration) ([]*ProxiedDevice, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cutoff := time.Now().Add(-threshold)
	var results []*ProxiedDevice

	for _, device := range r.devices {
		if !device.LastSeen.IsZero() && device.LastSeen.Before(cutoff) {
			results = append(results, device.Clone())
		}
	}

	return results, nil
}

// Clear removes all devices from the registry.
func (r *InMemoryDeviceRegistry) Clear(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.devices = make(map[string]*ProxiedDevice)
	return nil
}

// Stats returns statistics about registered devices.
type RegistryStats struct {
	TotalDevices    int
	OnlineDevices   int
	OfflineDevices  int
	DegradedDevices int
	UnknownDevices  int
	ByType          map[DeviceType]int
	ByProtocol      map[ProtocolType]int
	ByProxyAgent    map[string]int
}

// GetStats returns statistics about registered devices.
func (r *InMemoryDeviceRegistry) GetStats(ctx context.Context) (*RegistryStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := &RegistryStats{
		TotalDevices: len(r.devices),
		ByType:       make(map[DeviceType]int),
		ByProtocol:   make(map[ProtocolType]int),
		ByProxyAgent: make(map[string]int),
	}

	for _, device := range r.devices {
		// Count by status
		switch device.Status {
		case DeviceStatusOnline:
			stats.OnlineDevices++
		case DeviceStatusOffline, DeviceStatusUnreachable:
			stats.OfflineDevices++
		case DeviceStatusDegraded:
			stats.DegradedDevices++
		default:
			stats.UnknownDevices++
		}

		// Count by type
		stats.ByType[device.Type]++

		// Count by protocol
		stats.ByProtocol[device.Protocol]++

		// Count by proxy agent
		stats.ByProxyAgent[device.ProxyAgentID]++
	}

	return stats, nil
}

// Ensure InMemoryDeviceRegistry implements DeviceRegistry.
var _ DeviceRegistry = (*InMemoryDeviceRegistry)(nil)
