package nats

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// HealthStatus represents the health status of an endpoint
type HealthStatus int

const (
	// HealthStatusUnknown indicates health has not been checked
	HealthStatusUnknown HealthStatus = iota
	// HealthStatusHealthy indicates the endpoint is healthy
	HealthStatusHealthy
	// HealthStatusDegraded indicates the endpoint is partially healthy
	HealthStatusDegraded
	// HealthStatusUnhealthy indicates the endpoint is unhealthy
	HealthStatusUnhealthy
)

func (s HealthStatus) String() string {
	switch s {
	case HealthStatusUnknown:
		return "unknown"
	case HealthStatusHealthy:
		return "healthy"
	case HealthStatusDegraded:
		return "degraded"
	case HealthStatusUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// IsAvailable returns true if the status indicates the endpoint can accept connections
func (s HealthStatus) IsAvailable() bool {
	return s == HealthStatusHealthy || s == HealthStatusDegraded
}

// HealthCheckResult contains the result of a health check
type HealthCheckResult struct {
	Endpoint    *Endpoint
	Status      HealthStatus
	Latency     time.Duration
	CheckedAt   time.Time
	Error       error
	Details     map[string]interface{}
}

// HealthChecker checks the health of an endpoint
type HealthChecker interface {
	// Check performs a health check on the endpoint
	Check(ctx context.Context, endpoint *Endpoint) *HealthCheckResult

	// Name returns the checker name
	Name() string
}

// HealthConfig configures health checking
type HealthConfig struct {
	// CheckInterval is how often to run health checks
	CheckInterval time.Duration

	// Timeout is the health check timeout
	Timeout time.Duration

	// HealthyThreshold is consecutive successes to become healthy
	HealthyThreshold int

	// UnhealthyThreshold is consecutive failures to become unhealthy
	UnhealthyThreshold int

	// DegradedLatencyThreshold marks endpoint as degraded above this latency
	DegradedLatencyThreshold time.Duration
}

// DefaultHealthConfig returns sensible defaults
func DefaultHealthConfig() *HealthConfig {
	return &HealthConfig{
		CheckInterval:            30 * time.Second,
		Timeout:                  5 * time.Second,
		HealthyThreshold:         2,
		UnhealthyThreshold:       3,
		DegradedLatencyThreshold: 100 * time.Millisecond,
	}
}

// EndpointHealth tracks the health of an endpoint
type EndpointHealth struct {
	Endpoint           *Endpoint
	Status             HealthStatus
	LastCheck          time.Time
	LastHealthy        time.Time
	LastError          error
	ConsecutiveSuccess int
	ConsecutiveFailure int
	Latency            time.Duration
	LatencyP50         time.Duration
	LatencyP95         time.Duration
	LatencyP99         time.Duration
	CheckCount         int64
	SuccessCount       int64
	FailureCount       int64
	latencies          []time.Duration // Recent latencies for percentile calculation
}

// SuccessRate returns the health check success rate
func (h *EndpointHealth) SuccessRate() float64 {
	total := h.SuccessCount + h.FailureCount
	if total == 0 {
		return 1.0
	}
	return float64(h.SuccessCount) / float64(total)
}

// Score returns a composite health score (0.0 to 1.0)
func (h *EndpointHealth) Score() float64 {
	if h.Status == HealthStatusUnhealthy {
		return 0.0
	}

	// Start with success rate
	score := h.SuccessRate()

	// Adjust for current status
	switch h.Status {
	case HealthStatusHealthy:
		// No adjustment
	case HealthStatusDegraded:
		score *= 0.75
	case HealthStatusUnknown:
		score *= 0.5
	}

	return score
}

// recordLatency records a latency sample and updates percentiles
func (h *EndpointHealth) recordLatency(latency time.Duration) {
	h.Latency = latency

	// Keep last 100 latencies for percentile calculation
	h.latencies = append(h.latencies, latency)
	if len(h.latencies) > 100 {
		h.latencies = h.latencies[1:]
	}

	// Calculate percentiles
	if len(h.latencies) > 0 {
		sorted := make([]time.Duration, len(h.latencies))
		copy(sorted, h.latencies)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

		h.LatencyP50 = sorted[len(sorted)/2]
		h.LatencyP95 = sorted[int(float64(len(sorted))*0.95)]
		h.LatencyP99 = sorted[int(float64(len(sorted))*0.99)]
	}
}

// HealthTracker tracks health of multiple endpoints
type HealthTracker struct {
	config    *HealthConfig
	checker   HealthChecker
	endpoints map[string]*EndpointHealth
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	stopCh    chan struct{}
	running   bool
}

// NewHealthTracker creates a new health tracker
func NewHealthTracker(config *HealthConfig, checker HealthChecker) *HealthTracker {
	if config == nil {
		config = DefaultHealthConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &HealthTracker{
		config:    config,
		checker:   checker,
		endpoints: make(map[string]*EndpointHealth),
		ctx:       ctx,
		cancel:    cancel,
		stopCh:    make(chan struct{}),
	}
}

// AddEndpoint adds an endpoint to track
func (t *HealthTracker) AddEndpoint(endpoint *Endpoint) {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := endpoint.Address()
	if _, exists := t.endpoints[key]; !exists {
		t.endpoints[key] = &EndpointHealth{
			Endpoint: endpoint,
			Status:   HealthStatusUnknown,
		}
	}
}

// RemoveEndpoint removes an endpoint from tracking
func (t *HealthTracker) RemoveEndpoint(endpoint *Endpoint) {
	t.mu.Lock()
	defer t.mu.Unlock()

	delete(t.endpoints, endpoint.Address())
}

// GetHealth returns the health of an endpoint
func (t *HealthTracker) GetHealth(endpoint *Endpoint) *EndpointHealth {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if h, ok := t.endpoints[endpoint.Address()]; ok {
		// Return a copy to avoid data races
		healthCopy := *h
		return &healthCopy
	}
	return nil
}

// GetAllHealth returns health of all endpoints
func (t *HealthTracker) GetAllHealth() []*EndpointHealth {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*EndpointHealth, 0, len(t.endpoints))
	for _, h := range t.endpoints {
		healthCopy := *h
		result = append(result, &healthCopy)
	}
	return result
}

// GetHealthyEndpoints returns all healthy endpoints
func (t *HealthTracker) GetHealthyEndpoints() []*Endpoint {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var result []*Endpoint
	for _, h := range t.endpoints {
		if h.Status.IsAvailable() {
			result = append(result, h.Endpoint)
		}
	}
	return result
}

// Start starts the background health check loop
func (t *HealthTracker) Start() {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	t.running = true
	t.mu.Unlock()

	go t.runHealthChecks()
}

// Stop stops the health check loop
func (t *HealthTracker) Stop() {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return
	}
	t.running = false
	t.mu.Unlock()

	t.cancel()
	close(t.stopCh)
}

// runHealthChecks runs periodic health checks
func (t *HealthTracker) runHealthChecks() {
	ticker := time.NewTicker(t.config.CheckInterval)
	defer ticker.Stop()

	// Run initial check
	t.checkAllEndpoints()

	for {
		select {
		case <-ticker.C:
			t.checkAllEndpoints()
		case <-t.stopCh:
			return
		case <-t.ctx.Done():
			return
		}
	}
}

// checkAllEndpoints checks health of all tracked endpoints
func (t *HealthTracker) checkAllEndpoints() {
	t.mu.RLock()
	endpoints := make([]*Endpoint, 0, len(t.endpoints))
	for _, h := range t.endpoints {
		endpoints = append(endpoints, h.Endpoint)
	}
	t.mu.RUnlock()

	var wg sync.WaitGroup
	for _, ep := range endpoints {
		wg.Add(1)
		go func(endpoint *Endpoint) {
			defer wg.Done()
			t.checkEndpoint(endpoint)
		}(ep)
	}
	wg.Wait()
}

// checkEndpoint performs a health check on a single endpoint
func (t *HealthTracker) checkEndpoint(endpoint *Endpoint) {
	ctx, cancel := context.WithTimeout(t.ctx, t.config.Timeout)
	defer cancel()

	result := t.checker.Check(ctx, endpoint)

	t.mu.Lock()
	defer t.mu.Unlock()

	h, ok := t.endpoints[endpoint.Address()]
	if !ok {
		return
	}

	h.LastCheck = result.CheckedAt
	h.CheckCount++

	if result.Error != nil {
		h.LastError = result.Error
		h.FailureCount++
		h.ConsecutiveFailure++
		h.ConsecutiveSuccess = 0

		// Check if should become unhealthy
		if h.ConsecutiveFailure >= t.config.UnhealthyThreshold {
			h.Status = HealthStatusUnhealthy
		}
	} else {
		h.SuccessCount++
		h.ConsecutiveSuccess++
		h.ConsecutiveFailure = 0
		h.LastHealthy = result.CheckedAt
		h.recordLatency(result.Latency)

		// Determine status based on thresholds
		if h.ConsecutiveSuccess >= t.config.HealthyThreshold {
			if result.Latency > t.config.DegradedLatencyThreshold {
				h.Status = HealthStatusDegraded
			} else {
				h.Status = HealthStatusHealthy
			}
		}
	}
}

// CheckNow performs an immediate health check on an endpoint
func (t *HealthTracker) CheckNow(endpoint *Endpoint) *HealthCheckResult {
	ctx, cancel := context.WithTimeout(t.ctx, t.config.Timeout)
	defer cancel()

	result := t.checker.Check(ctx, endpoint)

	// Update tracking (async to not block caller)
	go func() {
		t.mu.Lock()
		defer t.mu.Unlock()

		if h, ok := t.endpoints[endpoint.Address()]; ok {
			h.LastCheck = result.CheckedAt
			h.CheckCount++
			if result.Error == nil {
				h.recordLatency(result.Latency)
			}
		}
	}()

	return result
}

// HealthBasedRouter routes to endpoints based on health
type HealthBasedRouter struct {
	tracker    *HealthTracker
	strategy   RoutingStrategy
	mu         sync.RWMutex
}

// RoutingStrategy defines how to select an endpoint
type RoutingStrategy int

const (
	// RoutingStrategyPriority selects by priority then health
	RoutingStrategyPriority RoutingStrategy = iota
	// RoutingStrategyRoundRobin rotates through healthy endpoints
	RoutingStrategyRoundRobin
	// RoutingStrategyLeastLatency selects lowest latency endpoint
	RoutingStrategyLeastLatency
	// RoutingStrategyWeighted selects based on weight and health
	RoutingStrategyWeighted
	// RoutingStrategyRandom selects randomly from healthy endpoints
	RoutingStrategyRandom
)

func (s RoutingStrategy) String() string {
	switch s {
	case RoutingStrategyPriority:
		return "priority"
	case RoutingStrategyRoundRobin:
		return "round-robin"
	case RoutingStrategyLeastLatency:
		return "least-latency"
	case RoutingStrategyWeighted:
		return "weighted"
	case RoutingStrategyRandom:
		return "random"
	default:
		return "unknown"
	}
}

// NewHealthBasedRouter creates a new health-based router
func NewHealthBasedRouter(tracker *HealthTracker, strategy RoutingStrategy) *HealthBasedRouter {
	return &HealthBasedRouter{
		tracker:  tracker,
		strategy: strategy,
	}
}

// SelectEndpoint selects the best endpoint based on health and strategy
func (r *HealthBasedRouter) SelectEndpoint() *Endpoint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	healthy := r.tracker.GetHealthyEndpoints()
	if len(healthy) == 0 {
		return nil
	}

	switch r.strategy {
	case RoutingStrategyPriority:
		return r.selectByPriority(healthy)
	case RoutingStrategyLeastLatency:
		return r.selectByLeastLatency(healthy)
	case RoutingStrategyWeighted:
		return r.selectByWeight(healthy)
	default:
		// Default to priority
		return r.selectByPriority(healthy)
	}
}

