//go:build integration

package ha

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"go.keystone-core.io/keystone-core/internal/cluster"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// stubHealth is the minimal coordHealth seam (health *detection* is
// unit-tested in internal/cluster; this scenario is about the
// NATS-down channel, not health aggregation).
type stubHealth struct{}

func (stubHealth) Status() cluster.MemberStatus { return cluster.MemberHealthy }
func (stubHealth) Quorum() cluster.QuorumState  { return cluster.QuorumOK }

// TestHA_NATSFailureCoordinationChannel: with NATS down, peers must
// still exchange health / leader / recovery over the mTLS
// CoordinationService (the NATS-fallback safety net). Tested over a
// real TLS TCP listener — a genuine mTLS path, not bufconn-insecure.
//
// Honest scope: CoordinationService is exercised directly, not via
// kscore-server boot (ClusterService/Coordination boot registration
// is the deferred gate-v1.0 entry this suite graduates against).
func TestHA_NATSFailureCoordinationChannel(t *testing.T) {
	etcd := startEtcd(t)
	nodes := newCluster(t, etcd, "m1", "m2", "m3")
	waitFor(t, settleBudget, "leader elected", func() bool { return leaderOf(t, nodes) != "" })
	lead := nodes[0]
	for _, n := range nodes {
		if n.Election.IsLeader() {
			lead = n
		}
	}

	nats := &toggleNATS{up: true}
	srv := &controlplane.CoordinationGRPCServer{
		Health:      stubHealth{},
		Leader:      lead.Election,
		Members:     lead.Membership,
		Shards:      lead.Store,
		NATS:        nats,
		SelfID:      lead.id,
		SelfVersion: "ha-test",
	}

	serverTLS, clientTLS := mtlsPair(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
	v1.RegisterCoordinationServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(),
		grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	cc := v1.NewCoordinationServiceClient(conn)

	call := func() (*v1.LookupLeaderResponse, *v1.NATSStatusResponse) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, herr := cc.ClusterHealth(ctx, &v1.ClusterHealthRequest{}); herr != nil {
			t.Fatalf("ClusterHealth over mTLS: %v", herr)
		}
		ll, lerr := cc.LookupLeader(ctx, &v1.LookupLeaderRequest{})
		if lerr != nil {
			t.Fatalf("LookupLeader over mTLS: %v", lerr)
		}
		ns, nerr := cc.NATSStatus(ctx, &v1.NATSStatusRequest{})
		if nerr != nil {
			t.Fatalf("NATSStatus over mTLS: %v", nerr)
		}
		return ll, ns
	}

	// NATS UP: baseline — leader resolvable, NATS reported connected.
	ll, ns := call()
	if ll.GetLeaderId() == "" {
		t.Fatal("LookupLeader returned empty leader with NATS up")
	}
	if !ns.GetConnected() {
		t.Fatal("NATSStatus.Connected = false, want true (NATS up)")
	}

	// NATS DOWN: the coordination channel must keep answering
	// health + leader (this IS the fallback), and report NATS down.
	nats.setUp(false)
	ll, ns = call()
	if ll.GetLeaderId() == "" {
		t.Fatal("leader unresolvable over CoordinationService with NATS down — fallback broken")
	}
	if ns.GetConnected() {
		t.Fatal("NATSStatus.Connected = true, want false (NATS down)")
	}

	// RecoveryCoordinate (a recovering peer's bootstrap) must work
	// with NATS down — best-effort snapshot from etcd-backed state.
	rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rcancel()
	rc, rerr := cc.RecoveryCoordinate(rctx, &v1.RecoveryCoordinateRequest{FromNodeId: "rejoiner"})
	if rerr != nil {
		t.Fatalf("RecoveryCoordinate with NATS down: %v", rerr)
	}
	if rc.GetLeaderId() == "" {
		t.Fatal("RecoveryCoordinate returned no leader with NATS down")
	}

	// NATS restored: status resumes.
	nats.setUp(true)
	_, ns = call()
	if !ns.GetConnected() {
		t.Fatal("NATSStatus.Connected = false after NATS restored")
	}
}

// TestHA_CoordinationRejectsNonMTLS: the §4.15 acceptance line —
// CoordinationService rejects callers without a verified client
// cert (insecure dial → Unauthenticated/transport failure).
func TestHA_CoordinationRejectsNonMTLS(t *testing.T) {
	srv := &controlplane.CoordinationGRPCServer{Health: stubHealth{}, SelfID: "m1"}
	serverTLS, _ := mtlsPair(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	gs := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)))
	v1.RegisterCoordinationServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if _, err := v1.NewCoordinationServiceClient(conn).
		ClusterHealth(ctx, &v1.ClusterHealthRequest{}); err == nil {
		t.Fatal("insecure caller reached CoordinationService — mTLS not enforced")
	}
}
