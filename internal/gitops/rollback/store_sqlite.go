// SPDX-License-Identifier: Apache-2.0

package rollback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/pkg/dbutil"
)

// SQLiteStore is the durable [RollbackStore] backed by SQLite via
// [dbutil.OpenSQLite] (the project's standard WAL / busy-timeout /
// foreign-key conventions). It is the drop-in replacement for
// [MemoryStore]; the engine picks one at boot.
//
// # Lossy round-trip
//
// [Result.Error] is an `error` interface. It is persisted as its
// .Error() string and rehydrated via [errors.New]: the concrete type
// and any wrap chain are lost, so errors.Is/As against sentinels will
// NOT match after a load. This mirrors the pkg/saga durable-log
// contract — the stored record is the audit trail, not a faithful
// resurrection of the live executor result.
type SQLiteStore struct {
	mu sync.Mutex
	db *sql.DB
}

const sqliteRollbackSchema = `
CREATE TABLE IF NOT EXISTS gitops_rollbacks (
	id               TEXT PRIMARY KEY,
	seq              INTEGER NOT NULL,
	application      TEXT NOT NULL DEFAULT '',
	executor_type    TEXT NOT NULL DEFAULT '',
	strategy         TEXT NOT NULL DEFAULT '',
	revision         TEXT NOT NULL DEFAULT '',
	reason           TEXT NOT NULL DEFAULT '',
	require_approval  INTEGER NOT NULL DEFAULT 0,
	state            TEXT NOT NULL,
	from_revision    TEXT NOT NULL DEFAULT '',
	to_revision      TEXT NOT NULL DEFAULT '',
	approver         TEXT NOT NULL DEFAULT '',
	error            TEXT NOT NULL DEFAULT '',
	result           TEXT NOT NULL DEFAULT '',
	transitions      TEXT NOT NULL DEFAULT '[]',
	config           TEXT NOT NULL DEFAULT '{}',
	created_at       TEXT NOT NULL,
	updated_at       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gitops_rollbacks_seq ON gitops_rollbacks(seq);
`

