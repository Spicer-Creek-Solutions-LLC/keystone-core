// Package controlplane provides the Keystone Core control plane implementation
package controlplane

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/internal/agent"
)

// AgentConnectionState represents the connection state to an agent's NATS
type AgentConnectionState int

const (
	// AgentConnectionStateDisconnected not connected to agent
	AgentConnectionStateDisconnected AgentConnectionState = iota
	// AgentConnectionStateConnecting attempting to connect
	AgentConnectionStateConnecting
	// AgentConnectionStateConnected successfully connected
	AgentConnectionStateConnected
	// AgentConnectionStateReconnecting connection lost, attempting to reconnect
	AgentConnectionStateReconnecting
	// AgentConnectionStateFailed connection failed (may retry)
	AgentConnectionStateFailed
)

// String returns string representation
func (s AgentConnectionState) String() string {
	switch s {
	case AgentConnectionStateDisconnected:
		return "disconnected"
	case AgentConnectionStateConnecting:
		return "connecting"
	case AgentConnectionStateConnected:
		return "connected"
	case AgentConnectionStateReconnecting:
		return "reconnecting"
	case AgentConnectionStateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// AgentConnection represents an outbound connection to an agent's embedded NATS
type AgentConnection struct {
	// AgentID is the agent's unique identifier
	AgentID string

	// Advertisement is the endpoint advertisement from the agent
	Advertisement *agent.EndpointAdvertisement

	// Connection is the NATS connection to the agent
	conn *nats.Conn

	// State tracking
	state           atomic.Int32
	connectAttempts atomic.Int64
	lastConnected   atomic.Value // time.Time
	lastError       atomic.Value // error

	mu sync.RWMutex
}

// State returns the current connection state
func (c *AgentConnection) State() AgentConnectionState {
	return AgentConnectionState(c.state.Load())
}

// IsConnected returns true if connected to the agent
func (c *AgentConnection) IsConnected() bool {
	return c.State() == AgentConnectionStateConnected
}

// Conn returns the NATS connection (may be nil if not connected)
func (c *AgentConnection) Conn() *nats.Conn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

// LastError returns the last connection error
func (c *AgentConnection) LastError() error {
	if v := c.lastError.Load(); v != nil {
		return v.(error)
	}
	return nil
}

// ConnectAttempts returns the number of connection attempts
func (c *AgentConnection) ConnectAttempts() int64 {
	return c.connectAttempts.Load()
}

// AgentConnectorConfig configures the agent connector
type AgentConnectorConfig struct {
	// ConnectTimeout is the timeout for initial connection
	ConnectTimeout time.Duration

	// ReconnectWait is the wait time before reconnection attempts
	ReconnectWait time.Duration

	// MaxReconnects is the maximum number of reconnection attempts (-1 for unlimited)
	MaxReconnects int

	// PingInterval is how often to ping the connection
	PingInterval time.Duration

	// MaxPingsOut is the maximum outstanding pings before considering connection dead
	MaxPingsOut int

	// MaxConnectionsPerAgent limits connections to each agent
	MaxConnectionsPerAgent int

	// DiscoveryInterval is how often to check for new agent endpoints
	DiscoveryInterval time.Duration

	// CleanupInterval is how often to clean up stale connections
	CleanupInterval time.Duration

	// TLSRequired requires TLS for all connections
	TLSRequired bool

	// NATSOptions additional NATS options to apply
	NATSOptions []nats.Option
}

// DefaultAgentConnectorConfig returns default configuration
func DefaultAgentConnectorConfig() *AgentConnectorConfig {
	return &AgentConnectorConfig{
		ConnectTimeout:         10 * time.Second,
		ReconnectWait:          2 * time.Second,
		MaxReconnects:          -1, // Unlimited
		PingInterval:           30 * time.Second,
		MaxPingsOut:            3,
		MaxConnectionsPerAgent: 1,
		DiscoveryInterval:      30 * time.Second,
		CleanupInterval:        60 * time.Second,
	}
}

// AgentConnector manages outbound connections to agent-hosted NATS servers
type AgentConnector struct {
	config   *AgentConnectorConfig
	registry *agent.EndpointRegistry

	// Connections keyed by agent ID
	connections map[string]*AgentConnection
	connMu      sync.RWMutex

	// Callbacks
	onConnect    func(agentID string, conn *nats.Conn)
	onDisconnect func(agentID string, err error)

	// State
	running atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewAgentConnector creates a new agent connector
func NewAgentConnector(config *AgentConnectorConfig, registry *agent.EndpointRegistry) (*AgentConnector, error) {
	if config == nil {
		config = DefaultAgentConnectorConfig()
	}
	if registry == nil {
		return nil, errors.New("registry is required")
	}

	return &AgentConnector{
		config:      config,
		registry:    registry,
		connections: make(map[string]*AgentConnection),
	}, nil
}

// SetConnectCallback sets a callback for successful connections
func (c *AgentConnector) SetConnectCallback(cb func(agentID string, conn *nats.Conn)) {
	c.connMu.Lock()
	c.onConnect = cb
	c.connMu.Unlock()
}

// SetDisconnectCallback sets a callback for disconnections
func (c *AgentConnector) SetDisconnectCallback(cb func(agentID string, err error)) {
	c.connMu.Lock()
	c.onDisconnect = cb
	c.connMu.Unlock()
}

// Start starts the agent connector
func (c *AgentConnector) Start(ctx context.Context) error {
	if c.running.Load() {
		return errors.New("connector already running")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.running.Store(true)

	// Subscribe to registry changes
	c.registry.SetChangeCallback(c.handleEndpointChange)

	// Start discovery loop
	c.wg.Add(1)
	go c.discoveryLoop()

	// Start cleanup loop
	c.wg.Add(1)
	go c.cleanupLoop()

	// Connect to existing endpoints
	for _, adv := range c.registry.GetAll() {
		go c.connectToAgent(adv)
	}

	return nil
}

// Stop stops the agent connector
func (c *AgentConnector) Stop() error {
	if !c.running.Load() {
		return nil
	}

	c.running.Store(false)
	c.registry.SetChangeCallback(nil)

	if c.cancel != nil {
		c.cancel()
	}

	// Close all connections
	c.connMu.Lock()
	for _, conn := range c.connections {
		if conn.conn != nil {
			conn.conn.Close()
		}
	}
	c.connMu.Unlock()

	c.wg.Wait()
	return nil
}

// GetConnection returns the connection to a specific agent
func (c *AgentConnector) GetConnection(agentID string) *AgentConnection {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.connections[agentID]
}

// GetConnectedAgents returns all connected agent IDs
func (c *AgentConnector) GetConnectedAgents() []string {
	c.connMu.RLock()
	defer c.connMu.RUnlock()

	var connected []string
	for agentID, conn := range c.connections {
		if conn.IsConnected() {
			connected = append(connected, agentID)
		}
	}
	return connected
}

// ConnectionCount returns the number of active connections
func (c *AgentConnector) ConnectionCount() int {
	c.connMu.RLock()
	defer c.connMu.RUnlock()

	count := 0
	for _, conn := range c.connections {
		if conn.IsConnected() {
			count++
		}
	}
	return count
}

// ConnectToAgent initiates a connection to a specific agent
func (c *AgentConnector) ConnectToAgent(agentID string) error {
	adv := c.registry.Get(agentID)
	if adv == nil {
		return fmt.Errorf("agent %s not found in registry", agentID)
	}

	go c.connectToAgent(adv)
	return nil
}

// DisconnectFromAgent disconnects from a specific agent
func (c *AgentConnector) DisconnectFromAgent(agentID string) error {
	c.connMu.Lock()
	conn, exists := c.connections[agentID]
	if !exists {
		c.connMu.Unlock()
		return nil
	}
	delete(c.connections, agentID)
	c.connMu.Unlock()

	if conn.conn != nil {
		conn.conn.Close()
	}
	return nil
}

// handleEndpointChange handles registry endpoint changes
func (c *AgentConnector) handleEndpointChange(agentID string, adv *agent.EndpointAdvertisement) {
	if adv == nil {
		// Agent unregistered, disconnect
		_ = c.DisconnectFromAgent(agentID) //nolint:errcheck // best-effort disconnect
		return
	}

	// Check if we should connect
	c.connMu.RLock()
	existing, exists := c.connections[agentID]
	c.connMu.RUnlock()

	if !exists {
		// New agent, connect
		go c.connectToAgent(adv)
		return
	}

	// Update advertisement
	existing.mu.Lock()
	existing.Advertisement = adv
	existing.mu.Unlock()
}

// connectToAgent connects to an agent's embedded NATS
func (c *AgentConnector) connectToAgent(adv *agent.EndpointAdvertisement) {
	if adv == nil {
		return
	}

	agentID := adv.AgentID

	// Check if already connected
	c.connMu.Lock()
	if existing, exists := c.connections[agentID]; exists && existing.IsConnected() {
		c.connMu.Unlock()
		return
	}

	// Create or update connection record
	agentConn := &AgentConnection{
		AgentID:       agentID,
		Advertisement: adv,
	}
	c.connections[agentID] = agentConn
	c.connMu.Unlock()

	agentConn.state.Store(int32(AgentConnectionStateConnecting))
	agentConn.connectAttempts.Add(1)

	// Build NATS URL
	natsURL := adv.GetURL()

	// Build connection options
	opts := make([]nats.Option, 0, 9+len(c.config.NATSOptions))
	opts = append(opts,
		nats.Timeout(c.config.ConnectTimeout),
		nats.PingInterval(c.config.PingInterval),
		nats.MaxPingsOutstanding(c.config.MaxPingsOut),
		nats.ReconnectWait(c.config.ReconnectWait),
		nats.MaxReconnects(c.config.MaxReconnects),
		nats.Name(fmt.Sprintf("kscore-server-to-agent-%s", agentID)),

		// Disconnection handler
		nats.DisconnectErrHandler(func(conn *nats.Conn, err error) {
			agentConn.state.Store(int32(AgentConnectionStateReconnecting))
			if err != nil {
				agentConn.lastError.Store(err)
			}

			c.connMu.RLock()
			cb := c.onDisconnect
			c.connMu.RUnlock()

			if cb != nil {
				cb(agentID, err)
			}
		}),

		// Reconnection handler
		nats.ReconnectHandler(func(conn *nats.Conn) {
			agentConn.state.Store(int32(AgentConnectionStateConnected))
			agentConn.lastConnected.Store(time.Now())
		}),

		// Closed handler
		nats.ClosedHandler(func(conn *nats.Conn) {
			agentConn.state.Store(int32(AgentConnectionStateDisconnected))
		}),
	)

	// Add user-provided options
	opts = append(opts, c.config.NATSOptions...)

	// Check TLS requirement
	if c.config.TLSRequired && !adv.TLSEnabled {
		err := errors.New("TLS required but agent endpoint does not support TLS")
		agentConn.lastError.Store(err)
		agentConn.state.Store(int32(AgentConnectionStateFailed))
		return
	}

	// Connect
	conn, err := nats.Connect(natsURL, opts...)
	if err != nil {
		agentConn.lastError.Store(err)
		agentConn.state.Store(int32(AgentConnectionStateFailed))
		return
	}

	agentConn.mu.Lock()
	agentConn.conn = conn
	agentConn.mu.Unlock()

	agentConn.state.Store(int32(AgentConnectionStateConnected))
	agentConn.lastConnected.Store(time.Now())

	// Notify callback
	c.connMu.RLock()
	cb := c.onConnect
	c.connMu.RUnlock()

	if cb != nil {
		cb(agentID, conn)
	}
}

// discoveryLoop periodically checks for new agent endpoints
func (c *AgentConnector) discoveryLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.DiscoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.discoverNewAgents()
		case <-c.ctx.Done():
			return
		}
	}
}

// discoverNewAgents connects to any new agents in the registry
func (c *AgentConnector) discoverNewAgents() {
	endpoints := c.registry.GetHealthy()

	c.connMu.RLock()
	existingAgents := make(map[string]bool)
	for agentID := range c.connections {
		existingAgents[agentID] = true
	}
	c.connMu.RUnlock()

	for _, adv := range endpoints {
		if !existingAgents[adv.AgentID] {
			go c.connectToAgent(adv)
		}
	}
}

// cleanupLoop periodically cleans up stale connections
func (c *AgentConnector) cleanupLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupStaleConnections()
		case <-c.ctx.Done():
			return
		}
	}
}

