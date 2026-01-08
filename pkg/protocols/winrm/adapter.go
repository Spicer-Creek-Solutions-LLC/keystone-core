// Package winrm provides a WinRM protocol adapter for proxy agents.
package winrm

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/masterzen/winrm"

	"github.com/shawnbutts/keystone-core/pkg/credentials"
	"github.com/shawnbutts/keystone-core/pkg/protocols"
	"github.com/shawnbutts/keystone-core/pkg/proxy"
)

// Config contains WinRM adapter configuration.
type Config struct {
	// ConnectionConfig contains common connection settings.
	*protocols.ConnectionConfig

	// Port is the WinRM port (default 5985 for HTTP, 5986 for HTTPS).
	Port int `json:"port,omitempty"`

	// HTTPS enables HTTPS transport.
	HTTPS bool `json:"https,omitempty"`

	// Insecure disables TLS certificate verification.
	Insecure bool `json:"insecure,omitempty"`

	// CACert is the CA certificate for TLS verification.
	CACert []byte `json:"ca_cert,omitempty"`

	// CertPEM is the client certificate for mTLS.
	CertPEM []byte `json:"cert_pem,omitempty"`

	// KeyPEM is the client private key for mTLS.
	KeyPEM []byte `json:"key_pem,omitempty"`

	// UseNTLM enables NTLM authentication.
	UseNTLM bool `json:"use_ntlm,omitempty"`

	// UseKerberos enables Kerberos authentication.
	UseKerberos bool `json:"use_kerberos,omitempty"`

	// OperationTimeout is the timeout for WinRM operations.
	OperationTimeout time.Duration `json:"operation_timeout,omitempty"`

	// DefaultShell is the default shell to use (powershell or cmd).
	DefaultShell string `json:"default_shell,omitempty"`
}

// DefaultConfig returns a default WinRM configuration.
func DefaultConfig() *Config {
	return &Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		Port:             5985,
		HTTPS:            false,
		Insecure:         false,
		UseNTLM:          true,
		OperationTimeout: 60 * time.Second,
		DefaultShell:     "powershell",
	}
}

// Adapter implements the WinRM protocol adapter.
type Adapter struct {
	config     *Config
	client     *winrm.Client
	device     *proxy.ProxiedDevice
	credential credentials.Credential
	endpoint   *winrm.Endpoint
	mu         sync.RWMutex

	// Connection state
	connected   bool
	lastError   error
	lastConnect time.Time

	// Metrics
	metrics *protocols.AdapterMetrics
}

// NewAdapter creates a new WinRM adapter.
func NewAdapter(config *Config) *Adapter {
	if config == nil {
		config = DefaultConfig()
	}
	if config.ConnectionConfig == nil {
		config.ConnectionConfig = protocols.DefaultConnectionConfig()
	}
	return &Adapter{
		config:  config,
		metrics: &protocols.AdapterMetrics{},
	}
}

// Type returns the protocol type.
func (a *Adapter) Type() protocols.ProtocolType {
	return protocols.ProtocolWinRM
}

