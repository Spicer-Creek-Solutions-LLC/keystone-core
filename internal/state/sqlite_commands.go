// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const commandSelect = `SELECT
    id, agent_id, command, args, env,
    COALESCE(working_dir, ''), COALESCE("user", ''), COALESCE(principal, ''),
    timeout_seconds, status,
    exit_code, COALESCE(stdout, ''), COALESCE(stderr, ''),
    started_at, completed_at
FROM commands`

func (s *SQLiteStore) CreateCommand(ctx context.Context, c *CommandRecord) error {
	if c == nil {
		return fmt.Errorf("state: CreateCommand: nil record")
	}

	args, err := marshalJSONColumn(c.Args)
	if err != nil {
		return fmt.Errorf("state: CreateCommand args: %w", err)
	}
	env, err := marshalJSONColumn(c.Env)
	if err != nil {
		return fmt.Errorf("state: CreateCommand env: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO commands (
    id, agent_id, command, args, env,
    working_dir, "user", principal, timeout_seconds, status,
    exit_code, stdout, stderr, started_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.AgentID, c.Command, args, env,
		nullableString(c.WorkingDir), nullableString(c.User), nullableString(c.Principal),
		c.TimeoutSeconds, string(c.Status),
		nullableExitCode(c.ExitCode, c.Status),
		nullableString(c.Stdout), nullableString(c.Stderr),
		tsArgNullable(c.StartedAt), tsArgNullable(c.CompletedAt),
	)
	if err != nil {
		return fmt.Errorf("state: CreateCommand: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetCommand(ctx context.Context, id string) (*CommandRecord, error) {
	row := s.db.QueryRowContext(ctx, commandSelect+" WHERE id = ?", id)
	c, err := scanCommand(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return c, nil
}

func (s *SQLiteStore) ListCommands(ctx context.Context, filter CommandFilter) ([]*CommandRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedCommandSortColumns); err != nil {
		return nil, err
	}

	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(commandSelect)

	if filter.AgentID != "" {
		conds = append(conds, "agent_id = ?")
		args = append(args, filter.AgentID)
	}
	if filter.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, string(filter.Status))
	}
	if !filter.Since.IsZero() {
		conds = append(conds, "started_at >= ?")
		args = append(args, tsArgRequired(filter.Since))
	}
	if !filter.Until.IsZero() {
		conds = append(conds, "started_at <= ?")
		args = append(args, tsArgRequired(filter.Until))
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
		c, err := scanCommand(rows)
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

func (s *SQLiteStore) DeleteCommandsBefore(ctx context.Context, cutoff time.Time, statuses []CommandStatus) (int, error) {
	if len(statuses) == 0 {
		return 0, fmt.Errorf("state: DeleteCommandsBefore: statuses required")
	}
	if cutoff.IsZero() {
		return 0, fmt.Errorf("state: DeleteCommandsBefore: cutoff must be non-zero")
	}

	args := make([]any, 0, len(statuses)+1)
	args = append(args, tsArgRequired(cutoff))

	var sb strings.Builder
	sb.WriteString(`DELETE FROM commands WHERE completed_at IS NOT NULL AND completed_at < ? AND status IN (`)
	for i, st := range statuses {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("?")
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

func (s *SQLiteStore) UpdateCommandResult(ctx context.Context, id string, result CommandResult) error {
	res, err := s.db.ExecContext(ctx, `UPDATE commands SET
    status = ?, exit_code = ?, stdout = ?, stderr = ?, completed_at = ?
WHERE id = ?`,
		string(result.Status),
		nullableExitCode(result.ExitCode, result.Status),
		nullableString(result.Stdout), nullableString(result.Stderr),
		tsArgNullable(result.CompletedAt),
		id,
	)
	if err != nil {
		return fmt.Errorf("state: UpdateCommandResult: %w", err)
	}
	return affectsRow(res)
}

// ---- helpers (command-specific) -------------------------------------------

// nullableExitCode treats exit code 0 as meaningful only when the
// command actually completed; for pending/running rows the exit code
// hasn't been set, store SQL NULL so future readers can distinguish
// "unknown" from "exited 0".
func nullableExitCode(code int, status CommandStatus) sql.NullInt64 {
	switch status {
	case CommandStatusCompleted, CommandStatusFailed,
		CommandStatusTimeout, CommandStatusCancelled:
		return sql.NullInt64{Int64: int64(code), Valid: true}
	default:
		return sql.NullInt64{}
	}
}

func scanCommand(r rowLike) (*CommandRecord, error) {
	var (
		c                 CommandRecord
		argsJSON, envJSON string
		statusRaw         string
		exitCode          sql.NullInt64
		startedAt, doneAt sql.NullString
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

	if err := unmarshalJSONColumn(argsJSON, &c.Args); err != nil {
		return nil, fmt.Errorf("state: scanCommand args: %w", err)
	}
	if err := unmarshalJSONColumn(envJSON, &c.Env); err != nil {
		return nil, fmt.Errorf("state: scanCommand env: %w", err)
	}

	var err error
	if c.StartedAt, err = tsParseNullable(startedAt); err != nil {
		return nil, fmt.Errorf("state: scanCommand started_at: %w", err)
	}
	if c.CompletedAt, err = tsParseNullable(doneAt); err != nil {
		return nil, fmt.Errorf("state: scanCommand completed_at: %w", err)
	}

	return &c, nil
}
