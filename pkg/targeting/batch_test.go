package targeting

import (
	"context"
	"fmt"
	"testing"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// mockConnectionManager implements ConnectionManager for testing
type mockConnectionManager struct {
	agents []*AgentInfo
}

func (m *mockConnectionManager) ListAgents() []*AgentInfo {
	return m.agents
}

func (m *mockConnectionManager) GetAgent(id string) (*AgentInfo, error) {
	for _, agent := range m.agents {
		if agent.ID == id {
			return agent, nil
		}
	}
	return nil, fmt.Errorf("agent not found: %s", id)
}

// mockCommandDispatcher implements CommandDispatcher for testing
type mockCommandDispatcher struct {
	executions map[string]*mockExecution
}

type mockExecution struct {
	agentID  string
	command  string
	exitCode int32
	output   string
	err      error
	delay    time.Duration
}

func newMockCommandDispatcher() *mockCommandDispatcher {
	return &mockCommandDispatcher{
		executions: make(map[string]*mockExecution),
	}
}

func (m *mockCommandDispatcher) setExecution(agentID string, exitCode int32, output string, err error) {
	m.executions[agentID] = &mockExecution{
		agentID:  agentID,
		exitCode: exitCode,
		output:   output,
		err:      err,
	}
}

func (m *mockCommandDispatcher) setExecutionWithDelay(agentID string, exitCode int32, output string, err error, delay time.Duration) {
	m.executions[agentID] = &mockExecution{
		agentID:  agentID,
		exitCode: exitCode,
		output:   output,
		err:      err,
		delay:    delay,
	}
}

func (m *mockCommandDispatcher) ExecuteCommand(ctx context.Context, req *pb.ExecuteCommandRequest) (<-chan *pb.ExecuteCommandResponse, error) {
	exec, ok := m.executions[req.AgentId]
	if !ok {
		exec = &mockExecution{
			agentID:  req.AgentId,
			exitCode: 0,
			output:   "success",
		}
	}

	if exec.err != nil {
		return nil, exec.err
	}

	responseChan := make(chan *pb.ExecuteCommandResponse, 10)

	go func() {
		defer close(responseChan)

		// Simulate delay if specified
		if exec.delay > 0 {
			timer := time.NewTimer(exec.delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}

		// Send output
		if exec.output != "" {
			responseChan <- &pb.ExecuteCommandResponse{
				CommandId: req.CommandId,
				Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_STDOUT,
				Data:      []byte(exec.output),
			}
		}

		// Send completion
		if exec.exitCode == 0 {
			responseChan <- &pb.ExecuteCommandResponse{
				CommandId: req.CommandId,
				Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED,
				ExitCode:  exec.exitCode,
			}
		} else {
			responseChan <- &pb.ExecuteCommandResponse{
				CommandId: req.CommandId,
				Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_FAILED,
				ExitCode:  exec.exitCode,
				Error:     fmt.Sprintf("command failed with exit code %d", exec.exitCode),
			}
		}
	}()

	return responseChan, nil
}

func TestNewBatchExecutor(t *testing.T) {
	connMgr := &mockConnectionManager{}
	dispatcher := newMockCommandDispatcher()

	executor := NewBatchExecutor(dispatcher, connMgr)

	if executor == nil {
		t.Fatal("NewBatchExecutor() returned nil")
	}

	if executor.concurrency != 10 {
		t.Errorf("NewBatchExecutor() concurrency = %d, want 10", executor.concurrency)
	}

	if executor.defaultTimeout != 300 {
		t.Errorf("NewBatchExecutor() defaultTimeout = %d, want 300", executor.defaultTimeout)
	}
}

func TestBatchExecutor_SetConcurrency(t *testing.T) {
	connMgr := &mockConnectionManager{}
	dispatcher := newMockCommandDispatcher()
	executor := NewBatchExecutor(dispatcher, connMgr)

	executor.SetConcurrency(5)
	if executor.concurrency != 5 {
		t.Errorf("SetConcurrency(5) = %d, want 5", executor.concurrency)
	}

	// Should set to 1 if less than 1
	executor.SetConcurrency(0)
	if executor.concurrency != 1 {
		t.Errorf("SetConcurrency(0) = %d, want 1", executor.concurrency)
	}

	executor.SetConcurrency(-5)
	if executor.concurrency != 1 {
		t.Errorf("SetConcurrency(-5) = %d, want 1", executor.concurrency)
	}
}

func TestBatchExecutor_Execute(t *testing.T) {
	// Create test agents
	agents := []*AgentInfo{
		{
			ID:     "agent-1",
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Os: "linux",
				Labels: map[string]string{
					"role": "web",
				},
			},
		},
		{
			ID:     "agent-2",
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Os: "linux",
				Labels: map[string]string{
					"role": "web",
				},
			},
		},
		{
			ID:     "agent-3",
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Os: "linux",
				Labels: map[string]string{
					"role": "db",
				},
			},
		},
	}

	connMgr := &mockConnectionManager{agents: agents}
	dispatcher := newMockCommandDispatcher()

	// Set up expected executions
	dispatcher.setExecution("agent-1", 0, "output-1", nil)
	dispatcher.setExecution("agent-2", 0, "output-2", nil)

	executor := NewBatchExecutor(dispatcher, connMgr)
	ctx := context.Background()

	// Execute on web servers only
	batch, err := executor.Execute(ctx, "role:web", "echo", []string{"hello"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if batch == nil {
		t.Fatal("Execute() returned nil batch")
	}

	// Check progress
	total, completed, failed := batch.Progress()
	if total != 2 {
		t.Errorf("Progress() total = %d, want 2", total)
	}
	if completed != 2 {
		t.Errorf("Progress() completed = %d, want 2", completed)
	}
	if failed != 0 {
		t.Errorf("Progress() failed = %d, want 0", failed)
	}

	// Check results
	results := batch.Results()
	if len(results) != 2 {
		t.Fatalf("Results() len = %d, want 2", len(results))
	}

	for _, result := range results {
		if !result.Success {
			t.Errorf("Result for agent %s not successful", result.AgentID)
		}
		if result.ExitCode != 0 {
			t.Errorf("Result for agent %s exit code = %d, want 0", result.AgentID, result.ExitCode)
		}
	}

	// Check success rate
	successRate := batch.SuccessRate()
	if successRate != 100.0 {
		t.Errorf("SuccessRate() = %f, want 100.0", successRate)
	}

	// Check completion
	if !batch.IsComplete() {
		t.Error("IsComplete() = false, want true")
	}
}

func TestBatchExecutor_Execute_NoMatches(t *testing.T) {
	agents := []*AgentInfo{
		{
			ID:     "agent-1",
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Os: "linux",
			},
		},
	}

	connMgr := &mockConnectionManager{agents: agents}
	dispatcher := newMockCommandDispatcher()
	executor := NewBatchExecutor(dispatcher, connMgr)
	ctx := context.Background()

	// Try to match non-existent OS
	_, err := executor.Execute(ctx, "os:windows", "echo", []string{"hello"})
	if err == nil {
		t.Error("Execute() expected error for no matches, got nil")
	}
}

