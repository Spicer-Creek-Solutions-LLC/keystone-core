// SPDX-License-Identifier: Apache-2.0

package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
)

// rttProbeInterval is how often ConnectionManager pings the active
// connection to record an RTT sample for the connected endpoint.
// 5s matches PROJECT-DETAILS §4.2's "default 5m" dedup window divided
// by ~60 — enough resolution for P50/P99 stability under v1.0 load
// without saturating the conn.
//
// A var, not a const, only so tests can shorten it to exercise the
// probe/shutdown interleaving deterministically; production never
// reassigns it.
var rttProbeInterval = 5 * time.Second

// rttProbeTimeout bounds a single RTT probe. nats.go RTT issues a
// PING and waits for PONG; on a healthy LAN it returns in <1ms, on a
// stalled connection it can hang indefinitely. The probe loop honors
// this ceiling and records a failure on timeout.
const rttProbeTimeout = 1 * time.Second

// ConnectionManager owns the *external-mode* NATS connection across
// multiple endpoints. v1.0 leans on nats.go's native multi-URL
// failover (passing the joined URL list to nats.Connect); per-endpoint
// state and breaker tracking layer on top via callbacks. Task 7 will
// switch to single-endpoint dialing so the breaker can actively evict
// OPEN endpoints from the dial list.
//
// Embedded mode does not use ConnectionManager — Manager owns the
// in-process conn directly.
type ConnectionManager struct {
	cfg       config.NATSConfig
	endpoints []Endpoint
	selector  StrategySelector
	log       *slog.Logger
	now       func() time.Time
	// rng feeds reconnectDelay's jitter calculation. nil-safe;
	// reconnectDelay falls back to un-jittered when nil. Tests
	// inject a deterministic *rand.Rand.
	rng *rand.Rand

	mu       sync.RWMutex
	started  bool
	stopped  bool
	conn     *nats.Conn
	states   map[string]*EndpointState
	breakers map[string]Breaker

	probeStop chan struct{}
	probeDone chan struct{}
}

// NewConnectionManager builds a manager from validated NATSConfig.
// External mode only — embedded mode callers stay on Manager.
func NewConnectionManager(cfg config.NATSConfig, log *slog.Logger) (*ConnectionManager, error) {
	if cfg.Mode != config.NATSModeExternal {
		return nil, fmt.Errorf("nats: ConnectionManager requires mode=external, got %q", string(cfg.Mode))
	}
	endpoints, err := endpointsFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.Default()
	}

	cm := &ConnectionManager{
		cfg:       cfg,
		endpoints: sortEndpointsByPriority(endpoints),
		selector:  NewStrategySelector(nil),
		log:       log,
		now:       time.Now,
		// math/rand/v2 is correct here — backoff jitter doesn't need
		// cryptographic entropy. crypto/rand would be wasteful.
		// #nosec G404 -- backoff jitter, not cryptographic.
		rng:      rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0xDEADBEEF)), //nolint:gosec // G404
		states:   make(map[string]*EndpointState, len(endpoints)),
		breakers: make(map[string]Breaker, len(endpoints)),
	}
	for _, e := range cm.endpoints {
		cm.states[e.URL] = newEndpointState(e.URL)
		cm.breakers[e.URL] = newBreaker(cfg.CircuitBreaker, cm.now)
	}
	return cm, nil
}

// endpointsFromConfig translates the user-facing config into the
// internal Endpoint slice. Endpoints (structured) are used as-is;
// URLs (simple) are translated to Priority=0, Weight=1 endpoints.
func endpointsFromConfig(cfg config.NATSConfig) ([]Endpoint, error) {
	if len(cfg.Endpoints) > 0 {
		out := make([]Endpoint, 0, len(cfg.Endpoints))
		for _, ec := range cfg.Endpoints {
			out = append(out, Endpoint{
				URL:      ec.URL,
				Scheme:   schemeFromURL(ec.URL),
				Priority: ec.Priority,
				Weight:   ec.Weight,
				Tags:     ec.Tags,
			})
		}
		return out, nil
	}
	if len(cfg.URLs) == 0 {
		return nil, errors.New("nats: no endpoints configured")
	}
	out := make([]Endpoint, 0, len(cfg.URLs))
	for _, u := range cfg.URLs {
		out = append(out, Endpoint{
			URL:      u,
			Scheme:   schemeFromURL(u),
			Priority: 0,
			Weight:   1,
		})
	}
	return out, nil
}

