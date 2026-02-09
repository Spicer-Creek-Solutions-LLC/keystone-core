package execution

import (
	"context"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
	"github.com/shawnbutts/keystone-core/internal/runbook/variables"
)

// Context manages the state and data for a runbook execution.
// It provides thread-safe access to inputs, step outputs, and execution metadata.
type Context struct {
	mu sync.RWMutex

	// Execution identity
	executionID    string
	runbookName    string
	runbookVersion string

	// State machine
	machine *Machine

	// Timing
	startedAt   *time.Time
	completedAt *time.Time
	createdAt   time.Time

	// Input values provided at execution time
	inputs map[string]interface{}

	// Step execution states keyed by step name
	steps map[string]*StepContext

	// Error message if execution failed
	errorMessage string

	// Cancellation function
	cancel context.CancelFunc

	// Variable context for template resolution
	varContext *variables.Context
}

// StepContext manages the state and data for a single step execution.
type StepContext struct {
	mu sync.RWMutex

	name    string
	step    *runbook.Step
	machine *StepMachine

	// Timing
	startedAt   *time.Time
	completedAt *time.Time

	// Execution data
	inputs     map[string]interface{}
	outputs    map[string]interface{}
	result     *runbook.StepResult
	retryCount int

	// Error message if step failed
	errorMessage string
}

// NewContext creates a new execution context for a runbook.
func NewContext(
	executionID string,
	rb *runbook.Runbook,
	inputs map[string]interface{},
) *Context {
	now := time.Now()

	// Copy inputs
	inputsCopy := make(map[string]interface{})
	for k, v := range inputs {
		inputsCopy[k] = v
	}

	ctx := &Context{
		executionID:    executionID,
		runbookName:    rb.Metadata.Name,
		runbookVersion: rb.Metadata.Version,
		machine:        NewMachine(),
		createdAt:      now,
		inputs:         inputsCopy,
		steps:          make(map[string]*StepContext),
		varContext:     variables.NewContext(executionID, rb.Metadata.Name, rb.Metadata.Version, inputsCopy),
	}

	// Initialize step contexts
	for i := range rb.Spec.Steps {
		step := &rb.Spec.Steps[i]
		ctx.steps[step.Name] = NewStepContext(step)
	}

	return ctx
}

// NewStepContext creates a new step context.
func NewStepContext(step *runbook.Step) *StepContext {
	return &StepContext{
		name:    step.Name,
		step:    step,
		machine: NewStepMachine(step.Name),
		inputs:  make(map[string]interface{}),
		outputs: make(map[string]interface{}),
	}
}

// ExecutionID returns the execution ID.
func (c *Context) ExecutionID() string {
	return c.executionID
}

// RunbookName returns the runbook name.
func (c *Context) RunbookName() string {
	return c.runbookName
}

// RunbookVersion returns the runbook version.
func (c *Context) RunbookVersion() string {
	return c.runbookVersion
}

// State returns the current execution state.
func (c *Context) State() runbook.ExecutionState {
	return c.machine.State()
}

// CreatedAt returns when the execution was created.
func (c *Context) CreatedAt() time.Time {
	return c.createdAt
}

// StartedAt returns when the execution started (nil if not started).
func (c *Context) StartedAt() *time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.startedAt
}

// CompletedAt returns when the execution completed (nil if not completed).
func (c *Context) CompletedAt() *time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.completedAt
}

// ErrorMessage returns the error message if execution failed.
func (c *Context) ErrorMessage() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.errorMessage
}

// Inputs returns a copy of the execution inputs.
func (c *Context) Inputs() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]interface{})
	for k, v := range c.inputs {
		result[k] = v
	}
	return result
}

// GetInput returns an input value by name.
func (c *Context) GetInput(name string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	v, ok := c.inputs[name]
	return v, ok
}

// SetCancel sets the cancellation function for this execution.
func (c *Context) SetCancel(cancel context.CancelFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancel = cancel
}

