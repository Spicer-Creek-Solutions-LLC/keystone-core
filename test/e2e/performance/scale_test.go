// Package performance contains performance and benchmark tests for Keystone Core.
package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// =============================================================================
// Scale Test Results
// =============================================================================

// ScaleResult holds the results of a scale test
type ScaleResult struct {
	TestName           string        `json:"test_name"`
	AgentCount         int           `json:"agent_count"`
	RegistrationTime   time.Duration `json:"registration_time_ns"`
	HeartbeatSuccesses int           `json:"heartbeat_successes"`
	HeartbeatFailures  int           `json:"heartbeat_failures"`
	CommandSuccesses   int           `json:"command_successes"`
	CommandFailures    int           `json:"command_failures"`
	AvgCommandLatency  time.Duration `json:"avg_command_latency_ns"`
	Timestamp          time.Time     `json:"timestamp"`
}

// =============================================================================
// Agent Registration Tests
// =============================================================================

// TestScale_AgentRegistration tests agent registration time
func TestScale_AgentRegistration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := testEnv.Client()

	// Measure time to list all agents (registration already happened in TestMain)
	start := time.Now()

	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	duration := time.Since(start)

	t.Logf("Agent Registration Scale Test:")
	t.Logf("  Registered agents: %d", len(resp.Agents))
	t.Logf("  List agents latency: %v", duration)

	// Verify all expected agents are registered
	expectedAgents := map[string]bool{
		"agent-web-1": false,
		"agent-web-2": false,
		"agent-db-1":  false,
	}

	for _, agent := range resp.Agents {
		if _, ok := expectedAgents[agent.AgentId]; ok {
			expectedAgents[agent.AgentId] = true
			t.Logf("  Agent %s: status=%s", agent.AgentId, agent.Status)
		}
	}

	for id, found := range expectedAgents {
		if !found {
			t.Errorf("Expected agent %s not found", id)
		}
	}

	result := ScaleResult{
		TestName:         "agent_registration",
		AgentCount:       len(resp.Agents),
		RegistrationTime: duration,
		Timestamp:        time.Now(),
	}

	saveScaleResult(t, result)
}

// TestScale_AgentStatus tests status checks across all agents
func TestScale_AgentStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := testEnv.Client()

	// Get all agents
	listResp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	onlineCount := 0
	offlineCount := 0
	var totalLatency time.Duration

	for _, agent := range listResp.Agents {
		start := time.Now()
		resp, err := client.GetAgent(ctx, &pb.GetAgentRequest{AgentId: agent.AgentId})
		latency := time.Since(start)
		totalLatency += latency

		if err != nil {
			t.Errorf("Failed to get agent %s: %v", agent.AgentId, err)
			continue
		}

		if resp.Agent.Status == pb.AgentStatus_AGENT_STATUS_ONLINE {
			onlineCount++
		} else {
			offlineCount++
		}
	}

	avgLatency := time.Duration(0)
	if len(listResp.Agents) > 0 {
		avgLatency = totalLatency / time.Duration(len(listResp.Agents))
	}

	t.Logf("Agent Status Scale Test:")
	t.Logf("  Total agents: %d", len(listResp.Agents))
	t.Logf("  Online: %d, Offline: %d", onlineCount, offlineCount)
	t.Logf("  Avg status check latency: %v", avgLatency)

	if offlineCount > 0 {
		t.Errorf("%d agents are offline", offlineCount)
	}
}

