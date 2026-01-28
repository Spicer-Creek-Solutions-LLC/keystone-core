// Package nats provides NATS messaging infrastructure for Keystone Core
package nats

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// GatewayConnectionState represents the state of a gateway connection
type GatewayConnectionState int

const (
	// GatewayConnectionStateDisconnected means no connection to gateway
	GatewayConnectionStateDisconnected GatewayConnectionState = iota
	// GatewayConnectionStateConnecting means gateway connection in progress
	GatewayConnectionStateConnecting
	// GatewayConnectionStateConnected means successfully connected to gateway
	GatewayConnectionStateConnected
	// GatewayConnectionStateReconnecting means attempting to reconnect to gateway
	GatewayConnectionStateReconnecting
	// GatewayConnectionStateFailed means gateway connection failed
	GatewayConnectionStateFailed
)

// String returns string representation of GatewayConnectionState
func (s GatewayConnectionState) String() string {
	switch s {
	case GatewayConnectionStateDisconnected:
		return "disconnected"
	case GatewayConnectionStateConnecting:
		return "connecting"
	case GatewayConnectionStateConnected:
		return "connected"
	case GatewayConnectionStateReconnecting:
		return "reconnecting"
	case GatewayConnectionStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// GatewayMode defines the operating mode for gateways
type GatewayMode int

const (
	// GatewayModeOptimistic uses optimistic interest propagation (default)
	GatewayModeOptimistic GatewayMode = iota
	// GatewayModeInterestOnly only propagates explicitly expressed interest
	GatewayModeInterestOnly
)

// String returns string representation of GatewayMode
func (m GatewayMode) String() string {
	switch m {
	case GatewayModeOptimistic:
		return "optimistic"
	case GatewayModeInterestOnly:
		return "interest-only"
	default:
		return "unknown"
	}
}

// ParseGatewayMode parses a string to GatewayMode
func ParseGatewayMode(s string) (GatewayMode, error) {
	switch s {
	case "optimistic", "":
		return GatewayModeOptimistic, nil
	case "interest-only", "interest_only", "interestonly":
		return GatewayModeInterestOnly, nil
	default:
		return GatewayModeOptimistic, fmt.Errorf("unknown gateway mode: %s", s)
	}
}

// GatewayConfig configures NATS gateway for supercluster support
type GatewayConfig struct {
	// Name is the name of this cluster (required)
	Name string

	// Listen is the address to accept gateway connections on
	Listen string

	// Port is the port to accept gateway connections on (default: 7222)
	Port int

	// AdvertiseHost is the host to advertise to other gateways
	AdvertiseHost string

	// AdvertisePort is the port to advertise to other gateways
	AdvertisePort int

	// Gateways are the remote cluster connections
	Gateways []GatewayRemoteConfig

	// TLS configuration for gateway connections
	TLS *GatewayTLSConfig

	// Authorization for gateway connections
	Auth *GatewayAuthConfig

	// RejectUnknown rejects connections from unknown gateways
	RejectUnknown bool

	// ConnectRetries is the number of connection retry attempts (default: 5)
	ConnectRetries int

	// ReconnectInterval is how long to wait before reconnecting
	ReconnectInterval time.Duration

	// SendQueueLimit is the internal queue limit for sending messages
	SendQueueLimit int

	// DefaultMode is the default gateway mode for outbound connections
	DefaultMode GatewayMode
}

// DefaultGatewayConfig returns a GatewayConfig with sensible defaults
func DefaultGatewayConfig() *GatewayConfig {
	return &GatewayConfig{
		Port:              7222,
		ConnectRetries:    5,
		ReconnectInterval: 2 * time.Second,
		SendQueueLimit:    16384,
		DefaultMode:       GatewayModeOptimistic,
		RejectUnknown:     false,
	}
}

// Validate validates the gateway configuration
func (c *GatewayConfig) Validate() error {
	if c.Name == "" {
		return errors.New("gateway cluster name is required")
	}

	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("invalid gateway port: %d", c.Port)
	}

	if c.AdvertisePort < 0 || c.AdvertisePort > 65535 {
		return fmt.Errorf("invalid advertise port: %d", c.AdvertisePort)
	}

	if c.ConnectRetries < 0 {
		return fmt.Errorf("connect retries cannot be negative: %d", c.ConnectRetries)
	}

	if c.ReconnectInterval < 0 {
		return fmt.Errorf("reconnect interval cannot be negative: %v", c.ReconnectInterval)
	}

	if c.SendQueueLimit < 0 {
		return fmt.Errorf("send queue limit cannot be negative: %d", c.SendQueueLimit)
	}

	// Validate each remote gateway
	for i, gw := range c.Gateways {
		if err := gw.Validate(); err != nil {
			return fmt.Errorf("gateway[%d]: %w", i, err)
		}
	}

	// Validate TLS if provided
	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return fmt.Errorf("tls: %w", err)
		}
	}

	return nil
}

// GatewayRemoteConfig configures a connection to a remote cluster
type GatewayRemoteConfig struct {
	// Name is the name of the remote cluster (required)
	Name string

	// URLs are the gateway URLs for the remote cluster
	URLs []string

	// TLS configuration for this remote
	TLS *GatewayTLSConfig

	// Mode is the gateway mode for this connection
	Mode GatewayMode

	// ConnectRetries overrides the default connect retries for this gateway
	ConnectRetries int
}

// Validate validates the remote gateway configuration
func (c *GatewayRemoteConfig) Validate() error {
	if c.Name == "" {
		return errors.New("remote gateway name is required")
	}

	if len(c.URLs) == 0 {
		return errors.New("at least one remote URL is required")
	}

	// Validate URLs
	for i, rawURL := range c.URLs {
		parsed, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("url[%d]: invalid URL %q: %w", i, rawURL, err)
		}

		// Validate scheme
		switch parsed.Scheme {
		case "nats", "tls", "":
			// Valid schemes
		default:
			return fmt.Errorf("url[%d]: unsupported scheme %q", i, parsed.Scheme)
		}

		// Validate host
		if parsed.Hostname() == "" {
			return fmt.Errorf("url[%d]: missing host in URL %q", i, rawURL)
		}
	}

	if c.ConnectRetries < 0 {
		return fmt.Errorf("connect retries cannot be negative: %d", c.ConnectRetries)
	}

	// Validate TLS if provided
	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return fmt.Errorf("tls: %w", err)
		}
	}

	return nil
}

