// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"database/sql"
	"encoding/json"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// sqlBool wraps a bool as sql.NullBool with Valid=true. Used to
// populate the tri-state columns in state.StateRunResultRecord
// when the engine actually ran a phase.
func sqlBool(b bool) sql.NullBool { return sql.NullBool{Valid: true, Bool: b} }

// timeOrNow yields a non-zero time. Used by the bridge to stamp
// EndedAt on FinalizeStateRun calls — if the caller passes the zero
// time we default to time.Now() at call time.
type timeOrNow struct{ t time.Time }

func nowOr(t time.Time) timeOrNow { return timeOrNow{t: t} }
func (t timeOrNow) Time() time.Time {
	if t.t.IsZero() {
		return time.Now().UTC()
	}
	return t.t
}

// state_bridge.go translates between the three universes Task 9 sits
// at the seam of:
//
//   • statemgmt.* (in-memory engine types)
//   • state.StateRun* (persistence records)
//   • v1.* protobuf (gRPC wire types)
//
// The bridge is intentionally a pile of pure functions — no
// goroutines, no logging, no I/O — so it's trivially testable and
// the gRPC server file stays focused on RPC plumbing.

// ---- Outcome / Status / Severity / DriftState enum mapping ---------

// outcomeToProto maps the in-memory enum to the proto enum. Used by
// the server when streaming per-decl results.
func outcomeToProto(o statemgmt.Outcome) v1.StateRunOutcome {
	switch o {
	case statemgmt.OutcomeUnchanged:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_UNCHANGED
	case statemgmt.OutcomeChanged:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED
	case statemgmt.OutcomeNoOp:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_NO_OP
	case statemgmt.OutcomeFailed:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_FAILED
	case statemgmt.OutcomeDriftDetected:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_DRIFT_DETECTED
	case statemgmt.OutcomeSkipped:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_SKIPPED
	default:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_UNSPECIFIED
	}
}

// outcomeToRecord maps the in-memory enum to the persisted string
// enum used by state.StateRunResultRecord.
func outcomeToRecord(o statemgmt.Outcome) state.StateRunOutcome {
	switch o {
	case statemgmt.OutcomeUnchanged:
		return state.StateRunOutcomeUnchanged
	case statemgmt.OutcomeChanged:
		return state.StateRunOutcomeChanged
	case statemgmt.OutcomeNoOp:
		return state.StateRunOutcomeNoOp
	case statemgmt.OutcomeFailed:
		return state.StateRunOutcomeFailed
	case statemgmt.OutcomeDriftDetected:
		return state.StateRunOutcomeDriftDetected
	case statemgmt.OutcomeSkipped:
		return state.StateRunOutcomeSkipped
	default:
		return state.StateRunOutcome("unknown")
	}
}

func recordStatusToProto(s state.StateRunStatus) v1.StateRunStatus {
	switch s {
	case state.StateRunStatusRunning:
		return v1.StateRunStatus_STATE_RUN_STATUS_RUNNING
	case state.StateRunStatusCompleted:
		return v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED
	case state.StateRunStatusFailed:
		return v1.StateRunStatus_STATE_RUN_STATUS_FAILED
	case state.StateRunStatusCancelled:
		return v1.StateRunStatus_STATE_RUN_STATUS_CANCELLED
	default:
		return v1.StateRunStatus_STATE_RUN_STATUS_UNSPECIFIED
	}
}

func protoStatusToRecord(s v1.StateRunStatus) state.StateRunStatus {
	switch s {
	case v1.StateRunStatus_STATE_RUN_STATUS_RUNNING:
		return state.StateRunStatusRunning
	case v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED:
		return state.StateRunStatusCompleted
	case v1.StateRunStatus_STATE_RUN_STATUS_FAILED:
		return state.StateRunStatusFailed
	case v1.StateRunStatus_STATE_RUN_STATUS_CANCELLED:
		return state.StateRunStatusCancelled
	default:
		return state.StateRunStatus("")
	}
}

func recordModeToProto(m state.StateRunMode) v1.StateRunMode {
	switch m {
	case state.StateRunModeApply:
		return v1.StateRunMode_STATE_RUN_MODE_APPLY
	case state.StateRunModeCheck:
		return v1.StateRunMode_STATE_RUN_MODE_CHECK
	case state.StateRunModeDrift:
		return v1.StateRunMode_STATE_RUN_MODE_DRIFT
	default:
		return v1.StateRunMode_STATE_RUN_MODE_UNSPECIFIED
	}
}

func protoModeToRecord(m v1.StateRunMode) state.StateRunMode {
	switch m {
	case v1.StateRunMode_STATE_RUN_MODE_APPLY:
		return state.StateRunModeApply
	case v1.StateRunMode_STATE_RUN_MODE_CHECK:
		return state.StateRunModeCheck
	case v1.StateRunMode_STATE_RUN_MODE_DRIFT:
		return state.StateRunModeDrift
	default:
		return state.StateRunMode("")
	}
}

