package schedule

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

// MaintenanceStateEvent represents events that trigger maintenance window state transitions.
//
// State diagram:
//
// ```mermaid
// stateDiagram-v2
//     [*] --> Scheduled
//     Scheduled --> PendingApproval: RequireApproval
//     Scheduled --> Active: Activate
//     Scheduled --> Cancelled: Cancel
//     Scheduled --> Expired: Expire
//     PendingApproval --> Scheduled: Approve
//     PendingApproval --> Cancelled: Cancel
//     PendingApproval --> Expired: Expire
//     Active --> Completed: Complete
//     Active --> Cancelled: Cancel
//     Completed --> [*]
//     Cancelled --> [*]
//     Expired --> [*]
// ```
type MaintenanceStateEvent string

const (
	// MaintEventRequireApproval transitions to pending approval
	MaintEventRequireApproval MaintenanceStateEvent = "require_approval"
	// MaintEventApprove approves the window
	MaintEventApprove MaintenanceStateEvent = "approve"
	// MaintEventActivate activates the window
	MaintEventActivate MaintenanceStateEvent = "activate"
	// MaintEventComplete completes the window
	MaintEventComplete MaintenanceStateEvent = "complete"
	// MaintEventCancel cancels the window
	MaintEventCancel MaintenanceStateEvent = "cancel"
	// MaintEventExpire expires the window
	MaintEventExpire MaintenanceStateEvent = "expire"
)

// MaintenanceCallbacks holds callbacks for maintenance window state transitions.
type MaintenanceCallbacks struct {
	// OnApprovalRequired is called when approval is required
	OnApprovalRequired func(windowID string)
	// OnApproved is called when window is approved
	OnApproved func(windowID, approvedBy string)
	// OnActivated is called when window becomes active
	OnActivated func(windowID string)
	// OnCompleted is called when window completes
	OnCompleted func(windowID string)
	// OnCancelled is called when window is cancelled
	OnCancelled func(windowID, cancelledBy, reason string)
	// OnExpired is called when window expires
	OnExpired func(windowID string)
}

// ManagedMaintenanceWindow wraps MaintenanceWindow with a state machine.
type ManagedMaintenanceWindow struct {
	Window  *MaintenanceWindow
	machine *statemachine.Machine[MaintenanceWindowStatus, MaintenanceStateEvent]

	// Tracking
	windowID    string
	callbacks   *MaintenanceCallbacks
	approvedBy  string
	cancelledBy string
	reason      string
}

