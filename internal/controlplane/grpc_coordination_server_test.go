package controlplane

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"go.keystone-core.io/keystone-core/internal/cluster"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// --- fakes -------------------------------------------------------------

type fakeCoordHealth struct {
	st cluster.MemberStatus
	q  cluster.QuorumState
}

func (f fakeCoordHealth) Status() cluster.MemberStatus { return f.st }
func (f fakeCoordHealth) Quorum() cluster.QuorumState  { return f.q }

type fakeCoordLeader struct {
	id   string
	err  error
	self bool
}

func (f fakeCoordLeader) LeaderID(context.Context) (string, error) { return f.id, f.err }
func (f fakeCoordLeader) IsLeader() bool                           { return f.self }

type fakeCoordMembers struct {
	ms  []cluster.Member
	err error
}

func (f fakeCoordMembers) LoadMembers(context.Context) ([]cluster.Member, error) {
	return f.ms, f.err
}

type fakeCoordShards struct {
	as  []cluster.ShardAssignment
	err error
}

func (f fakeCoordShards) List(context.Context) ([]cluster.ShardAssignment, error) {
	return f.as, f.err
}

type fakeCoordNATS struct {
	conn   bool
	detail string
}

func (f fakeCoordNATS) Connected() bool { return f.conn }
func (f fakeCoordNATS) Detail() string  { return f.detail }

// mtlsCtx fabricates a context carrying a verified client cert (no
// real handshake needed to exercise requireMTLS).
func mtlsCtx() context.Context {
	return peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{
			VerifiedChains: [][]*x509.Certificate{{&x509.Certificate{}}},
		}},
	})
}

func fullCoordServer() *CoordinationGRPCServer {
	return &CoordinationGRPCServer{
		Health:      fakeCoordHealth{st: cluster.MemberHealthy, q: cluster.QuorumOK},
		Leader:      fakeCoordLeader{id: "leader-1", self: true},
		Members:     fakeCoordMembers{ms: []cluster.Member{{ID: "m1", Name: "n1", Addr: "a1", Status: cluster.MemberHealthy}, {ID: "m2"}}},
		Shards:      fakeCoordShards{as: []cluster.ShardAssignment{{AgentID: "ag1", MemberID: "m1", Version: 7}}},
		NATS:        fakeCoordNATS{conn: true, detail: "ok"},
		SelfID:      "node-self",
		SelfVersion: "v0.x",
	}
}

// --- mTLS guard --------------------------------------------------------

func TestCoordination_RejectsNonMTLS(t *testing.T) {
	s := fullCoordServer()

	// No peer at all.
	if _, err := s.ClusterHealth(context.Background(), &v1.ClusterHealthRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no-peer ClusterHealth = %v, want Unauthenticated", err)
	}
	// TLS but zero verified chains (no client cert). The non-TLS
	// auth-info path is covered end-to-end by the bufconn insecure
	// dial test below (insecure's AuthInfo type is unexported).
	noChain := peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{}},
	})
	if _, err := s.NATSStatus(noChain, &v1.NATSStatusRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no-chain NATSStatus = %v, want Unauthenticated", err)
	}
	// All six RPCs enforce it.
	for name, call := range map[string]func(context.Context) error{
		"ClusterHealth": func(c context.Context) error { _, e := s.ClusterHealth(c, &v1.ClusterHealthRequest{}); return e },
		"LookupLeader":  func(c context.Context) error { _, e := s.LookupLeader(c, &v1.LookupLeaderRequest{}); return e },
		"NATSStatus":    func(c context.Context) error { _, e := s.NATSStatus(c, &v1.NATSStatusRequest{}); return e },
		"RecoveryCoordinate": func(c context.Context) error {
			_, e := s.RecoveryCoordinate(c, &v1.RecoveryCoordinateRequest{})
			return e
		},
		"NodeHeartbeat":  func(c context.Context) error { _, e := s.NodeHeartbeat(c, &v1.NodeHeartbeatRequest{}); return e },
		"PropagateState": func(c context.Context) error { _, e := s.PropagateState(c, &v1.PropagateStateRequest{}); return e },
	} {
		if err := call(context.Background()); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s without mTLS = %v, want Unauthenticated", name, err)
		}
	}
}

// --- delegation / translation -----------------------------------------

func TestCoordination_ClusterHealth(t *testing.T) {
	s := fullCoordServer()
	resp, err := s.ClusterHealth(mtlsCtx(), &v1.ClusterHealthRequest{})
	if err != nil {
		t.Fatalf("ClusterHealth: %v", err)
	}
	if resp.NodeId != "node-self" || resp.MemberStatus != "healthy" || resp.Quorum != "quorum" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.MemberCount != 2 || resp.LeaderId != "leader-1" || !resp.NatsHealthy || !resp.StorageHealthy {
		t.Fatalf("resp = %+v", resp)
	}

	s.Health = nil
	if _, err := s.ClusterHealth(mtlsCtx(), &v1.ClusterHealthRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("nil Health = %v, want Unavailable", err)
	}
}

