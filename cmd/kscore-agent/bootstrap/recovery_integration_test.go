package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRecoveryIntegration_ErrorClassificationAndRecovery tests the full flow
// from error occurrence through classification to recovery action execution.
func TestRecoveryIntegration_ErrorClassificationAndRecovery(t *testing.T) {
	output := &bytes.Buffer{}
	recoveryConfig := RecoveryConfig{
		EnableAutoRecovery:      true,
		MaxAutoRecoveryAttempts: 2,
		AutoRecoveryDelay:       10 * time.Millisecond,
		EnablePreflightChecks:   false,
		GenerateRecoveryScript:  false,
	}

	mgr := NewRecoveryManager(recoveryConfig, output, true)

	// Test timeout error classification and recovery
	err := errors.New("context deadline exceeded")
	bErr := ClassifyError(err, PhaseInstall)

	if bErr.Category != ErrorCategoryTimeout {
		t.Errorf("Expected timeout category, got %s", bErr.Category)
	}

	if !bErr.IsRetryable() {
		t.Error("Timeout errors should be retryable")
	}

	// Attempt automatic recovery
	results := mgr.AttemptAutomaticRecovery(context.Background(), bErr)
	if len(results) == 0 {
		t.Error("Expected recovery results for timeout error")
	}

	// Verify retry-operation action was attempted
	found := false
	for _, r := range results {
		if r.ActionID == "retry-operation" {
			found = true
			if !r.Success {
				t.Error("Retry operation should succeed (no commands to execute)")
			}
		}
	}
	if !found {
		t.Error("Expected retry-operation action to be attempted")
	}
}

// TestRecoveryIntegration_PreflightBlocksPhase tests that required preflight
// checks can block phase execution.
func TestRecoveryIntegration_PreflightBlocksPhase(t *testing.T) {
	output := &bytes.Buffer{}
	checker := &PreflightChecker{
		output:  output,
		verbose: true,
	}

	// Add a failing required check
	checker.checks = []PreflightCheck{
		{
			Name:        "custom-requirement",
			Description: "Custom requirement check",
			Required:    true,
			Check: func(ctx context.Context, state *State) error {
				return errors.New("requirement not met")
			},
		},
	}

	state := &State{}
	results, err := checker.RunChecks(context.Background(), state, PhaseInstall)

	if err == nil {
		t.Error("Expected error from failing required preflight check")
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	if results[0].Passed {
		t.Error("Failed check should not be marked as passed")
	}

	if !strings.Contains(err.Error(), "custom-requirement") {
		t.Errorf("Error should mention failed check: %v", err)
	}
}

// TestRecoveryIntegration_PreflightAutoFix tests that preflight checks
// can auto-fix issues and allow phases to proceed.
func TestRecoveryIntegration_PreflightAutoFix(t *testing.T) {
	output := &bytes.Buffer{}
	checker := &PreflightChecker{
		output:  output,
		verbose: true,
	}

	fixAttempted := false
	checker.checks = []PreflightCheck{
		{
			Name:        "fixable-issue",
			Description: "Issue that can be auto-fixed",
			Required:    true,
			Check: func(ctx context.Context, state *State) error {
				return errors.New("issue detected")
			},
			AutoFix: func(ctx context.Context, state *State) error {
				fixAttempted = true
				return nil // Fix succeeds
			},
		},
	}

	state := &State{}
	results, err := checker.RunChecks(context.Background(), state, PhaseInstall)

	if err != nil {
		t.Errorf("Should not error when auto-fix succeeds: %v", err)
	}

	if !fixAttempted {
		t.Error("Auto-fix should have been attempted")
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}

	if !results[0].Fixed {
		t.Error("Result should indicate issue was fixed")
	}

	if !results[0].Passed {
		t.Error("Result should pass after auto-fix")
	}
}

// TestRecoveryIntegration_RecoveryScriptGeneration tests that recovery scripts
// are correctly generated for different error types.
func TestRecoveryIntegration_RecoveryScriptGeneration(t *testing.T) {
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "recovery.sh")

	recoveryConfig := RecoveryConfig{
		GenerateRecoveryScript: true,
		RecoveryScriptPath:     scriptPath,
	}

	mgr := NewRecoveryManager(recoveryConfig, nil, false)

	// Test with permission error
	err := errors.New("permission denied: cannot write to /etc/keystone-core")
	bErr := ClassifyError(err, PhaseInstall)

	path, err := mgr.GenerateRecoveryScript(bErr)
	if err != nil {
		t.Fatalf("GenerateRecoveryScript failed: %v", err)
	}

	if path != scriptPath {
		t.Errorf("Path = %s, want %s", path, scriptPath)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read script: %v", err)
	}

	script := string(content)

	// Verify script structure
	expectations := []string{
		"#!/bin/bash",
		"Error Category: permission",
		"sudo",      // Permission errors suggest sudo
		"chown",     // File permission fixes
		"read -p",   // Manual actions have confirmation
		"y/n",       // Confirmation prompt
		"completed", // Script completion message
	}

	for _, expected := range expectations {
		if !strings.Contains(script, expected) {
			t.Errorf("Script should contain '%s'", expected)
		}
	}

	// Verify script is executable
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if info.Mode().Perm()&0111 == 0 {
		t.Error("Script should be executable")
	}
}

