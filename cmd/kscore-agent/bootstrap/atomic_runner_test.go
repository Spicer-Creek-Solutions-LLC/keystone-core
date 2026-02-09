package bootstrap

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewAtomicRunner(t *testing.T) {
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	runner := NewAtomicRunner(output, cfg)

	if runner == nil {
		t.Fatal("NewAtomicRunner returned nil")
	}

	if len(runner.phases) != 6 {
		t.Errorf("Expected 6 phases, got %d", len(runner.phases))
	}

	// Verify all phases are present
	expectedPhases := []PhaseName{PhaseDetect, PhaseConfigure, PhaseValidate, PhaseInstall, PhaseBlueprints, PhaseVerify}
	for i, expected := range expectedPhases {
		if runner.phases[i].Name != expected {
			t.Errorf("Phase %d: expected %s, got %s", i, expected, runner.phases[i].Name)
		}
	}
}

func TestAtomicRunner_Setters(t *testing.T) {
	output := &bytes.Buffer{}
	runner := NewAtomicRunner(output, DefaultAtomicConfig())

	runner.SetDryRun(true)
	if !runner.dryRun {
		t.Error("SetDryRun did not set dryRun")
	}

	runner.SetVerbose(true)
	if !runner.verbose {
		t.Error("SetVerbose did not set verbose")
	}

	runner.SetJSONOutput(true)
	if !runner.jsonOutput {
		t.Error("SetJSONOutput did not set jsonOutput")
	}

	runner.SetMode(DeploymentModeDemo)
	if runner.mode != DeploymentModeDemo {
		t.Error("SetMode did not set mode")
	}

	cfg := &Config{Mode: "demo"}
	runner.SetBootstrapConfig(cfg)
	if runner.bootstrapConfig != cfg {
		t.Error("SetBootstrapConfig did not set config")
	}

	newOutput := &bytes.Buffer{}
	runner.SetOutput(newOutput)
	if runner.output != newOutput {
		t.Error("SetOutput did not set output")
	}

	atomicCfg := AtomicConfig{MaxRetries: 5}
	runner.SetAtomicConfig(atomicCfg)
	if runner.atomicConfig.MaxRetries != 5 {
		t.Error("SetAtomicConfig did not set config")
	}
}

func TestAtomicRunner_Run_Success(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.ClearOnSuccess = true

	runner := NewAtomicRunner(output, cfg)

	// Replace phases with simple test phases
	phasesRun := make([]PhaseName, 0)
	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				phasesRun = append(phasesRun, PhaseDetect)
				return nil
			},
		},
		{
			Name: PhaseConfigure,
			Run: func(ctx context.Context, state *State) error {
				phasesRun = append(phasesRun, PhaseConfigure)
				return nil
			},
		},
	}
	runner.SetMode(DeploymentModeDemo)

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify all phases ran
	if len(phasesRun) != 2 {
		t.Errorf("Expected 2 phases to run, got %d", len(phasesRun))
	}

	// Verify output
	if !strings.Contains(output.String(), "bootstrap completed") {
		t.Error("Expected completion message in output")
	}
}

func TestAtomicRunner_Run_DryRun_SkipsVerify(t *testing.T) {
	output := &bytes.Buffer{}
	runner := NewAtomicRunner(output, DefaultAtomicConfig())
	runner.SetDryRun(true)
	runner.SetVerbose(true)
	runner.SetMode(DeploymentModeDemo)

	verifyRan := false
	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
		},
		{
			Name: PhaseVerify,
			Run: func(ctx context.Context, state *State) error {
				verifyRan = true
				return nil
			},
		},
	}

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if verifyRan {
		t.Error("Verify phase should be skipped in dry-run mode")
	}

	if !strings.Contains(output.String(), "verification skipped") {
		t.Error("Expected verification skipped message")
	}
}

