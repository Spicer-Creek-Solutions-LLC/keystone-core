package controlplane

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.keystone-core.io/keystone-core/internal/cluster"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// Provider interfaces — independently nilable; the RPC group that
// needs a missing one returns codes.Unavailable (the established
// grpc-server precedent). coordHealth is reused from
// grpc_coordination_server.go (same package).
type (
	clusterLeader interface {
		LeaderID(ctx context.Context) (string, error)
		IsLeader() bool
		TransferLeadership(ctx context.Context) error
	}
	clusterMembers interface {
		LoadMembers(ctx context.Context) ([]cluster.Member, error)
		GetMember(ctx context.Context, id string) (cluster.Member, error)
		WatchMembers(ctx context.Context) (<-chan cluster.MemberEvent, error)
	}
	clusterRebalancer interface {
		Rebalance(ctx context.Context) ([]cluster.ShardMove, error)
	}
	clusterShardStore interface {
		List(ctx context.Context) ([]cluster.ShardAssignment, error)
		Assign(ctx context.Context, agentID, memberID string) (cluster.ShardAssignment, error)
		AssignIf(ctx context.Context, agentID, memberID string, expectedVersion int64) (cluster.ShardAssignment, error)
	}
	clusterLeaderWatch interface {
		AddObserver(cluster.LeadershipObserver)
		RemoveObserver(cluster.LeadershipObserver)
	}
)

// ClusterGRPCServer implements v1.ClusterServiceServer (Epic 13
// task 15) — the operator-facing cluster topology + backup surface.
//
// AddMember returns codes.Unimplemented by contract: members
// self-register on start with an ephemeral lease (the honest
// analogue of the policy-CRUD-Unimplemented precedent — there is
// no "add" in the etcd self-registration model). RemoveMember is
// an admin evict, available only when an Evictor hook is wired.
//
// Registering this on the operator gRPC surface at boot is
// deferred (see the "Cluster gRPC services boot registration"
// ROADMAP entry).
type ClusterGRPCServer struct {
	v1.UnimplementedClusterServiceServer

	Health      coordHealth
	Leader      clusterLeader
	Members     clusterMembers
	Rebalancer  clusterRebalancer
	ShardStore  clusterShardStore
	LeaderWatch clusterLeaderWatch
	// Evictor administratively removes a member (boot wires
	// EtcdClient.Delete of the member key). nil ⇒ Unimplemented.
	Evictor func(ctx context.Context, memberID string) error

	ClusterName string
	ConfigJSON  []byte // opaque operator config included in backups
}

func clusterMemberStatusToProto(s cluster.MemberStatus) v1.ClusterMemberStatus {
	switch s {
	case cluster.MemberHealthy:
		return v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY
	case cluster.MemberDegraded:
		return v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_DEGRADED
	case cluster.MemberUnhealthy:
		return v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNREACHABLE
	case cluster.MemberLeaving:
		return v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_LEFT
	default:
		return v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNSPECIFIED
	}
}

func clusterMemberToProto(m cluster.Member, leaderID string) *v1.ClusterMember {
	role := v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_FOLLOWER
	if m.ID == leaderID && leaderID != "" {
		role = v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_LEADER
	}
	return &v1.ClusterMember{
		Id:         m.ID,
		Name:       m.Name,
		Address:    m.Addr,
		Role:       role,
		Status:     clusterMemberStatusToProto(m.Status),
		JoinedAt:   timestamppb.New(m.StartedAt),
		LastSeenAt: timestamppb.New(m.LastHeartbeat),
	}
}

func (s *ClusterGRPCServer) leaderID(ctx context.Context) string {
	if s.Leader == nil {
		return ""
	}
	id, err := s.Leader.LeaderID(ctx)
	if err != nil {
		return ""
	}
	return id
}

func (s *ClusterGRPCServer) GetClusterStatus(ctx context.Context, _ *v1.GetClusterStatusRequest) (*v1.GetClusterStatusResponse, error) {
	if s.Health == nil || s.Members == nil {
		return nil, status.Error(codes.Unavailable, "cluster: health/membership unavailable")
	}
	members, err := s.Members.LoadMembers(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load members: %v", err)
	}
	healthy := 0
	for _, m := range members {
		if m.Status == cluster.MemberHealthy {
			healthy++
		}
	}
	return &v1.GetClusterStatusResponse{
		ClusterId:    s.ClusterName,
		LeaderId:     s.leaderID(ctx),
		MemberCount:  int32(len(members)),
		HealthyCount: int32(healthy),
		Quorum:       s.Health.Quorum() == cluster.QuorumOK,
	}, nil
}

