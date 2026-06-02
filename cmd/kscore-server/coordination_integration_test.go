// SPDX-License-Identifier: Apache-2.0

//go:build integration

// Epic 13 PR-B — boot-wiring integration test for the server↔server
// CoordinationService channel: a dedicated mTLS listener + the
// peer-dialing client, plus the SIGTERM graceful-shutdown sequence.
//
// Boots startCluster with a real embedded etcd + a real embedded
// identity provider and asserts: the coordination listener answers a
// real mTLS dial (NATS-independent leader lookup), rejects a non-mTLS
// caller, the default path starts no listener, and graceful shutdown
// deregisters the member.
package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"go.keystone-core.io/keystone-core/internal/cluster"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/identity"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// startTestIdentityProvider builds + starts an embedded identity
// provider with in-memory join tokens, for tests that need real mTLS
// material. Stopped via t.Cleanup.
func startTestIdentityProvider(t *testing.T) *identity.EmbeddedProvider {
	t.Helper()
	caStorage, err := identity.NewFileCAStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCAStorage: %v", err)
	}
	store := identity.NewInMemoryJoinTokenStore()
	attestor, err := identity.NewJoinTokenAttestor(identity.JoinTokenAttestorConfig{
		Store:       store,
		TrustDomain: identity.DefaultTrustDomain,
	})
	if err != nil {
		t.Fatalf("NewJoinTokenAttestor: %v", err)
	}
	p, err := identity.NewEmbeddedProvider(identity.EmbeddedProviderConfig{
		CAConfig:       identity.DefaultCAConfig(identity.DefaultTrustDomain),
		Storage:        caStorage,
		Logger:         silentLogger(),
		JoinTokenStore: store,
		Attestors:      []identity.Attestor{attestor},
	})
	if err != nil {
		t.Fatalf("NewEmbeddedProvider: %v", err)
	}
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("identity provider Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	})
	return p
}

// coordClusterConfig extends enabledClusterConfig with a coordination
// listener bound to a free loopback port + a matching advertise addr.
func coordClusterConfig(t *testing.T, name string) config.ClusterConfig {
	t.Helper()
	c := enabledClusterConfig(t, name)
	port := freeClusterPort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	c.Node.AdvertiseAddr = addr
	c.Coordination = config.ClusterCoordinationConfig{
		ListenAddr:        addr,
		HeartbeatInterval: 500 * time.Millisecond,
		HeartbeatTimeout:  2 * time.Second,
		FailureThreshold:  3,
		RetryMax:          3,
		RetryBaseDelay:    50 * time.Millisecond,
		RetryMaxDelay:     500 * time.Millisecond,
	}
	c.Shutdown = config.ClusterShutdownConfig{Timeout: 10 * time.Second}
	return c
}

