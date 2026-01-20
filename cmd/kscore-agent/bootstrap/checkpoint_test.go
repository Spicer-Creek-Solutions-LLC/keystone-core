package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/platform"
)

func TestCheckpointManager_Initialize(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseConfigure},
		{Name: PhaseValidate},
		{Name: PhaseInstall},
	}

	cfg := &BootstrapConfig{
		NodeRole: "agent",
	}

	err := mgr.Initialize(DeploymentModeDemo, cfg, phases)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	// Verify checkpoint was created
	checkpoint := mgr.GetCheckpoint()
	if checkpoint == nil {
		t.Fatal("GetCheckpoint() returned nil")
	}

	if checkpoint.Status != CheckpointStatusInProgress {
		t.Errorf("Status = %v, want %v", checkpoint.Status, CheckpointStatusInProgress)
	}

	if checkpoint.Mode != DeploymentModeDemo {
		t.Errorf("Mode = %v, want %v", checkpoint.Mode, DeploymentModeDemo)
	}

	if len(checkpoint.Phases) != len(phases) {
		t.Errorf("Phases count = %d, want %d", len(checkpoint.Phases), len(phases))
	}

	// All phases should be pending initially
	for i, phase := range checkpoint.Phases {
		if phase.Status != CheckpointStatusPending {
			t.Errorf("Phase[%d].Status = %v, want %v", i, phase.Status, CheckpointStatusPending)
		}
	}

	// Verify file was persisted
	path := filepath.Join(tmpDir, CheckpointFileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Checkpoint file was not persisted")
	}
}

func TestCheckpointManager_LoadExisting(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseInstall},
	}

	err := mgr.Initialize(DeploymentModeProduction, nil, phases)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}

	originalID := mgr.GetCheckpoint().ID

	// Create new manager and load
	mgr2 := NewCheckpointManager(tmpDir)
	loaded, err := mgr2.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded == nil {
		t.Fatal("Load() returned nil")
	}

	if loaded.ID != originalID {
		t.Errorf("Loaded ID = %v, want %v", loaded.ID, originalID)
	}

	if loaded.Mode != DeploymentModeProduction {
		t.Errorf("Loaded Mode = %v, want %v", loaded.Mode, DeploymentModeProduction)
	}
}

func TestCheckpointManager_NoCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded != nil {
		t.Error("Load() should return nil when no checkpoint exists")
	}

	if mgr.HasCheckpoint() {
		t.Error("HasCheckpoint() should return false")
	}
}

func TestCheckpointManager_PhaseLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseInstall},
	}

	mgr.Initialize(DeploymentModeDemo, nil, phases)

	// Begin phase
	err := mgr.BeginPhase(0)
	if err != nil {
		t.Fatalf("BeginPhase() error = %v", err)
	}

	checkpoint := mgr.GetCheckpoint()
	if checkpoint.Phases[0].Status != CheckpointStatusInProgress {
		t.Errorf("Phase status = %v, want %v", checkpoint.Phases[0].Status, CheckpointStatusInProgress)
	}

	if checkpoint.Phases[0].StartTime == nil {
		t.Error("Phase StartTime should be set")
	}

	// Complete phase
	artifacts := []string{"/etc/kscore/config.yaml", "/var/lib/kscore/data.db"}
	err = mgr.CompletePhase(0, artifacts)
	if err != nil {
		t.Fatalf("CompletePhase() error = %v", err)
	}

	checkpoint = mgr.GetCheckpoint()
	if checkpoint.Phases[0].Status != CheckpointStatusCompleted {
		t.Errorf("Phase status = %v, want %v", checkpoint.Phases[0].Status, CheckpointStatusCompleted)
	}

	if checkpoint.Phases[0].EndTime == nil {
		t.Error("Phase EndTime should be set")
	}

	if len(checkpoint.Phases[0].Artifacts) != 2 {
		t.Errorf("Phase artifacts count = %d, want 2", len(checkpoint.Phases[0].Artifacts))
	}
}

func TestCheckpointManager_PhaseFailed(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseInstall},
	}

	mgr.Initialize(DeploymentModeDemo, nil, phases)
	mgr.BeginPhase(1)

	// Fail phase
	testErr := &TestError{msg: "simulated install failure"}
	err := mgr.FailPhase(1, testErr)
	if err != nil {
		t.Fatalf("FailPhase() error = %v", err)
	}

	checkpoint := mgr.GetCheckpoint()
	if checkpoint.Phases[1].Status != CheckpointStatusFailed {
		t.Errorf("Phase status = %v, want %v", checkpoint.Phases[1].Status, CheckpointStatusFailed)
	}

	if checkpoint.Phases[1].Error != testErr.Error() {
		t.Errorf("Phase error = %v, want %v", checkpoint.Phases[1].Error, testErr.Error())
	}

	if checkpoint.Status != CheckpointStatusFailed {
		t.Errorf("Overall status = %v, want %v", checkpoint.Status, CheckpointStatusFailed)
	}
}

