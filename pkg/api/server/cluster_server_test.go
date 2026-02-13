package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/shawnbutts/keystone-core/internal/cluster"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// --- Mock types ---

type mockClusterMembership struct {
	info       *cluster.Info
	members    []*cluster.Member
	member     *cluster.Member
	memberErr  error
	addResult  *cluster.Member
	addErr     error
	removeErr  error
	leader     *cluster.Member
	observers  []cluster.MembershipObserver
	readyCh    chan struct{}
}

func (m *mockClusterMembership) GetClusterInfo() *cluster.Info { return m.info }
func (m *mockClusterMembership) ListMembers() []*cluster.Member { return m.members }
func (m *mockClusterMembership) GetMember(_ string) (*cluster.Member, error) {
	if m.memberErr != nil {
		return nil, m.memberErr
	}
	return m.member, nil
}
func (m *mockClusterMembership) AddMember(_ context.Context, _ *cluster.AddMemberRequest) (*cluster.Member, error) {
	if m.addErr != nil {
		return nil, m.addErr
	}
	return m.addResult, nil
}
func (m *mockClusterMembership) RemoveMember(_ context.Context, _ string, _ bool) error {
	return m.removeErr
}
func (m *mockClusterMembership) GetLeader() *cluster.Member { return m.leader }
func (m *mockClusterMembership) HasQuorum() bool            { return m.info != nil && m.info.HasQuorum }
func (m *mockClusterMembership) MemberCount() int {
	if m.info != nil {
		return m.info.MemberCount
	}
	return 0
}
func (m *mockClusterMembership) HealthyMemberCount() int {
	if m.info != nil {
		return m.info.HealthyCount
	}
	return 0
}
func (m *mockClusterMembership) AddObserver(observer cluster.MembershipObserver) {
	m.observers = append(m.observers, observer)
	if m.readyCh != nil {
		close(m.readyCh)
	}
}
func (m *mockClusterMembership) RemoveObserver(_ cluster.MembershipObserver) {}

type mockClusterLeader struct {
	isLeader    bool
	leaderID    string
	transferErr error
	observers   []cluster.LeadershipObserver
	readyCh     chan struct{}
}

func (m *mockClusterLeader) IsLeader() bool     { return m.isLeader }
func (m *mockClusterLeader) GetLeaderID() string { return m.leaderID }
func (m *mockClusterLeader) TransferLeadership(_ context.Context, _ string) error {
	return m.transferErr
}
func (m *mockClusterLeader) AddObserver(observer cluster.LeadershipObserver) {
	m.observers = append(m.observers, observer)
	if m.readyCh != nil {
		close(m.readyCh)
	}
}
func (m *mockClusterLeader) RemoveObserver(_ cluster.LeadershipObserver) {}

type mockClusterShards struct {
	rebalanceErr error
}

func (m *mockClusterShards) TriggerRebalance(_ context.Context, _ string) error {
	return m.rebalanceErr
}

type mockMembershipStream struct {
	grpc.ServerStreamingServer[pb.MembershipEvent]
	sent   []*pb.MembershipEvent
	ctx    context.Context
	cancel context.CancelFunc
}

func newMockMembershipStream() *mockMembershipStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockMembershipStream{ctx: ctx, cancel: cancel}
}

func (m *mockMembershipStream) Send(evt *pb.MembershipEvent) error {
	m.sent = append(m.sent, evt)
	return nil
}

func (m *mockMembershipStream) Context() context.Context { return m.ctx }

type mockLeadershipStream struct {
	grpc.ServerStreamingServer[pb.LeadershipEvent]
	sent   []*pb.LeadershipEvent
	ctx    context.Context
	cancel context.CancelFunc
}

func newMockLeadershipStream() *mockLeadershipStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockLeadershipStream{ctx: ctx, cancel: cancel}
}

func (m *mockLeadershipStream) Send(evt *pb.LeadershipEvent) error {
	m.sent = append(m.sent, evt)
	return nil
}

func (m *mockLeadershipStream) Context() context.Context { return m.ctx }