// GatewayTLSConfig configures TLS for gateway connections
type GatewayTLSConfig struct {
	// CertFile is the certificate file path
	CertFile string

	// KeyFile is the key file path
	KeyFile string

	// CaFile is the CA certificate file path
	CaFile string

	// InsecureSkipVerify skips TLS verification (not recommended)
	InsecureSkipVerify bool

	// MinVersion is the minimum TLS version (e.g., "1.2", "1.3")
	MinVersion string

	// CipherSuites are the allowed cipher suites
	CipherSuites []string

	// CurvePreferences are the allowed curves
	CurvePreferences []string

	// Timeout is the TLS handshake timeout
	Timeout time.Duration
}

// Validate validates the TLS configuration
func (c *GatewayTLSConfig) Validate() error {
	// If cert is provided, key must also be provided
	if c.CertFile != "" && c.KeyFile == "" {
		return errors.New("key file required when cert file is provided")
	}
	if c.KeyFile != "" && c.CertFile == "" {
		return errors.New("cert file required when key file is provided")
	}

	// Validate TLS version if provided
	if c.MinVersion != "" {
		switch c.MinVersion {
		case "1.2", "1.3":
			// Valid versions
		default:
			return fmt.Errorf("invalid minimum TLS version: %s", c.MinVersion)
		}
	}

	if c.Timeout < 0 {
		return fmt.Errorf("TLS timeout cannot be negative: %v", c.Timeout)
	}

	return nil
}

// ToTLSConfig converts to a standard tls.Config
func (c *GatewayTLSConfig) ToTLSConfig() (*tls.Config, error) {
	if c == nil {
		return nil, nil
	}

	tlsConfig := &tls.Config{ // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.bypass-tls-verification.bypass-tls-verification -- InsecureSkipVerify only allowed with KSCORE_ALLOW_INSECURE_TLS=1 for dev/test
		MinVersion: tls.VersionTLS12,
	}

	// InsecureSkipVerify - blocked by default unless KSCORE_ALLOW_INSECURE_TLS=1 is set
	if c.InsecureSkipVerify {
		if os.Getenv("KSCORE_ALLOW_INSECURE_TLS") != "1" {
			return nil, fmt.Errorf("nats gateway: insecure_skip_verify is not allowed in production (allows MITM attacks). " +
				"Set KSCORE_ALLOW_INSECURE_TLS=1 to override for development/testing only")
		}
		log.Printf("WARNING: NATS Gateway TLS InsecureSkipVerify is enabled - this allows man-in-the-middle attacks")
		tlsConfig.InsecureSkipVerify = true // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.bypass-tls-verification.bypass-tls-verification -- gated by KSCORE_ALLOW_INSECURE_TLS
	}

	// Set minimum version
	if c.MinVersion != "" {
		switch c.MinVersion {
		case "1.2":
			tlsConfig.MinVersion = tls.VersionTLS12
		case "1.3":
			tlsConfig.MinVersion = tls.VersionTLS13
		}
	}

	// Load certificate if provided
	if c.CertFile != "" && c.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// GatewayAuthConfig configures authentication for gateway connections
type GatewayAuthConfig struct {
	// User is the username for authentication
	User string

	// Password is the password for authentication
	Password string

	// Token is the token for authentication
	Token string

	// NKeyFile is the path to the NKey file
	NKeyFile string

	// CredentialsFile is the path to the credentials file
	CredentialsFile string
}

// Validate validates the auth configuration
func (c *GatewayAuthConfig) Validate() error {
	// Check for conflicting auth methods
	methodCount := 0
	if c.User != "" || c.Password != "" {
		methodCount++
	}
	if c.Token != "" {
		methodCount++
	}
	if c.NKeyFile != "" {
		methodCount++
	}
	if c.CredentialsFile != "" {
		methodCount++
	}

	if methodCount > 1 {
		return errors.New("multiple authentication methods specified; use only one")
	}

	// Validate user/password pair
	if c.User != "" && c.Password == "" {
		return errors.New("password required when username is provided")
	}
	if c.Password != "" && c.User == "" {
		return errors.New("username required when password is provided")
	}

	return nil
}

// GatewayConnection represents a connection to a remote gateway cluster
type GatewayConnection struct {
	// Name is the name of the remote cluster
	Name string

	// state is the current connection state
	state atomic.Int32

	// URLs are the gateway URLs
	URLs []string

	// connectedAt is when the connection was established
	connectedAt time.Time

	// lastError is the last error encountered
	lastError error

	// connectAttempts is the number of connection attempts
	connectAttempts atomic.Int64

	// messagesReceived counts received messages
	messagesReceived atomic.Int64

	// messagesSent counts sent messages
	messagesSent atomic.Int64

	// bytesReceived counts received bytes
	bytesReceived atomic.Int64

	// bytesSent counts sent bytes
	bytesSent atomic.Int64

	// latency is the last measured RTT
	latency time.Duration

	// mu protects mutable fields
	mu sync.RWMutex
}

// State returns the current connection state
func (c *GatewayConnection) State() GatewayConnectionState {
	return GatewayConnectionState(c.state.Load())
}

// IsConnected returns true if the gateway is connected
func (c *GatewayConnection) IsConnected() bool {
	return c.State() == GatewayConnectionStateConnected
}

// ConnectedAt returns when the connection was established
func (c *GatewayConnection) ConnectedAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connectedAt
}

// LastError returns the last error encountered
func (c *GatewayConnection) LastError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastError
}

// ConnectAttempts returns the number of connection attempts
func (c *GatewayConnection) ConnectAttempts() int64 {
	return c.connectAttempts.Load()
}

// Latency returns the last measured RTT
func (c *GatewayConnection) Latency() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latency
}

// Stats returns connection statistics
func (c *GatewayConnection) Stats() GatewayConnectionStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return GatewayConnectionStats{
		MessagesReceived: c.messagesReceived.Load(),
		MessagesSent:     c.messagesSent.Load(),
		BytesReceived:    c.bytesReceived.Load(),
		BytesSent:        c.bytesSent.Load(),
		ConnectAttempts:  c.connectAttempts.Load(),
		Latency:          c.latency,
	}
}

// GatewayConnectionStats contains statistics for a gateway connection
type GatewayConnectionStats struct {
	MessagesReceived int64
	MessagesSent     int64
	BytesReceived    int64
	BytesSent        int64
	ConnectAttempts  int64
	Latency          time.Duration
}

