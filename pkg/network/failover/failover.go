// Package failover provides automatic IPv4/IPv6 failover functionality
package failover

import (
	"context"
	"fmt"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Protocol represents an IP protocol version
type Protocol string

const (
	// ProtocolIPv4 represents IPv4
	ProtocolIPv4 Protocol = "ipv4"
	// ProtocolIPv6 represents IPv6
	ProtocolIPv6 Protocol = "ipv6"
	// ProtocolAny allows any protocol
	ProtocolAny Protocol = "any"
)

// Strategy defines the failover strategy
type Strategy string

const (
	// StrategyPreferIPv6 prefers IPv6, falls back to IPv4
	StrategyPreferIPv6 Strategy = "prefer_ipv6"
	// StrategyPreferIPv4 prefers IPv4, falls back to IPv6
	StrategyPreferIPv4 Strategy = "prefer_ipv4"
	// StrategyHappyEyeballs uses RFC 8305 Happy Eyeballs algorithm
	StrategyHappyEyeballs Strategy = "happy_eyeballs"
	// StrategyRoundRobin alternates between protocols
	StrategyRoundRobin Strategy = "round_robin"
	// StrategyFastest races both and uses the fastest
	StrategyFastest Strategy = "fastest"
)

// EndpointStatus represents the health status of an endpoint
type EndpointStatus string

const (
	StatusUnknown  EndpointStatus = "unknown"
	StatusHealthy  EndpointStatus = "healthy"
	StatusUnhealthy EndpointStatus = "unhealthy"
	StatusDegraded EndpointStatus = "degraded"
)

// Endpoint represents a network endpoint
type Endpoint struct {
	// Address is the IP address or hostname
	Address string

	// Port is the port number
	Port int

	// Protocol is ipv4 or ipv6
	Protocol Protocol

	// Priority for ordering (lower = higher priority)
	Priority int

	// Weight for load balancing (higher = more traffic)
	Weight int

	// LastChecked is when the endpoint was last health checked
	LastChecked time.Time

	// Status is the current health status
	Status EndpointStatus

	// Latency is the last measured latency
	Latency time.Duration

	// ConsecutiveFailures tracks failures for circuit breaking
	ConsecutiveFailures int

	// Metadata contains additional endpoint info
	Metadata map[string]string
}

// String returns the endpoint address string
func (e *Endpoint) String() string {
	if e.Protocol == ProtocolIPv6 {
		return fmt.Sprintf("[%s]:%d", e.Address, e.Port)
	}
	return fmt.Sprintf("%s:%d", e.Address, e.Port)
}

// IsIPv6 returns true if the endpoint uses IPv6
func (e *Endpoint) IsIPv6() bool {
	return e.Protocol == ProtocolIPv6
}

// Config configures the failover behavior
type Config struct {
	// Strategy is the failover strategy
	Strategy Strategy

	// ConnectTimeout for connection attempts
	ConnectTimeout time.Duration

	// HealthCheckInterval between health checks
	HealthCheckInterval time.Duration

	// HealthCheckTimeout for health check operations
	HealthCheckTimeout time.Duration

	// MaxConsecutiveFailures before marking unhealthy
	MaxConsecutiveFailures int

	// HappyEyeballsDelay for RFC 8305 (250ms recommended)
	HappyEyeballsDelay time.Duration

	// RetryAttempts per endpoint
	RetryAttempts int

	// RetryDelay between retries
	RetryDelay time.Duration

	// CircuitBreakerEnabled enables circuit breaking
	CircuitBreakerEnabled bool

	// CircuitBreakerThreshold failures before opening
	CircuitBreakerThreshold int

	// CircuitBreakerRecoveryTime before retrying
	CircuitBreakerRecoveryTime time.Duration
}

// DefaultConfig returns a default failover configuration
func DefaultConfig() *Config {
	return &Config{
		Strategy:                   StrategyHappyEyeballs,
		ConnectTimeout:             10 * time.Second,
		HealthCheckInterval:        30 * time.Second,
		HealthCheckTimeout:         5 * time.Second,
		MaxConsecutiveFailures:     3,
		HappyEyeballsDelay:         250 * time.Millisecond,
		RetryAttempts:              3,
		RetryDelay:                 1 * time.Second,
		CircuitBreakerEnabled:      true,
		CircuitBreakerThreshold:    5,
		CircuitBreakerRecoveryTime: 30 * time.Second,
	}
}

// Resolver resolves hostnames to endpoints with failover support
type Resolver struct {
	config    *Config
	endpoints map[string][]*Endpoint
	mu        sync.RWMutex
	stats     *Stats
	dialer    *net.Dialer
	rrCounter uint64 // for round-robin
}

// NewResolver creates a new failover resolver
func NewResolver(config *Config) *Resolver {
	if config == nil {
		config = DefaultConfig()
	}

	return &Resolver{
		config:    config,
		endpoints: make(map[string][]*Endpoint),
		stats:     NewStats(),
		dialer: &net.Dialer{
			Timeout: config.ConnectTimeout,
		},
	}
}

// ResolveHost resolves a hostname to endpoints
func (r *Resolver) ResolveHost(ctx context.Context, host string, port int) ([]*Endpoint, error) {
	r.mu.RLock()
	cached, ok := r.endpoints[host]
	r.mu.RUnlock()

	if ok && len(cached) > 0 {
		return r.sortEndpoints(cached), nil
	}

	// Resolve DNS
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed: %w", err)
	}

	endpoints := make([]*Endpoint, 0, len(ips))
	for _, ip := range ips {
		protocol := ProtocolIPv4
		if ip.IP.To4() == nil {
			protocol = ProtocolIPv6
		}

		endpoints = append(endpoints, &Endpoint{
			Address:  ip.IP.String(),
			Port:     port,
			Protocol: protocol,
			Status:   StatusUnknown,
			Weight:   1,
		})
	}

	// Set priorities based on strategy
	r.assignPriorities(endpoints)

	// Cache endpoints
	r.mu.Lock()
	r.endpoints[host] = endpoints
	r.mu.Unlock()

	return r.sortEndpoints(endpoints), nil
}

