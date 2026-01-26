package cluster

import (
	"context"
	"fmt"
	"strings"
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

	// NATS control and state propagation
	natsController  NATSController
	statePropagator StatePropagator

	// Recovery state tracking
	recoveryMu    sync.RWMutex
	recoveryState pb.RecoveryState

	// State version tracking for consistency
	stateVersionMu sync.RWMutex
	stateVersions  map[string]int64 // key: state type, value: version

	// Metrics
	heartbeatCount     int64
	lastHeartbeatAt    time.Time
	recoveryCount      int64
	propagationCount   int64
	propagationErrors  int64
	metricsLock        sync.RWMutex

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

// NATSController provides control operations for NATS.
type NATSController interface {
	// RestartEmbedded restarts the embedded NATS server (returns error if not embedded mode)
	RestartEmbedded(ctx context.Context) error
	// Reconnect forces a reconnection to NATS servers
	Reconnect(ctx context.Context) error
	// Drain gracefully drains all connections before disconnect
	Drain(ctx context.Context) error
	// Failover switches to backup NATS servers
	Failover(ctx context.Context, targetURLs []string) error
	// IsEmbedded returns true if running in embedded NATS mode
	IsEmbedded() bool
}

// StatePropagator handles propagation of state changes to local storage.
// The data parameter contains serialized state that includes entity identifiers.
type StatePropagator interface {
	// PropagateAgentRegister handles agent registration propagation
	// data contains serialized agent registration info including agent ID
	PropagateAgentRegister(ctx context.Context, data []byte) error
	// PropagateAgentHeartbeat handles agent heartbeat propagation
	// data contains serialized heartbeat info including agent ID
	PropagateAgentHeartbeat(ctx context.Context, data []byte) error
	// PropagateAgentDisconnect handles agent disconnect propagation
	// data contains serialized disconnect info including agent ID
	PropagateAgentDisconnect(ctx context.Context, data []byte) error
	// PropagateCommandResult handles command result propagation
	// data contains serialized command result including command ID
	PropagateCommandResult(ctx context.Context, data []byte) error
	// PropagateMembershipChange handles membership change propagation
	// data contains serialized membership change including member ID
	PropagateMembershipChange(ctx context.Context, data []byte) error
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
		stateVersions: make(map[string]int64),
		recoveryState: pb.RecoveryState_RECOVERY_STATE_IDLE,
		startTime:     time.Now(),
	}, nil
}

// SetNATSController sets the NATS controller for recovery operations.
func (s *CoordinationServer) SetNATSController(controller NATSController) {
	s.natsController = controller
}

// SetStatePropagator sets the state propagator for state synchronization.
func (s *CoordinationServer) SetStatePropagator(propagator StatePropagator) {
	s.statePropagator = propagator
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
		if err := s.executeRestartEmbedded(ctx); err != nil {
			resp.Accepted = false
			resp.State = pb.RecoveryState_RECOVERY_STATE_FAILED
			resp.Error = err.Error()
			return resp, nil
		}
		resp.Accepted = true
		resp.State = pb.RecoveryState_RECOVERY_STATE_IDLE

	case pb.RecoveryAction_RECOVERY_ACTION_RECONNECT:
		if err := s.executeReconnect(ctx); err != nil {
			resp.Accepted = false
			resp.State = pb.RecoveryState_RECOVERY_STATE_FAILED
			resp.Error = err.Error()
			return resp, nil
		}
		resp.Accepted = true
		resp.State = pb.RecoveryState_RECOVERY_STATE_IDLE

	case pb.RecoveryAction_RECOVERY_ACTION_FAILOVER:
		// Extract target URLs from parameters (comma-separated list)
		var targetURLs []string
		if urlsStr, ok := req.Parameters["target_urls"]; ok && urlsStr != "" {
			targetURLs = strings.Split(urlsStr, ",")
		}
		if err := s.executeFailover(ctx, targetURLs); err != nil {
			resp.Accepted = false
			resp.State = pb.RecoveryState_RECOVERY_STATE_FAILED
			resp.Error = err.Error()
			return resp, nil
		}
		resp.Accepted = true
		resp.State = pb.RecoveryState_RECOVERY_STATE_IDLE

	case pb.RecoveryAction_RECOVERY_ACTION_DRAIN:
		if err := s.executeDrain(ctx); err != nil {
			resp.Accepted = false
			resp.State = pb.RecoveryState_RECOVERY_STATE_FAILED
			resp.Error = err.Error()
			return resp, nil
		}
		resp.Accepted = true
		resp.State = pb.RecoveryState_RECOVERY_STATE_IDLE

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

	// Update metrics
	s.metricsLock.Lock()
	s.recoveryCount++
	s.metricsLock.Unlock()

	return resp, nil
}

