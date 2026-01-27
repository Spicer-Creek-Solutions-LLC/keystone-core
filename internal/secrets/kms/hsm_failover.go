// Package kms provides HSM failover and load balancing capabilities.
package kms

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

// LoadBalancingStrategy defines how requests are distributed across HSMs.
type LoadBalancingStrategy string

const (
	// LBStrategyRoundRobin distributes requests in round-robin fashion.
	LBStrategyRoundRobin LoadBalancingStrategy = "round_robin"
	// LBStrategyRandom distributes requests randomly.
	LBStrategyRandom LoadBalancingStrategy = "random"
	// LBStrategyLeastConnections routes to the HSM with fewest active connections.
	LBStrategyLeastConnections LoadBalancingStrategy = "least_connections"
	// LBStrategyWeighted distributes requests based on configured weights.
	LBStrategyWeighted LoadBalancingStrategy = "weighted"
	// LBStrategyLatencyBased routes to the HSM with lowest latency.
	LBStrategyLatencyBased LoadBalancingStrategy = "latency_based"
)

// HSMClusterConfig contains configuration for an HSM cluster with failover.
type HSMClusterConfig struct {
	// Name is the cluster name.
	Name string `json:"name"`

	// Strategy is the load balancing strategy.
	Strategy LoadBalancingStrategy `json:"strategy,omitempty"`

	// HealthCheckInterval is the interval between health checks.
	HealthCheckInterval time.Duration `json:"health_check_interval,omitempty"`

	// FailoverThreshold is the number of consecutive failures before failover.
	FailoverThreshold int `json:"failover_threshold,omitempty"`

	// RecoveryThreshold is the number of consecutive successes before recovery.
	RecoveryThreshold int `json:"recovery_threshold,omitempty"`

	// CircuitBreakerTimeout is the timeout before attempting recovery.
	CircuitBreakerTimeout time.Duration `json:"circuit_breaker_timeout,omitempty"`

	// RequestTimeout is the timeout for individual requests.
	RequestTimeout time.Duration `json:"request_timeout,omitempty"`

	// RetryAttempts is the number of retry attempts across HSMs.
	RetryAttempts int `json:"retry_attempts,omitempty"`

	// RetryDelay is the delay between retry attempts.
	RetryDelay time.Duration `json:"retry_delay,omitempty"`
}

// DefaultHSMClusterConfig returns default cluster configuration.
func DefaultHSMClusterConfig() *HSMClusterConfig {
	return &HSMClusterConfig{
		Name:                  "default",
		Strategy:              LBStrategyRoundRobin,
		HealthCheckInterval:   30 * time.Second,
		FailoverThreshold:     3,
		RecoveryThreshold:     2,
		CircuitBreakerTimeout: 60 * time.Second,
		RequestTimeout:        30 * time.Second,
		RetryAttempts:         2,
		RetryDelay:            500 * time.Millisecond,
	}
}

// HSMNodeState represents the state of an HSM node.
type HSMNodeState int

const (
	HSMNodeStateHealthy HSMNodeState = iota
	HSMNodeStateDegraded
	HSMNodeStateUnhealthy
	HSMNodeStateCircuitOpen
)

func (s HSMNodeState) String() string {
	switch s {
	case HSMNodeStateHealthy:
		return "healthy"
	case HSMNodeStateDegraded:
		return "degraded"
	case HSMNodeStateUnhealthy:
		return "unhealthy"
	case HSMNodeStateCircuitOpen:
		return "circuit_open"
	default:
		return "unknown"
	}
}

// HSMNode represents a single HSM in a cluster.
type HSMNode struct {
	Name     string        `json:"name"`
	Provider Provider      `json:"-"`
	Weight   int           `json:"weight,omitempty"`
	Priority int           `json:"priority,omitempty"`

	mu                 sync.RWMutex
	state              HSMNodeState
	consecutiveFailures int
	consecutiveSuccesses int
	lastFailure        time.Time
	lastSuccess        time.Time
	circuitOpenedAt    time.Time
	activeConnections  int32
	totalRequests      uint64
	totalErrors        uint64
	avgLatency         time.Duration
	latencySum         time.Duration
	latencyCount       uint64
}

