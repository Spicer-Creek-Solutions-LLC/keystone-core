//go:build integration || slo

// Package ha holds the Epic 13 task 17 HA end-to-end suite + the
// task 18 SLO gate. The shared harness compiles under either tag;
// the functional scenario files are `integration`-only and the SLO
// timing file is `slo`-only, so the two never cross-pull.
//
// Scope (honest — see test/e2e/ha/README.md): these are
// *in-process, component-level* HA E2E tests against the real
// internal/cluster stack + real CoordinationService gRPC over a
// real mTLS listener + embedded etcd. A literal multi-process
// 3×kscore-server / iptables-partition form is blocked by the
// gate-v1.0 boot-wiring entries (ClusterService/CoordinationService
// not registered at boot, FencingManager.Guard not wired around
// write handlers, SingletonTaskManager not constructed at boot);
// this suite is the harness those entries graduate against — it
// proves the HA *mechanisms* now and the full server-integrated
// form lands when boot wiring does.
//
// The "3-node cluster" is the §4.15 model: N
// MembershipManager+LeaderElector+ShardManager+FailoverManager
// instances sharing one embedded EtcdClient (embedded etcd ≤3
// members; Keystone members are keyed by ID on shared etcd).
//
// Build-tagged `integration` so it stays out of the default
// `make test`; CI runs it on every PR via `make test-integration`.
package ha

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/cluster"
)

// ---- timing -------------------------------------------------------------

// Functional correctness uses generous CI-safe budgets. Tight SLO
// numbers (<3s leader, <10s failover, <15s recovery) are Task 18's
// job; the one tight bound asserted here is the 1s minority-block,
// because fast fencing is a correctness property (asserted with
// margin via fenceBudget).
const (
	settleBudget = 20 * time.Second
	fenceBudget  = 1 * time.Second
)

