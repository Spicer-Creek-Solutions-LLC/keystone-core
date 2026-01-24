package upgrade

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// UpgradeEvent represents events that trigger upgrade state transitions.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Idle
//     Idle --> Pending: Start
//     Pending --> Validating: Validate
//     Validating --> Preparing: PrepareOk
//     Validating --> Failed: ValidationFailed
//     Preparing --> Upgrading: BeginUpgrade
//     Preparing --> Failed: PrepareFailed
//     Upgrading --> Verifying: UpgradeComplete
//     Upgrading --> Failed: UpgradeFailed
//     Verifying --> Completed: VerificationPassed
//     Verifying --> Failed: VerificationFailed
//     Failed --> RollingBack: InitiateRollback
//     RollingBack --> RolledBack: RollbackComplete
//     RollingBack --> Failed: RollbackFailed
//     Completed --> [*]
//     RolledBack --> [*]
//     Pending --> Cancelled: Cancel
//     Validating --> Cancelled: Cancel
//     Preparing --> Cancelled: Cancel
//     Cancelled --> [*]
// ```
type UpgradeEvent string

const (
	// EventStart initiates the upgrade process
	EventStart UpgradeEvent = "start"
	// EventValidate begins validation
	EventValidate UpgradeEvent = "validate"
	// EventPrepareOk indicates validation passed
	EventPrepareOk UpgradeEvent = "prepare_ok"
	// EventValidationFailed indicates validation failed
	EventValidationFailed UpgradeEvent = "validation_failed"
	// EventBeginUpgrade starts the actual upgrade
	EventBeginUpgrade UpgradeEvent = "begin_upgrade"
	// EventPrepareFailed indicates preparation failed
	EventPrepareFailed UpgradeEvent = "prepare_failed"
	// EventUpgradeComplete indicates upgrade finished (may have errors)
	EventUpgradeComplete UpgradeEvent = "upgrade_complete"
	// EventUpgradeFailed indicates upgrade failed
	EventUpgradeFailed UpgradeEvent = "upgrade_failed"
	// EventVerificationPassed indicates verification succeeded
	EventVerificationPassed UpgradeEvent = "verification_passed"
	// EventVerificationFailed indicates verification failed
	EventVerificationFailed UpgradeEvent = "verification_failed"
	// EventInitiateRollback starts the rollback process
	EventInitiateRollback UpgradeEvent = "initiate_rollback"
	// EventRollbackComplete indicates rollback finished
	EventRollbackComplete UpgradeEvent = "rollback_complete"
	// EventRollbackFailed indicates rollback failed
	EventRollbackFailed UpgradeEvent = "rollback_failed"
	// EventCancel cancels the upgrade
	EventCancel UpgradeEvent = "cancel"
)

// UpgradeStateMachine manages the lifecycle of an upgrade operation.
type UpgradeStateMachine struct {
	machine  *statemachine.Machine[UpgradePhase, UpgradeEvent]
	strategy UpgradeStrategy
	state    *UpgradeState

	// Callbacks
	onPhaseChange func(from, to UpgradePhase)
	onError       func(phase UpgradePhase, err error)
}

// UpgradeStateMachineConfig configures the upgrade state machine.
type UpgradeStateMachineConfig struct {
	// Strategy determines which transitions are valid
	Strategy UpgradeStrategy
	// InitialState is the starting state (defaults to Idle)
	InitialState UpgradePhase
	// OnPhaseChange is called when the phase changes
	OnPhaseChange func(from, to UpgradePhase)
	// OnError is called when entering a failed state
	OnError func(phase UpgradePhase, err error)
	// HistorySize is the number of transitions to track (0 disables)
	HistorySize int
}

// DefaultUpgradeStateMachineConfig returns the default configuration.
func DefaultUpgradeStateMachineConfig() *UpgradeStateMachineConfig {
	return &UpgradeStateMachineConfig{
		Strategy:     StrategyRolling,
		InitialState: PhaseIdle,
		HistorySize:  50,
	}
}

