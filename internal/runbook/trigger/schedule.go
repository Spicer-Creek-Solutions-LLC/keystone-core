package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/internal/runbook"
	"github.com/shawnbutts/keystone-core/internal/schedule"
)

// ScheduleTrigger represents a schedule-based runbook trigger.
type ScheduleTrigger struct {
	// ID is the unique trigger identifier.
	ID string `yaml:"id" json:"id"`

	// Name is a human-readable name.
	Name string `yaml:"name" json:"name"`

	// Description explains what this trigger does.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// RunbookRef references the runbook to execute.
	RunbookRef RunbookRef `yaml:"runbook" json:"runbook"`

	// Cron is the cron expression (5 or 6 field).
	Cron string `yaml:"cron,omitempty" json:"cron,omitempty"`

	// Interval is the execution interval (alternative to cron).
	Interval time.Duration `yaml:"interval,omitempty" json:"interval,omitempty"`

	// Timezone for cron evaluation.
	Timezone string `yaml:"timezone,omitempty" json:"timezone,omitempty"`

	// Window restricts execution to specific times.
	Window *schedule.TimeWindow `yaml:"window,omitempty" json:"window,omitempty"`

	// Inputs are the static inputs for the runbook.
	Inputs map[string]interface{} `yaml:"inputs,omitempty" json:"inputs,omitempty"`

	// Enabled indicates if the trigger is active.
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Tags for categorization.
	Tags map[string]string `yaml:"tags,omitempty" json:"tags,omitempty"`

	// CreatedAt is when the trigger was created.
	CreatedAt time.Time `yaml:"createdAt,omitempty" json:"created_at,omitempty"`

	// UpdatedAt is when the trigger was last updated.
	UpdatedAt time.Time `yaml:"updatedAt,omitempty" json:"updated_at,omitempty"`
}

// Validate validates the schedule trigger.
func (t *ScheduleTrigger) Validate() error {
	if t.ID == "" {
		return &ValidationError{Field: "id", Message: "id is required"}
	}
	if t.Name == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	if t.RunbookRef.Name == "" {
		return &ValidationError{Field: "runbook.name", Message: "runbook name is required"}
	}
	if t.Cron == "" && t.Interval == 0 {
		return &ValidationError{Field: "cron/interval", Message: "either cron or interval is required"}
	}
	if t.Cron != "" && t.Interval != 0 {
		return &ValidationError{Field: "cron/interval", Message: "specify either cron or interval, not both"}
	}
	return nil
}