// GatewayManager manages NATS gateway connections for supercluster
type GatewayManager struct {
	config *GatewayConfig

	// connections holds connections to remote clusters
	connections map[string]*GatewayConnection

	// server is the embedded NATS server (if running)
	server *server.Server

	// client is the NATS client connection
	client *nats.Conn

	// running indicates if the manager is running
	running atomic.Bool

	// ctx is the context for the manager
	ctx context.Context

	// cancel cancels the context
	cancel context.CancelFunc

	// mu protects mutable fields
	mu sync.RWMutex

	// onConnect is called when a gateway connects
	onConnect func(name string)

	// onDisconnect is called when a gateway disconnects
	onDisconnect func(name string, err error)

	// onError is called when an error occurs
	onError func(name string, err error)
}

// NewGatewayManager creates a new gateway manager
func NewGatewayManager(config *GatewayConfig) (*GatewayManager, error) {
	if config == nil {
		config = DefaultGatewayConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid gateway config: %w", err)
	}

	return &GatewayManager{
		config:      config,
		connections: make(map[string]*GatewayConnection),
	}, nil
}

// Start starts the gateway manager
func (m *GatewayManager) Start(ctx context.Context) error {
	if m.running.Load() {
		return errors.New("gateway manager already running")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	// Initialize connections for all configured gateways
	m.mu.Lock()
	for _, gw := range m.config.Gateways {
		m.connections[gw.Name] = &GatewayConnection{
			Name: gw.Name,
			URLs: gw.URLs,
		}
	}
	m.mu.Unlock()

	m.running.Store(true)
	return nil
}

// Stop stops the gateway manager
func (m *GatewayManager) Stop() error {
	if !m.running.Load() {
		return nil
	}

	if m.cancel != nil {
		m.cancel()
	}

	// Close client connection
	m.mu.Lock()
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}
	m.mu.Unlock()

	m.running.Store(false)
	return nil
}

// IsRunning returns true if the manager is running
func (m *GatewayManager) IsRunning() bool {
	return m.running.Load()
}

// GetConnection returns the connection to a remote cluster
func (m *GatewayManager) GetConnection(name string) *GatewayConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connections[name]
}

// GetConnections returns all gateway connections
func (m *GatewayManager) GetConnections() map[string]*GatewayConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*GatewayConnection, len(m.connections))
	for k, v := range m.connections {
		result[k] = v
	}
	return result
}

// GetConnectedGateways returns the names of all connected gateways
func (m *GatewayManager) GetConnectedGateways() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var connected []string
	for name, conn := range m.connections {
		if conn.IsConnected() {
			connected = append(connected, name)
		}
	}
	return connected
}

// ConnectionCount returns the number of gateway connections
func (m *GatewayManager) ConnectionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections)
}

// ConnectedCount returns the number of connected gateways
func (m *GatewayManager) ConnectedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, conn := range m.connections {
		if conn.IsConnected() {
			count++
		}
	}
	return count
}

