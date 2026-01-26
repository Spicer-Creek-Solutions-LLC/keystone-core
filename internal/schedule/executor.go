package schedule

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// ExecutorConfig holds configuration for the schedule executor.
type ExecutorConfig struct {
	// MemberID is the cluster member ID.
	MemberID string

	// CheckInterval is how often to check for due schedules.
	CheckInterval time.Duration

	// LockTimeout is how long to wait for lock acquisition.
	LockTimeout time.Duration

	// MaxConcurrentExecutions limits concurrent schedule executions.
	MaxConcurrentExecutions int

	// ExecutionTimeout is the default timeout for schedule execution.
	ExecutionTimeout time.Duration

	// CleanupInterval is how often to cleanup old execution records.
	CleanupInterval time.Duration

	// MaintenanceCheckInterval is how often to check maintenance windows.
	MaintenanceCheckInterval time.Duration
}

// DefaultExecutorConfig returns default executor configuration.
func DefaultExecutorConfig() *ExecutorConfig {
	return &ExecutorConfig{
		CheckInterval:            time.Minute,
		LockTimeout:              10 * time.Second,
		MaxConcurrentExecutions:  10,
		ExecutionTimeout:         time.Hour,
		CleanupInterval:          time.Hour,
		MaintenanceCheckInterval: time.Minute,
	}
}

// Handler executes a specific schedule type.
type Handler interface {
	// Type returns the schedule type this handler handles.
	Type() ScheduleType

	// Execute executes the schedule.
	Execute(ctx context.Context, schedule *Schedule, execution *ScheduleExecution) error

	// Validate validates the schedule payload.
	Validate(schedule *Schedule) error
}

// Executor executes scheduled operations.
type Executor struct {
	config             *ExecutorConfig
	store              Store
	scheduleManager    *ScheduleManager
	maintenanceManager *MaintenanceWindowManager
	cronParser         *CronParser
	handlers           map[ScheduleType]Handler
	activeExecutions   map[string]*ScheduleExecution
	listeners          []ExecutorEventListener
	mu                 sync.RWMutex
	stopChan           chan struct{}
	doneChan           chan struct{}
	started            bool
}

// ExecutorEventListener receives executor events.
type ExecutorEventListener func(event *ExecutorEvent)

