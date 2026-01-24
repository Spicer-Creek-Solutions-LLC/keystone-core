package controlplane

import (
	"context"
	"time"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// BatchJobEvent represents events that trigger batch job state transitions.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Pending
//     Pending --> Running: Start
//     Pending --> Cancelled: Cancel
//     Running --> Completed: Complete
//     Running --> Failed: Fail
//     Running --> Cancelled: Cancel
//     Completed --> [*]
//     Failed --> [*]
//     Cancelled --> [*]
// ```
type BatchJobEvent string

const (
	// BatchEventStart starts the batch job
	BatchEventStart BatchJobEvent = "start"
	// BatchEventComplete marks the job as completed
	BatchEventComplete BatchJobEvent = "complete"
	// BatchEventFail marks the job as failed
	BatchEventFail BatchJobEvent = "fail"
	// BatchEventCancel cancels the job
	BatchEventCancel BatchJobEvent = "cancel"
)

// BatchJobCallbacks holds callbacks for batch job state transitions.
type BatchJobCallbacks struct {
	// OnStarted is called when the batch job starts
	OnStarted func(jobID string)
	// OnCompleted is called when the batch job completes
	OnCompleted func(jobID string, successRate float32)
	// OnFailed is called when the batch job fails
	OnFailed func(jobID string, err error)
	// OnCancelled is called when the batch job is cancelled
	OnCancelled func(jobID string)
}

// ManagedBatchJob wraps BatchJob with a state machine.
type ManagedBatchJob struct {
	Job     *BatchJob
	machine *statemachine.Machine[pb.BatchJobStatus, BatchJobEvent]

	// Tracking
	jobID       string
	callbacks   *BatchJobCallbacks
	lastError   error
	successRate float32
}

// NewManagedBatchJob creates a new managed batch job with state machine.
func NewManagedBatchJob(job *BatchJob, callbacks *BatchJobCallbacks) *ManagedBatchJob {
	mbj := &ManagedBatchJob{
		Job:       job,
		jobID:     job.ID,
		callbacks: callbacks,
	}

	mbj.machine = statemachine.New[pb.BatchJobStatus, BatchJobEvent](pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING).
		WithName("batch-job-" + job.ID).
		WithHistory(10).
		// From Pending
		AddTransition(pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING, BatchEventStart, pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING).
		AddTransition(pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING, BatchEventCancel, pb.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED).
		// From Running
		AddTransition(pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING, BatchEventComplete, pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED).
		AddTransition(pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING, BatchEventFail, pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED).
		AddTransition(pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING, BatchEventCancel, pb.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED).
		// Callbacks
		OnEnter(pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING, func(ctx context.Context, state, from pb.BatchJobStatus) {
			now := time.Now()
			mbj.Job.StartedAt = &now
			mbj.Job.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING
			if mbj.callbacks != nil && mbj.callbacks.OnStarted != nil {
				mbj.callbacks.OnStarted(mbj.jobID)
			}
		}).
		OnEnter(pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED, func(ctx context.Context, state, from pb.BatchJobStatus) {
			now := time.Now()
			mbj.Job.CompletedAt = &now
			mbj.Job.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED
			if mbj.callbacks != nil && mbj.callbacks.OnCompleted != nil {
				mbj.callbacks.OnCompleted(mbj.jobID, mbj.successRate)
			}
		}).
		OnEnter(pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED, func(ctx context.Context, state, from pb.BatchJobStatus) {
			now := time.Now()
			mbj.Job.CompletedAt = &now
			mbj.Job.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED
			if mbj.callbacks != nil && mbj.callbacks.OnFailed != nil {
				mbj.callbacks.OnFailed(mbj.jobID, mbj.lastError)
			}
		}).
		OnEnter(pb.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED, func(ctx context.Context, state, from pb.BatchJobStatus) {
			now := time.Now()
			mbj.Job.CompletedAt = &now
			mbj.Job.Status = pb.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED
			if mbj.callbacks != nil && mbj.callbacks.OnCancelled != nil {
				mbj.callbacks.OnCancelled(mbj.jobID)
			}
		}).
		MustBuild()

	return mbj
}

// Status returns the current batch job status.
func (mbj *ManagedBatchJob) Status() pb.BatchJobStatus {
	return mbj.machine.State()
}

// Start starts the batch job.
func (mbj *ManagedBatchJob) Start() error {
	return mbj.machine.Fire(BatchEventStart)
}

// Complete marks the batch job as completed.
func (mbj *ManagedBatchJob) Complete(successRate float32) error {
	mbj.successRate = successRate
	return mbj.machine.Fire(BatchEventComplete)
}

// Fail marks the batch job as failed.
func (mbj *ManagedBatchJob) Fail(err error) error {
	mbj.lastError = err
	return mbj.machine.Fire(BatchEventFail)
}

// Cancel cancels the batch job.
func (mbj *ManagedBatchJob) Cancel() error {
	return mbj.machine.Fire(BatchEventCancel)
}

// CanStart returns true if the job can be started.
func (mbj *ManagedBatchJob) CanStart() bool {
	return mbj.machine.CanFire(BatchEventStart)
}

// CanCancel returns true if the job can be cancelled.
func (mbj *ManagedBatchJob) CanCancel() bool {
	return mbj.machine.CanFire(BatchEventCancel)
}

// IsPending returns true if the job is pending.
func (mbj *ManagedBatchJob) IsPending() bool {
	return mbj.machine.IsInState(pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING)
}

// IsRunning returns true if the job is running.
func (mbj *ManagedBatchJob) IsRunning() bool {
	return mbj.machine.IsInState(pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING)
}

// IsTerminal returns true if the job is in a terminal state.
func (mbj *ManagedBatchJob) IsTerminal() bool {
	return mbj.machine.IsInAnyState(
		pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED,
		pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED,
		pb.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED,
	)
}

// IsSuccessful returns true if the job completed successfully.
func (mbj *ManagedBatchJob) IsSuccessful() bool {
	return mbj.machine.IsInState(pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED)
}

// History returns the state transition history.
func (mbj *ManagedBatchJob) History() *statemachine.History[pb.BatchJobStatus, BatchJobEvent] {
	return mbj.machine.History()
}

// AvailableEvents returns events that can be fired from the current state.
func (mbj *ManagedBatchJob) AvailableEvents() []BatchJobEvent {
	return mbj.machine.AvailableEvents()
}

// Duration returns the duration of the job (0 if not started or still running).
func (mbj *ManagedBatchJob) Duration() time.Duration {
	if mbj.Job.StartedAt == nil {
		return 0
	}
	if mbj.Job.CompletedAt == nil {
		return time.Since(*mbj.Job.StartedAt)
	}
	return mbj.Job.CompletedAt.Sub(*mbj.Job.StartedAt)
}

// BatchJobStatusToString returns a human-readable name for the status.
func BatchJobStatusToString(status pb.BatchJobStatus) string {
	switch status {
	case pb.BatchJobStatus_BATCH_JOB_STATUS_UNSPECIFIED:
		return "Unspecified"
	case pb.BatchJobStatus_BATCH_JOB_STATUS_PENDING:
		return "Pending"
	case pb.BatchJobStatus_BATCH_JOB_STATUS_RUNNING:
		return "Running"
	case pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED:
		return "Completed"
	case pb.BatchJobStatus_BATCH_JOB_STATUS_FAILED:
		return "Failed"
	case pb.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED:
		return "Cancelled"
	default:
		return "Unknown"
	}
}
