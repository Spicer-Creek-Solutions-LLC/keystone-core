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