func TestCheckpointManager_Rollback(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseInstall},
	}

	mgr.Initialize(DeploymentModeDemo, nil, phases)
	mgr.BeginPhase(0)
	mgr.CompletePhase(0, nil)
	mgr.BeginPhase(1)
	mgr.FailPhase(1, nil)

	// Begin rollback
	err := mgr.BeginRollback("test failure")
	if err != nil {
		t.Fatalf("BeginRollback() error = %v", err)
	}

	checkpoint := mgr.GetCheckpoint()
	if !checkpoint.RollbackTriggered {
		t.Error("RollbackTriggered should be true")
	}

	if checkpoint.RollbackReason != "test failure" {
		t.Errorf("RollbackReason = %v, want 'test failure'", checkpoint.RollbackReason)
	}

	if checkpoint.RollbackStartTime == nil {
		t.Error("RollbackStartTime should be set")
	}

	// Mark phase rolled back
	err = mgr.MarkPhaseRolledBack(0)
	if err != nil {
		t.Fatalf("MarkPhaseRolledBack() error = %v", err)
	}

	checkpoint = mgr.GetCheckpoint()
	if checkpoint.Phases[0].Status != CheckpointStatusRolledBack {
		t.Errorf("Phase status = %v, want %v", checkpoint.Phases[0].Status, CheckpointStatusRolledBack)
	}

	// Complete rollback
	err = mgr.CompleteRollback()
	if err != nil {
		t.Fatalf("CompleteRollback() error = %v", err)
	}

	checkpoint = mgr.GetCheckpoint()
	if checkpoint.Status != CheckpointStatusRolledBack {
		t.Errorf("Overall status = %v, want %v", checkpoint.Status, CheckpointStatusRolledBack)
	}

	if checkpoint.RollbackEndTime == nil {
		t.Error("RollbackEndTime should be set")
	}
}