// Cancel cancels the execution.
func (c *Context) Cancel() {
	c.mu.Lock()
	cancel := c.cancel
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// Start marks the execution as started.
func (c *Context) Start(ctx context.Context) error {
	if err := c.machine.Start(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	now := time.Now()
	c.startedAt = &now
	c.mu.Unlock()

	return nil
}

// Complete marks the execution as completed successfully.
func (c *Context) Complete(ctx context.Context) error {
	if err := c.machine.Complete(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	now := time.Now()
	c.completedAt = &now
	c.mu.Unlock()

	return nil
}

// Fail marks the execution as failed.
func (c *Context) Fail(ctx context.Context, errorMsg string) error {
	if err := c.machine.Fail(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	now := time.Now()
	c.completedAt = &now
	c.errorMessage = errorMsg
	c.mu.Unlock()

	return nil
}

// CancelExecution marks the execution as cancelled.
func (c *Context) CancelExecution(ctx context.Context) error {
	if err := c.machine.Cancel(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	now := time.Now()
	c.completedAt = &now
	c.errorMessage = "execution cancelled"
	c.mu.Unlock()

	return nil
}

// IsTerminal returns true if the execution is in a terminal state.
func (c *Context) IsTerminal() bool {
	return c.machine.IsTerminal()
}

// GetStep returns the step context for the given step name.
func (c *Context) GetStep(name string) (*StepContext, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	step, ok := c.steps[name]
	return step, ok
}

// GetStepOutput returns an output value from a completed step.
func (c *Context) GetStepOutput(stepName, outputName string) (interface{}, bool) {
	c.mu.RLock()
	step, ok := c.steps[stepName]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	return step.GetOutput(outputName)
}

// Resolve resolves a template string against the current variable context.
func (c *Context) Resolve(template string) (string, error) {
	// Sync step outputs to variable context before resolving
	c.syncStepOutputs()
	return c.varContext.Resolve(template)
}

// ResolveValue resolves a template and returns the typed value.
func (c *Context) ResolveValue(template string) (interface{}, error) {
	// Sync step outputs to variable context before resolving
	c.syncStepOutputs()
	return c.varContext.ResolveValue(template)
}

// EvaluateCondition evaluates a condition expression and returns a boolean.
func (c *Context) EvaluateCondition(expr string) (bool, error) {
	// Sync step outputs to variable context before evaluating
	c.syncStepOutputs()
	return c.varContext.EvaluateCondition(expr)
}

// syncStepOutputs synchronizes step outputs to the variable context.
func (c *Context) syncStepOutputs() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for name, step := range c.steps {
		outputs := step.Outputs()
		if len(outputs) > 0 {
			c.varContext.SetStepOutputs(name, outputs)
		}
	}
}

// AllStepsCompleted returns true if all steps have reached a terminal state.
func (c *Context) AllStepsCompleted() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, step := range c.steps {
		if !step.machine.IsTerminal() {
			return false
		}
	}
	return true
}

// AnyStepFailed returns true if any step has failed.
func (c *Context) AnyStepFailed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, step := range c.steps {
		if step.machine.IsFailed() {
			return true
		}
	}
	return false
}

// ToExecution converts the context to an Execution record.
func (c *Context) ToExecution() *runbook.Execution {
	c.mu.RLock()
	defer c.mu.RUnlock()

	steps := make(map[string]*runbook.StepExecution)
	for name, stepCtx := range c.steps {
		steps[name] = stepCtx.ToStepExecution()
	}

	// Aggregate outputs from all steps
	outputs := make(map[string]interface{})
	for stepName, stepCtx := range c.steps {
		stepOutputs := stepCtx.Outputs()
		for outName, outVal := range stepOutputs {
			outputs[stepName+"."+outName] = outVal
		}
	}

	return &runbook.Execution{
		ID:             c.executionID,
		RunbookName:    c.runbookName,
		RunbookVersion: c.runbookVersion,
		State:          c.machine.State(),
		Inputs:         c.inputs,
		Outputs:        outputs,
		StartedAt:      c.startedAt,
		CompletedAt:    c.completedAt,
		Error:          c.errorMessage,
		Steps:          steps,
		CreatedAt:      c.createdAt,
	}
}

// Name returns the step name.
func (s *StepContext) Name() string {
	return s.name
}

// Step returns the step definition.
func (s *StepContext) Step() *runbook.Step {
	return s.step
}

// State returns the current step state.
func (s *StepContext) State() runbook.StepState {
	return s.machine.State()
}

// StartedAt returns when the step started.
func (s *StepContext) StartedAt() *time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.startedAt
}

// CompletedAt returns when the step completed.
func (s *StepContext) CompletedAt() *time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.completedAt
}

