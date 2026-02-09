// Package schedule provides scheduled operations and maintenance window management.
package schedule

import (
	"context"
	"errors"
)

// Common errors for schedule storage and management operations.
var (
	// ErrScheduleNotFound indicates a schedule was not found.
	ErrScheduleNotFound = errors.New("schedule not found")

	// ErrExecutionNotFound indicates an execution was not found.
	ErrExecutionNotFound = errors.New("execution not found")

	// ErrMaintenanceWindowNotFound indicates a maintenance window was not found.
	ErrMaintenanceWindowNotFound = errors.New("maintenance window not found")

	// ErrScheduleExists indicates a schedule with the same ID already exists.
	ErrScheduleExists = errors.New("schedule already exists")

	// ErrMaintenanceWindowExists indicates a maintenance window with the same ID already exists.
	ErrMaintenanceWindowExists = errors.New("maintenance window already exists")

	// ErrLockAcquisitionFailed indicates lock acquisition failed.
	ErrLockAcquisitionFailed = errors.New("failed to acquire lock")

	// ErrStoreNotConnected indicates the store is not connected.
	ErrStoreNotConnected = errors.New("store not connected")

	// ErrStoreClosed indicates the store has been closed.
	ErrStoreClosed = errors.New("store closed")

	// ErrScheduleDisabled indicates the schedule is disabled.
	ErrScheduleDisabled = errors.New("schedule is disabled")

	// ErrScheduleInProgress indicates schedule execution is already in progress.
	ErrScheduleInProgress = errors.New("schedule execution already in progress")

	// ErrInvalidSchedule indicates invalid schedule configuration.
	ErrInvalidSchedule = errors.New("invalid schedule configuration")

	// ErrInvalidCron indicates invalid cron expression.
	ErrInvalidCron = errors.New("invalid cron expression")

	// ErrExecutionNotPending indicates the execution is not pending approval.
	ErrExecutionNotPending = errors.New("execution is not pending approval")

	// ErrMaintenanceActive indicates the maintenance window is currently active.
	ErrMaintenanceActive = errors.New("maintenance window is currently active")

	// ErrMaintenanceConflict indicates maintenance window conflict.
	ErrMaintenanceConflict = errors.New("maintenance window conflicts with existing window")

	// ErrApprovalRequired indicates execution requires approval.
	ErrApprovalRequired = errors.New("execution requires approval")

	// ErrLockNotAcquired indicates the execution lock could not be acquired.
	ErrLockNotAcquired = errors.New("could not acquire execution lock")
)

