package execution

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// PipelineState represents the state of a pipeline execution.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Pending
//     Pending --> Running: Start
//     Pending --> Cancelled: Cancel
//     Running --> Completed: Complete
//     Running --> PartiallyCompleted: PartialComplete
//     Running --> Failed: Fail
//     Running --> Cancelled: Cancel
//     Running --> Timeout: Timeout
//     Completed --> [*]
//     PartiallyCompleted --> [*]
//     Failed --> [*]
//     Cancelled --> [*]
//     Timeout --> [*]
// ```
type PipelineState string

const (
	// PipelineStatePending indicates pipeline is waiting to start
	PipelineStatePending PipelineState = "pending"
	// PipelineStateRunning indicates pipeline is executing
	PipelineStateRunning PipelineState = "running"
	// PipelineStateCompleted indicates all stages completed successfully
	PipelineStateCompleted PipelineState = "completed"
	// PipelineStatePartiallyCompleted indicates some stages failed but pipeline continued
	PipelineStatePartiallyCompleted PipelineState = "partially_completed"
	// PipelineStateFailed indicates pipeline stopped due to failure
	PipelineStateFailed PipelineState = "failed"
	// PipelineStateCancelled indicates pipeline was cancelled
	PipelineStateCancelled PipelineState = "cancelled"
	// PipelineStateTimeout indicates pipeline timed out
	PipelineStateTimeout PipelineState = "timeout"
)

// PipelineEvent represents events that trigger pipeline state transitions.
type PipelineEvent string

const (
	// PipelineEventStart starts the pipeline
	PipelineEventStart PipelineEvent = "start"
	// PipelineEventComplete marks pipeline as fully completed
	PipelineEventComplete PipelineEvent = "complete"
	// PipelineEventPartialComplete marks pipeline as partially completed
	PipelineEventPartialComplete PipelineEvent = "partial_complete"
	// PipelineEventFail marks pipeline as failed
	PipelineEventFail PipelineEvent = "fail"
	// PipelineEventCancel cancels the pipeline
	PipelineEventCancel PipelineEvent = "cancel"
	// PipelineEventTimeout marks pipeline as timed out
	PipelineEventTimeout PipelineEvent = "timeout"
)

// StageState represents the state of a pipeline stage.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Pending
//     Pending --> Running: Start
//     Pending --> Skipped: Skip
//     Running --> Completed: Complete
//     Running --> Failed: Fail
//     Completed --> [*]
//     Failed --> [*]
//     Skipped --> [*]
// ```
type StageState string

const (
	// StageStatePending indicates stage is waiting
	StageStatePending StageState = "pending"
	// StageStateRunning indicates stage is executing
	StageStateRunning StageState = "running"
	// StageStateCompleted indicates stage completed successfully
	StageStateCompleted StageState = "completed"
	// StageStateFailed indicates stage failed
	StageStateFailed StageState = "failed"
	// StageStateSkipped indicates stage was skipped
	StageStateSkipped StageState = "skipped"
)

// StageEvent represents events that trigger stage state transitions.
type StageEvent string

const (
	// StageEventStart starts the stage
	StageEventStart StageEvent = "start"
	// StageEventComplete marks stage as completed
	StageEventComplete StageEvent = "complete"
	// StageEventFail marks stage as failed
	StageEventFail StageEvent = "fail"
	// StageEventSkip marks stage as skipped
	StageEventSkip StageEvent = "skip"
)

// PipelineCallbacks holds callbacks for pipeline state transitions.
type PipelineCallbacks struct {
	// OnStarted is called when pipeline starts
	OnStarted func(pipelineID string)
	// OnCompleted is called when pipeline completes successfully
	OnCompleted func(pipelineID string, result *PipelineResult)
	// OnPartiallyCompleted is called when pipeline partially completes
	OnPartiallyCompleted func(pipelineID string, result *PipelineResult)
	// OnFailed is called when pipeline fails
	OnFailed func(pipelineID string, err error)
	// OnCancelled is called when pipeline is cancelled
	OnCancelled func(pipelineID string)
	// OnTimeout is called when pipeline times out
	OnTimeout func(pipelineID string)
	// OnStageStarted is called when a stage starts
	OnStageStarted func(pipelineID string, stageIndex int, stageID string)
	// OnStageCompleted is called when a stage completes
	OnStageCompleted func(pipelineID string, stageIndex int, stageID string, result *StageResult)
}

