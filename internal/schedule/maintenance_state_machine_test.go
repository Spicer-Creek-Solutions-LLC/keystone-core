package schedule

import (
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestManagedMaintenanceWindow_InitialState(t *testing.T) {
	window := &MaintenanceWindow{
		ID:        "test-window-1",
		Name:      "Test Maintenance",
		Status:    MaintenanceWindowStatusScheduled,
		StartTime: time.Now().Add(1 * time.Hour),
		EndTime:   time.Now().Add(3 * time.Hour),
	}

	mmw := NewManagedMaintenanceWindow(window, nil)

	if mmw.Status() != MaintenanceWindowStatusScheduled {
		t.Errorf("expected scheduled status, got %v", mmw.Status())
	}
	if !mmw.IsScheduled() {
		t.Error("expected IsScheduled() to be true")
	}
	if mmw.IsActive() {
		t.Error("expected IsActive() to be false")
	}
	if mmw.IsTerminal() {
		t.Error("expected IsTerminal() to be false")
	}
}

func TestManagedMaintenanceWindow_ActivateWorkflow(t *testing.T) {
	window := &MaintenanceWindow{
		ID:        "test-window-1",
		Name:      "Test Maintenance",
		Status:    MaintenanceWindowStatusScheduled,
		StartTime: time.Now().Add(1 * time.Hour),
		EndTime:   time.Now().Add(3 * time.Hour),
	}

	mmw := NewManagedMaintenanceWindow(window, nil)

	// Activate
	if !mmw.CanActivate() {
		t.Error("expected CanActivate() to be true")
	}
	if err := mmw.Activate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mmw.Status() != MaintenanceWindowStatusActive {
		t.Errorf("expected active status, got %v", mmw.Status())
	}
	if !mmw.IsActive() {
		t.Error("expected IsActive() to be true")
	}
	if window.ActualStartTime == nil {
		t.Error("expected ActualStartTime to be set")
	}
}

func TestManagedMaintenanceWindow_CompleteWorkflow(t *testing.T) {
	window := &MaintenanceWindow{
		ID:        "test-window-1",
		Name:      "Test Maintenance",
		Status:    MaintenanceWindowStatusScheduled,
		StartTime: time.Now().Add(1 * time.Hour),
		EndTime:   time.Now().Add(3 * time.Hour),
	}

	mmw := NewManagedMaintenanceWindow(window, nil)

	mmw.Activate()

	// Complete
	if err := mmw.Complete(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mmw.Status() != MaintenanceWindowStatusCompleted {
		t.Errorf("expected completed status, got %v", mmw.Status())
	}
	if !mmw.IsTerminal() {
		t.Error("expected IsTerminal() to be true")
	}
	if window.ActualEndTime == nil {
		t.Error("expected ActualEndTime to be set")
	}
}

func TestManagedMaintenanceWindow_ApprovalWorkflow(t *testing.T) {
	window := &MaintenanceWindow{
		ID:              "test-window-1",
		Name:            "Test Maintenance",
		Status:          MaintenanceWindowStatusScheduled,
		StartTime:       time.Now().Add(1 * time.Hour),
		EndTime:         time.Now().Add(3 * time.Hour),
		RequireApproval: true,
	}

	mmw := NewManagedMaintenanceWindow(window, nil)

	// Require approval
	if err := mmw.RequireApproval(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mmw.Status() != MaintenanceWindowStatusPendingApproval {
		t.Errorf("expected pending_approval status, got %v", mmw.Status())
	}
	if !mmw.IsPendingApproval() {
		t.Error("expected IsPendingApproval() to be true")
	}

	// Approve
	if !mmw.CanApprove() {
		t.Error("expected CanApprove() to be true")
	}
	if err := mmw.Approve("admin@example.com"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mmw.Status() != MaintenanceWindowStatusScheduled {
		t.Errorf("expected scheduled status after approval, got %v", mmw.Status())
	}
	if window.ApprovedBy != "admin@example.com" {
		t.Errorf("expected ApprovedBy to be admin@example.com, got %s", window.ApprovedBy)
	}
	if window.ApprovedAt == nil {
		t.Error("expected ApprovedAt to be set")
	}

	// Now can activate
	if err := mmw.Activate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mmw.Status() != MaintenanceWindowStatusActive {
		t.Errorf("expected active status, got %v", mmw.Status())
	}
}

func TestManagedMaintenanceWindow_DirectActivateAfterApproval(t *testing.T) {
	window := &MaintenanceWindow{
		ID:              "test-window-1",
		Name:            "Test Maintenance",
		Status:          MaintenanceWindowStatusScheduled,
		StartTime:       time.Now().Add(1 * time.Hour),
		EndTime:         time.Now().Add(3 * time.Hour),
		RequireApproval: true,
	}

	mmw := NewManagedMaintenanceWindow(window, nil)

	mmw.RequireApproval()

	// Direct activate from pending approval (for immediate approval during window)
	if err := mmw.Activate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if mmw.Status() != MaintenanceWindowStatusActive {
		t.Errorf("expected active status, got %v", mmw.Status())
	}
}

func TestManagedMaintenanceWindow_CancelWorkflow(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*ManagedMaintenanceWindow)
	}{
		{"cancel from scheduled", func(mmw *ManagedMaintenanceWindow) {}},
		{"cancel from pending_approval", func(mmw *ManagedMaintenanceWindow) { mmw.RequireApproval() }},
		{"cancel from active", func(mmw *ManagedMaintenanceWindow) { mmw.Activate() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window := &MaintenanceWindow{
				ID:        "test-window-1",
				Name:      "Test Maintenance",
				Status:    MaintenanceWindowStatusScheduled,
				StartTime: time.Now().Add(1 * time.Hour),
				EndTime:   time.Now().Add(3 * time.Hour),
			}

			mmw := NewManagedMaintenanceWindow(window, nil)
			tt.setup(mmw)

			if !mmw.CanCancel() {
				t.Error("expected CanCancel() to be true")
			}
			if err := mmw.Cancel("ops@example.com", "Emergency cancelled"); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if mmw.Status() != MaintenanceWindowStatusCancelled {
				t.Errorf("expected cancelled status, got %v", mmw.Status())
			}
			if !mmw.IsTerminal() {
				t.Error("expected IsTerminal() to be true")
			}
			if window.CancelledBy != "ops@example.com" {
				t.Errorf("expected CancelledBy to be ops@example.com, got %s", window.CancelledBy)
			}
			if window.CancellationReason != "Emergency cancelled" {
				t.Errorf("expected CancellationReason to be 'Emergency cancelled', got %s", window.CancellationReason)
			}
		})
	}
}

func TestManagedMaintenanceWindow_ExpireWorkflow(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*ManagedMaintenanceWindow)
	}{
		{"expire from scheduled", func(mmw *ManagedMaintenanceWindow) {}},
		{"expire from pending_approval", func(mmw *ManagedMaintenanceWindow) { mmw.RequireApproval() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window := &MaintenanceWindow{
				ID:        "test-window-1",
				Name:      "Test Maintenance",
				Status:    MaintenanceWindowStatusScheduled,
				StartTime: time.Now().Add(-1 * time.Hour),
				EndTime:   time.Now().Add(-30 * time.Minute),
			}

			mmw := NewManagedMaintenanceWindow(window, nil)
			tt.setup(mmw)

			if err := mmw.Expire(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if mmw.Status() != MaintenanceWindowStatusExpired {
				t.Errorf("expected expired status, got %v", mmw.Status())
			}
			if !mmw.IsTerminal() {
				t.Error("expected IsTerminal() to be true")
			}
		})
	}
}

func TestManagedMaintenanceWindow_InvalidTransitions(t *testing.T) {
	window := &MaintenanceWindow{
		ID:        "test-window-1",
		Name:      "Test Maintenance",
		Status:    MaintenanceWindowStatusScheduled,
		StartTime: time.Now().Add(1 * time.Hour),
		EndTime:   time.Now().Add(3 * time.Hour),
	}

	mmw := NewManagedMaintenanceWindow(window, nil)

	// Cannot complete from scheduled
	err := mmw.Complete()
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// Status should not have changed
	if mmw.Status() != MaintenanceWindowStatusScheduled {
		t.Errorf("status should not have changed, got %v", mmw.Status())
	}
}

func TestManagedMaintenanceWindow_Callbacks(t *testing.T) {
	var activatedCalls, completedCalls, cancelledCalls int
	var lastActivatedID, lastCompletedID string

	callbacks := &MaintenanceCallbacks{
		OnActivated: func(windowID string) {
			activatedCalls++
			lastActivatedID = windowID
		},
		OnCompleted: func(windowID string) {
			completedCalls++
			lastCompletedID = windowID
		},
		OnCancelled: func(windowID, cancelledBy, reason string) {
			cancelledCalls++
		},
	}

	window := &MaintenanceWindow{
		ID:        "test-window-1",
		Name:      "Test Maintenance",
		Status:    MaintenanceWindowStatusScheduled,
		StartTime: time.Now().Add(1 * time.Hour),
		EndTime:   time.Now().Add(3 * time.Hour),
	}

	mmw := NewManagedMaintenanceWindow(window, callbacks)

	// Activate triggers callback
	mmw.Activate()
	if activatedCalls != 1 || lastActivatedID != "test-window-1" {
		t.Errorf("expected OnActivated called once, got %d", activatedCalls)
	}

	// Complete triggers callback
	mmw.Complete()
	if completedCalls != 1 || lastCompletedID != "test-window-1" {
		t.Errorf("expected OnCompleted called once, got %d", completedCalls)
	}

	// Test cancelled callback
	window2 := &MaintenanceWindow{
		ID:        "test-window-2",
		Name:      "Test Maintenance 2",
		Status:    MaintenanceWindowStatusScheduled,
		StartTime: time.Now().Add(1 * time.Hour),
		EndTime:   time.Now().Add(3 * time.Hour),
	}
	mmw2 := NewManagedMaintenanceWindow(window2, callbacks)
	mmw2.Cancel("ops", "reason")
	if cancelledCalls != 1 {
		t.Errorf("expected OnCancelled called once, got %d", cancelledCalls)
	}
}

func TestManagedMaintenanceWindow_History(t *testing.T) {
	window := &MaintenanceWindow{
		ID:        "test-window-1",
		Name:      "Test Maintenance",
		Status:    MaintenanceWindowStatusScheduled,
		StartTime: time.Now().Add(1 * time.Hour),
		EndTime:   time.Now().Add(3 * time.Hour),
	}

	mmw := NewManagedMaintenanceWindow(window, nil)

	mmw.Activate()
	mmw.Complete()

	history := mmw.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 2 {
		t.Errorf("expected 2 history records, got %d", len(records))
	}
}

func TestManagedMaintenanceWindow_AvailableEvents(t *testing.T) {
	window := &MaintenanceWindow{
		ID:        "test-window-1",
		Name:      "Test Maintenance",
		Status:    MaintenanceWindowStatusScheduled,
		StartTime: time.Now().Add(1 * time.Hour),
		EndTime:   time.Now().Add(3 * time.Hour),
	}

	mmw := NewManagedMaintenanceWindow(window, nil)

	// From scheduled, can require approval, activate, cancel, or expire
	events := mmw.AvailableEvents()
	if len(events) != 4 {
		t.Errorf("expected 4 available events from scheduled, got %d", len(events))
	}

	mmw.Activate()

	// From active, can complete or cancel
	events = mmw.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 available events from active, got %d", len(events))
	}
}