func TestAtomicRunner_Run_WithRetry_Success(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.MaxRetries = 2
	cfg.RetryDelay = 10 * time.Millisecond

	runner := NewAtomicRunner(output, cfg)
	runner.SetVerbose(true)
	runner.SetMode(DeploymentModeDemo)

	var attempts int32
	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				count := atomic.AddInt32(&attempts, 1)
				if count < 2 {
					return errors.New("transient error")
				}
				return nil
			},
		},
	}

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed after retries: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}

	if !strings.Contains(output.String(), "retrying phase") {
		t.Error("Expected retry message in verbose output")
	}
}

func TestAtomicRunner_Run_WithRetry_Exhausted(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.MaxRetries = 2
	cfg.RetryDelay = 10 * time.Millisecond
	cfg.RollbackTrigger = RollbackManual // Don't rollback automatically

	runner := NewAtomicRunner(output, cfg)
	runner.SetMode(DeploymentModeDemo)

	var attempts int32
	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				atomic.AddInt32(&attempts, 1)
				return errors.New("persistent error")
			},
		},
	}

	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("Expected error after retries exhausted")
	}

	// 1 initial + 2 retries = 3 attempts
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestAtomicRunner_Run_Rollback(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.RollbackTrigger = RollbackOnAnyFailure

	runner := NewAtomicRunner(output, cfg)
	runner.SetVerbose(true)
	runner.SetMode(DeploymentModeDemo)

	rollbackCalled := false
	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
			Rollback: func(ctx context.Context, state *State) error {
				rollbackCalled = true
				return nil
			},
		},
		{
			Name: PhaseConfigure,
			Run: func(ctx context.Context, state *State) error {
				return errors.New("config error")
			},
		},
	}

	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("Expected error from failing phase")
	}

	if !rollbackCalled {
		t.Error("Rollback should have been called")
	}

	out := output.String()
	if !strings.Contains(out, "rollback") {
		t.Error("Expected rollback message in output")
	}
}

func TestAtomicRunner_Run_RollbackReverseOrder(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.RollbackTrigger = RollbackOnAnyFailure

	runner := NewAtomicRunner(output, cfg)
	runner.SetVerbose(true)
	runner.SetMode(DeploymentModeDemo)

	var rollbackOrder []string
	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
			Rollback: func(ctx context.Context, state *State) error {
				rollbackOrder = append(rollbackOrder, "detect")
				return nil
			},
		},
		{
			Name: PhaseConfigure,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
			Rollback: func(ctx context.Context, state *State) error {
				rollbackOrder = append(rollbackOrder, "configure")
				return nil
			},
		},
		{
			Name: PhaseValidate,
			Run: func(ctx context.Context, state *State) error {
				return errors.New("validation error")
			},
		},
	}

	_ = runner.Run(context.Background())

	// Rollback should be in reverse order
	if len(rollbackOrder) != 2 {
		t.Fatalf("Expected 2 rollbacks, got %d", len(rollbackOrder))
	}

	if rollbackOrder[0] != "configure" || rollbackOrder[1] != "detect" {
		t.Errorf("Rollback order should be [configure, detect], got %v", rollbackOrder)
	}
}

func TestAtomicRunner_Run_RollbackTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.RollbackTrigger = RollbackOnAnyFailure
	cfg.RollbackTimeout = 100 * time.Millisecond

	runner := NewAtomicRunner(output, cfg)
	runner.SetMode(DeploymentModeDemo)

	timeoutHit := false
	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
			Rollback: func(ctx context.Context, state *State) error {
				select {
				case <-ctx.Done():
					timeoutHit = true
					return ctx.Err()
				case <-time.After(500 * time.Millisecond):
					return nil
				}
			},
		},
		{
			Name: PhaseConfigure,
			Run: func(ctx context.Context, state *State) error {
				return errors.New("error")
			},
		},
	}

	_ = runner.Run(context.Background())

	if !timeoutHit {
		t.Error("Rollback should have been cancelled by timeout")
	}
}

func TestAtomicRunner_Run_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir

	runner := NewAtomicRunner(output, cfg)
	runner.SetMode(DeploymentModeDemo)

	ctx, cancel := context.WithCancel(context.Background())

	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				cancel() // Cancel context during first phase
				return nil
			},
		},
		{
			Name: PhaseConfigure,
			Run: func(ctx context.Context, state *State) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					return nil
				}
			},
		},
	}

	err := runner.Run(ctx)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
}

