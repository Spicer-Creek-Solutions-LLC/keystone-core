package runbook

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"go.keystone-core.io/keystone-core/pkg/statemachine"
)

// ErrUnknownInput is returned when Execute is given an input the
// runbook does not declare.
var ErrUnknownInput = errors.New("runbook: unknown input")

// ErrMissingInput is returned when a required input has no value and
// no default.
var ErrMissingInput = errors.New("runbook: missing required input")

// ErrExecutionFailed is returned by Execute when the run ends in the
// failed state. The *Execution is still returned (with its trail) so
// callers get the full picture; the error wraps the failing step's.
var ErrExecutionFailed = errors.New("runbook: execution failed")

// Execution lifecycle states/events for the pkg/statemachine machine.
type execState string
type execEvent string

const (
	exPending   execState = "pending"
	exRunning   execState = "running"
	exSucceeded execState = "succeeded"
	exFailed    execState = "failed"

	evStart   execEvent = "start"
	evSucceed execEvent = "succeed"
	evFail    execEvent = "fail"
)

// Executor runs runbooks. Clock and Sleep are test seams; nil falls
// back to time.Now and a context-aware timer. Backoff* default when
// zero. NewID defaults to a random UUID.
type Executor struct {
	Registry    *Registry
	Clock       func() time.Time
	Sleep       func(ctx context.Context, d time.Duration) error
	BackoffBase time.Duration
	BackoffCap  time.Duration
	NewID       func() string
}

func (e *Executor) now() time.Time {
	if e.Clock != nil {
		return e.Clock()
	}
	return time.Now()
}

