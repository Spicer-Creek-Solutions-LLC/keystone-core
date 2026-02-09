package telnet

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

// Compile-time interface compliance check.
var _ protocols.ProtocolAdapter = (*Adapter)(nil)

// Config contains Telnet adapter configuration.
type Config struct {
	*protocols.ConnectionConfig

	// Port is the Telnet port (default 23).
	Port int `json:"port,omitempty"`

	// Prompt is the expected shell prompt (default "# ").
	Prompt string `json:"prompt,omitempty"`

	// TermType is the terminal type to negotiate (default "vt100").
	TermType string `json:"term_type,omitempty"`

	// Rows is the terminal height (default 24).
	Rows uint16 `json:"rows,omitempty"`

	// Cols is the terminal width (default 80).
	Cols uint16 `json:"cols,omitempty"`

	// LoginPrompts are patterns that identify the login prompt.
	LoginPrompts []string `json:"login_prompts,omitempty"`

	// PasswordPrompts are patterns that identify the password prompt.
	PasswordPrompts []string `json:"password_prompts,omitempty"`

	// Security contains Telnet-specific security controls.
	Security *SecurityConfig `json:"security,omitempty"`
}

// DefaultConfig returns a default Telnet configuration.
func DefaultConfig() *Config {
	return &Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		Port:             23,
		Prompt:           "# ",
		TermType:         "vt100",
		Rows:             24,
		Cols:             80,
		LoginPrompts:     []string{"ogin:", "sername:"},
		PasswordPrompts:  []string{"assword:"},
		Security:         &SecurityConfig{DeprecationWarning: true},
	}
}

// Adapter implements the Telnet protocol adapter.
type Adapter struct {
	config       *Config
	conn         net.Conn
	session      *Session
	device       *proxy.ProxiedDevice
	credential   credentials.Credential
	security     *SecurityEnforcer
	logger       *slog.Logger
	mu           sync.RWMutex
	connected    bool
	sessionStart time.Time
	lastError    error
	lastConnect  time.Time
	metrics      *protocols.AdapterMetrics
}

// NewAdapter creates a new Telnet adapter.
func NewAdapter(config *Config, logger *slog.Logger) *Adapter {
	if config == nil {
		config = DefaultConfig()
	}
	if config.ConnectionConfig == nil {
		config.ConnectionConfig = protocols.DefaultConnectionConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Adapter{
		config:  config,
		logger:  logger,
		metrics: &protocols.AdapterMetrics{},
	}
}

// Type returns the protocol type.
func (a *Adapter) Type() protocols.ProtocolType {
	return protocols.ProtocolTelnet
}

// Connect establishes a Telnet connection to the device.
func (a *Adapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Close existing connection
	if a.conn != nil {
		a.closeInternal()
	}

	a.device = device
	a.credential = cred

	// Validate credential type
	passCred, ok := cred.(*credentials.SSHPasswordCredential)
	if !ok {
		a.metrics.ConnectionErrors++
		return fmt.Errorf("telnet adapter requires SSHPasswordCredential, got %T", cred)
	}

	// Initialize security enforcer
	enforcer, err := NewSecurityEnforcer(a.config.Security, a.logger)
	if err != nil {
		a.metrics.ConnectionErrors++
		return fmt.Errorf("security enforcer init: %w", err)
	}
	a.security = enforcer

	// Determine address
	host := device.Address
	port := a.config.Port
	if device.Port > 0 {
		port = device.Port
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	// Check IP allowlist
	if err := a.security.CheckConnection(addr); err != nil {
		a.metrics.ConnectionErrors++
		return err
	}

	// Emit deprecation warning
	a.security.EmitDeprecationWarning(device.ID, addr)

	// Dial TCP
	dialer := &net.Dialer{Timeout: a.config.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("telnet dial failed: %w", err)
	}

	// Create negotiator and session
	negotiator := NewNegotiator(a.config.TermType, a.config.Rows, a.config.Cols)
	session := NewSession(conn, negotiator, a.config.Prompt, a.config.Timeout)

	// Authenticate
	if err := session.Authenticate(ctx, passCred.Username, passCred.Password, a.config.LoginPrompts, a.config.PasswordPrompts); err != nil {
		conn.Close()
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("telnet authentication failed: %w", err)
	}

	a.conn = conn
	a.session = session
	a.connected = true
	a.sessionStart = time.Now()
	a.lastConnect = time.Now()
	a.lastError = nil
	a.metrics.ConnectionCount++
	a.metrics.LastActivity = time.Now()

	return nil
}

// Execute executes a command on the device.
func (a *Adapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	a.mu.RLock()
	session := a.session
	connected := a.connected
	security := a.security
	a.mu.RUnlock()

	if !connected || session == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Check session expiry
	if security != nil && security.IsSessionExpired(a.sessionStart) {
		return nil, fmt.Errorf("telnet session expired (max duration exceeded)")
	}

	result := &protocols.ExecuteResult{
		StartTime: time.Now(),
	}

	// Build command
	command := req.Command
	if len(req.Args) > 0 {
		for _, arg := range req.Args {
			command += " " + arg
		}
	}

	// Execute
	sessResult, err := session.Execute(ctx, command)
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	if err != nil {
		result.ExitCode = 1
		result.Error = err.Error()
	} else {
		result.Stdout = []byte(sessResult.Output)
		result.ExitCode = 0
	}

	// Audit log
	if security != nil {
		security.AuditCommand(a.device.ID, command, result.Duration, err)
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

// Disconnect closes the Telnet connection.
func (a *Adapter) Disconnect(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closeInternal()
}

// closeInternal closes the connection without locking (caller must hold mu).
func (a *Adapter) closeInternal() error {
	var err error
	if a.session != nil {
		err = a.session.Close()
		a.session = nil
		a.conn = nil // session.Close already closed the conn
	} else if a.conn != nil {
		err = a.conn.Close()
		a.conn = nil
	}
	a.connected = false
	return err
}

// HealthCheck performs a health check on the Telnet connection.
func (a *Adapter) HealthCheck(_ context.Context) (*protocols.HealthCheckResult, error) {
	a.mu.RLock()
	connected := a.connected
	session := a.session
	a.mu.RUnlock()

	result := &protocols.HealthCheckResult{
		LastCheck: time.Now(),
		Details:   make(map[string]interface{}),
	}

	if !connected || session == nil {
		result.Healthy = false
		result.Status = "not connected"
		return result, nil
	}

	// Check session expiry
	if a.security != nil && a.security.IsSessionExpired(a.sessionStart) {
		result.Healthy = false
		result.Status = "session expired"
		return result, nil
	}

	// Verify connection is still alive by writing a NOP
	start := time.Now()
	_, err := a.conn.Write([]byte{IAC, NOP})
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("connection check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["last_connect"] = a.lastConnect
	result.Details["session_start"] = a.sessionStart

	return result, nil
}

// IsConnected returns true if the Telnet connection is active.
func (a *Adapter) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected && a.conn != nil
}

// Metrics returns the adapter metrics.
func (a *Adapter) Metrics() *protocols.AdapterMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.metrics
}

// NewAdapterFactory creates an adapter factory for Telnet.
func NewAdapterFactory(config *Config) protocols.AdapterFactory {
	return func(connConfig *protocols.ConnectionConfig) (protocols.ProtocolAdapter, error) {
		cfg := config
		if cfg == nil {
			cfg = DefaultConfig()
		}
		cfg.ConnectionConfig = connConfig
		return NewAdapter(cfg, nil), nil
	}
}

func init() {
	protocols.Register(protocols.ProtocolTelnet, NewAdapterFactory(nil))
}
