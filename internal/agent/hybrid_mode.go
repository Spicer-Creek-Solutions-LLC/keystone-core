// Package agent provides the Keystone Core agent implementation
package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
)

// ConnectionRole defines how an agent connects to NATS
type ConnectionRole int

const (
	// ConnectionRoleUndetermined role has not been determined yet
	ConnectionRoleUndetermined ConnectionRole = iota
	// ConnectionRoleClient agent connects to external NATS
	ConnectionRoleClient
	// ConnectionRoleHost agent hosts embedded NATS and accepts connections
	ConnectionRoleHost
	// ConnectionRoleLeaf agent hosts embedded NATS that also connects as leaf
	ConnectionRoleLeaf
)

// String returns string representation of ConnectionRole
func (r ConnectionRole) String() string {
	switch r {
	case ConnectionRoleUndetermined:
		return "undetermined"
	case ConnectionRoleClient:
		return "client"
	case ConnectionRoleHost:
		return "host"
	case ConnectionRoleLeaf:
		return "leaf"
	default:
		return "unknown"
	}
}

// ParseConnectionRole parses a string to ConnectionRole
func ParseConnectionRole(s string) (ConnectionRole, error) {
	switch s {
	case "undetermined", "":
		return ConnectionRoleUndetermined, nil
	case "client":
		return ConnectionRoleClient, nil
	case "host":
		return ConnectionRoleHost, nil
	case "leaf":
		return ConnectionRoleLeaf, nil
	default:
		return ConnectionRoleUndetermined, fmt.Errorf("unknown connection role: %s", s)
	}
}

// RoleSelectionMode defines how the connection role is determined
type RoleSelectionMode int

const (
	// RoleSelectionAuto automatically determines role based on network
	RoleSelectionAuto RoleSelectionMode = iota
	// RoleSelectionManual uses the configured role override
	RoleSelectionManual
	// RoleSelectionPreferHost prefers hosting when possible
	RoleSelectionPreferHost
	// RoleSelectionPreferClient prefers connecting when possible
	RoleSelectionPreferClient
)

// String returns string representation of RoleSelectionMode
func (m RoleSelectionMode) String() string {
	switch m {
	case RoleSelectionAuto:
		return "auto"
	case RoleSelectionManual:
		return "manual"
	case RoleSelectionPreferHost:
		return "prefer-host"
	case RoleSelectionPreferClient:
		return "prefer-client"
	default:
		return "unknown"
	}
}

// ParseRoleSelectionMode parses a string to RoleSelectionMode
func ParseRoleSelectionMode(s string) (RoleSelectionMode, error) {
	switch s {
	case "auto", "":
		return RoleSelectionAuto, nil
	case "manual":
		return RoleSelectionManual, nil
	case "prefer-host":
		return RoleSelectionPreferHost, nil
	case "prefer-client":
		return RoleSelectionPreferClient, nil
	default:
		return RoleSelectionAuto, fmt.Errorf("unknown role selection mode: %s", s)
	}
}

// NetworkReachability describes the agent's network situation
type NetworkReachability int

const (
	// NetworkReachabilityUnknown reachability has not been determined
	NetworkReachabilityUnknown NetworkReachability = iota
	// NetworkReachabilityDirect agent is directly reachable (public IP or routable)
	NetworkReachabilityDirect
	// NetworkReachabilityNAT agent is behind NAT but port forwarding may be available
	NetworkReachabilityNAT
	// NetworkReachabilityRestricted agent is behind restrictive NAT/firewall
	NetworkReachabilityRestricted
)

// String returns string representation of NetworkReachability
func (n NetworkReachability) String() string {
	switch n {
	case NetworkReachabilityUnknown:
		return "unknown"
	case NetworkReachabilityDirect:
		return "direct"
	case NetworkReachabilityNAT:
		return "nat"
	case NetworkReachabilityRestricted:
		return "restricted"
	default:
		return "unknown"
	}
}

