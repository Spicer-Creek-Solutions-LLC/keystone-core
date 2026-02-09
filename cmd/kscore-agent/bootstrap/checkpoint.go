package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/platform"
)

const (
	// DefaultCheckpointDir is the default directory for checkpoint files.
	DefaultCheckpointDir = "/var/lib/keystone-core/bootstrap"

	// CheckpointFileName is the name of the checkpoint file.
	CheckpointFileName = "checkpoint.json"
)

// CheckpointStatus represents the status of a checkpoint.
type CheckpointStatus string

const (
	// CheckpointStatusPending indicates the phase hasn't started.
	CheckpointStatusPending CheckpointStatus = "pending"
	// CheckpointStatusInProgress indicates the phase is running.
	CheckpointStatusInProgress CheckpointStatus = "in_progress"
	// CheckpointStatusCompleted indicates the phase completed successfully.
	CheckpointStatusCompleted CheckpointStatus = "completed"
	// CheckpointStatusFailed indicates the phase failed.
	CheckpointStatusFailed CheckpointStatus = "failed"
	// CheckpointStatusRolledBack indicates the phase was rolled back.
	CheckpointStatusRolledBack CheckpointStatus = "rolled_back"
)

// PhaseCheckpoint captures the state of a single bootstrap phase.
type PhaseCheckpoint struct {
	Name      PhaseName        `json:"name"`
	Status    CheckpointStatus `json:"status"`
	StartTime *time.Time       `json:"start_time,omitempty"`
	EndTime   *time.Time       `json:"end_time,omitempty"`
	Error     string           `json:"error,omitempty"`
	Artifacts []string         `json:"artifacts,omitempty"` // Files/packages created
}