// Store defines the interface for schedule and maintenance window storage.
type Store interface {
	// Schedule operations

	// CreateSchedule creates a new schedule.
	// Returns ErrScheduleExists if a schedule with the same ID already exists.
	CreateSchedule(ctx context.Context, schedule *Schedule) error

	// GetSchedule retrieves a schedule by ID.
	// Returns ErrScheduleNotFound if the schedule does not exist.
	GetSchedule(ctx context.Context, id string) (*Schedule, error)

	// UpdateSchedule updates an existing schedule.
	// Returns ErrScheduleNotFound if the schedule does not exist.
	UpdateSchedule(ctx context.Context, schedule *Schedule) error

	// DeleteSchedule deletes a schedule by ID.
	// Returns ErrScheduleNotFound if the schedule does not exist.
	DeleteSchedule(ctx context.Context, id string) error

	// ListSchedules lists schedules matching the filter.
	ListSchedules(ctx context.Context, filter *Filter) ([]*Schedule, error)

	// WatchSchedules watches for schedule changes.
	// The handler is called for each change event.
	WatchSchedules(ctx context.Context, handler WatchHandler) error

	// Execution operations

	// CreateExecution creates a new execution record.
	CreateExecution(ctx context.Context, execution *Execution) error

	// GetExecution retrieves an execution by ID.
	// Returns ErrExecutionNotFound if the execution does not exist.
	GetExecution(ctx context.Context, id string) (*Execution, error)

	// UpdateExecution updates an existing execution.
	// Returns ErrExecutionNotFound if the execution does not exist.
	UpdateExecution(ctx context.Context, execution *Execution) error

	// ListExecutions lists executions matching the filter.
	ListExecutions(ctx context.Context, filter *ExecutionFilter) ([]*Execution, error)

	// DeleteOldExecutions deletes executions older than the specified count per schedule.
	// Keeps the most recent 'keepCount' executions per schedule.
	DeleteOldExecutions(ctx context.Context, scheduleID string, keepCount int) (int, error)

	// Maintenance window operations

	// CreateMaintenanceWindow creates a new maintenance window.
	// Returns ErrMaintenanceWindowExists if a window with the same ID already exists.
	CreateMaintenanceWindow(ctx context.Context, window *MaintenanceWindow) error

	// GetMaintenanceWindow retrieves a maintenance window by ID.
	// Returns ErrMaintenanceWindowNotFound if the window does not exist.
	GetMaintenanceWindow(ctx context.Context, id string) (*MaintenanceWindow, error)

	// UpdateMaintenanceWindow updates an existing maintenance window.
	// Returns ErrMaintenanceWindowNotFound if the window does not exist.
	UpdateMaintenanceWindow(ctx context.Context, window *MaintenanceWindow) error

	// DeleteMaintenanceWindow deletes a maintenance window by ID.
	// Returns ErrMaintenanceWindowNotFound if the window does not exist.
	DeleteMaintenanceWindow(ctx context.Context, id string) error

	// ListMaintenanceWindows lists maintenance windows matching the filter.
	ListMaintenanceWindows(ctx context.Context, filter *MaintenanceWindowFilter) ([]*MaintenanceWindow, error)

	// WatchMaintenanceWindows watches for maintenance window changes.
	WatchMaintenanceWindows(ctx context.Context, handler MaintenanceWindowWatchHandler) error

	// Lock operations

	// AcquireLock attempts to acquire a distributed lock for schedule execution.
	// Returns true if the lock was acquired, false if it was already held.
	// The lock is released when the context is cancelled or ReleaseLock is called.
	AcquireLock(ctx context.Context, lockID string, holderID string) (bool, error)

	// ReleaseLock releases a distributed lock.
	ReleaseLock(ctx context.Context, lockID string, holderID string) error

	// IsLocked checks if a lock is currently held.
	IsLocked(ctx context.Context, lockID string) (bool, string, error)

	// Lifecycle

	// Close closes the store connection.
	Close() error
}

// WatchEventType represents the type of watch event.
type WatchEventType string

const (
	// WatchEventCreated indicates an item was created.
	WatchEventCreated WatchEventType = "created"

	// WatchEventUpdated indicates an item was updated.
	WatchEventUpdated WatchEventType = "updated"

	// WatchEventDeleted indicates an item was deleted.
	WatchEventDeleted WatchEventType = "deleted"
)

// WatchEvent represents a schedule change event.
type WatchEvent struct {
	// Type is the event type.
	Type WatchEventType

	// Schedule is the schedule (nil for delete events).
	Schedule *Schedule

	// ScheduleID is the schedule ID (useful for delete events).
	ScheduleID string
}

// WatchHandler handles schedule watch events.
type WatchHandler func(event *WatchEvent)

// MaintenanceWindowWatchEvent represents a maintenance window change event.
type MaintenanceWindowWatchEvent struct {
	// Type is the event type.
	Type WatchEventType

	// Window is the maintenance window (nil for delete events).
	Window *MaintenanceWindow

	// WindowID is the window ID (useful for delete events).
	WindowID string
}

// MaintenanceWindowWatchHandler handles maintenance window watch events.
type MaintenanceWindowWatchHandler func(event *MaintenanceWindowWatchEvent)

// LockInfo contains information about a lock.
type LockInfo struct {
	// LockID is the lock identifier.
	LockID string

	// HolderID is the ID of the current holder.
	HolderID string

	// AcquiredAt is when the lock was acquired.
	AcquiredAt string
}