// ManagedPipeline wraps a Pipeline with a state machine.
type ManagedPipeline struct {
	Pipeline *Pipeline
	Result   *PipelineResult
	machine  *statemachine.Machine[PipelineState, PipelineEvent]

	// Stage machines
	stageMachines []*ManagedStage

	// Tracking
	pipelineID   string
	callbacks    *PipelineCallbacks
	currentStage int
	startTime    time.Time
	endTime      time.Time
	lastError    error
}

// ManagedStage wraps a stage with a state machine.
type ManagedStage struct {
	Stage   *PipelineStage
	Result  *StageResult
	machine *statemachine.Machine[StageState, StageEvent]

	// Tracking
	stageID    string
	stageIndex int
	startTime  time.Time
	endTime    time.Time
	lastError  error
}

// NewManagedPipeline creates a new managed pipeline with state machine.
func NewManagedPipeline(pipeline *Pipeline, callbacks *PipelineCallbacks) *ManagedPipeline {
	mp := &ManagedPipeline{
		Pipeline:   pipeline,
		pipelineID: pipeline.ID,
		callbacks:  callbacks,
		Result: &PipelineResult{
			PipelineID:   pipeline.ID,
			StageResults: make([]*StageResult, len(pipeline.Stages)),
			Success:      true,
		},
	}

	// Create stage machines
	mp.stageMachines = make([]*ManagedStage, len(pipeline.Stages))
	for i, stage := range pipeline.Stages {
		mp.stageMachines[i] = newManagedStage(stage, i)
		mp.Result.StageResults[i] = mp.stageMachines[i].Result
	}

	mp.machine = statemachine.New[PipelineState, PipelineEvent](PipelineStatePending).
		WithName("pipeline-" + pipeline.ID).
		WithHistory(20).
		// From Pending
		AddTransition(PipelineStatePending, PipelineEventStart, PipelineStateRunning).
		AddTransition(PipelineStatePending, PipelineEventCancel, PipelineStateCancelled).
		// From Running
		AddTransition(PipelineStateRunning, PipelineEventComplete, PipelineStateCompleted).
		AddTransition(PipelineStateRunning, PipelineEventPartialComplete, PipelineStatePartiallyCompleted).
		AddTransition(PipelineStateRunning, PipelineEventFail, PipelineStateFailed).
		AddTransition(PipelineStateRunning, PipelineEventCancel, PipelineStateCancelled).
		AddTransition(PipelineStateRunning, PipelineEventTimeout, PipelineStateTimeout).
		// Callbacks
		OnEnter(PipelineStateRunning, func(ctx context.Context, state, from PipelineState) {
			mp.startTime = time.Now()
			mp.Result.StartTime = mp.startTime
			if mp.callbacks != nil && mp.callbacks.OnStarted != nil {
				mp.callbacks.OnStarted(mp.pipelineID)
			}
		}).
		OnEnter(PipelineStateCompleted, func(ctx context.Context, state, from PipelineState) {
			mp.endTime = time.Now()
			mp.Result.EndTime = mp.endTime
			mp.Result.Success = true
			if mp.callbacks != nil && mp.callbacks.OnCompleted != nil {
				mp.callbacks.OnCompleted(mp.pipelineID, mp.Result)
			}
		}).
		OnEnter(PipelineStatePartiallyCompleted, func(ctx context.Context, state, from PipelineState) {
			mp.endTime = time.Now()
			mp.Result.EndTime = mp.endTime
			mp.Result.Success = false
			if mp.callbacks != nil && mp.callbacks.OnPartiallyCompleted != nil {
				mp.callbacks.OnPartiallyCompleted(mp.pipelineID, mp.Result)
			}
		}).
		OnEnter(PipelineStateFailed, func(ctx context.Context, state, from PipelineState) {
			mp.endTime = time.Now()
			mp.Result.EndTime = mp.endTime
			mp.Result.Success = false
			mp.Result.Error = mp.lastError
			if mp.callbacks != nil && mp.callbacks.OnFailed != nil {
				mp.callbacks.OnFailed(mp.pipelineID, mp.lastError)
			}
		}).
		OnEnter(PipelineStateCancelled, func(ctx context.Context, state, from PipelineState) {
			mp.endTime = time.Now()
			mp.Result.EndTime = mp.endTime
			mp.Result.Success = false
			mp.Result.Error = context.Canceled
			// Skip remaining stages
			for i := mp.currentStage; i < len(mp.stageMachines); i++ {
				if mp.stageMachines[i].State() == StageStatePending {
					mp.stageMachines[i].Skip()
				}
			}
			if mp.callbacks != nil && mp.callbacks.OnCancelled != nil {
				mp.callbacks.OnCancelled(mp.pipelineID)
			}
		}).
		OnEnter(PipelineStateTimeout, func(ctx context.Context, state, from PipelineState) {
			mp.endTime = time.Now()
			mp.Result.EndTime = mp.endTime
			mp.Result.Success = false
			mp.Result.Error = context.DeadlineExceeded
			// Skip remaining stages
			for i := mp.currentStage; i < len(mp.stageMachines); i++ {
				if mp.stageMachines[i].State() == StageStatePending {
					mp.stageMachines[i].Skip()
				}
			}
			if mp.callbacks != nil && mp.callbacks.OnTimeout != nil {
				mp.callbacks.OnTimeout(mp.pipelineID)
			}
		}).
		MustBuild()

	return mp
}

