// SPDX-License-Identifier: Apache-2.0

package cluster

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeObsSource satisfies both failoverMembership and failoverShards
// (Add/RemoveObserver no-ops). Tests drive events by calling the
// FailoverManager's observer methods directly — deterministic, no
// embedded etcd (FailoverManager's contract is observer-in →
// reassigner-out).
type fakeObsSource struct{}

func (fakeObsSource) AddObserver(MembershipObserver)    {}
func (fakeObsSource) RemoveObserver(MembershipObserver) {}

type fakeShardSrc struct{}

func (fakeShardSrc) AddObserver(RebalanceObserver)    {}
func (fakeShardSrc) RemoveObserver(RebalanceObserver) {}

type recAgent struct {
	mu      sync.Mutex
	batches [][]ShardMove
	total   int
	err     error
}

func (r *recAgent) ReassignAgents(_ context.Context, m []ShardMove) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.batches = append(r.batches, m)
	r.total += len(m)
	return nil
}
func (r *recAgent) snap() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.batches), r.total
}

type recJob struct {
	mu      sync.Mutex
	jobs    []JobRef
	keys    []string
	total   int
	listErr error
	reErr   error
}

func (r *recJob) ListJobs(context.Context, string) ([]JobRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.jobs, r.listErr
}
func (r *recJob) ReassignJobs(_ context.Context, j []JobRef, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.reErr != nil {
		return r.reErr
	}
	r.keys = append(r.keys, key)
	r.total += len(j)
	return nil
}
func (r *recJob) snap() ([]string, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.keys...), r.total
}

type failRec struct {
	mu sync.Mutex
	ev []FailoverEvent
}

func (r *failRec) OnFailover(e FailoverEvent) {
	r.mu.Lock()
	r.ev = append(r.ev, e)
	r.mu.Unlock()
}
func (r *failRec) states() []FailoverState {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]FailoverState, len(r.ev))
	for i, e := range r.ev {
		out[i] = e.State
	}
	return out
}
func (r *failRec) sawState(s FailoverState) bool {
	for _, x := range r.states() {
		if x == s {
			return true
		}
	}
	return false
}

func startFailover(t *testing.T, cfg FailoverManagerConfig) *FailoverManager {
	t.Helper()
	if cfg.Membership == nil {
		cfg.Membership = fakeObsSource{}
	}
	if cfg.Shards == nil {
		cfg.Shards = fakeShardSrc{}
	}
	fm, err := NewFailoverManager(cfg)
	if err != nil {
		t.Fatalf("NewFailoverManager: %v", err)
	}
	if err := fm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = fm.Stop(sctx)
	})
	return fm
}

func movesFrom(from, to string, n int) []ShardMove {
	out := make([]ShardMove, n)
	for i := range out {
		out[i] = ShardMove{AgentID: fmt.Sprintf("agent-%d", i), From: from, To: to}
	}
	return out
}

func TestNewFailoverManager_InvalidConfig(t *testing.T) {
	if _, err := NewFailoverManager(FailoverManagerConfig{Shards: fakeShardSrc{}}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil membership: %v", err)
	}
	if _, err := NewFailoverManager(FailoverManagerConfig{Membership: fakeObsSource{}}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil shards: %v", err)
	}
}

func TestFailover_EpisodeAgentsAndJobsBatched(t *testing.T) {
	ag := &recAgent{}
	jb := &recJob{jobs: make([]JobRef, 120)}
	for i := range jb.jobs {
		jb.jobs[i] = JobRef{ID: fmt.Sprintf("job-%d", i)}
	}
	rec := &failRec{}
	fm := startFailover(t, FailoverManagerConfig{
		AgentReassigner: ag, JobReassigner: jb,
		Cooldown: 0, AgentBatch: 100, JobBatch: 50,
	})
	fm.AddObserver(rec)

	fm.OnMembershipChange(MemberEvent{Type: MemberLeft, Member: Member{ID: "node-x"}})
	fm.OnRebalance(movesFrom("node-x", "node-y", 250))

	waitFor(t, func() bool { return fm.State() == FailoverCompleted }, "episode complete")

	nb, ntotal := ag.snap()
	if ntotal != 250 || nb != 3 { // 100,100,50
		t.Fatalf("agent reassign batches=%d total=%d, want 3/250", nb, ntotal)
	}
	keys, jtotal := jb.snap()
	if jtotal != 120 || len(keys) != 3 { // 50,50,20
		t.Fatalf("job reassign keys=%v total=%d, want 3/120", keys, jtotal)
	}
	// Idempotency keys: deterministic, unique, correctly shaped.
	seen := map[string]bool{}
	for i, k := range keys {
		want := fmt.Sprintf("failover/node-x/1/%d", i)
		if k != want {
			t.Fatalf("idempotency key[%d]=%q want %q", i, k, want)
		}
		if seen[k] {
			t.Fatalf("duplicate idempotency key %q", k)
		}
		seen[k] = true
	}
	for _, s := range []FailoverState{FailoverDetecting, FailoverInitiated, FailoverInProgress, FailoverCompleted} {
		if !rec.sawState(s) {
			t.Fatalf("missing state %q in %v", s, rec.states())
		}
	}
}

