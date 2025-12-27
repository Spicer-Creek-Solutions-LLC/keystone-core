package client

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// Client wraps the gRPC control plane client
type Client struct {
	conn   *grpc.ClientConn
	client pb.ControlPlaneServiceClient
	ctx    context.Context
}

// New creates a new control plane client
func New(ctx context.Context, address string) (*Client, error) {
	// Create gRPC connection
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to control plane: %w", err)
	}

	return &Client{
		conn:   conn,
		client: pb.NewControlPlaneServiceClient(conn),
		ctx:    ctx,
	}, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// AgentStats represents agent statistics
type AgentStats struct {
	Total     int
	Online    int
	Offline   int
	Degraded  int
	Agents    []*pb.AgentInfo
	Timestamp time.Time
}

// ListAgents retrieves all agents
func (c *Client) ListAgents(ctx context.Context) (*AgentStats, error) {
	req := &pb.ListAgentsRequest{
		PageSize: 1000, // Get all agents
	}

	resp, err := c.client.ListAgents(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	stats := &AgentStats{
		Total:     int(resp.TotalCount),
		Agents:    resp.Agents,
		Timestamp: time.Now(),
	}

	// Count agents by status
	for _, agent := range resp.Agents {
		switch agent.Status {
		case pb.AgentStatus_AGENT_STATUS_ONLINE:
			stats.Online++
		case pb.AgentStatus_AGENT_STATUS_OFFLINE:
			stats.Offline++
		case pb.AgentStatus_AGENT_STATUS_DEGRADED:
			stats.Degraded++
		}
	}

	return stats, nil
}

// GetAgent retrieves a specific agent
func (c *Client) GetAgent(ctx context.Context, agentID string) (*pb.AgentInfo, error) {
	req := &pb.GetAgentRequest{
		AgentId: agentID,
	}

	resp, err := c.client.GetAgent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	return resp.Agent, nil
}

// JobStats represents job execution statistics
type JobStats struct {
	TotalCommands    int
	RunningCommands  int
	CompletedCommands int
	FailedCommands   int
	TotalBatchJobs   int
	RunningBatchJobs int
	Commands         []*pb.CommandInfo
	BatchJobs        []*pb.BatchJobInfo
	Timestamp        time.Time
}

// ListCommands retrieves command execution history
func (c *Client) ListCommands(ctx context.Context, limit int) ([]*pb.CommandInfo, error) {
	req := &pb.ListCommandsRequest{
		PageSize: int32(limit),
	}

	resp, err := c.client.ListCommands(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to list commands: %w", err)
	}

	return resp.Commands, nil
}

// GetJobStats retrieves job execution statistics
func (c *Client) GetJobStats(ctx context.Context) (*JobStats, error) {
	// Get recent commands
	commands, err := c.ListCommands(ctx, 100)
	if err != nil {
		return nil, err
	}

	// Get recent batch jobs
	batchReq := &pb.ListBatchJobsRequest{
		PageSize: 100,
	}
	batchResp, err := c.client.ListBatchJobs(ctx, batchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to list batch jobs: %w", err)
	}

	stats := &JobStats{
		Commands:  commands,
		BatchJobs: batchResp.Jobs,
		Timestamp: time.Now(),
	}

	// Count commands by status
	for _, cmd := range commands {
		stats.TotalCommands++
		switch cmd.Status {
		case pb.CommandStatus_COMMAND_STATUS_RUNNING:
			stats.RunningCommands++
		case pb.CommandStatus_COMMAND_STATUS_COMPLETED:
			stats.CompletedCommands++
		case pb.CommandStatus_COMMAND_STATUS_FAILED, pb.CommandStatus_COMMAND_STATUS_TIMEOUT:
			stats.FailedCommands++
		}
	}

	// Count batch jobs by status
	for _, job := range batchResp.Jobs {
		stats.TotalBatchJobs++
		if job.Status == pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING {
			stats.RunningBatchJobs++
		}
	}

	return stats, nil
}

// GetCommandStatus retrieves the status of a command
func (c *Client) GetCommandStatus(ctx context.Context, commandID string) (*pb.GetCommandStatusResponse, error) {
	req := &pb.GetCommandStatusRequest{
		CommandId: commandID,
	}

	resp, err := c.client.GetCommandStatus(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get command status: %w", err)
	}

	return resp, nil
}

// GetBatchJobStatus retrieves the status of a batch job
func (c *Client) GetBatchJobStatus(ctx context.Context, batchJobID string) (*pb.BatchJobInfo, error) {
	req := &pb.GetBatchJobStatusRequest{
		BatchJobId: batchJobID,
	}

	resp, err := c.client.GetBatchJobStatus(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch job status: %w", err)
	}

	return resp.Job, nil
}

// SystemStats represents system-wide statistics
type SystemStats struct {
	Uptime           time.Duration
	Version          string
	AgentCount       int
	OnlineAgents     int
	RunningJobs      int
	CompletedJobs    int
	FailedJobs       int
	TotalCommands    int
	EventRate        float64
	APIRequestRate   float64
	MemoryUsageMB    float64
	GoroutineCount   int
	Timestamp        time.Time
}

// GetSystemStats retrieves system-wide statistics
func (c *Client) GetSystemStats(ctx context.Context) (*SystemStats, error) {
	// Get agent stats
	agentStats, err := c.ListAgents(ctx)
	if err != nil {
		return nil, err
	}

	// Get job stats
	jobStats, err := c.GetJobStats(ctx)
	if err != nil {
		return nil, err
	}

	stats := &SystemStats{
		AgentCount:     agentStats.Total,
		OnlineAgents:   agentStats.Online,
		RunningJobs:    jobStats.RunningCommands + jobStats.RunningBatchJobs,
		CompletedJobs:  jobStats.CompletedCommands,
		FailedJobs:     jobStats.FailedCommands,
		TotalCommands:  jobStats.TotalCommands,
		Timestamp:      time.Now(),
		Version:        "0.1.0", // TODO: Get from control plane
		Uptime:         0,       // TODO: Get from control plane
		EventRate:      0,       // TODO: Get from metrics
		APIRequestRate: 0,       // TODO: Get from metrics
		MemoryUsageMB:  0,       // TODO: Get from metrics
		GoroutineCount: 0,       // TODO: Get from metrics
	}

	return stats, nil
}
