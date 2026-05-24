// SPDX-License-Identifier: Apache-2.0

//go:build integration

package ha

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/cluster"
)

type recordingReclaimer struct {
	mu      sync.Mutex
	claimed []string
}

func (r *recordingReclaimer) ReclaimAgents(_ context.Context, owned []cluster.ShardAssignment) error {
	r.mu.Lock()
	for _, a := range owned {
		r.claimed = append(r.claimed, a.AgentID)
	}
	r.mu.Unlock()
	return nil
}

func (r *recordingReclaimer) sorted() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]string(nil), r.claimed...)
	sort.Strings(out)
	return out
}

// TestHA_MemberRestartReclaimsShards: a member crashes (lease
// revoked) but its shard-map entries persist. A restarted process
// re-registering under the SAME member ID reclaims exactly its own
// agents from the shard map — and only those (the stable-member-ID
// recovery contract).
func TestHA_MemberRestartReclaimsShards(t *testing.T) {
	etcd := startEtcd(t)
	ctx := context.Background()

	// Isolated real ShardStore on real etcd — no live ShardManager
	// rebalancing the map (a survivor's rebalancer reassigning the
	// crashed member's agents is exercised by the failover scenario;
	// here we isolate the stable-ID *reclaim* contract).
	const recPrefix = "/kscore/ha-recovery"
	store, err := cluster.NewShardStore(cluster.ShardStoreConfig{Etcd: etcd, KeyPrefix: recPrefix})
	if err != nil {
		t.Fatalf("NewShardStore: %v", err)
	}

	// Shard map persists across the crash: m2 owns ag2a/ag2b.
	want := []string{"ag2a", "ag2b"}
	for _, a := range want {
		if _, err := store.Assign(ctx, a, "m2"); err != nil {
			t.Fatalf("Assign(%s,m2): %v", a, err)
		}
	}
	for a, m := range map[string]string{"ag1a": "m1", "ag3a": "m3"} {
		if _, err := store.Assign(ctx, a, m); err != nil {
			t.Fatalf("Assign(%s,%s): %v", a, m, err)
		}
	}

	// Restart: a fresh MembershipManager re-registering under the
	// SAME stable ID "m2" + a one-shot RecoveryManager.
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
		t.Fatalf("restart MembershipManager: %v", err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mmNew.Stop(c)
	})

	rc := &recordingReclaimer{}
	rm, err := cluster.NewRecoveryManager(cluster.RecoveryManagerConfig{
		Etcd:       etcd,
		Membership: mmNew,
		Shards:     store,
		Reclaimer:  rc,
	})
	if err != nil {
		t.Fatalf("NewRecoveryManager: %v", err)
	}
	if err := rm.Recover(ctx); err != nil {
		t.Fatalf("Recover: %v (phase %s)", err, rm.Phase())
	}

	// Reclaimed exactly m2's agents — no more, no less.
	got := rc.sorted()
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("reclaimed %v, want %v (stable-ID recovery must reclaim only its own)", got, want)
	}
	// Recovery re-registered the member.
	waitFor(t, settleBudget, "m2 rejoined membership", func() bool {
		ms, lerr := mmNew.LoadMembers(ctx)
		if lerr != nil {
			return false
		}
		for _, m := range ms {
			if m.ID == "m2" {
				return true
			}
		}
		return false
	})
}
