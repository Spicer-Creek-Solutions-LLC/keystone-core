// Package handlers provides step handler implementations for runbook execution.
package handlers

import (
	"context"
	"fmt"
	"sync"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// Handler defines the interface for step execution handlers.
type Handler interface {
	// Type returns the step type this handler processes.
	Type() runbook.StepType

	// Validate checks step config before execution.
	Validate(step *runbook.Step) error

	// Execute runs the step and returns result.
	Execute(ctx context.Context, step *runbook.Step, vars VariableContext) (*runbook.StepResult, error)
}

// VariableContext provides access to execution variables for step handlers.
type VariableContext interface {
	// GetInput returns an input value by name.
	GetInput(name string) (interface{}, bool)

	// GetStepOutput returns an output value from a completed step.
	GetStepOutput(stepName, outputName string) (interface{}, bool)

	// ExecutionID returns the execution ID.
	ExecutionID() string

	// RunbookName returns the runbook name.
	RunbookName() string

	// Resolve resolves a template string against the current variable context.
	Resolve(template string) (string, error)

	// ResolveValue resolves a template and returns the typed value.
	ResolveValue(template string) (interface{}, error)

	// EvaluateCondition evaluates a condition expression and returns a boolean.
	EvaluateCondition(expr string) (bool, error)
}

// Registry manages step handlers.
type Registry struct {
	mu       sync.RWMutex
	handlers map[runbook.StepType]Handler
}

// NewRegistry creates a new handler registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[runbook.StepType]Handler),
	}
}

// Register adds a handler to the registry.
func (r *Registry) Register(handler Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.handlers[handler.Type()]; exists {
		return fmt.Errorf("handler for type %q already registered", handler.Type())
	}

	r.handlers[handler.Type()] = handler
	return nil
}

// MustRegister adds a handler to the registry, panicking on error.
func (r *Registry) MustRegister(handler Handler) {
	if err := r.Register(handler); err != nil {
		panic(err)
	}
}

// Get returns the handler for the given step type.
func (r *Registry) Get(stepType runbook.StepType) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	h, ok := r.handlers[stepType]
	return h, ok
}

// Types returns all registered step types.
func (r *Registry) Types() []runbook.StepType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]runbook.StepType, 0, len(r.handlers))
	for t := range r.handlers {
		types = append(types, t)
	}
	return types
}

// Validate validates a step using the appropriate handler.
func (r *Registry) Validate(step *runbook.Step) error {
	r.mu.RLock()
	handler, ok := r.handlers[step.Type]
	r.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no handler registered for step type %q", step.Type)
	}

	return handler.Validate(step)
}

// Execute executes a step using the appropriate handler.
func (r *Registry) Execute(ctx context.Context, step *runbook.Step, vars VariableContext) (*runbook.StepResult, error) {
	r.mu.RLock()
	handler, ok := r.handlers[step.Type]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no handler registered for step type %q", step.Type)
	}

	return handler.Execute(ctx, step, vars)
}

// DefaultRegistry returns a registry with all built-in handlers registered.
// Handlers that require runtime dependencies (like StateHandler, DeployHandler)
// are registered with nil dependencies and will return errors if executed
// without proper configuration via the execution context.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	// Register built-in handlers
	r.MustRegister(NewNoopHandler())
	r.MustRegister(NewFailHandler())
	r.MustRegister(NewWaitHandler())
	r.MustRegister(NewCommandHandler())
	r.MustRegister(NewAPIHandler())
	r.MustRegister(NewNotificationHandler())

	// Handlers requiring runtime dependencies (configured via execution context)
	r.MustRegister(NewStateHandler(nil))
	r.MustRegister(NewDeployHandler(nil))
	r.MustRegister(NewRollbackHandler(nil))

	// Script execution handlers
	r.MustRegister(NewScriptHandler())
	r.MustRegister(NewScriptFileHandler())

	return r
}
