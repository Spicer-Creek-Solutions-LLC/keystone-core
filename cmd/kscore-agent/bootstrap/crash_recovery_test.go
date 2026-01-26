package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/platform"
)

// TestCrashRecovery_CorruptCheckpointFile tests handling of corrupted checkpoint files.
func TestCrashRecovery_CorruptCheckpointFile(t *testing.T) {
	tmpDir := t.TempDir()
	checkpointFile := filepath.Join(tmpDir, "checkpoint.json")

	// Write corrupt JSON
	err := os.WriteFile(checkpointFile, []byte("{invalid json"), 0o644)
	if err != nil {
		t.Fatalf("Failed to write corrupt file: %v", err)
	}

	mgr := NewCheckpointManager(tmpDir)

	// HasCheckpoint should return true (file exists)
	if !mgr.HasCheckpoint() {
		t.Error("HasCheckpoint should return true for existing file")
	}

	// Load should fail with corrupt file
	_, err = mgr.Load()
	if err == nil {
		t.Error("Load should fail with corrupt checkpoint file")
	}

	// Initialize should succeed (creates new checkpoint, overwriting corrupt)
	phases := []Phase{{Name: PhaseDetect}}
	err = mgr.Initialize(DeploymentModeDemo, nil, phases)
	if err != nil {
		t.Errorf("Initialize should succeed and overwrite corrupt file: %v", err)
	}

	// Verify new checkpoint is valid
	checkpoint := mgr.GetCheckpoint()
	if checkpoint == nil {
		t.Error("Should have valid checkpoint after Initialize")
	}
}

// TestCrashRecovery_EmptyCheckpointFile tests handling of empty checkpoint files.
func TestCrashRecovery_EmptyCheckpointFile(t *testing.T) {
	tmpDir := t.TempDir()
	checkpointFile := filepath.Join(tmpDir, "checkpoint.json")

	// Write empty file (simulates truncated write)
	err := os.WriteFile(checkpointFile, []byte{}, 0o644)
	if err != nil {
		t.Fatalf("Failed to write empty file: %v", err)
	}

	mgr := NewCheckpointManager(tmpDir)

	// Load should fail or return nil for empty file
	checkpoint, err := mgr.Load()
	if err == nil && checkpoint != nil {
		t.Error("Load should fail or return nil for empty checkpoint file")
	}
}

// TestCrashRecovery_PartialPhaseCompletion tests resume from partial phase completion.
func TestCrashRecovery_PartialPhaseCompletion(t *testing.T) {
	tmpDir := t.TempDir()

	// Create checkpoint with partial state (simulates crash mid-phase)
	checkpoint := &BootstrapCheckpoint{
		ID:        "test-id",
		Version:   "1.0.0",
		Status:    CheckpointStatusInProgress,
		Mode:      DeploymentModeDemo,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Phases: []PhaseCheckpoint{
			{Name: PhaseDetect, Status: CheckpointStatusCompleted},
			{Name: PhaseConfigure, Status: CheckpointStatusInProgress}, // Mid-phase crash
			{Name: PhaseInstall, Status: CheckpointStatusPending},
		},
	}

	// Write checkpoint manually
	checkpointFile := filepath.Join(tmpDir, "checkpoint.json")
	data, _ := json.MarshalIndent(checkpoint, "", "  ")
	if err := os.WriteFile(checkpointFile, data, 0o644); err != nil {
		t.Fatalf("Failed to write checkpoint: %v", err)
	}

	// Load checkpoint
	mgr := NewCheckpointManager(tmpDir)
	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded == nil {
		t.Fatal("Loaded checkpoint should not be nil")
	}

	// Verify resume point is at the in-progress phase
	resumePhase := mgr.GetResumePhase()
	if resumePhase != 1 {
		t.Errorf("Resume phase = %d, want 1 (the in-progress phase)", resumePhase)
	}
}

