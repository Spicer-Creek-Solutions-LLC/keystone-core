package nats

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
)

// ConnectionState represents the state of a connection
type ConnectionState int

const (
	// ConnectionStateDisconnected indicates no connection
	ConnectionStateDisconnected ConnectionState = iota
	// ConnectionStateConnecting indicates connection in progress
	ConnectionStateConnecting
	// ConnectionStateConnected indicates active connection
	ConnectionStateConnected
	// ConnectionStateReconnecting indicates reconnection in progress
	ConnectionStateReconnecting
	// ConnectionStateClosed indicates connection permanently closed
	ConnectionStateClosed
)

func (s ConnectionState) String() string {
	switch s {
	case ConnectionStateDisconnected:
		return "disconnected"
	case ConnectionStateConnecting:
		return "connecting"
	case ConnectionStateConnected:
		return "connected"
	case ConnectionStateReconnecting:
		return "reconnecting"
	case ConnectionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// EndpointState tracks the health state of an endpoint
type EndpointState struct {
	Endpoint      *Endpoint
	State         ConnectionState
	LastConnected time.Time
	LastError     error
	LastErrorTime time.Time
	ConnectCount  int64
	FailureCount  int64
	SuccessCount  int64
	TotalLatency  time.Duration
	CircuitOpen   bool
	CircuitOpenAt time.Time
	NextRetryAt   time.Time
}

// AverageLatency returns the average connection latency
func (s *EndpointState) AverageLatency() time.Duration {
	if s.SuccessCount == 0 {
		return 0
	}
	return time.Duration(int64(s.TotalLatency) / s.SuccessCount)
}

// IsHealthy returns true if the endpoint is considered healthy
func (s *EndpointState) IsHealthy() bool {
	return s.State == ConnectionStateConnected && !s.CircuitOpen
}

// SuccessRate returns the connection success rate (0.0 to 1.0)
func (s *EndpointState) SuccessRate() float64 {
	total := s.SuccessCount + s.FailureCount
	if total == 0 {
		return 1.0 // No attempts yet, assume healthy
	}
	return float64(s.SuccessCount) / float64(total)
}

// CircuitBreakerConfig configures the circuit breaker behavior
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of failures before opening the circuit
	FailureThreshold int

	// SuccessThreshold is the number of successes before closing the circuit
	SuccessThreshold int

	// OpenDuration is how long the circuit stays open before half-open
	OpenDuration time.Duration

	// HalfOpenMaxAttempts is max attempts in half-open state
	HalfOpenMaxAttempts int
}

// DefaultCircuitBreakerConfig returns sensible defaults
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold:    5,
		SuccessThreshold:    3,
		OpenDuration:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
	}
}

// AddressFamilyPreference specifies the address family preference for connections
type AddressFamilyPreference int

const (
	// PreferIPv4 prefers IPv4 but falls back to IPv6 if unavailable
	PreferIPv4 AddressFamilyPreference = iota
	// PreferIPv6 prefers IPv6 but falls back to IPv4 if unavailable
	PreferIPv6
	// IPv4Only only connects to IPv4 endpoints
	IPv4Only
	// IPv6Only only connects to IPv6 endpoints
	IPv6Only
)

// String returns the string representation of the preference
func (p AddressFamilyPreference) String() string {
	switch p {
	case PreferIPv4:
		return "prefer_ipv4"
	case PreferIPv6:
		return "prefer_ipv6"
	case IPv4Only:
		return "ipv4_only"
	case IPv6Only:
		return "ipv6_only"
	default:
		return "unknown"
	}
}

// ConnectionManagerConfig configures the connection manager
type ConnectionManagerConfig struct {
	// EndpointConfig is the multi-endpoint configuration
	EndpointConfig *EndpointConfig

	// StrategyConfig is the connection strategy configuration
	StrategyConfig *StrategyConfig

	// CircuitBreaker configures circuit breaker behavior
	CircuitBreaker *CircuitBreakerConfig

	// HealthCheckInterval is how often to check endpoint health
	HealthCheckInterval time.Duration

	// FailoverTimeout is max time to wait during failover
	FailoverTimeout time.Duration

	// RetryBackoffInitial is the initial retry backoff
	RetryBackoffInitial time.Duration

	// RetryBackoffMax is the maximum retry backoff
	RetryBackoffMax time.Duration

	// RetryBackoffMultiplier is the backoff multiplier
	RetryBackoffMultiplier float64

	// AddressFamilyPreference controls which address family to prefer
	AddressFamilyPreference AddressFamilyPreference

	// ConnectionCallbacks are called on connection state changes
	ConnectionCallbacks ConnectionCallbacks
}

