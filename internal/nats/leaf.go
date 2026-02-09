// Package nats provides NATS messaging infrastructure for Keystone Core
package nats

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// LeafNodeRole defines the role of a node in the leaf topology
type LeafNodeRole int

const (
	// LeafNodeRoleLeaf is a leaf node that connects to a hub
	LeafNodeRoleLeaf LeafNodeRole = iota
	// LeafNodeRoleHub is a hub that accepts leaf connections
	LeafNodeRoleHub
	// LeafNodeRoleBridge is both a leaf and a hub (intermediate node)
	LeafNodeRoleBridge
)

// String returns string representation of LeafNodeRole
func (r LeafNodeRole) String() string {
	switch r {
	case LeafNodeRoleLeaf:
		return "leaf"
	case LeafNodeRoleHub:
		return "hub"
	case LeafNodeRoleBridge:
		return "bridge"
	default:
		return "unknown"
	}
}

// ParseLeafNodeRole parses a string to LeafNodeRole
func ParseLeafNodeRole(s string) (LeafNodeRole, error) {
	switch s {
	case "leaf", "":
		return LeafNodeRoleLeaf, nil
	case "hub":
		return LeafNodeRoleHub, nil
	case "bridge":
		return LeafNodeRoleBridge, nil
	default:
		return LeafNodeRoleLeaf, fmt.Errorf("unknown leaf node role: %s", s)
	}
}

// LeafNodeConfig configures a NATS leaf node
type LeafNodeConfig struct {
	// Role determines if this is a leaf, hub, or bridge
	Role LeafNodeRole

	// Name is an identifier for this leaf node
	Name string

	// Remotes are the upstream hub connections (for leaf/bridge roles)
	Remotes []LeafRemoteConfig

	// Listen is the address to accept leaf connections on (for hub/bridge roles)
	Listen string

	// Port is the port to accept leaf connections on (default: 7422)
	Port int

	// TLS configuration for leaf connections
	TLS *LeafTLSConfig

	// Authorization for leaf connections
	Auth *LeafAuthConfig

	// SubjectMappings defines subject transformations
	SubjectMappings []SubjectMapping

	// Imports defines subjects to import from remote
	Imports []SubjectPermission

	// Exports defines subjects to export to remote
	Exports []SubjectPermission

	// Compression enables compression for leaf connections
	Compression bool

	// ReconnectInterval is how long to wait before reconnecting
	ReconnectInterval time.Duration

	// MaxReconnects is the maximum reconnection attempts (-1 for unlimited)
	MaxReconnects int

	// NoRandomize disables random server selection
	NoRandomize bool
}

// LeafRemoteConfig configures a remote hub connection
type LeafRemoteConfig struct {
	// URLs are the remote hub URLs
	URLs []string

	// Credentials is the path to credentials file
	Credentials string

	// Account is the account to connect as
	Account string

	// TLS configuration for this remote
	TLS *LeafTLSConfig

	// Hub indicates if this remote should receive all messages (hub mode)
	Hub bool

	// DenyImports prevents importing messages from this remote
	DenyImports bool

	// DenyExports prevents exporting messages to this remote
	DenyExports bool

	// LocalAccountName is the local account for this connection
	LocalAccountName string

	// Imports defines subjects to import from this remote
	Imports []SubjectPermission

	// Exports defines subjects to export to this remote
	Exports []SubjectPermission

	// Compression enables compression for this remote
	Compression bool

	// ReconnectInterval for this specific remote
	ReconnectInterval time.Duration

	// SigningKey for JWT-based authentication
	SigningKey string
}

// LeafTLSConfig configures TLS for leaf connections
type LeafTLSConfig struct {
	// CertFile is the certificate file path
	CertFile string

	// KeyFile is the key file path
	KeyFile string

	// CAFile is the CA certificate file path
	CAFile string

	// InsecureSkipVerify skips certificate verification
	InsecureSkipVerify bool

	// TLSConfig allows providing a pre-configured *tls.Config
	TLSConfig *tls.Config
}

// LeafAuthConfig configures authorization for leaf connections
type LeafAuthConfig struct {
	// Users allowed to connect as leaf nodes
	Users []LeafUser

	// Token for simple token authentication
	Token string

	// Account is the default account for leaf connections
	Account string
}

