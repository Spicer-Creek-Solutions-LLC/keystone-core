package schedule

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ManagerConfig holds configuration for the schedule manager.
type ManagerConfig struct {
	// MemberID is the cluster member ID for this instance.
	MemberID string

	// MaxExecutionHistory is the maximum number of executions to keep per schedule.
	MaxExecutionHistory int

	// DefaultTimeout is the default execution timeout.
	DefaultTimeout time.Duration

	// DefaultRetryPolicy is the default retry policy.
	DefaultRetryPolicy *RetryPolicy
}

// DefaultManagerConfig returns a default configuration.
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		MaxExecutionHistory: 100,
		DefaultTimeout:      time.Hour,
		DefaultRetryPolicy: &RetryPolicy{
			MaxRetries:        3,
			RetryDelay:        time.Minute,
			BackoffMultiplier: 2.0,
			MaxDelay:          time.Hour,
		},
	}
}

// Manager manages schedule CRUD operations and lifecycle.
type Manager struct {
	config     *ManagerConfig
	store      Store
	cronParser *CronParser
	listeners  []EventListener
	mu         sync.RWMutex
	closed     bool
}

// EventListener receives schedule events.
type EventListener func(event *Event)

// EventType represents schedule event types.
type EventType string

// EventCreated constants define the events.
const (
	EventCreated   EventType = "schedule.created"
	EventUpdated   EventType = "schedule.updated"
	EventDeleted   EventType = "schedule.deleted"
	EventPaused    EventType = "schedule.paused"
	EventResumed   EventType = "schedule.resumed"
	EventTriggered EventType = "schedule.triggered"
	EventCompleted EventType = "schedule.completed"
	EventFailed    EventType = "schedule.failed"
)

// NewManager creates a new schedule manager.
func NewManager(config *ManagerConfig, store Store) (*Manager, error) {
	if config == nil {
		config = DefaultManagerConfig()
	}
	if store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if config.MemberID == "" {
		return nil, fmt.Errorf("member ID is required")
	}

	return &Manager{
		config:     config,
		store:      store,
		cronParser: NewCronParser(),
		listeners:  make([]EventListener, 0),
	}, nil
}

// Create creates a new schedule.
func (m *Manager) Create(ctx context.Context, schedule *Schedule) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	if schedule == nil {
		return fmt.Errorf("schedule is required")
	}

	// Generate ID if not provided
	if schedule.ID == "" {
		schedule.ID = uuid.New().String()
	}

	// Validate
	if err := m.validate(schedule); err != nil {
		return err
	}

	// Set defaults
	now := time.Now().UTC()
	if schedule.Status == "" {
		schedule.Status = StatusActive
	}
	if schedule.Timeout == 0 {
		schedule.Timeout = m.config.DefaultTimeout
	}
	if schedule.RetryPolicy == nil && m.config.DefaultRetryPolicy != nil {
		schedule.RetryPolicy = m.config.DefaultRetryPolicy
	}

	schedule.CreatedAt = now
	schedule.UpdatedAt = now

	// Calculate next run time
	nextRun, err := CalculateNextRun(schedule, nil, m.cronParser)
	if err != nil {
		return fmt.Errorf("failed to calculate next run: %w", err)
	}
	schedule.NextRun = nextRun

	// Store
	if err := m.store.CreateSchedule(ctx, schedule); err != nil {
		return err
	}

	// Emit event
	m.emitEvent(&Event{
		Type:       string(EventCreated),
		ScheduleID: schedule.ID,
		Schedule:   schedule,
		Timestamp:  now,
		Actor:      schedule.CreatedBy,
	})

	return nil
}

// Get retrieves a schedule by ID.
func (m *Manager) Get(ctx context.Context, id string) (*Schedule, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	return m.store.GetSchedule(ctx, id)
}

// Update updates an existing schedule.
func (m *Manager) Update(ctx context.Context, schedule *Schedule) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	if schedule == nil {
		return fmt.Errorf("schedule is required")
	}
	if schedule.ID == "" {
		return fmt.Errorf("schedule ID is required")
	}

	// Validate
	if err := m.validate(schedule); err != nil {
		return err
	}

	// Get existing to check if cron/interval changed
	existing, err := m.store.GetSchedule(ctx, schedule.ID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	schedule.UpdatedAt = now

	// Recalculate next run if schedule pattern changed
	if schedule.Cron != existing.Cron || schedule.Interval != existing.Interval {
		nextRun, err := CalculateNextRun(schedule, schedule.LastRun, m.cronParser)
		if err != nil {
			return fmt.Errorf("failed to calculate next run: %w", err)
		}
		schedule.NextRun = nextRun
	}

	// Store
	if err := m.store.UpdateSchedule(ctx, schedule); err != nil {
		return err
	}

	// Emit event
	m.emitEvent(&Event{
		Type:       string(EventUpdated),
		ScheduleID: schedule.ID,
		Schedule:   schedule,
		Timestamp:  now,
		Actor:      schedule.UpdatedBy,
	})

	return nil
}

