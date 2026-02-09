package schedule

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// ExecutionEvent represents events that trigger execution state transitions.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> Approved: Approve
//	Pending --> Rejected: Reject
//	Pending --> Running: StartNoApproval
//	Pending --> Cancelled: Cancel
//	Approved --> Running: Start
//	Approved --> Cancelled: Cancel
//	Running --> Completed: Complete
//	Running --> Failed: Fail
//	Running --> Timeout: Timeout
//	Running --> Cancelled: Cancel
//	Completed --> [*]
//	Failed --> [*]
//	Timeout --> [*]
//	Cancelled --> [*]
//	Rejected --> [*]
//	Skipped --> [*]
//
// ```
type ExecutionEvent string

const (
	// ExecEventApprove approves a pending execution
	ExecEventApprove ExecutionEvent = "approve"
	// ExecEventReject rejects a pending execution
	ExecEventReject ExecutionEvent = "reject"
	// ExecEventStart starts the execution
	ExecEventStart ExecutionEvent = "start"
	// ExecEventStartNoApproval starts without approval (when not required)
	ExecEventStartNoApproval ExecutionEvent = "start_no_approval"
	// ExecEventComplete marks execution as completed
	ExecEventComplete ExecutionEvent = "complete"
	// ExecEventFail marks execution as failed
	ExecEventFail ExecutionEvent = "fail"
	// ExecEventTimeout marks execution as timed out
	ExecEventTimeout ExecutionEvent = "timeout"
	// ExecEventCancel cancels the execution
	ExecEventCancel ExecutionEvent = "cancel"
	// ExecEventSkip skips the execution
	ExecEventSkip ExecutionEvent = "skip"
)

// ExecutionCallbacks holds callbacks for execution state transitions.
type ExecutionCallbacks struct {
	// OnApproved is called when execution is approved
	OnApproved func(execID, approvedBy string)
	// OnRejected is called when execution is rejected
	OnRejected func(execID, rejectedBy, reason string)
	// OnStarted is called when execution starts
	OnStarted func(execID string)
	// OnCompleted is called when execution completes
	OnCompleted func(execID string, successCount, failureCount int)
	// OnFailed is called when execution fails
	OnFailed func(execID string, err error)
	// OnTimeout is called when execution times out
	OnTimeout func(execID string)
	// OnCancelled is called when execution is cancelled
	OnCancelled func(execID string)
	// OnSkipped is called when execution is skipped
	OnSkipped func(execID, reason string)
}

// ManagedExecution wraps Execution with a state machine.
type ManagedExecution struct {
	Execution *Execution
	machine   *statemachine.Machine[ExecutionStatus, ExecutionEvent]

	// Tracking
	execID     string
	callbacks  *ExecutionCallbacks
	approvedBy string
	rejectedBy string
	reason     string
	lastError  error
}

