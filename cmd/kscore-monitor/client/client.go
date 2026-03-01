// Package client provides a gRPC client wrapper for the control plane service.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// Client wraps the gRPC control plane client
type Client struct {
	conn          *grpc.ClientConn
	client        pb.ControlPlaneServiceClient
	stateClient   pb.StateServiceClient
	policyClient  pb.PolicyServiceClient
	eventClient   pb.EventServiceClient
	clusterClient pb.ClusterServiceClient
	secretsClient pb.SecretsServiceClient
	ctx           context.Context
	httpAddress   string // HTTP address for status endpoint
}

// New creates a new control plane client
func New(ctx context.Context, address string) (*Client, error) {
	// Create gRPC connection
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),                //nolint:staticcheck // SA1019: grpc.WithBlock is deprecated but supported throughout gRPC 1.x; migration to NewClient requires significant refactoring
		grpc.WithTimeout(5*time.Second), //nolint:staticcheck // SA1019: grpc.WithTimeout is deprecated but supported throughout gRPC 1.x; migration to NewClient requires significant refactoring
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to control plane: %w", err)
	}

	// Derive HTTP address from gRPC address
	// gRPC typically runs on :9090, HTTP on :8080
	httpAddress := deriveHTTPAddress(address)

	return &Client{
		conn:          conn,
		client:        pb.NewControlPlaneServiceClient(conn),
		stateClient:   pb.NewStateServiceClient(conn),
		policyClient:  pb.NewPolicyServiceClient(conn),
		eventClient:   pb.NewEventServiceClient(conn),
		clusterClient: pb.NewClusterServiceClient(conn),
		secretsClient: pb.NewSecretsServiceClient(conn),
		ctx:           ctx,
		httpAddress:   httpAddress,
	}, nil
}

// deriveHTTPAddress converts a gRPC address to the HTTP address
func deriveHTTPAddress(grpcAddress string) string {
	// Default HTTP port
	httpPort := "8080"

	// Extract host from gRPC address
	host := grpcAddress
	if idx := strings.LastIndex(grpcAddress, ":"); idx != -1 {
		host = grpcAddress[:idx]
	}

	// Handle localhost/empty host
	if host == "" || host == "localhost" {
		host = "localhost"
	}

	return fmt.Sprintf("http://%s:%s", host, httpPort)
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
		default:
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
	TotalCommands     int
	RunningCommands   int
	CompletedCommands int
	FailedCommands    int
	TotalBatchJobs    int
	RunningBatchJobs  int
	Commands          []*pb.CommandInfo
	BatchJobs         []*pb.BatchJobInfo
	Timestamp         time.Time
}

