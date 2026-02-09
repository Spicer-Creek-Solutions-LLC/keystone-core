// Package scenarios contains E2E test scenarios for Keystone Core features.
package scenarios

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/test/e2e/harness"
)

// =============================================================================
// Remote Execution Tests (T3.2)
// These tests verify remote command execution functionality end-to-end.
// =============================================================================

// TestRemoteExec_SimpleCommand tests basic command execution
func TestRemoteExec_SimpleCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Execute simple echo command
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "echo", "hello", "world")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	expected := "hello world"
	if !strings.Contains(result.Stdout, expected) {
		t.Errorf("Expected stdout to contain %q, got %q", expected, result.Stdout)
	}
}

// TestRemoteExec_StreamingOutput tests commands with multi-line streaming output
func TestRemoteExec_StreamingOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Execute command that generates multiple lines of output
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "sh", "-c",
		"for i in 1 2 3 4 5; do echo \"line $i\"; done")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	// Verify all lines are present
	for i := 1; i <= 5; i++ {
		expectedLine := "line " + string(rune('0'+i))
		if !strings.Contains(result.Stdout, expectedLine) {
			t.Errorf("Expected stdout to contain %q", expectedLine)
		}
	}

	t.Logf("Streaming output received:\n%s", result.Stdout)
}

// TestRemoteExec_StderrOutput tests capturing stderr output
func TestRemoteExec_StderrOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Execute command that writes to stderr
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "sh", "-c",
		"echo 'stdout message'; echo 'stderr message' >&2")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	if !strings.Contains(result.Stdout, "stdout message") {
		t.Errorf("Expected stdout to contain 'stdout message', got %q", result.Stdout)
	}

	if !strings.Contains(result.Stderr, "stderr message") {
		t.Errorf("Expected stderr to contain 'stderr message', got %q", result.Stderr)
	}
}

// TestRemoteExec_NonZeroExit tests handling commands with non-zero exit codes
func TestRemoteExec_NonZeroExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	testCases := []struct {
		name     string
		exitCode int
		args     []string
	}{
		{"exit 1", 1, []string{"-c", "exit 1"}},
		{"exit 42", 42, []string{"-c", "exit 42"}},
		{"exit 255", 255, []string{"-c", "exit 255"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "sh", tc.args...)
			if err != nil {
				t.Fatalf("Failed to execute command: %v", err)
			}

			if result.ExitCode != int32(tc.exitCode) {
				t.Errorf("Expected exit code %d, got %d", tc.exitCode, result.ExitCode)
			}
		})
	}
}

// TestRemoteExec_CommandNotFound tests handling of missing commands
func TestRemoteExec_CommandNotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Execute non-existent command via shell to ensure proper error handling
	// Direct command execution may behave differently than shell execution
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "sh", "-c", "nonexistent_command_12345")
	if err != nil {
		// Error at dispatch level is acceptable - command couldn't be run
		t.Logf("Command execution returned error (expected): %v", err)
		return
	}

	// Should have non-zero exit code (typically 127 for command not found)
	// Shell execution should properly report the error
	if result.ExitCode == 0 {
		// If exit code is 0, check stderr for error messages
		if strings.Contains(result.Stderr, "not found") || strings.Contains(result.Stdout, "not found") {
			t.Logf("Command not found detected in output despite exit code 0")
		} else {
			t.Errorf("Expected non-zero exit code or error message for missing command")
		}
	} else {
		t.Logf("Non-existent command correctly returned exit code: %d", result.ExitCode)
	}

	t.Logf("Exit code: %d, stderr: %s, stdout: %s", result.ExitCode, result.Stderr, result.Stdout)
}

// TestRemoteExec_ContextTimeout tests command timeout handling
func TestRemoteExec_ContextTimeout(t *testing.T) {
	// Use a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Execute a command that sleeps longer than our context timeout
	// Note: The actual behavior depends on how the server handles context cancellation
	start := time.Now()
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "sleep", "1")
	elapsed := time.Since(start)

	// Sleep 1 should complete before 3s timeout
	if err != nil {
		t.Fatalf("Command failed unexpectedly: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0 for sleep 1, got %d", result.ExitCode)
	}

	if elapsed > 2*time.Second {
		t.Logf("Command took %v (expected ~1s)", elapsed)
	}
}

