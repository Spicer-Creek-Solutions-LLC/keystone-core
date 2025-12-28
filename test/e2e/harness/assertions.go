// Package harness provides assertion helpers for E2E testing.
package harness

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// CommandResult aggregates the streaming command response into a single result
type CommandResult struct {
	CommandID string
	ExitCode  int32
	Stdout    string
	Stderr    string
	Error     string
}

// ExecuteCommandAndWait executes a command and waits for completion, aggregating stream responses
func (e *TestEnvironment) ExecuteCommandAndWait(ctx context.Context, agentID, command string, args ...string) (*CommandResult, error) {
	stream, err := e.client.ExecuteCommand(ctx, &pb.ExecuteCommandRequest{
		AgentId: agentID,
		Command: command,
		Args:    args,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute command: %w", err)
	}

	result := &CommandResult{}
	var stdout, stderr strings.Builder

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("stream error: %w", err)
		}

		result.CommandID = resp.CommandId

		switch resp.Type {
		case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT:
			stdout.Write(resp.Data)
		case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDERR:
			stderr.Write(resp.Data)
		case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED:
			result.ExitCode = resp.ExitCode
		case pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED:
			result.ExitCode = resp.ExitCode
			result.Error = resp.Error
		}
	}

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	return result, nil
}

// WaitForBatchJobComplete waits for a batch job to complete
func (e *TestEnvironment) WaitForBatchJobComplete(ctx context.Context, batchJobID string, timeout time.Duration) (*pb.GetBatchJobStatusResponse, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := e.client.GetBatchJobStatus(ctx, &pb.GetBatchJobStatusRequest{
			BatchJobId: batchJobID,
		})
		if err == nil && resp.Job != nil {
			if resp.Job.Status == pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED ||
				resp.Job.Status == pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED {
				return resp, nil
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			continue
		}
	}

	return nil, fmt.Errorf("batch job %s did not complete within %s", batchJobID, timeout)
}

// WaitForAgentStatus waits for an agent to reach a specific status
func (e *TestEnvironment) WaitForAgentStatus(ctx context.Context, agentID string, status pb.AgentStatus, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := e.client.GetAgent(ctx, &pb.GetAgentRequest{
			AgentId: agentID,
		})
		if err == nil && resp.Agent != nil && resp.Agent.Status == status {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
			continue
		}
	}

	return fmt.Errorf("agent %s did not reach status %s within %s", agentID, status, timeout)
}

// WaitForCondition waits for a condition function to return true
func WaitForCondition(ctx context.Context, timeout time.Duration, interval time.Duration, condition func() (bool, error)) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		ok, err := condition()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
			continue
		}
	}

	return fmt.Errorf("condition not met within %s", timeout)
}

// AssertAgentOnline asserts that an agent is online
func (e *TestEnvironment) AssertAgentOnline(t *testing.T, ctx context.Context, agentID string) {
	t.Helper()

	resp, err := e.client.GetAgent(ctx, &pb.GetAgentRequest{
		AgentId: agentID,
	})
	if err != nil {
		t.Errorf("Failed to get agent %s: %v", agentID, err)
		return
	}
	if resp.Agent == nil {
		t.Errorf("Agent %s not found", agentID)
		return
	}
	if resp.Agent.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
		t.Errorf("Agent %s is not online, status: %s", agentID, resp.Agent.Status)
	}
}

// AssertAgentCount asserts the number of registered agents
func (e *TestEnvironment) AssertAgentCount(t *testing.T, ctx context.Context, expected int) {
	t.Helper()

	resp, err := e.client.ListAgents(ctx, &pb.ListAgentsRequest{
		PageSize: 100,
	})
	if err != nil {
		t.Errorf("Failed to list agents: %v", err)
		return
	}
	if len(resp.Agents) != expected {
		t.Errorf("Expected %d agents, got %d", expected, len(resp.Agents))
	}
}

// AssertCommandSuccess asserts that a command executed successfully
func (e *TestEnvironment) AssertCommandSuccess(t *testing.T, ctx context.Context, agentID, command string, args ...string) *CommandResult {
	t.Helper()

	result, err := e.ExecuteCommandAndWait(ctx, agentID, command, args...)
	if err != nil {
		t.Errorf("Failed to execute command '%s' on %s: %v", command, agentID, err)
		return nil
	}
	if result.ExitCode != 0 {
		t.Errorf("Command '%s' on %s failed with exit code %d: %s", command, agentID, result.ExitCode, result.Stderr)
	}
	return result
}

