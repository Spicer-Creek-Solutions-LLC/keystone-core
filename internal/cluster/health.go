// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// EtcdCheckerName is the reserved name of the etcd checker. A
// failing etcd checker is treated as critical (→ UNHEALTHY) and as
// a quorum-loss signal — etcd only serves with a quorum, so being
// unable to reach it means this node is in the minority.
const EtcdCheckerName = "etcd"

// HealthChecker is a pluggable liveness probe. Check returns nil
// when healthy; any error counts as one failure.
type HealthChecker interface {
	Name() string
	Check(ctx context.Context) error
}

type checkerFunc struct {
	name string
	fn   func(context.Context) error
}

func (c checkerFunc) Name() string                    { return c.name }
func (c checkerFunc) Check(ctx context.Context) error { return c.fn(ctx) }

// PingChecker adapts a func to a HealthChecker. This is how the
// DB/NATS checkers are supplied — boot wires the real *sql.DB /
// nats.Conn ping without internal/cluster importing those packages.
func PingChecker(name string, fn func(context.Context) error) HealthChecker {
	return checkerFunc{name: name, fn: fn}
}

// NewEtcdChecker returns the built-in etcd checker (a bounded Get
// probe). Reaching etcd requires a quorum, so its success is the
// canonical "we are not partitioned" signal.
func NewEtcdChecker(ec *EtcdClient) HealthChecker {
	return checkerFunc{
		name: EtcdCheckerName,
		fn: func(ctx context.Context) error {
			_, err := ec.Get(ctx, "/kscore/health/probe")
			return err
		},
	}
}

// NewHeartbeatChecker reports failure when the etcd client's
// lease-keepalive failure counter has increased since the last
// check (the membership lease is not being renewed).
func NewHeartbeatChecker(ec *EtcdClient) HealthChecker {
	var last int64
	var mu sync.Mutex
	return checkerFunc{
		name: "heartbeat",
		fn: func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			n := ec.KeepAliveFailures()
			if n > last {
				last = n
				return errors.New("membership lease keepalive failing")
			}
			last = n
			return nil
		},
	}
}

// QuorumState reflects whether this node can reach the etcd quorum.
type QuorumState string

const (
	QuorumOK       QuorumState = "quorum"
	QuorumMinority QuorumState = "minority"
)

// HealthEvent is delivered to observers on a status or quorum
// transition.
type HealthEvent struct {
	Status  MemberStatus
	Quorum  QuorumState
	Failing []string
}

// HealthObserver receives health transitions. Implementations must
// not block, and must be comparable (use a pointer type) so they
// can be passed to RemoveObserver — the cluster-package observer
// convention.
type HealthObserver interface {
	OnHealthChange(HealthEvent)
}

// memberStatusSink is the slice of MembershipManager HealthMonitor
// needs: read membership (for quorum) and drive this node's status.
// *MembershipManager satisfies it.
type memberStatusSink interface {
	LoadMembers(ctx context.Context) ([]Member, error)
	SetStatus(ctx context.Context, to MemberStatus) error
}

// HealthMonitorConfig wires the monitor.
type HealthMonitorConfig struct {
	Membership       memberStatusSink
	Checkers         []HealthChecker
	CriticalCheckers []string // names; nil ⇒ {"etcd"}
	Interval         time.Duration
	FailureThreshold int
	LatencyWindow    int
	Logger           *slog.Logger
}

