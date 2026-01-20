package bootstrap

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestTransactionManager_Begin(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	config := DefaultAtomicConfig()
	config.CheckpointDir = tmpDir

	txn := NewTransactionManager(config, output, true)

	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseInstall},
	}

	cfg := &BootstrapConfig{NodeRole: "agent"}

	err := txn.Begin(context.Background(), DeploymentModeDemo, cfg, phases)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	checkpoint := txn.GetCheckpoint()
	if checkpoint == nil {
		t.Fatal("GetCheckpoint() returned nil after Begin()")
	}

	if checkpoint.Status != CheckpointStatusInProgress {
		t.Errorf("Status = %v, want %v", checkpoint.Status, CheckpointStatusInProgress)
	}
}

func TestTransactionManager_DisabledCheckpoints(t *testing.T) {
	output := &bytes.Buffer{}

	config := DefaultAtomicConfig()
	config.EnableCheckpoints = false

	txn := NewTransactionManager(config, output, false)

	phases := []Phase{{Name: PhaseDetect}}
	err := txn.Begin(context.Background(), DeploymentModeDemo, nil, phases)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}

	// Should not create checkpoint when disabled
	if txn.GetCheckpoint() != nil {
		t.Error("GetCheckpoint() should return nil when checkpoints disabled")
	}

	// All other operations should be no-ops
	err = txn.BeforePhase(context.Background(), 0, Phase{Name: PhaseDetect})
	if err != nil {
		t.Errorf("BeforePhase() error = %v", err)
	}

	err = txn.AfterPhase(context.Background(), 0, Phase{Name: PhaseDetect}, nil)
	if err != nil {
		t.Errorf("AfterPhase() error = %v", err)
	}

	err = txn.Commit(context.Background())
	if err != nil {
		t.Errorf("Commit() error = %v", err)
	}
}

func TestTransactionManager_PhaseLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	config := DefaultAtomicConfig()
	config.CheckpointDir = tmpDir

	txn := NewTransactionManager(config, output, true)

	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseInstall},
	}

	txn.Begin(context.Background(), DeploymentModeDemo, nil, phases)

	// Before phase
	err := txn.BeforePhase(context.Background(), 0, phases[0])
	if err != nil {
		t.Fatalf("BeforePhase() error = %v", err)
	}

	checkpoint := txn.GetCheckpoint()
	if checkpoint.Phases[0].Status != CheckpointStatusInProgress {
		t.Errorf("Phase status = %v, want %v", checkpoint.Phases[0].Status, CheckpointStatusInProgress)
	}

	// After phase with artifacts
	artifacts := &InstallArtifacts{
		PackageManager: "apt",
		Packages:       []string{"test-pkg"},
		CreatedFiles:   []string{"/tmp/test.conf"},
	}

	err = txn.AfterPhase(context.Background(), 0, phases[0], artifacts)
	if err != nil {
		t.Fatalf("AfterPhase() error = %v", err)
	}

	checkpoint = txn.GetCheckpoint()
	if checkpoint.Phases[0].Status != CheckpointStatusCompleted {
		t.Errorf("Phase status = %v, want %v", checkpoint.Phases[0].Status, CheckpointStatusCompleted)
	}

	if checkpoint.InstallArtifacts == nil {
		t.Error("InstallArtifacts should be set")
	}
}

