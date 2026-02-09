// Package protocols provides protocol adapters for proxy agent communication.
package protocols

import (
	"context"
	"io"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// ProtocolType identifies the protocol used to communicate with a device.
type ProtocolType string

const (
	// ProtocolSSH is the SSH protocol.
	ProtocolSSH ProtocolType = "ssh"
	// ProtocolSNMP is the SNMP protocol.
	ProtocolSNMP ProtocolType = "snmp"
	// ProtocolREST is the REST/HTTP protocol.
	ProtocolREST ProtocolType = "rest"
	// ProtocolWinRM is the WinRM protocol.
	ProtocolWinRM ProtocolType = "winrm"
	// ProtocolNETCONF is the NETCONF protocol (RFC 6241).
	ProtocolNETCONF ProtocolType = "netconf"
	// ProtocolRESTCONF is the RESTCONF protocol (RFC 8040).
	ProtocolRESTCONF ProtocolType = "restconf"
	// ProtocolTelnet is the Telnet protocol (RFC 854).
	ProtocolTelnet ProtocolType = "telnet"
	// ProtocolGNMI is the gNMI protocol (gRPC Network Management Interface).
	ProtocolGNMI ProtocolType = "gnmi"
)

// ProtocolAdapter defines the interface for protocol-specific communication.
type ProtocolAdapter interface {
	// Type returns the protocol type.
	Type() ProtocolType

	// Connect establishes a connection to the device.
	Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error

	// Execute executes a command or request on the device.
	Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResult, error)

	// Disconnect closes the connection to the device.
	Disconnect(ctx context.Context) error

	// HealthCheck performs a health check on the connection.
	HealthCheck(ctx context.Context) (*HealthCheckResult, error)

	// IsConnected returns true if currently connected.
	IsConnected() bool
}

// ExecuteRequest represents a command execution request.
type ExecuteRequest struct {
	// Command is the command to execute (for shell-based protocols).
	Command string `json:"command,omitempty"`

	// Args are command arguments.
	Args []string `json:"args,omitempty"`

	// Stdin is optional input to the command.
	Stdin io.Reader `json:"-"`

	// Environment variables for the command.
	Environment map[string]string `json:"environment,omitempty"`

	// WorkingDir is the working directory for command execution.
	WorkingDir string `json:"working_dir,omitempty"`

	// Timeout for the command execution.
	Timeout time.Duration `json:"timeout,omitempty"`

	// Shell specifies the shell to use (e.g., "/bin/bash", "cmd.exe").
	Shell string `json:"shell,omitempty"`

	// PTY requests a pseudo-terminal for the command.
	PTY bool `json:"pty,omitempty"`

	// PTYConfig configures the pseudo-terminal.
	PTYConfig *PTYConfig `json:"pty_config,omitempty"`

	// StreamOutput enables streaming stdout/stderr as they're produced.
	StreamOutput bool `json:"stream_output,omitempty"`

	// StdoutWriter receives stdout if StreamOutput is true.
	StdoutWriter io.Writer `json:"-"`

	// StderrWriter receives stderr if StreamOutput is true.
	StderrWriter io.Writer `json:"-"`
}

// PTYConfig configures a pseudo-terminal.
type PTYConfig struct {
	// Term is the terminal type (e.g., "xterm", "vt100").
	Term string `json:"term,omitempty"`

	// Rows is the terminal height.
	Rows uint32 `json:"rows,omitempty"`

	// Cols is the terminal width.
	Cols uint32 `json:"cols,omitempty"`

	// Width is the terminal width in pixels.
	Width uint32 `json:"width,omitempty"`

	// Height is the terminal height in pixels.
	Height uint32 `json:"height,omitempty"`
}

// DefaultPTYConfig returns a default PTY configuration.
func DefaultPTYConfig() *PTYConfig {
	return &PTYConfig{
		Term: "xterm",
		Rows: 24,
		Cols: 80,
	}
}

// ExecuteResult contains the result of a command execution.
type ExecuteResult struct {
	// ExitCode is the command exit code.
	ExitCode int `json:"exit_code"`

	// Stdout is the standard output.
	Stdout []byte `json:"stdout,omitempty"`

	// Stderr is the standard error output.
	Stderr []byte `json:"stderr,omitempty"`

	// Duration is how long the command took.
	Duration time.Duration `json:"duration"`

	// StartTime is when the command started.
	StartTime time.Time `json:"start_time"`

	// EndTime is when the command ended.
	EndTime time.Time `json:"end_time"`

	// Error is any error that occurred during execution.
	Error string `json:"error,omitempty"`

	// Timeout indicates if the command timed out.
	Timeout bool `json:"timeout,omitempty"`

	// Killed indicates if the command was killed.
	Killed bool `json:"killed,omitempty"`
}

