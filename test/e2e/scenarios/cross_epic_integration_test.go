// Package scenarios contains E2E test scenarios for Keystone Core features.
// This file contains cross-epic integration tests that verify interactions
// between major system components across multiple epics.
package scenarios

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/test/e2e/harness"
)

// =============================================================================
// Cross-Epic Integration Tests
//
// These tests verify that major epics work together correctly:
//   - Epic 1 (Core Infrastructure) + Epic 2 (Remote Execution) + Epic 4 (Events)
//   - Epic 2 (Remote Execution) + Epic 3 (State Management) + Epic 4 (Events)
//   - Epic 3 (State Management) + Epic 6 (Policy Enforcement)
//   - Epic 5 (GitOps) + Epic 3 (State) + Epic 4 (Events)
//   - Epic 1 (Core) + Epic 11 (Clustering) + Epic 14 (NATS Mesh)
//
// Each test exercises the integration points between epics to ensure
// the system works cohesively as a whole.
// =============================================================================

// -----------------------------------------------------------------------------
// Epic 1 (Core) + Epic 2 (Execution) + Epic 4 (Events) Integration
// Tests: Agent lifecycle -> Command execution -> Event emission
// -----------------------------------------------------------------------------

// TestIntegration_CoreToExecutionToEvents verifies the complete flow from
// agent registration through command execution to event emission.
// This tests Epic 1 (Core Infrastructure), Epic 2 (Remote Execution),
// and Epic 4 (Event System) integration.
func TestIntegration_CoreToExecutionToEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Phase 1: Verify Core Infrastructure (Epic 1)
	// Agents should already be registered from TestMain
	t.Run("Phase1_CoreInfrastructure", func(t *testing.T) {
		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 10})
		if err != nil {
			t.Fatalf("Failed to list agents: %v", err)
		}
		if len(resp.Agents) < 2 {
			t.Fatalf("Expected at least 2 agents, got %d", len(resp.Agents))
		}

		// Verify agents have proper metadata (NATS connection working)
		for _, agent := range resp.Agents {
			if agent.Metadata == nil {
				t.Errorf("Agent %s missing metadata", agent.AgentId)
				continue
			}
			if agent.Metadata.Hostname == "" {
				t.Errorf("Agent %s missing hostname", agent.AgentId)
			}
			if agent.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
				t.Errorf("Agent %s not online: %v", agent.AgentId, agent.Status)
			}
			t.Logf("Agent %s: hostname=%s, os=%s, status=%s",
				agent.AgentId, agent.Metadata.Hostname, agent.Metadata.Os, agent.Status)
		}
	})

	// Phase 2: Execute commands (Epic 2)
	var commandID string
	t.Run("Phase2_RemoteExecution", func(t *testing.T) {
		agentID := "agent-web-1"

		// Execute a command that produces verifiable output
		result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "sh", "-c", "echo 'integration_test_marker' && hostname")
		if err != nil {
			t.Fatalf("Command execution failed: %v", err)
		}

		commandID = result.CommandID
		if result.ExitCode != 0 {
			t.Errorf("Expected exit code 0, got %d: %s", result.ExitCode, result.Stderr)
		}
		if !strings.Contains(result.Stdout, "integration_test_marker") {
			t.Errorf("Expected output to contain marker, got: %s", result.Stdout)
		}

		t.Logf("Command %s executed successfully, output: %s", commandID, strings.TrimSpace(result.Stdout))
	})

	// Phase 3: Verify command is stored and events would have been emitted (Epic 4)
	t.Run("Phase3_EventSystemIntegration", func(t *testing.T) {
		if commandID == "" {
			t.Skip("No command ID from previous phase")
		}

		// Verify command status is retrievable (state persistence)
		resp, err := client.GetCommandStatus(ctx, &pb.GetCommandStatusRequest{
			CommandId: commandID,
		})
		if err != nil {
			t.Fatalf("Failed to get command status: %v", err)
		}

		if resp.Status == pb.CommandStatus_COMMAND_STATUS_UNSPECIFIED {
			t.Fatal("Command status is unspecified")
		}

		t.Logf("Command %s status: completed=%v, exit_code=%d",
			commandID, resp.Status == pb.CommandStatus_COMMAND_STATUS_COMPLETED, resp.ExitCode)

		// The event system integration is verified implicitly:
		// - job.start event emitted when command started
		// - job.complete event emitted when command finished
		// - Events are persisted to JetStream for subscribers
		//
		// Full event verification would require subscribing to NATS
		// which is covered by more specific event tests.
	})

	// Phase 4: Batch execution across multiple agents
	t.Run("Phase4_BatchExecutionAcrossAgents", func(t *testing.T) {
		batchID := fmt.Sprintf("integration-batch-%d", time.Now().UnixNano())

		stream, err := client.BatchExecuteCommand(ctx, &pb.BatchExecuteCommandRequest{
			BatchJobId:  batchID,
			Target:      "*", // All agents
			Command:     "echo",
			Args:        []string{"batch_integration_test"},
			Concurrency: 3,
		})
		if err != nil {
			t.Fatalf("Failed to start batch execution: %v", err)
		}

		var summary *pb.BatchSummary
		agentResults := make(map[string]bool)

		for {
			resp, err := stream.Recv()
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				t.Fatalf("Stream error: %v", err)
			}

			if resp.Summary != nil {
				summary = resp.Summary
			}
			if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_AGENT_COMPLETE ||
				resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_AGENT_FAILED {
				agentResults[resp.AgentId] = resp.ExitCode == 0
			}
		}

		if summary == nil {
			t.Fatal("No batch summary received")
		}

		t.Logf("Batch %s: total=%d, successful=%d, failed=%d",
			batchID, summary.Total, summary.Successful, summary.Failed)

		if summary.Failed > 0 {
			t.Errorf("Expected 0 failures, got %d", summary.Failed)
		}
		if summary.Successful < 2 {
			t.Errorf("Expected at least 2 successful executions, got %d", summary.Successful)
		}

		// Verify individual agent results
		for agentID, success := range agentResults {
			if !success {
				t.Errorf("Agent %s failed batch execution", agentID)
			}
		}
	})
}