// TestRecoveryIntegration_CheckpointWithRecovery tests checkpoint coordination
// with recovery manager during bootstrap failure.
func TestRecoveryIntegration_CheckpointWithRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	// Setup checkpoint manager
	checkpointMgr := NewCheckpointManager(tmpDir)
	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseConfigure},
		{Name: PhaseInstall},
	}

	err := checkpointMgr.Initialize(DeploymentModeDemo, nil, phases)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Simulate phase progression
	checkpointMgr.BeginPhase(0)
	checkpointMgr.CompletePhase(0, nil)

	checkpointMgr.BeginPhase(1)
	checkpointMgr.CompletePhase(1, nil)

	checkpointMgr.BeginPhase(2)

	// Simulate failure during install phase
	installErr := errors.New("pq: connection refused to database")
	bErr := ClassifyError(installErr, PhaseInstall)

	// Mark phase as failed
	checkpointMgr.FailPhase(2, installErr)

	// Setup recovery manager
	recoveryConfig := RecoveryConfig{
		EnableAutoRecovery:     true,
		GenerateRecoveryScript: true,
		RecoveryScriptPath:     filepath.Join(tmpDir, "recovery.sh"),
	}
	recoveryMgr := NewRecoveryManager(recoveryConfig, output, true)

	// Generate recovery script
	scriptPath, err := recoveryMgr.GenerateRecoveryScript(bErr)
	if err != nil {
		t.Fatalf("GenerateRecoveryScript failed: %v", err)
	}

	// Verify checkpoint state
	checkpoint := checkpointMgr.GetCheckpoint()
	// After FailPhase, the overall status is set to failed
	if checkpoint.Status != CheckpointStatusFailed {
		t.Errorf("Checkpoint status = %s, want failed", checkpoint.Status)
	}

	// Verify failed phase is recorded
	if checkpoint.Phases[2].Status != CheckpointStatusFailed {
		t.Errorf("Install phase status = %s, want failed", checkpoint.Phases[2].Status)
	}

	// Verify recovery script exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		t.Error("Recovery script should exist")
	}

	// Verify resume point is at failed phase
	resumePoint := checkpointMgr.GetResumePhase()
	if resumePoint != 2 {
		t.Errorf("Resume point = %d, want 2", resumePoint)
	}
}

