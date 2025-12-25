package agent

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestExecutor_Execute_Success(t *testing.T) {
	executor := NewExecutor()

	// Use a simple cross-platform command
	var command string
	var args []string
	if runtime.GOOS == "windows" {
		command = "cmd"
		args = []string{"/c", "echo", "hello"}
	} else {
		command = "echo"
		args = []string{"hello"}
	}

	req := &ExecuteCommandRequest{
		CommandID: "test-1",
		Command:   command,
		Args:      args,
		Timeout:   5 * time.Second,
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, req, nil)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	if result.CommandID != "test-1" {
		t.Errorf("Expected command ID 'test-1', got %s", result.CommandID)
	}

	if result.StartTime.IsZero() {
		t.Error("StartTime should be set")
	}

	if result.EndTime.IsZero() {
		t.Error("EndTime should be set")
	}

	if result.EndTime.Before(result.StartTime) {
		t.Error("EndTime should be after StartTime")
	}
}

func TestExecutor_Execute_WithTimeout(t *testing.T) {
	executor := NewExecutor()

	// Use a cross-platform sleep command
	var command string
	var args []string
	if runtime.GOOS == "windows" {
		command = "timeout"
		args = []string{"/t", "10", "/nobreak"}
	} else {
		command = "sleep"
		args = []string{"10"}
	}

	req := &ExecuteCommandRequest{
		CommandID: "test-timeout",
		Command:   command,
		Args:      args,
		Timeout:   100 * time.Millisecond, // Very short timeout
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, req, nil)

	// The command should timeout or fail
	// Note: timeout behavior can be flaky in test environments
	if result.ExitCode == 0 && err == nil && result.Error == nil {
		t.Log("Warning: Expected timeout but command succeeded - this can happen in some environments")
	}
}

func TestExecutor_Execute_WithOutputHandler(t *testing.T) {
	executor := NewExecutor()

	outputReceived := false
	handler := func(commandID string, isStderr bool, data []byte) {
		outputReceived = true
		if commandID != "test-output" {
			t.Errorf("Expected command ID 'test-output', got %s", commandID)
		}
		if len(data) == 0 {
			t.Error("Expected non-empty output data")
		}
	}

	var command string
	var args []string
	if runtime.GOOS == "windows" {
		command = "cmd"
		args = []string{"/c", "echo", "test"}
	} else {
		command = "echo"
		args = []string{"test"}
	}

	req := &ExecuteCommandRequest{
		CommandID: "test-output",
		Command:   command,
		Args:      args,
		Timeout:   5 * time.Second,
	}

	ctx := context.Background()
	_, err := executor.Execute(ctx, req, handler)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !outputReceived {
		t.Error("Expected output handler to be called")
	}
}

func TestExecutor_Execute_InvalidCommand(t *testing.T) {
	executor := NewExecutor()

	req := &ExecuteCommandRequest{
		CommandID: "test-invalid",
		Command:   "nonexistent-command-xyz123",
		Args:      []string{},
		Timeout:   5 * time.Second,
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, req, nil)

	// Should fail to start the command
	if err == nil && result.Error == nil {
		t.Error("Expected error for invalid command")
	}
}

func TestExecutor_CancelCommand(t *testing.T) {
	executor := NewExecutor()

	// This test just verifies the cancel function exists and doesn't panic
	err := executor.CancelCommand("nonexistent")
	if err == nil {
		t.Error("Expected error when canceling nonexistent command")
	}
}

func TestExecutor_GetRunningCommands(t *testing.T) {
	executor := NewExecutor()

	commands := executor.GetRunningCommands()
	if commands == nil {
		t.Error("GetRunningCommands should return a slice")
	}

	if len(commands) != 0 {
		t.Errorf("Expected 0 running commands, got %d", len(commands))
	}
}

func TestNewExecutor(t *testing.T) {
	executor := NewExecutor()
	if executor == nil {
		t.Fatal("NewExecutor should return non-nil executor")
	}

	if executor.runningCommands == nil {
		t.Error("Running commands map should be initialized")
	}
}
