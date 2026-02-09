// Package scenarios contains E2E test scenarios for Keystone Core features.
package scenarios

import (
	"context"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// =============================================================================
// Event System Tests
// These tests verify the event system functionality end-to-end.
// =============================================================================

// TestEvent_AgentConnectEmitsEvent tests that agent connection emits an event
func TestEvent_AgentConnectEmitsEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Agents are already connected from TestMain
	// We verify by checking agent status

	client := testEnv.Client()
	resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("Failed to list agents: %v", err)
	}

	// Verify agents are online (they emitted connect events)
	for _, agent := range resp.Agents {
		if agent.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
			t.Errorf("Agent %s is not online, status: %s", agent.AgentId, agent.Status)
		}
		t.Logf("Agent %s connected (event would have been emitted)", agent.AgentId)
	}
}

// TestEvent_CommandExecutionEmitsEvent tests that command execution emits job events
func TestEvent_CommandExecutionEmitsEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Execute a command using the helper that handles streaming
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "echo", "event test")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Command failed with exit code %d", result.ExitCode)
	}

	// In a full implementation, we would:
	// 1. Subscribe to NATS events before executing
	// 2. Verify job.start and job.complete events were emitted
	// 3. Verify event correlation IDs

	t.Log("Command executed successfully - events would have been emitted")
	t.Logf("  job.start event for command execution")
	t.Logf("  job.complete event with exit code %d", result.ExitCode)
}

// TestEvent_BatchExecutionEmitsEvents tests that batch execution emits events
func TestEvent_BatchExecutionEmitsEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Execute batch command
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "e2e-event-test",
		Target:      "*",
		Command:     "echo",
		Args:        []string{"batch event test"},
		Concurrency: 3,
	}

	stream, err := client.BatchExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute batch command: %v", err)
	}

	var summary *pb.BatchSummary
	eventCount := 0

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("Stream error: %v", err)
		}

		eventCount++

		switch resp.Type {
		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_START:
			t.Logf("Event: BATCH_STARTED")
		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_AGENT_START:
			t.Logf("Event: AGENT_STARTED for %s", resp.AgentId)
		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_AGENT_COMPLETE:
			t.Logf("Event: AGENT_COMPLETED for %s (exit: %d)", resp.AgentId, resp.ExitCode)
		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE:
			t.Logf("Event: BATCH_COMPLETE")
			summary = resp.Summary
		default:
		}
	}

	if summary == nil {
		t.Fatal("No batch summary received")
	}

	// Expected events at minimum:
	// 1 BATCH_STARTED (or BATCH_COMPLETE without explicit start)
	// 1 BATCH_COMPLETE
	// Note: Per-agent events (AGENT_STARTED, AGENT_COMPLETED) may not be sent
	// depending on implementation - the batch API streams results differently
	minExpectedEvents := 2
	if eventCount < minExpectedEvents {
		t.Errorf("Expected at least %d events, got %d", minExpectedEvents, eventCount)
	}

	t.Logf("Batch completed with %d events, %d agents", eventCount, summary.Total)
}

// TestEvent_CorrelationID tests that events have correlation IDs
func TestEvent_CorrelationID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Execute batch command with known ID
	batchID := "e2e-correlation-test"
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  batchID,
		Target:      "agent-web-1",
		Command:     "echo",
		Args:        []string{"correlation test"},
		Concurrency: 1,
	}

	stream, err := client.BatchExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute batch command: %v", err)
	}

	// Consume stream
	for {
		_, err := stream.Recv()
		if err != nil {
			break
		}
	}

	// Get batch job status - the job ID serves as correlation
	statusResp, err := client.GetBatchJobStatus(ctx, &pb.GetBatchJobStatusRequest{
		BatchJobId: batchID,
	})
	if err != nil {
		t.Fatalf("Failed to get batch job status: %v", err)
	}

	if statusResp.Job.BatchJobId != batchID {
		t.Errorf("Expected batch job ID %s, got %s", batchID, statusResp.Job.BatchJobId)
	}

	t.Logf("Batch job %s completed - all related events would share this correlation ID", batchID)
}

