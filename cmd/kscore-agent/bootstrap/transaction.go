package bootstrap

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// RollbackTrigger defines when automatic rollback should occur.
type RollbackTrigger string

const (
	// RollbackOnAnyFailure triggers rollback on any phase failure.
	RollbackOnAnyFailure RollbackTrigger = "any_failure"
	// RollbackOnInstallFailure triggers rollback only on install/config failures.
	RollbackOnInstallFailure RollbackTrigger = "install_failure"
	// RollbackManual requires manual intervention to trigger rollback.
	RollbackManual RollbackTrigger = "manual"
)

// AtomicConfig configures the atomic bootstrap behavior.
type AtomicConfig struct {
	// EnableCheckpoints enables checkpoint persistence for crash recovery.
	EnableCheckpoints bool

	// CheckpointDir is the directory for checkpoint files.
	CheckpointDir string

	// RollbackTrigger defines when automatic rollback occurs.
	RollbackTrigger RollbackTrigger

	// EnableResume enables resuming from a previous checkpoint.
	EnableResume bool

	// ClearOnSuccess clears checkpoints after successful completion.
	ClearOnSuccess bool

	// RollbackTimeout is the maximum time allowed for rollback operations.
	RollbackTimeout time.Duration

	// MaxRetries is the maximum number of retry attempts for a phase.
	MaxRetries int

	// RetryDelay is the delay between retry attempts.
	RetryDelay time.Duration
}

// DefaultAtomicConfig returns the default atomic bootstrap configuration.
func DefaultAtomicConfig() AtomicConfig {
	return AtomicConfig{
		EnableCheckpoints: true,
		CheckpointDir:     DefaultCheckpointDir,
		RollbackTrigger:   RollbackOnAnyFailure,
		EnableResume:      true,
		ClearOnSuccess:    true,
		RollbackTimeout:   10 * time.Minute,
		MaxRetries:        0, // No retries by default
		RetryDelay:        5 * time.Second,
	}
}

// TransactionManager coordinates atomic bootstrap operations.
type TransactionManager struct {
	mu         sync.Mutex
	config     AtomicConfig
	checkpoint *CheckpointManager
	output     io.Writer
	verbose    bool
}

// NewTransactionManager creates a new transaction manager.
func NewTransactionManager(config AtomicConfig, output io.Writer, verbose bool) *TransactionManager {
	return &TransactionManager{
		config:     config,
		checkpoint: NewCheckpointManager(config.CheckpointDir),
		output:     output,
		verbose:    verbose,
	}
}

// Begin starts a new bootstrap transaction.
func (t *TransactionManager) Begin(ctx context.Context, mode DeploymentMode, cfg *Config, phases []Phase) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.config.EnableCheckpoints {
		return nil
	}

	// Check for existing checkpoint
	if t.config.EnableResume && t.checkpoint.HasCheckpoint() {
		existing, err := t.checkpoint.Load()
		if err != nil {
			return fmt.Errorf("load existing checkpoint: %w", err)
		}

		if existing != nil && existing.Status == CheckpointStatusInProgress {
			if t.verbose {
				fmt.Fprintf(t.output, "found existing checkpoint from %s, will attempt resume\n",
					existing.CreatedAt.Format(time.RFC3339))
			}
			return nil
		}
	}

	// Initialize new checkpoint
	if err := t.checkpoint.Initialize(mode, cfg, phases); err != nil {
		return fmt.Errorf("initialize checkpoint: %w", err)
	}

	if t.verbose {
		fmt.Fprintln(t.output, "checkpoint initialized for atomic bootstrap")
	}

	return nil
}

// GetResumePoint returns the phase index to resume from, or -1 for fresh start.
func (t *TransactionManager) GetResumePoint() int {
	if !t.config.EnableCheckpoints || !t.config.EnableResume {
		return -1
	}
	return t.checkpoint.GetResumePhase()
}

// ShouldResume returns true if we should resume from a checkpoint.
func (t *TransactionManager) ShouldResume() bool {
	return t.GetResumePoint() > 0
}

// GetRestoredArtifacts returns artifacts from checkpoint for resumed bootstrap.
func (t *TransactionManager) GetRestoredArtifacts() *InstallArtifacts {
	if !t.config.EnableCheckpoints {
		return nil
	}
	return t.checkpoint.RestoreArtifacts()
}

// BeforePhase is called before executing a phase.
func (t *TransactionManager) BeforePhase(ctx context.Context, phaseIndex int, phase Phase) error {
	if !t.config.EnableCheckpoints {
		return nil
	}

	if err := t.checkpoint.BeginPhase(phaseIndex); err != nil {
		return fmt.Errorf("checkpoint begin phase: %w", err)
	}

	if t.verbose {
		fmt.Fprintf(t.output, "checkpoint: starting phase %s\n", phase.Name)
	}

	return nil
}

