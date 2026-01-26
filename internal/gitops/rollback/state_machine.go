package rollback

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// RollbackEvent represents events that trigger rollback state transitions.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> Approved: Approve
//	Pending --> Rejected: Reject
//	Pending --> InProgress: StartDirect
//	Approved --> InProgress: Start
//	InProgress --> Completed: Complete
//	InProgress --> Failed: Fail
//	Completed --> Verifying: StartVerification
//	Verifying --> Verified: VerifyPass
//	Verifying --> VerificationFailed: VerifyFail
//	Rejected --> [*]
//	Failed --> [*]
//	Verified --> [*]
//	VerificationFailed --> [*]
//
// ```
type RollbackEvent string

const (
	// RollbackEventApprove approves the rollback
	RollbackEventApprove RollbackEvent = "approve"
	// RollbackEventReject rejects the rollback
	RollbackEventReject RollbackEvent = "reject"
	// RollbackEventStart starts the rollback (after approval)
	RollbackEventStart RollbackEvent = "start"
	// RollbackEventStartDirect starts rollback directly (no approval needed)
	RollbackEventStartDirect RollbackEvent = "start_direct"
	// RollbackEventComplete marks rollback as completed
	RollbackEventComplete RollbackEvent = "complete"
	// RollbackEventFail marks rollback as failed
	RollbackEventFail RollbackEvent = "fail"
	// RollbackEventStartVerification starts verification after completion
	RollbackEventStartVerification RollbackEvent = "start_verification"
	// RollbackEventVerifyPass marks verification as passed
	RollbackEventVerifyPass RollbackEvent = "verify_pass"
	// RollbackEventVerifyFail marks verification as failed
	RollbackEventVerifyFail RollbackEvent = "verify_fail"
)

// RollbackCallbacks holds callbacks for rollback state transitions.
type RollbackCallbacks struct {
	// OnApproved is called when rollback is approved
	OnApproved func(rollbackID, approvedBy string)
	// OnRejected is called when rollback is rejected
	OnRejected func(rollbackID, rejectedBy, reason string)
	// OnStarted is called when rollback execution starts
	OnStarted func(rollbackID string)
	// OnCompleted is called when rollback completes
	OnCompleted func(rollbackID string)
	// OnFailed is called when rollback fails
	OnFailed func(rollbackID string, err error)
	// OnVerificationStarted is called when verification begins
	OnVerificationStarted func(rollbackID string)
	// OnVerified is called when verification passes
	OnVerified func(rollbackID string)
	// OnVerificationFailed is called when verification fails
	OnVerificationFailed func(rollbackID string)
}

// ManagedRollback wraps a RollbackResult with a state machine.
type ManagedRollback struct {
	Result  *RollbackResult
	machine *statemachine.Machine[RollbackStatus, RollbackEvent]

	// Tracking
	rollbackID string
	callbacks  *RollbackCallbacks
	approvedBy string
	rejectedBy string
	reason     string
	execError  error
}

