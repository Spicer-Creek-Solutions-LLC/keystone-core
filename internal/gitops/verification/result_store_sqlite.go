// SPDX-License-Identifier: Apache-2.0

package verification

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

// SQLiteResultStore is the durable [ResultStore] backed by SQLite via
// [dbutil.OpenSQLite].
//
// # Lossy round-trip
//
// Each [Result.Error] is an `error` interface persisted as its
// .Error() string and rehydrated via [errors.New]; the concrete type
// and wrap chain are lost (errors.Is/As won't match after a load).
// Mirrors the pkg/saga durable-log contract — the record is the
// audit/history trail, not a faithful live-object resurrection.
type SQLiteResultStore struct {
	mu sync.Mutex
	db *sql.DB
}

const sqliteVerificationSchema = `
CREATE TABLE IF NOT EXISTS gitops_verifications (
	id          TEXT PRIMARY KEY,
	seq         INTEGER NOT NULL,
	application TEXT NOT NULL DEFAULT '',
	success     INTEGER NOT NULL DEFAULT 0,
	result      TEXT NOT NULL,
	created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_gitops_verifications_seq ON gitops_verifications(seq);
`

// NewSQLiteResultStore opens (or creates) a SQLite-backed
// [ResultStore] at path and applies its schema. Pass ":memory:" for
// an ephemeral DB. The store satisfies [io.Closer]; the caller owns
// its lifetime.
func NewSQLiteResultStore(path string, opts ...dbutil.Option) (*SQLiteResultStore, error) {
	db, err := dbutil.OpenSQLite(path, opts...)
	if err != nil {
		return nil, fmt.Errorf("verification: open sqlite: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), sqliteVerificationSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("verification: apply schema: %w", err)
	}
	return &SQLiteResultStore{db: db}, nil
}

// Close releases the underlying database. Safe on a nil receiver.
func (s *SQLiteResultStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// On-disk shapes: error interfaces flattened to strings, durations to
// nanoseconds.
type resultRow struct {
	Success    bool           `json:"success"`
	Message    string         `json:"message,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	DurationNS int64          `json:"duration_ns"`
	Error      string         `json:"error,omitempty"`
	Retries    int            `json:"retries"`
}

type stepResultRow struct {
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Optional bool      `json:"optional"`
	Skipped  bool      `json:"skipped"`
	Result   resultRow `json:"result"`
}

type workflowRow struct {
	Name       string          `json:"name"`
	Success    bool            `json:"success"`
	Steps      []stepResultRow `json:"steps"`
	DurationNS int64           `json:"duration_ns"`
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

func toResultRow(r Result) resultRow {
	return resultRow{
		Success:    r.Success,
		Message:    r.Message,
		Data:       r.Data,
		DurationNS: int64(r.Duration),
		Error:      errString(r.Error),
		Retries:    r.Retries,
	}
}

func fromResultRow(r resultRow) Result {
	return Result{
		Success:  r.Success,
		Message:  r.Message,
		Data:     r.Data,
		Duration: time.Duration(r.DurationNS),
		Error:    errFromString(r.Error),
		Retries:  r.Retries,
	}
}

func toWorkflowRow(w WorkflowResult) workflowRow {
	steps := make([]stepResultRow, len(w.Steps))
	for i, s := range w.Steps {
		steps[i] = stepResultRow{
			Name:     s.Name,
			Type:     s.Type,
			Optional: s.Optional,
			Skipped:  s.Skipped,
			Result:   toResultRow(s.Result),
		}
	}
	return workflowRow{Name: w.Name, Success: w.Success, Steps: steps, DurationNS: int64(w.Duration)}
}

func fromWorkflowRow(w workflowRow) WorkflowResult {
	steps := make([]StepResult, len(w.Steps))
	for i, s := range w.Steps {
		steps[i] = StepResult{
			Name:     s.Name,
			Type:     s.Type,
			Optional: s.Optional,
			Skipped:  s.Skipped,
			Result:   fromResultRow(s.Result),
		}
	}
	return WorkflowResult{Name: w.Name, Success: w.Success, Steps: steps, Duration: time.Duration(w.DurationNS)}
}

// Save upserts sv by ID, assigning the insertion-order sequence once
// on first insert. Nil record or empty ID is an error.
func (s *SQLiteResultStore) Save(ctx context.Context, sv *StoredVerification) error {
	if sv == nil || sv.ID == "" {
		return errors.New("verification: result store save: nil record or empty id")
	}
	resultJSON, err := json.Marshal(toWorkflowRow(sv.Result))
	if err != nil {
		return fmt.Errorf("verification: marshal result: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	const q = `
INSERT INTO gitops_verifications (id, seq, application, success, result, created_at)
VALUES (?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM gitops_verifications), ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	application = excluded.application,
	success     = excluded.success,
	result      = excluded.result`

	_, err = s.db.ExecContext(ctx, q,
		sv.ID,
		sv.Application,
		boolToInt(sv.Result.Success),
		string(resultJSON),
		sv.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("verification: save %q: %w", sv.ID, err)
	}
	return nil
}

// Get returns the [StoredVerification] for id; (nil,false,nil) when
// absent.
func (s *SQLiteResultStore) Get(ctx context.Context, id string) (*StoredVerification, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	const q = `SELECT application, result, created_at FROM gitops_verifications WHERE id = ?`
	sv, err := scanStored(id, s.db.QueryRowContext(ctx, q, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return sv, true, nil
}

// List returns every stored verification in insertion order.
func (s *SQLiteResultStore) List(ctx context.Context) ([]*StoredVerification, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	const q = `SELECT id, application, result, created_at FROM gitops_verifications ORDER BY seq ASC`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("verification: list: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*StoredVerification
	for rows.Next() {
		var id string
		sv, err := scanStored("", func(dest ...any) error {
			return rows.Scan(append([]any{&id}, dest...)...)
		})
		if err != nil {
			return nil, err
		}
		sv.ID = id
		out = append(out, sv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("verification: list: %w", err)
	}
	return out, nil
}

func scanStored(id string, scan func(dest ...any) error) (*StoredVerification, error) {
	var app, resultJSON, createdAt string
	if err := scan(&app, &resultJSON, &createdAt); err != nil {
		return nil, err
	}
	var wr workflowRow
	if err := json.Unmarshal([]byte(resultJSON), &wr); err != nil {
		return nil, fmt.Errorf("verification: unmarshal result: %w", err)
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("verification: parse created_at: %w", err)
	}
	return &StoredVerification{
		ID:          id,
		Application: app,
		Result:      fromWorkflowRow(wr),
		CreatedAt:   created.UTC(),
	}, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