// cleanupStaleConnections removes connections to unregistered agents
func (c *AgentConnector) cleanupStaleConnections() {
	// Get current registered agents
	registeredAgents := make(map[string]bool)
	for _, adv := range c.registry.GetAll() {
		registeredAgents[adv.AgentID] = true
	}

	// Find stale connections
	var staleAgents []string
	c.connMu.RLock()
	for agentID := range c.connections {
		if !registeredAgents[agentID] {
			staleAgents = append(staleAgents, agentID)
		}
	}
	c.connMu.RUnlock()

	// Remove stale connections
	for _, agentID := range staleAgents {
		_ = c.DisconnectFromAgent(agentID) //nolint:errcheck // best-effort cleanup
	}
}

// PublishToAgent publishes a message to a specific agent's NATS
func (c *AgentConnector) PublishToAgent(agentID, subject string, data []byte) error {
	conn := c.GetConnection(agentID)
	if conn == nil {
		return fmt.Errorf("no connection to agent %s", agentID)
	}

	natsConn := conn.Conn()
	if natsConn == nil || !natsConn.IsConnected() {
		return fmt.Errorf("not connected to agent %s", agentID)
	}

	return natsConn.Publish(subject, data)
}

// RequestToAgent sends a request to a specific agent and waits for response
func (c *AgentConnector) RequestToAgent(agentID, subject string, data []byte, timeout time.Duration) (*nats.Msg, error) {
	conn := c.GetConnection(agentID)
	if conn == nil {
		return nil, fmt.Errorf("no connection to agent %s", agentID)
	}

	natsConn := conn.Conn()
	if natsConn == nil || !natsConn.IsConnected() {
		return nil, fmt.Errorf("not connected to agent %s", agentID)
	}

	return natsConn.Request(subject, data, timeout)
}

