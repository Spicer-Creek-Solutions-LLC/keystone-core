// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// FailoverState is the §4.15 failover machine:
//
//	IDLE → DETECTING → INITIATED → IN_PROGRESS → COMPLETED
//	                                   ↓
//	                          FAILED → ROLLED_BACK
//
// After an episode the cooldown elapses before the next one.
type FailoverState string

const (
	FailoverIdle       FailoverState = "idle"
	FailoverDetecting  FailoverState = "detecting"
	FailoverInitiated  FailoverState = "initiated"
	FailoverInProgress FailoverState = "in_progress"
	FailoverCompleted  FailoverState = "completed"
	FailoverFailed     FailoverState = "failed"
	FailoverRolledBack FailoverState = "rolled_back"
)

// failoverSettle is a short grace at episode start so the
// near-simultaneous ShardManager rebalance (which carries the
// concrete agent→new-owner moves) has landed before the episode
// snapshots them. Bounded + interruptible.
const failoverSettle = 200 * time.Millisecond

// JobRef identifies an in-flight job owned by a failed member. The
// pluggable JobReassigner interprets it.
type JobRef struct {
	ID string
}

// AgentReassigner enacts agent ownership handoff (the agent runtime
// reconnecting to the new shard owner). Injected by boot so
// internal/cluster does not import the controlplane/agent layers.
type AgentReassigner interface {
	ReassignAgents(ctx context.Context, moves []ShardMove) error
}

// JobReassigner re-dispatches a failed member's in-flight jobs onto
// their new owners. ReassignJobs receives a deterministic
// idempotency key per batch so a retried/duplicate reassignment
// never double-executes.
type JobReassigner interface {
	ListJobs(ctx context.Context, failedMember string) ([]JobRef, error)
	ReassignJobs(ctx context.Context, jobs []JobRef, idempotencyKey string) error
}

// AgentReassignerFunc adapts a func to AgentReassigner.
type AgentReassignerFunc func(context.Context, []ShardMove) error

func (f AgentReassignerFunc) ReassignAgents(ctx context.Context, m []ShardMove) error {
	return f(ctx, m)
}

// FailoverEvent is delivered to observers on every state change.
type FailoverEvent struct {
	State        FailoverState
	FailedMember string
	Agents       int // agents reassigned this episode
	Jobs         int // jobs reassigned this episode
	Err          error
}

// FailoverObserver receives failover transitions. Must not block;
// must be comparable (pointer type) for RemoveObserver — the
// cluster-package observer convention.
type FailoverObserver interface {
	OnFailover(FailoverEvent)
}

type failoverMembership interface {
	AddObserver(MembershipObserver)
	RemoveObserver(MembershipObserver)
}

type failoverShards interface {
	AddObserver(RebalanceObserver)
	RemoveObserver(RebalanceObserver)
}

// FailoverManagerConfig wires the manager. AgentReassigner /
// JobReassigner / Rollback may be nil (that step is skipped).
type FailoverManagerConfig struct {
	Membership      failoverMembership
	Shards          failoverShards
	AgentReassigner AgentReassigner
	JobReassigner   JobReassigner

	// Rollback, if set, is invoked on a FAILED episode to undo
	// partial reassignment (→ ROLLED_BACK).
	Rollback func(ctx context.Context, failedMember string) error

	// LeaderCheck gates orchestration: a non-leader observes but
	// does not act (avoids every node duplicating failover).
	// §4.15 leader-only SingletonTask; wiring LeaderElector.IsLeader
	// here is Task 9. nil ⇒ always act (standalone / tests).
	LeaderCheck func() bool

	Cooldown   time.Duration
	AgentBatch int
	JobBatch   int
	Logger     *slog.Logger
}

