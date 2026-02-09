package execution

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// Manager provides management operations for runbook executions.
// It supports pause/resume, step retry, step skip, cancellation with rollback,
// and execution cloning.
type Manager struct {
	mu sync.RWMutex

	// Active executions tracked by ID
	executions map[string]*ManagedExecution

	// Storage for persistence
	storage Storage

	// Executor for running steps and cloned executions
	executor *Executor

	// Rollback handlers by step type
	rollbackHandlers map[runbook.StepType]RollbackHandler
}

// ManagedExecution extends Context with management state.
type ManagedExecution struct {
	*Context

	// Management state
	paused     bool
	pausedAt   *time.Time
	resumedAt  *time.Time
	pauseCh    chan struct{}
	resumeCh   chan struct{}
	rollbackFn []func(ctx context.Context) error

	// Parent execution for clones
	clonedFrom string

	// Step management
	skippedSteps map[string]bool
	retriedSteps map[string]int
}

// RollbackHandler defines how to rollback a step.
type RollbackHandler interface {
	// CanRollback returns true if the step can be rolled back.
	CanRollback(step *runbook.Step) bool

	// Rollback undoes the effect of a step execution.
	Rollback(ctx context.Context, step *runbook.Step, stepExec *runbook.StepExecution) error
}

// RollbackFunc is a function adapter for RollbackHandler.
type RollbackFunc func(ctx context.Context, step *runbook.Step, stepExec *runbook.StepExecution) error

// CanRollback always returns true for RollbackFunc.
func (f RollbackFunc) CanRollback(step *runbook.Step) bool {
	return true
}

// Rollback calls the underlying function.
func (f RollbackFunc) Rollback(ctx context.Context, step *runbook.Step, stepExec *runbook.StepExecution) error {
	return f(ctx, step, stepExec)
}

// NewManager creates a new execution manager.
func NewManager(executor *Executor, storage Storage) *Manager {
	return &Manager{
		executions:       make(map[string]*ManagedExecution),
		storage:          storage,
		executor:         executor,
		rollbackHandlers: make(map[runbook.StepType]RollbackHandler),
	}
}

// RegisterRollbackHandler registers a rollback handler for a step type.
func (m *Manager) RegisterRollbackHandler(stepType runbook.StepType, handler RollbackHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollbackHandlers[stepType] = handler
}

// Track starts tracking an execution for management.
func (m *Manager) Track(execCtx *Context) *ManagedExecution {
	m.mu.Lock()
	defer m.mu.Unlock()

	managed := &ManagedExecution{
		Context: execCtx,
		pauseCh:          make(chan struct{}),
		resumeCh:         make(chan struct{}),
		skippedSteps:     make(map[string]bool),
		retriedSteps:     make(map[string]int),
	}

	m.executions[execCtx.ExecutionID()] = managed
	return managed
}

// Untrack stops tracking an execution.
func (m *Manager) Untrack(executionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.executions, executionID)
}

// Get retrieves a managed execution by ID.
func (m *Manager) Get(executionID string) (*ManagedExecution, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exec, ok := m.executions[executionID]
	return exec, ok
}

// List returns all managed executions.
func (m *Manager) List() []*ManagedExecution {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ManagedExecution, 0, len(m.executions))
	for _, exec := range m.executions {
		result = append(result, exec)
	}
	return result
}

// Pause pauses a running execution.
func (m *Manager) Pause(executionID string) error {
	m.mu.Lock()
	exec, ok := m.executions[executionID]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("execution %s not found", executionID)
	}

	if exec.paused {
		return errors.New("execution already paused")
	}

	if exec.IsTerminal() {
		return errors.New("cannot pause terminal execution")
	}

	exec.paused = true
	now := time.Now()
	exec.pausedAt = &now
	close(exec.pauseCh)

	// Save state
	if m.storage != nil {
		_ = m.storage.SaveExecution(context.Background(), exec.ToExecution())
	}

	return nil
}

// Resume resumes a paused execution.
func (m *Manager) Resume(executionID string) error {
	m.mu.Lock()
	exec, ok := m.executions[executionID]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("execution %s not found", executionID)
	}

	if !exec.paused {
		return errors.New("execution not paused")
	}

	exec.paused = false
	now := time.Now()
	exec.resumedAt = &now

	// Create new pause channel and signal resume
	oldResumeCh := exec.resumeCh
	exec.resumeCh = make(chan struct{})
	exec.pauseCh = make(chan struct{})
	close(oldResumeCh)

	// Save state
	if m.storage != nil {
		_ = m.storage.SaveExecution(context.Background(), exec.ToExecution())
	}

	return nil
}

// IsPaused returns true if the execution is paused.
func (m *Manager) IsPaused(executionID string) bool {
	m.mu.RLock()
	exec, ok := m.executions[executionID]
	m.mu.RUnlock()

	if !ok {
		return false
	}

	return exec.paused
}