// SetConnectCallback sets the callback for gateway connections
func (m *GatewayManager) SetConnectCallback(cb func(name string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onConnect = cb
}

// SetDisconnectCallback sets the callback for gateway disconnections
func (m *GatewayManager) SetDisconnectCallback(cb func(name string, err error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDisconnect = cb
}

// SetErrorCallback sets the callback for gateway errors
func (m *GatewayManager) SetErrorCallback(cb func(name string, err error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onError = cb
}

// Config returns the gateway configuration
func (m *GatewayManager) Config() *GatewayConfig {
	return m.config
}

// GetStats returns statistics for all gateway connections
func (m *GatewayManager) GetStats() *GatewayManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &GatewayManagerStats{
		ClusterName:      m.config.Name,
		TotalGateways:    len(m.connections),
		ConnectionStats:  make(map[string]GatewayConnectionStats),
		ConnectionStates: make(map[string]GatewayConnectionState),
	}

	for name, conn := range m.connections {
		if conn.IsConnected() {
			stats.ConnectedGateways++
		}
		stats.ConnectionStats[name] = conn.Stats()
		stats.ConnectionStates[name] = conn.State()
	}

	return stats
}

// GatewayManagerStats contains statistics for the gateway manager
type GatewayManagerStats struct {
	ClusterName       string
	TotalGateways     int
	ConnectedGateways int
	ConnectionStats   map[string]GatewayConnectionStats
	ConnectionStates  map[string]GatewayConnectionState
}

// ConfigureNATSServer configures a NATS server for gateway support
func (m *GatewayManager) ConfigureNATSServer(opts *server.Options) error {
	if opts == nil {
		return errors.New("server options cannot be nil")
	}

	// Set gateway name
	opts.Gateway.Name = m.config.Name

	// Set gateway listen address
	if m.config.Listen != "" {
		opts.Gateway.Host = m.config.Listen
	}
	if m.config.Port > 0 {
		opts.Gateway.Port = m.config.Port
	}

	// Set advertise address
	if m.config.AdvertiseHost != "" {
		opts.Gateway.Advertise = m.config.AdvertiseHost
	}

	// Set reject unknown
	opts.Gateway.RejectUnknown = m.config.RejectUnknown

	// Set connect retries
	if m.config.ConnectRetries > 0 {
		opts.Gateway.ConnectRetries = m.config.ConnectRetries
	}

	// Configure TLS
	if m.config.TLS != nil {
		tlsConfig, err := m.config.TLS.ToTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to configure TLS: %w", err)
		}
		opts.Gateway.TLSConfig = tlsConfig

		if m.config.TLS.Timeout > 0 {
			opts.Gateway.TLSTimeout = float64(m.config.TLS.Timeout.Seconds())
		}
	}

	// Configure remote gateways
	for _, gw := range m.config.Gateways {
		remoteGateway := &server.RemoteGatewayOpts{
			Name: gw.Name,
		}

		// Parse URLs
		for _, rawURL := range gw.URLs {
			parsed, err := url.Parse(rawURL)
			if err != nil {
				return fmt.Errorf("invalid gateway URL %q: %w", rawURL, err)
			}

			gwURL := &url.URL{
				Scheme: parsed.Scheme,
				Host:   parsed.Host,
			}
			remoteGateway.URLs = append(remoteGateway.URLs, gwURL)
		}

		// Configure TLS for this gateway
		if gw.TLS != nil {
			tlsConfig, err := gw.TLS.ToTLSConfig()
			if err != nil {
				return fmt.Errorf("gateway %s TLS: %w", gw.Name, err)
			}
			remoteGateway.TLSConfig = tlsConfig
		}

		opts.Gateway.Gateways = append(opts.Gateway.Gateways, remoteGateway)
	}

	return nil
}

// BuildGatewayURLs builds gateway URLs from the configuration
func (m *GatewayManager) BuildGatewayURLs() []string {
	var urls []string

	for _, gw := range m.config.Gateways {
		urls = append(urls, gw.URLs...)
	}

	return urls
}

// SuperclusterConfig configures a complete NATS supercluster
type SuperclusterConfig struct {
	// LocalCluster is the configuration for this cluster
	LocalCluster *GatewayConfig

	// RemoteClusters are the remote cluster configurations
	RemoteClusters []GatewayRemoteConfig

	// EnableAutoDiscovery enables automatic discovery of new clusters
	EnableAutoDiscovery bool

	// DiscoveryInterval is how often to discover new clusters
	DiscoveryInterval time.Duration

	// PreferLocalCluster prefers routing to local cluster
	PreferLocalCluster bool

	// CrossClusterTimeout is the timeout for cross-cluster operations
	CrossClusterTimeout time.Duration

	// FailoverEnabled enables automatic failover to remote clusters
	FailoverEnabled bool

	// FailoverTimeout is how long to wait before failing over
	FailoverTimeout time.Duration
}

// DefaultSuperclusterConfig returns a SuperclusterConfig with sensible defaults
func DefaultSuperclusterConfig() *SuperclusterConfig {
	return &SuperclusterConfig{
		LocalCluster:        DefaultGatewayConfig(),
		EnableAutoDiscovery: false,
		DiscoveryInterval:   30 * time.Second,
		PreferLocalCluster:  true,
		CrossClusterTimeout: 10 * time.Second,
		FailoverEnabled:     true,
		FailoverTimeout:     5 * time.Second,
	}
}

// Validate validates the supercluster configuration
func (c *SuperclusterConfig) Validate() error {
	if c.LocalCluster == nil {
		return errors.New("local cluster configuration is required")
	}

	if err := c.LocalCluster.Validate(); err != nil {
		return fmt.Errorf("local cluster: %w", err)
	}

	// Validate remote clusters
	for i, remote := range c.RemoteClusters {
		if err := remote.Validate(); err != nil {
			return fmt.Errorf("remote cluster[%d]: %w", i, err)
		}
	}

	if c.DiscoveryInterval < 0 {
		return fmt.Errorf("discovery interval cannot be negative: %v", c.DiscoveryInterval)
	}

	if c.CrossClusterTimeout < 0 {
		return fmt.Errorf("cross-cluster timeout cannot be negative: %v", c.CrossClusterTimeout)
	}

	if c.FailoverTimeout < 0 {
		return fmt.Errorf("failover timeout cannot be negative: %v", c.FailoverTimeout)
	}

	return nil
}

// ClusterRoute represents a route to a cluster
type ClusterRoute struct {
	// ClusterName is the name of the target cluster
	ClusterName string

	// IsLocal indicates if this is the local cluster
	IsLocal bool

	// Latency is the estimated latency to this cluster
	Latency time.Duration

	// Priority is the routing priority (lower is higher priority)
	Priority int

	// Available indicates if the cluster is available
	Available bool
}

// SubjectRouter routes subjects to appropriate clusters
type SubjectRouter struct {
	// localCluster is the name of the local cluster
	localCluster string

	// routes maps cluster names to routes
	routes map[string]*ClusterRoute

	// subjectPrefixes maps subject prefixes to preferred clusters
	subjectPrefixes map[string]string

	// preferLocal prefers local cluster routing
	preferLocal bool

	// mu protects mutable fields
	mu sync.RWMutex
}

// NewSubjectRouter creates a new subject router
func NewSubjectRouter(localCluster string, preferLocal bool) *SubjectRouter {
	return &SubjectRouter{
		localCluster:    localCluster,
		routes:          make(map[string]*ClusterRoute),
		subjectPrefixes: make(map[string]string),
		preferLocal:     preferLocal,
	}
}

// AddRoute adds a route to a cluster
func (r *SubjectRouter) AddRoute(route *ClusterRoute) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes[route.ClusterName] = route
}

// RemoveRoute removes a route to a cluster
func (r *SubjectRouter) RemoveRoute(clusterName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.routes, clusterName)
}

// GetRoute returns the route for a cluster
func (r *SubjectRouter) GetRoute(clusterName string) *ClusterRoute {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.routes[clusterName]
}

// AddSubjectPrefix adds a subject prefix routing rule
func (r *SubjectRouter) AddSubjectPrefix(prefix, clusterName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subjectPrefixes[prefix] = clusterName
}

// RemoveSubjectPrefix removes a subject prefix routing rule
func (r *SubjectRouter) RemoveSubjectPrefix(prefix string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.subjectPrefixes, prefix)
}

// RouteSubject determines which cluster should handle a subject
func (r *SubjectRouter) RouteSubject(subject string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect prefixes and sort by length (longest first) for most-specific matching
	prefixes := make([]string, 0, len(r.subjectPrefixes))
	for prefix := range r.subjectPrefixes {
		prefixes = append(prefixes, prefix)
	}
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i]) > len(prefixes[j])
	})

	// Check for explicit prefix routing (longest prefix first)
	for _, prefix := range prefixes {
		if len(subject) >= len(prefix) && subject[:len(prefix)] == prefix {
			cluster := r.subjectPrefixes[prefix]
			if route, ok := r.routes[cluster]; ok && route.Available {
				return cluster
			}
		}
	}

	// If prefer local and local is available, use local
	if r.preferLocal {
		if route, ok := r.routes[r.localCluster]; ok && route.Available {
			return r.localCluster
		}
	}

	// Find the best available route by priority and latency
	var bestRoute *ClusterRoute
	for _, route := range r.routes {
		if !route.Available {
			continue
		}
		if bestRoute == nil {
			bestRoute = route
			continue
		}
		// Prefer lower priority (higher priority value = lower priority)
		if route.Priority < bestRoute.Priority {
			bestRoute = route
			continue
		}
		// If same priority, prefer lower latency
		if route.Priority == bestRoute.Priority && route.Latency < bestRoute.Latency {
			bestRoute = route
		}
	}

	if bestRoute != nil {
		return bestRoute.ClusterName
	}

	// Fallback to local cluster
	return r.localCluster
}

