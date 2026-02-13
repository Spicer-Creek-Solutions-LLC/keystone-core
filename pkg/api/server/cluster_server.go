package server

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/shawnbutts/keystone-core/internal/cluster"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// ClusterMembershipProvider provides cluster membership operations.
type ClusterMembershipProvider interface {
	GetClusterInfo() *cluster.Info
	ListMembers() []*cluster.Member
	GetMember(id string) (*cluster.Member, error)
	AddMember(ctx context.Context, req *cluster.AddMemberRequest) (*cluster.Member, error)
	RemoveMember(ctx context.Context, memberID string, force bool) error
	GetLeader() *cluster.Member
	HasQuorum() bool
	MemberCount() int
	HealthyMemberCount() int
	AddObserver(observer cluster.MembershipObserver)
	RemoveObserver(observer cluster.MembershipObserver)
}

// ClusterLeaderProvider provides leadership operations.
type ClusterLeaderProvider interface {
	IsLeader() bool
	GetLeaderID() string
	TransferLeadership(ctx context.Context, targetID string) error
	AddObserver(observer cluster.LeadershipObserver)
	RemoveObserver(observer cluster.LeadershipObserver)
}

// ClusterShardProvider provides shard rebalancing operations.
type ClusterShardProvider interface {
	TriggerRebalance(ctx context.Context, reason string) error
}

// ClusterServer implements the ClusterService gRPC server.
type ClusterServer struct {
	pb.UnimplementedClusterServiceServer
	membership ClusterMembershipProvider
	leader     ClusterLeaderProvider
	shards     ClusterShardProvider
}

// NewClusterServer creates a new ClusterServer.
// Any dependency may be nil — RPCs return codes.Unavailable if the required dep is nil.
func NewClusterServer(membership ClusterMembershipProvider, leader ClusterLeaderProvider, shards ClusterShardProvider) *ClusterServer {
	return &ClusterServer{
		membership: membership,
		leader:     leader,
		shards:     shards,
	}
}

// GetClusterStatus returns the overall cluster status.
func (s *ClusterServer) GetClusterStatus(_ context.Context, _ *pb.GetClusterStatusRequest) (*pb.GetClusterStatusResponse, error) {
	if s.membership == nil {
		return nil, status.Error(codes.Unavailable, "cluster membership not available")
	}

	info := s.membership.GetClusterInfo()
	if info == nil {
		return nil, status.Error(codes.Internal, "cluster info unavailable")
	}

	resp := &pb.GetClusterStatusResponse{
		Healthy:     info.Status == cluster.StatusHealthy,
		MemberCount: int32(info.MemberCount),  //nolint:gosec // G115: bounded by member count
		QuorumSize:  int32(info.QuorumSize),    //nolint:gosec // G115: bounded by member count
		HasQuorum:   info.HasQuorum,
		LeaderId:    info.LeaderID,
	}
	if !info.UpdatedAt.IsZero() {
		resp.UpdatedAt = timestamppb.New(info.UpdatedAt)
	}

	for _, m := range info.Members {
		resp.Members = append(resp.Members, clusterMemberToProto(m))
	}

	return resp, nil
}

// ListMembers lists all cluster members with optional filtering.
func (s *ClusterServer) ListMembers(_ context.Context, req *pb.ListMembersRequest) (*pb.ListMembersResponse, error) {
	if s.membership == nil {
		return nil, status.Error(codes.Unavailable, "cluster membership not available")
	}

	members := s.membership.ListMembers()

	// Filter by status if specified
	if req.Status != pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNSPECIFIED {
		filtered := members[:0:0]
		for _, m := range members {
			if clusterMemberStatusToProto(m.Status) == req.Status {
				filtered = append(filtered, m)
			}
		}
		members = filtered
	}

	// Pagination
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	offset := 0
	if req.PageToken != "" {
		offset = parsePageToken(req.PageToken)
	}

	total := len(members)
	end := offset + pageSize
	if end > total {
		end = total
	}
	var page []*cluster.Member
	if offset < total {
		page = members[offset:end]
	}

	resp := &pb.ListMembersResponse{
		TotalCount: int32(total), //nolint:gosec // G115: bounded by member count
	}
	for _, m := range page {
		resp.Members = append(resp.Members, clusterMemberToProto(m))
	}
	if end < total {
		resp.NextPageToken = encodePageToken(end)
	}

	return resp, nil
}

