//go:build integration

// Package ha holds the Epic 13 task 17 HA end-to-end suite.
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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
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

// fakeMembers is a controllable membershipSource (LoadMembers +
// observer registration) — the internal shardmanager_test
// composition. It lets a ShardManager exercise the real
// ring + ShardStore + etcd path without a live MembershipManager
// watch concurrently churning the topology.
type fakeMembers struct {
	mu        sync.Mutex
	members   []cluster.Member
	observers []cluster.MembershipObserver
}

func (f *fakeMembers) LoadMembers(context.Context) ([]cluster.Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]cluster.Member(nil), f.members...), nil
}

func (f *fakeMembers) AddObserver(o cluster.MembershipObserver) {
	f.mu.Lock()
	f.observers = append(f.observers, o)
	f.mu.Unlock()
}

func (f *fakeMembers) RemoveObserver(o cluster.MembershipObserver) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, x := range f.observers {
		if x == o {
			f.observers = append(f.observers[:i], f.observers[i+1:]...)
			return
		}
	}
}

// join adds a member and notifies observers (a MemberJoined the way
// a real MembershipManager watch would).
func (f *fakeMembers) join(id string) {
	f.mu.Lock()
	f.members = append(f.members, cluster.Member{ID: id, Status: cluster.MemberHealthy})
	obs := append([]cluster.MembershipObserver(nil), f.observers...)
	f.mu.Unlock()
	for _, o := range obs {
		o.OnMembershipChange(cluster.MemberEvent{
			Type:   cluster.MemberJoined,
			Member: cluster.Member{ID: id, Status: cluster.MemberHealthy},
		})
	}
}

// newShardOnly builds + starts an isolated real ShardManager (real
// HashRing + real ShardStore on the shared etcd) over a controllable
// fakeMembers — no live MembershipManager watch interfering. Leader
// by construction (LeaderCheck always true) so it persists.
func newShardOnly(t *testing.T, etcd *cluster.EtcdClient, prefix string) (*cluster.ShardManager, *fakeMembers) {
	t.Helper()
	ss, err := cluster.NewShardStore(cluster.ShardStoreConfig{Etcd: etcd, KeyPrefix: prefix})
	if err != nil {
		t.Fatalf("NewShardStore: %v", err)
	}
	fm := &fakeMembers{}
	sm, err := cluster.NewShardManager(cluster.ShardManagerConfig{
		Membership:        fm,
		Store:             ss,
		RebalanceCooldown: 0,
		// LeaderCheck nil ⇒ always true (standalone): persists.
	})
	if err != nil {
		t.Fatalf("NewShardManager: %v", err)
	}
	if err := sm.Start(context.Background()); err != nil {
		t.Fatalf("ShardManager.Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = sm.Stop(ctx)
	})
	return sm, fm
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

// toggleNATS is the controllable coordNATS seam: "NATS down" is
// exactly what a real nats Manager surfaces to CoordinationService
// (Connected()=false), so toggling it reproduces the NATS-failure
// signal the recovery channel reacts to.
type toggleNATS struct {
	mu sync.Mutex
	up bool
}

func (n *toggleNATS) Connected() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.up
}

func (n *toggleNATS) Detail() string {
	if n.Connected() {
		return "connected"
	}
	return "down"
}

func (n *toggleNATS) setUp(up bool) {
	n.mu.Lock()
	n.up = up
	n.mu.Unlock()
}

// ---- in-test mTLS (the ca_storage_test minting pattern) -----------------

// mtlsPair returns server + client TLS configs sharing one in-test
// CA, so a dial populates VerifiedChains and CoordinationService's
// requireMTLS guard passes — a genuine mTLS path, not insecure.
func mtlsPair(t *testing.T) (server, client *tls.Config) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	caTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ha-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("ca cert: %v", err)
	}
	caCert, _ := x509.ParseCertificate(caDER)
	pool := x509.NewCertPool()
	pool.AddCert(caCert)

	leaf := func(cn string) tls.Certificate {
		k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatalf("leaf key: %v", e)
		}
		tpl := &x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
			IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		}
		der, e := x509.CreateCertificate(rand.Reader, tpl, caCert, &k.PublicKey, caKey)
		if e != nil {
			t.Fatalf("leaf cert: %v", e)
		}
		return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: k, Leaf: caCert}
	}

	server = &tls.Config{
		Certificates: []tls.Certificate{leaf("ha-server")},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	client = &tls.Config{
		Certificates: []tls.Certificate{leaf("ha-client")},
		RootCAs:      pool,
		ServerName:   "127.0.0.1",
		MinVersion:   tls.VersionTLS12,
	}
	return server, client
}
