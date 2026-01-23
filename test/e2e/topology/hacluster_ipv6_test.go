// Package topology contains E2E tests for different deployment topologies.
// HA Cluster IPv6 tests validate Keystone Core HA clustering operation over IPv6-only networks.
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

var haIPv6TestEnv *harness.HAClusterEnvironment

// skipUnlessHAClusterIPv6Topology skips the test unless running HA cluster IPv6 topology
func skipUnlessHAClusterIPv6Topology(t *testing.T) {
	if os.Getenv("KSCORE_TOPOLOGY") != "ha-cluster-ipv6" {
		t.Skip("Skipping: HA cluster IPv6 tests require KSCORE_TOPOLOGY=ha-cluster-ipv6")
	}
}

// findHAClusterIPv6ComposeFile locates the HA cluster IPv6 docker-compose file
func findHAClusterIPv6ComposeFile() string {
	candidates := []string{
		"../topologies/ha-cluster-ipv6/docker-compose.yml",
		"test/e2e/topologies/ha-cluster-ipv6/docker-compose.yml",
		"../../topologies/ha-cluster-ipv6/docker-compose.yml",
	}

	// Also try from KSCORE_ROOT if set
	if root := os.Getenv("KSCORE_ROOT"); root != "" {
		candidates = append(candidates, filepath.Join(root, "test/e2e/topologies/ha-cluster-ipv6/docker-compose.yml"))
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

// setupHAClusterIPv6Environment initializes the HA cluster IPv6 test environment
func setupHAClusterIPv6Environment(t *testing.T) {
	if haIPv6TestEnv != nil {
		return // Already set up
	}

	var cfg *harness.HAClusterConfig
	if harness.IsVMMode() {
		vmCfg, _, _, err := harness.HAClusterConfigFromVM("", 5)
		if err != nil {
			t.Fatalf("Failed to load VM config: %v", err)
		}
		cfg = vmCfg
		cfg.ProjectName = "kscore-e2e-ha-ipv6"
		cfg.StartupTimeout = 300 * time.Second
	} else {
		composeFile := findHAClusterIPv6ComposeFile()
		if composeFile == "" {
			t.Fatal("Could not find HA cluster IPv6 docker-compose.yml")
		}

		// Skip building if KSCORE_SKIP_BUILD=1 (images already exist)
		buildImages := os.Getenv("KSCORE_SKIP_BUILD") != "1"

		cfg = &harness.HAClusterConfig{
			ComposeFile:    composeFile,
			ProjectName:    "kscore-e2e-ha-ipv6",
			BuildImages:    buildImages,
			StartupTimeout: 300 * time.Second, // HA cluster takes longer to start
			Servers: []harness.ServerInfo{
				{Name: "server-1", GRPCAddr: "localhost:8080", HTTPAddr: "http://localhost:8081"},
				{Name: "server-2", GRPCAddr: "localhost:8082", HTTPAddr: "http://localhost:8083"},
				{Name: "server-3", GRPCAddr: "localhost:8084", HTTPAddr: "http://localhost:8085"},
			},
			ExpectedAgents: 5,
		}
	}

	var err error
	haIPv6TestEnv, err = harness.NewHACluster(cfg)
	if err != nil {
		t.Fatalf("Failed to create HA cluster IPv6 test environment: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := haIPv6TestEnv.Start(ctx, cfg); err != nil {
		t.Fatalf("Failed to start HA cluster IPv6 test environment: %v", err)
	}

	// Wait for agents
	if err := haIPv6TestEnv.WaitForAgents(ctx, 5, 120*time.Second); err != nil {
		t.Fatalf("Failed waiting for agents: %v", err)
	}

	// Register cleanup
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		_ = haIPv6TestEnv.Stop(ctx)
		haIPv6TestEnv = nil
	})
}

// TestHAClusterIPv6_ClusterFormation tests that the HA cluster forms over IPv6
func TestHAClusterIPv6_ClusterFormation(t *testing.T) {
	skipUnlessHAClusterIPv6Topology(t)
	setupHAClusterIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// List agents to verify cluster is operational
	client := haIPv6TestEnv.Client()
	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	if len(resp.Agents) != 5 {
		t.Errorf("Expected 5 agents in HA IPv6 cluster, got %d", len(resp.Agents))
	}

	// Verify all agents have correct prefix
	for _, agent := range resp.Agents {
		if !strings.HasPrefix(agent.AgentId, "agent-ha-ipv6-") {
			t.Errorf("Unexpected agent ID format: %s (expected agent-ha-ipv6-*)", agent.AgentId)
		}
		t.Logf("Found HA IPv6 agent: %s (status: %s)", agent.AgentId, agent.Status)
	}
}

// TestHAClusterIPv6_AgentHealth tests that all agents in the HA IPv6 cluster are healthy
func TestHAClusterIPv6_AgentHealth(t *testing.T) {
	skipUnlessHAClusterIPv6Topology(t)
	setupHAClusterIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := haIPv6TestEnv.Client()
	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	for _, agent := range resp.Agents {
		if agent.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
			t.Errorf("HA IPv6 agent %s is not online: %s", agent.AgentId, agent.Status)
		}
	}
}