func TestStartCoordination_MTLSListenerAndShutdown(t *testing.T) {
	ctx := context.Background()
	provider := startTestIdentityProvider(t)
	cfg := coordClusterConfig(t, "coord-itest")

	rt, err := startCluster(ctx, cfg, provider, nil, silentLogger())
	if err != nil {
		t.Fatalf("startCluster: %v", err)
	}
	stopped := false
	t.Cleanup(func() {
		if stopped {
			return
		}
		stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		rt.stop(stopCtx)
	})

	if rt.coord == nil {
		t.Fatal("coordination runtime nil despite listen_addr set")
	}

	// Single member must win election so LookupLeader has an answer.
	waitForCond(t, 10*time.Second, "node becomes leader", rt.leaderCheck())

	// Real mTLS dial of the coordination listener: a NATS-independent
	// leader lookup must succeed over the server↔server channel.
	clientTLS, cancelTLS, err := identity.BuildClientTLSConfig(ctx, provider, &identity.ClientTLSOptions{Logger: silentLogger()})
	if err != nil {
		t.Fatalf("BuildClientTLSConfig: %v", err)
	}
	defer cancelTLS()

	conn, err := grpc.NewClient(cfg.Coordination.ListenAddr,
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
	if err != nil {
		t.Fatalf("dial coordination listener: %v", err)
	}
	defer func() { _ = conn.Close() }()

	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()
	resp, err := v1.NewCoordinationServiceClient(conn).LookupLeader(dialCtx, &v1.LookupLeaderRequest{})
	if err != nil {
		t.Fatalf("LookupLeader over mTLS coordination channel: %v", err)
	}
	if resp.GetLeaderId() == "" || !resp.GetIsSelf() {
		t.Fatalf("LookupLeader = %+v, want this node as leader", resp)
	}

	// ClusterHealth now answers over the channel (HealthMonitor wired):
	// before this PR the Health seam was nil and the RPC returned
	// Unavailable. A single node reaching etcd is healthy + quorate.
	health, err := v1.NewCoordinationServiceClient(conn).ClusterHealth(dialCtx, &v1.ClusterHealthRequest{})
	if err != nil {
		t.Fatalf("ClusterHealth over mTLS coordination channel: %v", err)
	}
	if health.GetMemberStatus() == "" {
		t.Fatal("ClusterHealth returned empty member status; HealthMonitor not wired")
	}
	if health.GetQuorum() != string(cluster.QuorumOK) {
		t.Fatalf("ClusterHealth quorum = %q, want %q", health.GetQuorum(), cluster.QuorumOK)
	}

	// A non-mTLS (plaintext) caller must be rejected.
	insecureConn, err := grpc.NewClient(cfg.Coordination.ListenAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial (insecure): %v", err)
	}
	defer func() { _ = insecureConn.Close() }()
	insCtx, insCancel := context.WithTimeout(ctx, 3*time.Second)
	defer insCancel()
	if _, err := v1.NewCoordinationServiceClient(insecureConn).LookupLeader(insCtx, &v1.LookupLeaderRequest{}); err == nil {
		t.Fatal("insecure caller reached CoordinationService — mTLS not enforced")
	}

	// Graceful shutdown deregisters this member (revokes the lease →
	// member key gone), then a clean stop. Verify the member key is
	// gone by reading etcd directly — the MembershipManager rejects
	// LoadMembers once deregistered, so we can't go through it.
	if err := rt.gracefulShutdown(ctx, cfg.Shutdown.Timeout); err != nil {
		t.Fatalf("gracefulShutdown: %v", err)
	}
	memberKeys, err := rt.etcd.Get(ctx, rt.memberKeyPrefix, clientv3.WithPrefix())
	if err != nil {
		t.Fatalf("etcd Get member prefix after shutdown: %v", err)
	}
	if len(memberKeys.Kvs) != 0 {
		t.Fatalf("after graceful shutdown %d member keys remain, want 0 (deregistered)", len(memberKeys.Kvs))
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	rt.stop(stopCtx) // idempotent with the graceful stop-accepting hook
	stopped = true
}

func TestStartCoordination_DisabledWhenNoListenAddr(t *testing.T) {
	ctx := context.Background()
	provider := startTestIdentityProvider(t)
	cfg := enabledClusterConfig(t, "no-coord-itest") // no Coordination.ListenAddr

	rt, err := startCluster(ctx, cfg, provider, nil, silentLogger())
	if err != nil {
		t.Fatalf("startCluster: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		rt.stop(stopCtx)
	})
	if rt.coord != nil {
		t.Fatal("coordination runtime started despite empty listen_addr")
	}
}

func TestStartCoordination_RequiresIdentity(t *testing.T) {
	ctx := context.Background()
	cfg := coordClusterConfig(t, "coord-noident-itest")

	// listen_addr set but identity provider nil ⇒ the mTLS-only channel
	// must refuse to start (and the whole boot fails loudly).
	rt, err := startCluster(ctx, cfg, nil, nil, silentLogger())
	if err == nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		rt.stop(stopCtx)
		cancel()
		t.Fatal("startCluster succeeded with coordination listener but no identity; want error")
	}
}