// TestRecoveryIntegration_DiagnosticsCollection tests enhanced diagnostics
// collection during error scenarios.
func TestRecoveryIntegration_DiagnosticsCollection(t *testing.T) {
	output := &bytes.Buffer{}

	// Create state and recovery manager for diagnostics collector
	state := &State{
		Output:  output,
		Verbose: true,
	}

	recoveryConfig := RecoveryConfig{
		EnableAutoRecovery:     true,
		GenerateRecoveryScript: false,
	}
	recoveryMgr := NewRecoveryManager(recoveryConfig, output, true)

	// Create enhanced diagnostics collector
	collector := NewEnhancedDiagnosticsCollector(state, recoveryMgr, true)

	// Simulate an error scenario
	originalErr := errors.New("connection refused to nats://localhost:4222")

	// Collect diagnostics
	report := collector.Collect(context.Background(), PhaseInstall, originalErr, nil)

	if report == nil {
		t.Fatal("Report should not be nil")
	}

	// Verify report structure
	if report.Phase != PhaseInstall {
		t.Errorf("Phase = %s, want install", report.Phase)
	}

	if report.Error == nil {
		t.Fatal("Error should be captured in report")
	}

	if report.Error.Category != ErrorCategoryNetwork {
		t.Errorf("Error category = %s, want network", report.Error.Category)
	}

	if report.Error.Message == "" {
		t.Error("Error message should be captured")
	}

	// Verify recovery information
	if report.Recovery == nil {
		t.Error("Recovery info should be present")
	}

	if len(report.Recovery.Actions) == 0 {
		t.Error("Recovery actions should be present")
	}
}

// TestRecoveryIntegration_MultipleErrorCategories tests error classification
// and recovery for all major error categories.
func TestRecoveryIntegration_MultipleErrorCategories(t *testing.T) {
	testCases := []struct {
		name             string
		errMsg           string
		expectedCategory ErrorCategory
		expectRetryable  bool
		expectRecovery   bool
	}{
		{
			name:             "permission",
			errMsg:           "operation not permitted",
			expectedCategory: ErrorCategoryPermission,
			expectRetryable:  false,
			expectRecovery:   true,
		},
		{
			name:             "network",
			errMsg:           "connection refused",
			expectedCategory: ErrorCategoryNetwork,
			expectRetryable:  true,
			expectRecovery:   true,
		},
		{
			name:             "timeout",
			errMsg:           "context deadline exceeded",
			expectedCategory: ErrorCategoryTimeout,
			expectRetryable:  true,
			expectRecovery:   true,
		},
		{
			name:             "database",
			errMsg:           "pq: password authentication failed",
			expectedCategory: ErrorCategoryDatabase,
			expectRetryable:  false,
			expectRecovery:   true,
		},
		{
			name:             "tls",
			errMsg:           "x509: certificate has expired",
			expectedCategory: ErrorCategoryTLS,
			expectRetryable:  false,
			expectRecovery:   true,
		},
		{
			name:             "package",
			errMsg:           "apt: unable to locate package",
			expectedCategory: ErrorCategoryPackage,
			expectRetryable:  false,
			expectRecovery:   true,
		},
		{
			name:             "service",
			errMsg:           "systemctl: failed to start service",
			expectedCategory: ErrorCategoryService,
			expectRetryable:  true,
			expectRecovery:   true,
		},
		{
			name:             "resource_disk",
			errMsg:           "no space left on device",
			expectedCategory: ErrorCategoryResource,
			expectRetryable:  false,
			expectRecovery:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := errors.New(tc.errMsg)
			bErr := ClassifyError(err, PhaseInstall)

			if bErr.Category != tc.expectedCategory {
				t.Errorf("Category = %s, want %s", bErr.Category, tc.expectedCategory)
			}

			if bErr.IsRetryable() != tc.expectRetryable {
				t.Errorf("IsRetryable = %v, want %v", bErr.IsRetryable(), tc.expectRetryable)
			}

			if tc.expectRecovery && len(bErr.RecoveryActions) == 0 {
				t.Error("Expected recovery actions")
			}

			if bErr.Suggestion == "" {
				t.Error("Expected suggestion for user")
			}
		})
	}
}

