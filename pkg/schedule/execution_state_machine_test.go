package schedule

import (
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestManagedExecution_InitialState(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, nil)

	if me.Status() != ExecutionStatusPending {
		t.Errorf("expected pending status, got %v", me.Status())
	}
	if !me.IsPending() {
		t.Error("expected IsPending() to be true")
	}
	if me.IsRunning() {
		t.Error("expected IsRunning() to be false")
	}
	if me.IsTerminal() {
		t.Error("expected IsTerminal() to be false")
	}
}

func TestManagedExecution_ApprovalWorkflow(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, nil)

	// Approve
	if !me.CanApprove() {
		t.Error("expected CanApprove() to be true")
	}
	if err := me.Approve("admin@example.com"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.Status() != ExecutionStatusApproved {
		t.Errorf("expected approved status, got %v", me.Status())
	}
	if exec.ApprovedBy != "admin@example.com" {
		t.Errorf("expected ApprovedBy to be admin@example.com, got %s", exec.ApprovedBy)
	}
	if exec.ApprovedAt == nil {
		t.Error("expected ApprovedAt to be set")
	}

	// Start after approval
	if err := me.Start(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.Status() != ExecutionStatusRunning {
		t.Errorf("expected running status, got %v", me.Status())
	}
	if !me.IsRunning() {
		t.Error("expected IsRunning() to be true")
	}
}

func TestManagedExecution_NoApprovalWorkflow(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, nil)

	// Start without approval
	if err := me.StartNoApproval(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.Status() != ExecutionStatusRunning {
		t.Errorf("expected running status, got %v", me.Status())
	}
}

func TestManagedExecution_RejectionWorkflow(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, nil)

	// Reject
	if !me.CanReject() {
		t.Error("expected CanReject() to be true")
	}
	if err := me.Reject("security@example.com", "Security review failed"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.Status() != ExecutionStatusRejected {
		t.Errorf("expected rejected status, got %v", me.Status())
	}
	if !me.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if exec.RejectedBy != "security@example.com" {
		t.Errorf("expected RejectedBy to be security@example.com, got %s", exec.RejectedBy)
	}
	if exec.RejectionReason != "Security review failed" {
		t.Errorf("expected RejectionReason to be 'Security review failed', got %s", exec.RejectionReason)
	}
}

func TestManagedExecution_CompleteWorkflow(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, nil)

	me.StartNoApproval()

	// Complete
	if err := me.Complete(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.Status() != ExecutionStatusCompleted {
		t.Errorf("expected completed status, got %v", me.Status())
	}
	if !me.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if !me.IsSuccessful() {
		t.Error("expected IsSuccessful() to be true")
	}
	if exec.EndTime == nil {
		t.Error("expected EndTime to be set")
	}
}

func TestManagedExecution_FailWorkflow(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, nil)

	me.StartNoApproval()

	// Fail
	testErr := errors.New("command failed")
	if err := me.Fail(testErr); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.Status() != ExecutionStatusFailed {
		t.Errorf("expected failed status, got %v", me.Status())
	}
	if !me.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if me.IsSuccessful() {
		t.Error("expected IsSuccessful() to be false")
	}
	if exec.Error != "command failed" {
		t.Errorf("expected Error to be 'command failed', got %s", exec.Error)
	}
}

func TestManagedExecution_TimeoutWorkflow(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, nil)

	me.StartNoApproval()

	// Timeout
	if err := me.Timeout(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.Status() != ExecutionStatusTimeout {
		t.Errorf("expected timeout status, got %v", me.Status())
	}
	if !me.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if exec.Error != "execution timed out" {
		t.Errorf("expected Error to be 'execution timed out', got %s", exec.Error)
	}
}

func TestManagedExecution_CancelWorkflow(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*ManagedExecution)
	}{
		{"cancel from pending", func(me *ManagedExecution) {}},
		{"cancel from approved", func(me *ManagedExecution) { me.Approve("admin") }},
		{"cancel from running", func(me *ManagedExecution) { me.StartNoApproval() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &ScheduleExecution{
				ID:         "test-exec-1",
				ScheduleID: "test-schedule-1",
				Status:     ExecutionStatusPending,
			}

			me := NewManagedExecution(exec, nil)
			tt.setup(me)

			if !me.CanCancel() {
				t.Error("expected CanCancel() to be true")
			}
			if err := me.Cancel(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if me.Status() != ExecutionStatusCancelled {
				t.Errorf("expected cancelled status, got %v", me.Status())
			}
			if !me.IsTerminal() {
				t.Error("expected IsTerminal() to be true")
			}
		})
	}
}

