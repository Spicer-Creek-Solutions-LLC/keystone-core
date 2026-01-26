package execution

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// ExecutionState represents the state of a command execution.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> Running: Start
//	Pending --> Cancelled: Cancel
//	Running --> Completed: Complete
//	Running --> Failed: Fail
//	Running --> Timeout: Timeout
//	Running --> Cancelled: Cancel
//	Running --> Retrying: RetryRequested
//	Retrying --> Running: Retry
//	Retrying --> Cancelled: Cancel
//	Retrying --> Failed: MaxRetriesExceeded
//	Completed --> [*]
//	Failed --> [*]
//	Cancelled --> [*]
//	Timeout --> [*]
//
// ```
type ExecutionState string

const (
	// ExecutionStatePending indicates execution is waiting to start
	ExecutionStatePending ExecutionState = "pending"
	// ExecutionStateRunning indicates command is executing
	ExecutionStateRunning ExecutionState = "running"
	// ExecutionStateRetrying indicates waiting to retry after failure
	ExecutionStateRetrying ExecutionState = "retrying"
	// ExecutionStateCompleted indicates execution finished successfully
	ExecutionStateCompleted ExecutionState = "completed"
	// ExecutionStateFailed indicates execution finished with error
	ExecutionStateFailed ExecutionState = "failed"
	// ExecutionStateCancelled indicates execution was cancelled
	ExecutionStateCancelled ExecutionState = "cancelled"
	// ExecutionStateTimeout indicates execution timed out
	ExecutionStateTimeout ExecutionState = "timeout"
)

// ExecutionEvent represents events that trigger execution state transitions.
type ExecutionEvent string

const (
	// ExecEventStart starts the execution
	ExecEventStart ExecutionEvent = "start"
	// ExecEventComplete marks execution as completed
	ExecEventComplete ExecutionEvent = "complete"
	// ExecEventFail marks execution as failed
	ExecEventFail ExecutionEvent = "fail"
	// ExecEventTimeout marks execution as timed out
	ExecEventTimeout ExecutionEvent = "timeout"
	// ExecEventCancel cancels the execution
	ExecEventCancel ExecutionEvent = "cancel"
	// ExecEventRetryRequested requests a retry after failure
	ExecEventRetryRequested ExecutionEvent = "retry_requested"
	// ExecEventRetry starts a retry attempt
	ExecEventRetry ExecutionEvent = "retry"
	// ExecEventMaxRetriesExceeded indicates max retries reached
	ExecEventMaxRetriesExceeded ExecutionEvent = "max_retries_exceeded"
)

// ExecutionCallbacks holds callbacks for execution state transitions.
type ExecutionCallbacks struct {
	// OnStarted is called when execution starts
	OnStarted func(commandID string)
	// OnCompleted is called when execution completes successfully
	OnCompleted func(commandID string, result *CommandResult)
	// OnFailed is called when execution fails
	OnFailed func(commandID string, err error)
	// OnTimeout is called when execution times out
	OnTimeout func(commandID string)
	// OnCancelled is called when execution is cancelled
	OnCancelled func(commandID string)
	// OnRetrying is called when entering retry state
	OnRetrying func(commandID string, attempt int, maxAttempts int)
	// OnRetry is called when starting a retry attempt
	OnRetry func(commandID string, attempt int)
}

// ManagedExecution wraps a command execution with a state machine.
type ManagedExecution struct {
	Request *ExecuteRequest
	Result  *CommandResult
	machine *statemachine.Machine[ExecutionState, ExecutionEvent]

	// Tracking
	commandID   string
	callbacks   *ExecutionCallbacks
	attempt     int
	maxAttempts int
	lastError   error
	startTime   time.Time
	endTime     time.Time
}

