package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// AtomicRunner orchestrates atomic bootstrap with checkpoints and rollback.
type AtomicRunner struct {
	phases          []Phase
	output          io.Writer
	dryRun          bool
	verbose         bool
	jsonOutput      bool
	mode            DeploymentMode
	bootstrapConfig *BootstrapConfig
	atomicConfig    AtomicConfig
	transaction     *TransactionManager
}

// NewAtomicRunner returns a runner with atomic transaction support.
func NewAtomicRunner(output io.Writer, atomicConfig AtomicConfig) *AtomicRunner {
	return &AtomicRunner{
		phases: []Phase{
			{Name: PhaseDetect, Run: detectPhase, Rollback: noopRollback("detect")},
			{Name: PhaseConfigure, Run: configurePhase, Rollback: noopRollback("configure")},
			{Name: PhaseValidate, Run: validatePhase, Rollback: noopRollback("validate")},
			{Name: PhaseInstall, Run: installPhase, Rollback: installRollback},
			{Name: PhaseBlueprints, Run: blueprintPhase, Rollback: noopRollback("blueprints")},
			{Name: PhaseVerify, Run: verifyPhase, Rollback: noopRollback("verify")},
		},
		output:       output,
		atomicConfig: atomicConfig,
	}
}

// SetDryRun toggles dry-run mode.
func (r *AtomicRunner) SetDryRun(dryRun bool) {
	r.dryRun = dryRun
}

// SetVerbose toggles verbose logging.
func (r *AtomicRunner) SetVerbose(verbose bool) {
	r.verbose = verbose
}

// SetOutput changes the output writer.
func (r *AtomicRunner) SetOutput(output io.Writer) {
	r.output = output
}

// SetJSONOutput toggles JSON output mode.
func (r *AtomicRunner) SetJSONOutput(jsonOutput bool) {
	r.jsonOutput = jsonOutput
}

// SetMode sets the deployment mode.
func (r *AtomicRunner) SetMode(mode DeploymentMode) {
	r.mode = mode
}

// SetBootstrapConfig sets the bootstrap configuration.
func (r *AtomicRunner) SetBootstrapConfig(cfg *BootstrapConfig) {
	r.bootstrapConfig = cfg
}

// SetAtomicConfig updates the atomic configuration.
func (r *AtomicRunner) SetAtomicConfig(cfg AtomicConfig) {
	r.atomicConfig = cfg
}

// Run executes the atomic bootstrap with checkpoint and rollback support.
func (r *AtomicRunner) Run(ctx context.Context) error {
	// Initialize transaction manager
	r.transaction = NewTransactionManager(r.atomicConfig, r.output, r.verbose)

	// Initialize state
	state := &State{
		Started:         time.Now(),
		Output:          r.output,
		DryRun:          r.dryRun,
		Verbose:         r.verbose,
		JSONOutput:      r.jsonOutput,
		BootstrapConfig: r.bootstrapConfig,
		Progress: Progress{
			Total: len(r.phases),
		},
	}

	// Detect and set mode
	mode := r.mode
	if mode == "" {
		mode = DetectMode()
	}
	cfg, ok := DefaultDeploymentConfig(mode)
	if !ok {
		return fmt.Errorf("no default configuration for mode %s", mode)
	}
	state.Mode = mode
	state.Config = cfg

	if state.Verbose || state.DryRun {
		fmt.Fprintf(state.Output, "deployment mode: %s\n", state.Mode)
	}

	// Begin transaction (creates checkpoint)
	if !state.DryRun {
		if err := r.transaction.Begin(ctx, mode, r.bootstrapConfig, r.phases); err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}
	}

	// Check for resume
	startPhase := 0
	if r.transaction.ShouldResume() {
		startPhase = r.transaction.GetResumePoint()
		if startPhase > 0 && startPhase < len(r.phases) {
			r.logEvent(state, "resume", map[string]interface{}{
				"phase": r.phases[startPhase].Name,
				"index": startPhase,
			})

			// Restore artifacts from checkpoint
			if artifacts := r.transaction.GetRestoredArtifacts(); artifacts != nil {
				state.InstallArtifacts = artifacts
			}
		}
	}

	// Execute phases
	var completed []Phase
	for i := startPhase; i < len(r.phases); i++ {
		phase := r.phases[i]

		// Skip verify in dry-run
		if state.DryRun && phase.Name == PhaseVerify {
			if state.Verbose {
				fmt.Fprintln(state.Output, "verification skipped in dry-run mode")
			}
			continue
		}

		// Update progress
		state.Progress.Completed = i + 1
		state.Progress.Phase = phase.Name
		formatPhase(state.Output, state.Progress, state.JSONOutput)

		// Signal phase start to transaction
		if !state.DryRun {
			if err := r.transaction.BeforePhase(ctx, i, phase); err != nil {
				// Log but don't fail on checkpoint errors
				if state.Verbose {
					fmt.Fprintf(state.Output, "warning: checkpoint error: %v\n", err)
				}
			}
		}

		// Execute phase with retry support
		phaseErr := r.executePhaseWithRetry(ctx, state, i, phase)
		if phaseErr != nil {
			r.logEvent(state, "error", map[string]interface{}{
				"phase": phase.Name,
				"error": phaseErr.Error(),
			})

			// Record failure
			if !state.DryRun {
				r.transaction.OnPhaseFailure(ctx, i, phase, phaseErr)
			}

			// Collect diagnostics
			collectDiagnostics(ctx, state, phase.Name, phaseErr)

			// Determine if we should rollback
			if r.transaction.ShouldRollback(phase, phaseErr) {
				r.performRollback(ctx, state, completed, phaseErr.Error())
			}

			return phaseErr
		}

		// Signal phase completion to transaction
		if !state.DryRun {
			if err := r.transaction.AfterPhase(ctx, i, phase, state.InstallArtifacts); err != nil {
				if state.Verbose {
					fmt.Fprintf(state.Output, "warning: checkpoint error: %v\n", err)
				}
			}

			// Update system info after detect phase
			if phase.Name == PhaseDetect && state.System != nil {
				r.transaction.SetSystemInfo(state.System)
			}
		}

		completed = append(completed, phase)
	}

	// Commit transaction (marks complete, optionally clears checkpoint)
	if !state.DryRun {
		if err := r.transaction.Commit(ctx); err != nil {
			if state.Verbose {
				fmt.Fprintf(state.Output, "warning: commit error: %v\n", err)
			}
		}
	}

	r.logEvent(state, "complete", nil)
	return nil
}

