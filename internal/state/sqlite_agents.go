// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const agentSelect = `SELECT
    id, hostname, os, architecture, ip_addresses,
    COALESCE(platform_version, ''), COALESCE(agent_version, ''),
    labels, status, registered_at, last_heartbeat_at, metrics
FROM agents`

func (s *SQLiteStore) CreateAgent(ctx context.Context, a *AgentRecord) error {
	if a == nil {
		return fmt.Errorf("state: CreateAgent: nil record")
	}

	ipAddrs, err := marshalJSONColumn(a.IPAddresses)
	if err != nil {
		return fmt.Errorf("state: CreateAgent ip_addresses: %w", err)
	}
	labels, err := marshalJSONColumn(a.Labels)
	if err != nil {
		return fmt.Errorf("state: CreateAgent labels: %w", err)
	}
	metrics, err := agentMetricsArg(a.Metrics)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO agents (
    id, hostname, os, architecture, ip_addresses,
    platform_version, agent_version, labels, status,
    registered_at, last_heartbeat_at, metrics
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Hostname, a.OS, a.Architecture, ipAddrs,
		nullableString(a.PlatformVersion), nullableString(a.AgentVersion),
		labels, string(a.Status),
		tsArgRequired(a.RegisteredAt), tsArgNullable(a.LastHeartbeatAt), metrics,
	)
	if err != nil {
		return fmt.Errorf("state: CreateAgent: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetAgent(ctx context.Context, id string) (*AgentRecord, error) {
	row := s.db.QueryRowContext(ctx, agentSelect+" WHERE id = ?", id)
	a, err := scanAgent(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return a, nil
}

func (s *SQLiteStore) ListAgents(ctx context.Context, filter AgentFilter) ([]*AgentRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedAgentSortColumns); err != nil {
		return nil, err
	}

	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(agentSelect)

	if filter.Status != "" {
		conds = append(conds, "status = ?")
		args = append(args, string(filter.Status))
	}
	if filter.LabelKey != "" {
		// JSON labels matched via LIKE on the canonical encoding. Adequate
		// for v1.0 — encoding is deterministic (no spaces). Keys/values
		// containing JSON-meta characters aren't supported; revisit when
		// epic 06 surfaces the need.
		if filter.LabelValue != "" {
			conds = append(conds, "labels LIKE ?")
			args = append(args, fmt.Sprintf(`%%"%s":"%s"%%`, filter.LabelKey, filter.LabelValue))
		} else {
			conds = append(conds, "labels LIKE ?")
			args = append(args, fmt.Sprintf(`%%"%s":%%`, filter.LabelKey))
		}
	}
	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}

	sb.WriteString(orderByClause(filter.SortColumn, "registered_at", filter.SortDesc))
	sb.WriteString(limitOffsetClause(filter.Limit, filter.Offset))

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("state: ListAgents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*AgentRecord
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, fmt.Errorf("state: ListAgents scan: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state: ListAgents iterate: %w", err)
	}
	return out, nil
}

func (s *SQLiteStore) UpdateAgent(ctx context.Context, a *AgentRecord) error {
	if a == nil {
		return fmt.Errorf("state: UpdateAgent: nil record")
	}

	ipAddrs, err := marshalJSONColumn(a.IPAddresses)
	if err != nil {
		return fmt.Errorf("state: UpdateAgent ip_addresses: %w", err)
	}
	labels, err := marshalJSONColumn(a.Labels)
	if err != nil {
		return fmt.Errorf("state: UpdateAgent labels: %w", err)
	}
	metrics, err := agentMetricsArg(a.Metrics)
	if err != nil {
		return err
	}

	res, err := s.db.ExecContext(ctx, `UPDATE agents SET
    hostname = ?, os = ?, architecture = ?, ip_addresses = ?,
    platform_version = ?, agent_version = ?, labels = ?, status = ?,
    registered_at = ?, last_heartbeat_at = ?, metrics = ?
WHERE id = ?`,
		a.Hostname, a.OS, a.Architecture, ipAddrs,
		nullableString(a.PlatformVersion), nullableString(a.AgentVersion),
		labels, string(a.Status),
		tsArgRequired(a.RegisteredAt), tsArgNullable(a.LastHeartbeatAt), metrics,
		a.ID,
	)
	if err != nil {
		return fmt.Errorf("state: UpdateAgent: %w", err)
	}
	return affectsRow(res)
}

func (s *SQLiteStore) UpdateAgentHeartbeat(ctx context.Context, id string, t time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE agents SET last_heartbeat_at = ? WHERE id = ?`,
		tsArgNullable(t), id)
	if err != nil {
		return fmt.Errorf("state: UpdateAgentHeartbeat: %w", err)
	}
	return affectsRow(res)
}

func (s *SQLiteStore) UpdateAgentStatus(ctx context.Context, id string, status AgentStatus) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE agents SET status = ? WHERE id = ?`,
		string(status), id)
	if err != nil {
		return fmt.Errorf("state: UpdateAgentStatus: %w", err)
	}
	return affectsRow(res)
}

func (s *SQLiteStore) DeleteAgent(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("state: DeleteAgent: %w", err)
	}
	return affectsRow(res)
}

// ---- helpers (agent-specific) ---------------------------------------------

// agentMetricsArg builds the sql.NullString for the nullable `metrics`
// JSON column. nil map -> SQL NULL; non-nil -> marshaled JSON.
func agentMetricsArg(m map[string]any) (sql.NullString, error) {
	if m == nil {
		return sql.NullString{}, nil
	}
	s, err := marshalJSONColumn(m)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("state: marshal metrics: %w", err)
	}
	return sql.NullString{String: s, Valid: true}, nil
}

// nullableString writes "" as SQL NULL for nullable string columns.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// scanAgent populates an AgentRecord from a *sql.Row or *sql.Rows.
func scanAgent(r rowLike) (*AgentRecord, error) {
	var (
		a              AgentRecord
		ipAddrs, lbls  string
		statusRaw      string
		registeredAt   string
		lastHB, metrics sql.NullString
	)
	if err := r.Scan(
		&a.ID, &a.Hostname, &a.OS, &a.Architecture, &ipAddrs,
		&a.PlatformVersion, &a.AgentVersion, &lbls, &statusRaw,
		&registeredAt, &lastHB, &metrics,
	); err != nil {
		return nil, err
	}

	a.Status = AgentStatus(statusRaw)

	if err := unmarshalJSONColumn(ipAddrs, &a.IPAddresses); err != nil {
		return nil, fmt.Errorf("state: scanAgent ip_addresses: %w", err)
	}
	if err := unmarshalJSONColumn(lbls, &a.Labels); err != nil {
		return nil, fmt.Errorf("state: scanAgent labels: %w", err)
	}

	var err error
	if a.RegisteredAt, err = tsParseRequired(registeredAt); err != nil {
		return nil, fmt.Errorf("state: scanAgent registered_at: %w", err)
	}
	if a.LastHeartbeatAt, err = tsParseNullable(lastHB); err != nil {
		return nil, fmt.Errorf("state: scanAgent last_heartbeat_at: %w", err)
	}

	if metrics.Valid {
		if err := unmarshalJSONColumn(metrics.String, &a.Metrics); err != nil {
			return nil, fmt.Errorf("state: scanAgent metrics: %w", err)
		}
	}

	return &a, nil
}
