package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const stateRunSelect = `SELECT
    id, mode, source, cluster_id, agent_id,
    started_at, ended_at, status, error_message,
    total_count, changed_count, unchanged_count,
    failed_count, skipped_count, drifted_count,
    declarations_json
FROM state_runs`

const stateRunResultSelect = `SELECT
    run_id, decl_id, module, outcome,
    check_matches, check_diff,
    apply_changed, apply_diff, apply_comment,
    test_result, error_message,
    started_at, duration_ms
FROM state_run_results`

func (s *SQLiteStore) CreateStateRun(ctx context.Context, r *StateRunRecord) error {
	if r == nil {
		return fmt.Errorf("state: CreateStateRun: nil record")
	}
	if r.ID == "" {
		return fmt.Errorf("state: CreateStateRun: empty ID")
	}
	declJSON := r.DeclarationsJSON
	if declJSON == "" {
		declJSON = "[]"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO state_runs (
    id, mode, source, cluster_id, agent_id,
    started_at, ended_at, status, error_message,
    total_count, changed_count, unchanged_count,
    failed_count, skipped_count, drifted_count,
    declarations_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, string(r.Mode), r.Source, r.ClusterID, r.AgentID,
		tsArgRequired(r.StartedAt), tsArgNullable(r.EndedAt),
		string(r.Status), r.ErrorMessage,
		r.Total, r.Changed, r.Unchanged,
		r.Failed, r.Skipped, r.Drifted,
		declJSON,
	)
	if err != nil {
		return fmt.Errorf("state: CreateStateRun: %w", err)
	}
	return nil
}

func (s *SQLiteStore) FinalizeStateRun(ctx context.Context, id string, end StateRunEnd) error {
	res, err := s.db.ExecContext(ctx, `UPDATE state_runs SET
    status = ?, ended_at = ?, error_message = ?,
    total_count = ?, changed_count = ?, unchanged_count = ?,
    failed_count = ?, skipped_count = ?, drifted_count = ?
WHERE id = ?`,
		string(end.Status), tsArgNullable(end.EndedAt), end.ErrorMessage,
		end.Total, end.Changed, end.Unchanged,
		end.Failed, end.Skipped, end.Drifted,
		id,
	)
	if err != nil {
		return fmt.Errorf("state: FinalizeStateRun: %w", err)
	}
	return affectsRow(res)
}

func (s *SQLiteStore) AddStateRunResult(ctx context.Context, runID string, r *StateRunResultRecord) error {
	if r == nil {
		return fmt.Errorf("state: AddStateRunResult: nil record")
	}
	if runID == "" || r.DeclID == "" {
		return fmt.Errorf("state: AddStateRunResult: empty runID or DeclID")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO state_run_results (
    run_id, decl_id, module, outcome,
    check_matches, check_diff,
    apply_changed, apply_diff, apply_comment,
    test_result, error_message,
    started_at, duration_ms
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		runID, r.DeclID, r.Module, string(r.Outcome),
		nullableBoolInt(r.CheckMatches), r.CheckDiff,
		nullableBoolInt(r.ApplyChanged), r.ApplyDiff, r.ApplyComment,
		nullableBoolInt(r.TestResult), r.ErrorMessage,
		tsArgRequired(r.StartedAt), r.DurationMS,
	)
	if err != nil {
		return fmt.Errorf("state: AddStateRunResult: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetStateRun(ctx context.Context, id string) (*StateRunRecord, []*StateRunResultRecord, error) {
	row := s.db.QueryRowContext(ctx, stateRunSelect+" WHERE id = ?", id)
	header, err := scanStateRun(row)
	if err != nil {
		return nil, nil, translateSQLError(err)
	}
	rows, err := s.db.QueryContext(ctx, stateRunResultSelect+" WHERE run_id = ? ORDER BY started_at ASC, decl_id ASC", id)
	if err != nil {
		return nil, nil, fmt.Errorf("state: GetStateRun results: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var results []*StateRunResultRecord
	for rows.Next() {
		rr, scanErr := scanStateRunResult(rows)
		if scanErr != nil {
			return nil, nil, fmt.Errorf("state: GetStateRun scan result: %w", scanErr)
		}
		results = append(results, rr)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("state: GetStateRun rows: %w", err)
	}
	return header, results, nil
}

func (s *SQLiteStore) ListStateRuns(ctx context.Context, filter StateRunFilter) ([]*StateRunRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedStateRunSortColumns); err != nil {
		return nil, err
	}
	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(stateRunSelect)
	if filter.AgentID != "" {
		conds = append(conds, "agent_id = ?")
		args = append(args, filter.AgentID)
	}
	if filter.Mode != "" {
		conds = append(conds, "mode = ?")
		args = append(args, string(filter.Mode))
	}
	if filter.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, string(filter.Status))
	}
	if !filter.After.IsZero() {
		conds = append(conds, "started_at >= ?")
		args = append(args, tsArgRequired(filter.After))
	}
	if !filter.Before.IsZero() {
		conds = append(conds, "started_at < ?")
		args = append(args, tsArgRequired(filter.Before))
	}
	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}
	sb.WriteString(orderByClause(filter.SortColumn, "started_at", filter.SortDesc || filter.SortColumn == ""))
	sb.WriteString(limitOffsetClause(filter.Limit, filter.Offset))

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("state: ListStateRuns: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*StateRunRecord
	for rows.Next() {
		r, scanErr := scanStateRun(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("state: ListStateRuns scan: %w", scanErr)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state: ListStateRuns rows: %w", err)
	}
	return out, nil
}

func (s *SQLiteStore) DeleteStateRunsBefore(ctx context.Context, cutoff time.Time, statuses []StateRunStatus) (int, error) {
	if len(statuses) == 0 {
		return 0, errors.New("state: DeleteStateRunsBefore: statuses must be non-empty")
	}
	placeholders := strings.Repeat("?,", len(statuses))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := []any{tsArgRequired(cutoff)}
	for _, st := range statuses {
		args = append(args, string(st))
	}
	// ended_at IS NOT NULL guards the never-finalised rows from
	// retention sweeps even if a 'running' status sneaks into the
	// allowlist. The concatenated placeholders are a fixed-length
	// "?,?,?" string derived from len(statuses) — never from user
	// input — so the gosec G202 warning is a false positive.
	stmt := `DELETE FROM state_runs WHERE ended_at IS NOT NULL AND ended_at < ? AND status IN (` + placeholders + `)` //nolint:gosec // placeholders are computed from len(statuses), not user input
	res, err := s.db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return 0, fmt.Errorf("state: DeleteStateRunsBefore: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("state: DeleteStateRunsBefore rows: %w", err)
	}
	// SQLite has FK ON DELETE CASCADE only when foreign_keys pragma
	// is ON. Our newSQLiteStore enables it; assert via dependent
	// cleanup test.
	return int(n), nil
}

// nullableBoolInt encodes a sql.NullBool as 0/1/NULL for the
// SQLite INTEGER columns we use for tri-state fields.
func nullableBoolInt(b sql.NullBool) any {
	if !b.Valid {
		return nil
	}
	if b.Bool {
		return 1
	}
	return 0
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanStateRun(row rowScanner) (*StateRunRecord, error) {
	var (
		r         StateRunRecord
		mode      string
		status    string
		startedAt string
		endedAt   sql.NullString
	)
	if err := row.Scan(
		&r.ID, &mode, &r.Source, &r.ClusterID, &r.AgentID,
		&startedAt, &endedAt, &status, &r.ErrorMessage,
		&r.Total, &r.Changed, &r.Unchanged,
		&r.Failed, &r.Skipped, &r.Drifted,
		&r.DeclarationsJSON,
	); err != nil {
		return nil, err
	}
	r.Mode = StateRunMode(mode)
	r.Status = StateRunStatus(status)
	t, err := tsParseRequired(startedAt)
	if err != nil {
		return nil, err
	}
	r.StartedAt = t
	endT, err := tsParseNullable(endedAt)
	if err != nil {
		return nil, err
	}
	r.EndedAt = endT
	return &r, nil
}

func scanStateRunResult(row rowScanner) (*StateRunResultRecord, error) {
	var (
		r            StateRunResultRecord
		outcome      string
		checkMatches sql.NullInt64
		applyChanged sql.NullInt64
		testResult   sql.NullInt64
		startedAt    string
	)
	if err := row.Scan(
		&r.RunID, &r.DeclID, &r.Module, &outcome,
		&checkMatches, &r.CheckDiff,
		&applyChanged, &r.ApplyDiff, &r.ApplyComment,
		&testResult, &r.ErrorMessage,
		&startedAt, &r.DurationMS,
	); err != nil {
		return nil, err
	}
	r.Outcome = StateRunOutcome(outcome)
	r.CheckMatches = nullInt64ToBool(checkMatches)
	r.ApplyChanged = nullInt64ToBool(applyChanged)
	r.TestResult = nullInt64ToBool(testResult)
	t, err := tsParseRequired(startedAt)
	if err != nil {
		return nil, err
	}
	r.StartedAt = t
	return &r, nil
}

func nullInt64ToBool(n sql.NullInt64) sql.NullBool {
	if !n.Valid {
		return sql.NullBool{}
	}
	return sql.NullBool{Valid: true, Bool: n.Int64 != 0}
}