// NewManagedMaintenanceWindow creates a new managed maintenance window with state machine.
func NewManagedMaintenanceWindow(window *MaintenanceWindow, callbacks *MaintenanceCallbacks) *ManagedMaintenanceWindow {
	mmw := &ManagedMaintenanceWindow{
		Window:    window,
		windowID:  window.ID,
		callbacks: callbacks,
	}

	mmw.machine = statemachine.New[MaintenanceWindowStatus, MaintenanceStateEvent](MaintenanceWindowStatusScheduled).
		WithName("maintenance-window-" + window.ID).
		WithHistory(10).
		// From Scheduled
		AddTransition(MaintenanceWindowStatusScheduled, MaintEventRequireApproval, MaintenanceWindowStatusPendingApproval).
		AddTransition(MaintenanceWindowStatusScheduled, MaintEventActivate, MaintenanceWindowStatusActive).
		AddTransition(MaintenanceWindowStatusScheduled, MaintEventCancel, MaintenanceWindowStatusCancelled).
		AddTransition(MaintenanceWindowStatusScheduled, MaintEventExpire, MaintenanceWindowStatusExpired).
		// From PendingApproval
		AddTransition(MaintenanceWindowStatusPendingApproval, MaintEventApprove, MaintenanceWindowStatusScheduled).
		AddTransition(MaintenanceWindowStatusPendingApproval, MaintEventCancel, MaintenanceWindowStatusCancelled).
		AddTransition(MaintenanceWindowStatusPendingApproval, MaintEventExpire, MaintenanceWindowStatusExpired).
		// Allow direct activation after approval if within window time
		AddTransition(MaintenanceWindowStatusPendingApproval, MaintEventActivate, MaintenanceWindowStatusActive).
		// From Active
		AddTransition(MaintenanceWindowStatusActive, MaintEventComplete, MaintenanceWindowStatusCompleted).
		AddTransition(MaintenanceWindowStatusActive, MaintEventCancel, MaintenanceWindowStatusCancelled).
		// Callbacks
		OnEnter(MaintenanceWindowStatusPendingApproval, func(ctx context.Context, state, from MaintenanceWindowStatus) {
			mmw.Window.Status = MaintenanceWindowStatusPendingApproval
			if mmw.callbacks != nil && mmw.callbacks.OnApprovalRequired != nil {
				mmw.callbacks.OnApprovalRequired(mmw.windowID)
			}
		}).
		OnEnter(MaintenanceWindowStatusScheduled, func(ctx context.Context, state, from MaintenanceWindowStatus) {
			mmw.Window.Status = MaintenanceWindowStatusScheduled
			if from == MaintenanceWindowStatusPendingApproval {
				// Coming from approval
				now := time.Now()
				mmw.Window.ApprovedAt = &now
				mmw.Window.ApprovedBy = mmw.approvedBy
				if mmw.callbacks != nil && mmw.callbacks.OnApproved != nil {
					mmw.callbacks.OnApproved(mmw.windowID, mmw.approvedBy)
				}
			}
		}).
		OnEnter(MaintenanceWindowStatusActive, func(ctx context.Context, state, from MaintenanceWindowStatus) {
			mmw.Window.Status = MaintenanceWindowStatusActive
			now := time.Now()
			mmw.Window.ActualStartTime = &now
			if mmw.callbacks != nil && mmw.callbacks.OnActivated != nil {
				mmw.callbacks.OnActivated(mmw.windowID)
			}
		}).
		OnEnter(MaintenanceWindowStatusCompleted, func(ctx context.Context, state, from MaintenanceWindowStatus) {
			mmw.Window.Status = MaintenanceWindowStatusCompleted
			now := time.Now()
			mmw.Window.ActualEndTime = &now
			if mmw.callbacks != nil && mmw.callbacks.OnCompleted != nil {
				mmw.callbacks.OnCompleted(mmw.windowID)
			}
		}).
		OnEnter(MaintenanceWindowStatusCancelled, func(ctx context.Context, state, from MaintenanceWindowStatus) {
			mmw.Window.Status = MaintenanceWindowStatusCancelled
			now := time.Now()
			mmw.Window.CancelledAt = &now
			mmw.Window.CancelledBy = mmw.cancelledBy
			mmw.Window.CancellationReason = mmw.reason
			if mmw.callbacks != nil && mmw.callbacks.OnCancelled != nil {
				mmw.callbacks.OnCancelled(mmw.windowID, mmw.cancelledBy, mmw.reason)
			}
		}).
		OnEnter(MaintenanceWindowStatusExpired, func(ctx context.Context, state, from MaintenanceWindowStatus) {
			mmw.Window.Status = MaintenanceWindowStatusExpired
			if mmw.callbacks != nil && mmw.callbacks.OnExpired != nil {
				mmw.callbacks.OnExpired(mmw.windowID)
			}
		}).
		MustBuild()

	return mmw
}

// Status returns the current maintenance window status.
func (mmw *ManagedMaintenanceWindow) Status() MaintenanceWindowStatus {
	return mmw.machine.State()
}

// RequireApproval marks the window as requiring approval.
func (mmw *ManagedMaintenanceWindow) RequireApproval() error {
	return mmw.machine.Fire(MaintEventRequireApproval)
}

// Approve approves the maintenance window.
func (mmw *ManagedMaintenanceWindow) Approve(approvedBy string) error {
	mmw.approvedBy = approvedBy
	return mmw.machine.Fire(MaintEventApprove)
}