func TestFailover_IgnoresJoinDrivenRebalance(t *testing.T) {
	ag := &recAgent{}
	fm := startFailover(t, FailoverManagerConfig{AgentReassigner: ag, Cooldown: 0})

	// From a healthy member (never marked failed) → not a failover.
	fm.OnRebalance(movesFrom("healthy-a", "new-b", 50))
	time.Sleep(400 * time.Millisecond) // > failoverSettle

	if nb, _ := ag.snap(); nb != 0 {
		t.Fatalf("join rebalance triggered %d agent reassign batches, want 0", nb)
	}
	if fm.State() != FailoverIdle {
		t.Fatalf("State = %q, want idle", fm.State())
	}
}

func TestFailover_ReassignerErrorFailsThenRollback(t *testing.T) {
	ag := &recAgent{err: errors.New("reassign boom")}
	var rolledBack tbool
	rec := &failRec{}
	fm := startFailover(t, FailoverManagerConfig{
		AgentReassigner: ag,
		Cooldown:        0,
		Rollback: func(context.Context, string) error {
			rolledBack.set(true)
			return nil
		},
	})
	fm.AddObserver(rec)

	fm.OnMembershipChange(MemberEvent{Type: MemberUpdated, Member: Member{ID: "node-x", Status: MemberUnhealthy}})
	fm.OnRebalance(movesFrom("node-x", "node-y", 10))

	waitFor(t, func() bool { return fm.State() == FailoverRolledBack }, "rolled back")
	if !rec.sawState(FailoverFailed) {
		t.Fatalf("expected FAILED before ROLLED_BACK: %v", rec.states())
	}
	if !rolledBack.get() {
		t.Fatal("rollback hook not invoked")
	}
}

func TestFailover_LeaderGate(t *testing.T) {
	ag := &recAgent{}
	var leader tbool // false
	fm := startFailover(t, FailoverManagerConfig{
		AgentReassigner: ag,
		Cooldown:        0,
		LeaderCheck:     leader.get,
	})

	fm.OnMembershipChange(MemberEvent{Type: MemberLeft, Member: Member{ID: "node-x"}})
	fm.OnRebalance(movesFrom("node-x", "node-y", 20))
	waitFor(t, func() bool { return fm.State() == FailoverCompleted }, "non-leader episode completes")
	if nb, _ := ag.snap(); nb != 0 {
		t.Fatalf("non-leader performed %d reassign batches, want 0", nb)
	}

	leader.set(true)
	fm.OnMembershipChange(MemberEvent{Type: MemberLeft, Member: Member{ID: "node-z"}})
	fm.OnRebalance(movesFrom("node-z", "node-y", 20))
	waitFor(t, func() bool { _, tot := ag.snap(); return tot == 20 }, "leader performs reassign")
}

func TestFailover_CooldownSpacesEpisodes(t *testing.T) {
	fm := startFailover(t, FailoverManagerConfig{Cooldown: 400 * time.Millisecond})

	var mu sync.Mutex
	var detects []time.Time
	fm.AddObserver(fObs(func(e FailoverEvent) {
		if e.State == FailoverDetecting {
			mu.Lock()
			detects = append(detects, time.Now())
			mu.Unlock()
		}
	}))

	fm.OnMembershipChange(MemberEvent{Type: MemberLeft, Member: Member{ID: "m1"}})
	fm.OnMembershipChange(MemberEvent{Type: MemberLeft, Member: Member{ID: "m2"}})
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(detects) >= 2
	}, "two episodes detected")

	mu.Lock()
	gap := detects[1].Sub(detects[0])
	mu.Unlock()
	if gap < 300*time.Millisecond { // ~cooldown, small slack
		t.Fatalf("episodes spaced %v, want >= cooldown (400ms)", gap)
	}
}

func TestFailover_ObserverAddRemove(t *testing.T) {
	fm := startFailover(t, FailoverManagerConfig{Cooldown: time.Second})
	o := &failRec{}
	fm.AddObserver(nil)
	fm.AddObserver(o)
	fm.RemoveObserver(o)
	fm.RemoveObserver(&failRec{})
}

func TestFailover_LifecycleErrors(t *testing.T) {
	fm, err := NewFailoverManager(FailoverManagerConfig{
		Membership: fakeObsSource{}, Shards: fakeShardSrc{}, Cooldown: 0,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	if err := fm.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := fm.Start(ctx); !errors.Is(err, ErrAlreadyStarted) {
		t.Errorf("double Start = %v", err)
	}
	if err := fm.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := fm.Stop(ctx); err != nil {
		t.Errorf("idempotent Stop = %v", err)
	}
	if err := fm.Start(ctx); !errors.Is(err, ErrStopped) {
		t.Errorf("Start after Stop = %v", err)
	}
}

// fObs adapts a func to FailoverObserver. Func values are not
// comparable, so a fObs must not be passed to RemoveObserver — only
// AddObserver (the cluster-package convention).
type fObs func(FailoverEvent)

func (f fObs) OnFailover(e FailoverEvent) { f(e) }

// tbool is a tiny mutex-guarded bool for tests.
type tbool struct {
	mu sync.Mutex
	v  bool
}

func (a *tbool) set(v bool) { a.mu.Lock(); a.v = v; a.mu.Unlock() }
func (a *tbool) get() bool  { a.mu.Lock(); defer a.mu.Unlock(); return a.v }
