package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const batchJobSelectPg = `SELECT
    id, target, command, args, status, concurrency,
    total_agents, completed_agents, successful_agents, failed_agents,
    created_at, started_at, completed_at
FROM batch_jobs`

const batchAgentResultSelectPg = `SELECT
    batch_job_id, agent_id, success,
    exit_code, COALESCE(error, ''),
    started_at, completed_at
FROM batch_agent_results`

func (s *PostgreSQLStore) CreateBatchJob(ctx context.Context, b *BatchJobRecord) error {
	if b == nil {
		return fmt.Errorf("state: CreateBatchJob: nil record")
	}

	target, err := marshalJSONBytes(b.Target)
	if err != nil {
		return fmt.Errorf("state: CreateBatchJob target: %w", err)
	}
	args, err := marshalJSONBytes(b.Args)
	if err != nil {
		return fmt.Errorf("state: CreateBatchJob args: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO batch_jobs (
    id, target, command, args, status, concurrency,
    total_agents, completed_agents, successful_agents, failed_agents,
    created_at, started_at, completed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		b.ID, target, b.Command, args, string(b.Status), b.Concurrency,
		b.TotalAgents, b.CompletedAgents, b.SuccessfulAgents, b.FailedAgents,
		b.CreatedAt.UTC(), nullableTime(b.StartedAt), nullableTime(b.CompletedAt),
	)
	if err != nil {
		return fmt.Errorf("state: CreateBatchJob: %w", err)
	}
	return nil
}

func (s *PostgreSQLStore) GetBatchJob(ctx context.Context, id string) (*BatchJobRecord, error) {
	row := s.db.QueryRowContext(ctx, batchJobSelectPg+" WHERE id = $1", id)
	b, err := scanBatchJobPg(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return b, nil
}

func (s *PostgreSQLStore) ListBatchJobs(ctx context.Context, filter BatchJobFilter) ([]*BatchJobRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedBatchJobSortColumns); err != nil {
		return nil, err
	}

	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(batchJobSelectPg)

	ph := newPlaceholderGen()
	if filter.Status != "" {
		conds = append(conds, "status = "+ph.next())
		args = append(args, string(filter.Status))
	}
	if !filter.Since.IsZero() {
		conds = append(conds, "created_at >= "+ph.next())
		args = append(args, filter.Since.UTC())
	}
	if !filter.Until.IsZero() {
		conds = append(conds, "created_at <= "+ph.next())
		args = append(args, filter.Until.UTC())
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
		b, err := scanBatchJobPg(rows)
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

func (s *PostgreSQLStore) UpdateBatchJobCounts(ctx context.Context, id string, completed, successful, failed int) error {
	res, err := s.db.ExecContext(ctx, `UPDATE batch_jobs SET
    completed_agents = $1, successful_agents = $2, failed_agents = $3
WHERE id = $4`,
		completed, successful, failed, id)
	if err != nil {
		return fmt.Errorf("state: UpdateBatchJobCounts: %w", err)
	}
	return affectsRow(res)
}

func (s *PostgreSQLStore) CreateBatchAgentResult(ctx context.Context, r *BatchAgentResultRecord) error {
	if r == nil {
		return fmt.Errorf("state: CreateBatchAgentResult: nil record")
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO batch_agent_results (
    batch_job_id, agent_id, success, exit_code, error, started_at, completed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		r.BatchJobID, r.AgentID, r.Success,
		sql.NullInt64{Int64: int64(r.ExitCode), Valid: r.ExitCode != 0 || !r.Success},
		nullableString(r.Error),
		nullableTime(r.StartedAt), nullableTime(r.CompletedAt),
	)
	if err != nil {
		return fmt.Errorf("state: CreateBatchAgentResult: %w", err)
	}
	return nil
}

func (s *PostgreSQLStore) ListBatchAgentResults(ctx context.Context, batchJobID string) ([]*BatchAgentResultRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		batchAgentResultSelectPg+` WHERE batch_job_id = $1 ORDER BY agent_id ASC`,
		batchJobID)
	if err != nil {
		return nil, fmt.Errorf("state: ListBatchAgentResults: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*BatchAgentResultRecord
	for rows.Next() {
		r, err := scanBatchAgentResultPg(rows)
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

// ---- helpers --------------------------------------------------------------

func scanBatchJobPg(r rowLike) (*BatchJobRecord, error) {
	var (
		b                 BatchJobRecord
		target, argsJSON  []byte
		statusRaw         string
		createdAt         time.Time
		startedAt, doneAt sql.NullTime
	)
	if err := r.Scan(
		&b.ID, &target, &b.Command, &argsJSON, &statusRaw, &b.Concurrency,
		&b.TotalAgents, &b.CompletedAgents, &b.SuccessfulAgents, &b.FailedAgents,
		&createdAt, &startedAt, &doneAt,
	); err != nil {
		return nil, err
	}

	b.Status = BatchJobStatus(statusRaw)

	if err := unmarshalJSONBytes(target, &b.Target); err != nil {
		return nil, fmt.Errorf("state: scanBatchJob target: %w", err)
	}
	if err := unmarshalJSONBytes(argsJSON, &b.Args); err != nil {
		return nil, fmt.Errorf("state: scanBatchJob args: %w", err)
	}

	b.CreatedAt = createdAt
	if startedAt.Valid {
		b.StartedAt = startedAt.Time
	}
	if doneAt.Valid {
		b.CompletedAt = doneAt.Time
	}

	return &b, nil
}

func scanBatchAgentResultPg(r rowLike) (*BatchAgentResultRecord, error) {
	var (
		out               BatchAgentResultRecord
		exitCode          sql.NullInt64
		startedAt, doneAt sql.NullTime
	)
	if err := r.Scan(
		&out.BatchJobID, &out.AgentID, &out.Success,
		&exitCode, &out.Error,
		&startedAt, &doneAt,
	); err != nil {
		return nil, err
	}

	if exitCode.Valid {
		out.ExitCode = int(exitCode.Int64)
	}
	if startedAt.Valid {
		out.StartedAt = startedAt.Time
	}
	if doneAt.Valid {
		out.CompletedAt = doneAt.Time
	}

	return &out, nil
}
