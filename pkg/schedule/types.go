// Package schedule provides scheduled operations and maintenance window management.
package schedule

import (
	"encoding/json"
	"time"
)


// ScheduleType identifies the type of scheduled operation.
type ScheduleType string

const (
	// ScheduleTypeCommand executes a remote command on targeted agents.
	ScheduleTypeCommand ScheduleType = "command"

	// ScheduleTypeState applies a state declaration.
	ScheduleTypeState ScheduleType = "state"

	// ScheduleTypeBlueprint applies a blueprint.
	ScheduleTypeBlueprint ScheduleType = "blueprint"

	// ScheduleTypeReactor triggers a reactor action.
	ScheduleTypeReactor ScheduleType = "reactor"

	// ScheduleTypeCustom executes a custom handler.
	ScheduleTypeCustom ScheduleType = "custom"
)

// ScheduleStatus represents the status of a schedule.
type ScheduleStatus string

const (
	// ScheduleStatusActive indicates the schedule is active and will run.
	ScheduleStatusActive ScheduleStatus = "active"

	// ScheduleStatusPaused indicates the schedule is temporarily paused.
	ScheduleStatusPaused ScheduleStatus = "paused"

	// ScheduleStatusDisabled indicates the schedule is disabled.
	ScheduleStatusDisabled ScheduleStatus = "disabled"

	// ScheduleStatusExpired indicates the schedule has passed its end date.
	ScheduleStatusExpired ScheduleStatus = "expired"
)