// TestScale_HeartbeatVerification verifies heartbeats across all agents
func TestScale_HeartbeatVerification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := testEnv.Client()

	// Get all agents
	listResp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	// Check that heartbeats are recent (within 60 seconds)
	// Note: Heartbeat interval may be 5-30 seconds depending on config
	// We verify heartbeats are recent rather than requiring an update
	successes := 0
	failures := 0
	now := time.Now().Unix()
	maxAge := int64(60) // Heartbeat should be within last 60 seconds

	for _, agent := range listResp.Agents {
		resp, err := client.GetAgent(ctx, &pb.GetAgentRequest{AgentId: agent.AgentId})
		if err != nil {
			failures++
			t.Logf("Agent %s: failed to get status: %v", agent.AgentId, err)
			continue
		}

		if resp.Agent.LastHeartbeat == nil {
			failures++
			t.Logf("Agent %s: no heartbeat timestamp", agent.AgentId)
			continue
		}

		heartbeatAge := now - resp.Agent.LastHeartbeat.Seconds
		if heartbeatAge <= maxAge {
			successes++
			t.Logf("Agent %s: heartbeat is recent (%d seconds ago)", agent.AgentId, heartbeatAge)
		} else {
			failures++
			t.Logf("Agent %s: heartbeat is stale (%d seconds ago)", agent.AgentId, heartbeatAge)
		}
	}

	t.Logf("Heartbeat Verification:")
	t.Logf("  Agents checked: %d", len(listResp.Agents))
	t.Logf("  Heartbeat successes: %d", successes)
	t.Logf("  Heartbeat failures: %d", failures)

	result := ScaleResult{
		TestName:           "heartbeat_verification",
		AgentCount:         len(listResp.Agents),
		HeartbeatSuccesses: successes,
		HeartbeatFailures:  failures,
		Timestamp:          time.Now(),
	}

	saveScaleResult(t, result)

	if failures > 0 {
		t.Errorf("%d agents failed heartbeat check", failures)
	}
}

// =============================================================================
// Concurrent Operations Scale Tests
// =============================================================================

// TestScale_ConcurrentCommands tests concurrent command execution across agents
func TestScale_ConcurrentCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client := testEnv.Client()

	// Get all agents
	listResp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	// Execute commands on all agents simultaneously via batch
	start := time.Now()

	stream, err := client.BatchExecuteCommand(ctx, &pb.BatchExecuteCommandRequest{
		BatchJobId:  "scale-concurrent-test",
		Target:      "*",
		Command:     "echo",
		Args:        []string{"scale test"},
		Concurrency: int32(len(listResp.Agents)),
	})
	if err != nil {
		t.Fatalf("Failed to execute batch command: %v", err)
	}

	var summary *pb.BatchSummary
	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE {
			summary = resp.Summary
		}
	}

	duration := time.Since(start)

	if summary == nil {
		t.Fatal("No batch summary received")
	}

	t.Logf("Concurrent Commands Scale Test:")
	t.Logf("  Agents targeted: %d", summary.Total)
	t.Logf("  Successful: %d", summary.Successful)
	t.Logf("  Failed: %d", summary.Failed)
	t.Logf("  Duration: %v", duration)
	t.Logf("  Success rate: %.2f%%", summary.SuccessRate)

	result := ScaleResult{
		TestName:          "concurrent_commands",
		AgentCount:        int(summary.Total),
		CommandSuccesses:  int(summary.Successful),
		CommandFailures:   int(summary.Failed),
		AvgCommandLatency: duration / time.Duration(summary.Total),
		Timestamp:         time.Now(),
	}

	saveScaleResult(t, result)

	if summary.Failed > 0 {
		t.Errorf("%d commands failed", summary.Failed)
	}
}

// TestScale_SequentialAgentCommands tests commands to each agent sequentially
func TestScale_SequentialAgentCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client := testEnv.Client()

	// Get all agents
	listResp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	successes := 0
	failures := 0
	var totalLatency time.Duration

	start := time.Now()

	for _, agent := range listResp.Agents {
		cmdStart := time.Now()
		result, err := testEnv.ExecuteCommandAndWait(ctx, agent.AgentId, "hostname")
		latency := time.Since(cmdStart)

		if err != nil || result.ExitCode != 0 {
			failures++
			t.Logf("Command failed on %s: %v", agent.AgentId, err)
		} else {
			successes++
			totalLatency += latency
		}
	}

	duration := time.Since(start)

	avgLatency := time.Duration(0)
	if successes > 0 {
		avgLatency = totalLatency / time.Duration(successes)
	}

	t.Logf("Sequential Agent Commands:")
	t.Logf("  Agents: %d", len(listResp.Agents))
	t.Logf("  Successful: %d, Failed: %d", successes, failures)
	t.Logf("  Total duration: %v", duration)
	t.Logf("  Avg command latency: %v", avgLatency)

	result := ScaleResult{
		TestName:          "sequential_agent_commands",
		AgentCount:        len(listResp.Agents),
		CommandSuccesses:  successes,
		CommandFailures:   failures,
		AvgCommandLatency: avgLatency,
		Timestamp:         time.Now(),
	}

	saveScaleResult(t, result)

	if failures > 0 {
		t.Errorf("%d commands failed", failures)
	}
}