// executePhaseWithRetry executes a phase with optional retry support.
func (r *AtomicRunner) executePhaseWithRetry(ctx context.Context, state *State, phaseIndex int, phase Phase) error {
	maxAttempts := r.atomicConfig.MaxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 && state.Verbose {
			fmt.Fprintf(state.Output, "retrying phase %s (attempt %d/%d)\n",
				phase.Name, attempt, maxAttempts)
		}

		lastErr = phase.Run(ctx, state)
		if lastErr == nil {
			return nil
		}

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return lastErr
		}

		// Don't retry if this is the last attempt
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(r.atomicConfig.RetryDelay):
			}
		}
	}

	return lastErr
}

// performRollback executes rollback for completed phases.
func (r *AtomicRunner) performRollback(ctx context.Context, state *State, completed []Phase, reason string) {
	if len(completed) == 0 {
		return
	}

	// Create rollback context with timeout
	rollbackCtx := ctx
	if r.atomicConfig.RollbackTimeout > 0 {
		var cancel context.CancelFunc
		rollbackCtx, cancel = context.WithTimeout(ctx, r.atomicConfig.RollbackTimeout)
		defer cancel()
	}

	// Signal rollback start
	if !state.DryRun {
		r.transaction.BeforeRollback(rollbackCtx, reason)
	}

	r.logEvent(state, "rollback_start", map[string]interface{}{
		"phases": len(completed),
		"reason": reason,
	})

	// Rollback in reverse order
	for i := len(completed) - 1; i >= 0; i-- {
		phase := completed[i]
		if phase.Rollback == nil {
			continue
		}

		if state.Verbose {
			fmt.Fprintf(state.Output, "rolling back phase %s\n", phase.Name)
		}

		if err := phase.Rollback(rollbackCtx, state); err != nil {
			fmt.Fprintf(state.Output, "rollback %s failed: %v\n", phase.Name, err)
			// Continue rolling back other phases even if one fails
		}

		// Signal phase rollback complete
		if !state.DryRun {
			// Find the phase index
			for j, p := range r.phases {
				if p.Name == phase.Name {
					r.transaction.AfterPhaseRollback(rollbackCtx, j, phase)
					break
				}
			}
		}
	}

	// Signal rollback complete
	if !state.DryRun {
		r.transaction.AfterRollback(rollbackCtx)
	}

	r.logEvent(state, "rollback_complete", nil)
}

// logEvent logs an event in the appropriate format.
func (r *AtomicRunner) logEvent(state *State, event string, data map[string]interface{}) {
	if state.JSONOutput {
		payload := map[string]interface{}{
			"event": event,
		}
		for k, v := range data {
			payload[k] = v
		}
		if encoded, err := json.Marshal(payload); err == nil {
			fmt.Fprintln(state.Output, string(encoded))
		}
	} else {
		switch event {
		case "complete":
			fmt.Fprintln(state.Output, "bootstrap completed")
		case "rollback_start":
			fmt.Fprintf(state.Output, "starting automatic rollback: %v\n", data["reason"])
		case "rollback_complete":
			fmt.Fprintln(state.Output, "rollback completed")
		case "resume":
			fmt.Fprintf(state.Output, "resuming from phase %s\n", data["phase"])
		}
	}
}

// GetCheckpoint returns the current checkpoint for inspection.
func (r *AtomicRunner) GetCheckpoint() *BootstrapCheckpoint {
	if r.transaction == nil {
		return nil
	}
	return r.transaction.GetCheckpoint()
}

// ClearCheckpoint removes checkpoint state.
func (r *AtomicRunner) ClearCheckpoint() error {
	if r.transaction == nil {
		return nil
	}
	return r.transaction.Clear()
}

// ForceRollback triggers a manual rollback using checkpoint state.
func (r *AtomicRunner) ForceRollback(ctx context.Context) error {
	if r.transaction == nil {
		return fmt.Errorf("transaction not initialized")
	}

	checkpoint := r.transaction.GetCheckpoint()
	if checkpoint == nil {
		return fmt.Errorf("no checkpoint available")
	}

	// Reconstruct state from checkpoint
	state := &State{
		Output:          r.output,
		Verbose:         r.verbose,
		JSONOutput:      r.jsonOutput,
		BootstrapConfig: checkpoint.BootstrapConfig,
		Mode:            checkpoint.Mode,
	}

	// Restore artifacts
	if artifacts := r.transaction.GetRestoredArtifacts(); artifacts != nil {
		state.InstallArtifacts = artifacts
	}

	// Find completed phases
	var completed []Phase
	for _, phaseCheckpoint := range checkpoint.Phases {
		if phaseCheckpoint.Status == CheckpointStatusCompleted {
			for _, phase := range r.phases {
				if phase.Name == phaseCheckpoint.Name {
					completed = append(completed, phase)
					break
				}
			}
		}
	}

	if len(completed) == 0 {
		return fmt.Errorf("no completed phases to rollback")
	}

	r.performRollback(ctx, state, completed, "manual rollback requested")
	return nil
}