// SubscribeOnAgent subscribes to a subject on a specific agent's NATS
func (c *AgentConnector) SubscribeOnAgent(agentID, subject string, handler nats.MsgHandler) (*nats.Subscription, error) {
	conn := c.GetConnection(agentID)
	if conn == nil {
		return nil, fmt.Errorf("no connection to agent %s", agentID)
	}

	natsConn := conn.Conn()
	if natsConn == nil || !natsConn.IsConnected() {
		return nil, fmt.Errorf("not connected to agent %s", agentID)
	}

	return natsConn.Subscribe(subject, handler)
}

// AgentConnectorStats contains connector statistics
type AgentConnectorStats struct {
	TotalConnections     int
	ConnectedCount       int
	ConnectingCount      int
	ReconnectingCount    int
	FailedCount          int
	DisconnectedCount    int
	TotalConnectAttempts int64
}

// GetStats returns connector statistics
func (c *AgentConnector) GetStats() *AgentConnectorStats {
	c.connMu.RLock()
	defer c.connMu.RUnlock()

	stats := &AgentConnectorStats{
		TotalConnections: len(c.connections),
	}

	for _, conn := range c.connections {
		stats.TotalConnectAttempts += conn.ConnectAttempts()

		switch conn.State() {
		case AgentConnectionStateConnected:
			stats.ConnectedCount++
		case AgentConnectionStateConnecting:
			stats.ConnectingCount++
		case AgentConnectionStateReconnecting:
			stats.ReconnectingCount++
		case AgentConnectionStateFailed:
			stats.FailedCount++
		case AgentConnectionStateDisconnected:
			stats.DisconnectedCount++
		}
	}

	return stats
}

// IsRunning returns true if the connector is running
func (c *AgentConnector) IsRunning() bool {
	return c.running.Load()
}
