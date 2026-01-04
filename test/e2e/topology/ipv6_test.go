// Package topology contains E2E tests for different deployment topologies.
// IPv6 tests validate Keystone Core operation in IPv6-only network environments.
package topology

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/test/e2e/harness"
)

var ipv6TestEnv *harness.TestEnvironment

// skipUnlessIPv6Topology skips the test unless running IPv6 topology
func skipUnlessIPv6Topology(t *testing.T) {
	if os.Getenv("KSCORE_TOPOLOGY") != "ipv6" {
		t.Skip("Skipping: IPv6 tests require KSCORE_TOPOLOGY=ipv6")
	}
}

// findIPv6ComposeFile locates the IPv6 docker-compose file
func findIPv6ComposeFile() string {
	candidates := []string{
		"../topologies/ipv6/docker-compose.yml",
		"test/e2e/topologies/ipv6/docker-compose.yml",
		"../../topologies/ipv6/docker-compose.yml",
	}

	// Also try from KSCORE_ROOT if set
	if root := os.Getenv("KSCORE_ROOT"); root != "" {
		candidates = append(candidates, filepath.Join(root, "test/e2e/topologies/ipv6/docker-compose.yml"))
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

// setupIPv6Environment initializes the IPv6 test environment
func setupIPv6Environment(t *testing.T) {
	if ipv6TestEnv != nil {
		return // Already set up
	}

	composeFile := findIPv6ComposeFile()
	if composeFile == "" {
		t.Fatal("Could not find IPv6 docker-compose.yml")
	}

	// Skip building if KSCORE_SKIP_BUILD=1 (images already exist)
	buildImages := os.Getenv("KSCORE_SKIP_BUILD") != "1"

	cfg := &harness.Config{
		ComposeFile:    composeFile,
		ProjectName:    "kscore-e2e-ipv6",
		BuildImages:    buildImages,
		StartupTimeout: 180 * time.Second,
		ServerGRPCPort: 8080,
		ServerHTTPPort: 8081,
	}

	var err error
	ipv6TestEnv, err = harness.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create IPv6 test environment: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := ipv6TestEnv.Start(ctx, cfg); err != nil {
		t.Fatalf("Failed to start IPv6 test environment: %v", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = ipv6TestEnv.Stop(ctx)
		ipv6TestEnv = nil
	})
}

// TestIPv6_AgentRegistration tests that agents register over IPv6
func TestIPv6_AgentRegistration(t *testing.T) {
	skipUnlessIPv6Topology(t)
	setupIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Wait for agents to register
	if err := ipv6TestEnv.WaitForAgents(ctx, 3, 30*time.Second); err != nil {
		t.Fatalf("Failed waiting for agents: %v", err)
	}

	// List agents
	client := ipv6TestEnv.Client()
	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	if len(resp.Agents) != 3 {
		t.Errorf("Expected 3 agents, got %d", len(resp.Agents))
	}

	// Verify agent IDs with IPv6 prefix
	expectedIDs := map[string]bool{
		"agent-ipv6-web-1": false,
		"agent-ipv6-web-2": false,
		"agent-ipv6-db-1":  false,
	}

	for _, agent := range resp.Agents {
		if _, ok := expectedIDs[agent.AgentId]; ok {
			expectedIDs[agent.AgentId] = true
			t.Logf("Found IPv6 agent: %s (status: %s)", agent.AgentId, agent.Status)
		}
	}

	for id, found := range expectedIDs {
		if !found {
			t.Errorf("Expected IPv6 agent %s not found", id)
		}
	}
}

// TestIPv6_AgentHealth tests that IPv6 agents are healthy
func TestIPv6_AgentHealth(t *testing.T) {
	skipUnlessIPv6Topology(t)
	setupIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := ipv6TestEnv.Client()
	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	for _, agent := range resp.Agents {
		if agent.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
			t.Errorf("IPv6 agent %s is not online: %s", agent.AgentId, agent.Status)
		}
	}
}

// TestIPv6_AgentMetadataNetworkLabel tests that agents have IPv6 network label
func TestIPv6_AgentMetadataNetworkLabel(t *testing.T) {
	skipUnlessIPv6Topology(t)
	setupIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := ipv6TestEnv.Client()
	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	for _, agent := range resp.Agents {
		// Check for network=ipv6 label
		networkLabel, hasLabel := agent.Metadata.Labels["network"]
		if !hasLabel {
			t.Errorf("IPv6 agent %s missing 'network' label", agent.AgentId)
			continue
		}
		if networkLabel != "ipv6" {
			t.Errorf("IPv6 agent %s has wrong network label: expected 'ipv6', got '%s'",
				agent.AgentId, networkLabel)
		}
		t.Logf("Agent %s has network label: %s", agent.AgentId, networkLabel)
	}
}

// TestIPv6_SingleAgentCommand tests executing a command on a single IPv6 agent
func TestIPv6_SingleAgentCommand(t *testing.T) {
	skipUnlessIPv6Topology(t)
	setupIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute command on a specific IPv6 agent
	result, err := ipv6TestEnv.ExecuteCommandAndWait(ctx, "agent-ipv6-web-1", "echo", "hello", "from", "ipv6", "test")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	expectedOutput := "hello from ipv6 test"
	if !strings.Contains(result.Stdout, expectedOutput) {
		t.Errorf("Expected output to contain '%s', got: %s", expectedOutput, result.Stdout)
	}

	t.Logf("Command output: %s", result.Stdout)
}

// TestIPv6_NetworkConnectivity tests IPv6 network connectivity between components
func TestIPv6_NetworkConnectivity(t *testing.T) {
	skipUnlessIPv6Topology(t)
	setupIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test IPv6 connectivity: ping the server from an agent
	// Note: The server is at fd00:1::10 in the IPv6 topology
	result, err := ipv6TestEnv.ExecuteCommandAndWait(ctx, "agent-ipv6-web-1", "ping6", "-c", "1", "-W", "5", "fd00:1::10")
	if err != nil {
		// ping6 might not be available, try ping with -6 flag
		result, err = ipv6TestEnv.ExecuteCommandAndWait(ctx, "agent-ipv6-web-1", "ping", "-6", "-c", "1", "-W", "5", "fd00:1::10")
		if err != nil {
			t.Logf("IPv6 ping not available or failed: %v", err)
			t.Skip("Skipping IPv6 connectivity test: ping6 not available")
		}
	}

	if result.ExitCode != 0 {
		t.Errorf("IPv6 ping failed with exit code %d: %s", result.ExitCode, result.Stderr)
	} else {
		t.Logf("IPv6 connectivity verified: %s", result.Stdout)
	}
}

// TestIPv6_BatchCommand tests batch command execution across IPv6 agents
func TestIPv6_BatchCommand(t *testing.T) {
	skipUnlessIPv6Topology(t)
	setupIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := ipv6TestEnv.Client()

	// Execute batch command targeting all IPv6 agents
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "e2e-ipv6-batch-test-1",
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
		t.Logf("IPv6 batch response: type=%s", resp.Type)
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
		t.Errorf("Expected 3 total IPv6 agents, got %d", summary.Total)
	}

	if summary.Successful != 3 {
		t.Errorf("Expected 3 successful executions, got %d", summary.Successful)
	}

	if summary.Failed != 0 {
		t.Errorf("Expected 0 failed executions, got %d", summary.Failed)
	}

	t.Logf("IPv6 batch completed: %d/%d successful (%.1f%%)",
		summary.Successful, summary.Total, summary.SuccessRate)
}

