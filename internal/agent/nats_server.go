// Package agent provides the Keystone Core agent implementation
package agent

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

// EmbeddedNATSMode defines how the agent's embedded NATS operates
type EmbeddedNATSMode int

const (
	// EmbeddedNATSModeDisabled means agent connects to external NATS only
	EmbeddedNATSModeDisabled EmbeddedNATSMode = iota
	// EmbeddedNATSModeStandalone agent hosts NATS, accepts connections
	EmbeddedNATSModeStandalone
	// EmbeddedNATSModeLeaf agent hosts NATS that also connects as leaf to upstream
	EmbeddedNATSModeLeaf
)

// String returns string representation of EmbeddedNATSMode
func (m EmbeddedNATSMode) String() string {
	switch m {
	case EmbeddedNATSModeDisabled:
		return "disabled"
	case EmbeddedNATSModeStandalone:
		return "standalone"
	case EmbeddedNATSModeLeaf:
		return "leaf"
	default:
		return "unknown"
	}
}

// ParseEmbeddedNATSMode parses a string to EmbeddedNATSMode
func ParseEmbeddedNATSMode(s string) (EmbeddedNATSMode, error) {
	switch s {
	case "disabled", "":
		return EmbeddedNATSModeDisabled, nil
	case "standalone":
		return EmbeddedNATSModeStandalone, nil
	case "leaf":
		return EmbeddedNATSModeLeaf, nil
	default:
		return EmbeddedNATSModeDisabled, fmt.Errorf("unknown embedded NATS mode: %s", s)
	}
}

// EmbeddedNATSConfig configures the agent's embedded NATS server
type EmbeddedNATSConfig struct {
	// Mode determines how embedded NATS operates
	Mode EmbeddedNATSMode

	// Host is the interface to bind to (default: "127.0.0.1" for security)
	// Use "0.0.0.0" to allow external connections (requires TLS+auth)
	Host string
	// Port is the client connection port (default: 4222)
	Port int

	// AdvertiseHost is the externally reachable host (for NAT traversal)
	AdvertiseHost string
	// AdvertisePort is the externally reachable port (for NAT traversal)
	AdvertisePort int

	// TLS configuration
	TLSConfig *EmbeddedNATSTLSConfig

	// Authentication configuration
	AuthConfig *EmbeddedNATSAuthConfig

	// Resource limits
	MaxConnections int           // Maximum client connections (default: 100)
	MaxPayload     int32         // Maximum message payload size (default: 1MB)
	MaxPending     int64         // Maximum pending bytes per connection (default: 64MB)
	WriteDeadline  time.Duration // Write deadline (default: 10s)

	// Leaf node configuration (only for EmbeddedNATSModeLeaf)
	LeafRemotes []LeafRemoteConfig

	// ServerName for identification
	ServerName string

	// Debug enables debug logging
	Debug bool
	// Trace enables trace logging
	Trace bool
}

// EmbeddedNATSTLSConfig configures TLS for the embedded NATS server
type EmbeddedNATSTLSConfig struct {
	// CertFile is the path to the TLS certificate
	CertFile string
	// KeyFile is the path to the TLS key
	KeyFile string
	// CAFile is the path to the CA certificate for client verification
	CAFile string
	// VerifyClient requires client certificate verification
	VerifyClient bool
	// MinVersion is the minimum TLS version (default: TLS 1.2)
	MinVersion uint16
	// TLSConfig allows providing a pre-configured *tls.Config
	TLSConfig *tls.Config
}

// EmbeddedNATSAuthConfig configures authentication for the embedded NATS server
type EmbeddedNATSAuthConfig struct {
	// Token for simple token authentication
	Token string
	// Users for username/password authentication
	Users []EmbeddedNATSUser
	// NKeyUsers for NKey-based authentication
	NKeyUsers []EmbeddedNATSNKeyUser
	// AllowAnonymous allows connections without credentials
	AllowAnonymous bool
}

// EmbeddedNATSUser defines a user for username/password authentication
type EmbeddedNATSUser struct {
	Username    string
	Password    string
	Permissions *EmbeddedNATSPermissions
}

// EmbeddedNATSNKeyUser defines a user for NKey authentication
type EmbeddedNATSNKeyUser struct {
	NKey        string
	Permissions *EmbeddedNATSPermissions
}

