// Package sync provides sync window scheduling for periodic-connectivity
// air-gapped environments. It uses cron-based scheduling, bandwidth limiting,
// and state machine-driven lifecycle management for sync operations.
package sync

import (
	"errors"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/schedule"
)

// State represents the lifecycle state of a sync window.
type State string

// Sync window states.
const (
	StateIdle      State = "idle"
	StateScheduled State = "scheduled"
	StateRunning   State = "running"
	StatePaused    State = "paused"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

// Event triggers state transitions in a sync window.
type Event string

// Sync window events.
const (
	EventSchedule Event = "schedule"
	EventStart    Event = "start"
	EventPause    Event = "pause"
	EventResume   Event = "resume"
	EventComplete Event = "complete"
	EventFail     Event = "fail"
	EventCancel   Event = "cancel"
	EventReset    Event = "reset"
)

// OperationType defines the kind of sync operation.
type OperationType string

// Supported operation types.
const (
	OpPullModules    OperationType = "pull_modules"
	OpPullBlueprints OperationType = "pull_blueprints"
	OpPushAuditLogs  OperationType = "push_audit_logs"
	OpPushMetrics    OperationType = "push_metrics"
	OpFullSync       OperationType = "full_sync"
)

// WindowConfig defines a scheduled sync window.
type WindowConfig struct {
	Name           string            `json:"name" yaml:"name"`
	CronSchedule   string            `json:"cron_schedule" yaml:"cron_schedule"`
	Duration       time.Duration     `json:"duration" yaml:"duration"`
	Operations     []OperationConfig `json:"operations" yaml:"operations"`
	BandwidthLimit int64             `json:"bandwidth_limit" yaml:"bandwidth_limit"`
	Timezone       string            `json:"timezone,omitempty" yaml:"timezone,omitempty"`
	Enabled        bool              `json:"enabled" yaml:"enabled"`
}

// OperationConfig defines a single sync operation within a window.
type OperationConfig struct {
	Type     OperationType `json:"type" yaml:"type"`
	Priority int           `json:"priority" yaml:"priority"`
	Endpoint string        `json:"endpoint" yaml:"endpoint"`
}

// Progress tracks resumable progress for a running window.
type Progress struct {
	TotalItems     int    `json:"total_items"`
	CompletedItems int    `json:"completed_items"`
	BytesSent      int64  `json:"bytes_sent"`
	BytesReceived  int64  `json:"bytes_received"`
	LastPosition   string `json:"last_position,omitempty"`
}

// Record stores the result of a completed sync window execution.
type Record struct {
	WindowName string        `json:"window_name"`
	StartedAt  time.Time     `json:"started_at"`
	EndedAt    time.Time     `json:"ended_at"`
	Duration   time.Duration `json:"duration"`
	State      State         `json:"state"`
	Operations int           `json:"operations"`
	Error      string        `json:"error,omitempty"`
}

// Errors returned by sync operations.
var (
	ErrWindowNotFound  = errors.New("sync window not found")
	ErrWindowExists    = errors.New("sync window already exists")
	ErrSchedulerClosed = errors.New("scheduler is closed")
	ErrInvalidConfig   = errors.New("invalid window configuration")
)

// Validate checks that the WindowConfig is well-formed.
func (c *WindowConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidConfig)
	}
	if c.CronSchedule == "" {
		return fmt.Errorf("%w: cron_schedule is required", ErrInvalidConfig)
	}
	if c.Duration <= 0 {
		return fmt.Errorf("%w: duration must be positive", ErrInvalidConfig)
	}
	if len(c.Operations) == 0 {
		return fmt.Errorf("%w: at least one operation is required", ErrInvalidConfig)
	}

	parser := schedule.NewCronParser()
	if err := parser.Validate(c.CronSchedule); err != nil {
		return fmt.Errorf("%w: invalid cron expression: %w", ErrInvalidConfig, err)
	}

	if c.Timezone != "" {
		if _, err := time.LoadLocation(c.Timezone); err != nil {
			return fmt.Errorf("%w: invalid timezone %q: %w", ErrInvalidConfig, c.Timezone, err)
		}
	}

	return nil
}

// WindowStatus reports the current state of a sync window.
type WindowStatus struct {
	Name     string     `json:"name"`
	State    State      `json:"state"`
	NextRun  *time.Time `json:"next_run,omitempty"`
	Progress *Progress  `json:"progress,omitempty"`
}