// LeafUser defines a user for leaf authentication
type LeafUser struct {
	// User is the username
	User string

	// Password is the password
	Password string

	// Account is the account for this user
	Account string

	// Permissions for this user
	Permissions *LeafPermissions
}

// LeafPermissions defines publish/subscribe permissions for leaf connections
type LeafPermissions struct {
	// Publish permissions
	Publish SubjectPermission

	// Subscribe permissions
	Subscribe SubjectPermission
}

// SubjectPermission defines allowed and denied subjects
type SubjectPermission struct {
	// Allow is a list of allowed subjects
	Allow []string

	// Deny is a list of denied subjects
	Deny []string
}

// SubjectMapping defines a subject transformation
type SubjectMapping struct {
	// Source is the source subject pattern
	Source string

	// Destination is the destination subject pattern
	Destination string

	// Weight is used for weighted mappings (0-100)
	Weight int

	// Cluster restricts mapping to specific cluster
	Cluster string
}

// DefaultLeafNodeConfig returns default leaf configuration
func DefaultLeafNodeConfig() *LeafNodeConfig {
	return &LeafNodeConfig{
		Role:              LeafNodeRoleLeaf,
		Port:              7422,
		ReconnectInterval: 2 * time.Second,
		MaxReconnects:     -1, // Unlimited
	}
}

// Validate validates the leaf node configuration
func (c *LeafNodeConfig) Validate() error {
	if c.Role == LeafNodeRoleLeaf || c.Role == LeafNodeRoleBridge {
		if len(c.Remotes) == 0 {
			return errors.New("leaf/bridge role requires at least one remote")
		}
		for i := range c.Remotes {
			remote := &c.Remotes[i]
			if len(remote.URLs) == 0 {
				return fmt.Errorf("remote %d has no URLs", i)
			}
			for _, urlStr := range remote.URLs {
				if _, err := url.Parse(urlStr); err != nil {
					return fmt.Errorf("remote %d has invalid URL %s: %w", i, urlStr, err)
				}
			}
		}
	}

	if c.Role == LeafNodeRoleHub || c.Role == LeafNodeRoleBridge {
		if c.Port <= 0 || c.Port > 65535 {
			return fmt.Errorf("invalid port: %d", c.Port)
		}
	}

	if c.ReconnectInterval < 0 {
		return errors.New("reconnect interval cannot be negative")
	}

	return nil
}

// LeafConnectionState represents the state of a leaf connection
type LeafConnectionState int

const (
	// LeafConnectionStateDisconnected no connection
	LeafConnectionStateDisconnected LeafConnectionState = iota
	// LeafConnectionStateConnecting establishing connection
	LeafConnectionStateConnecting
	// LeafConnectionStateConnected connected and operational
	LeafConnectionStateConnected
	// LeafConnectionStateReconnecting reconnecting after disconnect
	LeafConnectionStateReconnecting
	// LeafConnectionStateFailed connection failed
	LeafConnectionStateFailed
)

