// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Schema returns the DDL statements (CREATE TABLE / CREATE INDEX) for
// the v1.0 baseline schema for the given backend.
//
// Each statement is idempotent (CREATE ... IF NOT EXISTS) so applying
// the slice twice is a clean no-op.
//
// Statements are ordered: tables first (in FK-dependency order), then
// indexes. Tables: agents, commands, batch_jobs, batch_agent_results,
// apikeys, state_runs, state_run_results.
//
// Domain epics (events, secrets metadata, audit, policy, cluster) ship
// their own schema in their owning packages and run it from their own
// store constructors.
//
// Returns nil for an unknown backend.
func Schema(backend Backend) []string {
	switch backend {
	case BackendSQLite:
		return sqliteSchema
	case BackendPostgreSQL:
		return postgresSchema
	default:
		return nil
	}
}

// applySchema runs every statement in Schema(backend) against db in
// order, then runs the inline column-migrations (one-off ADD COLUMN
// statements that cover databases created before a column was added
// to the baseline CREATE TABLE). Used by SQLiteStore and
// PostgreSQLStore from their initSchema methods. Both phases are
// idempotent — repeated calls are safe.
//
// Inline migrations live here until the gate-v1.0 "Schema versioning
// via golang-migrate" backlog entry replaces them with a proper
// migration framework.
func applySchema(ctx context.Context, db *sql.DB, backend Backend) error {
	for _, stmt := range Schema(backend) {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("state: applySchema %q: %w",
				schemaStmtSummary(stmt), err)
		}
	}
	if err := migrateAddCommandsPrincipal(ctx, db, backend); err != nil {
		return fmt.Errorf("state: applySchema migrate principal: %w", err)
	}
	return nil
}

