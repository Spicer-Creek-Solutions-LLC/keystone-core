package upgrade

import (
	"errors"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func TestStateMachine_BasicWorkflow(t *testing.T) {
	usm := NewStateMachine(nil)

	// Initial state
	if usm.Phase() != PhaseIdle {
		t.Errorf("expected idle phase, got %v", usm.Phase())
	}

	// Start -> Pending
	if err := usm.Fire(EventStart); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if usm.Phase() != PhasePending {
		t.Errorf("expected pending phase, got %v", usm.Phase())
	}

	// Validate -> Validating
	if err := usm.Fire(EventValidate); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if usm.Phase() != PhaseValidating {
		t.Errorf("expected validating phase, got %v", usm.Phase())
	}

	// PrepareOk -> Preparing
	if err := usm.Fire(EventPrepareOk); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if usm.Phase() != PhasePreparing {
		t.Errorf("expected preparing phase, got %v", usm.Phase())
	}

	// BeginUpgrade -> Upgrading
	if err := usm.Fire(EventBeginUpgrade); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if usm.Phase() != PhaseUpgrading {
		t.Errorf("expected upgrading phase, got %v", usm.Phase())
	}

	// UpgradeComplete -> Verifying
	if err := usm.Fire(EventUpgradeComplete); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if usm.Phase() != PhaseVerifying {
		t.Errorf("expected verifying phase, got %v", usm.Phase())
	}

	// VerificationPassed -> Completed
	if err := usm.Fire(EventVerificationPassed); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if usm.Phase() != PhaseCompleted {
		t.Errorf("expected completed phase, got %v", usm.Phase())
	}

	// Should be terminal
	if !usm.IsTerminal() {
		t.Error("expected to be in terminal state")
	}
}

func TestStateMachine_FailureAndRollback(t *testing.T) {
	usm := NewStateMachine(nil)

	// Progress to upgrading
	usm.Fire(EventStart)
	usm.Fire(EventValidate)
	usm.Fire(EventPrepareOk)
	usm.Fire(EventBeginUpgrade)

	// Fail
	if err := usm.Fire(EventUpgradeFailed); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if usm.Phase() != PhaseFailed {
		t.Errorf("expected failed phase, got %v", usm.Phase())
	}
	if usm.Status() != StatusFailed {
		t.Errorf("expected failed status, got %v", usm.Status())
	}

	// Can rollback from failed
	if !usm.CanRollback() {
		t.Error("expected to be able to rollback")
	}

	// Initiate rollback
	if err := usm.Fire(EventInitiateRollback); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if usm.Phase() != PhaseRollingBack {
		t.Errorf("expected rolling_back phase, got %v", usm.Phase())
	}

	// Complete rollback
	if err := usm.Fire(EventRollbackComplete); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if usm.Phase() != PhaseRolledBack {
		t.Errorf("expected rolled_back phase, got %v", usm.Phase())
	}
	if usm.Status() != StatusRolledBack {
		t.Errorf("expected rolled_back status, got %v", usm.Status())
	}
}

func TestStateMachine_Cancel(t *testing.T) {
	tests := []struct {
		name        string
		setupEvents []Event
	}{
		{"cancel from pending", []Event{EventStart}},
		{"cancel from validating", []Event{EventStart, EventValidate}},
		{"cancel from preparing", []Event{EventStart, EventValidate, EventPrepareOk}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usm := NewStateMachine(nil)

			for _, event := range tt.setupEvents {
				usm.Fire(event)
			}

			if !usm.CanCancel() {
				t.Error("expected to be able to cancel")
			}

			if err := usm.Fire(EventCancel); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if usm.Phase() != PhaseCancelled {
				t.Errorf("expected cancelled phase, got %v", usm.Phase())
			}
			if usm.Status() != StatusCancelled {
				t.Errorf("expected cancelled status, got %v", usm.Status())
			}
		})
	}
}

