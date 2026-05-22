// Package moduletest is the Epic 14 task-15 Starlark module
// unit-test runner. It fills the internal/cli/module.TestRunner
// seam: `kscore-module test` discovers a module's `*_test.star`
// files, runs every top-level `test_*` function with the module's
// own functions, the capability SDK builtins, and an `assert`
// namespace in scope, and reports pass/fail.
//
// Import path is pkg/module/testing (the epic-mandated location)
// but the package is named moduletest so importers do not shadow
// the standard library `testing` package (the pkg/module/audit ->
// maudit aliasing precedent).
//
// Tests run under the same strict, deterministic Starlark file
// options and the same step + wall-clock watchdog as the task-11
// runtime: a runaway or non-terminating test is bounded, since the
// runner executes untrusted module code.
package moduletest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	star "go.starlark.net/starlark"
	"go.starlark.net/syntax"

	starlarksdk "go.keystone-core.io/keystone-core/modules/sdk/starlark"
	"go.keystone-core.io/keystone-core/pkg/module/capability"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	srt "go.keystone-core.io/keystone-core/pkg/module/runtime/starlark"
)

const (
	testFileSuffix = "_test.star"
	testFuncPrefix = "test_"
)

// strictOptions mirrors the task-11 runtime: forbids `while`,
// top-level control flow, global reassignment, and recursion —
// bounded, deterministic Starlark by construction.
var strictOptions = &syntax.FileOptions{}

var (
	// ErrManifest — the module's manifest is missing or invalid.
	ErrManifest = errors.New("moduletest: invalid module manifest")
	// ErrEntrypoint — the module entrypoint failed to compile.
	ErrEntrypoint = errors.New("moduletest: entrypoint compile failed")
	// ErrTestFile — a *_test.star file failed to read/compile.
	ErrTestFile = errors.New("moduletest: test file failed")
	// ErrAssertion — a test function returned a Starlark error
	// (a failed assertion or a runtime error).
	ErrAssertion = errors.New("moduletest: test failed")
	// ErrStepLimit — a test exceeded the bytecode-step cap.
	ErrStepLimit = errors.New("moduletest: test exceeded step limit")
	// ErrTimeout — a test exceeded its wall-clock budget.
	ErrTimeout = errors.New("moduletest: test timed out")
	// ErrAuditOption — an --audit-level / --audit-output value
	// was not recognised.
	ErrAuditOption = errors.New("moduletest: invalid audit option")
)

// Config tunes the per-test execution bounds (the task-11 runtime
// knobs).
type Config struct {
	MaxSteps       uint64        // 0 -> srt.DefaultMaxSteps
	DefaultTimeout time.Duration // per-test wall clock when the manifest sets none; 0 -> none
}

func (c Config) maxSteps() uint64 {
	if c.MaxSteps == 0 {
		return srt.DefaultMaxSteps
	}
	return c.MaxSteps
}

// Options configures a test run.
type Options struct {
	Config Config
	Audit  AuditOptions
	// Hosts overrides the default test capability hosts (os-backed
	// fs + discard logger; http/exec/secrets fail closed). When
	// nil, the defaults are used.
	Hosts *capability.Hosts
}

// Result is one executed test function (or a file-level
// read/compile failure, with Name "<read>"/"<compile>").
type Result struct {
	File     string
	Name     string
	Passed   bool
	Err      error
	Logs     []string
	Duration time.Duration
}

// Report aggregates a run.
type Report struct {
	Results []Result
	Passed  int
	Failed  int
}

func (r *Report) add(res Result) {
	r.Results = append(r.Results, res)
	if res.Passed {
		r.Passed++
	} else {
		r.Failed++
	}
}