// ConnectionCallbacks holds callbacks for connection events
type ConnectionCallbacks struct {
	OnConnect      func(endpoint *Endpoint)
	OnDisconnect   func(endpoint *Endpoint, err error)
	OnReconnect    func(endpoint *Endpoint)
	OnError        func(endpoint *Endpoint, err error)
	OnCircuitOpen  func(endpoint *Endpoint)
	OnCircuitClose func(endpoint *Endpoint)
}

// DefaultConnectionManagerConfig returns sensible defaults
func DefaultConnectionManagerConfig() *ConnectionManagerConfig {
	return &ConnectionManagerConfig{
		EndpointConfig:         DefaultEndpointConfig(),
		StrategyConfig:         &StrategyConfig{},
		CircuitBreaker:         DefaultCircuitBreakerConfig(),
		HealthCheckInterval:    30 * time.Second,
		FailoverTimeout:        10 * time.Second,
		RetryBackoffInitial:    100 * time.Millisecond,
		RetryBackoffMax:        30 * time.Second,
		RetryBackoffMultiplier: 2.0,
	}
}

// PooledConnectionManager manages connections to multiple NATS endpoints
type PooledConnectionManager struct {
	config          *ConnectionManagerConfig
	selector        *StrategySelector
	endpoints       []*EndpointState
	activeConn      *nats.Conn
	activeEndpoint  *Endpoint
	mu                 sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
	healthCheckStop    chan struct{}
	closed             bool
	failoverInProgress int32 // atomic
}

// NewPooledConnectionManager creates a new connection manager
func NewPooledConnectionManager(config *ConnectionManagerConfig) (*PooledConnectionManager, error) {
	if config == nil {
		config = DefaultConnectionManagerConfig()
	}

	if config.EndpointConfig == nil {
		return nil, errors.New("endpoint configuration required")
	}

	if err := config.EndpointConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid endpoint configuration: %w", err)
	}

	// Parse endpoints
	endpoints, err := config.EndpointConfig.GetEndpoints()
	if err != nil {
		return nil, fmt.Errorf("failed to parse endpoints: %w", err)
	}

	// Create endpoint states
	states := make([]*EndpointState, len(endpoints))
	for i, ep := range endpoints {
		states[i] = &EndpointState{
			Endpoint: ep,
			State:    ConnectionStateDisconnected,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &PooledConnectionManager{
		config:          config,
		selector:        DefaultStrategySelector(config.StrategyConfig),
		endpoints:       states,
		ctx:             ctx,
		cancel:          cancel,
		healthCheckStop: make(chan struct{}),
	}, nil
}

// Connect establishes a connection to the best available endpoint
func (m *PooledConnectionManager) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("connection manager is closed")
	}

	if m.activeConn != nil && m.activeConn.IsConnected() {
		return nil // Already connected
	}

	return m.connectToNextEndpoint()
}

