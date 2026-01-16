package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// PhaseName identifies a bootstrap phase.
type PhaseName string

const (
	PhaseDetect     PhaseName = "detect"
	PhaseConfigure  PhaseName = "configure"
	PhaseValidate   PhaseName = "validate"
	PhaseInstall    PhaseName = "install"
	PhaseBlueprints PhaseName = "blueprints"
	PhaseVerify     PhaseName = "verify"
)

// PhaseFunc executes a bootstrap phase.
type PhaseFunc func(context.Context, *State) error

// Phase defines a bootstrap phase and optional rollback.
type Phase struct {
	Name     PhaseName
	Run      PhaseFunc
	Rollback PhaseFunc
}

// Progress captures the current bootstrap progress.
type Progress struct {
	Phase     PhaseName
	Completed int
	Total     int
}

// State holds shared bootstrap execution context.
type State struct {
	Started          time.Time
	Progress         Progress
	Output           io.Writer
	DryRun           bool
	Verbose          bool
	JSONOutput       bool
	Mode             DeploymentMode
	Config           DeploymentConfig
	System           *SystemInfo
	BootstrapConfig  *BootstrapConfig
	InstallArtifacts *InstallArtifacts
}

// Runner orchestrates bootstrap phases.
type Runner struct {
	phases          []Phase
	output          io.Writer
	dryRun          bool
	verbose         bool
	jsonOutput      bool
	mode            DeploymentMode
	bootstrapConfig *BootstrapConfig
}

// NewRunner returns a runner with default phases.
func NewRunner(output io.Writer) *Runner {
	return &Runner{
		phases: []Phase{
			{Name: PhaseDetect, Run: detectPhase, Rollback: noopRollback("detect")},
			{Name: PhaseConfigure, Run: configurePhase, Rollback: noopRollback("configure")},
			{Name: PhaseValidate, Run: validatePhase, Rollback: noopRollback("validate")},
			{Name: PhaseInstall, Run: installPhase, Rollback: installRollback},
			{Name: PhaseBlueprints, Run: blueprintPhase, Rollback: noopRollback("blueprints")},
			{Name: PhaseVerify, Run: verifyPhase, Rollback: noopRollback("verify")},
		},
		output: output,
	}
}

// SetDryRun toggles dry-run mode.
func (r *Runner) SetDryRun(dryRun bool) {
	r.dryRun = dryRun
}

// SetVerbose toggles verbose logging.
func (r *Runner) SetVerbose(verbose bool) {
	r.verbose = verbose
}

// SetOutput changes the output writer for progress/logs.
func (r *Runner) SetOutput(output io.Writer) {
	r.output = output
}

// SetJSONOutput toggles JSON output mode.
func (r *Runner) SetJSONOutput(jsonOutput bool) {
	r.jsonOutput = jsonOutput
}

// SetMode sets the deployment mode for the run.
func (r *Runner) SetMode(mode DeploymentMode) {
	r.mode = mode
}

// SetBootstrapConfig sets the bootstrap configuration for validation.
func (r *Runner) SetBootstrapConfig(cfg *BootstrapConfig) {
	r.bootstrapConfig = cfg
}

// Run executes all phases in order and rolls back on failure.
func (r *Runner) Run(ctx context.Context) error {
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

	var completed []Phase
	for i, phase := range r.phases {
		if state.DryRun && phase.Name == PhaseVerify {
			if state.Verbose {
				fmt.Fprintln(state.Output, "verification skipped in dry-run mode")
			}
			continue
		}
		state.Progress.Completed = i + 1
		state.Progress.Phase = phase.Name
		formatPhase(state.Output, state.Progress, state.JSONOutput)

		if err := phase.Run(ctx, state); err != nil {
			if state.JSONOutput {
				payload := map[string]interface{}{
					"event": "error",
					"phase": phase.Name,
					"error": err.Error(),
				}
				if encoded, marshalErr := json.Marshal(payload); marshalErr == nil {
					fmt.Fprintln(state.Output, string(encoded))
				}
			} else {
				fmt.Fprintf(state.Output, "phase %s failed: %v\n", phase.Name, err)
			}
			collectDiagnostics(ctx, state, phase.Name, err)
			r.rollback(ctx, state, completed)
			return err
		}

		completed = append(completed, phase)
	}

	if state.JSONOutput {
		payload := map[string]interface{}{
			"event": "complete",
		}
		if encoded, err := json.Marshal(payload); err == nil {
			fmt.Fprintln(state.Output, string(encoded))
		}
	} else {
		fmt.Fprintln(state.Output, "bootstrap completed")
	}
	return nil
}

func (r *Runner) rollback(ctx context.Context, state *State, completed []Phase) {
	for i := len(completed) - 1; i >= 0; i-- {
		phase := completed[i]
		if phase.Rollback == nil {
			continue
		}
		if err := phase.Rollback(ctx, state); err != nil {
			fmt.Fprintf(state.Output, "rollback %s failed: %v\n", phase.Name, err)
		}
	}
}

func noopPhase(name string) PhaseFunc {
	return func(ctx context.Context, state *State) error {
		if state.DryRun || state.Verbose {
			fmt.Fprintf(state.Output, "phase %s: not implemented yet\n", name)
		}
		return nil
	}
}

func noopRollback(name string) PhaseFunc {
	return func(ctx context.Context, state *State) error {
		if state.Verbose {
			fmt.Fprintf(state.Output, "rollback %s: no actions\n", name)
		}
		return nil
	}
}