// GetAvailableClusters returns all available clusters
func (r *SubjectRouter) GetAvailableClusters() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var available []string
	for name, route := range r.routes {
		if route.Available {
			available = append(available, name)
		}
	}
	return available
}

// UpdateClusterAvailability updates the availability of a cluster
func (r *SubjectRouter) UpdateClusterAvailability(clusterName string, available bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if route, ok := r.routes[clusterName]; ok {
		route.Available = available
	}
}

// UpdateClusterLatency updates the latency of a cluster
func (r *SubjectRouter) UpdateClusterLatency(clusterName string, latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if route, ok := r.routes[clusterName]; ok {
		route.Latency = latency
	}
}

// ============================================================================
// Gateway Connection Manager - T5.2
// ============================================================================

// GatewayHealthConfig configures gateway health monitoring
type GatewayHealthConfig struct {
	// CheckInterval is how often to check gateway health
	CheckInterval time.Duration

	// Timeout is the timeout for health checks
	Timeout time.Duration

	// HealthyThreshold is the number of consecutive healthy checks required
	HealthyThreshold int

	// UnhealthyThreshold is the number of consecutive failed checks to mark unhealthy
	UnhealthyThreshold int

	// PingEnabled enables ping-based latency measurement
	PingEnabled bool

	// PingInterval is how often to measure latency
	PingInterval time.Duration
}

// DefaultGatewayHealthConfig returns a GatewayHealthConfig with sensible defaults
func DefaultGatewayHealthConfig() *GatewayHealthConfig {
	return &GatewayHealthConfig{
		CheckInterval:      10 * time.Second,
		Timeout:            5 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
		PingEnabled:        true,
		PingInterval:       30 * time.Second,
	}
}

// GatewayHealth represents the health status of a gateway
type GatewayHealth struct {
	// ClusterName is the name of the remote cluster
	ClusterName string

	// Healthy indicates if the gateway is healthy
	Healthy bool

	// LastCheck is when the last health check occurred
	LastCheck time.Time

	// LastHealthy is when the gateway was last healthy
	LastHealthy time.Time

	// LastUnhealthy is when the gateway was last unhealthy
	LastUnhealthy time.Time

	// ConsecutiveSuccesses is the number of consecutive successful checks
	ConsecutiveSuccesses int

	// ConsecutiveFailures is the number of consecutive failed checks
	ConsecutiveFailures int

	// Latency is the current measured latency
	Latency time.Duration

	// Error is the last error encountered
	Error error
}

// GatewayHealthMonitor monitors the health of gateway connections
type GatewayHealthMonitor struct {
	config  *GatewayHealthConfig
	manager *GatewayManager

	// health tracks health status per gateway
	health map[string]*GatewayHealth

	// running indicates if the monitor is running
	running atomic.Bool

	// ctx is the context for the monitor
	ctx context.Context

	// cancel cancels the context
	cancel context.CancelFunc

	// mu protects mutable fields
	mu sync.RWMutex

	// onHealthChange is called when gateway health changes
	onHealthChange func(name string, healthy bool)
}

// NewGatewayHealthMonitor creates a new gateway health monitor
func NewGatewayHealthMonitor(manager *GatewayManager, config *GatewayHealthConfig) *GatewayHealthMonitor {
	if config == nil {
		config = DefaultGatewayHealthConfig()
	}

	return &GatewayHealthMonitor{
		config:  config,
		manager: manager,
		health:  make(map[string]*GatewayHealth),
	}
}

// Start starts the health monitor
func (m *GatewayHealthMonitor) Start(ctx context.Context) error {
	if m.running.Load() {
		return errors.New("health monitor already running")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	// Initialize health status for all gateways
	m.mu.Lock()
	for name := range m.manager.GetConnections() {
		m.health[name] = &GatewayHealth{
			ClusterName: name,
			Healthy:     false,
		}
	}
	m.mu.Unlock()

	m.running.Store(true)

	// Start health check loop
	go m.healthCheckLoop()

	// Start ping loop if enabled
	if m.config.PingEnabled {
		go m.pingLoop()
	}

	return nil
}

// Stop stops the health monitor
func (m *GatewayHealthMonitor) Stop() error {
	if !m.running.Load() {
		return nil
	}

	if m.cancel != nil {
		m.cancel()
	}

	m.running.Store(false)
	return nil
}

// IsRunning returns true if the monitor is running
func (m *GatewayHealthMonitor) IsRunning() bool {
	return m.running.Load()
}

// GetHealth returns the health status of a gateway
func (m *GatewayHealthMonitor) GetHealth(name string) *GatewayHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.health[name]
}

// GetAllHealth returns health status for all gateways
func (m *GatewayHealthMonitor) GetAllHealth() map[string]*GatewayHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*GatewayHealth, len(m.health))
	for k, v := range m.health {
		result[k] = v
	}
	return result
}

// GetHealthyGateways returns names of all healthy gateways
func (m *GatewayHealthMonitor) GetHealthyGateways() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var healthy []string
	for name, health := range m.health {
		if health.Healthy {
			healthy = append(healthy, name)
		}
	}
	return healthy
}

// SetHealthChangeCallback sets the callback for health changes
func (m *GatewayHealthMonitor) SetHealthChangeCallback(cb func(name string, healthy bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onHealthChange = cb
}

// healthCheckLoop runs periodic health checks
func (m *GatewayHealthMonitor) healthCheckLoop() {
	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkAllGateways()
		}
	}
}

// pingLoop runs periodic latency measurements
func (m *GatewayHealthMonitor) pingLoop() {
	ticker := time.NewTicker(m.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.pingAllGateways()
		}
	}
}

// checkAllGateways checks health of all gateways
func (m *GatewayHealthMonitor) checkAllGateways() {
	connections := m.manager.GetConnections()

	for name, conn := range connections {
		healthy := conn.IsConnected()
		m.updateHealth(name, healthy, nil)
	}
}

// pingAllGateways measures latency to all gateways
func (m *GatewayHealthMonitor) pingAllGateways() {
	connections := m.manager.GetConnections()

	for name, conn := range connections {
		if conn.IsConnected() {
			// Use the stored latency from the connection
			latency := conn.Latency()
			m.updateLatency(name, latency)
		}
	}
}