// HybridModeConfig configures hybrid mode behavior
type HybridModeConfig struct {
	// SelectionMode determines how role is selected
	SelectionMode RoleSelectionMode

	// ManualRole is used when SelectionMode is RoleSelectionManual
	ManualRole ConnectionRole

	// ExternalNATSURLs are the external NATS cluster URLs (for client mode)
	ExternalNATSURLs []string

	// EmbeddedConfig is the configuration for embedded NATS (for host mode)
	EmbeddedConfig *EmbeddedNATSConfig

	// AdvertiserConfig is the configuration for endpoint advertising
	AdvertiserConfig *EndpointAdvertiserConfig

	// ReachabilityCheckTimeout is the timeout for reachability checks
	ReachabilityCheckTimeout time.Duration

	// ReachabilityCheckInterval is how often to re-check reachability
	ReachabilityCheckInterval time.Duration

	// FallbackToClient falls back to client mode if hosting fails
	FallbackToClient bool

	// FallbackToHost falls back to host mode if connecting fails
	FallbackToHost bool

	// MinConnectionsForHost is the minimum number of expected inbound connections
	// before considering hosting worthwhile (default: 1)
	MinConnectionsForHost int

	// NATS client options for client mode
	NATSOptions []nats.Option

	// AgentID is the agent's unique identifier
	AgentID string
}

// DefaultHybridModeConfig returns default configuration
func DefaultHybridModeConfig(agentID string) *HybridModeConfig {
	return &HybridModeConfig{
		SelectionMode:             RoleSelectionAuto,
		ManualRole:                ConnectionRoleUndetermined,
		ReachabilityCheckTimeout:  5 * time.Second,
		ReachabilityCheckInterval: 5 * time.Minute,
		FallbackToClient:          true,
		FallbackToHost:            false,
		MinConnectionsForHost:     1,
		AgentID:                   agentID,
	}
}

// Validate validates the configuration
func (c *HybridModeConfig) Validate() error {
	if c.AgentID == "" {
		return errors.New("agent_id is required")
	}

	if c.SelectionMode == RoleSelectionManual {
		if c.ManualRole == ConnectionRoleUndetermined {
			return errors.New("manual role must be specified when selection mode is manual")
		}
	}

	// Note: When in client mode or auto-selection mode, empty ExternalNATSURLs
	// is acceptable if FallbackToHost is enabled

	if c.ReachabilityCheckTimeout <= 0 {
		c.ReachabilityCheckTimeout = 5 * time.Second
	}

	if c.ReachabilityCheckInterval <= 0 {
		c.ReachabilityCheckInterval = 5 * time.Minute
	}

	return nil
}

// HybridModeState represents the current hybrid mode state
type HybridModeState int

const (
	// HybridModeStateIdle manager is idle
	HybridModeStateIdle HybridModeState = iota
	// HybridModeStateDetermining determining best role
	HybridModeStateDetermining
	// HybridModeStateConnecting connecting as client
	HybridModeStateConnecting
	// HybridModeStateHosting hosting embedded NATS
	HybridModeStateHosting
	// HybridModeStateActive connection is active
	HybridModeStateActive
	// HybridModeStateFailed connection failed
	HybridModeStateFailed
)

