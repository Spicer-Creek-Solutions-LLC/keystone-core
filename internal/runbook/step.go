// SPDX-License-Identifier: Apache-2.0

package runbook

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrUnknownStepType is returned by the engine when a step's Type has
// no registered [StepExecutor].
var ErrUnknownStepType = errors.New("runbook: unknown step type")

// StepContext is what a [StepExecutor] receives. Config is the
// already-templated step configuration; Inputs and Steps are the
// run's inputs and the completed steps' views (read-only).
type StepContext struct {
	Step   Step
	Config map[string]any
	Inputs map[string]any
	Steps  map[string]StepView
}

// StepOutput is what a [StepExecutor] returns. Outputs is exposed to
// later steps as `{{ .steps.<name>.outputs.<field> }}`.
type StepOutput struct {
	Outputs map[string]any
}

// StepExecutor runs one step type. Implementations must honour ctx
// cancellation (the engine enforces per-step timeouts via ctx).
type StepExecutor interface {
	Execute(ctx context.Context, sc StepContext) (StepOutput, error)
}

// StepFunc adapts a function to a [StepExecutor] — used by tests and
// trivial executors.
type StepFunc func(ctx context.Context, sc StepContext) (StepOutput, error)

// Execute implements [StepExecutor].
func (f StepFunc) Execute(ctx context.Context, sc StepContext) (StepOutput, error) {
	return f(ctx, sc)
}

// Registry maps step Type → [StepExecutor]. Safe for concurrent use;
// the engine only reads it during a run.
type Registry struct {
	mu sync.RWMutex
	m  map[string]StepExecutor
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{m: make(map[string]StepExecutor)}
}

// Register binds a step type to an executor. Re-registering a type
// overwrites. A nil executor or empty type is rejected.
func (r *Registry) Register(stepType string, ex StepExecutor) error {
	if stepType == "" {
		return errors.New("runbook: empty step type")
	}
	if ex == nil {
		return fmt.Errorf("runbook: nil executor for step type %q", stepType)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[stepType] = ex
	return nil
}

// Lookup returns the executor for stepType, or false.
func (r *Registry) Lookup(stepType string) (StepExecutor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ex, ok := r.m[stepType]
	return ex, ok
}
