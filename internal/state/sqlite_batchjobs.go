package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const batchJobSelect = `SELECT
    id, target, command, args, status, concurrency,
    total_agents, completed_agents, successful_agents, failed_agents,
    created_at, started_at, completed_at
FROM batch_jobs`

const batchAgentResultSelect = `SELECT
    batch_job_id, agent_id, success,
    exit_code, COALESCE(error, ''),
    started_at, completed_at
FROM batch_agent_results`

func (s *SQLiteStore) CreateBatchJob(ctx context.Context, b *BatchJobRecord) error {
	if b == nil {
		return fmt.Errorf("state: CreateBatchJob: nil record")
	}

	target, err := marshalJSONColumn(b.Target)
	if err != nil {
		return fmt.Errorf("state: CreateBatchJob target: %w", err)
	}
	args, err := marshalJSONColumn(b.Args)
	if err != nil {
		return fmt.Errorf("state: CreateBatchJob args: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO batch_jobs (
    id, target, command, args, status, concurrency,
    total_agents, completed_agents, successful_agents, failed_agents,
    created_at, started_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, target, b.Command, args, string(b.Status), b.Concurrency,
		b.TotalAgents, b.CompletedAgents, b.SuccessfulAgents, b.FailedAgents,
		tsArgRequired(b.CreatedAt), tsArgNullable(b.StartedAt), tsArgNullable(b.CompletedAt),
	)
	if err != nil {
		return fmt.Errorf("state: CreateBatchJob: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetBatchJob(ctx context.Context, id string) (*BatchJobRecord, error) {
	row := s.db.QueryRowContext(ctx, batchJobSelect+" WHERE id = ?", id)
	b, err := scanBatchJob(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return b, nil
}

func (s *SQLiteStore) ListBatchJobs(ctx context.Context, filter BatchJobFilter) ([]*BatchJobRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedBatchJobSortColumns); err != nil {
		return nil, err
	}

	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(batchJobSelect)

	if filter.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, string(filter.Status))
	}
	if !filter.Since.IsZero() {
		conds = append(conds, "created_at >= ?")
		args = append(args, tsArgRequired(filter.Since))
	}
	if !filter.Until.IsZero() {
		conds = append(conds, "created_at <= ?")
		args = append(args, tsArgRequired(filter.Until))
	}
	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}

	sb.WriteString(orderByClause(filter.SortColumn, "created_at", filter.SortDesc))
	sb.WriteString(limitOffsetClause(filter.Limit, filter.Offset))

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("state: ListBatchJobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*BatchJobRecord
	for rows.Next() {
		b, err := scanBatchJob(rows)
		if err != nil {
			return nil, fmt.Errorf("state: ListBatchJobs scan: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state: ListBatchJobs iterate: %w", err)
	}
	return out, nil
}

func (s *SQLiteStore) UpdateBatchJobCounts(ctx context.Context, id string, completed, successful, failed int) error {
	res, err := s.db.ExecContext(ctx, `UPDATE batch_jobs SET
    completed_agents = ?, successful_agents = ?, failed_agents = ?
WHERE id = ?`,
		completed, successful, failed, id)
	if err != nil {
		return fmt.Errorf("state: UpdateBatchJobCounts: %w", err)
	}
	return affectsRow(res)
}

func (s *SQLiteStore) MarkBatchJobRunning(ctx context.Context, id string, startedAt time.Time) error {
	if startedAt.IsZero() {
		return fmt.Errorf("state: MarkBatchJobRunning: startedAt must be non-zero")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE batch_jobs SET status = ?, started_at = ? WHERE id = ?`,
		string(BatchJobStatusRunning), tsArgRequired(startedAt), id,
	)
	if err != nil {
		return fmt.Errorf("state: MarkBatchJobRunning: %w", err)
	}
	return affectsRow(res)
}

func (s *SQLiteStore) FinalizeBatchJob(ctx context.Context, id string, status BatchJobStatus, completedAt time.Time) error {
	if !isTerminalBatchStatus(status) {
		return fmt.Errorf("state: FinalizeBatchJob: %q is not a terminal status", status)
	}
	if completedAt.IsZero() {
		return fmt.Errorf("state: FinalizeBatchJob: completedAt must be non-zero")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE batch_jobs SET status = ?, completed_at = ? WHERE id = ?`,
		string(status), tsArgRequired(completedAt), id,
	)
	if err != nil {
		return fmt.Errorf("state: FinalizeBatchJob: %w", err)
	}
	return affectsRow(res)
}

func (s *SQLiteStore) CreateBatchAgentResult(ctx context.Context, r *BatchAgentResultRecord) error {
	if r == nil {
		return fmt.Errorf("state: CreateBatchAgentResult: nil record")
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO batch_agent_results (
    batch_job_id, agent_id, success, exit_code, error, started_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.BatchJobID, r.AgentID, boolToInt(r.Success),
		sql.NullInt64{Int64: int64(r.ExitCode), Valid: r.ExitCode != 0 || !r.Success},
		nullableString(r.Error),
		tsArgNullable(r.StartedAt), tsArgNullable(r.CompletedAt),
	)
	if err != nil {
		return fmt.Errorf("state: CreateBatchAgentResult: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListBatchAgentResults(ctx context.Context, batchJobID string) ([]*BatchAgentResultRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		batchAgentResultSelect+` WHERE batch_job_id = ? ORDER BY agent_id ASC`,
		batchJobID)
	if err != nil {
		return nil, fmt.Errorf("state: ListBatchAgentResults: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*BatchAgentResultRecord
	for rows.Next() {
		r, err := scanBatchAgentResult(rows)
		if err != nil {
			return nil, fmt.Errorf("state: ListBatchAgentResults scan: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state: ListBatchAgentResults iterate: %w", err)
	}
	return out, nil
}

// ---- helpers (batch-specific) ---------------------------------------------

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scanBatchJob(r rowLike) (*BatchJobRecord, error) {
	var (
		b                BatchJobRecord
		target, argsJSON string
		statusRaw        string
		createdAt        string
		startedAt, doneAt sql.NullString
	)
	if err := r.Scan(
		&b.ID, &target, &b.Command, &argsJSON, &statusRaw, &b.Concurrency,
		&b.TotalAgents, &b.CompletedAgents, &b.SuccessfulAgents, &b.FailedAgents,
		&createdAt, &startedAt, &doneAt,
	); err != nil {
		return nil, err
	}

	b.Status = BatchJobStatus(statusRaw)

	if err := unmarshalJSONColumn(target, &b.Target); err != nil {
		return nil, fmt.Errorf("state: scanBatchJob target: %w", err)
	}
	if err := unmarshalJSONColumn(argsJSON, &b.Args); err != nil {
		return nil, fmt.Errorf("state: scanBatchJob args: %w", err)
	}

	var err error
	if b.CreatedAt, err = tsParseRequired(createdAt); err != nil {
		return nil, fmt.Errorf("state: scanBatchJob created_at: %w", err)
	}
	if b.StartedAt, err = tsParseNullable(startedAt); err != nil {
		return nil, fmt.Errorf("state: scanBatchJob started_at: %w", err)
	}
	if b.CompletedAt, err = tsParseNullable(doneAt); err != nil {
		return nil, fmt.Errorf("state: scanBatchJob completed_at: %w", err)
	}

	return &b, nil
}

func scanBatchAgentResult(r rowLike) (*BatchAgentResultRecord, error) {
	var (
		out                BatchAgentResultRecord
		successInt         int
		exitCode           sql.NullInt64
		startedAt, doneAt  sql.NullString
	)
	if err := r.Scan(
		&out.BatchJobID, &out.AgentID, &successInt,
		&exitCode, &out.Error,
		&startedAt, &doneAt,
	); err != nil {
		return nil, err
	}

	out.Success = successInt != 0
	if exitCode.Valid {
		out.ExitCode = int(exitCode.Int64)
	}

	var err error
	if out.StartedAt, err = tsParseNullable(startedAt); err != nil {
		return nil, fmt.Errorf("state: scanBatchAgentResult started_at: %w", err)
	}
	if out.CompletedAt, err = tsParseNullable(doneAt); err != nil {
		return nil, fmt.Errorf("state: scanBatchAgentResult completed_at: %w", err)
	}

	return &out, nil
}
