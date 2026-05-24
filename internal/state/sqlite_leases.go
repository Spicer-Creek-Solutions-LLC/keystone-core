// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const leaseSelect = `SELECT
    id, backend, secret_path,
    issued_at, expires_at, duration_ns,
    renewable, max_ttl_ns,
    state, strategy, issued_for,
    last_renewed_at, renew_count, revoked_at,
    metadata
FROM secret_leases`

// CreateLease persists a new lease record. Wraps state.ErrDuplicate
// on a PRIMARY KEY collision against id.
func (s *SQLiteStore) CreateLease(ctx context.Context, r *LeaseStoreRecord) error {
	if err := validateLeaseForCreate(r); err != nil {
		return err
	}
	meta, err := encodeMetadata(r.Metadata)
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
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Backend, r.SecretPath,
		tsArgRequired(r.IssuedAt),
		tsArgRequired(r.ExpiresAt),
		int64(r.Duration),
		boolArgSQLite(r.Renewable),
		int64(r.MaxTTL),
		r.State, r.Strategy, r.IssuedFor,
		tsArgNullable(r.LastRenewedAt),
		r.RenewCount,
		tsArgNullable(r.RevokedAt),
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

// GetLease returns the record by id, or state.ErrNotFound.
func (s *SQLiteStore) GetLease(ctx context.Context, id string) (*LeaseStoreRecord, error) {
	row := s.db.QueryRowContext(ctx, leaseSelect+" WHERE id = ?", id)
	rec, err := scanLeaseSQLite(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

// UpdateLease performs a full-row replace. Returns state.ErrNotFound
// when no row matches the supplied ID.
func (s *SQLiteStore) UpdateLease(ctx context.Context, r *LeaseStoreRecord) error {
	if r == nil || r.ID == "" {
		return fmt.Errorf("state: UpdateLease: ID is required")
	}
	meta, err := encodeMetadata(r.Metadata)
	if err != nil {
		return fmt.Errorf("state: UpdateLease metadata: %w", err)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE secret_leases SET
    backend = ?, secret_path = ?,
    issued_at = ?, expires_at = ?, duration_ns = ?,
    renewable = ?, max_ttl_ns = ?,
    state = ?, strategy = ?, issued_for = ?,
    last_renewed_at = ?, renew_count = ?, revoked_at = ?,
    metadata = ?
WHERE id = ?`,
		r.Backend, r.SecretPath,
		tsArgRequired(r.IssuedAt),
		tsArgRequired(r.ExpiresAt),
		int64(r.Duration),
		boolArgSQLite(r.Renewable),
		int64(r.MaxTTL),
		r.State, r.Strategy, r.IssuedFor,
		tsArgNullable(r.LastRenewedAt),
		r.RenewCount,
		tsArgNullable(r.RevokedAt),
		meta,
		r.ID,
	)
	if err != nil {
		return fmt.Errorf("state: UpdateLease: %w", err)
	}
	return affectsRow(res)
}

// ListLeases returns the records matching filter.
func (s *SQLiteStore) ListLeases(ctx context.Context, filter LeaseFilter) ([]*LeaseStoreRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedLeaseSortColumns); err != nil {
		return nil, err
	}

	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(leaseSelect)

	if filter.Backend != "" {
		conds = append(conds, "backend = ?")
		args = append(args, filter.Backend)
	}
	if filter.State != "" {
		conds = append(conds, "state = ?")
		args = append(args, filter.State)
	}
	if filter.PathPrefix != "" {
		conds = append(conds, "secret_path LIKE ?")
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
		rec, err := scanLeaseSQLite(rows)
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

// DeleteLease removes a lease by id. Returns state.ErrNotFound when absent.
func (s *SQLiteStore) DeleteLease(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM secret_leases WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("state: DeleteLease: %w", err)
	}
	return affectsRow(res)
}

// DeleteExpiredLeases removes every lease that's both past expiry AND
// no longer active. An active lease that's race-conditioned against
// expiry is still mid-renewal; the next scheduler tick moves it to
// expired and the following cleanup pass collects it.
func (s *SQLiteStore) DeleteExpiredLeases(ctx context.Context, before time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM secret_leases WHERE expires_at <= ? AND state != ?`,
		tsArgRequired(before), "active")
	if err != nil {
		return 0, fmt.Errorf("state: DeleteExpiredLeases: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("state: DeleteExpiredLeases rows: %w", err)
	}
	return int(n), nil
}

func scanLeaseSQLite(r rowLike) (*LeaseStoreRecord, error) {
	var (
		rec           LeaseStoreRecord
		issuedAt      string
		expiresAt     string
		durationNS    int64
		renewable     int
		maxTTLNS      int64
		lastRenewedAt sql.NullString
		revokedAt     sql.NullString
		metadata      string
	)
	if err := r.Scan(
		&rec.ID, &rec.Backend, &rec.SecretPath,
		&issuedAt, &expiresAt, &durationNS,
		&renewable, &maxTTLNS,
		&rec.State, &rec.Strategy, &rec.IssuedFor,
		&lastRenewedAt, &rec.RenewCount, &revokedAt,
		&metadata,
	); err != nil {
		return nil, err
	}
	var err error
	if rec.IssuedAt, err = tsParseRequired(issuedAt); err != nil {
		return nil, fmt.Errorf("state: scan lease issued_at: %w", err)
	}
	if rec.ExpiresAt, err = tsParseRequired(expiresAt); err != nil {
		return nil, fmt.Errorf("state: scan lease expires_at: %w", err)
	}
	rec.Duration = time.Duration(durationNS)
	rec.Renewable = renewable != 0
	rec.MaxTTL = time.Duration(maxTTLNS)
	if rec.LastRenewedAt, err = tsParseNullable(lastRenewedAt); err != nil {
		return nil, fmt.Errorf("state: scan lease last_renewed_at: %w", err)
	}
	if rec.RevokedAt, err = tsParseNullable(revokedAt); err != nil {
		return nil, fmt.Errorf("state: scan lease revoked_at: %w", err)
	}
	if rec.Metadata, err = decodeMetadata(metadata); err != nil {
		return nil, fmt.Errorf("state: scan lease metadata: %w", err)
	}
	return &rec, nil
}

// validateLeaseForCreate is the shared pre-INSERT shape check. Both
// backends apply it before hitting the database.
func validateLeaseForCreate(r *LeaseStoreRecord) error {
	if r == nil {
		return fmt.Errorf("state: CreateLease: nil record")
	}
	if r.ID == "" {
		return fmt.Errorf("state: CreateLease: ID is required")
	}
	if r.Backend == "" {
		return fmt.Errorf("state: CreateLease: Backend is required")
	}
	if r.SecretPath == "" {
		return fmt.Errorf("state: CreateLease: SecretPath is required")
	}
	if r.IssuedAt.IsZero() {
		return fmt.Errorf("state: CreateLease: IssuedAt is required")
	}
	if r.ExpiresAt.IsZero() {
		return fmt.Errorf("state: CreateLease: ExpiresAt is required")
	}
	if r.State == "" {
		return fmt.Errorf("state: CreateLease: State is required")
	}
	if r.Strategy == "" {
		return fmt.Errorf("state: CreateLease: Strategy is required")
	}
	return nil
}

// boolArgSQLite encodes a Go bool as the SQLite INTEGER 0/1 the
// schema uses. Postgres has native BOOLEAN; this helper is unused
// there.
func boolArgSQLite(b bool) int {
	if b {
		return 1
	}
	return 0
}