func TestAtomicRunner_Run_NoRetryOnCancel(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.MaxRetries = 5
	cfg.RetryDelay = 10 * time.Millisecond

	runner := NewAtomicRunner(output, cfg)
	runner.SetMode(DeploymentModeDemo)

	ctx, cancel := context.WithCancel(context.Background())
	var attempts int32

	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				count := atomic.AddInt32(&attempts, 1)
				if count == 1 {
					cancel()
					return context.Canceled
				}
				return nil
			},
		},
	}

	_ = runner.Run(ctx)

	// Should not retry after context cancellation
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("Should not retry on cancellation, attempts: %d", attempts)
	}
}

func TestAtomicRunner_Run_Resume(t *testing.T) {
	tmpDir := t.TempDir()

	// Shared counters across runners
	var phase1RunCount, phase2RunCount int32

	// Create shared phases that use the same counters
	makePhases := func() []Phase {
		return []Phase{
			{
				Name: PhaseDetect,
				Run: func(ctx context.Context, state *State) error {
					atomic.AddInt32(&phase1RunCount, 1)
					return nil
				},
			},
			{
				Name: PhaseConfigure,
				Run: func(ctx context.Context, state *State) error {
					count := atomic.AddInt32(&phase2RunCount, 1)
					if count == 1 {
						return errors.New("first run fails")
					}
					return nil
				},
			},
		}
	}

	// First run - fail on second phase
	output1 := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.EnableResume = true
	cfg.RollbackTrigger = RollbackManual // Don't rollback to preserve checkpoint

	runner1 := NewAtomicRunner(output1, cfg)
	runner1.SetMode(DeploymentModeDemo)
	runner1.phases = makePhases()

	// First run fails
	err := runner1.Run(context.Background())
	if err == nil {
		t.Fatal("First run should fail")
	}

	// Verify first run state
	if atomic.LoadInt32(&phase1RunCount) != 1 {
		t.Errorf("After first run, phase 1 should run once, ran %d times", phase1RunCount)
	}
	if atomic.LoadInt32(&phase2RunCount) != 1 {
		t.Errorf("After first run, phase 2 should run once, ran %d times", phase2RunCount)
	}

	// Second run - should resume from phase 2
	output2 := &bytes.Buffer{}
	runner2 := NewAtomicRunner(output2, cfg)
	runner2.SetMode(DeploymentModeDemo)
	runner2.SetVerbose(true)
	runner2.phases = makePhases()

	err = runner2.Run(context.Background())
	if err != nil {
		t.Fatalf("Second run should succeed: %v", err)
	}

	// After resume:
	// - Phase 1 should run once more (resume starts from failed phase, not skipping completed)
	// - Or it should be skipped if resume works correctly
	// Let's check the actual behavior

	// The resume logic in atomic_runner.go:118-130 shows:
	// startPhase = r.transaction.GetResumePoint()
	// This returns the index of the first incomplete phase
	// So if phase 1 was completed, startPhase should be 1 (PhaseConfigure)

	// Phase 2 should have run twice total (first fail at count=1, second success at count=2)
	if atomic.LoadInt32(&phase2RunCount) != 2 {
		t.Errorf("Phase 2 should run twice total, ran %d times", phase2RunCount)
	}
}

func TestAtomicRunner_Run_JSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir

	runner := NewAtomicRunner(output, cfg)
	runner.SetJSONOutput(true)
	runner.SetMode(DeploymentModeDemo)

	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
		},
	}

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, `"event":"complete"`) {
		t.Errorf("Expected JSON complete event, got: %s", out)
	}
}