// NewHSMNode creates a new HSM node.
func NewHSMNode(name string, provider Provider, weight, priority int) *HSMNode {
	if weight <= 0 {
		weight = 1
	}
	return &HSMNode{
		Name:     name,
		Provider: provider,
		Weight:   weight,
		Priority: priority,
		state:    HSMNodeStateHealthy,
	}
}

// State returns the current node state.
func (n *HSMNode) State() HSMNodeState {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.state
}

// IsAvailable returns true if the node can accept requests.
func (n *HSMNode) IsAvailable() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.state == HSMNodeStateHealthy || n.state == HSMNodeStateDegraded
}

// ActiveConnections returns the number of active connections.
func (n *HSMNode) ActiveConnections() int32 {
	return atomic.LoadInt32(&n.activeConnections)
}

// RecordSuccess records a successful request.
func (n *HSMNode) RecordSuccess(latency time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.lastSuccess = time.Now()
	n.consecutiveSuccesses++
	n.consecutiveFailures = 0
	n.totalRequests++

	n.latencySum += latency
	n.latencyCount++
	n.avgLatency = n.latencySum / time.Duration(n.latencyCount)

	if n.state == HSMNodeStateDegraded || n.state == HSMNodeStateCircuitOpen {
		n.state = HSMNodeStateHealthy
	}
}

// RecordFailure records a failed request.
func (n *HSMNode) RecordFailure(threshold int, circuitTimeout time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.lastFailure = time.Now()
	n.consecutiveFailures++
	n.consecutiveSuccesses = 0
	n.totalErrors++
	n.totalRequests++

	if n.consecutiveFailures >= threshold {
		if n.state != HSMNodeStateCircuitOpen {
			n.state = HSMNodeStateCircuitOpen
			n.circuitOpenedAt = time.Now()
		}
	} else if n.consecutiveFailures >= threshold/2 {
		n.state = HSMNodeStateDegraded
	}
}

// TryRecovery attempts to recover a node from circuit open state.
func (n *HSMNode) TryRecovery(circuitTimeout time.Duration) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.state != HSMNodeStateCircuitOpen {
		return true
	}

	if time.Since(n.circuitOpenedAt) < circuitTimeout {
		return false
	}

	n.state = HSMNodeStateDegraded
	return true
}

// Stats returns node statistics.
func (n *HSMNode) Stats() HSMNodeStats {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return HSMNodeStats{
		Name:                 n.Name,
		State:                n.state.String(),
		TotalRequests:        n.totalRequests,
		TotalErrors:          n.totalErrors,
		ConsecutiveFailures:  n.consecutiveFailures,
		ConsecutiveSuccesses: n.consecutiveSuccesses,
		ActiveConnections:    atomic.LoadInt32(&n.activeConnections),
		AverageLatency:       n.avgLatency,
		LastSuccess:          n.lastSuccess,
		LastFailure:          n.lastFailure,
	}
}

// HSMNodeStats contains statistics for an HSM node.
type HSMNodeStats struct {
	Name                 string        `json:"name"`
	State                string        `json:"state"`
	TotalRequests        uint64        `json:"total_requests"`
	TotalErrors          uint64        `json:"total_errors"`
	ConsecutiveFailures  int           `json:"consecutive_failures"`
	ConsecutiveSuccesses int           `json:"consecutive_successes"`
	ActiveConnections    int32         `json:"active_connections"`
	AverageLatency       time.Duration `json:"average_latency"`
	LastSuccess          time.Time     `json:"last_success"`
	LastFailure          time.Time     `json:"last_failure"`
}

// HSMCluster manages a cluster of HSM nodes with failover and load balancing.
type HSMCluster struct {
	config *HSMClusterConfig
	nodes  []*HSMNode

	mu           sync.RWMutex
	roundRobinIdx uint64
	totalWeights int

	stopCh  chan struct{}
	stopped bool
}