// updateHealth updates the health status of a gateway
func (m *GatewayHealthMonitor) updateHealth(name string, healthy bool, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	health, ok := m.health[name]
	if !ok {
		health = &GatewayHealth{
			ClusterName: name,
		}
		m.health[name] = health
	}

	now := time.Now()
	health.LastCheck = now
	health.Error = err

	wasHealthy := health.Healthy

	if healthy {
		health.ConsecutiveSuccesses++
		health.ConsecutiveFailures = 0
		health.LastHealthy = now

		if health.ConsecutiveSuccesses >= m.config.HealthyThreshold {
			health.Healthy = true
		}
	} else {
		health.ConsecutiveFailures++
		health.ConsecutiveSuccesses = 0
		health.LastUnhealthy = now

		if health.ConsecutiveFailures >= m.config.UnhealthyThreshold {
			health.Healthy = false
		}
	}

	// Call callback if health changed
	if wasHealthy != health.Healthy && m.onHealthChange != nil {
		go m.onHealthChange(name, health.Healthy)
	}
}

// updateLatency updates the latency for a gateway
func (m *GatewayHealthMonitor) updateLatency(name string, latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if health, ok := m.health[name]; ok {
		health.Latency = latency
	}
}

// ============================================================================
// Gateway Dynamic Management
// ============================================================================

// AddGateway dynamically adds a gateway connection
func (m *GatewayManager) AddGateway(remote GatewayRemoteConfig) error {
	if err := remote.Validate(); err != nil {
		return fmt.Errorf("invalid remote config: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if gateway already exists
	if _, exists := m.connections[remote.Name]; exists {
		return fmt.Errorf("gateway %q already exists", remote.Name)
	}

	// Add to config
	m.config.Gateways = append(m.config.Gateways, remote)

	// Create connection record
	m.connections[remote.Name] = &GatewayConnection{
		Name: remote.Name,
		URLs: remote.URLs,
	}

	return nil
}

// RemoveGateway dynamically removes a gateway connection
func (m *GatewayManager) RemoveGateway(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if gateway exists
	if _, exists := m.connections[name]; !exists {
		return fmt.Errorf("gateway %q not found", name)
	}

	// Remove from connections
	delete(m.connections, name)

	// Remove from config
	for i, gw := range m.config.Gateways {
		if gw.Name == name {
			m.config.Gateways = append(m.config.Gateways[:i], m.config.Gateways[i+1:]...)
			break
		}
	}

	return nil
}

// SetServer sets the embedded NATS server for the manager
func (m *GatewayManager) SetServer(s *server.Server) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.server = s
}

// GetServer returns the embedded NATS server
func (m *GatewayManager) GetServer() *server.Server {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.server
}

// SetClient sets the NATS client connection
func (m *GatewayManager) SetClient(c *nats.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.client = c
}

// GetClient returns the NATS client connection
func (m *GatewayManager) GetClient() *nats.Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.client
}

// UpdateGatewayStatusFromServer updates gateway status from the NATS server
func (m *GatewayManager) UpdateGatewayStatusFromServer() error {
	m.mu.RLock()
	s := m.server
	m.mu.RUnlock()

	if s == nil {
		return errors.New("no server configured")
	}

	// Get gateway status from server
	gatewayz, err := s.Gatewayz(nil)
	if err != nil {
		return fmt.Errorf("failed to get gateway status: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Update connection states (OutboundGateways is map[name]*RemoteGatewayz)
	for name, outbound := range gatewayz.OutboundGateways {
		conn, exists := m.connections[name]
		if !exists {
			continue
		}

		if outbound.IsConfigured {
			if len(outbound.Accounts) > 0 {
				conn.state.Store(int32(GatewayConnectionStateConnected))
				conn.mu.Lock()
				conn.connectedAt = time.Now()
				conn.mu.Unlock()
			} else {
				conn.state.Store(int32(GatewayConnectionStateConnecting))
			}
		}
	}

	return nil
}

// ============================================================================
// Cross-Cluster Agent Manager - T5.4
// ============================================================================

// CrossClusterAgentManager manages agents across clusters
type CrossClusterAgentManager struct {
	// localCluster is the name of the local cluster
	localCluster string

	// router is the subject router for cross-cluster routing
	router *SubjectRouter

	// gatewayManager is the gateway manager
	gatewayManager *GatewayManager

	// agentClusters maps agent IDs to their cluster
	agentClusters map[string]string

	// clusterAgents maps cluster names to agent IDs in that cluster
	clusterAgents map[string]map[string]bool

	// mu protects mutable fields
	mu sync.RWMutex

	// crossClusterTimeout is the timeout for cross-cluster operations
	crossClusterTimeout time.Duration

	// localityPreference indicates preference for local agents
	localityPreference bool
}

// NewCrossClusterAgentManager creates a new cross-cluster agent manager
func NewCrossClusterAgentManager(
	localCluster string,
	router *SubjectRouter,
	gatewayManager *GatewayManager,
	crossClusterTimeout time.Duration,
) *CrossClusterAgentManager {
	return &CrossClusterAgentManager{
		localCluster:        localCluster,
		router:              router,
		gatewayManager:      gatewayManager,
		agentClusters:       make(map[string]string),
		clusterAgents:       make(map[string]map[string]bool),
		crossClusterTimeout: crossClusterTimeout,
		localityPreference:  true,
	}
}

// RegisterAgent registers an agent with its cluster
func (m *CrossClusterAgentManager) RegisterAgent(agentID, clusterName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Remove from old cluster if exists
	if oldCluster, exists := m.agentClusters[agentID]; exists {
		if agents, ok := m.clusterAgents[oldCluster]; ok {
			delete(agents, agentID)
		}
	}

	// Add to new cluster
	m.agentClusters[agentID] = clusterName
	if _, ok := m.clusterAgents[clusterName]; !ok {
		m.clusterAgents[clusterName] = make(map[string]bool)
	}
	m.clusterAgents[clusterName][agentID] = true
}

// UnregisterAgent unregisters an agent
func (m *CrossClusterAgentManager) UnregisterAgent(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if clusterName, exists := m.agentClusters[agentID]; exists {
		if agents, ok := m.clusterAgents[clusterName]; ok {
			delete(agents, agentID)
		}
		delete(m.agentClusters, agentID)
	}
}

// GetAgentCluster returns the cluster an agent belongs to
func (m *CrossClusterAgentManager) GetAgentCluster(agentID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cluster, exists := m.agentClusters[agentID]
	return cluster, exists
}

// IsLocalAgent returns true if the agent is in the local cluster
func (m *CrossClusterAgentManager) IsLocalAgent(agentID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cluster, exists := m.agentClusters[agentID]
	return exists && cluster == m.localCluster
}

// GetLocalAgents returns all agents in the local cluster
func (m *CrossClusterAgentManager) GetLocalAgents() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var agents []string
	if localAgents, ok := m.clusterAgents[m.localCluster]; ok {
		for agentID := range localAgents {
			agents = append(agents, agentID)
		}
	}
	return agents
}

// GetAgentsInCluster returns all agents in a specific cluster
func (m *CrossClusterAgentManager) GetAgentsInCluster(clusterName string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var agents []string
	if clusterAgents, ok := m.clusterAgents[clusterName]; ok {
		for agentID := range clusterAgents {
			agents = append(agents, agentID)
		}
	}
	return agents
}

// GetAllAgents returns all agents across all clusters
func (m *CrossClusterAgentManager) GetAllAgents() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]string, len(m.agentClusters))
	for agentID, cluster := range m.agentClusters {
		result[agentID] = cluster
	}
	return result
}