func TestManagedMaintenanceWindow_ActualDuration(t *testing.T) {
	window := &MaintenanceWindow{
		ID:        "test-window-1",
		Name:      "Test Maintenance",
		Status:    MaintenanceWindowStatusScheduled,
		StartTime: time.Now().Add(1 * time.Hour),
		EndTime:   time.Now().Add(3 * time.Hour),
	}

	mmw := NewManagedMaintenanceWindow(window, nil)

	// No duration before activation
	if mmw.ActualDuration() != 0 {
		t.Error("expected 0 duration before activation")
	}

	mmw.Activate()

	// Duration should be non-zero while active
	if err := helpers.WaitForTimeout(100*time.Millisecond, 5*time.Millisecond, func() (bool, error) {
		return mmw.ActualDuration() > 0, nil
	}); err != nil {
		t.Fatalf("expected duration to start: %v", err)
	}
	activeDuration := mmw.ActualDuration()
	if activeDuration == 0 {
		t.Error("expected non-zero duration while active")
	}

	mmw.Complete()

	// Duration should be fixed after completion
	finalDuration := mmw.ActualDuration()
	if finalDuration < activeDuration {
		t.Error("expected final duration >= active duration")
	}
}

func TestManagedMaintenanceWindow_NilCallbacks(t *testing.T) {
	window := &MaintenanceWindow{
		ID:        "test-window-1",
		Name:      "Test Maintenance",
		Status:    MaintenanceWindowStatusScheduled,
		StartTime: time.Now().Add(1 * time.Hour),
		EndTime:   time.Now().Add(3 * time.Hour),
	}

	// Empty callbacks struct
	callbacks := &MaintenanceCallbacks{}

	mmw := NewManagedMaintenanceWindow(window, callbacks)

	// These should not panic
	mmw.RequireApproval()
	mmw.Approve("admin")
	mmw.Activate()
	mmw.Complete()
}

