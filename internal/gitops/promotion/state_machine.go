package promotion

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// PromotionEvent represents events that trigger promotion state transitions.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//
//	[*] --> Pending
//	Pending --> WaitingApproval: RequireApproval
//	Pending --> InProgress: StartPromotion
//	WaitingApproval --> Approved: Approve
//	WaitingApproval --> Rejected: Reject
//	Approved --> InProgress: StartPromotion
//	InProgress --> Verifying: BeginVerification
//	InProgress --> RollingOut: BeginRollout
//	InProgress --> Completed: Complete
//	InProgress --> Failed: Fail
//	Verifying --> RollingOut: VerificationPassed
//	Verifying --> Failed: VerificationFailed
//	RollingOut --> Completed: Complete
//	RollingOut --> Failed: Fail
//	RollingOut --> RollingBack: InitiateRollback
//	Failed --> RollingBack: InitiateRollback
//	RollingBack --> RolledBack: RollbackComplete
//	RollingBack --> Failed: RollbackFailed
//	Completed --> [*]
//	RolledBack --> [*]
//	Rejected --> [*]
//
// ```
type PromotionEvent string

const (
	// EventRequireApproval transitions to waiting for approval
	EventRequireApproval PromotionEvent = "require_approval"
	// EventApprove approves a pending promotion
	EventApprove PromotionEvent = "approve"
	// EventReject rejects a pending promotion
	EventReject PromotionEvent = "reject"
	// EventStartPromotion starts the promotion process
	EventStartPromotion PromotionEvent = "start_promotion"
	// EventBeginVerification begins the verification phase
	EventBeginVerification PromotionEvent = "begin_verification"
	// EventVerificationPassed indicates verification succeeded
	EventVerificationPassed PromotionEvent = "verification_passed"
	// EventVerificationFailed indicates verification failed
	EventVerificationFailed PromotionEvent = "verification_failed"
	// EventBeginRollout begins the rollout phase
	EventBeginRollout PromotionEvent = "begin_rollout"
	// EventComplete marks the promotion as completed
	EventComplete PromotionEvent = "complete"
	// EventFail marks the promotion as failed
	EventFail PromotionEvent = "fail"
	// EventInitiateRollback starts the rollback process
	EventInitiateRollback PromotionEvent = "initiate_rollback"
	// EventRollbackComplete marks rollback as complete
	EventRollbackComplete PromotionEvent = "rollback_complete"
	// EventRollbackFailed marks rollback as failed
	EventRollbackFailed PromotionEvent = "rollback_failed"
)

// PromotionStateMachineCallbacks holds callbacks for promotion state transitions.
type PromotionStateMachineCallbacks struct {
	// OnApprovalRequired is called when promotion requires approval
	OnApprovalRequired func(promotionID string)
	// OnApproved is called when promotion is approved
	OnApproved func(promotionID string, approvedBy string)
	// OnRejected is called when promotion is rejected
	OnRejected func(promotionID string, rejectedBy string, reason string)
	// OnStarted is called when promotion starts
	OnStarted func(promotionID string)
	// OnVerifying is called when verification begins
	OnVerifying func(promotionID string)
	// OnRollingOut is called when rollout begins
	OnRollingOut func(promotionID string)
	// OnCompleted is called when promotion completes
	OnCompleted func(promotionID string)
	// OnFailed is called when promotion fails
	OnFailed func(promotionID string, err error)
	// OnRollbackStarted is called when rollback starts
	OnRollbackStarted func(promotionID string)
	// OnRolledBack is called when rollback completes
	OnRolledBack func(promotionID string)
}

// ManagedPromotion wraps PromotionResult with a state machine.
type ManagedPromotion struct {
	Result  *PromotionResult
	machine *statemachine.Machine[PromotionStatus, PromotionEvent]

	// Tracking
	promotionID string
	callbacks   *PromotionStateMachineCallbacks

	// Approval info
	approvedBy string
	rejectedBy string
	reason     string
	lastError  error
}