// NewManagedExecution creates a new managed execution with state machine.
func NewManagedExecution(exec *Execution, callbacks *ExecutionCallbacks) *ManagedExecution {
	me := &ManagedExecution{
		Execution: exec,
		execID:    exec.ID,
		callbacks: callbacks,
	}

	me.machine = statemachine.New[ExecutionStatus, ExecutionEvent](ExecutionStatusPending).
		WithName("schedule-exec-"+exec.ID).
		WithHistory(15).
		// From Pending
		AddTransition(ExecutionStatusPending, ExecEventApprove, ExecutionStatusApproved).
		AddTransition(ExecutionStatusPending, ExecEventReject, ExecutionStatusRejected).
		AddTransition(ExecutionStatusPending, ExecEventStartNoApproval, ExecutionStatusRunning).
		AddTransition(ExecutionStatusPending, ExecEventCancel, ExecutionStatusCancelled).
		AddTransition(ExecutionStatusPending, ExecEventSkip, ExecutionStatusSkipped).
		// From Approved
		AddTransition(ExecutionStatusApproved, ExecEventStart, ExecutionStatusRunning).
		AddTransition(ExecutionStatusApproved, ExecEventCancel, ExecutionStatusCancelled).
		// From Running
		AddTransition(ExecutionStatusRunning, ExecEventComplete, ExecutionStatusCompleted).
		AddTransition(ExecutionStatusRunning, ExecEventFail, ExecutionStatusFailed).
		AddTransition(ExecutionStatusRunning, ExecEventTimeout, ExecutionStatusTimeout).
		AddTransition(ExecutionStatusRunning, ExecEventCancel, ExecutionStatusCancelled).
		// Callbacks
		OnEnter(ExecutionStatusApproved, func(ctx context.Context, state, from ExecutionStatus) {
			me.Execution.Status = ExecutionStatusApproved
			now := time.Now()
			me.Execution.ApprovedAt = &now
			me.Execution.ApprovedBy = me.approvedBy
			if me.callbacks != nil && me.callbacks.OnApproved != nil {
				me.callbacks.OnApproved(me.execID, me.approvedBy)
			}
		}).
		OnEnter(ExecutionStatusRejected, func(ctx context.Context, state, from ExecutionStatus) {
			me.Execution.Status = ExecutionStatusRejected
			now := time.Now()
			me.Execution.RejectedAt = &now
			me.Execution.RejectedBy = me.rejectedBy
			me.Execution.RejectionReason = me.reason
			if me.callbacks != nil && me.callbacks.OnRejected != nil {
				me.callbacks.OnRejected(me.execID, me.rejectedBy, me.reason)
			}
		}).
		OnEnter(ExecutionStatusRunning, func(ctx context.Context, state, from ExecutionStatus) {
			me.Execution.Status = ExecutionStatusRunning
			now := time.Now()
			me.Execution.StartTime = &now
			if me.callbacks != nil && me.callbacks.OnStarted != nil {
				me.callbacks.OnStarted(me.execID)
			}
		}).
		OnEnter(ExecutionStatusCompleted, func(ctx context.Context, state, from ExecutionStatus) {
			me.Execution.Status = ExecutionStatusCompleted
			now := time.Now()
			me.Execution.EndTime = &now
			if me.Execution.StartTime != nil {
				me.Execution.Duration = now.Sub(*me.Execution.StartTime)
			}
			if me.callbacks != nil && me.callbacks.OnCompleted != nil {
				me.callbacks.OnCompleted(me.execID, me.Execution.SuccessCount, me.Execution.FailureCount)
			}
		}).
		OnEnter(ExecutionStatusFailed, func(ctx context.Context, state, from ExecutionStatus) {
			me.Execution.Status = ExecutionStatusFailed
			now := time.Now()
			me.Execution.EndTime = &now
			if me.Execution.StartTime != nil {
				me.Execution.Duration = now.Sub(*me.Execution.StartTime)
			}
			if me.lastError != nil {
				me.Execution.Error = me.lastError.Error()
			}
			if me.callbacks != nil && me.callbacks.OnFailed != nil {
				me.callbacks.OnFailed(me.execID, me.lastError)
			}
		}).
		OnEnter(ExecutionStatusTimeout, func(ctx context.Context, state, from ExecutionStatus) {
			me.Execution.Status = ExecutionStatusTimeout
			now := time.Now()
			me.Execution.EndTime = &now
			if me.Execution.StartTime != nil {
				me.Execution.Duration = now.Sub(*me.Execution.StartTime)
			}
			me.Execution.Error = "execution timed out"
			if me.callbacks != nil && me.callbacks.OnTimeout != nil {
				me.callbacks.OnTimeout(me.execID)
			}
		}).
		OnEnter(ExecutionStatusCancelled, func(ctx context.Context, state, from ExecutionStatus) {
			me.Execution.Status = ExecutionStatusCancelled
			now := time.Now()
			me.Execution.EndTime = &now
			if me.Execution.StartTime != nil {
				me.Execution.Duration = now.Sub(*me.Execution.StartTime)
			}
			if me.callbacks != nil && me.callbacks.OnCancelled != nil {
				me.callbacks.OnCancelled(me.execID)
			}
		}).
		OnEnter(ExecutionStatusSkipped, func(ctx context.Context, state, from ExecutionStatus) {
			me.Execution.Status = ExecutionStatusSkipped
			if me.callbacks != nil && me.callbacks.OnSkipped != nil {
				me.callbacks.OnSkipped(me.execID, me.reason)
			}
		}).
		MustBuild()

	return me
}

// Status returns the current execution status.
func (me *ManagedExecution) Status() ExecutionStatus {
	return me.machine.State()
}

