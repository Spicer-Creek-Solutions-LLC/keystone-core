package state

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func modeLabel(m v1.StateRunMode) string {
	switch m {
	case v1.StateRunMode_STATE_RUN_MODE_APPLY:
		return "apply"
	case v1.StateRunMode_STATE_RUN_MODE_CHECK:
		return "check"
	case v1.StateRunMode_STATE_RUN_MODE_DRIFT:
		return "drift"
	default:
		return "unknown"
	}
}

// outcomeBadge returns the short tag that prefixes a decl row in
// table output.
func outcomeBadge(o v1.StateRunOutcome) string {
	switch o {
	case v1.StateRunOutcome_STATE_RUN_OUTCOME_UNCHANGED:
		return "ok    "
	case v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED:
		return "change"
	case v1.StateRunOutcome_STATE_RUN_OUTCOME_NO_OP:
		return "no-op "
	case v1.StateRunOutcome_STATE_RUN_OUTCOME_FAILED:
		return "FAIL  "
	case v1.StateRunOutcome_STATE_RUN_OUTCOME_DRIFT_DETECTED:
		return "drift "
	case v1.StateRunOutcome_STATE_RUN_OUTCOME_SKIPPED:
		return "skip  "
	default:
		return "?     "
	}
}

func severityLabel(s v1.DriftSeverity) string {
	switch s {
	case v1.DriftSeverity_DRIFT_SEVERITY_NONE:
		return "none"
	case v1.DriftSeverity_DRIFT_SEVERITY_LOW:
		return "low"
	case v1.DriftSeverity_DRIFT_SEVERITY_MEDIUM:
		return "medium"
	case v1.DriftSeverity_DRIFT_SEVERITY_HIGH:
		return "high"
	case v1.DriftSeverity_DRIFT_SEVERITY_CRITICAL:
		return "critical"
	default:
		return "unknown"
	}
}

func statusLabel(s v1.StateRunStatus) string {
	switch s {
	case v1.StateRunStatus_STATE_RUN_STATUS_RUNNING:
		return "running"
	case v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED:
		return "completed"
	case v1.StateRunStatus_STATE_RUN_STATUS_FAILED:
		return "failed"
	case v1.StateRunStatus_STATE_RUN_STATUS_CANCELLED:
		return "cancelled"
	default:
		return "unknown"
	}
}

// ---- Apply (streaming) -----------------------------------------------

// printApplyDecl renders one StateDeclarationResult line.
func printApplyDecl(out io.Writer, format string, d *v1.StateDeclarationResult) error {
	if d == nil {
		return nil
	}
	if format == FormatJSON {
		return writeJSON(out, d)
	}
	detail := ""
	switch {
	case d.GetApplyDiff() != "":
		detail = "applied (" + d.GetApplyDiff() + ")"
	case d.GetCheckDiff() != "":
		detail = d.GetCheckDiff()
	case d.GetErrorMessage() != "":
		detail = d.GetErrorMessage()
	}
	fmt.Fprintf(out, "[%s] %-32s %s\n", outcomeBadge(d.Outcome), d.DeclId, detail)
	return nil
}

// printApplyRunID prints the first event of an Apply stream.
func printApplyRunID(out io.Writer, format string, runID string) error {
	if format == FormatJSON {
		return writeJSON(out, map[string]string{"run_id": runID})
	}
	fmt.Fprintf(out, "run-id: %s\n", runID)
	return nil
}

// printApplyTerminal renders the closing summary of an Apply stream.
func printApplyTerminal(out io.Writer, format string, t *v1.StateRunTerminal) error {
	if t == nil {
		return nil
	}
	if format == FormatJSON {
		return writeJSON(out, t)
	}
	a := t.GetAggregates()
	if a == nil {
		a = &v1.StateRunAggregates{}
	}
	fmt.Fprintf(out, "done: status=%s changed=%d unchanged=%d failed=%d skipped=%d (%d declarations)\n",
		statusLabel(t.Status), a.Changed, a.Unchanged, a.Failed, a.Skipped, a.Total)
	if t.ErrorMessage != "" {
		fmt.Fprintf(out, "error: %s\n", t.ErrorMessage)
	}
	return nil
}

// ---- Check (unary) ---------------------------------------------------

// printCheck renders a CheckStateResponse.
func printCheck(out io.Writer, format string, resp *v1.CheckStateResponse) error {
	if format == FormatJSON {
		return writeJSON(out, resp)
	}
	fmt.Fprintf(out, "run-id: %s\n", resp.RunId)
	for _, d := range resp.GetDeclarations() {
		if err := printApplyDecl(out, FormatTable, d); err != nil {
			return err
		}
	}
	a := resp.GetAggregates()
	if a == nil {
		a = &v1.StateRunAggregates{}
	}
	fmt.Fprintf(out, "summary: status=%s total=%d drifted=%d unchanged=%d failed=%d\n",
		statusLabel(resp.Status), a.Total, a.Drifted, a.Unchanged, a.Failed)
	if resp.ErrorMessage != "" {
		fmt.Fprintf(out, "error: %s\n", resp.ErrorMessage)
	}
	return nil
}