func (c *FailoverManagerConfig) fillDefaults() {
	if c.Cooldown < 0 {
		c.Cooldown = 0
	}
	if c.AgentBatch < 1 {
		c.AgentBatch = 100
	}
	if c.JobBatch < 1 {
		c.JobBatch = 50
	}
	if c.LeaderCheck == nil {
		c.LeaderCheck = func() bool { return true }
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

func (c *FailoverManagerConfig) validate() error {
	if c.Membership == nil {
		return fmt.Errorf("%w: Membership is required", ErrInvalidConfig)
	}
	if c.Shards == nil {
		return fmt.Errorf("%w: Shards is required", ErrInvalidConfig)
	}
	return nil
}

// FailoverManager orchestrates failover when a member fails:
// detect (membership UNHEALTHY/Left, correlated with the resulting
// ShardManager moves) → reassign agents (batched) → reassign jobs
// (batched, idempotency keys) → verify → cooldown. Single-use
// lifecycle.
type FailoverManager struct {
	cfg FailoverManagerConfig
	log *slog.Logger

	mu          sync.Mutex
	state       lifecycle
	fstate      FailoverState
	lastEpisode time.Time
	episodeSeq  atomic.Int64
	failed      map[string]bool
	pending     map[string][]ShardMove
	// armed dedups signals: at most one queued/in-flight episode
	// per member. Cleared when the episode finishes, so a genuine
	// later failure of the same member re-arms a fresh episode but
	// the detect/rebalance signal pair for one failure coalesces.
	armed map[string]bool

	workerCtx    context.Context
	workerCancel context.CancelFunc
	sigCh        chan string
	doneCh       chan struct{}

	obsMu     sync.RWMutex
	observers []FailoverObserver
}

// NewFailoverManager validates cfg and returns a manager in the
// created state.
func NewFailoverManager(cfg FailoverManagerConfig) (*FailoverManager, error) {
	cfg.fillDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &FailoverManager{
		cfg:     cfg,
		log:     cfg.Logger,
		state:   lcCreated,
		fstate:  FailoverIdle,
		failed:  make(map[string]bool),
		pending: make(map[string][]ShardMove),
		armed:   make(map[string]bool),
	}, nil
}

// Start registers the membership + shard observers and launches the
// episode worker.
func (f *FailoverManager) Start(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch f.state {
	case lcStarted:
		return ErrAlreadyStarted
	case lcStopped:
		return ErrStopped
	}
	f.workerCtx, f.workerCancel = context.WithCancel(context.Background())
	f.sigCh = make(chan string, 64)
	f.doneCh = make(chan struct{})
	f.state = lcStarted
	f.cfg.Membership.AddObserver(f)
	f.cfg.Shards.AddObserver(f)
	go f.loop()
	f.log.Info("failover manager started")
	return nil
}

// OnMembershipChange marks a failed member and signals an episode.
func (f *FailoverManager) OnMembershipChange(ev MemberEvent) {
	failedNow := ev.Type == MemberLeft ||
		(ev.Type == MemberUpdated && ev.Member.Status == MemberUnhealthy)
	if !failedNow {
		return
	}
	f.mu.Lock()
	f.failed[ev.Member.ID] = true
	f.mu.Unlock()
	f.signal(ev.Member.ID)
}

// OnRebalance correlates moves with failed members: a move whose
// From is a failed member is a failover reassignment (a move from a
// healthy member is join-driven load rebalancing — ignored).
func (f *FailoverManager) OnRebalance(moves []ShardMove) {
	f.mu.Lock()
	touched := make(map[string]bool)
	for _, mv := range moves {
		if f.failed[mv.From] {
			f.pending[mv.From] = append(f.pending[mv.From], mv)
			touched[mv.From] = true
		}
	}
	f.mu.Unlock()
	for m := range touched {
		f.signal(m)
	}
}

// signal enqueues at most one episode per member until that
// episode finishes (disarm). OnRebalance still appends pending
// moves unconditionally, so a no-op signal here does not lose the
// moves for the in-flight episode.
func (f *FailoverManager) signal(member string) {
	f.mu.Lock()
	if f.armed[member] {
		f.mu.Unlock()
		return
	}
	f.armed[member] = true
	f.mu.Unlock()
	select {
	case f.sigCh <- member:
	default:
	}
}

func (f *FailoverManager) disarm(member string) {
	f.mu.Lock()
	delete(f.armed, member)
	f.mu.Unlock()
}

func (f *FailoverManager) loop() {
	defer close(f.doneCh)
	for {
		select {
		case <-f.workerCtx.Done():
			return
		case m := <-f.sigCh:
			if !f.waitCooldown() {
				return
			}
			f.runEpisode(m)
		}
	}
}

// waitCooldown enforces ≥ Cooldown spacing between episodes.
func (f *FailoverManager) waitCooldown() bool {
	f.mu.Lock()
	last := f.lastEpisode
	cd := f.cfg.Cooldown
	f.mu.Unlock()
	wait := cd - time.Since(last)
	if wait <= 0 {
		return true
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-f.workerCtx.Done():
		return false
	}
}

func (f *FailoverManager) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-f.workerCtx.Done():
		return false
	}
}

func (f *FailoverManager) runEpisode(member string) {
	// Cleared only after the episode reaches a terminal state, so
	// the detect+rebalance signals for one failure coalesce while
	// a genuinely new later failure can re-arm.
	defer f.disarm(member)
	f.transition(FailoverDetecting, member, 0, 0, nil)

	// Let the near-simultaneous rebalance moves land.
	if !f.sleep(failoverSettle) {
		return
	}

	f.mu.Lock()
	moves := f.pending[member]
	delete(f.pending, member)
	delete(f.failed, member)
	f.lastEpisode = time.Now()
	f.mu.Unlock()

	episodeID := f.episodeSeq.Add(1)
	f.transition(FailoverInitiated, member, 0, 0, nil)

	// Leader-only: non-leaders observe but do not orchestrate.
	if !f.cfg.LeaderCheck() {
		f.transition(FailoverCompleted, member, 0, 0, nil)
		return
	}

	f.transition(FailoverInProgress, member, 0, 0, nil)

	agents, err := f.reassignAgents(moves)
	if err != nil {
		f.fail(member, agents, 0, fmt.Errorf("agent reassignment: %w", err))
		return
	}
	jobs, err := f.reassignJobs(member, episodeID)
	if err != nil {
		f.fail(member, agents, jobs, fmt.Errorf("job reassignment: %w", err))
		return
	}
	f.transition(FailoverCompleted, member, agents, jobs, nil)
	f.log.Info("failover completed", "member", member, "agents", agents, "jobs", jobs)
}