func (s *ClusterGRPCServer) ListMembers(ctx context.Context, req *v1.ListMembersRequest) (*v1.ListMembersResponse, error) {
	if s.Members == nil {
		return nil, status.Error(codes.Unavailable, "cluster: membership unavailable")
	}
	members, err := s.Members.LoadMembers(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load members: %v", err)
	}
	lid := s.leaderID(ctx)
	out := make([]*v1.ClusterMember, 0, len(members))
	for _, m := range members {
		pm := clusterMemberToProto(m, lid)
		if req.GetStatus() != v1.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNSPECIFIED &&
			pm.Status != req.GetStatus() {
			continue
		}
		out = append(out, pm)
	}
	return &v1.ListMembersResponse{Members: out, TotalCount: int32(len(out))}, nil
}

func (s *ClusterGRPCServer) GetMember(ctx context.Context, req *v1.GetMemberRequest) (*v1.GetMemberResponse, error) {
	if s.Members == nil {
		return nil, status.Error(codes.Unavailable, "cluster: membership unavailable")
	}
	m, err := s.Members.GetMember(ctx, req.GetMemberId())
	if errors.Is(err, cluster.ErrMemberNotFound) {
		return nil, status.Errorf(codes.NotFound, "member %q not found", req.GetMemberId())
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get member: %v", err)
	}
	return &v1.GetMemberResponse{Member: clusterMemberToProto(m, s.leaderID(ctx))}, nil
}

func (s *ClusterGRPCServer) AddMember(context.Context, *v1.AddMemberRequest) (*v1.AddMemberResponse, error) {
	return nil, status.Error(codes.Unimplemented,
		"cluster: members self-register on start (no AddMember in the etcd membership model)")
}

func (s *ClusterGRPCServer) RemoveMember(ctx context.Context, req *v1.RemoveMemberRequest) (*v1.RemoveMemberResponse, error) {
	if s.Evictor == nil {
		return nil, status.Error(codes.Unimplemented, "cluster: member eviction not wired")
	}
	if err := s.Evictor(ctx, req.GetMemberId()); err != nil {
		return nil, status.Errorf(codes.Internal, "evict member %q: %v", req.GetMemberId(), err)
	}
	return &v1.RemoveMemberResponse{}, nil
}

func (s *ClusterGRPCServer) GetLeader(ctx context.Context, _ *v1.GetLeaderRequest) (*v1.GetLeaderResponse, error) {
	if s.Leader == nil {
		return nil, status.Error(codes.Unavailable, "cluster: leader elector unavailable")
	}
	id, err := s.Leader.LeaderID(ctx)
	if err != nil && !errors.Is(err, cluster.ErrNoLeader) {
		return nil, status.Errorf(codes.Internal, "get leader: %v", err)
	}
	if id == "" {
		return &v1.GetLeaderResponse{}, nil
	}
	leader := &v1.ClusterMember{Id: id, Role: v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_LEADER}
	if s.Members != nil {
		if m, mErr := s.Members.GetMember(ctx, id); mErr == nil {
			leader = clusterMemberToProto(m, id)
		}
	}
	return &v1.GetLeaderResponse{Leader: leader}, nil
}

func (s *ClusterGRPCServer) TransferLeader(ctx context.Context, _ *v1.TransferLeaderRequest) (*v1.TransferLeaderResponse, error) {
	if s.Leader == nil {
		return nil, status.Error(codes.Unavailable, "cluster: leader elector unavailable")
	}
	if !s.Leader.IsLeader() {
		return nil, status.Error(codes.FailedPrecondition, "cluster: not the leader; transfer must run on the leader")
	}
	if err := s.Leader.TransferLeadership(ctx); err != nil {
		return nil, status.Errorf(codes.Internal, "transfer leadership: %v", err)
	}
	return &v1.TransferLeaderResponse{}, nil
}

func (s *ClusterGRPCServer) Rebalance(ctx context.Context, req *v1.RebalanceRequest) (*v1.RebalanceResponse, error) {
	if s.Rebalancer == nil {
		return nil, status.Error(codes.Unavailable, "cluster: shard manager unavailable")
	}
	if req.GetDryRun() {
		return nil, status.Error(codes.Unimplemented, "cluster: rebalance dry-run not supported in v1.0")
	}
	moves, err := s.Rebalancer.Rebalance(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "rebalance: %v", err)
	}
	return &v1.RebalanceResponse{
		ReassignedAgents: int32(len(moves)),
		Detail:           fmt.Sprintf("%d agents reassigned", len(moves)),
	}, nil
}