// waitFor polls cond until true or the budget expires.
func waitFor(t *testing.T, budget time.Duration, what string, cond func() bool) {
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

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

// ---- embedded etcd ------------------------------------------------------

// startEtcd boots one embedded etcd the whole Keystone cluster
// shares (the §4.15 model).
func startEtcd(t *testing.T) *cluster.EtcdClient {
	t.Helper()
	c, err := cluster.NewEtcdClient(cluster.EtcdConfig{
		Mode:       cluster.ModeEmbedded,
		Name:       "ha-etcd",
		DataDir:    t.TempDir(),
		ClientURLs: []string{fmt.Sprintf("http://127.0.0.1:%d", freePort(t))},
		PeerURLs:   []string{fmt.Sprintf("http://127.0.0.1:%d", freePort(t))},
		LeaseTTL:   10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewEtcdClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("etcd Start: %v", err)
	}
	t.Cleanup(func() {
		sctx, sc := context.WithTimeout(context.Background(), 15*time.Second)
		defer sc()
		_ = c.Stop(sctx)
	})
	return c
}

// ---- a Keystone cluster member ------------------------------------------

const keyPrefix = "/kscore/ha"

// node is one Keystone cluster member: its own membership +
// election + shard manager + failover manager, all on the shared
// etcd. Built incrementally so individual scenarios wire only what
// they exercise.
type node struct {
	id   string
	etcd *cluster.EtcdClient

	Membership *cluster.MembershipManager
	Election   *cluster.LeaderElector
	Store      *cluster.ShardStore
	Shards     *cluster.ShardManager
	Failover   *cluster.FailoverManager
}

func newNode(t *testing.T, etcd *cluster.EtcdClient, id string) *node {
	t.Helper()
	mm, err := cluster.NewMembershipManager(cluster.MembershipConfig{
		Etcd:              etcd,
		MemberName:        id,
		MemberID:          id,
		Addr:              id + ":7000",
		KeyPrefix:         keyPrefix,
		HeartbeatInterval: 250 * time.Millisecond,
		LeaseTTL:          10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewMembershipManager(%s): %v", id, err)
	}
	le, err := cluster.NewLeaderElector(cluster.ElectionConfig{
		Etcd:       etcd,
		MemberID:   id,
		KeyPrefix:  keyPrefix + "/leader",
		SessionTTL: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewLeaderElector(%s): %v", id, err)
	}
	ss, err := cluster.NewShardStore(cluster.ShardStoreConfig{Etcd: etcd, KeyPrefix: keyPrefix})
	if err != nil {
		t.Fatalf("NewShardStore(%s): %v", id, err)
	}
	sm, err := cluster.NewShardManager(cluster.ShardManagerConfig{
		Membership: mm,
		Store:      ss,
		// 0 cooldown: the first failure must rebalance immediately
		// (production: HealthMonitor + ShardManager react to the
		// same membership event with no debounce). A non-zero
		// cooldown ≥ FailoverManager's 200ms settle would make the
		// reassignment moves miss the failover episode snapshot.
		RebalanceCooldown: 0,
		LeaderCheck:       le.IsLeader,
	})
	if err != nil {
		t.Fatalf("NewShardManager(%s): %v", id, err)
	}
	return &node{id: id, etcd: etcd, Membership: mm, Election: le, Store: ss, Shards: sm}
}

// start registers membership, starts election + shard manager, and
// registers cleanup in reverse order.
func (n *node) start(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if err := n.Membership.Register(ctx); err != nil {
		t.Fatalf("Register(%s): %v", n.id, err)
	}
	if err := n.Election.Start(ctx); err != nil {
		t.Fatalf("Election.Start(%s): %v", n.id, err)
	}
	if err := n.Shards.Start(ctx); err != nil {
		t.Fatalf("Shards.Start(%s): %v", n.id, err)
	}
	t.Cleanup(func() { n.stop() })
}

func (n *node) stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = n.Shards.Stop(ctx)
	_ = n.Election.Stop(ctx)
	_ = n.Membership.Stop(ctx)
}

// newCluster builds + starts n members on a shared etcd.
func newCluster(t *testing.T, etcd *cluster.EtcdClient, ids ...string) []*node {
	t.Helper()
	nodes := make([]*node, 0, len(ids))
	for _, id := range ids {
		nd := newNode(t, etcd, id)
		nd.start(t)
		nodes = append(nodes, nd)
	}
	return nodes
}

// leaderOf returns the single elected leader's id, or "" if not yet
// converged / split.
func leaderOf(t *testing.T, nodes []*node) string {
	t.Helper()
	leaders := make([]string, 0, 1)
	for _, n := range nodes {
		if n.Election.IsLeader() {
			leaders = append(leaders, n.id)
		}
	}
	if len(leaders) == 1 {
		return leaders[0]
	}
	return "" // 0 = no leader yet; >1 = split (must never persist)
}

// ---- controllable seams -------------------------------------------------

// fakeQuorum is a controllable cluster quorum source satisfying the
// FencingManager Quorum seam. HealthMonitor's *detection* is unit-
// tested separately; here we drive the quorum signal deterministically
// so the 1s minority-block bound is measurable without racing a real
// etcd partition.
type fakeQuorum struct {
	mu        sync.Mutex
	state     cluster.QuorumState
	observers []cluster.HealthObserver
}

func newFakeQuorum() *fakeQuorum {
	return &fakeQuorum{state: cluster.QuorumOK}
}

func (q *fakeQuorum) Quorum() cluster.QuorumState {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.state
}

func (q *fakeQuorum) AddObserver(o cluster.HealthObserver) {
	q.mu.Lock()
	q.observers = append(q.observers, o)
	q.mu.Unlock()
}

func (q *fakeQuorum) RemoveObserver(o cluster.HealthObserver) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, x := range q.observers {
		if x == o {
			q.observers = append(q.observers[:i], q.observers[i+1:]...)
			return
		}
	}
}

// set flips the quorum state and notifies observers (what a real
// HealthMonitor does on quorum loss/recovery).
func (q *fakeQuorum) set(s cluster.QuorumState) {
	q.mu.Lock()
	q.state = s
	obs := append([]cluster.HealthObserver(nil), q.observers...)
	q.mu.Unlock()
	for _, o := range obs {
		o.OnHealthChange(cluster.HealthEvent{Status: cluster.MemberHealthy, Quorum: s})
	}
}

// fakeLeadership is the minimal FencingManager Leadership seam (the
// epoch-fencing path is etcd-driven; quorum tests don't need real
// leadership churn).
type fakeLeadership struct {
	mu        sync.Mutex
	observers []cluster.LeadershipObserver
}

func (l *fakeLeadership) AddObserver(o cluster.LeadershipObserver) {
	l.mu.Lock()
	l.observers = append(l.observers, o)
	l.mu.Unlock()
}

func (l *fakeLeadership) RemoveObserver(o cluster.LeadershipObserver) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, x := range l.observers {
		if x == o {
			l.observers = append(l.observers[:i], l.observers[i+1:]...)
			return
		}
	}
}
