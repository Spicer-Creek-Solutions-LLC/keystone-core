package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// gosec G101 false-positive: the name contains "token" but the
// literal is a column list, not a credential.
//
// #nosec G101 -- SQL column list, not a credential.
//
//nolint:gosec
const joinTokenSelect = `SELECT
    id, hash, salt, prefix, agent_id,
    ttl_ns, created_at, expires_at, used_at,
    max_uses, used_count, metadata
FROM join_tokens`

// CreateJoinToken persists a new join-token record. Hash + Salt
// are stored verbatim as BLOB; metadata is JSON-encoded. Wraps
// state.ErrDuplicate on a UNIQUE-violation against id or prefix.
func (s *SQLiteStore) CreateJoinToken(ctx context.Context, r *JoinTokenRecord) error {
	if err := validateJoinTokenForCreate(r); err != nil {
		return err
	}
	meta, err := encodeMetadata(r.Metadata)
	if err != nil {
		return fmt.Errorf("state: CreateJoinToken metadata: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO join_tokens (
    id, hash, salt, prefix, agent_id,
    ttl_ns, created_at, expires_at, used_at,
    max_uses, used_count, metadata
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Hash, r.Salt, r.Prefix, r.AgentID,
		int64(r.TTL),
		tsArgRequired(r.CreatedAt),
		tsArgRequired(r.ExpiresAt),
		tsArgNullable(r.UsedAt),
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

// GetJoinToken returns the record by id, or ErrNotFound.
func (s *SQLiteStore) GetJoinToken(ctx context.Context, id string) (*JoinTokenRecord, error) {
	row := s.db.QueryRowContext(ctx, joinTokenSelect+" WHERE id = ?", id)
	rec, err := scanJoinTokenSQLite(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

// LookupJoinTokenByPrefix returns the record by prefix. Prefix is
// UNIQUE per the schema so at most one row matches.
func (s *SQLiteStore) LookupJoinTokenByPrefix(ctx context.Context, prefix string) (*JoinTokenRecord, error) {
	row := s.db.QueryRowContext(ctx, joinTokenSelect+" WHERE prefix = ?", prefix)
	rec, err := scanJoinTokenSQLite(row)
	if err != nil {
		return nil, translateSQLError(err)
	}
	return rec, nil
}

// ListJoinTokens returns the records matching filter.
func (s *SQLiteStore) ListJoinTokens(ctx context.Context, filter JoinTokenFilter) ([]*JoinTokenRecord, error) {
	if err := validateSortColumn(filter.SortColumn, AllowedJoinTokenSortColumns); err != nil {
		return nil, err
	}

	var (
		sb    strings.Builder
		args  []any
		conds []string
	)
	sb.WriteString(joinTokenSelect)

	if filter.AgentID != "" {
		conds = append(conds, "agent_id = ?")
		args = append(args, filter.AgentID)
	}
	if filter.Unused {
		conds = append(conds, "used_count < max_uses")
	}
	if !filter.UnexpiredAt.IsZero() {
		conds = append(conds, "expires_at > ?")
		args = append(args, tsArgRequired(filter.UnexpiredAt))
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
		rec, err := scanJoinTokenSQLite(rows)
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

// MarkJoinTokenUsed atomically increments used_count + sets used_at.
// Returns ErrNotFound when id doesn't match; the join-token
// attestor maps an "already exhausted" race outcome via the
// caller's UsedCount-vs-MaxUses re-check.
//
// The UPDATE's WHERE clause includes `used_count < max_uses` so a
// caller that races past MaxUses sees ErrNotFound (no row updated)
// rather than violating the contract.
func (s *SQLiteStore) MarkJoinTokenUsed(ctx context.Context, id string, now time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE join_tokens SET used_count = used_count + 1, used_at = ?
         WHERE id = ? AND used_count < max_uses`,
		tsArgNullable(now), id)
	if err != nil {
		return fmt.Errorf("state: MarkJoinTokenUsed: %w", err)
	}
	return affectsRow(res)
}

// DeleteJoinToken removes a token by id. Returns ErrNotFound when
// absent; callers that want idempotent delete branch on
// errors.Is(err, ErrNotFound).
func (s *SQLiteStore) DeleteJoinToken(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM join_tokens WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("state: DeleteJoinToken: %w", err)
	}
	return affectsRow(res)
}

// DeleteExpiredJoinTokens removes every token whose expires_at <=
// before. Returns the number of records removed.
func (s *SQLiteStore) DeleteExpiredJoinTokens(ctx context.Context, before time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM join_tokens WHERE expires_at <= ?`,
		tsArgRequired(before))
	if err != nil {
		return 0, fmt.Errorf("state: DeleteExpiredJoinTokens: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("state: DeleteExpiredJoinTokens rows: %w", err)
	}
	return int(n), nil
}

// scanJoinTokenSQLite populates a JoinTokenRecord from a row.
func scanJoinTokenSQLite(r rowLike) (*JoinTokenRecord, error) {
	var (
		rec        JoinTokenRecord
		ttlNS      int64
		createdAt  string
		expiresAt  string
		usedAt     sql.NullString
		metadata   string
	)
	if err := r.Scan(
		&rec.ID, &rec.Hash, &rec.Salt, &rec.Prefix, &rec.AgentID,
		&ttlNS, &createdAt, &expiresAt, &usedAt,
		&rec.MaxUses, &rec.UsedCount,
		&metadata,
	); err != nil {
		return nil, err
	}
	rec.TTL = time.Duration(ttlNS)
	var err error
	if rec.CreatedAt, err = tsParseRequired(createdAt); err != nil {
		return nil, fmt.Errorf("state: scan join_token created_at: %w", err)
	}
	if rec.ExpiresAt, err = tsParseRequired(expiresAt); err != nil {
		return nil, fmt.Errorf("state: scan join_token expires_at: %w", err)
	}
	if rec.UsedAt, err = tsParseNullable(usedAt); err != nil {
		return nil, fmt.Errorf("state: scan join_token used_at: %w", err)
	}
	if rec.Metadata, err = decodeMetadata(metadata); err != nil {
		return nil, fmt.Errorf("state: scan join_token metadata: %w", err)
	}
	return &rec, nil
}

// ---- shared helpers (also used by postgres_join_tokens.go) -------

// validateJoinTokenForCreate is the shared pre-INSERT shape check.
// Both backends apply it before hitting the database so the error
// message is consistent and so we don't waste a round-trip on
// rejections we can catch up-front.
func validateJoinTokenForCreate(r *JoinTokenRecord) error {
	if r == nil {
		return fmt.Errorf("state: CreateJoinToken: nil record")
	}
	if r.ID == "" {
		return fmt.Errorf("state: CreateJoinToken: ID is required")
	}
	if r.Prefix == "" {
		return fmt.Errorf("state: CreateJoinToken: Prefix is required")
	}
	if len(r.Hash) == 0 {
		return fmt.Errorf("state: CreateJoinToken: Hash is required")
	}
	if len(r.Salt) == 0 {
		return fmt.Errorf("state: CreateJoinToken: Salt is required")
	}
	if r.ExpiresAt.IsZero() {
		return fmt.Errorf("state: CreateJoinToken: ExpiresAt is required")
	}
	if r.MaxUses <= 0 {
		return fmt.Errorf("state: CreateJoinToken: MaxUses must be > 0")
	}
	return nil
}

// encodeMetadata renders a map[string]string as a stable JSON
// object. nil + empty both round-trip to "{}".
func encodeMetadata(m map[string]string) (string, error) {
	if len(m) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// decodeMetadata parses the JSON-encoded map[string]string. Empty
// input or "{}" returns nil — keeps test assertions cleaner.
func decodeMetadata(s string) (map[string]string, error) {
	if s == "" || s == "{}" {
		return nil, nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

