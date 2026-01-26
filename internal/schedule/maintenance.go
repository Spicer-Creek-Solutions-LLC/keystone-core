// Package schedule provides scheduled operations and maintenance window management.
package schedule

import "time"

// MaintenanceWindowStatus represents the status of a maintenance window.
type MaintenanceWindowStatus string

const (
	// MaintenanceWindowStatusScheduled indicates the window is scheduled.
	MaintenanceWindowStatusScheduled MaintenanceWindowStatus = "scheduled"

	// MaintenanceWindowStatusPendingApproval indicates waiting for approval.
	MaintenanceWindowStatusPendingApproval MaintenanceWindowStatus = "pending_approval"

	// MaintenanceWindowStatusActive indicates the window is currently active.
	MaintenanceWindowStatusActive MaintenanceWindowStatus = "active"

	// MaintenanceWindowStatusCompleted indicates the window has completed.
	MaintenanceWindowStatusCompleted MaintenanceWindowStatus = "completed"

	// MaintenanceWindowStatusCancelled indicates the window was cancelled.
	MaintenanceWindowStatusCancelled MaintenanceWindowStatus = "cancelled"

	// MaintenanceWindowStatusExpired indicates the window expired without starting.
	MaintenanceWindowStatusExpired MaintenanceWindowStatus = "expired"
)

// MaintenanceWindowType represents the type of maintenance.
type MaintenanceWindowType string

const (
	// MaintenanceWindowTypePlanned indicates planned maintenance.
	MaintenanceWindowTypePlanned MaintenanceWindowType = "planned"

	// MaintenanceWindowTypeEmergency indicates emergency maintenance.
	MaintenanceWindowTypeEmergency MaintenanceWindowType = "emergency"

	// MaintenanceWindowTypeRecurring indicates recurring maintenance.
	MaintenanceWindowTypeRecurring MaintenanceWindowType = "recurring"
)

