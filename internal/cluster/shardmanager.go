package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// membershipSource is the slice of MembershipManager that
// ShardManager needs. *MembershipManager satisfies it; the
// interface keeps ShardManager decoupled + testable.
type membershipSource interface {
	LoadMembers(ctx context.Context) ([]Member, error)
	AddObserver(MembershipObserver)
	RemoveObserver(MembershipObserver)
}

// ShardMove records one agent changing owner during a rebalance.
type ShardMove struct {
	AgentID string
	From    string
	To      string
}

// RebalanceObserver is notified after a rebalance that moved at
// least one agent. The FailoverManager (Task 8) / agent runtime
// consume this to drive reconnects; ShardManager itself only
// recomputes + persists. Implementations must not block.
type RebalanceObserver interface {
	OnRebalance([]ShardMove)
}

// ShardManagerConfig wires the pieces ShardManager composes.
type ShardManagerConfig struct {
	Membership membershipSource
	Store      *ShardStore

	// VNodes is the HashRing virtual-node count (≤0 → default).
	VNodes int

	// RebalanceCooldown is the minimum spacing between
	// topology-driven rebalances (rapid flaps coalesce).
	RebalanceCooldown time.Duration

	// LeaderCheck gates ShardStore *writes*: the ring is kept in
	// sync on every node (so Owner reads are correct everywhere),
	// but only a node for which this returns true persists
	// reassignments — §4.15 makes agent rebalance a leader-only
	// SingletonTask. nil ⇒ always true (standalone / tests).
	// Wiring LeaderElector.IsLeader here is Task 9, not this task.
	LeaderCheck func() bool

	Logger *slog.Logger
}

func (c *ShardManagerConfig) fillDefaults() {
	if c.VNodes <= 0 {
		c.VNodes = DefaultVirtualNodes
	}
	if c.RebalanceCooldown < 0 {
		c.RebalanceCooldown = 0
	}
	if c.LeaderCheck == nil {
		c.LeaderCheck = func() bool { return true }
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

func (c *ShardManagerConfig) validate() error {
	if c.Membership == nil {
		return fmt.Errorf("%w: Membership source is required", ErrInvalidConfig)
	}
	if c.Store == nil {
		return fmt.Errorf("%w: Store is required", ErrInvalidConfig)
	}
	return nil
}

// ShardManager keeps a consistent-hash ring in sync with cluster
// membership and reconciles the persisted shard map to it with
// minimal migration. Single-use lifecycle, mirroring the other
// cluster components.
type ShardManager struct {
	cfg  ShardManagerConfig
	log  *slog.Logger
	ring *HashRing

	mu            sync.Mutex
	state         lifecycle
	lastRebalance time.Time
	workerCtx     context.Context
	workerCancel  context.CancelFunc
	sigCh         chan struct{}
	doneCh        chan struct{}

	obsMu     sync.RWMutex
	observers []RebalanceObserver
}

// NewShardManager validates cfg and returns a manager in the
// created (not yet started) state.
func NewShardManager(cfg ShardManagerConfig) (*ShardManager, error) {
	cfg.fillDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &ShardManager{
		cfg:   cfg,
		log:   cfg.Logger,
		ring:  NewHashRing(cfg.VNodes),
		state: lcCreated,
	}, nil
}

func eligible(m Member) bool {
	return m.Status == MemberHealthy || m.Status == MemberDegraded
}

// Start seeds the ring from current membership, registers the
// membership observer, launches the debounce/rebalance loop, and
// signals an initial reconcile. Second call → ErrAlreadyStarted /
// ErrStopped.
func (m *ShardManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.state {
	case lcStarted:
		return ErrAlreadyStarted
	case lcStopped:
		return ErrStopped
	}

	members, err := m.cfg.Membership.LoadMembers(ctx)
	if err != nil {
		return fmt.Errorf("seed ring: %w", err)
	}
	for _, mem := range members {
		if eligible(mem) {
			m.ring.Add(mem.ID)
		}
	}

	m.workerCtx, m.workerCancel = context.WithCancel(context.Background())
	m.sigCh = make(chan struct{}, 1)
	m.doneCh = make(chan struct{})
	m.state = lcStarted

	m.cfg.Membership.AddObserver(m)
	go m.rebalanceLoop()
	m.signal() // initial reconcile of any pre-existing shard map
	m.log.Info("shard manager started", "members", len(members), "vnodes", m.cfg.VNodes)
	return nil
}

// OnMembershipChange keeps the ring eligible-member-accurate and
// debounces a rebalance. Called from the MembershipManager watch
// goroutine.
func (m *ShardManager) OnMembershipChange(ev MemberEvent) {
	switch ev.Type {
	case MemberLeft:
		m.ring.Remove(ev.Member.ID)
	default: // Joined / Updated
		if eligible(ev.Member) {
			m.ring.Add(ev.Member.ID)
		} else {
			m.ring.Remove(ev.Member.ID)
		}
	}
	m.signal()
}

// signal coalesces topology-change notifications (buffered-1).
func (m *ShardManager) signal() {
	select {
	case m.sigCh <- struct{}{}:
	default:
	}
}

func (m *ShardManager) rebalanceLoop() {
	defer close(m.doneCh)
	for {
		select {
		case <-m.workerCtx.Done():
			return
		case <-m.sigCh:
			if !m.waitCooldown() {
				return
			}
			if _, err := m.doRebalance(m.workerCtx); err != nil &&
				m.workerCtx.Err() == nil {
				m.log.Warn("rebalance failed", "err", err)
			}
		}
	}
}

// waitCooldown sleeps out the remaining cooldown since the last
// rebalance (interruptible). Returns false if the worker ctx was
// cancelled during the wait.
func (m *ShardManager) waitCooldown() bool {
	m.mu.Lock()
	last := m.lastRebalance
	cd := m.cfg.RebalanceCooldown
	m.mu.Unlock()

	wait := cd - time.Since(last)
	if wait <= 0 {
		return true
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-m.workerCtx.Done():
		return false
	}
}

// Rebalance reconciles the persisted shard map to the current ring
// immediately (no cooldown — the manual/explicit trigger).
func (m *ShardManager) Rebalance(ctx context.Context) ([]ShardMove, error) {
	if err := m.requireStarted(); err != nil {
		return nil, err
	}
	return m.doRebalance(ctx)
}

func (m *ShardManager) doRebalance(ctx context.Context) ([]ShardMove, error) {
	m.mu.Lock()
	m.lastRebalance = time.Now()
	m.mu.Unlock()

	// Ring is maintained on every node; only the leader persists.
	if !m.cfg.LeaderCheck() {
		return nil, nil
	}

	assignments, err := m.cfg.Store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list shards: %w", err)
	}
	var moves []ShardMove
	for _, a := range assignments {
		desired, ok := m.ring.Get(a.AgentID)
		if !ok {
			// Empty ring (no eligible members): keep the existing
			// assignment until members return.
			continue
		}
		if desired == a.MemberID {
			continue
		}
		if _, err := m.cfg.Store.AssignIf(ctx, a.AgentID, desired, a.Version); err != nil {
			if errors.Is(err, ErrVersionConflict) {
				m.log.Info("rebalance skip (version conflict; next cycle)",
					"agent", a.AgentID)
				continue
			}
			m.log.Warn("rebalance assign failed", "agent", a.AgentID, "err", err)
			continue
		}
		moves = append(moves, ShardMove{AgentID: a.AgentID, From: a.MemberID, To: desired})
	}
	if len(moves) > 0 {
		m.dispatch(moves)
		m.log.Info("rebalanced", "moved", len(moves))
	}
	return moves, nil
}

// Owner returns the member that owns agentID: the sticky persisted
// assignment if present, else the deterministic ring position
// (every node computes the same one). ErrNoEligibleMembers if the
// ring is empty and there is no persisted assignment.
func (m *ShardManager) Owner(ctx context.Context, agentID string) (string, error) {
	if err := m.requireStarted(); err != nil {
		return "", err
	}
	got, err := m.cfg.Store.Get(ctx, agentID)
	if err == nil {
		return got.MemberID, nil
	}
	if !errors.Is(err, ErrShardNotFound) {
		return "", err
	}
	owner, ok := m.ring.Get(agentID)
	if !ok {
		return "", ErrNoEligibleMembers
	}
	return owner, nil
}

// AssignAgent durably assigns a (typically newly-registered) agent
// to its ring owner. Leader-gated: a non-leader returns the
// computed/looked-up owner without persisting (the leader's
// rebalance/assignment is authoritative for writes).
func (m *ShardManager) AssignAgent(ctx context.Context, agentID string) (string, error) {
	if err := m.requireStarted(); err != nil {
		return "", err
	}
	if got, err := m.cfg.Store.Get(ctx, agentID); err == nil {
		return got.MemberID, nil
	} else if !errors.Is(err, ErrShardNotFound) {
		return "", err
	}
	owner, ok := m.ring.Get(agentID)
	if !ok {
		return "", ErrNoEligibleMembers
	}
	if !m.cfg.LeaderCheck() {
		return owner, nil // non-leader: don't persist
	}
	if _, err := m.cfg.Store.AssignIf(ctx, agentID, owner, 0); err != nil {
		if errors.Is(err, ErrVersionConflict) {
			// Raced with another writer — return the winner.
			got, gErr := m.cfg.Store.Get(ctx, agentID)
			if gErr != nil {
				return "", gErr
			}
			return got.MemberID, nil
		}
		return "", err
	}
	return owner, nil
}

func (m *ShardManager) requireStarted() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state != lcStarted {
		return ErrNotStarted
	}
	return nil
}