// String returns string representation of LeafConnectionState
func (s LeafConnectionState) String() string {
	switch s {
	case LeafConnectionStateDisconnected:
		return "disconnected"
	case LeafConnectionStateConnecting:
		return "connecting"
	case LeafConnectionStateConnected:
		return "connected"
	case LeafConnectionStateReconnecting:
		return "reconnecting"
	case LeafConnectionStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// LeafConnection represents a connection to a remote hub
type LeafConnection struct {
	// Remote configuration
	Remote *LeafRemoteConfig

	// State of this connection
	state atomic.Int32

	// ConnectedURL is the currently connected URL
	ConnectedURL string

	// ConnectTime is when the connection was established
	ConnectTime time.Time

	// LastError is the last connection error
	lastError atomic.Value // error

	// Reconnects is the number of reconnection attempts
	reconnects atomic.Int64

	// Latency is the current connection latency
	latency atomic.Int64 // nanoseconds
}

// State returns the current connection state
func (c *LeafConnection) State() LeafConnectionState {
	return LeafConnectionState(c.state.Load())
}

// IsConnected returns true if connected
func (c *LeafConnection) IsConnected() bool {
	return c.State() == LeafConnectionStateConnected
}

// LastError returns the last error
func (c *LeafConnection) LastError() error {
	if v := c.lastError.Load(); v != nil {
		return v.(error)
	}
	return nil
}

// Latency returns the connection latency
func (c *LeafConnection) Latency() time.Duration {
	return time.Duration(c.latency.Load())
}

// Reconnects returns the reconnection count
func (c *LeafConnection) Reconnects() int64 {
	return c.reconnects.Load()
}

// LeafNodeManager manages leaf node connections
type LeafNodeManager struct {
	config *LeafNodeConfig

	// Embedded NATS server for hub mode
	server *server.Server

	// Client connection to the embedded server
	client *nats.Conn

	// Leaf connections (for leaf/bridge mode)
	connections []*LeafConnection

	// State
	state   atomic.Int32
	running atomic.Bool

	// Callbacks
	onStateChange      func(state LeafConnectionState)
	onRemoteConnect    func(remote *LeafRemoteConfig)
	onRemoteDisconnect func(remote *LeafRemoteConfig, err error)
	onMessage          func(subject string, data []byte, isFromRemote bool)

	// Internal
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewLeafNodeManager creates a new leaf node manager
func NewLeafNodeManager(config *LeafNodeConfig) (*LeafNodeManager, error) {
	if config == nil {
		config = DefaultLeafNodeConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	manager := &LeafNodeManager{
		config:      config,
		connections: make([]*LeafConnection, 0, len(config.Remotes)),
	}

	// Initialize leaf connections
	for i := range config.Remotes {
		remoteCopy := config.Remotes[i]
		manager.connections = append(manager.connections, &LeafConnection{
			Remote: &remoteCopy,
		})
	}

	return manager, nil
}

// SetStateChangeCallback sets a callback for state changes
func (m *LeafNodeManager) SetStateChangeCallback(cb func(state LeafConnectionState)) {
	m.mu.Lock()
	m.onStateChange = cb
	m.mu.Unlock()
}

// SetRemoteConnectCallback sets a callback for remote connections
func (m *LeafNodeManager) SetRemoteConnectCallback(cb func(remote *LeafRemoteConfig)) {
	m.mu.Lock()
	m.onRemoteConnect = cb
	m.mu.Unlock()
}

// SetRemoteDisconnectCallback sets a callback for remote disconnections
func (m *LeafNodeManager) SetRemoteDisconnectCallback(cb func(remote *LeafRemoteConfig, err error)) {
	m.mu.Lock()
	m.onRemoteDisconnect = cb
	m.mu.Unlock()
}

// SetMessageCallback sets a callback for messages (for debugging/monitoring)
func (m *LeafNodeManager) SetMessageCallback(cb func(subject string, data []byte, isFromRemote bool)) {
	m.mu.Lock()
	m.onMessage = cb
	m.mu.Unlock()
}

// Start starts the leaf node manager
func (m *LeafNodeManager) Start(ctx context.Context) error {
	if m.running.Load() {
		return errors.New("manager already running")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)
	m.running.Store(true)
	m.setState(LeafConnectionStateConnecting)

	// Start based on role
	var err error
	switch m.config.Role {
	case LeafNodeRoleHub:
		err = m.startAsHub()
	case LeafNodeRoleLeaf:
		err = m.startAsLeaf()
	case LeafNodeRoleBridge:
		err = m.startAsBridge()
	default:
		err = fmt.Errorf("unknown role: %v", m.config.Role)
	}

	if err != nil {
		m.running.Store(false)
		m.setState(LeafConnectionStateFailed)
		return err
	}

	m.setState(LeafConnectionStateConnected)
	return nil
}

// Stop stops the leaf node manager
func (m *LeafNodeManager) Stop() error {
	if !m.running.Load() {
		return nil
	}

	m.running.Store(false)
	if m.cancel != nil {
		m.cancel()
	}

	m.wg.Wait()

	// Close client connection
	m.mu.Lock()
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}

	// Shutdown server
	if m.server != nil {
		m.server.Shutdown()
		m.server.WaitForShutdown()
		m.server = nil
	}
	m.mu.Unlock()

	m.setState(LeafConnectionStateDisconnected)
	return nil
}

// State returns the current state
func (m *LeafNodeManager) State() LeafConnectionState {
	return LeafConnectionState(m.state.Load())
}

// IsRunning returns true if the manager is running
func (m *LeafNodeManager) IsRunning() bool {
	return m.running.Load()
}

// GetClient returns the NATS client connection
func (m *LeafNodeManager) GetClient() *nats.Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.client
}

// GetConnections returns all leaf connections
func (m *LeafNodeManager) GetConnections() []*LeafConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*LeafConnection, len(m.connections))
	copy(result, m.connections)
	return result
}

