// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

// fakeHistory is a [state.StateHistoryStore] that returns a canned
// list from ListStateRuns. Only the methods RunSaga's lookup
// touches are implemented; the rest panic so accidental new uses
// surface loudly in tests.
type fakeHistory struct {
	runs    []*state.StateRunRecord
	listErr error
}

func (f *fakeHistory) ListStateRuns(_ context.Context, _ state.StateRunFilter) ([]*state.StateRunRecord, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*state.StateRunRecord, len(f.runs))
	copy(out, f.runs)
	return out, nil
}

func (f *fakeHistory) CreateStateRun(context.Context, *state.StateRunRecord) error {
	panic("CreateStateRun not used by RunSaga")
}
func (f *fakeHistory) FinalizeStateRun(context.Context, string, state.StateRunEnd) error {
	panic("FinalizeStateRun not used by RunSaga")
}
func (f *fakeHistory) AddStateRunResult(context.Context, string, *state.StateRunResultRecord) error {
	panic("AddStateRunResult not used by RunSaga")
}
func (f *fakeHistory) GetStateRun(context.Context, string) (*state.StateRunRecord, []*state.StateRunResultRecord, error) {
	panic("GetStateRun not used by RunSaga")
}
func (f *fakeHistory) DeleteStateRunsBefore(context.Context, time.Time, []state.StateRunStatus) (int, error) {
	panic("DeleteStateRunsBefore not used by RunSaga")
}

// historicalRun builds a StateRunRecord whose DeclarationsJSON
// encodes the given decls. Tests use it to seed the rollback
// lookup.
func historicalRun(t *testing.T, decls ...*Declaration) *state.StateRunRecord {
	t.Helper()
	b, err := json.Marshal(decls)
	if err != nil {
		t.Fatalf("marshal decls: %v", err)
	}
	return &state.StateRunRecord{
		ID:               "run-" + decls[0].ID,
		Status:           state.StateRunStatusCompleted,
		StartedAt:        time.Now().Add(-time.Hour),
		EndedAt:          time.Now().Add(-time.Hour).Add(time.Second),
		DeclarationsJSON: string(b),
	}
}

// --- RunSaga: happy path --------------------------------------------

func TestRunSaga_HappyPath_NoCompensation(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	a := newScriptModule("a", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: true}
	})
	b := newScriptModule("b", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: true}
	})

	runner := NewRunner(reg, nil)
	report, err := runner.RunSaga(context.Background(),
		[]*Declaration{runnerDecl("a", "x"), runnerDecl("b", "y")},
		SagaConfig{History: &fakeHistory{}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if report.Total != 2 || report.Changed != 2 || report.Failed != 0 {
		t.Errorf("counters: total=%d changed=%d failed=%d", report.Total, report.Changed, report.Failed)
	}
	for _, r := range report.Results {
		if r.Compensated {
			t.Errorf("decl %s should not be compensated on a clean run", r.DeclID)
		}
	}
	if a.applyCalls.Load() != 1 || b.applyCalls.Load() != 1 {
		t.Errorf("apply call counts: a=%d b=%d", a.applyCalls.Load(), b.applyCalls.Load())
	}
}

func TestRunSaga_NilHistory_Errors(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("a", reg)
	runner := NewRunner(reg, nil)
	_, err := runner.RunSaga(context.Background(), []*Declaration{runnerDecl("a", "x")}, SagaConfig{})
	if err == nil {
		t.Error("RunSaga without History should error")
	}
}

// --- RunSaga: compensation triggered by failure ----------------------

func TestRunSaga_FailureTriggersReverseCompensation(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	// "a" succeeds on the forward path. Its prior state in history is
	// "a"'s previous Apply — same module, different params (we don't
	// care about params here; the test asserts Apply was called
	// during compensation).
	priorA := runnerDecl("a", "x")
	a := newScriptModule("a", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: true}
	})

	// "b" fails forward. No need for prior state — b never compensates
	// (only completed steps do; the failing step doesn't).
	boom := errors.New("b apply failed")
	b := newScriptModule("b", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyErr = boom
	})

	hist := &fakeHistory{runs: []*state.StateRunRecord{
		historicalRun(t, priorA),
	}}

	runner := NewRunner(reg, nil)
	report, err := runner.RunSaga(context.Background(),
		[]*Declaration{runnerDecl("a", "x"), runnerDecl("b", "y")},
		SagaConfig{History: hist})
	if err == nil {
		t.Fatal("expected an error from b's failure")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err should wrap %v, got %v", boom, err)
	}

	// a: completed forward then compensated (Apply called twice: once
	// in forward, once in rollback).
	if a.applyCalls.Load() != 2 {
		t.Errorf("a.applyCalls = %d, want 2 (forward + rollback)", a.applyCalls.Load())
	}
	// b: failed; one Apply call (the forward one).
	if b.applyCalls.Load() != 1 {
		t.Errorf("b.applyCalls = %d, want 1", b.applyCalls.Load())
	}

	var aResult, bResult *DeclarationResult
	for i := range report.Results {
		switch report.Results[i].DeclID {
		case "a:x":
			aResult = &report.Results[i]
		case "b:y":
			bResult = &report.Results[i]
		}
	}
	if aResult == nil || bResult == nil {
		t.Fatalf("missing decl results: %+v", report.Results)
		return
	}
	if !aResult.Compensated {
		t.Error("a should be marked Compensated")
	}
	if aResult.CompensateError != nil {
		t.Errorf("a should compensate cleanly, got %v", aResult.CompensateError)
	}
	if bResult.Compensated {
		t.Error("b (the failing step) must not be marked Compensated")
	}
	if bResult.Outcome != OutcomeFailed {
		t.Errorf("b outcome = %s, want failed", bResult.Outcome)
	}
}

