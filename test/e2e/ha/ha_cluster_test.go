// SPDX-License-Identifier: Apache-2.0

//go:build integration

package ha

import (
	"context"
	"fmt"
	"testing"
)

// TestHA_ClusterFormsAndElectsLeader: a 3-member cluster forms on a
// shared embedded etcd and converges on exactly one leader (never
// two — the split-brain invariant). Functional, not SLO-timed.
func TestHA_ClusterFormsAndElectsLeader(t *testing.T) {
	etcd := startEtcd(t)
	nodes := newCluster(t, etcd, "m1", "m2", "m3")

	waitFor(t, settleBudget, "exactly one leader", func() bool {
		return leaderOf(t, nodes) != ""
	})

	// Membership is visible to every node (shared etcd).
	waitFor(t, settleBudget, "all 3 members visible", func() bool {
		ms, err := nodes[0].Membership.LoadMembers(context.Background())
		return err == nil && len(ms) == 3
	})

	// The single-leader invariant must hold continuously, not just
	// at one sample (a transient double-leader is a split-brain bug).
	for i := 0; i < 20; i++ {
		count := 0
		for _, n := range nodes {
			if n.Election.IsLeader() {
				count++
			}
		}
		if count > 1 {
			t.Fatalf("split-brain: %d simultaneous leaders", count)
		}
	}
}

// TestHA_AddMemberMinimalRebalance: adding a 4th member rebalances
// onto the newcomer only — survivors' ownership is mostly preserved
// (the consistent-hash acceptance line).
//
// Topology is injected via the leader's ShardManager.OnMembership
// change seam directly (the proven internal/cluster shardmanager_test
// pattern), not via cross-node etcd-watch convergence — that watch
// latency is exactly the documented flake the internal suite avoids
// the same way. The ring + ShardStore + etcd + consistent-hash math
// under test are all real; only the membership *delivery* is made
// deterministic.
func TestHA_AddMemberMinimalRebalance(t *testing.T) {
	etcd := startEtcd(t)
	sm, fm := newShardOnly(t, etcd, "/kscore/ha-rebal")
	ctx := context.Background()

	// 3-member ring.
	fm.join("m1")
	fm.join("m2")
	fm.join("m3")

	// `agent-%d` keys + a large N is the proven sampling style of the
	// internal hashring_test (short, near-identical ids FNV-cluster
	// into a narrow ring arc and don't sample it). N is bounded so
	// the per-agent etcd Txn cost stays e2e-reasonable.
	const n = 500
	agents := make([]string, n)
	before := make(map[string]string, n)
	for i := 0; i < n; i++ {
		a := fmt.Sprintf("agent-%d", i)
		agents[i] = a
		owner, err := sm.AssignAgent(ctx, a)
		if err != nil {
			t.Fatalf("AssignAgent(%s): %v", a, err)
		}
		before[a] = owner
	}

	// Add the 4th member, then rebalance the persisted set.
	fm.join("m4")
	if _, err := sm.Rebalance(ctx); err != nil {
		t.Fatalf("Rebalance: %v", err)
	}

	moved, ontoM4 := 0, 0
	for _, a := range agents {
		now, err := sm.Owner(ctx, a)
		if err != nil {
			t.Fatalf("Owner(%s): %v", a, err)
		}
		if now != before[a] {
			moved++
			if now == "m4" {
				ontoM4++
			}
		}
	}
	// Consistent hashing: some keys move, but a minority (existing
	// assignments mostly preserved), and every moved key lands on
	// the newcomer — survivors are never reshuffled among the old
	// members (the acceptance invariant).
	if moved == 0 {
		t.Fatal("expected some agents to move onto m4")
	}
	if moved > n/2 {
		t.Fatalf("rebalance moved %d/%d agents — not minimal (assignments not mostly preserved)", moved, n)
	}
	if ontoM4 != moved {
		t.Fatalf("%d agents moved but only %d onto m4 — survivors reshuffled", moved, ontoM4)
	}
}