// String returns string representation of HybridModeState
func (s HybridModeState) String() string {
	switch s {
	case HybridModeStateIdle:
		return "idle"
	case HybridModeStateDetermining:
		return "determining"
	case HybridModeStateConnecting:
		return "connecting"
	case HybridModeStateHosting:
		return "hosting"
	case HybridModeStateActive:
		return "active"
	case HybridModeStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// HybridModeManager manages hybrid mode connections
type HybridModeManager struct {
	config *HybridModeConfig

	// Current state
	state        atomic.Int32
	role         atomic.Int32
	reachability atomic.Int32

	// Components
	embeddedServer *EmbeddedNATSServer
	advertiser     *EndpointAdvertiser
	clientConn     *nats.Conn

	// Callbacks
	onStateChange     func(state HybridModeState)
	onRoleChange      func(role ConnectionRole)
	onConnectionReady func(role ConnectionRole, conn *nats.Conn)
	onConnectionLost  func(role ConnectionRole, err error)

	// Internal state
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewHybridModeManager creates a new hybrid mode manager
func NewHybridModeManager(config *HybridModeConfig) (*HybridModeManager, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &HybridModeManager{
		config: config,
	}, nil
}

// SetStateChangeCallback sets a callback for state changes
func (m *HybridModeManager) SetStateChangeCallback(cb func(state HybridModeState)) {
	m.mu.Lock()
	m.onStateChange = cb
	m.mu.Unlock()
}

// SetRoleChangeCallback sets a callback for role changes
func (m *HybridModeManager) SetRoleChangeCallback(cb func(role ConnectionRole)) {
	m.mu.Lock()
	m.onRoleChange = cb
	m.mu.Unlock()
}

// SetConnectionReadyCallback sets a callback for when connection is ready
func (m *HybridModeManager) SetConnectionReadyCallback(cb func(role ConnectionRole, conn *nats.Conn)) {
	m.mu.Lock()
	m.onConnectionReady = cb
	m.mu.Unlock()
}

// SetConnectionLostCallback sets a callback for connection loss
func (m *HybridModeManager) SetConnectionLostCallback(cb func(role ConnectionRole, err error)) {
	m.mu.Lock()
	m.onConnectionLost = cb
	m.mu.Unlock()
}

// Start starts the hybrid mode manager
func (m *HybridModeManager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.State() != HybridModeStateIdle {
		m.mu.Unlock()
		return errors.New("manager already started")
	}
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Unlock()

	m.setState(HybridModeStateDetermining)

	// Determine role
	role, err := m.determineRole(m.ctx) //nolint:contextcheck // m.ctx is derived from ctx on line 336
	if err != nil {
		m.setState(HybridModeStateFailed)
		return fmt.Errorf("failed to determine role: %w", err)
	}

	m.setRole(role)

	// Start in the determined role
	if err := m.startInRole(role); err != nil {
		// Try fallback if configured
		if m.tryFallback(role, err) {
			return nil
		}
		m.setState(HybridModeStateFailed)
		return fmt.Errorf("failed to start in role %s: %w", role, err)
	}

	m.setState(HybridModeStateActive)

	// Start background monitoring
	m.wg.Add(1)
	go m.monitorLoop(m.ctx) //nolint:contextcheck // m.ctx is derived from ctx on line 336

	return nil
}

// Stop stops the hybrid mode manager
func (m *HybridModeManager) Stop() error {
	m.mu.Lock()
	cancel := m.cancel
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	m.wg.Wait()

	// Stop components
	if m.embeddedServer != nil {
		m.embeddedServer.Stop()
	}
	if m.advertiser != nil {
		m.advertiser.Stop()
	}
	if m.clientConn != nil {
		m.clientConn.Close()
	}

	m.setState(HybridModeStateIdle)
	return nil
}

// State returns the current state
func (m *HybridModeManager) State() HybridModeState {
	return HybridModeState(m.state.Load())
}

// Role returns the current connection role
func (m *HybridModeManager) Role() ConnectionRole {
	return ConnectionRole(m.role.Load())
}

// Reachability returns the detected network reachability
func (m *HybridModeManager) Reachability() NetworkReachability {
	return NetworkReachability(m.reachability.Load())
}

// GetConnection returns the active NATS connection (client mode only)
func (m *HybridModeManager) GetConnection() *nats.Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clientConn
}

// GetEmbeddedServer returns the embedded NATS server (host mode only)
func (m *HybridModeManager) GetEmbeddedServer() *EmbeddedNATSServer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.embeddedServer
}

// GetAdvertiser returns the endpoint advertiser (host mode only)
func (m *HybridModeManager) GetAdvertiser() *EndpointAdvertiser {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.advertiser
}

// IsActive returns true if the manager is in active state
func (m *HybridModeManager) IsActive() bool {
	return m.State() == HybridModeStateActive
}

// determineRole determines the best connection role
func (m *HybridModeManager) determineRole(ctx context.Context) (ConnectionRole, error) {
	switch m.config.SelectionMode {
	case RoleSelectionManual:
		return m.config.ManualRole, nil

	case RoleSelectionPreferHost:
		if m.canHost() {
			return m.roleFromEmbeddedMode(), nil
		}
		return ConnectionRoleClient, nil

	case RoleSelectionPreferClient:
		if m.canConnect() {
			return ConnectionRoleClient, nil
		}
		if m.canHost() {
			return m.roleFromEmbeddedMode(), nil
		}
		return ConnectionRoleUndetermined, errors.New("cannot connect or host")

	default: // includes RoleSelectionAuto
		return m.autoSelectRole(ctx)
	}
}

// autoSelectRole automatically selects the best role
func (m *HybridModeManager) autoSelectRole(ctx context.Context) (ConnectionRole, error) {
	// Check network reachability
	reachability := m.checkReachability(ctx)
	//nolint:gosec // G115: NetworkReachability is a small enum (0-3), fits in int32
	m.reachability.Store(int32(reachability))

	// If directly reachable and hosting is configured, prefer hosting
	if reachability == NetworkReachabilityDirect && m.canHost() {
		return m.roleFromEmbeddedMode(), nil
	}

	// If we can connect to external NATS, use client mode
	if m.canConnect() {
		return ConnectionRoleClient, nil
	}

	// If behind NAT but we can host, use host mode with advertisement
	if reachability == NetworkReachabilityNAT && m.canHost() {
		return m.roleFromEmbeddedMode(), nil
	}

	// Restricted network and have external URLs - use client
	if len(m.config.ExternalNATSURLs) > 0 {
		return ConnectionRoleClient, nil
	}

	// Last resort - host if possible
	if m.canHost() {
		return m.roleFromEmbeddedMode(), nil
	}

	return ConnectionRoleUndetermined, errors.New("cannot determine suitable role")
}

// roleFromEmbeddedMode converts embedded mode to connection role
func (m *HybridModeManager) roleFromEmbeddedMode() ConnectionRole {
	if m.config.EmbeddedConfig == nil {
		return ConnectionRoleHost
	}

	switch m.config.EmbeddedConfig.Mode {
	case EmbeddedNATSModeLeaf:
		return ConnectionRoleLeaf
	case EmbeddedNATSModeStandalone:
		return ConnectionRoleHost
	default:
		return ConnectionRoleHost
	}
}

// canHost returns true if hosting is possible
func (m *HybridModeManager) canHost() bool {
	return m.config.EmbeddedConfig != nil && m.config.EmbeddedConfig.Mode != EmbeddedNATSModeDisabled
}

// canConnect returns true if connecting is possible
func (m *HybridModeManager) canConnect() bool {
	return len(m.config.ExternalNATSURLs) > 0
}

// checkReachability checks the network reachability
func (m *HybridModeManager) checkReachability(ctx context.Context) NetworkReachability {
	// Check if we have a public IP
	publicIP := ""
	if m.config.AdvertiserConfig != nil {
		adv, _ := NewEndpointAdvertiser(m.config.AdvertiserConfig)
		if adv != nil {
			timeoutCtx, cancel := context.WithTimeout(ctx, m.config.ReachabilityCheckTimeout)
			defer cancel()
			for _, service := range m.config.AdvertiserConfig.PublicIPServices {
				ip, err := fetchPublicIP(timeoutCtx, service)
				if err == nil {
					publicIP = ip
					break
				}
			}
		}
	}

	// Check if our local IP matches the public IP (direct connection)
	localAddrs, _ := getLocalAddresses()
	for _, addr := range localAddrs {
		if addr == publicIP {
			return NetworkReachabilityDirect
		}
	}

	// If we have a public IP but it doesn't match local, we're behind NAT
	if publicIP != "" {
		// Try to check if port is reachable (simplified check)
		if m.isPortReachable(ctx) {
			return NetworkReachabilityNAT
		}
		return NetworkReachabilityRestricted
	}

	// Unable to detect public IP
	return NetworkReachabilityUnknown
}

// isPortReachable checks if our port is reachable from outside
func (m *HybridModeManager) isPortReachable(ctx context.Context) bool {
	if m.config.EmbeddedConfig == nil {
		return false
	}

	// Try to bind to the port to see if it's available
	port := m.config.EmbeddedConfig.Port
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	listener.Close()

	// Port is available locally - external reachability would need STUN/TURN
	// For now, we assume NAT with potential port forwarding
	return true
}

// startInRole starts the manager in the specified role
func (m *HybridModeManager) startInRole(role ConnectionRole) error {
	switch role {
	case ConnectionRoleClient:
		return m.startAsClient()
	case ConnectionRoleHost:
		return m.startAsHost(EmbeddedNATSModeStandalone)
	case ConnectionRoleLeaf:
		return m.startAsHost(EmbeddedNATSModeLeaf)
	default:
		return fmt.Errorf("cannot start in role: %s", role)
	}
}

// startAsClient starts in client mode
func (m *HybridModeManager) startAsClient() error {
	m.setState(HybridModeStateConnecting)

	opts := make([]nats.Option, 0, 5+len(m.config.NATSOptions))
	opts = append(opts,
		nats.Name(fmt.Sprintf("kscore-agent-%s", m.config.AgentID)),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			m.notifyConnectionLost(ConnectionRoleClient, err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			m.notifyConnectionReady(ConnectionRoleClient, nc)
		}),
	)

	// Add any configured options
	opts = append(opts, m.config.NATSOptions...)

	// Try each URL
	var lastErr error
	for _, url := range m.config.ExternalNATSURLs {
		conn, err := nats.Connect(url, opts...)
		if err != nil {
			lastErr = err
			continue
		}

		m.mu.Lock()
		m.clientConn = conn
		m.mu.Unlock()

		m.notifyConnectionReady(ConnectionRoleClient, conn)
		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("failed to connect to any NATS server: %w", lastErr)
	}

	return errors.New("no NATS URLs configured")
}