// Connect establishes a connection using the failover strategy
func (r *Resolver) Connect(ctx context.Context, host string, port int) (net.Conn, *Endpoint, error) {
	endpoints, err := r.ResolveHost(ctx, host, port)
	if err != nil {
		return nil, nil, err
	}

	if len(endpoints) == 0 {
		return nil, nil, fmt.Errorf("no endpoints available for %s", host)
	}

	switch r.config.Strategy {
	case StrategyHappyEyeballs:
		return r.connectHappyEyeballs(ctx, endpoints)
	case StrategyFastest:
		return r.connectFastest(ctx, endpoints)
	case StrategyRoundRobin:
		return r.connectRoundRobin(ctx, endpoints)
	default:
		return r.connectSequential(ctx, endpoints)
	}
}

func (r *Resolver) connectSequential(ctx context.Context, endpoints []*Endpoint) (net.Conn, *Endpoint, error) {
	var lastErr error

	for _, ep := range endpoints {
		if ep.Status == StatusUnhealthy && r.config.CircuitBreakerEnabled {
			continue
		}

		for attempt := 0; attempt < r.config.RetryAttempts; attempt++ {
			conn, err := r.dialEndpoint(ctx, ep)
			if err == nil {
				r.recordSuccess(ep)
				return conn, ep, nil
			}

			lastErr = err
			r.recordFailure(ep)

			if attempt < r.config.RetryAttempts-1 {
				select {
				case <-ctx.Done():
					return nil, nil, ctx.Err()
				case <-time.After(r.config.RetryDelay):
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("all endpoints failed: %w", lastErr)
}

func (r *Resolver) connectHappyEyeballs(ctx context.Context, endpoints []*Endpoint) (net.Conn, *Endpoint, error) {
	// Separate IPv6 and IPv4 endpoints
	var ipv6, ipv4 []*Endpoint
	for _, ep := range endpoints {
		if ep.Status == StatusUnhealthy && r.config.CircuitBreakerEnabled {
			continue
		}
		if ep.IsIPv6() {
			ipv6 = append(ipv6, ep)
		} else {
			ipv4 = append(ipv4, ep)
		}
	}

	// Interleave: IPv6, IPv4, IPv6, IPv4...
	var interleaved []*Endpoint
	for i := 0; i < len(ipv6) || i < len(ipv4); i++ {
		if i < len(ipv6) {
			interleaved = append(interleaved, ipv6[i])
		}
		if i < len(ipv4) {
			interleaved = append(interleaved, ipv4[i])
		}
	}

	if len(interleaved) == 0 {
		return nil, nil, fmt.Errorf("no healthy endpoints available")
	}

	// Try with staggered starts
	type result struct {
		conn net.Conn
		ep   *Endpoint
		err  error
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan result, len(interleaved))

	for i, ep := range interleaved {
		ep := ep
		delay := time.Duration(i) * r.config.HappyEyeballsDelay

		go func() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}

			conn, err := r.dialEndpoint(ctx, ep)
			select {
			case results <- result{conn, ep, err}:
			case <-ctx.Done():
				if conn != nil {
					conn.Close()
				}
			}
		}()
	}

	// Wait for first success or all failures
	var lastErr error
	for i := 0; i < len(interleaved); i++ {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case res := <-results:
			if res.err == nil {
				r.recordSuccess(res.ep)
				return res.conn, res.ep, nil
			}
			lastErr = res.err
			r.recordFailure(res.ep)
		}
	}

	return nil, nil, fmt.Errorf("all endpoints failed: %w", lastErr)
}

func (r *Resolver) connectFastest(ctx context.Context, endpoints []*Endpoint) (net.Conn, *Endpoint, error) {
	type result struct {
		conn net.Conn
		ep   *Endpoint
		err  error
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan result, len(endpoints))

	for _, ep := range endpoints {
		if ep.Status == StatusUnhealthy && r.config.CircuitBreakerEnabled {
			continue
		}

		ep := ep
		go func() {
			conn, err := r.dialEndpoint(ctx, ep)
			select {
			case results <- result{conn, ep, err}:
			case <-ctx.Done():
				if conn != nil {
					conn.Close()
				}
			}
		}()
	}

	// Return first success, close others
	var lastErr error
	pending := len(endpoints)
	for pending > 0 {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case res := <-results:
			pending--
			if res.err == nil {
				r.recordSuccess(res.ep)
				return res.conn, res.ep, nil
			}
			lastErr = res.err
			r.recordFailure(res.ep)
		}
	}

	return nil, nil, fmt.Errorf("all endpoints failed: %w", lastErr)
}