// GetConnectedRemotes returns currently connected remotes
func (m *LeafNodeManager) GetConnectedRemotes() []*LeafRemoteConfig {
	var connected []*LeafRemoteConfig
	for _, conn := range m.GetConnections() {
		if conn.IsConnected() {
			connected = append(connected, conn.Remote)
		}
	}
	return connected
}

// startAsHub starts as a hub accepting leaf connections
func (m *LeafNodeManager) startAsHub() error {
	opts, err := m.buildHubServerOptions()
	if err != nil {
		return fmt.Errorf("build server options: %w", err)
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return fmt.Errorf("create NATS server: %w", err)
	}

	go ns.Start()

	if !ns.ReadyForConnections(10 * time.Second) {
		return errors.New("server failed to become ready")
	}

	m.mu.Lock()
	m.server = ns
	m.mu.Unlock()

	// Connect as local client
	clientURL := ns.ClientURL()

	conn, err := nats.Connect(clientURL,
		nats.Name("kscore-leaf-hub-client"),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		ns.Shutdown()
		return fmt.Errorf("connect to local server: %w", err)
	}

	m.mu.Lock()
	m.client = conn
	m.mu.Unlock()

	return nil
}

// startAsLeaf starts as a leaf connecting to remotes
func (m *LeafNodeManager) startAsLeaf() error {
	opts, err := m.buildLeafServerOptions()
	if err != nil {
		return fmt.Errorf("build server options: %w", err)
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return fmt.Errorf("create NATS server: %w", err)
	}

	go ns.Start()

	if !ns.ReadyForConnections(10 * time.Second) {
		return errors.New("server failed to become ready")
	}

	m.mu.Lock()
	m.server = ns
	m.mu.Unlock()

	// Connect as local client
	clientURL := ns.ClientURL()

	conn, err := nats.Connect(clientURL,
		nats.Name("kscore-leaf-client"),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		ns.Shutdown()
		return fmt.Errorf("connect to local server: %w", err)
	}

	m.mu.Lock()
	m.client = conn
	m.mu.Unlock()

	// Start connection monitoring
	m.wg.Add(1)
	go m.monitorConnections()

	return nil
}

// startAsBridge starts as both hub and leaf
func (m *LeafNodeManager) startAsBridge() error {
	opts, err := m.buildBridgeServerOptions()
	if err != nil {
		return fmt.Errorf("build server options: %w", err)
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		return fmt.Errorf("create NATS server: %w", err)
	}

	go ns.Start()

	if !ns.ReadyForConnections(10 * time.Second) {
		return errors.New("server failed to become ready")
	}

	m.mu.Lock()
	m.server = ns
	m.mu.Unlock()

	// Connect as local client
	clientURL := ns.ClientURL()

	conn, err := nats.Connect(clientURL,
		nats.Name("kscore-leaf-bridge-client"),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		ns.Shutdown()
		return fmt.Errorf("connect to local server: %w", err)
	}

	m.mu.Lock()
	m.client = conn
	m.mu.Unlock()

	// Start connection monitoring
	m.wg.Add(1)
	go m.monitorConnections()

	return nil
}

// buildHubServerOptions builds NATS server options for hub mode
func (m *LeafNodeManager) buildHubServerOptions() (*server.Options, error) {
	opts := &server.Options{
		Host:   "127.0.0.1",
		Port:   -1, // Auto-assign port for client connections
		NoLog:  true,
		NoSigs: true,
	}

	// Configure leaf node listener
	listen := m.config.Listen
	if listen == "" {
		listen = "0.0.0.0"
	}

	opts.LeafNode.Host = listen
	opts.LeafNode.Port = m.config.Port
	opts.LeafNode.NoAdvertise = false

	// Configure TLS for leaf connections
	if m.config.TLS != nil {
		if err := m.configureLeafTLS(opts); err != nil {
			return nil, err
		}
	}

	// Configure authorization
	if m.config.Auth != nil {
		m.configureLeafAuth(opts)
	}

	return opts, nil
}

