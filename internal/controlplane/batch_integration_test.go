package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/internal/config"
	natsmgr "github.com/shawnbutts/keystone-core/internal/nats"
	"github.com/shawnbutts/keystone-core/internal/state"
	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
	"google.golang.org/protobuf/proto"
)

// TestBatchExecution_EndToEnd tests the complete batch execution flow
func TestBatchExecution_EndToEnd(t *testing.T) {
	// Create temporary directory for test database
	tmpDir, err := os.MkdirTemp("", "kscore-batch-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Initialize NATS manager with dynamic port
	natsPort := helpers.FreePort(t)
	natsConfig := &config.NATSConfig{
		Mode: config.NATSModeEmbedded,
		Embedded: config.NATSEmbeddedConfig{
			Port:            natsPort,
			EnableJetStream: true,
			StoreDir:        filepath.Join(tmpDir, "nats"),
		},
	}

	natsManager, err := natsmgr.NewManager(natsConfig)
	if err != nil {
		t.Fatalf("Failed to create NATS manager: %v", err)
	}

	if err := natsManager.Start(); err != nil {
		t.Fatalf("Failed to start NATS manager: %v", err)
	}
	defer natsManager.Shutdown()

	// Wait for NATS to be ready
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 100*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("NATS wait did not elapse: %v", err)
	}

	// Initialize state store
	storeConfig := &state.Config{
		Backend:    "sqlite",
		SQLitePath: dbPath,
		SQLiteWAL:  false,
	}
	stateStore, err := state.NewStore(storeConfig)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}
	defer stateStore.Close()

	ctx := context.Background()
	if err := stateStore.Ping(ctx); err != nil {
		t.Fatalf("Failed to ping state store: %v", err)
	}

	// Initialize connection manager with short timeouts for testing
	connMgrConfig := &ConnectionManagerConfig{
		HeartbeatTimeout: 100 * time.Millisecond, // Very short for tests
		StaleThreshold:   2,
		MonitorInterval:  50 * time.Millisecond,
	}
	connMgr := NewConnectionManagerWithConfig(natsManager, connMgrConfig)
	if err := connMgr.Start(); err != nil {
		t.Fatalf("Failed to start connection manager: %v", err)
	}
	defer connMgr.Stop()

	// Wait for connection manager to be ready
	start = time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 100*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("connection manager wait did not elapse: %v", err)
	}

	// Register test agents
	testAgents := []*pb.RegisterRequest{
		{
			AgentId: "test-agent-1",
			Metadata: &pb.AgentMetadata{
				Hostname: "web-server-01",
				Os:       "linux",
				Arch:     "amd64",
				Labels: map[string]string{
					"role": "web",
					"env":  "prod",
				},
			},
		},
		{
			AgentId: "test-agent-2",
			Metadata: &pb.AgentMetadata{
				Hostname: "web-server-02",
				Os:       "linux",
				Arch:     "amd64",
				Labels: map[string]string{
					"role": "web",
					"env":  "prod",
				},
			},
		},
		{
			AgentId: "test-agent-3",
			Metadata: &pb.AgentMetadata{
				Hostname: "db-server-01",
				Os:       "linux",
				Arch:     "amd64",
				Labels: map[string]string{
					"role": "db",
					"env":  "prod",
				},
			},
		},
	}

	// Simulate agent registrations
	for _, agent := range testAgents {
		data, err := proto.Marshal(agent)
		if err != nil {
			t.Fatalf("Failed to marshal agent registration: %v", err)
		}
		_, err = natsManager.PublishRequest("kscore.default.agent.register", data, 1*time.Second)
		if err != nil {
			t.Fatalf("Failed to publish agent registration: %v", err)
		}
	}

	// Wait for agents to be registered
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return connMgr.GetAgentCount() == len(testAgents), nil
	}); err != nil {
		t.Fatalf("agents did not register: %v", err)
	}

	// Verify agents are registered
	agentCount := connMgr.GetAgentCount()
	if agentCount != len(testAgents) {
		t.Fatalf("Expected %d agents, got %d", len(testAgents), agentCount)
	}

	// Initialize command dispatcher
	cmdDispatcher := NewCommandDispatcher(connMgr, stateStore)
	if err := cmdDispatcher.Start(); err != nil {
		t.Fatalf("Failed to start command dispatcher: %v", err)
	}
	defer cmdDispatcher.Stop()

	// Initialize batch dispatcher
	batchDispatcher := NewBatchDispatcher(connMgr, cmdDispatcher, stateStore)

	// Simulate agent command handling by responding on reply subjects.
	subject := connMgr.subjects.AgentCommand("*")
	sub, err := natsManager.Conn().Subscribe(subject, func(msg *nats.Msg) {
		var req pb.ExecuteCommandRequest
		if err := proto.Unmarshal(msg.Data, &req); err != nil {
			t.Logf("Failed to unmarshal command request: %v", err)
			return
		}
		if msg.Reply == "" {
			t.Log("Missing reply subject for command request")
			return
		}

		stdoutResp := &pb.ExecuteCommandResponse{
			CommandId: req.CommandId,
			Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT,
			Data:      []byte("ok\n"),
		}
		completeResp := &pb.ExecuteCommandResponse{
			CommandId: req.CommandId,
			Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED,
			ExitCode:  0,
		}

		for _, resp := range []*pb.ExecuteCommandResponse{stdoutResp, completeResp} {
			data, err := proto.Marshal(resp)
			if err != nil {
				t.Logf("Failed to marshal command response: %v", err)
				return
			}
			if err := natsManager.Conn().Publish(msg.Reply, data); err != nil {
				t.Logf("Failed to publish command response: %v", err)
				return
			}
		}
	})
	if err != nil {
		t.Fatalf("Failed to subscribe for agent commands: %v", err)
	}
	defer sub.Unsubscribe()
	if err := natsManager.Conn().FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("Failed to flush command subscription: %v", err)
	}

	// Execute batch command targeting web servers
	batchReq := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "test-batch-1",
		Target:      "role:web",
		Command:     "echo",
		Args:        []string{"hello", "from", "batch"},
		Concurrency: 2,
		Timeout:     2,
	}

	responseChan, err := batchDispatcher.ExecuteBatch(ctx, batchReq)
	if err != nil {
		t.Fatalf("Failed to execute batch: %v", err)
	}

	// Collect responses
	var responses []*pb.BatchExecuteCommandResponse
	timeout := time.After(5 * time.Second)
	for {
		select {
		case resp, ok := <-responseChan:
			if !ok {
				timeout = nil
				break
			}
			responses = append(responses, resp)
			t.Logf("Received response type: %s", resp.Type)
		case <-timeout:
			t.Fatal("Timed out waiting for batch responses")
		}
		if timeout == nil {
			break
		}
	}

	// Verify we got expected response types
	if len(responses) == 0 {
		t.Fatal("Expected at least one response")
	}

	// First response should be BATCH_START
	if responses[0].Type != pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_START {
		t.Errorf("First response should be BATCH_START, got %s", responses[0].Type)
	}

	// Last response should be BATCH_COMPLETE
	lastResp := responses[len(responses)-1]
	if lastResp.Type != pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE {
		t.Errorf("Last response should be BATCH_COMPLETE, got %s", lastResp.Type)
	}

	// Verify batch summary
	if lastResp.Summary == nil {
		t.Fatal("Expected summary in final response")
	}

	summary := lastResp.Summary
	if summary.Total != 2 {
		t.Errorf("Expected 2 total agents, got %d", summary.Total)
	}

	// Note: In this test environment, agents aren't actually responding to commands
	// so we expect all to be marked as failed (offline or no response)
	// In a real deployment, agents would respond and we'd see successful executions

	// Verify batch job was persisted
	jobInfo, err := batchDispatcher.GetBatchJobStatus(batchReq.BatchJobId)
	if err != nil {
		t.Fatalf("Failed to get batch job status: %v", err)
	}

	if jobInfo.BatchJobId != batchReq.BatchJobId {
		t.Errorf("Expected batch job ID %s, got %s", batchReq.BatchJobId, jobInfo.BatchJobId)
	}

	if jobInfo.Target != batchReq.Target {
		t.Errorf("Expected target %s, got %s", batchReq.Target, jobInfo.Target)
	}

	if jobInfo.Command != batchReq.Command {
		t.Errorf("Expected command %s, got %s", batchReq.Command, jobInfo.Command)
	}

	// Verify job is completed
	if jobInfo.Status != pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED {
		t.Errorf("Expected job status COMPLETED, got %s", jobInfo.Status)
	}

	// Verify progress was tracked
	if jobInfo.Progress == nil {
		t.Fatal("Expected progress information")
	}

	if jobInfo.Progress.Total != 2 {
		t.Errorf("Expected total 2, got %d", jobInfo.Progress.Total)
	}

	// List batch jobs
	jobs := batchDispatcher.ListBatchJobs(pb.BatchJobStatus_BATCH_JOB_STATUS_UNSPECIFIED, 10)
	if len(jobs) != 1 {
		t.Errorf("Expected 1 batch job, got %d", len(jobs))
	}

	// Test filtering by status
	completedJobs := batchDispatcher.ListBatchJobs(pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED, 10)
	if len(completedJobs) != 1 {
		t.Errorf("Expected 1 completed job, got %d", len(completedJobs))
	}

	pendingJobs := batchDispatcher.ListBatchJobs(pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING, 10)
	if len(pendingJobs) != 0 {
		t.Errorf("Expected 0 pending jobs, got %d", len(pendingJobs))
	}
}