// ListCommands retrieves command execution history
func (c *Client) ListCommands(ctx context.Context, limit int) ([]*pb.CommandInfo, error) {
	req := &pb.ListCommandsRequest{
		//nolint:gosec // G115: limit is a small user-provided page size, fits in int32
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
		default:
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
	Uptime         time.Duration
	Version        string
	AgentCount     int
	OnlineAgents   int
	RunningJobs    int
	CompletedJobs  int
	FailedJobs     int
	TotalCommands  int
	EventRate      float64
	APIRequestRate float64
	MemoryUsageMB  float64
	GoroutineCount int
	Timestamp      time.Time
}

// serverStatusResponse represents the JSON response from /api/status
type serverStatusResponse struct {
	Version       string `json:"version"`
	GitCommit     string `json:"git_commit"`
	BuildDate     string `json:"build_date"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	StartedAt     string `json:"started_at"`
	Agents        struct {
		Total   int `json:"total"`
		Online  int `json:"online"`
		Offline int `json:"offline"`
	} `json:"agents"`
	Runtime struct {
		Goroutines    int     `json:"goroutines"`
		MemoryAllocMB float64 `json:"memory_alloc_mb"`
		MemorySysMB   float64 `json:"memory_sys_mb"`
		GCRuns        uint32  `json:"gc_runs"`
	} `json:"runtime"`
	Health string `json:"health"`
}

// fetchServerStatus fetches server status from the HTTP endpoint
func (c *Client) fetchServerStatus(ctx context.Context) (*serverStatusResponse, error) {
	url := c.httpAddress + "/api/status"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch server status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var status serverStatusResponse
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &status, nil
}

// Conn returns the underlying gRPC connection
func (c *Client) Conn() *grpc.ClientConn {
	return c.conn
}

// GetStateHistory retrieves state run history
func (c *Client) GetStateHistory(ctx context.Context) (*pb.GetStateHistoryResponse, error) {
	resp, err := c.stateClient.GetStateHistory(ctx, &pb.GetStateHistoryRequest{
		PageSize: 50,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get state history: %w", err)
	}
	return resp, nil
}

// GetStateStatus retrieves current state status for an agent
func (c *Client) GetStateStatus(ctx context.Context) (*pb.GetStateStatusResponse, error) {
	resp, err := c.stateClient.GetStateStatus(ctx, &pb.GetStateStatusRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get state status: %w", err)
	}
	return resp, nil
}

// ListViolations retrieves policy violations
func (c *Client) ListViolations(ctx context.Context) (*pb.ListViolationsResponse, error) {
	resp, err := c.policyClient.ListViolations(ctx, &pb.ListViolationsRequest{
		PageSize: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list violations: %w", err)
	}
	return resp, nil
}

// GetComplianceReport retrieves the compliance report
func (c *Client) GetComplianceReport(ctx context.Context) (*pb.GetComplianceReportResponse, error) {
	resp, err := c.policyClient.GetComplianceReport(ctx, &pb.GetComplianceReportRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get compliance report: %w", err)
	}
	return resp, nil
}

// GetEventStats retrieves event statistics
func (c *Client) GetEventStats(ctx context.Context) (*pb.GetEventStatsResponse, error) {
	resp, err := c.eventClient.GetEventStats(ctx, &pb.GetEventStatsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get event stats: %w", err)
	}
	return resp, nil
}

// GetClusterStatus retrieves the cluster status
func (c *Client) GetClusterStatus(ctx context.Context) (*pb.GetClusterStatusResponse, error) {
	resp, err := c.clusterClient.GetClusterStatus(ctx, &pb.GetClusterStatusRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster status: %w", err)
	}
	return resp, nil
}

// ListMembers retrieves cluster members
func (c *Client) ListMembers(ctx context.Context) (*pb.ListMembersResponse, error) {
	resp, err := c.clusterClient.ListMembers(ctx, &pb.ListMembersRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}
	return resp, nil
}

// GetLeader retrieves the current cluster leader
func (c *Client) GetLeader(ctx context.Context) (*pb.GetClusterLeaderResponse, error) {
	resp, err := c.clusterClient.GetLeader(ctx, &pb.GetClusterLeaderRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get leader: %w", err)
	}
	return resp, nil
}

// ListLeases retrieves secret leases
func (c *Client) ListLeases(ctx context.Context) (*pb.ListLeasesResponse, error) {
	resp, err := c.secretsClient.ListLeases(ctx, &pb.ListLeasesRequest{
		PageSize: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list leases: %w", err)
	}
	return resp, nil
}

// ListSecrets retrieves secret keys
func (c *Client) ListSecrets(ctx context.Context) (*pb.ListSecretsResponse, error) {
	resp, err := c.secretsClient.ListSecrets(ctx, &pb.ListSecretsRequest{
		PageSize: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}
	return resp, nil
}

// doJSON performs an HTTP request and decodes the JSON response
func (c *Client) doJSON(ctx context.Context, method, path string, result interface{}) error {
	url := c.httpAddress + path
	req, err := http.NewRequestWithContext(ctx, method, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return nil
}

// ScheduleResponse represents a schedule from the REST API
type ScheduleResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CronExpr  string `json:"cron_expression"`
	Status    string `json:"status"`
	LastRun   string `json:"last_run"`
	NextRun   string `json:"next_run"`
	CreatedAt string `json:"created_at"`
}

// MaintenanceWindowResponse represents a maintenance window
type MaintenanceWindowResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Scope     string `json:"scope"`
	Active    bool   `json:"active"`
}

// RunbookResponse represents a runbook
type RunbookResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

// RunbookExecutionResponse represents a runbook execution
type RunbookExecutionResponse struct {
	ID         string `json:"id"`
	RunbookID  string `json:"runbook_id"`
	RunbookNm  string `json:"runbook_name"`
	Status     string `json:"status"`
	Step       string `json:"current_step"`
	StartedAt  string `json:"started_at"`
	Duration   string `json:"duration"`
	Requester  string `json:"requester"`
}

// ApprovalResponse represents a pending approval
type ApprovalResponse struct {
	ID          string `json:"id"`
	RunbookID   string `json:"runbook_id"`
	RunbookName string `json:"runbook_name"`
	Requester   string `json:"requester"`
	RequestedAt string `json:"requested_at"`
	Status      string `json:"status"`
}

// WebhookSubscriptionResponse represents an outbound webhook subscription
type WebhookSubscriptionResponse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Status    string   `json:"status"`
	CreatedAt string   `json:"created_at"`
}

// WebhookDeliveryResponse represents a webhook delivery attempt
type WebhookDeliveryResponse struct {
	ID             string `json:"id"`
	SubscriptionID string `json:"subscription_id"`
	EventType      string `json:"event_type"`
	StatusCode     int    `json:"status_code"`
	Success        bool   `json:"success"`
	Attempt        int    `json:"attempt"`
	DeliveredAt    string `json:"delivered_at"`
}

// ListSchedules retrieves schedules from the REST API
func (c *Client) ListSchedules(ctx context.Context) ([]ScheduleResponse, error) {
	var resp struct {
		Schedules []ScheduleResponse `json:"schedules"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/schedules", &resp); err != nil {
		return nil, err
	}
	return resp.Schedules, nil
}

