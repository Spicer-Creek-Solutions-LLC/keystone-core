// Package topology contains E2E tests for different deployment topologies.
// These tests require Docker/Podman to be running.
package topology

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/test/e2e/harness"
)

var testEnv *harness.TestEnvironment

// TestMain sets up and tears down the test environment
func TestMain(m *testing.M) {
	// Skip if not running E2E tests
	if os.Getenv("KSCORE_E2E_TESTS") != "1" {
		os.Exit(0)
	}

	// Check which topology we're testing
	topology := os.Getenv("KSCORE_TOPOLOGY")

	// If HA cluster topology, don't set up all-in-one environment
	// HA cluster tests manage their own environment via setupHACluster()
	if topology == "ha-cluster" {
		code := m.Run()
		os.Exit(code)
	}

	// Default: all-in-one topology
	// Find compose file
	composeFile := findComposeFile()
	if composeFile == "" {
		panic("could not find docker-compose.yml")
	}

	// Skip building if KSCORE_SKIP_BUILD=1 (images already exist)
	buildImages := os.Getenv("KSCORE_SKIP_BUILD") != "1"

	cfg := &harness.Config{
		ComposeFile:    composeFile,
		ProjectName:    "kscore-e2e-allinone",
		BuildImages:    buildImages,
		StartupTimeout: 180 * time.Second,
		ServerGRPCPort: 8080,
		ServerHTTPPort: 8081,
	}

	var err error
	testEnv, err = harness.New(cfg)
	if err != nil {
		panic("failed to create test environment: " + err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := testEnv.Start(ctx, cfg); err != nil {
		panic("failed to start test environment: " + err.Error())
	}

	// Run tests
	code := m.Run()

	// Cleanup
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = testEnv.Stop(ctx)

	os.Exit(code)
}

// skipIfHACluster skips the test if running HA cluster topology
// All-in-one tests should skip when KSCORE_TOPOLOGY=ha-cluster
func skipIfHACluster(t *testing.T) {
	if os.Getenv("KSCORE_TOPOLOGY") == "ha-cluster" {
		t.Skip("Skipping: all-in-one tests are not run in HA cluster mode")
	}
}

func findComposeFile() string {
	// Try various relative paths
	candidates := []string{
		"../containers/docker-compose.yml",
		"test/e2e/containers/docker-compose.yml",
		"../../containers/docker-compose.yml",
	}

	// Also try from KSCORE_ROOT if set
	if root := os.Getenv("KSCORE_ROOT"); root != "" {
		candidates = append(candidates, filepath.Join(root, "test/e2e/containers/docker-compose.yml"))
	}

	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}

	return ""
}

// TestAllInOne_AgentRegistration tests that agents register with the control plane
func TestAllInOne_AgentRegistration(t *testing.T) {
	skipIfHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Wait for agents to register
	if err := testEnv.WaitForAgents(ctx, 3, 30*time.Second); err != nil {
		t.Fatalf("Failed waiting for agents: %v", err)
	}

	// List agents
	client := testEnv.Client()
	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	if len(resp.Agents) != 3 {
		t.Errorf("Expected 3 agents, got %d", len(resp.Agents))
	}

	// Verify agent IDs
	expectedIDs := map[string]bool{
		"agent-web-1": false,
		"agent-web-2": false,
		"agent-db-1":  false,
	}

	for _, agent := range resp.Agents {
		if _, ok := expectedIDs[agent.AgentId]; ok {
			expectedIDs[agent.AgentId] = true
			t.Logf("Found agent: %s (status: %s)", agent.AgentId, agent.Status)
		}
	}

	for id, found := range expectedIDs {
		if !found {
			t.Errorf("Expected agent %s not found", id)
		}
	}
}

// TestAllInOne_AgentHealth tests that agents are healthy
func TestAllInOne_AgentHealth(t *testing.T) {
	skipIfHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := testEnv.Client()
	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	for _, agent := range resp.Agents {
		if agent.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
			t.Errorf("Agent %s is not online: %s", agent.AgentId, agent.Status)
		}
	}
}