func severityToProto(s statemgmt.DriftSeverity) v1.DriftSeverity {
	switch s {
	case statemgmt.DriftSeverityNone:
		return v1.DriftSeverity_DRIFT_SEVERITY_NONE
	case statemgmt.DriftSeverityLow:
		return v1.DriftSeverity_DRIFT_SEVERITY_LOW
	case statemgmt.DriftSeverityMedium:
		return v1.DriftSeverity_DRIFT_SEVERITY_MEDIUM
	case statemgmt.DriftSeverityHigh:
		return v1.DriftSeverity_DRIFT_SEVERITY_HIGH
	case statemgmt.DriftSeverityCritical:
		return v1.DriftSeverity_DRIFT_SEVERITY_CRITICAL
	default:
		return v1.DriftSeverity_DRIFT_SEVERITY_UNSPECIFIED
	}
}

func driftStateToProto(s statemgmt.DriftState) v1.DriftState {
	switch s {
	case statemgmt.DriftStateInSync:
		return v1.DriftState_DRIFT_STATE_IN_SYNC
	case statemgmt.DriftStateDrifted:
		return v1.DriftState_DRIFT_STATE_DRIFTED
	case statemgmt.DriftStateError:
		return v1.DriftState_DRIFT_STATE_ERROR
	case statemgmt.DriftStateSkipped:
		return v1.DriftState_DRIFT_STATE_SKIPPED
	default:
		return v1.DriftState_DRIFT_STATE_UNSPECIFIED
	}
}

// ---- Result + Status translation ----------------------------------

// declResultToProto maps one engine DeclarationResult to the wire
// type the gRPC stream emits.
func declResultToProto(r *statemgmt.DeclarationResult) *v1.StateDeclarationResult {
	if r == nil {
		return nil
	}
	out := &v1.StateDeclarationResult{
		DeclId:     r.DeclID,
		Module:     r.Module,
		Outcome:    outcomeToProto(r.Outcome),
		StartedAt:  timestamppb.New(r.StartedAt),
		DurationMs: r.Duration.Milliseconds(),
	}
	if r.Check != nil {
		out.CheckDiff = r.Check.Diff
	}
	if r.Apply != nil {
		out.ApplyChanged = r.Apply.Changed
		out.ApplyDiff = r.Apply.Diff
		out.ApplyComment = r.Apply.Comment
	}
	if r.Error != nil {
		out.ErrorMessage = r.Error.Error()
	}
	return out
}

// declResultToRecord maps one engine DeclarationResult to the
// persistence record. runID is supplied by the caller.
func declResultToRecord(runID string, r *statemgmt.DeclarationResult) *state.StateRunResultRecord {
	if r == nil {
		return nil
	}
	rec := &state.StateRunResultRecord{
		RunID:        runID,
		DeclID:       r.DeclID,
		Module:       r.Module,
		Outcome:      outcomeToRecord(r.Outcome),
		StartedAt:    r.StartedAt,
		DurationMS:   r.Duration.Milliseconds(),
	}
	if r.Check != nil {
		rec.CheckMatches = sqlBool(r.Check.Matches)
		rec.CheckDiff = r.Check.Diff
	}
	if r.Apply != nil {
		rec.ApplyChanged = sqlBool(r.Apply.Changed)
		rec.ApplyDiff = r.Apply.Diff
		rec.ApplyComment = r.Apply.Comment
	}
	if r.Test != nil {
		rec.TestResult = sqlBool(*r.Test)
	}
	if r.Error != nil {
		rec.ErrorMessage = r.Error.Error()
	}
	return rec
}

// driftStatusToProto maps one engine DriftStatus to the wire type.
func driftStatusToProto(s statemgmt.DriftStatus) *v1.DriftDeclaration {
	out := &v1.DriftDeclaration{
		DeclId:   s.DeclID,
		Module:   s.Module,
		State:    driftStateToProto(s.State),
		Severity: severityToProto(s.Severity),
		Diff:     s.Diff,
	}
	if s.Error != nil {
		out.ErrorMessage = s.Error.Error()
	}
	return out
}

// runRecordToProto maps a header record to the wire StateRun.
func runRecordToProto(r *state.StateRunRecord) *v1.StateRun {
	if r == nil {
		return nil
	}
	out := &v1.StateRun{
		Id:           r.ID,
		Mode:         recordModeToProto(r.Mode),
		Source:       r.Source,
		ClusterId:    r.ClusterID,
		AgentId:      r.AgentID,
		StartedAt:    timestamppb.New(r.StartedAt),
		Status:       recordStatusToProto(r.Status),
		ErrorMessage: r.ErrorMessage,
		Aggregates: &v1.StateRunAggregates{
			Total:     int32(r.Total),     //nolint:gosec // counters cap well below int32 max
			Changed:   int32(r.Changed),   //nolint:gosec
			Unchanged: int32(r.Unchanged), //nolint:gosec
			Failed:    int32(r.Failed),    //nolint:gosec
			Skipped:   int32(r.Skipped),   //nolint:gosec
			Drifted:   int32(r.Drifted),   //nolint:gosec
		},
	}
	if !r.EndedAt.IsZero() {
		out.EndedAt = timestamppb.New(r.EndedAt)
	}
	return out
}