// startAsHost starts in host mode
func (m *HybridModeManager) startAsHost(mode EmbeddedNATSMode) error {
	m.setState(HybridModeStateHosting)

	// Configure embedded NATS
	config := m.config.EmbeddedConfig
	if config == nil {
		config = DefaultEmbeddedNATSConfig()
	}
	config.Mode = mode

	// Create and start embedded server
	server, err := NewEmbeddedNATSServer(config)
	if err != nil {
		return fmt.Errorf("failed to create embedded NATS server: %w", err)
	}

	if err := server.Start(m.ctx); err != nil {
		return fmt.Errorf("failed to start embedded NATS server: %w", err)
	}

	m.mu.Lock()
	m.embeddedServer = server
	m.mu.Unlock()

	// Start endpoint advertiser if configured
	if m.config.AdvertiserConfig != nil {
		advConfig := m.config.AdvertiserConfig
		advConfig.AgentID = m.config.AgentID
		advConfig.LocalPort = config.Port
		advConfig.TLSEnabled = config.TLSConfig != nil

		advertiser, err := NewEndpointAdvertiser(advConfig)
		if err != nil {
			server.Stop()
			return fmt.Errorf("failed to create endpoint advertiser: %w", err)
		}

		if err := advertiser.Start(m.ctx); err != nil {
			server.Stop()
			return fmt.Errorf("failed to start endpoint advertiser: %w", err)
		}

		m.mu.Lock()
		m.advertiser = advertiser
		m.mu.Unlock()
	}

	// Connect locally as a client to the embedded server
	url := server.GetClientURL(m.ctx)
	if url == "" {
		return errors.New("embedded server not providing client URL")
	}

	opts := []nats.Option{
		nats.Name(fmt.Sprintf("kscore-agent-%s-local", m.config.AgentID)),
	}

	// Add auth if configured
	if config.AuthConfig != nil && config.AuthConfig.Token != "" {
		opts = append(opts, nats.Token(config.AuthConfig.Token))
	}

	conn, err := nats.Connect(url, opts...)
	if err != nil {
		server.Stop()
		if m.advertiser != nil {
			m.advertiser.Stop()
		}
		return fmt.Errorf("failed to connect to local embedded NATS: %w", err)
	}

	m.mu.Lock()
	m.clientConn = conn
	m.mu.Unlock()

	role := ConnectionRoleHost
	if mode == EmbeddedNATSModeLeaf {
		role = ConnectionRoleLeaf
	}
	m.notifyConnectionReady(role, conn)

	return nil
}