func (r *Resolver) connectRoundRobin(ctx context.Context, endpoints []*Endpoint) (net.Conn, *Endpoint, error) {
	n := len(endpoints)
	if n == 0 {
		return nil, nil, fmt.Errorf("no endpoints available")
	}

	// Get starting index
	start := int(atomic.AddUint64(&r.rrCounter, 1) % uint64(n))

	// Try endpoints starting from round-robin position
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		ep := endpoints[idx]

		if ep.Status == StatusUnhealthy && r.config.CircuitBreakerEnabled {
			continue
		}

		conn, err := r.dialEndpoint(ctx, ep)
		if err == nil {
			r.recordSuccess(ep)
			return conn, ep, nil
		}
		r.recordFailure(ep)
	}

	return nil, nil, fmt.Errorf("all endpoints failed")
}

func (r *Resolver) dialEndpoint(ctx context.Context, ep *Endpoint) (net.Conn, error) {
	addr := ep.String()
	start := time.Now()

	conn, err := r.dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	// Record latency
	ep.Latency = time.Since(start)
	return conn, nil
}

func (r *Resolver) assignPriorities(endpoints []*Endpoint) {
	switch r.config.Strategy {
	case StrategyPreferIPv6:
		for _, ep := range endpoints {
			if ep.IsIPv6() {
				ep.Priority = 0
			} else {
				ep.Priority = 1
			}
		}
	case StrategyPreferIPv4:
		for _, ep := range endpoints {
			if ep.IsIPv6() {
				ep.Priority = 1
			} else {
				ep.Priority = 0
			}
		}
	default:
		// Equal priority
		for _, ep := range endpoints {
			ep.Priority = 0
		}
	}
}