// newManagedStage creates a new managed stage with state machine.
func newManagedStage(stage *PipelineStage, index int) *ManagedStage {
	ms := &ManagedStage{
		Stage:      stage,
		stageID:    stage.ID,
		stageIndex: index,
		Result: &StageResult{
			StageID:    stage.ID,
			StageIndex: index,
		},
	}

	ms.machine = statemachine.New[StageState, StageEvent](StageStatePending).
		WithName("stage-" + stage.ID).
		WithHistory(10).
		// From Pending
		AddTransition(StageStatePending, StageEventStart, StageStateRunning).
		AddTransition(StageStatePending, StageEventSkip, StageStateSkipped).
		// From Running
		AddTransition(StageStateRunning, StageEventComplete, StageStateCompleted).
		AddTransition(StageStateRunning, StageEventFail, StageStateFailed).
		// Callbacks
		OnEnter(StageStateRunning, func(ctx context.Context, state, from StageState) {
			ms.startTime = time.Now()
			ms.Result.StartTime = ms.startTime
		}).
		OnEnter(StageStateCompleted, func(ctx context.Context, state, from StageState) {
			ms.endTime = time.Now()
			ms.Result.EndTime = ms.endTime
			ms.Result.ExitCode = 0
		}).
		OnEnter(StageStateFailed, func(ctx context.Context, state, from StageState) {
			ms.endTime = time.Now()
			ms.Result.EndTime = ms.endTime
			ms.Result.Error = ms.lastError
		}).
		OnEnter(StageStateSkipped, func(ctx context.Context, state, from StageState) {
			ms.Result.Skipped = true
		}).
		MustBuild()

	return ms
}

// State returns the current pipeline state.
func (mp *ManagedPipeline) State() PipelineState {
	return mp.machine.State()
}

// Start starts the pipeline.
func (mp *ManagedPipeline) Start() error {
	return mp.machine.Fire(PipelineEventStart)
}

// Complete marks the pipeline as completed.
func (mp *ManagedPipeline) Complete(finalOutput []byte) error {
	mp.Result.FinalOutput = finalOutput
	return mp.machine.Fire(PipelineEventComplete)
}

// PartialComplete marks the pipeline as partially completed.
func (mp *ManagedPipeline) PartialComplete(finalOutput []byte) error {
	mp.Result.FinalOutput = finalOutput
	return mp.machine.Fire(PipelineEventPartialComplete)
}

