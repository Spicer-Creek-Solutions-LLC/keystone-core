// SPDX-License-Identifier: Apache-2.0

//go:build slo

// Epic 13 task 18 — performance SLO gate.
//
// These measure the §4.15 "v1.0 SLO targets (must meet in CI)"
// against the *real* in-process cluster mechanisms, reusing the
// task-17 harness. Run via `make slo` WITHOUT -race (race
// instrumentation inflates wall-clock 2–10×, so the numbers must be
// taken uninstrumented). Every measurement is logged even when it
// passes, so a regression that still squeaks under the bound is
// visible in CI output.
//
// Honest scope (the recurring boot-wiring pattern): these are the
// SLOs of the real mechanisms. The server-integrated end-to-end
// numbers (agent reconnection latency through a running
// multi-process kscore-server, graceful-shutdown zero-disconnect
// timing) ride with the "HA E2E multi-process / iptables-partition
// form" gate-v1.0 ROADMAP entry.
package ha

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/cluster"
)

// SLO bounds — PROJECT-DETAILS §4.15 "v1.0 SLO targets".
const (
	sloFirstLeader     = 3 * time.Second
	sloClusterForms    = 10 * time.Second
	sloFailoverDetect  = 5 * time.Second
	sloFailoverDone    = 10 * time.Second
	sloAgentReassign   = 10 * time.Second
	sloMinorityBlock   = 1 * time.Second
	sloRecoveryRestart = 15 * time.Second
)

// assertWithin logs the measured duration (always) and fails if it
// exceeds the SLO bound.
func assertWithin(t *testing.T, what string, got, bound time.Duration) {
	t.Helper()
	t.Logf("SLO %-26s measured=%-12s bound=%s", what, got.Round(time.Millisecond), bound)
	if got > bound {
		t.Fatalf("SLO VIOLATION: %s took %s, bound %s", what, got, bound)
	}
}

// TestSLO_FirstLeaderAndClusterForms: a 3-node cluster forms and
// elects its first leader within the SLO budgets (etcd already up —
// the bound is election/formation time, not etcd process boot).
func TestSLO_FirstLeaderAndClusterForms(t *testing.T) {
	etcd := startEtcd(t)

	start := time.Now()
	nodes := newCluster(t, etcd, "m1", "m2", "m3")

	waitFor(t, settleBudget, "first leader", func() bool { return leaderOf(t, nodes) != "" })
	firstLeader := time.Since(start)

	waitFor(t, settleBudget, "cluster formed", func() bool {
		ms, err := nodes[0].Membership.LoadMembers(context.Background())
		return err == nil && len(ms) == 3 && leaderOf(t, nodes) != ""
	})
	clusterForms := time.Since(start)

	assertWithin(t, "first leader", firstLeader, sloFirstLeader)
	assertWithin(t, "cluster forms", clusterForms, sloClusterForms)
}

// sloFailoverObs records the wall-clock of each failover state
// transition so detection + completion intervals are measurable.
type sloFailoverObs struct {
	mu sync.Mutex
	at map[cluster.FailoverState]time.Time
}

func newSLOFailoverObs() *sloFailoverObs {
	return &sloFailoverObs{at: map[cluster.FailoverState]time.Time{}}
}

func (o *sloFailoverObs) OnFailover(ev cluster.FailoverEvent) {
	o.mu.Lock()
	if _, seen := o.at[ev.State]; !seen {
		o.at[ev.State] = time.Now()
	}
	o.mu.Unlock()
}

func (o *sloFailoverObs) when(s cluster.FailoverState) (time.Time, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	tm, ok := o.at[s]
	return tm, ok
}

type sloReassigner struct {
	mu    sync.Mutex
	moves []cluster.ShardMove
}

func (r *sloReassigner) ReassignAgents(_ context.Context, mv []cluster.ShardMove) error {
	r.mu.Lock()
	r.moves = append(r.moves, mv...)
	r.mu.Unlock()
	return nil
}

func (r *sloReassigner) seen() []cluster.ShardMove {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]cluster.ShardMove(nil), r.moves...)
}

