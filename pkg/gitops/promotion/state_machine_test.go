package promotion

import (
	"errors"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestManagedPromotion_InitialState(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	if mp.Status() != StatusPending {
		t.Errorf("expected pending status, got %v", mp.Status())
	}
	if !mp.IsPending() {
		t.Error("expected IsPending() to be true")
	}
	if mp.IsActive() {
		t.Error("expected IsActive() to be false")
	}
	if mp.IsTerminal() {
		t.Error("expected IsTerminal() to be false")
	}
}

func TestManagedPromotion_BasicWorkflow(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	// Start promotion
	if err := mp.StartPromotion(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusInProgress {
		t.Errorf("expected in_progress status, got %v", mp.Status())
	}
	if !mp.IsActive() {
		t.Error("expected IsActive() to be true")
	}

	// Begin verification
	if err := mp.BeginVerification(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusVerifying {
		t.Errorf("expected verifying status, got %v", mp.Status())
	}

	// Verification passed -> rolling out
	if err := mp.VerificationPassed(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusRollingOut {
		t.Errorf("expected rolling_out status, got %v", mp.Status())
	}

	// Complete
	if err := mp.Complete("Promotion successful"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusCompleted {
		t.Errorf("expected completed status, got %v", mp.Status())
	}
	if !mp.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
}

func TestManagedPromotion_ApprovalWorkflow(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	// Require approval
	if err := mp.RequireApproval(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Result.ApprovalInfo == nil {
		t.Fatal("expected ApprovalInfo to be set")
	}
	if !mp.Result.ApprovalInfo.Required {
		t.Error("expected Required to be true")
	}

	// Approve
	if !mp.CanApprove() {
		t.Error("expected CanApprove() to be true")
	}
	if err := mp.Approve("admin@example.com", "Looks good"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusApproved {
		t.Errorf("expected approved status, got %v", mp.Status())
	}
	if mp.Result.ApprovalInfo.ApprovedBy != "admin@example.com" {
		t.Errorf("expected ApprovedBy to be admin@example.com, got %s", mp.Result.ApprovalInfo.ApprovedBy)
	}

	// Start promotion after approval
	if err := mp.StartPromotion(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusInProgress {
		t.Errorf("expected in_progress status, got %v", mp.Status())
	}
}

func TestManagedPromotion_Rejection(t *testing.T) {
	result := &PromotionResult{
		ID:           "test-promotion-1",
		Status:       StatusPending,
		ApprovalInfo: &ApprovalInfo{Required: true},
	}

	mp := NewManagedPromotion(result, nil)

	// Reject
	if !mp.CanReject() {
		t.Error("expected CanReject() to be true")
	}
	if err := mp.Reject("security@example.com", "Security review failed"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusRejected {
		t.Errorf("expected rejected status, got %v", mp.Status())
	}
	if !mp.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if mp.Result.ApprovalInfo.RejectedBy != "security@example.com" {
		t.Errorf("expected RejectedBy to be security@example.com, got %s", mp.Result.ApprovalInfo.RejectedBy)
	}
}

func TestManagedPromotion_VerificationFailure(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	mp.StartPromotion()
	mp.BeginVerification()

	// Verification fails
	err := errors.New("health check failed")
	if verErr := mp.VerificationFailed(err); verErr != nil {
		t.Errorf("unexpected error: %v", verErr)
	}
	if mp.Status() != StatusFailed {
		t.Errorf("expected failed status, got %v", mp.Status())
	}
	if mp.Result.Error == nil {
		t.Error("expected Result.Error to be set")
	}
}

func TestManagedPromotion_RollbackWorkflow(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	// Get to rolling out state
	mp.StartPromotion()
	mp.BeginRollout()

	if mp.Status() != StatusRollingOut {
		t.Errorf("expected rolling_out status, got %v", mp.Status())
	}

	// Initiate rollback
	if !mp.CanRollback() {
		t.Error("expected CanRollback() to be true")
	}
	if err := mp.InitiateRollback(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusRollingBack {
		t.Errorf("expected rolling_back status, got %v", mp.Status())
	}

	// Complete rollback
	if err := mp.RollbackComplete("Rolled back to v1.2.3"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusRolledBack {
		t.Errorf("expected rolled_back status, got %v", mp.Status())
	}
	if !mp.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
}

func TestManagedPromotion_RollbackFromFailed(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	// Get to failed state
	mp.StartPromotion()
	mp.Fail(errors.New("deployment failed"))

	if mp.Status() != StatusFailed {
		t.Errorf("expected failed status, got %v", mp.Status())
	}

	// Can rollback from failed
	if !mp.CanRollback() {
		t.Error("expected CanRollback() to be true")
	}
	if err := mp.InitiateRollback(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusRollingBack {
		t.Errorf("expected rolling_back status, got %v", mp.Status())
	}
}

func TestManagedPromotion_RollbackFailed(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	// Get to rolling back state
	mp.StartPromotion()
	mp.Fail(errors.New("deployment failed"))
	mp.InitiateRollback()

	// Rollback fails
	if err := mp.RollbackFailed(errors.New("rollback failed")); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusFailed {
		t.Errorf("expected failed status, got %v", mp.Status())
	}
}

func TestManagedPromotion_InvalidTransitions(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	// Cannot verify from pending
	err := mp.BeginVerification()
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// Status should not have changed
	if mp.Status() != StatusPending {
		t.Errorf("status should not have changed, got %v", mp.Status())
	}
}

func TestManagedPromotion_Callbacks(t *testing.T) {
	var startedCalls, completedCalls, failedCalls int
	var lastStartedID, lastCompletedID string
	var lastFailedError error

	callbacks := &PromotionStateMachineCallbacks{
		OnStarted: func(promotionID string) {
			startedCalls++
			lastStartedID = promotionID
		},
		OnCompleted: func(promotionID string) {
			completedCalls++
			lastCompletedID = promotionID
		},
		OnFailed: func(promotionID string, err error) {
			failedCalls++
			lastFailedError = err
		},
	}

	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, callbacks)

	// Start triggers callback
	mp.StartPromotion()
	if startedCalls != 1 || lastStartedID != "test-promotion-1" {
		t.Errorf("expected OnStarted called once, got %d", startedCalls)
	}

	// Complete triggers callback
	mp.Complete("Done")
	if completedCalls != 1 || lastCompletedID != "test-promotion-1" {
		t.Errorf("expected OnCompleted called once, got %d", completedCalls)
	}

	// Test failure callback
	result2 := &PromotionResult{
		ID:     "test-promotion-2",
		Status: StatusPending,
	}
	mp2 := NewManagedPromotion(result2, callbacks)
	mp2.StartPromotion()
	testErr := errors.New("test error")
	mp2.Fail(testErr)
	if failedCalls != 1 || lastFailedError != testErr {
		t.Errorf("expected OnFailed called once with error, got %d calls", failedCalls)
	}
}

func TestManagedPromotion_History(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	mp.StartPromotion()
	mp.BeginVerification()
	mp.VerificationPassed()
	mp.Complete("Done")

	history := mp.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 4 {
		t.Errorf("expected 4 history records, got %d", len(records))
	}
}

func TestManagedPromotion_AvailableEvents(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	// From pending, can approve, reject, or start
	events := mp.AvailableEvents()
	if len(events) < 2 {
		t.Errorf("expected at least 2 available events, got %d", len(events))
	}

	mp.StartPromotion()

	// From in_progress, can verify, rollout, complete, or fail
	events = mp.AvailableEvents()
	if len(events) < 3 {
		t.Errorf("expected at least 3 available events, got %d", len(events))
	}
}

func TestManagedPromotion_DirectComplete(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	// Start and complete directly (skip verification)
	mp.StartPromotion()
	if err := mp.Complete("Immediate deployment"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusCompleted {
		t.Errorf("expected completed status, got %v", mp.Status())
	}
}

func TestManagedPromotion_CanChecks(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	// From pending
	if !mp.CanStart() {
		t.Error("should be able to start from pending")
	}
	if !mp.CanApprove() {
		t.Error("should be able to approve from pending")
	}
	if mp.CanRollback() {
		t.Error("should not be able to rollback from pending")
	}

	// After starting
	mp.StartPromotion()
	if mp.CanStart() {
		t.Error("should not be able to start again")
	}
	if mp.CanApprove() {
		t.Error("should not be able to approve after started")
	}

	// Go to rolling out
	mp.BeginRollout()
	if !mp.CanRollback() {
		t.Error("should be able to rollback from rolling_out")
	}
}

func TestPromotionStatusToString(t *testing.T) {
	tests := []struct {
		status  PromotionStatus
		display string
	}{
		{StatusPending, "Pending"},
		{StatusApproved, "Approved"},
		{StatusRejected, "Rejected"},
		{StatusInProgress, "In Progress"},
		{StatusVerifying, "Verifying"},
		{StatusRollingOut, "Rolling Out"},
		{StatusCompleted, "Completed"},
		{StatusFailed, "Failed"},
		{StatusRollingBack, "Rolling Back"},
		{StatusRolledBack, "Rolled Back"},
		{PromotionStatus("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := PromotionStatusToString(tt.status); got != tt.display {
				t.Errorf("PromotionStatusToString(%v) = %v, want %v", tt.status, got, tt.display)
			}
		})
	}
}

func TestManagedPromotion_Duration(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	// Start promotion
	mp.StartPromotion()
	if result.StartTime.IsZero() {
		t.Error("expected StartTime to be set")
	}

	// Complete promotion
	mp.Complete("Done")
	if result.EndTime.IsZero() {
		t.Error("expected EndTime to be set")
	}
	if result.Duration == 0 {
		t.Error("expected Duration to be set")
	}
}

func TestManagedPromotion_NilCallbacks(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	// Empty callbacks struct
	callbacks := &PromotionStateMachineCallbacks{}

	mp := NewManagedPromotion(result, callbacks)

	// These should not panic
	mp.RequireApproval()
	mp.Approve("admin", "ok")
	mp.StartPromotion()
	mp.Complete("Done")
}

func TestManagedPromotion_VerifyCompleteFromVerifying(t *testing.T) {
	result := &PromotionResult{
		ID:     "test-promotion-1",
		Status: StatusPending,
	}

	mp := NewManagedPromotion(result, nil)

	mp.StartPromotion()
	mp.BeginVerification()

	// Can complete directly from verifying
	if err := mp.Complete("Verification passed, no rollout needed"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mp.Status() != StatusCompleted {
		t.Errorf("expected completed status, got %v", mp.Status())
	}
}