func (e *Executor) sleep(ctx context.Context, d time.Duration) error {
	if e.Sleep != nil {
		return e.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (e *Executor) newID() string {
	if e.NewID != nil {
		return e.NewID()
	}
	return uuid.NewString()
}

// Execute runs rb with the given inputs. It returns the Execution
// (always populated, even on failure) and an error that is nil on
// success or wraps ErrExecutionFailed / a setup error otherwise.
func (e *Executor) Execute(ctx context.Context, rb *Runbook, inputs map[string]any) (*Execution, error) {
	exec := &Execution{
		ID:        e.newID(),
		Runbook:   rb.Metadata.Name,
		Status:    StatusPending,
		StartedAt: e.now(),
	}

	resolved, err := e.resolveInputs(rb, inputs)
	if err != nil {
		exec.Status = StatusFailed
		exec.EndedAt = e.now()
		exec.Error = err
		return exec, err
	}
	exec.Inputs = resolved

	rc := runCfg{specMaxRetries: rb.Spec.MaxRetries}
	if rb.Spec.Timeout != "" {
		// Validate already confirmed this parses.
		rc.specTimeout, _ = time.ParseDuration(rb.Spec.Timeout)
	}

	order, err := resolveOrder(rb.Spec.Steps)
	if err != nil {
		exec.Status = StatusFailed
		exec.EndedAt = e.now()
		exec.Error = err
		return exec, err
	}

	byName := make(map[string]Step, len(rb.Spec.Steps))
	for _, s := range rb.Spec.Steps {
		byName[s.Name] = s
	}

	machine := e.buildMachine()
	resultIdx := make(map[string]int, len(rb.Spec.Steps))
	for _, s := range rb.Spec.Steps {
		resultIdx[s.Name] = len(exec.Steps)
		exec.Steps = append(exec.Steps, StepResult{Name: s.Name, Type: s.Type, Status: StatusPending})
	}

	_ = machine.Fire(ctx, string(evStart))
	e.trail(exec, "", StatusPending, StatusRunning, "execution started")
	exec.Status = StatusRunning

	// completed step views for templating.
	stepsCtx := make(map[string]any, len(rb.Spec.Steps))

	var failErr error
	for _, name := range order {
		step := byName[name]
		ri := resultIdx[name]
		res := e.runStep(ctx, exec, &step, rc, resolved, stepsCtx)
		exec.Steps[ri] = res
		stepsCtx[name] = map[string]any{
			"outputs": orEmpty(res.Output),
			"status":  string(res.Status),
		}
		if res.Status == StatusFailed {
			failErr = res.Error
			break
		}
	}

	if failErr != nil {
		_ = machine.Fire(ctx, string(evFail))
		e.trail(exec, "", StatusRunning, StatusFailed, "a step failed")
		exec.Status = StatusFailed
		exec.Error = fmt.Errorf("%w: %w", ErrExecutionFailed, failErr)
		e.runChain(ctx, exec, rb.Spec.OnFailure, byName, rc, resolved, stepsCtx, resultIdx)
		exec.EndedAt = e.now()
		return exec, exec.Error
	}

	_ = machine.Fire(ctx, string(evSucceed))
	e.trail(exec, "", StatusRunning, StatusSucceeded, "all steps succeeded")
	exec.Status = StatusSucceeded
	e.runChain(ctx, exec, rb.Spec.OnSuccess, byName, rc, resolved, stepsCtx, resultIdx)
	exec.EndedAt = e.now()
	return exec, nil
}

// runCfg carries the runbook-level defaults a step falls back to when
// it does not set its own Timeout / Retries.
type runCfg struct {
	specTimeout    time.Duration
	specMaxRetries int
}

// buildMachine constructs the run lifecycle FSM. Construction cannot
// fail (static definition) so MustBuild is safe.
func (e *Executor) buildMachine() *statemachine.Machine[string, string] {
	return statemachine.NewBuilder[string, string]().
		Initial(string(exPending)).
		Transition(string(exPending), string(evStart), string(exRunning)).
		Transition(string(exRunning), string(evSucceed), string(exSucceeded)).
		Transition(string(exRunning), string(evFail), string(exFailed)).
		Clock(e.now).
		MustBuild()
}

// runChain executes a list of step names in order (OnSuccess /
// OnFailure). Failures inside the chain are recorded but do not
// recurse into another chain.
func (e *Executor) runChain(ctx context.Context, exec *Execution, names []string, byName map[string]Step, rc runCfg, inputs map[string]any, stepsCtx map[string]any, resultIdx map[string]int) {
	for _, name := range names {
		step, ok := byName[name]
		if !ok {
			continue // Validate guarantees closure; defensive.
		}
		res := e.runStep(ctx, exec, &step, rc, inputs, stepsCtx)
		if ri, ok := resultIdx[name]; ok {
			exec.Steps[ri] = res
		} else {
			exec.Steps = append(exec.Steps, res)
		}
		stepsCtx[name] = map[string]any{"outputs": orEmpty(res.Output), "status": string(res.Status)}
	}
}

// runStep evaluates one step: condition gate, config render, dispatch
// with per-step timeout + retry/backoff. It returns a finished
// StepResult and appends trail entries.
func (e *Executor) runStep(ctx context.Context, exec *Execution, step *Step, rc runCfg, inputs map[string]any, stepsCtx map[string]any) StepResult {
	res := StepResult{Name: step.Name, Type: step.Type, StartedAt: e.now()}
	rr := renderRoot{inputs: inputs, steps: stepsCtx}

	if step.Condition != "" {
		cond, err := renderString(step.Condition, rr)
		if err != nil {
			return e.failStep(exec, res, err)
		}
		if !truthy(cond) {
			res.Status = StatusSkipped
			res.Duration = e.now().Sub(res.StartedAt)
			e.trail(exec, step.Name, StatusPending, StatusSkipped, "condition falsey")
			return res
		}
	}

	cfg, err := renderConfig(step.Config, rr)
	if err != nil {
		return e.failStep(exec, res, err)
	}

	ex, ok := e.Registry.Lookup(step.Type)
	if !ok {
		return e.failStep(exec, res, fmt.Errorf("%w: %q", ErrUnknownStepType, step.Type))
	}

	e.trail(exec, step.Name, StatusPending, StatusRunning, "step started")
	maxAttempts := 1 + retriesFor(step, rc)
	timeout := timeoutFor(step, rc)

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		res.Attempts = attempt
		sctx := StepContext{Step: *step, Config: cfg, Inputs: inputs, Steps: viewMap(stepsCtx)}

		cctx := ctx
		var cancel context.CancelFunc
		if timeout > 0 {
			cctx, cancel = context.WithTimeout(ctx, timeout)
		}
		out, runErr := ex.Execute(cctx, sctx)
		if cancel != nil {
			cancel()
		}

		if runErr == nil {
			res.Status = StatusSucceeded
			res.Output = out.Outputs
			res.Duration = e.now().Sub(res.StartedAt)
			e.trail(exec, step.Name, StatusRunning, StatusSucceeded, fmt.Sprintf("ok (attempt %d)", attempt))
			return res
		}
		lastErr = runErr
		if attempt < maxAttempts {
			if serr := e.sleep(ctx, expBackoff(attempt-1, e.BackoffBase, e.BackoffCap)); serr != nil {
				lastErr = fmt.Errorf("%w (backoff interrupted: %v)", runErr, serr)
				break
			}
		}
	}
	res.Attempts = maxAttempts
	return e.failStep(exec, res, lastErr)
}

func (e *Executor) failStep(exec *Execution, res StepResult, err error) StepResult {
	res.Status = StatusFailed
	res.Error = err
	res.Duration = e.now().Sub(res.StartedAt)
	e.trail(exec, res.Name, StatusRunning, StatusFailed, err.Error())
	return res
}

// retriesFor / timeoutFor apply the precedence: an explicit per-step
// setting wins; otherwise the runbook-level spec default.
func retriesFor(step *Step, rc runCfg) int {
	if step.Retries > 0 {
		return step.Retries
	}
	return rc.specMaxRetries
}

func timeoutFor(step *Step, rc runCfg) time.Duration {
	if step.Timeout != "" {
		if d, err := time.ParseDuration(step.Timeout); err == nil {
			return d
		}
	}
	return rc.specTimeout
}

func (e *Executor) trail(exec *Execution, step string, from, to Status, note string) {
	exec.Trail = append(exec.Trail, TrailEntry{At: e.now(), Step: step, From: from, To: to, Note: note})
}

// resolveInputs applies declared defaults, enforces required, and
// rejects undeclared inputs. It also captures the spec-level timeout
// / max-retries defaults onto the Execution for per-step fallback.
func (e *Executor) resolveInputs(rb *Runbook, given map[string]any) (map[string]any, error) {
	declared := make(map[string]InputSpec, len(rb.Spec.Inputs))
	for _, in := range rb.Spec.Inputs {
		declared[in.Name] = in
	}
	for name := range given {
		if _, ok := declared[name]; !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownInput, name)
		}
	}
	out := make(map[string]any, len(declared))
	for name, spec := range declared {
		if v, ok := given[name]; ok {
			out[name] = v
			continue
		}
		if spec.Default != nil {
			out[name] = spec.Default
			continue
		}
		if spec.Required {
			return nil, fmt.Errorf("%w: %q", ErrMissingInput, name)
		}
	}
	return out, nil
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// viewMap converts the internal stepsCtx (map[string]any of
// {outputs,status}) into the typed StepView map handed to executors.
func viewMap(stepsCtx map[string]any) map[string]StepView {
	out := make(map[string]StepView, len(stepsCtx))
	for name, v := range stepsCtx {
		m, _ := v.(map[string]any)
		sv := StepView{}
		if o, ok := m["outputs"].(map[string]any); ok {
			sv.Outputs = o
		}
		if s, ok := m["status"].(string); ok {
			sv.Status = s
		}
		out[name] = sv
	}
	return out
}
