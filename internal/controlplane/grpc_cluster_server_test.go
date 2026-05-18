package controlplane

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"go.keystone-core.io/keystone-core/internal/cluster"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

type fakeClLeader struct {
	id          string
	err         error
	self        bool
	transferErr error
	mu          sync.Mutex
	transferred bool
}

func (f *fakeClLeader) LeaderID(context.Context) (string, error) { return f.id, f.err }
func (f *fakeClLeader) IsLeader() bool                           { return f.self }
func (f *fakeClLeader) TransferLeadership(context.Context) error {
	f.mu.Lock()
	f.transferred = true
	f.mu.Unlock()
	return f.transferErr
}

type fakeClMembers struct {
	members []cluster.Member
	getErr  error
	watchCh chan cluster.MemberEvent
}

func (f *fakeClMembers) LoadMembers(context.Context) ([]cluster.Member, error) {
	return f.members, nil
}
func (f *fakeClMembers) GetMember(_ context.Context, id string) (cluster.Member, error) {
	if f.getErr != nil {
		return cluster.Member{}, f.getErr
	}
	for _, m := range f.members {
		if m.ID == id {
			return m, nil
		}
	}
	return cluster.Member{}, cluster.ErrMemberNotFound
}
func (f *fakeClMembers) WatchMembers(context.Context) (<-chan cluster.MemberEvent, error) {
	if f.watchCh == nil {
		f.watchCh = make(chan cluster.MemberEvent, 8)
	}
	return f.watchCh, nil
}

type fakeClRebalancer struct {
	moves []cluster.ShardMove
	err   error
}

func (f *fakeClRebalancer) Rebalance(context.Context) ([]cluster.ShardMove, error) {
	return f.moves, f.err
}

type fakeClShardStore struct {
	list []cluster.ShardAssignment
}

func (f *fakeClShardStore) List(context.Context) ([]cluster.ShardAssignment, error) {
	return f.list, nil
}
func (f *fakeClShardStore) Assign(_ context.Context, a, m string) (cluster.ShardAssignment, error) {
	return cluster.ShardAssignment{AgentID: a, MemberID: m, Version: 1}, nil
}
func (f *fakeClShardStore) AssignIf(_ context.Context, a, m string, _ int64) (cluster.ShardAssignment, error) {
	return cluster.ShardAssignment{AgentID: a, MemberID: m, Version: 1}, nil
}

type fakeClLeaderWatch struct {
	mu  sync.Mutex
	obs []cluster.LeadershipObserver
}

func (f *fakeClLeaderWatch) AddObserver(o cluster.LeadershipObserver) {
	f.mu.Lock()
	f.obs = append(f.obs, o)
	f.mu.Unlock()
}
func (f *fakeClLeaderWatch) RemoveObserver(o cluster.LeadershipObserver) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, x := range f.obs {
		if x == o {
			f.obs = append(f.obs[:i], f.obs[i+1:]...)
			return
		}
	}
}
func (f *fakeClLeaderWatch) fire(ev cluster.LeadershipEvent) {
	f.mu.Lock()
	obs := append([]cluster.LeadershipObserver(nil), f.obs...)
	f.mu.Unlock()
	for _, o := range obs {
		o.OnLeadershipChange(ev)
	}
}

func fullClusterServer() *ClusterGRPCServer {
	return &ClusterGRPCServer{
		Health:      fakeCoordHealth{st: cluster.MemberHealthy, q: cluster.QuorumOK},
		Leader:      &fakeClLeader{id: "m1", self: true},
		Members:     &fakeClMembers{members: []cluster.Member{{ID: "m1", Name: "n1", Addr: "a1", Status: cluster.MemberHealthy}, {ID: "m2", Status: cluster.MemberDegraded}}},
		Rebalancer:  &fakeClRebalancer{moves: []cluster.ShardMove{{AgentID: "ag1", From: "m2", To: "m1"}}},
		ShardStore:  &fakeClShardStore{list: []cluster.ShardAssignment{{AgentID: "ag1", MemberID: "m1", Version: 2}}},
		LeaderWatch: &fakeClLeaderWatch{},
		ClusterName: "c1",
	}
}