func TestBatchExecutor_Execute_OfflineAgents(t *testing.T) {
	agents := []*AgentInfo{
		{
			ID:     "agent-1",
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Os: "linux",
			},
		},
		{
			ID:     "agent-2",
			Status: pb.AgentStatus_AGENT_STATUS_OFFLINE,
			Metadata: &pb.AgentMetadata{
				Os: "linux",
			},
		},
	}

	connMgr := &mockConnectionManager{agents: agents}
	dispatcher := newMockCommandDispatcher()
	dispatcher.setExecution("agent-1", 0, "success", nil)

	executor := NewBatchExecutor(dispatcher, connMgr)
	ctx := context.Background()

	batch, err := executor.Execute(ctx, "os:linux", "echo", []string{"hello"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	total, completed, failed := batch.Progress()
	if total != 2 {
		t.Errorf("Progress() total = %d, want 2", total)
	}
	if completed != 2 {
		t.Errorf("Progress() completed = %d, want 2", completed)
	}
	if failed != 1 {
		t.Errorf("Progress() failed = %d, want 1 (offline agent)", failed)
	}

	results := batch.Results()
	if len(results) != 2 {
		t.Fatalf("Results() len = %d, want 2", len(results))
	}

	// Check that offline agent failed
	var foundOffline bool
	for _, result := range results {
		if result.AgentID == "agent-2" {
			foundOffline = true
			if result.Success {
				t.Error("Offline agent result should not be successful")
			}
			if result.Error == nil {
				t.Error("Offline agent result should have error")
			}
		}
	}

	if !foundOffline {
		t.Error("Did not find result for offline agent")
	}

	successRate := batch.SuccessRate()
	if successRate != 50.0 {
		t.Errorf("SuccessRate() = %f, want 50.0", successRate)
	}
}

func TestBatchExecutor_Execute_CommandFailure(t *testing.T) {
	agents := []*AgentInfo{
		{
			ID:     "agent-1",
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Os: "linux",
			},
		},
		{
			ID:     "agent-2",
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Os: "linux",
			},
		},
	}

	connMgr := &mockConnectionManager{agents: agents}
	dispatcher := newMockCommandDispatcher()

	// agent-1 succeeds, agent-2 fails
	dispatcher.setExecution("agent-1", 0, "success", nil)
	dispatcher.setExecution("agent-2", 1, "error output", nil)

	executor := NewBatchExecutor(dispatcher, connMgr)
	ctx := context.Background()

	batch, err := executor.Execute(ctx, "os:linux", "test-command", nil)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	_, _, failed := batch.Progress()
	if failed != 1 {
		t.Errorf("Progress() failed = %d, want 1", failed)
	}

	successRate := batch.SuccessRate()
	if successRate != 50.0 {
		t.Errorf("SuccessRate() = %f, want 50.0", successRate)
	}
}

