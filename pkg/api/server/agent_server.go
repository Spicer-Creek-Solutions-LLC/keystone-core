package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/shawnbutts/keystone-core/internal/controlplane"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// AgentProvider retrieves agent information from the control plane.
type AgentProvider interface {
	GetAgent(agentID string) (*controlplane.AgentInfo, error)
}

// AgentServer implements the AgentService gRPC server.
type AgentServer struct {
	pb.UnimplementedAgentServiceServer
	agents AgentProvider
}

// NewAgentServer creates a new AgentServer.
// agents may be nil — GetAgentInfo returns codes.Unavailable when nil.
func NewAgentServer(agents AgentProvider) *AgentServer {
	return &AgentServer{agents: agents}
}

// GetAgentInfo retrieves agent information by ID.
func (s *AgentServer) GetAgentInfo(ctx context.Context, req *pb.GetAgentInfoRequest) (*pb.GetAgentInfoResponse, error) {
	if s.agents == nil {
		return nil, status.Error(codes.Unavailable, "agent provider not available")
	}
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	info, err := s.agents.GetAgent(req.AgentId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "agent not found: %s", req.AgentId)
	}

	resp := &pb.GetAgentInfoResponse{
		AgentId:  info.ID,
		Metadata: info.Metadata,
		Status:   info.Status,
		Metrics:  info.LastMetrics,
	}
	if !info.LastHeartbeat.IsZero() {
		resp.LastHeartbeat = timestamppb.New(info.LastHeartbeat)
	}
	if !info.RegisteredAt.IsZero() {
		resp.RegisteredAt = timestamppb.New(info.RegisteredAt)
	}
	return resp, nil
}

// Register is not implemented via gRPC — agents register through NATS.
func (s *AgentServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.RegisterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "agent registration uses NATS, not gRPC")
}

// Heartbeat is not implemented via gRPC — agents send heartbeats through NATS.
func (s *AgentServer) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	return nil, status.Error(codes.Unimplemented, "agent heartbeat uses NATS, not gRPC")
}

// ExecuteCommand is not implemented via gRPC — command execution goes through NATS.
func (s *AgentServer) ExecuteCommand(req *pb.ExecuteCommandRequest, stream pb.AgentService_ExecuteCommandServer) error {
	return status.Error(codes.Unimplemented, "command execution uses NATS, not gRPC")
}

// Ensure AgentServer satisfies the interface at compile time.
var _ pb.AgentServiceServer = (*AgentServer)(nil)

// Ensure ConnectionManager satisfies AgentProvider at compile time.
var _ AgentProvider = (*controlplane.ConnectionManager)(nil)