// GetMember retrieves a specific cluster member.
func (s *ClusterServer) GetMember(_ context.Context, req *pb.GetMemberRequest) (*pb.GetMemberResponse, error) {
	if s.membership == nil {
		return nil, status.Error(codes.Unavailable, "cluster membership not available")
	}
	if req.MemberId == "" {
		return nil, status.Error(codes.InvalidArgument, "member_id is required")
	}

	m, err := s.membership.GetMember(req.MemberId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "member %q not found", req.MemberId)
	}

	return &pb.GetMemberResponse{
		Member: clusterMemberToProto(m),
	}, nil
}

// AddMember adds a new member to the cluster.
func (s *ClusterServer) AddMember(ctx context.Context, req *pb.AddMemberRequest) (*pb.AddMemberResponse, error) {
	if s.membership == nil {
		return nil, status.Error(codes.Unavailable, "cluster membership not available")
	}
	if req.Address == "" {
		return nil, status.Error(codes.InvalidArgument, "address is required")
	}

	addReq := &cluster.AddMemberRequest{
		ID:          req.Id,
		Name:        req.Name,
		Address:     req.Address,
		GRPCAddress: req.GrpcAddress,
		NATSAddress: req.NatsAddress,
		Metadata:    req.Metadata,
	}

	m, err := s.membership.AddMember(ctx, addReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to add member: %v", err)
	}

	return &pb.AddMemberResponse{
		Member: clusterMemberToProto(m),
	}, nil
}

// RemoveMember removes a member from the cluster.
func (s *ClusterServer) RemoveMember(ctx context.Context, req *pb.RemoveMemberRequest) (*pb.RemoveMemberResponse, error) {
	if s.membership == nil {
		return nil, status.Error(codes.Unavailable, "cluster membership not available")
	}
	if req.MemberId == "" {
		return nil, status.Error(codes.InvalidArgument, "member_id is required")
	}

	if err := s.membership.RemoveMember(ctx, req.MemberId, req.Force); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to remove member: %v", err)
	}

	return &pb.RemoveMemberResponse{Removed: true}, nil
}

// GetLeader returns the current cluster leader.
func (s *ClusterServer) GetLeader(_ context.Context, _ *pb.GetClusterLeaderRequest) (*pb.GetClusterLeaderResponse, error) {
	if s.membership == nil {
		return nil, status.Error(codes.Unavailable, "cluster membership not available")
	}

	leader := s.membership.GetLeader()
	if leader == nil {
		return nil, status.Error(codes.NotFound, "no leader elected")
	}

	return &pb.GetClusterLeaderResponse{
		Leader: clusterMemberToProto(leader),
	}, nil
}

// TransferLeader transfers leadership to another member.
func (s *ClusterServer) TransferLeader(ctx context.Context, req *pb.TransferLeaderRequest) (*pb.TransferLeaderResponse, error) {
	if s.leader == nil {
		return nil, status.Error(codes.Unavailable, "leader election not available")
	}
	if req.TargetId == "" {
		return nil, status.Error(codes.InvalidArgument, "target_id is required")
	}
	if !s.leader.IsLeader() {
		return nil, status.Error(codes.FailedPrecondition, "not the current leader")
	}

	if err := s.leader.TransferLeadership(ctx, req.TargetId); err != nil {
		return nil, status.Errorf(codes.Internal, "leadership transfer failed: %v", err)
	}

	return &pb.TransferLeaderResponse{
		Initiated: true,
		Message:   "leadership transfer initiated",
		TargetId:  req.TargetId,
	}, nil
}

// Rebalance triggers agent rebalancing across cluster members.
func (s *ClusterServer) Rebalance(ctx context.Context, req *pb.RebalanceRequest) (*pb.RebalanceResponse, error) {
	if s.shards == nil {
		return nil, status.Error(codes.Unavailable, "shard manager not available")
	}

	reason := req.Reason
	if reason == "" {
		reason = "manual rebalance via gRPC"
	}

	if err := s.shards.TriggerRebalance(ctx, reason); err != nil {
		return nil, status.Errorf(codes.Internal, "rebalance failed: %v", err)
	}

	return &pb.RebalanceResponse{
		Success: true,
		Reason:  reason,
	}, nil
}