func (c *HealthMonitorConfig) fillDefaults() {
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
	if c.FailureThreshold < 1 {
		c.FailureThreshold = 3
	}
	if c.LatencyWindow < 1 {
		c.LatencyWindow = 100
	}
	if c.CriticalCheckers == nil {
		c.CriticalCheckers = []string{EtcdCheckerName}
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

func (c *HealthMonitorConfig) validate() error {
	if c.Membership == nil {
		return fmt.Errorf("%w: Membership sink is required", ErrInvalidConfig)
	}
	return nil
}

// latencyRing is a fixed-size ring of recent check durations.
type latencyRing struct {
	mu   sync.Mutex
	buf  []time.Duration
	next int
	n    int
}

func newLatencyRing(size int) *latencyRing {
	return &latencyRing{buf: make([]time.Duration, size)}
}

func (r *latencyRing) add(d time.Duration) {
	r.mu.Lock()
	r.buf[r.next] = d
	r.next = (r.next + 1) % len(r.buf)
	if r.n < len(r.buf) {
		r.n++
	}
	r.mu.Unlock()
}

// percentile returns the nearest-rank p (0–100) latency.
func (r *latencyRing) percentile(p int) time.Duration {
	r.mu.Lock()
	if r.n == 0 {
		r.mu.Unlock()
		return 0
	}
	s := make([]time.Duration, r.n)
	copy(s, r.buf[:r.n])
	r.mu.Unlock()
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	idx := (p*len(s)+99)/100 - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(s) {
		idx = len(s) - 1
	}
	return s[idx]
}

type checkerState struct {
	consecutive int
	failing     bool
	lat         *latencyRing
}

// HealthMonitor runs pluggable checkers on an interval, tracks
// consecutive failures + P50/P99 latency, detects quorum loss, and
// drives this node's MemberStatus through the Task 2 lifecycle
// state machine (HEALTHY↔DEGRADED↔UNHEALTHY) via valid edges only.
// Single-use lifecycle, mirroring the other cluster components.
type HealthMonitor struct {
	cfg      HealthMonitorConfig
	log      *slog.Logger
	critical map[string]bool

	mu        sync.Mutex
	state     lifecycle
	curStatus MemberStatus
	curQuorum QuorumState
	checkers  map[string]*checkerState

	workerCtx    context.Context
	workerCancel context.CancelFunc
	doneCh       chan struct{}

	obsMu     sync.RWMutex
	observers []HealthObserver
}

// NewHealthMonitor validates cfg and returns a monitor in the
// created state.
func NewHealthMonitor(cfg HealthMonitorConfig) (*HealthMonitor, error) {
	cfg.fillDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	crit := make(map[string]bool, len(cfg.CriticalCheckers))
	for _, n := range cfg.CriticalCheckers {
		crit[n] = true
	}
	cs := make(map[string]*checkerState, len(cfg.Checkers))
	for _, ch := range cfg.Checkers {
		cs[ch.Name()] = &checkerState{lat: newLatencyRing(cfg.LatencyWindow)}
	}
	return &HealthMonitor{
		cfg:       cfg,
		log:       cfg.Logger,
		critical:  crit,
		state:     lcCreated,
		curStatus: MemberHealthy,
		curQuorum: QuorumOK,
		checkers:  cs,
	}, nil
}

// Start launches the periodic check loop.
func (h *HealthMonitor) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch h.state {
	case lcStarted:
		return ErrAlreadyStarted
	case lcStopped:
		return ErrStopped
	}
	h.workerCtx, h.workerCancel = context.WithCancel(context.Background())
	h.doneCh = make(chan struct{})
	h.state = lcStarted
	go h.loop()
	h.log.Info("health monitor started", "checkers", len(h.cfg.Checkers))
	return nil
}

func (h *HealthMonitor) loop() {
	defer close(h.doneCh)
	t := time.NewTicker(h.cfg.Interval)
	defer t.Stop()
	h.evaluate() // initial check
	for {
		select {
		case <-h.workerCtx.Done():
			return
		case <-t.C:
			h.evaluate()
		}
	}
}

// evaluate runs every checker once, updates failure/latency state,
// determines quorum + desired status, and reconciles.
func (h *HealthMonitor) evaluate() {
	failing := make([]string, 0)
	criticalFail := false
	for _, ch := range h.cfg.Checkers {
		st := h.checkers[ch.Name()]
		cctx, cancel := context.WithTimeout(h.workerCtx, h.cfg.Interval)
		start := time.Now()
		err := ch.Check(cctx)
		st.lat.add(time.Since(start))
		cancel()

		if err != nil {
			st.consecutive++
		} else {
			st.consecutive = 0
		}
		st.failing = st.consecutive >= h.cfg.FailureThreshold
		if st.failing {
			failing = append(failing, ch.Name())
			if h.critical[ch.Name()] {
				criticalFail = true
			}
		}
	}

	quorum := QuorumOK
	if _, err := h.cfg.Membership.LoadMembers(h.workerCtx); err != nil {
		quorum = QuorumMinority // can't reach etcd ⇒ no quorum
	}
	if h.checkerFailing(EtcdCheckerName) {
		quorum = QuorumMinority
	}

	desired := MemberHealthy
	switch {
	case quorum == QuorumMinority || criticalFail:
		desired = MemberUnhealthy
	case len(failing) > 0:
		desired = MemberDegraded
	}

	h.reconcile(desired, quorum, failing)
}

func (h *HealthMonitor) checkerFailing(name string) bool {
	st, ok := h.checkers[name]
	return ok && st.failing
}

// statusRank orders the linear lifecycle for stepwise transitions.
var statusRank = map[MemberStatus]int{
	MemberHealthy:   0,
	MemberDegraded:  1,
	MemberUnhealthy: 2,
}

var rankStatus = []MemberStatus{MemberHealthy, MemberDegraded, MemberUnhealthy}

// reconcile walks curStatus toward desired one valid SM edge at a
// time (HEALTHY↔DEGRADED↔UNHEALTHY are adjacent/allowed; the
// invalid HEALTHY→UNHEALTHY direct jump is never attempted), and
// emits a HealthEvent on any status/quorum change.
func (h *HealthMonitor) reconcile(desired MemberStatus, quorum QuorumState, failing []string) {
	h.mu.Lock()
	cur := h.curStatus
	prevQuorum := h.curQuorum
	h.mu.Unlock()

	changed := quorum != prevQuorum
	for cur != desired {
		var step MemberStatus
		if statusRank[desired] > statusRank[cur] {
			step = rankStatus[statusRank[cur]+1]
		} else {
			step = rankStatus[statusRank[cur]-1]
		}
		if err := h.cfg.Membership.SetStatus(h.workerCtx, step); err != nil {
			if h.workerCtx.Err() == nil {
				h.log.Warn("health status transition failed",
					"from", cur, "to", step, "err", err)
			}
			break
		}
		cur = step
		changed = true
	}

	h.mu.Lock()
	h.curStatus = cur
	h.curQuorum = quorum
	h.mu.Unlock()

	if changed {
		h.dispatch(HealthEvent{Status: cur, Quorum: quorum, Failing: failing})
	}
}

func (h *HealthMonitor) dispatch(ev HealthEvent) {
	h.obsMu.RLock()
	obs := make([]HealthObserver, len(h.observers))
	copy(obs, h.observers)
	h.obsMu.RUnlock()
	for _, o := range obs {
		o.OnHealthChange(ev)
	}
}

// AddObserver registers o for health transitions. o must be a
// comparable value (pointer type) so it can be passed to
// RemoveObserver — the cluster-package observer convention.
func (h *HealthMonitor) AddObserver(o HealthObserver) {
	if o == nil {
		return
	}
	h.obsMu.Lock()
	h.observers = append(h.observers, o)
	h.obsMu.Unlock()
}

// RemoveObserver deregisters o (identity comparison).
func (h *HealthMonitor) RemoveObserver(o HealthObserver) {
	h.obsMu.Lock()
	defer h.obsMu.Unlock()
	for i, x := range h.observers {
		if x == o {
			h.observers = append(h.observers[:i], h.observers[i+1:]...)
			return
		}
	}
}

// Status returns the current member status as the monitor sees it.
func (h *HealthMonitor) Status() MemberStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.curStatus
}

// Quorum returns the current quorum state.
func (h *HealthMonitor) Quorum() QuorumState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.curQuorum
}

// Latency returns the P50/P99 check latency for a checker (zero if
// unknown).
func (h *HealthMonitor) Latency(name string) (p50, p99 time.Duration) {
	st, ok := h.checkers[name]
	if !ok {
		return 0, 0
	}
	return st.lat.percentile(50), st.lat.percentile(99)
}

// Stop ends the check loop. Idempotent.
func (h *HealthMonitor) Stop(ctx context.Context) error {
	h.mu.Lock()
	if h.state != lcStarted {
		h.state = lcStopped
		h.mu.Unlock()
		return nil
	}
	cancel := h.workerCancel
	doneCh := h.doneCh
	h.state = lcStopped
	h.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	select {
	case <-doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	h.log.Info("health monitor stopped")
	return nil
}
