// SPDX-License-Identifier: Apache-2.0

package selfmgmt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.keystone-core.io/keystone-core/pkg/statemachine"
)

// PhaseHandler runs the per-phase work for a [BootstrapManager].
// Every method MUST be idempotent: a crash mid-phase resumes by
// re-entering the same transient state, which re-invokes the same
// method. Returning a non-nil error transitions the machine to
// [StateFailed]; the manager will then call [PhaseHandler.Rollback]
// when WithAutoRollback is enabled.
//
// Real implementations (host detect, binary install, blueprint
// engine invocation, /health probe) land with Epic 18 task 7 —
// see the gate-v1.0 ROADMAP entry "Bootstrap phase handlers".
type PhaseHandler interface {
	Detect(ctx context.Context, seed *SeedConfig) error
	Configure(ctx context.Context, seed *SeedConfig) error
	Validate(ctx context.Context, seed *SeedConfig) error
	Install(ctx context.Context, seed *SeedConfig) error
	ApplyBlueprints(ctx context.Context, seed *SeedConfig) error
	Verify(ctx context.Context, seed *SeedConfig) error

	// Rollback is invoked by [BootstrapManager.Run] when a phase
	// fails and the manager was constructed with AutoRollback (the
	// default). failedAt is the transient state the failure occurred
	// in (e.g. [StateConfiguring]), so the rollback can scope its
	// cleanup to the work that ran.
	Rollback(ctx context.Context, seed *SeedConfig, failedAt BootstrapState) error
}

// ErrBootstrap is the sentinel wrap returned by [BootstrapManager.Run]
// when a phase handler errors. Use [errors.Is] to detect the failure
// without inspecting the wrapped phase-handler error.
var ErrBootstrap = errors.New("selfmgmt: bootstrap failed")

// Option configures a [BootstrapManager] at construction.
type Option func(*managerOptions)

type managerOptions struct {
	checkpointer  statemachine.Checkpointer[BootstrapState, BootstrapEvent]
	logger        *slog.Logger
	autoRollback  bool
}

// WithCheckpointer overrides the default in-memory checkpointer.
// Pass a durable backend (SQLite/etcd) for crash-resume across process
// restarts. Task 2 ships only the in-memory default; durable backends
// are tracked under the gate-v1.0 ROADMAP entry.
func WithCheckpointer(cp statemachine.Checkpointer[BootstrapState, BootstrapEvent]) Option {
	return func(o *managerOptions) { o.checkpointer = cp }
}