// NewManagedExecution creates a new managed execution with state machine.
func NewManagedExecution(req *ExecuteRequest, maxAttempts int, callbacks *ExecutionCallbacks) *ManagedExecution {
	me := &ManagedExecution{
		Request:     req,
		commandID:   req.CommandID,
		callbacks:   callbacks,
		attempt:     0,
		maxAttempts: maxAttempts,
		Result: &CommandResult{
			CommandID: req.CommandID,
		},
	}

	me.machine = statemachine.New[ExecutionState, ExecutionEvent](ExecutionStatePending).
		WithName("execution-"+req.CommandID).
		WithHistory(20).
		// From Pending
		AddTransition(ExecutionStatePending, ExecEventStart, ExecutionStateRunning).
		AddTransition(ExecutionStatePending, ExecEventCancel, ExecutionStateCancelled).
		// From Running
		AddTransition(ExecutionStateRunning, ExecEventComplete, ExecutionStateCompleted).
		AddTransition(ExecutionStateRunning, ExecEventFail, ExecutionStateFailed).
		AddTransition(ExecutionStateRunning, ExecEventTimeout, ExecutionStateTimeout).
		AddTransition(ExecutionStateRunning, ExecEventCancel, ExecutionStateCancelled).
		AddTransition(ExecutionStateRunning, ExecEventRetryRequested, ExecutionStateRetrying).
		// From Retrying
		AddTransition(ExecutionStateRetrying, ExecEventRetry, ExecutionStateRunning).
		AddTransition(ExecutionStateRetrying, ExecEventCancel, ExecutionStateCancelled).
		AddTransition(ExecutionStateRetrying, ExecEventMaxRetriesExceeded, ExecutionStateFailed).
		// Callbacks
		OnEnter(ExecutionStateRunning, func(ctx context.Context, state, from ExecutionState) {
			if from == ExecutionStatePending {
				me.startTime = time.Now()
				me.Result.StartTime = me.startTime
			}
			me.attempt++
			me.Result.Attempts = me.attempt
			if me.callbacks != nil {
				if me.callbacks.OnStarted != nil && me.attempt == 1 {
					me.callbacks.OnStarted(me.commandID)
				}
				if me.callbacks.OnRetry != nil && me.attempt > 1 {
					me.callbacks.OnRetry(me.commandID, me.attempt)
				}
			}
		}).
		OnEnter(ExecutionStateRetrying, func(ctx context.Context, state, from ExecutionState) {
			if me.callbacks != nil && me.callbacks.OnRetrying != nil {
				me.callbacks.OnRetrying(me.commandID, me.attempt, me.maxAttempts)
			}
		}).
		OnEnter(ExecutionStateCompleted, func(ctx context.Context, state, from ExecutionState) {
			me.endTime = time.Now()
			me.Result.EndTime = me.endTime
			me.Result.ExitCode = 0
			if me.callbacks != nil && me.callbacks.OnCompleted != nil {
				me.callbacks.OnCompleted(me.commandID, me.Result)
			}
		}).
		OnEnter(ExecutionStateFailed, func(ctx context.Context, state, from ExecutionState) {
			me.endTime = time.Now()
			me.Result.EndTime = me.endTime
			me.Result.Error = me.lastError
			if me.callbacks != nil && me.callbacks.OnFailed != nil {
				me.callbacks.OnFailed(me.commandID, me.lastError)
			}
		}).
		OnEnter(ExecutionStateTimeout, func(ctx context.Context, state, from ExecutionState) {
			me.endTime = time.Now()
			me.Result.EndTime = me.endTime
			me.Result.Error = context.DeadlineExceeded
			if me.callbacks != nil && me.callbacks.OnTimeout != nil {
				me.callbacks.OnTimeout(me.commandID)
			}
		}).
		OnEnter(ExecutionStateCancelled, func(ctx context.Context, state, from ExecutionState) {
			me.endTime = time.Now()
			me.Result.EndTime = me.endTime
			me.Result.Error = context.Canceled
			if me.callbacks != nil && me.callbacks.OnCancelled != nil {
				me.callbacks.OnCancelled(me.commandID)
			}
		}).
		MustBuild()

	return me
}

// State returns the current execution state.
func (me *ManagedExecution) State() ExecutionState {
	return me.machine.State()
}

// Start starts the execution.
func (me *ManagedExecution) Start() error {
	return me.machine.Fire(ExecEventStart)
}