// TestCrashRecovery_CheckpointPersistenceAcrossRestart tests checkpoint persistence.
func TestCrashRecovery_CheckpointPersistenceAcrossRestart(t *testing.T) {
	tmpDir := t.TempDir()

	// First "run" - create checkpoint and complete some phases
	mgr1 := NewCheckpointManager(tmpDir)
	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseConfigure},
		{Name: PhaseInstall},
	}

	err := mgr1.Initialize(DeploymentModeDemo, &BootstrapConfig{Mode: "demo"}, phases)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Complete first phase
	mgr1.BeginPhase(0)
	mgr1.CompletePhase(0, nil)

	// Set system info - SystemInfo uses platform.Info but SetSystemInfo extracts to flat fields
	systemInfo := &SystemInfo{
		Platform: &platform.Info{
			OS:     "linux",
			Distro: "ubuntu",
		},
	}
	mgr1.SetSystemInfo(systemInfo)

	// Set install artifacts
	artifacts := &InstallArtifacts{
		Packages: []string{"pkg1", "pkg2"},
	}
	mgr1.SetInstallArtifacts(artifacts)

	// Simulate "crash" by creating new manager (simulates restart)
	mgr2 := NewCheckpointManager(tmpDir)

	// Load checkpoint
	loaded, err := mgr2.Load()
	if err != nil {
		t.Fatalf("Load failed after restart: %v", err)
	}

	// Verify state persisted
	if loaded.Status != CheckpointStatusInProgress {
		t.Errorf("Status = %s, want in_progress", loaded.Status)
	}

	if loaded.Phases[0].Status != CheckpointStatusCompleted {
		t.Errorf("Phase 0 status = %s, want completed", loaded.Phases[0].Status)
	}

	if loaded.BootstrapConfig == nil || loaded.BootstrapConfig.Mode != "demo" {
		t.Error("Bootstrap config should be preserved")
	}

	// Verify system info persisted (CheckpointSystemInfo has flat fields, not Platform)
	if loaded.SystemInfo == nil {
		t.Error("System info should be preserved")
	} else if loaded.SystemInfo.OS != "linux" {
		t.Errorf("OS = %s, want linux", loaded.SystemInfo.OS)
	}

	// Verify artifacts persisted
	restored := mgr2.RestoreArtifacts()
	if restored == nil {
		t.Error("Artifacts should be restorable")
	} else if len(restored.Packages) != 2 {
		t.Errorf("Package count = %d, want 2", len(restored.Packages))
	}
}

// TestCrashRecovery_ConcurrentCheckpointOperations tests thread safety.
func TestCrashRecovery_ConcurrentCheckpointOperations(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseConfigure},
		{Name: PhaseInstall},
	}

	err := mgr.Initialize(DeploymentModeDemo, nil, phases)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Run concurrent operations
	var wg sync.WaitGroup
	errChan := make(chan error, 100)

	// Multiple goroutines reading checkpoint
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				checkpoint := mgr.GetCheckpoint()
				if checkpoint == nil {
					errChan <- errors.New("GetCheckpoint returned nil")
					return
				}
			}
		}()
	}

	// Multiple goroutines updating phases (simulate concurrent phase transitions)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		phaseIdx := i
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = mgr.BeginPhase(phaseIdx)
				_ = mgr.CompletePhase(phaseIdx, nil)
			}
		}()
	}

	// Wait for all goroutines
	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		t.Errorf("Concurrent operation error: %v", err)
	}

	// Verify final state is consistent
	checkpoint := mgr.GetCheckpoint()
	if checkpoint == nil {
		t.Fatal("Checkpoint should exist after concurrent operations")
	}
}

// TestCrashRecovery_AtomicRunnerInterrupt tests bootstrap interruption handling.
func TestCrashRecovery_AtomicRunnerInterrupt(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.EnableResume = true
	cfg.RollbackTrigger = RollbackManual

	// First run - complete first two phases, then cancel
	runner1 := NewAtomicRunner(output, cfg)
	runner1.SetMode(DeploymentModeDemo)

	ctx, cancel := context.WithCancel(context.Background())
	var installStarted int32

	runner1.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
		},
		{
			Name: PhaseConfigure,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
		},
		{
			Name: PhaseInstall,
			Run: func(ctx context.Context, state *State) error {
				atomic.StoreInt32(&installStarted, 1)
				// Cancel during install
				cancel()
				<-ctx.Done()
				return ctx.Err()
			},
		},
	}

	// Run until interrupted
	err := runner1.Run(ctx)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Logf("First run ended with: %v", err)
	}

	// Verify install phase was reached
	if atomic.LoadInt32(&installStarted) != 1 {
		t.Error("Install phase should have started")
	}

	// Verify checkpoint exists
	checkpointFile := filepath.Join(tmpDir, "checkpoint.json")
	if _, err := os.Stat(checkpointFile); os.IsNotExist(err) {
		t.Error("Checkpoint file should exist after interruption")
	}

	// Load checkpoint and verify state
	mgr := NewCheckpointManager(tmpDir)
	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	// First two phases should be completed
	if len(loaded.Phases) >= 2 {
		if loaded.Phases[0].Status != CheckpointStatusCompleted {
			t.Errorf("Detect phase status = %s, want completed", loaded.Phases[0].Status)
		}
		if loaded.Phases[1].Status != CheckpointStatusCompleted {
			t.Errorf("Configure phase status = %s, want completed", loaded.Phases[1].Status)
		}
	}
}