// ---- Drift -----------------------------------------------------------

// printDrift renders a DetectDriftResponse, severity-grouped.
func printDrift(out io.Writer, format string, resp *v1.DetectDriftResponse) error {
	if format == FormatJSON {
		return writeJSON(out, resp)
	}
	a := resp.GetAggregates()
	if a == nil {
		a = &v1.StateRunAggregates{}
	}
	fmt.Fprintf(out, "run-id: %s\n", resp.RunId)
	fmt.Fprintf(out, "aggregate severity: %s (drifted=%d in-sync=%d errors=%d skipped=%d)\n\n",
		severityLabel(resp.AggregateSeverity), a.Drifted, a.Unchanged, a.Failed, a.Skipped)

	// Severity-DESC ordering so the worst lines show first.
	order := map[v1.DriftSeverity]int{
		v1.DriftSeverity_DRIFT_SEVERITY_CRITICAL: 5,
		v1.DriftSeverity_DRIFT_SEVERITY_HIGH:     4,
		v1.DriftSeverity_DRIFT_SEVERITY_MEDIUM:   3,
		v1.DriftSeverity_DRIFT_SEVERITY_LOW:      2,
		v1.DriftSeverity_DRIFT_SEVERITY_NONE:     1,
	}
	statuses := append([]*v1.DriftDeclaration(nil), resp.GetStatuses()...)
	sort.SliceStable(statuses, func(i, j int) bool {
		return order[statuses[i].Severity] > order[statuses[j].Severity]
	})
	for _, s := range statuses {
		state := strings.ToLower(strings.TrimPrefix(s.State.String(), "DRIFT_STATE_"))
		fmt.Fprintf(out, "[%-8s] %-32s %s %s\n",
			severityLabel(s.Severity), s.DeclId, state, s.Diff)
		if s.ErrorMessage != "" {
			fmt.Fprintf(out, "           error: %s\n", s.ErrorMessage)
		}
	}
	if resp.ErrorMessage != "" {
		fmt.Fprintf(out, "\nerror: %s\n", resp.ErrorMessage)
	}
	return nil
}

// ---- Compile (local) -------------------------------------------------

// printCompile renders the resolved declaration list. Used by both
// `compile` and as a debug surface for the other subcommands.
func printCompile(out io.Writer, format string, decls []*statemgmt.Declaration) error {
	if format == FormatJSON {
		return writeJSON(out, decls)
	}
	for i, d := range decls {
		fmt.Fprintf(out, "%d. %-32s state=%-12s", i+1, d.ID, d.State)
		if len(d.Params) > 0 {
			fmt.Fprintf(out, " %s", paramSummary(d.Params))
		}
		fmt.Fprintln(out)
	}
	return nil
}

// paramSummary picks a few useful Params keys for the compile output.
// The full Params map can be huge; we surface requisites (most
// useful for verifying topo order) and skip the rest. Tests pin this
// behaviour.
func paramSummary(params map[string]any) string {
	if len(params) == 0 {
		return ""
	}
	keys := []string{"require", "require_in", "watch", "watch_in", "prereq", "prereq_in", "onchanges", "onchanges_in"}
	var parts []string
	for _, k := range keys {
		v, ok := params[k]
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, " ")
}

// ---- Vars (local) ----------------------------------------------------

// printVars renders the merged Variables map. When key is non-empty
// only that key's value is printed (no key prefix) so callers can
// pipe it to other tools.
func printVars(out io.Writer, format string, vars map[string]any, key string) error {
	if key != "" {
		v, ok := vars[key]
		if !ok {
			return fmt.Errorf("state: variable %q not found", key)
		}
		if format == FormatJSON {
			return writeJSON(out, v)
		}
		fmt.Fprintln(out, v)
		return nil
	}
	if format == FormatJSON {
		return writeJSON(out, vars)
	}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(out, "%s=%v\n", k, vars[k])
	}
	return nil
}

// ---- History --------------------------------------------------------