func TestMaintenanceWindowStatusToString(t *testing.T) {
	tests := []struct {
		status  MaintenanceWindowStatus
		display string
	}{
		{MaintenanceWindowStatusScheduled, "Scheduled"},
		{MaintenanceWindowStatusPendingApproval, "Pending Approval"},
		{MaintenanceWindowStatusActive, "Active"},
		{MaintenanceWindowStatusCompleted, "Completed"},
		{MaintenanceWindowStatusCancelled, "Cancelled"},
		{MaintenanceWindowStatusExpired, "Expired"},
		{MaintenanceWindowStatus("unknown"), "unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := MaintenanceWindowStatusToString(tt.status); got != tt.display {
				t.Errorf("MaintenanceWindowStatusToString(%v) = %v, want %v", tt.status, got, tt.display)
			}
		})
	}
}

func TestManagedMaintenanceWindow_StatusSync(t *testing.T) {
	window := &MaintenanceWindow{
		ID:        "test-window-1",
		Name:      "Test Maintenance",
		Status:    MaintenanceWindowStatusScheduled,
		StartTime: time.Now().Add(1 * time.Hour),
		EndTime:   time.Now().Add(3 * time.Hour),
	}

	mmw := NewManagedMaintenanceWindow(window, nil)

	// Verify window.Status is synced with state machine
	if window.Status != MaintenanceWindowStatusScheduled {
		t.Errorf("expected window.Status to be scheduled, got %v", window.Status)
	}

	mmw.Activate()
	if window.Status != MaintenanceWindowStatusActive {
		t.Errorf("expected window.Status to be active, got %v", window.Status)
	}

	mmw.Complete()
	if window.Status != MaintenanceWindowStatusCompleted {
		t.Errorf("expected window.Status to be completed, got %v", window.Status)
	}
}