// resultRecordToProto reconstitutes a StateDeclarationResult from a
// persisted row (used by GetStateStatus).
func resultRecordToProto(r *state.StateRunResultRecord) *v1.StateDeclarationResult {
	if r == nil {
		return nil
	}
	out := &v1.StateDeclarationResult{
		DeclId:       r.DeclID,
		Module:       r.Module,
		Outcome:      recordOutcomeToProto(r.Outcome),
		CheckDiff:    r.CheckDiff,
		ApplyDiff:    r.ApplyDiff,
		ApplyComment: r.ApplyComment,
		ErrorMessage: r.ErrorMessage,
		StartedAt:    timestamppb.New(r.StartedAt),
		DurationMs:   r.DurationMS,
	}
	if r.ApplyChanged.Valid {
		out.ApplyChanged = r.ApplyChanged.Bool
	}
	return out
}

func recordOutcomeToProto(o state.StateRunOutcome) v1.StateRunOutcome {
	switch o {
	case state.StateRunOutcomeUnchanged:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_UNCHANGED
	case state.StateRunOutcomeChanged:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED
	case state.StateRunOutcomeNoOp:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_NO_OP
	case state.StateRunOutcomeFailed:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_FAILED
	case state.StateRunOutcomeDriftDetected:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_DRIFT_DETECTED
	case state.StateRunOutcomeSkipped:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_SKIPPED
	default:
		return v1.StateRunOutcome_STATE_RUN_OUTCOME_UNSPECIFIED
	}
}

// ---- Aggregates + DeclarationsJSON --------------------------------

// reportAggregatesToProto maps the engine's RunReport counters to the
// wire aggregate block.
func reportAggregatesToProto(rep *statemgmt.RunReport) *v1.StateRunAggregates {
	if rep == nil {
		return nil
	}
	return &v1.StateRunAggregates{
		Total:     int32(rep.Total),     //nolint:gosec
		Changed:   int32(rep.Changed),   //nolint:gosec
		Unchanged: int32(rep.Unchanged), //nolint:gosec
		Failed:    int32(rep.Failed),    //nolint:gosec
		Skipped:   int32(rep.Skipped),   //nolint:gosec
		Drifted:   int32(rep.Drifted),   //nolint:gosec
	}
}

// reportAggregatesToEnd builds the StateRunEnd to stamp via
// FinalizeStateRun.
func reportAggregatesToEnd(status state.StateRunStatus, endedAt timeOrNow, errMsg string, rep *statemgmt.RunReport) state.StateRunEnd {
	end := state.StateRunEnd{
		Status:       status,
		EndedAt:      endedAt.Time(),
		ErrorMessage: errMsg,
	}
	if rep != nil {
		end.Total = rep.Total
		end.Changed = rep.Changed
		end.Unchanged = rep.Unchanged
		end.Failed = rep.Failed
		end.Skipped = rep.Skipped
		end.Drifted = rep.Drifted
	}
	return end
}

// driftReportAggregatesToProto maps a DriftReport's counters.
func driftReportAggregatesToProto(rep *statemgmt.DriftReport) *v1.StateRunAggregates {
	if rep == nil {
		return nil
	}
	return &v1.StateRunAggregates{
		Total:     int32(rep.TotalChecked), //nolint:gosec
		Unchanged: int32(rep.InSync),       //nolint:gosec
		Failed:    int32(rep.Errors),       //nolint:gosec
		Skipped:   int32(rep.Skipped),      //nolint:gosec
		Drifted:   int32(rep.Drifted),      //nolint:gosec
	}
}

// declarationsToJSON encodes the rendered declaration list for
// StateRunRecord.DeclarationsJSON. The CLI's rollback flow (Task 10)
// reads this back out.
func declarationsToJSON(decls []*statemgmt.Declaration) (string, error) {
	if len(decls) == 0 {
		return "[]", nil
	}
	bytes, err := json.Marshal(decls)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// unmarshalDeclarations is the inverse of declarationsToJSON. Used by
// RollbackState to reconstitute the original run's rendered
// declarations from StateRunRecord.DeclarationsJSON.
func unmarshalDeclarations(s string) ([]*statemgmt.Declaration, error) {
	if s == "" || s == "[]" {
		return nil, nil
	}
	var out []*statemgmt.Declaration
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}
