// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const commandSelectPg = `SELECT
    id, agent_id, command, args, env,
    COALESCE(working_dir, ''), COALESCE("user", ''), COALESCE(principal, ''),
    timeout_seconds, status,
    exit_code, COALESCE(stdout, ''), COALESCE(stderr, ''),
    started_at, completed_at
FROM commands`

func (s *PostgreSQLStore) CreateCommand(ctx context.Context, c *CommandRecord) error {
	if c == nil {
		return fmt.Errorf("state: CreateCommand: nil record")
	}

	args, err := marshalJSONBytes(c.Args)
	if err != nil {
		return fmt.Errorf("state: CreateCommand args: %w", err)
	}
	env, err := marshalJSONBytes(c.Env)
	if err != nil {
		return fmt.Errorf("state: CreateCommand env: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO commands (
    id, agent_id, command, args, env,
    working_dir, "user", principal, timeout_seconds, status,
    exit_code, stdout, stderr, started_at, completed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		c.ID, c.AgentID, c.Command, args, env,
		nullableString(c.WorkingDir), nullableString(c.User), nullableString(c.Principal),
		c.TimeoutSeconds, string(c.Status),
		nullableExitCode(c.ExitCode, c.Status),
		nullableString(c.Stdout), nullableString(c.Stderr),
		nullableTime(c.StartedAt), nullableTime(c.CompletedAt),
	)
	if err != nil {
		return fmt.Errorf("state: CreateCommand: %w", err)
	}
	return nil
}

func (s *PostgreSQLStore) GetCommand(ctx context.Context, id string) (*CommandRecord, error) {
	row := s.db.QueryRowContext(ctx, commandSelectPg+" WHERE id = $1", id)
	c, err := scanCommandPg(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return c, nil
}

func (s *PostgreSQLStore) ListCommands(ctx context.Context, filter CommandFilter) ([]*CommandRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedCommandSortColumns); err != nil {
		return nil, err
	}

	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(commandSelectPg)

	ph := newPlaceholderGen()
	if filter.AgentID != "" {
		conds = append(conds, "agent_id = "+ph.next())
		args = append(args, filter.AgentID)
	}
	if filter.Status != "" {
		conds = append(conds, "status = "+ph.next())
		args = append(args, string(filter.Status))
	}
	if !filter.Since.IsZero() {
		conds = append(conds, "started_at >= "+ph.next())
		args = append(args, filter.Since.UTC())
	}
	if !filter.Until.IsZero() {
		conds = append(conds, "started_at <= "+ph.next())
		args = append(args, filter.Until.UTC())
	}
	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}

	sb.WriteString(orderByClause(filter.SortColumn, "started_at", filter.SortDesc))
	sb.WriteString(limitOffsetClause(filter.Limit, filter.Offset))

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("state: ListCommands: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*CommandRecord
	for rows.Next() {
		c, err := scanCommandPg(rows)
		if err != nil {
			return nil, fmt.Errorf("state: ListCommands scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state: ListCommands iterate: %w", err)
	}
	return out, nil
}

func (s *PostgreSQLStore) DeleteCommandsBefore(ctx context.Context, cutoff time.Time, statuses []CommandStatus) (int, error) {
	if len(statuses) == 0 {
		return 0, fmt.Errorf("state: DeleteCommandsBefore: statuses required")
	}
	if cutoff.IsZero() {
		return 0, fmt.Errorf("state: DeleteCommandsBefore: cutoff must be non-zero")
	}

	ph := newPlaceholderGen()
	args := make([]any, 0, len(statuses)+1)
	args = append(args, cutoff.UTC())

	var sb strings.Builder
	sb.WriteString(`DELETE FROM commands WHERE completed_at IS NOT NULL AND completed_at < `)
	sb.WriteString(ph.next())
	sb.WriteString(` AND status IN (`)
	for i, st := range statuses {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(ph.next())
		args = append(args, string(st))
	}
	sb.WriteString(")")

	res, err := s.db.ExecContext(ctx, sb.String(), args...)
	if err != nil {
		return 0, fmt.Errorf("state: DeleteCommandsBefore: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("state: DeleteCommandsBefore RowsAffected: %w", err)
	}
	return int(n), nil
}

func (s *PostgreSQLStore) UpdateCommandResult(ctx context.Context, id string, result CommandResult) error {
	res, err := s.db.ExecContext(ctx, `UPDATE commands SET
    status = $1, exit_code = $2, stdout = $3, stderr = $4, completed_at = $5
WHERE id = $6`,
		string(result.Status),
		nullableExitCode(result.ExitCode, result.Status),
		nullableString(result.Stdout), nullableString(result.Stderr),
		nullableTime(result.CompletedAt),
		id,
	)
	if err != nil {
		return fmt.Errorf("state: UpdateCommandResult: %w", err)
	}
	return affectsRow(res)
}

// ---- helpers --------------------------------------------------------------

func scanCommandPg(r rowLike) (*CommandRecord, error) {
	var (
		c                 CommandRecord
		argsJSON, envJSON []byte
		statusRaw         string
		exitCode          sql.NullInt64
		startedAt, doneAt sql.NullTime
	)
	if err := r.Scan(
		&c.ID, &c.AgentID, &c.Command, &argsJSON, &envJSON,
		&c.WorkingDir, &c.User, &c.Principal,
		&c.TimeoutSeconds, &statusRaw,
		&exitCode, &c.Stdout, &c.Stderr,
		&startedAt, &doneAt,
	); err != nil {
		return nil, err
	}

	c.Status = CommandStatus(statusRaw)
	if exitCode.Valid {
		c.ExitCode = int(exitCode.Int64)
	}

	if err := unmarshalJSONBytes(argsJSON, &c.Args); err != nil {
		return nil, fmt.Errorf("state: scanCommand args: %w", err)
	}
	if err := unmarshalJSONBytes(envJSON, &c.Env); err != nil {
		return nil, fmt.Errorf("state: scanCommand env: %w", err)
	}

	if startedAt.Valid {
		c.StartedAt = startedAt.Time
	}
	if doneAt.Valid {
		c.CompletedAt = doneAt.Time
	}

	return &c, nil
}
