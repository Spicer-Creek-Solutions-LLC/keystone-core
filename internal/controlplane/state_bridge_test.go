// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func TestOutcomeMapping_RoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		o      statemgmt.Outcome
		proto  v1.StateRunOutcome
		record state.StateRunOutcome
	}{
		{statemgmt.OutcomeUnchanged, v1.StateRunOutcome_STATE_RUN_OUTCOME_UNCHANGED, state.StateRunOutcomeUnchanged},
		{statemgmt.OutcomeChanged, v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED, state.StateRunOutcomeChanged},
		{statemgmt.OutcomeNoOp, v1.StateRunOutcome_STATE_RUN_OUTCOME_NO_OP, state.StateRunOutcomeNoOp},
		{statemgmt.OutcomeFailed, v1.StateRunOutcome_STATE_RUN_OUTCOME_FAILED, state.StateRunOutcomeFailed},
		{statemgmt.OutcomeDriftDetected, v1.StateRunOutcome_STATE_RUN_OUTCOME_DRIFT_DETECTED, state.StateRunOutcomeDriftDetected},
		{statemgmt.OutcomeSkipped, v1.StateRunOutcome_STATE_RUN_OUTCOME_SKIPPED, state.StateRunOutcomeSkipped},
	}
	for _, c := range cases {
		if got := outcomeToProto(c.o); got != c.proto {
			t.Errorf("outcomeToProto(%v) = %v, want %v", c.o, got, c.proto)
		}
		if got := outcomeToRecord(c.o); got != c.record {
			t.Errorf("outcomeToRecord(%v) = %v, want %v", c.o, got, c.record)
		}
		if got := recordOutcomeToProto(c.record); got != c.proto {
			t.Errorf("recordOutcomeToProto(%v) = %v, want %v", c.record, got, c.proto)
		}
	}
}

func TestSeverityMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s    statemgmt.DriftSeverity
		want v1.DriftSeverity
	}{
		{statemgmt.DriftSeverityNone, v1.DriftSeverity_DRIFT_SEVERITY_NONE},
		{statemgmt.DriftSeverityLow, v1.DriftSeverity_DRIFT_SEVERITY_LOW},
		{statemgmt.DriftSeverityMedium, v1.DriftSeverity_DRIFT_SEVERITY_MEDIUM},
		{statemgmt.DriftSeverityHigh, v1.DriftSeverity_DRIFT_SEVERITY_HIGH},
		{statemgmt.DriftSeverityCritical, v1.DriftSeverity_DRIFT_SEVERITY_CRITICAL},
	}
	for _, c := range cases {
		if got := severityToProto(c.s); got != c.want {
			t.Errorf("severityToProto(%v) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestDriftStateMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		s    statemgmt.DriftState
		want v1.DriftState
	}{
		{statemgmt.DriftStateInSync, v1.DriftState_DRIFT_STATE_IN_SYNC},
		{statemgmt.DriftStateDrifted, v1.DriftState_DRIFT_STATE_DRIFTED},
		{statemgmt.DriftStateError, v1.DriftState_DRIFT_STATE_ERROR},
		{statemgmt.DriftStateSkipped, v1.DriftState_DRIFT_STATE_SKIPPED},
	}
	for _, c := range cases {
		if got := driftStateToProto(c.s); got != c.want {
			t.Errorf("driftStateToProto(%v) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestStatusMapping_RoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		record state.StateRunStatus
		proto  v1.StateRunStatus
	}{
		{state.StateRunStatusRunning, v1.StateRunStatus_STATE_RUN_STATUS_RUNNING},
		{state.StateRunStatusCompleted, v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED},
		{state.StateRunStatusFailed, v1.StateRunStatus_STATE_RUN_STATUS_FAILED},
		{state.StateRunStatusCancelled, v1.StateRunStatus_STATE_RUN_STATUS_CANCELLED},
	}
	for _, c := range cases {
		if got := recordStatusToProto(c.record); got != c.proto {
			t.Errorf("recordStatusToProto(%v) = %v, want %v", c.record, got, c.proto)
		}
		if got := protoStatusToRecord(c.proto); got != c.record {
			t.Errorf("protoStatusToRecord(%v) = %v, want %v", c.proto, got, c.record)
		}
	}
}

func TestModeMapping_RoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		record state.StateRunMode
		proto  v1.StateRunMode
	}{
		{state.StateRunModeApply, v1.StateRunMode_STATE_RUN_MODE_APPLY},
		{state.StateRunModeCheck, v1.StateRunMode_STATE_RUN_MODE_CHECK},
		{state.StateRunModeDrift, v1.StateRunMode_STATE_RUN_MODE_DRIFT},
	}
	for _, c := range cases {
		if got := recordModeToProto(c.record); got != c.proto {
			t.Errorf("recordModeToProto(%v) = %v, want %v", c.record, got, c.proto)
		}
		if got := protoModeToRecord(c.proto); got != c.record {
			t.Errorf("protoModeToRecord(%v) = %v, want %v", c.proto, got, c.record)
		}
	}
}

func TestDeclResultToProto(t *testing.T) {
	t.Parallel()
	testOK := true
	r := &statemgmt.DeclarationResult{
		DeclID:    "file:/etc/hosts",
		Module:    "file",
		Outcome:   statemgmt.OutcomeChanged,
		Check:     &statemgmt.ModuleCheckResult{Matches: false, Diff: "diff"},
		Apply:     &statemgmt.StateResult{Success: true, Changed: true, Diff: "diff", Comment: "ok"},
		Test:      &testOK,
		Error:     nil,
		StartedAt: time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC),
		Duration:  12 * time.Millisecond,
	}
	out := declResultToProto(r)
	if out.DeclId != "file:/etc/hosts" || out.Module != "file" {
		t.Errorf("scalar fields lost: %+v", out)
	}
	if out.Outcome != v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED {
		t.Errorf("Outcome = %v, want CHANGED", out.Outcome)
	}
	if out.CheckDiff != "diff" || out.ApplyDiff != "diff" || out.ApplyComment != "ok" {
		t.Errorf("diff/comment lost: %+v", out)
	}
	if !out.ApplyChanged {
		t.Error("ApplyChanged should be true")
	}
	if out.DurationMs != 12 {
		t.Errorf("DurationMs = %d, want 12", out.DurationMs)
	}
}

