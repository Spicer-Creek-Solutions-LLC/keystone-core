// Copyright 2026 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package execution

import (
	"context"
	"testing"
	"time"
)

func TestNewPowerShellExecutor_Windows(t *testing.T) {
	exec := NewPowerShellExecutor()
	if exec == nil {
		t.Fatal("NewPowerShellExecutor returned nil")
	}

	// Check default values
	if !exec.PreferCore {
		t.Error("PreferCore should default to true")
	}
	if !exec.UseBypassPolicy {
		t.Error("UseBypassPolicy should default to true")
	}
	if !exec.NoProfile {
		t.Error("NoProfile should default to true")
	}
	if !exec.NoLogo {
		t.Error("NoLogo should default to true")
	}
	if exec.OutputEncoding != "UTF8" {
		t.Errorf("OutputEncoding should default to UTF8, got %s", exec.OutputEncoding)
	}
}

func TestNewCmdExecutor_Windows(t *testing.T) {
	exec := NewCmdExecutor()
	if exec == nil {
		t.Fatal("NewCmdExecutor returned nil")
	}

	// Check default values
	if !exec.HideWindow {
		t.Error("HideWindow should default to true")
	}
}

func TestPowerShellExecutor_DetectPowerShell_Windows(t *testing.T) {
	exec := NewPowerShellExecutor()

	version, err := exec.DetectPowerShell()
	if err != nil {
		t.Skipf("PowerShell not detected: %v", err)
	}

	if version == nil {
		t.Fatal("Version should not be nil when error is nil")
	}

	if version.Path == "" {
		t.Error("Version.Path should not be empty")
	}
	if version.Edition != "Desktop" && version.Edition != "Core" {
		t.Errorf("Version.Edition should be Desktop or Core, got %s", version.Edition)
	}
	if version.VersionText == "" {
		t.Error("Version.VersionText should not be empty")
	}
	if version.Major < 1 {
		t.Errorf("Version.Major should be at least 1, got %d", version.Major)
	}

	t.Logf("Detected PowerShell: %s %s (v%s)", version.Edition, version.Path, version.VersionText)
}

func TestPowerShellExecutor_DetectPowerShell_Caching_Windows(t *testing.T) {
	exec := NewPowerShellExecutor()

	// First call
	version1, err := exec.DetectPowerShell()
	if err != nil {
		t.Skipf("PowerShell not detected: %v", err)
	}

	// Second call should return cached version
	version2, err := exec.DetectPowerShell()
	if err != nil {
		t.Fatalf("Second DetectPowerShell failed: %v", err)
	}

	if version1 != version2 {
		t.Error("Expected cached version to be returned")
	}
}

func TestPowerShellExecutor_GetPolicy_Windows(t *testing.T) {
	exec := NewPowerShellExecutor()
	ctx := context.Background()

	policy, err := exec.GetPolicy(ctx)
	if err != nil {
		t.Skipf("GetPolicy failed: %v", err)
	}

	validPolicies := map[Policy]bool{
		PolicyRestricted:   true,
		PolicyAllSigned:    true,
		PolicyRemoteSigned: true,
		PolicyUnrestricted: true,
		PolicyBypass:       true,
		PolicyUndefined:    true,
	}

	if !validPolicies[policy] {
		t.Errorf("Unexpected execution policy: %s", policy)
	}

	t.Logf("Execution policy: %s", policy)
}

func TestPowerShellExecutor_Execute_Windows(t *testing.T) {
	exec := NewPowerShellExecutor()
	ctx := context.Background()

	result, err := exec.Execute(ctx, "Write-Output 'Hello, World!'")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.Success() {
		t.Errorf("Expected success, got exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	if result.Stdout == "" {
		t.Error("Expected output from Write-Output")
	}

	if result.Duration == 0 {
		t.Error("Expected non-zero duration")
	}

	t.Logf("Output: %s (took %s)", result.Stdout, result.Duration)
}

func TestPowerShellExecutor_Execute_ExitCode_Windows(t *testing.T) {
	exec := NewPowerShellExecutor()
	ctx := context.Background()

	result, err := exec.Execute(ctx, "exit 42")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("Expected exit code 42, got %d", result.ExitCode)
	}

	if result.Success() {
		t.Error("Expected Success() to return false for exit code 42")
	}
}

func TestPowerShellExecutor_Execute_Stderr_Windows(t *testing.T) {
	exec := NewPowerShellExecutor()
	ctx := context.Background()

	result, err := exec.Execute(ctx, "Write-Error 'This is an error'")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Write-Error writes to stderr
	if result.Stderr == "" && result.Stdout == "" {
		t.Error("Expected error output")
	}
}

func TestPowerShellExecutor_Timeout_Windows(t *testing.T) {
	exec := NewPowerShellExecutor()
	exec.Timeout = 100 * time.Millisecond

	ctx := context.Background()
	result, err := exec.Execute(ctx, "Start-Sleep -Seconds 10")

	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded error, got: %v", err)
	}

	if result != nil && result.ExitCode != -1 {
		t.Errorf("Expected exit code -1 for timeout, got %d", result.ExitCode)
	}
}

func TestPowerShellExecutor_ContextCancellation_Windows(t *testing.T) {
	exec := NewPowerShellExecutor()

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	result, err := exec.Execute(ctx, "Start-Sleep -Seconds 10")

	if err == nil {
		t.Error("Expected error from cancelled context")
	}

	if result != nil && result.ExitCode == 0 {
		t.Error("Expected non-zero exit code for cancelled execution")
	}
}