// buildLeafServerOptions builds NATS server options for leaf mode
func (m *LeafNodeManager) buildLeafServerOptions() (*server.Options, error) {
	opts := &server.Options{
		Host:   "127.0.0.1",
		Port:   -1, // Auto-assign port
		NoLog:  true,
		NoSigs: true,
	}

	// Configure remote connections
	for i := range m.config.Remotes {
		remoteOpts, err := m.buildRemoteOptions(&m.config.Remotes[i])
		if err != nil {
			return nil, fmt.Errorf("build remote options: %w", err)
		}
		opts.LeafNode.Remotes = append(opts.LeafNode.Remotes, remoteOpts)
	}

	return opts, nil
}

// buildBridgeServerOptions builds NATS server options for bridge mode
func (m *LeafNodeManager) buildBridgeServerOptions() (*server.Options, error) {
	// Start with hub options
	opts, err := m.buildHubServerOptions()
	if err != nil {
		return nil, err
	}

	// Add remote connections
	for i := range m.config.Remotes {
		remoteOpts, err := m.buildRemoteOptions(&m.config.Remotes[i])
		if err != nil {
			return nil, fmt.Errorf("build remote options: %w", err)
		}
		opts.LeafNode.Remotes = append(opts.LeafNode.Remotes, remoteOpts)
	}

	return opts, nil
}

// buildRemoteOptions builds leaf remote options
func (m *LeafNodeManager) buildRemoteOptions(remote *LeafRemoteConfig) (*server.RemoteLeafOpts, error) {
	remoteOpts := &server.RemoteLeafOpts{
		Hub: remote.Hub,
	}

	// Parse URLs
	for _, urlStr := range remote.URLs {
		u, err := url.Parse(urlStr)
		if err != nil {
			return nil, fmt.Errorf("parse URL %s: %w", urlStr, err)
		}
		remoteOpts.URLs = append(remoteOpts.URLs, u)
	}

	// Credentials
	if remote.Credentials != "" {
		remoteOpts.Credentials = remote.Credentials
	}

	// TLS configuration
	if remote.TLS != nil {
		tlsConfig, err := m.buildTLSConfig(remote.TLS)
		if err != nil {
			return nil, fmt.Errorf("build TLS config: %w", err)
		}
		remoteOpts.TLSConfig = tlsConfig
	}

	return remoteOpts, nil
}

// configureLeafTLS configures TLS for leaf node listener
func (m *LeafNodeManager) configureLeafTLS(opts *server.Options) error {
	tlsConfig := m.config.TLS

	if tlsConfig.TLSConfig != nil {
		opts.LeafNode.TLSConfig = tlsConfig.TLSConfig
		return nil
	}

	if tlsConfig.CertFile == "" || tlsConfig.KeyFile == "" {
		return errors.New("TLS requires both cert and key files")
	}

	// Load TLS config from files
	cert, err := tls.LoadX509KeyPair(tlsConfig.CertFile, tlsConfig.KeyFile)
	if err != nil {
		return fmt.Errorf("load TLS key pair: %w", err)
	}

	opts.LeafNode.TLSConfig = &tls.Config{
		Certificates:       []tls.Certificate{cert},
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: tlsConfig.InsecureSkipVerify, // #nosec G402 -- user-configured TLS option
	}

	return nil
}

// configureLeafAuth configures authorization for leaf connections
// Note: Leaf node authentication is typically done via accounts and JWTs
// in NATS 2.x. Basic user/pass authentication at the leaf level is limited.
func (m *LeafNodeManager) configureLeafAuth(_ *server.Options) {
	// Leaf node authentication in NATS 2.x is handled through:
	// 1. Credentials files (NKey/JWT)
	// 2. Account-based authorization
	// 3. TLS client certificates
	//
	// The LeafAuthConfig stores the configuration for reference,
	// but actual auth is applied via the remote configuration (credentials)
	// or via TLS configuration
}

// buildTLSConfig builds a TLS config from LeafTLSConfig
func (m *LeafNodeManager) buildTLSConfig(config *LeafTLSConfig) (*tls.Config, error) {
	if config.TLSConfig != nil {
		return config.TLSConfig, nil
	}

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: config.InsecureSkipVerify, // #nosec G402 -- user-configured TLS option
	}

	if config.CertFile != "" && config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load key pair: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// monitorConnections monitors the state of leaf connections
