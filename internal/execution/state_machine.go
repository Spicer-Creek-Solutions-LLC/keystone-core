package execution

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// State represents the state of a command execution.
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
type State string

const (
	// StatePending indicates execution is waiting to start
	StatePending State = "pending"
	// StateRunning indicates command is executing
	StateRunning State = "running"
	// StateRetrying indicates waiting to retry after failure
	StateRetrying State = "retrying"
	// StateCompleted indicates execution finished successfully
	StateCompleted State = "completed"
	// StateFailed indicates execution finished with error
	StateFailed State = "failed"
	// StateCancelled indicates execution was cancelled
	StateCancelled State = "cancelled"
	// StateTimeout indicates execution timed out
	StateTimeout State = "timeout"
)

// Event represents events that trigger execution state transitions.
type Event string

const (
	// ExecEventStart starts the execution
	ExecEventStart Event = "start"
	// ExecEventComplete marks execution as completed
	ExecEventComplete Event = "complete"
	// ExecEventFail marks execution as failed
	ExecEventFail Event = "fail"
	// ExecEventTimeout marks execution as timed out
	ExecEventTimeout Event = "timeout"
	// ExecEventCancel cancels the execution
	ExecEventCancel Event = "cancel"
	// ExecEventRetryRequested requests a retry after failure
	ExecEventRetryRequested Event = "retry_requested"
	// ExecEventRetry starts a retry attempt
	ExecEventRetry Event = "retry"
	// ExecEventMaxRetriesExceeded indicates max retries reached
	ExecEventMaxRetriesExceeded Event = "max_retries_exceeded"
)

// Callbacks holds callbacks for execution state transitions.
type Callbacks struct {
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
	machine *statemachine.Machine[State, Event]

	// Tracking
	commandID   string
	callbacks   *Callbacks
	attempt     int
	maxAttempts int
	lastError   error
	startTime   time.Time
	endTime     time.Time
}

// NewManagedExecution creates a new managed execution with state machine.
func NewManagedExecution(req *ExecuteRequest, maxAttempts int, callbacks *Callbacks) *ManagedExecution {
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

	me.machine = statemachine.New[State, Event](StatePending).
		WithName("execution-"+req.CommandID).
		WithHistory(20).
		// From Pending
		AddTransition(StatePending, ExecEventStart, StateRunning).
		AddTransition(StatePending, ExecEventCancel, StateCancelled).
		// From Running
		AddTransition(StateRunning, ExecEventComplete, StateCompleted).
		AddTransition(StateRunning, ExecEventFail, StateFailed).
		AddTransition(StateRunning, ExecEventTimeout, StateTimeout).
		AddTransition(StateRunning, ExecEventCancel, StateCancelled).
		AddTransition(StateRunning, ExecEventRetryRequested, StateRetrying).
		// From Retrying
		AddTransition(StateRetrying, ExecEventRetry, StateRunning).
		AddTransition(StateRetrying, ExecEventCancel, StateCancelled).
		AddTransition(StateRetrying, ExecEventMaxRetriesExceeded, StateFailed).
		// Callbacks
		OnEnter(StateRunning, func(ctx context.Context, state, from State) {
			if from == StatePending {
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
		OnEnter(StateRetrying, func(ctx context.Context, state, from State) {
			if me.callbacks != nil && me.callbacks.OnRetrying != nil {
				me.callbacks.OnRetrying(me.commandID, me.attempt, me.maxAttempts)
			}
		}).
		OnEnter(StateCompleted, func(ctx context.Context, state, from State) {
			me.endTime = time.Now()
			me.Result.EndTime = me.endTime
			me.Result.ExitCode = 0
			if me.callbacks != nil && me.callbacks.OnCompleted != nil {
				me.callbacks.OnCompleted(me.commandID, me.Result)
			}
		}).
		OnEnter(StateFailed, func(ctx context.Context, state, from State) {
			me.endTime = time.Now()
			me.Result.EndTime = me.endTime
			me.Result.Error = me.lastError
			if me.callbacks != nil && me.callbacks.OnFailed != nil {
				me.callbacks.OnFailed(me.commandID, me.lastError)
			}
		}).
		OnEnter(StateTimeout, func(ctx context.Context, state, from State) {
			me.endTime = time.Now()
			me.Result.EndTime = me.endTime
			me.Result.Error = context.DeadlineExceeded
			if me.callbacks != nil && me.callbacks.OnTimeout != nil {
				me.callbacks.OnTimeout(me.commandID)
			}
		}).
		OnEnter(StateCancelled, func(ctx context.Context, state, from State) {
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
func (me *ManagedExecution) State() State {
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
	return me.machine.IsInState(StatePending)
}

// IsRunning returns true if execution is running.
func (me *ManagedExecution) IsRunning() bool {
	return me.machine.IsInState(StateRunning)
}

// IsRetrying returns true if execution is in retry state.
func (me *ManagedExecution) IsRetrying() bool {
	return me.machine.IsInState(StateRetrying)
}

// IsCompleted returns true if execution completed successfully.
func (me *ManagedExecution) IsCompleted() bool {
	return me.machine.IsInState(StateCompleted)
}

// IsTerminal returns true if execution is in a terminal state.
func (me *ManagedExecution) IsTerminal() bool {
	return me.machine.IsInAnyState(
		StateCompleted,
		StateFailed,
		StateCancelled,
		StateTimeout,
	)
}

// IsSuccessful returns true if execution completed successfully.
func (me *ManagedExecution) IsSuccessful() bool {
	return me.machine.IsInState(StateCompleted)
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
func (me *ManagedExecution) History() *statemachine.History[State, Event] {
	return me.machine.History()
}

// AvailableEvents returns events that can be fired from the current state.
func (me *ManagedExecution) AvailableEvents() []Event {
	return me.machine.AvailableEvents()
}

// StateToString returns a human-readable name for the state.
func StateToString(state State) string {
	switch state {
	case StatePending:
		return "Pending"
	case StateRunning:
		return "Running"
	case StateRetrying:
		return "Retrying"
	case StateCompleted:
		return "Completed"
	case StateFailed:
		return "Failed"
	case StateCancelled:
		return "Cancelled"
	case StateTimeout:
		return "Timeout"
	default:
		return string(state)
	}
}
