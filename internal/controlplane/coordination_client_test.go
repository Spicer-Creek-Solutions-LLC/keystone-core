package controlplane

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type fakeCoordSrv struct {
	v1.UnimplementedCoordinationServiceServer
	mu          sync.Mutex
	hbCalls     int
	chCalls     int
	lastFrom    string
	hbFailUntil int   // NodeHeartbeat: Unavailable for the first N calls
	hbErr       error // NodeHeartbeat: persistent error if set
	chFailUntil int   // ClusterHealth: Unavailable for the first N calls
	chErr       error // ClusterHealth: persistent error if set
}

func (s *fakeCoordSrv) NodeHeartbeat(_ context.Context, req *v1.NodeHeartbeatRequest) (*v1.NodeHeartbeatResponse, error) {
	s.mu.Lock()
	s.hbCalls++
	s.lastFrom = req.GetFromNodeId()
	n, until, perr := s.hbCalls, s.hbFailUntil, s.hbErr
	s.mu.Unlock()
	if perr != nil {
		return nil, perr
	}
	if n <= until {
		return nil, status.Error(codes.Unavailable, "warming up")
	}
	return &v1.NodeHeartbeatResponse{MemberId: "srv", Status: "healthy", ServerTime: timestamppb.Now()}, nil
}

func (s *fakeCoordSrv) ClusterHealth(_ context.Context, req *v1.ClusterHealthRequest) (*v1.ClusterHealthResponse, error) {
	s.mu.Lock()
	s.chCalls++
	s.lastFrom = req.GetFromNodeId()
	n, until, perr := s.chCalls, s.chFailUntil, s.chErr
	s.mu.Unlock()
	if perr != nil {
		return nil, perr
	}
	if n <= until {
		return nil, status.Error(codes.Unavailable, "warming up")
	}
	return &v1.ClusterHealthResponse{NodeId: "srv", MemberStatus: "healthy", LeaderId: "L"}, nil
}

func (s *fakeCoordSrv) LookupLeader(context.Context, *v1.LookupLeaderRequest) (*v1.LookupLeaderResponse, error) {
	return &v1.LookupLeaderResponse{LeaderId: "L", IsSelf: false}, nil
}
func (s *fakeCoordSrv) NATSStatus(context.Context, *v1.NATSStatusRequest) (*v1.NATSStatusResponse, error) {
	return &v1.NATSStatusResponse{Connected: true, Detail: "ok"}, nil
}
func (s *fakeCoordSrv) RecoveryCoordinate(context.Context, *v1.RecoveryCoordinateRequest) (*v1.RecoveryCoordinateResponse, error) {
	return &v1.RecoveryCoordinateResponse{Acknowledged: true, LeaderId: "L"}, nil
}
func (s *fakeCoordSrv) PropagateState(context.Context, *v1.PropagateStateRequest) (*v1.PropagateStateResponse, error) {
	return &v1.PropagateStateResponse{Accepted: true}, nil
}

func (s *fakeCoordSrv) setHBErr(e error) {
	s.mu.Lock()
	s.hbErr = e
	s.mu.Unlock()
}

func newCoordRig(t *testing.T, mutate func(*CoordinationClientConfig)) (*CoordinationClient, *fakeCoordSrv) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	fs := &fakeCoordSrv{}
	v1.RegisterCoordinationServiceServer(srv, fs)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	cfg := CoordinationClientConfig{
		SelfID: "self",
		DialOptions: []grpc.DialOption{
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		},
		HeartbeatInterval: 25 * time.Millisecond,
		HeartbeatTimeout:  500 * time.Millisecond,
		FailureThreshold:  3,
		RetryMax:          4,
		RetryBaseDelay:    time.Millisecond,
		RetryMaxDelay:     5 * time.Millisecond,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	cc, err := NewCoordinationClient(cfg)
	if err != nil {
		t.Fatalf("NewCoordinationClient: %v", err)
	}
	if err := cc.AddPeer("p1", "passthrough:///bufnet"); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	return cc, fs
}

func ccWaitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}

func TestNewCoordinationClient_InvalidConfig(t *testing.T) {
	opt := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if _, err := NewCoordinationClient(CoordinationClientConfig{DialOptions: opt}); !errors.Is(err, ErrCoordConfig) {
		t.Fatalf("no SelfID: %v", err)
	}
	if _, err := NewCoordinationClient(CoordinationClientConfig{SelfID: "s"}); !errors.Is(err, ErrCoordConfig) {
		t.Fatalf("no DialOptions: %v", err)
	}
}

func TestCoordinationClient_PoolAndDelegation(t *testing.T) {
	cc, fs := newCoordRig(t, nil)
	ctx := context.Background()

	r, err := cc.ClusterHealth(ctx, "p1")
	if err != nil || r.NodeId != "srv" || r.LeaderId != "L" {
		t.Fatalf("ClusterHealth = %+v, %v", r, err)
	}
	if fs.lastFrom != "self" {
		t.Fatalf("FromNodeId not propagated: %q", fs.lastFrom)
	}
	if lr, err := cc.LookupLeader(ctx, "p1"); err != nil || lr.LeaderId != "L" {
		t.Fatalf("LookupLeader = %+v, %v", lr, err)
	}
	if ns, err := cc.NATSStatus(ctx, "p1"); err != nil || !ns.Connected {
		t.Fatalf("NATSStatus = %+v, %v", ns, err)
	}
	if rc, err := cc.RecoveryCoordinate(ctx, "p1", v1.RecoveryPhase_RECOVERY_PHASE_SYNCING); err != nil || !rc.Acknowledged {
		t.Fatalf("RecoveryCoordinate = %+v, %v", rc, err)
	}
	if ps, err := cc.PropagateState(ctx, "p1", "members", []byte("x"), 1); err != nil || !ps.Accepted {
		t.Fatalf("PropagateState = %+v, %v", ps, err)
	}

	// Unknown peer.
	if _, err := cc.ClusterHealth(ctx, "nope"); status.Code(err) != codes.NotFound {
		t.Fatalf("unknown peer = %v, want NotFound", err)
	}
	// Remove → NotFound.
	cc.RemovePeer("p1")
	if _, err := cc.ClusterHealth(ctx, "p1"); status.Code(err) != codes.NotFound {
		t.Fatalf("after RemovePeer = %v, want NotFound", err)
	}
	// SetPeers reconcile re-adds.
	if err := cc.SetPeers(map[string]string{"p1": "passthrough:///bufnet"}); err != nil {
		t.Fatalf("SetPeers: %v", err)
	}
	if _, err := cc.ClusterHealth(ctx, "p1"); err != nil {
		t.Fatalf("after SetPeers: %v", err)
	}
}

func TestCoordinationClient_RetryTransientThenSuccess(t *testing.T) {
	cc, fs := newCoordRig(t, nil)
	fs.mu.Lock()
	fs.chFailUntil = 2 // two Unavailable then success
	fs.mu.Unlock()

	r, err := cc.ClusterHealth(context.Background(), "p1")
	if err != nil || r.NodeId != "srv" {
		t.Fatalf("retry should succeed: %+v %v", r, err)
	}
	fs.mu.Lock()
	calls := fs.chCalls
	fs.mu.Unlock()
	if calls != 3 {
		t.Fatalf("chCalls = %d, want 3 (2 fail + 1 ok)", calls)
	}
}

func TestCoordinationClient_NoRetryOnPermanent(t *testing.T) {
	cc, fs := newCoordRig(t, nil)
	fs.mu.Lock()
	fs.chErr = status.Error(codes.Unauthenticated, "mTLS required")
	fs.mu.Unlock()

	_, err := cc.ClusterHealth(context.Background(), "p1")
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("err = %v, want Unauthenticated", err)
	}
	fs.mu.Lock()
	calls := fs.chCalls
	fs.mu.Unlock()
	if calls != 1 {
		t.Fatalf("permanent error retried (%d calls), want 1", calls)
	}
}