// GetClusterStats returns statistics for each cluster
func (m *CrossClusterAgentManager) GetClusterStats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]int)
	for cluster, agents := range m.clusterAgents {
		stats[cluster] = len(agents)
	}
	return stats
}

// BuildAgentSubject builds the subject for an agent considering its cluster
func (m *CrossClusterAgentManager) BuildAgentSubject(agentID, subject string) string {
	cluster, exists := m.GetAgentCluster(agentID)
	if !exists {
		// Default to local cluster
		cluster = m.localCluster
	}

	// Format: kscore.{cluster}.agent.{id}.{subject}
	return fmt.Sprintf("kscore.%s.agent.%s.%s", cluster, agentID, subject)
}

// GetTimeoutForAgent returns the appropriate timeout for an agent operation
func (m *CrossClusterAgentManager) GetTimeoutForAgent(agentID string, baseTimeout time.Duration) time.Duration {
	if m.IsLocalAgent(agentID) {
		return baseTimeout
	}

	// Add cross-cluster timeout for remote agents
	return baseTimeout + m.crossClusterTimeout
}

// SetLocalityPreference sets whether to prefer local agents
func (m *CrossClusterAgentManager) SetLocalityPreference(prefer bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.localityPreference = prefer
}

// ============================================================================
// Supercluster Failover Manager - T5.5
// ============================================================================

// FailoverState represents the failover state
type FailoverState int

const (
	// FailoverStateNormal means no failover is active
	FailoverStateNormal FailoverState = iota
	// FailoverStateDetecting means failure detection in progress
	FailoverStateDetecting
	// FailoverStateFailingOver means failover is in progress
	FailoverStateFailingOver
	// FailoverStateFailedOver means failover is complete
	FailoverStateFailedOver
	// FailoverStateFailingBack means failback is in progress
	FailoverStateFailingBack
)

// String returns string representation of FailoverState
func (s FailoverState) String() string {
	switch s {
	case FailoverStateNormal:
		return "normal"
	case FailoverStateDetecting:
		return "detecting"
	case FailoverStateFailingOver:
		return "failing-over"
	case FailoverStateFailedOver:
		return "failed-over"
	case FailoverStateFailingBack:
		return "failing-back"
	default:
		return "unknown"
	}
}

// FailoverConfig configures supercluster failover
type FailoverConfig struct {
	// Enabled enables automatic failover
	Enabled bool

	// DetectionTimeout is how long to wait before declaring failure
	DetectionTimeout time.Duration

	// FailoverTimeout is how long to wait for failover to complete
	FailoverTimeout time.Duration

	// FailbackDelay is how long to wait before failing back
	FailbackDelay time.Duration

	// MinHealthyNodes is the minimum healthy nodes required in target cluster
	MinHealthyNodes int

	// PreferredFailoverCluster is the preferred cluster to fail over to
	PreferredFailoverCluster string
}

// DefaultFailoverConfig returns a FailoverConfig with sensible defaults
func DefaultFailoverConfig() *FailoverConfig {
	return &FailoverConfig{
		Enabled:          true,
		DetectionTimeout: 10 * time.Second,
		FailoverTimeout:  30 * time.Second,
		FailbackDelay:    60 * time.Second,
		MinHealthyNodes:  1,
	}
}

// FailoverManager manages supercluster failover
type FailoverManager struct {
	config        *FailoverConfig
	localCluster  string
	healthMonitor *GatewayHealthMonitor
	agentManager  *CrossClusterAgentManager
	router        *SubjectRouter

	// state is the current failover state
	state atomic.Int32

	// activeCluster is the currently active cluster
	activeCluster string

	// failedOverTo is the cluster we failed over to (if any)
	failedOverTo string

	// failoverTime is when failover occurred
	failoverTime time.Time

	// running indicates if the manager is running
	running atomic.Bool

	// ctx is the context for the manager
	ctx context.Context

	// cancel cancels the context
	cancel context.CancelFunc

	// mu protects mutable fields
	mu sync.RWMutex

	// onFailover is called when failover occurs
	onFailover func(from, to string)

	// onFailback is called when failback occurs
	onFailback func(from, to string)
}

// NewFailoverManager creates a new failover manager
func NewFailoverManager(
	config *FailoverConfig,
	localCluster string,
	healthMonitor *GatewayHealthMonitor,
	agentManager *CrossClusterAgentManager,
	router *SubjectRouter,
) *FailoverManager {
	if config == nil {
		config = DefaultFailoverConfig()
	}

	return &FailoverManager{
		config:        config,
		localCluster:  localCluster,
		healthMonitor: healthMonitor,
		agentManager:  agentManager,
		router:        router,
		activeCluster: localCluster,
	}
}

// Start starts the failover manager
func (m *FailoverManager) Start(ctx context.Context) error {
	if m.running.Load() {
		return errors.New("failover manager already running")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)
	m.running.Store(true)

	// Start monitoring loop
	go m.monitorLoop()

	return nil
}

// Stop stops the failover manager
func (m *FailoverManager) Stop() error {
	if !m.running.Load() {
		return nil
	}

	if m.cancel != nil {
		m.cancel()
	}

	m.running.Store(false)
	return nil
}

// IsRunning returns true if the manager is running
func (m *FailoverManager) IsRunning() bool {
	return m.running.Load()
}

// State returns the current failover state
func (m *FailoverManager) State() FailoverState {
	return FailoverState(m.state.Load())
}