// Checkpoint represents the complete bootstrap state for persistence.
type Checkpoint struct {
	// Metadata
	ID        string    `json:"id"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Overall status
	Status      CheckpointStatus `json:"status"`
	CurrentStep string           `json:"current_step,omitempty"`

	// Configuration snapshot
	Mode            DeploymentMode   `json:"mode"`
	BootstrapConfig *Config `json:"bootstrap_config,omitempty"`

	// Phase tracking
	Phases       []PhaseCheckpoint `json:"phases"`
	CurrentPhase int               `json:"current_phase"` // Index of current phase

	// Artifacts for rollback
	InstallArtifacts *CheckpointArtifacts `json:"install_artifacts,omitempty"`

	// System info snapshot
	SystemInfo *CheckpointSystemInfo `json:"system_info,omitempty"`

	// Rollback tracking
	RollbackTriggered bool       `json:"rollback_triggered"`
	RollbackReason    string     `json:"rollback_reason,omitempty"`
	RollbackStartTime *time.Time `json:"rollback_start_time,omitempty"`
	RollbackEndTime   *time.Time `json:"rollback_end_time,omitempty"`
}

// CheckpointArtifacts captures installed artifacts for rollback.
type CheckpointArtifacts struct {
	PackageManager string   `json:"package_manager"`
	Packages       []string `json:"packages"`
	CreatedFiles   []string `json:"created_files"`
	Services       []string `json:"services"`
}

// CheckpointSystemInfo captures system information.
type CheckpointSystemInfo struct {
	OS             string `json:"os"`
	Distro         string `json:"distro"`
	Arch           string `json:"arch"`
	PackageManager string `json:"package_manager"`
	InitSystem     string `json:"init_system"`
}

// CheckpointManager handles checkpoint persistence and recovery.
type CheckpointManager struct {
	mu            sync.RWMutex
	checkpointDir string
	checkpoint    *Checkpoint
}

// NewCheckpointManager creates a new checkpoint manager.
func NewCheckpointManager(checkpointDir string) *CheckpointManager {
	if checkpointDir == "" {
		checkpointDir = DefaultCheckpointDir
	}
	return &CheckpointManager{
		checkpointDir: checkpointDir,
	}
}

// Initialize creates a new checkpoint for a fresh bootstrap.
func (m *CheckpointManager) Initialize(mode DeploymentMode, cfg *Config, phases []Phase) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	checkpoint := &Checkpoint{
		ID:              generateCheckpointID(),
		Version:         "1.0",
		CreatedAt:       now,
		UpdatedAt:       now,
		Status:          CheckpointStatusInProgress,
		Mode:            mode,
		BootstrapConfig: cfg,
		Phases:          make([]PhaseCheckpoint, len(phases)),
		CurrentPhase:    0,
	}

	// Initialize phase checkpoints
	for i, phase := range phases {
		checkpoint.Phases[i] = PhaseCheckpoint{
			Name:   phase.Name,
			Status: CheckpointStatusPending,
		}
	}

	m.checkpoint = checkpoint
	return m.persist()
}

// Load attempts to load an existing checkpoint.
func (m *CheckpointManager) Load() (*Checkpoint, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.checkpointDir, CheckpointFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No checkpoint exists
		}
		return nil, fmt.Errorf("read checkpoint: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("parse checkpoint: %w", err)
	}

	m.checkpoint = &checkpoint
	return &checkpoint, nil
}

// HasCheckpoint returns true if a checkpoint file exists.
func (m *CheckpointManager) HasCheckpoint() bool {
	path := filepath.Join(m.checkpointDir, CheckpointFileName)
	_, err := os.Stat(path)
	return err == nil
}

// GetCheckpoint returns the current checkpoint.
func (m *CheckpointManager) GetCheckpoint() *Checkpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.checkpoint
}

// BeginPhase marks a phase as in-progress.
func (m *CheckpointManager) BeginPhase(phaseIndex int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.checkpoint == nil {
		return fmt.Errorf("checkpoint not initialized")
	}
	if phaseIndex >= len(m.checkpoint.Phases) {
		return fmt.Errorf("phase index out of range")
	}

	now := time.Now()
	m.checkpoint.Phases[phaseIndex].Status = CheckpointStatusInProgress
	m.checkpoint.Phases[phaseIndex].StartTime = &now
	m.checkpoint.CurrentPhase = phaseIndex
	m.checkpoint.CurrentStep = string(m.checkpoint.Phases[phaseIndex].Name)
	m.checkpoint.UpdatedAt = now

	return m.persist()
}

// CompletePhase marks a phase as completed.
func (m *CheckpointManager) CompletePhase(phaseIndex int, artifacts []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.checkpoint == nil {
		return fmt.Errorf("checkpoint not initialized")
	}
	if phaseIndex >= len(m.checkpoint.Phases) {
		return fmt.Errorf("phase index out of range")
	}

	now := time.Now()
	m.checkpoint.Phases[phaseIndex].Status = CheckpointStatusCompleted
	m.checkpoint.Phases[phaseIndex].EndTime = &now
	m.checkpoint.Phases[phaseIndex].Artifacts = artifacts
	m.checkpoint.UpdatedAt = now

	return m.persist()
}

// FailPhase marks a phase as failed.
func (m *CheckpointManager) FailPhase(phaseIndex int, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.checkpoint == nil {
		return fmt.Errorf("checkpoint not initialized")
	}
	if phaseIndex >= len(m.checkpoint.Phases) {
		return fmt.Errorf("phase index out of range")
	}

	now := time.Now()
	m.checkpoint.Phases[phaseIndex].Status = CheckpointStatusFailed
	m.checkpoint.Phases[phaseIndex].EndTime = &now
	if err != nil {
		m.checkpoint.Phases[phaseIndex].Error = err.Error()
	}
	m.checkpoint.Status = CheckpointStatusFailed
	m.checkpoint.UpdatedAt = now

	return m.persist()
}

// BeginRollback marks rollback as started.
func (m *CheckpointManager) BeginRollback(reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.checkpoint == nil {
		return fmt.Errorf("checkpoint not initialized")
	}

	now := time.Now()
	m.checkpoint.RollbackTriggered = true
	m.checkpoint.RollbackReason = reason
	m.checkpoint.RollbackStartTime = &now
	m.checkpoint.UpdatedAt = now

	return m.persist()
}

// CompleteRollback marks rollback as completed.
func (m *CheckpointManager) CompleteRollback() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.checkpoint == nil {
		return fmt.Errorf("checkpoint not initialized")
	}

	now := time.Now()
	m.checkpoint.RollbackEndTime = &now
	m.checkpoint.Status = CheckpointStatusRolledBack
	m.checkpoint.UpdatedAt = now

	return m.persist()
}

// MarkPhaseRolledBack marks a phase as rolled back.
func (m *CheckpointManager) MarkPhaseRolledBack(phaseIndex int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.checkpoint == nil {
		return fmt.Errorf("checkpoint not initialized")
	}
	if phaseIndex >= len(m.checkpoint.Phases) {
		return fmt.Errorf("phase index out of range")
	}

	m.checkpoint.Phases[phaseIndex].Status = CheckpointStatusRolledBack
	m.checkpoint.UpdatedAt = time.Now()

	return m.persist()
}

// SetInstallArtifacts updates the install artifacts.
func (m *CheckpointManager) SetInstallArtifacts(artifacts *InstallArtifacts) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.checkpoint == nil {
		return fmt.Errorf("checkpoint not initialized")
	}

	if artifacts != nil {
		m.checkpoint.InstallArtifacts = &CheckpointArtifacts{
			PackageManager: artifacts.PackageManager.String(),
			Packages:       artifacts.Packages,
			CreatedFiles:   artifacts.CreatedFiles,
		}
	}
	m.checkpoint.UpdatedAt = time.Now()

	return m.persist()
}

// SetSystemInfo updates the system info snapshot.
func (m *CheckpointManager) SetSystemInfo(info *SystemInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.checkpoint == nil {
		return fmt.Errorf("checkpoint not initialized")
	}

	if info != nil && info.Platform != nil {
		m.checkpoint.SystemInfo = &CheckpointSystemInfo{
			OS:             string(info.Platform.OS),
			Distro:         string(info.Platform.Distro),
			Arch:           string(info.Platform.Arch),
			PackageManager: info.Platform.PackageManager.String(),
			InitSystem:     string(info.Platform.InitSystem),
		}
	}
	m.checkpoint.UpdatedAt = time.Now()

	return m.persist()
}

// Complete marks the entire bootstrap as completed.
func (m *CheckpointManager) Complete() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.checkpoint == nil {
		return fmt.Errorf("checkpoint not initialized")
	}

	m.checkpoint.Status = CheckpointStatusCompleted
	m.checkpoint.UpdatedAt = time.Now()

	return m.persist()
}

// Clear removes the checkpoint file.
func (m *CheckpointManager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := filepath.Join(m.checkpointDir, CheckpointFileName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove checkpoint: %w", err)
	}
	m.checkpoint = nil
	return nil
}

// GetResumePhase returns the phase index to resume from.
// Returns -1 if bootstrap should start from beginning.
func (m *CheckpointManager) GetResumePhase() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.checkpoint == nil {
		return -1
	}

	// If rollback was triggered, don't resume
	if m.checkpoint.RollbackTriggered {
		return -1
	}

	// Find the first non-completed phase
	for i, phase := range m.checkpoint.Phases {
		if phase.Status == CheckpointStatusPending ||
			phase.Status == CheckpointStatusInProgress ||
			phase.Status == CheckpointStatusFailed {
			return i
		}
	}

	// All phases completed
	return len(m.checkpoint.Phases)
}

// GetCompletedPhases returns the indices of completed phases.
func (m *CheckpointManager) GetCompletedPhases() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.checkpoint == nil {
		return nil
	}

	var completed []int
	for i, phase := range m.checkpoint.Phases {
		if phase.Status == CheckpointStatusCompleted {
			completed = append(completed, i)
		}
	}
	return completed
}

// RestoreArtifacts restores InstallArtifacts from checkpoint.
func (m *CheckpointManager) RestoreArtifacts() *InstallArtifacts {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.checkpoint == nil || m.checkpoint.InstallArtifacts == nil {
		return nil
	}

	artifacts := m.checkpoint.InstallArtifacts
	return &InstallArtifacts{
		PackageManager: platform.PackageManagerFromString(artifacts.PackageManager),
		Packages:       artifacts.Packages,
		CreatedFiles:   artifacts.CreatedFiles,
	}
}

// persist writes the checkpoint to disk.
func (m *CheckpointManager) persist() error {
	//nolint:gosec // G301: checkpoint directory needs to be accessible by admin users
	if err := os.MkdirAll(m.checkpointDir, 0o755); err != nil {
		return fmt.Errorf("create checkpoint directory: %w", err)
	}

	data, err := json.MarshalIndent(m.checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	path := filepath.Join(m.checkpointDir, CheckpointFileName)
	// Write atomically via temp file
	tmpPath := path + ".tmp"
	//nolint:gosec // G306: checkpoint files need to be readable for recovery
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename checkpoint: %w", err)
	}

	return nil
}

func generateCheckpointID() string {
	return fmt.Sprintf("bootstrap-%d", time.Now().UnixNano())
}