// selectByPriority selects the highest priority (lowest number) healthy endpoint
func (r *HealthBasedRouter) selectByPriority(endpoints []*Endpoint) *Endpoint {
	if len(endpoints) == 0 {
		return nil
	}

	best := endpoints[0]
	for _, ep := range endpoints[1:] {
		if ep.Priority < best.Priority {
			best = ep
		}
	}
	return best
}

// selectByLeastLatency selects the endpoint with lowest latency
func (r *HealthBasedRouter) selectByLeastLatency(endpoints []*Endpoint) *Endpoint {
	if len(endpoints) == 0 {
		return nil
	}

	var best *Endpoint
	var bestLatency time.Duration = time.Hour

	for _, ep := range endpoints {
		health := r.tracker.GetHealth(ep)
		if health == nil {
			continue
		}
		if health.Latency < bestLatency {
			bestLatency = health.Latency
			best = ep
		}
	}

	if best == nil {
		return endpoints[0]
	}
	return best
}

// selectByWeight selects based on endpoint weight and health score
func (r *HealthBasedRouter) selectByWeight(endpoints []*Endpoint) *Endpoint {
	if len(endpoints) == 0 {
		return nil
	}

	// Calculate weighted scores
	type scored struct {
		endpoint *Endpoint
		score    float64
	}

	scores := make([]scored, 0, len(endpoints))
	for _, ep := range endpoints {
		health := r.tracker.GetHealth(ep)
		score := float64(ep.Weight)
		if health != nil {
			score *= health.Score()
		}
		scores = append(scores, scored{ep, score})
	}

	// Select highest score
	best := scores[0]
	for _, s := range scores[1:] {
		if s.score > best.score {
			best = s
		}
	}

	return best.endpoint
}

