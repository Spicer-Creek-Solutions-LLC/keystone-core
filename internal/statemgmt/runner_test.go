// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// scriptModule is a Module whose Check / Apply / Test outcomes are
// driven entirely by test setup. Each call counter helps tests prove
// the runner skipped (or invoked) a phase as expected.
type scriptModule struct {
	name        string
	validStates []string

	checkResult *ModuleCheckResult
	checkErr    error
	checkCalls  atomic.Int64
	checkBlock  chan struct{} // optional: blocks Check until closed

	applyResult *StateResult
	applyErr    error
	applyCalls  atomic.Int64

	testResult bool
	testErr    error
	testCalls  atomic.Int64
}

func (m *scriptModule) Name() string          { return m.name }
func (m *scriptModule) ValidStates() []string { return m.validStates }
func (m *scriptModule) Check(ctx context.Context, decl *Declaration) (*ModuleCheckResult, error) {
	m.checkCalls.Add(1)
	if m.checkBlock != nil {
		select {
		case <-m.checkBlock:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return m.checkResult, m.checkErr
}
func (m *scriptModule) Apply(ctx context.Context, decl *Declaration) (*StateResult, error) {
	m.applyCalls.Add(1)
	return m.applyResult, m.applyErr
}
func (m *scriptModule) Test(ctx context.Context, decl *Declaration) (bool, error) {
	m.testCalls.Add(1)
	return m.testResult, m.testErr
}

func scriptFactory(m *scriptModule) Factory { return func() Module { return m } }

// recorderObserver captures the order of observer calls so tests can
// assert sequences like "start, drift, change, done".
type recorderObserver struct {
	mu     sync.Mutex
	events []string
}

func (o *recorderObserver) push(s string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, s)
}
func (o *recorderObserver) snapshot() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.events...)
}

func (o *recorderObserver) Start(_ context.Context, decl *Declaration) {
	o.push("start:" + decl.ID)
}
func (o *recorderObserver) Drift(_ context.Context, decl *Declaration, _ *ModuleCheckResult) {
	o.push("drift:" + decl.ID)
}
func (o *recorderObserver) Change(_ context.Context, decl *Declaration, _ *StateResult) {
	o.push("change:" + decl.ID)
}
func (o *recorderObserver) Done(_ context.Context, res *DeclarationResult) {
	o.push("done:" + res.DeclID + ":" + res.Outcome.String())
}
func (o *recorderObserver) Skip(_ context.Context, decl *Declaration, _ error) {
	o.push("skip:" + decl.ID)
}

func newScriptModule(name string, registry *Registry, opts ...func(*scriptModule)) *scriptModule {
	m := &scriptModule{
		name:        name,
		validStates: []string{"present"},
		checkResult: &ModuleCheckResult{Matches: true},
		applyResult: &StateResult{Success: true},
		testResult:  true,
	}
	for _, opt := range opts {
		opt(m)
	}
	if err := registry.Register(name, scriptFactory(m)); err != nil {
		panic("scriptModule register: " + err.Error())
	}
	return m
}

func runnerDecl(module, name string) *Declaration {
	return &Declaration{
		ID:     module + ":" + name,
		Module: module,
		Name:   name,
		State:  "present",
	}
}

// --- Apply mode -------------------------------------------------------

func TestRunner_Empty(t *testing.T) {
	t.Parallel()
	r := NewRunner(NewRegistry(), nil)
	rep, err := r.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run(nil): %v", err)
	}
	if rep.Total != 0 || len(rep.Results) != 0 {
		t.Errorf("Total=%d, len(Results)=%d, want 0/0", rep.Total, len(rep.Results))
	}
}

func TestRunner_Apply_CheckMatches_SkipsApplyAndTest(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	mod := newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: true}
	})
	obs := &recorderObserver{}
	r := NewRunner(reg, obs)

	rep, err := r.Run(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Results[0].Outcome != OutcomeUnchanged {
		t.Errorf("Outcome = %v, want Unchanged", rep.Results[0].Outcome)
	}
	if mod.applyCalls.Load() != 0 {
		t.Errorf("Apply called %d times, want 0 (Check matched)", mod.applyCalls.Load())
	}
	if mod.testCalls.Load() != 0 {
		t.Errorf("Test called %d times, want 0 (Apply skipped)", mod.testCalls.Load())
	}
	wantEvents := []string{"start:file:/a", "done:file:/a:unchanged"}
	assertEvents(t, obs, wantEvents)
}