// NewManagedPromotion creates a new managed promotion with state machine.
func NewManagedPromotion(result *PromotionResult, callbacks *PromotionStateMachineCallbacks) *ManagedPromotion {
	mp := &ManagedPromotion{
		Result:      result,
		promotionID: result.ID,
		callbacks:   callbacks,
	}

	mp.machine = statemachine.New[PromotionStatus, PromotionEvent](StatusPending).
		WithName("promotion-"+result.ID).
		WithHistory(30).
		// From Pending
		AddTransition(StatusPending, EventRequireApproval, StatusPending). // Stay pending but mark as waiting
		AddTransition(StatusPending, EventStartPromotion, StatusInProgress).
		// From Pending (when approval required - use a waiting state internally)
		AddTransition(StatusPending, EventApprove, StatusApproved).
		AddTransition(StatusPending, EventReject, StatusRejected).
		// From Approved
		AddTransition(StatusApproved, EventStartPromotion, StatusInProgress).
		// From InProgress
		AddTransition(StatusInProgress, EventBeginVerification, StatusVerifying).
		AddTransition(StatusInProgress, EventBeginRollout, StatusRollingOut).
		AddTransition(StatusInProgress, EventComplete, StatusCompleted).
		AddTransition(StatusInProgress, EventFail, StatusFailed).
		// From Verifying
		AddTransition(StatusVerifying, EventVerificationPassed, StatusRollingOut).
		AddTransition(StatusVerifying, EventVerificationFailed, StatusFailed).
		AddTransition(StatusVerifying, EventComplete, StatusCompleted).
		// From RollingOut
		AddTransition(StatusRollingOut, EventComplete, StatusCompleted).
		AddTransition(StatusRollingOut, EventFail, StatusFailed).
		AddTransition(StatusRollingOut, EventInitiateRollback, StatusRollingBack).
		// From Failed
		AddTransition(StatusFailed, EventInitiateRollback, StatusRollingBack).
		// From RollingBack
		AddTransition(StatusRollingBack, EventRollbackComplete, StatusRolledBack).
		AddTransition(StatusRollingBack, EventRollbackFailed, StatusFailed).
		// Callbacks
		OnEnter(StatusInProgress, func(ctx context.Context, state, from PromotionStatus) {
			mp.Result.Status = StatusInProgress
			if mp.Result.StartTime.IsZero() {
				mp.Result.StartTime = time.Now()
			}
			if mp.callbacks != nil && mp.callbacks.OnStarted != nil {
				mp.callbacks.OnStarted(mp.promotionID)
			}
		}).
		OnEnter(StatusApproved, func(ctx context.Context, state, from PromotionStatus) {
			mp.Result.Status = StatusApproved
			if mp.Result.ApprovalInfo != nil {
				mp.Result.ApprovalInfo.Status = StatusApproved
				mp.Result.ApprovalInfo.ApprovedBy = mp.approvedBy
				mp.Result.ApprovalInfo.ApprovedAt = time.Now()
			}
			if mp.callbacks != nil && mp.callbacks.OnApproved != nil {
				mp.callbacks.OnApproved(mp.promotionID, mp.approvedBy)
			}
		}).
		OnEnter(StatusRejected, func(ctx context.Context, state, from PromotionStatus) {
			mp.Result.Status = StatusRejected
			mp.Result.EndTime = time.Now()
			mp.Result.Duration = mp.Result.EndTime.Sub(mp.Result.StartTime)
			if mp.Result.ApprovalInfo != nil {
				mp.Result.ApprovalInfo.Status = StatusRejected
				mp.Result.ApprovalInfo.RejectedBy = mp.rejectedBy
				mp.Result.ApprovalInfo.RejectedAt = time.Now()
				mp.Result.ApprovalInfo.Reason = mp.reason
			}
			mp.Result.Message = fmt.Sprintf("Promotion rejected by %s: %s", mp.rejectedBy, mp.reason)
			if mp.callbacks != nil && mp.callbacks.OnRejected != nil {
				mp.callbacks.OnRejected(mp.promotionID, mp.rejectedBy, mp.reason)
			}
		}).
		OnEnter(StatusVerifying, func(ctx context.Context, state, from PromotionStatus) {
			mp.Result.Status = StatusVerifying
			if mp.callbacks != nil && mp.callbacks.OnVerifying != nil {
				mp.callbacks.OnVerifying(mp.promotionID)
			}
		}).
		OnEnter(StatusRollingOut, func(ctx context.Context, state, from PromotionStatus) {
			mp.Result.Status = StatusRollingOut
			if mp.callbacks != nil && mp.callbacks.OnRollingOut != nil {
				mp.callbacks.OnRollingOut(mp.promotionID)
			}
		}).
		OnEnter(StatusCompleted, func(ctx context.Context, state, from PromotionStatus) {
			mp.Result.Status = StatusCompleted
			mp.Result.EndTime = time.Now()
			if !mp.Result.StartTime.IsZero() {
				mp.Result.Duration = mp.Result.EndTime.Sub(mp.Result.StartTime)
			}
			if mp.callbacks != nil && mp.callbacks.OnCompleted != nil {
				mp.callbacks.OnCompleted(mp.promotionID)
			}
		}).
		OnEnter(StatusFailed, func(ctx context.Context, state, from PromotionStatus) {
			mp.Result.Status = StatusFailed
			mp.Result.EndTime = time.Now()
			if !mp.Result.StartTime.IsZero() {
				mp.Result.Duration = mp.Result.EndTime.Sub(mp.Result.StartTime)
			}
			mp.Result.Error = mp.lastError
			if mp.callbacks != nil && mp.callbacks.OnFailed != nil {
				mp.callbacks.OnFailed(mp.promotionID, mp.lastError)
			}
		}).
		OnEnter(StatusRollingBack, func(ctx context.Context, state, from PromotionStatus) {
			mp.Result.Status = StatusRollingBack
			if mp.callbacks != nil && mp.callbacks.OnRollbackStarted != nil {
				mp.callbacks.OnRollbackStarted(mp.promotionID)
			}
		}).
		OnEnter(StatusRolledBack, func(ctx context.Context, state, from PromotionStatus) {
			mp.Result.Status = StatusRolledBack
			mp.Result.EndTime = time.Now()
			if !mp.Result.StartTime.IsZero() {
				mp.Result.Duration = mp.Result.EndTime.Sub(mp.Result.StartTime)
			}
			if mp.callbacks != nil && mp.callbacks.OnRolledBack != nil {
				mp.callbacks.OnRolledBack(mp.promotionID)
			}
		}).
		MustBuild()

	return mp
}