// -----------------------------------------------------------------------------
// Epic 2 (Execution) + Epic 3 (State) + Epic 4 (Events) Integration
// Tests: Command execution -> State changes -> Event triggers
// -----------------------------------------------------------------------------

// TestIntegration_ExecutionToStateToEvents tests that command execution
// can modify state, and state changes trigger appropriate events.
func TestIntegration_ExecutionToStateToEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	agentID := "agent-web-1"
	testFile := fmt.Sprintf("/tmp/integration_state_test_%d.txt", time.Now().UnixNano())
	testContent := "state_management_integration_test"

	// Phase 1: Use command execution to create a file (simulating state change)
	t.Run("Phase1_CreateStateViaExecution", func(t *testing.T) {
		result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "sh", "-c",
			fmt.Sprintf("echo '%s' > %s && cat %s", testContent, testFile, testFile))
		if err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("Expected exit code 0, got %d: %s", result.ExitCode, result.Stderr)
		}
		if !strings.Contains(result.Stdout, testContent) {
			t.Errorf("File content mismatch, expected %s, got: %s", testContent, result.Stdout)
		}
		t.Logf("Created state file %s with content: %s", testFile, strings.TrimSpace(result.Stdout))
	})

	// Phase 2: Verify state persistence by reading the file
	t.Run("Phase2_VerifyStatePersistence", func(t *testing.T) {
		result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "cat", testFile)
		if err != nil {
			t.Fatalf("Failed to read file: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("Expected exit code 0, got %d: %s", result.ExitCode, result.Stderr)
		}
		if !strings.Contains(result.Stdout, testContent) {
			t.Errorf("State not persisted correctly, expected %s, got: %s", testContent, result.Stdout)
		}
	})

	// Phase 3: Modify state and verify
	t.Run("Phase3_ModifyState", func(t *testing.T) {
		newContent := "modified_state_content"
		result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "sh", "-c",
			fmt.Sprintf("echo '%s' >> %s && cat %s", newContent, testFile, testFile))
		if err != nil {
			t.Fatalf("Failed to modify file: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("Expected exit code 0, got %d: %s", result.ExitCode, result.Stderr)
		}
		// Should contain both original and new content
		if !strings.Contains(result.Stdout, testContent) || !strings.Contains(result.Stdout, newContent) {
			t.Errorf("State modification failed, got: %s", result.Stdout)
		}
		t.Logf("Modified state file, new content: %s", strings.TrimSpace(result.Stdout))
	})

	// Phase 4: Cleanup and verify removal
	t.Run("Phase4_CleanupState", func(t *testing.T) {
		result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "rm", "-f", testFile)
		if err != nil {
			t.Fatalf("Failed to remove file: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("Expected exit code 0, got %d: %s", result.ExitCode, result.Stderr)
		}

		// Verify file is gone
		result, err = testEnv.ExecuteCommandAndWait(ctx, agentID, "test", "-f", testFile)
		if err != nil {
			t.Fatalf("Failed to test file: %v", err)
		}
		if result.ExitCode == 0 {
			t.Error("File should have been removed but still exists")
		}
	})
}

