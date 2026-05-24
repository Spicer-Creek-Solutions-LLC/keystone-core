// SPDX-License-Identifier: Apache-2.0

package saga

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ExecutionStatus is the terminal classification of one [Execution].
// Running is reserved for in-flight executions visible via the [Log]
// during a long-running saga (v0.1 only writes Pending→Running→one of
// the three terminal states; per-step incremental writes are V1X
// checkpoint-resume territory).
type ExecutionStatus string

const (
	StatusPending   ExecutionStatus = "pending"   // saga created, not yet running
	StatusRunning   ExecutionStatus = "running"   // forward walk in progress
	StatusCompleted ExecutionStatus = "completed" // every Action succeeded
	StatusFailed    ExecutionStatus = "failed"    // an Action failed; every Compensate that ran succeeded
	StatusAborted   ExecutionStatus = "aborted"   // an Action failed; at least one Compensate also failed
)

// StepStatus is the per-step terminal classification recorded on the
// matching [StepResult].
type StepStatus int

const (
	StepPending          StepStatus = iota // not reached
	StepRunning                            // Action in flight
	StepCompleted                          // Action returned nil error
	StepFailed                             // Action returned an error; this step triggered the unwind
	StepCompensated                        // a previously-completed step's Compensate ran with nil error
	StepCompensateFailed                   // a previously-completed step's Compensate ran with a non-nil error
	StepSkipped                            // not reached by the forward walk (because an earlier step failed)
)

// String renders a [StepStatus] in canonical form.
func (s StepStatus) String() string {
	switch s {
	case StepPending:
		return "pending"
	case StepRunning:
		return "running"
	case StepCompleted:
		return "completed"
	case StepFailed:
		return "failed"
	case StepCompensated:
		return "compensated"
	case StepCompensateFailed:
		return "compensate-failed"
	case StepSkipped:
		return "skipped"
	}
	return fmt.Sprintf("StepStatus(%d)", int(s))
}

// Step is one atomic forward + compensation pair.
//
// Action is the forward operation. It receives the data threaded
// from the previous step's Action return (or `initial` for the first
// step) and returns the data to thread to the next step, or an
// error. A non-nil error halts the forward walk and triggers the
// reverse-compensation walk.
//
// Compensate is the inverse of Action. It runs ONLY if Action
// completed successfully *and* a later step's Action returned an
// error. It receives the same data Action *received* as input —
// representing the world before this step ran — so it can roll back
// to that state. A nil Compensate is treated as a no-op success
// (useful for read-only steps).
//
// Both Action and Compensate must honour ctx cancellation.
type Step struct {
	Name       string
	Action     func(ctx context.Context, data any) (any, error)
	Compensate func(ctx context.Context, data any) error
}

// StepResult records what happened to one [Step] during an
// [Execution].
//
// Status is the terminal classification. Error is the Action error
// when Status is StepFailed; CompensateError is the Compensate error
// when Status is StepCompensateFailed. Compensated is true whenever
// Compensate ran (regardless of outcome) — including the no-op case
// where Compensate was nil.
type StepResult struct {
	Name            string
	Status          StepStatus
	StartedAt       time.Time
	Duration        time.Duration
	Error           error
	Compensated     bool
	CompensateError error
	CompensateAt    time.Time
}

// Execution is the run record for one saga.
//
// Steps captures every declared step's outcome — completed forward
// steps that later got compensated, the step that triggered the
// unwind, and steps that never ran (StepSkipped). The slice is in
// declaration order; the reverse-compensation walk is reflected in
// the per-step Compensated/CompensateError fields rather than a
// re-ordered slice.
//
// Error is the first forward Action error that triggered the
// unwind, or nil for a [StatusCompleted] saga. CompensateErrors
// contains every Compensate error that occurred during the unwind
// (in reverse-walk order); empty for [StatusCompleted] or
// [StatusFailed].
type Execution struct {
	ID                string
	Name              string
	Status            ExecutionStatus
	Data              any
	Steps             []StepResult
	StartedAt         time.Time
	EndedAt           time.Time
	Error             error
	CompensateErrors  []error
}

// Coordinator runs sagas.
//
// Log is the persistence hook; nil disables logging (the
// in-memory-only mode used by most tests). Clock is the time source;
// nil falls back to [time.Now] (the test seam for deterministic
// timestamps).
type Coordinator struct {
	Log   Log
	Clock func() time.Time
}

// now returns the current time via the coordinator's Clock, or
// [time.Now] when Clock is nil.
func (c *Coordinator) now() time.Time {
	if c.Clock != nil {
		return c.Clock()
	}
	return time.Now()
}

