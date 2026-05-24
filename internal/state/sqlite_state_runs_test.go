// SPDX-License-Identifier: Apache-2.0

package state

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func sampleStateRun(id string) *StateRunRecord {
	return &StateRunRecord{
		ID:               id,
		Mode:             StateRunModeApply,
		Source:           "tests/webserver.yaml",
		ClusterID:        "default",
		AgentID:          "agent-1",
		StartedAt:        time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC),
		Status:           StateRunStatusRunning,
		DeclarationsJSON: `[{"id":"file:/etc/hosts","module":"file","state":"present"}]`,
	}
}

func sampleStateRunResult(runID, declID string) *StateRunResultRecord {
	return &StateRunResultRecord{
		RunID:        runID,
		DeclID:       declID,
		Module:       "file",
		Outcome:      StateRunOutcomeChanged,
		CheckMatches: sql.NullBool{Valid: true, Bool: false},
		CheckDiff:    "mode 0600 -> 0644",
		ApplyChanged: sql.NullBool{Valid: true, Bool: true},
		ApplyDiff:    "mode 0600 -> 0644",
		ApplyComment: "chmod applied",
		TestResult:   sql.NullBool{Valid: true, Bool: true},
		StartedAt:    time.Date(2026, 5, 11, 10, 0, 1, 0, time.UTC),
		DurationMS:   12,
	}
}

func TestSQLite_StateRun_CRUD(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	r := sampleStateRun("run-1")
	if err := s.CreateStateRun(ctx, r); err != nil {
		t.Fatalf("CreateStateRun: %v", err)
	}

	got, results, err := s.GetStateRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("GetStateRun: %v", err)
	}
	if got.ID != r.ID || got.Mode != r.Mode || got.Source != r.Source {
		t.Errorf("scalar fields lost: %+v", got)
	}
	if got.Status != StateRunStatusRunning {
		t.Errorf("Status = %v, want running", got.Status)
	}
	if got.DeclarationsJSON != r.DeclarationsJSON {
		t.Errorf("DeclarationsJSON not round-tripped: %q vs %q", got.DeclarationsJSON, r.DeclarationsJSON)
	}
	if !got.EndedAt.IsZero() {
		t.Errorf("EndedAt should be zero before Finalize; got %v", got.EndedAt)
	}
	if len(results) != 0 {
		t.Errorf("expected zero results before any added, got %d", len(results))
	}
}

func TestSQLite_StateRun_CreateRejectsNilAndEmptyID(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateStateRun(ctx, nil); err == nil {
		t.Error("expected nil-record error")
	}
	if err := s.CreateStateRun(ctx, &StateRunRecord{}); err == nil {
		t.Error("expected empty-ID error")
	}
}

func TestSQLite_StateRun_FinalizeStamps(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateStateRun(ctx, sampleStateRun("run-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	end := StateRunEnd{
		Status:    StateRunStatusCompleted,
		EndedAt:   time.Date(2026, 5, 11, 10, 5, 0, 0, time.UTC),
		Total:     3,
		Changed:   2,
		Unchanged: 1,
	}
	if err := s.FinalizeStateRun(ctx, "run-1", end); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	got, _, err := s.GetStateRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("GetStateRun: %v", err)
	}
	if got.Status != StateRunStatusCompleted {
		t.Errorf("Status = %v, want completed", got.Status)
	}
	if !got.EndedAt.Equal(end.EndedAt) {
		t.Errorf("EndedAt = %v, want %v", got.EndedAt, end.EndedAt)
	}
	if got.Total != 3 || got.Changed != 2 || got.Unchanged != 1 {
		t.Errorf("aggregates lost: total=%d changed=%d unchanged=%d", got.Total, got.Changed, got.Unchanged)
	}
}