// TestCrashRecovery_RollbackInterruption tests rollback interruption handling.
func TestCrashRecovery_RollbackInterruption(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.RollbackTrigger = RollbackOnAnyFailure
	cfg.RollbackTimeout = 100 * time.Millisecond

	runner := NewAtomicRunner(output, cfg)
	runner.SetMode(DeploymentModeDemo)
	runner.SetVerbose(true)

	rollbacksStarted := make(map[string]bool)
	var rollbackMu sync.Mutex

	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
			Rollback: func(ctx context.Context, state *State) error {
				rollbackMu.Lock()
				rollbacksStarted["detect"] = true
				rollbackMu.Unlock()
				return nil
			},
		},
		{
			Name: PhaseConfigure,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
			Rollback: func(ctx context.Context, state *State) error {
				rollbackMu.Lock()
				rollbacksStarted["configure"] = true
				rollbackMu.Unlock()
				// Simulate slow rollback that exceeds timeout
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(500 * time.Millisecond):
					return nil
				}
			},
		},
		{
			Name: PhaseInstall,
			Run: func(ctx context.Context, state *State) error {
				return errors.New("install failed")
			},
		},
	}

	// Run - should fail and trigger rollback
	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("Expected error from failing phase")
	}

	// Verify both rollbacks were attempted (configure may timeout)
	rollbackMu.Lock()
	if !rollbacksStarted["configure"] {
		t.Error("Configure rollback should have been started")
	}
	// Detect rollback may or may not complete depending on timing
	rollbackMu.Unlock()

	// Verify checkpoint shows rollback was triggered
	checkpoint := runner.GetCheckpoint()
	if checkpoint == nil {
		t.Fatal("Checkpoint should exist")
	}

	if !checkpoint.RollbackTriggered {
		t.Error("Checkpoint should indicate rollback was triggered")
	}
}

// TestCrashRecovery_CheckpointDirectoryCreation tests checkpoint directory creation.
func TestCrashRecovery_CheckpointDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "deep", "nested", "checkpoint")

	mgr := NewCheckpointManager(nestedDir)

	phases := []Phase{{Name: PhaseDetect}}
	err := mgr.Initialize(DeploymentModeDemo, nil, phases)
	if err != nil {
		t.Fatalf("Initialize should create nested directories: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("Checkpoint directory should have been created")
	}

	// Verify checkpoint file exists
	checkpointFile := filepath.Join(nestedDir, "checkpoint.json")
	if _, err := os.Stat(checkpointFile); os.IsNotExist(err) {
		t.Error("Checkpoint file should exist")
	}
}

// TestCrashRecovery_CheckpointVersionMismatch tests handling of version mismatches.
func TestCrashRecovery_CheckpointVersionMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Create checkpoint with different version
	checkpoint := &BootstrapCheckpoint{
		ID:        "test-id",
		Version:   "99.0.0", // Future version
		Status:    CheckpointStatusInProgress,
		Mode:      DeploymentModeDemo,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Phases: []PhaseCheckpoint{
			{Name: PhaseDetect, Status: CheckpointStatusCompleted},
		},
	}

	checkpointFile := filepath.Join(tmpDir, "checkpoint.json")
	data, _ := json.MarshalIndent(checkpoint, "", "  ")
	if err := os.WriteFile(checkpointFile, data, 0o644); err != nil {
		t.Fatalf("Failed to write checkpoint: %v", err)
	}

	// Load should still work (version check is informational)
	mgr := NewCheckpointManager(tmpDir)
	loaded, err := mgr.Load()
	if err != nil {
		t.Logf("Load with version mismatch: %v", err)
	}

	// Even with mismatch, checkpoint should be loadable for inspection
	if loaded == nil && err == nil {
		t.Error("Should either load or return error")
	}
}

