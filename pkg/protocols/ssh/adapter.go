// Package ssh provides an SSH protocol adapter for proxy agents.
package ssh

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/shawnbutts/keystone-core/pkg/credentials"
	"github.com/shawnbutts/keystone-core/pkg/protocols"
	"github.com/shawnbutts/keystone-core/pkg/proxy"
)

// Config contains SSH adapter configuration.
type Config struct {
	// ConnectionConfig contains common connection settings.
	*protocols.ConnectionConfig

	// Port is the SSH port (default 22).
	Port int `json:"port,omitempty"`

	// HostKeyCallback is the host key verification callback.
	// If nil, InsecureIgnoreHostKey is used (not recommended for production).
	HostKeyCallback ssh.HostKeyCallback `json:"-"`

	// BannerCallback is called when a banner is received.
	BannerCallback ssh.BannerCallback `json:"-"`

	// ClientVersion is the SSH client version string.
	ClientVersion string `json:"client_version,omitempty"`

	// Ciphers is the list of allowed ciphers.
	Ciphers []string `json:"ciphers,omitempty"`

	// KeyExchanges is the list of allowed key exchange algorithms.
	KeyExchanges []string `json:"key_exchanges,omitempty"`

	// MACs is the list of allowed MAC algorithms.
	MACs []string `json:"macs,omitempty"`

	// HostKeyAlgorithms is the list of allowed host key algorithms.
	HostKeyAlgorithms []string `json:"host_key_algorithms,omitempty"`
}

// DefaultConfig returns a default SSH configuration.
func DefaultConfig() *Config {
	return &Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		Port:             22,
		HostKeyCallback:  ssh.InsecureIgnoreHostKey(), // TODO: Implement proper host key verification
		ClientVersion:    "SSH-2.0-KeystoneCore",
	}
}

// Adapter implements the SSH protocol adapter.
type Adapter struct {
	config     *Config
	client     *ssh.Client
	device     *proxy.ProxiedDevice
	credential credentials.Credential
	mu         sync.RWMutex

	// Connection state
	connected   bool
	lastError   error
	lastConnect time.Time
	lastPing    time.Time

	// Metrics
	metrics *protocols.AdapterMetrics
}

// NewAdapter creates a new SSH adapter.
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
	return protocols.ProtocolSSH
}

// Connect establishes an SSH connection to the device.
func (a *Adapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Close existing connection if any
	if a.client != nil {
		a.client.Close()
		a.client = nil
		a.connected = false
	}

	a.device = device
	a.credential = cred

	// Build SSH client config
	sshConfig, err := a.buildSSHConfig(cred)
	if err != nil {
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("failed to build SSH config: %w", err)
	}

	// Determine host and port
	host := device.Address
	port := a.config.Port
	if device.Port > 0 {
		port = device.Port
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	// Connect with context timeout
	var client *ssh.Client
	var dialErr error

	if a.config.ProxyHost != "" {
		// Connect through proxy/bastion
		client, dialErr = a.connectViaProxy(ctx, addr, sshConfig)
	} else {
		// Direct connection
		client, dialErr = a.connectDirect(ctx, addr, sshConfig)
	}

	if dialErr != nil {
		a.lastError = dialErr
		a.metrics.ConnectionErrors++
		return fmt.Errorf("SSH connection failed: %w", dialErr)
	}

	a.client = client
	a.connected = true
	a.lastConnect = time.Now()
	a.lastError = nil
	a.metrics.ConnectionCount++
	a.metrics.LastActivity = time.Now()

	return nil
}

// buildSSHConfig builds an SSH client config from credentials.
func (a *Adapter) buildSSHConfig(cred credentials.Credential) (*ssh.ClientConfig, error) {
	config := &ssh.ClientConfig{
		HostKeyCallback: a.config.HostKeyCallback,
		BannerCallback:  a.config.BannerCallback,
		Timeout:         a.config.Timeout,
	}

	if a.config.ClientVersion != "" {
		config.ClientVersion = a.config.ClientVersion
	}
	if len(a.config.Ciphers) > 0 {
		config.Ciphers = a.config.Ciphers
	}
	if len(a.config.KeyExchanges) > 0 {
		config.KeyExchanges = a.config.KeyExchanges
	}
	if len(a.config.MACs) > 0 {
		config.MACs = a.config.MACs
	}
	if len(a.config.HostKeyAlgorithms) > 0 {
		config.HostKeyAlgorithms = a.config.HostKeyAlgorithms
	}

	// Build auth methods based on credential type
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
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}

		config.Auth = []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		}

	default:
		return nil, fmt.Errorf("unsupported credential type for SSH: %T", cred)
	}

	return config, nil
}

// connectDirect establishes a direct SSH connection.
func (a *Adapter) connectDirect(ctx context.Context, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	// Create dialer with timeout
	dialer := &net.Dialer{
		Timeout: a.config.Timeout,
	}

	// Dial with context
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	// Establish SSH connection
	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return ssh.NewClient(clientConn, chans, reqs), nil
}