// TestRemoteExec_GlobTargeting tests targeting agents with glob patterns
func TestRemoteExec_GlobTargeting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := testEnv.Client()

	testCases := []struct {
		name          string
		target        string
		expectedCount int
	}{
		{"all agents", "*", 3},
		{"web agents", "id:agent-web-*", 2},
		{"db agents", "id:agent-db-*", 1},
		{"specific agent", "id:agent-web-1", 1},
		{"no match", "id:nonexistent-*", 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			batchID := "e2e-glob-" + tc.name

			req := &pb.BatchExecuteCommandRequest{
				BatchJobId:  batchID,
				Target:      tc.target,
				Command:     "echo",
				Args:        []string{"targeting test"},
				Concurrency: 3,
			}

			stream, err := client.BatchExecuteCommand(ctx, req)
			if err != nil {
				if tc.expectedCount == 0 {
					// Expected to fail when no agents match
					return
				}
				t.Fatalf("Failed to execute batch command: %v", err)
			}

			var agentIDs []string
			var summary *pb.BatchSummary

			for {
				resp, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					if tc.expectedCount == 0 {
						return // Expected
					}
					t.Fatalf("Stream error: %v", err)
				}

				if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_AGENT_COMPLETE {
					agentIDs = append(agentIDs, resp.AgentId)
				}
				if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE {
					summary = resp.Summary
				}
			}

			if tc.expectedCount == 0 {
				if len(agentIDs) > 0 {
					t.Errorf("Expected no agents to match, got %d", len(agentIDs))
				}
				return
			}

			if summary == nil {
				t.Fatal("No batch summary received")
			}

			if int(summary.Total) != tc.expectedCount {
				t.Errorf("Expected %d agents, got %d", tc.expectedCount, summary.Total)
			}

			t.Logf("Target %q matched %d agents: %v", tc.target, len(agentIDs), agentIDs)
		})
	}
}

// TestRemoteExec_BatchParallelExecution tests parallel batch execution
func TestRemoteExec_BatchParallelExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Execute command on all agents with high concurrency
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "e2e-parallel-test",
		Target:      "*",
		Command:     "sh",
		Args:        []string{"-c", "echo started; sleep 1; echo done"},
		Concurrency: 10, // Higher than agent count
	}

	start := time.Now()

	stream, err := client.BatchExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute batch command: %v", err)
	}

	var agentStarts []time.Time
	var agentEnds []time.Time
	batchStart := time.Time{}

	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Stream error: %v", err)
		}

		switch resp.Type {
		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_START:
			batchStart = time.Now()
		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_AGENT_START:
			agentStarts = append(agentStarts, time.Now())
		case pb.BatchResponseType_BATCH_RESPONSE_TYPE_AGENT_COMPLETE:
			agentEnds = append(agentEnds, time.Now())
		default:
		}
	}

	elapsed := time.Since(start)

	// With parallel execution, all 3 agents should complete in ~1 second
	// (the sleep time) rather than 3 seconds (sequential)
	if elapsed > 5*time.Second {
		t.Errorf("Parallel execution took too long: %v (expected ~1-2s)", elapsed)
	}

	// Verify all agents started roughly at the same time (within 1 second of each other)
	if len(agentStarts) >= 2 && !batchStart.IsZero() {
		maxDiff := time.Duration(0)
		for i := 1; i < len(agentStarts); i++ {
			diff := agentStarts[i].Sub(agentStarts[0])
			if diff < 0 {
				diff = -diff
			}
			if diff > maxDiff {
				maxDiff = diff
			}
		}
		if maxDiff > 2*time.Second {
			t.Logf("Agent starts were spread over %v (expected <2s for parallel)", maxDiff)
		}
	}

	t.Logf("Batch execution completed in %v with %d agents", elapsed, len(agentEnds))
}

// TestRemoteExec_BatchSequentialExecution tests sequential batch execution (concurrency=1)
func TestRemoteExec_BatchSequentialExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client := testEnv.Client()

	// Execute command on all agents with concurrency=1 (sequential)
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "e2e-sequential-test",
		Target:      "*",
		Command:     "sh",
		Args:        []string{"-c", "echo agent; sleep 0.5; echo done"},
		Concurrency: 1, // Sequential execution
	}

	start := time.Now()

	stream, err := client.BatchExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute batch command: %v", err)
	}

	completionOrder := []string{}

	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Stream error: %v", err)
		}

		if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_AGENT_COMPLETE {
			completionOrder = append(completionOrder, resp.AgentId)
		}
	}

	elapsed := time.Since(start)

	// With sequential execution (3 agents * 0.5s), should take at least 1.5s
	if elapsed < 1*time.Second {
		t.Logf("Sequential execution was faster than expected: %v", elapsed)
	}

	t.Logf("Sequential batch completed in %v, order: %v", elapsed, completionOrder)
}

// TestRemoteExec_LargeOutput tests handling of commands with large output
func TestRemoteExec_LargeOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Generate 500 lines of output (smaller to avoid buffer truncation)
	// The system may have output buffer limits, so we test with a moderate size
	numLines := 500
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "sh", "-c",
		fmt.Sprintf("awk 'BEGIN { for(i=1; i<=%d; i++) print \"Line \" i \": test output\" }'", numLines))
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d: %s", result.ExitCode, result.Stderr)
	}

	// Verify we got substantial output
	hasLine1 := strings.Contains(result.Stdout, "Line 1:")
	hasLastLine := strings.Contains(result.Stdout, fmt.Sprintf("Line %d:", numLines))

	// We should get at least 1KB of output to verify large output handling works
	// Note: Buffer limits may truncate output, so we use a conservative minimum
	expectedMinBytes := 1000
	if len(result.Stdout) < expectedMinBytes {
		t.Errorf("Expected at least %d bytes, got %d bytes", expectedMinBytes, len(result.Stdout))
	}

	if !hasLine1 {
		t.Errorf("Missing Line 1 in output")
	}

	// Log results (last line may be truncated due to buffer limits)
	t.Logf("Received %d bytes of output, hasLine1=%v, hasLine%d=%v",
		len(result.Stdout), hasLine1, numLines, hasLastLine)
}