// NewManagedRollback creates a new managed rollback with state machine.
func NewManagedRollback(result *RollbackResult, callbacks *RollbackCallbacks) *ManagedRollback {
	mr := &ManagedRollback{
		Result:     result,
		rollbackID: result.ID,
		callbacks:  callbacks,
	}

	mr.machine = statemachine.New[RollbackStatus, RollbackEvent](StatusPending).
		WithName("rollback-"+result.ID).
		WithHistory(20).
		// From Pending
		AddTransition(StatusPending, RollbackEventApprove, StatusApproved).
		AddTransition(StatusPending, RollbackEventReject, StatusRejected).
		AddTransition(StatusPending, RollbackEventStartDirect, StatusInProgress).
		// From Approved
		AddTransition(StatusApproved, RollbackEventStart, StatusInProgress).
		// From InProgress
		AddTransition(StatusInProgress, RollbackEventComplete, StatusCompleted).
		AddTransition(StatusInProgress, RollbackEventFail, StatusFailed).
		// From Completed
		AddTransition(StatusCompleted, RollbackEventStartVerification, StatusVerifying).
		// From Verifying
		AddTransition(StatusVerifying, RollbackEventVerifyPass, StatusVerified).
		AddTransition(StatusVerifying, RollbackEventVerifyFail, StatusVerificationFailed).
		// Callbacks
		OnEnter(StatusApproved, func(ctx context.Context, state, from RollbackStatus) {
			mr.Result.Status = StatusApproved
			now := time.Now()
			if mr.Result.ApprovalInfo == nil {
				mr.Result.ApprovalInfo = &ApprovalInfo{}
			}
			mr.Result.ApprovalInfo.Status = StatusApproved
			mr.Result.ApprovalInfo.ApprovedBy = mr.approvedBy
			mr.Result.ApprovalInfo.ApprovedAt = now
			if mr.callbacks != nil && mr.callbacks.OnApproved != nil {
				mr.callbacks.OnApproved(mr.rollbackID, mr.approvedBy)
			}
		}).
		OnEnter(StatusRejected, func(ctx context.Context, state, from RollbackStatus) {
			mr.Result.Status = StatusRejected
			now := time.Now()
			mr.Result.EndTime = now
			if mr.Result.ApprovalInfo == nil {
				mr.Result.ApprovalInfo = &ApprovalInfo{}
			}
			mr.Result.ApprovalInfo.Status = StatusRejected
			mr.Result.ApprovalInfo.RejectedBy = mr.rejectedBy
			mr.Result.ApprovalInfo.RejectedAt = now
			mr.Result.ApprovalInfo.Reason = mr.reason
			mr.Result.Message = "Rollback rejected: " + mr.reason
			if mr.callbacks != nil && mr.callbacks.OnRejected != nil {
				mr.callbacks.OnRejected(mr.rollbackID, mr.rejectedBy, mr.reason)
			}
		}).
		OnEnter(StatusInProgress, func(ctx context.Context, state, from RollbackStatus) {
			mr.Result.Status = StatusInProgress
			mr.Result.StartTime = time.Now()
			if mr.callbacks != nil && mr.callbacks.OnStarted != nil {
				mr.callbacks.OnStarted(mr.rollbackID)
			}
		}).
		OnEnter(StatusCompleted, func(ctx context.Context, state, from RollbackStatus) {
			mr.Result.Status = StatusCompleted
			mr.Result.EndTime = time.Now()
			mr.Result.Duration = mr.Result.EndTime.Sub(mr.Result.StartTime)
			if mr.callbacks != nil && mr.callbacks.OnCompleted != nil {
				mr.callbacks.OnCompleted(mr.rollbackID)
			}
		}).
		OnEnter(StatusFailed, func(ctx context.Context, state, from RollbackStatus) {
			mr.Result.Status = StatusFailed
			mr.Result.EndTime = time.Now()
			mr.Result.Duration = mr.Result.EndTime.Sub(mr.Result.StartTime)
			mr.Result.Error = mr.execError
			if mr.callbacks != nil && mr.callbacks.OnFailed != nil {
				mr.callbacks.OnFailed(mr.rollbackID, mr.execError)
			}
		}).
		OnEnter(StatusVerifying, func(ctx context.Context, state, from RollbackStatus) {
			mr.Result.Status = StatusVerifying
			if mr.callbacks != nil && mr.callbacks.OnVerificationStarted != nil {
				mr.callbacks.OnVerificationStarted(mr.rollbackID)
			}
		}).
		OnEnter(StatusVerified, func(ctx context.Context, state, from RollbackStatus) {
			mr.Result.Status = StatusVerified
			if mr.callbacks != nil && mr.callbacks.OnVerified != nil {
				mr.callbacks.OnVerified(mr.rollbackID)
			}
		}).
		OnEnter(StatusVerificationFailed, func(ctx context.Context, state, from RollbackStatus) {
			mr.Result.Status = StatusVerificationFailed
			if mr.callbacks != nil && mr.callbacks.OnVerificationFailed != nil {
				mr.callbacks.OnVerificationFailed(mr.rollbackID)
			}
		}).
		MustBuild()

	return mr
}

// Status returns the current rollback status.
func (mr *ManagedRollback) Status() RollbackStatus {
	return mr.machine.State()
}

// Approve approves the rollback.
func (mr *ManagedRollback) Approve(approvedBy string) error {
	mr.approvedBy = approvedBy
	return mr.machine.Fire(RollbackEventApprove)
}

// Reject rejects the rollback.
func (mr *ManagedRollback) Reject(rejectedBy, reason string) error {
	mr.rejectedBy = rejectedBy
	mr.reason = reason
	return mr.machine.Fire(RollbackEventReject)
}

// Start starts the rollback execution (after approval).
func (mr *ManagedRollback) Start() error {
	return mr.machine.Fire(RollbackEventStart)
}