// TestRecoveryIntegration_FullBootstrapWithRecovery tests a complete bootstrap
// flow with error, recovery attempt, and checkpoint coordination.
func TestRecoveryIntegration_FullBootstrapWithRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	atomicConfig := DefaultAtomicConfig()
	atomicConfig.CheckpointDir = tmpDir
	atomicConfig.MaxRetries = 1
	atomicConfig.RetryDelay = 10 * time.Millisecond
	atomicConfig.RollbackTrigger = RollbackManual

	runner := NewAtomicRunner(output, atomicConfig)
	runner.SetMode(DeploymentModeDemo)
	runner.SetVerbose(true)

	// Track execution
	detectRan := false
	installAttempts := 0
	rollbackRan := false

	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				detectRan = true
				return nil
			},
			Rollback: func(ctx context.Context, state *State) error {
				rollbackRan = true
				return nil
			},
		},
		{
			Name: PhaseInstall,
			Run: func(ctx context.Context, state *State) error {
				installAttempts++
				// Simulate transient error that fails on retry too
				return errors.New("connection refused")
			},
		},
	}

	// Run bootstrap - should fail after retries
	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("Expected error from failing phase")
	}

	// Verify execution flow
	if !detectRan {
		t.Error("Detect phase should have run")
	}

	// Should have initial attempt + 1 retry = 2 attempts
	if installAttempts != 2 {
		t.Errorf("Install attempts = %d, want 2", installAttempts)
	}

	// Rollback should NOT have run (RollbackManual)
	if rollbackRan {
		t.Error("Rollback should not run with RollbackManual trigger")
	}

	// Verify checkpoint state
	checkpoint := runner.GetCheckpoint()
	if checkpoint == nil {
		t.Fatal("Checkpoint should exist")
	}

	// Detect should be completed
	if checkpoint.Phases[0].Status != CheckpointStatusCompleted {
		t.Errorf("Detect phase status = %s, want completed", checkpoint.Phases[0].Status)
	}

	// Install should be failed
	if checkpoint.Phases[1].Status != CheckpointStatusFailed {
		t.Errorf("Install phase status = %s, want failed", checkpoint.Phases[1].Status)
	}

	// Error should be classified
	bErr := ClassifyError(err, PhaseInstall)
	if bErr.Category != ErrorCategoryNetwork {
		t.Errorf("Error category = %s, want network", bErr.Category)
	}
}

// TestRecoveryIntegration_RollbackWithArtifacts tests rollback preserves
// and uses artifact information for cleanup.
func TestRecoveryIntegration_RollbackWithArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	atomicConfig := DefaultAtomicConfig()
	atomicConfig.CheckpointDir = tmpDir
	atomicConfig.RollbackTrigger = RollbackOnAnyFailure

	runner := NewAtomicRunner(output, atomicConfig)
	runner.SetMode(DeploymentModeDemo)
	runner.SetVerbose(true)

	var artifactsInRollback *InstallArtifacts

	runner.phases = []Phase{
		{
			Name: PhaseInstall,
			Run: func(ctx context.Context, state *State) error {
				// Simulate installing packages and creating files
				state.InstallArtifacts = &InstallArtifacts{
					Packages:     []string{"kscore-server", "kscore-agent"},
					CreatedFiles: []string{"/etc/keystone-core/config.yaml", "/var/lib/keystone-core/data.db"},
				}
				return nil
			},
			Rollback: func(ctx context.Context, state *State) error {
				artifactsInRollback = state.InstallArtifacts
				return nil
			},
		},
		{
			Name: PhaseVerify,
			Run: func(ctx context.Context, state *State) error {
				return errors.New("verification failed")
			},
		},
	}

	// Run and trigger rollback
	_ = runner.Run(context.Background())

	// Verify artifacts were available during rollback
	if artifactsInRollback == nil {
		t.Error("Artifacts should be available during rollback")
	}

	if len(artifactsInRollback.Packages) != 2 {
		t.Errorf("Expected 2 packages in rollback, got %d", len(artifactsInRollback.Packages))
	}

	if len(artifactsInRollback.CreatedFiles) != 2 {
		t.Errorf("Expected 2 files in rollback, got %d", len(artifactsInRollback.CreatedFiles))
	}
}
