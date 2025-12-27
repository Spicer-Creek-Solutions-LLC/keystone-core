package server

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/controlplane"
	"github.com/shawnbutts/keystone-core/pkg/state"
)

// ControlPlaneServer implements the ControlPlaneService
type ControlPlaneServer struct {
	pb.UnimplementedControlPlaneServiceServer
	connMgr         *controlplane.ConnectionManager
	dispatcher      *controlplane.CommandDispatcher
	batchDispatcher *controlplane.BatchDispatcher
	store           state.Store
}

// NewControlPlaneServer creates a new control plane API server
func NewControlPlaneServer(connMgr *controlplane.ConnectionManager, dispatcher *controlplane.CommandDispatcher, batchDispatcher *controlplane.BatchDispatcher, store state.Store) *ControlPlaneServer {
	return &ControlPlaneServer{
		connMgr:         connMgr,
		dispatcher:      dispatcher,
		batchDispatcher: batchDispatcher,
		store:           store,
	}
}

// ListAgents lists all registered agents
func (s *ControlPlaneServer) ListAgents(ctx context.Context, req *pb.ListAgentsRequest) (*pb.ListAgentsResponse, error) {
	// Build filter
	filter := &state.AgentFilter{
		Limit:  int(req.PageSize),
		Offset: 0, // TODO: Parse page_token for offset
	}

	if req.Status != pb.AgentStatus_AGENT_STATUS_UNSPECIFIED {
		filter.Status = &req.Status
	}

	// Get agents from store
	agents, err := s.store.ListAgents(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	// Convert to protobuf
	pbAgents := make([]*pb.AgentInfo, 0, len(agents))
	for _, agent := range agents {
		pbAgents = append(pbAgents, convertAgentRecordToProto(agent))
	}

	return &pb.ListAgentsResponse{
		Agents:     pbAgents,
		TotalCount: int32(len(pbAgents)),
	}, nil
}

// GetAgent retrieves information about a specific agent
func (s *ControlPlaneServer) GetAgent(ctx context.Context, req *pb.GetAgentRequest) (*pb.GetAgentResponse, error) {
	agent, err := s.store.GetAgent(ctx, req.AgentId)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	return &pb.GetAgentResponse{
		Agent: convertAgentRecordToProto(agent),
	}, nil
}

// ExecuteCommand executes a command on an agent
func (s *ControlPlaneServer) ExecuteCommand(req *pb.ExecuteCommandRequest, stream pb.ControlPlaneService_ExecuteCommandServer) error {
	// Dispatch command
	responseChan, err := s.dispatcher.ExecuteCommand(stream.Context(), req)
	if err != nil {
		return fmt.Errorf("failed to execute command: %w", err)
	}

	// Stream responses back to client
	for resp := range responseChan {
		if err := stream.Send(resp); err != nil {
			return fmt.Errorf("failed to send response: %w", err)
		}
	}

	return nil
}

// GetCommandStatus retrieves command execution status
func (s *ControlPlaneServer) GetCommandStatus(ctx context.Context, req *pb.GetCommandStatusRequest) (*pb.GetCommandStatusResponse, error) {
	cmd, err := s.store.GetCommand(ctx, req.CommandId)
	if err != nil {
		return nil, fmt.Errorf("failed to get command: %w", err)
	}

	resp := &pb.GetCommandStatusResponse{
		CommandId: cmd.ID,
		Status:    cmd.Status,
		AgentId:   cmd.AgentID,
		Command:   cmd.Command,
		Args:      cmd.Args,
		ExitCode:  cmd.ExitCode,
		Stdout:    cmd.Stdout,
		Stderr:    cmd.Stderr,
		Error:     cmd.Error,
		CreatedAt: timestamppb.New(cmd.CreatedAt),
	}

	if cmd.StartedAt != nil {
		resp.StartedAt = timestamppb.New(*cmd.StartedAt)
	}
	if cmd.CompletedAt != nil {
		resp.CompletedAt = timestamppb.New(*cmd.CompletedAt)
	}
	resp.DurationMs = cmd.DurationMs

	return resp, nil
}

// ListCommands lists command execution history
func (s *ControlPlaneServer) ListCommands(ctx context.Context, req *pb.ListCommandsRequest) (*pb.ListCommandsResponse, error) {
	// Build filter
	filter := &state.CommandFilter{
		AgentID: req.AgentId,
		Limit:   int(req.PageSize),
		Offset:  0, // TODO: Parse page_token for offset
	}

	if req.Status != pb.CommandStatus_COMMAND_STATUS_UNSPECIFIED {
		filter.Status = &req.Status
	}

	// Get commands from store
	commands, err := s.store.ListCommands(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list commands: %w", err)
	}

	// Convert to protobuf
	pbCommands := make([]*pb.CommandInfo, 0, len(commands))
	for _, cmd := range commands {
		cmdInfo := &pb.CommandInfo{
			CommandId:  cmd.ID,
			AgentId:    cmd.AgentID,
			Command:    cmd.Command,
			Args:       cmd.Args,
			Status:     cmd.Status,
			ExitCode:   cmd.ExitCode,
			CreatedAt:  timestamppb.New(cmd.CreatedAt),
			DurationMs: cmd.DurationMs,
		}
		if cmd.StartedAt != nil {
			cmdInfo.StartedAt = timestamppb.New(*cmd.StartedAt)
		}
		if cmd.CompletedAt != nil {
			cmdInfo.CompletedAt = timestamppb.New(*cmd.CompletedAt)
		}
		pbCommands = append(pbCommands, cmdInfo)
	}

	return &pb.ListCommandsResponse{
		Commands:   pbCommands,
		TotalCount: int32(len(pbCommands)),
	}, nil
}

// BatchExecuteCommand executes a command across multiple agents using a target expression
func (s *ControlPlaneServer) BatchExecuteCommand(req *pb.BatchExecuteCommandRequest, stream pb.ControlPlaneService_BatchExecuteCommandServer) error {
	// Execute batch command
	responseChan, err := s.batchDispatcher.ExecuteBatch(stream.Context(), req)
	if err != nil {
		return fmt.Errorf("failed to execute batch command: %w", err)
	}

	// Stream responses back to client
	for resp := range responseChan {
		if err := stream.Send(resp); err != nil {
			return fmt.Errorf("failed to send response: %w", err)
		}
	}

	return nil
}

// GetBatchJobStatus retrieves the status of a batch job
func (s *ControlPlaneServer) GetBatchJobStatus(ctx context.Context, req *pb.GetBatchJobStatusRequest) (*pb.GetBatchJobStatusResponse, error) {
	info, err := s.batchDispatcher.GetBatchJobStatus(req.BatchJobId)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch job status: %w", err)
	}

	return &pb.GetBatchJobStatusResponse{
		Job: info,
	}, nil
}