// Connect establishes a WinRM connection to the device.
func (a *Adapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Close existing connection
	if a.client != nil {
		a.client = nil
		a.connected = false
	}

	a.device = device
	a.credential = cred

	// Validate credential type
	winrmCred, ok := cred.(*credentials.WinRMCredential)
	if !ok {
		return fmt.Errorf("WinRM requires WinRMCredential, got %T", cred)
	}

	// Determine port
	port := a.config.Port
	if device.Port > 0 {
		port = device.Port
	}
	if port == 0 {
		if a.config.HTTPS {
			port = 5986
		} else {
			port = 5985
		}
	}

	// Create endpoint
	endpoint := winrm.NewEndpoint(
		device.Address,
		port,
		a.config.HTTPS,
		a.config.Insecure,
		a.config.CACert,
		a.config.CertPEM,
		a.config.KeyPEM,
		a.config.OperationTimeout,
	)

	a.endpoint = endpoint

	// Create parameters
	params := winrm.NewParameters(
		fmt.Sprintf("PT%dS", int(a.config.OperationTimeout.Seconds())),
		"en-US",
		153600,
	)

	// Set NTLM transport decorator if using NTLM
	if a.config.UseNTLM {
		params.TransportDecorator = func() winrm.Transporter {
			return &winrm.ClientNTLM{}
		}
	} else if a.config.UseKerberos && winrmCred.Domain != "" {
		// Kerberos - note: requires proper Kerberos setup on the system
		// This is a simplified example - full Kerberos support would need
		// additional libraries like gokrb5
		return fmt.Errorf("Kerberos authentication not yet implemented")
	}

	// Create client
	client, err := winrm.NewClientWithParameters(
		endpoint,
		winrmCred.Username,
		winrmCred.Password,
		params,
	)
	if err != nil {
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("failed to create WinRM client: %w", err)
	}

	// Verify connectivity with a simple command
	stdout, stderr, exitCode, err := client.RunCmdWithContext(ctx, "echo connected")
	if err != nil {
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("WinRM connectivity check failed: %w", err)
	}

	if exitCode != 0 {
		a.lastError = fmt.Errorf("connectivity check failed with exit code %d: %s", exitCode, stderr)
		a.metrics.ConnectionErrors++
		return a.lastError
	}

	_ = stdout // Ignore output

	a.client = client
	a.connected = true
	a.lastConnect = time.Now()
	a.lastError = nil
	a.metrics.ConnectionCount++
	a.metrics.LastActivity = time.Now()

	return nil
}

// Execute implements the ProtocolAdapter interface.
func (a *Adapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	result := &protocols.ExecuteResult{
		StartTime: time.Now(),
	}

	// Determine shell and command
	command := req.Command
	if len(req.Args) > 0 {
		command = command + " " + strings.Join(req.Args, " ")
	}

	// Wrap in appropriate shell if needed
	shell := a.config.DefaultShell
	if req.PTY {
		// Use PowerShell for PTY-like behavior
		shell = "powershell"
	}

	var stdout, stderr string
	var exitCode int
	var err error

	// Setup stdin if provided
	var stdin io.Reader
	if req.Stdin != nil {
		stdin = req.Stdin
	}

	// Execute based on shell type
	switch strings.ToLower(shell) {
	case "powershell", "ps":
		stdout, stderr, exitCode, err = a.runPowerShell(ctx, client, command, stdin)
	case "cmd":
		stdout, stderr, exitCode, err = a.runCmd(ctx, client, command, stdin)
	default:
		// Default to PowerShell
		stdout, stderr, exitCode, err = a.runPowerShell(ctx, client, command, stdin)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.ExitCode = exitCode
	result.Stdout = []byte(stdout)
	result.Stderr = []byte(stderr)

	if err != nil {
		result.Error = err.Error()
		a.metrics.ExecutionErrors++
	}

	// Update metrics
	a.mu.Lock()
	a.metrics.ExecutionCount++
	if exitCode != 0 {
		a.metrics.ExecutionErrors++
	}
	a.metrics.LastActivity = time.Now()
	a.mu.Unlock()

	return result, nil
}

// runPowerShell executes a PowerShell command.
func (a *Adapter) runPowerShell(ctx context.Context, client *winrm.Client, command string, stdin io.Reader) (string, string, int, error) {
	// Use RunPSWithContext which handles PowerShell encoding
	if stdin != nil {
		// Read stdin to string
		stdinData, err := io.ReadAll(stdin)
		if err != nil {
			return "", "", -1, fmt.Errorf("failed to read stdin: %w", err)
		}
		return client.RunPSWithContextWithString(ctx, command, string(stdinData))
	}
	return client.RunPSWithContext(ctx, command)
}

// runCmd executes a CMD command.
func (a *Adapter) runCmd(ctx context.Context, client *winrm.Client, command string, stdin io.Reader) (string, string, int, error) {
	cmdCommand := fmt.Sprintf("cmd.exe /c %s", command)
	if stdin != nil {
		// Read stdin to string
		stdinData, err := io.ReadAll(stdin)
		if err != nil {
			return "", "", -1, fmt.Errorf("failed to read stdin: %w", err)
		}
		return client.RunWithContextWithString(ctx, cmdCommand, string(stdinData))
	}
	return client.RunCmdWithContext(ctx, cmdCommand)
}

// encodePowerShellCommand encodes a command for PowerShell -EncodedCommand.
func encodePowerShellCommand(command string) string {
	// Convert to UTF-16LE
	utf16 := utf16LEEncode(command)
	// Base64 encode
	return base64.StdEncoding.EncodeToString(utf16)
}

// utf16LEEncode converts a string to UTF-16LE bytes.
func utf16LEEncode(s string) []byte {
	var buf bytes.Buffer
	for _, r := range s {
		if r > 0xFFFF {
			// Handle surrogate pairs for characters outside BMP
			r1, r2 := utf16EncodePair(r)
			buf.WriteByte(byte(r1))
			buf.WriteByte(byte(r1 >> 8))
			buf.WriteByte(byte(r2))
			buf.WriteByte(byte(r2 >> 8))
		} else {
			buf.WriteByte(byte(r))
			buf.WriteByte(byte(r >> 8))
		}
	}
	return buf.Bytes()
}

// utf16EncodePair encodes a Unicode code point as a UTF-16 surrogate pair.
func utf16EncodePair(r rune) (r1, r2 rune) {
	r -= 0x10000
	r1 = 0xD800 + (r>>10)&0x3FF
	r2 = 0xDC00 + r&0x3FF
	return
}

// RunPowerShell executes a PowerShell command and returns the result.
func (a *Adapter) RunPowerShell(ctx context.Context, command string) (string, string, int, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return "", "", -1, fmt.Errorf("not connected")
	}

	return a.runPowerShell(ctx, client, command, nil)
}