// TestHAClusterIPv6_AgentNetworkLabel tests that agents have correct network labels
func TestHAClusterIPv6_AgentNetworkLabel(t *testing.T) {
	skipUnlessHAClusterIPv6Topology(t)
	setupHAClusterIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := haIPv6TestEnv.Client()
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
			t.Errorf("HA IPv6 agent %s missing 'network' label", agent.AgentId)
			continue
		}
		if networkLabel != "ipv6" {
			t.Errorf("HA IPv6 agent %s has wrong network label: expected 'ipv6', got '%s'",
				agent.AgentId, networkLabel)
		}

		// Check for topology=ha-cluster label
		topologyLabel, hasLabel := agent.Metadata.Labels["topology"]
		if !hasLabel {
			t.Errorf("HA IPv6 agent %s missing 'topology' label", agent.AgentId)
			continue
		}
		if topologyLabel != "ha-cluster" {
			t.Errorf("HA IPv6 agent %s has wrong topology label: expected 'ha-cluster', got '%s'",
				agent.AgentId, topologyLabel)
		}

		t.Logf("Agent %s has labels: network=%s, topology=%s", agent.AgentId, networkLabel, topologyLabel)
	}
}

// TestHAClusterIPv6_SingleAgentCommand tests executing a command on a single agent in the HA IPv6 cluster
func TestHAClusterIPv6_SingleAgentCommand(t *testing.T) {
	skipUnlessHAClusterIPv6Topology(t)
	setupHAClusterIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute command on a specific agent
	result, err := haIPv6TestEnv.ExecuteCommandAndWait(ctx, "agent-ha-ipv6-1", "echo", "hello", "from", "ha", "ipv6", "cluster")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	expectedOutput := "hello from ha ipv6 cluster"
	if !strings.Contains(result.Stdout, expectedOutput) {
		t.Errorf("Expected output to contain '%s', got: %s", expectedOutput, result.Stdout)
	}

	t.Logf("Command output: %s", result.Stdout)
}

// TestHAClusterIPv6_BatchCommand tests batch command execution across HA IPv6 cluster
func TestHAClusterIPv6_BatchCommand(t *testing.T) {
	skipUnlessHAClusterIPv6Topology(t)
	setupHAClusterIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := haIPv6TestEnv.Client()

	// Execute batch command targeting all agents in the HA IPv6 cluster
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "e2e-ha-ipv6-batch-test-1",
		Target:      "*", // All agents
		Command:     "hostname",
		Concurrency: 5,
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
		t.Logf("HA IPv6 batch response: type=%s", resp.Type)
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

	if summary.Total != 5 {
		t.Errorf("Expected 5 total HA IPv6 agents, got %d", summary.Total)
	}

	if summary.Successful != 5 {
		t.Errorf("Expected 5 successful executions, got %d", summary.Successful)
	}

	if summary.Failed != 0 {
		t.Errorf("Expected 0 failed executions, got %d", summary.Failed)
	}

	t.Logf("HA IPv6 batch completed: %d/%d successful (%.1f%%)",
		summary.Successful, summary.Total, summary.SuccessRate)
}