// Success returns true if the command succeeded (exit code 0, no error).
func (r *ExecuteResult) Success() bool {
	return r.ExitCode == 0 && r.Error == "" && !r.Timeout && !r.Killed
}

// HealthCheckResult contains the result of a health check.
type HealthCheckResult struct {
	// Healthy indicates if the connection is healthy.
	Healthy bool `json:"healthy"`

	// Status is a human-readable status message.
	Status string `json:"status"`

	// Latency is the health check round-trip time.
	Latency time.Duration `json:"latency"`

	// LastCheck is when the health check was performed.
	LastCheck time.Time `json:"last_check"`

	// Consecutive failures since last success.
	ConsecutiveFailures int `json:"consecutive_failures,omitempty"`

	// Details contains protocol-specific health details.
	Details map[string]interface{} `json:"details,omitempty"`
}

// FileTransferAdapter extends ProtocolAdapter with file transfer capabilities.
type FileTransferAdapter interface {
	ProtocolAdapter

	// Upload uploads a file to the device.
	Upload(ctx context.Context, req *UploadRequest) (*TransferResult, error)

	// Download downloads a file from the device.
	Download(ctx context.Context, req *DownloadRequest) (*TransferResult, error)

	// Stat returns file information.
	Stat(ctx context.Context, path string) (*FileInfo, error)

	// ReadDir lists directory contents.
	ReadDir(ctx context.Context, path string) ([]FileInfo, error)

	// Mkdir creates a directory.
	Mkdir(ctx context.Context, path string, mode uint32) error

	// Remove removes a file or empty directory.
	Remove(ctx context.Context, path string) error
}

// UploadRequest represents a file upload request.
type UploadRequest struct {
	// LocalPath is the local file path to upload.
	LocalPath string `json:"local_path,omitempty"`

	// Content is the file content to upload (alternative to LocalPath).
	Content io.Reader `json:"-"`

	// RemotePath is the destination path on the device.
	RemotePath string `json:"remote_path"`

	// Mode is the file permissions.
	Mode uint32 `json:"mode,omitempty"`

	// Overwrite allows overwriting existing files.
	Overwrite bool `json:"overwrite,omitempty"`

	// CreateDirs creates parent directories if they don't exist.
	CreateDirs bool `json:"create_dirs,omitempty"`
}

// DownloadRequest represents a file download request.
type DownloadRequest struct {
	// RemotePath is the source path on the device.
	RemotePath string `json:"remote_path"`

	// LocalPath is the destination local path (optional).
	LocalPath string `json:"local_path,omitempty"`

	// Writer receives the file content (alternative to LocalPath).
	Writer io.Writer `json:"-"`
}

// TransferResult contains the result of a file transfer.
type TransferResult struct {
	// BytesTransferred is the number of bytes transferred.
	BytesTransferred int64 `json:"bytes_transferred"`

	// Duration is how long the transfer took.
	Duration time.Duration `json:"duration"`

	// Checksum is the file checksum (if computed).
	Checksum string `json:"checksum,omitempty"`

	// ChecksumType is the type of checksum (e.g., "sha256").
	ChecksumType string `json:"checksum_type,omitempty"`

	// Error is any error that occurred.
	Error string `json:"error,omitempty"`
}

// Success returns true if the transfer succeeded.
func (r *TransferResult) Success() bool {
	return r.Error == ""
}

// FileInfo contains information about a file.
type FileInfo struct {
	// Name is the file name.
	Name string `json:"name"`

	// Path is the full path.
	Path string `json:"path"`

	// Size is the file size in bytes.
	Size int64 `json:"size"`

	// Mode is the file permissions.
	Mode uint32 `json:"mode"`

	// ModTime is the modification time.
	ModTime time.Time `json:"mod_time"`

	// IsDir indicates if this is a directory.
	IsDir bool `json:"is_dir"`

	// IsSymlink indicates if this is a symbolic link.
	IsSymlink bool `json:"is_symlink"`

	// LinkTarget is the symlink target if IsSymlink is true.
	LinkTarget string `json:"link_target,omitempty"`

	// Owner is the file owner.
	Owner string `json:"owner,omitempty"`

	// Group is the file group.
	Group string `json:"group,omitempty"`
}

