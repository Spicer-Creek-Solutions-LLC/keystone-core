package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const postgresLeaseSelect = `SELECT
    id, backend, secret_path,
    issued_at, expires_at, duration_ns,
    renewable, max_ttl_ns,
    state, strategy, issued_for,
    last_renewed_at, renew_count, revoked_at,
    metadata
FROM secret_leases`

// CreateLease is the Postgres counterpart of the SQLite impl. JSONB
// metadata via marshalJSONBytes; timestamps in UTC.
func (s *PostgreSQLStore) CreateLease(ctx context.Context, r *LeaseStoreRecord) error {
	if err := validateLeaseForCreate(r); err != nil {
		return err
	}
	meta, err := marshalJSONBytes(r.Metadata)
	if err != nil {
		return fmt.Errorf("state: CreateLease metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO secret_leases (
    id, backend, secret_path,
    issued_at, expires_at, duration_ns,
    renewable, max_ttl_ns,
    state, strategy, issued_for,
    last_renewed_at, renew_count, revoked_at,
    metadata
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		r.ID, r.Backend, r.SecretPath,
		r.IssuedAt.UTC(), r.ExpiresAt.UTC(),
		int64(r.Duration),
		r.Renewable,
		int64(r.MaxTTL),
		r.State, r.Strategy, r.IssuedFor,
		nullableTime(r.LastRenewedAt),
		r.RenewCount,
		nullableTime(r.RevokedAt),
		meta,
	)
	if err != nil {
		if isDuplicateKeyError(err) {
			return fmt.Errorf("state: CreateLease: %w", ErrDuplicate)
		}
		return fmt.Errorf("state: CreateLease: %w", err)
	}
	return nil
}

func (s *PostgreSQLStore) GetLease(ctx context.Context, id string) (*LeaseStoreRecord, error) {
	row := s.db.QueryRowContext(ctx, postgresLeaseSelect+" WHERE id = $1", id)
	rec, err := scanLeasePostgres(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

func (s *PostgreSQLStore) UpdateLease(ctx context.Context, r *LeaseStoreRecord) error {
	if r == nil || r.ID == "" {
		return fmt.Errorf("state: UpdateLease: ID is required")
	}
	meta, err := marshalJSONBytes(r.Metadata)
	if err != nil {
		return fmt.Errorf("state: UpdateLease metadata: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE secret_leases SET
    backend = $1, secret_path = $2,
    issued_at = $3, expires_at = $4, duration_ns = $5,
    renewable = $6, max_ttl_ns = $7,
    state = $8, strategy = $9, issued_for = $10,
    last_renewed_at = $11, renew_count = $12, revoked_at = $13,
    metadata = $14
WHERE id = $15`,
		r.Backend, r.SecretPath,
		r.IssuedAt.UTC(), r.ExpiresAt.UTC(),
		int64(r.Duration),
		r.Renewable,
		int64(r.MaxTTL),
		r.State, r.Strategy, r.IssuedFor,
		nullableTime(r.LastRenewedAt),
		r.RenewCount,
		nullableTime(r.RevokedAt),
		meta,
		r.ID,
	)
	if err != nil {
		return fmt.Errorf("state: UpdateLease: %w", err)
	}
	return affectsRow(res)
}

func (s *PostgreSQLStore) ListLeases(ctx context.Context, filter LeaseFilter) ([]*LeaseStoreRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedLeaseSortColumns); err != nil {
		return nil, err
	}

	var (
		sb    strings.Builder
		args  []any
		conds []string
		argN  int
	)
	sb.WriteString(postgresLeaseSelect)

	next := func() string {
		argN++
		return fmt.Sprintf("$%d", argN)
	}

	if filter.Backend != "" {
		conds = append(conds, "backend = "+next())
		args = append(args, filter.Backend)
	}
	if filter.State != "" {
		conds = append(conds, "state = "+next())
		args = append(args, filter.State)
	}
	if filter.PathPrefix != "" {
		conds = append(conds, "secret_path LIKE "+next())
		args = append(args, filter.PathPrefix+"%")
	}
	if !filter.IncludeRevoked {
		conds = append(conds, "revoked_at IS NULL")
	}
	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}
	sb.WriteString(orderByClause(filter.SortColumn, "expires_at", filter.SortDesc))
	sb.WriteString(limitOffsetClause(filter.Limit, filter.Offset))

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("state: ListLeases: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*LeaseStoreRecord
	for rows.Next() {
		rec, err := scanLeasePostgres(rows)
		if err != nil {
			return nil, fmt.Errorf("state: ListLeases scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state: ListLeases iterate: %w", err)
	}
	return out, nil
}

func (s *PostgreSQLStore) DeleteLease(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM secret_leases WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("state: DeleteLease: %w", err)
	}
	return affectsRow(res)
}

func (s *PostgreSQLStore) DeleteExpiredLeases(ctx context.Context, before time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM secret_leases WHERE expires_at <= $1 AND state != $2`,
		before.UTC(), "active")
	if err != nil {
		return 0, fmt.Errorf("state: DeleteExpiredLeases: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("state: DeleteExpiredLeases rows: %w", err)
	}
	return int(n), nil
}

func scanLeasePostgres(r rowLike) (*LeaseStoreRecord, error) {
	var (
		rec           LeaseStoreRecord
		durationNS    int64
		maxTTLNS      int64
		lastRenewedAt sql.NullTime
		revokedAt     sql.NullTime
		metadata      []byte
	)
	if err := r.Scan(
		&rec.ID, &rec.Backend, &rec.SecretPath,
		&rec.IssuedAt, &rec.ExpiresAt, &durationNS,
		&rec.Renewable, &maxTTLNS,
		&rec.State, &rec.Strategy, &rec.IssuedFor,
		&lastRenewedAt, &rec.RenewCount, &revokedAt,
		&metadata,
	); err != nil {
		return nil, err
	}
	rec.Duration = time.Duration(durationNS)
	rec.MaxTTL = time.Duration(maxTTLNS)
	if lastRenewedAt.Valid {
		rec.LastRenewedAt = lastRenewedAt.Time
	}
	if revokedAt.Valid {
		rec.RevokedAt = revokedAt.Time
	}
	if len(metadata) > 0 && string(metadata) != "{}" {
		if err := unmarshalJSONBytes(metadata, &rec.Metadata); err != nil {
			return nil, fmt.Errorf("state: scan lease metadata: %w", err)
		}
	}
	return &rec, nil
}