func TestSQLite_StateRun_FinalizeNotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	err := s.FinalizeStateRun(ctx, "ghost", StateRunEnd{Status: StateRunStatusCompleted})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_StateRun_AddResult(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateStateRun(ctx, sampleStateRun("run-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.AddStateRunResult(ctx, "run-1", sampleStateRunResult("run-1", "file:/a")); err != nil {
		t.Fatalf("AddStateRunResult: %v", err)
	}
	_, results, err := s.GetStateRun(ctx, "run-1")
	if err != nil {
		t.Fatalf("GetStateRun: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	r := results[0]
	if r.Outcome != StateRunOutcomeChanged {
		t.Errorf("Outcome = %v, want changed", r.Outcome)
	}
	if !r.CheckMatches.Valid || r.CheckMatches.Bool {
		t.Errorf("CheckMatches = %+v, want {Valid:true Bool:false}", r.CheckMatches)
	}
	if !r.ApplyChanged.Valid || !r.ApplyChanged.Bool {
		t.Errorf("ApplyChanged = %+v, want {Valid:true Bool:true}", r.ApplyChanged)
	}
	if !r.TestResult.Valid || !r.TestResult.Bool {
		t.Errorf("TestResult = %+v, want {Valid:true Bool:true}", r.TestResult)
	}
	if r.DurationMS != 12 {
		t.Errorf("DurationMS = %d, want 12", r.DurationMS)
	}
}

func TestSQLite_StateRun_AddResult_NullableTriState(t *testing.T) {
	// Validator-shaped scenario: Check matched, so Apply was
	// skipped; ApplyChanged + TestResult are Valid=false.
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateStateRun(ctx, sampleStateRun("run-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	r := sampleStateRunResult("run-1", "file:/a")
	r.Outcome = StateRunOutcomeUnchanged
	r.CheckMatches = sql.NullBool{Valid: true, Bool: true}
	r.ApplyChanged = sql.NullBool{}
	r.TestResult = sql.NullBool{}
	r.CheckDiff = ""
	r.ApplyDiff = ""
	r.ApplyComment = ""
	if err := s.AddStateRunResult(ctx, "run-1", r); err != nil {
		t.Fatalf("Add: %v", err)
	}
	_, results, _ := s.GetStateRun(ctx, "run-1")
	got := results[0]
	if got.ApplyChanged.Valid || got.TestResult.Valid {
		t.Errorf("skipped phases should round-trip as Valid=false; got ApplyChanged=%+v TestResult=%+v", got.ApplyChanged, got.TestResult)
	}
}

func TestSQLite_StateRun_AddResult_RejectsEmpty(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	if err := s.AddStateRunResult(ctx, "run-1", nil); err == nil {
		t.Error("expected nil-record error")
	}
	if err := s.AddStateRunResult(ctx, "", sampleStateRunResult("", "x")); err == nil {
		t.Error("expected empty-runID error")
	}
}

func TestSQLite_StateRun_AddResult_DuplicateRejected(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateStateRun(ctx, sampleStateRun("run-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.AddStateRunResult(ctx, "run-1", sampleStateRunResult("run-1", "file:/a")); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	err := s.AddStateRunResult(ctx, "run-1", sampleStateRunResult("run-1", "file:/a"))
	if err == nil {
		t.Error("expected unique-constraint error on duplicate (run_id, decl_id)")
	}
}

func TestSQLite_StateRun_AddResult_FKViolationOnUnknownRun(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	err := s.AddStateRunResult(ctx, "ghost", sampleStateRunResult("ghost", "file:/a"))
	if err == nil {
		t.Error("expected FK violation for unknown run_id")
	}
}

func TestSQLite_StateRun_GetNotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	_, _, err := s.GetStateRun(ctx, "ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_StateRun_ListFilters(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	t0 := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)

	mk := func(id string, mode StateRunMode, agent string, status StateRunStatus, started time.Time) {
		t.Helper()
		r := &StateRunRecord{
			ID: id, Mode: mode, Source: "x.yaml",
			AgentID: agent, StartedAt: started, Status: status,
		}
		if err := s.CreateStateRun(ctx, r); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}
	mk("a1", StateRunModeApply, "agent-1", StateRunStatusCompleted, t0.Add(0))
	mk("a2", StateRunModeApply, "agent-2", StateRunStatusFailed, t0.Add(1*time.Minute))
	mk("c1", StateRunModeCheck, "agent-1", StateRunStatusCompleted, t0.Add(2*time.Minute))
	mk("d1", StateRunModeDrift, "agent-1", StateRunStatusCompleted, t0.Add(3*time.Minute))

	all, err := s.ListStateRuns(ctx, StateRunFilter{})
	if err != nil {
		t.Fatalf("ListStateRuns: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("len(all) = %d, want 4", len(all))
	}
	// Default ordering is started_at DESC: newest first.
	if all[0].ID != "d1" {
		t.Errorf("default ordering broken; first = %q, want d1", all[0].ID)
	}

	byAgent, _ := s.ListStateRuns(ctx, StateRunFilter{AgentID: "agent-1"})
	if len(byAgent) != 3 {
		t.Errorf("byAgent len = %d, want 3", len(byAgent))
	}

	byMode, _ := s.ListStateRuns(ctx, StateRunFilter{Mode: StateRunModeApply})
	if len(byMode) != 2 {
		t.Errorf("byMode len = %d, want 2", len(byMode))
	}

	byStatus, _ := s.ListStateRuns(ctx, StateRunFilter{Status: StateRunStatusFailed})
	if len(byStatus) != 1 || byStatus[0].ID != "a2" {
		t.Errorf("byStatus = %v, want [a2]", ids(byStatus))
	}

	bounded, _ := s.ListStateRuns(ctx, StateRunFilter{
		After:  t0.Add(1 * time.Minute),
		Before: t0.Add(3 * time.Minute),
	})
	if len(bounded) != 2 {
		t.Errorf("bounded len = %d, want 2 (after exclusive of a1, before exclusive of d1)", len(bounded))
	}

	withLimit, _ := s.ListStateRuns(ctx, StateRunFilter{Limit: 2})
	if len(withLimit) != 2 {
		t.Errorf("limit = %d, want 2", len(withLimit))
	}
}

func TestSQLite_StateRun_ListRejectsBadSort(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	_, err := s.ListStateRuns(ctx, StateRunFilter{SortColumn: "drop_table"})
	if err == nil || !strings.Contains(err.Error(), "sort column") {
		t.Errorf("err = %v, want SQL-injection guard", err)
	}
}

func TestSQLite_StateRun_DeleteBefore(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	t0 := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)

	// Three terminal completed; one running.
	for i, id := range []string{"old1", "old2", "fresh"} {
		r := sampleStateRun(id)
		r.StartedAt = t0.Add(time.Duration(i) * time.Hour)
		if err := s.CreateStateRun(ctx, r); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
		if err := s.FinalizeStateRun(ctx, id, StateRunEnd{
			Status:  StateRunStatusCompleted,
			EndedAt: t0.Add(time.Duration(i) * time.Hour).Add(1 * time.Minute),
		}); err != nil {
			t.Fatalf("Finalize %s: %v", id, err)
		}
	}
	// One still running.
	running := sampleStateRun("running")
	running.StartedAt = t0
	if err := s.CreateStateRun(ctx, running); err != nil {
		t.Fatalf("Create running: %v", err)
	}

	// Seed a result row on one of the old rows so cascade can be
	// verified.
	if err := s.AddStateRunResult(ctx, "old1", sampleStateRunResult("old1", "file:/a")); err != nil {
		t.Fatalf("seed result: %v", err)
	}

	// Cutoff between old2's end (t0+1h+1m) and fresh's end (t0+2h+1m).
	cutoff := t0.Add(2 * time.Hour)
	n, err := s.DeleteStateRunsBefore(ctx, cutoff, []StateRunStatus{StateRunStatusCompleted})
	if err != nil {
		t.Fatalf("DeleteStateRunsBefore: %v", err)
	}
	if n != 2 {
		t.Errorf("deleted = %d, want 2 (old1 + old2)", n)
	}

	// Cascade: old1's result must be gone.
	row := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM state_run_results WHERE run_id = ?", "old1")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("cascade failed: old1 still has %d result rows", count)
	}

	// Running row must survive even though cutoff is past its
	// started_at — ended_at IS NULL.
	all, _ := s.ListStateRuns(ctx, StateRunFilter{})
	hasRunning := false
	for _, r := range all {
		if r.ID == "running" {
			hasRunning = true
		}
	}
	if !hasRunning {
		t.Errorf("retention deleted a non-terminal row; surviving: %v", ids(all))
	}
}

func TestSQLite_StateRun_DeleteBefore_RejectsEmptyStatuses(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	_, err := s.DeleteStateRunsBefore(ctx, time.Now(), nil)
	if err == nil {
		t.Error("expected error for empty statuses")
	}
}

func TestSQLite_StateRun_EmptyDeclarationsJSONDefaultsToArray(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	r := sampleStateRun("run-empty")
	r.DeclarationsJSON = ""
	if err := s.CreateStateRun(ctx, r); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _, _ := s.GetStateRun(ctx, "run-empty")
	if got.DeclarationsJSON != "[]" {
		t.Errorf("DeclarationsJSON = %q, want [] for empty input", got.DeclarationsJSON)
	}
}

func TestSQLite_StateRun_ResultsOrderedByStartedAt(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateStateRun(ctx, sampleStateRun("run-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	base := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{"z", "a", "m"} {
		r := sampleStateRunResult("run-1", "file:/"+id)
		r.StartedAt = base.Add(time.Duration(i) * time.Second)
		if err := s.AddStateRunResult(ctx, "run-1", r); err != nil {
			t.Fatalf("Add %s: %v", id, err)
		}
	}
	_, results, _ := s.GetStateRun(ctx, "run-1")
	wantDeclIDs := []string{"file:/z", "file:/a", "file:/m"}
	if len(results) != 3 {
		t.Fatalf("len = %d, want 3", len(results))
	}
	for i, want := range wantDeclIDs {
		if results[i].DeclID != want {
			t.Errorf("[%d] = %q, want %q (ordering broken: %v)",
				i, results[i].DeclID, want, resultDeclIDs(results))
		}
	}
}

func ids(rs []*StateRunRecord) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID
	}
	return out
}

func resultDeclIDs(rs []*StateRunResultRecord) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.DeclID
	}
	return out
}