// Delete deletes a schedule.
func (m *Manager) Delete(ctx context.Context, id string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	// Get schedule first for event emission
	schedule, err := m.store.GetSchedule(ctx, id)
	if err != nil {
		return err
	}

	// Delete
	if err := m.store.DeleteSchedule(ctx, id); err != nil {
		return err
	}

	// Emit event
	m.emitEvent(&Event{
		Type:       string(EventDeleted),
		ScheduleID: id,
		Schedule:   schedule,
		Timestamp:  time.Now().UTC(),
	})

	return nil
}

// List lists schedules matching the filter.
func (m *Manager) List(ctx context.Context, filter *Filter) ([]*Schedule, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	return m.store.ListSchedules(ctx, filter)
}

// Pause pauses a schedule.
func (m *Manager) Pause(ctx context.Context, id, by string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	schedule, err := m.store.GetSchedule(ctx, id)
	if err != nil {
		return err
	}

	if schedule.Status == StatusDisabled {
		return ErrScheduleDisabled
	}

	schedule.Status = StatusPaused
	schedule.UpdatedAt = time.Now().UTC()
	schedule.UpdatedBy = by

	if err := m.store.UpdateSchedule(ctx, schedule); err != nil {
		return err
	}

	m.emitEvent(&Event{
		Type:       string(EventPaused),
		ScheduleID: id,
		Schedule:   schedule,
		Timestamp:  schedule.UpdatedAt,
		Actor:      by,
	})

	return nil
}

// Resume resumes a paused schedule.
func (m *Manager) Resume(ctx context.Context, id, by string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	schedule, err := m.store.GetSchedule(ctx, id)
	if err != nil {
		return err
	}

	if schedule.Status == StatusDisabled {
		return ErrScheduleDisabled
	}

	schedule.Status = StatusActive
	schedule.UpdatedAt = time.Now().UTC()
	schedule.UpdatedBy = by

	// Recalculate next run
	nextRun, err := CalculateNextRun(schedule, schedule.LastRun, m.cronParser)
	if err != nil {
		return fmt.Errorf("failed to calculate next run: %w", err)
	}
	schedule.NextRun = nextRun

	if err := m.store.UpdateSchedule(ctx, schedule); err != nil {
		return err
	}

	m.emitEvent(&Event{
		Type:       string(EventResumed),
		ScheduleID: id,
		Schedule:   schedule,
		Timestamp:  schedule.UpdatedAt,
		Actor:      by,
	})

	return nil
}

// Disable disables a schedule.
func (m *Manager) Disable(ctx context.Context, id, by string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	schedule, err := m.store.GetSchedule(ctx, id)
	if err != nil {
		return err
	}

	schedule.Status = StatusDisabled
	schedule.NextRun = nil // No more runs
	schedule.UpdatedAt = time.Now().UTC()
	schedule.UpdatedBy = by

	if err := m.store.UpdateSchedule(ctx, schedule); err != nil {
		return err
	}

	m.emitEvent(&Event{
		Type:       string(EventUpdated),
		ScheduleID: id,
		Schedule:   schedule,
		Timestamp:  schedule.UpdatedAt,
		Actor:      by,
		Message:    "schedule disabled",
	})

	return nil
}

// Enable enables a disabled schedule.
func (m *Manager) Enable(ctx context.Context, id, by string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	schedule, err := m.store.GetSchedule(ctx, id)
	if err != nil {
		return err
	}

	schedule.Status = StatusActive
	schedule.UpdatedAt = time.Now().UTC()
	schedule.UpdatedBy = by

	// Recalculate next run
	nextRun, err := CalculateNextRun(schedule, schedule.LastRun, m.cronParser)
	if err != nil {
		return fmt.Errorf("failed to calculate next run: %w", err)
	}
	schedule.NextRun = nextRun

	if err := m.store.UpdateSchedule(ctx, schedule); err != nil {
		return err
	}

	m.emitEvent(&Event{
		Type:       string(EventUpdated),
		ScheduleID: id,
		Schedule:   schedule,
		Timestamp:  schedule.UpdatedAt,
		Actor:      by,
		Message:    "schedule enabled",
	})

	return nil
}