// EmbeddedNATSPermissions defines publish/subscribe permissions
type EmbeddedNATSPermissions struct {
	Publish   PermissionScope
	Subscribe PermissionScope
}

// PermissionScope defines allowed and denied subjects
type PermissionScope struct {
	Allow []string
	Deny  []string
}

// LeafRemoteConfig configures a remote leaf node connection
type LeafRemoteConfig struct {
	// URLs are the remote NATS server URLs
	URLs []string
	// Credentials is the path to the credentials file
	Credentials string
	// TLSConfig for the remote connection
	TLSConfig *EmbeddedNATSTLSConfig
	// ReconnectInterval for automatic reconnection
	ReconnectInterval time.Duration
	// Hub marks this as a hub connection (for interest propagation)
	Hub bool
}

// DefaultEmbeddedNATSConfig returns the default configuration
// Security: When enabled, binds to all interfaces but requires TLS+auth
func DefaultEmbeddedNATSConfig() *EmbeddedNATSConfig {
	return &EmbeddedNATSConfig{
		Mode:           EmbeddedNATSModeDisabled,
		Host:           "0.0.0.0", // Listen on all interfaces (TLS+auth required)
		Port:           4222,
		MaxConnections: 100,
		MaxPayload:     1024 * 1024,      // 1MB
		MaxPending:     64 * 1024 * 1024, // 64MB
		WriteDeadline:  10 * time.Second,
	}
}