// RunCmd executes a CMD command and returns the result.
func (a *Adapter) RunCmd(ctx context.Context, command string) (string, string, int, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return "", "", -1, fmt.Errorf("not connected")
	}

	return a.runCmd(ctx, client, command, nil)
}

// Disconnect closes the WinRM connection.
func (a *Adapter) Disconnect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.client = nil
	a.connected = false
	return nil
}

// HealthCheck performs a health check on the WinRM connection.
func (a *Adapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	result := &protocols.HealthCheckResult{
		LastCheck: time.Now(),
		Details:   make(map[string]interface{}),
	}

	if !connected || client == nil {
		result.Healthy = false
		result.Status = "not connected"
		return result, nil
	}

	// Try a simple command
	start := time.Now()
	_, _, exitCode, err := client.RunCmdWithContext(ctx, "echo ok")
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed: %v", err)
		return result, nil
	}

	if exitCode != 0 {
		result.Healthy = false
		result.Status = fmt.Sprintf("health check failed with exit code %d", exitCode)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["last_connect"] = a.lastConnect
	result.Details["endpoint"] = fmt.Sprintf("%s:%d", a.endpoint.Host, a.endpoint.Port)

	return result, nil
}

// IsConnected returns true if the WinRM connection is active.
func (a *Adapter) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected && a.client != nil
}

// Client returns the underlying WinRM client.
func (a *Adapter) Client() *winrm.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.client
}

// Metrics returns the adapter metrics.
func (a *Adapter) Metrics() *protocols.AdapterMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.metrics
}

// NewAdapterFactory creates an adapter factory for WinRM.
func NewAdapterFactory(config *Config) protocols.AdapterFactory {
	return func(connConfig *protocols.ConnectionConfig) (protocols.ProtocolAdapter, error) {
		cfg := config
		if cfg == nil {
			cfg = DefaultConfig()
		}
		cfg.ConnectionConfig = connConfig
		return NewAdapter(cfg), nil
	}
}

// init registers the WinRM adapter with the default registry.
func init() {
	protocols.Register(protocols.ProtocolWinRM, NewAdapterFactory(nil))
}