// Status returns the current promotion status.
func (mp *ManagedPromotion) Status() PromotionStatus {
	return mp.machine.State()
}

// RequireApproval marks the promotion as requiring approval.
func (mp *ManagedPromotion) RequireApproval() error {
	if mp.Result.ApprovalInfo == nil {
		mp.Result.ApprovalInfo = &ApprovalInfo{
			Required: true,
			Status:   StatusPending,
		}
	}
	if mp.callbacks != nil && mp.callbacks.OnApprovalRequired != nil {
		mp.callbacks.OnApprovalRequired(mp.promotionID)
	}
	return nil // Stay in pending state
}

// Approve approves the promotion.
func (mp *ManagedPromotion) Approve(approvedBy, reason string) error {
	mp.approvedBy = approvedBy
	mp.reason = reason
	return mp.machine.Fire(EventApprove)
}

// Reject rejects the promotion.
func (mp *ManagedPromotion) Reject(rejectedBy, reason string) error {
	mp.rejectedBy = rejectedBy
	mp.reason = reason
	return mp.machine.Fire(EventReject)
}

// StartPromotion begins the promotion process.
func (mp *ManagedPromotion) StartPromotion() error {
	return mp.machine.Fire(EventStartPromotion)
}

// BeginVerification transitions to verification phase.
func (mp *ManagedPromotion) BeginVerification() error {
	return mp.machine.Fire(EventBeginVerification)
}

