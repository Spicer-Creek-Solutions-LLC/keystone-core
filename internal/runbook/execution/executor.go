package execution

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shawnbutts/keystone-core/internal/runbook"
	"github.com/shawnbutts/keystone-core/internal/runbook/handlers"
)

// Executor executes runbooks with step dependency resolution.
type Executor struct {
	registry *handlers.Registry

	// Optional storage for persisting execution state
	storage Storage

	// Callbacks
	onExecutionStart    func(ctx context.Context, exec *Context)
	onExecutionComplete func(ctx context.Context, exec *Context)
	onStepStart         func(ctx context.Context, exec *Context, step *StepContext)
	onStepComplete      func(ctx context.Context, exec *Context, step *StepContext)
}

// Storage interface for persisting execution state.
type Storage interface {
	// SaveExecution saves or updates an execution record.
	SaveExecution(ctx context.Context, exec *runbook.Execution) error

	// GetExecution retrieves an execution by ID.
	GetExecution(ctx context.Context, id string) (*runbook.Execution, error)

	// SaveStepExecution saves or updates a step execution record.
	SaveStepExecution(ctx context.Context, executionID string, step *runbook.StepExecution) error
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithStorage sets the storage backend for persistence.
func WithStorage(storage Storage) ExecutorOption {
	return func(e *Executor) {
		e.storage = storage
	}
}

// WithRegistry sets a custom handler registry.
func WithRegistry(registry *handlers.Registry) ExecutorOption {
	return func(e *Executor) {
		e.registry = registry
	}
}

// WithExecutionCallbacks sets execution lifecycle callbacks.
func WithExecutionCallbacks(
	onStart func(ctx context.Context, exec *Context),
	onComplete func(ctx context.Context, exec *Context),
) ExecutorOption {
	return func(e *Executor) {
		e.onExecutionStart = onStart
		e.onExecutionComplete = onComplete
	}
}

// WithStepCallbacks sets step lifecycle callbacks.
func WithStepCallbacks(
	onStart func(ctx context.Context, exec *Context, step *StepContext),
	onComplete func(ctx context.Context, exec *Context, step *StepContext),
) ExecutorOption {
	return func(e *Executor) {
		e.onStepStart = onStart
		e.onStepComplete = onComplete
	}
}

// NewExecutor creates a new runbook executor.
func NewExecutor(opts ...ExecutorOption) *Executor {
	e := &Executor{
		registry: handlers.DefaultRegistry(),
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// Execute runs a runbook with the given inputs.
func (e *Executor) Execute(ctx context.Context, rb *runbook.Runbook, inputs map[string]interface{}) (*runbook.Execution, error) {
	// Validate runbook
	if err := runbook.Validate(rb); err != nil {
		return nil, fmt.Errorf("runbook validation failed: %w", err)
	}

	// Validate inputs against input definitions
	if err := e.validateInputs(rb, inputs); err != nil {
		return nil, fmt.Errorf("input validation failed: %w", err)
	}

	// Create execution context
	executionID := uuid.New().String()
	execCtx := NewContext(executionID, rb, inputs)

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	execCtx.SetCancel(cancel)
	defer cancel()

	// Apply runbook timeout if specified
	if rb.Spec.Timeout != "" {
		if timeout, err := time.ParseDuration(rb.Spec.Timeout); err == nil {
			var timeoutCancel context.CancelFunc
			ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
			defer timeoutCancel()
		}
	}

	// Run execution
	return e.executeRunbook(ctx, rb, execCtx)
}

// executeRunbook performs the actual runbook execution.
func (e *Executor) executeRunbook(ctx context.Context, rb *runbook.Runbook, execCtx *Context) (*runbook.Execution, error) {
	// Start execution
	if err := execCtx.Start(ctx); err != nil {
		return execCtx.ToExecution(), fmt.Errorf("failed to start execution: %w", err)
	}

	// Save initial state
	if e.storage != nil {
		if err := e.storage.SaveExecution(ctx, execCtx.ToExecution()); err != nil {
			// Log but don't fail
			_ = err
		}
	}

	// Callback
	if e.onExecutionStart != nil {
		e.onExecutionStart(ctx, execCtx)
	}

	// Build dependency graph and execute steps
	err := e.executeSteps(ctx, rb.Spec.Steps, execCtx)

	// Handle completion
	if err != nil {
		// Check if it was cancelled
		if errors.Is(err, context.Canceled) {
			_ = execCtx.CancelExecution(ctx)
		} else {
			_ = execCtx.Fail(ctx, err.Error())
		}

		// Run onFailure handlers
		if len(rb.Spec.OnFailure) > 0 {
			_ = e.executeSteps(ctx, rb.Spec.OnFailure, execCtx)
		}
	} else {
		_ = execCtx.Complete(ctx)

		// Run onSuccess handlers
		if len(rb.Spec.OnSuccess) > 0 {
			_ = e.executeSteps(ctx, rb.Spec.OnSuccess, execCtx)
		}
	}

	// Save final state
	if e.storage != nil {
		if saveErr := e.storage.SaveExecution(ctx, execCtx.ToExecution()); saveErr != nil {
			// Log but don't fail
			_ = saveErr
		}
	}

	// Callback
	if e.onExecutionComplete != nil {
		e.onExecutionComplete(ctx, execCtx)
	}

	return execCtx.ToExecution(), err
}

// executeSteps executes a list of steps respecting dependencies.
func (e *Executor) executeSteps(ctx context.Context, steps []runbook.Step, execCtx *Context) error {
	// Build dependency graph
	graph := buildDependencyGraph(steps)

	// Topological sort to determine execution order
	order, err := topologicalSort(graph)
	if err != nil {
		return fmt.Errorf("dependency resolution failed: %w", err)
	}

	// Create step name to step mapping
	stepMap := make(map[string]*runbook.Step)
	for i := range steps {
		stepMap[steps[i].Name] = &steps[i]
	}

	// Execute steps in order
	for _, stepName := range order {
		step := stepMap[stepName]
		stepCtx, ok := execCtx.GetStep(stepName)
		if !ok {
			// For onSuccess/onFailure handlers, create step context on demand
			stepCtx = NewStepContext(step)
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Execute single step
		if err := e.executeStep(ctx, step, stepCtx, execCtx); err != nil {
			// Check if step allows continue on error
			if !step.ContinueOnError {
				return err
			}
		}
	}

	return nil
}

// executeStep executes a single step with retry support.
func (e *Executor) executeStep(ctx context.Context, step *runbook.Step, stepCtx *StepContext, execCtx *Context) error {
	// Create variable context for this step
	varCtx := &variableContext{
		execCtx: execCtx,
	}

	// Evaluate condition if present
	if step.Condition != "" {
		shouldRun, err := e.evaluateCondition(ctx, step.Condition, varCtx)
		if err != nil {
			return fmt.Errorf("condition evaluation failed: %w", err)
		}
		if !shouldRun {
			return stepCtx.Skip(ctx)
		}
	}

	// Start step
	if err := stepCtx.Start(ctx); err != nil {
		return err
	}

	// Callback
	if e.onStepStart != nil {
		e.onStepStart(ctx, execCtx, stepCtx)
	}

	// Get handler
	handler, ok := e.registry.Get(step.Type)
	if !ok {
		err := fmt.Errorf("no handler for step type %q", step.Type)
		_ = stepCtx.Fail(ctx, err.Error())
		return err
	}

	// Apply step timeout
	execCtxForStep := ctx
	if step.Timeout != "" {
		if timeout, err := time.ParseDuration(step.Timeout); err == nil {
			var cancel context.CancelFunc
			execCtxForStep, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
	}

	// Execute with retry
	maxAttempts := 1
	if step.Retries != nil && step.Retries.MaxAttempts > 1 {
		maxAttempts = step.Retries.MaxAttempts
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err := handler.Execute(execCtxForStep, step, varCtx)

		if err == nil && result.Success {
			// Success
			stepCtx.SetResult(result)
			stepCtx.SetOutputs(result.Outputs)
			if completeErr := stepCtx.Complete(ctx); completeErr != nil {
				return completeErr
			}

			// Callback
			if e.onStepComplete != nil {
				e.onStepComplete(ctx, execCtx, stepCtx)
			}

			// Save step state
			if e.storage != nil {
				_ = e.storage.SaveStepExecution(ctx, execCtx.ExecutionID(), stepCtx.ToStepExecution())
			}

			return nil
		}

		lastErr = err
		if result != nil {
			stepCtx.SetResult(result)
		}

		// Check if we should retry
		if attempt < maxAttempts {
			stepCtx.IncrementRetry()

			// Apply backoff delay
			delay := step.Retries.GetDelay()
			if delay > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
			}
		}
	}

	// All attempts failed
	errMsg := "step failed"
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	_ = stepCtx.Fail(ctx, errMsg)

	// Callback
	if e.onStepComplete != nil {
		e.onStepComplete(ctx, execCtx, stepCtx)
	}

	// Save step state
	if e.storage != nil {
		_ = e.storage.SaveStepExecution(ctx, execCtx.ExecutionID(), stepCtx.ToStepExecution())
	}

	return lastErr
}

// evaluateCondition evaluates a step condition.
func (e *Executor) evaluateCondition(ctx context.Context, condition string, varCtx *variableContext) (bool, error) {
	return varCtx.EvaluateCondition(condition)
}

// validateInputs validates execution inputs against runbook input definitions.
func (e *Executor) validateInputs(rb *runbook.Runbook, inputs map[string]interface{}) error {
	for _, inputDef := range rb.Spec.Inputs {
		value, exists := inputs[inputDef.Name]

		if inputDef.Required && !exists {
			if inputDef.Default == nil {
				return fmt.Errorf("required input %q not provided", inputDef.Name)
			}
		}

		if exists {
			// Type validation could be added here
			_ = value
		}
	}

	return nil
}

// variableContext implements handlers.VariableContext.
type variableContext struct {
	execCtx *Context
}

func (v *variableContext) GetInput(name string) (interface{}, bool) {
	return v.execCtx.GetInput(name)
}

func (v *variableContext) GetStepOutput(stepName, outputName string) (interface{}, bool) {
	return v.execCtx.GetStepOutput(stepName, outputName)
}

func (v *variableContext) ExecutionID() string {
	return v.execCtx.ExecutionID()
}

func (v *variableContext) RunbookName() string {
	return v.execCtx.RunbookName()
}

func (v *variableContext) Resolve(template string) (string, error) {
	return v.execCtx.Resolve(template)
}

func (v *variableContext) ResolveValue(template string) (interface{}, error) {
	return v.execCtx.ResolveValue(template)
}

func (v *variableContext) EvaluateCondition(expr string) (bool, error) {
	return v.execCtx.EvaluateCondition(expr)
}

// dependencyNode represents a node in the dependency graph.
type dependencyNode struct {
	name     string
	deps     []string
	incoming int
}

// buildDependencyGraph creates a dependency graph from steps.
func buildDependencyGraph(steps []runbook.Step) map[string]*dependencyNode {
	graph := make(map[string]*dependencyNode)

	// Initialize nodes
	for i := range steps {
		step := &steps[i]
		graph[step.Name] = &dependencyNode{
			name: step.Name,
			deps: step.DependsOn,
		}
	}

	// Count incoming edges
	for _, node := range graph {
		for _, dep := range node.deps {
			if depNode, ok := graph[dep]; ok {
				depNode.incoming++
			}
		}
	}

	return graph
}

// topologicalSort performs Kahn's algorithm for topological sorting.
func topologicalSort(graph map[string]*dependencyNode) ([]string, error) {
	// Find nodes with no incoming edges
	var queue []string
	for name, node := range graph {
		if len(node.deps) == 0 {
			queue = append(queue, name)
		}
	}

	var order []string
	visited := make(map[string]bool)

	for len(queue) > 0 {
		// Pop from queue
		name := queue[0]
		queue = queue[1:]

		if visited[name] {
			continue
		}
		visited[name] = true
		order = append(order, name)

		// Find nodes that depend on this one
		for otherName, otherNode := range graph {
			if visited[otherName] {
				continue
			}

			// Check if this node is a dependency
			allDepsVisited := true
			for _, dep := range otherNode.deps {
				if !visited[dep] {
					allDepsVisited = false
					break
				}
			}

			if allDepsVisited {
				queue = append(queue, otherName)
			}
		}
	}

	// Check if all nodes were visited
	if len(order) != len(graph) {
		return nil, errors.New("circular dependency detected")
	}

	return order, nil
}

// ExecuteAsync starts a runbook execution asynchronously.
func (e *Executor) ExecuteAsync(ctx context.Context, rb *runbook.Runbook, inputs map[string]interface{}) (string, error) {
	// Validate runbook
	if err := runbook.Validate(rb); err != nil {
		return "", fmt.Errorf("runbook validation failed: %w", err)
	}

	// Validate inputs
	if err := e.validateInputs(rb, inputs); err != nil {
		return "", fmt.Errorf("input validation failed: %w", err)
	}

	// Create execution context
	executionID := uuid.New().String()

	// Start execution in background
	go func() {
		execCtx := NewContext(executionID, rb, inputs)
		_, _ = e.executeRunbook(ctx, rb, execCtx)
	}()

	return executionID, nil
}

// activeExecutions tracks running executions for cancellation.
var activeExecutions = struct {
	sync.RWMutex
	executions map[string]*Context
}{
	executions: make(map[string]*Context),
}

// CancelExecution cancels a running execution by ID.
func CancelExecution(executionID string) bool {
	activeExecutions.RLock()
	exec, ok := activeExecutions.executions[executionID]
	activeExecutions.RUnlock()

	if !ok {
		return false
	}

	exec.Cancel()
	return true
}