func TestCheckpointManager_GetResumePhase(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*CheckpointManager)
		expected int
	}{
		{
			name: "no checkpoint",
			setup: func(mgr *CheckpointManager) {
				// Don't initialize
			},
			expected: -1,
		},
		{
			name: "all pending",
			setup: func(mgr *CheckpointManager) {
				phases := []Phase{{Name: PhaseDetect}, {Name: PhaseInstall}}
				mgr.Initialize(DeploymentModeDemo, nil, phases)
			},
			expected: 0,
		},
		{
			name: "first completed",
			setup: func(mgr *CheckpointManager) {
				phases := []Phase{{Name: PhaseDetect}, {Name: PhaseInstall}}
				mgr.Initialize(DeploymentModeDemo, nil, phases)
				mgr.BeginPhase(0)
				mgr.CompletePhase(0, nil)
			},
			expected: 1,
		},
		{
			name: "all completed",
			setup: func(mgr *CheckpointManager) {
				phases := []Phase{{Name: PhaseDetect}, {Name: PhaseInstall}}
				mgr.Initialize(DeploymentModeDemo, nil, phases)
				mgr.BeginPhase(0)
				mgr.CompletePhase(0, nil)
				mgr.BeginPhase(1)
				mgr.CompletePhase(1, nil)
			},
			expected: 2,
		},
		{
			name: "rollback triggered",
			setup: func(mgr *CheckpointManager) {
				phases := []Phase{{Name: PhaseDetect}, {Name: PhaseInstall}}
				mgr.Initialize(DeploymentModeDemo, nil, phases)
				mgr.BeginPhase(0)
				mgr.CompletePhase(0, nil)
				mgr.BeginRollback("test")
			},
			expected: -1, // Don't resume after rollback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			mgr := NewCheckpointManager(tmpDir)
			tt.setup(mgr)

			result := mgr.GetResumePhase()
			if result != tt.expected {
				t.Errorf("GetResumePhase() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestCheckpointManager_InstallArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{{Name: PhaseInstall}}
	mgr.Initialize(DeploymentModeDemo, nil, phases)

	artifacts := &InstallArtifacts{
		PackageManager: "apt",
		Packages:       []string{"kscore-server", "kscore-agent"},
		CreatedFiles:   []string{"/etc/kscore/config.yaml"},
	}

	err := mgr.SetInstallArtifacts(artifacts)
	if err != nil {
		t.Fatalf("SetInstallArtifacts() error = %v", err)
	}

	checkpoint := mgr.GetCheckpoint()
	if checkpoint.InstallArtifacts == nil {
		t.Fatal("InstallArtifacts should not be nil")
	}

	if checkpoint.InstallArtifacts.PackageManager != "apt" {
		t.Errorf("PackageManager = %v, want 'apt'", checkpoint.InstallArtifacts.PackageManager)
	}

	if len(checkpoint.InstallArtifacts.Packages) != 2 {
		t.Errorf("Packages count = %d, want 2", len(checkpoint.InstallArtifacts.Packages))
	}

	// Test restore
	restored := mgr.RestoreArtifacts()
	if restored == nil {
		t.Fatal("RestoreArtifacts() returned nil")
	}

	if len(restored.Packages) != 2 {
		t.Errorf("Restored packages count = %d, want 2", len(restored.Packages))
	}
}

func TestCheckpointManager_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{{Name: PhaseDetect}}
	mgr.Initialize(DeploymentModeDemo, nil, phases)

	// Verify checkpoint exists
	if !mgr.HasCheckpoint() {
		t.Fatal("Checkpoint should exist before clear")
	}

	// Clear
	err := mgr.Clear()
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	if mgr.HasCheckpoint() {
		t.Error("Checkpoint should not exist after clear")
	}

	if mgr.GetCheckpoint() != nil {
		t.Error("GetCheckpoint() should return nil after clear")
	}
}

func TestCheckpointManager_Complete(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{{Name: PhaseDetect}}
	mgr.Initialize(DeploymentModeDemo, nil, phases)
	mgr.BeginPhase(0)
	mgr.CompletePhase(0, nil)

	err := mgr.Complete()
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	checkpoint := mgr.GetCheckpoint()
	if checkpoint.Status != CheckpointStatusCompleted {
		t.Errorf("Status = %v, want %v", checkpoint.Status, CheckpointStatusCompleted)
	}
}

func TestCheckpointManager_AtomicPersist(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{{Name: PhaseDetect}}
	mgr.Initialize(DeploymentModeDemo, nil, phases)

	// Verify no temp file remains
	tmpPath := filepath.Join(tmpDir, CheckpointFileName+".tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("Temp file should not remain after persist")
	}
}

func TestCheckpointManager_GetCompletedPhases(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{
		{Name: PhaseDetect},
		{Name: PhaseConfigure},
		{Name: PhaseInstall},
	}

	mgr.Initialize(DeploymentModeDemo, nil, phases)
	mgr.BeginPhase(0)
	mgr.CompletePhase(0, nil)
	mgr.BeginPhase(1)
	mgr.CompletePhase(1, nil)
	// Phase 2 (Install) not completed

	completed := mgr.GetCompletedPhases()
	if len(completed) != 2 {
		t.Errorf("Completed phases count = %d, want 2", len(completed))
	}

	if completed[0] != 0 || completed[1] != 1 {
		t.Errorf("Completed phases = %v, want [0, 1]", completed)
	}
}

func TestCheckpointManager_SetSystemInfo(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{{Name: PhaseDetect}}
	mgr.Initialize(DeploymentModeDemo, nil, phases)

	sysInfo := &SystemInfo{
		Platform: &platform.Info{
			OS:             platform.OSLinux,
			Distro:         platform.DistroUbuntu,
			Arch:           platform.ArchAMD64,
			PackageManager: platform.PackageManagerAPT,
			InitSystem:     platform.InitSystemd,
		},
	}

	err := mgr.SetSystemInfo(sysInfo)
	if err != nil {
		t.Fatalf("SetSystemInfo() error = %v", err)
	}

	checkpoint := mgr.GetCheckpoint()
	if checkpoint.SystemInfo == nil {
		t.Fatal("SystemInfo should not be nil")
	}

	if checkpoint.SystemInfo.OS != "linux" {
		t.Errorf("OS = %v, want 'linux'", checkpoint.SystemInfo.OS)
	}

	if checkpoint.SystemInfo.PackageManager != "apt" {
		t.Errorf("PackageManager = %v, want 'apt'", checkpoint.SystemInfo.PackageManager)
	}
}

// TestError is a helper for testing error handling
type TestError struct {
	msg string
}

func (e *TestError) Error() string {
	return e.msg
}

func TestCheckpointTimestamps(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCheckpointManager(tmpDir)

	phases := []Phase{{Name: PhaseDetect}}

	before := time.Now()
	mgr.Initialize(DeploymentModeDemo, nil, phases)
	after := time.Now()

	checkpoint := mgr.GetCheckpoint()

	if checkpoint.CreatedAt.Before(before) || checkpoint.CreatedAt.After(after) {
		t.Error("CreatedAt should be within test timeframe")
	}

	if checkpoint.UpdatedAt.Before(before) || checkpoint.UpdatedAt.After(after) {
		t.Error("UpdatedAt should be within test timeframe")
	}

	// Update and verify UpdatedAt changes
	time.Sleep(10 * time.Millisecond)
	mgr.BeginPhase(0)

	checkpoint = mgr.GetCheckpoint()
	if !checkpoint.UpdatedAt.After(checkpoint.CreatedAt) {
		t.Error("UpdatedAt should be updated after BeginPhase")
	}
}
