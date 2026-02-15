// Package dbutil provides shared utilities for database setup.
//
// OpenSQLite opens a SQLite database with standard production settings:
// WAL journal mode, MaxOpenConns=1, and optional PRAGMAs via options.
//
//	db, err := dbutil.OpenSQLite("/var/lib/kscore/data.db")
//	db, err := dbutil.OpenSQLite(path, dbutil.WithForeignKeys(), dbutil.WithBusyTimeout(5000))
package dbutil

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // Register pure-Go SQLite driver
)

// Option configures a SQLite database after opening.
type Option func(ctx context.Context, db *sql.DB) error

// WithForeignKeys enables foreign key constraint enforcement.
func WithForeignKeys() Option {
	return func(ctx context.Context, db *sql.DB) error {
		if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
			return fmt.Errorf("enable foreign keys: %w", err)
		}
		return nil
	}
}

// WithBusyTimeout sets the busy timeout in milliseconds.
func WithBusyTimeout(ms int) Option {
	return func(ctx context.Context, db *sql.DB) error {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout=%d", ms)); err != nil {
			return fmt.Errorf("set busy timeout: %w", err)
		}
		return nil
	}
}

// OpenSQLite opens a SQLite database with standard production settings:
//   - MaxOpenConns=1 (SQLite only supports one writer)
//   - WAL journal mode (better concurrent read performance)
//   - Additional PRAGMAs via options
//
// The caller is responsible for closing the returned *sql.DB.
func OpenSQLite(path string, opts ...Option) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	for _, opt := range opts {
		if err := opt(ctx, db); err != nil {
			db.Close()
			return nil, err
		}
	}

	return db, nil
}