func TestStateMachine_InvalidTransitions(t *testing.T) {
	usm := NewStateMachine(nil)

	// Can't go directly to upgrading from idle
	err := usm.Fire(EventBeginUpgrade)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
	if !errors.Is(err, statemachine.ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// Phase should not have changed
	if usm.Phase() != PhaseIdle {
		t.Errorf("phase should not have changed, got %v", usm.Phase())
	}
}

func TestStateMachine_CanFire(t *testing.T) {
	usm := NewStateMachine(nil)

	// From idle, can only start
	if !usm.CanFire(EventStart) {
		t.Error("should be able to fire start")
	}
	if usm.CanFire(EventValidate) {
		t.Error("should not be able to fire validate from idle")
	}

	usm.Fire(EventStart)

	// From pending, can validate or cancel
	if !usm.CanFire(EventValidate) {
		t.Error("should be able to fire validate")
	}
	if !usm.CanFire(EventCancel) {
		t.Error("should be able to fire cancel")
	}
	if usm.CanFire(EventBeginUpgrade) {
		t.Error("should not be able to fire begin_upgrade from pending")
	}
}

func TestStateMachine_AvailableEvents(t *testing.T) {
	usm := NewStateMachine(nil)

	events := usm.AvailableEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 available event, got %d", len(events))
	}
	if events[0] != EventStart {
		t.Errorf("expected start event, got %v", events[0])
	}

	usm.Fire(EventStart)

	events = usm.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("expected 2 available events, got %d", len(events))
	}
}

func TestStateMachine_Progress(t *testing.T) {
	usm := NewStateMachine(nil)

	tests := []struct {
		event    Event
		progress int
	}{
		{EventStart, 5},                // Pending
		{EventValidate, 15},            // Validating
		{EventPrepareOk, 30},           // Preparing
		{EventBeginUpgrade, 60},        // Upgrading
		{EventUpgradeComplete, 85},     // Verifying
		{EventVerificationPassed, 100}, // Completed
	}

	for _, tt := range tests {
		usm.Fire(tt.event)
		if usm.Progress() != tt.progress {
			t.Errorf("after %v: expected progress %d, got %d",
				tt.event, tt.progress, usm.Progress())
		}
	}
}

func TestStateMachine_IsActive(t *testing.T) {
	usm := NewStateMachine(nil)

	// Not active in idle
	if usm.IsActive() {
		t.Error("should not be active in idle")
	}

	usm.Fire(EventStart)
	if usm.IsActive() {
		t.Error("should not be active in pending")
	}

	usm.Fire(EventValidate)
	if !usm.IsActive() {
		t.Error("should be active in validating")
	}
}

func TestStateMachine_IsTerminal(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*StateMachine)
		terminal bool
	}{
		{
			"idle is not terminal",
			func(usm *StateMachine) {},
			false,
		},
		{
			"completed is terminal",
			func(usm *StateMachine) {
				usm.Fire(EventStart)
				usm.Fire(EventValidate)
				usm.Fire(EventPrepareOk)
				usm.Fire(EventBeginUpgrade)
				usm.Fire(EventUpgradeComplete)
				usm.Fire(EventVerificationPassed)
			},
			true,
		},
		{
			"cancelled is terminal",
			func(usm *StateMachine) {
				usm.Fire(EventStart)
				usm.Fire(EventCancel)
			},
			true,
		},
		{
			"rolled_back is terminal",
			func(usm *StateMachine) {
				usm.Fire(EventStart)
				usm.Fire(EventValidate)
				usm.Fire(EventValidationFailed)
				usm.Fire(EventInitiateRollback)
				usm.Fire(EventRollbackComplete)
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usm := NewStateMachine(nil)
			tt.setup(usm)
			if usm.IsTerminal() != tt.terminal {
				t.Errorf("expected terminal=%v, got %v", tt.terminal, usm.IsTerminal())
			}
		})
	}
}