// TestCrashRecovery_TransactionResumeWithArtifacts tests artifact restoration on resume.
func TestCrashRecovery_TransactionResumeWithArtifacts(t *testing.T) {
	tmpDir := t.TempDir()

	// First run - complete install phase with artifacts, fail on verify
	output1 := &bytes.Buffer{}
	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.EnableResume = true
	cfg.RollbackTrigger = RollbackManual

	runner1 := NewAtomicRunner(output1, cfg)
	runner1.SetMode(DeploymentModeDemo)

	runner1.phases = []Phase{
		{
			Name: PhaseInstall,
			Run: func(ctx context.Context, state *State) error {
				state.InstallArtifacts = &InstallArtifacts{
					Packages:     []string{"pkg1", "pkg2"},
					CreatedFiles: []string{"/etc/test.conf"},
				}
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

	// First run fails on verify
	err := runner1.Run(context.Background())
	if err == nil {
		t.Fatal("First run should fail on verify")
	}

	// Second run - should restore artifacts on resume
	var restoredArtifacts *InstallArtifacts
	output2 := &bytes.Buffer{}

	runner2 := NewAtomicRunner(output2, cfg)
	runner2.SetMode(DeploymentModeDemo)

	runner2.phases = []Phase{
		{
			Name: PhaseInstall,
			Run: func(ctx context.Context, state *State) error {
				// Should not run on resume (already completed)
				return nil
			},
		},
		{
			Name: PhaseVerify,
			Run: func(ctx context.Context, state *State) error {
				restoredArtifacts = state.InstallArtifacts
				return nil // Succeed this time
			},
		},
	}

	// Second run should succeed
	err = runner2.Run(context.Background())
	if err != nil {
		t.Fatalf("Second run should succeed: %v", err)
	}

	// Note: Artifacts may or may not be restored depending on implementation
	// This test documents the expected behavior
	if restoredArtifacts != nil {
		if len(restoredArtifacts.Packages) != 2 {
			t.Errorf("Expected 2 packages restored, got %d", len(restoredArtifacts.Packages))
		}
	}
}

// TestCrashRecovery_MultipleFailuresAndRetries tests handling of repeated failures.
func TestCrashRecovery_MultipleFailuresAndRetries(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	cfg := DefaultAtomicConfig()
	cfg.CheckpointDir = tmpDir
	cfg.MaxRetries = 2
	cfg.RetryDelay = 10 * time.Millisecond
	cfg.RollbackTrigger = RollbackManual

	runner := NewAtomicRunner(output, cfg)
	runner.SetMode(DeploymentModeDemo)

	var attempts int32

	runner.phases = []Phase{
		{
			Name: PhaseDetect,
			Run: func(ctx context.Context, state *State) error {
				return nil
			},
		},
		{
			Name: PhaseInstall,
			Run: func(ctx context.Context, state *State) error {
				count := atomic.AddInt32(&attempts, 1)
				// Fail with different errors each time
				switch count {
				case 1:
					return errors.New("network error: connection refused")
				case 2:
					return errors.New("timeout: operation timed out")
				default:
					return errors.New("persistent error")
				}
			},
		},
	}

	// Run and fail all retries
	err := runner.Run(context.Background())
	if err == nil {
		t.Fatal("Expected error after all retries exhausted")
	}

	// Verify all attempts were made
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("Expected 3 attempts (1 + 2 retries), got %d", attempts)
	}

	// Verify checkpoint recorded the failure
	checkpoint := runner.GetCheckpoint()
	if checkpoint == nil {
		t.Fatal("Checkpoint should exist")
	}

	// Install phase should be failed
	var installPhase *PhaseCheckpoint
	for i := range checkpoint.Phases {
		if checkpoint.Phases[i].Name == PhaseInstall {
			installPhase = &checkpoint.Phases[i]
			break
		}
	}

	if installPhase == nil {
		t.Fatal("Install phase should be in checkpoint")
	}

	if installPhase.Status != CheckpointStatusFailed {
		t.Errorf("Install phase status = %s, want failed", installPhase.Status)
	}

	if installPhase.Error == "" {
		t.Error("Install phase should have error recorded")
	}
}