func TestCoordinationClient_RetryExhausted(t *testing.T) {
	cc, fs := newCoordRig(t, func(c *CoordinationClientConfig) { c.RetryMax = 2 })
	fs.mu.Lock()
	fs.chFailUntil = 100 // always Unavailable
	fs.mu.Unlock()

	_, err := cc.ClusterHealth(context.Background(), "p1")
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("err = %v, want Unavailable", err)
	}
	fs.mu.Lock()
	calls := fs.chCalls
	fs.mu.Unlock()
	if calls != 2 {
		t.Fatalf("chCalls = %d, want 2 (RetryMax)", calls)
	}
}

func TestCoordinationClient_RetryCtxCancel(t *testing.T) {
	cc, fs := newCoordRig(t, func(c *CoordinationClientConfig) {
		c.RetryMax = 5
		c.RetryBaseDelay = 200 * time.Millisecond
		c.RetryMaxDelay = time.Second
	})
	fs.mu.Lock()
	fs.chFailUntil = 100
	fs.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	_, err := cc.ClusterHealth(ctx, "p1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

type peerRec struct {
	mu sync.Mutex
	ev []bool
}

func (r *peerRec) OnPeerChange(_ string, reachable bool) {
	r.mu.Lock()
	r.ev = append(r.ev, reachable)
	r.mu.Unlock()
}
func (r *peerRec) sawFalse() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.ev {
		if !v {
			return true
		}
	}
	return false
}

func TestCoordinationClient_HeartbeatLivenessTracking(t *testing.T) {
	cc, fs := newCoordRig(t, nil)
	rec := &peerRec{}
	cc.AddObserver(nil) // no-op
	cc.AddObserver(rec)
	r2 := &peerRec{}
	cc.AddObserver(r2)
	cc.RemoveObserver(r2)         // present — removed
	cc.RemoveObserver(&peerRec{}) // absent — no-op
	if err := cc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = cc.Stop(context.Background()) })

	// Healthy heartbeats → reachable, lastSeen advances.
	ccWaitFor(t, func() bool {
		ok, ls, found := cc.PeerLiveness("p1")
		return found && ok && !ls.IsZero()
	}, "peer reachable via heartbeat")
	if got := cc.ReachablePeers(); len(got) != 1 || got[0] != "p1" {
		t.Fatalf("ReachablePeers = %v", got)
	}

	// Heartbeats start failing → after FailureThreshold, unreachable.
	fs.setHBErr(status.Error(codes.Unavailable, "down"))
	ccWaitFor(t, func() bool {
		ok, _, _ := cc.PeerLiveness("p1")
		return !ok
	}, "peer marked unreachable")
	if !rec.sawFalse() {
		t.Fatal("observer never saw reachable=false")
	}
	if len(cc.ReachablePeers()) != 0 {
		t.Fatalf("ReachablePeers should be empty when down")
	}

	// Recovers.
	fs.setHBErr(nil)
	ccWaitFor(t, func() bool {
		ok, _, _ := cc.PeerLiveness("p1")
		return ok
	}, "peer recovers")
}

func TestCoordinationClient_LifecycleErrors(t *testing.T) {
	cc, _ := newCoordRig(t, nil)
	ctx := context.Background()
	if err := cc.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := cc.Start(ctx); err == nil {
		t.Error("double Start should error")
	}
	if err := cc.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := cc.Stop(ctx); err != nil {
		t.Errorf("idempotent Stop = %v", err)
	}
	if err := cc.Start(ctx); err == nil {
		t.Error("Start after Stop should error")
	}
	// Pool emptied on Stop.
	if _, err := cc.ClusterHealth(ctx, "p1"); status.Code(err) != codes.NotFound {
		t.Fatalf("peer should be gone after Stop: %v", err)
	}
}