// connectToNextEndpoint attempts to connect to the next available endpoint
func (m *PooledConnectionManager) connectToNextEndpoint() error {
	// Sort endpoints by priority and health
	orderedEndpoints := m.getOrderedEndpoints()

	var lastErr error
	for _, state := range orderedEndpoints {
		if state.CircuitOpen && time.Now().Before(state.NextRetryAt) {
			continue // Skip endpoints with open circuit
		}

		err := m.connectToEndpoint(state)
		if err == nil {
			return nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return fmt.Errorf("failed to connect to any endpoint: %w", lastErr)
	}
	return errors.New("no available endpoints")
}

// getOrderedEndpoints returns endpoints ordered by priority, address family preference, and health
func (m *PooledConnectionManager) getOrderedEndpoints() []*EndpointState {
	ordered := make([]*EndpointState, len(m.endpoints))
	copy(ordered, m.endpoints)

	// Filter by address family if configured for strict mode
	switch m.config.AddressFamilyPreference {
	case IPv4Only:
		filtered := make([]*EndpointState, 0, len(ordered))
		for _, s := range ordered {
			if !s.Endpoint.IsIPv6() {
				filtered = append(filtered, s)
			}
		}
		ordered = filtered
	case IPv6Only:
		filtered := make([]*EndpointState, 0, len(ordered))
		for _, s := range ordered {
			if s.Endpoint.IsIPv6() {
				filtered = append(filtered, s)
			}
		}
		ordered = filtered
	default:
	}

	// Determine if IPv6 is preferred
	preferIPv6 := m.config.AddressFamilyPreference == PreferIPv6 || m.config.AddressFamilyPreference == IPv6Only

	sort.Slice(ordered, func(i, j int) bool {
		// First by address family preference (if prefer mode, not only mode)
		if m.config.AddressFamilyPreference == PreferIPv4 || m.config.AddressFamilyPreference == PreferIPv6 {
			iIsPreferred := ordered[i].Endpoint.IsIPv6() == preferIPv6
			jIsPreferred := ordered[j].Endpoint.IsIPv6() == preferIPv6
			if iIsPreferred != jIsPreferred {
				return iIsPreferred // Preferred family comes first
			}
		}
		// Then by priority (lower is better)
		if ordered[i].Endpoint.Priority != ordered[j].Endpoint.Priority {
			return ordered[i].Endpoint.Priority < ordered[j].Endpoint.Priority
		}
		// Then by success rate (higher is better)
		if ordered[i].SuccessRate() != ordered[j].SuccessRate() {
			return ordered[i].SuccessRate() > ordered[j].SuccessRate()
		}
		// Then by average latency (lower is better)
		return ordered[i].AverageLatency() < ordered[j].AverageLatency()
	})

	return ordered
}

// connectToEndpoint attempts to connect to a specific endpoint
func (m *PooledConnectionManager) connectToEndpoint(state *EndpointState) error {
	state.State = ConnectionStateConnecting
	state.ConnectCount++

	start := time.Now()

	// Select strategy for this endpoint
	strategy := m.selector.SelectStrategy(state.Endpoint)
	if strategy == nil {
		return fmt.Errorf("no strategy available for endpoint %s", state.Endpoint.Address())
	}

	// Configure NATS options
	opts, err := strategy.ConfigureOptions(state.Endpoint, m.config.EndpointConfig)
	if err != nil {
		m.recordFailure(state, err)
		return fmt.Errorf("configure options: %w", err)
	}

	// Add connection event handlers
	opts = append(opts, m.buildEventHandlers(state)...)

	// Build URL
	url := state.Endpoint.ToNATSURL()

	// Attempt connection
	conn, err := nats.Connect(url, opts...)
	if err != nil {
		m.recordFailure(state, err)
		return fmt.Errorf("connect to %s: %w", state.Endpoint.Address(), err)
	}

	// Record success
	latency := time.Since(start)
	m.recordSuccess(state, latency)

	// Store connection
	m.activeConn = conn
	m.activeEndpoint = state.Endpoint
	state.State = ConnectionStateConnected

	// Notify callback
	if m.config.ConnectionCallbacks.OnConnect != nil {
		m.config.ConnectionCallbacks.OnConnect(state.Endpoint)
	}

	return nil
}

// buildEventHandlers returns NATS options for connection event handling
func (m *PooledConnectionManager) buildEventHandlers(state *EndpointState) []nats.Option {
	return []nats.Option{
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			m.mu.Lock()
			state.State = ConnectionStateDisconnected
			state.LastError = err
			state.LastErrorTime = time.Now()
			m.mu.Unlock()

			if m.config.ConnectionCallbacks.OnDisconnect != nil {
				m.config.ConnectionCallbacks.OnDisconnect(state.Endpoint, err)
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			m.mu.Lock()
			state.State = ConnectionStateConnected
			state.LastConnected = time.Now()
			m.mu.Unlock()

			if m.config.ConnectionCallbacks.OnReconnect != nil {
				m.config.ConnectionCallbacks.OnReconnect(state.Endpoint)
			}
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			if m.config.ConnectionCallbacks.OnError != nil {
				m.config.ConnectionCallbacks.OnError(state.Endpoint, err)
			}
		}),
	}
}

// recordSuccess records a successful connection
func (m *PooledConnectionManager) recordSuccess(state *EndpointState, latency time.Duration) {
	state.SuccessCount++
	state.TotalLatency += latency
	state.LastConnected = time.Now()
	state.LastError = nil

	// Close circuit breaker if open
	if state.CircuitOpen {
		state.CircuitOpen = false
		if m.config.ConnectionCallbacks.OnCircuitClose != nil {
			m.config.ConnectionCallbacks.OnCircuitClose(state.Endpoint)
		}
	}
}

// recordFailure records a failed connection attempt
func (m *PooledConnectionManager) recordFailure(state *EndpointState, err error) {
	state.FailureCount++
	state.LastError = err
	state.LastErrorTime = time.Now()
	state.State = ConnectionStateDisconnected

	// Check if circuit should open
	if m.config.CircuitBreaker != nil {
		recentFailures := state.FailureCount - state.SuccessCount
		if recentFailures >= int64(m.config.CircuitBreaker.FailureThreshold) && !state.CircuitOpen {
			state.CircuitOpen = true
			state.CircuitOpenAt = time.Now()
			state.NextRetryAt = time.Now().Add(m.config.CircuitBreaker.OpenDuration)

			if m.config.ConnectionCallbacks.OnCircuitOpen != nil {
				m.config.ConnectionCallbacks.OnCircuitOpen(state.Endpoint)
			}
		}
	}
}

// Connection returns the active NATS connection
func (m *PooledConnectionManager) Connection() *nats.Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeConn
}