// isLocalhost returns true if the host is a loopback address
func isLocalhost(host string) bool {
	if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Validate validates the configuration
func (c *EmbeddedNATSConfig) Validate() error {
	if c.Mode == EmbeddedNATSModeDisabled {
		return nil // No validation needed for disabled mode
	}

	if c.Port <= 0 || c.Port > 65535 {
		return errors.New("invalid port number")
	}

	if c.MaxConnections < 0 {
		return errors.New("max connections must be non-negative")
	}

	if c.MaxPayload < 0 {
		return errors.New("max payload must be non-negative")
	}

	if c.Mode == EmbeddedNATSModeLeaf && len(c.LeafRemotes) == 0 {
		return errors.New("leaf mode requires at least one remote configuration")
	}

	// Security: Require TLS and authentication when binding to non-localhost
	if !isLocalhost(c.Host) {
		// Check TLS is configured
		if c.TLSConfig == nil {
			return errors.New("security: TLS is required when binding to non-localhost address (use host=127.0.0.1 for local-only access)")
		}

		// Check authentication is configured
		if c.AuthConfig == nil {
			return errors.New("security: authentication is required when binding to non-localhost address")
		}

		// Verify auth is not anonymous-only
		if c.AuthConfig.AllowAnonymous && c.AuthConfig.Token == "" &&
			len(c.AuthConfig.Users) == 0 && len(c.AuthConfig.NKeyUsers) == 0 {
			return errors.New("security: anonymous-only access is not allowed when binding to non-localhost address")
		}
	}

	// Validate TLS config if present
	if c.TLSConfig != nil {
		if c.TLSConfig.TLSConfig == nil {
			if c.TLSConfig.CertFile == "" || c.TLSConfig.KeyFile == "" {
				return errors.New("TLS requires both cert and key files")
			}
		}
	}

	return nil
}

// GetAdvertiseAddress returns the address to advertise for discovery
func (c *EmbeddedNATSConfig) GetAdvertiseAddress() string {
	host := c.AdvertiseHost
	if host == "" {
		host = c.Host
		if host == "0.0.0.0" || host == "::" {
			// Try to get local IP
			if ip := getOutboundIP(); ip != "" {
				host = ip
			} else {
				host = "localhost"
			}
		}
	}

	port := c.AdvertisePort
	if port == 0 {
		port = c.Port
	}

	return fmt.Sprintf("%s:%d", host, port)
}

// EmbeddedNATSState represents the state of the embedded NATS server
type EmbeddedNATSState int

const (
	// EmbeddedNATSStateStopped server is not running
	EmbeddedNATSStateStopped EmbeddedNATSState = iota
	// EmbeddedNATSStateStarting server is starting
	EmbeddedNATSStateStarting
	// EmbeddedNATSStateRunning server is running and accepting connections
	EmbeddedNATSStateRunning
	// EmbeddedNATSStateStopping server is shutting down
	EmbeddedNATSStateStopping
)

// String returns string representation of EmbeddedNATSState
func (s EmbeddedNATSState) String() string {
	switch s {
	case EmbeddedNATSStateStopped:
		return "stopped"
	case EmbeddedNATSStateStarting:
		return "starting"
	case EmbeddedNATSStateRunning:
		return "running"
	case EmbeddedNATSStateStopping:
		return "stopping"
	default:
		return "unknown"
	}
}

// EmbeddedNATSServer manages the agent's embedded NATS server
type EmbeddedNATSServer struct {
	config *EmbeddedNATSConfig
	server *server.Server
	state  atomic.Int32

	// Callbacks
	onStateChange func(state EmbeddedNATSState)
	onClientConn  func(clientID uint64, connected bool)

	// Stats tracking
	statsLock       sync.RWMutex
	totalConns      int64
	currentConns    int64
	totalMsgs       int64
	totalBytes      int64
	startTime       time.Time
	lastClientConn  time.Time
	lastClientDisc  time.Time
	connectedClients map[uint64]time.Time

	mu sync.RWMutex
}

// NewEmbeddedNATSServer creates a new embedded NATS server
func NewEmbeddedNATSServer(config *EmbeddedNATSConfig) (*EmbeddedNATSServer, error) {
	if config == nil {
		config = DefaultEmbeddedNATSConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &EmbeddedNATSServer{
		config:           config,
		connectedClients: make(map[uint64]time.Time),
	}, nil
}

// SetStateChangeCallback sets a callback for state changes
func (e *EmbeddedNATSServer) SetStateChangeCallback(cb func(state EmbeddedNATSState)) {
	e.mu.Lock()
	e.onStateChange = cb
	e.mu.Unlock()
}

// SetClientConnCallback sets a callback for client connections/disconnections
func (e *EmbeddedNATSServer) SetClientConnCallback(cb func(clientID uint64, connected bool)) {
	e.mu.Lock()
	e.onClientConn = cb
	e.mu.Unlock()
}

// Start starts the embedded NATS server
func (e *EmbeddedNATSServer) Start(ctx context.Context) error {
	if e.config.Mode == EmbeddedNATSModeDisabled {
		return nil
	}

	e.setState(EmbeddedNATSStateStarting)

	opts, err := e.buildServerOptions()
	if err != nil {
		e.setState(EmbeddedNATSStateStopped)
		return fmt.Errorf("failed to build server options: %w", err)
	}

	// Create NATS server
	ns, err := server.NewServer(opts)
	if err != nil {
		e.setState(EmbeddedNATSStateStopped)
		return fmt.Errorf("failed to create NATS server: %w", err)
	}

	e.mu.Lock()
	e.server = ns
	e.startTime = time.Now()
	e.mu.Unlock()

	// Start the server in a goroutine
	go ns.Start()

	// Wait for server to be ready
	if !ns.ReadyForConnections(10 * time.Second) {
		e.setState(EmbeddedNATSStateStopped)
		return errors.New("NATS server failed to become ready")
	}

	e.setState(EmbeddedNATSStateRunning)
	return nil
}

// Stop stops the embedded NATS server
func (e *EmbeddedNATSServer) Stop() error {
	e.mu.Lock()
	ns := e.server
	e.mu.Unlock()

	if ns == nil {
		return nil
	}

	e.setState(EmbeddedNATSStateStopping)

	// Graceful shutdown
	ns.Shutdown()

	// Wait for shutdown to complete
	ns.WaitForShutdown()

	e.mu.Lock()
	e.server = nil
	e.mu.Unlock()

	e.setState(EmbeddedNATSStateStopped)
	return nil
}

// State returns the current server state
func (e *EmbeddedNATSServer) State() EmbeddedNATSState {
	return EmbeddedNATSState(e.state.Load())
}

// IsRunning returns true if the server is running
func (e *EmbeddedNATSServer) IsRunning() bool {
	return e.State() == EmbeddedNATSStateRunning
}

// GetClientURL returns the NATS URL for clients to connect to
func (e *EmbeddedNATSServer) GetClientURL() string {
	e.mu.RLock()
	ns := e.server
	e.mu.RUnlock()

	if ns == nil {
		return ""
	}

	// Check if TLS is enabled
	scheme := "nats"
	if e.config.TLSConfig != nil {
		scheme = "tls"
	}

	return fmt.Sprintf("%s://%s", scheme, e.config.GetAdvertiseAddress())
}

// GetStats returns server statistics
func (e *EmbeddedNATSServer) GetStats() *EmbeddedNATSStats {
	e.mu.RLock()
	ns := e.server
	e.mu.RUnlock()

	stats := &EmbeddedNATSStats{
		State: e.State(),
	}

	if ns == nil {
		return stats
	}

	// Get NATS server stats
	varz, err := ns.Varz(nil)
	if err == nil {
		stats.Connections = int(varz.Connections)
		stats.TotalConnections = int64(varz.TotalConnections)
		stats.InMsgs = int64(varz.InMsgs)
		stats.OutMsgs = int64(varz.OutMsgs)
		stats.InBytes = int64(varz.InBytes)
		stats.OutBytes = int64(varz.OutBytes)
		stats.SlowConsumers = int64(varz.SlowConsumers)
		stats.Uptime = time.Since(e.startTime)
	}

	// Add tracked stats
	e.statsLock.RLock()
	stats.LastClientConnect = e.lastClientConn
	stats.LastClientDisconnect = e.lastClientDisc
	e.statsLock.RUnlock()

	return stats
}

// EmbeddedNATSStats contains server statistics
type EmbeddedNATSStats struct {
	State                EmbeddedNATSState
	Connections          int
	TotalConnections     int64
	InMsgs               int64
	OutMsgs              int64
	InBytes              int64
	OutBytes             int64
	SlowConsumers        int64
	Uptime               time.Duration
	LastClientConnect    time.Time
	LastClientDisconnect time.Time
}

// buildServerOptions builds NATS server options from config
func (e *EmbeddedNATSServer) buildServerOptions() (*server.Options, error) {
	opts := &server.Options{
		Host:           e.config.Host,
		Port:           e.config.Port,
		MaxConn:        e.config.MaxConnections,
		MaxPayload:     e.config.MaxPayload,
		MaxPending:     e.config.MaxPending,
		WriteDeadline:  e.config.WriteDeadline,
		NoLog:          true,
		NoSigs:         true,
		Debug:          e.config.Debug,
		Trace:          e.config.Trace,
	}

	// Set server name
	if e.config.ServerName != "" {
		opts.ServerName = e.config.ServerName
	}

	// Configure TLS
	if e.config.TLSConfig != nil {
		if err := e.configureTLS(opts, e.config.TLSConfig); err != nil {
			return nil, err
		}
	}

	// Configure authentication
	if e.config.AuthConfig != nil {
		e.configureAuth(opts, e.config.AuthConfig)
	}

	// Configure leaf nodes
	if e.config.Mode == EmbeddedNATSModeLeaf {
		if err := e.configureLeafNodes(opts); err != nil {
			return nil, err
		}
	}

	return opts, nil
}

// configureTLS configures TLS for the server
func (e *EmbeddedNATSServer) configureTLS(opts *server.Options, tlsConfig *EmbeddedNATSTLSConfig) error {
	if tlsConfig.TLSConfig != nil {
		opts.TLSConfig = tlsConfig.TLSConfig
		opts.TLS = true
		return nil
	}

	opts.TLSCert = tlsConfig.CertFile
	opts.TLSKey = tlsConfig.KeyFile
	opts.TLSCaCert = tlsConfig.CAFile
	opts.TLSVerify = tlsConfig.VerifyClient
	opts.TLS = true

	return nil
}

// configureAuth configures authentication for the server
func (e *EmbeddedNATSServer) configureAuth(opts *server.Options, authConfig *EmbeddedNATSAuthConfig) {
	if authConfig.Token != "" {
		opts.Authorization = authConfig.Token
		return
	}

	if len(authConfig.Users) > 0 {
		opts.Users = make([]*server.User, len(authConfig.Users))
		for i, u := range authConfig.Users {
			opts.Users[i] = &server.User{
				Username: u.Username,
				Password: u.Password,
			}
			if u.Permissions != nil {
				opts.Users[i].Permissions = e.convertPermissions(u.Permissions)
			}
		}
		return
	}

	if len(authConfig.NKeyUsers) > 0 {
		opts.Nkeys = make([]*server.NkeyUser, len(authConfig.NKeyUsers))
		for i, u := range authConfig.NKeyUsers {
			opts.Nkeys[i] = &server.NkeyUser{
				Nkey: u.NKey,
			}
			if u.Permissions != nil {
				opts.Nkeys[i].Permissions = e.convertPermissions(u.Permissions)
			}
		}
		return
	}

	if authConfig.AllowAnonymous {
		opts.NoAuthUser = ""
	}
}

// convertPermissions converts our permission type to NATS server type
func (e *EmbeddedNATSServer) convertPermissions(perms *EmbeddedNATSPermissions) *server.Permissions {
	if perms == nil {
		return nil
	}

	result := &server.Permissions{}

	if len(perms.Publish.Allow) > 0 || len(perms.Publish.Deny) > 0 {
		result.Publish = &server.SubjectPermission{
			Allow: perms.Publish.Allow,
			Deny:  perms.Publish.Deny,
		}
	}

	if len(perms.Subscribe.Allow) > 0 || len(perms.Subscribe.Deny) > 0 {
		result.Subscribe = &server.SubjectPermission{
			Allow: perms.Subscribe.Allow,
			Deny:  perms.Subscribe.Deny,
		}
	}

	return result
}

// configureLeafNodes configures leaf node connections
func (e *EmbeddedNATSServer) configureLeafNodes(opts *server.Options) error {
	if len(e.config.LeafRemotes) == 0 {
		return nil
	}

	opts.LeafNode.Remotes = make([]*server.RemoteLeafOpts, len(e.config.LeafRemotes))
	for i, remote := range e.config.LeafRemotes {
		rem := &server.RemoteLeafOpts{
			Hub: remote.Hub,
		}

		// Parse URLs
		for _, urlStr := range remote.URLs {
			parsedURL, err := url.Parse(urlStr)
			if err != nil {
				return fmt.Errorf("failed to parse leaf remote URL %s: %w", urlStr, err)
			}
			rem.URLs = append(rem.URLs, parsedURL)
		}

		// Set credentials
		if remote.Credentials != "" {
			rem.Credentials = remote.Credentials
		}

		// Configure TLS for leaf connection
		if remote.TLSConfig != nil {
			if remote.TLSConfig.TLSConfig != nil {
				rem.TLSConfig = remote.TLSConfig.TLSConfig
			} else if remote.TLSConfig.CertFile != "" {
				// Load TLS config from files
				cert, err := tls.LoadX509KeyPair(remote.TLSConfig.CertFile, remote.TLSConfig.KeyFile)
				if err != nil {
					return fmt.Errorf("failed to load leaf TLS cert: %w", err)
				}
				rem.TLSConfig = &tls.Config{
					Certificates: []tls.Certificate{cert},
					MinVersion:   tls.VersionTLS12,
				}
			}
		}

		opts.LeafNode.Remotes[i] = rem
	}

	return nil
}

// setState updates the server state and calls callback
func (e *EmbeddedNATSServer) setState(state EmbeddedNATSState) {
	e.state.Store(int32(state))

	e.mu.RLock()
	cb := e.onStateChange
	e.mu.RUnlock()

	if cb != nil {
		cb(state)
	}
}

// recordClientConnect records a client connection
func (e *EmbeddedNATSServer) recordClientConnect(clientID uint64) {
	e.statsLock.Lock()
	e.totalConns++
	e.currentConns++
	e.lastClientConn = time.Now()
	e.connectedClients[clientID] = e.lastClientConn
	e.statsLock.Unlock()

	e.mu.RLock()
	cb := e.onClientConn
	e.mu.RUnlock()

	if cb != nil {
		cb(clientID, true)
	}
}

// recordClientDisconnect records a client disconnection
func (e *EmbeddedNATSServer) recordClientDisconnect(clientID uint64) {
	e.statsLock.Lock()
	e.currentConns--
	e.lastClientDisc = time.Now()
	delete(e.connectedClients, clientID)
	e.statsLock.Unlock()

	e.mu.RLock()
	cb := e.onClientConn
	e.mu.RUnlock()

	if cb != nil {
		cb(clientID, false)
	}
}

// getOutboundIP gets the preferred outbound IP of this machine
func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// Config returns the server configuration
func (e *EmbeddedNATSServer) Config() *EmbeddedNATSConfig {
	return e.config
}

// Server returns the underlying NATS server (for advanced usage)
func (e *EmbeddedNATSServer) Server() *server.Server {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.server
}
