// Package protocols provides protocol adapters for proxy agent communication.
package protocols

import (
	"context"
	"fmt"
	"sync"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// Registry manages protocol adapter factories.
type Registry struct {
	mu          sync.RWMutex
	adapters    map[ProtocolType]AdapterFactory
	ftAdapters  map[ProtocolType]FileTransferAdapterFactory
	tunAdapters map[ProtocolType]TunnelAdapterFactory
	ncAdapters   map[ProtocolType]NetconfAdapterFactory
	rcAdapters   map[ProtocolType]RestconfAdapterFactory
	gnmiAdapters map[ProtocolType]GNMIAdapterFactory
}

// NewRegistry creates a new adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters:    make(map[ProtocolType]AdapterFactory),
		ftAdapters:  make(map[ProtocolType]FileTransferAdapterFactory),
		tunAdapters: make(map[ProtocolType]TunnelAdapterFactory),
		ncAdapters:   make(map[ProtocolType]NetconfAdapterFactory),
		rcAdapters:   make(map[ProtocolType]RestconfAdapterFactory),
		gnmiAdapters: make(map[ProtocolType]GNMIAdapterFactory),
	}
}

// Register registers an adapter factory for a protocol type.
func (r *Registry) Register(protocol ProtocolType, factory AdapterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[protocol] = factory
}

// RegisterFileTransfer registers a file transfer adapter factory.
func (r *Registry) RegisterFileTransfer(protocol ProtocolType, factory FileTransferAdapterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ftAdapters[protocol] = factory
}

// RegisterTunnel registers a tunnel adapter factory.
func (r *Registry) RegisterTunnel(protocol ProtocolType, factory TunnelAdapterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tunAdapters[protocol] = factory
}

// RegisterNetconf registers a NETCONF adapter factory.
func (r *Registry) RegisterNetconf(protocol ProtocolType, factory NetconfAdapterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ncAdapters[protocol] = factory
}

// Create creates a new adapter for the specified protocol.
func (r *Registry) Create(protocol ProtocolType, config *ConnectionConfig) (ProtocolAdapter, error) {
	r.mu.RLock()
	factory, ok := r.adapters[protocol]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no adapter registered for protocol: %s", protocol)
	}

	if config == nil {
		config = DefaultConnectionConfig()
	}

	return factory(config)
}

// CreateFileTransfer creates a new file transfer adapter.
func (r *Registry) CreateFileTransfer(protocol ProtocolType, config *ConnectionConfig) (FileTransferAdapter, error) {
	r.mu.RLock()
	factory, ok := r.ftAdapters[protocol]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no file transfer adapter registered for protocol: %s", protocol)
	}

	if config == nil {
		config = DefaultConnectionConfig()
	}

	return factory(config)
}

// CreateTunnel creates a new tunnel adapter.
func (r *Registry) CreateTunnel(protocol ProtocolType, config *ConnectionConfig) (TunnelAdapter, error) {
	r.mu.RLock()
	factory, ok := r.tunAdapters[protocol]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no tunnel adapter registered for protocol: %s", protocol)
	}

	if config == nil {
		config = DefaultConnectionConfig()
	}

	return factory(config)
}

// Has returns true if an adapter is registered for the protocol.
func (r *Registry) Has(protocol ProtocolType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.adapters[protocol]
	return ok
}

// HasFileTransfer returns true if a file transfer adapter is registered.
func (r *Registry) HasFileTransfer(protocol ProtocolType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.ftAdapters[protocol]
	return ok
}

// HasTunnel returns true if a tunnel adapter is registered.
func (r *Registry) HasTunnel(protocol ProtocolType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tunAdapters[protocol]
	return ok
}

// CreateNetconf creates a new NETCONF adapter.
func (r *Registry) CreateNetconf(protocol ProtocolType, config *ConnectionConfig) (NetconfAdapter, error) {
	r.mu.RLock()
	factory, ok := r.ncAdapters[protocol]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no NETCONF adapter registered for protocol: %s", protocol)
	}

	if config == nil {
		config = DefaultConnectionConfig()
	}

	return factory(config)
}