// printHistory renders the list of past runs returned by
// GetStateHistory. Table format includes the headline counters
// inline so an operator scanning history sees outcome at a glance.
func printHistory(out io.Writer, format string, runs []*v1.StateRun) error {
	if format == FormatJSON {
		return writeJSON(out, runs)
	}
	if len(runs) == 0 {
		fmt.Fprintln(out, "no state runs found")
		return nil
	}
	fmt.Fprintln(out, "RUN ID                                MODE   STATUS    AGENT            STARTED                  DURATION  COUNTS")
	for _, r := range runs {
		fmt.Fprintf(out, "%-36s  %-5s  %-9s %-16s %-23s  %-8s  %s\n",
			r.GetId(),
			modeLabel(r.GetMode()),
			statusLabel(r.GetStatus()),
			truncate(r.GetAgentId(), 16),
			formatTimestamp(r.GetStartedAt()),
			formatDuration(r.GetStartedAt(), r.GetEndedAt()),
			countsSummary(r.GetAggregates()))
	}
	return nil
}

// ---- Show -----------------------------------------------------------

// printShow renders one full GetStateStatusResponse.
func printShow(out io.Writer, format string, resp *v1.GetStateStatusResponse) error {
	if format == FormatJSON {
		return writeJSON(out, resp)
	}
	if resp == nil || resp.Run == nil {
		fmt.Fprintln(out, "no run data")
		return nil
	}
	r := resp.Run
	fmt.Fprintf(out, "Run %s\n", r.GetId())
	fmt.Fprintf(out, "  Mode:      %s\n", modeLabel(r.GetMode()))
	fmt.Fprintf(out, "  Status:    %s\n", statusLabel(r.GetStatus()))
	fmt.Fprintf(out, "  Source:    %s\n", r.GetSource())
	if r.GetClusterId() != "" {
		fmt.Fprintf(out, "  Cluster:   %s\n", r.GetClusterId())
	}
	if r.GetAgentId() != "" {
		fmt.Fprintf(out, "  Agent:     %s\n", r.GetAgentId())
	}
	fmt.Fprintf(out, "  Started:   %s\n", formatTimestamp(r.GetStartedAt()))
	if r.GetEndedAt() != nil {
		fmt.Fprintf(out, "  Ended:     %s (%s)\n", formatTimestamp(r.GetEndedAt()),
			formatDuration(r.GetStartedAt(), r.GetEndedAt()))
	} else {
		fmt.Fprintln(out, "  Ended:     (still running)")
	}
	fmt.Fprintf(out, "  Counts:    %s\n", countsSummary(r.GetAggregates()))
	if r.GetErrorMessage() != "" {
		fmt.Fprintf(out, "  Error:     %s\n", r.GetErrorMessage())
	}
	if len(resp.GetDeclarations()) > 0 {
		fmt.Fprintln(out, "\nDeclarations:")
		for _, d := range resp.GetDeclarations() {
			detail := pickShowDetail(d)
			fmt.Fprintf(out, "  [%s] %-32s %s\n", outcomeBadge(d.GetOutcome()), d.GetDeclId(), detail)
		}
	}
	return nil
}

// pickShowDetail returns the most useful per-decl annotation: the
// apply diff, then the check diff, then the error message.
func pickShowDetail(d *v1.StateDeclarationResult) string {
	switch {
	case d.GetApplyDiff() != "":
		return "applied (" + d.GetApplyDiff() + ")"
	case d.GetCheckDiff() != "":
		return d.GetCheckDiff()
	case d.GetErrorMessage() != "":
		return d.GetErrorMessage()
	default:
		return ""
	}
}

// countsSummary collapses the 6 aggregate counters into a compact
// "T=N C=N U=N F=N S=N D=N" string. Zeros stay visible so an
// operator can tell at a glance whether all dimensions are zero.
func countsSummary(a *v1.StateRunAggregates) string {
	if a == nil {
		return "(no counts)"
	}
	return fmt.Sprintf("T=%d C=%d U=%d F=%d S=%d D=%d",
		a.GetTotal(), a.GetChanged(), a.GetUnchanged(),
		a.GetFailed(), a.GetSkipped(), a.GetDrifted())
}

// formatTimestamp pulls a *timestamppb.Timestamp into a stable
// "YYYY-MM-DD HH:MM:SS UTC" string. Empty timestamp returns "(none)".
func formatTimestamp(ts interface{ AsTime() time.Time }) string {
	if ts == nil {
		return "(none)"
	}
	t := ts.AsTime()
	if t.IsZero() {
		return "(none)"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

// formatDuration renders started→ended as a milliseconds string. If
// ended is zero we return "—".
func formatDuration(started, ended interface{ AsTime() time.Time }) string {
	if started == nil || ended == nil {
		return "—"
	}
	startT := started.AsTime()
	endT := ended.AsTime()
	if startT.IsZero() || endT.IsZero() {
		return "—"
	}
	d := endT.Sub(startT)
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return d.Truncate(time.Millisecond).String()
}

// truncate shortens s to at most n bytes, suffixing "…" if cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return s[:n-1] + "…"
}

// writeJSON is the centralised JSON writer — newline-terminated so
// streaming subcommands produce NDJSON usable with `jq -c .` etc.
func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