// executeRestartEmbedded restarts the embedded NATS server.
func (s *CoordinationServer) executeRestartEmbedded(ctx context.Context) error {
	if s.natsController == nil {
		return fmt.Errorf("NATS controller not configured")
	}

	if !s.natsController.IsEmbedded() {
		return fmt.Errorf("not running in embedded NATS mode")
	}

	s.recoveryMu.Lock()
	s.recoveryState = pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS
	s.recoveryMu.Unlock()

	defer func() {
		s.recoveryMu.Lock()
		s.recoveryState = pb.RecoveryState_RECOVERY_STATE_IDLE
		s.recoveryMu.Unlock()
	}()

	if err := s.natsController.RestartEmbedded(ctx); err != nil {
		s.recoveryMu.Lock()
		s.recoveryState = pb.RecoveryState_RECOVERY_STATE_FAILED
		s.recoveryMu.Unlock()
		return fmt.Errorf("failed to restart embedded NATS: %w", err)
	}

	return nil
}

// executeReconnect forces a reconnection to NATS servers.
func (s *CoordinationServer) executeReconnect(ctx context.Context) error {
	if s.natsController == nil {
		return fmt.Errorf("NATS controller not configured")
	}

	s.recoveryMu.Lock()
	s.recoveryState = pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS
	s.recoveryMu.Unlock()

	defer func() {
		s.recoveryMu.Lock()
		s.recoveryState = pb.RecoveryState_RECOVERY_STATE_IDLE
		s.recoveryMu.Unlock()
	}()

	if err := s.natsController.Reconnect(ctx); err != nil {
		s.recoveryMu.Lock()
		s.recoveryState = pb.RecoveryState_RECOVERY_STATE_FAILED
		s.recoveryMu.Unlock()
		return fmt.Errorf("failed to reconnect to NATS: %w", err)
	}

	return nil
}

// executeFailover switches to backup NATS servers.
func (s *CoordinationServer) executeFailover(ctx context.Context, targetURLs []string) error {
	if s.natsController == nil {
		return fmt.Errorf("NATS controller not configured")
	}

	if len(targetURLs) == 0 {
		return fmt.Errorf("no target URLs provided for failover")
	}

	s.recoveryMu.Lock()
	s.recoveryState = pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS
	s.recoveryMu.Unlock()

	defer func() {
		s.recoveryMu.Lock()
		s.recoveryState = pb.RecoveryState_RECOVERY_STATE_IDLE
		s.recoveryMu.Unlock()
	}()

	if err := s.natsController.Failover(ctx, targetURLs); err != nil {
		s.recoveryMu.Lock()
		s.recoveryState = pb.RecoveryState_RECOVERY_STATE_FAILED
		s.recoveryMu.Unlock()
		return fmt.Errorf("failed to failover to target NATS servers: %w", err)
	}

	return nil
}

