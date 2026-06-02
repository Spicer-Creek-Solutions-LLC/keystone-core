// SPDX-License-Identifier: Apache-2.0

//go:build integration

// Epic 13 — boot-wiring integration test for the clustering stack.
//
// Boots startCluster against a real embedded etcd (single Keystone
// member) and asserts the boot glue: the node becomes leader so the
// canonical leader-check opens, the REST providers read live topology,
// the backup round-trips, and stop() tears everything down cleanly.
// Re-entrancy (the in-process harness calls boot helpers repeatedly) is
// covered by running the whole cycle twice on fresh etcd.
//
// Build-tagged `integration`: it starts a real embedded etcd + binds
// loopback ports. Not part of `make test`; run with
// `make test-integration`.
package main

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// freeClusterPort grabs an ephemeral loopback port for the embedded
// etcd client/peer listeners.
func freeClusterPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeClusterPort: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// enabledClusterConfig builds a valid single-node embedded-etcd cluster
// config with CI-fast timings (lease ≥ 3× heartbeat is preserved).
func enabledClusterConfig(t *testing.T, name string) config.ClusterConfig {
	t.Helper()
	c := config.ClusterConfig{Enabled: true}
	c.Node = config.ClusterNodeConfig{Name: name}
	c.Etcd = config.ClusterEtcdConfig{
		Mode:            "embedded",
		Name:            name,
		DataDir:         t.TempDir(),
		ClientURLs:      []string{fmt.Sprintf("http://127.0.0.1:%d", freeClusterPort(t))},
		PeerURLs:        []string{fmt.Sprintf("http://127.0.0.1:%d", freeClusterPort(t))},
		LeaseTTLSeconds: 3,
	}
	c.Membership = config.ClusterMembershipConfig{
		HeartbeatInterval: 1 * time.Second,
		KeyPrefix:         "/kscore/itest",
	}
	c.Election = config.ClusterElectionConfig{SessionTTLSeconds: 2}
	c.Shard = config.ClusterShardConfig{VirtualNodes: 150}
	return c
}

func TestStartCluster_Disabled(t *testing.T) {
	rt, err := startCluster(context.Background(), config.ClusterConfig{Enabled: false}, nil, nil, silentLogger())
	if err != nil {
		t.Fatalf("startCluster(disabled) error: %v", err)
	}
	if rt != nil {
		t.Fatalf("startCluster(disabled) = %v, want nil", rt)
	}
	// The nil runtime's leader-check must still be safe + permissive
	// (single-node default: every leader-only side-effect runs locally).
	if !rt.leaderCheck()() {
		t.Fatal("nil clusterRuntime leaderCheck must return true")
	}
}

func TestStartCluster_EnabledLifecycle(t *testing.T) {
	// Run twice on fresh etcd to prove the boot helper is re-entrant
	// (the in-process integration harness boots repeatedly).
	for i := 0; i < 2; i++ {
		t.Run(fmt.Sprintf("iteration-%d", i), func(t *testing.T) {
			ctx := context.Background()
			cfg := enabledClusterConfig(t, fmt.Sprintf("itest-%d", i))

			rt, err := startCluster(ctx, cfg, nil, nil, silentLogger())
			if err != nil {
				t.Fatalf("startCluster: %v", err)
			}
			if rt == nil {
				t.Fatal("startCluster(enabled) returned nil runtime")
			}
			t.Cleanup(func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				rt.stop(stopCtx)
			})

			// Single member ⇒ it must win election, opening the
			// canonical leader-check within the session TTL.
			leaderCheck := rt.leaderCheck()
			waitForCond(t, 10*time.Second, "node becomes leader", leaderCheck)

			rest := rt.restProviders()

			// Status provider is live (HealthMonitor-backed): a single
			// node that reaches its embedded etcd has quorum, and the
			// gRPC GetClusterStatus reports it as the one healthy member.
			if rest.Status == nil {
				t.Fatal("Status provider nil; HealthMonitor not wired")
			}
			if !rest.Status.Quorate() {
				t.Fatal("Status.Quorate() = false on a single node reaching etcd")
			}
			if rest.Status.ClusterName() != cfg.Node.Name {
				t.Fatalf("Status.ClusterName() = %q, want %q", rest.Status.ClusterName(), cfg.Node.Name)
			}
			st, err := rt.grpcServer().GetClusterStatus(ctx, &v1.GetClusterStatusRequest{})
			if err != nil {
				t.Fatalf("GetClusterStatus: %v", err)
			}
			if !st.GetQuorum() {
				t.Fatal("GetClusterStatus Quorum = false, want true")
			}
			if st.GetMemberCount() != 1 || st.GetHealthyCount() != 1 {
				t.Fatalf("GetClusterStatus members=%d healthy=%d, want 1/1",
					st.GetMemberCount(), st.GetHealthyCount())
			}

			// Leader provider reflects the won election.
			if !rest.Leader.IsLeader() {
				t.Fatal("Leader.IsLeader() = false after becoming leader")
			}

			// Members provider lists exactly this self member.
			members, err := rest.Members.List()
			if err != nil {
				t.Fatalf("Members.List: %v", err)
			}
			if len(members) != 1 {
				t.Fatalf("Members.List len = %d, want 1", len(members))
			}
			if members[0].Name != cfg.Node.Name {
				t.Fatalf("member name = %q, want %q", members[0].Name, cfg.Node.Name)
			}

			// Rebalance with zero agents is a no-op, not an error.
			moved, err := rest.Rebalance.Rebalance()
			if err != nil {
				t.Fatalf("Rebalance: %v", err)
			}
			if moved != 0 {
				t.Fatalf("Rebalance moved = %d, want 0 (no agents)", moved)
			}

			// Backup round-trips: create a snapshot, restore it.
			snap, err := rest.Backup.CreateBackup()
			if err != nil {
				t.Fatalf("CreateBackup: %v", err)
			}
			if len(snap) == 0 {
				t.Fatal("CreateBackup returned empty snapshot")
			}
			if _, err := rest.Backup.RestoreBackup(snap, true); err != nil {
				t.Fatalf("RestoreBackup: %v", err)
			}

			// Evict on the live runtime removes a (non-existent) member
			// key without error — proves the Evictor hook is wired.
			if err := rt.evict(ctx, "ghost-member"); err != nil {
				t.Fatalf("evict: %v", err)
			}
		})
	}
}

// waitForCond polls cond until true or the budget expires.
func waitForCond(t *testing.T, budget time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for: %s", budget, what)
}
