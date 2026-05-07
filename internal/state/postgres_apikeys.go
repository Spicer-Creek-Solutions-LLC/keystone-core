package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// gosec G101 false-positive: SQL fragment, not a credential.
//
//nolint:gosec
const apiKeySelectPg = `SELECT
    id, name, key_hash, role,
    created_at, last_used, expires_at
FROM apikeys`

func (s *PostgreSQLStore) CreateAPIKey(ctx context.Context, k *APIKeyRecord) error {
	if k == nil {
		return fmt.Errorf("state: CreateAPIKey: nil record")
	}
	if k.KeyHash == "" {
		return fmt.Errorf("state: CreateAPIKey: KeyHash is required")
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO apikeys (
    id, name, key_hash, role, created_at, expires_at, last_used
) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		k.ID, k.Name, k.KeyHash, k.Role,
		k.CreatedAt.UTC(),
		nullableTime(k.ExpiresAt),
		nullableTime(k.LastUsed),
	)
	if err != nil {
		return fmt.Errorf("state: CreateAPIKey: %w", err)
	}
	return nil
}

func (s *PostgreSQLStore) GetAPIKey(ctx context.Context, id string) (*APIKeyRecord, error) {
	row := s.db.QueryRowContext(ctx, apiKeySelectPg+" WHERE id = $1", id)
	rec, err := scanAPIKeyPg(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

func (s *PostgreSQLStore) GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKeyRecord, error) {
	row := s.db.QueryRowContext(ctx, apiKeySelectPg+" WHERE key_hash = $1", keyHash)
	rec, err := scanAPIKeyPg(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

func (s *PostgreSQLStore) ListAPIKeys(ctx context.Context, filter APIKeyFilter) ([]*APIKeyRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedAPIKeySortColumns); err != nil {
		return nil, err
	}

	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(apiKeySelectPg)

	ph := newPlaceholderGen()
	if filter.Role != "" {
		conds = append(conds, "role = "+ph.next())
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
		rec, err := scanAPIKeyPg(rows)
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

func (s *PostgreSQLStore) UpdateAPIKeyLastUsed(ctx context.Context, id string, t time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE apikeys SET last_used = $1 WHERE id = $2`,
		nullableTime(t), id)
	if err != nil {
		return fmt.Errorf("state: UpdateAPIKeyLastUsed: %w", err)
	}
	return affectsRow(res)
}

func (s *PostgreSQLStore) DeleteAPIKey(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM apikeys WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("state: DeleteAPIKey: %w", err)
	}
	return affectsRow(res)
}

func scanAPIKeyPg(r rowLike) (*APIKeyRecord, error) {
	var (
		k                       APIKeyRecord
		createdAt               time.Time
		expiresAt, lastUsedRaw  sql.NullTime
	)
	if err := r.Scan(
		&k.ID, &k.Name, &k.KeyHash, &k.Role,
		&createdAt, &lastUsedRaw, &expiresAt,
	); err != nil {
		return nil, err
	}

	k.CreatedAt = createdAt
	if expiresAt.Valid {
		k.ExpiresAt = expiresAt.Time
	}
	if lastUsedRaw.Valid {
		k.LastUsed = lastUsedRaw.Time
	}

	return &k, nil
}
