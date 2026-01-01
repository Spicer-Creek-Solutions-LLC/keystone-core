package cluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// CoordinationServer implements the gRPC CoordinationService for server-to-server
// coordination when NATS is unavailable.
type CoordinationServer struct {
	pb.UnimplementedCoordinationServiceServer

	serverID   string
	membership *MembershipManager
	health     *HealthMonitor
	leader     LeaderElection
	nats       NATSStatusProvider

	// Recovery state tracking
	recoveryMu    sync.RWMutex
	recoveryState pb.RecoveryState

	// Metrics
	heartbeatCount   int64
	lastHeartbeatAt  time.Time
	metricsLock      sync.RWMutex

	// Server start time for uptime calculation
	startTime time.Time
}

// NATSStatusProvider provides NATS connection status information.
type NATSStatusProvider interface {
	// IsConnected returns true if connected to NATS
	IsConnected() bool
	// ConnectedURLs returns the list of connected NATS server URLs
	ConnectedURLs() []string
	// JetStreamAvailable returns true if JetStream is available
	JetStreamAvailable() bool
	// LastPublishTime returns the time of the last successful publish
	LastPublishTime() time.Time
	// LastSubscribeTime returns the time of the last successful subscribe
	LastSubscribeTime() time.Time
}

// CoordinationServerConfig holds configuration for the coordination server.
type CoordinationServerConfig struct {
	// ServerID is the unique identifier for this server
	ServerID string
	// TLSConfig for mTLS authentication (optional)
	TLSConfig credentials.TransportCredentials
}

// NewCoordinationServer creates a new coordination server.
func NewCoordinationServer(
	config *CoordinationServerConfig,
	membership *MembershipManager,
	health *HealthMonitor,
	leader LeaderElection,
	nats NATSStatusProvider,
) (*CoordinationServer, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.ServerID == "" {
		return nil, fmt.Errorf("server_id is required")
	}
	if membership == nil {
		return nil, fmt.Errorf("membership manager is required")
	}

	return &CoordinationServer{
		serverID:      config.ServerID,
		membership:    membership,
		health:        health,
		leader:        leader,
		nats:          nats,
		recoveryState: pb.RecoveryState_RECOVERY_STATE_IDLE,
		startTime:     time.Now(),
	}, nil
}

// Register registers the coordination service with a gRPC server.
func (s *CoordinationServer) Register(server *grpc.Server) {
	pb.RegisterCoordinationServiceServer(server, s)
}

// ClusterHealth returns the health status of the cluster from this server's perspective.
func (s *CoordinationServer) ClusterHealth(ctx context.Context, req *pb.ClusterHealthRequest) (*pb.ClusterHealthResponse, error) {
	info := s.membership.GetClusterInfo()
	if info == nil {
		return nil, status.Error(codes.Internal, "failed to get cluster info")
	}

	resp := &pb.ClusterHealthResponse{
		RequestId:      req.RequestId,
		Status:         convertClusterStatus(info.Status),
		Cluster:        info.Name,
		HealthyMembers: int32(info.HealthyCount),
		TotalMembers:   int32(info.MemberCount),
		HasQuorum:      info.HasQuorum,
		LeaderId:       info.LeaderID,
		Timestamp:      timestamppb.Now(),
	}

	// Include member details if requested
	if req.IncludeMembers {
		resp.Members = make([]*pb.MemberStatus, 0, len(info.Members))
		for _, m := range info.Members {
			memberStatus := &pb.MemberStatus{
				MemberId:      m.ID,
				Address:       m.Address,
				Status:        convertMemberStatus(m.Status),
				IsLeader:      m.IsLeader,
				LastHeartbeat: timestamppb.New(m.LastHeartbeat),
				Uptime:        durationpb.New(time.Since(m.JoinedAt)),
				AgentCount:    int32(m.AgentCount),
			}

			// Check NATS connectivity for each member
			if s.nats != nil && m.ID == s.serverID {
				memberStatus.NatsConnected = s.nats.IsConnected()
			}

			resp.Members = append(resp.Members, memberStatus)
		}
	}

	// Include NATS status if requested
	if req.IncludeNats && s.nats != nil {
		resp.NatsStatus = s.getNATSClusterStatus()
	}

	return resp, nil
}

// GetLeader returns information about the current cluster leader.
func (s *CoordinationServer) GetLeader(ctx context.Context, req *pb.GetLeaderRequest) (*pb.GetLeaderResponse, error) {
	resp := &pb.GetLeaderResponse{
		RequestId: req.RequestId,
		Timestamp: timestamppb.Now(),
	}

	if s.leader == nil {
		resp.HasLeader = false
		return resp, nil
	}

	leaderID, err := s.leader.GetLeader(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get leader: %v", err)
	}

	if leaderID == "" {
		resp.HasLeader = false
		return resp, nil
	}

	resp.HasLeader = true
	resp.LeaderId = leaderID

	// Get leader member info
	leader := s.membership.GetLeader()
	if leader != nil {
		resp.LeaderAddress = leader.GRPCAddress
		resp.LeaderSince = timestamppb.New(leader.JoinedAt)
	}

	return resp, nil
}