// NewHSMCluster creates a new HSM cluster.
func NewHSMCluster(config *HSMClusterConfig) *HSMCluster {
	if config == nil {
		config = DefaultHSMClusterConfig()
	}

	// Apply defaults for zero values
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 30 * time.Second
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}

	cluster := &HSMCluster{
		config: config,
		nodes:  make([]*HSMNode, 0),
		stopCh: make(chan struct{}),
	}

	go cluster.healthCheckLoop()

	return cluster
}

// AddNode adds a node to the cluster.
func (c *HSMCluster) AddNode(node *HSMNode) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nodes = append(c.nodes, node)
	c.totalWeights += node.Weight
}

// RemoveNode removes a node from the cluster.
func (c *HSMCluster) RemoveNode(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, node := range c.nodes {
		if node.Name == name {
			c.totalWeights -= node.Weight
			c.nodes = append(c.nodes[:i], c.nodes[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("node %s not found", name)
}

// SelectNode selects a node based on the configured strategy.
func (c *HSMCluster) SelectNode(ctx context.Context) (*HSMNode, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	available := c.availableNodes()
	if len(available) == 0 {
		return nil, errors.New("no available HSM nodes")
	}

	switch c.config.Strategy {
	case LBStrategyRoundRobin:
		return c.selectRoundRobin(available), nil
	case LBStrategyRandom:
		return c.selectRandom(available), nil
	case LBStrategyLeastConnections:
		return c.selectLeastConnections(available), nil
	case LBStrategyWeighted:
		return c.selectWeighted(available), nil
	case LBStrategyLatencyBased:
		return c.selectLatencyBased(available), nil
	default:
		return c.selectRoundRobin(available), nil
	}
}

// availableNodes returns nodes that can accept requests.
func (c *HSMCluster) availableNodes() []*HSMNode {
	available := make([]*HSMNode, 0, len(c.nodes))
	for _, node := range c.nodes {
		if node.IsAvailable() || node.TryRecovery(c.config.CircuitBreakerTimeout) {
			available = append(available, node)
		}
	}
	return available
}

// selectRoundRobin selects a node using round-robin.
func (c *HSMCluster) selectRoundRobin(nodes []*HSMNode) *HSMNode {
	idx := atomic.AddUint64(&c.roundRobinIdx, 1) - 1
	return nodes[idx%uint64(len(nodes))]
}

// selectRandom selects a random node.
func (c *HSMCluster) selectRandom(nodes []*HSMNode) *HSMNode {
	return nodes[rand.IntN(len(nodes))]
}

// selectLeastConnections selects the node with fewest connections.
func (c *HSMCluster) selectLeastConnections(nodes []*HSMNode) *HSMNode {
	var minNode *HSMNode
	minConns := int32(1<<31 - 1)

	for _, node := range nodes {
		conns := node.ActiveConnections()
		if conns < minConns {
			minConns = conns
			minNode = node
		}
	}

	return minNode
}

// selectWeighted selects a node based on weights.
func (c *HSMCluster) selectWeighted(nodes []*HSMNode) *HSMNode {
	totalWeight := 0
	for _, node := range nodes {
		totalWeight += node.Weight
	}

	r := rand.IntN(totalWeight)
	for _, node := range nodes {
		r -= node.Weight
		if r < 0 {
			return node
		}
	}

	return nodes[0]
}

// selectLatencyBased selects the node with lowest latency.
func (c *HSMCluster) selectLatencyBased(nodes []*HSMNode) *HSMNode {
	var minNode *HSMNode
	var minLatency time.Duration = 1<<63 - 1

	for _, node := range nodes {
		node.mu.RLock()
		latency := node.avgLatency
		node.mu.RUnlock()

		if latency < minLatency {
			minLatency = latency
			minNode = node
		}
	}

	if minNode == nil {
		return nodes[0]
	}
	return minNode
}

// Execute executes a function on a selected node with failover.
func (c *HSMCluster) Execute(ctx context.Context, fn func(Provider) error) error {
	var lastErr error

	for attempt := 0; attempt <= c.config.RetryAttempts; attempt++ {
		node, err := c.SelectNode(ctx)
		if err != nil {
			lastErr = err
			continue
		}

		atomic.AddInt32(&node.activeConnections, 1)
		startTime := time.Now()

		// Create a timeout context for the operation
		_, cancel := context.WithTimeout(ctx, c.config.RequestTimeout)
		err = fn(node.Provider)
		cancel()

		atomic.AddInt32(&node.activeConnections, -1)
		latency := time.Since(startTime)

		if err == nil {
			node.RecordSuccess(latency)
			return nil
		}

		node.RecordFailure(c.config.FailoverThreshold, c.config.CircuitBreakerTimeout)
		lastErr = err

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.config.RetryDelay):
		}
	}

	return fmt.Errorf("all HSM nodes failed after %d attempts: %w", c.config.RetryAttempts+1, lastErr)
}

// Encrypt executes an encryption operation with failover.
func (c *HSMCluster) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
	var resp *EncryptResponse
	err := c.Execute(ctx, func(p Provider) error {
		var err error
		resp, err = p.Encrypt(ctx, req)
		return err
	})
	return resp, err
}

// Decrypt executes a decryption operation with failover.
func (c *HSMCluster) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
	var resp *DecryptResponse
	err := c.Execute(ctx, func(p Provider) error {
		var err error
		resp, err = p.Decrypt(ctx, req)
		return err
	})
	return resp, err
}