// --- RunSaga: compensation when no prior state exists ---------------

func TestRunSaga_NoPriorState_CompensationIsNoop(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	a := newScriptModule("a", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: true}
	})
	boom := errors.New("b boom")
	newScriptModule("b", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyErr = boom
	})

	// No prior state for a.
	hist := &fakeHistory{runs: nil}

	runner := NewRunner(reg, nil)
	report, err := runner.RunSaga(context.Background(),
		[]*Declaration{runnerDecl("a", "x"), runnerDecl("b", "y")},
		SagaConfig{History: hist})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrap of %v", err, boom)
	}
	// a was compensated (the attempt happened) but only one Apply call
	// (no rollback target).
	if a.applyCalls.Load() != 1 {
		t.Errorf("a.applyCalls = %d, want 1 (no rollback ran)", a.applyCalls.Load())
	}
	for _, r := range report.Results {
		if r.DeclID == "a:x" {
			if !r.Compensated {
				t.Error("a should be Compensated=true even without a prior state")
			}
			if r.CompensateError != nil {
				t.Errorf("a CompensateError = %v, want nil (no prior = no-op success)", r.CompensateError)
			}
		}
	}
}

// --- RunSaga: compensation Apply itself fails -----------------------

func TestRunSaga_RollbackApplyFails_AggregatedInError(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	rollbackErr := errors.New("a rollback failed")
	a := newScriptModule("a", reg, func(m *scriptModule) {
		// Forward succeeds; the *rollback* Apply call errors.
		// scriptModule's applyErr is sticky, so we'd need a more
		// clever fake. Use a counter trick: first call succeeds,
		// second call fails.
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: true}
	})
	// Override Apply by swapping the registry factory with a wrapper.
	reg = NewRegistry()
	wrapped := &flakyApplyModule{inner: a, failOnCall: 2, err: rollbackErr}
	if err := reg.Register("a", func() Module { return wrapped }); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("b boom")
	newScriptModule("b", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyErr = boom
	})

	hist := &fakeHistory{runs: []*state.StateRunRecord{
		historicalRun(t, runnerDecl("a", "x")),
	}}

	runner := NewRunner(reg, nil)
	report, err := runner.RunSaga(context.Background(),
		[]*Declaration{runnerDecl("a", "x"), runnerDecl("b", "y")},
		SagaConfig{History: hist})
	// Saga aggregates both the forward failure (boom) and the
	// rollback Apply failure (rollbackErr).
	if !errors.Is(err, boom) {
		t.Errorf("err should wrap %v, got %v", boom, err)
	}
	if !errors.Is(err, rollbackErr) {
		t.Errorf("err should wrap %v, got %v", rollbackErr, err)
	}

	for _, r := range report.Results {
		if r.DeclID == "a:x" {
			if !r.Compensated {
				t.Error("a should be Compensated=true even though the rollback Apply failed")
			}
			if r.CompensateError == nil {
				t.Error("a should have a CompensateError")
			}
		}
	}
}

// --- RunSaga: history lookup error ----------------------------------

func TestRunSaga_HistoryListError_RecordsCompensateError(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("a", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: true}
	})
	boom := errors.New("b boom")
	newScriptModule("b", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyErr = boom
	})

	listErr := errors.New("history db down")
	hist := &fakeHistory{listErr: listErr}

	runner := NewRunner(reg, nil)
	report, _ := runner.RunSaga(context.Background(),
		[]*Declaration{runnerDecl("a", "x"), runnerDecl("b", "y")},
		SagaConfig{History: hist})
	for _, r := range report.Results {
		if r.DeclID == "a:x" {
			if r.CompensateError == nil || !errors.Is(r.CompensateError, listErr) {
				t.Errorf("a CompensateError should wrap %v, got %v", listErr, r.CompensateError)
			}
		}
	}
}

// --- RunSaga: AgentID scope -----------------------------------------

