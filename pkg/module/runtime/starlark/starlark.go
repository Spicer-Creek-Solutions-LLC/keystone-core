// Package starlark is the v1.0 module runtime (Epic 14 task 11):
// a go.starlark.net-backed sandboxed interpreter implementing the
// task-10 loader.Runtime / loader.Instance interfaces.
//
// Deterministic by default: the core Starlark universe excludes the
// time/random library modules, so nothing non-deterministic is
// reachable unless a capability builtin exposes it (task 12).
// Strict file options additionally forbid `while`, top-level
// control flow, global reassignment, and recursion — bounding the
// program by construction. Per-call limits add an execution-step
// cap (thread.SetMaxExecutionSteps) and a wall-clock timeout
// (thread.Cancel). A precise heap-bytes ceiling is not in
// go.starlark.net's public API for v1.0 (see the "Starlark hard
// heap-bytes cap" ROADMAP entry) — approximated by the step+time
// bounds.
//
// The capability→builtin shims that fill the BuiltinProvider are
// task 12; task 11 ships the interpreter, limits, value
// conversion, and the loader.Runtime impl.
package starlark

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	star "go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"go.keystone-core.io/keystone-core/pkg/module/loader"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

var (
	// ErrCompile — the entrypoint source failed to parse/resolve.
	ErrCompile = errors.New("starlark: compile failed")
	// ErrNoMain — the module has no callable `main` global.
	ErrNoMain = errors.New("starlark: module has no main()")
	// ErrExec — a Starlark runtime error during execution.
	ErrExec = errors.New("starlark: execution error")
	// ErrTimeout — execution exceeded the per-call timeout.
	ErrTimeout = errors.New("starlark: execution timed out")
	// ErrStepLimit — execution exceeded the bytecode-step cap.
	ErrStepLimit = errors.New("starlark: execution step limit exceeded")
)

// DefaultMaxSteps bounds bytecode execution when neither Config nor
// the manifest constrains it.
const DefaultMaxSteps uint64 = 100_000_000

// BuiltinProvider turns the granted capability backends into the
// Starlark predeclared globals a module sees. Task 11 leaves it
// nil (deterministic core only); task 12 supplies the real shims.
type BuiltinProvider func(caps map[string]any) (star.StringDict, error)

// Config tunes the runtime.
type Config struct {
	MaxSteps       uint64        // 0 → DefaultMaxSteps
	DefaultTimeout time.Duration // per-call wall clock when the manifest sets none; 0 → no timeout
	Builtins       BuiltinProvider
}

// Runtime is the go.starlark.net-backed loader.Runtime.
type Runtime struct {
	cfg Config
}

var _ loader.Runtime = (*Runtime)(nil)

// New returns a Starlark runtime.
func New(cfg Config) *Runtime {
	if cfg.MaxSteps == 0 {
		cfg.MaxSteps = DefaultMaxSteps
	}
	return &Runtime{cfg: cfg}
}

// strictOptions forbids while / top-level control / global
// reassignment / recursion — bounded, deterministic Starlark.
var strictOptions = &syntax.FileOptions{}

// Init compiles entrypoint and binds the granted capabilities.
func (r *Runtime) Init(_ context.Context, m *manifest.Manifest, entrypoint []byte, caps map[string]any) (loader.Instance, error) {
	predeclared := star.StringDict{}
	if r.cfg.Builtins != nil {
		b, err := r.cfg.Builtins(caps)
		if err != nil {
			return nil, fmt.Errorf("starlark: builtins: %w", err)
		}
		predeclared = b
	}

	thread := &star.Thread{Name: m.Name}
	globals, err := star.ExecFileOptions(strictOptions, thread, m.Entrypoint, entrypoint, predeclared)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCompile, err)
	}
	mainVal, ok := globals["main"]
	if !ok {
		return nil, ErrNoMain
	}
	if _, ok := mainVal.(star.Callable); !ok {
		return nil, fmt.Errorf("%w: `main` is not callable", ErrNoMain)
	}

	timeout := r.cfg.DefaultTimeout
	if m.Limits.Timeout != "" {
		if d, perr := time.ParseDuration(m.Limits.Timeout); perr == nil {
			timeout = d
		}
	}
	return &instance{
		name:     m.Name,
		main:     mainVal,
		maxSteps: r.cfg.MaxSteps,
		timeout:  timeout,
	}, nil
}

// instance is one loaded module.
type instance struct {
	name     string
	main     star.Value
	maxSteps uint64
	timeout  time.Duration
}

// Execute calls main(input) under the step + timeout limits.
func (i *instance) Execute(ctx context.Context, input map[string]any) (*loader.ExecuteResult, error) {
	arg, err := toStarlark(input)
	if err != nil {
		return nil, fmt.Errorf("starlark: input: %w", err)
	}

	var logs []string
	var logMu sync.Mutex
	thread := &star.Thread{
		Name: i.name,
		Print: func(_ *star.Thread, msg string) {
			logMu.Lock()
			logs = append(logs, msg)
			logMu.Unlock()
		},
	}
	var stepHit bool
	thread.SetMaxExecutionSteps(i.maxSteps)
	thread.OnMaxSteps = func(t *star.Thread) {
		stepHit = true
		t.Cancel("step limit")
	}

	cctx := ctx
	cancelTimeout := func() {}
	if i.timeout > 0 {
		cctx, cancelTimeout = context.WithTimeout(ctx, i.timeout)
	}
	defer cancelTimeout()

	type res struct {
		v   star.Value
		err error
	}
	done := make(chan res, 1)
	go func() {
		v, e := star.Call(thread, i.main, star.Tuple{arg}, nil)
		done <- res{v, e}
	}()

	select {
	case <-cctx.Done():
		thread.Cancel("cancelled")
		<-done // let the goroutine unwind
		switch {
		case stepHit:
			return nil, ErrStepLimit
		case errors.Is(cctx.Err(), context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.Canceled):
			return nil, ErrTimeout
		default:
			return nil, ctx.Err()
		}
	case rr := <-done:
		if rr.err != nil {
			if stepHit {
				return nil, ErrStepLimit
			}
			var ee *star.EvalError
			if errors.As(rr.err, &ee) {
				return nil, fmt.Errorf("%w: %s", ErrExec, ee.Backtrace())
			}
			return nil, fmt.Errorf("%w: %v", ErrExec, rr.err)
		}
		out, cerr := fromStarlark(rr.v)
		if cerr != nil {
			return nil, fmt.Errorf("starlark: output: %w", cerr)
		}
		om, _ := out.(map[string]any)
		if om == nil {
			om = map[string]any{"result": out}
		}
		return &loader.ExecuteResult{Output: om, Logs: logs}, nil
	}
}

// Close is a no-op — Starlark holds no OS resources. Idempotent.
func (i *instance) Close() error { return nil }