// HasNetconf returns true if a NETCONF adapter is registered.
func (r *Registry) HasNetconf(protocol ProtocolType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.ncAdapters[protocol]
	return ok
}

// RegisterRestconf registers a RESTCONF adapter factory.
func (r *Registry) RegisterRestconf(protocol ProtocolType, factory RestconfAdapterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rcAdapters[protocol] = factory
}

// CreateRestconf creates a new RESTCONF adapter.
func (r *Registry) CreateRestconf(protocol ProtocolType, config *ConnectionConfig) (RestconfAdapter, error) {
	r.mu.RLock()
	factory, ok := r.rcAdapters[protocol]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no RESTCONF adapter registered for protocol: %s", protocol)
	}

	if config == nil {
		config = DefaultConnectionConfig()
	}

	return factory(config)
}

// HasRestconf returns true if a RESTCONF adapter is registered.
func (r *Registry) HasRestconf(protocol ProtocolType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.rcAdapters[protocol]
	return ok
}

// RegisterGNMI registers a gNMI adapter factory.
func (r *Registry) RegisterGNMI(protocol ProtocolType, factory GNMIAdapterFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gnmiAdapters[protocol] = factory
}

// CreateGNMI creates a new gNMI adapter.
func (r *Registry) CreateGNMI(protocol ProtocolType, config *ConnectionConfig) (GNMIAdapter, error) {
	r.mu.RLock()
	factory, ok := r.gnmiAdapters[protocol]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no gNMI adapter registered for protocol: %s", protocol)
	}

	if config == nil {
		config = DefaultConnectionConfig()
	}

	return factory(config)
}

// HasGNMI returns true if a gNMI adapter is registered.
func (r *Registry) HasGNMI(protocol ProtocolType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.gnmiAdapters[protocol]
	return ok
}

// ListProtocols returns all registered protocol types.
func (r *Registry) ListProtocols() []ProtocolType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	protocols := make([]ProtocolType, 0, len(r.adapters))
	for p := range r.adapters {
		protocols = append(protocols, p)
	}
	return protocols
}

// Unregister removes an adapter factory.
func (r *Registry) Unregister(protocol ProtocolType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.adapters, protocol)
	delete(r.ftAdapters, protocol)
	delete(r.tunAdapters, protocol)
	delete(r.ncAdapters, protocol)
	delete(r.rcAdapters, protocol)
	delete(r.gnmiAdapters, protocol)
}

// DefaultRegistry is the default global adapter registry.
var DefaultRegistry = NewRegistry()

// Register registers an adapter factory in the default registry.
func Register(protocol ProtocolType, factory AdapterFactory) {
	DefaultRegistry.Register(protocol, factory)
}

// RegisterFileTransfer registers a file transfer adapter in the default registry.
func RegisterFileTransfer(protocol ProtocolType, factory FileTransferAdapterFactory) {
	DefaultRegistry.RegisterFileTransfer(protocol, factory)
}

// RegisterTunnel registers a tunnel adapter in the default registry.
func RegisterTunnel(protocol ProtocolType, factory TunnelAdapterFactory) {
	DefaultRegistry.RegisterTunnel(protocol, factory)
}

// RegisterNetconf registers a NETCONF adapter in the default registry.
func RegisterNetconf(protocol ProtocolType, factory NetconfAdapterFactory) {
	DefaultRegistry.RegisterNetconf(protocol, factory)
}

// Create creates an adapter from the default registry.
func Create(protocol ProtocolType, config *ConnectionConfig) (ProtocolAdapter, error) {
	return DefaultRegistry.Create(protocol, config)
}

// CreateFileTransfer creates a file transfer adapter from the default registry.
func CreateFileTransfer(protocol ProtocolType, config *ConnectionConfig) (FileTransferAdapter, error) {
	return DefaultRegistry.CreateFileTransfer(protocol, config)
}