func TestPowerShellExecutor_WorkingDirectory_Windows(t *testing.T) {
	exec := NewPowerShellExecutor()
	exec.WorkingDirectory = "C:\\Windows"

	ctx := context.Background()
	result, err := exec.Execute(ctx, "(Get-Location).Path")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Stdout == "" {
		t.Error("Expected working directory in output")
	}

	t.Logf("Working directory: %s", result.Stdout)
}

func TestPowerShellExecutor_Environment_Windows(t *testing.T) {
	exec := NewPowerShellExecutor()
	exec.Environment = map[string]string{
		"TEST_VAR_PS": "test_value_powershell",
	}

	ctx := context.Background()
	result, err := exec.Execute(ctx, "$env:TEST_VAR_PS")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Stdout != "test_value_powershell" {
		t.Errorf("Expected 'test_value_powershell', got '%s'", result.Stdout)
	}
}

func TestPowerShellExecutor_PreferCore_Windows(t *testing.T) {
	// Test with PreferCore = true (default)
	execCore := NewPowerShellExecutor()
	execCore.PreferCore = true

	version, err := execCore.DetectPowerShell()
	if err != nil {
		t.Skipf("PowerShell not detected: %v", err)
	}
	t.Logf("PreferCore=true: %s (%s)", version.Edition, version.Path)

	// Test with PreferCore = false
	execDesktop := NewPowerShellExecutor()
	execDesktop.PreferCore = false
	execDesktop.version = nil // Clear cache

	version2, err := execDesktop.DetectPowerShell()
	if err != nil {
		t.Skipf("PowerShell not detected: %v", err)
	}
	t.Logf("PreferCore=false: %s (%s)", version2.Edition, version2.Path)
}

func TestCmdExecutor_Execute_Windows(t *testing.T) {
	exec := NewCmdExecutor()
	ctx := context.Background()

	result, err := exec.Execute(ctx, "echo Hello from cmd")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !result.Success() {
		t.Errorf("Expected success, got exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	if result.Stdout == "" {
		t.Error("Expected output from echo")
	}

	if result.Duration == 0 {
		t.Error("Expected non-zero duration")
	}

	t.Logf("Output: %s (took %s)", result.Stdout, result.Duration)
}

func TestCmdExecutor_Execute_ExitCode_Windows(t *testing.T) {
	exec := NewCmdExecutor()
	ctx := context.Background()

	result, err := exec.Execute(ctx, "exit /b 42")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("Expected exit code 42, got %d", result.ExitCode)
	}

	if result.Success() {
		t.Error("Expected Success() to return false for exit code 42")
	}
}

func TestCmdExecutor_Timeout_Windows(t *testing.T) {
	exec := NewCmdExecutor()
	exec.Timeout = 100 * time.Millisecond

	ctx := context.Background()
	result, err := exec.Execute(ctx, "ping -n 10 127.0.0.1")

	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded error, got: %v", err)
	}

	if result != nil && result.ExitCode != -1 {
		t.Errorf("Expected exit code -1 for timeout, got %d", result.ExitCode)
	}
}

func TestCmdExecutor_WorkingDirectory_Windows(t *testing.T) {
	exec := NewCmdExecutor()
	exec.WorkingDirectory = "C:\\Windows"

	ctx := context.Background()
	result, err := exec.Execute(ctx, "cd")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Stdout == "" {
		t.Error("Expected working directory in output")
	}

	t.Logf("Working directory: %s", result.Stdout)
}

func TestCmdExecutor_Environment_Windows(t *testing.T) {
	exec := NewCmdExecutor()
	exec.Environment = map[string]string{
		"TEST_VAR_CMD": "test_value_cmd",
	}

	ctx := context.Background()
	result, err := exec.Execute(ctx, "echo %TEST_VAR_CMD%")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Stdout != "test_value_cmd" {
		t.Errorf("Expected 'test_value_cmd', got '%s'", result.Stdout)
	}
}

func TestCmdExecutor_ExecuteBatch_Windows(t *testing.T) {
	// This test would require creating a temp batch file
	// For now, just verify the method exists and returns error for non-existent file
	exec := NewCmdExecutor()
	ctx := context.Background()

	_, err := exec.ExecuteBatch(ctx, "C:\\nonexistent.bat")
	if err == nil {
		t.Error("Expected error for non-existent batch file")
	}
}

func TestResult_Methods_Windows(t *testing.T) {
	// Test Success()
	successResult := &Result{ExitCode: 0}
	if !successResult.Success() {
		t.Error("Expected Success() to return true for exit code 0")
	}

	failResult := &Result{ExitCode: 1}
	if failResult.Success() {
		t.Error("Expected Success() to return false for exit code 1")
	}

	// Test Output()
	stdoutOnly := &Result{Stdout: "stdout"}
	if stdoutOnly.Output() != "stdout" {
		t.Errorf("Expected 'stdout', got '%s'", stdoutOnly.Output())
	}

	stdoutAndStderr := &Result{Stdout: "stdout", Stderr: "stderr"}
	expected := "stdout\nstderr"
	if stdoutAndStderr.Output() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, stdoutAndStderr.Output())
	}
}

func TestPolicy_Values_Windows(t *testing.T) {
	policies := []Policy{
		PolicyRestricted,
		PolicyAllSigned,
		PolicyRemoteSigned,
		PolicyUnrestricted,
		PolicyBypass,
		PolicyUndefined,
	}

	for _, policy := range policies {
		if string(policy) == "" {
			t.Errorf("Policy should have a non-empty string value: %v", policy)
		}
	}
}