// Activate activates the maintenance window.
func (mmw *ManagedMaintenanceWindow) Activate() error {
	return mmw.machine.Fire(MaintEventActivate)
}

// Complete completes the maintenance window.
func (mmw *ManagedMaintenanceWindow) Complete() error {
	return mmw.machine.Fire(MaintEventComplete)
}

// Cancel cancels the maintenance window.
func (mmw *ManagedMaintenanceWindow) Cancel(cancelledBy, reason string) error {
	mmw.cancelledBy = cancelledBy
	mmw.reason = reason
	return mmw.machine.Fire(MaintEventCancel)
}

// Expire expires the maintenance window.
func (mmw *ManagedMaintenanceWindow) Expire() error {
	return mmw.machine.Fire(MaintEventExpire)
}

// CanApprove returns true if the window can be approved.
func (mmw *ManagedMaintenanceWindow) CanApprove() bool {
	return mmw.machine.CanFire(MaintEventApprove)
}

// CanActivate returns true if the window can be activated.
func (mmw *ManagedMaintenanceWindow) CanActivate() bool {
	return mmw.machine.CanFire(MaintEventActivate)
}

// CanCancel returns true if the window can be cancelled.
func (mmw *ManagedMaintenanceWindow) CanCancel() bool {
	return mmw.machine.CanFire(MaintEventCancel)
}

// IsPendingApproval returns true if the window is pending approval.
func (mmw *ManagedMaintenanceWindow) IsPendingApproval() bool {
	return mmw.machine.IsInState(MaintenanceWindowStatusPendingApproval)
}

// IsScheduled returns true if the window is scheduled.
func (mmw *ManagedMaintenanceWindow) IsScheduled() bool {
	return mmw.machine.IsInState(MaintenanceWindowStatusScheduled)
}

// IsActive returns true if the window is active.
func (mmw *ManagedMaintenanceWindow) IsActive() bool {
	return mmw.machine.IsInState(MaintenanceWindowStatusActive)
}

// IsTerminal returns true if the window is in a terminal state.
func (mmw *ManagedMaintenanceWindow) IsTerminal() bool {
	return mmw.machine.IsInAnyState(
		MaintenanceWindowStatusCompleted,
		MaintenanceWindowStatusCancelled,
		MaintenanceWindowStatusExpired,
	)
}

// History returns the state transition history.
func (mmw *ManagedMaintenanceWindow) History() *statemachine.History[MaintenanceWindowStatus, MaintenanceStateEvent] {
	return mmw.machine.History()
}

// AvailableEvents returns events that can be fired from the current state.
func (mmw *ManagedMaintenanceWindow) AvailableEvents() []MaintenanceStateEvent {
	return mmw.machine.AvailableEvents()
}

// ActualDuration returns the actual duration if completed, or elapsed time if active.
func (mmw *ManagedMaintenanceWindow) ActualDuration() time.Duration {
	if mmw.Window.ActualStartTime == nil {
		return 0
	}
	if mmw.Window.ActualEndTime == nil {
		return time.Since(*mmw.Window.ActualStartTime)
	}
	return mmw.Window.ActualEndTime.Sub(*mmw.Window.ActualStartTime)
}

// MaintenanceWindowStatusToString returns a human-readable name for the status.
func MaintenanceWindowStatusToString(status MaintenanceWindowStatus) string {
	switch status {
	case MaintenanceWindowStatusScheduled:
		return "Scheduled"
	case MaintenanceWindowStatusPendingApproval:
		return "Pending Approval"
	case MaintenanceWindowStatusActive:
		return "Active"
	case MaintenanceWindowStatusCompleted:
		return "Completed"
	case MaintenanceWindowStatusCancelled:
		return "Cancelled"
	case MaintenanceWindowStatusExpired:
		return "Expired"
	default:
		return string(status)
	}
}