// NATSStatus returns the NATS connectivity status for this server.
func (s *CoordinationServer) NATSStatus(ctx context.Context, req *pb.NATSStatusRequest) (*pb.NATSStatusResponse, error) {
	resp := &pb.NATSStatusResponse{
		RequestId: req.RequestId,
		ServerId:  s.serverID,
		Timestamp: timestamppb.Now(),
	}

	if s.nats == nil {
		resp.ConnectionStatus = pb.NATSConnectionStatus_NATS_CONNECTION_STATUS_DISCONNECTED
		resp.Error = "NATS provider not configured"
		return resp, nil
	}

	if s.nats.IsConnected() {
		resp.ConnectionStatus = pb.NATSConnectionStatus_NATS_CONNECTION_STATUS_CONNECTED
		resp.ConnectedUrls = s.nats.ConnectedURLs()

		// JetStream status
		resp.JetstreamStatus = &pb.JetStreamStatus{
			Available: s.nats.JetStreamAvailable(),
		}

		// Timestamps
		lastPub := s.nats.LastPublishTime()
		if !lastPub.IsZero() {
			resp.LastPublish = timestamppb.New(lastPub)
		}
		lastSub := s.nats.LastSubscribeTime()
		if !lastSub.IsZero() {
			resp.LastSubscribe = timestamppb.New(lastSub)
		}
	} else {
		resp.ConnectionStatus = pb.NATSConnectionStatus_NATS_CONNECTION_STATUS_DISCONNECTED
		resp.Error = "NATS not connected"
	}

	return resp, nil
}

// RecoveryCoordinate coordinates NATS recovery actions across servers.
func (s *CoordinationServer) RecoveryCoordinate(ctx context.Context, req *pb.RecoveryCoordinateRequest) (*pb.RecoveryCoordinateResponse, error) {
	resp := &pb.RecoveryCoordinateResponse{
		RequestId: req.RequestId,
		ServerId:  s.serverID,
		Timestamp: timestamppb.Now(),
	}

	s.recoveryMu.Lock()
	currentState := s.recoveryState
	s.recoveryMu.Unlock()

	// RESUME is always allowed (to exit recovery state)
	if req.Action == pb.RecoveryAction_RECOVERY_ACTION_RESUME {
		s.recoveryMu.Lock()
		s.recoveryState = pb.RecoveryState_RECOVERY_STATE_IDLE
		s.recoveryMu.Unlock()
		resp.Accepted = true
		resp.State = pb.RecoveryState_RECOVERY_STATE_IDLE
		return resp, nil
	}

	// For other actions, check if we're already in a recovery operation
	if currentState == pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS {
		resp.Accepted = false
		resp.State = currentState
		resp.Error = "recovery already in progress"
		return resp, nil
	}

	// Process the recovery action
	switch req.Action {
	case pb.RecoveryAction_RECOVERY_ACTION_RESTART_EMBEDDED:
		// TODO: Implement embedded NATS restart
		resp.Accepted = true
		resp.State = pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS

	case pb.RecoveryAction_RECOVERY_ACTION_RECONNECT:
		// TODO: Implement NATS reconnection
		resp.Accepted = true
		resp.State = pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS

	case pb.RecoveryAction_RECOVERY_ACTION_FAILOVER:
		// TODO: Implement NATS failover
		resp.Accepted = true
		resp.State = pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS

	case pb.RecoveryAction_RECOVERY_ACTION_DRAIN:
		// TODO: Implement connection draining
		resp.Accepted = true
		resp.State = pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS

	case pb.RecoveryAction_RECOVERY_ACTION_PAUSE:
		s.recoveryMu.Lock()
		s.recoveryState = pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS
		s.recoveryMu.Unlock()
		resp.Accepted = true
		resp.State = pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS

	default:
		resp.Accepted = false
		resp.Error = fmt.Sprintf("unknown recovery action: %v", req.Action)
	}

	return resp, nil
}

// Heartbeat performs a lightweight liveness check between servers.
func (s *CoordinationServer) Heartbeat(ctx context.Context, req *pb.ServerHeartbeatRequest) (*pb.ServerHeartbeatResponse, error) {
	now := time.Now()

	// Calculate latency from sender's timestamp
	var latency time.Duration
	if req.Timestamp != nil {
		latency = now.Sub(req.Timestamp.AsTime())
	}

	// Update metrics
	s.metricsLock.Lock()
	s.heartbeatCount++
	s.lastHeartbeatAt = now
	s.metricsLock.Unlock()

	// Record heartbeat from peer if health monitor is available
	if s.health != nil {
		s.health.RecordHeartbeat(req.SenderId)
	}

	return &pb.ServerHeartbeatResponse{
		ResponderId: s.serverID,
		Timestamp:   timestamppb.New(now),
		Sequence:    req.Sequence,
		Latency:     durationpb.New(latency),
	}, nil
}