// TunnelAdapter extends ProtocolAdapter with port forwarding capabilities.
type TunnelAdapter interface {
	ProtocolAdapter

	// LocalForward creates a local port forward.
	LocalForward(ctx context.Context, req *ForwardRequest) (*Tunnel, error)

	// RemoteForward creates a remote port forward.
	RemoteForward(ctx context.Context, req *ForwardRequest) (*Tunnel, error)

	// DynamicForward creates a SOCKS proxy.
	DynamicForward(ctx context.Context, localAddr string) (*Tunnel, error)
}

// ForwardRequest represents a port forward request.
type ForwardRequest struct {
	// LocalHost is the local bind address.
	LocalHost string `json:"local_host"`

	// LocalPort is the local bind port.
	LocalPort int `json:"local_port"`

	// RemoteHost is the remote host to forward to.
	RemoteHost string `json:"remote_host"`

	// RemotePort is the remote port to forward to.
	RemotePort int `json:"remote_port"`
}

// Tunnel represents an active port forward.
type Tunnel struct {
	// ID is the tunnel identifier.
	ID string `json:"id"`

	// Type is the tunnel type (local, remote, dynamic).
	Type string `json:"type"`

	// LocalAddr is the local address.
	LocalAddr string `json:"local_addr"`

	// RemoteAddr is the remote address.
	RemoteAddr string `json:"remote_addr"`

	// Active indicates if the tunnel is active.
	Active bool `json:"active"`

	// BytesSent is the total bytes sent through the tunnel.
	BytesSent int64 `json:"bytes_sent"`

	// BytesReceived is the total bytes received through the tunnel.
	BytesReceived int64 `json:"bytes_received"`

	// Close closes the tunnel.
	Close func() error `json:"-"`
}

// ConnectionConfig contains common connection configuration.
type ConnectionConfig struct {
	// Timeout is the connection timeout.
	Timeout time.Duration `json:"timeout,omitempty"`

	// KeepAlive enables connection keep-alive.
	KeepAlive bool `json:"keep_alive,omitempty"`

	// KeepAliveInterval is the keep-alive interval.
	KeepAliveInterval time.Duration `json:"keep_alive_interval,omitempty"`

	// MaxRetries is the maximum number of connection retries.
	MaxRetries int `json:"max_retries,omitempty"`

	// RetryDelay is the delay between retries.
	RetryDelay time.Duration `json:"retry_delay,omitempty"`

	// ProxyHost is a proxy/bastion host for the connection.
	ProxyHost string `json:"proxy_host,omitempty"`

	// ProxyPort is the proxy/bastion port.
	ProxyPort int `json:"proxy_port,omitempty"`

	// ProxyCredential is the credential for the proxy.
	ProxyCredential credentials.Credential `json:"-"`
}

// DefaultConnectionConfig returns a default connection configuration.
func DefaultConnectionConfig() *ConnectionConfig {
	return &ConnectionConfig{
		Timeout:           30 * time.Second,
		KeepAlive:         true,
		KeepAliveInterval: 30 * time.Second,
		MaxRetries:        3,
		RetryDelay:        5 * time.Second,
	}
}

// AdapterMetrics contains metrics for a protocol adapter.
type AdapterMetrics struct {
	// ConnectionCount is the number of successful connections.
	ConnectionCount int64 `json:"connection_count"`

	// ConnectionErrors is the number of connection errors.
	ConnectionErrors int64 `json:"connection_errors"`

	// ExecutionCount is the number of executions.
	ExecutionCount int64 `json:"execution_count"`

	// ExecutionErrors is the number of execution errors.
	ExecutionErrors int64 `json:"execution_errors"`

	// BytesSent is the total bytes sent.
	BytesSent int64 `json:"bytes_sent"`

	// BytesReceived is the total bytes received.
	BytesReceived int64 `json:"bytes_received"`

	// AverageLatency is the average operation latency.
	AverageLatency time.Duration `json:"average_latency"`

	// LastActivity is the time of the last activity.
	LastActivity time.Time `json:"last_activity"`
}

// AdapterFactory creates protocol adapters.
type AdapterFactory func(config *ConnectionConfig) (ProtocolAdapter, error)

// FileTransferAdapterFactory creates file transfer adapters.
type FileTransferAdapterFactory func(config *ConnectionConfig) (FileTransferAdapter, error)