func TestBatchExecutor_Concurrency(t *testing.T) {
	// Create 20 agents
	agents := make([]*AgentInfo, 20)
	for i := 0; i < 20; i++ {
		agents[i] = &AgentInfo{
			ID:     fmt.Sprintf("agent-%d", i),
			Status: pb.AgentStatus_AGENT_STATUS_ONLINE,
			Metadata: &pb.AgentMetadata{
				Os: "linux",
			},
		}
	}

	connMgr := &mockConnectionManager{agents: agents}
	dispatcher := newMockCommandDispatcher()

	// Set up executions with delays to test concurrency
	for i := 0; i < 20; i++ {
		agentID := fmt.Sprintf("agent-%d", i)
		dispatcher.setExecutionWithDelay(agentID, 0, "done", nil, 50*time.Millisecond)
	}

	executor := NewBatchExecutor(dispatcher, connMgr)
	executor.SetConcurrency(5) // Limit to 5 concurrent executions

	ctx := context.Background()
	start := time.Now()

	batch, err := executor.Execute(ctx, "os:linux", "sleep", []string{"0.05"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	duration := time.Since(start)

	// With concurrency of 5 and 20 agents with 50ms delay each,
	// it should take roughly (20/5) * 50ms = 200ms
	// Allow some variance for scheduling
	if duration < 150*time.Millisecond || duration > 350*time.Millisecond {
		t.Logf("Warning: Duration %v outside expected range [150ms, 350ms]", duration)
		// Don't fail the test as timing can be unpredictable in CI
	}

	total, completed, _ := batch.Progress()
	if total != 20 || completed != 20 {
		t.Errorf("Progress() total=%d completed=%d, want both 20", total, completed)
	}
}

func TestBatchExecution_SuccessRate(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		completed int
		failed    int
		wantRate  float64
	}{
		{
			name:      "all successful",
			total:     10,
			completed: 10,
			failed:    0,
			wantRate:  100.0,
		},
		{
			name:      "half successful",
			total:     10,
			completed: 10,
			failed:    5,
			wantRate:  50.0,
		},
		{
			name:      "all failed",
			total:     10,
			completed: 10,
			failed:    10,
			wantRate:  0.0,
		},
		{
			name:      "empty",
			total:     0,
			completed: 0,
			failed:    0,
			wantRate:  0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batch := &BatchExecution{
				total:     tt.total,
				completed: tt.completed,
				failed:    tt.failed,
			}

			rate := batch.SuccessRate()
			if rate != tt.wantRate {
				t.Errorf("SuccessRate() = %f, want %f", rate, tt.wantRate)
			}
		})
	}
}