// connectViaProxy establishes an SSH connection through a proxy/bastion host.
func (a *Adapter) connectViaProxy(ctx context.Context, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	// Build proxy SSH config
	proxyCred := a.config.ProxyCredential
	if proxyCred == nil {
		// Use the same credential for proxy if not specified
		proxyCred = a.credential
	}

	proxyConfig, err := a.buildSSHConfig(proxyCred)
	if err != nil {
		return nil, fmt.Errorf("failed to build proxy SSH config: %w", err)
	}

	proxyPort := a.config.ProxyPort
	if proxyPort == 0 {
		proxyPort = 22
	}
	proxyAddr := fmt.Sprintf("%s:%d", a.config.ProxyHost, proxyPort)

	// Connect to proxy
	proxyConn, err := a.connectDirect(ctx, proxyAddr, proxyConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to proxy: %w", err)
	}

	// Open a channel to the target through the proxy
	targetConn, err := proxyConn.Dial("tcp", addr)
	if err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("failed to dial target through proxy: %w", err)
	}

	// Establish SSH connection to target
	clientConn, chans, reqs, err := ssh.NewClientConn(targetConn, addr, config)
	if err != nil {
		targetConn.Close()
		proxyConn.Close()
		return nil, err
	}

	return ssh.NewClient(clientConn, chans, reqs), nil
}

// Execute executes a command on the device.
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

	// Create session
	session, err := client.NewSession()
	if err != nil {
		result.Error = fmt.Sprintf("failed to create session: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		a.metrics.ExecutionErrors++
		return result, err
	}
	defer session.Close()

	// Configure PTY if requested
	if req.PTY {
		ptyConfig := req.PTYConfig
		if ptyConfig == nil {
			ptyConfig = protocols.DefaultPTYConfig()
		}
		modes := ssh.TerminalModes{
			ssh.ECHO:          0,     // disable echoing
			ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
			ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
		}
		if err := session.RequestPty(ptyConfig.Term, int(ptyConfig.Rows), int(ptyConfig.Cols), modes); err != nil {
			result.Error = fmt.Sprintf("failed to request PTY: %v", err)
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			return result, err
		}
	}

	// Set environment variables
	for k, v := range req.Environment {
		if err := session.Setenv(k, v); err != nil {
			// Some servers don't allow env vars - log but continue
		}
	}

	// Set stdin
	if req.Stdin != nil {
		session.Stdin = req.Stdin
	}

	// Build command
	command := req.Command
	if len(req.Args) > 0 {
		for _, arg := range req.Args {
			command += " " + arg
		}
	}

	// Set working directory if specified
	if req.WorkingDir != "" {
		command = fmt.Sprintf("cd %s && %s", req.WorkingDir, command)
	}

	// Execute with timeout
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create channels for output
	outputDone := make(chan struct{})
	var stdout []byte

	if req.StreamOutput && (req.StdoutWriter != nil || req.StderrWriter != nil) {
		// Streaming mode
		if req.StdoutWriter != nil {
			session.Stdout = req.StdoutWriter
		}
		if req.StderrWriter != nil {
			session.Stderr = req.StderrWriter
		}
	} else {
		// Buffered mode - capture output
		go func() {
			stdout, _ = session.Output(command)
			close(outputDone)
		}()
	}

	// Wait for completion or timeout
	var execErr error
	if req.StreamOutput {
		execErr = session.Run(command)
	} else {
		select {
		case <-outputDone:
			// Command completed
		case <-execCtx.Done():
			// Timeout - try to signal the process
			_ = session.Signal(ssh.SIGKILL)
			result.Timeout = true
			execErr = execCtx.Err()
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	result.Stdout = stdout

	// Handle execution error
	if execErr != nil {
		if exitErr, ok := execErr.(*ssh.ExitError); ok {
			result.ExitCode = exitErr.ExitStatus()
		} else {
			result.Error = execErr.Error()
		}
	}

	// Update metrics
	a.mu.Lock()
	a.metrics.ExecutionCount++
	if result.Error != "" || result.ExitCode != 0 {
		a.metrics.ExecutionErrors++
	}
	a.metrics.LastActivity = time.Now()
	a.mu.Unlock()

	return result, nil
}

// Disconnect closes the SSH connection.
func (a *Adapter) Disconnect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil {
		err := a.client.Close()
		a.client = nil
		a.connected = false
		return err
	}
	return nil
}

// HealthCheck performs a health check on the SSH connection.
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

	// Try to create a session as health check
	start := time.Now()
	session, err := client.NewSession()
	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("session creation failed: %v", err)
		result.Latency = time.Since(start)
		return result, nil
	}
	session.Close()

	result.Healthy = true
	result.Status = "connected"
	result.Latency = time.Since(start)
	result.Details["last_connect"] = a.lastConnect
	result.Details["last_ping"] = a.lastPing

	a.mu.Lock()
	a.lastPing = time.Now()
	a.mu.Unlock()

	return result, nil
}

// IsConnected returns true if the SSH connection is active.
func (a *Adapter) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected && a.client != nil
}

// Client returns the underlying SSH client.
func (a *Adapter) Client() *ssh.Client {
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

// NewAdapterFactory creates an adapter factory for SSH.
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

// init registers the SSH adapter with the default registry.
func init() {
	protocols.Register(protocols.ProtocolSSH, NewAdapterFactory(nil))
}