// Run discovers and executes the module's unit tests.
//
// A missing/invalid manifest, a non-buildable capability scope, or
// a module entrypoint that does not compile is a hard error (a
// broken module cannot be tested). A *_test.star that fails to
// read or compile is recorded as a single failed Result so the
// other files still run; a test function that errors is a failed
// Result. Infrastructure is distinguished from test failure: Run
// returns (report, nil) whenever the harness itself ran, even if
// tests failed — the caller decides exit status from report.Failed.
func Run(ctx context.Context, moduleDir string, opts Options) (*Report, error) {
	m, src, err := loadModule(moduleDir)
	if err != nil {
		return nil, err
	}

	hosts := defaultHosts()
	if opts.Hosts != nil {
		hosts = *opts.Hosts
	}
	caps, err := capability.BuildCapabilities(m, hosts)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	reg, err := capability.NewRegistryFromManifest(m)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	auditor, closeAudit, err := newAuditor(opts.Audit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeAudit() }()
	inv := capability.NewInvoker(reg, auditor)
	builtins, err := starlarksdk.BuildStringDict(caps, inv)
	if err != nil {
		return nil, err
	}

	modGlobals, err := star.ExecFileOptions(
		strictOptions, &star.Thread{Name: m.Name}, m.Entrypoint, src, builtins)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEntrypoint, err)
	}

	files, err := filepath.Glob(filepath.Join(moduleDir, "*"+testFileSuffix))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	timeout := opts.Config.DefaultTimeout
	if m.Limits.Timeout != "" {
		if d, perr := time.ParseDuration(m.Limits.Timeout); perr == nil {
			timeout = d
		}
	}

	rep := &Report{}
	for _, f := range files {
		base := filepath.Base(f)
		fsrc, rerr := os.ReadFile(f) //nolint:gosec // G304: *_test.star inside the operator-supplied module dir
		if rerr != nil {
			rep.add(Result{File: base, Name: "<read>", Err: fmt.Errorf("%w: %v", ErrTestFile, rerr)})
			continue
		}
		pre := mergeDicts(builtins, modGlobals)
		pre["assert"] = assertNS
		tGlobals, eerr := star.ExecFileOptions(
			strictOptions, &star.Thread{Name: m.Name + "/" + base}, base, fsrc, pre)
		if eerr != nil {
			rep.add(Result{File: base, Name: "<compile>", Err: fmt.Errorf("%w: %v", ErrTestFile, eerr)})
			continue
		}
		for _, tn := range testFuncs(tGlobals) {
			logs, dur, terr := opts.Config.runOne(
				ctx, m.Name+"/"+base+"/"+tn, tGlobals[tn], timeout)
			rep.add(Result{
				File: base, Name: tn,
				Passed: terr == nil, Err: terr, Logs: logs, Duration: dur,
			})
		}
	}
	return rep, nil
}

// loadModule reads + validates manifest.yaml and reads the
// entrypoint source.
func loadModule(dir string) (*manifest.Manifest, []byte, error) {
	my, err := os.ReadFile(filepath.Join(dir, "manifest.yaml")) //nolint:gosec // G304: operator-supplied module dir
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	m, err := manifest.UnmarshalManifest(my)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	if err := m.Validate(); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrManifest, err)
	}
	// #nosec G304 G703 -- manifest-declared entrypoint joined under the
	// caller-supplied module dir; this is the module loader's contract.
	src, err := os.ReadFile(filepath.Join(dir, m.Entrypoint)) //nolint:gosec
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrEntrypoint, err)
	}
	return m, src, nil
}

// mergeDicts builds a fresh predeclared environment: SDK capability
// builtins, then the module's own globals (a module-defined name
// shadows a builtin — the module's choice). Neither input is
// mutated (both are reused across files).
func mergeDicts(builtins, modGlobals star.StringDict) star.StringDict {
	out := make(star.StringDict, len(builtins)+len(modGlobals)+1)
	for k, v := range builtins {
		out[k] = v
	}
	for k, v := range modGlobals {
		out[k] = v
	}
	return out
}

// testFuncs returns the sorted names of the callable test_*
// globals.
func testFuncs(g star.StringDict) []string {
	var names []string
	for name, v := range g {
		if !strings.HasPrefix(name, testFuncPrefix) {
			continue
		}
		if _, ok := v.(star.Callable); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// runOne executes a single test function on its own bounded thread
// (mirrors the task-11 runtime watchdog: step cap + ctx/timeout
// cancel). Test functions take no arguments.
func (c Config) runOne(
	ctx context.Context, name string, fn star.Value, timeout time.Duration,
) (logs []string, dur time.Duration, err error) {
	var logMu sync.Mutex
	thread := &star.Thread{
		Name: name,
		Print: func(_ *star.Thread, msg string) {
			logMu.Lock()
			logs = append(logs, msg)
			logMu.Unlock()
		},
	}
	var stepHit bool
	thread.SetMaxExecutionSteps(c.maxSteps())
	thread.OnMaxSteps = func(t *star.Thread) {
		stepHit = true
		t.Cancel("step limit")
	}

	cctx := ctx
	cancel := func() {}
	if timeout > 0 {
		cctx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	start := time.Now()
	done := make(chan error, 1)
	go func() {
		_, e := star.Call(thread, fn, nil, nil)
		done <- e
	}()

	select {
	case <-cctx.Done():
		thread.Cancel("cancelled")
		<-done // let the goroutine unwind
		dur = time.Since(start)
		switch {
		case stepHit:
			return logs, dur, ErrStepLimit
		case errors.Is(cctx.Err(), context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.Canceled):
			return logs, dur, ErrTimeout
		default:
			return logs, dur, ctx.Err()
		}
	case e := <-done:
		dur = time.Since(start)
		if e == nil {
			return logs, dur, nil
		}
		if stepHit {
			return logs, dur, ErrStepLimit
		}
		var ee *star.EvalError
		if errors.As(e, &ee) {
			return logs, dur, fmt.Errorf("%w: %s", ErrAssertion, ee.Backtrace())
		}
		return logs, dur, fmt.Errorf("%w: %v", ErrAssertion, e)
	}
}