func (m *LeafNodeManager) monitorConnections() {
	defer m.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.updateConnectionStates()
		case <-m.ctx.Done():
			return
		}
	}
}

// updateConnectionStates updates the state of all connections
func (m *LeafNodeManager) updateConnectionStates() {
	m.mu.RLock()
	ns := m.server
	m.mu.RUnlock()

	if ns == nil {
		return
	}

	// Get leaf node info from server
	leafz, err := ns.Leafz(nil)
	if err != nil {
		return
	}

	// Update connection states based on leafz
	for i, conn := range m.connections {
		if i < len(leafz.Leafs) {
			leaf := leafz.Leafs[i]
			if leaf.IsSpoke {
				conn.state.Store(int32(LeafConnectionStateConnected))
				// RTT is a string like "1.5ms", parse it
				if rtt, err := time.ParseDuration(leaf.RTT); err == nil {
					conn.latency.Store(int64(rtt))
				}
			} else {
				conn.state.Store(int32(LeafConnectionStateDisconnected))
			}
		}
	}
}

// setState updates the state and calls callback
func (m *LeafNodeManager) setState(state LeafConnectionState) {
	//nolint:gosec // G115: LeafConnectionState is a small enum (0-3), fits in int32
	m.state.Store(int32(state))

	m.mu.RLock()
	cb := m.onStateChange
	m.mu.RUnlock()

	if cb != nil {
		cb(state)
	}
}

// LeafNodeStats contains statistics for the leaf node manager
type LeafNodeStats struct {
	State            LeafConnectionState
	Role             LeafNodeRole
	ConnectedRemotes int
	TotalRemotes     int
	Uptime           time.Duration
	MessagesIn       int64
	MessagesOut      int64
	BytesIn          int64
	BytesOut         int64
	Reconnects       int64
	RemoteStats      []LeafRemoteStats
}

// LeafRemoteStats contains statistics for a remote connection
type LeafRemoteStats struct {
	URL            string
	State          LeafConnectionState
	Latency        time.Duration
	Reconnects     int64
	MessagesIn     int64
	MessagesOut    int64
	LastConnected  time.Time
	LastDisconnect time.Time
	LastError      string
}

// GetStats returns current statistics
func (m *LeafNodeManager) GetStats() *LeafNodeStats {
	stats := &LeafNodeStats{
		State:        m.State(),
		Role:         m.config.Role,
		TotalRemotes: len(m.connections),
	}

	m.mu.RLock()
	ns := m.server
	m.mu.RUnlock()

	if ns != nil {
		// Get server varz
		if varz, err := ns.Varz(nil); err == nil {
			stats.MessagesIn = varz.InMsgs
			stats.MessagesOut = varz.OutMsgs
			stats.BytesIn = varz.InBytes
			stats.BytesOut = varz.OutBytes
		}

		// Get leafz
		if leafz, err := ns.Leafz(nil); err == nil {
			stats.ConnectedRemotes = leafz.NumLeafs
		}
	}

	// Get per-connection stats
	for _, conn := range m.connections {
		remoteStat := LeafRemoteStats{
			State:      conn.State(),
			Latency:    conn.Latency(),
			Reconnects: conn.Reconnects(),
		}
		if len(conn.Remote.URLs) > 0 {
			remoteStat.URL = conn.Remote.URLs[0]
		}
		if err := conn.LastError(); err != nil {
			remoteStat.LastError = err.Error()
		}
		stats.RemoteStats = append(stats.RemoteStats, remoteStat)
		stats.Reconnects += conn.Reconnects()
	}

	return stats
}

// Config returns the configuration
func (m *LeafNodeManager) Config() *LeafNodeConfig {
	return m.config
}

// LeafChainConfig configures a multi-hop leaf chain
type LeafChainConfig struct {
	// Hops defines each hop in the chain
	Hops []LeafChainHop

	// TotalTimeout is the total timeout for the entire chain
	TotalTimeout time.Duration

	// HopTimeout is the timeout per hop (calculated if TotalTimeout set)
	HopTimeout time.Duration

	// EnableJetStream enables JetStream on each hop for persistence
	EnableJetStream bool

	// BufferSize is the buffer size for messages during outage (per hop)
	BufferSize int64

	// DeduplicationWindow is the window for message deduplication
	DeduplicationWindow time.Duration
}

