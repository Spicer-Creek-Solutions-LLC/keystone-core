// SPDX-License-Identifier: Apache-2.0

//go:build integration

package ha

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/cluster"
)

// startFencing wires a real FencingManager on the shared etcd with a
// controllable quorum source (HealthMonitor detection is unit-tested
// separately; here the quorum signal is driven deterministically so
// the 1s minority-block bound is measurable without racing a real
// etcd partition).
func startFencing(t *testing.T, etcd *cluster.EtcdClient, q *fakeQuorum) *cluster.FencingManager {
	t.Helper()
	fm, err := cluster.NewFencingManager(cluster.FencingManagerConfig{
		Quorum:     q,
		Leadership: &fakeLeadership{},
		Etcd:       etcd,
		KeyPrefix:  keyPrefix,
		Mode:       cluster.FenceReadOnly, // §4.15 default
	})
	if err != nil {
		t.Fatalf("NewFencingManager: %v", err)
	}
	if err := fm.Start(context.Background()); err != nil {
		t.Fatalf("Fencing.Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = fm.Stop(ctx)
	})
	return fm
}

// TestHA_MinorityPartitionBlocksWritesWithin1s: the acceptance line
// — a minority partition rejects writes within 1s while reads
// continue (FenceReadOnly), and heals automatically when quorum
// returns.
func TestHA_MinorityPartitionBlocksWritesWithin1s(t *testing.T) {
	etcd := startEtcd(t)
	q := newFakeQuorum()
	fm := startFencing(t, etcd, q)

	// Healthy: writes allowed.
	rel, err := fm.Guard(cluster.OpWrite)
	if err != nil {
		t.Fatalf("write guard with quorum OK: %v", err)
	}
	rel()

	// Partition → minority. Assert the write block engages within
	// the 1s correctness budget (asserted with margin).
	start := time.Now()
	q.set(cluster.QuorumMinority)
	waitFor(t, fenceBudget, "writes blocked after minority partition", func() bool {
		_, gerr := fm.Guard(cluster.OpWrite)
		return errors.Is(gerr, cluster.ErrFenced)
	})
	if elapsed := time.Since(start); elapsed > fenceBudget {
		t.Fatalf("write block took %s, want <= %s", elapsed, fenceBudget)
	}

	// Reads continue during the partition (FenceReadOnly).
	rrel, rerr := fm.Guard(cluster.OpRead)
	if rerr != nil {
		t.Fatalf("read guard during minority partition: %v (reads must continue)", rerr)
	}
	rrel()
	if !fm.Fenced() {
		t.Fatal("Fenced() = false during minority partition")
	}

	// Heal: quorum restored → writes resume automatically.
	q.set(cluster.QuorumOK)
	waitFor(t, settleBudget, "writes resume after partition heals", func() bool {
		r, gerr := fm.Guard(cluster.OpWrite)
		if gerr == nil {
			r()
			return true
		}
		return false
	})
	if fm.Fenced() {
		t.Fatal("Fenced() = true after quorum restored — did not auto-heal")
	}
}

// TestHA_SplitBrainNoDoubleLeader: under a symmetric partition the
// single-leader invariant must never break, and the minority side
// must fence (no two nodes both believing they lead + accepting
// writes). The cluster + fencing are real; the partition is the
// quorum-signal seam.
func TestHA_SplitBrainNoDoubleLeader(t *testing.T) {
	etcd := startEtcd(t)
	nodes := newCluster(t, etcd, "m1", "m2", "m3")
	waitFor(t, settleBudget, "leader elected", func() bool { return leaderOf(t, nodes) != "" })

	q := newFakeQuorum()
	fm := startFencing(t, etcd, q)

	// Symmetric partition: the minority side loses quorum and must
	// fence writes; the single-leader invariant must hold throughout.
	q.set(cluster.QuorumMinority)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		count := 0
		for _, n := range nodes {
			if n.Election.IsLeader() {
				count++
			}
		}
		if count > 1 {
			t.Fatalf("split-brain: %d simultaneous leaders during partition", count)
		}
		time.Sleep(20 * time.Millisecond)
	}
	waitFor(t, fenceBudget, "minority side fenced for writes", func() bool {
		_, gerr := fm.Guard(cluster.OpWrite)
		return errors.Is(gerr, cluster.ErrFenced)
	})

	// Heal restores write availability (automatic healing).
	q.set(cluster.QuorumOK)
	waitFor(t, settleBudget, "split-brain heals", func() bool {
		r, gerr := fm.Guard(cluster.OpWrite)
		if gerr == nil {
			r()
			return true
		}
		return false
	})
}

// TestHA_EtcdFailureFailsSafe: when etcd becomes unreachable,
// cluster operations fail loudly rather than silently succeeding
// (no data loss / no false success). The literal "stop + restart
// etcd node" multi-process form is the boot-gated variant — here we
// assert the fail-safe property an unreachable backing store must
// have (EtcdClient is single-use, so this asserts the failure edge,
// not in-place restart).
func TestHA_EtcdFailureFailsSafe(t *testing.T) {
	etcd := startEtcd(t)
	nodes := newCluster(t, etcd, "m1", "m2", "m3")
	waitFor(t, settleBudget, "leader elected", func() bool { return leaderOf(t, nodes) != "" })
	lead := nodes[0]

	ctx := context.Background()
	if _, err := lead.Shards.AssignAgent(ctx, "a1"); err != nil {
		t.Fatalf("baseline AssignAgent: %v", err)
	}

	// Take etcd down.
	sctx, scancel := context.WithTimeout(ctx, 15*time.Second)
	defer scancel()
	if err := etcd.Stop(sctx); err != nil {
		t.Fatalf("etcd Stop: %v", err)
	}

	// Operations must now error, not silently succeed.
	if _, err := lead.Store.Assign(ctx, "a2", "m1"); err == nil {
		t.Fatal("ShardStore.Assign succeeded with etcd down — fail-safe violated")
	}
	if _, err := lead.Membership.LoadMembers(ctx); err == nil {
		t.Fatal("LoadMembers succeeded with etcd down — fail-safe violated")
	}
}