// AssertCommandOutput asserts that a command produces expected output
func (e *TestEnvironment) AssertCommandOutput(t *testing.T, ctx context.Context, agentID, command string, expectedOutput string, args ...string) {
	t.Helper()

	result := e.AssertCommandSuccess(t, ctx, agentID, command, args...)
	if result == nil {
		return
	}

	if !strings.Contains(result.Stdout, expectedOutput) {
		t.Errorf("Expected output to contain '%s', got: %s", expectedOutput, result.Stdout)
	}
}

// AssertBatchSuccess asserts that a batch command completed successfully on all targets
func (e *TestEnvironment) AssertBatchSuccess(t *testing.T, ctx context.Context, batchJobID string, expectedAgents int) *pb.GetBatchJobStatusResponse {
	t.Helper()

	resp, err := e.WaitForBatchJobComplete(ctx, batchJobID, 60*time.Second)
	if err != nil {
		t.Errorf("Batch job %s did not complete: %v", batchJobID, err)
		return nil
	}

	if resp.Job.Summary == nil {
		t.Errorf("Batch job %s has no summary", batchJobID)
		return resp
	}

	if resp.Job.Summary.Total != int32(expectedAgents) {
		t.Errorf("Expected batch job to target %d agents, got %d", expectedAgents, resp.Job.Summary.Total)
	}
	if resp.Job.Summary.Successful != int32(expectedAgents) {
		t.Errorf("Expected %d successful executions, got %d", expectedAgents, resp.Job.Summary.Successful)
	}
	if resp.Job.Summary.Failed != 0 {
		t.Errorf("Expected 0 failed executions, got %d", resp.Job.Summary.Failed)
	}

	return resp
}

// AssertNoErrors checks container logs for errors
func (e *TestEnvironment) AssertNoErrors(t *testing.T, ctx context.Context, service string, errorPatterns ...string) {
	t.Helper()

	output, err := e.Exec(ctx, service, "cat", "/var/log/kscore.log")
	if err != nil {
		// Log file might not exist, that's okay
		return
	}

	// Default error patterns
	if len(errorPatterns) == 0 {
		errorPatterns = []string{
			"level=error",
			"level=fatal",
			"panic:",
			"FATAL",
		}
	}

	for _, pattern := range errorPatterns {
		if strings.Contains(output, pattern) {
			t.Errorf("Found error pattern '%s' in %s logs", pattern, service)
		}
	}
}

// AssertFileExists checks if a file exists on an agent
func (e *TestEnvironment) AssertFileExists(t *testing.T, ctx context.Context, agentID, filePath string) {
	t.Helper()

	result, err := e.ExecuteCommandAndWait(ctx, agentID, "test", "-f", filePath)
	if err != nil {
		t.Errorf("Failed to check file %s on %s: %v", filePath, agentID, err)
		return
	}
	if result.ExitCode != 0 {
		t.Errorf("File %s does not exist on %s", filePath, agentID)
	}
}

// AssertFileContents checks if a file contains expected content
func (e *TestEnvironment) AssertFileContents(t *testing.T, ctx context.Context, agentID, filePath, expectedContent string) {
	t.Helper()

	result, err := e.ExecuteCommandAndWait(ctx, agentID, "cat", filePath)
	if err != nil {
		t.Errorf("Failed to read file %s on %s: %v", filePath, agentID, err)
		return
	}
	if result.ExitCode != 0 {
		t.Errorf("Failed to read file %s on %s: %s", filePath, agentID, result.Stderr)
		return
	}
	if !strings.Contains(result.Stdout, expectedContent) {
		t.Errorf("File %s on %s does not contain expected content '%s', got: %s",
			filePath, agentID, expectedContent, result.Stdout)
	}
}

// AssertServiceRunning checks if a service is running on an agent (for systemd containers)
func (e *TestEnvironment) AssertServiceRunning(t *testing.T, ctx context.Context, agentID, serviceName string) {
	t.Helper()

	result, err := e.ExecuteCommandAndWait(ctx, agentID, "pgrep", "-x", serviceName)
	if err != nil {
		t.Errorf("Failed to check service %s on %s: %v", serviceName, agentID, err)
		return
	}
	if result.ExitCode != 0 {
		t.Errorf("Service %s is not running on %s", serviceName, agentID)
	}
}

// RetryWithBackoff retries a function with exponential backoff
func RetryWithBackoff(ctx context.Context, maxRetries int, initialDelay time.Duration, fn func() error) error {
	delay := initialDelay

	for i := 0; i < maxRetries; i++ {
		err := fn()
		if err == nil {
			return nil
		}

		if i == maxRetries-1 {
			return fmt.Errorf("after %d retries: %w", maxRetries, err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			delay *= 2
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
		}
	}

	return nil
}
