// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/pkg/dbutil"
)

// SQLiteStore is the modernc.org/sqlite-backed Store implementation.
//
// CRUD lives in sqlite_agents.go, sqlite_commands.go,
// sqlite_batchjobs.go. This file owns the constructor, the Store-level
// methods (Close, Ping), and helpers shared across the CRUD files
// (JSON / timestamp / sort-column / pagination).
type SQLiteStore struct {
	db  *sql.DB
	cfg *Config
}

// newSQLiteStore opens the SQLite database at cfg.SQLite.Path with the
// project's standard pragmas (WAL, busy timeout, foreign keys), applies
// the v1.0 baseline schema, and returns a ready-to-use store. Called
// from NewStore (factory.go) after Config validation + applyDefaults.
func newSQLiteStore(cfg *Config) (*SQLiteStore, error) {
	db, err := dbutil.OpenSQLite(cfg.SQLite.Path,
		dbutil.WithBusyTimeout(cfg.SQLite.BusyTimeout))
	if err != nil {
		return nil, fmt.Errorf("state: open sqlite: %w", err)
	}

	// dbutil.OpenSQLite already sets MaxOpenConns=1 (sqlite single writer);
	// honor an explicit override and also wire idle/lifetime if requested.
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLife > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLife)
	}

	if err := applySchema(context.Background(), db, BackendSQLite); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("state: %w", err)
	}

	return &SQLiteStore{db: db, cfg: cfg}, nil
}

// Close releases the underlying *sql.DB. Safe to call on a nil receiver
// or a half-initialized store.
func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Ping verifies the connection is usable.
func (s *SQLiteStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// ---- helpers --------------------------------------------------------------

// rowLike is satisfied by both *sql.Row and *sql.Rows so scan helpers
// can be shared between Get* and List* paths.
type rowLike interface {
	Scan(dest ...any) error
}

// marshalJSONColumn marshals v as JSON. Used for NOT-NULL JSON columns
// (labels, ip_addresses, args, env, target). nil maps/slices encode as
// "null", empty maps/slices as "{}"/"[]" — both valid JSON.
func marshalJSONColumn(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("state: marshal json: %w", err)
	}
	return string(b), nil
}

// unmarshalJSONColumn unmarshals s into v. Empty string is a no-op
// (leaves v zero-valued). Malformed JSON returns a wrapped error rather
// than silently leaving v empty (PROJECT-DETAILS §4.3 explicitly calls
// out: "Don't silently swallow json.Unmarshal errors").
func unmarshalJSONColumn(s string, v any) error {
	if s == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(s), v); err != nil {
		return fmt.Errorf("state: unmarshal json: %w", err)
	}
	return nil
}

// tsArgRequired formats t as RFC3339Nano in UTC for a NOT-NULL column.
func tsArgRequired(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// tsArgNullable formats t for a nullable timestamp column. Zero time
// becomes SQL NULL.
func tsArgNullable(t time.Time) sql.NullString {
	if t.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: t.UTC().Format(time.RFC3339Nano), Valid: true}
}

// tsParseRequired parses an RFC3339Nano timestamp.
func tsParseRequired(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("state: parse timestamp %q: %w", s, err)
	}
	return t, nil
}

// tsParseNullable parses s if Valid; returns zero time otherwise.
func tsParseNullable(s sql.NullString) (time.Time, error) {
	if !s.Valid {
		return time.Time{}, nil
	}
	return tsParseRequired(s.String)
}

// validateSortColumn rejects col if it's non-empty and not in allowed.
// SQL injection guard for ORDER BY (PROJECT-DETAILS §4.3).
func validateSortColumn(col string, allowed []string) error {
	if col == "" {
		return nil
	}
	for _, a := range allowed {
		if col == a {
			return nil
		}
	}
	return fmt.Errorf("state: sort column %q not allowed; allowed: %v", col, allowed)
}

// orderByClause returns " ORDER BY <col> <DIR>". When col is empty it
// uses defaultCol with DESC (newest-first listing).
func orderByClause(col, defaultCol string, desc bool) string {
	if col == "" {
		col = defaultCol
		desc = true
	}
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	return " ORDER BY " + col + " " + dir
}

// limitOffsetClause appends LIMIT and OFFSET. SQLite (and Postgres)
// require LIMIT before OFFSET; when only Offset is set we pad with a
// large LIMIT so the syntax is valid.
const maxLimit = 1<<31 - 1

func limitOffsetClause(limit, offset int) string {
	var sb strings.Builder
	switch {
	case limit > 0:
		fmt.Fprintf(&sb, " LIMIT %d", limit)
	case offset > 0:
		fmt.Fprintf(&sb, " LIMIT %d", maxLimit)
	}
	if offset > 0 {
		fmt.Fprintf(&sb, " OFFSET %d", offset)
	}
	return sb.String()
}

// affectsRow returns ErrNotFound if Exec touched zero rows.
func affectsRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("state: rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// translateSQLError maps sql.ErrNoRows to state.ErrNotFound; passes
// other errors through. Used by Get* paths.
func translateSQLError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// isDuplicateKeyError reports whether err is a UNIQUE-constraint
// violation from either backend driver.
//
//   - lib/pq (Postgres) → *pq.Error with Code "23505" (unique_violation)
//   - modernc.org/sqlite → error message contains "UNIQUE constraint"
//
// Used by Create* paths to wrap state.ErrDuplicate so callers can
// branch with errors.Is without driver-specific knowledge.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	// Driver-agnostic substring match. modernc.org/sqlite emits
	// "constraint failed: UNIQUE constraint failed: ..."; lib/pq's
	// String() includes "unique constraint" in the human-readable
	// message. Both substrings are stable across the supported
	// driver versions; if a future driver changes wording, the
	// state package adds a typed-error fallback here.
	msg := err.Error()
	return contains(msg, "UNIQUE constraint") || contains(msg, "unique constraint")
}

// contains is a strings.Contains shim that avoids the strings import
// in this file (kept minimal — only used by isDuplicateKeyError).
func contains(haystack, needle string) bool {
	if len(needle) > len(haystack) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
