package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	_ "github.com/lib/pq" // postgres driver
)

// PostgreSQLStore is the lib/pq-backed Store implementation.
//
// CRUD lives in postgres_agents.go, postgres_commands.go,
// postgres_batchjobs.go. This file owns the constructor, Close, Ping,
// and Postgres-specific helpers (DSN building, JSONB unmarshal).
type PostgreSQLStore struct {
	db  *sql.DB
	cfg *Config
}

// newPostgreSQLStore opens the connection at cfg.PostgreSQL.DSN (or the
// DSN built from struct fields), applies pool settings, runs the v1.0
// baseline schema, and returns a ready-to-use store.
func newPostgreSQLStore(cfg *Config) (*PostgreSQLStore, error) {
	db, err := sql.Open("postgres", buildPostgresDSN(&cfg.PostgreSQL))
	if err != nil {
		return nil, fmt.Errorf("state: open postgres: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLife > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLife)
	}

	// Surface unreachable Postgres immediately so callers see a clean
	// connect failure rather than a deferred error on first query.
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("state: ping postgres: %w", err)
	}

	if err := applySchema(context.Background(), db, BackendPostgreSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("state: %w", err)
	}

	return &PostgreSQLStore{db: db, cfg: cfg}, nil
}

// Close releases the underlying *sql.DB. Safe on nil receiver.
func (s *PostgreSQLStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Ping verifies the connection is usable.
func (s *PostgreSQLStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// ---- helpers (postgres-specific) ------------------------------------------

// buildPostgresDSN returns cfg.DSN verbatim if set; otherwise builds a
// URL-form connection string from the struct fields:
//
//	postgres://user:password@host:port/database?sslmode=mode
//
// Special characters in user/password are URL-encoded via
// url.UserPassword, and IPv6 literals are bracketed via net.JoinHostPort
// per PROJECT-DETAILS §4.3:
//
//	postgres://u:p@[::1]:5432/db?sslmode=disable          // IPv6
//	postgres://u:p@10.0.0.1:5432/db?sslmode=require       // IPv4
//	postgres://u:p@db.example.com:5432/db?sslmode=require // hostname
//
// Hostnames and IPv4 literals are passed through unchanged; only IPv6
// gets brackets, which is exactly what PROJECT-DETAILS §4.3 requires
// ("brackets only for IPv6 literals, not hostnames or v4").
func buildPostgresDSN(cfg *PostgreSQLConfig) string {
	if cfg.DSN != "" {
		return cfg.DSN
	}
	port := cfg.Port
	if port == 0 {
		port = 5432
	}
	sslmode := cfg.SSLMode
	if sslmode == "" {
		sslmode = "require"
	}

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     net.JoinHostPort(cfg.Host, strconv.Itoa(port)),
		Path:     "/" + cfg.Database,
		RawQuery: url.Values{"sslmode": {sslmode}}.Encode(),
	}
	return u.String()
}

// unmarshalJSONBytes is the []byte counterpart to unmarshalJSONColumn.
// JSONB columns scan into []byte under lib/pq; nil ([]byte zero value)
// means SQL NULL — leave v unchanged.
func unmarshalJSONBytes(b []byte, v any) error {
	if len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("state: unmarshal json: %w", err)
	}
	return nil
}

// marshalJSONBytes is symmetric to unmarshalJSONBytes for the write side.
// Used for INSERT/UPDATE on JSONB columns.
func marshalJSONBytes(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("state: marshal json: %w", err)
	}
	return b, nil
}

// nullableTime converts a Go time.Time to sql.NullTime, mapping zero to
// SQL NULL. Postgres stores TIMESTAMPTZ natively; lib/pq accepts both
// time.Time and sql.NullTime.
func nullableTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}
