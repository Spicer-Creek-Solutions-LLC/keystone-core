//go:build integration

package state

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPg_StateRun_CRUD(t *testing.T) {
	s := newPgStoreForTest(t)
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
	if got.DeclarationsJSON != r.DeclarationsJSON {
		t.Errorf("DeclarationsJSON not round-tripped: %q vs %q", got.DeclarationsJSON, r.DeclarationsJSON)
	}
	if len(results) != 0 {
		t.Errorf("results before any added = %d, want 0", len(results))
	}
}

func TestPg_StateRun_CreateRejectsNilAndEmptyID(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateStateRun(ctx, nil); err == nil {
		t.Error("expected nil-record error")
	}
	if err := s.CreateStateRun(ctx, &StateRunRecord{}); err == nil {
		t.Error("expected empty-ID error")
	}
}

func TestPg_StateRun_FinalizeStamps(t *testing.T) {
	s := newPgStoreForTest(t)
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
	got, _, _ := s.GetStateRun(ctx, "run-1")
	if got.Status != StateRunStatusCompleted {
		t.Errorf("Status = %v, want completed", got.Status)
	}
	if !got.EndedAt.Equal(end.EndedAt) {
		t.Errorf("EndedAt = %v, want %v", got.EndedAt, end.EndedAt)
	}
	if got.Total != 3 || got.Changed != 2 || got.Unchanged != 1 {
		t.Errorf("aggregates lost")
	}
}

func TestPg_StateRun_FinalizeNotFound(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	err := s.FinalizeStateRun(ctx, "ghost", StateRunEnd{Status: StateRunStatusCompleted})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestPg_StateRun_AddResult(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateStateRun(ctx, sampleStateRun("run-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.AddStateRunResult(ctx, "run-1", sampleStateRunResult("run-1", "file:/a")); err != nil {
		t.Fatalf("AddStateRunResult: %v", err)
	}
	_, results, _ := s.GetStateRun(ctx, "run-1")
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
}

func TestPg_StateRun_AddResult_NullableTriState(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateStateRun(ctx, sampleStateRun("run-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	r := sampleStateRunResult("run-1", "file:/a")
	r.Outcome = StateRunOutcomeUnchanged
	r.CheckMatches = sql.NullBool{Valid: true, Bool: true}
	r.ApplyChanged = sql.NullBool{}
	r.TestResult = sql.NullBool{}
	if err := s.AddStateRunResult(ctx, "run-1", r); err != nil {
		t.Fatalf("Add: %v", err)
	}
	_, results, _ := s.GetStateRun(ctx, "run-1")
	got := results[0]
	if got.ApplyChanged.Valid || got.TestResult.Valid {
		t.Errorf("skipped phases should round-trip as Valid=false; got ApplyChanged=%+v TestResult=%+v",
			got.ApplyChanged, got.TestResult)
	}
}

func TestPg_StateRun_AddResult_FKViolationOnUnknownRun(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	err := s.AddStateRunResult(ctx, "ghost", sampleStateRunResult("ghost", "file:/a"))
	if err == nil {
		t.Error("expected FK violation for unknown run_id")
	}
}

func TestPg_StateRun_AddResult_DuplicateRejected(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateStateRun(ctx, sampleStateRun("run-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.AddStateRunResult(ctx, "run-1", sampleStateRunResult("run-1", "file:/a")); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := s.AddStateRunResult(ctx, "run-1", sampleStateRunResult("run-1", "file:/a")); err == nil {
		t.Error("expected PK violation on duplicate (run_id, decl_id)")
	}
}

func TestPg_StateRun_GetNotFound(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	_, _, err := s.GetStateRun(ctx, "ghost")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestPg_StateRun_ListFilters(t *testing.T) {
	s := newPgStoreForTest(t)
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

	all, _ := s.ListStateRuns(ctx, StateRunFilter{})
	if len(all) != 4 || all[0].ID != "d1" {
		t.Errorf("default ordering broken: %v", ids(all))
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
		t.Errorf("bounded len = %d, want 2", len(bounded))
	}

	limited, _ := s.ListStateRuns(ctx, StateRunFilter{Limit: 2})
	if len(limited) != 2 {
		t.Errorf("limit = %d, want 2", len(limited))
	}
}

func TestPg_StateRun_ListRejectsBadSort(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	_, err := s.ListStateRuns(ctx, StateRunFilter{SortColumn: "drop_table"})
	if err == nil || !strings.Contains(err.Error(), "sort column") {
		t.Errorf("err = %v, want SQL-injection guard", err)
	}
}

func TestPg_StateRun_DeleteBeforeCascades(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	t0 := time.Date(2026, 5, 11, 9, 0, 0, 0, time.UTC)

	for i, id := range []string{"old1", "fresh"} {
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
	if err := s.AddStateRunResult(ctx, "old1", sampleStateRunResult("old1", "file:/a")); err != nil {
		t.Fatalf("seed result: %v", err)
	}

	cutoff := t0.Add(30 * time.Minute) // between old1's end and fresh's end
	n, err := s.DeleteStateRunsBefore(ctx, cutoff, []StateRunStatus{StateRunStatusCompleted})
	if err != nil {
		t.Fatalf("DeleteStateRunsBefore: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1 (old1 only)", n)
	}
	// Cascade: old1's result must be gone.
	row := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM state_run_results WHERE run_id = $1", "old1")
	var count int
	_ = row.Scan(&count)
	if count != 0 {
		t.Errorf("cascade failed: old1 still has %d result rows", count)
	}
}

func TestPg_StateRun_DeleteBefore_RejectsEmptyStatuses(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	_, err := s.DeleteStateRunsBefore(ctx, time.Now(), nil)
	if err == nil {
		t.Error("expected error for empty statuses")
	}
}

// JSONB-specific: large declaration JSON (~100 KB) round-trips
// without truncation. Confirms the JSONB column accepts non-trivial
// payloads.
func TestPg_StateRun_LargeDeclarationJSON(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	// Build a ~100 KB JSON array of trivial decl objects.
	var sb strings.Builder
	sb.WriteByte('[')
	for i := 0; i < 1000; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		// ~100 bytes per element × 1000 ≈ 100 KB
		sb.WriteString(`{"id":"file:/path/that/is/somewhat/long/`)
		sb.WriteString(strings.Repeat("x", 60))
		sb.WriteString(`","module":"file","state":"present"}`)
	}
	sb.WriteByte(']')

	r := sampleStateRun("big-run")
	r.DeclarationsJSON = sb.String()
	if err := s.CreateStateRun(ctx, r); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, _, err := s.GetStateRun(ctx, "big-run")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.DeclarationsJSON) < 90_000 {
		t.Errorf("DeclarationsJSON truncated: got %d bytes", len(got.DeclarationsJSON))
	}
}