// CreateBackup creates a cluster state backup.
func (s *ClusterServer) CreateBackup(_ context.Context, req *pb.CreateBackupRequest) (*pb.CreateBackupResponse, error) {
	if s.membership == nil {
		return nil, status.Error(codes.Unavailable, "cluster membership not available")
	}

	info := s.membership.GetClusterInfo()
	if info == nil {
		return nil, status.Error(codes.Internal, "cluster info unavailable")
	}

	backup := &pb.BackupData{
		Version:   "1",
		Timestamp: timestamppb.Now(),
		Cluster: &pb.ClusterBackup{
			Name:       info.Name,
			QuorumSize: int32(info.QuorumSize), //nolint:gosec // G115: bounded
			LeaderId:   info.LeaderID,
		},
	}

	for _, m := range info.Members {
		backup.Cluster.Members = append(backup.Cluster.Members, clusterMemberToProto(m))
	}

	// Include config placeholder (actual config backup requires additional stores)
	if req.IncludeConfig {
		backup.Config = map[string]string{
			"cluster_name": info.Name,
			"member_count": string(rune(info.MemberCount + '0')),
		}
	}

	// Serialize backup to JSON
	backupJSON, err := json.Marshal(backup)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to serialize backup: %v", err)
	}

	return &pb.CreateBackupResponse{
		Backup:     backup,
		BackupJson: backupJSON,
	}, nil
}

// RestoreBackup restores cluster state from a backup.
func (s *ClusterServer) RestoreBackup(_ context.Context, _ *pb.RestoreBackupRequest) (*pb.RestoreBackupResponse, error) {
	// Cluster restore requires careful coordination with etcd, membership, and shard stores.
	// This is a dangerous operation that needs more infrastructure than what's currently wired.
	return nil, status.Error(codes.Unimplemented, "cluster restore not yet available — use kscore-cluster-backup CLI")
}

