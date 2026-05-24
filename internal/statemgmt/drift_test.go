// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"context"
	"errors"
	"testing"
)

// driftSeverityModule wraps scriptModule and additionally implements
// DriftSeverityModule so tests can prove the module-level layer of
// the policy fires when no Params override is set.
type driftSeverityModule struct {
	*scriptModule
	severity DriftSeverity
}

func (m *driftSeverityModule) DriftSeverity(_ *Declaration, _ *ModuleCheckResult) DriftSeverity {
	return m.severity
}

func newDriftSeverityModule(name string, registry *Registry, severity DriftSeverity, opts ...func(*scriptModule)) *driftSeverityModule {
	base := &scriptModule{
		name:        name,
		validStates: []string{"present"},
		checkResult: &ModuleCheckResult{Matches: true},
		applyResult: &StateResult{Success: true},
		testResult:  true,
	}
	for _, opt := range opts {
		opt(base)
	}
	wrapper := &driftSeverityModule{scriptModule: base, severity: severity}
	if err := registry.Register(name, func() Module { return wrapper }); err != nil {
		panic("register: " + err.Error())
	}
	return wrapper
}

// --- DriftSeverity / DriftState formatting ---------------------------

func TestDriftSeverity_String(t *testing.T) {
	t.Parallel()
	cases := map[DriftSeverity]string{
		DriftSeverityNone:     "none",
		DriftSeverityLow:      "low",
		DriftSeverityMedium:   "medium",
		DriftSeverityHigh:     "high",
		DriftSeverityCritical: "critical",
		DriftSeverity(99):     "DriftSeverity(99)",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("DriftSeverity(%d).String() = %q, want %q", int(s), got, want)
		}
	}
}

func TestDriftState_String(t *testing.T) {
	t.Parallel()
	cases := map[DriftState]string{
		DriftStateInSync:  "in-sync",
		DriftStateDrifted: "drifted",
		DriftStateError:   "error",
		DriftStateSkipped: "skipped",
		DriftState(99):    "DriftState(99)",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("DriftState(%d).String() = %q, want %q", int(s), got, want)
		}
	}
}

func TestParseDriftSeverity(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		want DriftSeverity
		ok   bool
	}{
		"low":        {DriftSeverityLow, true},
		"Low":        {DriftSeverityLow, true},
		"  HIGH  ":   {DriftSeverityHigh, true},
		"critical":   {DriftSeverityCritical, true},
		"none":       {DriftSeverityNone, true},
		"medium":     {DriftSeverityMedium, true},
		"nuclear":    {DriftSeverityNone, false},
		"":           {DriftSeverityNone, false},
	}
	for in, want := range cases {
		got, ok := parseDriftSeverity(in)
		if got != want.want || ok != want.ok {
			t.Errorf("parseDriftSeverity(%q) = (%v, %v), want (%v, %v)", in, got, ok, want.want, want.ok)
		}
	}
}

// --- Default severity policy ----------------------------------------

func TestDefaultSeverityResolver_FallbackToMedium(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	mod := newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false, Diff: "x"}
	})
	rep, _ := NewDetector(reg, nil).Detect(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if rep.Statuses[0].Severity != DriftSeverityMedium {
		t.Errorf("Severity = %v, want medium (default)", rep.Statuses[0].Severity)
	}
	if mod.checkCalls.Load() != 1 {
		t.Errorf("Check called %d, want 1", mod.checkCalls.Load())
	}
}

func TestDefaultSeverityResolver_ModuleDefault(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newDriftSeverityModule("file", reg, DriftSeverityHigh, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false, Diff: "x"}
	})
	rep, _ := NewDetector(reg, nil).Detect(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if rep.Statuses[0].Severity != DriftSeverityHigh {
		t.Errorf("Severity = %v, want high (module default)", rep.Statuses[0].Severity)
	}
}

func TestDefaultSeverityResolver_ParamsOverrideBeatsModule(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newDriftSeverityModule("file", reg, DriftSeverityHigh, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
	})
	d := runnerDecl("file", "/a")
	d.Params = map[string]any{"severity": "critical"}
	rep, _ := NewDetector(reg, nil).Detect(context.Background(), []*Declaration{d})
	if rep.Statuses[0].Severity != DriftSeverityCritical {
		t.Errorf("Severity = %v, want critical (Params override > module default)", rep.Statuses[0].Severity)
	}
}