// LeafChainHop defines a single hop in a leaf chain
type LeafChainHop struct {
	// Name identifies this hop
	Name string

	// Role is the role at this hop (leaf, hub, or bridge)
	Role LeafNodeRole

	// UpstreamURLs are the URLs to connect to upstream
	UpstreamURLs []string

	// ListenPort is the port to listen on for downstream (hub/bridge only)
	ListenPort int

	// Credentials for upstream connection
	Credentials string

	// TLS configuration
	TLS *LeafTLSConfig

	// Latency is the expected latency to upstream (for timeout calculation)
	ExpectedLatency time.Duration
}

// DefaultLeafChainConfig returns default chain configuration
func DefaultLeafChainConfig() *LeafChainConfig {
	return &LeafChainConfig{
		TotalTimeout:        30 * time.Second,
		EnableJetStream:     true,
		BufferSize:          64 * 1024 * 1024, // 64MB
		DeduplicationWindow: 2 * time.Minute,
	}
}

// Validate validates the chain configuration
func (c *LeafChainConfig) Validate() error {
	if len(c.Hops) == 0 {
		return errors.New("chain requires at least one hop")
	}

	for i, hop := range c.Hops {
		if hop.Name == "" {
			return fmt.Errorf("hop %d has no name", i)
		}

		// First hop should be a leaf
		if i == 0 && hop.Role != LeafNodeRoleLeaf && hop.Role != LeafNodeRoleBridge {
			return fmt.Errorf("first hop must be leaf or bridge, got %s", hop.Role)
		}

		// Last hop should be a hub (or bridge if connecting further)
		if i == len(c.Hops)-1 && hop.Role != LeafNodeRoleHub && hop.Role != LeafNodeRoleBridge {
			return fmt.Errorf("last hop must be hub or bridge, got %s", hop.Role)
		}

		// Middle hops should be bridges
		if i > 0 && i < len(c.Hops)-1 && hop.Role != LeafNodeRoleBridge {
			return fmt.Errorf("middle hop %d must be bridge, got %s", i, hop.Role)
		}

		// Validate upstream URLs for leaf/bridge
		if hop.Role == LeafNodeRoleLeaf || hop.Role == LeafNodeRoleBridge {
			if len(hop.UpstreamURLs) == 0 {
				return fmt.Errorf("hop %d (%s) requires upstream URLs", i, hop.Name)
			}
		}

		// Validate listen port for hub/bridge
		if hop.Role == LeafNodeRoleHub || hop.Role == LeafNodeRoleBridge {
			if hop.ListenPort <= 0 || hop.ListenPort > 65535 {
				return fmt.Errorf("hop %d (%s) has invalid listen port: %d", i, hop.Name, hop.ListenPort)
			}
		}
	}

	return nil
}

// CalculateHopTimeout calculates the per-hop timeout based on total timeout
func (c *LeafChainConfig) CalculateHopTimeout() time.Duration {
	if c.HopTimeout > 0 {
		return c.HopTimeout
	}
	if c.TotalTimeout > 0 && len(c.Hops) > 0 {
		return c.TotalTimeout / time.Duration(len(c.Hops))
	}
	return 10 * time.Second
}

// BuildHopConfigs builds LeafNodeConfig for each hop
func (c *LeafChainConfig) BuildHopConfigs() ([]*LeafNodeConfig, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	configs := make([]*LeafNodeConfig, len(c.Hops))

	for i, hop := range c.Hops {
		config := &LeafNodeConfig{
			Name: hop.Name,
			Role: hop.Role,
			Port: hop.ListenPort,
			TLS:  hop.TLS,
		}

		// Configure upstream connection for leaf/bridge
		if hop.Role == LeafNodeRoleLeaf || hop.Role == LeafNodeRoleBridge {
			config.Remotes = []LeafRemoteConfig{
				{
					URLs:        hop.UpstreamURLs,
					Credentials: hop.Credentials,
					Hub:         true, // Always hub mode for chain
					TLS:         hop.TLS,
				},
			}
		}

		configs[i] = config
	}

	return configs, nil
}