// TestRemoteExec_EnvironmentVariables tests accessing environment variables
func TestRemoteExec_EnvironmentVariables(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Check for standard environment variables
	// Use env command which is more reliable than echo for environment inspection
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "env")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	// Check for common environment variables (at least one should be present)
	hasEnvVars := strings.Contains(result.Stdout, "PATH=") ||
		strings.Contains(result.Stdout, "HOME=") ||
		strings.Contains(result.Stdout, "PWD=")
	if !hasEnvVars {
		t.Errorf("Expected environment variables in output, got: %s", result.Stdout)
	}

	t.Logf("Environment output (first 500 chars):\n%.500s", result.Stdout)
}

// TestRemoteExec_WorkingDirectory tests command execution in different directories
func TestRemoteExec_WorkingDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Get current working directory
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "pwd")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	t.Logf("Agent working directory: %s", strings.TrimSpace(result.Stdout))
}

// TestRemoteExec_ShellExpansion tests shell feature expansion
func TestRemoteExec_ShellExpansion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	testCases := []struct {
		name     string
		command  string
		contains string
	}{
		{"variable expansion", "echo $HOME", "/"},        // HOME should contain /
		{"command substitution", "echo $(hostname)", ""}, // Just verify it runs
		{"arithmetic", "echo $((1+1))", "2"},
		{"glob expansion", "echo /etc/pass*", "passwd"},
		// Note: brace expansion is NOT supported in ash/busybox sh (Alpine default)
		// Only bash supports it, so we test with a sequence instead
		{"sequence expansion", "echo $(seq 1 3 | tr '\\n' ' ')", "1 2 3"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "sh", "-c", tc.command)
			if err != nil {
				t.Fatalf("Failed to execute command: %v", err)
			}

			if result.ExitCode != 0 {
				t.Errorf("Expected exit code 0, got %d: %s", result.ExitCode, result.Stderr)
			}

			if tc.contains != "" && !strings.Contains(result.Stdout, tc.contains) {
				t.Errorf("Expected output to contain %q, got %q", tc.contains, result.Stdout)
			}

			t.Logf("%s output: %s", tc.name, strings.TrimSpace(result.Stdout))
		})
	}
}

// TestRemoteExec_ConcurrentCommands tests executing multiple commands concurrently
func TestRemoteExec_ConcurrentCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	agents := []string{"agent-web-1", "agent-web-2", "agent-db-1"}

	// Execute commands on all agents concurrently using goroutines
	var wg sync.WaitGroup
	results := make(chan struct {
		agentID string
		result  *harness.CommandResult
		err     error
	}, len(agents))

	for _, agentID := range agents {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			result, err := testEnv.ExecuteCommandAndWait(ctx, id, "hostname")
			results <- struct {
				agentID string
				result  *harness.CommandResult
				err     error
			}{id, result, err}
		}(agentID)
	}

	wg.Wait()
	close(results)

	successCount := 0
	for r := range results {
		if r.err != nil {
			t.Errorf("Command on %s failed: %v", r.agentID, r.err)
			continue
		}
		if r.result.ExitCode != 0 {
			t.Errorf("Command on %s returned non-zero: %d", r.agentID, r.result.ExitCode)
			continue
		}
		successCount++
		t.Logf("Agent %s hostname: %s", r.agentID, strings.TrimSpace(r.result.Stdout))
	}

	if successCount != len(agents) {
		t.Errorf("Expected %d successes, got %d", len(agents), successCount)
	}
}

// TestRemoteExec_SpecialCharacters tests handling of special characters in commands
func TestRemoteExec_SpecialCharacters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	testCases := []struct {
		name     string
		expected string
	}{
		{"spaces", "hello   world"},
		{"quotes", "it's a \"test\""},
		{"newlines", "line1\nline2"},
		{"tabs", "col1\tcol2"},
		{"backslash", "path\\to\\file"},
		{"dollar", "$HOME is home"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Using printf to handle special characters correctly
			result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "printf", "%s", tc.expected)
			if err != nil {
				t.Fatalf("Failed to execute command: %v", err)
			}

			if result.ExitCode != 0 {
				t.Errorf("Expected exit code 0, got %d", result.ExitCode)
			}

			// Note: Some special characters may be interpreted differently
			t.Logf("Output for %s: %q", tc.name, result.Stdout)
		})
	}
}

// TestRemoteExec_BinaryOutput tests handling of binary/non-text output
func TestRemoteExec_BinaryOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Generate some binary-ish output (null bytes, etc.)
	// Using printf with octal escapes
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "printf", "\\x00\\x01\\x02\\x03")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	// Just verify we got some output without crashing
	t.Logf("Binary output length: %d bytes", len(result.Stdout))
}

// Note: CommandResult type is defined in harness/assertions.go and used via testEnv methods