// TestBatchExecution_StateRecovery tests that batch jobs can be recovered from database
func TestBatchExecution_StateRecovery(t *testing.T) {
	// Create temporary directory for test database
	tmpDir, err := os.MkdirTemp("", "kscore-batch-recovery-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Initialize state store
	storeConfig := &state.Config{
		Backend:    "sqlite",
		SQLitePath: dbPath,
		SQLiteWAL:  false,
	}
	stateStore, err := state.NewStore(storeConfig)
	if err != nil {
		t.Fatalf("Failed to create state store: %v", err)
	}

	ctx := context.Background()

	// Create a batch job record directly in database
	now := time.Now()
	testJob := &state.BatchJobRecord{
		ID:               "test-job-1",
		Target:           "role:web",
		Command:          "test-command",
		Args:             []string{"arg1", "arg2"},
		Status:           pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED,
		CreatedAt:        now.Add(-5 * time.Minute),
		StartedAt:        timePtr(now.Add(-4 * time.Minute)),
		CompletedAt:      timePtr(now.Add(-1 * time.Minute)),
		TotalAgents:      5,
		CompletedAgents:  5,
		SuccessfulAgents: 4,
		FailedAgents:     1,
		SuccessRate:      80.0,
	}

	if err := stateStore.SaveBatchJob(ctx, testJob); err != nil {
		t.Fatalf("Failed to save batch job: %v", err)
	}

	// Save agent results
	agentResults := []*state.BatchAgentResultRecord{
		{
			BatchJobID: testJob.ID,
			AgentID:    "agent-1",
			Success:    true,
			ExitCode:   0,
			DurationMs: 100,
			CreatedAt:  now,
		},
		{
			BatchJobID: testJob.ID,
			AgentID:    "agent-2",
			Success:    true,
			ExitCode:   0,
			DurationMs: 150,
			CreatedAt:  now,
		},
		{
			BatchJobID: testJob.ID,
			AgentID:    "agent-3",
			Success:    false,
			ExitCode:   1,
			Error:      "command failed",
			DurationMs: 50,
			CreatedAt:  now,
		},
	}

	for _, result := range agentResults {
		if err := stateStore.SaveBatchAgentResult(ctx, testJob.ID, result); err != nil {
			t.Fatalf("Failed to save agent result: %v", err)
		}
	}

	// Close and reopen store to ensure persistence
	stateStore.Close()

	stateStore, err = state.NewStore(storeConfig)
	if err != nil {
		t.Fatalf("Failed to reopen state store: %v", err)
	}
	defer stateStore.Close()

	// Retrieve batch job
	retrievedJob, err := stateStore.GetBatchJob(ctx, testJob.ID)
	if err != nil {
		t.Fatalf("Failed to get batch job: %v", err)
	}

	// Verify all fields
	if retrievedJob.ID != testJob.ID {
		t.Errorf("Expected ID %s, got %s", testJob.ID, retrievedJob.ID)
	}
	if retrievedJob.Target != testJob.Target {
		t.Errorf("Expected target %s, got %s", testJob.Target, retrievedJob.Target)
	}
	if retrievedJob.Command != testJob.Command {
		t.Errorf("Expected command %s, got %s", testJob.Command, retrievedJob.Command)
	}
	if retrievedJob.Status != testJob.Status {
		t.Errorf("Expected status %v, got %v", testJob.Status, retrievedJob.Status)
	}
	if retrievedJob.TotalAgents != testJob.TotalAgents {
		t.Errorf("Expected total agents %d, got %d", testJob.TotalAgents, retrievedJob.TotalAgents)
	}
	if retrievedJob.SuccessfulAgents != testJob.SuccessfulAgents {
		t.Errorf("Expected successful agents %d, got %d", testJob.SuccessfulAgents, retrievedJob.SuccessfulAgents)
	}
	if retrievedJob.FailedAgents != testJob.FailedAgents {
		t.Errorf("Expected failed agents %d, got %d", testJob.FailedAgents, retrievedJob.FailedAgents)
	}

	// List all batch jobs
	filter := &state.BatchJobFilter{
		Limit: 10,
	}
	jobs, err := stateStore.ListBatchJobs(ctx, filter)
	if err != nil {
		t.Fatalf("Failed to list batch jobs: %v", err)
	}

	if len(jobs) != 1 {
		t.Errorf("Expected 1 batch job, got %d", len(jobs))
	}

	// Test filtering by status
	completedFilter := &state.BatchJobFilter{
		Status: statusPtr(pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED),
		Limit:  10,
	}
	completedJobs, err := stateStore.ListBatchJobs(ctx, completedFilter)
	if err != nil {
		t.Fatalf("Failed to list completed jobs: %v", err)
	}

	if len(completedJobs) != 1 {
		t.Errorf("Expected 1 completed job, got %d", len(completedJobs))
	}

	pendingFilter := &state.BatchJobFilter{
		Status: statusPtr(pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING),
		Limit:  10,
	}
	pendingJobs, err := stateStore.ListBatchJobs(ctx, pendingFilter)
	if err != nil {
		t.Fatalf("Failed to list pending jobs: %v", err)
	}

	if len(pendingJobs) != 0 {
		t.Errorf("Expected 0 pending jobs, got %d", len(pendingJobs))
	}
}

// TestBatchExecution_ConcurrencyControl tests that concurrency limits are respected
func TestBatchExecution_ConcurrencyControl(t *testing.T) {
	// This test would require a more sophisticated setup with actual agent responses
	// For now, we'll test that the concurrency parameter is accepted and used
	t.Skip("Concurrency control testing requires live agents - covered by batch_test.go")
}

// Helper functions

func timePtr(t time.Time) *time.Time {
	return &t
}

func statusPtr(s pb.BatchJobStatus) *pb.BatchJobStatus {
	return &s
}