func TestTransactionManager_ShouldRollback(t *testing.T) {
	tests := []struct {
		name    string
		trigger RollbackTrigger
		phase   Phase
		want    bool
	}{
		{
			name:    "any failure - detect",
			trigger: RollbackOnAnyFailure,
			phase:   Phase{Name: PhaseDetect},
			want:    true,
		},
		{
			name:    "any failure - install",
			trigger: RollbackOnAnyFailure,
			phase:   Phase{Name: PhaseInstall},
			want:    true,
		},
		{
			name:    "install failure - detect",
			trigger: RollbackOnInstallFailure,
			phase:   Phase{Name: PhaseDetect},
			want:    false,
		},
		{
			name:    "install failure - install",
			trigger: RollbackOnInstallFailure,
			phase:   Phase{Name: PhaseInstall},
			want:    true,
		},
		{
			name:    "install failure - verify",
			trigger: RollbackOnInstallFailure,
			phase:   Phase{Name: PhaseVerify},
			want:    true,
		},
		{
			name:    "manual - any phase",
			trigger: RollbackManual,
			phase:   Phase{Name: PhaseInstall},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultAtomicConfig()
			config.RollbackTrigger = tt.trigger
			config.EnableCheckpoints = false

			txn := NewTransactionManager(config, &bytes.Buffer{}, false)

			got := txn.ShouldRollback(tt.phase, nil)
			if got != tt.want {
				t.Errorf("ShouldRollback() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransactionManager_Resume(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	config := DefaultAtomicConfig()
	config.CheckpointDir = tmpDir
	config.EnableResume = true

	// First run - complete one phase
	txn1 := NewTransactionManager(config, output, false)
	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseInstall},
	}

	txn1.Begin(context.Background(), DeploymentModeDemo, nil, phases)
	txn1.BeforePhase(context.Background(), 0, phases[0])
	txn1.AfterPhase(context.Background(), 0, phases[0], nil)

	// Second run - should resume from phase 1
	txn2 := NewTransactionManager(config, output, true)

	// Load existing checkpoint
	txn2.Begin(context.Background(), DeploymentModeDemo, nil, phases)

	resumePoint := txn2.GetResumePoint()
	if resumePoint != 1 {
		t.Errorf("GetResumePoint() = %d, want 1", resumePoint)
	}

	if !txn2.ShouldResume() {
		t.Error("ShouldResume() should return true")
	}
}

func TestTransactionManager_NoResume(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	config := DefaultAtomicConfig()
	config.CheckpointDir = tmpDir
	config.EnableResume = false

	// First run - complete one phase
	txn1 := NewTransactionManager(config, output, false)
	phases := []Phase{{Name: PhaseDetect}, {Name: PhaseInstall}}

	txn1.Begin(context.Background(), DeploymentModeDemo, nil, phases)
	txn1.BeforePhase(context.Background(), 0, phases[0])
	txn1.AfterPhase(context.Background(), 0, phases[0], nil)

	// Second run - resume disabled
	txn2 := NewTransactionManager(config, output, false)

	resumePoint := txn2.GetResumePoint()
	if resumePoint != -1 {
		t.Errorf("GetResumePoint() = %d, want -1 (resume disabled)", resumePoint)
	}
}

func TestTransactionManager_RollbackTracking(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	config := DefaultAtomicConfig()
	config.CheckpointDir = tmpDir

	txn := NewTransactionManager(config, output, true)
	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseInstall},
	}

	txn.Begin(context.Background(), DeploymentModeDemo, nil, phases)
	txn.BeforePhase(context.Background(), 0, phases[0])
	txn.AfterPhase(context.Background(), 0, phases[0], nil)

	// Simulate failure and rollback
	txn.OnPhaseFailure(context.Background(), 1, phases[1], &TestError{msg: "test failure"})
	txn.BeforeRollback(context.Background(), "test failure")
	txn.AfterPhaseRollback(context.Background(), 0, phases[0])
	txn.AfterRollback(context.Background())

	checkpoint := txn.GetCheckpoint()
	if !checkpoint.RollbackTriggered {
		t.Error("RollbackTriggered should be true")
	}

	if checkpoint.RollbackReason != "test failure" {
		t.Errorf("RollbackReason = %v, want 'test failure'", checkpoint.RollbackReason)
	}

	if checkpoint.Status != CheckpointStatusRolledBack {
		t.Errorf("Status = %v, want %v", checkpoint.Status, CheckpointStatusRolledBack)
	}
}