func TestRunner_Apply_Drift_AppliesAndTests(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	mod := newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false, Diff: "mode 0600 → 0644"}
		m.applyResult = &StateResult{Success: true, Changed: true, Diff: "mode 0600 → 0644"}
		m.testResult = true
	})
	obs := &recorderObserver{}
	r := NewRunner(reg, obs)
	rep, err := r.Run(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if mod.applyCalls.Load() != 1 || mod.testCalls.Load() != 1 {
		t.Errorf("expected Apply+Test once each, got apply=%d test=%d", mod.applyCalls.Load(), mod.testCalls.Load())
	}
	if rep.Results[0].Outcome != OutcomeChanged {
		t.Errorf("Outcome = %v, want Changed", rep.Results[0].Outcome)
	}
	assertEvents(t, obs, []string{
		"start:file:/a",
		"drift:file:/a",
		"change:file:/a",
		"done:file:/a:changed",
	})
}

func TestRunner_Apply_AppliedButNotChanged_IsNoOp(t *testing.T) {
	t.Parallel()
	// Module reports drift in Check but Apply runs to completion
	// with Changed=false (e.g., a module that idempotently
	// converges and discovers nothing needed doing).
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false, Diff: "drift"}
		m.applyResult = &StateResult{Success: true, Changed: false}
		m.testResult = true
	})
	obs := &recorderObserver{}
	r := NewRunner(reg, obs)
	rep, _ := r.Run(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if rep.Results[0].Outcome != OutcomeNoOp {
		t.Errorf("Outcome = %v, want NoOp", rep.Results[0].Outcome)
	}
	// Change observer should NOT fire when there was no actual change.
	for _, e := range obs.snapshot() {
		if strings.HasPrefix(e, "change:") {
			t.Errorf("Change observer fired despite Changed=false: %v", obs.snapshot())
		}
	}
}

func TestRunner_Apply_CheckError(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	mod := newScriptModule("file", reg, func(m *scriptModule) {
		m.checkErr = errors.New("disk unreachable")
	})
	r := NewRunner(reg, nil)
	rep, err := r.Run(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if err == nil {
		t.Fatal("expected error from failed Check")
	}
	if !strings.Contains(err.Error(), "disk unreachable") {
		t.Errorf("err = %v, want underlying message", err)
	}
	if rep.Results[0].Outcome != OutcomeFailed {
		t.Errorf("Outcome = %v, want Failed", rep.Results[0].Outcome)
	}
	if mod.applyCalls.Load() != 0 || mod.testCalls.Load() != 0 {
		t.Errorf("Apply+Test must be skipped after Check failure (apply=%d test=%d)", mod.applyCalls.Load(), mod.testCalls.Load())
	}
}

func TestRunner_Apply_ApplyError(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	mod := newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyErr = errors.New("permission denied")
	})
	r := NewRunner(reg, nil)
	rep, err := r.Run(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if err == nil {
		t.Fatal("expected error from failed Apply")
	}
	if rep.Results[0].Outcome != OutcomeFailed {
		t.Errorf("Outcome = %v, want Failed", rep.Results[0].Outcome)
	}
	if mod.testCalls.Load() != 0 {
		t.Errorf("Test must be skipped after Apply failure (test=%d)", mod.testCalls.Load())
	}
}

func TestRunner_Apply_TestReturnsFalse_IsFailure(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: true}
		m.testResult = false
	})
	r := NewRunner(reg, nil)
	rep, err := r.Run(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if err == nil {
		t.Fatal("expected error when Test returns false")
	}
	if rep.Results[0].Outcome != OutcomeFailed {
		t.Errorf("Outcome = %v, want Failed", rep.Results[0].Outcome)
	}
	if !strings.Contains(err.Error(), "Test returned false") {
		t.Errorf("err = %v, want \"Test returned false\" message", err)
	}
}

func TestRunner_Apply_TestError(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: true}
		m.testErr = errors.New("post-check unreachable")
	})
	r := NewRunner(reg, nil)
	rep, _ := r.Run(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if rep.Results[0].Outcome != OutcomeFailed {
		t.Errorf("Outcome = %v, want Failed", rep.Results[0].Outcome)
	}
}

// --- Multi-decl + cascade ---------------------------------------------

func TestRunner_MultiDecl_AllOK_PreservesOrder(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: true}
	})
	obs := &recorderObserver{}
	r := NewRunner(reg, obs)
	decls := []*Declaration{
		runnerDecl("file", "/a"),
		runnerDecl("file", "/b"),
		runnerDecl("file", "/c"),
	}
	rep, err := r.Run(context.Background(), decls)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 3 || rep.Unchanged != 3 {
		t.Errorf("Total=%d Unchanged=%d, want 3 3", rep.Total, rep.Unchanged)
	}
	wantOrder := []string{
		"start:file:/a", "done:file:/a:unchanged",
		"start:file:/b", "done:file:/b:unchanged",
		"start:file:/c", "done:file:/c:unchanged",
	}
	assertEvents(t, obs, wantOrder)
}