func TestDefaultSeverityResolver_InvalidParamFallsThrough(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newDriftSeverityModule("file", reg, DriftSeverityLow, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
	})
	d := runnerDecl("file", "/a")
	d.Params = map[string]any{"severity": "nuclear"}
	rep, _ := NewDetector(reg, nil).Detect(context.Background(), []*Declaration{d})
	if rep.Statuses[0].Severity != DriftSeverityLow {
		t.Errorf("Severity = %v, want low (invalid Params override falls back to module)", rep.Statuses[0].Severity)
	}
}

func TestDefaultSeverityResolver_NonStringParamIgnored(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
	})
	d := runnerDecl("file", "/a")
	d.Params = map[string]any{"severity": 42} // non-string ignored
	rep, _ := NewDetector(reg, nil).Detect(context.Background(), []*Declaration{d})
	if rep.Statuses[0].Severity != DriftSeverityMedium {
		t.Errorf("Severity = %v, want medium (non-string ignored, fallthrough)", rep.Statuses[0].Severity)
	}
}

// --- Detector outcomes ----------------------------------------------

func TestDetector_Empty(t *testing.T) {
	t.Parallel()
	rep, err := NewDetector(NewRegistry(), nil).Detect(context.Background(), nil)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if rep.TotalChecked != 0 || rep.AggregateSeverity != DriftSeverityNone {
		t.Errorf("Aggregate=%v TotalChecked=%d, want none/0", rep.AggregateSeverity, rep.TotalChecked)
	}
}

func TestDetector_InSync_NoDrift(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: true}
	})
	rep, err := NewDetector(reg, nil).Detect(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if rep.InSync != 1 || rep.Drifted != 0 {
		t.Errorf("InSync=%d Drifted=%d, want 1/0", rep.InSync, rep.Drifted)
	}
	if rep.AggregateSeverity != DriftSeverityNone {
		t.Errorf("AggregateSeverity = %v, want none (no drift)", rep.AggregateSeverity)
	}
	if rep.Statuses[0].State != DriftStateInSync {
		t.Errorf("State = %v, want in-sync", rep.Statuses[0].State)
	}
	if rep.Statuses[0].Severity != DriftSeverityNone {
		t.Errorf("Severity = %v, want none (in-sync has no severity)", rep.Statuses[0].Severity)
	}
}

func TestDetector_Drifted(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false, Diff: "mode 0644 → 0600"}
	})
	rep, _ := NewDetector(reg, nil).Detect(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if rep.Drifted != 1 {
		t.Errorf("Drifted = %d, want 1", rep.Drifted)
	}
	if rep.Statuses[0].State != DriftStateDrifted {
		t.Errorf("State = %v, want drifted", rep.Statuses[0].State)
	}
	if rep.Statuses[0].Diff != "mode 0644 → 0600" {
		t.Errorf("Diff = %q, want full Diff carried through from Check", rep.Statuses[0].Diff)
	}
	if rep.AggregateSeverity != DriftSeverityMedium {
		t.Errorf("AggregateSeverity = %v, want medium (default)", rep.AggregateSeverity)
	}
}

func TestDetector_CheckError(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkErr = errors.New("io")
	})
	rep, err := NewDetector(reg, nil).Detect(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if err == nil {
		t.Fatal("expected error from failing Check")
	}
	if rep.Errors != 1 {
		t.Errorf("Errors = %d, want 1", rep.Errors)
	}
	if rep.Statuses[0].State != DriftStateError {
		t.Errorf("State = %v, want error", rep.Statuses[0].State)
	}
	if rep.Statuses[0].Error == nil {
		t.Error("Error field should be populated for error state")
	}
}

func TestDetector_CascadeSkip(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("good", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: true}
	})
	newScriptModule("bad", reg, func(m *scriptModule) {
		m.checkErr = errors.New("nope")
	})
	rep, _ := NewDetector(reg, nil).Detect(context.Background(), []*Declaration{
		runnerDecl("good", "/a"),
		runnerDecl("bad", "/b"),
		runnerDecl("good", "/c"),
	})
	if rep.InSync != 1 || rep.Errors != 1 || rep.Skipped != 1 {
		t.Errorf("counts InSync=%d Errors=%d Skipped=%d, want 1/1/1", rep.InSync, rep.Errors, rep.Skipped)
	}
	if rep.Statuses[2].State != DriftStateSkipped {
		t.Errorf("Statuses[2].State = %v, want skipped", rep.Statuses[2].State)
	}
}