func (m *ShardManager) dispatch(moves []ShardMove) {
	m.obsMu.RLock()
	obs := make([]RebalanceObserver, len(m.observers))
	copy(obs, m.observers)
	m.obsMu.RUnlock()
	for _, o := range obs {
		o.OnRebalance(moves)
	}
}

// AddObserver registers o for post-rebalance move notifications.
// o must be a comparable value (use a pointer type) so it can be
// passed to RemoveObserver — the cluster-package observer
// convention (shared with Membership/Leadership observers).
func (m *ShardManager) AddObserver(o RebalanceObserver) {
	if o == nil {
		return
	}
	m.obsMu.Lock()
	m.observers = append(m.observers, o)
	m.obsMu.Unlock()
}

// RemoveObserver deregisters o (identity comparison).
func (m *ShardManager) RemoveObserver(o RebalanceObserver) {
	m.obsMu.Lock()
	defer m.obsMu.Unlock()
	for i, x := range m.observers {
		if x == o {
			m.observers = append(m.observers[:i], m.observers[i+1:]...)
			return
		}
	}
}

// Members returns the ring's current member set (sorted).
func (m *ShardManager) Members() []string { return m.ring.Members() }

// Stop deregisters the membership observer and ends the rebalance
// loop. Idempotent: never-started / already-stopped → nil.
func (m *ShardManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if m.state != lcStarted {
		m.state = lcStopped
		m.mu.Unlock()
		return nil
	}
	cancel := m.workerCancel
	doneCh := m.doneCh
	m.state = lcStopped
	m.mu.Unlock()

	m.cfg.Membership.RemoveObserver(m)
	if cancel != nil {
		cancel()
	}
	select {
	case <-doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	m.log.Info("shard manager stopped")
	return nil
}
