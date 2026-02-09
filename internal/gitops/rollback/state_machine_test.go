package rollback

import (
	"errors"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestManagedRollback_InitialState(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, nil)

	if mr.Status() != StatusPending {
		t.Errorf("expected pending status, got %v", mr.Status())
	}
	if !mr.IsPending() {
		t.Error("expected IsPending() to be true")
	}
	if mr.IsInProgress() {
		t.Error("expected IsInProgress() to be false")
	}
	if mr.IsTerminal() {
		t.Error("expected IsTerminal() to be false")
	}
}

func TestManagedRollback_ApprovalWorkflow(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, nil)

	// Approve
	if !mr.CanApprove() {
		t.Error("expected CanApprove() to be true")
	}
	if err := mr.Approve("admin@example.com"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mr.Status() != StatusApproved {
		t.Errorf("expected approved status, got %v", mr.Status())
	}
	if !mr.IsApproved() {
		t.Error("expected IsApproved() to be true")
	}
	if result.ApprovalInfo.ApprovedBy != "admin@example.com" {
		t.Errorf("expected ApprovedBy to be admin@example.com, got %s", result.ApprovalInfo.ApprovedBy)
	}

	// Start after approval
	if !mr.CanStart() {
		t.Error("expected CanStart() to be true")
	}
	if err := mr.Start(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mr.Status() != StatusInProgress {
		t.Errorf("expected in_progress status, got %v", mr.Status())
	}
	if !mr.IsInProgress() {
		t.Error("expected IsInProgress() to be true")
	}
}

func TestManagedRollback_DirectStartWorkflow(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, nil)

	// Start directly without approval
	if !mr.CanStartDirect() {
		t.Error("expected CanStartDirect() to be true")
	}
	if err := mr.StartDirect(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mr.Status() != StatusInProgress {
		t.Errorf("expected in_progress status, got %v", mr.Status())
	}
}

func TestManagedRollback_RejectionWorkflow(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, nil)

	// Reject
	if !mr.CanReject() {
		t.Error("expected CanReject() to be true")
	}
	if err := mr.Reject("security@example.com", "Security concerns"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mr.Status() != StatusRejected {
		t.Errorf("expected rejected status, got %v", mr.Status())
	}
	if !mr.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if result.ApprovalInfo.RejectedBy != "security@example.com" {
		t.Errorf("expected RejectedBy to be security@example.com, got %s", result.ApprovalInfo.RejectedBy)
	}
	if result.ApprovalInfo.Reason != "Security concerns" {
		t.Errorf("expected Reason to be 'Security concerns', got %s", result.ApprovalInfo.Reason)
	}
}

func TestManagedRollback_CompleteWorkflow(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, nil)

	mr.StartDirect()

	// Complete
	if err := mr.Complete("rev-123", "rev-122"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mr.Status() != StatusCompleted {
		t.Errorf("expected completed status, got %v", mr.Status())
	}
	if !mr.IsCompleted() {
		t.Error("expected IsCompleted() to be true")
	}
	if !mr.IsSuccessful() {
		t.Error("expected IsSuccessful() to be true")
	}
	if result.PreviousRevision != "rev-123" {
		t.Errorf("expected PreviousRevision to be rev-123, got %s", result.PreviousRevision)
	}
	if result.CurrentRevision != "rev-122" {
		t.Errorf("expected CurrentRevision to be rev-122, got %s", result.CurrentRevision)
	}
	if result.Duration == 0 {
		t.Error("expected Duration to be set")
	}
}

func TestManagedRollback_FailWorkflow(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, nil)

	mr.StartDirect()

	// Fail
	testErr := errors.New("rollback execution failed")
	if err := mr.Fail(testErr); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mr.Status() != StatusFailed {
		t.Errorf("expected failed status, got %v", mr.Status())
	}
	if !mr.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if mr.IsSuccessful() {
		t.Error("expected IsSuccessful() to be false")
	}
	if result.Error.Error() != "rollback execution failed" {
		t.Errorf("expected Error message, got %v", result.Error)
	}
}

func TestManagedRollback_VerificationWorkflow(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, nil)

	mr.StartDirect()
	mr.Complete("rev-123", "rev-122")

	// Start verification
	if err := mr.StartVerification(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mr.Status() != StatusVerifying {
		t.Errorf("expected verifying status, got %v", mr.Status())
	}
	if !mr.IsVerifying() {
		t.Error("expected IsVerifying() to be true")
	}

	// Pass verification
	verifyResult := map[string]string{"status": "healthy"}
	if err := mr.VerifyPass(verifyResult); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mr.Status() != StatusVerified {
		t.Errorf("expected verified status, got %v", mr.Status())
	}
	if !mr.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if !mr.IsSuccessful() {
		t.Error("expected IsSuccessful() to be true")
	}
}

func TestManagedRollback_VerificationFailedWorkflow(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, nil)

	mr.StartDirect()
	mr.Complete("rev-123", "rev-122")
	mr.StartVerification()

	// Fail verification
	verifyResult := map[string]string{"status": "unhealthy"}
	if err := mr.VerifyFail(verifyResult); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mr.Status() != StatusVerificationFailed {
		t.Errorf("expected verification_failed status, got %v", mr.Status())
	}
	if !mr.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if mr.IsSuccessful() {
		t.Error("expected IsSuccessful() to be false")
	}
}