// TunnelAdapterFactory creates tunnel adapters.
type TunnelAdapterFactory func(config *ConnectionConfig) (TunnelAdapter, error)

// NetconfAdapterFactory creates NETCONF adapters.
type NetconfAdapterFactory func(config *ConnectionConfig) (NetconfAdapter, error)

// NetconfAdapter extends ProtocolAdapter with NETCONF-specific operations (RFC 6241).
type NetconfAdapter interface {
	ProtocolAdapter

	// GetConfig retrieves configuration from the specified datastore.
	GetConfig(ctx context.Context, source string, filter *NetconfFilter) ([]byte, error)

	// EditConfig modifies the specified datastore.
	EditConfig(ctx context.Context, target string, config []byte, opts *NetconfEditOptions) error

	// CopyConfig copies one datastore to another.
	CopyConfig(ctx context.Context, source, target string) error

	// DeleteConfig deletes the specified datastore.
	DeleteConfig(ctx context.Context, target string) error

	// Lock acquires a lock on the specified datastore.
	Lock(ctx context.Context, target string) error

	// Unlock releases a lock on the specified datastore.
	Unlock(ctx context.Context, target string) error

	// Commit commits the candidate configuration to running.
	Commit(ctx context.Context) error

	// DiscardChanges discards uncommitted candidate changes.
	DiscardChanges(ctx context.Context) error

	// Validate validates the specified datastore or candidate configuration.
	Validate(ctx context.Context, source string) error

	// Get retrieves running configuration and device state data.
	Get(ctx context.Context, filter *NetconfFilter) ([]byte, error)

	// ServerCapabilities returns the capabilities advertised by the server.
	ServerCapabilities() []string

	// SessionID returns the NETCONF session ID assigned by the server.
	SessionID() uint32
}

// NetconfFilter specifies a filter for NETCONF get/get-config operations.
type NetconfFilter struct {
	// Type is the filter type: "subtree" or "xpath".
	Type string `json:"type"`

	// Content is the filter body (XML subtree or XPath expression).
	Content string `json:"content"`
}

// NetconfEditOptions specifies options for NETCONF edit-config operations.
type NetconfEditOptions struct {
	// DefaultOperation is the default edit operation: "merge", "replace", "none".
	DefaultOperation string `json:"default_operation,omitempty"`

	// TestOption controls validation: "test-then-set", "set", "test-only".
	TestOption string `json:"test_option,omitempty"`

	// ErrorOption controls error handling: "stop-on-error", "continue-on-error", "rollback-on-error".
	ErrorOption string `json:"error_option,omitempty"`
}

// GNMIAdapterFactory creates gNMI adapters.
type GNMIAdapterFactory func(config *ConnectionConfig) (GNMIAdapter, error)

// GNMIAdapter extends ProtocolAdapter with gNMI-specific operations.
type GNMIAdapter interface {
	ProtocolAdapter

	// Capabilities retrieves the gNMI capabilities from the target.
	Capabilities(ctx context.Context) (*GNMICapabilitiesResult, error)

	// Get retrieves data from the specified paths.
	Get(ctx context.Context, paths []GNMIPath, opts *GNMIGetOptions) (*GNMIGetResult, error)

	// Set modifies data on the target.
	Set(ctx context.Context, req *GNMISetRequest) (*GNMISetResult, error)

	// Subscribe creates a subscription for streaming telemetry.
	Subscribe(ctx context.Context, req *GNMISubscribeRequest) (*GNMISubscription, error)

	// Reboot requests a device reboot via gNOI.
	Reboot(ctx context.Context, method GNMIRebootMethod, message string) error

	// Ping executes a network ping via gNOI.
	Ping(ctx context.Context, destination string, count int32, interval int64) ([]*GNMIPingResponse, error)

	// Traceroute executes a traceroute via gNOI.
	Traceroute(ctx context.Context, destination string, maxTTL int32) ([]*GNMITracerouteResponse, error)
}

// GNMIPath represents a gNMI path element.
type GNMIPath struct {
	// Elements is the list of path elements (e.g., ["interfaces", "interface[name=eth0]"]).
	Elements []string `json:"elements"`
	// Origin is the data model origin (e.g., "openconfig").
	Origin string `json:"origin,omitempty"`
	// Target is the target name for the path.
	Target string `json:"target,omitempty"`
}