// SelectEndpoints returns all available endpoints sorted by preference
func (r *HealthBasedRouter) SelectEndpoints() []*Endpoint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	healthy := r.tracker.GetHealthyEndpoints()
	if len(healthy) == 0 {
		return nil
	}

	// Sort based on strategy
	switch r.strategy {
	case RoutingStrategyPriority:
		sort.Slice(healthy, func(i, j int) bool {
			return healthy[i].Priority < healthy[j].Priority
		})
	case RoutingStrategyLeastLatency:
		sort.Slice(healthy, func(i, j int) bool {
			hi := r.tracker.GetHealth(healthy[i])
			hj := r.tracker.GetHealth(healthy[j])
			if hi == nil || hj == nil {
				return false
			}
			return hi.Latency < hj.Latency
		})
	case RoutingStrategyWeighted:
		sort.Slice(healthy, func(i, j int) bool {
			hi := r.tracker.GetHealth(healthy[i])
			hj := r.tracker.GetHealth(healthy[j])
			scorei := float64(healthy[i].Weight)
			scorej := float64(healthy[j].Weight)
			if hi != nil {
				scorei *= hi.Score()
			}
			if hj != nil {
				scorej *= hj.Score()
			}
			return scorei > scorej
		})
	}

	return healthy
}

// SetStrategy changes the routing strategy
func (r *HealthBasedRouter) SetStrategy(strategy RoutingStrategy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.strategy = strategy
}

