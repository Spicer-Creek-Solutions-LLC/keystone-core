// Package verification is the GitOps deployment-verification engine
// (Epic 16). It runs verification steps — HTTP probes, gRPC health
// checks, command/script assertions — against a freshly-deployed
// target and reports whether the deployment is healthy.
//
// Task 5 scope: the [Verifier] interface, the [Step]/[Result] value
// types, the [Registry], and the three v1.0 verifiers (HTTP, gRPC,
// command). Orchestration — sequential/parallel execution, per-step
// retries and timeout, optional-step semantics — is the [Workflow]
// engine (task 6). A [Step] therefore *carries* Optional/Timeout/
// Retries as data but this package does not act on them: each
// [Verifier.Verify] is a single attempt that honours ctx (the engine
// sets the deadline in task 6).
package verification

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrConfig wraps a malformed or missing [Step.Config] value. A
// verifier returns a failed [Result] with this as Error rather than a
// Go error or a panic.
var ErrConfig = errors.New("verification: invalid step config")

// ErrUnknownVerifier is reported by [Registry.Lookup]'s callers when a
// step's Type has no registered [Verifier].
var ErrUnknownVerifier = errors.New("verification: unknown verifier type")

// Step is one verification step. Config holds the verifier-specific
// options. Optional, Timeout and Retries are carried here for the
// task-6 [Workflow] engine — this package does not interpret them.
type Step struct {
	// Name is a human label for logs / results (e.g. "api-health").
	Name string
	// Type selects the verifier: "http" | "grpc" | "command".
	Type string
	// Optional, when true, means the workflow engine (task 6) does
	// not fail the workflow if this step fails.
	Optional bool
	// Timeout is the per-step deadline the engine applies via ctx
	// (task 6). Zero means the engine's default. Unused in task 5.
	Timeout time.Duration
	// Retries is the per-step retry budget the engine applies
	// (task 6). Unused in task 5.
	Retries int
	// Config is the verifier-specific configuration.
	Config map[string]any
}

// Result is the outcome of a single [Verifier.Verify]. Duration is
// set by the verifier; Retries is set by the engine (task 6).
type Result struct {
	// Success is the verdict.
	Success bool
	// Message is a short human summary (pass or fail reason).
	Message string
	// Data carries verifier-specific detail (e.g. http status).
	Data map[string]any
	// Duration is how long the single attempt took.
	Duration time.Duration
	// Error is the failure cause when Success is false (config
	// error, transport error, assertion miss). Nil on success.
	Error error
	// Retries is filled by the workflow engine (task 6).
	Retries int
}

// Verifier runs one verification type. Implementations must:
//   - honour ctx cancellation (the engine enforces per-step timeout
//     via ctx in task 6),
//   - never panic on malformed [Step.Config] — return a failed
//     [Result] with Error set,
//   - never return a Go error (the verdict is the Result).
type Verifier interface {
	// Type returns the step type this verifier handles.
	Type() string
	// Verify performs a single verification attempt.
	Verify(ctx context.Context, step Step) Result
}

// Registry maps step Type → [Verifier]. Safe for concurrent use; the
// workflow engine (task 6) only reads it during a run.
type Registry struct {
	mu sync.RWMutex
	m  map[string]Verifier
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{m: make(map[string]Verifier)}
}

// Register binds a verifier to its [Verifier.Type]. Re-registering a
// type overwrites. A nil verifier or empty type is rejected.
func (r *Registry) Register(v Verifier) error {
	if v == nil {
		return errors.New("verification: nil verifier")
	}
	t := v.Type()
	if t == "" {
		return errors.New("verification: verifier reports empty type")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[t] = v
	return nil
}

// Lookup returns the verifier for stepType, or false.
func (r *Registry) Lookup(stepType string) (Verifier, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, ok := r.m[stepType]
	return v, ok
}

// failf builds a failed [Result] with a formatted message wrapping
// err. Shared by the verifiers so config/transport failures render
// consistently.
func failf(start time.Time, err error, format string, args ...any) Result {
	return Result{
		Success:  false,
		Message:  fmt.Sprintf(format, args...),
		Duration: time.Since(start),
		Error:    err,
	}
}