// TriggerNow triggers a schedule for immediate execution.
func (m *Manager) TriggerNow(ctx context.Context, id, triggeredBy string) (*Execution, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	schedule, err := m.store.GetSchedule(ctx, id)
	if err != nil {
		return nil, err
	}

	if schedule.Status == StatusDisabled {
		return nil, ErrScheduleDisabled
	}

	// Create execution record
	now := time.Now().UTC()
	execution := &Execution{
		ID:            uuid.New().String(),
		ScheduleID:    id,
		ScheduleName:  schedule.Name,
		Status:        ExecutionStatusPending,
		TriggerType:   TriggerTypeManual,
		TriggeredBy:   triggeredBy,
		ScheduledTime: now,
		CreatedAt:     now,
	}

	// If approval required, keep pending
	if schedule.RequireApproval {
		execution.Status = ExecutionStatusPending
	} else {
		execution.Status = ExecutionStatusApproved
	}

	if err := m.store.CreateExecution(ctx, execution); err != nil {
		return nil, fmt.Errorf("failed to create execution: %w", err)
	}

	m.emitEvent(&Event{
		Type:        string(EventTriggered),
		ScheduleID:  id,
		Schedule:    schedule,
		ExecutionID: execution.ID,
		Timestamp:   now,
		Actor:       triggeredBy,
		Message:     "schedule triggered manually",
	})

	return execution, nil
}

// GetNextRun returns the next scheduled run time.
func (m *Manager) GetNextRun(ctx context.Context, id string) (*time.Time, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	schedule, err := m.store.GetSchedule(ctx, id)
	if err != nil {
		return nil, err
	}

	if schedule.Status != StatusActive {
		return nil, nil
	}

	return schedule.NextRun, nil
}

// GetStats returns schedule statistics.
func (m *Manager) GetStats(ctx context.Context) (*Stats, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	schedules, err := m.store.ListSchedules(ctx, nil)
	if err != nil {
		return nil, err
	}

	stats := &Stats{
		ByType:   make(map[Type]int),
		ByStatus: make(map[Status]int),
	}

	now := time.Now().UTC()
	nextHour := now.Add(time.Hour)

	for _, s := range schedules {
		stats.TotalSchedules++
		stats.ByType[s.Type]++
		stats.ByStatus[s.Status]++

		switch s.Status {
		case StatusActive:
			stats.ActiveSchedules++
		case StatusPaused:
			stats.PausedSchedules++
		case StatusDisabled:
			stats.DisabledSchedules++
		default:
			// ScheduleStatusExpired counted via ByStatus map
		}

		stats.TotalExecutions += s.RunCount
		stats.SuccessfulExecutions += s.SuccessCount
		stats.FailedExecutions += s.FailureCount

		// Check if upcoming in next hour
		if s.NextRun != nil && s.NextRun.Before(nextHour) && s.NextRun.After(now) {
			stats.UpcomingCount++
		}
	}

	if stats.TotalExecutions > 0 {
		stats.SuccessRate = float64(stats.SuccessfulExecutions) / float64(stats.TotalExecutions) * 100
	}

	return stats, nil
}

// ApproveExecution approves a pending execution.
func (m *Manager) ApproveExecution(ctx context.Context, executionID, approvedBy string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	execution, err := m.store.GetExecution(ctx, executionID)
	if err != nil {
		return err
	}

	if execution.Status != ExecutionStatusPending {
		return ErrExecutionNotPending
	}

	now := time.Now().UTC()
	execution.Status = ExecutionStatusApproved
	execution.ApprovedBy = approvedBy
	execution.ApprovedAt = &now

	return m.store.UpdateExecution(ctx, execution)
}

// RejectExecution rejects a pending execution.
func (m *Manager) RejectExecution(ctx context.Context, executionID, rejectedBy, reason string) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	execution, err := m.store.GetExecution(ctx, executionID)
	if err != nil {
		return err
	}

	if execution.Status != ExecutionStatusPending {
		return ErrExecutionNotPending
	}

	now := time.Now().UTC()
	execution.Status = ExecutionStatusRejected
	execution.RejectedBy = rejectedBy
	execution.RejectedAt = &now
	execution.RejectionReason = reason

	return m.store.UpdateExecution(ctx, execution)
}

// GetExecution retrieves an execution by ID.
func (m *Manager) GetExecution(ctx context.Context, id string) (*Execution, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	return m.store.GetExecution(ctx, id)
}

// ListExecutions lists executions matching the filter.
func (m *Manager) ListExecutions(ctx context.Context, filter *ExecutionFilter) ([]*Execution, error) {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return nil, ErrStoreClosed
	}
	m.mu.RUnlock()

	return m.store.ListExecutions(ctx, filter)
}

