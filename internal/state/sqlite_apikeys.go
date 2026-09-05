// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SQL fragment for the apikeys table. gosec G101 false-positive: the
// const name contains "Key" + the literal mentions key_hash; neither
// is a credential.
//
// #nosec G101 -- SQL column list, not a credential.
//
//nolint:gosec
const apiKeySelect = `SELECT
    id, name, key_hash, role,
    created_at, last_used, expires_at
FROM apikeys`

func (s *SQLiteStore) CreateAPIKey(ctx context.Context, k *APIKeyRecord) error {
	if k == nil {
		return fmt.Errorf("state: CreateAPIKey: nil record")
	}
	if k.KeyHash == "" {
		return fmt.Errorf("state: CreateAPIKey: KeyHash is required")
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO apikeys (
    id, name, key_hash, role, created_at, expires_at, last_used
) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.Name, k.KeyHash, k.Role,
		tsArgRequired(k.CreatedAt),
		tsArgNullable(k.ExpiresAt),
		tsArgNullable(k.LastUsed),
	)
	if err != nil {
		return fmt.Errorf("state: CreateAPIKey: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetAPIKey(ctx context.Context, id string) (*APIKeyRecord, error) {
	row := s.db.QueryRowContext(ctx, apiKeySelect+" WHERE id = ?", id)
	rec, err := scanAPIKey(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

func (s *SQLiteStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKeyRecord, error) {
	row := s.db.QueryRowContext(ctx, apiKeySelect+" WHERE key_hash = ?", keyHash)
	rec, err := scanAPIKey(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

func (s *SQLiteStore) ListAPIKeys(ctx context.Context, filter APIKeyFilter) ([]*APIKeyRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedAPIKeySortColumns); err != nil {
		return nil, err
	}

	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(apiKeySelect)

	if filter.Role != "" {
		conds = append(conds, "role = ?")
		args = append(args, filter.Role)
	}
	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}

	sb.WriteString(orderByClause(filter.SortColumn, "created_at", filter.SortDesc))
	sb.WriteString(limitOffsetClause(filter.Limit, filter.Offset))

	rows, err := s.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("state: ListAPIKeys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*APIKeyRecord
	for rows.Next() {
		rec, err := scanAPIKey(rows)
		if err != nil {
			return nil, fmt.Errorf("state: ListAPIKeys scan: %w", err)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("state: ListAPIKeys iterate: %w", err)
	}
	return out, nil
}

func (s *SQLiteStore) UpdateAPIKeyLastUsed(ctx context.Context, id string, t time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE apikeys SET last_used = ? WHERE id = ?`,
		tsArgNullable(t), id)
	if err != nil {
		return fmt.Errorf("state: UpdateAPIKeyLastUsed: %w", err)
	}
	return affectsRow(res)
}

func (s *SQLiteStore) DeleteAPIKey(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM apikeys WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("state: DeleteAPIKey: %w", err)
	}
	return affectsRow(res)
}

// scanAPIKey populates an APIKeyRecord from a *sql.Row or *sql.Rows.
func scanAPIKey(r rowLike) (*APIKeyRecord, error) {
	var (
		k                      APIKeyRecord
		createdAt              string
		expiresAt, lastUsedRaw sql.NullString
	)
	if err := r.Scan(
		&k.ID, &k.Name, &k.KeyHash, &k.Role,
		&createdAt, &lastUsedRaw, &expiresAt,
	); err != nil {
		return nil, err
	}

	var err error
	if k.CreatedAt, err = tsParseRequired(createdAt); err != nil {
		return nil, fmt.Errorf("state: scanAPIKey created_at: %w", err)
	}
	if k.ExpiresAt, err = tsParseNullable(expiresAt); err != nil {
		return nil, fmt.Errorf("state: scanAPIKey expires_at: %w", err)
	}
	if k.LastUsed, err = tsParseNullable(lastUsedRaw); err != nil {
		return nil, fmt.Errorf("state: scanAPIKey last_used: %w", err)
	}

	return &k, nil
}