func TestCoordination_LookupLeader(t *testing.T) {
	s := fullCoordServer()
	resp, err := s.LookupLeader(mtlsCtx(), &v1.LookupLeaderRequest{})
	if err != nil || resp.LeaderId != "leader-1" || !resp.IsSelf {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}

	s.Leader = fakeCoordLeader{err: cluster.ErrNoLeader}
	resp, err = s.LookupLeader(mtlsCtx(), &v1.LookupLeaderRequest{})
	if err != nil || resp.LeaderId != "" {
		t.Fatalf("ErrNoLeader should yield empty leader, no error: resp=%+v err=%v", resp, err)
	}

	s.Leader = fakeCoordLeader{err: errors.New("boom")}
	if _, err := s.LookupLeader(mtlsCtx(), &v1.LookupLeaderRequest{}); status.Code(err) != codes.Internal {
		t.Fatalf("transient leader error = %v, want Internal", err)
	}

	s.Leader = nil
	if _, err := s.LookupLeader(mtlsCtx(), &v1.LookupLeaderRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("nil Leader = %v, want Unavailable", err)
	}
}

func TestCoordination_NATSStatus(t *testing.T) {
	s := fullCoordServer()
	resp, err := s.NATSStatus(mtlsCtx(), &v1.NATSStatusRequest{})
	if err != nil || !resp.Connected || resp.Detail != "ok" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	// nil NATS ⇒ "unknown", NOT Unavailable (down is a valid answer).
	s.NATS = nil
	resp, err = s.NATSStatus(mtlsCtx(), &v1.NATSStatusRequest{})
	if err != nil || resp.Connected || resp.Detail != "unknown" {
		t.Fatalf("nil NATS resp=%+v err=%v", resp, err)
	}
}

func TestCoordination_RecoveryCoordinateSnapshot(t *testing.T) {
	s := fullCoordServer()
	resp, err := s.RecoveryCoordinate(mtlsCtx(), &v1.RecoveryCoordinateRequest{
		FromNodeId: "peer-x", Phase: v1.RecoveryPhase_RECOVERY_PHASE_SYNCING,
	})
	if err != nil || !resp.Acknowledged {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if resp.LeaderId != "leader-1" || len(resp.Members) != 2 || len(resp.ShardAssignments) != 1 {
		t.Fatalf("snapshot wrong: %+v", resp)
	}
	if resp.Members[0].Id != "m1" || resp.Members[0].Status != "healthy" {
		t.Fatalf("member translation: %+v", resp.Members[0])
	}
	if resp.ShardAssignments[0].AgentId != "ag1" || resp.ShardAssignments[0].Version != 7 {
		t.Fatalf("shard translation: %+v", resp.ShardAssignments[0])
	}

	// Best-effort: missing providers still ack with partial data.
	s.Members, s.Shards = nil, nil
	resp, err = s.RecoveryCoordinate(mtlsCtx(), &v1.RecoveryCoordinateRequest{})
	if err != nil || !resp.Acknowledged || len(resp.Members) != 0 {
		t.Fatalf("best-effort recovery failed: %+v err=%v", resp, err)
	}
}

func TestCoordination_NodeHeartbeat(t *testing.T) {
	s := fullCoordServer()
	resp, err := s.NodeHeartbeat(mtlsCtx(), &v1.NodeHeartbeatRequest{FromNodeId: "peer", Epoch: 5})
	if err != nil || resp.MemberId != "node-self" || resp.Status != "healthy" || resp.ServerTime == nil {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	s.Health = nil
	resp, _ = s.NodeHeartbeat(mtlsCtx(), &v1.NodeHeartbeatRequest{})
	if resp.Status != "" {
		t.Fatalf("nil Health status = %q, want empty", resp.Status)
	}
}

func TestCoordination_PropagateState(t *testing.T) {
	s := fullCoordServer()
	// No hook ⇒ accept (don't stall the sender's NATS-down fan-out).
	resp, err := s.PropagateState(mtlsCtx(), &v1.PropagateStateRequest{Kind: "members", Snapshot: []byte("x")})
	if err != nil || !resp.Accepted {
		t.Fatalf("no-hook resp=%+v err=%v", resp, err)
	}

	var gotKind string
	var gotPayload []byte
	s.Propagate = func(_ context.Context, kind string, payload []byte) error {
		gotKind, gotPayload = kind, payload
		return nil
	}
	if _, err := s.PropagateState(mtlsCtx(), &v1.PropagateStateRequest{Kind: "shards", Snapshot: []byte("blob")}); err != nil {
		t.Fatalf("PropagateState: %v", err)
	}
	if gotKind != "shards" || string(gotPayload) != "blob" {
		t.Fatalf("hook got kind=%q payload=%q", gotKind, gotPayload)
	}

	s.Propagate = func(context.Context, string, []byte) error { return errors.New("apply failed") }
	if _, err := s.PropagateState(mtlsCtx(), &v1.PropagateStateRequest{}); status.Code(err) != codes.Internal {
		t.Fatalf("hook error = %v, want Internal", err)
	}
}

// --- end-to-end over bufconn: insecure dial must be rejected -----------

func TestCoordination_BufconnInsecureRejected(t *testing.T) {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	v1.RegisterCoordinationServiceServer(srv, fullCoordServer())
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = v1.NewCoordinationServiceClient(conn).ClusterHealth(ctx, &v1.ClusterHealthRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("insecure e2e call = %v, want Unauthenticated", err)
	}
}