// MaintenanceWindow defines a maintenance period.
type MaintenanceWindow struct {
	// ID is the unique window identifier.
	ID string `json:"id"`

	// Name is a human-readable name.
	Name string `json:"name"`

	// Description explains the maintenance.
	Description string `json:"description,omitempty"`

	// Type is the maintenance type.
	Type MaintenanceWindowType `json:"type"`

	// Status is the current status.
	Status MaintenanceWindowStatus `json:"status"`

	// StartTime is when the window opens.
	StartTime time.Time `json:"start_time"`

	// EndTime is when the window closes.
	EndTime time.Time `json:"end_time"`

	// ActualStartTime is when maintenance actually started.
	ActualStartTime *time.Time `json:"actual_start_time,omitempty"`

	// ActualEndTime is when maintenance actually ended.
	ActualEndTime *time.Time `json:"actual_end_time,omitempty"`

	// Timezone for the window times.
	Timezone string `json:"timezone,omitempty"`

	// RecurrenceRule is the RRULE for recurring windows (RFC 5545).
	RecurrenceRule string `json:"recurrence_rule,omitempty"`

	// Scope defines what is affected during maintenance.
	Scope *MaintenanceScope `json:"scope"`

	// AgentBehavior defines how agents behave during maintenance.
	AgentBehavior *AgentMaintenanceBehavior `json:"agent_behavior,omitempty"`

	// NotifyBefore specifies notification lead time.
	NotifyBefore time.Duration `json:"notify_before,omitempty"`

	// NotifyChannels lists notification channels.
	NotifyChannels []string `json:"notify_channels,omitempty"`

	// RequireApproval requires approval to start.
	RequireApproval bool `json:"require_approval"`

	// ApprovedBy is who approved the window.
	ApprovedBy string `json:"approved_by,omitempty"`

	// ApprovedAt is when it was approved.
	ApprovedAt *time.Time `json:"approved_at,omitempty"`

	// LinkedSchedules are schedules that run during this window.
	LinkedSchedules []string `json:"linked_schedules,omitempty"`

	// Labels for categorization.
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations for additional metadata.
	Annotations map[string]string `json:"annotations,omitempty"`

	// CancellationReason if the window was cancelled.
	CancellationReason string `json:"cancellation_reason,omitempty"`

	// CancelledBy is who cancelled the window.
	CancelledBy string `json:"cancelled_by,omitempty"`

	// CancelledAt is when the window was cancelled.
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`

	// Audit fields
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

// MaintenanceScope defines what is affected during maintenance.
type MaintenanceScope struct {
	// AgentIDs targets specific agents.
	AgentIDs []string `json:"agent_ids,omitempty"`

	// Glob pattern for agent IDs.
	Glob string `json:"glob,omitempty"`

	// Tags targets agents with matching tags.
	Tags map[string]string `json:"tags,omitempty"`

	// Roles targets agents with specific roles.
	Roles []string `json:"roles,omitempty"`

	// All affects all agents.
	All bool `json:"all,omitempty"`

	// Regions affected.
	Regions []string `json:"regions,omitempty"`

	// Environments affected.
	Environments []string `json:"environments,omitempty"`

	// Services affected (for documentation/filtering).
	Services []string `json:"services,omitempty"`
}

// AgentMaintenanceBehavior defines agent behavior during maintenance.
type AgentMaintenanceBehavior struct {
	// SuppressAlerts stops alerting during window.
	SuppressAlerts bool `json:"suppress_alerts"`

	// SuppressDriftDetection stops drift detection.
	SuppressDriftDetection bool `json:"suppress_drift_detection"`

	// AllowOperations allows manual operations to run.
	AllowOperations bool `json:"allow_operations"`

	// AllowScheduledOperations allows scheduled operations to run.
	AllowScheduledOperations bool `json:"allow_scheduled_operations"`

	// PauseHeartbeat pauses heartbeat requirements.
	PauseHeartbeat bool `json:"pause_heartbeat"`

	// MarkAgentsUnavailable marks affected agents as in maintenance.
	MarkAgentsUnavailable bool `json:"mark_agents_unavailable"`

	// DrainConnections gracefully drains connections before maintenance.
	DrainConnections bool `json:"drain_connections"`

	// DrainTimeout is how long to wait for connections to drain.
	DrainTimeout time.Duration `json:"drain_timeout,omitempty"`
}

// MaintenanceConflict represents a scheduling conflict.
type MaintenanceConflict struct {
	// WindowID is the window being checked.
	WindowID string `json:"window_id"`

	// WindowName is the name of the window being checked.
	WindowName string `json:"window_name"`

	// ConflictType is the type of conflict.
	ConflictType MaintenanceConflictType `json:"conflict_type"`

	// ConflictingID is the ID of the conflicting entity.
	ConflictingID string `json:"conflicting_id"`

	// ConflictingName is the name of the conflicting entity.
	ConflictingName string `json:"conflicting_name"`

	// OverlapStart is when the overlap starts.
	OverlapStart time.Time `json:"overlap_start"`

	// OverlapEnd is when the overlap ends.
	OverlapEnd time.Time `json:"overlap_end"`

	// AffectedAgents are agents affected by both windows.
	AffectedAgents []string `json:"affected_agents,omitempty"`

	// Severity indicates how serious the conflict is.
	Severity ConflictSeverity `json:"severity"`

	// Message describes the conflict.
	Message string `json:"message"`
}

// MaintenanceConflictType represents the type of conflict.
type MaintenanceConflictType string

const (
	// ConflictTypeOverlap indicates time overlap.
	ConflictTypeOverlap MaintenanceConflictType = "overlap"

	// ConflictTypeAgentOverlap indicates overlapping agent scope.
	ConflictTypeAgentOverlap MaintenanceConflictType = "agent_overlap"

	// ConflictTypeScheduleConflict indicates schedule conflict.
	ConflictTypeScheduleConflict MaintenanceConflictType = "schedule_conflict"

	// ConflictTypeResourceConflict indicates resource conflict.
	ConflictTypeResourceConflict MaintenanceConflictType = "resource_conflict"
)

// ConflictSeverity indicates how serious a conflict is.
type ConflictSeverity string

const (
	// ConflictSeverityInfo is informational only.
	ConflictSeverityInfo ConflictSeverity = "info"

	// ConflictSeverityWarning is a warning but can proceed.
	ConflictSeverityWarning ConflictSeverity = "warning"

	// ConflictSeverityError prevents the operation.
	ConflictSeverityError ConflictSeverity = "error"
)

// MaintenanceWindowFilter filters maintenance windows for listing.
type MaintenanceWindowFilter struct {
	// Status filters by window status.
	Status []MaintenanceWindowStatus `json:"status,omitempty"`

	// Type filters by window type.
	Type []MaintenanceWindowType `json:"type,omitempty"`

	// StartAfter filters by start time.
	StartAfter *time.Time `json:"start_after,omitempty"`

	// EndBefore filters by end time.
	EndBefore *time.Time `json:"end_before,omitempty"`

	// IncludesTime filters windows that include this time.
	IncludesTime *time.Time `json:"includes_time,omitempty"`

	// AgentID filters by affected agent.
	AgentID string `json:"agent_id,omitempty"`

	// Labels filters by labels (all must match).
	Labels map[string]string `json:"labels,omitempty"`

	// NameContains filters by name substring.
	NameContains string `json:"name_contains,omitempty"`

	// Limit limits the number of results.
	Limit int `json:"limit,omitempty"`

	// Offset for pagination.
	Offset int `json:"offset,omitempty"`
}

// MaintenanceStats holds maintenance statistics.
type MaintenanceStats struct {
	// TotalWindows is the total number of windows.
	TotalWindows int `json:"total_windows"`

	// ActiveWindows is the number of currently active windows.
	ActiveWindows int `json:"active_windows"`

	// ScheduledWindows is the number of scheduled windows.
	ScheduledWindows int `json:"scheduled_windows"`

	// CompletedWindows is the number of completed windows.
	CompletedWindows int `json:"completed_windows"`

	// CancelledWindows is the number of cancelled windows.
	CancelledWindows int `json:"cancelled_windows"`

	// ByType shows count by window type.
	ByType map[MaintenanceWindowType]int `json:"by_type"`

	// ByStatus shows count by status.
	ByStatus map[MaintenanceWindowStatus]int `json:"by_status"`

	// TotalDuration is the total maintenance duration.
	TotalDuration time.Duration `json:"total_duration"`

	// AverageDuration is the average window duration.
	AverageDuration time.Duration `json:"average_duration"`

	// AffectedAgentsNow is the number of agents currently in maintenance.
	AffectedAgentsNow int `json:"affected_agents_now"`

	// UpcomingIn24h is the number of windows starting in the next 24 hours.
	UpcomingIn24h int `json:"upcoming_in_24h"`
}

// MaintenanceEvent represents a maintenance-related event.
type MaintenanceEvent struct {
	// Type is the event type.
	Type string `json:"type"`

	// WindowID is the maintenance window ID.
	WindowID string `json:"window_id"`

	// WindowName is the window name.
	WindowName string `json:"window_name"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Actor is who triggered the event.
	Actor string `json:"actor,omitempty"`

	// Data contains event-specific data.
	Data map[string]interface{} `json:"data,omitempty"`
}