// Approve approves the execution.
func (me *ManagedExecution) Approve(approvedBy string) error {
	me.approvedBy = approvedBy
	return me.machine.Fire(ExecEventApprove)
}

// Reject rejects the execution.
func (me *ManagedExecution) Reject(rejectedBy, reason string) error {
	me.rejectedBy = rejectedBy
	me.reason = reason
	return me.machine.Fire(ExecEventReject)
}

// Start starts the execution (from approved state).
func (me *ManagedExecution) Start() error {
	return me.machine.Fire(ExecEventStart)
}

// StartNoApproval starts the execution without requiring approval.
func (me *ManagedExecution) StartNoApproval() error {
	return me.machine.Fire(ExecEventStartNoApproval)
}

// Complete marks the execution as completed.
func (me *ManagedExecution) Complete() error {
	return me.machine.Fire(ExecEventComplete)
}

// Fail marks the execution as failed.
func (me *ManagedExecution) Fail(err error) error {
	me.lastError = err
	return me.machine.Fire(ExecEventFail)
}

// Timeout marks the execution as timed out.
func (me *ManagedExecution) Timeout() error {
	return me.machine.Fire(ExecEventTimeout)
}

// Cancel cancels the execution.
func (me *ManagedExecution) Cancel() error {
	return me.machine.Fire(ExecEventCancel)
}

// Skip skips the execution.
func (me *ManagedExecution) Skip(reason string) error {
	me.reason = reason
	return me.machine.Fire(ExecEventSkip)
}

// CanApprove returns true if the execution can be approved.
func (me *ManagedExecution) CanApprove() bool {
	return me.machine.CanFire(ExecEventApprove)
}

// CanReject returns true if the execution can be rejected.
func (me *ManagedExecution) CanReject() bool {
	return me.machine.CanFire(ExecEventReject)
}

// CanStart returns true if the execution can be started.
func (me *ManagedExecution) CanStart() bool {
	return me.machine.CanFire(ExecEventStart) || me.machine.CanFire(ExecEventStartNoApproval)
}

// CanCancel returns true if the execution can be cancelled.
func (me *ManagedExecution) CanCancel() bool {
	return me.machine.CanFire(ExecEventCancel)
}

// IsPending returns true if the execution is pending.
func (me *ManagedExecution) IsPending() bool {
	return me.machine.IsInState(ExecutionStatusPending)
}

// IsRunning returns true if the execution is running.
func (me *ManagedExecution) IsRunning() bool {
	return me.machine.IsInState(ExecutionStatusRunning)
}

// IsTerminal returns true if the execution is in a terminal state.
func (me *ManagedExecution) IsTerminal() bool {
	return me.machine.IsInAnyState(
		ExecutionStatusCompleted,
		ExecutionStatusFailed,
		ExecutionStatusTimeout,
		ExecutionStatusCancelled,
		ExecutionStatusRejected,
		ExecutionStatusSkipped,
	)
}

// IsSuccessful returns true if the execution completed successfully.
func (me *ManagedExecution) IsSuccessful() bool {
	return me.machine.IsInState(ExecutionStatusCompleted)
}

// History returns the state transition history.
func (me *ManagedExecution) History() *statemachine.History[ExecutionStatus, ExecutionEvent] {
	return me.machine.History()
}

// AvailableEvents returns events that can be fired from the current state.
func (me *ManagedExecution) AvailableEvents() []ExecutionEvent {
	return me.machine.AvailableEvents()
}

// ExecutionStatusToString returns a human-readable name for the status.
func ExecutionStatusToString(status ExecutionStatus) string {
	switch status {
	case ExecutionStatusPending:
		return "Pending"
	case ExecutionStatusApproved:
		return "Approved"
	case ExecutionStatusRunning:
		return "Running"
	case ExecutionStatusCompleted:
		return "Completed"
	case ExecutionStatusFailed:
		return "Failed"
	case ExecutionStatusCancelled:
		return "Cancelled"
	case ExecutionStatusSkipped:
		return "Skipped"
	case ExecutionStatusTimeout:
		return "Timeout"
	case ExecutionStatusRejected:
		return "Rejected"
	default:
		return string(status)
	}
}