// GNMICapabilitiesResult contains the result of a Capabilities RPC.
type GNMICapabilitiesResult struct {
	// SupportedModels lists the YANG models supported by the target.
	SupportedModels []GNMIModelData `json:"supported_models"`
	// SupportedEncodings lists the supported data encodings.
	SupportedEncodings []string `json:"supported_encodings"`
	// GNMIVersion is the gNMI protocol version.
	GNMIVersion string `json:"gnmi_version"`
}

// GNMIModelData describes a YANG model supported by the target.
type GNMIModelData struct {
	Name         string `json:"name"`
	Organization string `json:"organization"`
	Version      string `json:"version"`
}

// GNMIGetOptions specifies options for a Get RPC.
type GNMIGetOptions struct {
	// Encoding is the requested data encoding (e.g., "json_ietf", "proto").
	Encoding string `json:"encoding,omitempty"`
	// DataType filters the type of data: "all", "config", "state", "operational".
	DataType string `json:"data_type,omitempty"`
}

// GNMIGetResult contains the result of a Get RPC.
type GNMIGetResult struct {
	// Notifications is the list of notifications returned.
	Notifications []GNMINotification `json:"notifications"`
}

// GNMINotification represents a gNMI notification message.
type GNMINotification struct {
	// Timestamp is the notification timestamp in nanoseconds.
	Timestamp int64 `json:"timestamp"`
	// Prefix is the common path prefix for updates.
	Prefix GNMIPath `json:"prefix,omitempty"`
	// Updates contains the updated values.
	Updates []GNMIUpdate `json:"updates,omitempty"`
	// Deletes contains deleted paths.
	Deletes []GNMIPath `json:"deletes,omitempty"`
}

// GNMIUpdate represents a single path-value update.
type GNMIUpdate struct {
	// Path is the data path.
	Path GNMIPath `json:"path"`
	// Value is the JSON-encoded value.
	Value []byte `json:"value"`
}

// GNMISetRequest specifies a Set RPC request.
type GNMISetRequest struct {
	// Delete is the list of paths to delete.
	Delete []GNMIPath `json:"delete,omitempty"`
	// Replace is the list of path-value pairs to replace.
	Replace []GNMIUpdate `json:"replace,omitempty"`
	// Update is the list of path-value pairs to update (merge).
	Update []GNMIUpdate `json:"update,omitempty"`
}

// GNMISetResult contains the result of a Set RPC.
type GNMISetResult struct {
	// Timestamp is the server timestamp of the set operation.
	Timestamp int64 `json:"timestamp"`
	// Results contains per-operation results.
	Results []GNMIUpdateResult `json:"results"`
}

// GNMIUpdateResult contains the result of a single set operation.
type GNMIUpdateResult struct {
	// Path is the affected path.
	Path GNMIPath `json:"path"`
	// Op is the operation type: "delete", "replace", "update".
	Op string `json:"op"`
}

// GNMISubscribeRequest specifies a Subscribe RPC request.
type GNMISubscribeRequest struct {
	// Paths is the list of paths to subscribe to.
	Paths []GNMIPath `json:"paths"`
	// Mode is the subscription mode: "stream", "once", "poll".
	Mode string `json:"mode"`
	// StreamMode is the streaming mode: "target_defined", "on_change", "sample".
	StreamMode string `json:"stream_mode,omitempty"`
	// SampleInterval is the sampling interval in nanoseconds (for sample mode).
	SampleInterval int64 `json:"sample_interval,omitempty"`
	// Encoding is the data encoding (e.g., "json_ietf", "proto").
	Encoding string `json:"encoding,omitempty"`
}

// GNMISubscription represents an active gNMI subscription.
type GNMISubscription struct {
	notifications chan GNMINotification
	errors        chan error
	syncReceived  chan struct{}
	cancel        context.CancelFunc
	done          chan struct{}
}

// NewGNMISubscription creates a new GNMISubscription.
func NewGNMISubscription(cancel context.CancelFunc) *GNMISubscription {
	return &GNMISubscription{
		notifications: make(chan GNMINotification, 100),
		errors:        make(chan error, 10),
		syncReceived:  make(chan struct{}),
		cancel:        cancel,
		done:          make(chan struct{}),
	}
}

// Notifications returns the channel for receiving notifications.
func (s *GNMISubscription) Notifications() <-chan GNMINotification {
	return s.notifications
}

// Errors returns the channel for receiving errors.
func (s *GNMISubscription) Errors() <-chan error {
	return s.errors
}