// ListBatchJobs lists batch jobs with optional filtering
func (s *ControlPlaneServer) ListBatchJobs(ctx context.Context, req *pb.ListBatchJobsRequest) (*pb.ListBatchJobsResponse, error) {
	limit := int(req.PageSize)
	if limit == 0 {
		limit = 100 // Default limit
	}

	jobs := s.batchDispatcher.ListBatchJobs(req.Status, limit)

	return &pb.ListBatchJobsResponse{
		Jobs:       jobs,
		TotalCount: int32(len(jobs)),
	}, nil
}

// Helper function to convert agent record to protobuf
func convertAgentRecordToProto(agent *state.AgentRecord) *pb.AgentInfo {
	return &pb.AgentInfo{
		AgentId: agent.ID,
		Metadata: &pb.AgentMetadata{
			Hostname:        agent.Hostname,
			Os:              agent.OS,
			Arch:            agent.Architecture,
			IpAddresses:     agent.IPAddresses,
			PlatformVersion: agent.PlatformVersion,
			AgentVersion:    agent.AgentVersion,
			Labels:          agent.Labels,
		},
		Status:        agent.Status,
		LastHeartbeat: timestamppb.New(agent.LastHeartbeat),
		RegisteredAt:  timestamppb.New(agent.RegisteredAt),
		Metrics: &pb.SystemMetrics{
			CpuPercent:    agent.CPUPercent,
			MemoryPercent: agent.MemoryPercent,
			DiskPercent:   agent.DiskPercent,
			LoadAverage:   agent.LoadAverage,
		},
	}
}
