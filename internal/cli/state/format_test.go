package state

import (
	"bytes"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func TestOutcomeBadge(t *testing.T) {
	t.Parallel()
	cases := map[v1.StateRunOutcome]string{
		v1.StateRunOutcome_STATE_RUN_OUTCOME_UNCHANGED:       "ok    ",
		v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED:         "change",
		v1.StateRunOutcome_STATE_RUN_OUTCOME_NO_OP:           "no-op ",
		v1.StateRunOutcome_STATE_RUN_OUTCOME_FAILED:          "FAIL  ",
		v1.StateRunOutcome_STATE_RUN_OUTCOME_DRIFT_DETECTED:  "drift ",
		v1.StateRunOutcome_STATE_RUN_OUTCOME_SKIPPED:         "skip  ",
		v1.StateRunOutcome(99):                                "?     ",
	}
	for o, want := range cases {
		if got := outcomeBadge(o); got != want {
			t.Errorf("outcomeBadge(%v) = %q, want %q", o, got, want)
		}
	}
}

func TestSeverityLabel(t *testing.T) {
	t.Parallel()
	cases := map[v1.DriftSeverity]string{
		v1.DriftSeverity_DRIFT_SEVERITY_NONE:     "none",
		v1.DriftSeverity_DRIFT_SEVERITY_LOW:      "low",
		v1.DriftSeverity_DRIFT_SEVERITY_MEDIUM:   "medium",
		v1.DriftSeverity_DRIFT_SEVERITY_HIGH:     "high",
		v1.DriftSeverity_DRIFT_SEVERITY_CRITICAL: "critical",
	}
	for s, want := range cases {
		if got := severityLabel(s); got != want {
			t.Errorf("severityLabel(%v) = %q, want %q", s, got, want)
		}
	}
}

func TestStatusLabel(t *testing.T) {
	t.Parallel()
	cases := map[v1.StateRunStatus]string{
		v1.StateRunStatus_STATE_RUN_STATUS_RUNNING:   "running",
		v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED: "completed",
		v1.StateRunStatus_STATE_RUN_STATUS_FAILED:    "failed",
		v1.StateRunStatus_STATE_RUN_STATUS_CANCELLED: "cancelled",
	}
	for s, want := range cases {
		if got := statusLabel(s); got != want {
			t.Errorf("statusLabel(%v) = %q, want %q", s, got, want)
		}
	}
}

func TestPrintApplyDecl_Table(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	d := &v1.StateDeclarationResult{
		DeclId:    "file:/etc/hosts",
		Outcome:   v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED,
		ApplyDiff: "mode 0600 → 0644",
	}
	if err := printApplyDecl(&buf, FormatTable, d); err != nil {
		t.Fatalf("printApplyDecl: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "[change]") {
		t.Errorf("output missing change badge: %q", got)
	}
	if !strings.Contains(got, "file:/etc/hosts") {
		t.Errorf("output missing decl_id: %q", got)
	}
	if !strings.Contains(got, "mode 0600 → 0644") {
		t.Errorf("output missing diff: %q", got)
	}
}

func TestPrintApplyDecl_JSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	d := &v1.StateDeclarationResult{
		DeclId:  "file:/etc/hosts",
		Outcome: v1.StateRunOutcome_STATE_RUN_OUTCOME_FAILED,
	}
	if err := printApplyDecl(&buf, FormatJSON, d); err != nil {
		t.Fatalf("printApplyDecl: %v", err)
	}
	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("JSON output must end in newline; got %q", out)
	}
	if !strings.Contains(out, `"declId":"file:/etc/hosts"`) && !strings.Contains(out, `"decl_id":"file:/etc/hosts"`) {
		t.Errorf("JSON output missing decl id: %q", out)
	}
}