// CreateTunnel creates a tunnel adapter from the default registry.
func CreateTunnel(protocol ProtocolType, config *ConnectionConfig) (TunnelAdapter, error) {
	return DefaultRegistry.CreateTunnel(protocol, config)
}

// CreateNetconf creates a NETCONF adapter from the default registry.
func CreateNetconf(protocol ProtocolType, config *ConnectionConfig) (NetconfAdapter, error) {
	return DefaultRegistry.CreateNetconf(protocol, config)
}

// RegisterRestconf registers a RESTCONF adapter in the default registry.
func RegisterRestconf(protocol ProtocolType, factory RestconfAdapterFactory) {
	DefaultRegistry.RegisterRestconf(protocol, factory)
}

// CreateRestconf creates a RESTCONF adapter from the default registry.
func CreateRestconf(protocol ProtocolType, config *ConnectionConfig) (RestconfAdapter, error) {
	return DefaultRegistry.CreateRestconf(protocol, config)
}

// RegisterGNMI registers a gNMI adapter in the default registry.
func RegisterGNMI(protocol ProtocolType, factory GNMIAdapterFactory) {
	DefaultRegistry.RegisterGNMI(protocol, factory)
}

// CreateGNMI creates a gNMI adapter from the default registry.
func CreateGNMI(protocol ProtocolType, config *ConnectionConfig) (GNMIAdapter, error) {
	return DefaultRegistry.CreateGNMI(protocol, config)
}

// AdapterPool manages a pool of connected adapters.
type AdapterPool struct {
	mu       sync.Mutex
	adapters map[string]ProtocolAdapter
	registry *Registry
}

// NewAdapterPool creates a new adapter pool.
func NewAdapterPool(registry *Registry) *AdapterPool {
	if registry == nil {
		registry = DefaultRegistry
	}
	return &AdapterPool{
		adapters: make(map[string]ProtocolAdapter),
		registry: registry,
	}
}

// Get gets or creates an adapter for the device.
func (p *AdapterPool) Get(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) (ProtocolAdapter, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check for existing adapter
	key := device.ID
	if adapter, ok := p.adapters[key]; ok && adapter.IsConnected() {
		return adapter, nil
	}

	// Determine protocol type from device
	protocol := ProtocolType(device.Protocol)
	if protocol == "" {
		protocol = ProtocolSSH // Default to SSH
	}

	// Create new adapter
	adapter, err := p.registry.Create(protocol, DefaultConnectionConfig())
	if err != nil {
		return nil, err
	}

	// Connect
	if err := adapter.Connect(ctx, device, cred); err != nil {
		return nil, err
	}

	p.adapters[key] = adapter
	return adapter, nil
}

// Release releases an adapter back to the pool.
func (p *AdapterPool) Release(deviceID string) {
	// No-op for now - adapters stay connected until explicitly closed
}

// Close closes and removes an adapter from the pool.
func (p *AdapterPool) Close(ctx context.Context, deviceID string) error {
	p.mu.Lock()
	adapter, ok := p.adapters[deviceID]
	if ok {
		delete(p.adapters, deviceID)
	}
	p.mu.Unlock()

	if ok && adapter != nil {
		return adapter.Disconnect(ctx)
	}
	return nil
}

// CloseAll closes all adapters in the pool.
func (p *AdapterPool) CloseAll(ctx context.Context) error {
	p.mu.Lock()
	adapters := make(map[string]ProtocolAdapter)
	for k, v := range p.adapters {
		adapters[k] = v
	}
	p.adapters = make(map[string]ProtocolAdapter)
	p.mu.Unlock()

	var lastErr error
	for _, adapter := range adapters {
		if err := adapter.Disconnect(ctx); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// Count returns the number of adapters in the pool.
func (p *AdapterPool) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.adapters)
}

// ActiveCount returns the number of connected adapters.
func (p *AdapterPool) ActiveCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	count := 0
	for _, adapter := range p.adapters {
		if adapter.IsConnected() {
			count++
		}
	}
	return count
}