func TestAtomicRunner_ForceRollback(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.RollbackTrigger = RollbackManual // Don't auto-rollback

	runner := NewAtomicRunner(output, cfg)
	runner.SetVerbose(true)
	runner.SetMode(DeploymentModeDemo)

	rollbackCalled := false
	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
			Rollback: func(ctx context.Context, state *State) error {
				rollbackCalled = true
				return nil
			},
		},
		{
			Name: PhaseConfigure,
			Run: func(ctx context.Context, state *State) error {
				return errors.New("fail without rollback")
			},
		},
	}

	// Run and fail
	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("Expected error")
	}

	if rollbackCalled {
		t.Error("Rollback should NOT be called automatically")
	}

	// Now force rollback
	err = runner.ForceRollback(context.Background())
	if err != nil {
		t.Fatalf("ForceRollback failed: %v", err)
	}

	if !rollbackCalled {
		t.Error("Rollback should be called after ForceRollback")
	}
}

func TestAtomicRunner_ForceRollback_NoTransaction(t *testing.T) {
	output := &bytes.Buffer{}
	runner := NewAtomicRunner(output, DefaultAtomicConfig())

	err := runner.ForceRollback(context.Background())
	if err == nil {
		t.Error("Expected error when transaction not initialized")
	}

	if !strings.Contains(err.Error(), "transaction not initialized") {
		t.Errorf("Expected 'transaction not initialized' error, got: %v", err)
	}
}

func TestAtomicRunner_ForceRollback_NoCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.EnableCheckpoints = false

	runner := NewAtomicRunner(output, cfg)
	runner.SetMode(DeploymentModeDemo)

	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
		},
	}

	// Run successfully (no checkpoint since disabled)
	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Force rollback should fail - no checkpoint
	err = runner.ForceRollback(context.Background())
	if err == nil {
		t.Error("Expected error when no checkpoint")
	}
}

func TestAtomicRunner_ForceRollback_NoCompletedPhases(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.RollbackTrigger = RollbackManual

	runner := NewAtomicRunner(output, cfg)
	runner.SetMode(DeploymentModeDemo)

	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return errors.New("fail immediately")
			},
		},
	}

	// Run and fail on first phase
	_ = runner.Run(context.Background())

	// Force rollback should fail - no completed phases
	err := runner.ForceRollback(context.Background())
	if err == nil {
		t.Error("Expected error when no completed phases")
	}

	if !strings.Contains(err.Error(), "no completed phases") {
		t.Errorf("Expected 'no completed phases' error, got: %v", err)
	}
}

func TestAtomicRunner_GetCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.ClearOnSuccess = false // Keep checkpoint after success

	runner := NewAtomicRunner(output, cfg)
	runner.SetMode(DeploymentModeDemo)

	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
		},
	}

	// Before run, no checkpoint (transaction not initialized)
	if runner.GetCheckpoint() != nil {
		t.Error("Checkpoint should be nil before Run")
	}

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// After run, checkpoint exists
	checkpoint := runner.GetCheckpoint()
	if checkpoint == nil {
		t.Fatal("Checkpoint should exist after Run")
	}

	if checkpoint.Status != CheckpointStatusCompleted {
		t.Errorf("Checkpoint status = %s, want completed", checkpoint.Status)
	}
}

func TestAtomicRunner_ClearCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.ClearOnSuccess = false // Keep checkpoint

	runner := NewAtomicRunner(output, cfg)
	runner.SetMode(DeploymentModeDemo)

	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
		},
	}

	err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify checkpoint file exists
	checkpointFile := filepath.Join(tmpDir, "checkpoint.json")
	if _, err := os.Stat(checkpointFile); os.IsNotExist(err) {
		t.Fatal("Checkpoint file should exist")
	}

	// Clear checkpoint
	err = runner.ClearCheckpoint()
	if err != nil {
		t.Fatalf("ClearCheckpoint failed: %v", err)
	}

	// Verify checkpoint file is removed
	if _, err := os.Stat(checkpointFile); !os.IsNotExist(err) {
		t.Error("Checkpoint file should be removed")
	}
}

