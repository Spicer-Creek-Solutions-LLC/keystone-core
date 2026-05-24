// SPDX-License-Identifier: Apache-2.0

//go:build integration

// Integration-only harness helpers for the task-17 functional HA
// scenarios. Kept out of the `integration || slo` shared harness so
// the `slo` build (which does not compile the functional scenario
// files) sees no unused symbols.
package ha

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/cluster"
)

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