// WaitIfPaused blocks until the execution is resumed or context is cancelled.
// Returns true if resumed, false if context was cancelled.
func (m *Manager) WaitIfPaused(ctx context.Context, executionID string) bool {
	m.mu.RLock()
	exec, ok := m.executions[executionID]
	m.mu.RUnlock()

	if !ok || !exec.paused {
		return true
	}

	select {
	case <-ctx.Done():
		return false
	case <-exec.resumeCh:
		return true
	}
}

// RetryStep retries a failed step.
func (m *Manager) RetryStep(ctx context.Context, executionID, stepName string) error {
	m.mu.RLock()
	exec, ok := m.executions[executionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("execution %s not found", executionID)
	}

	stepCtx, ok := exec.GetStep(stepName)
	if !ok {
		return fmt.Errorf("step %s not found", stepName)
	}

	// Only allow retry of failed steps
	if !stepCtx.IsFailed() {
		return fmt.Errorf("step %s is not in failed state", stepName)
	}

	// Reset step state for retry
	stepCtx.mu.Lock()
	stepCtx.machine = NewStepMachine(stepName)
	stepCtx.errorMessage = ""
	stepCtx.startedAt = nil
	stepCtx.completedAt = nil
	stepCtx.retryCount++
	exec.retriedSteps[stepName]++
	stepCtx.mu.Unlock()

	// Re-execute the step
	step := stepCtx.Step()
	if m.executor != nil {
		err := m.executor.executeStep(ctx, step, stepCtx, exec.Context)
		if err != nil {
			return fmt.Errorf("retry failed: %w", err)
		}
	}

	// Save state
	if m.storage != nil {
		_ = m.storage.SaveStepExecution(ctx, executionID, stepCtx.ToStepExecution())
	}

	return nil
}

// SkipStep marks a pending step as skipped.
func (m *Manager) SkipStep(ctx context.Context, executionID, stepName string) error {
	m.mu.RLock()
	exec, ok := m.executions[executionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("execution %s not found", executionID)
	}

	stepCtx, ok := exec.GetStep(stepName)
	if !ok {
		return fmt.Errorf("step %s not found", stepName)
	}

	// Only allow skipping pending steps
	if !stepCtx.IsPending() {
		return fmt.Errorf("step %s is not in pending state", stepName)
	}

	if err := stepCtx.Skip(ctx); err != nil {
		return fmt.Errorf("skip failed: %w", err)
	}

	exec.skippedSteps[stepName] = true

	// Save state
	if m.storage != nil {
		_ = m.storage.SaveStepExecution(ctx, executionID, stepCtx.ToStepExecution())
	}

	return nil
}

// Cancel cancels a running execution.
func (m *Manager) Cancel(ctx context.Context, executionID string) error {
	m.mu.RLock()
	exec, ok := m.executions[executionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("execution %s not found", executionID)
	}

	if exec.IsTerminal() {
		return errors.New("cannot cancel terminal execution")
	}

	// If paused, resume first to allow cancellation
	if exec.paused {
		_ = m.Resume(executionID) //nolint:contextcheck // Resume API doesn't take context
	}

	exec.Cancel()

	return nil
}

// CancelWithRollback cancels a running execution and rolls back completed steps.
func (m *Manager) CancelWithRollback(ctx context.Context, executionID string) error {
	m.mu.RLock()
	exec, ok := m.executions[executionID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("execution %s not found", executionID)
	}

	// Cancel the execution first
	if err := m.Cancel(ctx, executionID); err != nil {
		return err
	}

	// Get execution data for rollback
	execData := exec.ToExecution()

	// Rollback completed steps in reverse order
	var rollbackErrors []error
	for stepName, stepExec := range execData.Steps {
		if stepExec.State != runbook.StepStateCompleted {
			continue
		}

		stepCtx, ok := exec.GetStep(stepName)
		if !ok {
			continue
		}

		step := stepCtx.Step()
		handler, hasHandler := m.rollbackHandlers[step.Type]

		if hasHandler && handler.CanRollback(step) {
			if err := handler.Rollback(ctx, step, stepExec); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("rollback %s: %w", stepName, err))
			}
		}
	}

	// Also execute any registered rollback functions
	for i := len(exec.rollbackFn) - 1; i >= 0; i-- {
		if err := exec.rollbackFn[i](ctx); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}

	if len(rollbackErrors) > 0 {
		return fmt.Errorf("rollback completed with %d errors: %v", len(rollbackErrors), rollbackErrors)
	}

	return nil
}

// Clone creates a new execution with the same inputs as an existing one.
func (m *Manager) Clone(ctx context.Context, executionID string) (*ManagedExecution, error) {
	m.mu.RLock()
	exec, ok := m.executions[executionID]
	m.mu.RUnlock()

	if !ok {
		// Try to load from storage
		if m.storage != nil {
			storedExec, err := m.storage.GetExecution(ctx, executionID)
			if err != nil {
				return nil, fmt.Errorf("execution %s not found", executionID)
			}

			// Create new execution context from stored data
			newID := uuid.New().String()

			// We need the original runbook to clone - this is a limitation
			_ = storedExec
			_ = newID
			return nil, fmt.Errorf("cannot clone execution from storage without runbook reference")
		}
		return nil, fmt.Errorf("execution %s not found", executionID)
	}

	return m.CloneExecution(ctx, exec)
}