// Fail marks the pipeline as failed.
func (mp *ManagedPipeline) Fail(err error) error {
	mp.lastError = err
	return mp.machine.Fire(PipelineEventFail)
}

// Cancel cancels the pipeline.
func (mp *ManagedPipeline) Cancel() error {
	mp.lastError = context.Canceled
	return mp.machine.Fire(PipelineEventCancel)
}

// Timeout marks the pipeline as timed out.
func (mp *ManagedPipeline) Timeout() error {
	mp.lastError = context.DeadlineExceeded
	return mp.machine.Fire(PipelineEventTimeout)
}

// StartStage starts the specified stage.
func (mp *ManagedPipeline) StartStage(index int) error {
	if index < 0 || index >= len(mp.stageMachines) {
		return nil
	}
	mp.currentStage = index
	if mp.callbacks != nil && mp.callbacks.OnStageStarted != nil {
		mp.callbacks.OnStageStarted(mp.pipelineID, index, mp.stageMachines[index].stageID)
	}
	return mp.stageMachines[index].Start()
}

// CompleteStage marks the specified stage as completed.
func (mp *ManagedPipeline) CompleteStage(index int, output, stderr []byte, exitCode int) error {
	if index < 0 || index >= len(mp.stageMachines) {
		return nil
	}
	err := mp.stageMachines[index].Complete(output, stderr, exitCode)
	if mp.callbacks != nil && mp.callbacks.OnStageCompleted != nil {
		mp.callbacks.OnStageCompleted(mp.pipelineID, index, mp.stageMachines[index].stageID, mp.stageMachines[index].Result)
	}
	return err
}

// FailStage marks the specified stage as failed.
func (mp *ManagedPipeline) FailStage(index int, err error, stderr []byte, exitCode int) error {
	if index < 0 || index >= len(mp.stageMachines) {
		return nil
	}
	stageErr := mp.stageMachines[index].Fail(err, stderr, exitCode)
	if mp.callbacks != nil && mp.callbacks.OnStageCompleted != nil {
		mp.callbacks.OnStageCompleted(mp.pipelineID, index, mp.stageMachines[index].stageID, mp.stageMachines[index].Result)
	}
	return stageErr
}

// SkipStage marks the specified stage as skipped.
func (mp *ManagedPipeline) SkipStage(index int) error {
	if index < 0 || index >= len(mp.stageMachines) {
		return nil
	}
	return mp.stageMachines[index].Skip()
}

// GetStage returns the managed stage at the given index.
func (mp *ManagedPipeline) GetStage(index int) *ManagedStage {
	if index < 0 || index >= len(mp.stageMachines) {
		return nil
	}
	return mp.stageMachines[index]
}

// CurrentStageIndex returns the current stage index.
func (mp *ManagedPipeline) CurrentStageIndex() int {
	return mp.currentStage
}

// StageCount returns the number of stages.
func (mp *ManagedPipeline) StageCount() int {
	return len(mp.stageMachines)
}

// IsPending returns true if pipeline is pending.
func (mp *ManagedPipeline) IsPending() bool {
	return mp.machine.IsInState(PipelineStatePending)
}

// IsRunning returns true if pipeline is running.
func (mp *ManagedPipeline) IsRunning() bool {
	return mp.machine.IsInState(PipelineStateRunning)
}

// IsCompleted returns true if pipeline completed successfully.
func (mp *ManagedPipeline) IsCompleted() bool {
	return mp.machine.IsInState(PipelineStateCompleted)
}

// IsTerminal returns true if pipeline is in a terminal state.
func (mp *ManagedPipeline) IsTerminal() bool {
	return mp.machine.IsInAnyState(
		PipelineStateCompleted,
		PipelineStatePartiallyCompleted,
		PipelineStateFailed,
		PipelineStateCancelled,
		PipelineStateTimeout,
	)
}