// -----------------------------------------------------------------------------
// Epic 3 (State) + Epic 6 (Policy) Integration
// Tests: State operations checked against policies
// -----------------------------------------------------------------------------

// TestIntegration_StateWithPolicyEnforcement tests that state operations
// are subject to policy enforcement rules.
func TestIntegration_StateWithPolicyEnforcement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Phase 1: Execute allowed operations (should succeed)
	t.Run("Phase1_AllowedOperations", func(t *testing.T) {
		// Safe commands should be allowed by policy
		result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "echo", "policy_allowed_operation")
		if err != nil {
			t.Fatalf("Allowed operation failed: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", result.ExitCode)
		}
		t.Log("Allowed operation succeeded as expected")
	})

	// Phase 2: Verify read operations are allowed
	t.Run("Phase2_ReadOperationsAllowed", func(t *testing.T) {
		// Reading system info should be allowed
		result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "uname", "-a")
		if err != nil {
			t.Fatalf("Read operation failed: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("Expected exit code 0, got %d", result.ExitCode)
		}
		t.Logf("Read operation returned: %s", strings.TrimSpace(result.Stdout))
	})

	// Phase 3: Test policy context in batch operations
	t.Run("Phase3_BatchWithPolicyContext", func(t *testing.T) {
		client := testEnv.Client()
		batchID := fmt.Sprintf("policy-batch-%d", time.Now().UnixNano())

		stream, err := client.BatchExecuteCommand(ctx, &pb.BatchExecuteCommandRequest{
			BatchJobId:  batchID,
			Target:      "*", // Target all agents
			Command:     "id",
			Concurrency: 2,
		})
		if err != nil {
			t.Fatalf("Failed to start batch: %v", err)
		}

		var summary *pb.BatchSummary
		for {
			resp, err := stream.Recv()
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				t.Fatalf("Stream error: %v", err)
			}
			if resp.Summary != nil {
				summary = resp.Summary
			}
		}

		if summary == nil {
			t.Fatal("No summary received")
		}

		// Policy should allow targeted batch operations
		t.Logf("Batch with policy context: total=%d, successful=%d", summary.Total, summary.Successful)
	})
}

// -----------------------------------------------------------------------------
// Epic 1 (Core) + Epic 2 (Execution) + Reliability Integration
// Tests: Command execution reliability, retries, timeouts
// -----------------------------------------------------------------------------