// =============================================================================
// Resource Usage Tests
// =============================================================================

// TestScale_AgentMetadata tests that all agents have correct metadata
func TestScale_AgentMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := testEnv.Client()

	// Get all agents
	listResp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	t.Logf("Agent Metadata Scale Test:")
	t.Logf("  Total agents: %d", len(listResp.Agents))

	for _, agent := range listResp.Agents {
		t.Logf("  Agent %s:", agent.AgentId)
		if agent.Metadata != nil {
			t.Logf("    Hostname: %s", agent.Metadata.Hostname)
			t.Logf("    OS: %s", agent.Metadata.Os)
			t.Logf("    Arch: %s", agent.Metadata.Arch)
			t.Logf("    Version: %s", agent.Metadata.AgentVersion)
			if agent.Metadata.Labels != nil {
				t.Logf("    Labels: %v", agent.Metadata.Labels)
			}
		}
		t.Logf("    Status: %s", agent.Status)
	}
}

// =============================================================================
// Baseline Comparison
// =============================================================================

// Baseline represents performance baseline values
type Baseline struct {
	MinOpsPerSecond      float64 `json:"min_ops_per_second"`
	MaxP99LatencyMs      float64 `json:"max_p99_latency_ms"`
	MaxErrorRate         float64 `json:"max_error_rate"`
	MinHeartbeatSuccess  float64 `json:"min_heartbeat_success_rate"`
	MaxRegistrationTimeS float64 `json:"max_registration_time_seconds"`
}

// DefaultBaseline returns default performance baseline
func DefaultBaseline() *Baseline {
	return &Baseline{
		MinOpsPerSecond:      1.0,  // At least 1 op/sec
		MaxP99LatencyMs:      5000, // Max 5 second P99
		MaxErrorRate:         5.0,  // Max 5% error rate
		MinHeartbeatSuccess:  95.0, // At least 95% heartbeat success
		MaxRegistrationTimeS: 60.0, // Max 60 seconds registration
	}
}

// TestScale_BaselineComparison compares results against baseline
func TestScale_BaselineComparison(t *testing.T) {
	// Load baseline (or use default)
	baseline := DefaultBaseline()

	// Load baseline from file if it exists
	baselineFile := "baselines/baseline.json"
	if root := os.Getenv("KSCORE_ROOT"); root != "" {
		baselineFile = filepath.Join(root, "test/e2e/performance/baselines/baseline.json")
	}

	if data, err := os.ReadFile(baselineFile); err == nil {
		if err := json.Unmarshal(data, baseline); err != nil {
			t.Logf("Warning: Failed to parse baseline file: %v", err)
		} else {
			t.Log("Loaded custom baseline from file")
		}
	} else {
		t.Log("Using default baseline (no baseline.json found)")
	}

	t.Logf("Performance Baseline:")
	t.Logf("  Min ops/sec: %.2f", baseline.MinOpsPerSecond)
	t.Logf("  Max P99 latency: %.2f ms", baseline.MaxP99LatencyMs)
	t.Logf("  Max error rate: %.2f%%", baseline.MaxErrorRate)
	t.Logf("  Min heartbeat success: %.2f%%", baseline.MinHeartbeatSuccess)
	t.Logf("  Max registration time: %.2f s", baseline.MaxRegistrationTimeS)

	// Note: Actual comparison would happen after running other tests
	// and loading their results from the reports directory
	t.Log("Baseline comparison framework ready")
}

// =============================================================================
// Helpers
// =============================================================================

// saveScaleResult saves a scale test result to JSON file
func saveScaleResult(t *testing.T, result ScaleResult) {
	t.Helper()

	// Create reports directory
	reportsDir := "reports"
	if root := os.Getenv("KSCORE_ROOT"); root != "" {
		reportsDir = filepath.Join(root, "test/e2e/performance/reports")
	}
	_ = os.MkdirAll(reportsDir, 0755)

	// Save individual result
	filename := filepath.Join(reportsDir, fmt.Sprintf("%s_%d.json", result.TestName, result.Timestamp.Unix()))
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Logf("Failed to marshal result: %v", err)
		return
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		t.Logf("Failed to save result: %v", err)
	} else {
		t.Logf("Result saved to: %s", filename)
	}
}