// Schedule defines a scheduled operation.
type Schedule struct {
	// ID is the unique schedule identifier.
	ID string `json:"id"`

	// Name is a human-readable name.
	Name string `json:"name"`

	// Description explains what this schedule does.
	Description string `json:"description,omitempty"`

	// Type is the operation type.
	Type ScheduleType `json:"type"`

	// Cron is the cron expression (5 or 6 fields).
	// If empty, uses Interval for frequency-based scheduling.
	Cron string `json:"cron,omitempty"`

	// Interval is the frequency for non-cron schedules.
	Interval time.Duration `json:"interval,omitempty"`

	// Timezone for schedule evaluation (default: UTC).
	Timezone string `json:"timezone,omitempty"`

	// Window restricts when the schedule can run.
	Window *TimeWindow `json:"window,omitempty"`

	// Target defines which agents to target.
	Target *ScheduleTarget `json:"target"`

	// Payload contains operation-specific data.
	Payload json.RawMessage `json:"payload,omitempty"`

	// Status is the current schedule status.
	Status ScheduleStatus `json:"status"`

	// Priority determines execution order (higher = more important).
	Priority int `json:"priority"`

	// MaxConcurrent limits concurrent executions (0 = unlimited).
	MaxConcurrent int `json:"max_concurrent"`

	// Timeout is the maximum execution time per run.
	Timeout time.Duration `json:"timeout"`

	// RetryPolicy defines retry behavior on failure.
	RetryPolicy *RetryPolicy `json:"retry_policy,omitempty"`

	// MaintenanceWindowID links to a maintenance window (optional).
	// If set, schedule only runs during this window.
	MaintenanceWindowID string `json:"maintenance_window_id,omitempty"`

	// RequireApproval requires manual approval before execution.
	RequireApproval bool `json:"require_approval"`

	// NotifyBefore specifies how long before to send notifications.
	NotifyBefore time.Duration `json:"notify_before,omitempty"`

	// NotifyChannels lists notification channels.
	NotifyChannels []string `json:"notify_channels,omitempty"`

	// StartDate is when the schedule becomes active (optional).
	StartDate *time.Time `json:"start_date,omitempty"`

	// EndDate is when the schedule expires (optional).
	EndDate *time.Time `json:"end_date,omitempty"`

	// Labels for categorization and filtering.
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations for additional metadata.
	Annotations map[string]string `json:"annotations,omitempty"`

	// LastRun is the time of the last execution.
	LastRun *time.Time `json:"last_run,omitempty"`

	// NextRun is the calculated next execution time.
	NextRun *time.Time `json:"next_run,omitempty"`

	// RunCount is the total number of executions.
	RunCount int64 `json:"run_count"`

	// SuccessCount is the number of successful executions.
	SuccessCount int64 `json:"success_count"`

	// FailureCount is the number of failed executions.
	FailureCount int64 `json:"failure_count"`

	// Audit fields
	CreatedAt time.Time `json:"created_at"`
	CreatedBy string    `json:"created_by"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

// TimeWindow defines when schedules can run.
type TimeWindow struct {
	// DaysOfWeek (0=Sunday, 6=Saturday).
	DaysOfWeek []int `json:"days_of_week,omitempty"`

	// StartTime is the start of the window (HH:MM format).
	StartTime string `json:"start_time"`

	// EndTime is the end of the window (HH:MM format).
	EndTime string `json:"end_time"`

	// Timezone for the window (default: UTC).
	Timezone string `json:"timezone,omitempty"`

	// ExcludeDates lists dates to skip (YYYY-MM-DD format).
	ExcludeDates []string `json:"exclude_dates,omitempty"`

	// IncludeOnlyDates limits to specific dates (YYYY-MM-DD format).
	// If set, only these dates are allowed.
	IncludeOnlyDates []string `json:"include_only_dates,omitempty"`
}

// ScheduleTarget defines which agents to target.
type ScheduleTarget struct {
	// AgentIDs targets specific agents.
	AgentIDs []string `json:"agent_ids,omitempty"`

	// Glob pattern for agent IDs.
	Glob string `json:"glob,omitempty"`

	// Tags targets agents with matching tags.
	Tags map[string]string `json:"tags,omitempty"`

	// Roles targets agents with specific roles.
	Roles []string `json:"roles,omitempty"`

	// All targets all agents.
	All bool `json:"all,omitempty"`

	// Percent targets a percentage of matching agents (for canary).
	Percent int `json:"percent,omitempty"`

	// MaxAgents limits the number of agents targeted.
	MaxAgents int `json:"max_agents,omitempty"`

	// Regions targets agents in specific regions.
	Regions []string `json:"regions,omitempty"`

	// Environments targets agents in specific environments.
	Environments []string `json:"environments,omitempty"`
}

// RetryPolicy defines retry behavior.
type RetryPolicy struct {
	// MaxRetries is the maximum number of retries.
	MaxRetries int `json:"max_retries"`

	// RetryDelay is the delay between retries.
	RetryDelay time.Duration `json:"retry_delay"`

	// BackoffMultiplier multiplies delay on each retry.
	BackoffMultiplier float64 `json:"backoff_multiplier,omitempty"`

	// MaxDelay caps the retry delay.
	MaxDelay time.Duration `json:"max_delay,omitempty"`

	// RetryOn specifies which error types to retry on.
	RetryOn []string `json:"retry_on,omitempty"`
}

// ExecutionStatus represents the status of an execution.
type ExecutionStatus string

const (
	// ExecutionStatusPending indicates waiting for approval.
	ExecutionStatusPending ExecutionStatus = "pending"

	// ExecutionStatusApproved indicates approved and ready to run.
	ExecutionStatusApproved ExecutionStatus = "approved"

	// ExecutionStatusRunning indicates currently executing.
	ExecutionStatusRunning ExecutionStatus = "running"

	// ExecutionStatusCompleted indicates successful completion.
	ExecutionStatusCompleted ExecutionStatus = "completed"

	// ExecutionStatusFailed indicates execution failed.
	ExecutionStatusFailed ExecutionStatus = "failed"

	// ExecutionStatusCancelled indicates execution was cancelled.
	ExecutionStatusCancelled ExecutionStatus = "cancelled"

	// ExecutionStatusSkipped indicates execution was skipped.
	ExecutionStatusSkipped ExecutionStatus = "skipped"

	// ExecutionStatusTimeout indicates execution timed out.
	ExecutionStatusTimeout ExecutionStatus = "timeout"

	// ExecutionStatusRejected indicates approval was rejected.
	ExecutionStatusRejected ExecutionStatus = "rejected"
)

// TriggerType indicates how an execution was triggered.
type TriggerType string

const (
	// TriggerTypeScheduled indicates triggered by schedule.
	TriggerTypeScheduled TriggerType = "scheduled"

	// TriggerTypeManual indicates triggered manually.
	TriggerTypeManual TriggerType = "manual"

	// TriggerTypeAPI indicates triggered via API.
	TriggerTypeAPI TriggerType = "api"

	// TriggerTypeReactor indicates triggered by a reactor.
	TriggerTypeReactor TriggerType = "reactor"

	// TriggerTypeMaintenance indicates triggered by maintenance window.
	TriggerTypeMaintenance TriggerType = "maintenance"
)

// ScheduleExecution represents a single execution of a schedule.
type ScheduleExecution struct {
	// ID is the unique execution identifier.
	ID string `json:"id"`

	// ScheduleID links to the schedule.
	ScheduleID string `json:"schedule_id"`

	// ScheduleName for convenience.
	ScheduleName string `json:"schedule_name"`

	// Status is the execution status.
	Status ExecutionStatus `json:"status"`

	// TriggerType indicates how this execution was triggered.
	TriggerType TriggerType `json:"trigger_type"`

	// TriggeredBy is who/what triggered the execution.
	TriggeredBy string `json:"triggered_by,omitempty"`

	// ScheduledTime is when the execution was supposed to run.
	ScheduledTime time.Time `json:"scheduled_time"`

	// StartTime is when the execution actually started.
	StartTime *time.Time `json:"start_time,omitempty"`

	// EndTime is when the execution completed.
	EndTime *time.Time `json:"end_time,omitempty"`

	// Duration is the total execution time.
	Duration time.Duration `json:"duration,omitempty"`

	// TargetCount is the number of agents targeted.
	TargetCount int `json:"target_count"`

	// SuccessCount is the number of successful agent executions.
	SuccessCount int `json:"success_count"`

	// FailureCount is the number of failed agent executions.
	FailureCount int `json:"failure_count"`

	// SkippedCount is the number of skipped agents.
	SkippedCount int `json:"skipped_count"`

	// Results contains per-agent results.
	Results []*AgentExecutionResult `json:"results,omitempty"`

	// Error is the overall error if execution failed.
	Error string `json:"error,omitempty"`

	// ApprovedBy is who approved the execution (if approval required).
	ApprovedBy string `json:"approved_by,omitempty"`

	// ApprovedAt is when the execution was approved.
	ApprovedAt *time.Time `json:"approved_at,omitempty"`

	// RejectedBy is who rejected the execution.
	RejectedBy string `json:"rejected_by,omitempty"`

	// RejectedAt is when the execution was rejected.
	RejectedAt *time.Time `json:"rejected_at,omitempty"`

	// RejectionReason explains why the execution was rejected.
	RejectionReason string `json:"rejection_reason,omitempty"`

	// RetryCount is the number of retries attempted.
	RetryCount int `json:"retry_count"`

	// Metadata for additional execution data.
	Metadata map[string]string `json:"metadata,omitempty"`

	// CreatedAt is when this execution record was created.
	CreatedAt time.Time `json:"created_at"`
}

// AgentExecutionResult holds the result for a single agent.
type AgentExecutionResult struct {
	// AgentID identifies the agent.
	AgentID string `json:"agent_id"`

	// AgentName for convenience.
	AgentName string `json:"agent_name,omitempty"`

	// Status is the execution status for this agent.
	Status ExecutionStatus `json:"status"`

	// StartTime is when execution started on this agent.
	StartTime time.Time `json:"start_time"`

	// EndTime is when execution completed on this agent.
	EndTime time.Time `json:"end_time"`

	// Duration is the execution time on this agent.
	Duration time.Duration `json:"duration"`

	// Output is the command/state output.
	Output string `json:"output,omitempty"`

	// Error is the error message if failed.
	Error string `json:"error,omitempty"`

	// ExitCode is the exit code for command executions.
	ExitCode int `json:"exit_code,omitempty"`

	// Retries is the number of retries for this agent.
	Retries int `json:"retries"`

	// Changes made on this agent (for state executions).
	Changes map[string]interface{} `json:"changes,omitempty"`
}

// ScheduleFilter filters schedules for listing.
type ScheduleFilter struct {
	// Status filters by schedule status.
	Status []ScheduleStatus `json:"status,omitempty"`

	// Type filters by schedule type.
	Type []ScheduleType `json:"type,omitempty"`

	// Labels filters by labels (all must match).
	Labels map[string]string `json:"labels,omitempty"`

	// MaintenanceWindowID filters by maintenance window.
	MaintenanceWindowID string `json:"maintenance_window_id,omitempty"`

	// IncludeDisabled includes disabled schedules.
	IncludeDisabled bool `json:"include_disabled,omitempty"`

	// CreatedAfter filters by creation time.
	CreatedAfter *time.Time `json:"created_after,omitempty"`

	// CreatedBefore filters by creation time.
	CreatedBefore *time.Time `json:"created_before,omitempty"`

	// NameContains filters by name substring.
	NameContains string `json:"name_contains,omitempty"`

	// Limit limits the number of results.
	Limit int `json:"limit,omitempty"`

	// Offset for pagination.
	Offset int `json:"offset,omitempty"`
}

// ExecutionFilter filters executions for listing.
type ExecutionFilter struct {
	// ScheduleID filters by schedule.
	ScheduleID string `json:"schedule_id,omitempty"`

	// Status filters by execution status.
	Status []ExecutionStatus `json:"status,omitempty"`

	// TriggerType filters by trigger type.
	TriggerType []TriggerType `json:"trigger_type,omitempty"`

	// StartAfter filters by start time.
	StartAfter *time.Time `json:"start_after,omitempty"`

	// StartBefore filters by start time.
	StartBefore *time.Time `json:"start_before,omitempty"`

	// Limit limits the number of results.
	Limit int `json:"limit,omitempty"`

	// Offset for pagination.
	Offset int `json:"offset,omitempty"`
}

// ScheduleStats holds schedule statistics.
type ScheduleStats struct {
	// TotalSchedules is the total number of schedules.
	TotalSchedules int `json:"total_schedules"`

	// ActiveSchedules is the number of active schedules.
	ActiveSchedules int `json:"active_schedules"`

	// PausedSchedules is the number of paused schedules.
	PausedSchedules int `json:"paused_schedules"`

	// DisabledSchedules is the number of disabled schedules.
	DisabledSchedules int `json:"disabled_schedules"`

	// ByType shows count by schedule type.
	ByType map[ScheduleType]int `json:"by_type"`

	// ByStatus shows count by status.
	ByStatus map[ScheduleStatus]int `json:"by_status"`

	// TotalExecutions is the total execution count.
	TotalExecutions int64 `json:"total_executions"`

	// SuccessfulExecutions is the successful execution count.
	SuccessfulExecutions int64 `json:"successful_executions"`

	// FailedExecutions is the failed execution count.
	FailedExecutions int64 `json:"failed_executions"`

	// SuccessRate is the success rate percentage.
	SuccessRate float64 `json:"success_rate"`

	// UpcomingCount is the number of schedules due in the next hour.
	UpcomingCount int `json:"upcoming_count"`

	// AverageExecutionTime is the average execution duration.
	AverageExecutionTime time.Duration `json:"average_execution_time"`
}

// CommandPayload is the payload for command schedules.
type CommandPayload struct {
	// Command is the command to execute.
	Command string `json:"command"`

	// Args are command arguments.
	Args []string `json:"args,omitempty"`

	// Environment variables.
	Env map[string]string `json:"env,omitempty"`

	// WorkingDirectory for command execution.
	WorkingDirectory string `json:"working_directory,omitempty"`

	// Shell specifies the shell to use.
	Shell string `json:"shell,omitempty"`

	// RunAs specifies the user to run as.
	RunAs string `json:"run_as,omitempty"`
}

// StatePayload is the payload for state schedules.
type StatePayload struct {
	// StatePath is the path to the state file.
	StatePath string `json:"state_path,omitempty"`

	// StateContent is inline state content.
	StateContent string `json:"state_content,omitempty"`

	// Variables to pass to the state.
	Variables map[string]interface{} `json:"variables,omitempty"`

	// DryRun only checks without applying.
	DryRun bool `json:"dry_run,omitempty"`
}

// BlueprintPayload is the payload for blueprint schedules.
type BlueprintPayload struct {
	// BlueprintName is the blueprint to apply.
	BlueprintName string `json:"blueprint_name"`

	// BlueprintVersion is the specific version.
	BlueprintVersion string `json:"blueprint_version,omitempty"`

	// Variables to pass to the blueprint.
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// ReactorPayload is the payload for reactor schedules.
type ReactorPayload struct {
	// ReactorID is the reactor to trigger.
	ReactorID string `json:"reactor_id"`

	// EventData is the event data to pass.
	EventData map[string]interface{} `json:"event_data,omitempty"`
}