func TestManagedExecution_SkipWorkflow(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, nil)

	// Skip
	if err := me.Skip("Outside maintenance window"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if me.Status() != ExecutionStatusSkipped {
		t.Errorf("expected skipped status, got %v", me.Status())
	}
	if !me.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
}

func TestManagedExecution_InvalidTransitions(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, nil)

	// Cannot complete from pending
	err := me.Complete()
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// Status should not have changed
	if me.Status() != ExecutionStatusPending {
		t.Errorf("status should not have changed, got %v", me.Status())
	}
}

func TestManagedExecution_Callbacks(t *testing.T) {
	var startedCalls, completedCalls, failedCalls int
	var lastStartedID, lastCompletedID string

	callbacks := &ExecutionCallbacks{
		OnStarted: func(execID string) {
			startedCalls++
			lastStartedID = execID
		},
		OnCompleted: func(execID string, successCount, failureCount int) {
			completedCalls++
			lastCompletedID = execID
		},
		OnFailed: func(execID string, err error) {
			failedCalls++
		},
	}

	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, callbacks)

	// Start triggers callback
	me.StartNoApproval()
	if startedCalls != 1 || lastStartedID != "test-exec-1" {
		t.Errorf("expected OnStarted called once, got %d", startedCalls)
	}

	// Complete triggers callback
	me.Complete()
	if completedCalls != 1 || lastCompletedID != "test-exec-1" {
		t.Errorf("expected OnCompleted called once, got %d", completedCalls)
	}
}

func TestManagedExecution_History(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, nil)

	me.Approve("admin")
	me.Start()
	me.Complete()

	history := me.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 3 {
		t.Errorf("expected 3 history records, got %d", len(records))
	}
}

func TestManagedExecution_AvailableEvents(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, nil)

	// From pending, can approve, reject, start, cancel, or skip
	events := me.AvailableEvents()
	if len(events) < 4 {
		t.Errorf("expected at least 4 available events from pending, got %d", len(events))
	}

	me.StartNoApproval()

	// From running, can complete, fail, timeout, or cancel
	events = me.AvailableEvents()
	if len(events) != 4 {
		t.Errorf("expected 4 available events from running, got %d", len(events))
	}
}

func TestManagedExecution_Duration(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	me := NewManagedExecution(exec, nil)

	me.StartNoApproval()
	time.Sleep(1 * time.Millisecond)
	me.Complete()

	if exec.Duration == 0 {
		t.Error("expected Duration to be set after completion")
	}
}

func TestManagedExecution_NilCallbacks(t *testing.T) {
	exec := &ScheduleExecution{
		ID:         "test-exec-1",
		ScheduleID: "test-schedule-1",
		Status:     ExecutionStatusPending,
	}

	// Empty callbacks struct
	callbacks := &ExecutionCallbacks{}

	me := NewManagedExecution(exec, callbacks)

	// These should not panic
	me.Approve("admin")
	me.Start()
	me.Complete()
}

func TestExecutionStatusToString(t *testing.T) {
	tests := []struct {
		status  ExecutionStatus
		display string
	}{
		{ExecutionStatusPending, "Pending"},
		{ExecutionStatusApproved, "Approved"},
		{ExecutionStatusRunning, "Running"},
		{ExecutionStatusCompleted, "Completed"},
		{ExecutionStatusFailed, "Failed"},
		{ExecutionStatusCancelled, "Cancelled"},
		{ExecutionStatusSkipped, "Skipped"},
		{ExecutionStatusTimeout, "Timeout"},
		{ExecutionStatusRejected, "Rejected"},
		{ExecutionStatus("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := ExecutionStatusToString(tt.status); got != tt.display {
				t.Errorf("ExecutionStatusToString(%v) = %v, want %v", tt.status, got, tt.display)
			}
		})
	}
}
