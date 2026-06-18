// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const agentSelectPg = `SELECT
    id, hostname, os, architecture, ip_addresses,
    COALESCE(platform_version, ''), COALESCE(agent_version, ''),
    labels, status, registered_at, last_heartbeat_at, metrics,
    COALESCE(cert_chain_pem, ''), COALESCE(cert_fingerprint, ''),
    cert_not_after, COALESCE(spiffe_id, '')
FROM agents`

func (s *PostgreSQLStore) CreateAgent(ctx context.Context, a *AgentRecord) error {
	if a == nil {
		return fmt.Errorf("state: CreateAgent: nil record")
	}

	ipAddrs, err := marshalJSONBytes(a.IPAddresses)
	if err != nil {
		return fmt.Errorf("state: CreateAgent ip_addresses: %w", err)
	}
	labels, err := marshalJSONBytes(a.Labels)
	if err != nil {
		return fmt.Errorf("state: CreateAgent labels: %w", err)
	}
	metrics, err := agentMetricsArgPg(a.Metrics)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO agents (
    id, hostname, os, architecture, ip_addresses,
    platform_version, agent_version, labels, status,
    registered_at, last_heartbeat_at, metrics,
    cert_chain_pem, cert_fingerprint, cert_not_after, spiffe_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		a.ID, a.Hostname, a.OS, a.Architecture, ipAddrs,
		nullableString(a.PlatformVersion), nullableString(a.AgentVersion),
		labels, string(a.Status),
		a.RegisteredAt.UTC(), nullableTime(a.LastHeartbeatAt), metrics,
		nullableString(a.CertChainPEM), nullableString(a.CertFingerprint),
		nullableTime(a.CertNotAfter), nullableString(a.SPIFFEID),
	)
	if err != nil {
		return fmt.Errorf("state: CreateAgent: %w", err)
	}
	return nil
}

func (s *PostgreSQLStore) GetAgent(ctx context.Context, id string) (*AgentRecord, error) {
	row := s.db.QueryRowContext(ctx, agentSelectPg+" WHERE id = $1", id)
	a, err := scanAgentPg(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return a, nil
}

func (s *PostgreSQLStore) ListAgents(ctx context.Context, filter AgentFilter) ([]*AgentRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedAgentSortColumns); err != nil {
		return nil, err
	}

	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(agentSelectPg)

	ph := newPlaceholderGen()
	if filter.Status != "" {
		conds = append(conds, "status = "+ph.next())
		args = append(args, string(filter.Status))
	}
	if filter.LabelKey != "" {
		// JSONB ?-operator (key existence) for key-only; equality
		// for key+value via the @> containment operator.
		if filter.LabelValue != "" {
			conds = append(conds, "labels @> "+ph.next()+"::jsonb")
			args = append(args, fmt.Sprintf(`{"%s":"%s"}`, filter.LabelKey, filter.LabelValue))
		} else {
			conds = append(conds, "labels ? "+ph.next())
			args = append(args, filter.LabelKey)
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
		a, err := scanAgentPg(rows)
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

func (s *PostgreSQLStore) UpdateAgent(ctx context.Context, a *AgentRecord) error {
	if a == nil {
		return fmt.Errorf("state: UpdateAgent: nil record")
	}

	ipAddrs, err := marshalJSONBytes(a.IPAddresses)
	if err != nil {
		return fmt.Errorf("state: UpdateAgent ip_addresses: %w", err)
	}
	labels, err := marshalJSONBytes(a.Labels)
	if err != nil {
		return fmt.Errorf("state: UpdateAgent labels: %w", err)
	}
	metrics, err := agentMetricsArgPg(a.Metrics)
	if err != nil {
		return err
	}

	res, err := s.db.ExecContext(ctx, `UPDATE agents SET
    hostname = $1, os = $2, architecture = $3, ip_addresses = $4,
    platform_version = $5, agent_version = $6, labels = $7, status = $8,
    registered_at = $9, last_heartbeat_at = $10, metrics = $11,
    cert_chain_pem = $12, cert_fingerprint = $13, cert_not_after = $14, spiffe_id = $15