// NewSQLiteStore opens (or creates) a SQLite-backed [RollbackStore] at
// path and applies its schema. Pass ":memory:" for an ephemeral DB.
// The returned store also satisfies [io.Closer]; the caller owns its
// lifetime.
func NewSQLiteStore(path string, opts ...dbutil.Option) (*SQLiteStore, error) {
	db, err := dbutil.OpenSQLite(path, opts...)
	if err != nil {
		return nil, fmt.Errorf("rollback: open sqlite: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), sqliteRollbackSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("rollback: apply schema: %w", err)
	}
	if err := ensureConfigColumn(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("rollback: migrate config column: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// ensureConfigColumn brings any pre-task-10 DB up to the current
// schema (the config column was added in task 10 to persist the
// rollback's executor-specific configuration). Idempotent — for a
// freshly-created DB the column already exists and this is a no-op.
// A formal migration framework is the deferred
// "Schema versioning via golang-migrate" ROADMAP item.
func ensureConfigColumn(db *sql.DB) error {
	rows, err := db.QueryContext(context.Background(), `PRAGMA table_info(gitops_rollbacks)`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == "config" {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.ExecContext(context.Background(),
		`ALTER TABLE gitops_rollbacks ADD COLUMN config TEXT NOT NULL DEFAULT '{}'`)
	return err
}

// Close releases the underlying database. Safe on a nil receiver.
func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// resultRow is the on-disk shape of a [Result] — the error interface
// flattened to a string.
type resultRow struct {
	Success      bool           `json:"success"`
	Message      string         `json:"message,omitempty"`
	FromRevision string         `json:"from_revision,omitempty"`
	ToRevision   string         `json:"to_revision,omitempty"`
	Data         map[string]any `json:"data,omitempty"`
	DurationNS   int64          `json:"duration_ns"`
	Error        string         `json:"error,omitempty"`
}

func errString(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

func errFromString(s string) error {
	if s == "" {
		return nil
	}
	return errors.New(s)
}

func marshalResult(r *Result) (string, error) {
	if r == nil {
		return "", nil
	}
	b, err := json.Marshal(resultRow{
		Success:      r.Success,
		Message:      r.Message,
		FromRevision: r.FromRevision,
		ToRevision:   r.ToRevision,
		Data:         r.Data,
		DurationNS:   int64(r.Duration),
		Error:        errString(r.Error),
	})
	if err != nil {
		return "", fmt.Errorf("rollback: marshal result: %w", err)
	}
	return string(b), nil
}

func unmarshalResult(s string) (*Result, error) {
	if s == "" {
		return nil, nil
	}
	var rr resultRow
	if err := json.Unmarshal([]byte(s), &rr); err != nil {
		return nil, fmt.Errorf("rollback: unmarshal result: %w", err)
	}
	return &Result{
		Success:      rr.Success,
		Message:      rr.Message,
		FromRevision: rr.FromRevision,
		ToRevision:   rr.ToRevision,
		Data:         rr.Data,
		Duration:     time.Duration(rr.DurationNS),
		Error:        errFromString(rr.Error),
	}, nil
}

// Save upserts rb by ID, assigning the insertion-order sequence once
// on first insert (preserved across overwrites so [List] order is
// stable). A nil record or empty ID is an error, matching
// [MemoryStore].
func (s *SQLiteStore) Save(ctx context.Context, rb *Rollback) error {
	if rb == nil || rb.ID == "" {
		return errors.New("rollback: store save: nil record or empty id")
	}
	resultJSON, err := marshalResult(rb.Result)
	if err != nil {
		return err
	}
	transitions := rb.Transitions
	if transitions == nil {
		transitions = []TransitionRecord{}
	}
	transJSON, err := json.Marshal(transitions)
	if err != nil {
		return fmt.Errorf("rollback: marshal transitions: %w", err)
	}
	cfg := rb.Config
	if cfg == nil {
		cfg = Config{}
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("rollback: marshal config: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	const q = `
INSERT INTO gitops_rollbacks
	(id, seq, application, executor_type, strategy, revision, reason,
	 require_approval, state, from_revision, to_revision, approver,
	 error, result, transitions, config, created_at, updated_at)
VALUES
	(?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM gitops_rollbacks),
	 ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	application      = excluded.application,
	executor_type    = excluded.executor_type,
	strategy         = excluded.strategy,
	revision         = excluded.revision,
	reason           = excluded.reason,
	require_approval  = excluded.require_approval,
	state            = excluded.state,
	from_revision    = excluded.from_revision,
	to_revision      = excluded.to_revision,
	approver         = excluded.approver,
	error            = excluded.error,
	result           = excluded.result,
	transitions      = excluded.transitions,
	config           = excluded.config,
	updated_at       = excluded.updated_at`

	_, err = s.db.ExecContext(ctx, q,
		rb.ID,
		rb.Application,
		rb.ExecutorType,
		string(rb.Strategy),
		rb.Revision,
		rb.Reason,
		boolToInt(rb.RequireApproval),
		string(rb.State),
		rb.FromRevision,
		rb.ToRevision,
		rb.Approver,
		rb.Error,
		resultJSON,
		string(transJSON),
		string(cfgJSON),
		rb.CreatedAt.UTC().Format(time.RFC3339Nano),
		rb.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("rollback: save %q: %w", rb.ID, err)
	}
	return nil
}

// Get returns the [Rollback] for id. The bool is false (no error)
// when no record exists — matching the [RollbackStore] contract.
func (s *SQLiteStore) Get(ctx context.Context, id string) (*Rollback, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	const q = `
SELECT application, executor_type, strategy, revision, reason,
	require_approval, state, from_revision, to_revision, approver,
	error, result, transitions, config, created_at, updated_at
FROM gitops_rollbacks WHERE id = ?`
	rb, err := scanRollback(id, rowScanner(s.db.QueryRowContext(ctx, q, id).Scan))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return rb, true, nil
}

// List returns every rollback in insertion order (oldest first).
func (s *SQLiteStore) List(ctx context.Context) ([]*Rollback, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	const q = `
SELECT id, application, executor_type, strategy, revision, reason,
	require_approval, state, from_revision, to_revision, approver,
	error, result, transitions, config, created_at, updated_at
FROM gitops_rollbacks ORDER BY seq ASC`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("rollback: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*Rollback
	for rows.Next() {
		var id string
		rb, err := scanRollback("", func(dest ...any) error {
			return rows.Scan(append([]any{&id}, dest...)...)
		})
		if err != nil {
			return nil, err
		}
		rb.ID = id
		out = append(out, rb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rollback: list: %w", err)
	}
	return out, nil
}

// rowScanner abstracts *sql.Row / *sql.Rows scanning so the decode is
// shared between Get and List.
type rowScanner func(dest ...any) error

func scanRollback(id string, scan rowScanner) (*Rollback, error) {
	var (
		app, exec, strat, rev, reason           string
		reqApproval                             int
		state, fromRev, toRev, approver, errStr string
		resultJSON, transJSON, cfgJSON          string
		createdAt, updatedAt                    string
	)
	if err := scan(&app, &exec, &strat, &rev, &reason, &reqApproval, &state,
		&fromRev, &toRev, &approver, &errStr, &resultJSON, &transJSON, &cfgJSON,
		&createdAt, &updatedAt); err != nil {
		return nil, err
	}

	result, err := unmarshalResult(resultJSON)
	if err != nil {
		return nil, err
	}
	var transitions []TransitionRecord
	if err := json.Unmarshal([]byte(transJSON), &transitions); err != nil {
		return nil, fmt.Errorf("rollback: unmarshal transitions: %w", err)
	}
	var cfg Config
	if cfgJSON != "" {
		if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
			return nil, fmt.Errorf("rollback: unmarshal config: %w", err)
		}
	}
	if len(cfg) == 0 {
		cfg = nil
	}
	created, err := parseStoreTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("rollback: parse created_at: %w", err)
	}
	updated, err := parseStoreTime(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("rollback: parse updated_at: %w", err)
	}

	return &Rollback{
		ID:              id,
		Application:     app,
		ExecutorType:    exec,
		Strategy:        Strategy(strat),
		Revision:        rev,
		Reason:          reason,
		RequireApproval: reqApproval != 0,
		State:           RollbackState(state),
		Config:          cfg,
		FromRevision:    fromRev,
		ToRevision:      toRev,
		Result:          result,
		Approver:        approver,
		Error:           errStr,
		Transitions:     transitions,
		CreatedAt:       created,
		UpdatedAt:       updated,
	}, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func parseStoreTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