// GenerateDataKey generates a data key with failover.
func (c *HSMCluster) GenerateDataKey(ctx context.Context, req *GenerateDataKeyRequest) (*DataKey, error) {
	var resp *DataKey
	err := c.Execute(ctx, func(p Provider) error {
		var err error
		resp, err = p.GenerateDataKey(ctx, req)
		return err
	})
	return resp, err
}

// WrapKey wraps a key with failover.
func (c *HSMCluster) WrapKey(ctx context.Context, req *WrapKeyRequest) (*WrapKeyResponse, error) {
	var resp *WrapKeyResponse
	err := c.Execute(ctx, func(p Provider) error {
		var err error
		resp, err = p.WrapKey(ctx, req)
		return err
	})
	return resp, err
}

// UnwrapKey unwraps a key with failover.
func (c *HSMCluster) UnwrapKey(ctx context.Context, req *UnwrapKeyRequest) (*UnwrapKeyResponse, error) {
	var resp *UnwrapKeyResponse
	err := c.Execute(ctx, func(p Provider) error {
		var err error
		resp, err = p.UnwrapKey(ctx, req)
		return err
	})
	return resp, err
}

// Sign signs data with failover.
func (c *HSMCluster) Sign(ctx context.Context, req *SignRequest) (*SignResponse, error) {
	var resp *SignResponse
	err := c.Execute(ctx, func(p Provider) error {
		sp, ok := p.(SigningProvider)
		if !ok {
			return ErrUnsupportedOperation
		}
		var err error
		resp, err = sp.Sign(ctx, req)
		return err
	})
	return resp, err
}

// Verify verifies a signature with failover.
func (c *HSMCluster) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	var resp *VerifyResponse
	err := c.Execute(ctx, func(p Provider) error {
		sp, ok := p.(SigningProvider)
		if !ok {
			return ErrUnsupportedOperation
		}
		var err error
		resp, err = sp.Verify(ctx, req)
		return err
	})
	return resp, err
}

// healthCheckLoop periodically checks node health.
func (c *HSMCluster) healthCheckLoop() {
	ticker := time.NewTicker(c.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.performHealthChecks()
		}
	}
}

// performHealthChecks checks the health of all nodes.
func (c *HSMCluster) performHealthChecks() {
	c.mu.RLock()
	nodes := make([]*HSMNode, len(c.nodes))
	copy(nodes, c.nodes)
	c.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, node := range nodes {
		healthy := node.Provider.Healthy(ctx)
		if healthy {
			node.RecordSuccess(0)
		} else {
			node.RecordFailure(c.config.FailoverThreshold, c.config.CircuitBreakerTimeout)
		}
	}
}