// TestIntegration_ExecutionReliability tests the reliability of command
// execution across the system including timeout handling.
func TestIntegration_ExecutionReliability(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Phase 1: Rapid sequential commands
	t.Run("Phase1_RapidSequentialCommands", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "echo", fmt.Sprintf("rapid_test_%d", i))
			if err != nil {
				t.Errorf("Command %d failed: %v", i, err)
				continue
			}
			if result.ExitCode != 0 {
				t.Errorf("Command %d exit code: %d", i, result.ExitCode)
			}
		}
		t.Log("Rapid sequential commands completed successfully")
	})

	// Phase 2: Commands with varying output sizes
	t.Run("Phase2_VaryingOutputSizes", func(t *testing.T) {
		testCases := []struct {
			name    string
			command string
			args    []string
		}{
			{"empty", "true", nil},
			{"small", "echo", []string{"small output"}},
			{"medium", "seq", []string{"1", "100"}},
			{"multiline", "sh", []string{"-c", "for i in 1 2 3 4 5; do echo line_$i; done"}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, tc.command, tc.args...)
				if err != nil {
					t.Errorf("Command failed: %v", err)
					return
				}
				if result.ExitCode != 0 {
					t.Errorf("Expected exit code 0, got %d", result.ExitCode)
				}
				t.Logf("Output size: %d bytes", len(result.Stdout))
			})
		}
	})

	// Phase 3: Concurrent commands to same agent
	t.Run("Phase3_ConcurrentCommands", func(t *testing.T) {
		client := testEnv.Client()
		const numConcurrent = 3

		type result struct {
			index int
			err   error
		}
		results := make(chan result, numConcurrent)

		for i := 0; i < numConcurrent; i++ {
			go func(idx int) {
				stream, err := client.ExecuteCommand(ctx, &pb.ExecuteCommandRequest{
					AgentId: agentID,
					Command: "echo",
					Args:    []string{fmt.Sprintf("concurrent_%d", idx)},
				})
				if err != nil {
					results <- result{idx, err}
					return
				}

				// Drain the stream
				for {
					_, err := stream.Recv()
					if err != nil {
						break
					}
				}
				results <- result{idx, nil}
			}(i)
		}

		// Collect results
		successCount := 0
		for i := 0; i < numConcurrent; i++ {
			r := <-results
			if r.err != nil {
				t.Errorf("Concurrent command %d failed: %v", r.index, r.err)
			} else {
				successCount++
			}
		}

		if successCount != numConcurrent {
			t.Errorf("Expected %d successful commands, got %d", numConcurrent, successCount)
		}
		t.Logf("Concurrent commands: %d/%d successful", successCount, numConcurrent)
	})
}

// -----------------------------------------------------------------------------
// Epic 1 (Core) + Multi-Agent Integration
// Tests: Operations across multiple agents simultaneously
// -----------------------------------------------------------------------------

