package upgrade

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// Event represents events that trigger upgrade state transitions.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Idle
//	Idle --> Pending: Start
//	Pending --> Validating: Validate
//	Validating --> Preparing: PrepareOk
//	Validating --> Failed: ValidationFailed
//	Preparing --> Upgrading: BeginUpgrade
//	Preparing --> Failed: PrepareFailed
//	Upgrading --> Verifying: UpgradeComplete
//	Upgrading --> Failed: UpgradeFailed
//	Verifying --> Completed: VerificationPassed
//	Verifying --> Failed: VerificationFailed
//	Failed --> RollingBack: InitiateRollback
//	RollingBack --> RolledBack: RollbackComplete
//	RollingBack --> Failed: RollbackFailed
//	Completed --> [*]
//	RolledBack --> [*]
//	Pending --> Cancelled: Cancel
//	Validating --> Cancelled: Cancel
//	Preparing --> Cancelled: Cancel
//	Cancelled --> [*]
//
// ```
type Event string

const (
	// EventStart initiates the upgrade process
	EventStart Event = "start"
	// EventValidate begins validation
	EventValidate Event = "validate"
	// EventPrepareOk indicates validation passed
	EventPrepareOk Event = "prepare_ok"
	// EventValidationFailed indicates validation failed
	EventValidationFailed Event = "validation_failed"
	// EventBeginUpgrade starts the actual upgrade
	EventBeginUpgrade Event = "begin_upgrade"
	// EventPrepareFailed indicates preparation failed
	EventPrepareFailed Event = "prepare_failed"
	// EventUpgradeComplete indicates upgrade finished (may have errors)
	EventUpgradeComplete Event = "upgrade_complete"
	// EventUpgradeFailed indicates upgrade failed
	EventUpgradeFailed Event = "upgrade_failed"
	// EventVerificationPassed indicates verification succeeded
	EventVerificationPassed Event = "verification_passed"
	// EventVerificationFailed indicates verification failed
	EventVerificationFailed Event = "verification_failed"
	// EventInitiateRollback starts the rollback process
	EventInitiateRollback Event = "initiate_rollback"
	// EventRollbackComplete indicates rollback finished
	EventRollbackComplete Event = "rollback_complete"
	// EventRollbackFailed indicates rollback failed
	EventRollbackFailed Event = "rollback_failed"
	// EventCancel cancels the upgrade
	EventCancel Event = "cancel"
)

// StateMachine manages the lifecycle of an upgrade operation.
type StateMachine struct {
	machine  *statemachine.Machine[Phase, Event]
	strategy Strategy
	state    *State

	// Callbacks
	onPhaseChange func(from, to Phase)
	onError       func(phase Phase, err error)
}

// StateMachineConfig configures the upgrade state machine.
type StateMachineConfig struct {
	// Strategy determines which transitions are valid
	Strategy Strategy
	// InitialState is the starting state (defaults to Idle)
	InitialState Phase
	// OnPhaseChange is called when the phase changes
	OnPhaseChange func(from, to Phase)
	// OnError is called when entering a failed state
	OnError func(phase Phase, err error)
	// HistorySize is the number of transitions to track (0 disables)
	HistorySize int
}

// DefaultStateMachineConfig returns the default configuration.
func DefaultStateMachineConfig() *StateMachineConfig {
	return &StateMachineConfig{
		Strategy:     StrategyRolling,
		InitialState: PhaseIdle,
		HistorySize:  50,
	}
}