// CloneExecution creates a new execution from an existing managed execution.
func (m *Manager) CloneExecution(ctx context.Context, exec *ManagedExecution) (*ManagedExecution, error) {
	// Get inputs from original execution
	inputs := exec.Inputs()

	// Create new execution ID
	newID := uuid.New().String()

	// We need to get the runbook to create a proper clone
	// For now, create a new context that copies the structure
	newExecCtx := &Context{
		executionID:    newID,
		runbookName:    exec.RunbookName(),
		runbookVersion: exec.RunbookVersion(),
		machine:        NewMachine(),
		createdAt:      time.Now(),
		inputs:         inputs,
		steps:          make(map[string]*StepContext),
	}

	// Copy step definitions (need fresh step contexts)
	exec.mu.RLock()
	for name, stepCtx := range exec.steps {
		newExecCtx.steps[name] = NewStepContext(stepCtx.Step())
	}
	exec.mu.RUnlock()

	managed := &ManagedExecution{
		Context: newExecCtx,
		pauseCh:          make(chan struct{}),
		resumeCh:         make(chan struct{}),
		skippedSteps:     make(map[string]bool),
		retriedSteps:     make(map[string]int),
		clonedFrom:       exec.ExecutionID(),
	}

	m.mu.Lock()
	m.executions[newID] = managed
	m.mu.Unlock()

	return managed, nil
}

// GetCloneSource returns the execution ID this execution was cloned from.
func (m *ManagedExecution) GetCloneSource() string {
	return m.clonedFrom
}

// GetSkippedSteps returns a list of manually skipped steps.
func (m *ManagedExecution) GetSkippedSteps() []string {
	result := make([]string, 0, len(m.skippedSteps))
	for name := range m.skippedSteps {
		result = append(result, name)
	}
	return result
}

// GetRetriedSteps returns a map of step names to retry counts.
func (m *ManagedExecution) GetRetriedSteps() map[string]int {
	result := make(map[string]int)
	for k, v := range m.retriedSteps {
		result[k] = v
	}
	return result
}

// AddRollbackFn registers a rollback function to be called on cancellation.
func (m *ManagedExecution) AddRollbackFn(fn func(ctx context.Context) error) {
	m.rollbackFn = append(m.rollbackFn, fn)
}

// IsPaused returns true if the execution is paused.
func (m *ManagedExecution) IsPaused() bool {
	return m.paused
}

// PausedAt returns when the execution was paused.
func (m *ManagedExecution) PausedAt() *time.Time {
	return m.pausedAt
}

// ResumedAt returns when the execution was resumed.
func (m *ManagedExecution) ResumedAt() *time.Time {
	return m.resumedAt
}

// ToManagedExecutionInfo converts to a serializable info structure.
func (m *ManagedExecution) ToManagedExecutionInfo() *ManagedExecutionInfo {
	return &ManagedExecutionInfo{
		Execution:    m.ToExecution(),
		Paused:       m.paused,
		PausedAt:     m.pausedAt,
		ResumedAt:    m.resumedAt,
		ClonedFrom:   m.clonedFrom,
		SkippedSteps: m.GetSkippedSteps(),
		RetriedSteps: m.GetRetriedSteps(),
	}
}

// ManagedExecutionInfo is a serializable representation of a managed execution.
type ManagedExecutionInfo struct {
	Execution    *runbook.Execution `json:"execution"`
	Paused       bool               `json:"paused"`
	PausedAt     *time.Time         `json:"paused_at,omitempty"`
	ResumedAt    *time.Time         `json:"resumed_at,omitempty"`
	ClonedFrom   string             `json:"cloned_from,omitempty"`
	SkippedSteps []string           `json:"skipped_steps,omitempty"`
	RetriedSteps map[string]int     `json:"retried_steps,omitempty"`
}

// History tracks execution history for an execution.
type History struct {
	ExecutionID string         `json:"execution_id"`
	Events      []HistoryEvent `json:"events"`
}

// HistoryEvent represents a historical event.
type HistoryEvent struct {
	Timestamp time.Time   `json:"timestamp"`
	Type      string      `json:"type"`
	Details   interface{} `json:"details,omitempty"`
}

// PauseCheckpoint allows the executor to check for pauses during execution.
type PauseCheckpoint struct {
	manager     *Manager
	executionID string
}

// NewPauseCheckpoint creates a new pause checkpoint.
func NewPauseCheckpoint(manager *Manager, executionID string) *PauseCheckpoint {
	return &PauseCheckpoint{
		manager:     manager,
		executionID: executionID,
	}
}

// Check blocks if the execution is paused until resumed.
// Returns an error if the context is cancelled while waiting.
func (p *PauseCheckpoint) Check(ctx context.Context) error {
	if !p.manager.WaitIfPaused(ctx, p.executionID) {
		return ctx.Err()
	}
	return nil
}