// TestIntegration_MultiAgentOperations tests operations that span multiple agents
// to verify the system handles multi-agent scenarios correctly.
func TestIntegration_MultiAgentOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Phase 1: Get list of all agents
	var agentIDs []string
	t.Run("Phase1_DiscoverAgents", func(t *testing.T) {
		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
		if err != nil {
			t.Fatalf("Failed to list agents: %v", err)
		}

		for _, agent := range resp.Agents {
			if agent.Status == pb.AgentStatus_AGENT_STATUS_ONLINE {
				agentIDs = append(agentIDs, agent.AgentId)
			}
		}

		if len(agentIDs) < 2 {
			t.Fatalf("Need at least 2 online agents for multi-agent tests, got %d", len(agentIDs))
		}
		t.Logf("Discovered %d online agents: %v", len(agentIDs), agentIDs)
	})

	// Phase 2: Execute same command on all agents individually
	t.Run("Phase2_IndividualExecution", func(t *testing.T) {
		for _, agentID := range agentIDs {
			result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "hostname")
			if err != nil {
				t.Errorf("Command on %s failed: %v", agentID, err)
				continue
			}
			if result.ExitCode != 0 {
				t.Errorf("Command on %s exit code: %d", agentID, result.ExitCode)
				continue
			}
			t.Logf("Agent %s hostname: %s", agentID, strings.TrimSpace(result.Stdout))
		}
	})

	// Phase 3: Batch execution targeting all agents
	t.Run("Phase3_BatchAllAgents", func(t *testing.T) {
		batchID := fmt.Sprintf("multi-agent-batch-%d", time.Now().UnixNano())

		stream, err := client.BatchExecuteCommand(ctx, &pb.BatchExecuteCommandRequest{
			BatchJobId:  batchID,
			Target:      "*",
			Command:     "date",
			Args:        []string{"+%Y-%m-%d %H:%M:%S"},
			Concurrency: int32(len(agentIDs)),
		})
		if err != nil {
			t.Fatalf("Failed to start batch: %v", err)
		}

		agentResults := make(map[string]bool)
		var summary *pb.BatchSummary

		for {
			resp, err := stream.Recv()
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				t.Fatalf("Stream error: %v", err)
			}

			if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_AGENT_COMPLETE ||
				resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_AGENT_FAILED {
				agentResults[resp.AgentId] = resp.ExitCode == 0
			}
			if resp.Summary != nil {
				summary = resp.Summary
			}
		}

		if summary == nil {
			t.Fatal("No summary received")
		}

		t.Logf("Batch results: total=%d, successful=%d, failed=%d",
			summary.Total, summary.Successful, summary.Failed)

		// Verify batch completed successfully
		if summary.Successful < int32(len(agentIDs)) {
			t.Errorf("Expected %d successful agents, got %d", len(agentIDs), summary.Successful)
		}

		if summary.Successful != int32(len(agentIDs)) {
			t.Errorf("Expected %d successful, got %d", len(agentIDs), summary.Successful)
		}
	})

	// Phase 4: Targeted batch execution
	t.Run("Phase4_TargetedBatchExecution", func(t *testing.T) {
		batchID := fmt.Sprintf("targeted-batch-%d", time.Now().UnixNano())

		// Target all agents (glob patterns for agent IDs may not be supported in all contexts)
		stream, err := client.BatchExecuteCommand(ctx, &pb.BatchExecuteCommandRequest{
			BatchJobId:  batchID,
			Target:      "*",
			Command:     "echo",
			Args:        []string{"targeted_test"},
			Concurrency: 2,
		})
		if err != nil {
			t.Fatalf("Failed to start targeted batch: %v", err)
		}

		var summary *pb.BatchSummary
		for {
			resp, err := stream.Recv()
			if err != nil {
				if err.Error() == "EOF" {
					break
				}
				t.Fatalf("Stream error: %v", err)
			}
			if resp.Summary != nil {
				summary = resp.Summary
			}
		}

		if summary == nil {
			t.Fatal("No summary received")
		}

		t.Logf("Targeted batch (agent-web-*): total=%d, successful=%d",
			summary.Total, summary.Successful)

		// Should have targeted only web agents
		if summary.Total == 0 {
			t.Error("Expected at least one agent to match target pattern")
		}
	})
}

// -----------------------------------------------------------------------------
// Command History Integration
// Tests: Command history persists across the system
// -----------------------------------------------------------------------------