// NewStateMachine creates a new upgrade state machine.
func NewStateMachine(config *StateMachineConfig) *StateMachine {
	if config == nil {
		config = DefaultStateMachineConfig()
	}

	// Apply defaults for unset fields
	initialState := config.InitialState
	if initialState == "" {
		initialState = PhaseIdle
	}
	strategy := config.Strategy
	if strategy == "" {
		strategy = StrategyRolling
	}
	historySize := config.HistorySize
	if historySize == 0 {
		historySize = 50
	}

	usm := &StateMachine{
		strategy:      strategy,
		onPhaseChange: config.OnPhaseChange,
		onError:       config.OnError,
		state: &State{
			Phase:  initialState,
			Status: StatusPending,
		},
	}

	builder := statemachine.New[Phase, Event](initialState).
		WithName(fmt.Sprintf("upgrade-%s", strategy))

	if historySize > 0 {
		builder.WithHistory(historySize)
	}

	// Common transitions for all strategies
	builder.
		// Start from idle
		AddTransition(PhaseIdle, EventStart, PhasePending).
		// Begin validation
		AddTransition(PhasePending, EventValidate, PhaseValidating).
		AddTransition(PhasePending, EventCancel, PhaseCancelled).
		// Validation outcomes
		AddTransition(PhaseValidating, EventPrepareOk, PhasePreparing).
		AddTransition(PhaseValidating, EventValidationFailed, PhaseFailed).
		AddTransition(PhaseValidating, EventCancel, PhaseCancelled).
		// Preparation outcomes
		AddTransition(PhasePreparing, EventBeginUpgrade, PhaseUpgrading).
		AddTransition(PhasePreparing, EventPrepareFailed, PhaseFailed).
		AddTransition(PhasePreparing, EventCancel, PhaseCancelled).
		// Upgrade outcomes
		AddTransition(PhaseUpgrading, EventUpgradeComplete, PhaseVerifying).
		AddTransition(PhaseUpgrading, EventUpgradeFailed, PhaseFailed).
		// Verification outcomes
		AddTransition(PhaseVerifying, EventVerificationPassed, PhaseCompleted).
		AddTransition(PhaseVerifying, EventVerificationFailed, PhaseFailed).
		// Rollback from failed state
		AddTransition(PhaseFailed, EventInitiateRollback, PhaseRollingBack).
		// Rollback outcomes
		AddTransition(PhaseRollingBack, EventRollbackComplete, PhaseRolledBack).
		AddTransition(PhaseRollingBack, EventRollbackFailed, PhaseFailed)

	// Add callbacks - capture usm.onPhaseChange in closure since usm is already set
	onPhaseChange := usm.onPhaseChange
	if onPhaseChange != nil {
		builder.OnTransition(func(ctx context.Context, from, to Phase, event Event) {
			onPhaseChange(from, to)
		})
	}

	// Track state entry for error handling
	onError := usm.onError
	builder.
		OnEnter(PhaseFailed, func(ctx context.Context, phase, from Phase) {
			usm.state.Status = StatusFailed
			if onError != nil {
				// Extract error from context if available
				if err, ok := ctx.Value(contextKeyError{}).(error); ok {
					onError(from, err)
				}
			}
		}).
		OnEnter(PhaseCompleted, func(ctx context.Context, phase, from Phase) {
			usm.state.Status = StatusCompleted
			now := time.Now()
			usm.state.EndTime = &now
		}).
		OnEnter(PhaseRolledBack, func(ctx context.Context, phase, from Phase) {
			usm.state.Status = StatusRolledBack
			now := time.Now()
			usm.state.EndTime = &now
		}).
		OnEnter(PhaseCancelled, func(ctx context.Context, phase, from Phase) {
			usm.state.Status = StatusCancelled
			now := time.Now()
			usm.state.EndTime = &now
		}).
		OnEnter(PhaseUpgrading, func(ctx context.Context, phase, from Phase) {
			usm.state.Status = StatusInProgress
		})

	usm.machine = builder.MustBuild()
	return usm
}

// contextKeyError is a context key for passing errors.
type contextKeyError struct{}

// Phase returns the current upgrade phase.
func (usm *StateMachine) Phase() Phase {
	return usm.machine.State()
}

// Status returns the derived status from the current phase.
func (usm *StateMachine) Status() Status {
	phase := usm.machine.State()
	return PhaseToStatus(phase)
}