func (s *ClusterGRPCServer) CreateBackup(ctx context.Context, req *v1.CreateBackupRequest) (*v1.CreateBackupResponse, error) {
	if s.Members == nil || s.ShardStore == nil {
		return nil, status.Error(codes.Unavailable, "cluster: membership/shard store unavailable")
	}
	// Leader-initiated for ordering (§4.15).
	if s.Leader != nil && !s.Leader.IsLeader() {
		return nil, status.Error(codes.FailedPrecondition, "cluster: backup must run on the leader")
	}
	members, err := s.Members.LoadMembers(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load members: %v", err)
	}
	shards, err := s.ShardStore.List(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list shards: %v", err)
	}
	snap := cluster.BuildSnapshot(s.ClusterName, s.leaderID(ctx), members, shards, s.ConfigJSON)
	blob, err := cluster.MarshalSnapshot(snap)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal snapshot: %v", err)
	}
	return &v1.CreateBackupResponse{
		BackupId:  uuid.NewString(),
		CreatedAt: timestamppb.New(snap.Meta.TakenAt),
		SizeBytes: int64(len(blob)),
		Snapshot:  blob,
	}, nil
}

func (s *ClusterGRPCServer) RestoreBackup(ctx context.Context, req *v1.RestoreBackupRequest) (*v1.RestoreBackupResponse, error) {
	if s.ShardStore == nil {
		return nil, status.Error(codes.Unavailable, "cluster: shard store unavailable")
	}
	snap, err := cluster.UnmarshalSnapshot(req.GetSnapshot())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid snapshot: %v", err)
	}
	if req.GetDryRun() {
		return &v1.RestoreBackupResponse{
			Success: true,
			Detail:  fmt.Sprintf("valid snapshot: %d members, %d shard assignments", len(snap.Members), len(snap.Shards)),
		}, nil
	}
	applied, err := cluster.RestoreShards(ctx, s.ShardStore, snap, req.GetForce())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "restore shards: %v", err)
	}
	return &v1.RestoreBackupResponse{
		Success: true,
		Detail:  fmt.Sprintf("restored %d shard assignments (force=%v)", applied, req.GetForce()),
	}, nil
}

func (s *ClusterGRPCServer) WatchMembership(req *v1.WatchMembershipRequest, stream v1.ClusterService_WatchMembershipServer) error {
	if s.Members == nil {
		return status.Error(codes.Unavailable, "cluster: membership unavailable")
	}
	ctx := stream.Context()
	ch, err := s.Members.WatchMembers(ctx)
	if err != nil {
		return status.Errorf(codes.Internal, "watch members: %v", err)
	}
	filter := map[string]bool{}
	for _, id := range req.GetMemberIds() {
		filter[id] = true
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			if len(filter) > 0 && !filter[ev.Member.ID] {
				continue
			}
			kind := v1.ClusterMembershipChangeKind_CLUSTER_MEMBERSHIP_CHANGE_KIND_STATUS_CHANGED
			switch ev.Type {
			case cluster.MemberJoined:
				kind = v1.ClusterMembershipChangeKind_CLUSTER_MEMBERSHIP_CHANGE_KIND_JOINED
			case cluster.MemberLeft:
				kind = v1.ClusterMembershipChangeKind_CLUSTER_MEMBERSHIP_CHANGE_KIND_LEFT
			}
			if err := stream.Send(&v1.WatchMembershipResponse{
				Member: clusterMemberToProto(ev.Member, ""),
				Kind:   kind,
				At:     timestamppb.New(time.Now().UTC()),
			}); err != nil {
				return err
			}
		}
	}
}

// leaderStreamObserver bridges LeaderElector observer callbacks to
// a bounded channel for the WatchLeadership stream.
type leaderStreamObserver struct {
	ch chan cluster.LeadershipEvent
}

func (o *leaderStreamObserver) OnLeadershipChange(ev cluster.LeadershipEvent) {
	select {
	case o.ch <- ev:
	default: // slow consumer; drop (next event re-establishes truth)
	}
}

func (s *ClusterGRPCServer) WatchLeadership(_ *v1.WatchLeadershipRequest, stream v1.ClusterService_WatchLeadershipServer) error {
	if s.LeaderWatch == nil {
		return status.Error(codes.Unavailable, "cluster: leader elector unavailable")
	}
	ctx := stream.Context()
	obs := &leaderStreamObserver{ch: make(chan cluster.LeadershipEvent, 32)}
	s.LeaderWatch.AddObserver(obs)
	defer s.LeaderWatch.RemoveObserver(obs)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-obs.ch:
			leader := &v1.ClusterMember{}
			if ev.LeaderID != "" {
				leader = &v1.ClusterMember{Id: ev.LeaderID, Role: v1.ClusterMemberRole_CLUSTER_MEMBER_ROLE_LEADER}
			}
			if err := stream.Send(&v1.WatchLeadershipResponse{
				Leader: leader,
				At:     timestamppb.New(time.Now().UTC()),
			}); err != nil {
				return err
			}
		}
	}
}