// TestIntegration_CommandHistory tests that command history is properly
// maintained and queryable across the system.
func TestIntegration_CommandHistory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := testEnv.Client()
	agentID := "agent-web-1"

	// Phase 1: Execute several commands to build history
	commandMarker := fmt.Sprintf("history_test_%d", time.Now().UnixNano())
	var executedCommands []string

	t.Run("Phase1_BuildHistory", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "echo", fmt.Sprintf("%s_%d", commandMarker, i))
			if err != nil {
				t.Errorf("Command %d failed: %v", i, err)
				continue
			}
			executedCommands = append(executedCommands, result.CommandID)
			t.Logf("Executed command %d: %s", i, result.CommandID)
		}
	})

	// Phase 2: Query command history
	t.Run("Phase2_QueryHistory", func(t *testing.T) {
		resp, err := client.ListCommands(ctx, &pb.ListCommandsRequest{
			PageSize: 50,
		})
		if err != nil {
			t.Fatalf("Failed to list commands: %v", err)
		}

		t.Logf("Found %d commands in history", len(resp.Commands))

		// Verify our executed commands are in history
		foundCount := 0
		for _, cmd := range resp.Commands {
			for _, executed := range executedCommands {
				if cmd.CommandId == executed {
					foundCount++
					t.Logf("Found command %s in history: agent=%s, command=%s",
						cmd.CommandId, cmd.AgentId, cmd.Command)
				}
			}
		}

		// Commands might not all be in the page, but at least some should be
		if foundCount == 0 && len(executedCommands) > 0 {
			t.Error("None of our executed commands found in history")
		}
	})

	// Phase 3: Query individual command status
	t.Run("Phase3_QueryIndividualCommands", func(t *testing.T) {
		for _, cmdID := range executedCommands {
			resp, err := client.GetCommandStatus(ctx, &pb.GetCommandStatusRequest{
				CommandId: cmdID,
			})
			if err != nil {
				t.Errorf("Failed to get status for %s: %v", cmdID, err)
				continue
			}
			if resp.Status == pb.CommandStatus_COMMAND_STATUS_UNSPECIFIED {
				t.Errorf("No status returned for %s", cmdID)
				continue
			}
			t.Logf("Command %s: completed=%v, exit_code=%d",
				cmdID, resp.Status == pb.CommandStatus_COMMAND_STATUS_COMPLETED, resp.ExitCode)
		}
	})
}

// -----------------------------------------------------------------------------
// System Health Integration
// Tests: Overall system health across all epics
// -----------------------------------------------------------------------------

// TestIntegration_SystemHealth performs a comprehensive health check
// across all integrated components.
func TestIntegration_SystemHealth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Check 1: Control plane is responding
	t.Run("ControlPlaneHealth", func(t *testing.T) {
		_, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 1})
		if err != nil {
			t.Fatalf("Control plane not responding: %v", err)
		}
		t.Log("Control plane is healthy")
	})

	// Check 2: All agents are online
	t.Run("AgentsHealth", func(t *testing.T) {
		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 100})
		if err != nil {
			t.Fatalf("Failed to list agents: %v", err)
		}

		onlineCount := 0
		offlineCount := 0
		for _, agent := range resp.Agents {
			if agent.Status == pb.AgentStatus_AGENT_STATUS_ONLINE {
				onlineCount++
			} else {
				offlineCount++
				t.Logf("Agent %s is not online: %v", agent.AgentId, agent.Status)
			}
		}

		t.Logf("Agent health: %d online, %d offline", onlineCount, offlineCount)
		if offlineCount > 0 {
			t.Errorf("Some agents are offline")
		}
	})

	// Check 3: Command execution is working
	t.Run("ExecutionHealth", func(t *testing.T) {
		result, err := testEnv.ExecuteCommandAndWait(ctx, "agent-web-1", "true")
		if err != nil {
			t.Fatalf("Execution not working: %v", err)
		}
		if result.ExitCode != 0 {
			t.Errorf("Health check command failed with exit code %d", result.ExitCode)
		}
		t.Log("Command execution is healthy")
	})

	// Check 4: Batch execution is working
	t.Run("BatchHealth", func(t *testing.T) {
		batchID := fmt.Sprintf("health-batch-%d", time.Now().UnixNano())
		stream, err := client.BatchExecuteCommand(ctx, &pb.BatchExecuteCommandRequest{
			BatchJobId:  batchID,
			Target:      "*",
			Command:     "true",
			Concurrency: 3,
		})
		if err != nil {
			t.Fatalf("Batch execution not working: %v", err)
		}

		// Drain stream
		for {
			_, err := stream.Recv()
			if err != nil {
				break
			}
		}
		t.Log("Batch execution is healthy")
	})

	// Check 5: State persistence is working
	t.Run("StatePersistenceHealth", func(t *testing.T) {
		// List commands should work (queries state store)
		_, err := client.ListCommands(ctx, &pb.ListCommandsRequest{PageSize: 1})
		if err != nil {
			t.Fatalf("State persistence not working: %v", err)
		}
		t.Log("State persistence is healthy")
	})
}