// Start opens the connection. v1.0 hands the full URL list to
// nats.Connect (priority-sorted) and registers callbacks that update
// per-endpoint state on Connect/Disconnect/Reconnect events. The
// active endpoint is whichever URL nats.Conn.ConnectedUrl() reports.
func (cm *ConnectionManager) Start(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.stopped {
		return errors.New("nats: connection manager already shut down")
	}
	if cm.started {
		return nil
	}

	urlList := make([]string, 0, len(cm.endpoints))
	for _, e := range cm.endpoints {
		urlList = append(urlList, e.URL)
	}
	if len(urlList) == 0 {
		return errors.New("nats: no endpoints to connect")
	}

	for _, e := range cm.endpoints {
		cm.states[e.URL].SetStatus(EndpointStatusConnecting, cm.now())
	}

	primary := cm.endpoints[0]
	strategy := cm.selector.Select(primary.Scheme)

	opts := buildClientOptions(cm.cfg,
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			cm.onDisconnect(err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			cm.onReconnect(c.ConnectedUrl())
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			cm.onClosed()
		}),
		// CustomReconnectDelay supersedes nats.ReconnectWait (the
		// constant). nats.go calls this with attempts ∈ [1, ∞) on
		// every reconnect try, so we log every attempt + emit an
		// exp-backoff-with-jitter delay. Epic 06 task 10.
		nats.CustomReconnectDelay(func(attempts int) time.Duration {
			d := reconnectDelay(attempts, cm.cfg.ReconnectWait, cm.cfg.MaxReconnectDelay, cm.cfg.ReconnectJitter, cm.rng)
			cm.log.Info("nats reconnect attempt",
				"attempt", attempts,
				"delay_ms", d.Milliseconds(),
				"max_reconnects", cm.cfg.MaxReconnects,
			)
			return d
		}),
		// ReconnectErrHandler fires on each *failed* reconnect
		// attempt — without this, operators only see "disconnected"
		// + "reconnected" with no visibility into the per-attempt
		// failure between them.
		nats.ReconnectErrHandler(func(_ *nats.Conn, err error) {
			cm.log.Warn("nats reconnect attempt failed", "err", err)
		}),
	)
	// Pass the joined list as a single Endpoint URL so the strategy
	// applies once (Direct vs TLS based on the *first* endpoint's
	// scheme). v1.0 does not support per-endpoint scheme mixing in
	// nats.go's native failover; flagged for Task 7's redesign.
	primaryWithList := Endpoint{
		URL:    strings.Join(urlList, ","),
		Scheme: primary.Scheme,
	}
	conn, err := strategy.Connect(primaryWithList, opts)
	if err != nil {
		for _, e := range cm.endpoints {
			cm.states[e.URL].SetStatus(EndpointStatusFailed, cm.now())
			cm.states[e.URL].RecordFailure(err)
		}
		return fmt.Errorf("nats: connect %v: %w", urlList, err)
	}
	cm.conn = conn
	cm.markActiveLocked(conn.ConnectedUrl())
	cm.started = true

	cm.probeStop = make(chan struct{})
	cm.probeDone = make(chan struct{})
	go cm.runRTTProbe()

	cm.log.InfoContext(ctx, "nats connection manager started",
		"endpoints", urlList,
		"active", conn.ConnectedUrl(),
	)
	return nil
}