func TestTransactionManager_Commit(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	config := DefaultAtomicConfig()
	config.CheckpointDir = tmpDir
	config.ClearOnSuccess = false // Keep checkpoint for verification

	txn := NewTransactionManager(config, output, false)
	phases := []Phase{{Name: PhaseDetect}}

	txn.Begin(context.Background(), DeploymentModeDemo, nil, phases)
	txn.BeforePhase(context.Background(), 0, phases[0])
	txn.AfterPhase(context.Background(), 0, phases[0], nil)

	err := txn.Commit(context.Background())
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	checkpoint := txn.GetCheckpoint()
	if checkpoint.Status != CheckpointStatusCompleted {
		t.Errorf("Status = %v, want %v", checkpoint.Status, CheckpointStatusCompleted)
	}
}

func TestTransactionManager_CommitWithClear(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	config := DefaultAtomicConfig()
	config.CheckpointDir = tmpDir
	config.ClearOnSuccess = true

	txn := NewTransactionManager(config, output, false)
	phases := []Phase{{Name: PhaseDetect}}

	txn.Begin(context.Background(), DeploymentModeDemo, nil, phases)
	txn.BeforePhase(context.Background(), 0, phases[0])
	txn.AfterPhase(context.Background(), 0, phases[0], nil)

	txn.Commit(context.Background())

	// Checkpoint should be cleared
	mgr := txn.GetCheckpointManager()
	if mgr.HasCheckpoint() {
		t.Error("Checkpoint should be cleared after commit with ClearOnSuccess")
	}
}

func TestTransactionManager_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	config := DefaultAtomicConfig()
	config.CheckpointDir = tmpDir

	txn := NewTransactionManager(config, output, false)
	phases := []Phase{{Name: PhaseDetect}}

	txn.Begin(context.Background(), DeploymentModeDemo, nil, phases)

	err := txn.Clear()
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	mgr := txn.GetCheckpointManager()
	if mgr.HasCheckpoint() {
		t.Error("Checkpoint should not exist after Clear()")
	}
}

func TestTransactionManager_RestoreArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	output := &bytes.Buffer{}

	config := DefaultAtomicConfig()
	config.CheckpointDir = tmpDir

	txn := NewTransactionManager(config, output, false)
	phases := []Phase{{Name: PhaseInstall}}

	txn.Begin(context.Background(), DeploymentModeDemo, nil, phases)

	artifacts := &InstallArtifacts{
		PackageManager: "apt",
		Packages:       []string{"kscore-server"},
		CreatedFiles:   []string{"/etc/kscore/config.yaml"},
	}

	txn.BeforePhase(context.Background(), 0, phases[0])
	txn.AfterPhase(context.Background(), 0, phases[0], artifacts)

	// Simulate new run loading artifacts
	restored := txn.GetRestoredArtifacts()
	if restored == nil {
		t.Fatal("GetRestoredArtifacts() returned nil")
	}

	if len(restored.Packages) != 1 || restored.Packages[0] != "kscore-server" {
		t.Errorf("Restored packages = %v, want ['kscore-server']", restored.Packages)
	}
}

func TestDefaultAtomicConfig(t *testing.T) {
	config := DefaultAtomicConfig()

	if !config.EnableCheckpoints {
		t.Error("EnableCheckpoints should be true by default")
	}

	if config.CheckpointDir != DefaultCheckpointDir {
		t.Errorf("CheckpointDir = %v, want %v", config.CheckpointDir, DefaultCheckpointDir)
	}

	if config.RollbackTrigger != RollbackOnAnyFailure {
		t.Errorf("RollbackTrigger = %v, want %v", config.RollbackTrigger, RollbackOnAnyFailure)
	}

	if !config.EnableResume {
		t.Error("EnableResume should be true by default")
	}

	if !config.ClearOnSuccess {
		t.Error("ClearOnSuccess should be true by default")
	}

	if config.RollbackTimeout != 10*time.Minute {
		t.Errorf("RollbackTimeout = %v, want 10m", config.RollbackTimeout)
	}
}