// RecordExecutionResult records the result of an execution.
func (m *Manager) RecordExecutionResult(ctx context.Context, execution *Execution) error {
	m.mu.RLock()
	if m.closed {
		m.mu.RUnlock()
		return ErrStoreClosed
	}
	m.mu.RUnlock()

	// Update execution record
	if err := m.store.UpdateExecution(ctx, execution); err != nil {
		return err
	}

	// Update schedule statistics
	schedule, err := m.store.GetSchedule(ctx, execution.ScheduleID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	schedule.LastRun = &now
	schedule.RunCount++

	switch execution.Status {
	case ExecutionStatusCompleted:
		schedule.SuccessCount++
	case ExecutionStatusFailed:
		schedule.FailureCount++
	default:
		// Other statuses don't update success/failure counts
	}

	// Calculate next run
	nextRun, err := CalculateNextRun(schedule, schedule.LastRun, m.cronParser)
	if err == nil {
		schedule.NextRun = nextRun
	}

	schedule.UpdatedAt = now

	if err := m.store.UpdateSchedule(ctx, schedule); err != nil {
		return err
	}

	// Emit completion event
	eventType := string(EventCompleted)
	if execution.Status == ExecutionStatusFailed {
		eventType = string(EventFailed)
	}

	m.emitEvent(&Event{
		Type:        eventType,
		ScheduleID:  execution.ScheduleID,
		Schedule:    schedule,
		ExecutionID: execution.ID,
		Timestamp:   now,
	})

	// Cleanup old executions
	if m.config.MaxExecutionHistory > 0 {
		_, _ = m.store.DeleteOldExecutions(ctx, execution.ScheduleID, m.config.MaxExecutionHistory)
	}

	return nil
}

// AddListener adds an event listener.
func (m *Manager) AddListener(listener EventListener) {
	m.mu.Lock()
	m.listeners = append(m.listeners, listener)
	m.mu.Unlock()
}

// Close closes the manager.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}
	m.closed = true

	return nil
}

// validate validates a schedule.
func (m *Manager) validate(schedule *Schedule) error {
	if schedule.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidSchedule)
	}

	if schedule.Type == "" {
		return fmt.Errorf("%w: type is required", ErrInvalidSchedule)
	}

	// Validate type
	switch schedule.Type {
	case TypeCommand, TypeState, TypeBlueprint, TypeReactor, TypeCustom:
		// Valid types
	default:
		return fmt.Errorf("%w: invalid type %q", ErrInvalidSchedule, schedule.Type)
	}

	// Must have either cron or interval
	if schedule.Cron == "" && schedule.Interval <= 0 {
		return fmt.Errorf("%w: either cron expression or interval is required", ErrInvalidSchedule)
	}

	// Validate cron if provided
	if schedule.Cron != "" {
		if err := m.cronParser.Validate(schedule.Cron); err != nil {
			return err
		}
	}

	// Validate target
	if schedule.Target == nil {
		return fmt.Errorf("%w: target is required", ErrInvalidSchedule)
	}

	// Target must have at least one selection criteria
	if !schedule.Target.All &&
		len(schedule.Target.AgentIDs) == 0 &&
		schedule.Target.Glob == "" &&
		len(schedule.Target.Tags) == 0 &&
		len(schedule.Target.Roles) == 0 {
		return fmt.Errorf("%w: target must specify agents, glob, tags, roles, or all", ErrInvalidSchedule)
	}

	// Validate time window if provided
	if schedule.Window != nil {
		if schedule.Window.StartTime != "" {
			if _, err := time.Parse("15:04", schedule.Window.StartTime); err != nil {
				return fmt.Errorf("%w: invalid start_time format (expected HH:MM)", ErrInvalidSchedule)
			}
		}
		if schedule.Window.EndTime != "" {
			if _, err := time.Parse("15:04", schedule.Window.EndTime); err != nil {
				return fmt.Errorf("%w: invalid end_time format (expected HH:MM)", ErrInvalidSchedule)
			}
		}
	}

	// Validate timezone if provided
	if schedule.Timezone != "" {
		if _, err := time.LoadLocation(schedule.Timezone); err != nil {
			return fmt.Errorf("%w: invalid timezone %q", ErrInvalidSchedule, schedule.Timezone)
		}
	}

	return nil
}

// emitEvent emits an event to all listeners.
func (m *Manager) emitEvent(event *Event) {
	m.mu.RLock()
	listeners := m.listeners
	m.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}