// TestHAClusterIPv6_TargetByLabels tests targeting agents by network and topology labels
func TestHAClusterIPv6_TargetByLabels(t *testing.T) {
	skipUnlessHAClusterIPv6Topology(t)
	setupHAClusterIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := haIPv6TestEnv.Client()

	// Target agents with network=ipv6
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "e2e-ha-ipv6-label-test",
		Target:      "labels.network:ipv6",
		Command:     "echo",
		Args:        []string{"targeted by network label"},
		Concurrency: 5,
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

	// All 5 agents should have network=ipv6 label
	if summary.Total != 5 {
		t.Errorf("Expected 5 agents with network=ipv6, got %d", summary.Total)
	}

	if summary.Successful != 5 {
		t.Errorf("Expected 5 successful executions, got %d", summary.Successful)
	}

	t.Logf("Successfully targeted %d agents by network=ipv6 label", summary.Total)
}

// TestHAClusterIPv6_GetBatchJobStatus tests retrieving batch job status
func TestHAClusterIPv6_GetBatchJobStatus(t *testing.T) {
	skipUnlessHAClusterIPv6Topology(t)
	setupHAClusterIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := haIPv6TestEnv.Client()

	// First, create a batch job so we have something to query
	batchReq := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "e2e-ha-ipv6-status-test",
		Target:      "*",
		Command:     "echo",
		Args:        []string{"status test"},
		Concurrency: 5,
	}

	stream, err := client.BatchExecuteCommand(ctx, batchReq)
	if err != nil {
		t.Fatalf("Failed to execute batch command: %v", err)
	}

	// Consume the stream to completion
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Stream error: %v", err)
		}
	}

	// Now get the status of the job we just created
	req := &pb.GetBatchJobStatusRequest{
		BatchJobId: "e2e-ha-ipv6-status-test",
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
		t.Logf("HA IPv6 Job %s: status=%s, total=%d, successful=%d",
			resp.Job.BatchJobId, resp.Job.Status,
			resp.Job.Summary.Total, resp.Job.Summary.Successful)
	} else {
		t.Logf("HA IPv6 Job %s: status=%s (no summary available)",
			resp.Job.BatchJobId, resp.Job.Status)
	}
}

// TestHAClusterIPv6_ListBatchJobs tests listing batch jobs
func TestHAClusterIPv6_ListBatchJobs(t *testing.T) {
	skipUnlessHAClusterIPv6Topology(t)
	setupHAClusterIPv6Environment(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := haIPv6TestEnv.Client()

	// Create multiple batch jobs so we have something to list
	jobIDs := []string{
		"e2e-ha-ipv6-list-test-1",
		"e2e-ha-ipv6-list-test-2",
	}

	for _, jobID := range jobIDs {
		batchReq := &pb.BatchExecuteCommandRequest{
			BatchJobId:  jobID,
			Target:      "*",
			Command:     "echo",
			Args:        []string{"list test", jobID},
			Concurrency: 5,
		}

		stream, err := client.BatchExecuteCommand(ctx, batchReq)
		if err != nil {
			t.Fatalf("Failed to execute batch command %s: %v", jobID, err)
		}

		// Consume the stream to completion
		for {
			_, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Stream error for %s: %v", jobID, err)
			}
		}
	}

	// Now list the batch jobs
	req := &pb.ListBatchJobsRequest{
		PageSize: 10,
	}

	resp, err := client.ListBatchJobs(ctx, req)
	if err != nil {
		t.Fatalf("Failed to list batch jobs: %v", err)
	}

	// Count HA IPv6-related jobs
	haIPv6Jobs := 0
	for _, job := range resp.Jobs {
		if strings.Contains(job.BatchJobId, "ha-ipv6") {
			haIPv6Jobs++
			t.Logf("HA IPv6 Job: %s (status: %s)", job.BatchJobId, job.Status)
		}
	}

	// Should have at least the 2 jobs we created in this test
	if haIPv6Jobs < 2 {
		t.Errorf("Expected at least 2 HA IPv6 batch jobs, got %d", haIPv6Jobs)
	}
}
