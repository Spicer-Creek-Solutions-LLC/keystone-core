// Package mirror implements mirror groups and geographic routing for file distribution.
package mirror

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// HealthMonitor monitors the health of mirrors in a group.
type HealthMonitor struct {
	group      *Group
	nc         *nats.Conn
	config     *HealthCheckConfig
	stopCh     chan struct{}
	wg         sync.WaitGroup
	callbacks  []HealthChangeCallback
	callbackMu sync.RWMutex
}

// HealthChangeCallback is called when mirror health changes.
type HealthChangeCallback func(mirrorID string, oldState, newState State)

// NewHealthMonitor creates a new health monitor.
func NewHealthMonitor(group *Group, nc *nats.Conn) *HealthMonitor {
	return &HealthMonitor{
		group:  group,
		nc:     nc,
		config: group.config.HealthCheck,
		stopCh: make(chan struct{}),
	}
}

// Start begins health monitoring.
func (m *HealthMonitor) Start() {
	m.wg.Add(1)
	go m.runHealthChecks()
}

// Stop stops health monitoring.
func (m *HealthMonitor) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// OnHealthChange registers a callback for health changes.
func (m *HealthMonitor) OnHealthChange(cb HealthChangeCallback) {
	m.callbackMu.Lock()
	defer m.callbackMu.Unlock()
	m.callbacks = append(m.callbacks, cb)
}

func (m *HealthMonitor) runHealthChecks() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.Interval)
	defer ticker.Stop()

	// Initial check
	m.checkAllMirrors()

	for {
		select {
		case <-ticker.C:
			m.checkAllMirrors()
		case <-m.stopCh:
			return
		}
	}
}

func (m *HealthMonitor) checkAllMirrors() {
	mirrors := m.group.GetMirrors()
	var wg sync.WaitGroup

	for _, mirror := range mirrors {
		if !mirror.Enabled {
			continue
		}

		wg.Add(1)
		go func(mir *Mirror) {
			defer wg.Done()
			m.checkMirror(mir)
		}(mirror)
	}

	wg.Wait()
}

func (m *HealthMonitor) checkMirror(mirror *Mirror) {
	// Get current state before check
	oldHealth, _ := m.group.GetHealth(mirror.ID)
	oldState := StateUnknown
	if oldHealth != nil {
		oldState = oldHealth.State
	}

	// Create health check subject
	subject := fmt.Sprintf("kscore.%s.files.health", mirror.ClusterID)
	if mirror.InstanceID != "" {
		subject = fmt.Sprintf("kscore.%s.files.%s.health", mirror.ClusterID, mirror.InstanceID)
	}

	// Perform health check with timeout
	ctx, cancel := context.WithTimeout(context.Background(), m.config.Timeout)
	defer cancel()

	start := time.Now()
	_, err := m.nc.RequestWithContext(ctx, subject, []byte(`{"type":"health_check"}`))
	latency := time.Since(start)

	// Update health
	var newState State
	if err != nil {
		newState = StateUnhealthy
		m.group.UpdateHealth(mirror.ID, newState, latency, err)
	} else {
		newState = StateHealthy
		m.group.UpdateHealth(mirror.ID, newState, latency, nil)
	}

	// Notify callbacks if state changed
	if oldState != newState {
		m.notifyHealthChange(mirror.ID, oldState, newState)
	}
}

func (m *HealthMonitor) notifyHealthChange(mirrorID string, oldState, newState State) {
	m.callbackMu.RLock()
	callbacks := make([]HealthChangeCallback, len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.callbackMu.RUnlock()

	for _, cb := range callbacks {
		cb(mirrorID, oldState, newState)
	}
}

// LatencyProber measures latency to mirrors.
type LatencyProber struct {
	group     *Group
	nc        *nats.Conn
	config    *LatencyProbeConfig
	router    *NearestRouter
	stopCh    chan struct{}
	wg        sync.WaitGroup
	latencies map[string]*latencyHistory
	latencyMu sync.RWMutex
}

type latencyHistory struct {
	samples    []time.Duration
	maxSamples int
	idx        int
	ema        time.Duration // Exponential moving average
	min        time.Duration
	max        time.Duration
	p50        time.Duration
	p95        time.Duration
	p99        time.Duration
}

// NewLatencyProber creates a new latency prober.
func NewLatencyProber(group *Group, nc *nats.Conn, router *NearestRouter) *LatencyProber {
	return &LatencyProber{
		group:     group,
		nc:        nc,
		config:    group.config.LatencyProbe,
		router:    router,
		stopCh:    make(chan struct{}),
		latencies: make(map[string]*latencyHistory),
	}
}

// Start begins latency probing.
func (p *LatencyProber) Start() {
	p.wg.Add(1)
	go p.runProbes()
}

// Stop stops latency probing.
func (p *LatencyProber) Stop() {
	close(p.stopCh)
	p.wg.Wait()
}

func (p *LatencyProber) runProbes() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.Interval)
	defer ticker.Stop()

	// Initial probe
	p.probeAllMirrors()

	for {
		select {
		case <-ticker.C:
			p.probeAllMirrors()
		case <-p.stopCh:
			return
		}
	}
}

func (p *LatencyProber) probeAllMirrors() {
	mirrors := p.group.GetMirrors()
	var wg sync.WaitGroup

	for _, mirror := range mirrors {
		if !mirror.Enabled {
			continue
		}

		wg.Add(1)
		go func(mir *Mirror) {
			defer wg.Done()
			p.probeMirror(mir)
		}(mirror)
	}

	wg.Wait()
}

