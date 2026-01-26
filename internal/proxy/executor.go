// Package proxy implements proxy agent support for managing devices that cannot
// run native Keystone Core agents.
package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ProtocolAdapter defines the interface for protocol-specific command execution.
// Implementations include SSH, SNMP, REST, WinRM adapters.
type ProtocolAdapter interface {
	// Protocol returns the protocol type this adapter handles.
	Protocol() ProtocolType

	// Connect establishes a connection to the device.
	Connect(ctx context.Context, device *ProxiedDevice) error

	// Execute executes a command on the connected device.
	Execute(ctx context.Context, req *ProxiedExecuteRequest) (*ProxiedExecuteResult, error)

	// ExecuteWithOutput executes a command with streaming output.
	ExecuteWithOutput(ctx context.Context, req *ProxiedExecuteRequest, handler OutputHandler) (*ProxiedExecuteResult, error)

	// HealthCheck performs a health check on the device.
	HealthCheck(ctx context.Context, device *ProxiedDevice) (*DeviceHealthResult, error)

	// GetCapabilities returns the device's capabilities.
	GetCapabilities(ctx context.Context, device *ProxiedDevice) (*DeviceCapabilities, error)

	// Close closes the connection to the device.
	Close(ctx context.Context, device *ProxiedDevice) error

	// IsConnected returns true if connected to the device.
	IsConnected(deviceID string) bool
}

// CredentialProvider provides credentials for device connections.
type CredentialProvider interface {
	// GetCredential retrieves a credential by reference.
	GetCredential(ctx context.Context, ref string) (*Credential, error)
}

// Credential represents authentication credentials for a device.
type Credential struct {
	// Type is the credential type (password, key, snmp_community, token, etc.)
	Type string

	// Username is the username for authentication.
	Username string

	// Password is the password (for password auth).
	Password string

	// PrivateKey is the private key (for SSH key auth).
	PrivateKey []byte

	// Passphrase is the passphrase for the private key.
	Passphrase string

	// Community is the SNMP community string.
	Community string

	// Token is an API token or bearer token.
	Token string

	// Additional fields for various auth types
	Metadata map[string]string

	// ExpiresAt is when this credential expires (zero = no expiry).
	ExpiresAt time.Time
}

// IsExpired returns true if the credential has expired.
func (c *Credential) IsExpired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

// RoutingExecutor routes commands to the appropriate protocol adapter.
type RoutingExecutor struct {
	mu       sync.RWMutex
	adapters map[ProtocolType]ProtocolAdapter
	registry DeviceRegistry
	credProv CredentialProvider
}

// NewRoutingExecutor creates a new routing executor.
func NewRoutingExecutor(registry DeviceRegistry, credProv CredentialProvider) *RoutingExecutor {
	return &RoutingExecutor{
		adapters: make(map[ProtocolType]ProtocolAdapter),
		registry: registry,
		credProv: credProv,
	}
}

// RegisterAdapter registers a protocol adapter.
func (e *RoutingExecutor) RegisterAdapter(adapter ProtocolAdapter) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.adapters[adapter.Protocol()] = adapter
}

// GetAdapter returns the adapter for a protocol.
func (e *RoutingExecutor) GetAdapter(protocol ProtocolType) (ProtocolAdapter, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	adapter, ok := e.adapters[protocol]
	return adapter, ok
}

// Execute executes a command on a proxied device.
func (e *RoutingExecutor) Execute(ctx context.Context, req *ProxiedExecuteRequest) (*ProxiedExecuteResult, error) {
	startTime := time.Now()

	// Get device from registry
	device, err := e.registry.Get(ctx, req.DeviceID)
	if err != nil {
		return nil, err
	}

	// Get adapter for protocol
	adapter, ok := e.GetAdapter(device.Protocol)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProtocol, device.Protocol)
	}

	// Ensure connection
	if !adapter.IsConnected(device.ID) {
		if err := adapter.Connect(ctx, device); err != nil {
			return nil, fmt.Errorf("failed to connect to device: %w", err)
		}
	}

	// Execute command
	result, err := adapter.Execute(ctx, req)
	if err != nil {
		return nil, err
	}

	// Set timing info
	result.StartTime = startTime
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}

// ExecuteWithOutput executes a command and streams output via callback.
func (e *RoutingExecutor) ExecuteWithOutput(ctx context.Context, req *ProxiedExecuteRequest, handler OutputHandler) (*ProxiedExecuteResult, error) {
	startTime := time.Now()

	// Get device from registry
	device, err := e.registry.Get(ctx, req.DeviceID)
	if err != nil {
		return nil, err
	}

	// Get adapter for protocol
	adapter, ok := e.GetAdapter(device.Protocol)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProtocol, device.Protocol)
	}

	// Ensure connection
	if !adapter.IsConnected(device.ID) {
		if err := adapter.Connect(ctx, device); err != nil {
			return nil, fmt.Errorf("failed to connect to device: %w", err)
		}
	}

	// Execute command with streaming
	result, err := adapter.ExecuteWithOutput(ctx, req, handler)
	if err != nil {
		return nil, err
	}

	// Set timing info
	result.StartTime = startTime
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, nil
}

