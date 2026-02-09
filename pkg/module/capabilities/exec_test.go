package capabilities

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestExecCapability_Validate(t *testing.T) {
	tests := []struct {
		name        string
		cap         *ExecCapability
		expectError bool
	}{
		{
			name: "valid capability",
			cap: &ExecCapability{
				AllowedCommands: []string{"/bin/echo"},
				TimeoutMax:      30 * time.Second,
			},
			expectError: false,
		},
		{
			name: "no allowed commands",
			cap: &ExecCapability{
				TimeoutMax: 30 * time.Second,
			},
			expectError: true,
		},
		{
			name: "zero timeout",
			cap: &ExecCapability{
				AllowedCommands: []string{"/bin/echo"},
				TimeoutMax:      0,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cap.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestExecCapability_CheckCommand(t *testing.T) {
	execCap := &ExecCapability{
		AllowedCommands: []string{
			"/bin/echo",
			"/usr/bin/grep",
		},
		TimeoutMax: 30 * time.Second,
	}

	tests := []struct {
		name        string
		command     string
		expectError error
	}{
		{
			name:        "allowed command",
			command:     "/bin/echo",
			expectError: nil,
		},
		{
			name:        "not allowed command",
			command:     "/bin/rm",
			expectError: ErrCommandNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := execCap.CheckCommand(tt.command)
			if tt.expectError != nil {
				if err == nil {
					t.Errorf("expected error %v but got nil", tt.expectError)
				} else if !errors.Is(err, tt.expectError) {
					t.Errorf("expected error %v but got %v", tt.expectError, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestExecCapability_Exec(t *testing.T) {
	// Get platform-specific echo command
	echoCmd := "/bin/echo"
	if runtime.GOOS == "windows" {
		echoCmd = "C:\\Windows\\System32\\cmd.exe"
	}

	execCap := &ExecCapability{
		AllowedCommands: []string{echoCmd},
		TimeoutMax:      5 * time.Second,
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test successful execution
	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"/C", "echo", "hello world"}
	} else {
		args = []string{"hello", "world"}
	}

	result, err := execCap.Exec(ctx, echoCmd, args...)
	if err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	if !result.Success() {
		t.Errorf("expected success, got exit code %d", result.ExitCode)
	}

	// Check output (may have newline)
	var expectedOutput string
	if runtime.GOOS == "windows" {
		expectedOutput = "hello world\r\n"
	} else {
		expectedOutput = "hello world\n"
	}

	if result.Stdout != expectedOutput {
		t.Errorf("expected stdout %q, got %q", expectedOutput, result.Stdout)
	}
}

func TestExecCapability_ExecNotAllowed(t *testing.T) {
	execCap := &ExecCapability{
		AllowedCommands: []string{"/bin/echo"},
		TimeoutMax:      5 * time.Second,
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test execution of non-allowed command
	_, err := execCap.Exec(ctx, "/bin/rm", "-rf", "/")
	if !errors.Is(err, ErrCommandNotAllowed) {
		t.Errorf("expected ErrCommandNotAllowed, got %v", err)
	}
}

func TestExecCapability_ExecWithInput(t *testing.T) {
	// Get platform-specific cat/type command
	catCmd := "/bin/cat"
	if runtime.GOOS == "windows" {
		catCmd = "C:\\Windows\\System32\\findstr.exe"
	}

	execCap := &ExecCapability{
		AllowedCommands: []string{catCmd},
		TimeoutMax:      5 * time.Second,
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	input := "test input"
	args := []string{}
	if runtime.GOOS == "windows" {
		args = []string{"."} // findstr . (match any character, effectively cat)
	}

	result, err := execCap.ExecWithInput(ctx, input, catCmd, args...)
	if err != nil {
		t.Fatalf("command execution failed: %v", err)
	}

	if !result.Success() {
		t.Errorf("expected success, got exit code %d", result.ExitCode)
	}

	// Check that input was echoed back
	if !contains(result.Stdout, "test") {
		t.Errorf("expected stdout to contain 'test', got %q", result.Stdout)
	}
}

func TestExecCapability_ExecTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("timeout test not reliable on Windows")
	}

	// Use yes command which generates infinite output (will definitely not finish)
	sleepCmd := "/usr/bin/yes"
	args := []string{}

	execCap := &ExecCapability{
		AllowedCommands: []string{sleepCmd},
		TimeoutMax:      200 * time.Millisecond,
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test timeout
	startTime := time.Now()
	result, err := execCap.Exec(ctx, sleepCmd, args...)
	duration := time.Since(startTime)

	// The command should be killed by timeout
	// Duration should be close to timeout value (allow 2x overhead for process cleanup)
	if duration > 2*time.Second {
		t.Errorf("command took too long: %v (expected ~200ms)", duration)
	}

	// Should get an error (either timeout or exit error from killed process)
	if err == nil && result != nil && result.Success() {
		t.Error("expected error or non-zero exit code, got success")
	}
}

func TestExecResult_Success(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		expected bool
	}{
		{
			name:     "success",
			exitCode: 0,
			expected: true,
		},
		{
			name:     "failure",
			exitCode: 1,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ExecResult{ExitCode: tt.exitCode}
			if result.Success() != tt.expected {
				t.Errorf("expected Success() = %v, got %v", tt.expected, result.Success())
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
