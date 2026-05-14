package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// gosec G101 false-positive: column list, not a credential.
//
//nolint:gosec
const postgresJoinTokenSelect = `SELECT
    id, hash, salt, prefix, agent_id,
    ttl_ns, created_at, expires_at, used_at,
    max_uses, used_count, metadata
FROM join_tokens`

// CreateJoinToken — Postgres counterpart of the SQLite impl.
// Hash + Salt → BYTEA (lib/pq accepts []byte natively); metadata
// → JSONB via marshalJSONBytes.
func (s *PostgreSQLStore) CreateJoinToken(ctx context.Context, r *JoinTokenRecord) error {
	if err := validateJoinTokenForCreate(r); err != nil {
		return err
	}
	meta, err := marshalJSONBytes(r.Metadata)
	if err != nil {
		return fmt.Errorf("state: CreateJoinToken metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO join_tokens (
    id, hash, salt, prefix, agent_id,
    ttl_ns, created_at, expires_at, used_at,
    max_uses, used_count, metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		r.ID, r.Hash, r.Salt, r.Prefix, r.AgentID,
		int64(r.TTL),
		r.CreatedAt.UTC(), r.ExpiresAt.UTC(),
		nullableTime(r.UsedAt),
		r.MaxUses, r.UsedCount,
		meta,
	)
	if err != nil {
		if isDuplicateKeyError(err) {
			return fmt.Errorf("state: CreateJoinToken: %w", ErrDuplicate)
		}
		return fmt.Errorf("state: CreateJoinToken: %w", err)
	}
	return nil
}

func (s *PostgreSQLStore) GetJoinToken(ctx context.Context, id string) (*JoinTokenRecord, error) {
	row := s.db.QueryRowContext(ctx, postgresJoinTokenSelect+" WHERE id = $1", id)
	rec, err := scanJoinTokenPostgres(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

func (s *PostgreSQLStore) LookupJoinTokenByPrefix(ctx context.Context, prefix string) (*JoinTokenRecord, error) {
	row := s.db.QueryRowContext(ctx, postgresJoinTokenSelect+" WHERE prefix = $1", prefix)
	rec, err := scanJoinTokenPostgres(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

func (s *PostgreSQLStore) ListJoinTokens(ctx context.Context, filter JoinTokenFilter) ([]*JoinTokenRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedJoinTokenSortColumns); err != nil {
		return nil, err
	}

	var (
		sb    strings.Builder
		args  []any
		conds []string
		argN  int
	)
	sb.WriteString(postgresJoinTokenSelect)

	next := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	if filter.AgentID != "" {
		conds = append(conds, "agent_id = "+next())
		args = append(args, filter.AgentID)
	}
	if filter.Unused {
		conds = append(conds, "used_count < max_uses")
	}
	if !filter.UnexpiredAt.IsZero() {
		conds = append(conds, "expires_at > "+next())
		args = append(args, filter.UnexpiredAt.UTC())
	}
	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}
	sb.WriteString(orderByClause(filter.SortColumn, "created_at", filter.SortDesc))
	sb.WriteString(limitOffsetClause(filter.Limit, filter.Offset))

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("state: ListJoinTokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*JoinTokenRecord
	for rows.Next() {
		rec, err := scanJoinTokenPostgres(rows)
		if err != nil {
			return nil, fmt.Errorf("state: ListJoinTokens scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state: ListJoinTokens iterate: %w", err)
	}
	return out, nil
}

func (s *PostgreSQLStore) MarkJoinTokenUsed(ctx context.Context, id string, now time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE join_tokens SET used_count = used_count + 1, used_at = $1
         WHERE id = $2 AND used_count < max_uses`,
		nullableTime(now), id)
	if err != nil {
		return fmt.Errorf("state: MarkJoinTokenUsed: %w", err)
	}
	return affectsRow(res)
}

func (s *PostgreSQLStore) DeleteJoinToken(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM join_tokens WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("state: DeleteJoinToken: %w", err)
	}
	return affectsRow(res)
}

func (s *PostgreSQLStore) DeleteExpiredJoinTokens(ctx context.Context, before time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM join_tokens WHERE expires_at <= $1`,
		before.UTC())
	if err != nil {
		return 0, fmt.Errorf("state: DeleteExpiredJoinTokens: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("state: DeleteExpiredJoinTokens rows: %w", err)
	}
	return int(n), nil
}

// scanJoinTokenPostgres reads a join-token row from lib/pq. JSONB
// is delivered as []byte; UsedAt comes through sql.NullTime.
func scanJoinTokenPostgres(r rowLike) (*JoinTokenRecord, error) {
	var (
		rec       JoinTokenRecord
		ttlNS     int64
		usedAt    sql.NullTime
		metadata  []byte
	)
	if err := r.Scan(
		&rec.ID, &rec.Hash, &rec.Salt, &rec.Prefix, &rec.AgentID,
		&ttlNS, &rec.CreatedAt, &rec.ExpiresAt, &usedAt,
		&rec.MaxUses, &rec.UsedCount,
		&metadata,
	); err != nil {
		return nil, err
	}
	rec.TTL = time.Duration(ttlNS)
	if usedAt.Valid {
		rec.UsedAt = usedAt.Time
	}
	if len(metadata) > 0 && string(metadata) != "{}" {
		if err := unmarshalJSONBytes(metadata, &rec.Metadata); err != nil {
			return nil, fmt.Errorf("state: scan join_token metadata: %w", err)
		}
	}
	return &rec, nil
}