func TestManagedRollback_InvalidTransitions(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, nil)

	// Cannot complete from pending
	err := mr.Complete("rev-123", "rev-122")
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// Status should not have changed
	if mr.Status() != StatusPending {
		t.Errorf("status should not have changed, got %v", mr.Status())
	}
}

func TestManagedRollback_Callbacks(t *testing.T) {
	var approvedCalls, rejectedCalls, startedCalls, completedCalls, failedCalls int
	var lastApprovedBy, lastRejectedBy string

	callbacks := &Callbacks{
		OnApproved: func(rollbackID, approvedBy string) {
			approvedCalls++
			lastApprovedBy = approvedBy
		},
		OnRejected: func(rollbackID, rejectedBy, reason string) {
			rejectedCalls++
			lastRejectedBy = rejectedBy
		},
		OnStarted: func(rollbackID string) {
			startedCalls++
		},
		OnCompleted: func(rollbackID string) {
			completedCalls++
		},
		OnFailed: func(rollbackID string, err error) {
			failedCalls++
		},
	}

	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, callbacks)

	// Approve triggers callback
	mr.Approve("admin")
	if approvedCalls != 1 || lastApprovedBy != "admin" {
		t.Errorf("expected OnApproved called once, got %d", approvedCalls)
	}

	// Start triggers callback
	mr.Start()
	if startedCalls != 1 {
		t.Errorf("expected OnStarted called once, got %d", startedCalls)
	}

	// Complete triggers callback
	mr.Complete("rev-123", "rev-122")
	if completedCalls != 1 {
		t.Errorf("expected OnCompleted called once, got %d", completedCalls)
	}

	// Test rejection callback
	result2 := &Result{ID: "test-rollback-2"}
	mr2 := NewManagedRollback(result2, callbacks)
	mr2.Reject("security", "reason")
	if rejectedCalls != 1 || lastRejectedBy != "security" {
		t.Errorf("expected OnRejected called once, got %d", rejectedCalls)
	}
}

func TestManagedRollback_History(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, nil)

	mr.Approve("admin")
	mr.Start()
	mr.Complete("rev-123", "rev-122")
	mr.StartVerification()
	mr.VerifyPass(nil)

	history := mr.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 5 {
		t.Errorf("expected 5 history records, got %d", len(records))
	}
}

func TestManagedRollback_AvailableEvents(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, nil)

	// From pending, can approve, reject, or start directly
	events := mr.AvailableEvents()
	if len(events) != 3 {
		t.Errorf("expected 3 available events from pending, got %d", len(events))
	}

	mr.StartDirect()

	// From in_progress, can complete or fail
	events = mr.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 available events from in_progress, got %d", len(events))
	}

	mr.Complete("rev-123", "rev-122")

	// From completed, can start verification
	events = mr.AvailableEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 available event from completed, got %d", len(events))
	}
}

func TestManagedRollback_NilCallbacks(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	// Empty callbacks struct
	callbacks := &Callbacks{}

	mr := NewManagedRollback(result, callbacks)

	// These should not panic
	mr.Approve("admin")
	mr.Start()
	mr.Complete("rev-123", "rev-122")
	mr.StartVerification()
	mr.VerifyPass(nil)
}

func TestStatusToString(t *testing.T) {
	tests := []struct {
		status  Status
		display string
	}{
		{StatusPending, "Pending"},
		{StatusApproved, "Approved"},
		{StatusRejected, "Rejected"},
		{StatusInProgress, "In Progress"},
		{StatusCompleted, "Completed"},
		{StatusFailed, "Failed"},
		{StatusVerifying, "Verifying"},
		{StatusVerified, "Verified"},
		{StatusVerificationFailed, "Verification Failed"},
		{Status("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := StatusToString(tt.status); got != tt.display {
				t.Errorf("StatusToString(%v) = %v, want %v", tt.status, got, tt.display)
			}
		})
	}
}

func TestManagedRollback_FullWorkflowWithApproval(t *testing.T) {
	result := &Result{
		ID: "test-rollback-1",
	}

	mr := NewManagedRollback(result, nil)

	// Full workflow: pending -> approved -> in_progress -> completed -> verifying -> verified
	mr.Approve("admin")
	if !mr.IsApproved() {
		t.Error("expected approved")
	}

	mr.Start()
	if !mr.IsInProgress() {
		t.Error("expected in_progress")
	}

	mr.Complete("rev-123", "rev-122")
	if !mr.IsCompleted() {
		t.Error("expected completed")
	}

	mr.StartVerification()
	if !mr.IsVerifying() {
		t.Error("expected verifying")
	}

	mr.VerifyPass(nil)
	if mr.Status() != StatusVerified {
		t.Errorf("expected verified, got %v", mr.Status())
	}
	if !mr.IsTerminal() {
		t.Error("expected terminal state")
	}
}

func TestManagedRollback_StatusSync(t *testing.T) {
	result := &Result{
		ID:     "test-rollback-1",
		Status: StatusPending,
	}

	mr := NewManagedRollback(result, nil)

	// Verify result.Status is synced with state machine
	if result.Status != StatusPending {
		t.Errorf("expected result.Status to be pending, got %v", result.Status)
	}

	mr.StartDirect()
	if result.Status != StatusInProgress {
		t.Errorf("expected result.Status to be in_progress, got %v", result.Status)
	}

	mr.Complete("rev-123", "rev-122")
	if result.Status != StatusCompleted {
		t.Errorf("expected result.Status to be completed, got %v", result.Status)
	}
}