// Run executes steps in order. On the first Action error it walks
// the completed steps in **reverse** invoking each step's
// Compensate. Compensate errors do NOT abort the unwind — they are
// recorded on the relevant [StepResult] and aggregated onto
// [Execution.CompensateErrors].
//
// Returns the [Execution] with its terminal Status (Completed /
// Failed / Aborted) regardless of outcome. The returned error is:
//   - nil when Status == Completed.
//   - the originating Action error when Status == Failed.
//   - the originating Action error joined with every Compensate
//     error when Status == Aborted (use [errors.Is]/[errors.As] to
//     traverse).
//
// Run respects ctx cancellation: a canceled ctx is treated as the
// current Action's failure and triggers compensation. Compensation
// itself does NOT bail on ctx cancellation — per §4.17 line 1223,
// cleanup keeps going so far as each Compensate's own code allows.
func (c *Coordinator) Run(ctx context.Context, name string, steps []Step, initial any) (*Execution, error) {
	exec := &Execution{
		ID:        uuid.NewString(),
		Name:      name,
		Status:    StatusRunning,
		Data:      initial,
		StartedAt: c.now(),
		Steps:     make([]StepResult, len(steps)),
	}
	// Pre-fill StepResults so callers always get a per-step row.
	for i, s := range steps {
		exec.Steps[i] = StepResult{Name: s.Name, Status: StepPending}
	}
	if c.Log != nil {
		_ = c.Log.SaveExecution(ctx, exec)
	}

	// inputs[i] is the data that was passed *into* steps[i].Action.
	// Compensate receives the same value so it can roll back to the
	// pre-step state.
	inputs := make([]any, len(steps))

	data := initial
	failedAt := -1
	for i, step := range steps {
		// Honor ctx cancellation as the current Action's failure
		// (forces compensation of prior steps).
		if err := ctx.Err(); err != nil {
			exec.Steps[i] = StepResult{
				Name:      step.Name,
				Status:    StepFailed,
				StartedAt: c.now(),
				Error:     err,
			}
			exec.Error = err
			failedAt = i
			break
		}

		inputs[i] = data
		started := c.now()
		exec.Steps[i].Status = StepRunning
		exec.Steps[i].StartedAt = started

		next, actErr := step.Action(ctx, data)
		duration := c.now().Sub(started)

		if actErr != nil {
			exec.Steps[i].Status = StepFailed
			exec.Steps[i].Duration = duration
			exec.Steps[i].Error = actErr
			exec.Error = actErr
			failedAt = i
			break
		}

		exec.Steps[i].Status = StepCompleted
		exec.Steps[i].Duration = duration
		data = next
	}
	exec.Data = data

	if failedAt < 0 {
		// All steps succeeded.
		exec.Status = StatusCompleted
		exec.EndedAt = c.now()
		if c.Log != nil {
			_ = c.Log.SaveExecution(ctx, exec)
		}
		return exec, nil
	}

	// Compensation walk: every step BEFORE failedAt got Completed
	// (so its inverse Compensate must run). Steps AFTER failedAt
	// never reached (mark Skipped).
	anyCompensateFailed := false
	for i := failedAt - 1; i >= 0; i-- {
		step := steps[i]
		exec.Steps[i].Compensated = true
		exec.Steps[i].CompensateAt = c.now()
		if step.Compensate == nil {
			// nil Compensate = no-op success.
			exec.Steps[i].Status = StepCompensated
			continue
		}
		// Use a context derived from ctx but NOT cancelled by it —
		// compensation must run as far as possible even if the
		// parent context cancelled. We let the Compensate's own
		// implementation decide whether to bail.
		if err := step.Compensate(context.WithoutCancel(ctx), inputs[i]); err != nil {
			exec.Steps[i].Status = StepCompensateFailed
			exec.Steps[i].CompensateError = err
			exec.CompensateErrors = append(exec.CompensateErrors, fmt.Errorf("compensate %q: %w", step.Name, err))
			anyCompensateFailed = true
			continue
		}
		exec.Steps[i].Status = StepCompensated
	}
	for i := failedAt + 1; i < len(steps); i++ {
		exec.Steps[i].Status = StepSkipped
	}

	if anyCompensateFailed {
		exec.Status = StatusAborted
	} else {
		exec.Status = StatusFailed
	}
	exec.EndedAt = c.now()
	if c.Log != nil {
		_ = c.Log.SaveExecution(ctx, exec)
	}

	// Return the originating Action error, joined with any
	// Compensate errors so callers using [errors.Is]/[errors.As] can
	// walk the full set.
	if len(exec.CompensateErrors) > 0 {
		all := append([]error{exec.Error}, exec.CompensateErrors...)
		return exec, errors.Join(all...)
	}
	return exec, exec.Error
}