func TestDeclResultToProto_NilSafety(t *testing.T) {
	t.Parallel()
	if got := declResultToProto(nil); got != nil {
		t.Errorf("nil input must yield nil; got %+v", got)
	}
}

func TestDeclResultToProto_ErrorPath(t *testing.T) {
	t.Parallel()
	r := &statemgmt.DeclarationResult{
		DeclID:    "file:/x",
		Module:    "file",
		Outcome:   statemgmt.OutcomeFailed,
		Error:     errors.New("apply: permission denied"),
		StartedAt: time.Now(),
	}
	out := declResultToProto(r)
	if out.ErrorMessage != "apply: permission denied" {
		t.Errorf("ErrorMessage = %q, want underlying error", out.ErrorMessage)
	}
	if out.Outcome != v1.StateRunOutcome_STATE_RUN_OUTCOME_FAILED {
		t.Errorf("Outcome = %v, want FAILED", out.Outcome)
	}
}

func TestDeclResultToRecord_TriStateNulls(t *testing.T) {
	t.Parallel()
	// Check matched → Apply skipped → Test skipped. Only Check
	// should populate; Apply + Test must round-trip as Valid=false.
	r := &statemgmt.DeclarationResult{
		DeclID:    "file:/x",
		Module:    "file",
		Outcome:   statemgmt.OutcomeUnchanged,
		Check:     &statemgmt.ModuleCheckResult{Matches: true},
		StartedAt: time.Now(),
	}
	rec := declResultToRecord("run-1", r)
	if rec.RunID != "run-1" {
		t.Errorf("RunID = %q, want run-1", rec.RunID)
	}
	if !rec.CheckMatches.Valid || !rec.CheckMatches.Bool {
		t.Errorf("CheckMatches = %+v, want {Valid:true Bool:true}", rec.CheckMatches)
	}
	if rec.ApplyChanged.Valid || rec.TestResult.Valid {
		t.Errorf("skipped phases must round-trip as Valid=false; got Apply=%+v Test=%+v", rec.ApplyChanged, rec.TestResult)
	}
}