// IsSuccessful returns true if pipeline completed successfully.
func (mp *ManagedPipeline) IsSuccessful() bool {
	return mp.machine.IsInState(PipelineStateCompleted)
}

// Duration returns the pipeline duration.
func (mp *ManagedPipeline) Duration() time.Duration {
	if mp.startTime.IsZero() {
		return 0
	}
	if mp.endTime.IsZero() {
		return time.Since(mp.startTime)
	}
	return mp.endTime.Sub(mp.startTime)
}

// History returns the state transition history.
func (mp *ManagedPipeline) History() *statemachine.History[PipelineState, PipelineEvent] {
	return mp.machine.History()
}

// AvailableEvents returns events that can be fired from the current state.
func (mp *ManagedPipeline) AvailableEvents() []PipelineEvent {
	return mp.machine.AvailableEvents()
}

// Stage state machine methods

// State returns the current stage state.
func (ms *ManagedStage) State() StageState {
	return ms.machine.State()
}

// Start starts the stage.
func (ms *ManagedStage) Start() error {
	return ms.machine.Fire(StageEventStart)
}

// Complete marks the stage as completed.
func (ms *ManagedStage) Complete(output, stderr []byte, exitCode int) error {
	ms.Result.Output = output
	ms.Result.Stderr = stderr
	ms.Result.ExitCode = exitCode
	return ms.machine.Fire(StageEventComplete)
}

// Fail marks the stage as failed.
func (ms *ManagedStage) Fail(err error, stderr []byte, exitCode int) error {
	ms.lastError = err
	ms.Result.Stderr = stderr
	ms.Result.ExitCode = exitCode
	return ms.machine.Fire(StageEventFail)
}

// Skip marks the stage as skipped.
func (ms *ManagedStage) Skip() error {
	return ms.machine.Fire(StageEventSkip)
}

// IsPending returns true if stage is pending.
func (ms *ManagedStage) IsPending() bool {
	return ms.machine.IsInState(StageStatePending)
}

// IsRunning returns true if stage is running.
func (ms *ManagedStage) IsRunning() bool {
	return ms.machine.IsInState(StageStateRunning)
}

// IsCompleted returns true if stage completed successfully.
func (ms *ManagedStage) IsCompleted() bool {
	return ms.machine.IsInState(StageStateCompleted)
}

// IsFailed returns true if stage failed.
func (ms *ManagedStage) IsFailed() bool {
	return ms.machine.IsInState(StageStateFailed)
}

// IsSkipped returns true if stage was skipped.
func (ms *ManagedStage) IsSkipped() bool {
	return ms.machine.IsInState(StageStateSkipped)
}

// IsTerminal returns true if stage is in a terminal state.
func (ms *ManagedStage) IsTerminal() bool {
	return ms.machine.IsInAnyState(StageStateCompleted, StageStateFailed, StageStateSkipped)
}

// Duration returns the stage duration.
func (ms *ManagedStage) Duration() time.Duration {
	if ms.startTime.IsZero() {
		return 0
	}
	if ms.endTime.IsZero() {
		return time.Since(ms.startTime)
	}
	return ms.endTime.Sub(ms.startTime)
}

// PipelineStateToString returns a human-readable name for the pipeline state.
func PipelineStateToString(state PipelineState) string {
	switch state {
	case PipelineStatePending:
		return "Pending"
	case PipelineStateRunning:
		return "Running"
	case PipelineStateCompleted:
		return "Completed"
	case PipelineStatePartiallyCompleted:
		return "Partially Completed"
	case PipelineStateFailed:
		return "Failed"
	case PipelineStateCancelled:
		return "Cancelled"
	case PipelineStateTimeout:
		return "Timeout"
	default:
		return string(state)
	}
}

// StageStateToString returns a human-readable name for the stage state.
func StageStateToString(state StageState) string {
	switch state {
	case StageStatePending:
		return "Pending"
	case StageStateRunning:
		return "Running"
	case StageStateCompleted:
		return "Completed"
	case StageStateFailed:
		return "Failed"
	case StageStateSkipped:
		return "Skipped"
	default:
		return string(state)
	}
}