// migrateAddCommandsPrincipal adds the `principal` column to the
// commands table if it doesn't already exist. Idempotent on both
// backends. Fresh installs get the column from the baseline CREATE
// TABLE; this function exists to upgrade databases that pre-date
// the column.
//
// SQLite doesn't support ADD COLUMN IF NOT EXISTS until very recent
// versions and modernc/sqlite's bundled version isn't guaranteed —
// so we probe via PRAGMA table_info and only add when missing.
// Postgres has supported ADD COLUMN IF NOT EXISTS since 9.6.
func migrateAddCommandsPrincipal(ctx context.Context, db *sql.DB, backend Backend) error {
	switch backend {
	case BackendPostgreSQL:
		_, err := db.ExecContext(ctx,
			`ALTER TABLE commands ADD COLUMN IF NOT EXISTS principal TEXT`)
		return err
	case BackendSQLite:
		rows, err := db.QueryContext(ctx, `PRAGMA table_info(commands)`)
		if err != nil {
			return fmt.Errorf("pragma table_info: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var cid int
			var name, typ string
			var notnull, pk int
			var dfltValue sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
				return fmt.Errorf("scan table_info: %w", err)
			}
			if name == "principal" {
				return rows.Err()
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		_, err = db.ExecContext(ctx, `ALTER TABLE commands ADD COLUMN principal TEXT`)
		return err
	}
	return nil
}

// schemaStmtSummary returns a short, single-line description of a DDL
// statement for use in error messages: "CREATE TABLE IF NOT EXISTS agents"
// instead of the full multi-line body.
func schemaStmtSummary(stmt string) string {
	for _, line := range strings.Split(stmt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if i := strings.Index(line, "("); i > 0 {
			return strings.TrimSpace(line[:i])
		}
		return line
	}
	return "<empty>"
}

// SQLite v1.0 baseline schema.
//
// JSON columns: TEXT (we marshal/unmarshal in Go).
// Timestamps:   TEXT (RFC3339).
// Booleans:     INTEGER (0/1).
// Foreign keys: declared; enforcement requires PRAGMA foreign_keys=ON
// which pkg/dbutil.OpenSQLite sets at connection time.
var sqliteSchema = []string{
	`CREATE TABLE IF NOT EXISTS agents (
    id                TEXT PRIMARY KEY,
    hostname          TEXT NOT NULL,
    os                TEXT NOT NULL,
    architecture      TEXT NOT NULL,
    ip_addresses      TEXT NOT NULL,
    platform_version  TEXT,
    agent_version     TEXT,
    labels            TEXT NOT NULL,
    status            TEXT NOT NULL,
    registered_at     TEXT NOT NULL,
    last_heartbeat_at TEXT,
    metrics           TEXT
)`,
	`CREATE TABLE IF NOT EXISTS commands (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL REFERENCES agents(id),
    command         TEXT NOT NULL,
    args            TEXT NOT NULL,
    env             TEXT NOT NULL,
    working_dir     TEXT,
    "user"          TEXT,
    principal       TEXT,
    timeout_seconds INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL,
    exit_code       INTEGER,
    stdout          TEXT,
    stderr          TEXT,
    started_at      TEXT,
    completed_at    TEXT
)`,
	`CREATE TABLE IF NOT EXISTS batch_jobs (
    id                TEXT PRIMARY KEY,
    target            TEXT NOT NULL,
    command           TEXT NOT NULL,
    args              TEXT NOT NULL,
    status            TEXT NOT NULL,
    concurrency       INTEGER NOT NULL DEFAULT 0,
    total_agents      INTEGER NOT NULL DEFAULT 0,
    completed_agents  INTEGER NOT NULL DEFAULT 0,
    successful_agents INTEGER NOT NULL DEFAULT 0,
    failed_agents     INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL,
    started_at        TEXT,
    completed_at      TEXT
)`,
	`CREATE TABLE IF NOT EXISTS batch_agent_results (
    batch_job_id     TEXT NOT NULL REFERENCES batch_jobs(id),
    agent_id         TEXT NOT NULL REFERENCES agents(id),
    success          INTEGER NOT NULL,
    exit_code        INTEGER,
    error            TEXT,
    stdout           BLOB,
    stderr           BLOB,
    stdout_truncated INTEGER NOT NULL DEFAULT 0,
    stderr_truncated INTEGER NOT NULL DEFAULT 0,
    started_at       TEXT,
    completed_at     TEXT,
    PRIMARY KEY (batch_job_id, agent_id)
)`,
	`CREATE TABLE IF NOT EXISTS apikeys (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    key_hash   TEXT NOT NULL UNIQUE,
    role       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT,
    last_used  TEXT
)`,
	`CREATE TABLE IF NOT EXISTS state_runs (
    id                TEXT PRIMARY KEY,
    mode              TEXT NOT NULL,
    source            TEXT NOT NULL,
    cluster_id        TEXT NOT NULL DEFAULT '',
    agent_id          TEXT NOT NULL DEFAULT '',
    started_at        TEXT NOT NULL,
    ended_at          TEXT,
    status            TEXT NOT NULL,
    error_message     TEXT NOT NULL DEFAULT '',
    total_count       INTEGER NOT NULL DEFAULT 0,
    changed_count     INTEGER NOT NULL DEFAULT 0,
    unchanged_count   INTEGER NOT NULL DEFAULT 0,
    failed_count      INTEGER NOT NULL DEFAULT 0,
    skipped_count     INTEGER NOT NULL DEFAULT 0,
    drifted_count     INTEGER NOT NULL DEFAULT 0,
    declarations_json TEXT NOT NULL DEFAULT '[]'
)`,
	`CREATE TABLE IF NOT EXISTS state_run_results (
    run_id        TEXT NOT NULL REFERENCES state_runs(id) ON DELETE CASCADE,
    decl_id       TEXT NOT NULL,
    module        TEXT NOT NULL,
    outcome       TEXT NOT NULL,
    check_matches INTEGER,
    check_diff    TEXT NOT NULL DEFAULT '',
    apply_changed INTEGER,
    apply_diff    TEXT NOT NULL DEFAULT '',
    apply_comment TEXT NOT NULL DEFAULT '',
    test_result   INTEGER,
    error_message TEXT NOT NULL DEFAULT '',
    started_at    TEXT NOT NULL,
    duration_ms   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, decl_id)
)`,
	`CREATE TABLE IF NOT EXISTS join_tokens (
    id         TEXT PRIMARY KEY,
    hash       BLOB NOT NULL,
    salt       BLOB NOT NULL,
    prefix     TEXT NOT NULL UNIQUE,
    agent_id   TEXT NOT NULL DEFAULT '',
    ttl_ns     INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at    TEXT,
    max_uses   INTEGER NOT NULL DEFAULT 1,
    used_count INTEGER NOT NULL DEFAULT 0,
    metadata   TEXT NOT NULL DEFAULT '{}'
)`,
	`CREATE TABLE IF NOT EXISTS secret_leases (
    id              TEXT PRIMARY KEY,
    backend         TEXT NOT NULL,
    secret_path     TEXT NOT NULL,
    issued_at       TEXT NOT NULL,
    expires_at      TEXT NOT NULL,
    duration_ns     INTEGER NOT NULL DEFAULT 0,
    renewable       INTEGER NOT NULL DEFAULT 0,
    max_ttl_ns      INTEGER NOT NULL DEFAULT 0,
    state           TEXT NOT NULL,
    strategy        TEXT NOT NULL,
    issued_for      TEXT NOT NULL DEFAULT '',
    last_renewed_at TEXT,
    renew_count     INTEGER NOT NULL DEFAULT 0,
    revoked_at      TEXT,
    metadata        TEXT NOT NULL DEFAULT '{}'
)`,
	`CREATE TABLE IF NOT EXISTS events (
    id              TEXT PRIMARY KEY,
    type            TEXT NOT NULL,
    source          TEXT NOT NULL,
    time            TEXT NOT NULL,
    severity        TEXT NOT NULL,
    correlation_id  TEXT NOT NULL DEFAULT '',
    tags            TEXT NOT NULL DEFAULT '{}',
    data            TEXT NOT NULL DEFAULT '{}',
    subject         TEXT NOT NULL DEFAULT ''
)`,
	`CREATE TABLE IF NOT EXISTS audit_entries (
    id               TEXT PRIMARY KEY,
    timestamp        TEXT NOT NULL,
    policy_id        TEXT NOT NULL DEFAULT '',
    policy_name      TEXT NOT NULL DEFAULT '',
    policy_type      TEXT NOT NULL DEFAULT '',
    resource_type    TEXT NOT NULL DEFAULT '',
    allowed          INTEGER NOT NULL DEFAULT 0,
    duration_ns      INTEGER NOT NULL DEFAULT 0,
    violations       TEXT NOT NULL DEFAULT '[]',
    enforcement_mode TEXT NOT NULL,
    severity         TEXT NOT NULL,
    "user"           TEXT NOT NULL DEFAULT '',
    action           TEXT NOT NULL,
    metadata         TEXT NOT NULL DEFAULT '{}'
)`,
	`CREATE INDEX IF NOT EXISTS agents_status_idx ON agents (status)`,
	`CREATE INDEX IF NOT EXISTS agents_last_heartbeat_at_idx ON agents (last_heartbeat_at)`,
	`CREATE INDEX IF NOT EXISTS commands_status_idx ON commands (status)`,
	`CREATE INDEX IF NOT EXISTS commands_agent_id_idx ON commands (agent_id)`,
	`CREATE INDEX IF NOT EXISTS commands_started_at_idx ON commands (started_at)`,
	`CREATE INDEX IF NOT EXISTS batch_jobs_status_idx ON batch_jobs (status)`,
	`CREATE INDEX IF NOT EXISTS batch_jobs_created_at_idx ON batch_jobs (created_at)`,
	`CREATE INDEX IF NOT EXISTS batch_agent_results_agent_id_idx ON batch_agent_results (agent_id)`,
	`CREATE INDEX IF NOT EXISTS apikeys_role_idx ON apikeys (role)`,
	`CREATE INDEX IF NOT EXISTS state_runs_started_at_idx ON state_runs (started_at)`,
	`CREATE INDEX IF NOT EXISTS state_runs_agent_id_idx ON state_runs (agent_id, started_at)`,
	`CREATE INDEX IF NOT EXISTS state_runs_status_idx ON state_runs (status)`,
	`CREATE INDEX IF NOT EXISTS join_tokens_agent_id_idx ON join_tokens (agent_id)`,
	`CREATE INDEX IF NOT EXISTS join_tokens_expires_at_idx ON join_tokens (expires_at)`,
	`CREATE INDEX IF NOT EXISTS secret_leases_expires_at_idx ON secret_leases (expires_at)`,
	`CREATE INDEX IF NOT EXISTS secret_leases_backend_idx ON secret_leases (backend)`,
	`CREATE INDEX IF NOT EXISTS secret_leases_state_idx ON secret_leases (state)`,
	`CREATE INDEX IF NOT EXISTS events_type_idx ON events (type)`,
	`CREATE INDEX IF NOT EXISTS events_source_idx ON events (source)`,
	`CREATE INDEX IF NOT EXISTS events_time_idx ON events (time DESC)`,
	`CREATE INDEX IF NOT EXISTS events_severity_idx ON events (severity)`,
	`CREATE INDEX IF NOT EXISTS events_correlation_id_idx ON events (correlation_id)`,
	`CREATE INDEX IF NOT EXISTS audit_entries_policy_id_idx ON audit_entries (policy_id)`,
	`CREATE INDEX IF NOT EXISTS audit_entries_user_idx ON audit_entries ("user")`,
	`CREATE INDEX IF NOT EXISTS audit_entries_resource_type_idx ON audit_entries (resource_type)`,
	`CREATE INDEX IF NOT EXISTS audit_entries_timestamp_idx ON audit_entries (timestamp DESC)`,
	`CREATE INDEX IF NOT EXISTS audit_entries_severity_idx ON audit_entries (severity)`,
	`CREATE INDEX IF NOT EXISTS audit_entries_allowed_idx ON audit_entries (allowed)`,
}

// PostgreSQL v1.0 baseline schema.
//
// JSON columns: JSONB.
// Timestamps:   TIMESTAMPTZ.
// Booleans:     BOOLEAN.
// Foreign keys: declared and natively enforced.
var postgresSchema = []string{
	`CREATE TABLE IF NOT EXISTS agents (
    id                TEXT PRIMARY KEY,
    hostname          TEXT NOT NULL,
    os                TEXT NOT NULL,
    architecture      TEXT NOT NULL,
    ip_addresses      JSONB NOT NULL,
    platform_version  TEXT,
    agent_version     TEXT,
    labels            JSONB NOT NULL,
    status            TEXT NOT NULL,
    registered_at     TIMESTAMPTZ NOT NULL,
    last_heartbeat_at TIMESTAMPTZ,
    metrics           JSONB
)`,
	`CREATE TABLE IF NOT EXISTS commands (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL REFERENCES agents(id),
    command         TEXT NOT NULL,
    args            JSONB NOT NULL,
    env             JSONB NOT NULL,
    working_dir     TEXT,
    "user"          TEXT,
    principal       TEXT,
    timeout_seconds INTEGER NOT NULL DEFAULT 0,
    status          TEXT NOT NULL,
    exit_code       INTEGER,
    stdout          TEXT,
    stderr          TEXT,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ
)`,
	`CREATE TABLE IF NOT EXISTS batch_jobs (
    id                TEXT PRIMARY KEY,
    target            JSONB NOT NULL,
    command           TEXT NOT NULL,
    args              JSONB NOT NULL,
    status            TEXT NOT NULL,
    concurrency       INTEGER NOT NULL DEFAULT 0,
    total_agents      INTEGER NOT NULL DEFAULT 0,
    completed_agents  INTEGER NOT NULL DEFAULT 0,
    successful_agents INTEGER NOT NULL DEFAULT 0,
    failed_agents     INTEGER NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL,
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ
)`,
	`CREATE TABLE IF NOT EXISTS batch_agent_results (
    batch_job_id     TEXT NOT NULL REFERENCES batch_jobs(id),
    agent_id         TEXT NOT NULL REFERENCES agents(id),
    success          BOOLEAN NOT NULL,
    exit_code        INTEGER,
    error            TEXT,
    stdout           BYTEA,
    stderr           BYTEA,
    stdout_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    stderr_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    started_at       TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ,
    PRIMARY KEY (batch_job_id, agent_id)
)`,
	`CREATE TABLE IF NOT EXISTS apikeys (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    key_hash   TEXT NOT NULL UNIQUE,
    role       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ,
    last_used  TIMESTAMPTZ
)`,
	`CREATE TABLE IF NOT EXISTS state_runs (
    id                TEXT PRIMARY KEY,
    mode              TEXT NOT NULL,
    source            TEXT NOT NULL,
    cluster_id        TEXT NOT NULL DEFAULT '',
    agent_id          TEXT NOT NULL DEFAULT '',
    started_at        TIMESTAMPTZ NOT NULL,
    ended_at          TIMESTAMPTZ,
    status            TEXT NOT NULL,
    error_message     TEXT NOT NULL DEFAULT '',
    total_count       INTEGER NOT NULL DEFAULT 0,
    changed_count     INTEGER NOT NULL DEFAULT 0,
    unchanged_count   INTEGER NOT NULL DEFAULT 0,
    failed_count      INTEGER NOT NULL DEFAULT 0,
    skipped_count     INTEGER NOT NULL DEFAULT 0,
    drifted_count     INTEGER NOT NULL DEFAULT 0,
    declarations_json JSONB NOT NULL DEFAULT '[]'::jsonb
)`,
	`CREATE TABLE IF NOT EXISTS state_run_results (
    run_id        TEXT NOT NULL REFERENCES state_runs(id) ON DELETE CASCADE,
    decl_id       TEXT NOT NULL,
    module        TEXT NOT NULL,
    outcome       TEXT NOT NULL,
    check_matches BOOLEAN,
    check_diff    TEXT NOT NULL DEFAULT '',
    apply_changed BOOLEAN,
    apply_diff    TEXT NOT NULL DEFAULT '',
    apply_comment TEXT NOT NULL DEFAULT '',
    test_result   BOOLEAN,
    error_message TEXT NOT NULL DEFAULT '',
    started_at    TIMESTAMPTZ NOT NULL,
    duration_ms   BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (run_id, decl_id)
)`,
	`CREATE TABLE IF NOT EXISTS join_tokens (
    id         TEXT PRIMARY KEY,
    hash       BYTEA NOT NULL,
    salt       BYTEA NOT NULL,
    prefix     TEXT NOT NULL UNIQUE,
    agent_id   TEXT NOT NULL DEFAULT '',
    ttl_ns     BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    max_uses   INTEGER NOT NULL DEFAULT 1,
    used_count INTEGER NOT NULL DEFAULT 0,
    metadata   JSONB NOT NULL DEFAULT '{}'::jsonb
)`,
	`CREATE TABLE IF NOT EXISTS secret_leases (
    id              TEXT PRIMARY KEY,
    backend         TEXT NOT NULL,
    secret_path     TEXT NOT NULL,
    issued_at       TIMESTAMPTZ NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    duration_ns     BIGINT NOT NULL DEFAULT 0,
    renewable       BOOLEAN NOT NULL DEFAULT FALSE,
    max_ttl_ns      BIGINT NOT NULL DEFAULT 0,
    state           TEXT NOT NULL,
    strategy        TEXT NOT NULL,
    issued_for      TEXT NOT NULL DEFAULT '',
    last_renewed_at TIMESTAMPTZ,
    renew_count     INTEGER NOT NULL DEFAULT 0,
    revoked_at      TIMESTAMPTZ,
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb
)`,
	`CREATE TABLE IF NOT EXISTS events (
    id              TEXT PRIMARY KEY,
    type            TEXT NOT NULL,
    source          TEXT NOT NULL,
    time            TIMESTAMPTZ NOT NULL,
    severity        TEXT NOT NULL,
    correlation_id  TEXT NOT NULL DEFAULT '',
    tags            JSONB NOT NULL DEFAULT '{}'::jsonb,
    data            JSONB NOT NULL DEFAULT '{}'::jsonb,
    subject         TEXT NOT NULL DEFAULT ''
)`,
	`CREATE TABLE IF NOT EXISTS audit_entries (
    id               TEXT PRIMARY KEY,
    timestamp        TIMESTAMPTZ NOT NULL,
    policy_id        TEXT NOT NULL DEFAULT '',
    policy_name      TEXT NOT NULL DEFAULT '',
    policy_type      TEXT NOT NULL DEFAULT '',
    resource_type    TEXT NOT NULL DEFAULT '',
    allowed          BOOLEAN NOT NULL DEFAULT FALSE,
    duration_ns      BIGINT NOT NULL DEFAULT 0,
    violations       JSONB NOT NULL DEFAULT '[]'::jsonb,
    enforcement_mode TEXT NOT NULL,
    severity         TEXT NOT NULL,
    "user"           TEXT NOT NULL DEFAULT '',
    action           TEXT NOT NULL,
    metadata         JSONB NOT NULL DEFAULT '{}'::jsonb
)`,
	`CREATE INDEX IF NOT EXISTS agents_status_idx ON agents (status)`,
	`CREATE INDEX IF NOT EXISTS agents_last_heartbeat_at_idx ON agents (last_heartbeat_at)`,
	`CREATE INDEX IF NOT EXISTS commands_status_idx ON commands (status)`,
	`CREATE INDEX IF NOT EXISTS commands_agent_id_idx ON commands (agent_id)`,
	`CREATE INDEX IF NOT EXISTS commands_started_at_idx ON commands (started_at)`,
	`CREATE INDEX IF NOT EXISTS batch_jobs_status_idx ON batch_jobs (status)`,
	`CREATE INDEX IF NOT EXISTS batch_jobs_created_at_idx ON batch_jobs (created_at)`,
	`CREATE INDEX IF NOT EXISTS batch_agent_results_agent_id_idx ON batch_agent_results (agent_id)`,
	`CREATE INDEX IF NOT EXISTS apikeys_role_idx ON apikeys (role)`,
	`CREATE INDEX IF NOT EXISTS state_runs_started_at_idx ON state_runs (started_at)`,
	`CREATE INDEX IF NOT EXISTS state_runs_agent_id_idx ON state_runs (agent_id, started_at)`,
	`CREATE INDEX IF NOT EXISTS state_runs_status_idx ON state_runs (status)`,
	`CREATE INDEX IF NOT EXISTS join_tokens_agent_id_idx ON join_tokens (agent_id)`,
	`CREATE INDEX IF NOT EXISTS join_tokens_expires_at_idx ON join_tokens (expires_at)`,
	`CREATE INDEX IF NOT EXISTS secret_leases_expires_at_idx ON secret_leases (expires_at)`,
	`CREATE INDEX IF NOT EXISTS secret_leases_backend_idx ON secret_leases (backend)`,
	`CREATE INDEX IF NOT EXISTS secret_leases_state_idx ON secret_leases (state)`,
	`CREATE INDEX IF NOT EXISTS events_type_idx ON events (type)`,
	`CREATE INDEX IF NOT EXISTS events_source_idx ON events (source)`,
	`CREATE INDEX IF NOT EXISTS events_time_idx ON events (time DESC)`,
	`CREATE INDEX IF NOT EXISTS events_severity_idx ON events (severity)`,
	`CREATE INDEX IF NOT EXISTS events_correlation_id_idx ON events (correlation_id)`,
	`CREATE INDEX IF NOT EXISTS audit_entries_policy_id_idx ON audit_entries (policy_id)`,
	`CREATE INDEX IF NOT EXISTS audit_entries_user_idx ON audit_entries ("user")`,
	`CREATE INDEX IF NOT EXISTS audit_entries_resource_type_idx ON audit_entries (resource_type)`,
	`CREATE INDEX IF NOT EXISTS audit_entries_timestamp_idx ON audit_entries (timestamp DESC)`,
	`CREATE INDEX IF NOT EXISTS audit_entries_severity_idx ON audit_entries (severity)`,
	`CREATE INDEX IF NOT EXISTS audit_entries_allowed_idx ON audit_entries (allowed)`,
}