// SyncComplete returns a channel that is closed when the initial sync is done.
func (s *GNMISubscription) SyncComplete() <-chan struct{} {
	return s.syncReceived
}

// Done returns a channel that is closed when the subscription ends.
func (s *GNMISubscription) Done() <-chan struct{} {
	return s.done
}

// Close cancels the subscription and releases resources.
func (s *GNMISubscription) Close() {
	s.cancel()
}

// SendNotification sends a notification to the subscription channel (non-blocking).
func (s *GNMISubscription) SendNotification(n GNMINotification) {
	select {
	case s.notifications <- n:
	default:
	}
}

// SendError sends an error to the subscription channel (non-blocking).
func (s *GNMISubscription) SendError(err error) {
	select {
	case s.errors <- err:
	default:
	}
}

// CloseSyncComplete signals that the initial sync is complete.
func (s *GNMISubscription) CloseSyncComplete() {
	close(s.syncReceived)
}

// CloseDone signals that the subscription has ended.
func (s *GNMISubscription) CloseDone() {
	close(s.done)
}

// GNMIRebootMethod specifies the reboot method for gNOI.
type GNMIRebootMethod string

const (
	// GNMIRebootCold performs a cold reboot.
	GNMIRebootCold GNMIRebootMethod = "cold"
	// GNMIRebootWarm performs a warm reboot.
	GNMIRebootWarm GNMIRebootMethod = "warm"
	// GNMIRebootPowerUp performs a power-cycle reboot.
	GNMIRebootPowerUp GNMIRebootMethod = "powerup"
)

// GNMIPingResponse contains a single ping response.
type GNMIPingResponse struct {
	Source  string `json:"source"`
	Time    int64  `json:"time_ns"`
	Bytes   int32  `json:"bytes"`
	TTL     int32  `json:"ttl"`
	Sequence int32 `json:"sequence"`
}

// GNMITracerouteResponse contains a single traceroute hop.
type GNMITracerouteResponse struct {
	Hop     int32  `json:"hop"`
	Address string `json:"address"`
	RTT     int64  `json:"rtt_ns"`
}

// RestconfAdapterFactory creates RESTCONF adapters.
type RestconfAdapterFactory func(config *ConnectionConfig) (RestconfAdapter, error)

// RestconfAdapter extends ProtocolAdapter with RESTCONF-specific operations (RFC 8040).
type RestconfAdapter interface {
	ProtocolAdapter

	// GetData retrieves YANG data from the specified path.
	GetData(ctx context.Context, path string, opts *RestconfQueryOptions) ([]byte, error)

	// PostData creates a new data resource at the specified path.
	PostData(ctx context.Context, path string, data []byte) error

	// PutData creates or replaces a data resource at the specified path.
	PutData(ctx context.Context, path string, data []byte) error

	// PatchData merges data into an existing resource at the specified path.
	PatchData(ctx context.Context, path string, data []byte) error

	// DeleteData removes a data resource at the specified path.
	DeleteData(ctx context.Context, path string) error

	// InvokeOperation calls a YANG RPC or action.
	InvokeOperation(ctx context.Context, operation string, input []byte) ([]byte, error)

	// YANGLibraryVersion returns the YANG library version supported by the server.
	YANGLibraryVersion(ctx context.Context) (string, error)

	// ServerModules returns the YANG modules reported by the server.
	ServerModules(ctx context.Context) ([]RestconfModule, error)

	// RootPath returns the RESTCONF API root path.
	RootPath() string
}

// RestconfQueryOptions specifies query parameters for RESTCONF GET requests.
type RestconfQueryOptions struct {
	// Depth limits the depth of returned data (1-65535, 0 for unbounded).
	Depth int `json:"depth,omitempty"`

	// Fields selects specific fields to return.
	Fields string `json:"fields,omitempty"`

	// Content filters by data type: "all", "config", "nonconfig".
	Content string `json:"content,omitempty"`

	// WithDefaults controls default value reporting: "report-all", "trim", "explicit", "report-all-tagged".
	WithDefaults string `json:"with_defaults,omitempty"`

	// Filter is an XPath or subtree filter expression.
	Filter string `json:"filter,omitempty"`
}

// RestconfModule describes a YANG module reported by a RESTCONF server.
type RestconfModule struct {
	// Name is the module name.
	Name string `json:"name"`

	// Revision is the module revision date.
	Revision string `json:"revision"`

	// Namespace is the module XML namespace.
	Namespace string `json:"namespace"`
}