func TestRunner_MultiDecl_FailureCascadesAsSkip(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	// Register two modules so we can distinguish which one is
	// invoked. "good" succeeds; "bad" fails Check.
	good := newScriptModule("good", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: true}
	})
	bad := newScriptModule("bad", reg, func(m *scriptModule) {
		m.checkErr = errors.New("nope")
	})
	obs := &recorderObserver{}
	r := NewRunner(reg, obs)
	decls := []*Declaration{
		runnerDecl("good", "/a"),
		runnerDecl("bad", "/b"),
		runnerDecl("good", "/c"),
	}
	rep, err := r.Run(context.Background(), decls)
	if err == nil {
		t.Fatal("expected error from failing decl")
	}
	if rep.Failed != 1 {
		t.Errorf("Failed = %d, want 1", rep.Failed)
	}
	if rep.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (decl after the failure)", rep.Skipped)
	}
	if rep.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1 (decl before the failure)", rep.Unchanged)
	}
	// good is called once for /a (it never runs /c because of the skip).
	if good.checkCalls.Load() != 1 {
		t.Errorf("good.checkCalls = %d, want 1 (skipped after failure)", good.checkCalls.Load())
	}
	if bad.checkCalls.Load() != 1 {
		t.Errorf("bad.checkCalls = %d, want 1", bad.checkCalls.Load())
	}

	// Verify the cascaded Skip carries the originating error.
	skipped := rep.Results[2]
	if skipped.Outcome != OutcomeSkipped {
		t.Errorf("Results[2].Outcome = %v, want Skipped", skipped.Outcome)
	}
	if !strings.Contains(skipped.Error.Error(), "nope") {
		t.Errorf("Results[2].Error = %v, want it to carry the originating error", skipped.Error)
	}

	// Observer must have fired Skip for the cascaded decl — the
	// event stream is the canonical surface external subscribers
	// rely on, so it can't be silent.
	events := obs.snapshot()
	foundSkip := false
	for _, e := range events {
		if e == "skip:good:/c" {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Errorf("Skip observer never fired for cascaded decl; events: %v", events)
	}
}

// --- Check (dry-run) mode --------------------------------------------

func TestRunner_Check_NoApplyEverInvoked(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	mod := newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
	})
	obs := &recorderObserver{}
	r := NewRunner(reg, obs)
	rep, err := r.Check(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if mod.applyCalls.Load() != 0 || mod.testCalls.Load() != 0 {
		t.Errorf("Apply/Test invoked in Check mode: apply=%d test=%d", mod.applyCalls.Load(), mod.testCalls.Load())
	}
	if rep.Mode != ModeCheck {
		t.Errorf("Mode = %v, want check", rep.Mode)
	}
	if rep.Results[0].Outcome != OutcomeDriftDetected {
		t.Errorf("Outcome = %v, want DriftDetected", rep.Results[0].Outcome)
	}
	if rep.Drifted != 1 {
		t.Errorf("Drifted = %d, want 1", rep.Drifted)
	}
	// Drift event must fire in Check mode.
	hasDrift := false
	for _, e := range obs.snapshot() {
		if e == "drift:file:/a" {
			hasDrift = true
		}
	}
	if !hasDrift {
		t.Errorf("Drift event not observed in Check mode: %v", obs.snapshot())
	}
}

func TestRunner_Check_NoDrift_OutcomeUnchanged(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: true}
	})
	obs := &recorderObserver{}
	r := NewRunner(reg, obs)
	rep, _ := r.Check(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if rep.Results[0].Outcome != OutcomeUnchanged {
		t.Errorf("Outcome = %v, want Unchanged", rep.Results[0].Outcome)
	}
	for _, e := range obs.snapshot() {
		if strings.HasPrefix(e, "drift:") {
			t.Errorf("Drift event fired despite Check matching: %v", obs.snapshot())
		}
	}
}

func TestRunner_Check_CheckError_Cascades(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkErr = errors.New("io")
	})
	r := NewRunner(reg, nil)
	rep, err := r.Check(context.Background(), []*Declaration{
		runnerDecl("file", "/a"),
		runnerDecl("file", "/b"),
	})
	if err == nil {
		t.Fatal("expected error from Check phase")
	}
	if rep.Failed != 1 || rep.Skipped != 1 {
		t.Errorf("Failed=%d Skipped=%d, want 1 1", rep.Failed, rep.Skipped)
	}
}

// --- Edge cases ------------------------------------------------------

func TestRunner_NilDeclSkipped(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: true}
	})
	r := NewRunner(reg, nil)
	rep, err := r.Run(context.Background(), []*Declaration{nil, runnerDecl("file", "/a"), nil})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 1 {
		t.Errorf("Total = %d, want 1 (nils silently dropped)", rep.Total)
	}
}