// VerificationPassed marks verification as successful.
func (mp *ManagedPromotion) VerificationPassed() error {
	return mp.machine.Fire(EventVerificationPassed)
}

// VerificationFailed marks verification as failed.
func (mp *ManagedPromotion) VerificationFailed(err error) error {
	mp.lastError = err
	return mp.machine.Fire(EventVerificationFailed)
}

// BeginRollout transitions to rollout phase.
func (mp *ManagedPromotion) BeginRollout() error {
	return mp.machine.Fire(EventBeginRollout)
}

// Complete marks the promotion as completed.
func (mp *ManagedPromotion) Complete(message string) error {
	mp.Result.Message = message
	return mp.machine.Fire(EventComplete)
}

// Fail marks the promotion as failed.
func (mp *ManagedPromotion) Fail(err error) error {
	mp.lastError = err
	if err != nil {
		mp.Result.Message = fmt.Sprintf("Promotion failed: %v", err)
	}
	return mp.machine.Fire(EventFail)
}

// InitiateRollback starts the rollback process.
func (mp *ManagedPromotion) InitiateRollback() error {
	return mp.machine.Fire(EventInitiateRollback)
}

// RollbackComplete marks rollback as complete.
func (mp *ManagedPromotion) RollbackComplete(message string) error {
	mp.Result.Message = message
	return mp.machine.Fire(EventRollbackComplete)
}

// RollbackFailed marks rollback as failed.
func (mp *ManagedPromotion) RollbackFailed(err error) error {
	mp.lastError = err
	return mp.machine.Fire(EventRollbackFailed)
}

// CanApprove returns true if the promotion can be approved.
func (mp *ManagedPromotion) CanApprove() bool {
	return mp.machine.CanFire(EventApprove)
}

// CanReject returns true if the promotion can be rejected.
func (mp *ManagedPromotion) CanReject() bool {
	return mp.machine.CanFire(EventReject)
}

// CanStart returns true if the promotion can be started.
func (mp *ManagedPromotion) CanStart() bool {
	return mp.machine.CanFire(EventStartPromotion)
}

// CanRollback returns true if rollback can be initiated.
func (mp *ManagedPromotion) CanRollback() bool {
	return mp.machine.CanFire(EventInitiateRollback)
}

// IsTerminal returns true if the promotion is in a terminal state.
func (mp *ManagedPromotion) IsTerminal() bool {
	return mp.machine.IsInAnyState(StatusCompleted, StatusFailed, StatusRolledBack, StatusRejected)
}

// IsActive returns true if the promotion is actively processing.
func (mp *ManagedPromotion) IsActive() bool {
	return mp.machine.IsInAnyState(StatusInProgress, StatusVerifying, StatusRollingOut, StatusRollingBack)
}

// IsPending returns true if the promotion is pending.
func (mp *ManagedPromotion) IsPending() bool {
	return mp.machine.IsInState(StatusPending)
}

// IsApproved returns true if the promotion is approved.
func (mp *ManagedPromotion) IsApproved() bool {
	return mp.machine.IsInState(StatusApproved)
}

// History returns the state transition history.
func (mp *ManagedPromotion) History() *statemachine.History[PromotionStatus, PromotionEvent] {
	return mp.machine.History()
}

// AvailableEvents returns events that can be fired from the current state.
func (mp *ManagedPromotion) AvailableEvents() []PromotionEvent {
	return mp.machine.AvailableEvents()
}

// PromotionStatusToString returns a human-readable name for the status.
func PromotionStatusToString(status PromotionStatus) string {
	switch status {
	case StatusPending:
		return "Pending"
	case StatusApproved:
		return "Approved"
	case StatusRejected:
		return "Rejected"
	case StatusInProgress:
		return "In Progress"
	case StatusVerifying:
		return "Verifying"
	case StatusRollingOut:
		return "Rolling Out"
	case StatusCompleted:
		return "Completed"
	case StatusFailed:
		return "Failed"
	case StatusRollingBack:
		return "Rolling Back"
	case StatusRolledBack:
		return "Rolled Back"
	default:
		return string(status)
	}
}
