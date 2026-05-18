package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// RecoveryPhase is the §4.15 restart-recovery machine (linear):
//
//	STARTING → CONNECTING → SYNCING → VERIFYING → REJOINING →
//	RECLAIMING → COMPLETED
//
// Any phase erroring ends in the terminal FAILED (carrying which
// phase failed).
type RecoveryPhase string

const (
	RecoveryStarting   RecoveryPhase = "starting"
	RecoveryConnecting RecoveryPhase = "connecting"
	RecoverySyncing    RecoveryPhase = "syncing"
	RecoveryVerifying  RecoveryPhase = "verifying"
	RecoveryRejoining  RecoveryPhase = "rejoining"
	RecoveryReclaiming RecoveryPhase = "reclaiming"
	RecoveryCompleted  RecoveryPhase = "completed"
	RecoveryFailed     RecoveryPhase = "failed"
)

// RecoveryEvent is delivered to observers on every phase change.
// Err is set only with RecoveryFailed.
type RecoveryEvent struct {
	Phase RecoveryPhase
	Err   error
}

// RecoveryObserver receives recovery phase transitions. Must not
// block; must be comparable (pointer type) for RemoveObserver.
type RecoveryObserver interface {
	OnRecovery(RecoveryEvent)
}

// recoveryEtcd is the slice of EtcdClient used by the CONNECTING
// probe. *EtcdClient satisfies it.
type recoveryEtcd interface {
	Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
}

// recoveryMembership is the slice of MembershipManager recovery
// needs. *MembershipManager satisfies it.
type recoveryMembership interface {
	Register(ctx context.Context) error
	LoadMembers(ctx context.Context) ([]Member, error)
	Self() Member
}

// recoveryShards is the slice of ShardStore recovery needs.
// *ShardStore satisfies it.
type recoveryShards interface {
	List(ctx context.Context) ([]ShardAssignment, error)
}

// AgentReclaimer re-establishes serving for the agents this node
// owns after rejoin. Injected by boot so internal/cluster does not
// import the controlplane/agent layers.
type AgentReclaimer interface {
	ReclaimAgents(ctx context.Context, owned []ShardAssignment) error
}

// AgentReclaimerFunc adapts a func to AgentReclaimer.
type AgentReclaimerFunc func(context.Context, []ShardAssignment) error

func (f AgentReclaimerFunc) ReclaimAgents(ctx context.Context, owned []ShardAssignment) error {
	return f(ctx, owned)
}

// RecoveryManagerConfig wires the manager. Reclaimer may be nil
// (the RECLAIMING action is skipped).
type RecoveryManagerConfig struct {
	Etcd       recoveryEtcd
	Membership recoveryMembership
	Shards     recoveryShards
	Reclaimer  AgentReclaimer

	ConnectTimeout time.Duration
	ConnectRetries int
	Logger         *slog.Logger
}