func (p *LatencyProber) probeMirror(mirror *Mirror) {
	// Create probe subject
	subject := fmt.Sprintf("kscore.%s.files.probe", mirror.ClusterID)
	if mirror.InstanceID != "" {
		subject = fmt.Sprintf("kscore.%s.files.%s.probe", mirror.ClusterID, mirror.InstanceID)
	}

	// Perform probe with timeout
	timeout := 5 * time.Second
	if p.config.Interval < timeout {
		timeout = p.config.Interval / 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()

	// If probe file is configured, request it; otherwise just ping
	var payload []byte
	if p.config.ProbeFile != "" {
		payload = []byte(fmt.Sprintf(`{"type":"probe","file":%q}`, p.config.ProbeFile))
	} else {
		payload = []byte(`{"type":"ping"}`)
	}

	_, err := p.nc.RequestWithContext(ctx, subject, payload)
	latency := time.Since(start)

	if err == nil {
		p.recordLatency(mirror.ID, latency)
		if p.router != nil {
			p.router.UpdateLatency(mirror.ID, p.getEMA(mirror.ID))
		}
	}
}

func (p *LatencyProber) recordLatency(mirrorID string, latency time.Duration) {
	p.latencyMu.Lock()
	defer p.latencyMu.Unlock()

	hist, ok := p.latencies[mirrorID]
	if !ok {
		hist = &latencyHistory{
			samples:    make([]time.Duration, 100),
			maxSamples: 100,
			min:        latency,
			max:        latency,
		}
		p.latencies[mirrorID] = hist
	}

	// Record sample
	hist.samples[hist.idx%hist.maxSamples] = latency
	hist.idx++

	// Update min/max
	if latency < hist.min {
		hist.min = latency
	}
	if latency > hist.max {
		hist.max = latency
	}

	// Update EMA
	if hist.ema == 0 {
		hist.ema = latency
	} else {
		alpha := p.config.SmoothingFactor
		hist.ema = time.Duration(float64(hist.ema)*(1-alpha) + float64(latency)*alpha)
	}

	// Update percentiles periodically
	if hist.idx%10 == 0 {
		p.updatePercentiles(hist)
	}
}

func (p *LatencyProber) updatePercentiles(hist *latencyHistory) {
	count := hist.idx
	if count > hist.maxSamples {
		count = hist.maxSamples
	}

	if count == 0 {
		return
	}

	// Copy samples for sorting
	sorted := make([]time.Duration, count)
	for i := 0; i < count; i++ {
		sorted[i] = hist.samples[i]
	}

	// Simple insertion sort for small arrays
	for i := 1; i < len(sorted); i++ {
		j := i
		for j > 0 && sorted[j-1] > sorted[j] {
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
			j--
		}
	}

	hist.p50 = sorted[len(sorted)*50/100]
	hist.p95 = sorted[len(sorted)*95/100]
	hist.p99 = sorted[len(sorted)*99/100]
}

func (p *LatencyProber) getEMA(mirrorID string) time.Duration {
	p.latencyMu.RLock()
	defer p.latencyMu.RUnlock()

	hist, ok := p.latencies[mirrorID]
	if !ok {
		return 0
	}
	return hist.ema
}

// GetLatencyStats returns latency statistics for a mirror.
func (p *LatencyProber) GetLatencyStats(mirrorID string) *LatencyStats {
	p.latencyMu.RLock()
	defer p.latencyMu.RUnlock()

	hist, ok := p.latencies[mirrorID]
	if !ok {
		return nil
	}

	return &LatencyStats{
		MirrorID:   mirrorID,
		EMA:        hist.ema,
		Min:        hist.min,
		Max:        hist.max,
		P50:        hist.p50,
		P95:        hist.p95,
		P99:        hist.p99,
		SampleSize: hist.idx,
	}
}

// LatencyStats contains latency statistics for a mirror.
type LatencyStats struct {
	MirrorID   string        `json:"mirror_id"`
	EMA        time.Duration `json:"ema"`
	Min        time.Duration `json:"min"`
	Max        time.Duration `json:"max"`
	P50        time.Duration `json:"p50"`
	P95        time.Duration `json:"p95"`
	P99        time.Duration `json:"p99"`
	SampleSize int           `json:"sample_size"`
}

// CircuitBreaker provides circuit breaker pattern for mirrors.
type CircuitBreaker struct {
	mirrorID     string
	state        CircuitState
	failures     int
	successes    int
	threshold    int
	resetTimeout time.Duration
	lastFailure  time.Time
	mu           sync.RWMutex
}

// CircuitState represents circuit breaker state.
type CircuitState int

// CircuitClosed constants define the circuit states.
const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(mirrorID string, threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		mirrorID:     mirrorID,
		state:        CircuitClosed,
		threshold:    threshold,
		resetTimeout: resetTimeout,
	}
}

// Allow checks if requests should be allowed.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	}
	return true
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitHalfOpen {
		cb.successes++
		if cb.successes >= 2 {
			cb.state = CircuitClosed
			cb.failures = 0
			cb.successes = 0
		}
	} else {
		cb.failures = 0
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.state == CircuitHalfOpen {
		cb.state = CircuitOpen
		cb.successes = 0
	} else if cb.failures >= cb.threshold {
		cb.state = CircuitOpen
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the circuit breaker.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
}
