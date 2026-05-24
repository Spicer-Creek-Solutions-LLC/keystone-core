// SPDX-License-Identifier: Apache-2.0

// Package dbutil provides storage-engine helpers used across the project.
package dbutil

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

const defaultBusyTimeout = 5 * time.Second

// Option configures OpenSQLite.
type Option func(*config)

type config struct {
	busyTimeout time.Duration
}

// WithBusyTimeout overrides the default 5s busy timeout.
func WithBusyTimeout(d time.Duration) Option {
	return func(c *config) { c.busyTimeout = d }
}

// OpenSQLite opens a SQLite database at path with the conventions every
// Keystone Core component expects: WAL journaling, foreign-key enforcement,
// a busy timeout, and a single-writer connection pool.
func OpenSQLite(path string, opts ...Option) (*sql.DB, error) {
	cfg := config{busyTimeout: defaultBusyTimeout}
	for _, opt := range opts {
		opt(&cfg)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite at %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		fmt.Sprintf("PRAGMA busy_timeout=%d", cfg.busyTimeout.Milliseconds()),
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("apply %q: %w", p, err)
		}
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite at %q: %w", path, err)
	}

	return db, nil
}