func clusterRig(t *testing.T, srv *ClusterGRPCServer) v1.ClusterServiceClient {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	v1.RegisterClusterServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return v1.NewClusterServiceClient(conn)
}

func TestClusterService_StatusMembersLeader(t *testing.T) {
	cl := clusterRig(t, fullClusterServer())
	ctx := context.Background()

	st, err := cl.GetClusterStatus(ctx, &v1.GetClusterStatusRequest{})
	if err != nil || st.ClusterId != "c1" || st.MemberCount != 2 || st.HealthyCount != 1 || !st.Quorum || st.LeaderId != "m1" {
		t.Fatalf("status = %+v, %v", st, err)
	}

	lm, err := cl.ListMembers(ctx, &v1.ListMembersRequest{})
	if err != nil || len(lm.Members) != 2 {
		t.Fatalf("ListMembers = %+v, %v", lm, err)
	}
	// Status filter.
	lm, _ = cl.ListMembers(ctx, &v1.ListMembersRequest{Status: v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY})
	if len(lm.Members) != 1 || lm.Members[0].Id != "m1" || lm.Members[0].Role != v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_LEADER {
		t.Fatalf("filtered ListMembers = %+v", lm.Members)
	}

	gm, err := cl.GetMember(ctx, &v1.GetMemberRequest{MemberId: "m2"})
	if err != nil || gm.Member.Id != "m2" || gm.Member.Status != v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_DEGRADED {
		t.Fatalf("GetMember = %+v, %v", gm, err)
	}
	if _, err := cl.GetMember(ctx, &v1.GetMemberRequest{MemberId: "nope"}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetMember(nope) = %v, want NotFound", err)
	}

	gl, err := cl.GetLeader(ctx, &v1.GetLeaderRequest{})
	if err != nil || gl.Leader.Id != "m1" {
		t.Fatalf("GetLeader = %+v, %v", gl, err)
	}
}

func TestClusterService_AddRemoveMember(t *testing.T) {
	srv := fullClusterServer()
	cl := clusterRig(t, srv)
	ctx := context.Background()

	if _, err := cl.AddMember(ctx, &v1.AddMemberRequest{Name: "x"}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("AddMember = %v, want Unimplemented", err)
	}
	// No evictor wired → Unimplemented.
	if _, err := cl.RemoveMember(ctx, &v1.RemoveMemberRequest{MemberId: "m2"}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("RemoveMember (no evictor) = %v, want Unimplemented", err)
	}
	// Wired evictor.
	var evicted string
	srv.Evictor = func(_ context.Context, id string) error { evicted = id; return nil }
	if _, err := cl.RemoveMember(ctx, &v1.RemoveMemberRequest{MemberId: "m2"}); err != nil {
		t.Fatalf("RemoveMember = %v", err)
	}
	if evicted != "m2" {
		t.Fatalf("evictor got %q", evicted)
	}
}

func TestClusterService_TransferLeaderAndRebalance(t *testing.T) {
	srv := fullClusterServer()
	cl := clusterRig(t, srv)
	ctx := context.Background()

	if _, err := cl.TransferLeader(ctx, &v1.TransferLeaderRequest{}); err != nil {
		t.Fatalf("TransferLeader (leader) = %v", err)
	}
	// Not leader → FailedPrecondition.
	srv.Leader = &fakeClLeader{id: "m9", self: false}
	if _, err := cl.TransferLeader(ctx, &v1.TransferLeaderRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("TransferLeader (non-leader) = %v, want FailedPrecondition", err)
	}

	rb, err := cl.Rebalance(ctx, &v1.RebalanceRequest{})
	if err != nil || rb.ReassignedAgents != 1 {
		t.Fatalf("Rebalance = %+v, %v", rb, err)
	}
	if _, err := cl.Rebalance(ctx, &v1.RebalanceRequest{DryRun: true}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("Rebalance dry-run = %v, want Unimplemented", err)
	}
}