func TestStateMachine_Callbacks(t *testing.T) {
	var phaseChanges []struct{ from, to Phase }

	config := &StateMachineConfig{
		Strategy: StrategyRolling,
		OnPhaseChange: func(from, to Phase) {
			phaseChanges = append(phaseChanges, struct{ from, to Phase }{from, to})
		},
		HistorySize: 50,
	}

	usm := NewStateMachine(config)

	usm.Fire(EventStart)
	usm.Fire(EventValidate)
	usm.Fire(EventPrepareOk)

	if len(phaseChanges) != 3 {
		t.Errorf("expected 3 phase changes, got %d", len(phaseChanges))
	}

	// Verify transitions
	if phaseChanges[0].from != PhaseIdle || phaseChanges[0].to != PhasePending {
		t.Error("first transition incorrect")
	}
	if phaseChanges[1].from != PhasePending || phaseChanges[1].to != PhaseValidating {
		t.Error("second transition incorrect")
	}
}

func TestStateMachine_History(t *testing.T) {
	config := &StateMachineConfig{
		Strategy:    StrategyRolling,
		HistorySize: 50,
	}

	usm := NewStateMachine(config)

	usm.Fire(EventStart)
	usm.Fire(EventValidate)
	usm.Fire(EventPrepareOk)

	history := usm.History()
	if history == nil {
		t.Fatal("history should not be nil")
	}

	records := history.All()
	if len(records) != 3 {
		t.Errorf("expected 3 history records, got %d", len(records))
	}
}

func TestStateMachine_ValidationFailure(t *testing.T) {
	usm := NewStateMachine(nil)

	usm.Fire(EventStart)
	usm.Fire(EventValidate)

	// Validation fails
	if err := usm.Fire(EventValidationFailed); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if usm.Phase() != PhaseFailed {
		t.Errorf("expected failed phase, got %v", usm.Phase())
	}
}

func TestStateMachine_RollbackFailure(t *testing.T) {
	usm := NewStateMachine(nil)

	// Progress to failed state
	usm.Fire(EventStart)
	usm.Fire(EventValidate)
	usm.Fire(EventValidationFailed)

	// Start rollback
	usm.Fire(EventInitiateRollback)
	if usm.Phase() != PhaseRollingBack {
		t.Errorf("expected rolling_back phase, got %v", usm.Phase())
	}

	// Rollback fails
	if err := usm.Fire(EventRollbackFailed); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if usm.Phase() != PhaseFailed {
		t.Errorf("expected failed phase after rollback failure, got %v", usm.Phase())
	}
}

func TestPhaseToStatus(t *testing.T) {
	tests := []struct {
		phase  Phase
		status Status
	}{
		{PhaseIdle, StatusPending},
		{PhasePending, StatusPending},
		{PhaseValidating, StatusInProgress},
		{PhasePreparing, StatusInProgress},
		{PhaseUpgrading, StatusInProgress},
		{PhaseVerifying, StatusInProgress},
		{PhaseCompleted, StatusCompleted},
		{PhaseFailed, StatusFailed},
		{PhaseRollingBack, StatusInProgress},
		{PhaseRolledBack, StatusRolledBack},
		{PhaseCancelled, StatusCancelled},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			if PhaseToStatus(tt.phase) != tt.status {
				t.Errorf("expected %v, got %v", tt.status, PhaseToStatus(tt.phase))
			}
		})
	}
}

func TestPhaseDisplayName(t *testing.T) {
	tests := []struct {
		phase   Phase
		display string
	}{
		{PhaseIdle, "Idle"},
		{PhaseUpgrading, "Upgrading"},
		{PhaseRollingBack, "Rolling Back"},
		{PhaseCompleted, "Completed"},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			if PhaseDisplayName(tt.phase) != tt.display {
				t.Errorf("expected %v, got %v", tt.display, PhaseDisplayName(tt.phase))
			}
		})
	}
}