func (f *FailoverManager) reassignAgents(moves []ShardMove) (int, error) {
	if f.cfg.AgentReassigner == nil || len(moves) == 0 {
		return 0, nil
	}
	n := 0
	for _, batch := range chunkMoves(moves, f.cfg.AgentBatch) {
		if err := f.cfg.AgentReassigner.ReassignAgents(f.workerCtx, batch); err != nil {
			return n, err
		}
		n += len(batch)
	}
	return n, nil
}

func (f *FailoverManager) reassignJobs(member string, episodeID int64) (int, error) {
	if f.cfg.JobReassigner == nil {
		return 0, nil
	}
	jobs, err := f.cfg.JobReassigner.ListJobs(f.workerCtx, member)
	if err != nil {
		return 0, fmt.Errorf("list jobs: %w", err)
	}
	n := 0
	for i, batch := range chunkJobs(jobs, f.cfg.JobBatch) {
		key := fmt.Sprintf("failover/%s/%d/%d", member, episodeID, i)
		if err := f.cfg.JobReassigner.ReassignJobs(f.workerCtx, batch, key); err != nil {
			return n, err
		}
		n += len(batch)
	}
	return n, nil
}

func (f *FailoverManager) fail(member string, agents, jobs int, err error) {
	f.log.Warn("failover failed", "member", member, "err", err)
	f.transition(FailoverFailed, member, agents, jobs, err)
	if f.cfg.Rollback == nil {
		return
	}
	if rbErr := f.cfg.Rollback(f.workerCtx, member); rbErr != nil {
		f.log.Warn("failover rollback failed", "member", member, "err", rbErr)
		return
	}
	f.transition(FailoverRolledBack, member, agents, jobs, err)
}

func (f *FailoverManager) transition(s FailoverState, member string, agents, jobs int, err error) {
	f.mu.Lock()
	f.fstate = s
	f.mu.Unlock()
	f.dispatch(FailoverEvent{State: s, FailedMember: member, Agents: agents, Jobs: jobs, Err: err})
}

func (f *FailoverManager) dispatch(ev FailoverEvent) {
	f.obsMu.RLock()
	obs := make([]FailoverObserver, len(f.observers))
	copy(obs, f.observers)
	f.obsMu.RUnlock()
	for _, o := range obs {
		o.OnFailover(ev)
	}
}

// AddObserver registers o for failover transitions. o must be a
// comparable value (pointer type) for RemoveObserver.
func (f *FailoverManager) AddObserver(o FailoverObserver) {
	if o == nil {
		return
	}
	f.obsMu.Lock()
	f.observers = append(f.observers, o)
	f.obsMu.Unlock()
}

// RemoveObserver deregisters o (identity comparison).
func (f *FailoverManager) RemoveObserver(o FailoverObserver) {
	f.obsMu.Lock()
	defer f.obsMu.Unlock()
	for i, x := range f.observers {
		if x == o {
			f.observers = append(f.observers[:i], f.observers[i+1:]...)
			return
		}
	}
}

// State returns the current failover state.
func (f *FailoverManager) State() FailoverState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fstate
}

// Stop deregisters observers and ends the episode worker.
// Idempotent.
func (f *FailoverManager) Stop(ctx context.Context) error {
	f.mu.Lock()
	if f.state != lcStarted {
		f.state = lcStopped
		f.mu.Unlock()
		return nil
	}
	cancel := f.workerCancel
	doneCh := f.doneCh
	f.state = lcStopped
	f.mu.Unlock()

	f.cfg.Membership.RemoveObserver(f)
	f.cfg.Shards.RemoveObserver(f)
	if cancel != nil {
		cancel()
	}
	select {
	case <-doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	f.log.Info("failover manager stopped")
	return nil
}

func chunkMoves(s []ShardMove, n int) [][]ShardMove {
	var out [][]ShardMove
	for i := 0; i < len(s); i += n {
		end := i + n
		if end > len(s) {
			end = len(s)
		}
		out = append(out, s[i:end])
	}
	return out
}

func chunkJobs(s []JobRef, n int) [][]JobRef {
	var out [][]JobRef
	for i := 0; i < len(s); i += n {
		end := i + n
		if end > len(s) {
			end = len(s)
		}
		out = append(out, s[i:end])
	}
	return out
}