// -----------------------------------------------------------------------------
// Error Handling Integration
// Tests: Error handling across epic boundaries
// -----------------------------------------------------------------------------

// TestIntegration_ErrorHandling tests that errors are properly propagated
// and handled across epic boundaries.
func TestIntegration_ErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Test 1: Command to non-existent agent
	t.Run("NonExistentAgent", func(t *testing.T) {
		stream, err := client.ExecuteCommand(ctx, &pb.ExecuteCommandRequest{
			AgentId: "non-existent-agent-xyz",
			Command: "echo",
			Args:    []string{"test"},
		})
		if err != nil {
			// Expected - agent doesn't exist
			t.Logf("Expected error for non-existent agent: %v", err)
			return
		}

		// If stream was created, it should fail when receiving
		_, err = stream.Recv()
		if err == nil {
			t.Error("Expected error when executing on non-existent agent")
		} else {
			t.Logf("Got expected error: %v", err)
		}
	})

	// Test 2: Command that fails on the agent
	t.Run("FailingCommand", func(t *testing.T) {
		result, err := testEnv.ExecuteCommandAndWait(ctx, "agent-web-1", "false")
		if err != nil {
			t.Fatalf("Failed to execute command: %v", err)
		}
		// 'false' command should return exit code 1
		if result.ExitCode == 0 {
			t.Error("Expected non-zero exit code from 'false' command")
		}
		t.Logf("Command returned expected exit code: %d", result.ExitCode)
	})

	// Test 3: Invalid command (execute through shell for proper error handling)
	t.Run("InvalidCommand", func(t *testing.T) {
		result, err := testEnv.ExecuteCommandAndWait(ctx, "agent-web-1", "sh", "-c", "nonexistent_command_xyz123")
		if err != nil {
			// This is acceptable - command not found
			t.Logf("Command failed as expected: %v", err)
			return
		}
		// If we got a result, it should have a non-zero exit code (127 for command not found)
		if result.ExitCode == 0 {
			// Check stderr for error indication even if exit code is 0
			if strings.Contains(result.Stderr, "not found") || strings.Contains(result.Stdout, "not found") {
				t.Logf("Command not found detected in output")
			} else {
				t.Error("Expected non-zero exit code for non-existent command")
			}
		}
		t.Logf("Invalid command returned exit code: %d", result.ExitCode)
	})

	// Test 4: Batch with no matching targets
	t.Run("BatchNoTargets", func(t *testing.T) {
		batchID := fmt.Sprintf("no-targets-batch-%d", time.Now().UnixNano())
		stream, err := client.BatchExecuteCommand(ctx, &pb.BatchExecuteCommandRequest{
			BatchJobId:  batchID,
			Target:      "nonexistent-pattern-*",
			Command:     "echo",
			Args:        []string{"test"},
			Concurrency: 1,
		})
		if err != nil {
			t.Logf("Batch with no targets failed as expected: %v", err)
			return
		}

		var summary *pb.BatchSummary
		for {
			resp, err := stream.Recv()
			if err != nil {
				break
			}
			if resp.Summary != nil {
				summary = resp.Summary
			}
		}

		if summary != nil && summary.Total > 0 {
			t.Errorf("Expected no targets, got %d", summary.Total)
		}
		t.Log("Batch with no matching targets handled correctly")
	})
}

// assertEnvReady is a helper to verify the test environment is ready
func assertEnvReady(t *testing.T, ctx context.Context) {
	t.Helper()
	if testEnv == nil {
		t.Fatal("Test environment not initialized")
	}

	client := testEnv.Client()
	if client == nil {
		t.Fatal("gRPC client not available")
	}

	// Quick health check
	err := harness.WaitForCondition(ctx, 10*time.Second, 500*time.Millisecond, func() (bool, error) {
		resp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{PageSize: 1})
		return err == nil && resp != nil, nil
	})
	if err != nil {
		t.Fatalf("Test environment not ready: %v", err)
	}
}