// Stats returns statistics for all nodes.
func (c *HSMCluster) Stats() []HSMNodeStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := make([]HSMNodeStats, len(c.nodes))
	for i, node := range c.nodes {
		stats[i] = node.Stats()
	}

	return stats
}

// Healthy returns true if at least one node is available.
func (c *HSMCluster) Healthy(ctx context.Context) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, node := range c.nodes {
		if node.IsAvailable() {
			return true
		}
	}

	return false
}

// Close closes the cluster and all nodes.
func (c *HSMCluster) Close() error {
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return nil
	}
	c.stopped = true
	close(c.stopCh)
	nodes := c.nodes
	c.mu.Unlock()

	var firstErr error
	for _, node := range nodes {
		if err := node.Provider.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// HSMClusterProvider wraps an HSM cluster as a Provider.
type HSMClusterProvider struct {
	cluster *HSMCluster
	name    string
	ptype   ProviderType
}

// NewHSMClusterProvider creates a provider backed by an HSM cluster.
func NewHSMClusterProvider(name string, ptype ProviderType, cluster *HSMCluster) *HSMClusterProvider {
	return &HSMClusterProvider{
		cluster: cluster,
		name:    name,
		ptype:   ptype,
	}
}

// Type returns the provider type.
func (p *HSMClusterProvider) Type() ProviderType {
	return p.ptype
}

// Name returns the provider instance name.
func (p *HSMClusterProvider) Name() string {
	return p.name
}

// Healthy checks if the provider is healthy.
func (p *HSMClusterProvider) Healthy(ctx context.Context) bool {
	return p.cluster.Healthy(ctx)
}

// GetKeyMetadata retrieves metadata for a key.
func (p *HSMClusterProvider) GetKeyMetadata(ctx context.Context, keyID string) (*KeyMetadata, error) {
	var meta *KeyMetadata
	err := p.cluster.Execute(ctx, func(provider Provider) error {
		var err error
		meta, err = provider.GetKeyMetadata(ctx, keyID)
		return err
	})
	return meta, err
}

// Encrypt encrypts plaintext data.
func (p *HSMClusterProvider) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
	return p.cluster.Encrypt(ctx, req)
}

// Decrypt decrypts ciphertext data.
func (p *HSMClusterProvider) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
	return p.cluster.Decrypt(ctx, req)
}

// GenerateDataKey generates a data encryption key.
func (p *HSMClusterProvider) GenerateDataKey(ctx context.Context, req *GenerateDataKeyRequest) (*DataKey, error) {
	return p.cluster.GenerateDataKey(ctx, req)
}

// WrapKey wraps (encrypts) a key with the KMS key.
func (p *HSMClusterProvider) WrapKey(ctx context.Context, req *WrapKeyRequest) (*WrapKeyResponse, error) {
	return p.cluster.WrapKey(ctx, req)
}

// UnwrapKey unwraps (decrypts) a key with the KMS key.
func (p *HSMClusterProvider) UnwrapKey(ctx context.Context, req *UnwrapKeyRequest) (*UnwrapKeyResponse, error) {
	return p.cluster.UnwrapKey(ctx, req)
}

// Sign signs data with the HSM key.
func (p *HSMClusterProvider) Sign(ctx context.Context, req *SignRequest) (*SignResponse, error) {
	return p.cluster.Sign(ctx, req)
}

// Verify verifies a signature with the HSM key.
func (p *HSMClusterProvider) Verify(ctx context.Context, req *VerifyRequest) (*VerifyResponse, error) {
	return p.cluster.Verify(ctx, req)
}

// Close closes the provider.
func (p *HSMClusterProvider) Close() error {
	return p.cluster.Close()
}

// Ensure HSMClusterProvider implements the interfaces.
var (
	_ Provider        = (*HSMClusterProvider)(nil)
	_ SigningProvider = (*HSMClusterProvider)(nil)
)