func (r *Resolver) sortEndpoints(endpoints []*Endpoint) []*Endpoint {
	sorted := make([]*Endpoint, len(endpoints))
	copy(sorted, endpoints)

	sort.SliceStable(sorted, func(i, j int) bool {
		// Primary: priority
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority < sorted[j].Priority
		}
		// Secondary: latency (if known)
		if sorted[i].Latency > 0 && sorted[j].Latency > 0 {
			return sorted[i].Latency < sorted[j].Latency
		}
		return false
	})

	return sorted
}

func (r *Resolver) recordSuccess(ep *Endpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ep.ConsecutiveFailures = 0
	ep.Status = StatusHealthy
	ep.LastChecked = time.Now()

	r.stats.RecordSuccess(ep.Protocol)
}

func (r *Resolver) recordFailure(ep *Endpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ep.ConsecutiveFailures++
	ep.LastChecked = time.Now()

	if ep.ConsecutiveFailures >= r.config.MaxConsecutiveFailures {
		ep.Status = StatusUnhealthy
	} else {
		ep.Status = StatusDegraded
	}

	r.stats.RecordFailure(ep.Protocol)
}

// HealthCheck performs health checks on all cached endpoints
func (r *Resolver) HealthCheck(ctx context.Context) {
	r.mu.RLock()
	allEndpoints := make([]*Endpoint, 0)
	for _, eps := range r.endpoints {
		allEndpoints = append(allEndpoints, eps...)
	}
	r.mu.RUnlock()

	for _, ep := range allEndpoints {
		checkCtx, cancel := context.WithTimeout(ctx, r.config.HealthCheckTimeout)
		conn, err := r.dialEndpoint(checkCtx, ep)
		cancel()

		if err != nil {
			r.recordFailure(ep)
		} else {
			conn.Close()
			r.recordSuccess(ep)
		}
	}
}

// Stats returns the current statistics
func (r *Resolver) Stats() *Stats {
	return r.stats
}

// ClearCache clears the endpoint cache
func (r *Resolver) ClearCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endpoints = make(map[string][]*Endpoint)
}

// Stats tracks failover statistics
type Stats struct {
	mu              sync.Mutex
	IPv4Connections int64
	IPv6Connections int64
	IPv4Failures    int64
	IPv6Failures    int64
	Failovers       int64
}

// NewStats creates a new stats tracker
func NewStats() *Stats {
	return &Stats{}
}

// RecordSuccess records a successful connection
func (s *Stats) RecordSuccess(protocol Protocol) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if protocol == ProtocolIPv6 {
		s.IPv6Connections++
	} else {
		s.IPv4Connections++
	}
}

// RecordFailure records a connection failure
func (s *Stats) RecordFailure(protocol Protocol) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if protocol == ProtocolIPv6 {
		s.IPv6Failures++
	} else {
		s.IPv4Failures++
	}
}

// RecordFailover records a failover event
func (s *Stats) RecordFailover() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Failovers++
}

// Snapshot returns a copy of current stats
func (s *Stats) Snapshot() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()

	return Stats{
		IPv4Connections: s.IPv4Connections,
		IPv6Connections: s.IPv6Connections,
		IPv4Failures:    s.IPv4Failures,
		IPv6Failures:    s.IPv6Failures,
		Failovers:       s.Failovers,
	}
}

// ParseAddress parses an address and returns the protocol
func ParseAddress(addr string) (Protocol, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return ProtocolAny, nil // hostname
	}

	if ip.To4() != nil {
		return ProtocolIPv4, nil
	}
	return ProtocolIPv6, nil
}

// IsIPv6Available checks if IPv6 connectivity is available
func IsIPv6Available() bool {
	// Try to connect to a well-known IPv6 address
	conn, err := net.DialTimeout("tcp6", "[2001:4860:4860::8888]:53", 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// IsIPv4Available checks if IPv4 connectivity is available
func IsIPv4Available() bool {
	// Try to connect to a well-known IPv4 address
	conn, err := net.DialTimeout("tcp4", "8.8.8.8:53", 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