// tryFallback attempts to fall back to an alternative mode
func (m *HybridModeManager) tryFallback(failedRole ConnectionRole, err error) bool {
	switch failedRole {
	case ConnectionRoleClient:
		if m.config.FallbackToHost && m.canHost() {
			newRole := m.roleFromEmbeddedMode()
			if startErr := m.startInRole(newRole); startErr == nil {
				m.setRole(newRole)
				m.setState(HybridModeStateActive)
				return true
			}
		}
	case ConnectionRoleHost, ConnectionRoleLeaf:
		if m.config.FallbackToClient && m.canConnect() {
			if startErr := m.startAsClient(); startErr == nil {
				m.setRole(ConnectionRoleClient)
				m.setState(HybridModeStateActive)
				return true
			}
		}
	default:
	}
	return false
}

// monitorLoop monitors the connection and role
func (m *HybridModeManager) monitorLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.ReachabilityCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Periodically re-check reachability
			reachability := m.checkReachability(ctx)
			oldReach := m.Reachability()
			if reachability != oldReach {
				//nolint:gosec // G115: NetworkReachability is a small enum (0-3), fits in int32
				m.reachability.Store(int32(reachability))
				// Could trigger role change here if needed
			}

		case <-ctx.Done():
			return
		}
	}
}

// setState updates the state and calls callback
func (m *HybridModeManager) setState(state HybridModeState) {
	//nolint:gosec // G115: HybridModeState is a small enum (0-4), fits in int32
	m.state.Store(int32(state))

	m.mu.RLock()
	cb := m.onStateChange
	m.mu.RUnlock()

	if cb != nil {
		cb(state)
	}
}