// PropagateState propagates state changes when NATS is down.
func (s *CoordinationServer) PropagateState(ctx context.Context, req *pb.PropagateStateRequest) (*pb.PropagateStateResponse, error) {
	resp := &pb.PropagateStateResponse{
		RequestId: req.RequestId,
		ServerId:  s.serverID,
		Timestamp: timestamppb.Now(),
	}

	// Process the state update based on type
	switch req.UpdateType {
	case pb.StateUpdateType_STATE_UPDATE_TYPE_AGENT_REGISTER:
		// TODO: Handle agent registration state propagation
		resp.Applied = true
		resp.CurrentVersion = req.Version

	case pb.StateUpdateType_STATE_UPDATE_TYPE_AGENT_HEARTBEAT:
		// TODO: Handle agent heartbeat state propagation
		resp.Applied = true
		resp.CurrentVersion = req.Version

	case pb.StateUpdateType_STATE_UPDATE_TYPE_AGENT_DISCONNECT:
		// TODO: Handle agent disconnect state propagation
		resp.Applied = true
		resp.CurrentVersion = req.Version

	case pb.StateUpdateType_STATE_UPDATE_TYPE_COMMAND_RESULT:
		// TODO: Handle command result state propagation
		resp.Applied = true
		resp.CurrentVersion = req.Version

	case pb.StateUpdateType_STATE_UPDATE_TYPE_MEMBERSHIP_CHANGE:
		// TODO: Handle membership change state propagation
		resp.Applied = true
		resp.CurrentVersion = req.Version

	default:
		resp.Applied = false
		resp.Error = fmt.Sprintf("unknown state update type: %v", req.UpdateType)
	}

	return resp, nil
}

// getNATSClusterStatus builds the NATS cluster status response.
func (s *CoordinationServer) getNATSClusterStatus() *pb.NATSClusterStatus {
	if s.nats == nil {
		return &pb.NATSClusterStatus{
			Status: pb.NATSHealthStatus_NATS_HEALTH_STATUS_UNKNOWN,
		}
	}

	natsStatus := &pb.NATSClusterStatus{
		JetstreamAvailable: s.nats.JetStreamAvailable(),
	}

	urls := s.nats.ConnectedURLs()
	natsStatus.ConnectedServers = int32(len(urls))

	if s.nats.IsConnected() {
		natsStatus.Status = pb.NATSHealthStatus_NATS_HEALTH_STATUS_HEALTHY
	} else {
		natsStatus.Status = pb.NATSHealthStatus_NATS_HEALTH_STATUS_UNHEALTHY
	}

	// Build server status list
	natsStatus.Servers = make([]*pb.NATSServerStatus, 0, len(urls))
	for _, url := range urls {
		natsStatus.Servers = append(natsStatus.Servers, &pb.NATSServerStatus{
			Address:   url,
			Connected: true,
			LastSeen:  timestamppb.Now(),
		})
	}

	return natsStatus
}

// Helper functions to convert between internal and protobuf types

func convertClusterStatus(s ClusterStatus) pb.ClusterHealthStatus {
	switch s {
	case ClusterStatusHealthy:
		return pb.ClusterHealthStatus_CLUSTER_HEALTH_STATUS_HEALTHY
	case ClusterStatusDegraded:
		return pb.ClusterHealthStatus_CLUSTER_HEALTH_STATUS_DEGRADED
	case ClusterStatusUnhealthy:
		return pb.ClusterHealthStatus_CLUSTER_HEALTH_STATUS_UNHEALTHY
	default:
		return pb.ClusterHealthStatus_CLUSTER_HEALTH_STATUS_UNKNOWN
	}
}

func convertMemberStatus(s MemberStatus) pb.MemberHealthStatus {
	switch s {
	case MemberStatusHealthy:
		return pb.MemberHealthStatus_MEMBER_HEALTH_STATUS_HEALTHY
	case MemberStatusDegraded:
		return pb.MemberHealthStatus_MEMBER_HEALTH_STATUS_DEGRADED
	case MemberStatusUnhealthy:
		return pb.MemberHealthStatus_MEMBER_HEALTH_STATUS_UNHEALTHY
	default:
		return pb.MemberHealthStatus_MEMBER_HEALTH_STATUS_UNKNOWN
	}
}

// GetHeartbeatMetrics returns heartbeat metrics for monitoring.
func (s *CoordinationServer) GetHeartbeatMetrics() (count int64, lastAt time.Time) {
	s.metricsLock.RLock()
	defer s.metricsLock.RUnlock()
	return s.heartbeatCount, s.lastHeartbeatAt
}

// GetRecoveryState returns the current recovery state.
func (s *CoordinationServer) GetRecoveryState() pb.RecoveryState {
	s.recoveryMu.RLock()
	defer s.recoveryMu.RUnlock()
	return s.recoveryState
}

// SetRecoveryState sets the recovery state (for testing).
func (s *CoordinationServer) SetRecoveryState(state pb.RecoveryState) {
	s.recoveryMu.Lock()
	defer s.recoveryMu.Unlock()
	s.recoveryState = state
}