// Complete marks the execution as completed.
func (me *ManagedExecution) Complete(stdout, stderr []byte, exitCode int) error {
	me.Result.Stdout = stdout
	me.Result.Stderr = stderr
	me.Result.ExitCode = exitCode
	return me.machine.Fire(ExecEventComplete)
}

// Fail marks the execution as failed.
func (me *ManagedExecution) Fail(err error) error {
	me.lastError = err
	return me.machine.Fire(ExecEventFail)
}

// Timeout marks the execution as timed out.
func (me *ManagedExecution) Timeout() error {
	me.lastError = context.DeadlineExceeded
	return me.machine.Fire(ExecEventTimeout)
}

// Cancel cancels the execution.
func (me *ManagedExecution) Cancel() error {
	me.lastError = context.Canceled
	return me.machine.Fire(ExecEventCancel)
}

// RequestRetry requests a retry after failure.
func (me *ManagedExecution) RequestRetry() error {
	return me.machine.Fire(ExecEventRetryRequested)
}

// Retry starts a retry attempt.
func (me *ManagedExecution) Retry() error {
	return me.machine.Fire(ExecEventRetry)
}

// FailMaxRetries marks that max retries have been exceeded.
func (me *ManagedExecution) FailMaxRetries() error {
	return me.machine.Fire(ExecEventMaxRetriesExceeded)
}

// CanRetry returns true if retry is possible (not at max attempts).
func (me *ManagedExecution) CanRetry() bool {
	return me.attempt < me.maxAttempts && me.machine.CanFire(ExecEventRetryRequested)
}

// IsPending returns true if execution is pending.
func (me *ManagedExecution) IsPending() bool {
	return me.machine.IsInState(ExecutionStatePending)
}

// IsRunning returns true if execution is running.
func (me *ManagedExecution) IsRunning() bool {
	return me.machine.IsInState(ExecutionStateRunning)
}

// IsRetrying returns true if execution is in retry state.
func (me *ManagedExecution) IsRetrying() bool {
	return me.machine.IsInState(ExecutionStateRetrying)
}

// IsCompleted returns true if execution completed successfully.
func (me *ManagedExecution) IsCompleted() bool {
	return me.machine.IsInState(ExecutionStateCompleted)
}

// IsTerminal returns true if execution is in a terminal state.
func (me *ManagedExecution) IsTerminal() bool {
	return me.machine.IsInAnyState(
		ExecutionStateCompleted,
		ExecutionStateFailed,
		ExecutionStateCancelled,
		ExecutionStateTimeout,
	)
}

// IsSuccessful returns true if execution completed successfully.
func (me *ManagedExecution) IsSuccessful() bool {
	return me.machine.IsInState(ExecutionStateCompleted)
}

// Attempt returns the current attempt number.
func (me *ManagedExecution) Attempt() int {
	return me.attempt
}

// MaxAttempts returns the maximum number of attempts.
func (me *ManagedExecution) MaxAttempts() int {
	return me.maxAttempts
}

// Duration returns the execution duration.
func (me *ManagedExecution) Duration() time.Duration {
	if me.startTime.IsZero() {
		return 0
	}
	if me.endTime.IsZero() {
		return time.Since(me.startTime)
	}
	return me.endTime.Sub(me.startTime)
}

// History returns the state transition history.
func (me *ManagedExecution) History() *statemachine.History[ExecutionState, ExecutionEvent] {
	return me.machine.History()
}

// AvailableEvents returns events that can be fired from the current state.
func (me *ManagedExecution) AvailableEvents() []ExecutionEvent {
	return me.machine.AvailableEvents()
}

// ExecutionStateToString returns a human-readable name for the state.
func ExecutionStateToString(state ExecutionState) string {
	switch state {
	case ExecutionStatePending:
		return "Pending"
	case ExecutionStateRunning:
		return "Running"
	case ExecutionStateRetrying:
		return "Retrying"
	case ExecutionStateCompleted:
		return "Completed"
	case ExecutionStateFailed:
		return "Failed"
	case ExecutionStateCancelled:
		return "Cancelled"
	case ExecutionStateTimeout:
		return "Timeout"
	default:
		return string(state)
	}
}