// GetActiveMaintenanceWindows retrieves active maintenance windows
func (c *Client) GetActiveMaintenanceWindows(ctx context.Context) ([]MaintenanceWindowResponse, error) {
	var resp struct {
		Windows []MaintenanceWindowResponse `json:"windows"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/maintenance/windows", &resp); err != nil {
		return nil, err
	}
	return resp.Windows, nil
}

// ListRunbooks retrieves runbooks from the REST API
func (c *Client) ListRunbooks(ctx context.Context) ([]RunbookResponse, error) {
	var resp struct {
		Runbooks []RunbookResponse `json:"runbooks"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/runbooks", &resp); err != nil {
		return nil, err
	}
	return resp.Runbooks, nil
}

// ListRunbookExecutions retrieves recent runbook executions
func (c *Client) ListRunbookExecutions(ctx context.Context) ([]RunbookExecutionResponse, error) {
	var resp struct {
		Executions []RunbookExecutionResponse `json:"executions"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/runbooks/executions", &resp); err != nil {
		return nil, err
	}
	return resp.Executions, nil
}

// ListPendingApprovals retrieves pending runbook approvals
func (c *Client) ListPendingApprovals(ctx context.Context) ([]ApprovalResponse, error) {
	var resp struct {
		Approvals []ApprovalResponse `json:"approvals"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/runbook/approvals", &resp); err != nil {
		return nil, err
	}
	return resp.Approvals, nil
}

// ListWebhookSubscriptions retrieves outbound webhook subscriptions
func (c *Client) ListWebhookSubscriptions(ctx context.Context) ([]WebhookSubscriptionResponse, error) {
	var resp struct {
		Subscriptions []WebhookSubscriptionResponse `json:"subscriptions"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/webhooks/subscriptions", &resp); err != nil {
		return nil, err
	}
	return resp.Subscriptions, nil
}

// GetWebhookDeliveries retrieves deliveries for a webhook subscription
func (c *Client) GetWebhookDeliveries(ctx context.Context, subscriptionID string) ([]WebhookDeliveryResponse, error) {
	var resp struct {
		Deliveries []WebhookDeliveryResponse `json:"deliveries"`
	}
	path := fmt.Sprintf("/api/v1/webhooks/subscriptions/%s/deliveries", subscriptionID)
	if err := c.doJSON(ctx, http.MethodGet, path, &resp); err != nil {
		return nil, err
	}
	return resp.Deliveries, nil
}

// ListEventsByCorrelation retrieves events filtered by correlation ID
func (c *Client) ListEventsByCorrelation(ctx context.Context, correlationID string) (*pb.ListEventsResponse, error) {
	resp, err := c.eventClient.ListEvents(ctx, &pb.ListEventsRequest{
		CorrelationId: correlationID,
		PageSize:      100,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list events by correlation: %w", err)
	}
	return resp, nil
}

// GetAgentStateHistory retrieves state history for a specific agent
func (c *Client) GetAgentStateHistory(ctx context.Context, agentID string) (*pb.GetStateHistoryResponse, error) {
	resp, err := c.stateClient.GetStateHistory(ctx, &pb.GetStateHistoryRequest{
		AgentId:  agentID,
		PageSize: 20,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get agent state history: %w", err)
	}
	return resp, nil
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
		AgentCount:    agentStats.Total,
		OnlineAgents:  agentStats.Online,
		RunningJobs:   jobStats.RunningCommands + jobStats.RunningBatchJobs,
		CompletedJobs: jobStats.CompletedCommands,
		FailedJobs:    jobStats.FailedCommands,
		TotalCommands: jobStats.TotalCommands,
		Timestamp:     time.Now(),
	}

	// Fetch real server status from HTTP endpoint
	serverStatus, err := c.fetchServerStatus(ctx)
	if err != nil {
		// Fall back to default values if HTTP call fails
		stats.Version = "unknown"
		stats.Uptime = 0
		stats.EventRate = 0
		stats.APIRequestRate = 0
		stats.MemoryUsageMB = 0
		stats.GoroutineCount = 0
	} else {
		// Use real values from the server
		stats.Version = serverStatus.Version
		stats.Uptime = time.Duration(serverStatus.UptimeSeconds) * time.Second
		stats.MemoryUsageMB = serverStatus.Runtime.MemoryAllocMB
		stats.GoroutineCount = serverStatus.Runtime.Goroutines
		// Note: EventRate and APIRequestRate require metrics aggregation
		// which is not yet implemented on the server side
		stats.EventRate = 0
		stats.APIRequestRate = 0
	}

	return stats, nil
}