WHERE id = $16`,
		a.Hostname, a.OS, a.Architecture, ipAddrs,
		nullableString(a.PlatformVersion), nullableString(a.AgentVersion),
		labels, string(a.Status),
		a.RegisteredAt.UTC(), nullableTime(a.LastHeartbeatAt), metrics,
		nullableString(a.CertChainPEM), nullableString(a.CertFingerprint),
		nullableTime(a.CertNotAfter), nullableString(a.SPIFFEID),
		a.ID,
	)
	if err != nil {
		return fmt.Errorf("state: UpdateAgent: %w", err)
	}
	return affectsRow(res)
}

func (s *PostgreSQLStore) UpdateAgentHeartbeat(ctx context.Context, id string, t time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE agents SET last_heartbeat_at = $1 WHERE id = $2`,
		nullableTime(t), id)
	if err != nil {
		return fmt.Errorf("state: UpdateAgentHeartbeat: %w", err)
	}
	return affectsRow(res)
}

func (s *PostgreSQLStore) UpdateAgentStatus(ctx context.Context, id string, status AgentStatus) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE agents SET status = $1 WHERE id = $2`,
		string(status), id)
	if err != nil {
		return fmt.Errorf("state: UpdateAgentStatus: %w", err)
	}
	return affectsRow(res)
}

func (s *PostgreSQLStore) DeleteAgent(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("state: DeleteAgent: %w", err)
	}
	return affectsRow(res)
}

// ---- helpers --------------------------------------------------------------

// agentMetricsArgPg returns NULL for nil maps; otherwise marshaled JSON
// suitable for a JSONB column.
func agentMetricsArgPg(m map[string]any) ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return marshalJSONBytes(m)
}

// scanAgentPg populates an AgentRecord from a *sql.Row or *sql.Rows
// using lib/pq's native scan types (JSONB->[]byte, TIMESTAMPTZ->time.Time
// or sql.NullTime).
func scanAgentPg(r rowLike) (*AgentRecord, error) {
	var (
		a              AgentRecord
		ipAddrs, lbls  []byte
		statusRaw      string
		registeredAt   time.Time
		lastHB, certNA sql.NullTime
		metrics        []byte
	)
	if err := r.Scan(
		&a.ID, &a.Hostname, &a.OS, &a.Architecture, &ipAddrs,
		&a.PlatformVersion, &a.AgentVersion, &lbls, &statusRaw,
		&registeredAt, &lastHB, &metrics,
		&a.CertChainPEM, &a.CertFingerprint, &certNA, &a.SPIFFEID,
	); err != nil {
		return nil, err
	}

	a.Status = AgentStatus(statusRaw)

	if err := unmarshalJSONBytes(ipAddrs, &a.IPAddresses); err != nil {
		return nil, fmt.Errorf("state: scanAgent ip_addresses: %w", err)
	}
	if err := unmarshalJSONBytes(lbls, &a.Labels); err != nil {
		return nil, fmt.Errorf("state: scanAgent labels: %w", err)
	}

	a.RegisteredAt = registeredAt
	if lastHB.Valid {
		a.LastHeartbeatAt = lastHB.Time
	}
	if certNA.Valid {
		a.CertNotAfter = certNA.Time
	}

	if err := unmarshalJSONBytes(metrics, &a.Metrics); err != nil {
		return nil, fmt.Errorf("state: scanAgent metrics: %w", err)
	}

	return &a, nil
}

// placeholderGen produces $1, $2, $3, ... in order. Used by ListX
// builders so each filter condition gets the next placeholder.
type placeholderGen struct{ n int }

func newPlaceholderGen() *placeholderGen { return &placeholderGen{} }

func (p *placeholderGen) next() string {
	p.n++
	return fmt.Sprintf("$%d", p.n)
}
