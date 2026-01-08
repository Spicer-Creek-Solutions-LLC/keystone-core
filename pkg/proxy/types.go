// Package proxy implements proxy agent support for managing devices that cannot
// run native Keystone Core agents. This includes legacy systems, network hardware,
// IoT devices, and appliances with locked-down operating systems.
package proxy

import (
	"context"
	"errors"
	"time"
)

// DeviceType identifies the type of proxied device.
type DeviceType string

const (
	DeviceTypeLinux    DeviceType = "linux"
	DeviceTypeWindows  DeviceType = "windows"
	DeviceTypeNetwork  DeviceType = "network"
	DeviceTypeFirewall DeviceType = "firewall"
	DeviceTypeRouter   DeviceType = "router"
	DeviceTypeSwitch   DeviceType = "switch"
	DeviceTypeAPM      DeviceType = "apm"
	DeviceTypeIoT      DeviceType = "iot"
	DeviceTypeCustom   DeviceType = "custom"
)

// String returns the string representation of the device type.
func (d DeviceType) String() string {
	return string(d)
}

// Valid returns true if the device type is valid.
func (d DeviceType) Valid() bool {
	switch d {
	case DeviceTypeLinux, DeviceTypeWindows, DeviceTypeNetwork,
		DeviceTypeFirewall, DeviceTypeRouter, DeviceTypeSwitch,
		DeviceTypeAPM, DeviceTypeIoT, DeviceTypeCustom:
		return true
	default:
		return false
	}
}

// ProtocolType identifies the communication protocol.
type ProtocolType string

const (
	ProtocolSSH   ProtocolType = "ssh"
	ProtocolSNMP  ProtocolType = "snmp"
	ProtocolREST  ProtocolType = "rest"
	ProtocolWinRM ProtocolType = "winrm"
)

// String returns the string representation of the protocol type.
func (p ProtocolType) String() string {
	return string(p)
}

// Valid returns true if the protocol type is valid.
func (p ProtocolType) Valid() bool {
	switch p {
	case ProtocolSSH, ProtocolSNMP, ProtocolREST, ProtocolWinRM:
		return true
	default:
		return false
	}
}

// DeviceStatus represents the current status of a proxied device.
type DeviceStatus string

const (
	DeviceStatusUnknown     DeviceStatus = "unknown"
	DeviceStatusOnline      DeviceStatus = "online"
	DeviceStatusOffline     DeviceStatus = "offline"
	DeviceStatusDegraded    DeviceStatus = "degraded"
	DeviceStatusUnreachable DeviceStatus = "unreachable"
	DeviceStatusAuthFailed  DeviceStatus = "auth_failed"
)

// String returns the string representation of the device status.
func (s DeviceStatus) String() string {
	return string(s)
}

// IsHealthy returns true if the status indicates a healthy device.
func (s DeviceStatus) IsHealthy() bool {
	return s == DeviceStatusOnline
}

// IsAvailable returns true if the device might be reachable.
func (s DeviceStatus) IsAvailable() bool {
	return s == DeviceStatusOnline || s == DeviceStatusDegraded
}

// ProxiedDevice represents a device managed through a proxy agent.
type ProxiedDevice struct {
	// ID is the unique identifier for this device.
	// It appears as a virtual agent ID in the control plane,
	// typically prefixed with the proxy agent ID: "proxy-dc1/switch-01"
	ID string

	// ProxyAgentID is the ID of the proxy agent managing this device.
	ProxyAgentID string

	// Name is a human-readable name for the device.
	Name string

	// Type is the device type (linux, network, firewall, etc.).
	Type DeviceType

	// Vendor is the device vendor (e.g., "cisco", "juniper", "pfsense").
	Vendor string

	// Model is the specific device model.
	Model string

	// ProfileID is the device profile used for interaction.
	ProfileID string

	// Protocol is the primary communication protocol.
	Protocol ProtocolType

	// Address is the network address (IP or hostname) of the device.
	Address string

	// Port is the port number for the protocol (0 = use default).
	Port int

	// CredentialRef is a reference to the credential to use.
	CredentialRef string

	// Metadata contains device-specific metadata.
	Metadata map[string]string

	// Labels are key-value pairs for targeting and organization.
	Labels map[string]string

	// Status is the current device status.
	Status DeviceStatus

	// StatusMessage provides additional status information.
	StatusMessage string

	// LastSeen is when the device was last successfully contacted.
	LastSeen time.Time

	// LastHealthCheck is when the last health check was performed.
	LastHealthCheck time.Time

	// HealthCheckInterval is how often to check device health.
	HealthCheckInterval time.Duration

	// RegisteredAt is when the device was registered.
	RegisteredAt time.Time

	// UpdatedAt is when the device was last updated.
	UpdatedAt time.Time
}