// TestAllInOne_SingleAgentCommand tests executing a command on a single agent
func TestAllInOne_SingleAgentCommand(t *testing.T) {
	skipIfHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute command on a specific agent using the harness helper
	result, err := testEnv.ExecuteCommandAndWait(ctx, "agent-web-1", "echo", "hello", "from", "e2e", "test")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	t.Logf("Command output: %s", result.Stdout)
}

// TestAllInOne_BatchCommand tests batch command execution across multiple agents
func TestAllInOne_BatchCommand(t *testing.T) {
	skipIfHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Execute batch command targeting all agents
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "e2e-batch-test-1",
		Target:      "*", // All agents
		Command:     "hostname",
		Concurrency: 3,
	}

	stream, err := client.BatchExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute batch command: %v", err)
	}

	var responses []*pb.BatchExecuteCommandResponse
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Stream error: %v", err)
		}
		responses = append(responses, resp)
		t.Logf("Batch response: type=%s", resp.Type)
	}

	// Find the summary response
	var summary *pb.BatchSummary
	for _, resp := range responses {
		if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE {
			summary = resp.Summary
			break
		}
	}

	if summary == nil {
		t.Fatal("No batch summary received")
	}

	if summary.Total != 3 {
		t.Errorf("Expected 3 total agents, got %d", summary.Total)
	}

	if summary.Successful != 3 {
		t.Errorf("Expected 3 successful executions, got %d", summary.Successful)
	}

	if summary.Failed != 0 {
		t.Errorf("Expected 0 failed executions, got %d", summary.Failed)
	}

	t.Logf("Batch completed: %d/%d successful (%.1f%%)",
		summary.Successful, summary.Total, summary.SuccessRate)
}

// TestAllInOne_TargetedBatchCommand tests targeting specific agents
func TestAllInOne_TargetedBatchCommand(t *testing.T) {
	skipIfHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Target only web agents using glob pattern on ID field
	// Targeting syntax requires field:pattern format (e.g., id:agent-web-*)
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "e2e-batch-web-only",
		Target:      "id:agent-web-*",
		Command:     "echo",
		Args:        []string{"web server reporting"},
		Concurrency: 2,
	}

	stream, err := client.BatchExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute batch command: %v", err)
	}

	var summary *pb.BatchSummary
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Stream error: %v", err)
		}
		if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE {
			summary = resp.Summary
		}
	}

	if summary == nil {
		t.Fatal("No batch summary received")
	}

	// Should only target web-1 and web-2
	if summary.Total != 2 {
		t.Errorf("Expected 2 web agents, got %d", summary.Total)
	}
}

// TestAllInOne_GetBatchJobStatus tests retrieving batch job status
func TestAllInOne_GetBatchJobStatus(t *testing.T) {
	skipIfHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Get status of previously executed batch job
	req := &pb.GetBatchJobStatusRequest{
		BatchJobId: "e2e-batch-test-1",
	}

	resp, err := client.GetBatchJobStatus(ctx, req)
	if err != nil {
		t.Fatalf("Failed to get batch job status: %v", err)
	}

	if resp.Job == nil {
		t.Fatal("No job returned")
	}

	if resp.Job.Status != pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED {
		t.Errorf("Expected job status COMPLETED, got %s", resp.Job.Status)
	}

	if resp.Job.Summary != nil {
		t.Logf("Job %s: status=%s, total=%d, successful=%d",
			resp.Job.BatchJobId, resp.Job.Status,
			resp.Job.Summary.Total, resp.Job.Summary.Successful)
	} else {
		t.Logf("Job %s: status=%s (no summary available)",
			resp.Job.BatchJobId, resp.Job.Status)
	}
}

// TestAllInOne_ListBatchJobs tests listing batch jobs
func TestAllInOne_ListBatchJobs(t *testing.T) {
	skipIfHACluster(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := testEnv.Client()

	req := &pb.ListBatchJobsRequest{
		PageSize: 10,
	}

	resp, err := client.ListBatchJobs(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list batch jobs: %v", err)
	}

	// Should have at least the jobs we created
	if len(resp.Jobs) < 2 {
		t.Errorf("Expected at least 2 batch jobs, got %d", len(resp.Jobs))
	}

	for _, job := range resp.Jobs {
		t.Logf("Job: %s (status: %s)", job.BatchJobId, job.Status)
	}
}
