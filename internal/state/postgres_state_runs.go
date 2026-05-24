// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const stateRunSelectPg = `SELECT
    id, mode, source, cluster_id, agent_id,
    started_at, ended_at, status, error_message,
    total_count, changed_count, unchanged_count,
    failed_count, skipped_count, drifted_count,
    declarations_json
FROM state_runs`

const stateRunResultSelectPg = `SELECT
    run_id, decl_id, module, outcome,
    check_matches, check_diff,
    apply_changed, apply_diff, apply_comment,
    test_result, error_message,
    started_at, duration_ms
FROM state_run_results`

func (s *PostgreSQLStore) CreateStateRun(ctx context.Context, r *StateRunRecord) error {
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
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16::jsonb)`,
		r.ID, string(r.Mode), r.Source, r.ClusterID, r.AgentID,
		r.StartedAt.UTC(), nullableTime(r.EndedAt),
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

func (s *PostgreSQLStore) FinalizeStateRun(ctx context.Context, id string, end StateRunEnd) error {
	res, err := s.db.ExecContext(ctx, `UPDATE state_runs SET
    status = $1, ended_at = $2, error_message = $3,
    total_count = $4, changed_count = $5, unchanged_count = $6,
    failed_count = $7, skipped_count = $8, drifted_count = $9
WHERE id = $10`,
		string(end.Status), nullableTime(end.EndedAt), end.ErrorMessage,
		end.Total, end.Changed, end.Unchanged,
		end.Failed, end.Skipped, end.Drifted,
		id,
	)
	if err != nil {
		return fmt.Errorf("state: FinalizeStateRun: %w", err)
	}
	return affectsRow(res)
}

func (s *PostgreSQLStore) AddStateRunResult(ctx context.Context, runID string, r *StateRunResultRecord) error {
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
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		runID, r.DeclID, r.Module, string(r.Outcome),
		nullableBool(r.CheckMatches), r.CheckDiff,
		nullableBool(r.ApplyChanged), r.ApplyDiff, r.ApplyComment,
		nullableBool(r.TestResult), r.ErrorMessage,
		r.StartedAt.UTC(), r.DurationMS,
	)
	if err != nil {
		return fmt.Errorf("state: AddStateRunResult: %w", err)
	}
	return nil
}

func (s *PostgreSQLStore) GetStateRun(ctx context.Context, id string) (*StateRunRecord, []*StateRunResultRecord, error) {
	row := s.db.QueryRowContext(ctx, stateRunSelectPg+" WHERE id = $1", id)
	header, err := scanStateRunPg(row)
	if err != nil {
		return nil, nil, translateSQLError(err)
	}
	rows, err := s.db.QueryContext(ctx, stateRunResultSelectPg+" WHERE run_id = $1 ORDER BY started_at ASC, decl_id ASC", id)
	if err != nil {
		return nil, nil, fmt.Errorf("state: GetStateRun results: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var results []*StateRunResultRecord
	for rows.Next() {
		rr, scanErr := scanStateRunResultPg(rows)
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

func (s *PostgreSQLStore) ListStateRuns(ctx context.Context, filter StateRunFilter) ([]*StateRunRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedStateRunSortColumns); err != nil {
		return nil, err
	}
	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(stateRunSelectPg)
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if filter.AgentID != "" {
		add("agent_id = $%d", filter.AgentID)
	}
	if filter.Mode != "" {
		add("mode = $%d", string(filter.Mode))
	}
	if filter.Status != "" {
		add("status = $%d", string(filter.Status))
	}
	if !filter.After.IsZero() {
		add("started_at >= $%d", filter.After.UTC())
	}
	if !filter.Before.IsZero() {
		add("started_at < $%d", filter.Before.UTC())
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
		r, scanErr := scanStateRunPg(rows)
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

func (s *PostgreSQLStore) DeleteStateRunsBefore(ctx context.Context, cutoff time.Time, statuses []StateRunStatus) (int, error) {
	if len(statuses) == 0 {
		return 0, errors.New("state: DeleteStateRunsBefore: statuses must be non-empty")
	}
	args := []any{cutoff.UTC()}
	placeholders := make([]string, 0, len(statuses))
	for _, st := range statuses {
		args = append(args, string(st))
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
	}
	// Placeholders are "$2,$3,..." derived from len(statuses); not
	// user input. gosec G202 is a false positive.
	stmt := `DELETE FROM state_runs WHERE ended_at IS NOT NULL AND ended_at < $1 AND status IN (` + strings.Join(placeholders, ",") + `)` //nolint:gosec // placeholders are computed from len(statuses), not user input
	res, err := s.db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return 0, fmt.Errorf("state: DeleteStateRunsBefore: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("state: DeleteStateRunsBefore rows: %w", err)
	}
	return int(n), nil
}

// nullableBool converts a sql.NullBool to the value Postgres'
// pgx driver writes as BOOLEAN/NULL. sql.NullBool is wire-compatible
// with both lib/pq and pgx; pass it through directly.
func nullableBool(b sql.NullBool) any {
	if !b.Valid {
		return nil
	}
	return b.Bool
}

func scanStateRunPg(row rowScanner) (*StateRunRecord, error) {
	var (
		r       StateRunRecord
		mode    string
		status  string
		endedAt sql.NullTime
	)
	if err := row.Scan(
		&r.ID, &mode, &r.Source, &r.ClusterID, &r.AgentID,
		&r.StartedAt, &endedAt, &status, &r.ErrorMessage,
		&r.Total, &r.Changed, &r.Unchanged,
		&r.Failed, &r.Skipped, &r.Drifted,
		&r.DeclarationsJSON,
	); err != nil {
		return nil, err
	}
	r.Mode = StateRunMode(mode)
	r.Status = StateRunStatus(status)
	r.StartedAt = r.StartedAt.UTC()
	if endedAt.Valid {
		r.EndedAt = endedAt.Time.UTC()
	}
	return &r, nil
}

func scanStateRunResultPg(row rowScanner) (*StateRunResultRecord, error) {
	var (
		r            StateRunResultRecord
		outcome      string
		checkMatches sql.NullBool
		applyChanged sql.NullBool
		testResult   sql.NullBool
	)
	if err := row.Scan(
		&r.RunID, &r.DeclID, &r.Module, &outcome,
		&checkMatches, &r.CheckDiff,
		&applyChanged, &r.ApplyDiff, &r.ApplyComment,
		&testResult, &r.ErrorMessage,
		&r.StartedAt, &r.DurationMS,
	); err != nil {
		return nil, err
	}
	r.Outcome = StateRunOutcome(outcome)
	r.CheckMatches = checkMatches
	r.ApplyChanged = applyChanged
	r.TestResult = testResult
	r.StartedAt = r.StartedAt.UTC()
	return &r, nil
}