// AfterPhase is called after a phase completes successfully.
func (t *TransactionManager) AfterPhase(ctx context.Context, phaseIndex int, phase Phase, artifacts *InstallArtifacts) error {
	if !t.config.EnableCheckpoints {
		return nil
	}

	// Update artifacts
	if artifacts != nil {
		if err := t.checkpoint.SetInstallArtifacts(artifacts); err != nil {
			return fmt.Errorf("checkpoint set artifacts: %w", err)
		}
	}

	// Collect artifact paths for this phase
	var artifactPaths []string
	if artifacts != nil {
		artifactPaths = append(artifactPaths, artifacts.CreatedFiles...)
	}

	if err := t.checkpoint.CompletePhase(phaseIndex, artifactPaths); err != nil {
		return fmt.Errorf("checkpoint complete phase: %w", err)
	}

	if t.verbose {
		fmt.Fprintf(t.output, "checkpoint: completed phase %s\n", phase.Name)
	}

	return nil
}

// OnPhaseFailure is called when a phase fails.
func (t *TransactionManager) OnPhaseFailure(ctx context.Context, phaseIndex int, phase Phase, phaseErr error) error {
	if !t.config.EnableCheckpoints {
		return nil
	}

	if err := t.checkpoint.FailPhase(phaseIndex, phaseErr); err != nil {
		return fmt.Errorf("checkpoint fail phase: %w", err)
	}

	if t.verbose {
		fmt.Fprintf(t.output, "checkpoint: phase %s failed: %v\n", phase.Name, phaseErr)
	}

	return nil
}

// ShouldRollback determines if automatic rollback should occur.
func (t *TransactionManager) ShouldRollback(phase Phase, err error) bool {
	switch t.config.RollbackTrigger {
	case RollbackOnAnyFailure:
		return true
	case RollbackOnInstallFailure:
		// Only rollback on install, blueprints, or verify phases
		return phase.Name == PhaseInstall ||
			phase.Name == PhaseBlueprints ||
			phase.Name == PhaseVerify
	case RollbackManual:
		return false
	default:
		return true
	}
}

// BeforeRollback is called before starting rollback.
func (t *TransactionManager) BeforeRollback(ctx context.Context, reason string) error {
	if !t.config.EnableCheckpoints {
		return nil
	}

	if err := t.checkpoint.BeginRollback(reason); err != nil {
		return fmt.Errorf("checkpoint begin rollback: %w", err)
	}

	if t.verbose {
		fmt.Fprintf(t.output, "checkpoint: beginning rollback - %s\n", reason)
	}

	return nil
}

// AfterPhaseRollback is called after rolling back a single phase.
func (t *TransactionManager) AfterPhaseRollback(ctx context.Context, phaseIndex int, phase Phase) error {
	if !t.config.EnableCheckpoints {
		return nil
	}

	if err := t.checkpoint.MarkPhaseRolledBack(phaseIndex); err != nil {
		return fmt.Errorf("checkpoint mark phase rolled back: %w", err)
	}

	if t.verbose {
		fmt.Fprintf(t.output, "checkpoint: rolled back phase %s\n", phase.Name)
	}

	return nil
}

// AfterRollback is called after rollback completes.
func (t *TransactionManager) AfterRollback(ctx context.Context) error {
	if !t.config.EnableCheckpoints {
		return nil
	}

	if err := t.checkpoint.CompleteRollback(); err != nil {
		return fmt.Errorf("checkpoint complete rollback: %w", err)
	}

	if t.verbose {
		fmt.Fprintln(t.output, "checkpoint: rollback completed")
	}

	return nil
}

// Commit finalizes a successful bootstrap.
func (t *TransactionManager) Commit(ctx context.Context) error {
	if !t.config.EnableCheckpoints {
		return nil
	}

	if err := t.checkpoint.Complete(); err != nil {
		return fmt.Errorf("checkpoint complete: %w", err)
	}

	if t.config.ClearOnSuccess {
		if err := t.checkpoint.Clear(); err != nil {
			// Log but don't fail on cleanup error
			if t.verbose {
				fmt.Fprintf(t.output, "warning: failed to clear checkpoint: %v\n", err)
			}
		} else if t.verbose {
			fmt.Fprintln(t.output, "checkpoint cleared after successful bootstrap")
		}
	}

	return nil
}

// SetSystemInfo updates system info in the checkpoint.
func (t *TransactionManager) SetSystemInfo(info *SystemInfo) error {
	if !t.config.EnableCheckpoints {
		return nil
	}
	return t.checkpoint.SetSystemInfo(info)
}

// GetCheckpoint returns the current checkpoint for inspection.
func (t *TransactionManager) GetCheckpoint() *Checkpoint {
	if !t.config.EnableCheckpoints {
		return nil
	}
	return t.checkpoint.GetCheckpoint()
}

// GetCheckpointManager returns the underlying checkpoint manager.
func (t *TransactionManager) GetCheckpointManager() *CheckpointManager {
	return t.checkpoint
}

// ForceRollback forces a rollback even if automatic rollback is disabled.
func (t *TransactionManager) ForceRollback(ctx context.Context, reason string) error {
	return t.BeforeRollback(ctx, reason)
}

// Clear removes checkpoint state.
func (t *TransactionManager) Clear() error {
	if !t.config.EnableCheckpoints {
		return nil
	}
	return t.checkpoint.Clear()
}