// --- Aggregation -----------------------------------------------------

func TestDetector_AggregateSeverity_MaxAcrossDrifted(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
	})
	low := runnerDecl("file", "/low")
	low.Params = map[string]any{"severity": "low"}
	high := runnerDecl("file", "/high")
	high.Params = map[string]any{"severity": "high"}
	ok := runnerDecl("file", "/ok")
	// Have ok match — Detector should only see the other two as drifted.
	// Use a separate module name so we don't fight over a single
	// scriptModule's Check result.
	newScriptModule("okmod", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: true}
	})
	ok.Module = "okmod"
	ok.ID = "okmod:/ok"
	rep, _ := NewDetector(reg, nil).Detect(context.Background(), []*Declaration{low, high, ok})
	if rep.AggregateSeverity != DriftSeverityHigh {
		t.Errorf("AggregateSeverity = %v, want high (max across drifted)", rep.AggregateSeverity)
	}
	if rep.InSync != 1 || rep.Drifted != 2 {
		t.Errorf("counts InSync=%d Drifted=%d, want 1/2", rep.InSync, rep.Drifted)
	}
}

func TestDetector_AggregateSeverity_NoneWhenAllInSync(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: true}
	})
	rep, _ := NewDetector(reg, nil).Detect(context.Background(), []*Declaration{
		runnerDecl("file", "/a"),
		runnerDecl("file", "/b"),
	})
	if rep.AggregateSeverity != DriftSeverityNone {
		t.Errorf("AggregateSeverity = %v, want none (all in-sync)", rep.AggregateSeverity)
	}
}

// --- Robustness ------------------------------------------------------

func TestDetector_NilRegistry_FallsBackToDefault(t *testing.T) {
	// Not parallel: touches DefaultRegistry.
	name := fakeDefaultModule(t)
	d := NewDetector(nil, nil) // nil → DefaultRegistry
	rep, err := d.Detect(context.Background(), []*Declaration{runnerDecl(name, "/x")})
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if rep.InSync != 1 {
		t.Errorf("InSync = %d, want 1", rep.InSync)
	}
}

func TestDetector_NilObserver_NoPanic(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg)
	if _, err := NewDetector(reg, nil).Detect(context.Background(), []*Declaration{runnerDecl("file", "/a")}); err != nil {
		t.Fatalf("Detect: %v", err)
	}
}

func TestDetector_NilResolver_FallsBackToDefault(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("file", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
	})
	d := NewDetector(reg, nil)
	d.SeverityResolver = nil // explicit
	rep, _ := d.Detect(context.Background(), []*Declaration{runnerDecl("file", "/a")})
	if rep.Statuses[0].Severity != DriftSeverityMedium {
		t.Errorf("Severity = %v, want medium (default resolver)", rep.Statuses[0].Severity)
	}
}

// --- helpers --------------------------------------------------------

// fakeDefaultModule registers an in-sync scriptModule in DefaultRegistry
// under a unique name and arranges cleanup. Returns the module name.
func fakeDefaultModule(t *testing.T) string {
	t.Helper()
	name := "drift-default-" + driftCounter.nextString()
	mod := &scriptModule{
		name:        name,
		validStates: []string{"present"},
		checkResult: &ModuleCheckResult{Matches: true},
	}
	if err := RegisterModule(name, func() Module { return mod }); err != nil {
		t.Fatalf("RegisterModule: %v", err)
	}
	t.Cleanup(func() {
		DefaultRegistry.mu.Lock()
		delete(DefaultRegistry.factories, name)
		DefaultRegistry.mu.Unlock()
	})
	return name
}

// driftCounter is independent from testCounter so parallel module
// registrations across packages cannot collide.
var driftCounter = &driftCounterT{}

type driftCounterT struct {
	c counter
}

func (d *driftCounterT) nextString() string {
	return intToStr(d.c.next())
}

func intToStr(i int) string {
	// avoid pulling strconv into a tiny helper.
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
