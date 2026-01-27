// Package execution provides the runbook execution engine.
package execution

import (
	"context"

	"github.com/shawnbutts/keystone-core/internal/runbook"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// ExecutionEvent represents events that drive execution state transitions.
type ExecutionEvent string

// Execution event constants.
const (
	EventStart    ExecutionEvent = "start"
	EventComplete ExecutionEvent = "complete"
	EventFail     ExecutionEvent = "fail"
	EventCancel   ExecutionEvent = "cancel"
)

// StepEvent represents events that drive step state transitions.
type StepEvent string

// Step event constants.
const (
	EventStepStart    StepEvent = "start"
	EventStepComplete StepEvent = "complete"
	EventStepFail     StepEvent = "fail"
	EventStepSkip     StepEvent = "skip"
)

// ExecutionMachine wraps a state machine for tracking runbook execution state.
// It provides type-safe methods for managing execution lifecycle.
//
// State diagram (Mermaid):
//
//	stateDiagram-v2
//	    [*] --> pending
//	    pending --> running: start
//	    pending --> cancelled: cancel
//	    running --> completed: complete
//	    running --> failed: fail
//	    running --> cancelled: cancel
//	    completed --> [*]
//	    failed --> [*]
//	    cancelled --> [*]
type ExecutionMachine struct {
	machine *statemachine.Machine[runbook.ExecutionState, ExecutionEvent]
}

// NewExecutionMachine creates a new state machine for tracking runbook execution.
func NewExecutionMachine() *ExecutionMachine {
	machine := statemachine.New[runbook.ExecutionState, ExecutionEvent](runbook.ExecutionStatePending).
		WithName("runbook-execution").
		WithHistory(100).
		// From pending
		AddTransition(runbook.ExecutionStatePending, EventStart, runbook.ExecutionStateRunning).
		AddTransition(runbook.ExecutionStatePending, EventCancel, runbook.ExecutionStateCancelled).
		// From running
		AddTransition(runbook.ExecutionStateRunning, EventComplete, runbook.ExecutionStateCompleted).
		AddTransition(runbook.ExecutionStateRunning, EventFail, runbook.ExecutionStateFailed).
		AddTransition(runbook.ExecutionStateRunning, EventCancel, runbook.ExecutionStateCancelled).
		MustBuild()

	return &ExecutionMachine{machine: machine}
}

// NewExecutionMachineWithCallbacks creates a new execution state machine with callbacks.
func NewExecutionMachineWithCallbacks(
	onStateChange func(ctx context.Context, from, to runbook.ExecutionState, event ExecutionEvent),
) *ExecutionMachine {
	builder := statemachine.New[runbook.ExecutionState, ExecutionEvent](runbook.ExecutionStatePending).
		WithName("runbook-execution").
		WithHistory(100).
		// From pending
		AddTransition(runbook.ExecutionStatePending, EventStart, runbook.ExecutionStateRunning).
		AddTransition(runbook.ExecutionStatePending, EventCancel, runbook.ExecutionStateCancelled).
		// From running
		AddTransition(runbook.ExecutionStateRunning, EventComplete, runbook.ExecutionStateCompleted).
		AddTransition(runbook.ExecutionStateRunning, EventFail, runbook.ExecutionStateFailed).
		AddTransition(runbook.ExecutionStateRunning, EventCancel, runbook.ExecutionStateCancelled)

	if onStateChange != nil {
		builder.OnTransition(onStateChange)
	}

	return &ExecutionMachine{machine: builder.MustBuild()}
}

// State returns the current execution state.
func (m *ExecutionMachine) State() runbook.ExecutionState {
	return m.machine.State()
}

// Start transitions from pending to running.
func (m *ExecutionMachine) Start(ctx context.Context) error {
	return m.machine.FireCtx(ctx, EventStart)
}

// Complete transitions from running to completed.
func (m *ExecutionMachine) Complete(ctx context.Context) error {
	return m.machine.FireCtx(ctx, EventComplete)
}

// Fail transitions from running to failed.
func (m *ExecutionMachine) Fail(ctx context.Context) error {
	return m.machine.FireCtx(ctx, EventFail)
}

// Cancel transitions to cancelled from pending or running.
func (m *ExecutionMachine) Cancel(ctx context.Context) error {
	return m.machine.FireCtx(ctx, EventCancel)
}

// CanStart returns true if the execution can be started.
func (m *ExecutionMachine) CanStart() bool {
	return m.machine.CanFire(EventStart)
}

// CanCancel returns true if the execution can be cancelled.
func (m *ExecutionMachine) CanCancel() bool {
	return m.machine.CanFire(EventCancel)
}

// IsTerminal returns true if the execution is in a terminal state.
func (m *ExecutionMachine) IsTerminal() bool {
	return m.machine.State().IsTerminal()
}

// IsPending returns true if the execution is pending.
func (m *ExecutionMachine) IsPending() bool {
	return m.machine.IsInState(runbook.ExecutionStatePending)
}

// IsRunning returns true if the execution is running.
func (m *ExecutionMachine) IsRunning() bool {
	return m.machine.IsInState(runbook.ExecutionStateRunning)
}

// StepMachine wraps a state machine for tracking step execution state.
// It provides type-safe methods for managing step lifecycle.
//
// State diagram (Mermaid):
//
//	stateDiagram-v2
//	    [*] --> pending
//	    pending --> running: start
//	    pending --> skipped: skip
//	    running --> completed: complete
//	    running --> failed: fail
//	    completed --> [*]
//	    failed --> [*]
//	    skipped --> [*]
type StepMachine struct {
	machine *statemachine.Machine[runbook.StepState, StepEvent]
	name    string
}

// NewStepMachine creates a new state machine for tracking step execution.
func NewStepMachine(stepName string) *StepMachine {
	machine := statemachine.New[runbook.StepState, StepEvent](runbook.StepStatePending).
		WithName("runbook-step-" + stepName).
		WithHistory(50).
		// From pending
		AddTransition(runbook.StepStatePending, EventStepStart, runbook.StepStateRunning).
		AddTransition(runbook.StepStatePending, EventStepSkip, runbook.StepStateSkipped).
		// From running
		AddTransition(runbook.StepStateRunning, EventStepComplete, runbook.StepStateCompleted).
		AddTransition(runbook.StepStateRunning, EventStepFail, runbook.StepStateFailed).
		MustBuild()

	return &StepMachine{
		machine: machine,
		name:    stepName,
	}
}

// NewStepMachineWithCallbacks creates a new step state machine with callbacks.
func NewStepMachineWithCallbacks(
	stepName string,
	onStateChange func(ctx context.Context, from, to runbook.StepState, event StepEvent),
) *StepMachine {
	builder := statemachine.New[runbook.StepState, StepEvent](runbook.StepStatePending).
		WithName("runbook-step-" + stepName).
		WithHistory(50).
		// From pending
		AddTransition(runbook.StepStatePending, EventStepStart, runbook.StepStateRunning).
		AddTransition(runbook.StepStatePending, EventStepSkip, runbook.StepStateSkipped).
		// From running
		AddTransition(runbook.StepStateRunning, EventStepComplete, runbook.StepStateCompleted).
		AddTransition(runbook.StepStateRunning, EventStepFail, runbook.StepStateFailed)

	if onStateChange != nil {
		builder.OnTransition(onStateChange)
	}

	return &StepMachine{
		machine: builder.MustBuild(),
		name:    stepName,
	}
}

// Name returns the step name.
func (m *StepMachine) Name() string {
	return m.name
}

// State returns the current step state.
func (m *StepMachine) State() runbook.StepState {
	return m.machine.State()
}

// Start transitions from pending to running.
func (m *StepMachine) Start(ctx context.Context) error {
	return m.machine.FireCtx(ctx, EventStepStart)
}

// Complete transitions from running to completed.
func (m *StepMachine) Complete(ctx context.Context) error {
	return m.machine.FireCtx(ctx, EventStepComplete)
}

// Fail transitions from running to failed.
func (m *StepMachine) Fail(ctx context.Context) error {
	return m.machine.FireCtx(ctx, EventStepFail)
}

// Skip transitions from pending to skipped (when condition is false).
func (m *StepMachine) Skip(ctx context.Context) error {
	return m.machine.FireCtx(ctx, EventStepSkip)
}

// CanStart returns true if the step can be started.
func (m *StepMachine) CanStart() bool {
	return m.machine.CanFire(EventStepStart)
}

// CanSkip returns true if the step can be skipped.
func (m *StepMachine) CanSkip() bool {
	return m.machine.CanFire(EventStepSkip)
}

// IsTerminal returns true if the step is in a terminal state.
func (m *StepMachine) IsTerminal() bool {
	return m.machine.State().IsTerminal()
}

// IsPending returns true if the step is pending.
func (m *StepMachine) IsPending() bool {
	return m.machine.IsInState(runbook.StepStatePending)
}

// IsRunning returns true if the step is running.
func (m *StepMachine) IsRunning() bool {
	return m.machine.IsInState(runbook.StepStateRunning)
}

// IsCompleted returns true if the step completed successfully.
func (m *StepMachine) IsCompleted() bool {
	return m.machine.IsInState(runbook.StepStateCompleted)
}

// IsFailed returns true if the step failed.
func (m *StepMachine) IsFailed() bool {
	return m.machine.IsInState(runbook.StepStateFailed)
}

// IsSkipped returns true if the step was skipped.
func (m *StepMachine) IsSkipped() bool {
	return m.machine.IsInState(runbook.StepStateSkipped)
}