// PhaseToStatus maps a phase to its corresponding status.
func PhaseToStatus(phase Phase) Status {
	switch phase {
	case PhaseIdle, PhasePending:
		return StatusPending
	case PhaseValidating, PhasePreparing, PhaseUpgrading, PhaseVerifying:
		return StatusInProgress
	case PhaseCompleted:
		return StatusCompleted
	case PhaseFailed:
		return StatusFailed
	case PhaseRollingBack:
		return StatusInProgress
	case PhaseRolledBack:
		return StatusRolledBack
	case PhaseCancelled:
		return StatusCancelled
	default:
		return StatusPending
	}
}

// Fire triggers a state transition.
func (usm *StateMachine) Fire(event Event) error {
	return usm.FireCtx(context.Background(), event)
}

// FireCtx triggers a state transition with context.
func (usm *StateMachine) FireCtx(ctx context.Context, event Event) error {
	err := usm.machine.FireCtx(ctx, event)
	if err == nil {
		usm.state.Phase = usm.machine.State()
		usm.state.Status = usm.Status()
	}
	return err
}

// FireWithError triggers a transition to a failed state with error context.
func (usm *StateMachine) FireWithError(event Event, err error) error {
	ctx := context.WithValue(context.Background(), contextKeyError{}, err)
	return usm.machine.FireCtx(ctx, event)
}

// CanFire returns true if the event can trigger a transition.
func (usm *StateMachine) CanFire(event Event) bool {
	return usm.machine.CanFire(event)
}

// AvailableEvents returns events that can be fired from the current phase.
func (usm *StateMachine) AvailableEvents() []Event {
	return usm.machine.AvailableEvents()
}

// State returns the internal upgrade state.
func (usm *StateMachine) State() *State {
	return usm.state
}

// History returns the transition history.
func (usm *StateMachine) History() *statemachine.History[Phase, Event] {
	return usm.machine.History()
}

// IsTerminal returns true if the upgrade is in a terminal state.
func (usm *StateMachine) IsTerminal() bool {
	return usm.machine.IsInAnyState(PhaseCompleted, PhaseFailed, PhaseRolledBack, PhaseCancelled)
}

// IsActive returns true if the upgrade is actively processing.
func (usm *StateMachine) IsActive() bool {
	return usm.machine.IsInAnyState(PhaseValidating, PhasePreparing, PhaseUpgrading, PhaseVerifying, PhaseRollingBack)
}

// CanRollback returns true if rollback can be initiated.
func (usm *StateMachine) CanRollback() bool {
	return usm.machine.CanFire(EventInitiateRollback)
}

// CanCancel returns true if the upgrade can be cancelled.
func (usm *StateMachine) CanCancel() bool {
	return usm.machine.CanFire(EventCancel)
}

// Progress returns the estimated progress percentage (0-100).
func (usm *StateMachine) Progress() int {
	phase := usm.machine.State()
	switch phase {
	case PhaseIdle:
		return 0
	case PhasePending:
		return 5
	case PhaseValidating:
		return 15
	case PhasePreparing:
		return 30
	case PhaseUpgrading:
		return 60
	case PhaseVerifying:
		return 85
	case PhaseCompleted:
		return 100
	case PhaseFailed:
		return usm.state.Progress // Keep last progress
	case PhaseRollingBack:
		return 50 // Rolling back
	case PhaseRolledBack:
		return 0 // Back to start
	case PhaseCancelled:
		return usm.state.Progress // Keep last progress
	default:
		return 0
	}
}

// PhaseDisplayName returns a human-readable name for a phase.
func PhaseDisplayName(phase Phase) string {
	switch phase {
	case PhaseIdle:
		return "Idle"
	case PhasePending:
		return "Pending"
	case PhaseValidating:
		return "Validating"
	case PhasePreparing:
		return "Preparing"
	case PhaseUpgrading:
		return "Upgrading"
	case PhaseVerifying:
		return "Verifying"
	case PhaseCompleted:
		return "Completed"
	case PhaseFailed:
		return "Failed"
	case PhaseRollingBack:
		return "Rolling Back"
	case PhaseRolledBack:
		return "Rolled Back"
	case PhaseCancelled:
		return "Cancelled"
	default:
		return string(phase)
	}
}

// PhaseCancelled is the cancelled phase constant.
const PhaseCancelled Phase = "cancelled"