// TestIPv6_TargetedBatchCommand tests targeting specific IPv6 agents by role
func TestIPv6_TargetedBatchCommand(t *testing.T) {
	skipUnlessIPv6Topology(t)
	setupIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := ipv6TestEnv.Client()

	// Target only web agents using label selector
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "e2e-ipv6-batch-web-only",
		Target:      "labels.role:webserver",
		Command:     "echo",
		Args:        []string{"web server on ipv6"},
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

	// Should only target web-1 and web-2 (role: webserver)
	if summary.Total != 2 {
		t.Errorf("Expected 2 web agents with IPv6, got %d", summary.Total)
	}

	if summary.Successful != 2 {
		t.Errorf("Expected 2 successful executions, got %d", summary.Successful)
	}
}

// TestIPv6_TargetByNetworkLabel tests targeting agents by network=ipv6 label
func TestIPv6_TargetByNetworkLabel(t *testing.T) {
	skipUnlessIPv6Topology(t)
	setupIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := ipv6TestEnv.Client()

	// Target agents with network=ipv6 label
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "e2e-ipv6-network-label",
		Target:      "labels.network:ipv6",
		Command:     "echo",
		Args:        []string{"running on ipv6 network"},
		Concurrency: 3,
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

	// All 3 agents should have network=ipv6 label
	if summary.Total != 3 {
		t.Errorf("Expected 3 agents with network=ipv6, got %d", summary.Total)
	}

	if summary.Successful != 3 {
		t.Errorf("Expected 3 successful executions, got %d", summary.Successful)
	}

	t.Logf("Successfully targeted %d agents by network=ipv6 label", summary.Total)
}

// TestIPv6_GetBatchJobStatus tests retrieving IPv6 batch job status
func TestIPv6_GetBatchJobStatus(t *testing.T) {
	skipUnlessIPv6Topology(t)
	setupIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := ipv6TestEnv.Client()

	// Get status of previously executed IPv6 batch job
	req := &pb.GetBatchJobStatusRequest{
		BatchJobId: "e2e-ipv6-batch-test-1",
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
		t.Logf("IPv6 Job %s: status=%s, total=%d, successful=%d",
			resp.Job.BatchJobId, resp.Job.Status,
			resp.Job.Summary.Total, resp.Job.Summary.Successful)
	} else {
		t.Logf("IPv6 Job %s: status=%s (no summary available)",
			resp.Job.BatchJobId, resp.Job.Status)
	}
}

// TestIPv6_ListBatchJobs tests listing IPv6 batch jobs
func TestIPv6_ListBatchJobs(t *testing.T) {
	skipUnlessIPv6Topology(t)
	setupIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := ipv6TestEnv.Client()

	req := &pb.ListBatchJobsRequest{
		PageSize: 10,
	}

	resp, err := client.ListBatchJobs(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list batch jobs: %v", err)
	}

	// Count IPv6-related jobs
	ipv6Jobs := 0
	for _, job := range resp.Jobs {
		if strings.Contains(job.BatchJobId, "ipv6") {
			ipv6Jobs++
			t.Logf("IPv6 Job: %s (status: %s)", job.BatchJobId, job.Status)
		}
	}

	// Should have at least the IPv6 jobs we created
	if ipv6Jobs < 3 {
		t.Errorf("Expected at least 3 IPv6 batch jobs, got %d", ipv6Jobs)
	}
}
