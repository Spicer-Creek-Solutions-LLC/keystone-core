// Package rollback is the GitOps manual-rollback engine (Epic 16).
// It reverts a bad deployment via one of three v1.0 executors —
// Git revert, ArgoCD sync-to-revision, Kubernetes rollout-undo —
// selected by the rollback strategy.
//
// Task 7 scope: the [Executor] interface, the [Request]/[Config]/
// [Result] value types, the [Strategy] enum, the three executors, and
// the [Registry]. The orchestrating Engine (state machine + approval
// gates) is task 8; the CLI/REST surface is task 10.
//
// Heavy/external clients are kept behind narrow seams (GitClient,
// ArgoClient, K8sRolloutClient) so this package stays dependency-light
// and unit-testable with fakes; the concrete adapters live in the
// gitexec / argoexec subpackages (and, for Kubernetes, a deferred
// client-go adapter wired at boot — see the gate-v1.0 ROADMAP entry).
package rollback

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrConfig wraps a malformed or missing [Config] value. An executor
// returns a failed [Result] with this as Error rather than panicking
// or returning a Go error.
var ErrConfig = errors.New("rollback: invalid executor config")

// ErrUnknownExecutor is reported by [Registry.Lookup]'s callers when a
// rollback has no registered [Executor] for its type.
var ErrUnknownExecutor = errors.New("rollback: unknown executor type")

// ErrNotConfigured is returned by an executor whose required client
// seam was not supplied (e.g. the Kubernetes client-go adapter is
// deferred to boot).
var ErrNotConfigured = errors.New("rollback: executor client not configured")

// Strategy selects which revision a rollback targets.
type Strategy string

const (
	// StrategyPrevious rolls back to the revision immediately before
	// the current one (provider history order).
	StrategyPrevious Strategy = "previous"
	// StrategySpecific rolls back to [Request.Revision] verbatim.
	StrategySpecific Strategy = "specific"
	// StrategyLastKnownGood rolls back to the last revision the
	// provider considers good. v1.0 is best-effort (provider
	// history); verification-engine-confirmed "good" is post-v1.0.
	StrategyLastKnownGood Strategy = "last-known-good"
)

// Valid reports whether s is one of the three v1.0 strategies.
func (s Strategy) Valid() bool {
	switch s {
	case StrategyPrevious, StrategySpecific, StrategyLastKnownGood:
		return true
	default:
		return false
	}
}

// Request is one rollback ask.
type Request struct {
	// Application is the deployed unit (ArgoCD app, K8s deployment,
	// or a label for the Git target).
	Application string
	// Strategy selects the target revision.
	Strategy Strategy
	// Revision is the explicit target for [StrategySpecific].
	Revision string
	// Reason is the operator-supplied audit reason.
	Reason string
}

// Config is the executor-specific configuration (repo URL + branch,
// ArgoCD server + token, kube namespace + deployment, …).
type Config map[string]any

// Result is the outcome of one [Executor.Execute].
type Result struct {
	Success      bool
	Message      string
	FromRevision string
	ToRevision   string
	Data         map[string]any
	Duration     time.Duration
	Error        error
}

// Executor performs a rollback for one provider. Implementations
// must: honour ctx cancellation; never panic on malformed [Config]
// (return a failed [Result] with Error set); report the verdict in
// [Result] rather than a Go error from Execute.
type Executor interface {
	// Type returns the executor type: "git" | "argocd" | "k8s".
	Type() string
	// Execute performs the rollback described by req using cfg.
	Execute(ctx context.Context, cfg Config, req Request) Result
	// GetPreviousRevision resolves the revision immediately before
	// the current one for the target in req/cfg.
	GetPreviousRevision(ctx context.Context, cfg Config, req Request) (string, error)
	// GetLastKnownGood resolves the last good revision (v1.0:
	// best-effort from provider history).
	GetLastKnownGood(ctx context.Context, cfg Config, req Request) (string, error)
}

// Registry maps executor Type → [Executor]. Safe for concurrent use;
// the engine (task 8) only reads it during a rollback.
type Registry struct {
	mu sync.RWMutex
	m  map[string]Executor
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{m: make(map[string]Executor)}
}

// Register binds an executor to its [Executor.Type]. Re-registering a
// type overwrites. A nil executor or empty type is rejected.
func (r *Registry) Register(e Executor) error {
	if e == nil {
		return errors.New("rollback: nil executor")
	}
	t := e.Type()
	if t == "" {
		return errors.New("rollback: executor reports empty type")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[t] = e
	return nil
}

// Lookup returns the executor for execType, or false.
func (r *Registry) Lookup(execType string) (Executor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.m[execType]
	return e, ok
}

// resolveTarget maps a [Request] onto the revision to roll back to,
// using the executor's previous / last-known-good resolvers. Shared
// by the three executors so strategy handling is uniform.
func resolveTarget(ctx context.Context, e Executor, cfg Config, req Request) (string, error) {
	switch req.Strategy {
	case StrategySpecific:
		if req.Revision == "" {
			return "", fmt.Errorf("%w: strategy=specific requires a revision", ErrConfig)
		}
		return req.Revision, nil
	case StrategyPrevious:
		return e.GetPreviousRevision(ctx, cfg, req)
	case StrategyLastKnownGood:
		return e.GetLastKnownGood(ctx, cfg, req)
	default:
		return "", fmt.Errorf("%w: unknown strategy %q", ErrConfig, req.Strategy)
	}
}

// failf builds a failed [Result] wrapping err with a formatted
// message. Shared so config/transport failures render consistently.
func failf(start time.Time, err error, format string, args ...any) Result {
	return Result{
		Success:  false,
		Message:  fmt.Sprintf(format, args...),
		Duration: time.Since(start),
		Error:    err,
	}
}