func TestDeclResultToRecord_AllPhases(t *testing.T) {
	t.Parallel()
	testOK := true
	r := &statemgmt.DeclarationResult{
		DeclID:    "file:/x",
		Module:    "file",
		Outcome:   statemgmt.OutcomeChanged,
		Check:     &statemgmt.ModuleCheckResult{Matches: false, Diff: "d"},
		Apply:     &statemgmt.StateResult{Success: true, Changed: true, Diff: "d2", Comment: "ok"},
		Test:      &testOK,
		StartedAt: time.Now(),
		Duration:  5 * time.Millisecond,
	}
	rec := declResultToRecord("run-1", r)
	if !rec.ApplyChanged.Valid || !rec.ApplyChanged.Bool {
		t.Errorf("ApplyChanged = %+v, want true", rec.ApplyChanged)
	}
	if !rec.TestResult.Valid || !rec.TestResult.Bool {
		t.Errorf("TestResult = %+v, want true", rec.TestResult)
	}
	if rec.ApplyDiff != "d2" || rec.ApplyComment != "ok" {
		t.Errorf("apply fields lost: %+v", rec)
	}
	if rec.DurationMS != 5 {
		t.Errorf("DurationMS = %d, want 5", rec.DurationMS)
	}
}

func TestDriftStatusToProto(t *testing.T) {
	t.Parallel()
	s := statemgmt.DriftStatus{
		DeclID:   "file:/etc/passwd",
		Module:   "file",
		State:    statemgmt.DriftStateDrifted,
		Severity: statemgmt.DriftSeverityCritical,
		Diff:     "mode changed",
	}
	out := driftStatusToProto(s)
	if out.DeclId != "file:/etc/passwd" {
		t.Errorf("DeclId = %q", out.DeclId)
	}
	if out.State != v1.DriftState_DRIFT_STATE_DRIFTED {
		t.Errorf("State = %v, want DRIFTED", out.State)
	}
	if out.Severity != v1.DriftSeverity_DRIFT_SEVERITY_CRITICAL {
		t.Errorf("Severity = %v, want CRITICAL", out.Severity)
	}
}

func TestRunRecordToProto(t *testing.T) {
	t.Parallel()
	r := &state.StateRunRecord{
		ID:        "run-1",
		Mode:      state.StateRunModeApply,
		Source:    "tests/webserver.yaml",
		ClusterID: "default",
		AgentID:   "agent-1",
		StartedAt: time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 5, 11, 10, 5, 0, 0, time.UTC),
		Status:    state.StateRunStatusCompleted,
		Total:     3, Changed: 2, Unchanged: 1,
	}
	out := runRecordToProto(r)
	if out.Id != "run-1" || out.Source != "tests/webserver.yaml" {
		t.Errorf("scalars lost: %+v", out)
	}
	if out.Mode != v1.StateRunMode_STATE_RUN_MODE_APPLY {
		t.Errorf("Mode = %v, want APPLY", out.Mode)
	}
	if out.Status != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
		t.Errorf("Status = %v, want COMPLETED", out.Status)
	}
	if out.Aggregates == nil || out.Aggregates.Total != 3 || out.Aggregates.Changed != 2 {
		t.Errorf("Aggregates lost: %+v", out.Aggregates)
	}
	if out.EndedAt == nil {
		t.Error("EndedAt should be set when non-zero")
	}
}

func TestRunRecordToProto_NilAndZero(t *testing.T) {
	t.Parallel()
	if got := runRecordToProto(nil); got != nil {
		t.Errorf("nil input must yield nil; got %+v", got)
	}
	// Zero EndedAt should produce nil EndedAt on the wire — not
	// epoch — so the CLI can distinguish "still running" from
	// "ended at the start of time."
	r := &state.StateRunRecord{ID: "x", StartedAt: time.Now()}
	out := runRecordToProto(r)
	if out.EndedAt != nil {
		t.Errorf("EndedAt should be nil when record's EndedAt is zero; got %v", out.EndedAt)
	}
}