// ScheduleEvent represents a schedule-related event.
type ScheduleEvent struct {
	// Type is the event type.
	Type string `json:"type"`

	// ScheduleID is the schedule ID.
	ScheduleID string `json:"schedule_id"`

	// ScheduleName is the schedule name.
	ScheduleName string `json:"schedule_name"`

	// Schedule is the schedule (optional, for internal use).
	Schedule *Schedule `json:"schedule,omitempty"`

	// ExecutionID is the execution ID if applicable.
	ExecutionID string `json:"execution_id,omitempty"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Actor is who triggered the event.
	Actor string `json:"actor,omitempty"`

	// Message is an optional message.
	Message string `json:"message,omitempty"`

	// Data contains event-specific data.
	Data map[string]interface{} `json:"data,omitempty"`
}

// Duration returns the planned duration of the maintenance window.
func (w *MaintenanceWindow) Duration() time.Duration {
	return w.EndTime.Sub(w.StartTime)
}

// IsActive returns true if the maintenance window is currently active.
func (w *MaintenanceWindow) IsActive() bool {
	return w.Status == MaintenanceWindowStatusActive
}

// IsScheduled returns true if the maintenance window is scheduled but not yet active.
func (w *MaintenanceWindow) IsScheduled() bool {
	return w.Status == MaintenanceWindowStatusScheduled ||
		w.Status == MaintenanceWindowStatusPendingApproval
}

// ContainsTime returns true if the given time falls within the maintenance window.
func (w *MaintenanceWindow) ContainsTime(t time.Time) bool {
	return !t.Before(w.StartTime) && !t.After(w.EndTime)
}

// Overlaps returns true if this window overlaps with another window.
func (w *MaintenanceWindow) Overlaps(other *MaintenanceWindow) bool {
	return w.StartTime.Before(other.EndTime) && w.EndTime.After(other.StartTime)
}