// ToSchedule converts a ScheduleTrigger to a schedule.Schedule.
func (t *ScheduleTrigger) ToSchedule() (*schedule.Schedule, error) {
	payload := &RunbookPayload{
		RunbookName:    t.RunbookRef.Name,
		RunbookVersion: t.RunbookRef.Version,
		Inputs:         t.Inputs,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	status := schedule.StatusActive
	if !t.Enabled {
		status = schedule.StatusDisabled
	}

	return &schedule.Schedule{
		ID:          t.ID,
		Name:        t.Name,
		Description: t.Description,
		Type:        ScheduleTypeRunbook,
		Cron:        t.Cron,
		Interval:    t.Interval,
		Timezone:    t.Timezone,
		Window:      t.Window,
		Payload:     payloadJSON,
		Status:      status,
		Labels:      t.Tags,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}, nil
}

// ScheduleTypeRunbook is the schedule type for runbook execution.
const ScheduleTypeRunbook schedule.Type = "runbook"

// RunbookPayload is the payload for runbook schedule execution.
type RunbookPayload struct {
	RunbookName    string                 `json:"runbook_name"`
	RunbookVersion string                 `json:"runbook_version,omitempty"`
	Inputs         map[string]interface{} `json:"inputs,omitempty"`
}

// ScheduleHandler handles runbook execution from schedules.
type ScheduleHandler struct {
	repository RunbookRepository
	executor   RunbookExecutor
	publisher  events.EventPublisher

	mu          sync.Mutex
	executions  map[string]*ScheduleExecution
	lastResults map[string]*ScheduleExecutionResult
}

// ScheduleExecution tracks a schedule-triggered execution.
type ScheduleExecution struct {
	ID          string     `json:"id"`
	TriggerID   string     `json:"trigger_id"`
	ScheduleID  string     `json:"schedule_id"`
	ExecutionID string     `json:"execution_id"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
}

// ScheduleExecutionResult is the result of a schedule execution.
type ScheduleExecutionResult struct {
	Success bool
	Error   error
	Output  map[string]interface{}
}

// NewScheduleHandler creates a new schedule handler.
func NewScheduleHandler(repo RunbookRepository, executor RunbookExecutor, publisher events.EventPublisher) *ScheduleHandler {
	return &ScheduleHandler{
		repository:  repo,
		executor:    executor,
		publisher:   publisher,
		executions:  make(map[string]*ScheduleExecution),
		lastResults: make(map[string]*ScheduleExecutionResult),
	}
}

// Type returns the schedule type this handler processes.
func (h *ScheduleHandler) Type() schedule.Type {
	return ScheduleTypeRunbook
}

// Validate validates the schedule payload.
func (h *ScheduleHandler) Validate(s *schedule.Schedule) error {
	var payload RunbookPayload
	if err := json.Unmarshal(s.Payload, &payload); err != nil {
		return fmt.Errorf("invalid runbook payload: %w", err)
	}

	if payload.RunbookName == "" {
		return &ValidationError{Field: "payload.runbook_name", Message: "runbook name is required"}
	}

	// Verify runbook exists
	_, err := h.repository.GetRunbook(payload.RunbookName, payload.RunbookVersion)
	if err != nil {
		return fmt.Errorf("runbook not found: %w", err)
	}

	return nil
}

// Execute runs the scheduled runbook.
func (h *ScheduleHandler) Execute(ctx context.Context, s *schedule.Schedule, exec *schedule.Execution) error {
	var payload RunbookPayload
	if err := json.Unmarshal(s.Payload, &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	// Track execution
	schedExec := &ScheduleExecution{
		ID:         uuid.New().String(),
		TriggerID:  s.ID,
		ScheduleID: s.ID,
		StartedAt:  time.Now(),
		Status:     "running",
	}

	h.mu.Lock()
	h.executions[schedExec.ID] = schedExec
	h.mu.Unlock()

	// Publish start event
	if h.publisher != nil {
		_ = h.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.schedule.started"),
			Source: "/runbook/schedule/" + s.ID,
			Time:   time.Now(),
			Data: map[string]interface{}{
				"schedule_id":   s.ID,
				"schedule_name": s.Name,
				"runbook_name":  payload.RunbookName,
			},
		})
	}

	// Get runbook
	rb, err := h.repository.GetRunbook(payload.RunbookName, payload.RunbookVersion)
	if err != nil {
		h.recordError(schedExec, err)
		return fmt.Errorf("get runbook: %w", err)
	}

	// Build inputs
	inputs := make(map[string]interface{})
	for k, v := range payload.Inputs {
		inputs[k] = v
	}

	// Add schedule metadata
	inputs["__schedule_id"] = s.ID
	inputs["__schedule_name"] = s.Name
	inputs["__execution_id"] = exec.ID
	inputs["__trigger_type"] = "schedule"

	// Execute runbook
	rbExec, err := h.executor.Execute(rb, inputs)

	// Record result
	now := time.Now()
	schedExec.CompletedAt = &now

	if err != nil {
		h.recordError(schedExec, err)
		return err
	}

	if rbExec.State == runbook.ExecutionStateFailed {
		err = fmt.Errorf("runbook execution failed: %s", rbExec.Error)
		h.recordError(schedExec, err)
		return err
	}

	schedExec.ExecutionID = rbExec.ID
	schedExec.Status = "completed"

	h.mu.Lock()
	h.lastResults[s.ID] = &ScheduleExecutionResult{
		Success: true,
	}
	h.mu.Unlock()

	// Publish completion event
	if h.publisher != nil {
		_ = h.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.schedule.completed"),
			Source: "/runbook/schedule/" + s.ID,
			Time:   time.Now(),
			Data: map[string]interface{}{
				"schedule_id":   s.ID,
				"schedule_name": s.Name,
				"runbook_name":  payload.RunbookName,
				"execution_id":  rbExec.ID,
				"success":       true,
			},
		})
	}

	return nil
}

// recordError records an execution error.
func (h *ScheduleHandler) recordError(exec *ScheduleExecution, err error) {
	exec.Status = "failed"
	exec.Error = err.Error()

	h.mu.Lock()
	h.lastResults[exec.ScheduleID] = &ScheduleExecutionResult{
		Success: false,
		Error:   err,
	}
	h.mu.Unlock()

	// Publish failure event
	if h.publisher != nil {
		_ = h.publisher.Publish(&events.Event{
			ID:     uuid.New().String(),
			Type:   events.EventType("runbook.schedule.failed"),
			Source: "/runbook/schedule/" + exec.ScheduleID,
			Time:   time.Now(),
			Data: map[string]interface{}{
				"schedule_id": exec.ScheduleID,
				"error":       err.Error(),
			},
		})
	}
}

// GetLastResult returns the last execution result for a schedule.
func (h *ScheduleHandler) GetLastResult(scheduleID string) (*ScheduleExecutionResult, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	result, ok := h.lastResults[scheduleID]
	return result, ok
}

// ScheduleTriggerManager manages schedule-based triggers.
type ScheduleTriggerManager struct {
	mu       sync.RWMutex
	triggers map[string]*ScheduleTrigger

	scheduleManager *schedule.Manager
	scheduleHandler *ScheduleHandler
}

// NewScheduleTriggerManager creates a new schedule trigger manager.
func NewScheduleTriggerManager(
	scheduleManager *schedule.Manager,
	repository RunbookRepository,
	executor RunbookExecutor,
	publisher events.EventPublisher,
) *ScheduleTriggerManager {
	return &ScheduleTriggerManager{
		triggers:        make(map[string]*ScheduleTrigger),
		scheduleManager: scheduleManager,
		scheduleHandler: NewScheduleHandler(repository, executor, publisher),
	}
}

// Register adds a schedule trigger.
func (m *ScheduleTriggerManager) Register(trigger *ScheduleTrigger) error {
	if err := trigger.Validate(); err != nil {
		return fmt.Errorf("invalid trigger: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.triggers[trigger.ID]; exists {
		return fmt.Errorf("trigger %s already registered", trigger.ID)
	}

	// Convert to schedule and register with manager
	sched, err := trigger.ToSchedule()
	if err != nil {
		return fmt.Errorf("convert to schedule: %w", err)
	}

	if m.scheduleManager != nil {
		if err := m.scheduleManager.Create(context.Background(), sched); err != nil {
			return fmt.Errorf("create schedule: %w", err)
		}
	}

	now := time.Now()
	if trigger.CreatedAt.IsZero() {
		trigger.CreatedAt = now
	}
	trigger.UpdatedAt = now

	m.triggers[trigger.ID] = trigger
	return nil
}

// Unregister removes a schedule trigger.
func (m *ScheduleTriggerManager) Unregister(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.triggers[id]; !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	// Remove from schedule manager
	if m.scheduleManager != nil {
		if err := m.scheduleManager.Delete(context.Background(), id); err != nil {
			// Log but continue
			_ = err
		}
	}

	delete(m.triggers, id)
	return nil
}

// Get retrieves a schedule trigger by ID.
func (m *ScheduleTriggerManager) Get(id string) (*ScheduleTrigger, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	trigger, ok := m.triggers[id]
	return trigger, ok
}

// List returns all schedule triggers.
func (m *ScheduleTriggerManager) List() []*ScheduleTrigger {
	m.mu.RLock()
	defer m.mu.RUnlock()

	triggers := make([]*ScheduleTrigger, 0, len(m.triggers))
	for _, t := range m.triggers {
		triggers = append(triggers, t)
	}
	return triggers
}

// Enable enables a schedule trigger.
func (m *ScheduleTriggerManager) Enable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	trigger, exists := m.triggers[id]
	if !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	if !trigger.Enabled {
		trigger.Enabled = true
		trigger.UpdatedAt = time.Now()

		if m.scheduleManager != nil {
			_ = m.scheduleManager.Enable(context.Background(), id, "system")
		}
	}

	return nil
}

// Disable disables a schedule trigger.
func (m *ScheduleTriggerManager) Disable(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	trigger, exists := m.triggers[id]
	if !exists {
		return fmt.Errorf("trigger %s not found", id)
	}

	if trigger.Enabled {
		trigger.Enabled = false
		trigger.UpdatedAt = time.Now()

		if m.scheduleManager != nil {
			_ = m.scheduleManager.Disable(context.Background(), id, "system")
		}
	}

	return nil
}

// GetHandler returns the schedule handler for registration with the executor.
func (m *ScheduleTriggerManager) GetHandler() *ScheduleHandler {
	return m.scheduleHandler
}