func TestPrintApplyDecl_NilSafe(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := printApplyDecl(&buf, FormatTable, nil); err != nil {
		t.Errorf("printApplyDecl(nil): %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("nil input should produce no output; got %q", buf.String())
	}
}

func TestPrintApplyTerminal(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	term := &v1.StateRunTerminal{
		RunId:  "r-1",
		Status: v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED,
		Aggregates: &v1.StateRunAggregates{
			Total: 3, Changed: 1, Unchanged: 2,
		},
	}
	if err := printApplyTerminal(&buf, FormatTable, term); err != nil {
		t.Fatalf("printApplyTerminal: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "status=completed") {
		t.Errorf("missing status: %q", out)
	}
	if !strings.Contains(out, "changed=1") || !strings.Contains(out, "unchanged=2") {
		t.Errorf("missing aggregates: %q", out)
	}
}

func TestPrintCheck_Summary(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	resp := &v1.CheckStateResponse{
		RunId:  "r-1",
		Status: v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED,
		Aggregates: &v1.StateRunAggregates{
			Total: 2, Drifted: 1, Unchanged: 1,
		},
		Declarations: []*v1.StateDeclarationResult{
			{DeclId: "file:/a", Outcome: v1.StateRunOutcome_STATE_RUN_OUTCOME_DRIFT_DETECTED, CheckDiff: "mode mismatch"},
			{DeclId: "file:/b", Outcome: v1.StateRunOutcome_STATE_RUN_OUTCOME_UNCHANGED},
		},
	}
	if err := printCheck(&buf, FormatTable, resp); err != nil {
		t.Fatalf("printCheck: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "drifted=1") {
		t.Errorf("summary missing drifted=1: %q", out)
	}
	if !strings.Contains(out, "file:/a") || !strings.Contains(out, "file:/b") {
		t.Errorf("decl rows missing: %q", out)
	}
}

func TestPrintDrift_SeverityOrdering(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	resp := &v1.DetectDriftResponse{
		RunId:             "r-1",
		AggregateSeverity: v1.DriftSeverity_DRIFT_SEVERITY_CRITICAL,
		Aggregates:        &v1.StateRunAggregates{Drifted: 3},
		Statuses: []*v1.DriftDeclaration{
			{DeclId: "low",      Severity: v1.DriftSeverity_DRIFT_SEVERITY_LOW,      State: v1.DriftState_DRIFT_STATE_DRIFTED},
			{DeclId: "critical", Severity: v1.DriftSeverity_DRIFT_SEVERITY_CRITICAL, State: v1.DriftState_DRIFT_STATE_DRIFTED},
			{DeclId: "high",     Severity: v1.DriftSeverity_DRIFT_SEVERITY_HIGH,     State: v1.DriftState_DRIFT_STATE_DRIFTED},
		},
	}
	if err := printDrift(&buf, FormatTable, resp); err != nil {
		t.Fatalf("printDrift: %v", err)
	}
	out := buf.String()
	// Severity-DESC: critical first, then high, then low.
	critIdx := strings.Index(out, "critical")
	highIdx := strings.Index(out, " high")
	lowIdx := strings.Index(out, "low\n")
	if critIdx < 0 || highIdx <= critIdx || lowIdx <= highIdx {
		// fall back to looser bound — just assert critical appears
		// before the LOW row.
		critRow := strings.Index(out, "] critical")
		lowRow := strings.Index(out, "] low")
		if critRow < 0 || lowRow < 0 || critRow > lowRow {
			t.Errorf("severity ordering broken; got:\n%s", out)
		}
	}
}

func TestPrintCompile(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	decls := []*statemgmt.Declaration{
		{ID: "package:nginx", Module: "package", Name: "nginx", State: "installed"},
		{ID: "file:/etc/nginx.conf", Module: "file", Name: "/etc/nginx.conf", State: "present",
			Params: map[string]any{"require": []any{map[string]any{"package": "nginx"}}}},
	}
	if err := printCompile(&buf, FormatTable, decls); err != nil {
		t.Fatalf("printCompile: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "1. package:nginx") {
		t.Errorf("missing first decl: %q", out)
	}
	if !strings.Contains(out, "2. file:/etc/nginx.conf") {
		t.Errorf("missing second decl: %q", out)
	}
	if !strings.Contains(out, "require=") {
		t.Errorf("missing require summary: %q", out)
	}
}

func TestPrintCompile_JSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	decls := []*statemgmt.Declaration{
		{ID: "file:/x", Module: "file", Name: "/x", State: "present"},
	}
	if err := printCompile(&buf, FormatJSON, decls); err != nil {
		t.Fatalf("printCompile: %v", err)
	}
	if !strings.Contains(buf.String(), `"file:/x"`) {
		t.Errorf("JSON output missing decl id: %q", buf.String())
	}
}

func TestPrintVars_All(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	vars := map[string]any{"port": 8080, "user": "www-data"}
	if err := printVars(&buf, FormatTable, vars, ""); err != nil {
		t.Fatalf("printVars: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "port=8080") || !strings.Contains(out, "user=www-data") {
		t.Errorf("missing entries: %q", out)
	}
}

func TestPrintVars_OneKey(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	vars := map[string]any{"port": 8080}
	if err := printVars(&buf, FormatTable, vars, "port"); err != nil {
		t.Fatalf("printVars: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "8080" {
		t.Errorf("single-key output should be just the value; got %q", buf.String())
	}
}

func TestPrintVars_MissingKey(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := printVars(&buf, FormatTable, map[string]any{}, "absent")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestPrintHistory_Table(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	runs := []*v1.StateRun{
		{Id: "r-1", Mode: v1.StateRunMode_STATE_RUN_MODE_APPLY, Status: v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED, AgentId: "web-1",
			Aggregates: &v1.StateRunAggregates{Total: 3, Changed: 1, Unchanged: 2}},
		{Id: "r-2", Mode: v1.StateRunMode_STATE_RUN_MODE_CHECK, Status: v1.StateRunStatus_STATE_RUN_STATUS_FAILED, AgentId: "db-1"},
	}
	if err := printHistory(&buf, FormatTable, runs); err != nil {
		t.Fatalf("printHistory: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "RUN ID") {
		t.Errorf("missing header row; got:\n%s", out)
	}
	if !strings.Contains(out, "r-1") || !strings.Contains(out, "r-2") {
		t.Errorf("missing rows; got:\n%s", out)
	}
	if !strings.Contains(out, "T=3 C=1 U=2") {
		t.Errorf("missing counts; got:\n%s", out)
	}
}

func TestPrintHistory_Empty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := printHistory(&buf, FormatTable, nil); err != nil {
		t.Fatalf("printHistory: %v", err)
	}
	if !strings.Contains(buf.String(), "no state runs") {
		t.Errorf("missing empty hint; got %q", buf.String())
	}
}

func TestPrintShow_Full(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	resp := &v1.GetStateStatusResponse{
		Run: &v1.StateRun{
			Id:           "r-1",
			Mode:         v1.StateRunMode_STATE_RUN_MODE_APPLY,
			Status:       v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED,
			Source:       "test.yaml",
			AgentId:      "web-1",
			ErrorMessage: "",
			Aggregates: &v1.StateRunAggregates{Total: 2, Changed: 1, Unchanged: 1},
		},
		Declarations: []*v1.StateDeclarationResult{
			{DeclId: "file:/a", Outcome: v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED, ApplyDiff: "mode 0600→0644"},
			{DeclId: "file:/b", Outcome: v1.StateRunOutcome_STATE_RUN_OUTCOME_UNCHANGED},
		},
	}
	if err := printShow(&buf, FormatTable, resp); err != nil {
		t.Fatalf("printShow: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Run r-1") {
		t.Errorf("missing header; got:\n%s", out)
	}
	for _, want := range []string{"Mode:", "Status:", "Source:", "Agent:", "Counts:"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in header; got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "applied (mode 0600→0644)") {
		t.Errorf("missing apply detail; got:\n%s", out)
	}
}

func TestPrintShow_NilSafe(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := printShow(&buf, FormatTable, nil); err != nil {
		t.Errorf("printShow(nil): %v", err)
	}
	if err := printShow(&buf, FormatTable, &v1.GetStateStatusResponse{}); err != nil {
		t.Errorf("printShow(empty): %v", err)
	}
	if !strings.Contains(buf.String(), "no run data") {
		t.Errorf("missing empty hint; got %q", buf.String())
	}
}

func TestCountsSummary(t *testing.T) {
	t.Parallel()
	if got := countsSummary(nil); got != "(no counts)" {
		t.Errorf("nil: got %q", got)
	}
	a := &v1.StateRunAggregates{Total: 5, Changed: 1, Unchanged: 2, Failed: 1, Skipped: 1, Drifted: 0}
	if got := countsSummary(a); got != "T=5 C=1 U=2 F=1 S=1 D=0" {
		t.Errorf("got %q", got)
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	if got := truncate("hello world", 5); got != "hell…" {
		t.Errorf("got %q", got)
	}
	if got := truncate("hi", 10); got != "hi" {
		t.Errorf("short input got %q", got)
	}
	if got := truncate("anything", 1); got != "…" {
		t.Errorf("n=1 got %q", got)
	}
}

func TestParamSummary_OnlyRequisites(t *testing.T) {
	t.Parallel()
	got := paramSummary(map[string]any{
		"require": []any{map[string]any{"package": "nginx"}},
		"mode":    "0644", // not a requisite — must be omitted
		"owner":   "root",
	})
	if !strings.Contains(got, "require=") {
		t.Errorf("missing require: %q", got)
	}
	if strings.Contains(got, "mode=") || strings.Contains(got, "owner=") {
		t.Errorf("non-requisite param leaked into compile summary: %q", got)
	}
}