// TestSLO_FailoverDetectAndComplete: a member failure is detected
// within 5s and the reassignment episode completes within 10s
// (which also bounds the agent-reassignment SLO), with no agent
// reassigned twice (idempotency — zero job loss/duplication).
//
// Honest scope: this measures the failover *mechanism* (detect →
// reassign episode). Real agents reconnecting to the new owner is
// the boot-gated multi-process form.
func TestSLO_FailoverDetectAndComplete(t *testing.T) {
	etcd := startEtcd(t)
	nodes := newCluster(t, etcd, "m1", "m2", "m3")
	waitFor(t, settleBudget, "leader elected", func() bool { return leaderOf(t, nodes) != "" })

	var lead, victim *node
	for _, n := range nodes {
		if n.Election.IsLeader() {
			lead = n
		} else if victim == nil {
			victim = n
		}
	}
	if lead == nil || victim == nil {
		t.Fatal("need a leader and a non-leader victim")
	}

	ctx := context.Background()
	rr := &sloReassigner{}
	fm, err := cluster.NewFailoverManager(cluster.FailoverManagerConfig{
		Membership:      lead.Membership,
		Shards:          lead.Shards,
		AgentReassigner: rr,
	})
	if err != nil {
		t.Fatalf("NewFailoverManager: %v", err)
	}
	obs := newSLOFailoverObs()
	fm.AddObserver(obs)
	if err := fm.Start(ctx); err != nil {
		t.Fatalf("Failover.Start: %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = fm.Stop(c)
	})

	// Assign agents so the victim owns some — its departure then
	// forces real reassignment moves.
	for i := 0; i < 12; i++ {
		a := "agent-" + string(rune('A'+i))
		if _, err := lead.Shards.AssignAgent(ctx, a); err != nil {
			t.Fatalf("AssignAgent: %v", err)
		}
	}

	failAt := time.Now()
	victim.stop() // graceful leave → MemberLeft → failover trigger

	waitFor(t, settleBudget, "failover detected", func() bool {
		_, ok := obs.when(cluster.FailoverDetecting)
		return ok
	})
	waitFor(t, settleBudget, "failover completed", func() bool {
		_, ok := obs.when(cluster.FailoverCompleted)
		return ok
	})

	detectAt, _ := obs.when(cluster.FailoverDetecting)
	doneAt, _ := obs.when(cluster.FailoverCompleted)
	assertWithin(t, "failover detection", detectAt.Sub(failAt), sloFailoverDetect)
	assertWithin(t, "failover completion", doneAt.Sub(failAt), sloFailoverDone)
	assertWithin(t, "agent reassignment", doneAt.Sub(failAt), sloAgentReassign)

	// Zero duplication: no agent reassigned more than once.
	seenAgent := map[string]bool{}
	for _, mv := range rr.seen() {
		if seenAgent[mv.AgentID] {
			t.Fatalf("agent %s reassigned twice — idempotency/zero-dup SLO violated", mv.AgentID)
		}
		seenAgent[mv.AgentID] = true
	}
}

// TestSLO_MinorityBlocksWrites: on quorum loss the real
// FencingManager rejects writes within 1s (the exact bound, taken
// without -race so it is the true number).
func TestSLO_MinorityBlocksWrites(t *testing.T) {
	etcd := startEtcd(t)
	q := newFakeQuorum()
	fm, err := cluster.NewFencingManager(cluster.FencingManagerConfig{
		Quorum:     q,
		Leadership: &fakeLeadership{},
		Etcd:       etcd,
		KeyPrefix:  "/kscore/slo-fence",
		Mode:       cluster.FenceReadOnly,
	})
	if err != nil {
		t.Fatalf("NewFencingManager: %v", err)
	}
	if err := fm.Start(context.Background()); err != nil {
		t.Fatalf("Fencing.Start: %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = fm.Stop(c)
	})

	rel, err := fm.Guard(cluster.OpWrite)
	if err != nil {
		t.Fatalf("write guard with quorum OK: %v", err)
	}
	rel()

	start := time.Now()
	q.set(cluster.QuorumMinority)
	waitFor(t, settleBudget, "writes blocked", func() bool {
		_, gerr := fm.Guard(cluster.OpWrite)
		return errors.Is(gerr, cluster.ErrFenced)
	})
	assertWithin(t, "minority blocks writes", time.Since(start), sloMinorityBlock)
}

type sloReclaimer struct {
	mu      sync.Mutex
	claimed int
}

func (r *sloReclaimer) ReclaimAgents(_ context.Context, owned []cluster.ShardAssignment) error {
	r.mu.Lock()
	r.claimed += len(owned)
	r.mu.Unlock()
	return nil
}

// TestSLO_RecoveryRestart: a restarted member completes the full
// recovery sequence (connect → sync → verify → rejoin → reclaim)
// within 15s. Isolated real ShardStore (no live rebalancer churning
// the seeded map — the task-17 recovery composition).
func TestSLO_RecoveryRestart(t *testing.T) {
	etcd := startEtcd(t)
	ctx := context.Background()
	const recPrefix = "/kscore/slo-recovery"

	store, err := cluster.NewShardStore(cluster.ShardStoreConfig{Etcd: etcd, KeyPrefix: recPrefix})
	if err != nil {
		t.Fatalf("NewShardStore: %v", err)
	}
	for _, a := range []string{"ag2a", "ag2b", "ag2c"} {
		if _, err := store.Assign(ctx, a, "m2"); err != nil {
			t.Fatalf("Assign: %v", err)
		}
	}

	mmNew, err := cluster.NewMembershipManager(cluster.MembershipConfig{
		Etcd:              etcd,
		MemberName:        "m2",
		MemberID:          "m2",
		Addr:              "m2:7000",
		KeyPrefix:         recPrefix,
		HeartbeatInterval: 250 * time.Millisecond,
		LeaseTTL:          10 * time.Second,
	})
	if err != nil {
		t.Fatalf("MembershipManager: %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mmNew.Stop(c)
	})

	rc := &sloReclaimer{}
	rm, err := cluster.NewRecoveryManager(cluster.RecoveryManagerConfig{
		Etcd:       etcd,
		Membership: mmNew,
		Shards:     store,
		Reclaimer:  rc,
	})
	if err != nil {
		t.Fatalf("NewRecoveryManager: %v", err)
	}

	start := time.Now()
	if err := rm.Recover(ctx); err != nil {
		t.Fatalf("Recover: %v (phase %s)", err, rm.Phase())
	}
	assertWithin(t, "recovery (restart)", time.Since(start), sloRecoveryRestart)

	if rc.claimed != 3 {
		t.Fatalf("reclaimed %d agents, want 3 (recovery must reclaim its own shards)", rc.claimed)
	}
}
