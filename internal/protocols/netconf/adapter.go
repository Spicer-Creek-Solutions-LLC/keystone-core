package netconf

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

func init() {
	protocols.Register(protocols.ProtocolNETCONF, NewAdapterFactory(nil))
	protocols.RegisterNetconf(protocols.ProtocolNETCONF, NewNetconfAdapterFactory(nil))
}

// Config contains NETCONF adapter configuration.
type Config struct {
	*protocols.ConnectionConfig

	// Port is the NETCONF port (default 830).
	Port int `json:"port,omitempty"`

	// Capabilities is the client capability set to advertise.
	Capabilities []Capability `json:"capabilities,omitempty"`

	// HostKeyCallback verifies the server's host key.
	// If nil, ssh.InsecureIgnoreHostKey is used.
	HostKeyCallback ssh.HostKeyCallback `json:"-"`
}

// DefaultConfig returns a default NETCONF configuration.
func DefaultConfig() *Config {
	return &Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		Port:             DefaultPort,
	}
}

// Adapter implements the NETCONF protocol adapter.
type Adapter struct {
	config     *Config
	sshClient  *ssh.Client
	session    *Session
	device     *proxy.ProxiedDevice
	credential credentials.Credential
	mu         sync.RWMutex

	connected   bool
	lastError   error
	lastConnect time.Time
	lastPing    time.Time

	metrics *protocols.AdapterMetrics
}

// NewAdapter creates a new NETCONF adapter.
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
	return protocols.ProtocolNETCONF
}

// Connect establishes a NETCONF session over SSH.
func (a *Adapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.sshClient != nil {
		a.closeInternal()
	}

	a.device = device
	a.credential = cred

	sshConfig, err := a.buildSSHConfig(cred)
	if err != nil {
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("build SSH config: %w", err)
	}

	host := device.Address
	port := a.config.Port
	if device.Port > 0 {
		port = device.Port
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	client, err := a.connectDirect(ctx, addr, sshConfig)
	if err != nil {
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	transport, err := NewTransport(client)
	if err != nil {
		client.Close()
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("NETCONF transport: %w", err)
	}

	session, err := NewSession(transport)
	if err != nil {
		client.Close()
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("NETCONF session: %w", err)
	}

	a.sshClient = client
	a.session = session
	a.connected = true
	a.lastConnect = time.Now()
	a.lastError = nil
	a.metrics.ConnectionCount++
	a.metrics.LastActivity = time.Now()

	return nil
}

// Execute executes a NETCONF command string and returns the result.
func (a *Adapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected || a.session == nil {
		return nil, fmt.Errorf("not connected")
	}

	start := time.Now()
	result := &protocols.ExecuteResult{StartTime: start}

	data, err := a.executeCommand(ctx, req.Command)
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(start)

	a.metrics.ExecutionCount++
	a.metrics.LastActivity = time.Now()

	if err != nil {
		a.metrics.ExecutionErrors++
		result.ExitCode = 1
		result.Error = err.Error()
		return result, nil
	}

	result.Stdout = data
	a.metrics.BytesReceived += int64(len(data))
	return result, nil
}

// Disconnect closes the NETCONF session and SSH connection.
func (a *Adapter) Disconnect(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closeInternal()
}

func (a *Adapter) closeInternal() error {
	var lastErr error
	if a.session != nil {
		if err := a.session.Close(); err != nil {
			lastErr = err
		}
		a.session = nil
	}
	if a.sshClient != nil {
		if err := a.sshClient.Close(); err != nil && lastErr == nil {
			lastErr = err
		}
		a.sshClient = nil
	}
	a.connected = false
	return lastErr
}

// HealthCheck verifies the NETCONF session is operational.
func (a *Adapter) HealthCheck(_ context.Context) (*protocols.HealthCheckResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := &protocols.HealthCheckResult{
		LastCheck: time.Now(),
		Details:   make(map[string]interface{}),
	}

	if !a.connected || a.session == nil {
		result.Status = "disconnected"
		return result, nil
	}

	start := time.Now()
	reply, err := a.session.SendRPC(&rpcGet{})
	result.Latency = time.Since(start)

	if err != nil {
		result.Status = fmt.Sprintf("rpc failed: %s", err)
		result.ConsecutiveFailures++
		return result, nil
	}

	if reply.Errors.HasError() {
		// An RPC error on a basic <get> doesn't necessarily mean unhealthy;
		// the session is still functional.
		result.Healthy = true
		result.Status = "connected (get returned errors)"
	} else {
		result.Healthy = true
		result.Status = "connected"
	}

	result.Details["session_id"] = a.session.SessionID()
	a.metrics.LastActivity = time.Now()

	return result, nil
}

// IsConnected returns true if the adapter is connected.
func (a *Adapter) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected
}

// Metrics returns the adapter metrics.
func (a *Adapter) Metrics() *protocols.AdapterMetrics {
	return a.metrics
}

func (a *Adapter) buildSSHConfig(cred credentials.Credential) (*ssh.ClientConfig, error) {
	hostKeyCallback := a.config.HostKeyCallback
	if hostKeyCallback == nil {
		hostKeyCallback = ssh.InsecureIgnoreHostKey() //nolint:gosec
	}

	config := &ssh.ClientConfig{
		HostKeyCallback: hostKeyCallback,
		Timeout:         a.config.Timeout,
	}

	switch c := cred.(type) {
	case *credentials.SSHPasswordCredential:
		config.User = c.Username
		config.Auth = []ssh.AuthMethod{
			ssh.Password(c.Password),
		}
	case *credentials.SSHKeyCredential:
		config.User = c.Username
		var signer ssh.Signer
		var err error
		if c.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(c.PrivateKey, []byte(c.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(c.PrivateKey)
		}
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		config.Auth = []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		}
	default:
		return nil, fmt.Errorf("unsupported credential type for NETCONF: %T", cred)
	}

	return config, nil
}

func (a *Adapter) connectDirect(ctx context.Context, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	dialer := &net.Dialer{Timeout: a.config.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return ssh.NewClient(clientConn, chans, reqs), nil
}

// NewAdapterFactory creates a factory for NETCONF ProtocolAdapters.
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

// NewNetconfAdapterFactory creates a factory for NetconfAdapters.
func NewNetconfAdapterFactory(config *Config) protocols.NetconfAdapterFactory {
	return func(connConfig *protocols.ConnectionConfig) (protocols.NetconfAdapter, error) {
		cfg := config
		if cfg == nil {
			cfg = DefaultConfig()
		}
		cfg.ConnectionConfig = connConfig
		return NewAdapter(cfg), nil
	}
}

// Interface compliance checks.
var (
	_ protocols.ProtocolAdapter = (*Adapter)(nil)
	_ protocols.NetconfAdapter  = (*Adapter)(nil)
)