// WatchMembership watches for membership changes via server-side streaming.
func (s *ClusterServer) WatchMembership(req *pb.WatchMembershipRequest, stream grpc.ServerStreamingServer[pb.MembershipEvent]) error {
	if s.membership == nil {
		return status.Error(codes.Unavailable, "cluster membership not available")
	}

	// Build filter sets
	typeFilter := make(map[pb.MembershipEventType]struct{}, len(req.Types))
	for _, t := range req.Types {
		typeFilter[t] = struct{}{}
	}
	memberFilter := make(map[string]struct{}, len(req.MemberIds))
	for _, id := range req.MemberIds {
		memberFilter[id] = struct{}{}
	}

	eventCh := make(chan *cluster.MembershipEvent, 64)

	observer := func(event cluster.MembershipEvent) {
		// Apply filters
		if len(typeFilter) > 0 {
			if _, ok := typeFilter[membershipEventTypeToProto(event.Type)]; !ok {
				return
			}
		}
		if len(memberFilter) > 0 && event.Member != nil {
			if _, ok := memberFilter[event.Member.ID]; !ok {
				return
			}
		}
		select {
		case eventCh <- &event:
		case <-stream.Context().Done():
		}
	}

	s.membership.AddObserver(observer)
	defer s.membership.RemoveObserver(observer)

	for {
		select {
		case evt := <-eventCh:
			protoEvt := &pb.MembershipEvent{
				Type:      membershipEventTypeToProto(evt.Type),
				Timestamp: timestamppb.New(evt.Timestamp),
				Reason:    evt.Reason,
			}
			if evt.Member != nil {
				protoEvt.Member = clusterMemberToProto(evt.Member)
			}
			if err := stream.Send(protoEvt); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

// WatchLeadership watches for leadership changes via server-side streaming.
func (s *ClusterServer) WatchLeadership(req *pb.WatchLeadershipRequest, stream grpc.ServerStreamingServer[pb.LeadershipEvent]) error {
	if s.leader == nil {
		return status.Error(codes.Unavailable, "leader election not available")
	}

	typeFilter := make(map[pb.LeadershipEventType]struct{}, len(req.Types))
	for _, t := range req.Types {
		typeFilter[t] = struct{}{}
	}

	eventCh := make(chan *cluster.LeadershipEvent, 64)

	observer := func(event cluster.LeadershipEvent) {
		if len(typeFilter) > 0 {
			if _, ok := typeFilter[leadershipEventTypeToProto(event.Type)]; !ok {
				return
			}
		}
		select {
		case eventCh <- &event:
		case <-stream.Context().Done():
		}
	}

	s.leader.AddObserver(observer)
	defer s.leader.RemoveObserver(observer)

	for {
		select {
		case evt := <-eventCh:
			if err := stream.Send(&pb.LeadershipEvent{
				Type:             leadershipEventTypeToProto(evt.Type),
				LeaderId:         evt.LeaderID,
				PreviousLeaderId: evt.PreviousLeaderID,
				Timestamp:        timestamppb.New(evt.Timestamp),
				Reason:           evt.Reason,
			}); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

// --- Conversion helpers ---

func clusterMemberToProto(m *cluster.Member) *pb.Member {
	proto := &pb.Member{
		Id:          m.ID,
		Name:        m.Name,
		Address:     m.Address,
		GrpcAddress: m.GRPCAddress,
		NatsAddress: m.NATSAddress,
		Status:      clusterMemberStatusToProto(m.Status),
		IsLeader:    m.IsLeader,
		Version:     m.Version,
		Metadata:    m.Metadata,
		AgentCount:  int32(m.AgentCount), //nolint:gosec // G115: bounded
		JobCount:    int32(m.JobCount),   //nolint:gosec // G115: bounded
	}
	if !m.JoinedAt.IsZero() {
		proto.JoinedAt = timestamppb.New(m.JoinedAt)
	}
	if !m.LastHeartbeat.IsZero() {
		proto.LastHeartbeat = timestamppb.New(m.LastHeartbeat)
	}
	return proto
}

func clusterMemberStatusToProto(s cluster.MemberStatus) pb.ClusterMemberStatus {
	switch s {
	case cluster.MemberStatusHealthy:
		return pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_HEALTHY
	case cluster.MemberStatusDegraded:
		return pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_DEGRADED
	case cluster.MemberStatusUnhealthy:
		return pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNHEALTHY
	case cluster.MemberStatusUnknown:
		return pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNKNOWN
	case cluster.MemberStatusLeaving:
		return pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_LEAVING
	default:
		return pb.ClusterMemberStatus_CLUSTER_MEMBER_STATUS_UNSPECIFIED
	}
}

func membershipEventTypeToProto(t cluster.MembershipEventType) pb.MembershipEventType {
	switch t {
	case cluster.MembershipEventJoined:
		return pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_JOINED
	case cluster.MembershipEventLeft:
		return pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_LEFT
	case cluster.MembershipEventFailed:
		return pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_FAILED
	case cluster.MembershipEventRecovered:
		return pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_RECOVERED
	case cluster.MembershipEventUpdated:
		return pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_UPDATED
	default:
		return pb.MembershipEventType_MEMBERSHIP_EVENT_TYPE_UNSPECIFIED
	}
}

func leadershipEventTypeToProto(t cluster.LeadershipEventType) pb.LeadershipEventType {
	switch t {
	case cluster.LeadershipEventElected:
		return pb.LeadershipEventType_LEADERSHIP_EVENT_TYPE_ELECTED
	case cluster.LeadershipEventResigned:
		return pb.LeadershipEventType_LEADERSHIP_EVENT_TYPE_RESIGNED
	case cluster.LeadershipEventLost:
		return pb.LeadershipEventType_LEADERSHIP_EVENT_TYPE_LOST
	case cluster.LeadershipEventTransferred:
		return pb.LeadershipEventType_LEADERSHIP_EVENT_TYPE_TRANSFERRED
	default:
		return pb.LeadershipEventType_LEADERSHIP_EVENT_TYPE_UNSPECIFIED
	}
}

// Ensure ClusterServer satisfies the interface at compile time.
var _ pb.ClusterServiceServer = (*ClusterServer)(nil)