// ActiveCluster returns the currently active cluster
func (m *FailoverManager) ActiveCluster() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeCluster
}

// IsFailedOver returns true if we're currently failed over
func (m *FailoverManager) IsFailedOver() bool {
	return m.State() == FailoverStateFailedOver
}

// FailedOverTo returns the cluster we failed over to (empty if not failed over)
func (m *FailoverManager) FailedOverTo() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.failedOverTo
}

// SetFailoverCallback sets the callback for failover events
func (m *FailoverManager) SetFailoverCallback(cb func(from, to string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onFailover = cb
}

// SetFailbackCallback sets the callback for failback events
func (m *FailoverManager) SetFailbackCallback(cb func(from, to string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onFailback = cb
}

// monitorLoop monitors cluster health and triggers failover/failback
func (m *FailoverManager) monitorLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkAndTriggerFailover()
		}
	}
}

// checkAndTriggerFailover checks if failover/failback is needed
func (m *FailoverManager) checkAndTriggerFailover() {
	if !m.config.Enabled {
		return
	}

	state := m.State()

	switch state {
	case FailoverStateNormal:
		// Check if local cluster is healthy
		if !m.isClusterHealthy(m.localCluster) {
			m.state.Store(int32(FailoverStateDetecting))
		}

	case FailoverStateDetecting:
		// Continue to check if failure persists
		if m.isClusterHealthy(m.localCluster) {
			// Recovered, go back to normal
			m.state.Store(int32(FailoverStateNormal))
		} else {
			// Trigger failover
			m.triggerFailover()
		}

	case FailoverStateFailedOver:
		// Check if original cluster is healthy again
		if m.isClusterHealthy(m.localCluster) {
			// Check if we've waited long enough
			m.mu.RLock()
			timeSinceFailover := time.Since(m.failoverTime)
			m.mu.RUnlock()

			if timeSinceFailover >= m.config.FailbackDelay {
				m.triggerFailback()
			}
		}
	}
}

// isClusterHealthy checks if a cluster is healthy
func (m *FailoverManager) isClusterHealthy(clusterName string) bool {
	if clusterName == m.localCluster {
		// For local cluster, check if we can reach any gateway
		healthyCount := len(m.healthMonitor.GetHealthyGateways())
		return healthyCount > 0 || m.agentManager != nil
	}

	// For remote clusters, check gateway health
	health := m.healthMonitor.GetHealth(clusterName)
	return health != nil && health.Healthy
}

// triggerFailover triggers failover to a healthy cluster
func (m *FailoverManager) triggerFailover() {
	m.state.Store(int32(FailoverStateFailingOver))

	// Find a healthy cluster to fail over to
	targetCluster := m.findFailoverTarget()
	if targetCluster == "" {
		// No healthy cluster found, stay in detecting state
		m.state.Store(int32(FailoverStateDetecting))
		return
	}

	m.mu.Lock()
	oldCluster := m.activeCluster
	m.activeCluster = targetCluster
	m.failedOverTo = targetCluster
	m.failoverTime = time.Now()
	callback := m.onFailover
	m.mu.Unlock()

	// Update router to prefer new cluster
	m.router.UpdateClusterAvailability(m.localCluster, false)
	m.router.UpdateClusterAvailability(targetCluster, true)

	m.state.Store(int32(FailoverStateFailedOver))

	// Call callback
	if callback != nil {
		go callback(oldCluster, targetCluster)
	}
}

// triggerFailback triggers failback to the local cluster
func (m *FailoverManager) triggerFailback() {
	m.state.Store(int32(FailoverStateFailingBack))

	m.mu.Lock()
	oldCluster := m.activeCluster
	m.activeCluster = m.localCluster
	m.failedOverTo = ""
	callback := m.onFailback
	m.mu.Unlock()

	// Update router to prefer local cluster again
	m.router.UpdateClusterAvailability(m.localCluster, true)

	m.state.Store(int32(FailoverStateNormal))

	// Call callback
	if callback != nil {
		go callback(oldCluster, m.localCluster)
	}
}

// findFailoverTarget finds a healthy cluster to fail over to
func (m *FailoverManager) findFailoverTarget() string {
	// First try the preferred cluster
	if m.config.PreferredFailoverCluster != "" {
		if m.isClusterHealthy(m.config.PreferredFailoverCluster) {
			return m.config.PreferredFailoverCluster
		}
	}

	// Try any healthy cluster
	for _, name := range m.healthMonitor.GetHealthyGateways() {
		if name != m.localCluster {
			return name
		}
	}

	return ""
}

// ManualFailover triggers a manual failover to a specific cluster
func (m *FailoverManager) ManualFailover(targetCluster string) error {
	if !m.isClusterHealthy(targetCluster) {
		return fmt.Errorf("target cluster %q is not healthy", targetCluster)
	}

	m.mu.Lock()
	oldCluster := m.activeCluster
	m.activeCluster = targetCluster
	m.failedOverTo = targetCluster
	m.failoverTime = time.Now()
	callback := m.onFailover
	m.mu.Unlock()

	// Update router
	m.router.UpdateClusterAvailability(oldCluster, false)
	m.router.UpdateClusterAvailability(targetCluster, true)

	m.state.Store(int32(FailoverStateFailedOver))

	// Call callback
	if callback != nil {
		go callback(oldCluster, targetCluster)
	}

	return nil
}

// ManualFailback triggers a manual failback to the local cluster
func (m *FailoverManager) ManualFailback() error {
	if m.State() != FailoverStateFailedOver {
		return errors.New("not in failed-over state")
	}

	if !m.isClusterHealthy(m.localCluster) {
		return fmt.Errorf("local cluster %q is not healthy", m.localCluster)
	}

	m.triggerFailback()
	return nil
}

// GetStatus returns the current failover status
func (m *FailoverManager) GetStatus() *FailoverStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &FailoverStatus{
		State:           m.State(),
		LocalCluster:    m.localCluster,
		ActiveCluster:   m.activeCluster,
		FailedOverTo:    m.failedOverTo,
		FailoverTime:    m.failoverTime,
		IsFailedOver:    m.State() == FailoverStateFailedOver,
		HealthyGateways: m.healthMonitor.GetHealthyGateways(),
	}
}

// FailoverStatus contains the current failover status
type FailoverStatus struct {
	State           FailoverState
	LocalCluster    string
	ActiveCluster   string
	FailedOverTo    string
	FailoverTime    time.Time
	IsFailedOver    bool
	HealthyGateways []string
}