// NewUpgradeStateMachine creates a new upgrade state machine.
func NewUpgradeStateMachine(config *UpgradeStateMachineConfig) *UpgradeStateMachine {
	if config == nil {
		config = DefaultUpgradeStateMachineConfig()
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

	usm := &UpgradeStateMachine{
		strategy:      strategy,
		onPhaseChange: config.OnPhaseChange,
		onError:       config.OnError,
		state: &UpgradeState{
			Phase:  initialState,
			Status: StatusPending,
		},
	}

	builder := statemachine.New[UpgradePhase, UpgradeEvent](initialState).
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
		builder.OnTransition(func(ctx context.Context, from, to UpgradePhase, event UpgradeEvent) {
			onPhaseChange(from, to)
		})
	}

	// Track state entry for error handling
	onError := usm.onError
	builder.
		OnEnter(PhaseFailed, func(ctx context.Context, phase, from UpgradePhase) {
			usm.state.Status = StatusFailed
			if onError != nil {
				// Extract error from context if available
				if err, ok := ctx.Value(contextKeyError{}).(error); ok {
					onError(from, err)
				}
			}
		}).
		OnEnter(PhaseCompleted, func(ctx context.Context, phase, from UpgradePhase) {
			usm.state.Status = StatusCompleted
			now := time.Now()
			usm.state.EndTime = &now
		}).
		OnEnter(PhaseRolledBack, func(ctx context.Context, phase, from UpgradePhase) {
			usm.state.Status = StatusRolledBack
			now := time.Now()
			usm.state.EndTime = &now
		}).
		OnEnter(PhaseCancelled, func(ctx context.Context, phase, from UpgradePhase) {
			usm.state.Status = StatusCancelled
			now := time.Now()
			usm.state.EndTime = &now
		}).
		OnEnter(PhaseUpgrading, func(ctx context.Context, phase, from UpgradePhase) {
			usm.state.Status = StatusInProgress
		})

	usm.machine = builder.MustBuild()
	return usm
}

// contextKeyError is a context key for passing errors.
type contextKeyError struct{}

// Phase returns the current upgrade phase.
func (usm *UpgradeStateMachine) Phase() UpgradePhase {
	return usm.machine.State()
}

// Status returns the derived status from the current phase.
func (usm *UpgradeStateMachine) Status() UpgradeStatus {
	phase := usm.machine.State()
	return PhaseToStatus(phase)
}

// PhaseToStatus maps a phase to its corresponding status.
func PhaseToStatus(phase UpgradePhase) UpgradeStatus {
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
func (usm *UpgradeStateMachine) Fire(event UpgradeEvent) error {
	return usm.FireCtx(context.Background(), event)
}

// FireCtx triggers a state transition with context.
func (usm *UpgradeStateMachine) FireCtx(ctx context.Context, event UpgradeEvent) error {
	err := usm.machine.FireCtx(ctx, event)
	if err == nil {
		usm.state.Phase = usm.machine.State()
		usm.state.Status = usm.Status()
	}
	return err
}

// FireWithError triggers a transition to a failed state with error context.
func (usm *UpgradeStateMachine) FireWithError(event UpgradeEvent, err error) error {
	ctx := context.WithValue(context.Background(), contextKeyError{}, err)
	return usm.machine.FireCtx(ctx, event)
}

// CanFire returns true if the event can trigger a transition.
func (usm *UpgradeStateMachine) CanFire(event UpgradeEvent) bool {
	return usm.machine.CanFire(event)
}

// AvailableEvents returns events that can be fired from the current phase.
func (usm *UpgradeStateMachine) AvailableEvents() []UpgradeEvent {
	return usm.machine.AvailableEvents()
}

// State returns the internal upgrade state.
func (usm *UpgradeStateMachine) State() *UpgradeState {
	return usm.state
}

// History returns the transition history.
func (usm *UpgradeStateMachine) History() *statemachine.History[UpgradePhase, UpgradeEvent] {
	return usm.machine.History()
}

// IsTerminal returns true if the upgrade is in a terminal state.
func (usm *UpgradeStateMachine) IsTerminal() bool {
	return usm.machine.IsInAnyState(PhaseCompleted, PhaseFailed, PhaseRolledBack, PhaseCancelled)
}

// IsActive returns true if the upgrade is actively processing.
func (usm *UpgradeStateMachine) IsActive() bool {
	return usm.machine.IsInAnyState(PhaseValidating, PhasePreparing, PhaseUpgrading, PhaseVerifying, PhaseRollingBack)
}

// CanRollback returns true if rollback can be initiated.
func (usm *UpgradeStateMachine) CanRollback() bool {
	return usm.machine.CanFire(EventInitiateRollback)
}

// CanCancel returns true if the upgrade can be cancelled.
func (usm *UpgradeStateMachine) CanCancel() bool {
	return usm.machine.CanFire(EventCancel)
}

// Progress returns the estimated progress percentage (0-100).
func (usm *UpgradeStateMachine) Progress() int {
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
func PhaseDisplayName(phase UpgradePhase) string {
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
const PhaseCancelled UpgradePhase = "cancelled"