func TestResultRecordToProto(t *testing.T) {
	t.Parallel()
	r := &state.StateRunResultRecord{
		RunID:        "run-1",
		DeclID:       "file:/x",
		Module:       "file",
		Outcome:      state.StateRunOutcomeChanged,
		CheckDiff:    "d",
		ApplyChanged: sql.NullBool{Valid: true, Bool: true},
		ApplyDiff:    "d2",
		ApplyComment: "ok",
		StartedAt:    time.Now(),
		DurationMS:   7,
	}
	out := resultRecordToProto(r)
	if out.Outcome != v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED {
		t.Errorf("Outcome = %v, want CHANGED", out.Outcome)
	}
	if !out.ApplyChanged {
		t.Error("ApplyChanged should be true")
	}
	if out.DurationMs != 7 {
		t.Errorf("DurationMs = %d, want 7", out.DurationMs)
	}
}

func TestReportAggregatesToProto(t *testing.T) {
	t.Parallel()
	rep := &statemgmt.RunReport{Total: 5, Changed: 2, Unchanged: 1, Failed: 1, Skipped: 1}
	a := reportAggregatesToProto(rep)
	if a.Total != 5 || a.Changed != 2 || a.Unchanged != 1 || a.Failed != 1 || a.Skipped != 1 {
		t.Errorf("aggregates lost: %+v", a)
	}
}

func TestReportAggregatesToEnd(t *testing.T) {
	t.Parallel()
	rep := &statemgmt.RunReport{Total: 3, Changed: 1, Unchanged: 2}
	end := reportAggregatesToEnd(state.StateRunStatusCompleted, nowOr(time.Time{}), "", rep)
	if end.Status != state.StateRunStatusCompleted {
		t.Errorf("Status = %v, want completed", end.Status)
	}
	if end.EndedAt.IsZero() {
		t.Error("EndedAt should default to now when zero passed in")
	}
	if end.Total != 3 || end.Changed != 1 || end.Unchanged != 2 {
		t.Errorf("aggregates lost: %+v", end)
	}
}

func TestDeclarationsToJSON_RoundTrip(t *testing.T) {
	t.Parallel()
	decls := []*statemgmt.Declaration{
		{ID: "file:/x", Module: "file", Name: "/x", State: "present"},
		{ID: "package:nginx", Module: "package", Name: "nginx", State: "installed"},
	}
	s, err := declarationsToJSON(decls)
	if err != nil {
		t.Fatalf("declarationsToJSON: %v", err)
	}
	if s == "[]" {
		t.Error("expected non-empty JSON for non-empty decls")
	}
	var back []*statemgmt.Declaration
	if err := json.Unmarshal([]byte(s), &back); err != nil {
		t.Fatalf("round-trip decode: %v", err)
	}
	if len(back) != 2 || back[0].ID != "file:/x" || back[1].Module != "package" {
		t.Errorf("round-trip lost data: %+v", back)
	}
}

func TestDeclarationsToJSON_Empty(t *testing.T) {
	t.Parallel()
	s, err := declarationsToJSON(nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if s != "[]" {
		t.Errorf("got %q, want [] for nil decls", s)
	}
}

func TestUnmarshalDeclarations_RoundTrip(t *testing.T) {
	t.Parallel()
	in := []*statemgmt.Declaration{
		{ID: "file:/x", Module: "file", Name: "/x", State: "present"},
		{ID: "package:nginx", Module: "package", Name: "nginx", State: "installed"},
	}
	s, err := declarationsToJSON(in)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, err := unmarshalDeclarations(s)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0].ID != "file:/x" || out[1].Module != "package" {
		t.Errorf("round-trip lost data: %+v", out)
	}
}

func TestUnmarshalDeclarations_EmptyForms(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "[]"} {
		out, err := unmarshalDeclarations(in)
		if err != nil {
			t.Errorf("unmarshalDeclarations(%q): %v", in, err)
		}
		if out != nil {
			t.Errorf("unmarshalDeclarations(%q) = %v, want nil", in, out)
		}
	}
}

func TestUnmarshalDeclarations_Malformed(t *testing.T) {
	t.Parallel()
	if _, err := unmarshalDeclarations("not json"); err == nil {
		t.Error("expected error on malformed input")
	}
}