// --- Helper ---

func testClusterInfo() *cluster.Info {
	return &cluster.Info{
		Name:         "test-cluster",
		Status:       cluster.StatusHealthy,
		LeaderID:     "member-1",
		MemberCount:  3,
		HealthyCount: 3,
		QuorumSize:   2,
		HasQuorum:    true,
		UpdatedAt:    time.Now(),
		Members: []*cluster.Member{
			{ID: "member-1", Name: "node1", Status: cluster.MemberStatusHealthy, IsLeader: true},
			{ID: "member-2", Name: "node2", Status: cluster.MemberStatusHealthy},
			{ID: "member-3", Name: "node3", Status: cluster.MemberStatusDegraded},
		},
	}
}

// --- GetClusterStatus tests ---

func TestClusterServer_GetClusterStatus_NilMembership(t *testing.T) {
	srv := NewClusterServer(nil, nil, nil)
	_, err := srv.GetClusterStatus(context.Background(), &pb.GetClusterStatusRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestClusterServer_GetClusterStatus_Success(t *testing.T) {
	mem := &mockClusterMembership{info: testClusterInfo()}
	srv := NewClusterServer(mem, nil, nil)

	resp, err := srv.GetClusterStatus(context.Background(), &pb.GetClusterStatusRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Healthy {
		t.Error("expected healthy=true")
	}
	if resp.MemberCount != 3 {
		t.Errorf("got member_count %d, want 3", resp.MemberCount)
	}
	if resp.QuorumSize != 2 {
		t.Errorf("got quorum_size %d, want 2", resp.QuorumSize)
	}
	if !resp.HasQuorum {
		t.Error("expected has_quorum=true")
	}
	if resp.LeaderId != "member-1" {
		t.Errorf("got leader_id %q, want %q", resp.LeaderId, "member-1")
	}
	if len(resp.Members) != 3 {
		t.Errorf("got %d members, want 3", len(resp.Members))
	}
}

// --- ListMembers tests ---

func TestClusterServer_ListMembers_NilMembership(t *testing.T) {
	srv := NewClusterServer(nil, nil, nil)
	_, err := srv.ListMembers(context.Background(), &pb.ListMembersRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestClusterServer_ListMembers_All(t *testing.T) {
	mem := &mockClusterMembership{
		members: []*cluster.Member{
			{ID: "m1", Status: cluster.MemberStatusHealthy},
			{ID: "m2", Status: cluster.MemberStatusDegraded},
			{ID: "m3", Status: cluster.MemberStatusHealthy},
		},
	}
	srv := NewClusterServer(mem, nil, nil)

	resp, err := srv.ListMembers(context.Background(), &pb.ListMembersRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Members) != 3 {
		t.Errorf("got %d members, want 3", len(resp.Members))
	}
	if resp.TotalCount != 3 {
		t.Errorf("got total_count %d, want 3", resp.TotalCount)
	}
}

func TestClusterServer_ListMembers_FilterByStatus(t *testing.T) {
	mem := &mockClusterMembership{
		members: []*cluster.Member{
			{ID: "m1", Status: cluster.MemberStatusHealthy},
			{ID: "m2", Status: cluster.MemberStatusDegraded},
			{ID: "m3", Status: cluster.MemberStatusHealthy},
		},
	}
	srv := NewClusterServer(mem, nil, nil)

	resp, err := srv.ListMembers(context.Background(), &pb.ListMembersRequest{
		Status: pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Members) != 2 {
		t.Fatalf("got %d members, want 2 (healthy only)", len(resp.Members))
	}
}

func TestClusterServer_ListMembers_Pagination(t *testing.T) {
	mem := &mockClusterMembership{
		members: []*cluster.Member{
			{ID: "m1"}, {ID: "m2"}, {ID: "m3"}, {ID: "m4"}, {ID: "m5"},
		},
	}
	srv := NewClusterServer(mem, nil, nil)

	resp, err := srv.ListMembers(context.Background(), &pb.ListMembersRequest{PageSize: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Members) != 2 {
		t.Fatalf("got %d members, want 2", len(resp.Members))
	}
	if resp.NextPageToken == "" {
		t.Error("expected next_page_token")
	}
	if resp.TotalCount != 5 {
		t.Errorf("got total_count %d, want 5", resp.TotalCount)
	}
}

// --- GetMember tests ---

func TestClusterServer_GetMember_NilMembership(t *testing.T) {
	srv := NewClusterServer(nil, nil, nil)
	_, err := srv.GetMember(context.Background(), &pb.GetMemberRequest{MemberId: "m1"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestClusterServer_GetMember_EmptyID(t *testing.T) {
	mem := &mockClusterMembership{}
	srv := NewClusterServer(mem, nil, nil)
	_, err := srv.GetMember(context.Background(), &pb.GetMemberRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestClusterServer_GetMember_NotFound(t *testing.T) {
	mem := &mockClusterMembership{memberErr: errors.New("not found")}
	srv := NewClusterServer(mem, nil, nil)
	_, err := srv.GetMember(context.Background(), &pb.GetMemberRequest{MemberId: "missing"})
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("got code %v, want NotFound", st.Code())
	}
}

func TestClusterServer_GetMember_Success(t *testing.T) {
	mem := &mockClusterMembership{
		member: &cluster.Member{
			ID: "m1", Name: "node1", Address: "10.0.0.1:8500",
			Status: cluster.MemberStatusHealthy, IsLeader: true, Version: "0.1.0",
		},
	}
	srv := NewClusterServer(mem, nil, nil)

	resp, err := srv.GetMember(context.Background(), &pb.GetMemberRequest{MemberId: "m1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Member.Id != "m1" {
		t.Errorf("got id %q, want %q", resp.Member.Id, "m1")
	}
	if resp.Member.Status != pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY {
		t.Errorf("got status %v, want HEALTHY", resp.Member.Status)
	}
	if !resp.Member.IsLeader {
		t.Error("expected is_leader=true")
	}
}

// --- AddMember tests ---

func TestClusterServer_AddMember_NilMembership(t *testing.T) {
	srv := NewClusterServer(nil, nil, nil)
	_, err := srv.AddMember(context.Background(), &pb.AddMemberRequest{Address: "10.0.0.1:8500"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestClusterServer_AddMember_MissingAddress(t *testing.T) {
	mem := &mockClusterMembership{}
	srv := NewClusterServer(mem, nil, nil)
	_, err := srv.AddMember(context.Background(), &pb.AddMemberRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestClusterServer_AddMember_Success(t *testing.T) {
	mem := &mockClusterMembership{
		addResult: &cluster.Member{ID: "new-member", Name: "node4", Address: "10.0.0.4:8500"},
	}
	srv := NewClusterServer(mem, nil, nil)

	resp, err := srv.AddMember(context.Background(), &pb.AddMemberRequest{
		Name:    "node4",
		Address: "10.0.0.4:8500",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Member.Id != "new-member" {
		t.Errorf("got id %q, want %q", resp.Member.Id, "new-member")
	}
}

func TestClusterServer_AddMember_Error(t *testing.T) {
	mem := &mockClusterMembership{addErr: errors.New("etcd unreachable")}
	srv := NewClusterServer(mem, nil, nil)
	_, err := srv.AddMember(context.Background(), &pb.AddMemberRequest{Address: "10.0.0.4:8500"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("got code %v, want Internal", st.Code())
	}
}

// --- RemoveMember tests ---

func TestClusterServer_RemoveMember_NilMembership(t *testing.T) {
	srv := NewClusterServer(nil, nil, nil)
	_, err := srv.RemoveMember(context.Background(), &pb.RemoveMemberRequest{MemberId: "m1"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestClusterServer_RemoveMember_EmptyID(t *testing.T) {
	mem := &mockClusterMembership{}
	srv := NewClusterServer(mem, nil, nil)
	_, err := srv.RemoveMember(context.Background(), &pb.RemoveMemberRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestClusterServer_RemoveMember_Success(t *testing.T) {
	mem := &mockClusterMembership{}
	srv := NewClusterServer(mem, nil, nil)

	resp, err := srv.RemoveMember(context.Background(), &pb.RemoveMemberRequest{MemberId: "m2", Force: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Removed {
		t.Error("expected removed=true")
	}
}

// --- GetLeader tests ---

func TestClusterServer_GetLeader_NilMembership(t *testing.T) {
	srv := NewClusterServer(nil, nil, nil)
	_, err := srv.GetLeader(context.Background(), &pb.GetClusterLeaderRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestClusterServer_GetLeader_NoLeader(t *testing.T) {
	mem := &mockClusterMembership{leader: nil}
	srv := NewClusterServer(mem, nil, nil)
	_, err := srv.GetLeader(context.Background(), &pb.GetClusterLeaderRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("got code %v, want NotFound", st.Code())
	}
}

func TestClusterServer_GetLeader_Success(t *testing.T) {
	mem := &mockClusterMembership{
		leader: &cluster.Member{ID: "m1", Name: "leader-node", IsLeader: true},
	}
	srv := NewClusterServer(mem, nil, nil)

	resp, err := srv.GetLeader(context.Background(), &pb.GetClusterLeaderRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Leader.Id != "m1" {
		t.Errorf("got leader id %q, want %q", resp.Leader.Id, "m1")
	}
}

// --- TransferLeader tests ---

func TestClusterServer_TransferLeader_NilLeader(t *testing.T) {
	srv := NewClusterServer(nil, nil, nil)
	_, err := srv.TransferLeader(context.Background(), &pb.TransferLeaderRequest{TargetId: "m2"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestClusterServer_TransferLeader_EmptyTarget(t *testing.T) {
	leader := &mockClusterLeader{isLeader: true}
	srv := NewClusterServer(nil, leader, nil)
	_, err := srv.TransferLeader(context.Background(), &pb.TransferLeaderRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestClusterServer_TransferLeader_NotLeader(t *testing.T) {
	leader := &mockClusterLeader{isLeader: false}
	srv := NewClusterServer(nil, leader, nil)
	_, err := srv.TransferLeader(context.Background(), &pb.TransferLeaderRequest{TargetId: "m2"})
	st, _ := status.FromError(err)
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("got code %v, want FailedPrecondition", st.Code())
	}
}

func TestClusterServer_TransferLeader_Success(t *testing.T) {
	leader := &mockClusterLeader{isLeader: true}
	srv := NewClusterServer(nil, leader, nil)

	resp, err := srv.TransferLeader(context.Background(), &pb.TransferLeaderRequest{TargetId: "m2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Initiated {
		t.Error("expected initiated=true")
	}
	if resp.TargetId != "m2" {
		t.Errorf("got target_id %q, want %q", resp.TargetId, "m2")
	}
}

func TestClusterServer_TransferLeader_Error(t *testing.T) {
	leader := &mockClusterLeader{isLeader: true, transferErr: errors.New("transfer failed")}
	srv := NewClusterServer(nil, leader, nil)
	_, err := srv.TransferLeader(context.Background(), &pb.TransferLeaderRequest{TargetId: "m2"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("got code %v, want Internal", st.Code())
	}
}

// --- Rebalance tests ---

func TestClusterServer_Rebalance_NilShards(t *testing.T) {
	srv := NewClusterServer(nil, nil, nil)
	_, err := srv.Rebalance(context.Background(), &pb.RebalanceRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestClusterServer_Rebalance_Success(t *testing.T) {
	shards := &mockClusterShards{}
	srv := NewClusterServer(nil, nil, shards)

	resp, err := srv.Rebalance(context.Background(), &pb.RebalanceRequest{Reason: "scale-out"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
	if resp.Reason != "scale-out" {
		t.Errorf("got reason %q, want %q", resp.Reason, "scale-out")
	}
}

func TestClusterServer_Rebalance_DefaultReason(t *testing.T) {
	shards := &mockClusterShards{}
	srv := NewClusterServer(nil, nil, shards)

	resp, err := srv.Rebalance(context.Background(), &pb.RebalanceRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Reason != "manual rebalance via gRPC" {
		t.Errorf("got reason %q, want default", resp.Reason)
	}
}

func TestClusterServer_Rebalance_Error(t *testing.T) {
	shards := &mockClusterShards{rebalanceErr: errors.New("no quorum")}
	srv := NewClusterServer(nil, nil, shards)
	_, err := srv.Rebalance(context.Background(), &pb.RebalanceRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("got code %v, want Internal", st.Code())
	}
}

// --- CreateBackup tests ---

func TestClusterServer_CreateBackup_NilMembership(t *testing.T) {
	srv := NewClusterServer(nil, nil, nil)
	_, err := srv.CreateBackup(context.Background(), &pb.CreateBackupRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestClusterServer_CreateBackup_Success(t *testing.T) {
	mem := &mockClusterMembership{info: testClusterInfo()}
	srv := NewClusterServer(mem, nil, nil)

	resp, err := srv.CreateBackup(context.Background(), &pb.CreateBackupRequest{IncludeConfig: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Backup == nil {
		t.Fatal("expected backup data")
	}
	if resp.Backup.Cluster.Name != "test-cluster" {
		t.Errorf("got cluster name %q, want %q", resp.Backup.Cluster.Name, "test-cluster")
	}
	if len(resp.Backup.Cluster.Members) != 3 {
		t.Errorf("got %d members in backup, want 3", len(resp.Backup.Cluster.Members))
	}
	if len(resp.BackupJson) == 0 {
		t.Error("expected non-empty backup_json")
	}
	if resp.Backup.Config == nil {
		t.Error("expected config in backup")
	}
}

// --- RestoreBackup tests ---

func TestClusterServer_RestoreBackup_Unimplemented(t *testing.T) {
	srv := NewClusterServer(nil, nil, nil)
	_, err := srv.RestoreBackup(context.Background(), &pb.RestoreBackupRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unimplemented {
		t.Errorf("got code %v, want Unimplemented", st.Code())
	}
}

// --- WatchMembership tests ---

func TestClusterServer_WatchMembership_NilMembership(t *testing.T) {
	srv := NewClusterServer(nil, nil, nil)
	stream := newMockMembershipStream()
	defer stream.cancel()
	err := srv.WatchMembership(&pb.WatchMembershipRequest{}, stream)
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestClusterServer_WatchMembership_StreamsEvents(t *testing.T) {
	mem := &mockClusterMembership{readyCh: make(chan struct{})}
	srv := NewClusterServer(mem, nil, nil)
	stream := newMockMembershipStream()

	done := make(chan error, 1)
	go func() {
		done <- srv.WatchMembership(&pb.WatchMembershipRequest{}, stream)
	}()

	// Wait for observer to be registered
	<-mem.readyCh

	if len(mem.observers) == 0 {
		t.Fatal("expected observer to be registered")
	}

	// Send event through observer
	mem.observers[0](cluster.MembershipEvent{
		Type:      cluster.MembershipEventJoined,
		Member:    &cluster.Member{ID: "new-node"},
		Timestamp: time.Now(),
		Reason:    "scale-out",
	})

	time.Sleep(10 * time.Millisecond)
	stream.cancel()
	<-done

	if len(stream.sent) != 1 {
		t.Fatalf("got %d events, want 1", len(stream.sent))
	}
	if stream.sent[0].Type != pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_JOINED {
		t.Errorf("got type %v, want JOINED", stream.sent[0].Type)
	}
	if stream.sent[0].Member.Id != "new-node" {
		t.Errorf("got member id %q, want %q", stream.sent[0].Member.Id, "new-node")
	}
}

// --- WatchLeadership tests ---

func TestClusterServer_WatchLeadership_NilLeader(t *testing.T) {
	srv := NewClusterServer(nil, nil, nil)
	stream := newMockLeadershipStream()
	defer stream.cancel()
	err := srv.WatchLeadership(&pb.WatchLeadershipRequest{}, stream)
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestClusterServer_WatchLeadership_StreamsEvents(t *testing.T) {
	leader := &mockClusterLeader{readyCh: make(chan struct{})}
	srv := NewClusterServer(nil, leader, nil)
	stream := newMockLeadershipStream()

	done := make(chan error, 1)
	go func() {
		done <- srv.WatchLeadership(&pb.WatchLeadershipRequest{}, stream)
	}()

	<-leader.readyCh

	if len(leader.observers) == 0 {
		t.Fatal("expected observer to be registered")
	}

	leader.observers[0](cluster.LeadershipEvent{
		Type:             cluster.LeadershipEventElected,
		LeaderID:         "m2",
		PreviousLeaderID: "m1",
		Timestamp:        time.Now(),
		Reason:           "failover",
	})

	time.Sleep(10 * time.Millisecond)
	stream.cancel()
	<-done

	if len(stream.sent) != 1 {
		t.Fatalf("got %d events, want 1", len(stream.sent))
	}
	if stream.sent[0].Type != pb.LeadershipEventType_LEADERSHIP_EVENT_TYPE_ELECTED {
		t.Errorf("got type %v, want ELECTED", stream.sent[0].Type)
	}
	if stream.sent[0].LeaderId != "m2" {
		t.Errorf("got leader_id %q, want %q", stream.sent[0].LeaderId, "m2")
	}
	if stream.sent[0].PreviousLeaderId != "m1" {
		t.Errorf("got previous_leader_id %q, want %q", stream.sent[0].PreviousLeaderId, "m1")
	}
}

// --- Enum conversion tests ---

func TestClusterMemberStatusToProto(t *testing.T) {
	tests := []struct {
		input cluster.MemberStatus
		want  pb.ClusterMemberStatus
	}{
		{cluster.MemberStatusHealthy, pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY},
		{cluster.MemberStatusDegraded, pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_DEGRADED},
		{cluster.MemberStatusUnhealthy, pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNHEALTHY},
		{cluster.MemberStatusUnknown, pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNKNOWN},
		{cluster.MemberStatusLeaving, pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_LEAVING},
		{"other", pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNSPECIFIED},
	}
	for _, tt := range tests {
		if got := clusterMemberStatusToProto(tt.input); got != tt.want {
			t.Errorf("clusterMemberStatusToProto(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestMembershipEventTypeToProto(t *testing.T) {
	tests := []struct {
		input cluster.MembershipEventType
		want  pb.MembershipEventType
	}{
		{cluster.MembershipEventJoined, pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_JOINED},
		{cluster.MembershipEventLeft, pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_LEFT},
		{cluster.MembershipEventFailed, pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_FAILED},
		{cluster.MembershipEventRecovered, pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_RECOVERED},
		{cluster.MembershipEventUpdated, pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_UPDATED},
		{"other", pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_UNSPECIFIED},
	}
	for _, tt := range tests {
		if got := membershipEventTypeToProto(tt.input); got != tt.want {
			t.Errorf("membershipEventTypeToProto(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestLeadershipEventTypeToProto(t *testing.T) {
	tests := []struct {
		input cluster.LeadershipEventType
		want  pb.LeadershipEventType
	}{
		{cluster.LeadershipEventElected, pb.LeadershipEventType_LEADERSHIP_EVENT_TYPE_ELECTED},
		{cluster.LeadershipEventResigned, pb.LeadershipEventType_LEADERSHIP_EVENT_TYPE_RESIGNED},
		{cluster.LeadershipEventLost, pb.LeadershipEventType_LEADERSHIP_EVENT_TYPE_LOST},
		{cluster.LeadershipEventTransferred, pb.LeadershipEventType_LEADERSHIP_EVENT_TYPE_TRANSFERRED},
		{"other", pb.LeadershipEventType_LEADERSHIP_EVENT_TYPE_UNSPECIFIED},
	}
	for _, tt := range tests {
		if got := leadershipEventTypeToProto(tt.input); got != tt.want {
			t.Errorf("leadershipEventTypeToProto(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
