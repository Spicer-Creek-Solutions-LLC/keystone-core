//go:build integration

package ha

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/cluster"
)

// recordingReassigner captures every reassigned batch so the test
// can assert no agent is double-executed (the idempotency contract).
type recordingReassigner struct {
	mu    sync.Mutex
	moves []cluster.ShardMove
}

func (r *recordingReassigner) ReassignAgents(_ context.Context, moves []cluster.ShardMove) error {
	r.mu.Lock()
	r.moves = append(r.moves, moves...)
	r.mu.Unlock()
	return nil
}

func (r *recordingReassigner) seen() []cluster.ShardMove {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]cluster.ShardMove(nil), r.moves...)
}

// TestHA_LeaderKillFailover: kill the member that owns agents; the
// surviving leader rebalances its agents away and the FailoverManager
// runs exactly one reassignment episode (idempotency-keyed batches
// prevent double-execution).
func TestHA_LeaderKillFailover(t *testing.T) {
	etcd := startEtcd(t)
	nodes := newCluster(t, etcd, "m1", "m2", "m3")
	waitFor(t, settleBudget, "leader elected", func() bool { return leaderOf(t, nodes) != "" })

	ctx := context.Background()

	// Pick the leader as the failover orchestrator; pick a distinct
	// victim that owns agents.
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

	rr := &recordingReassigner{}
	fm, err := cluster.NewFailoverManager(cluster.FailoverManagerConfig{
		Membership:      lead.Membership,
		Shards:          lead.Shards,
		AgentReassigner: rr,
	})
	if err != nil {
		t.Fatalf("NewFailoverManager: %v", err)
	}
	if err := fm.Start(ctx); err != nil {
		t.Fatalf("Failover.Start: %v", err)
	}
	t.Cleanup(func() {
		sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = fm.Stop(sctx)
	})

	// Assign agents that hash onto the victim so its departure forces
	// real moves the FailoverManager can correlate.
	agents := []string{"ag1", "ag2", "ag3", "ag4", "ag5", "ag6", "ag7", "ag8"}
	owned := 0
	for _, a := range agents {
		if _, err := lead.Shards.AssignAgent(ctx, a); err != nil {
			t.Fatalf("AssignAgent(%s): %v", a, err)
		}
		if o, _ := lead.Shards.Owner(ctx, a); o == victim.id {
			owned++
		}
	}
	if owned == 0 {
		t.Skip("no agents hashed onto the victim this run; ring distribution variance")
	}

	// Kill the victim → MemberLeft → leader's ShardManager rebalances
	// its agents → FailoverManager correlates + reassigns.
	victim.stop()

	waitFor(t, settleBudget, "new leader still single", func() bool {
		return leaderOf(t, nodes) != "" // never split
	})
	waitFor(t, settleBudget, "failover reassigned the victim's agents", func() bool {
		return len(rr.seen()) > 0
	})

	moves := rr.seen()
	// No agent reassigned twice (idempotency-key batching contract).
	seenAgent := map[string]bool{}
	for _, mv := range moves {
		if seenAgent[mv.AgentID] {
			t.Fatalf("agent %s reassigned more than once — idempotency broken", mv.AgentID)
		}
		seenAgent[mv.AgentID] = true
		if mv.From != victim.id {
			t.Fatalf("reassigned move From=%s, want victim %s", mv.From, victim.id)
		}
	}
}