func (c *RecoveryManagerConfig) fillDefaults() {
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 5 * time.Second
	}
	if c.ConnectRetries < 1 {
		c.ConnectRetries = 3
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

func (c *RecoveryManagerConfig) validate() error {
	if c.Etcd == nil {
		return fmt.Errorf("%w: Etcd is required", ErrInvalidConfig)
	}
	if c.Membership == nil {
		return fmt.Errorf("%w: Membership is required", ErrInvalidConfig)
	}
	if c.Shards == nil {
		return fmt.Errorf("%w: Shards is required", ErrInvalidConfig)
	}
	return nil
}

// recoveryProbeKey is the (read-only) key the CONNECTING phase
// gets to confirm etcd is reachable with a quorum.
const recoveryProbeKey = "/kscore/recovery/probe"

// RecoveryManager runs the one-shot 7-phase restart recovery. The
// stable member ID (Task 2 MemberIDFile) makes RECLAIMING correct:
// a restarted process re-registers as the same identity, so the
// shard-map entries still pointing at it are recognised as its own.
type RecoveryManager struct {
	cfg RecoveryManagerConfig
	log *slog.Logger

	mu    sync.Mutex
	phase RecoveryPhase
	ran   bool

	obsMu     sync.RWMutex
	observers []RecoveryObserver
}

// NewRecoveryManager validates cfg and returns a manager ready to
// Recover.
func NewRecoveryManager(cfg RecoveryManagerConfig) (*RecoveryManager, error) {
	cfg.fillDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &RecoveryManager{cfg: cfg, log: cfg.Logger, phase: RecoveryStarting}, nil
}

// Phase returns the current recovery phase.
func (r *RecoveryManager) Phase() RecoveryPhase {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.phase
}

func (r *RecoveryManager) setPhase(p RecoveryPhase) {
	r.mu.Lock()
	r.phase = p
	r.mu.Unlock()
	r.dispatch(RecoveryEvent{Phase: p})
}

func (r *RecoveryManager) failPhase(failed RecoveryPhase, err error) error {
	wrapped := fmt.Errorf("recovery failed in %s: %w", failed, err)
	r.mu.Lock()
	r.phase = RecoveryFailed
	r.mu.Unlock()
	r.dispatch(RecoveryEvent{Phase: RecoveryFailed, Err: wrapped})
	r.log.Warn("recovery failed", "phase", failed, "err", err)
	return wrapped
}

// Recover runs the recovery sequence. It is single-use: a second
// call returns ErrAlreadyStarted. Returns nil on COMPLETED, the
// wrapped phase error on FAILED.
func (r *RecoveryManager) Recover(ctx context.Context) error {
	r.mu.Lock()
	if r.ran {
		r.mu.Unlock()
		return fmt.Errorf("%w: recovery already run", ErrAlreadyStarted)
	}
	r.ran = true
	r.mu.Unlock()

	// 1. STARTING — load local state (member identity/key-prefix
	//    are owned by MembershipManager; nothing heavier here at
	//    v1.0 — StateStore/ConfigStore are later tasks).
	r.setPhase(RecoveryStarting)

	// 2. CONNECTING — bounded etcd reachability probe with retry.
	r.setPhase(RecoveryConnecting)
	if err := r.connect(ctx); err != nil {
		return r.failPhase(RecoveryConnecting, err)
	}

	// 3. SYNCING — load member info + shard assignments. The shard
	//    map (via ShardStore, no registration needed) is the
	//    operative input for RECLAIMING; the member list is
	//    best-effort context and is legitimately unavailable until
	//    REJOINING re-registers (MembershipManager.LoadMembers
	//    gates on registration) — ErrNotRegistered is tolerated.
	r.setPhase(RecoverySyncing)
	if _, err := r.cfg.Membership.LoadMembers(ctx); err != nil && !errors.Is(err, ErrNotRegistered) {
		return r.failPhase(RecoverySyncing, fmt.Errorf("load members: %w", err))
	}
	assignments, err := r.cfg.Shards.List(ctx)
	if err != nil {
		return r.failPhase(RecoverySyncing, fmt.Errorf("list shards: %w", err))
	}

	// 4. VERIFYING — structural validation of the synced state.
	r.setPhase(RecoveryVerifying)
	if err := verifyAssignments(assignments); err != nil {
		return r.failPhase(RecoveryVerifying, err)
	}

	// 5. REJOINING — register with a fresh ephemeral lease under
	//    the stable member ID.
	r.setPhase(RecoveryRejoining)
	if err := r.cfg.Membership.Register(ctx); err != nil {
		return r.failPhase(RecoveryRejoining, fmt.Errorf("register: %w", err))
	}

	// 6. RECLAIMING — claim the agents that are ours per the
	//    (post-rejoin) shard map.
	r.setPhase(RecoveryReclaiming)
	if err := r.reclaim(ctx); err != nil {
		return r.failPhase(RecoveryReclaiming, err)
	}

	// 7. COMPLETED.
	r.setPhase(RecoveryCompleted)
	r.log.Info("recovery completed", "member", r.cfg.Membership.Self().ID)
	return nil
}

func (r *RecoveryManager) connect(ctx context.Context) error {
	var lastErr error
	for attempt := 1; attempt <= r.cfg.ConnectRetries; attempt++ {
		pctx, cancel := context.WithTimeout(ctx, r.cfg.ConnectTimeout)
		_, err := r.cfg.Etcd.Get(pctx, recoveryProbeKey)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == r.cfg.ConnectRetries {
			break
		}
		select {
		case <-time.After(200 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("etcd unreachable after %d attempts: %w", r.cfg.ConnectRetries, lastErr)
}

func verifyAssignments(as []ShardAssignment) error {
	for _, a := range as {
		if a.AgentID == "" || a.MemberID == "" {
			return fmt.Errorf("corrupt shard assignment %+v", a)
		}
	}
	return nil
}

func (r *RecoveryManager) reclaim(ctx context.Context) error {
	selfID := r.cfg.Membership.Self().ID
	assignments, err := r.cfg.Shards.List(ctx)
	if err != nil {
		return fmt.Errorf("re-list shards: %w", err)
	}
	owned := make([]ShardAssignment, 0)
	for _, a := range assignments {
		if a.MemberID == selfID {
			owned = append(owned, a)
		}
	}
	if r.cfg.Reclaimer == nil || len(owned) == 0 {
		r.log.Info("recovery reclaim", "owned", len(owned), "reclaimer", r.cfg.Reclaimer != nil)
		return nil
	}
	if err := r.cfg.Reclaimer.ReclaimAgents(ctx, owned); err != nil {
		return fmt.Errorf("reclaim %d agents: %w", len(owned), err)
	}
	r.log.Info("recovery reclaimed agents", "count", len(owned))
	return nil
}

func (r *RecoveryManager) dispatch(ev RecoveryEvent) {
	r.obsMu.RLock()
	obs := make([]RecoveryObserver, len(r.observers))
	copy(obs, r.observers)
	r.obsMu.RUnlock()
	for _, o := range obs {
		o.OnRecovery(ev)
	}
}

// AddObserver registers o for recovery phase transitions. o must
// be a comparable value (pointer type) for RemoveObserver.
func (r *RecoveryManager) AddObserver(o RecoveryObserver) {
	if o == nil {
		return
	}
	r.obsMu.Lock()
	r.observers = append(r.observers, o)
	r.obsMu.Unlock()
}

// RemoveObserver deregisters o (identity comparison).
func (r *RecoveryManager) RemoveObserver(o RecoveryObserver) {
	r.obsMu.Lock()
	defer r.obsMu.Unlock()
	for i, x := range r.observers {
		if x == o {
			r.observers = append(r.observers[:i], r.observers[i+1:]...)
			return
		}
	}
}
