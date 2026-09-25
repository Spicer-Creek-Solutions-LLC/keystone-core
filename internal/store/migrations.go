package store

import (
	"context"
	"fmt"
)

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{1, `CREATE TABLE agents (
		agent_id TEXT PRIMARY KEY,
		signing_public_key BLOB NOT NULL,
		encryption_public_key BLOB NOT NULL,
		state TEXT NOT NULL,
		created_at INTEGER NOT NULL
	);
	CREATE TABLE enrollment (
		token_id TEXT PRIMARY KEY,
		agent_id TEXT,
		status TEXT NOT NULL,
		issued_at INTEGER NOT NULL,
		denied_at INTEGER
	);
	CREATE TABLE jobs (
		job_id TEXT PRIMARY KEY,
		correlation_id TEXT NOT NULL,
		target TEXT NOT NULL,
		argv_json TEXT NOT NULL,
		execution_user TEXT NOT NULL,
		state TEXT NOT NULL,
		accepted_at INTEGER NOT NULL
	);
	CREATE TABLE results (
		job_id TEXT PRIMARY KEY REFERENCES jobs(job_id),
		state TEXT NOT NULL,
		exit_status INTEGER NOT NULL,
		stdout BLOB NOT NULL,
		stderr BLOB NOT NULL,
		truncated INTEGER NOT NULL,
		discarded_bytes INTEGER NOT NULL,
		recorded_at INTEGER NOT NULL
	);
	CREATE TABLE audit (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		job_id TEXT NOT NULL,
		correlation_id TEXT NOT NULL,
		actor TEXT NOT NULL,
		target TEXT NOT NULL,
		action TEXT NOT NULL,
		result TEXT NOT NULL
	);`},
	{2, `CREATE INDEX audit_job_time ON audit(job_id, timestamp);
	CREATE INDEX jobs_state ON jobs(state);`},
	{3, `ALTER TABLE audit RENAME TO audit_v2;
	CREATE TABLE audit (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		job_id TEXT,
		correlation_id TEXT,
		actor TEXT,
		actor_uid INTEGER,
		actor_username_snapshot TEXT,
		target TEXT,
		action TEXT NOT NULL,
		result TEXT NOT NULL
	);
	INSERT INTO audit (id, timestamp, job_id, correlation_id, actor, target, action, result)
		SELECT id, timestamp, job_id, correlation_id, actor, target, action, result FROM audit_v2;
	DROP TABLE audit_v2;
	CREATE INDEX audit_job_time ON audit(job_id, timestamp);`},
	// C05's one migration: the enrollment record ADR-0003 § 6 describes, for
	// every stage at once. Migration 1's table is replaced rather than copied:
	// nothing wrote it before C05, and its nullable agent_id and denied_at are
	// not ADR-0003's shape -- the identifier is assigned at issuance (D-C05-2)
	// and denials are audit records, not token state.
	//
	// state is the token's: issued until S4's single write marks it spent
	// (RFC 0005). The request's public halves and the JWT minted for them are
	// written together at S1, so a retry with the same halves returns the same
	// JWT and one with different halves is refused (ADR-0003 § 7).
	{4, `DROP TABLE enrollment;
	CREATE TABLE enrollment (
		token_id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL UNIQUE,
		agent_name TEXT NOT NULL,
		bootstrap_public_key TEXT NOT NULL UNIQUE,
		state TEXT NOT NULL CHECK (state IN ('issued', 'spent')),
		issued_at INTEGER NOT NULL,
		expires_at INTEGER NOT NULL,
		issued_by_uid INTEGER NOT NULL,
		nats_public_key TEXT,
		signing_public_key BLOB,
		encryption_public_key BLOB,
		permanent_jwt TEXT,
		credential_issued_at INTEGER,
		spent_at INTEGER,
		CHECK ((nats_public_key IS NULL) = (permanent_jwt IS NULL)),
		CHECK ((state = 'spent') = (spent_at IS NOT NULL))
	);
	ALTER TABLE agents ADD COLUMN nats_public_key TEXT;
	ALTER TABLE agents ADD COLUMN agent_name TEXT;`},
}

func (s *Store) migrate(failAt int) error {
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY CHECK(version > 0),
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return err
	}
	for _, m := range migrations {
		var applied int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version).Scan(&applied); err != nil {
			return err
		}
		if applied != 0 {
			continue
		}
		if failAt == m.version {
			return fmt.Errorf("migration %d injected failure", m.version)
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, m.sql); err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, strftime('%s','now') * 1000)`, m.version)
		}
		if err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