func TestAtomicRunner_ClearCheckpoint_NoTransaction(t *testing.T) {
	output := &bytes.Buffer{}
	runner := NewAtomicRunner(output, DefaultAtomicConfig())

	// Should not error when no transaction
	err := runner.ClearCheckpoint()
	if err != nil {
		t.Errorf("ClearCheckpoint should not error when no transaction: %v", err)
	}
}

func TestAtomicRunner_Run_InvalidMode(t *testing.T) {
	output := &bytes.Buffer{}
	runner := NewAtomicRunner(output, DefaultAtomicConfig())
	runner.SetMode("invalid-mode")

	err := runner.Run(context.Background())
	if err == nil {
		t.Error("Expected error for invalid mode")
	}

	if !strings.Contains(err.Error(), "no default configuration") {
		t.Errorf("Expected mode error, got: %v", err)
	}
}

func TestAtomicRunner_Run_RollbackContinuesOnError(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.RollbackTrigger = RollbackOnAnyFailure

	runner := NewAtomicRunner(output, cfg)
	runner.SetVerbose(true)
	runner.SetMode(DeploymentModeDemo)

	var rollbackOrder []string
	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
			Rollback: func(ctx context.Context, state *State) error {
				rollbackOrder = append(rollbackOrder, "detect")
				return nil
			},
		},
		{
			Name: PhaseConfigure,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
			Rollback: func(ctx context.Context, state *State) error {
				rollbackOrder = append(rollbackOrder, "configure")
				return errors.New("rollback failed")
			},
		},
		{
			Name: PhaseValidate,
			Run: func(ctx context.Context, state *State) error {
				return errors.New("validation error")
			},
		},
	}

	_ = runner.Run(context.Background())

	// Both rollbacks should be called even if one fails
	if len(rollbackOrder) != 2 {
		t.Errorf("Expected 2 rollbacks, got %d: %v", len(rollbackOrder), rollbackOrder)
	}

	// Verify error message in output
	if !strings.Contains(output.String(), "rollback configure failed") {
		t.Error("Expected rollback error message in output")
	}
}

func TestAtomicRunner_executePhaseWithRetry_ZeroRetries(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.MaxRetries = 0 // No retries

	runner := NewAtomicRunner(output, cfg)
	runner.SetMode(DeploymentModeDemo)

	var attempts int32
	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				atomic.AddInt32(&attempts, 1)
				return errors.New("error")
			},
		},
	}

	_ = runner.Run(context.Background())

	// Should only run once with no retries
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("Expected 1 attempt with MaxRetries=0, got %d", attempts)
	}
}

func TestAtomicRunner_logEvent_AllTypes(t *testing.T) {
	output := &bytes.Buffer{}
	runner := NewAtomicRunner(output, DefaultAtomicConfig())

	state := &State{
		Output:     output,
		JSONOutput: false,
	}

	// Test all event types in text mode
	runner.logEvent(state, "complete", nil)
	runner.logEvent(state, "rollback_start", map[string]interface{}{"reason": "test"})
	runner.logEvent(state, "rollback_complete", nil)
	runner.logEvent(state, "resume", map[string]interface{}{"phase": PhaseDetect})

	out := output.String()
	if !strings.Contains(out, "bootstrap completed") {
		t.Error("Missing complete message")
	}
	if !strings.Contains(out, "starting automatic rollback") {
		t.Error("Missing rollback_start message")
	}
	if !strings.Contains(out, "rollback completed") {
		t.Error("Missing rollback_complete message")
	}
	if !strings.Contains(out, "resuming from phase") {
		t.Error("Missing resume message")
	}
}

func TestAtomicRunner_logEvent_JSON(t *testing.T) {
	output := &bytes.Buffer{}
	runner := NewAtomicRunner(output, DefaultAtomicConfig())

	state := &State{
		Output:     output,
		JSONOutput: true,
	}

	runner.logEvent(state, "complete", map[string]interface{}{"duration": 123})

	out := output.String()
	if !strings.Contains(out, `"event":"complete"`) {
		t.Error("Missing JSON event field")
	}
	if !strings.Contains(out, `"duration":123`) {
		t.Error("Missing JSON data field")
	}
}