// Shutdown closes the conn and stops the RTT probe. Idempotent; safe
// before Start.
func (cm *ConnectionManager) Shutdown(_ context.Context) error {
	cm.mu.Lock()
	if !cm.started || cm.stopped {
		cm.stopped = true
		cm.mu.Unlock()
		return nil
	}
	// Claim shutdown and snapshot the probe channels + conn under the
	// lock, then do the blocking work (waiting for the probe to exit,
	// closing the conn) WITHOUT holding cm.mu. runRTTProbe's probeOnce
	// takes cm.mu.RLock(), so waiting on <-probeDone under the write lock
	// deadlocks whenever a probe tick is in flight at shutdown. The probe
	// channels are left intact (runRTTProbe selects on cm.probeStop); the
	// stopped flag makes a concurrent or repeat Shutdown a no-op, so the
	// channel is closed exactly once.
	cm.stopped = true
	probeStop, probeDone := cm.probeStop, cm.probeDone
	conn := cm.conn
	cm.conn = nil
	cm.mu.Unlock()

	if probeStop != nil {
		close(probeStop)
		<-probeDone
	}
	if conn != nil {
		conn.Close()
	}
	return nil
}

// Health returns nil iff the connection is currently usable. A nil
// error means at least one endpoint is in EndpointStatusConnected
// AND the underlying conn reports IsConnected. Additionally, the
// breaker gating (Task 7): if every endpoint's circuit breaker is
// OPEN we return an error even if the conn is currently connected,
// because OPEN breakers signal sustained failure that /health/ready
// should surface as 503.
func (cm *ConnectionManager) Health(_ context.Context) error {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cm.stopped {
		return errors.New("nats: connection manager shut down")
	}
	if !cm.started || cm.conn == nil {
		return errors.New("nats: connection manager not started")
	}
	if !cm.conn.IsConnected() {
		return fmt.Errorf("nats: not connected (status=%s)", cm.conn.Status())
	}
	if cm.allBreakersOpenLocked() {
		return errors.New("nats: all endpoint circuit breakers are open")
	}
	return nil
}

// allBreakersOpenLocked reports whether every configured endpoint's
// breaker is in OPEN state. Returns false on an empty endpoint list
// (no breakers means we don't have a degraded-route signal).
// Caller holds cm.mu (read or write).
func (cm *ConnectionManager) allBreakersOpenLocked() bool {
	if len(cm.endpoints) == 0 {
		return false
	}
	for _, e := range cm.endpoints {
		if cm.breakers[e.URL].Status() != CircuitOpen {
			return false
		}
	}
	return true
}

// publishBytes sends pre-validated bytes on subject through the
// active conn. Package-private — Manager.PublishEnvelope is the
// public path, and ConnectionManager-direct publishes are not part
// of the v1.0 contract. Records per-endpoint success/failure for
// observability.
func (cm *ConnectionManager) publishBytes(_ context.Context, subject string, data []byte) error {
	cm.mu.RLock()
	conn := cm.conn
	stopped := cm.stopped
	started := cm.started
	cm.mu.RUnlock()

	if stopped {
		return errors.New("nats: connection manager shut down")
	}
	if !started || conn == nil {
		return errors.New("nats: connection manager not started")
	}
	if err := conn.Publish(subject, data); err != nil {
		if active := conn.ConnectedUrl(); active != "" {
			cm.recordFailure(active, err)
		}
		return fmt.Errorf("nats: publish %q: %w", subject, err)
	}
	if active := conn.ConnectedUrl(); active != "" {
		cm.recordSuccess(active)
	}
	return nil
}

// ActiveEndpoint returns the URL nats.Conn currently reports as
// connected (or "", false if disconnected).
func (cm *ConnectionManager) ActiveEndpoint() (string, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.conn == nil || !cm.conn.IsConnected() {
		return "", false
	}
	return cm.conn.ConnectedUrl(), true
}

// Snapshot returns a stable list of EndpointSnapshot — one per
// configured endpoint, ordered by priority. Used by /api/status (Task
// 11 wiring) and by tests.
func (cm *ConnectionManager) Snapshot() []EndpointSnapshot {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	out := make([]EndpointSnapshot, 0, len(cm.endpoints))
	for _, e := range cm.endpoints {
		st := cm.states[e.URL].Snapshot()
		st.Circuit = cm.breakers[e.URL].Status()
		out = append(out, st)
	}
	return out
}