// executeDrain gracefully drains all NATS connections.
func (s *CoordinationServer) executeDrain(ctx context.Context) error {
	if s.natsController == nil {
		return fmt.Errorf("NATS controller not configured")
	}

	s.recoveryMu.Lock()
	s.recoveryState = pb.RecoveryState_RECOVERY_STATE_IN_PROGRESS
	s.recoveryMu.Unlock()

	defer func() {
		s.recoveryMu.Lock()
		s.recoveryState = pb.RecoveryState_RECOVERY_STATE_IDLE
		s.recoveryMu.Unlock()
	}()

	if err := s.natsController.Drain(ctx); err != nil {
		s.recoveryMu.Lock()
		s.recoveryState = pb.RecoveryState_RECOVERY_STATE_FAILED
		s.recoveryMu.Unlock()
		return fmt.Errorf("failed to drain NATS connections: %w", err)
	}

	return nil
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

	// Check version to avoid applying stale updates
	if !s.shouldApplyUpdate(req.UpdateType.String(), req.Version) {
		resp.Applied = false
		resp.Error = "stale update: version too old"
		resp.CurrentVersion = s.getCurrentVersion(req.UpdateType.String())
		return resp, nil
	}

	// Process the state update based on type
	var err error
	switch req.UpdateType {
	case pb.StateUpdateType_STATE_UPDATE_TYPE_AGENT_REGISTER:
		err = s.propagateAgentRegister(ctx, req.StateData)

	case pb.StateUpdateType_STATE_UPDATE_TYPE_AGENT_HEARTBEAT:
		err = s.propagateAgentHeartbeat(ctx, req.StateData)

	case pb.StateUpdateType_STATE_UPDATE_TYPE_AGENT_DISCONNECT:
		err = s.propagateAgentDisconnect(ctx, req.StateData)

	case pb.StateUpdateType_STATE_UPDATE_TYPE_COMMAND_RESULT:
		err = s.propagateCommandResult(ctx, req.StateData)

	case pb.StateUpdateType_STATE_UPDATE_TYPE_MEMBERSHIP_CHANGE:
		err = s.propagateMembershipChange(ctx, req.StateData)

	default:
		resp.Applied = false
		resp.Error = fmt.Sprintf("unknown state update type: %v", req.UpdateType)
		return resp, nil
	}

	// Update metrics
	s.metricsLock.Lock()
	s.propagationCount++
	if err != nil {
		s.propagationErrors++
	}
	s.metricsLock.Unlock()

	if err != nil {
		resp.Applied = false
		resp.Error = err.Error()
		resp.CurrentVersion = s.getCurrentVersion(req.UpdateType.String())
		return resp, nil
	}

	// Update version tracking
	s.updateVersion(req.UpdateType.String(), req.Version)

	resp.Applied = true
	resp.CurrentVersion = req.Version
	return resp, nil
}

// shouldApplyUpdate checks if an update should be applied based on version.
func (s *CoordinationServer) shouldApplyUpdate(updateType string, version int64) bool {
	s.stateVersionMu.RLock()
	defer s.stateVersionMu.RUnlock()

	currentVersion, exists := s.stateVersions[updateType]
	if !exists {
		return true // First update of this type
	}

	return version > currentVersion
}

// getCurrentVersion returns the current version for a state type.
func (s *CoordinationServer) getCurrentVersion(updateType string) int64 {
	s.stateVersionMu.RLock()
	defer s.stateVersionMu.RUnlock()
	return s.stateVersions[updateType]
}

// updateVersion updates the version for a state type.
func (s *CoordinationServer) updateVersion(updateType string, version int64) {
	s.stateVersionMu.Lock()
	defer s.stateVersionMu.Unlock()
	if s.stateVersions == nil {
		s.stateVersions = make(map[string]int64)
	}
	s.stateVersions[updateType] = version
}

// propagateAgentRegister handles agent registration propagation.
func (s *CoordinationServer) propagateAgentRegister(ctx context.Context, data []byte) error {
	if s.statePropagator == nil {
		// No propagator configured - just accept the update
		return nil
	}
	return s.statePropagator.PropagateAgentRegister(ctx, data)
}

// propagateAgentHeartbeat handles agent heartbeat propagation.
func (s *CoordinationServer) propagateAgentHeartbeat(ctx context.Context, data []byte) error {
	if s.statePropagator == nil {
		// No propagator configured - just accept the update
		return nil
	}
	return s.statePropagator.PropagateAgentHeartbeat(ctx, data)
}

// propagateAgentDisconnect handles agent disconnect propagation.
func (s *CoordinationServer) propagateAgentDisconnect(ctx context.Context, data []byte) error {
	if s.statePropagator == nil {
		// No propagator configured - just accept the update
		return nil
	}
	return s.statePropagator.PropagateAgentDisconnect(ctx, data)
}

// propagateCommandResult handles command result propagation.
func (s *CoordinationServer) propagateCommandResult(ctx context.Context, data []byte) error {
	if s.statePropagator == nil {
		// No propagator configured - just accept the update
		return nil
	}
	return s.statePropagator.PropagateCommandResult(ctx, data)
}

// propagateMembershipChange handles membership change propagation.
func (s *CoordinationServer) propagateMembershipChange(ctx context.Context, data []byte) error {
	if s.statePropagator == nil {
		// No propagator configured - just accept the update
		return nil
	}
	return s.statePropagator.PropagateMembershipChange(ctx, data)
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