// GetStrategy returns the current routing strategy
func (r *HealthBasedRouter) GetStrategy() RoutingStrategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.strategy
}

// PingHealthChecker checks health by pinging the NATS server
type PingHealthChecker struct {
	connManager *PooledConnectionManager
}

// NewPingHealthChecker creates a new ping-based health checker
func NewPingHealthChecker(connManager *PooledConnectionManager) *PingHealthChecker {
	return &PingHealthChecker{connManager: connManager}
}

func (c *PingHealthChecker) Name() string {
	return "ping"
}

func (c *PingHealthChecker) Check(ctx context.Context, endpoint *Endpoint) *HealthCheckResult {
	result := &HealthCheckResult{
		Endpoint:  endpoint,
		CheckedAt: time.Now(),
		Status:    HealthStatusUnknown,
	}

	conn := c.connManager.Connection()
	if conn == nil {
		result.Status = HealthStatusUnhealthy
		result.Error = errors.New("not connected")
		return result
	}

	start := time.Now()
	rtt, err := conn.RTT()
	result.Latency = time.Since(start)

	if err != nil {
		result.Status = HealthStatusUnhealthy
		result.Error = err
		return result
	}

	result.Status = HealthStatusHealthy
	result.Latency = rtt
	result.Details = map[string]interface{}{
		"rtt": rtt.String(),
	}

	return result
}

// NoOpHealthChecker always returns healthy (for testing)
type NoOpHealthChecker struct{}

func (c *NoOpHealthChecker) Name() string {
	return "noop"
}

func (c *NoOpHealthChecker) Check(ctx context.Context, endpoint *Endpoint) *HealthCheckResult {
	return &HealthCheckResult{
		Endpoint:  endpoint,
		Status:    HealthStatusHealthy,
		Latency:   time.Millisecond,
		CheckedAt: time.Now(),
	}
}