func TestRunSaga_AgentIDScope_PassedToFilter(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newScriptModule("a", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: true}
	})
	boom := errors.New("b boom")
	newScriptModule("b", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyErr = boom
	})

	hist := &recordingHistory{}
	runner := NewRunner(reg, nil)
	_, _ = runner.RunSaga(context.Background(),
		[]*Declaration{runnerDecl("a", "x"), runnerDecl("b", "y")},
		SagaConfig{History: hist, AgentID: "agent-007"})

	if hist.lastFilter.AgentID != "agent-007" {
		t.Errorf("AgentID not propagated: got %q", hist.lastFilter.AgentID)
	}
	if hist.lastFilter.Status != state.StateRunStatusCompleted {
		t.Errorf("Status filter = %q, want completed", hist.lastFilter.Status)
	}
	if !hist.lastFilter.SortDesc {
		t.Error("SortDesc should be true (newest-first)")
	}
}

// --- RunSaga: ClusterID post-hoc filter -----------------------------

func TestRunSaga_ClusterIDPostHocFilter(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	a := newScriptModule("a", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: true}
	})
	boom := errors.New("b boom")
	newScriptModule("b", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyErr = boom
	})

	wrongCluster := historicalRun(t, runnerDecl("a", "x"))
	wrongCluster.ClusterID = "other"
	rightCluster := historicalRun(t, runnerDecl("a", "x"))
	rightCluster.ClusterID = "mine"

	hist := &fakeHistory{runs: []*state.StateRunRecord{wrongCluster, rightCluster}}
	runner := NewRunner(reg, nil)
	_, _ = runner.RunSaga(context.Background(),
		[]*Declaration{runnerDecl("a", "x"), runnerDecl("b", "y")},
		SagaConfig{History: hist, ClusterID: "mine"})

	// Module a's Apply was called once in forward + once in rollback
	// (the rollback matched the 'mine' cluster run).
	if a.applyCalls.Load() != 2 {
		t.Errorf("a.applyCalls = %d, want 2 (forward + rollback from matching cluster)", a.applyCalls.Load())
	}
}

// --- RunSaga: malformed historical JSON is skipped ------------------

func TestRunSaga_MalformedHistoricalJSON_Skipped(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	a := newScriptModule("a", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyResult = &StateResult{Success: true, Changed: true}
	})
	boom := errors.New("b boom")
	newScriptModule("b", reg, func(m *scriptModule) {
		m.checkResult = &ModuleCheckResult{Matches: false}
		m.applyErr = boom
	})

	bad := &state.StateRunRecord{
		ID:               "broken",
		Status:           state.StateRunStatusCompleted,
		DeclarationsJSON: "{not valid json",
	}
	good := historicalRun(t, runnerDecl("a", "x"))
	hist := &fakeHistory{runs: []*state.StateRunRecord{bad, good}}

	runner := NewRunner(reg, nil)
	_, _ = runner.RunSaga(context.Background(),
		[]*Declaration{runnerDecl("a", "x"), runnerDecl("b", "y")},
		SagaConfig{History: hist})

	// a's rollback Apply ran (the malformed run was skipped, the
	// good run matched).
	if a.applyCalls.Load() != 2 {
		t.Errorf("a.applyCalls = %d, want 2 (malformed history skipped)", a.applyCalls.Load())
	}
}

// --- Helpers --------------------------------------------------------

// flakyApplyModule wraps a Module so the Nth Apply call returns the
// configured error. Used to simulate "the rollback Apply itself
// fails" without making the forward Apply fail too.
type flakyApplyModule struct {
	inner      Module
	failOnCall int64
	err        error
	calls      int64
}

func (m *flakyApplyModule) Name() string          { return m.inner.Name() }
func (m *flakyApplyModule) ValidStates() []string { return m.inner.ValidStates() }
func (m *flakyApplyModule) Check(ctx context.Context, d *Declaration) (*ModuleCheckResult, error) {
	return m.inner.Check(ctx, d)
}
func (m *flakyApplyModule) Apply(ctx context.Context, d *Declaration) (*StateResult, error) {
	m.calls++
	if m.calls == m.failOnCall {
		return nil, m.err
	}
	return m.inner.Apply(ctx, d)
}
func (m *flakyApplyModule) Test(ctx context.Context, d *Declaration) (bool, error) {
	return m.inner.Test(ctx, d)
}

// recordingHistory captures the most-recent ListStateRuns filter so
// tests can assert filter propagation.
type recordingHistory struct {
	fakeHistory
	lastFilter state.StateRunFilter
}

func (h *recordingHistory) ListStateRuns(ctx context.Context, f state.StateRunFilter) ([]*state.StateRunRecord, error) {
	h.lastFilter = f
	return h.fakeHistory.ListStateRuns(ctx, f)
}