// ExecutorEvent represents an executor event.
type ExecutorEvent struct {
	Type        string         `json:"type"`
	ScheduleID  string         `json:"schedule_id"`
	ExecutionID string         `json:"execution_id"`
	Timestamp   time.Time      `json:"timestamp"`
	Message     string         `json:"message,omitempty"`
	Error       string         `json:"error,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
}

// NewExecutor creates a new schedule executor.
func NewExecutor(
	config *ExecutorConfig,
	store Store,
	scheduleManager *ScheduleManager,
	maintenanceManager *MaintenanceWindowManager,
) (*Executor, error) {
	if config == nil {
		config = DefaultExecutorConfig()
	}
	if store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if scheduleManager == nil {
		return nil, fmt.Errorf("schedule manager is required")
	}
	if config.MemberID == "" {
		return nil, fmt.Errorf("member ID is required")
	}

	return &Executor{
		config:             config,
		store:              store,
		scheduleManager:    scheduleManager,
		maintenanceManager: maintenanceManager,
		cronParser:         NewCronParser(),
		handlers:           make(map[ScheduleType]Handler),
		activeExecutions:   make(map[string]*ScheduleExecution),
		listeners:          make([]ExecutorEventListener, 0),
		stopChan:           make(chan struct{}),
		doneChan:           make(chan struct{}),
	}, nil
}

// RegisterHandler registers a handler for a schedule type.
func (e *Executor) RegisterHandler(handler Handler) error {
	if handler == nil {
		return fmt.Errorf("handler is required")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	scheduleType := handler.Type()
	if _, exists := e.handlers[scheduleType]; exists {
		return fmt.Errorf("handler for type %s already registered", scheduleType)
	}

	e.handlers[scheduleType] = handler
	return nil
}

// UnregisterHandler unregisters a handler.
func (e *Executor) UnregisterHandler(scheduleType ScheduleType) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.handlers, scheduleType)
}

// Start starts the executor.
func (e *Executor) Start(ctx context.Context) error {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return fmt.Errorf("executor already started")
	}
	e.started = true
	e.mu.Unlock()

	// Start the main loop
	go e.run(ctx)

	// Start cleanup goroutine
	go e.cleanupLoop(ctx)

	// Start maintenance window checker
	if e.maintenanceManager != nil {
		go e.maintenanceLoop(ctx)
	}

	return nil
}

// Stop stops the executor.
func (e *Executor) Stop(ctx context.Context) error {
	e.mu.Lock()
	if !e.started {
		e.mu.Unlock()
		return nil
	}
	e.started = false
	e.mu.Unlock()

	close(e.stopChan)

	// Wait for completion or timeout
	wait.ForContextOrSignal(ctx, e.doneChan, 30*time.Second)

	return nil
}

// run is the main executor loop.
func (e *Executor) run(ctx context.Context) {
	defer func() {
		select {
		case e.doneChan <- struct{}{}:
		default:
		}
	}()

	// Initial check
	e.checkSchedules(ctx)

	ticker := time.NewTicker(e.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopChan:
			return
		case <-ticker.C:
			e.checkSchedules(ctx)
		}
	}
}

// checkSchedules checks for due schedules and executes them.
func (e *Executor) checkSchedules(ctx context.Context) {
	// Get all active schedules
	filter := &ScheduleFilter{
		Status: []ScheduleStatus{ScheduleStatusActive},
	}

	schedules, err := e.scheduleManager.List(ctx, filter)
	if err != nil {
		log.Printf("[ERROR] Failed to list schedules: %v", err)
		return
	}

	now := time.Now().UTC()

	for _, schedule := range schedules {
		// Check if schedule is due
		if schedule.NextRun == nil || schedule.NextRun.After(now) {
			continue
		}

		// Check concurrent execution limit
		e.mu.RLock()
		activeCount := len(e.activeExecutions)
		e.mu.RUnlock()

		if activeCount >= e.config.MaxConcurrentExecutions {
			log.Printf("[WARN] Max concurrent executions reached (%d), skipping schedule %s",
				e.config.MaxConcurrentExecutions, schedule.ID)
			continue
		}

		// Check if in maintenance window
		if e.maintenanceManager != nil {
			_, inMaintenance, err := e.maintenanceManager.IsInMaintenance(ctx, "")
			if err == nil && inMaintenance && schedule.MaintenanceWindowID == "" {
				// Skip schedules not linked to maintenance during maintenance
				log.Printf("[INFO] Skipping schedule %s due to active maintenance window", schedule.ID)
				continue
			}
		}

		// Execute the schedule
		go e.executeSchedule(ctx, schedule)
	}
}

// executeSchedule executes a single schedule.
func (e *Executor) executeSchedule(ctx context.Context, schedule *Schedule) {
	// Try to acquire lock
	lockID := fmt.Sprintf("schedule-exec-%s", schedule.ID)
	acquired, err := e.store.AcquireLock(ctx, lockID, e.config.MemberID)
	if err != nil {
		log.Printf("[ERROR] Failed to acquire lock for schedule %s: %v", schedule.ID, err)
		return
	}
	if !acquired {
		log.Printf("[DEBUG] Lock not acquired for schedule %s, another instance is executing", schedule.ID)
		return
	}

	defer func() {
		if err := e.store.ReleaseLock(ctx, lockID, e.config.MemberID); err != nil {
			log.Printf("[WARN] Failed to release lock for schedule %s: %v", schedule.ID, err)
		}
	}()

	// Create execution record
	now := time.Now().UTC()
	execution := &ScheduleExecution{
		ID:            uuid.New().String(),
		ScheduleID:    schedule.ID,
		ScheduleName:  schedule.Name,
		Status:        ExecutionStatusRunning,
		TriggerType:   TriggerTypeScheduled,
		ScheduledTime: *schedule.NextRun,
		StartTime:     &now,
		CreatedAt:     now,
	}

	// Check if approval is required
	if schedule.RequireApproval {
		execution.Status = ExecutionStatusPending
		if err := e.store.CreateExecution(ctx, execution); err != nil {
			log.Printf("[ERROR] Failed to create execution record: %v", err)
			return
		}

		e.emitEvent(&ExecutorEvent{
			Type:        "execution.pending_approval",
			ScheduleID:  schedule.ID,
			ExecutionID: execution.ID,
			Timestamp:   now,
			Message:     "Execution pending approval",
		})
		return
	}

	// Save execution
	if err := e.store.CreateExecution(ctx, execution); err != nil {
		log.Printf("[ERROR] Failed to create execution record: %v", err)
		return
	}

	// Track active execution
	e.mu.Lock()
	e.activeExecutions[execution.ID] = execution
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.activeExecutions, execution.ID)
		e.mu.Unlock()
	}()

	e.emitEvent(&ExecutorEvent{
		Type:        "execution.started",
		ScheduleID:  schedule.ID,
		ExecutionID: execution.ID,
		Timestamp:   now,
		Message:     fmt.Sprintf("Started execution of schedule %s", schedule.Name),
	})

	// Get handler
	e.mu.RLock()
	handler, exists := e.handlers[schedule.Type]
	e.mu.RUnlock()

	if !exists {
		execution.Status = ExecutionStatusFailed
		execution.Error = fmt.Sprintf("no handler for schedule type %s", schedule.Type)
		e.completeExecution(ctx, schedule, execution)
		return
	}

	// Validate
	if err := handler.Validate(schedule); err != nil {
		execution.Status = ExecutionStatusFailed
		execution.Error = fmt.Sprintf("validation failed: %v", err)
		e.completeExecution(ctx, schedule, execution)
		return
	}

	// Create execution context with timeout
	timeout := schedule.Timeout
	if timeout == 0 {
		timeout = e.config.ExecutionTimeout
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute with retries
	var execErr error
	maxRetries := 0
	if schedule.RetryPolicy != nil {
		maxRetries = schedule.RetryPolicy.MaxRetries
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			execution.RetryCount = attempt

			// Calculate retry delay
			delay := e.calculateRetryDelay(schedule.RetryPolicy, attempt)
			if err := wait.ForContext(execCtx, delay); err != nil {
				execErr = err
				break
			}

			e.emitEvent(&ExecutorEvent{
				Type:        "execution.retry",
				ScheduleID:  schedule.ID,
				ExecutionID: execution.ID,
				Timestamp:   time.Now().UTC(),
				Message:     fmt.Sprintf("Retry attempt %d", attempt),
			})
		}

		execErr = handler.Execute(execCtx, schedule, execution)
		if execErr == nil {
			break
		}

		log.Printf("[WARN] Execution attempt %d failed for schedule %s: %v", attempt+1, schedule.ID, execErr)
	}

	// Update execution status
	endTime := time.Now().UTC()
	execution.EndTime = &endTime
	execution.Duration = endTime.Sub(*execution.StartTime)

	if execErr != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			execution.Status = ExecutionStatusTimeout
			execution.Error = "execution timed out"
		} else {
			execution.Status = ExecutionStatusFailed
			execution.Error = execErr.Error()
		}
	} else {
		execution.Status = ExecutionStatusCompleted
	}

	e.completeExecution(ctx, schedule, execution)
}

// completeExecution completes an execution and updates records.
func (e *Executor) completeExecution(ctx context.Context, schedule *Schedule, execution *ScheduleExecution) {
	// Record result through manager
	if err := e.scheduleManager.RecordExecutionResult(ctx, execution); err != nil {
		log.Printf("[ERROR] Failed to record execution result: %v", err)
	}

	eventType := "execution.completed"
	if execution.Status == ExecutionStatusFailed {
		eventType = "execution.failed"
	} else if execution.Status == ExecutionStatusTimeout {
		eventType = "execution.timeout"
	}

	e.emitEvent(&ExecutorEvent{
		Type:        eventType,
		ScheduleID:  schedule.ID,
		ExecutionID: execution.ID,
		Timestamp:   time.Now().UTC(),
		Message:     fmt.Sprintf("Execution %s: %s", execution.Status, schedule.Name),
		Error:       execution.Error,
		Data: map[string]any{
			"duration":      execution.Duration.String(),
			"success_count": execution.SuccessCount,
			"failure_count": execution.FailureCount,
		},
	})
}

// calculateRetryDelay calculates the delay before a retry.
func (e *Executor) calculateRetryDelay(policy *RetryPolicy, attempt int) time.Duration {
	if policy == nil {
		return time.Minute // Default
	}

	delay := policy.RetryDelay
	if policy.BackoffMultiplier > 0 {
		for i := 0; i < attempt; i++ {
			delay = time.Duration(float64(delay) * policy.BackoffMultiplier)
		}
	}

	if policy.MaxDelay > 0 && delay > policy.MaxDelay {
		delay = policy.MaxDelay
	}

	return delay
}

// cleanupLoop periodically cleans up old execution records.
func (e *Executor) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(e.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopChan:
			return
		case <-ticker.C:
			// Cleanup is handled per-schedule in RecordExecutionResult
			// This could be used for additional cleanup tasks
		}
	}
}

// maintenanceLoop checks for maintenance window state changes.
func (e *Executor) maintenanceLoop(ctx context.Context) {
	ticker := time.NewTicker(e.config.MaintenanceCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopChan:
			return
		case <-ticker.C:
			e.checkMaintenanceWindows(ctx)
		}
	}
}

// checkMaintenanceWindows checks for maintenance windows that need to start/end.
func (e *Executor) checkMaintenanceWindows(ctx context.Context) {
	if e.maintenanceManager == nil {
		return
	}

	now := time.Now().UTC()

	// Check for windows that should start
	filter := &MaintenanceWindowFilter{
		Status:    []MaintenanceWindowStatus{MaintenanceWindowStatusScheduled},
		EndBefore: &now, // EndBefore used for StartAfter check - get windows where start <= now
	}

	// Use a different approach - list all scheduled and check
	allScheduled, err := e.maintenanceManager.List(ctx, &MaintenanceWindowFilter{
		Status: []MaintenanceWindowStatus{MaintenanceWindowStatusScheduled},
	})
	if err != nil {
		log.Printf("[ERROR] Failed to list maintenance windows: %v", err)
		return
	}

	for _, window := range allScheduled {
		// Check if window should start
		if window.StartTime.Before(now) || window.StartTime.Equal(now) {
			if err := e.maintenanceManager.Start(ctx, window.ID); err != nil {
				log.Printf("[ERROR] Failed to start maintenance window %s: %v", window.ID, err)
			}
		}
	}

	// Check for active windows that should end
	activeWindows, err := e.maintenanceManager.GetActiveWindows(ctx)
	if err != nil {
		log.Printf("[ERROR] Failed to get active maintenance windows: %v", err)
		return
	}

	for _, window := range activeWindows {
		if window.EndTime.Before(now) {
			if err := e.maintenanceManager.End(ctx, window.ID); err != nil {
				log.Printf("[ERROR] Failed to end maintenance window %s: %v", window.ID, err)
			}
		}
	}

	// We don't use filter here since it was incorrectly using EndBefore
	_ = filter
}

// ExecuteNow executes a schedule immediately.
func (e *Executor) ExecuteNow(ctx context.Context, scheduleID string, triggeredBy string) (*ScheduleExecution, error) {
	// Delegate to schedule manager
	return e.scheduleManager.TriggerNow(ctx, scheduleID, triggeredBy)
}

// GetActiveExecutions returns currently active executions.
func (e *Executor) GetActiveExecutions() []*ScheduleExecution {
	e.mu.RLock()
	defer e.mu.RUnlock()

	executions := make([]*ScheduleExecution, 0, len(e.activeExecutions))
	for _, exec := range e.activeExecutions {
		executions = append(executions, exec)
	}
	return executions
}

// CancelExecution cancels a running execution.
func (e *Executor) CancelExecution(ctx context.Context, executionID string) error {
	e.mu.RLock()
	exec, exists := e.activeExecutions[executionID]
	e.mu.RUnlock()

	if !exists {
		return fmt.Errorf("execution %s not found or not active", executionID)
	}

	// Mark as cancelled
	now := time.Now().UTC()
	exec.Status = ExecutionStatusCancelled
	exec.EndTime = &now
	exec.Duration = now.Sub(*exec.StartTime)

	return e.store.UpdateExecution(ctx, exec)
}

// AddListener adds an event listener.
func (e *Executor) AddListener(listener ExecutorEventListener) {
	e.mu.Lock()
	e.listeners = append(e.listeners, listener)
	e.mu.Unlock()
}

// emitEvent emits an event to all listeners.
func (e *Executor) emitEvent(event *ExecutorEvent) {
	e.mu.RLock()
	listeners := e.listeners
	e.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// CommandHandler handles command schedule execution.
type CommandHandler struct {
	// ExecuteFunc is the function that executes commands on agents.
	// This should be provided by the control plane.
	ExecuteFunc func(ctx context.Context, target *ScheduleTarget, payload *CommandPayload) (map[string]*AgentExecutionResult, error)
}

// Type returns the schedule type.
func (h *CommandHandler) Type() ScheduleType {
	return ScheduleTypeCommand
}

// Validate validates the schedule payload.
func (h *CommandHandler) Validate(schedule *Schedule) error {
	if schedule.Payload == nil {
		return fmt.Errorf("command payload is required")
	}

	var payload CommandPayload
	if err := json.Unmarshal(schedule.Payload, &payload); err != nil {
		return fmt.Errorf("invalid command payload: %w", err)
	}

	if payload.Command == "" {
		return fmt.Errorf("command is required")
	}

	return nil
}

// Execute executes the schedule.
func (h *CommandHandler) Execute(ctx context.Context, schedule *Schedule, execution *ScheduleExecution) error {
	if h.ExecuteFunc == nil {
		return fmt.Errorf("execute function not configured")
	}

	var payload CommandPayload
	if err := json.Unmarshal(schedule.Payload, &payload); err != nil {
		return fmt.Errorf("invalid command payload: %w", err)
	}

	results, err := h.ExecuteFunc(ctx, schedule.Target, &payload)
	if err != nil {
		return err
	}

	// Aggregate results
	for _, result := range results {
		execution.TargetCount++
		if result.Status == ExecutionStatusCompleted {
			execution.SuccessCount++
		} else {
			execution.FailureCount++
		}
		execution.Results = append(execution.Results, result)
	}

	if execution.FailureCount > 0 {
		return fmt.Errorf("%d of %d agents failed", execution.FailureCount, execution.TargetCount)
	}

	return nil
}

// StateHandler handles state schedule execution.
type StateHandler struct {
	// ApplyFunc is the function that applies state on agents.
	ApplyFunc func(ctx context.Context, target *ScheduleTarget, payload *StatePayload) (map[string]*AgentExecutionResult, error)
}

// Type returns the schedule type.
func (h *StateHandler) Type() ScheduleType {
	return ScheduleTypeState
}

// Validate validates the schedule payload.
func (h *StateHandler) Validate(schedule *Schedule) error {
	if schedule.Payload == nil {
		return fmt.Errorf("state payload is required")
	}

	var payload StatePayload
	if err := json.Unmarshal(schedule.Payload, &payload); err != nil {
		return fmt.Errorf("invalid state payload: %w", err)
	}

	if payload.StatePath == "" && payload.StateContent == "" {
		return fmt.Errorf("state_path or state_content is required")
	}

	return nil
}

// Execute executes the schedule.
func (h *StateHandler) Execute(ctx context.Context, schedule *Schedule, execution *ScheduleExecution) error {
	if h.ApplyFunc == nil {
		return fmt.Errorf("apply function not configured")
	}

	var payload StatePayload
	if err := json.Unmarshal(schedule.Payload, &payload); err != nil {
		return fmt.Errorf("invalid state payload: %w", err)
	}

	results, err := h.ApplyFunc(ctx, schedule.Target, &payload)
	if err != nil {
		return err
	}

	// Aggregate results
	for _, result := range results {
		execution.TargetCount++
		if result.Status == ExecutionStatusCompleted {
			execution.SuccessCount++
		} else {
			execution.FailureCount++
		}
		execution.Results = append(execution.Results, result)
	}

	if execution.FailureCount > 0 {
		return fmt.Errorf("%d of %d agents failed", execution.FailureCount, execution.TargetCount)
	}

	return nil
}

// BlueprintHandler handles blueprint schedule execution.
type BlueprintHandler struct {
	// ApplyFunc is the function that applies blueprints.
	ApplyFunc func(ctx context.Context, target *ScheduleTarget, payload *BlueprintPayload) (map[string]*AgentExecutionResult, error)
}

// Type returns the schedule type.
func (h *BlueprintHandler) Type() ScheduleType {
	return ScheduleTypeBlueprint
}

// Validate validates the schedule payload.
func (h *BlueprintHandler) Validate(schedule *Schedule) error {
	if schedule.Payload == nil {
		return fmt.Errorf("blueprint payload is required")
	}

	var payload BlueprintPayload
	if err := json.Unmarshal(schedule.Payload, &payload); err != nil {
		return fmt.Errorf("invalid blueprint payload: %w", err)
	}

	if payload.BlueprintName == "" {
		return fmt.Errorf("blueprint_name is required")
	}

	return nil
}

// Execute executes the schedule.
func (h *BlueprintHandler) Execute(ctx context.Context, schedule *Schedule, execution *ScheduleExecution) error {
	if h.ApplyFunc == nil {
		return fmt.Errorf("apply function not configured")
	}

	var payload BlueprintPayload
	if err := json.Unmarshal(schedule.Payload, &payload); err != nil {
		return fmt.Errorf("invalid blueprint payload: %w", err)
	}

	results, err := h.ApplyFunc(ctx, schedule.Target, &payload)
	if err != nil {
		return err
	}

	// Aggregate results
	for _, result := range results {
		execution.TargetCount++
		if result.Status == ExecutionStatusCompleted {
			execution.SuccessCount++
		} else {
			execution.FailureCount++
		}
		execution.Results = append(execution.Results, result)
	}

	if execution.FailureCount > 0 {
		return fmt.Errorf("%d of %d agents failed", execution.FailureCount, execution.TargetCount)
	}

	return nil
}