// ErrorMessage returns the error message if step failed.
func (s *StepContext) ErrorMessage() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.errorMessage
}

// RetryCount returns the number of retries attempted.
func (s *StepContext) RetryCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.retryCount
}

// IncrementRetry increments the retry count.
func (s *StepContext) IncrementRetry() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retryCount++
	return s.retryCount
}

// Outputs returns a copy of the step outputs.
func (s *StepContext) Outputs() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]interface{})
	for k, v := range s.outputs {
		result[k] = v
	}
	return result
}

// GetOutput returns an output value by name.
func (s *StepContext) GetOutput(name string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.outputs[name]
	return v, ok
}

// SetOutput sets an output value.
func (s *StepContext) SetOutput(name string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outputs[name] = value
}

// SetOutputs sets multiple output values.
func (s *StepContext) SetOutputs(outputs map[string]interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for k, v := range outputs {
		s.outputs[k] = v
	}
}

// Result returns the step result.
func (s *StepContext) Result() *runbook.StepResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.result
}

// SetResult sets the step result.
func (s *StepContext) SetResult(result *runbook.StepResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.result = result
}

// Start marks the step as started.
func (s *StepContext) Start(ctx context.Context) error {
	if err := s.machine.Start(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	now := time.Now()
	s.startedAt = &now
	s.mu.Unlock()

	return nil
}

// Complete marks the step as completed successfully.
func (s *StepContext) Complete(ctx context.Context) error {
	if err := s.machine.Complete(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	now := time.Now()
	s.completedAt = &now
	s.mu.Unlock()

	return nil
}

// Fail marks the step as failed.
func (s *StepContext) Fail(ctx context.Context, errorMsg string) error {
	if err := s.machine.Fail(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	now := time.Now()
	s.completedAt = &now
	s.errorMessage = errorMsg
	s.mu.Unlock()

	return nil
}

// Skip marks the step as skipped.
func (s *StepContext) Skip(ctx context.Context) error {
	if err := s.machine.Skip(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	now := time.Now()
	s.completedAt = &now
	s.mu.Unlock()

	return nil
}

// IsTerminal returns true if the step is in a terminal state.
func (s *StepContext) IsTerminal() bool {
	return s.machine.IsTerminal()
}

// IsPending returns true if the step is pending.
func (s *StepContext) IsPending() bool {
	return s.machine.IsPending()
}

// IsCompleted returns true if the step completed successfully.
func (s *StepContext) IsCompleted() bool {
	return s.machine.IsCompleted()
}

// IsFailed returns true if the step failed.
func (s *StepContext) IsFailed() bool {
	return s.machine.IsFailed()
}

// IsSkipped returns true if the step was skipped.
func (s *StepContext) IsSkipped() bool {
	return s.machine.IsSkipped()
}

// ToStepExecution converts the step context to a StepExecution record.
func (s *StepContext) ToStepExecution() *runbook.StepExecution {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var duration time.Duration
	if s.result != nil {
		duration = s.result.Duration
	}

	return &runbook.StepExecution{
		Name:        s.name,
		Type:        s.step.Type,
		State:       s.machine.State(),
		Inputs:      s.inputs,
		Outputs:     s.outputs,
		StartedAt:   s.startedAt,
		CompletedAt: s.completedAt,
		Error:       s.errorMessage,
		RetryCount:  s.retryCount,
		Duration:    duration,
	}
}