// Endpoints returns the configured endpoints in priority order.
// Exposed for tests and for the bootstrap registration handler
// (Task 9) which needs the URL list to publish responses.
func (cm *ConnectionManager) Endpoints() []Endpoint {
	out := make([]Endpoint, len(cm.endpoints))
	copy(out, cm.endpoints)
	return out
}

func (cm *ConnectionManager) onDisconnect(err error) {
	cm.mu.Lock()
	active := ""
	if cm.conn != nil {
		active = cm.conn.ConnectedUrl()
	}
	if active != "" {
		if state, ok := cm.states[active]; ok {
			state.SetStatus(EndpointStatusDisconnected, cm.now())
			state.RecordFailure(err)
			cm.breakers[active].OnFailure()
		}
	}
	cm.mu.Unlock()
	cm.log.Warn("nats endpoint disconnected", "active", active, "err", err)
}

func (cm *ConnectionManager) onReconnect(activeURL string) {
	cm.mu.Lock()
	cm.markActiveLocked(activeURL)
	cm.mu.Unlock()
	cm.log.Info("nats endpoint reconnected", "active", activeURL)
}

func (cm *ConnectionManager) onClosed() {
	cm.mu.Lock()
	for _, state := range cm.states {
		state.SetStatus(EndpointStatusDisconnected, cm.now())
	}
	cm.mu.Unlock()
}

// markActiveLocked sets all endpoints to Disconnected except the
// active one which goes Connected. Caller holds cm.mu.
func (cm *ConnectionManager) markActiveLocked(activeURL string) {
	now := cm.now()
	for url, state := range cm.states {
		if url == activeURL {
			state.SetStatus(EndpointStatusConnected, now)
			state.RecordSuccess()
			cm.breakers[url].OnSuccess()
		} else {
			// Other endpoints could be reachable but not the current
			// nats.go pick. v1.0 surfaces them as Disconnected; Task
			// 7's per-endpoint dialing will track them more precisely.
			if state.Snapshot().Status == EndpointStatusConnecting {
				state.SetStatus(EndpointStatusDisconnected, now)
			}
		}
	}
}

func (cm *ConnectionManager) recordSuccess(activeURL string) {
	cm.mu.RLock()
	state, ok := cm.states[activeURL]
	breaker := cm.breakers[activeURL]
	cm.mu.RUnlock()
	if !ok {
		return
	}
	state.RecordSuccess()
	if breaker != nil {
		breaker.OnSuccess()
	}
}

func (cm *ConnectionManager) recordFailure(activeURL string, err error) {
	cm.mu.RLock()
	state, ok := cm.states[activeURL]
	breaker := cm.breakers[activeURL]
	cm.mu.RUnlock()
	if !ok {
		return
	}
	state.RecordFailure(err)
	if breaker != nil {
		breaker.OnFailure()
	}
}

// runRTTProbe issues a periodic PING on the active conn and records
// the result on the active endpoint's state. Stops on cm.probeStop.
func (cm *ConnectionManager) runRTTProbe() {
	defer close(cm.probeDone)
	t := time.NewTicker(rttProbeInterval)
	defer t.Stop()
	for {
		select {
		case <-cm.probeStop:
			return
		case <-t.C:
			cm.probeOnce()
		}
	}
}

func (cm *ConnectionManager) probeOnce() {
	cm.mu.RLock()
	conn := cm.conn
	cm.mu.RUnlock()
	if conn == nil || !conn.IsConnected() {
		return
	}
	active := conn.ConnectedUrl()
	if active == "" {
		return
	}

	done := make(chan time.Duration, 1)
	go func() {
		rtt, err := conn.RTT()
		if err != nil {
			done <- -1
			return
		}
		done <- rtt
	}()

	select {
	case rtt := <-done:
		if rtt < 0 {
			cm.recordFailure(active, errors.New("rtt probe failed"))
			return
		}
		cm.mu.RLock()
		state, ok := cm.states[active]
		cm.mu.RUnlock()
		if ok {
			state.RecordRTT(rtt)
		}
	case <-time.After(rttProbeTimeout):
		cm.recordFailure(active, errors.New("rtt probe timeout"))
	}
}