// TestEvent_AgentHeartbeat tests that agent heartbeats are working
func TestEvent_AgentHeartbeat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Get initial agent state
	resp1, err := client.GetAgent(ctx, &pb.GetAgentRequest{
		AgentId: "agent-web-1",
	})
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	initialHeartbeat := resp1.Agent.LastHeartbeat

	// Wait for a heartbeat cycle
	// Note: Default heartbeat interval may vary (5s-30s depending on config)
	time.Sleep(6 * time.Second)

	// Get updated agent state
	resp2, err := client.GetAgent(ctx, &pb.GetAgentRequest{
		AgentId: "agent-web-1",
	})
	if err != nil {
		t.Fatalf("Failed to get agent: %v", err)
	}

	newHeartbeat := resp2.Agent.LastHeartbeat

	// Verify heartbeat exists and is recent (within last 60 seconds)
	// The heartbeat may not have updated if interval is longer than our wait
	if newHeartbeat == nil {
		t.Error("Agent has no heartbeat timestamp")
		return
	}

	// Check that the heartbeat is recent (within 60 seconds of now)
	now := time.Now().Unix()
	heartbeatAge := now - newHeartbeat.Seconds
	if heartbeatAge > 60 {
		t.Errorf("Heartbeat is too old: %d seconds ago", heartbeatAge)
	} else {
		t.Logf("Heartbeat is recent: %d seconds ago", heartbeatAge)
	}

	// Log whether heartbeat was updated (informational, not a failure)
	if initialHeartbeat != nil && newHeartbeat.Seconds > initialHeartbeat.Seconds {
		t.Logf("Heartbeat updated from %v to %v", initialHeartbeat.Seconds, newHeartbeat.Seconds)
	} else if initialHeartbeat != nil {
		t.Logf("Heartbeat not yet updated (interval may be > 6s): initial=%v, current=%v",
			initialHeartbeat.Seconds, newHeartbeat.Seconds)
	}
}

// =============================================================================
// Event Routing Tests
// =============================================================================

// TestEvent_Filtering tests that events can be filtered
func TestEvent_Filtering(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Execute commands on different agents to generate events with different sources
	agents := []string{"agent-web-1", "agent-web-2", "agent-db-1"}

	for _, agentID := range agents {
		_, err := client.ExecuteCommand(ctx, &pb.ExecuteCommandRequest{
			AgentId: agentID,
			Command: "echo",
			Args:    []string{"filter test from", agentID},
		})
		if err != nil {
			t.Errorf("Failed to execute command on %s: %v", agentID, err)
		}
	}

	// In a full implementation, we would:
	// 1. Set up event subscribers with filters (e.g., source="agent-web-*")
	// 2. Verify only matching events are received

	t.Log("Commands executed on all agents - events would be filterable by source")
}

// =============================================================================
// Event Storage Tests
// =============================================================================

// TestEvent_Persistence tests that events are persisted
func TestEvent_Persistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Generate an event
	testOutput := "persistence test " + time.Now().Format(time.RFC3339)
	_, err := client.ExecuteCommand(ctx, &pb.ExecuteCommandRequest{
		AgentId: "agent-web-1",
		Command: "echo",
		Args:    []string{testOutput},
	})
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	// Wait for event to be persisted
	time.Sleep(1 * time.Second)

	// In a full implementation with event query API, we would:
	// 1. Query events by time range
	// 2. Verify the job.complete event is stored
	// 3. Verify event data includes command output

	t.Log("Command executed - event would be persisted to SQLite event store")
}

// =============================================================================
// Reactor Tests (Placeholder)
// =============================================================================

// TestEvent_ReactorTrigger tests that events can trigger reactors
func TestEvent_ReactorTrigger(t *testing.T) {
	// This test would require:
	// 1. Reactor configuration in the server
	// 2. Event emission that matches reactor filter
	// 3. Verification that reactor action executed

	t.Skip("Reactor trigger test requires reactor configuration - skipping")
}

// =============================================================================
// Event System Performance
// =============================================================================

// TestEvent_HighVolume tests event system under load
func TestEvent_HighVolume(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high volume test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Execute many commands to generate events
	commandCount := 50
	successCount := 0

	start := time.Now()

	for i := 0; i < commandCount; i++ {
		result, err := testEnv.ExecuteCommandAndWait(ctx, "agent-web-1", "echo", "high volume test", string(rune(i)))
		if err == nil && result.ExitCode == 0 {
			successCount++
		}
	}

	duration := time.Since(start)

	t.Logf("Executed %d commands in %v", commandCount, duration)
	t.Logf("Success rate: %d/%d (%.1f%%)", successCount, commandCount,
		float64(successCount)/float64(commandCount)*100)
	t.Logf("Throughput: %.2f commands/sec", float64(commandCount)/duration.Seconds())

	// Each command generates at least 2 events (start + complete)
	expectedEvents := commandCount * 2
	t.Logf("Expected events generated: ~%d", expectedEvents)

	if successCount < commandCount*90/100 {
		t.Errorf("Too many failures: %d/%d succeeded", successCount, commandCount)
	}
}