// Validate validates the device configuration.
func (d *ProxiedDevice) Validate() error {
	if d.ID == "" {
		return errors.New("device ID is required")
	}
	if d.ProxyAgentID == "" {
		return errors.New("proxy agent ID is required")
	}
	if !d.Type.Valid() {
		return errors.New("invalid device type")
	}
	if !d.Protocol.Valid() {
		return errors.New("invalid protocol type")
	}
	if d.Address == "" {
		return errors.New("device address is required")
	}
	if d.ProfileID == "" {
		return errors.New("profile ID is required")
	}
	return nil
}

// FullID returns the full agent ID as seen by the control plane.
// Format: "{proxyAgentID}/{deviceID}"
func (d *ProxiedDevice) FullID() string {
	return d.ProxyAgentID + "/" + d.ID
}

// Clone creates a deep copy of the device.
func (d *ProxiedDevice) Clone() *ProxiedDevice {
	clone := *d
	if d.Metadata != nil {
		clone.Metadata = make(map[string]string, len(d.Metadata))
		for k, v := range d.Metadata {
			clone.Metadata[k] = v
		}
	}
	if d.Labels != nil {
		clone.Labels = make(map[string]string, len(d.Labels))
		for k, v := range d.Labels {
			clone.Labels[k] = v
		}
	}
	return &clone
}

// DeviceFilter specifies criteria for filtering devices.
type DeviceFilter struct {
	// IDs filters by specific device IDs.
	IDs []string

	// ProxyAgentID filters by proxy agent.
	ProxyAgentID string

	// Types filters by device types.
	Types []DeviceType

	// Vendors filters by vendors.
	Vendors []string

	// Protocols filters by protocols.
	Protocols []ProtocolType

	// Statuses filters by device status.
	Statuses []DeviceStatus

	// Labels filters by label key-value pairs (all must match).
	Labels map[string]string

	// Limit limits the number of results.
	Limit int

	// Offset skips the first N results.
	Offset int
}

// DeviceRegistry manages the collection of proxied devices.
type DeviceRegistry interface {
	// Register registers a new proxied device.
	Register(ctx context.Context, device *ProxiedDevice) error

	// Unregister removes a device from the registry.
	Unregister(ctx context.Context, deviceID string) error

	// Get retrieves a device by ID.
	Get(ctx context.Context, deviceID string) (*ProxiedDevice, error)

	// List lists devices with optional filtering.
	List(ctx context.Context, filter *DeviceFilter) ([]*ProxiedDevice, error)

	// Update updates device metadata, status, or configuration.
	Update(ctx context.Context, device *ProxiedDevice) error

	// GetByProxyAgent returns all devices for a specific proxy agent.
	GetByProxyAgent(ctx context.Context, proxyAgentID string) ([]*ProxiedDevice, error)

	// UpdateStatus updates only the device status.
	UpdateStatus(ctx context.Context, deviceID string, status DeviceStatus, message string) error

	// Count returns the total number of registered devices.
	Count(ctx context.Context) (int, error)
}

// ProxiedExecuteRequest represents a command execution request for a proxied device.
type ProxiedExecuteRequest struct {
	// DeviceID is the target device ID.
	DeviceID string

	// Command is the command to execute.
	Command string

	// Args are command arguments.
	Args []string

	// Env is environment variables to set.
	Env map[string]string

	// WorkingDir is the working directory for command execution.
	WorkingDir string

	// Timeout is the maximum execution time.
	Timeout time.Duration

	// Shell specifies the shell to use (if applicable).
	Shell string

	// Interactive indicates if the command requires interactive input.
	Interactive bool

	// PTY indicates if a pseudo-terminal should be allocated.
	PTY bool

	// CorrelationID is used for tracing and logging.
	CorrelationID string
}

// ProxiedExecuteResult represents the result of command execution.
type ProxiedExecuteResult struct {
	// DeviceID is the device that executed the command.
	DeviceID string

	// ExitCode is the command exit code.
	ExitCode int

	// Stdout is the standard output.
	Stdout []byte

	// Stderr is the standard error output.
	Stderr []byte

	// Duration is how long the command took.
	Duration time.Duration

	// Error is any error that occurred.
	Error error

	// StartTime is when execution started.
	StartTime time.Time

	// EndTime is when execution completed.
	EndTime time.Time
}

// Success returns true if the command succeeded.
func (r *ProxiedExecuteResult) Success() bool {
	return r.Error == nil && r.ExitCode == 0
}

// DeviceHealthResult represents the result of a device health check.
type DeviceHealthResult struct {
	// DeviceID is the device that was checked.
	DeviceID string

	// Status is the resulting status.
	Status DeviceStatus

	// Message is a human-readable status message.
	Message string

	// Latency is the response time.
	Latency time.Duration

	// CheckTime is when the check was performed.
	CheckTime time.Time

	// Details contains additional health check details.
	Details map[string]interface{}
}

