// Package trigger provides event-driven triggering of runbooks.
package trigger

import (
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// Trigger defines an event-driven runbook trigger.
type Trigger struct {
	// ID is the unique trigger identifier.
	ID string `yaml:"id" json:"id"`

	// Name is a human-readable name.
	Name string `yaml:"name" json:"name"`

	// Description explains what this trigger does.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// RunbookRef references the runbook to execute.
	RunbookRef RunbookRef `yaml:"runbook" json:"runbook"`

	// Filter specifies which events trigger this runbook.
	Filter string `yaml:"filter" json:"filter"`

	// Conditions provides additional trigger control.
	Conditions *TriggerConditions `yaml:"conditions,omitempty" json:"conditions,omitempty"`

	// InputMappings maps event data to runbook inputs.
	InputMappings map[string]string `yaml:"inputMappings,omitempty" json:"input_mappings,omitempty"`

	// Enabled indicates if the trigger is active.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Priority determines execution order (higher = first).
	Priority int `yaml:"priority,omitempty" json:"priority,omitempty"`

	// Tags for categorization.
	Tags map[string]string `yaml:"tags,omitempty" json:"tags,omitempty"`

	// CreatedAt is when the trigger was created.
	CreatedAt time.Time `yaml:"createdAt,omitempty" json:"created_at,omitempty"`

	// UpdatedAt is when the trigger was last updated.
	UpdatedAt time.Time `yaml:"updatedAt,omitempty" json:"updated_at,omitempty"`
}

// RunbookRef references a runbook to execute.
type RunbookRef struct {
	// Name is the runbook name.
	Name string `yaml:"name" json:"name"`

	// Version is the optional runbook version (latest if not specified).
	Version string `yaml:"version,omitempty" json:"version,omitempty"`
}

// TriggerConditions provides advanced trigger control.
type TriggerConditions struct {
	// OnlyIf specifies an additional condition that must be true.
	OnlyIf string `yaml:"onlyIf,omitempty" json:"only_if,omitempty"`

	// Unless specifies a condition that prevents execution.
	Unless string `yaml:"unless,omitempty" json:"unless,omitempty"`

	// Throttle is the minimum time between trigger activations.
	Throttle time.Duration `yaml:"throttle,omitempty" json:"throttle,omitempty"`

	// Debounce waits for a quiet period before triggering.
	Debounce time.Duration `yaml:"debounce,omitempty" json:"debounce,omitempty"`

	// MaxConcurrent limits concurrent executions (0 = unlimited).
	MaxConcurrent int `yaml:"maxConcurrent,omitempty" json:"max_concurrent,omitempty"`

	// Deduplication configures deduplication behavior.
	Deduplication *DeduplicationConfig `yaml:"deduplication,omitempty" json:"deduplication,omitempty"`

	// RateLimit configures rate limiting.
	RateLimit *RateLimitConfig `yaml:"rateLimit,omitempty" json:"rate_limit,omitempty"`

	// TimeWindow restricts trigger to specific times.
	TimeWindow *TimeWindowConfig `yaml:"timeWindow,omitempty" json:"time_window,omitempty"`
}

// DeduplicationConfig configures event deduplication.
type DeduplicationConfig struct {
	// Enabled enables deduplication.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Key is a template for generating dedup keys from events.
	// Example: "{{ .Type }}-{{ .Tags.host }}"
	Key string `yaml:"key" json:"key"`

	// Window is the deduplication window.
	Window time.Duration `yaml:"window" json:"window"`
}

// RateLimitConfig configures rate limiting.
type RateLimitConfig struct {
	// MaxExecutions is the maximum executions within the window.
	MaxExecutions int `yaml:"maxExecutions" json:"max_executions"`

	// Window is the rate limit window.
	Window time.Duration `yaml:"window" json:"window"`
}

// TimeWindowConfig restricts trigger activation to specific times.
type TimeWindowConfig struct {
	// Start is the start time in HH:MM format (24-hour).
	Start string `yaml:"start" json:"start"`

	// End is the end time in HH:MM format (24-hour).
	End string `yaml:"end" json:"end"`

	// Days are the days of week when trigger is active (0=Sunday).
	Days []int `yaml:"days,omitempty" json:"days,omitempty"`

	// Timezone is the timezone for time calculations.
	Timezone string `yaml:"timezone,omitempty" json:"timezone,omitempty"`
}

// TriggerExecution records a trigger activation.
type TriggerExecution struct {
	// ID is the unique execution identifier.
	ID string `json:"id"`

	// TriggerID is the trigger that was activated.
	TriggerID string `json:"trigger_id"`

	// EventID is the event that triggered execution.
	EventID string `json:"event_id"`

	// ExecutionID is the runbook execution ID.
	ExecutionID string `json:"execution_id"`

	// StartedAt is when the trigger activated.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when execution finished.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Status is the execution status.
	Status TriggerExecutionStatus `json:"status"`

	// Error contains any error message.
	Error string `json:"error,omitempty"`
}

// TriggerExecutionStatus represents trigger execution status.
type TriggerExecutionStatus string

const (
	// TriggerStatusPending indicates the trigger is waiting to execute.
	TriggerStatusPending TriggerExecutionStatus = "pending"

	// TriggerStatusRunning indicates the runbook is executing.
	TriggerStatusRunning TriggerExecutionStatus = "running"

	// TriggerStatusCompleted indicates successful completion.
	TriggerStatusCompleted TriggerExecutionStatus = "completed"

	// TriggerStatusFailed indicates execution failed.
	TriggerStatusFailed TriggerExecutionStatus = "failed"

	// TriggerStatusSkipped indicates the trigger was skipped (dedup, rate limit, etc.).
	TriggerStatusSkipped TriggerExecutionStatus = "skipped"

	// TriggerStatusThrottled indicates the trigger was throttled.
	TriggerStatusThrottled TriggerExecutionStatus = "throttled"
)

// TriggerMetrics provides trigger statistics.
type TriggerMetrics struct {
	// TotalTriggers is the number of registered triggers.
	TotalTriggers int `json:"total_triggers"`

	// EnabledTriggers is the number of enabled triggers.
	EnabledTriggers int `json:"enabled_triggers"`

	// TotalActivations is the total number of trigger activations.
	TotalActivations int64 `json:"total_activations"`

	// SuccessfulExecutions is the number of successful runbook executions.
	SuccessfulExecutions int64 `json:"successful_executions"`

	// FailedExecutions is the number of failed runbook executions.
	FailedExecutions int64 `json:"failed_executions"`

	// SkippedExecutions is the number of skipped executions (dedup, rate limit).
	SkippedExecutions int64 `json:"skipped_executions"`

	// AverageLatencyMs is the average trigger-to-execution latency.
	AverageLatencyMs float64 `json:"average_latency_ms"`

	// ByTrigger has per-trigger metrics.
	ByTrigger map[string]*TriggerStats `json:"by_trigger,omitempty"`
}

// TriggerStats provides statistics for a single trigger.
type TriggerStats struct {
	TriggerID    string    `json:"trigger_id"`
	Activations  int64     `json:"activations"`
	Successes    int64     `json:"successes"`
	Failures     int64     `json:"failures"`
	Skipped      int64     `json:"skipped"`
	LastActivation time.Time `json:"last_activation,omitempty"`
}

// RunbookRepository provides access to runbook definitions.
type RunbookRepository interface {
	// GetRunbook retrieves a runbook by name and optional version.
	GetRunbook(name, version string) (*runbook.Runbook, error)

	// ListRunbooks lists available runbooks.
	ListRunbooks() ([]*runbook.Runbook, error)
}

// RunbookExecutor executes runbooks.
type RunbookExecutor interface {
	// Execute runs a runbook with the given inputs.
	Execute(rb *runbook.Runbook, inputs map[string]interface{}) (*runbook.Execution, error)
}

// EventMatcher matches events against filter expressions.
type EventMatcher interface {
	// Matches returns true if the event matches the filter.
	Matches(event *events.Event) bool
}

// triggerState tracks runtime state for a trigger.
type triggerState struct {
	mu             sync.Mutex
	lastActivation time.Time
	activations    int64
	successes      int64
	failures       int64
	skipped        int64
	activeCount    int
	debounceTimer  *time.Timer
	pendingEvent   *events.Event
}

// Validate validates the trigger configuration.
func (t *Trigger) Validate() error {
	if t.ID == "" {
		return &ValidationError{Field: "id", Message: "id is required"}
	}
	if t.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if t.RunbookRef.Name == "" {
		return &ValidationError{Field: "runbook.name", Message: "runbook name is required"}
	}
	if t.Filter == "" {
		return &ValidationError{Field: "filter", Message: "filter is required"}
	}

	// Validate conditions
	if t.Conditions != nil {
		if t.Conditions.Throttle < 0 {
			return &ValidationError{Field: "conditions.throttle", Message: "throttle must be non-negative"}
		}
		if t.Conditions.Debounce < 0 {
			return &ValidationError{Field: "conditions.debounce", Message: "debounce must be non-negative"}
		}
		if t.Conditions.MaxConcurrent < 0 {
			return &ValidationError{Field: "conditions.maxConcurrent", Message: "maxConcurrent must be non-negative"}
		}

		if t.Conditions.Deduplication != nil {
			if t.Conditions.Deduplication.Enabled && t.Conditions.Deduplication.Key == "" {
				return &ValidationError{Field: "conditions.deduplication.key", Message: "deduplication key is required when enabled"}
			}
			if t.Conditions.Deduplication.Window < 0 {
				return &ValidationError{Field: "conditions.deduplication.window", Message: "deduplication window must be non-negative"}
			}
		}

		if t.Conditions.RateLimit != nil {
			if t.Conditions.RateLimit.MaxExecutions <= 0 {
				return &ValidationError{Field: "conditions.rateLimit.maxExecutions", Message: "maxExecutions must be positive"}
			}
			if t.Conditions.RateLimit.Window <= 0 {
				return &ValidationError{Field: "conditions.rateLimit.window", Message: "window must be positive"}
			}
		}
	}

	return nil
}

// ValidationError represents a trigger validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
