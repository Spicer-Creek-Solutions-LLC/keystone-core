// SPDX-License-Identifier: Apache-2.0

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// EngineConfig wires the Engine's dependencies. Detector,
// Configurer, Validator, Installer, Verifier are required;
// StatePath defaults to DefaultStatePath
// (/var/lib/kscore-agent/bootstrap.json) when empty.
type EngineConfig struct {
	StatePath  string
	Logger     *slog.Logger
	Now        func() time.Time
	Detector   Detector
	Configurer Configurer
	Validator  Validator
	Installer  Installer
	Verifier   Verifier
}

// Engine drives the Detect → Configure → Validate → Install →
// Verify FSM. Persists State after each phase so a re-run resumes
// from the last checkpoint.
type Engine struct {
	statePath  string
	log        *slog.Logger
	now        func() time.Time
	detector   Detector
	configurer Configurer
	validator  Validator
	installer  Installer
	verifier   Verifier
}

// NewEngine validates cfg and returns an Engine. All phase impls
// are required — engine.Run can't continue past a missing phase
// even via state-resume because future runs may re-enter the
// missing phase.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	if cfg.Detector == nil {
		return nil, errors.New("bootstrap: EngineConfig.Detector is required")
	}
	if cfg.Configurer == nil {
		return nil, errors.New("bootstrap: EngineConfig.Configurer is required")
	}
	if cfg.Validator == nil {
		return nil, errors.New("bootstrap: EngineConfig.Validator is required")
	}
	if cfg.Installer == nil {
		return nil, errors.New("bootstrap: EngineConfig.Installer is required")
	}
	if cfg.Verifier == nil {
		return nil, errors.New("bootstrap: EngineConfig.Verifier is required")
	}
	if cfg.StatePath == "" {
		cfg.StatePath = DefaultStatePath
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Engine{
		statePath:  cfg.StatePath,
		log:        cfg.Logger,
		now:        cfg.Now,
		detector:   cfg.Detector,
		configurer: cfg.Configurer,
		validator:  cfg.Validator,
		installer:  cfg.Installer,
		verifier:   cfg.Verifier,
	}, nil
}

// Run drives the FSM. Loads any persisted State, jumps past phases
// that already completed, and runs the rest in order. Returns the
// final State whether Run succeeds or fails — operators can
// inspect ValidationResult / VerifyResult / LastError.
//
// On a re-run after PhaseDone, returns the existing State with no
// side effects. Operators wanting to re-bootstrap delete the state
// file (or, when Task 8 lands, pass --force).
func (e *Engine) Run(ctx context.Context) (*State, error) {
	state, err := LoadState(e.statePath)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: load state: %w", err)
	}
	if state == nil {
		state = NewState(e.now())
		e.log.InfoContext(ctx, "bootstrap: starting fresh", "state_path", e.statePath)
	} else if state.Phase == PhaseDone {
		e.log.InfoContext(ctx, "bootstrap: already complete; nothing to do",
			"state_path", e.statePath, "completed_at", state.CompletedAt)
		return state, nil
	} else {
		e.log.InfoContext(ctx, "bootstrap: resuming from checkpoint",
			"state_path", e.statePath, "phase", state.Phase)
	}

	for {
		switch state.Phase {
		case PhaseDetect:
			if err := e.runDetect(ctx, state); err != nil {
				return state, err
			}
		case PhaseConfigure:
			if err := e.runConfigure(ctx, state); err != nil {
				return state, err
			}
		case PhaseValidate:
			if err := e.runValidate(ctx, state); err != nil {
				return state, err
			}
		case PhaseInstall:
			if err := e.runInstall(ctx, state); err != nil {
				return state, err
			}
		case PhaseVerify:
			if err := e.runVerify(ctx, state); err != nil {
				return state, err
			}
		case PhaseDone:
			completed := e.now().UTC()
			state.CompletedAt = &completed
			if err := e.persist(state); err != nil {
				return state, err
			}
			e.log.InfoContext(ctx, "bootstrap: complete",
				"state_path", e.statePath, "completed_at", completed)
			return state, nil
		default:
			return state, fmt.Errorf("bootstrap: unknown phase %q", state.Phase)
		}
	}
}

func (e *Engine) runDetect(ctx context.Context, state *State) error {
	res, err := e.detector.Detect(ctx)
	if err != nil {
		return e.failPhase(state, fmt.Errorf("detect: %w", err))
	}
	state.Detection = res
	return e.advance(ctx, state, "detect")
}

func (e *Engine) runConfigure(ctx context.Context, state *State) error {
	cfg, err := e.configurer.Configure(ctx, state.Detection)
	if err != nil {
		return e.failPhase(state, fmt.Errorf("configure: %w", err))
	}
	if cfg == nil {
		return e.failPhase(state, errors.New("configure: returned nil Configuration"))
	}
	if err := cfg.Validate(); err != nil {
		return e.failPhase(state, fmt.Errorf("configure: %w", err))
	}
	state.Config = cfg
	return e.advance(ctx, state, "configure")
}

func (e *Engine) runValidate(ctx context.Context, state *State) error {
	if state.Config == nil {
		return e.failPhase(state, errors.New("validate: state has no Configuration"))
	}
	res, err := e.validator.Validate(ctx, state.Config)
	if err != nil {
		return e.failPhase(state, fmt.Errorf("validate: %w", err))
	}
	state.Validation = res
	if res != nil && !res.AllOK() {
		return e.failPhase(state, errors.New("validate: one or more checks failed"))
	}
	return e.advance(ctx, state, "validate")
}

func (e *Engine) runInstall(ctx context.Context, state *State) error {
	if state.Config == nil {
		return e.failPhase(state, errors.New("install: state has no Configuration"))
	}
	res, err := e.installer.Install(ctx, state.Config)
	if err != nil {
		return e.failPhase(state, fmt.Errorf("install: %w", err))
	}
	state.Install = res
	return e.advance(ctx, state, "install")
}

func (e *Engine) runVerify(ctx context.Context, state *State) error {
	if state.Config == nil {
		return e.failPhase(state, errors.New("verify: state has no Configuration"))
	}
	res, err := e.verifier.Verify(ctx, state.Config)
	if err != nil {
		return e.failPhase(state, fmt.Errorf("verify: %w", err))
	}
	state.Verify = res
	if res != nil && !res.AllOK() {
		return e.failPhase(state, errors.New("verify: one or more checks failed"))
	}
	return e.advance(ctx, state, "verify")
}

// advance moves state.Phase to the next phase and persists. Called
// only on success.
func (e *Engine) advance(ctx context.Context, state *State, completed string) error {
	state.Phase = nextPhase(state.Phase)
	state.UpdatedAt = e.now().UTC()
	state.LastError = ""
	if err := e.persist(state); err != nil {
		return err
	}
	e.log.InfoContext(ctx, "bootstrap: phase complete",
		"completed_phase", completed, "next_phase", state.Phase)
	return nil
}

// failPhase records the error on State, persists, and returns it.
// Caller (Engine.Run) returns immediately so operators inspect the
// state file + fix the problem + re-run.
func (e *Engine) failPhase(state *State, err error) error {
	state.LastError = err.Error()
	state.UpdatedAt = e.now().UTC()
	if persistErr := e.persist(state); persistErr != nil {
		// Both errors lose information if we only return one — wrap.
		return fmt.Errorf("%w (also: persist after failure: %v)", err, persistErr)
	}
	return err
}

func (e *Engine) persist(state *State) error {
	if err := state.Save(e.statePath); err != nil {
		return fmt.Errorf("bootstrap: persist state: %w", err)
	}
	return nil
}