// WithLogger overrides the default slog.Default() logger.
func WithLogger(l *slog.Logger) Option {
	return func(o *managerOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithAutoRollback toggles the post-failure rollback transition.
// Default true: a phase failure auto-fires EventRollback after the
// handler's Rollback method returns. Pass false to leave the machine
// at [StateFailed] for manual operator handling.
func WithAutoRollback(enabled bool) Option {
	return func(o *managerOptions) { o.autoRollback = enabled }
}

// BootstrapManager drives the per-phase state machine that an
// operator-supplied [PhaseHandler] implements. It is safe to call
// [BootstrapManager.Run] multiple times: the second call on a
// machine already in a terminal state ([StateVerified] /
// [StateRolledBack]) returns nil immediately.
type BootstrapManager struct {
	seed    *SeedConfig
	handler PhaseHandler
	cp      statemachine.Checkpointer[BootstrapState, BootstrapEvent]
	logger  *slog.Logger
	auto    bool
	machine *statemachine.Machine[BootstrapState, BootstrapEvent]
}

// NewBootstrapManager constructs a manager around seed and h. seed
// must already be Validated; h must be non-nil. The machine starts at
// [StateNotStarted]; if a [WithCheckpointer] checkpoint exists it is
// adopted on the first call to [BootstrapManager.Run].
func NewBootstrapManager(seed *SeedConfig, h PhaseHandler, opts ...Option) (*BootstrapManager, error) {
	if seed == nil {
		return nil, errors.New("selfmgmt: NewBootstrapManager: seed must not be nil")
	}
	if h == nil {
		return nil, errors.New("selfmgmt: NewBootstrapManager: handler must not be nil")
	}

	o := managerOptions{
		checkpointer: statemachine.NewMemoryCheckpointer[BootstrapState, BootstrapEvent](),
		logger:       slog.Default(),
		autoRollback: true,
	}
	for _, opt := range opts {
		opt(&o)
	}

	machine, err := newMachine(StateNotStarted, o.checkpointer)
	if err != nil {
		return nil, fmt.Errorf("selfmgmt: build state machine: %w", err)
	}

	return &BootstrapManager{
		seed:    seed,
		handler: h,
		cp:      o.checkpointer,
		logger:  o.logger,
		auto:    o.autoRollback,
		machine: machine,
	}, nil
}

// State returns the current machine state.
func (m *BootstrapManager) State() BootstrapState { return m.machine.Current() }

// History returns the ordered transition history of the underlying
// machine. Each [BootstrapManager.Run] invocation appends to this
// list; a crashed-then-resumed bootstrap shows the pre-crash records
// loaded from the checkpoint plus the resume records.
func (m *BootstrapManager) History() []statemachine.Record[BootstrapState, BootstrapEvent] {
	return m.machine.History()
}

// phaseStep maps one phase of the bootstrap into the events and
// handler call that advance it. The Run loop selects a step based on
// the machine's current state.
type phaseStep struct {
	name      string
	before    BootstrapState // stable state preceding the phase
	start     BootstrapEvent // fire to enter transient
	transient BootstrapState // mid-phase state (handler runs here)
	done      BootstrapEvent
	fail      BootstrapEvent
	handler   func(context.Context, *SeedConfig) error
}

func (m *BootstrapManager) phases() []phaseStep {
	return []phaseStep{
		{name: "detect", before: StateNotStarted, start: EventStartDetect, transient: StateDetecting, done: EventDetectDone, fail: EventDetectFail, handler: m.handler.Detect},
		{name: "configure", before: StateDetected, start: EventStartConfigure, transient: StateConfiguring, done: EventConfigureDone, fail: EventConfigureFail, handler: m.handler.Configure},
		{name: "validate", before: StateConfigured, start: EventStartValidate, transient: StateValidating, done: EventValidateDone, fail: EventValidateFail, handler: m.handler.Validate},
		{name: "install", before: StateValidated, start: EventStartInstall, transient: StateInstalling, done: EventInstallDone, fail: EventInstallFail, handler: m.handler.Install},
		{name: "blueprints", before: StateInstalled, start: EventStartBlueprints, transient: StateApplyingBlueprints, done: EventBlueprintsDone, fail: EventBlueprintsFail, handler: m.handler.ApplyBlueprints},
		{name: "verify", before: StateBlueprintsApplied, start: EventStartVerify, transient: StateVerifying, done: EventVerifyDone, fail: EventVerifyFail, handler: m.handler.Verify},
	}
}

// Run drives the machine from its current state to a terminal state,
// invoking the phase handler for each phase along the way. It is the
// caller's responsibility to ensure handler methods are idempotent —
// a crash mid-phase resumes by re-calling the same method.
//
// On the first call, Run adopts a persisted checkpoint if one
// exists. Subsequent calls on a terminal machine return nil
// immediately (idempotent re-run).
//
// A phase failure transitions the machine to [StateFailed]. If
// WithAutoRollback is enabled (the default), Run then invokes
// [PhaseHandler.Rollback] and fires [EventRollback] — Run still
// returns the wrapped phase error so the operator knows the
// bootstrap did not succeed. If WithAutoRollback is disabled, Run
// returns the phase error and leaves state at [StateFailed].
func (m *BootstrapManager) Run(ctx context.Context) error {
	if _, err := m.machine.RestoreFrom(ctx); err != nil && !errors.Is(err, statemachine.ErrNoCheckpointer) {
		return fmt.Errorf("selfmgmt: restore checkpoint: %w", err)
	}

	steps := m.phases()
	for {
		cur := m.machine.Current()

		switch {
		case cur.IsTerminal():
			return nil
		case cur == StateFailed:
			// A prior Run landed in Failed without rolling back
			// (WithAutoRollback=false). Re-entering Run does not
			// retry — the operator must decide.
			return fmt.Errorf("%w: bootstrap is in %s; rerun is a no-op", ErrBootstrap, cur)
		}

		step, ok := findStep(steps, cur)
		if !ok {
			return fmt.Errorf("selfmgmt: no phase handles state %q", cur)
		}

		// Enter the transient state if not already there (resume case).
		if cur == step.before {
			if err := m.fireAndCheckpoint(ctx, step.start); err != nil {
				return err
			}
		}

		m.logger.DebugContext(ctx, "selfmgmt: phase starting", "phase", step.name, "state", m.machine.Current())
		phaseErr := step.handler(ctx, m.seed)

		if phaseErr != nil {
			failedAt := m.machine.Current()
			if err := m.fireAndCheckpoint(ctx, step.fail); err != nil {
				return errors.Join(phaseErr, err)
			}
			return m.handleFailure(ctx, step.name, failedAt, phaseErr)
		}

		if err := m.fireAndCheckpoint(ctx, step.done); err != nil {
			return err
		}
	}
}

// fireAndCheckpoint advances the machine and persists the new state.
// A checkpointing error is fatal: the caller would otherwise observe
// an in-memory advance that a crash would lose silently.
func (m *BootstrapManager) fireAndCheckpoint(ctx context.Context, event BootstrapEvent) error {
	if err := m.machine.Fire(ctx, event); err != nil {
		return fmt.Errorf("selfmgmt: fire %s: %w", event, err)
	}
	if err := m.machine.Checkpoint(ctx); err != nil && !errors.Is(err, statemachine.ErrNoCheckpointer) {
		return fmt.Errorf("selfmgmt: checkpoint after %s: %w", event, err)
	}
	return nil
}

// handleFailure is invoked once the machine has settled in
// [StateFailed]. With auto-rollback it asks the handler to roll back
// the phase scope (failedAt is the transient state) and advances to
// [StateRolledBack]. Either way Run reports the original phase
// error wrapped with [ErrBootstrap]; a rollback handler error joins
// onto the phase error.
func (m *BootstrapManager) handleFailure(ctx context.Context, phaseName string, failedAt BootstrapState, phaseErr error) error {
	wrapped := fmt.Errorf("%w at %s: %w", ErrBootstrap, phaseName, phaseErr)

	if !m.auto {
		return wrapped
	}

	if err := m.handler.Rollback(ctx, m.seed, failedAt); err != nil {
		return errors.Join(wrapped, fmt.Errorf("selfmgmt: rollback: %w", err))
	}
	if err := m.fireAndCheckpoint(ctx, EventRollback); err != nil {
		return errors.Join(wrapped, err)
	}
	return wrapped
}

// findStep picks the phase responsible for the current state. Each
// state belongs to exactly one phase — its `before` (resume between
// phases) or its `transient` (resume mid-phase).
func findStep(steps []phaseStep, cur BootstrapState) (phaseStep, bool) {
	for _, s := range steps {
		if cur == s.before || cur == s.transient {
			return s, true
		}
	}
	return phaseStep{}, false
}
