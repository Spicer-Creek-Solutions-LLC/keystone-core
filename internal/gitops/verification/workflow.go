package verification

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// FailurePolicy governs what a workflow does when a required (non-
// optional) step fails.
type FailurePolicy string

const (
	// FailAbort (the default; empty string normalizes to this) stops
	// the workflow at the first failed required step. Sequential:
	// remaining steps are marked Skipped. Parallel: the shared ctx is
	// cancelled to short-circuit in-flight steps (best-effort).
	FailAbort FailurePolicy = "abort"
	// FailContinue runs every step regardless of failures and
	// aggregates the results.
	FailContinue FailurePolicy = "continue"
)

// Workflow is a verification plan: an ordered set of [Step]s run
// sequentially (default) or in parallel, with an overall timeout and
// a failure policy.
type Workflow struct {
	// Name labels the workflow in the result and logs.
	Name string
	// Steps are executed in order (sequential) or concurrently
	// (Parallel); the result preserves this order regardless.
	Steps []Step
	// Parallel runs all steps concurrently. Default false = sequential.
	Parallel bool
	// Timeout bounds the whole workflow. 0 = no overall deadline
	// (per-step timeouts still apply).
	Timeout time.Duration
	// OnFailure is the required-step failure policy. Empty = FailAbort.
	OnFailure FailurePolicy
	// MaxParallel caps concurrency when Parallel. 0 = len(Steps).
	MaxParallel int
}

// StepResult is one step's outcome within a [WorkflowResult].
type StepResult struct {
	Name     string
	Type     string
	Optional bool
	// Skipped is true when the step was not run because an earlier
	// required step failed under FailAbort (sequential only).
	Skipped bool
	// Result is the verifier's outcome from the final attempt;
	// Result.Retries is the number of retries the engine used.
	Result Result
}

// WorkflowResult aggregates a run. Success is true iff every required
// step succeeded; optional-step failures are recorded but ignored.
type WorkflowResult struct {
	Name     string
	Success  bool
	Steps    []StepResult
	Duration time.Duration
}

// Engine runs [Workflow]s against a [Registry] of verifiers.
type Engine struct {
	// Registry resolves a Step.Type to its Verifier (required).
	Registry *Registry
	// DefaultStepTimeout applies to a step whose Timeout is 0. 0 =
	// no per-step deadline beyond the workflow / ctx.
	DefaultStepTimeout time.Duration
	// BackoffBase / BackoffCap tune the inter-retry delay. Zero
	// values fall back to the package defaults (100ms / 30s).
	BackoffBase time.Duration
	BackoffCap  time.Duration
}

// NewEngine returns an Engine bound to reg with default backoff.
func NewEngine(reg *Registry) *Engine {
	return &Engine{Registry: reg}
}

// Run executes wf and returns the aggregate result. It never returns
// an error: every failure (config, transport, assertion, unknown
// verifier, timeout) is captured in the per-step [Result].
func (e *Engine) Run(ctx context.Context, wf Workflow) WorkflowResult {
	start := time.Now()
	if wf.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, wf.Timeout)
		defer cancel()
	}

	results := make([]StepResult, len(wf.Steps))
	if wf.Parallel {
		e.runParallel(ctx, wf, results)
	} else {
		e.runSequential(ctx, wf, results)
	}

	success := true
	for _, sr := range results {
		if sr.Optional {
			continue
		}
		if sr.Skipped || !sr.Result.Success {
			success = false
		}
	}
	return WorkflowResult{
		Name:     wf.Name,
		Success:  success,
		Steps:    results,
		Duration: time.Since(start),
	}
}

func (e *Engine) runSequential(ctx context.Context, wf Workflow, out []StepResult) {
	abort := wf.OnFailure != FailContinue
	stopped := false
	for i := range wf.Steps {
		step := wf.Steps[i]
		if stopped {
			out[i] = StepResult{Name: step.Name, Type: step.Type, Optional: step.Optional, Skipped: true}
			continue
		}
		out[i] = e.runStep(ctx, step)
		if abort && !step.Optional && !out[i].Result.Success {
			stopped = true
		}
	}
}

func (e *Engine) runParallel(ctx context.Context, wf Workflow, out []StepResult) {
	limit := wf.MaxParallel
	if limit <= 0 || limit > len(wf.Steps) {
		limit = len(wf.Steps)
	}
	abort := wf.OnFailure != FailContinue

	runCtx := ctx
	var cancel context.CancelFunc
	if abort {
		runCtx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i := range wf.Steps {
		wg.Add(1)
		go func(idx int, step Step) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			out[idx] = e.runStep(runCtx, step)
			if abort && !step.Optional && !out[idx].Result.Success && cancel != nil {
				cancel() // short-circuit remaining in-flight steps
			}
		}(i, wf.Steps[i])
	}
	wg.Wait()
}

// runStep runs one step with per-step timeout + retry/backoff,
// mirroring the runbook executor. The verifier reports its verdict in
// Result (never a Go error), so the retry loop iterates while
// Result.Success is false.
func (e *Engine) runStep(ctx context.Context, step Step) StepResult {
	sr := StepResult{Name: step.Name, Type: step.Type, Optional: step.Optional}

	v, ok := e.Registry.Lookup(step.Type)
	if !ok {
		sr.Result = Result{
			Success: false,
			Message: fmt.Sprintf("unknown verifier type %q", step.Type),
			Error:   fmt.Errorf("%w: %q", ErrUnknownVerifier, step.Type),
		}
		return sr
	}

	maxAttempts := 1 + step.Retries
	timeout := step.Timeout
	if timeout <= 0 {
		timeout = e.DefaultStepTimeout
	}

	var res Result
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		cctx := ctx
		var cancel context.CancelFunc
		if timeout > 0 {
			cctx, cancel = context.WithTimeout(ctx, timeout)
		}
		res = v.Verify(cctx, step)
		if cancel != nil {
			cancel()
		}
		res.Retries = attempt - 1
		if res.Success {
			sr.Result = res
			return sr
		}
		if attempt < maxAttempts {
			if serr := ctxSleep(ctx, expBackoff(attempt-1, e.BackoffBase, e.BackoffCap)); serr != nil {
				// Parent ctx cancelled/expired mid-backoff: stop
				// retrying and return the last failed result.
				res.Retries = attempt - 1
				break
			}
		}
	}
	sr.Result = res
	return sr
}