// DeviceCapabilities describes what a device supports.
type DeviceCapabilities struct {
	// DeviceID is the device ID.
	DeviceID string

	// CanExecuteCommands indicates if commands can be executed.
	CanExecuteCommands bool

	// CanTransferFiles indicates if file transfer is supported.
	CanTransferFiles bool

	// CanManageConfig indicates if config management is supported.
	CanManageConfig bool

	// CanReboot indicates if reboot is supported.
	CanReboot bool

	// SupportedOperations lists specific supported operations.
	SupportedOperations []string

	// MaxConcurrentCommands limits concurrent command execution.
	MaxConcurrentCommands int
}

// ProxiedExecutor executes commands on proxied devices.
type ProxiedExecutor interface {
	// Execute executes a command on a proxied device.
	Execute(ctx context.Context, req *ProxiedExecuteRequest) (*ProxiedExecuteResult, error)

	// ExecuteWithOutput executes a command and streams output via callback.
	ExecuteWithOutput(ctx context.Context, req *ProxiedExecuteRequest, outputHandler OutputHandler) (*ProxiedExecuteResult, error)

	// Check performs a health check on a device.
	Check(ctx context.Context, deviceID string) (*DeviceHealthResult, error)

	// GetCapabilities returns the device's capabilities.
	GetCapabilities(ctx context.Context, deviceID string) (*DeviceCapabilities, error)

	// Close closes any open connections to the device.
	Close(ctx context.Context, deviceID string) error
}

// OutputHandler is called with streaming output from command execution.
type OutputHandler func(deviceID string, isStderr bool, data []byte)

// ProxyAgentState represents the state of a proxy agent.
type ProxyAgentState string

const (
	ProxyAgentStateStopped   ProxyAgentState = "stopped"
	ProxyAgentStateStarting  ProxyAgentState = "starting"
	ProxyAgentStateRunning   ProxyAgentState = "running"
	ProxyAgentStateStopping  ProxyAgentState = "stopping"
	ProxyAgentStateDegraded  ProxyAgentState = "degraded"
)

// ProxyAgentStats contains statistics about a proxy agent.
type ProxyAgentStats struct {
	// DevicesTotal is the total number of registered devices.
	DevicesTotal int

	// DevicesOnline is the number of devices currently online.
	DevicesOnline int

	// DevicesOffline is the number of devices currently offline.
	DevicesOffline int

	// DevicesDegraded is the number of devices in degraded state.
	DevicesDegraded int

	// CommandsExecuted is the total commands executed.
	CommandsExecuted int64

	// CommandsSucceeded is the number of successful commands.
	CommandsSucceeded int64

	// CommandsFailed is the number of failed commands.
	CommandsFailed int64

	// HealthChecksTotal is the total health checks performed.
	HealthChecksTotal int64

	// StartTime is when the proxy agent started.
	StartTime time.Time

	// Uptime is how long the proxy agent has been running.
	Uptime time.Duration
}

// ProxyAgentManager manages the proxy agent lifecycle.
type ProxyAgentManager interface {
	// Start starts the proxy agent.
	Start(ctx context.Context) error

	// Stop stops the proxy agent.
	Stop(ctx context.Context) error

	// State returns the current state.
	State() ProxyAgentState

	// Stats returns current statistics.
	Stats() *ProxyAgentStats

	// Registry returns the device registry.
	Registry() DeviceRegistry

	// Executor returns the proxied executor.
	Executor() ProxiedExecutor

	// RefreshDevice triggers an immediate health check for a device.
	RefreshDevice(ctx context.Context, deviceID string) error

	// ReloadConfig reloads the proxy agent configuration.
	ReloadConfig(ctx context.Context) error
}

// Common errors.
var (
	ErrDeviceNotFound       = errors.New("device not found")
	ErrDeviceAlreadyExists  = errors.New("device already exists")
	ErrDeviceUnreachable    = errors.New("device unreachable")
	ErrDeviceAuthFailed     = errors.New("device authentication failed")
	ErrProxyAgentNotRunning = errors.New("proxy agent not running")
	ErrInvalidDeviceConfig  = errors.New("invalid device configuration")
	ErrUnsupportedProtocol  = errors.New("unsupported protocol")
	ErrCredentialNotFound   = errors.New("credential not found")
	ErrCredentialExpired    = errors.New("credential expired")
	ErrProfileNotFound      = errors.New("device profile not found")
	ErrCommandTimeout       = errors.New("command execution timeout")
	ErrConnectionPoolFull   = errors.New("connection pool is full")
)