// Check performs a health check on a device.
func (e *RoutingExecutor) Check(ctx context.Context, deviceID string) (*DeviceHealthResult, error) {
	// Get device from registry
	device, err := e.registry.Get(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	// Get adapter for protocol
	adapter, ok := e.GetAdapter(device.Protocol)
	if !ok {
		return &DeviceHealthResult{
			DeviceID:  deviceID,
			Status:    DeviceStatusUnknown,
			Message:   fmt.Sprintf("unsupported protocol: %s", device.Protocol),
			CheckTime: time.Now(),
		}, nil
	}

	// Perform health check
	return adapter.HealthCheck(ctx, device)
}

// GetCapabilities returns the device's capabilities.
func (e *RoutingExecutor) GetCapabilities(ctx context.Context, deviceID string) (*DeviceCapabilities, error) {
	// Get device from registry
	device, err := e.registry.Get(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	// Get adapter for protocol
	adapter, ok := e.GetAdapter(device.Protocol)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedProtocol, device.Protocol)
	}

	return adapter.GetCapabilities(ctx, device)
}

// Close closes any open connections to the device.
func (e *RoutingExecutor) Close(ctx context.Context, deviceID string) error {
	// Get device from registry
	device, err := e.registry.Get(ctx, deviceID)
	if err != nil {
		return err
	}

	// Get adapter for protocol
	adapter, ok := e.GetAdapter(device.Protocol)
	if !ok {
		return nil // No adapter, nothing to close
	}

	return adapter.Close(ctx, device)
}

// CloseAll closes all open connections.
func (e *RoutingExecutor) CloseAll(ctx context.Context) error {
	e.mu.RLock()
	adapters := make([]ProtocolAdapter, 0, len(e.adapters))
	for _, adapter := range e.adapters {
		adapters = append(adapters, adapter)
	}
	e.mu.RUnlock()

	devices, err := e.registry.List(ctx, nil)
	if err != nil {
		return err
	}

	var lastErr error
	for _, device := range devices {
		for _, adapter := range adapters {
			if adapter.Protocol() == device.Protocol && adapter.IsConnected(device.ID) {
				if err := adapter.Close(ctx, device); err != nil {
					lastErr = err
				}
			}
		}
	}

	return lastErr
}

// Ensure RoutingExecutor implements ProxiedExecutor.
var _ ProxiedExecutor = (*RoutingExecutor)(nil)

// StubAdapter is a test adapter that returns success for all operations.
type StubAdapter struct {
	protocol    ProtocolType
	connected   map[string]bool
	connectedMu sync.RWMutex
}

// NewStubAdapter creates a new stub adapter for testing.
func NewStubAdapter(protocol ProtocolType) *StubAdapter {
	return &StubAdapter{
		protocol:  protocol,
		connected: make(map[string]bool),
	}
}

// Protocol returns the protocol type.
func (a *StubAdapter) Protocol() ProtocolType {
	return a.protocol
}

// Connect establishes a connection to the device.
func (a *StubAdapter) Connect(ctx context.Context, device *ProxiedDevice) error {
	a.connectedMu.Lock()
	defer a.connectedMu.Unlock()
	a.connected[device.ID] = true
	return nil
}

// Execute executes a command on the connected device.
func (a *StubAdapter) Execute(ctx context.Context, req *ProxiedExecuteRequest) (*ProxiedExecuteResult, error) {
	return &ProxiedExecuteResult{
		DeviceID: req.DeviceID,
		ExitCode: 0,
		Stdout:   []byte("stub output"),
		Stderr:   []byte{},
	}, nil
}

// ExecuteWithOutput executes a command with streaming output.
func (a *StubAdapter) ExecuteWithOutput(ctx context.Context, req *ProxiedExecuteRequest, handler OutputHandler) (*ProxiedExecuteResult, error) {
	output := []byte("stub output")
	handler(req.DeviceID, false, output)
	return &ProxiedExecuteResult{
		DeviceID: req.DeviceID,
		ExitCode: 0,
		Stdout:   output,
		Stderr:   []byte{},
	}, nil
}

// HealthCheck performs a health check on the device.
func (a *StubAdapter) HealthCheck(ctx context.Context, device *ProxiedDevice) (*DeviceHealthResult, error) {
	return &DeviceHealthResult{
		DeviceID:  device.ID,
		Status:    DeviceStatusOnline,
		Message:   "stub health check passed",
		Latency:   time.Millisecond,
		CheckTime: time.Now(),
	}, nil
}

// GetCapabilities returns the device's capabilities.
func (a *StubAdapter) GetCapabilities(ctx context.Context, device *ProxiedDevice) (*DeviceCapabilities, error) {
	return &DeviceCapabilities{
		DeviceID:              device.ID,
		CanExecuteCommands:    true,
		CanTransferFiles:      false,
		CanManageConfig:       false,
		CanReboot:             false,
		SupportedOperations:   []string{"execute"},
		MaxConcurrentCommands: 1,
	}, nil
}

// Close closes the connection to the device.
func (a *StubAdapter) Close(ctx context.Context, device *ProxiedDevice) error {
	a.connectedMu.Lock()
	defer a.connectedMu.Unlock()
	delete(a.connected, device.ID)
	return nil
}

// IsConnected returns true if connected to the device.
func (a *StubAdapter) IsConnected(deviceID string) bool {
	a.connectedMu.RLock()
	defer a.connectedMu.RUnlock()
	return a.connected[deviceID]
}

// Ensure StubAdapter implements ProtocolAdapter.
var _ ProtocolAdapter = (*StubAdapter)(nil)