// ActiveEndpoint returns the currently connected endpoint
func (m *PooledConnectionManager) ActiveEndpoint() *Endpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeEndpoint
}

// IsConnected returns true if connected to any endpoint
func (m *PooledConnectionManager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeConn != nil && m.activeConn.IsConnected()
}

// State returns the current connection state
func (m *PooledConnectionManager) State() ConnectionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return ConnectionStateClosed
	}
	if m.activeConn == nil {
		return ConnectionStateDisconnected
	}
	if m.activeConn.IsConnected() {
		return ConnectionStateConnected
	}
	if m.activeConn.IsReconnecting() {
		return ConnectionStateReconnecting
	}
	return ConnectionStateDisconnected
}

// EndpointStates returns the current state of all endpoints
func (m *PooledConnectionManager) EndpointStates() []*EndpointState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make([]*EndpointState, len(m.endpoints))
	for i, s := range m.endpoints {
		// Create a copy to avoid data races
		stateCopy := *s
		states[i] = &stateCopy
	}
	return states
}

// Failover attempts to connect to the next available endpoint
func (m *PooledConnectionManager) Failover() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("connection manager is closed")
	}

	// Close current connection if any
	if m.activeConn != nil {
		m.activeConn.Close()
		m.activeConn = nil
	}

	// Mark current endpoint as failed
	if m.activeEndpoint != nil {
		for _, state := range m.endpoints {
			if state.Endpoint == m.activeEndpoint {
				state.State = ConnectionStateDisconnected
				break
			}
		}
		m.activeEndpoint = nil
	}

	// Connect to next available endpoint
	ctx, cancel := context.WithTimeout(m.ctx, m.config.FailoverTimeout)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- m.connectToNextEndpoint()
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		return fmt.Errorf("failover timeout: %w", ctx.Err())
	}
}

// StartHealthCheck starts a background health check routine
func (m *PooledConnectionManager) StartHealthCheck() {
	if m.config.HealthCheckInterval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(m.config.HealthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.checkHealth()
			case <-m.healthCheckStop:
				return
			case <-m.ctx.Done():
				return
			}
		}
	}()
}

// checkHealth performs health checks on all endpoints
func (m *PooledConnectionManager) checkHealth() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	for _, state := range m.endpoints {
		// Check if circuit should transition to half-open
		if state.CircuitOpen && now.After(state.NextRetryAt) {
			// Allow a retry attempt
			state.NextRetryAt = now.Add(m.config.CircuitBreaker.OpenDuration)
		}
	}

	// Check if current connection is healthy — only spawn one failover at a time
	if m.activeConn != nil && !m.activeConn.IsConnected() && !m.closed {
		if atomic.CompareAndSwapInt32(&m.failoverInProgress, 0, 1) {
			// Capture callback and endpoint before releasing lock
			onError := m.config.ConnectionCallbacks.OnError
			endpoint := m.activeEndpoint
			go func() {
				defer atomic.StoreInt32(&m.failoverInProgress, 0)
				if err := m.Failover(); err != nil {
					if onError != nil {
						onError(endpoint, err)
					}
				}
			}()
		}
	}
}

// Reconnect closes the current connection and reconnects
func (m *PooledConnectionManager) Reconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return errors.New("connection manager is closed")
	}

	// Close current connection
	if m.activeConn != nil {
		m.activeConn.Close()
		m.activeConn = nil
	}

	return m.connectToNextEndpoint()
}

// Close closes all connections and stops the manager
func (m *PooledConnectionManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true
	m.cancel()
	close(m.healthCheckStop)

	if m.activeConn != nil {
		m.activeConn.Close()
		m.activeConn = nil
	}

	for _, state := range m.endpoints {
		state.State = ConnectionStateClosed
	}

	return nil
}

// Drain drains the connection (waits for pending operations)
func (m *PooledConnectionManager) Drain() error {
	m.mu.RLock()
	conn := m.activeConn
	m.mu.RUnlock()

	if conn == nil {
		return nil
	}

	return conn.Drain()
}

// Flush flushes pending data to the server
func (m *PooledConnectionManager) Flush() error {
	m.mu.RLock()
	conn := m.activeConn
	m.mu.RUnlock()

	if conn == nil {
		return errors.New("not connected")
	}

	return conn.Flush()
}

// Stats returns connection statistics
func (m *PooledConnectionManager) Stats() nats.Statistics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.activeConn == nil {
		return nats.Statistics{}
	}

	return m.activeConn.Stats()
}

// RTT returns the round-trip time to the server
func (m *PooledConnectionManager) RTT() (time.Duration, error) {
	m.mu.RLock()
	conn := m.activeConn
	m.mu.RUnlock()

	if conn == nil {
		return 0, errors.New("not connected")
	}

	return conn.RTT()
}