// StartDirect starts the rollback directly (no approval needed).
func (mr *ManagedRollback) StartDirect() error {
	return mr.machine.Fire(RollbackEventStartDirect)
}

// Complete marks the rollback as completed.
func (mr *ManagedRollback) Complete(prevRevision, currentRevision string) error {
	mr.Result.PreviousRevision = prevRevision
	mr.Result.CurrentRevision = currentRevision
	mr.Result.Message = "Rolled back from " + prevRevision + " to " + currentRevision
	return mr.machine.Fire(RollbackEventComplete)
}

// Fail marks the rollback as failed.
func (mr *ManagedRollback) Fail(err error) error {
	mr.execError = err
	mr.Result.Message = "Rollback failed: " + err.Error()
	return mr.machine.Fire(RollbackEventFail)
}

// StartVerification starts the verification process.
func (mr *ManagedRollback) StartVerification() error {
	return mr.machine.Fire(RollbackEventStartVerification)
}

// VerifyPass marks verification as passed.
func (mr *ManagedRollback) VerifyPass(result interface{}) error {
	mr.Result.VerificationResult = result
	return mr.machine.Fire(RollbackEventVerifyPass)
}

// VerifyFail marks verification as failed.
func (mr *ManagedRollback) VerifyFail(result interface{}) error {
	mr.Result.VerificationResult = result
	return mr.machine.Fire(RollbackEventVerifyFail)
}

// CanApprove returns true if the rollback can be approved.
func (mr *ManagedRollback) CanApprove() bool {
	return mr.machine.CanFire(RollbackEventApprove)
}

// CanReject returns true if the rollback can be rejected.
func (mr *ManagedRollback) CanReject() bool {
	return mr.machine.CanFire(RollbackEventReject)
}

// CanStart returns true if the rollback can be started.
func (mr *ManagedRollback) CanStart() bool {
	return mr.machine.CanFire(RollbackEventStart)
}

// CanStartDirect returns true if the rollback can start directly.
func (mr *ManagedRollback) CanStartDirect() bool {
	return mr.machine.CanFire(RollbackEventStartDirect)
}

// IsPending returns true if the rollback is pending approval.
func (mr *ManagedRollback) IsPending() bool {
	return mr.machine.IsInState(StatusPending)
}

// IsApproved returns true if the rollback has been approved.
func (mr *ManagedRollback) IsApproved() bool {
	return mr.machine.IsInState(StatusApproved)
}

// IsInProgress returns true if the rollback is in progress.
func (mr *ManagedRollback) IsInProgress() bool {
	return mr.machine.IsInState(StatusInProgress)
}

// IsCompleted returns true if the rollback completed successfully.
func (mr *ManagedRollback) IsCompleted() bool {
	return mr.machine.IsInState(StatusCompleted)
}

// IsVerifying returns true if verification is in progress.
func (mr *ManagedRollback) IsVerifying() bool {
	return mr.machine.IsInState(StatusVerifying)
}

// IsTerminal returns true if the rollback is in a terminal state.
func (mr *ManagedRollback) IsTerminal() bool {
	return mr.machine.IsInAnyState(
		StatusRejected,
		StatusFailed,
		StatusVerified,
		StatusVerificationFailed,
	)
}

// IsSuccessful returns true if the rollback completed successfully (with or without verification).
func (mr *ManagedRollback) IsSuccessful() bool {
	return mr.machine.IsInAnyState(StatusCompleted, StatusVerified)
}

// History returns the state transition history.
func (mr *ManagedRollback) History() *statemachine.History[RollbackStatus, RollbackEvent] {
	return mr.machine.History()
}

// AvailableEvents returns events that can be fired from the current state.
func (mr *ManagedRollback) AvailableEvents() []RollbackEvent {
	return mr.machine.AvailableEvents()
}

// RollbackStatusToString returns a human-readable name for the status.
func RollbackStatusToString(status RollbackStatus) string {
	switch status {
	case StatusPending:
		return "Pending"
	case StatusApproved:
		return "Approved"
	case StatusRejected:
		return "Rejected"
	case StatusInProgress:
		return "In Progress"
	case StatusCompleted:
		return "Completed"
	case StatusFailed:
		return "Failed"
	case StatusVerifying:
		return "Verifying"
	case StatusVerified:
		return "Verified"
	case StatusVerificationFailed:
		return "Verification Failed"
	default:
		return string(status)
	}
}