func TestClusterService_BackupRestoreRoundTrip(t *testing.T) {
	srv := fullClusterServer()
	cl := clusterRig(t, srv)
	ctx := context.Background()

	bk, err := cl.CreateBackup(ctx, &v1.CreateBackupRequest{})
	if err != nil || len(bk.Snapshot) == 0 || bk.SizeBytes != int64(len(bk.Snapshot)) {
		t.Fatalf("CreateBackup = %+v, %v", bk, err)
	}
	// Snapshot decodes as a valid envelope.
	if _, derr := cluster.UnmarshalSnapshot(bk.Snapshot); derr != nil {
		t.Fatalf("backup snapshot invalid: %v", derr)
	}

	// Non-leader cannot back up (leader-initiated for ordering).
	srv.Leader = &fakeClLeader{self: false}
	if _, err := cl.CreateBackup(ctx, &v1.CreateBackupRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("CreateBackup (non-leader) = %v, want FailedPrecondition", err)
	}

	// Restore: invalid blob → InvalidArgument; dry-run validates;
	// real restore applies.
	if _, err := cl.RestoreBackup(ctx, &v1.RestoreBackupRequest{Snapshot: []byte("garbage")}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("RestoreBackup(garbage) = %v, want InvalidArgument", err)
	}
	dr, err := cl.RestoreBackup(ctx, &v1.RestoreBackupRequest{Snapshot: bk.Snapshot, DryRun: true})
	if err != nil || !dr.Success {
		t.Fatalf("RestoreBackup dry-run = %+v, %v", dr, err)
	}
	rr, err := cl.RestoreBackup(ctx, &v1.RestoreBackupRequest{Snapshot: bk.Snapshot, Force: true})
	if err != nil || !rr.Success {
		t.Fatalf("RestoreBackup = %+v, %v", rr, err)
	}
}

func TestClusterService_NilProvidersUnavailable(t *testing.T) {
	cl := clusterRig(t, &ClusterGRPCServer{}) // nothing wired
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"status":    func() error { _, e := cl.GetClusterStatus(ctx, &v1.GetClusterStatusRequest{}); return e },
		"members":   func() error { _, e := cl.ListMembers(ctx, &v1.ListMembersRequest{}); return e },
		"leader":    func() error { _, e := cl.GetLeader(ctx, &v1.GetLeaderRequest{}); return e },
		"transfer":  func() error { _, e := cl.TransferLeader(ctx, &v1.TransferLeaderRequest{}); return e },
		"rebalance": func() error { _, e := cl.Rebalance(ctx, &v1.RebalanceRequest{}); return e },
		"backup":    func() error { _, e := cl.CreateBackup(ctx, &v1.CreateBackupRequest{}); return e },
	} {
		if status.Code(call()) != codes.Unavailable {
			t.Fatalf("%s with nil providers = %v, want Unavailable", name, call())
		}
	}
}

func TestClusterService_WatchMembershipStream(t *testing.T) {
	srv := fullClusterServer()
	mem := srv.Members.(*fakeClMembers)
	mem.watchCh = make(chan cluster.MemberEvent, 8)
	cl := clusterRig(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := cl.WatchMembership(ctx, &v1.WatchMembershipRequest{})
	if err != nil {
		t.Fatalf("WatchMembership: %v", err)
	}
	mem.watchCh <- cluster.MemberEvent{Type: cluster.MemberJoined, Member: cluster.Member{ID: "m3", Status: cluster.MemberHealthy}}
	got, err := stream.Recv()
	if err != nil || got.Member.Id != "m3" || got.Kind != v1.ClusterMembershipChangeKind_CLUSTER_MEMBERSHIP_CHANGE_KIND_JOINED {
		t.Fatalf("stream recv = %+v, %v", got, err)
	}
}

func TestClusterService_WatchLeadershipStream(t *testing.T) {
	srv := fullClusterServer()
	lw := srv.LeaderWatch.(*fakeClLeaderWatch)
	cl := clusterRig(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := cl.WatchLeadership(ctx, &v1.WatchLeadershipRequest{})
	if err != nil {
		t.Fatalf("WatchLeadership: %v", err)
	}
	// Give the server a moment to register its observer.
	deadline := time.Now().Add(3 * time.Second)
	for {
		lw.mu.Lock()
		n := len(lw.obs)
		lw.mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	lw.fire(cluster.LeadershipEvent{State: cluster.LeaderElected, LeaderID: "m7", Self: true})
	got, err := stream.Recv()
	if err != nil || got.Leader.Id != "m7" {
		t.Fatalf("leadership stream recv = %+v, %v", got, err)
	}
}