func TestRunner_UnknownModule_IsFailure(t *testing.T) {
	t.Parallel()
	r := NewRunner(NewRegistry(), nil)
	rep, err := r.Run(context.Background(), []*Declaration{runnerDecl("ghost", "/a")})
	if err == nil {
		t.Fatal("expected error for unknown module")
	}
	if rep.Results[0].Outcome != OutcomeFailed {
		t.Errorf("Outcome = %v, want Failed", rep.Results[0].Outcome)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("err = %v, want module name cited", err)
	}
}

func TestRunner_NilObserver_NoPanic(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg)
	r := NewRunner(reg, nil) // explicitly nil observer
	if _, err := r.Run(context.Background(), []*Declaration{runnerDecl("file", "/a")}); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunner_NilRegistry_FallsBackToDefault(t *testing.T) {
	// Not parallel: mutates DefaultRegistry.
	name := fmt.Sprintf("runner-default-%d", testCounter.next())
	mod := &scriptModule{
		name:        name,
		validStates: []string{"present"},
		checkResult: &ModuleCheckResult{Matches: true},
	}
	if err := RegisterModule(name, scriptFactory(mod)); err != nil {
		t.Fatalf("RegisterModule: %v", err)
	}
	t.Cleanup(func() {
		DefaultRegistry.mu.Lock()
		delete(DefaultRegistry.factories, name)
		DefaultRegistry.mu.Unlock()
	})
	r := NewRunner(nil, nil) // nil registry → DefaultRegistry
	if _, err := r.Run(context.Background(), []*Declaration{runnerDecl(name, "/x")}); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunner_ContextCancellation_CascadesAsSkip(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	gate := make(chan struct{})
	mod := newScriptModule("slow", reg, func(m *scriptModule) {
		m.checkBlock = gate
		m.checkResult = &ModuleCheckResult{Matches: true}
	})
	r := NewRunner(reg, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		rep *RunReport
		err error
	}, 1)
	go func() {
		rep, err := r.Run(ctx, []*Declaration{
			runnerDecl("slow", "/a"),
			runnerDecl("slow", "/b"),
		})
		done <- struct {
			rep *RunReport
			err error
		}{rep, err}
	}()

	// Wait until the runner is inside Check, then cancel + unblock.
	for mod.checkCalls.Load() < 1 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	close(gate)
	result := <-done
	if result.err == nil || !errors.Is(result.err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", result.err)
	}
	// First decl completed; second decl is cascade-skipped.
	if result.rep.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.rep.Skipped)
	}
}

func TestRunner_DeclTimeout(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	gate := make(chan struct{}) // never closed in this test
	newScriptModule("slow", reg, func(m *scriptModule) {
		m.checkBlock = gate
	})
	r := NewRunner(reg, nil)
	r.DeclTimeout = 50 * time.Millisecond

	rep, err := r.Run(context.Background(), []*Declaration{runnerDecl("slow", "/a")})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
	if rep.Results[0].Outcome != OutcomeFailed {
		t.Errorf("Outcome = %v, want Failed", rep.Results[0].Outcome)
	}
}

func TestOutcome_StringFormatting(t *testing.T) {
	t.Parallel()
	cases := map[Outcome]string{
		OutcomeUnchanged:     "unchanged",
		OutcomeChanged:       "changed",
		OutcomeNoOp:          "no-op",
		OutcomeFailed:        "failed",
		OutcomeDriftDetected: "drift-detected",
		OutcomeSkipped:       "skipped",
		Outcome(99):          "Outcome(99)",
	}
	for o, want := range cases {
		if got := o.String(); got != want {
			t.Errorf("Outcome(%d).String() = %q, want %q", int(o), got, want)
		}
	}
}

func TestRunReport_Aggregates(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	// "okmatch" → Unchanged
	newScriptModule("okmatch", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: true}
	})
	// "okchange" → Changed
	newScriptModule("okchange", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: true}
		m.testResult = true
	})
	// "okidem" → NoOp (Apply runs but reports no change)
	newScriptModule("okidem", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: false}
		m.testResult = true
	})
	r := NewRunner(reg, nil)
	rep, err := r.Run(context.Background(), []*Declaration{
		runnerDecl("okmatch", "/a"),
		runnerDecl("okchange", "/b"),
		runnerDecl("okidem", "/c"),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Total != 3 || rep.Changed != 1 || rep.Unchanged != 2 || rep.Failed != 0 || rep.Skipped != 0 {
		t.Errorf("aggregates = %+v, want Total=3 Changed=1 Unchanged=2", rep)
	}
}

// --- helpers ---------------------------------------------------------

func assertEvents(t *testing.T, obs *recorderObserver, want []string) {
	t.Helper()
	got := obs.snapshot()
	if len(got) != len(want) {
		t.Fatalf("event count = %d, want %d (got %v, want %v)", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("events[%d] = %q, want %q (full got: %v)", i, got[i], want[i], got)
		}
	}
}