// setRole updates the role and calls callback
func (m *HybridModeManager) setRole(role ConnectionRole) {
	//nolint:gosec // G115: ConnectionRole is a small enum (0-3), fits in int32
	m.role.Store(int32(role))

	m.mu.RLock()
	cb := m.onRoleChange
	m.mu.RUnlock()

	if cb != nil {
		cb(role)
	}
}

// notifyConnectionReady notifies that connection is ready
func (m *HybridModeManager) notifyConnectionReady(role ConnectionRole, conn *nats.Conn) {
	m.mu.RLock()
	cb := m.onConnectionReady
	m.mu.RUnlock()

	if cb != nil {
		cb(role, conn)
	}
}

// notifyConnectionLost notifies that connection was lost
func (m *HybridModeManager) notifyConnectionLost(role ConnectionRole, err error) {
	m.mu.RLock()
	cb := m.onConnectionLost
	m.mu.RUnlock()

	if cb != nil {
		cb(role, err)
	}
}

// HybridModeStats contains hybrid mode statistics
type HybridModeStats struct {
	State        HybridModeState
	Role         ConnectionRole
	Reachability NetworkReachability
	Uptime       time.Duration
	// Client mode stats
	ClientConnected bool
	ClientURL       string
	// Host mode stats
	ServerRunning    bool
	ServerStats      *EmbeddedNATSStats
	AdvertiserActive bool
	LastAdvertised   *EndpointAdvertisement
}

// GetStats returns current statistics
func (m *HybridModeManager) GetStats() *HybridModeStats {
	stats := &HybridModeStats{
		State:        m.State(),
		Role:         m.Role(),
		Reachability: m.Reachability(),
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.clientConn != nil {
		stats.ClientConnected = m.clientConn.IsConnected()
		stats.ClientURL = m.clientConn.ConnectedUrl()
	}

	if m.embeddedServer != nil {
		stats.ServerRunning = m.embeddedServer.IsRunning()
		stats.ServerStats = m.embeddedServer.GetStats()
	}

	if m.advertiser != nil {
		stats.AdvertiserActive = m.advertiser.IsRunning()
		stats.LastAdvertised = m.advertiser.GetLastAdvertisement()
	}

	return stats
}

// Config returns the configuration
func (m *HybridModeManager) Config() *HybridModeConfig {
	return m.config
}
