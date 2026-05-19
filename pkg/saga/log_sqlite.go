package saga

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

// sqliteLog is the durable [Log] implementation backed by SQLite via
// [dbutil.OpenSQLite] (the project's standard WAL / busy-timeout /
// foreign-key conventions).
//
// # Lossy round-trip — read this
//
// [Execution] carries an `any` Data field and several `error`
// interface values ([Execution.Error], [StepResult.Error],
// [StepResult.CompensateError], [Execution.CompensateErrors]). Neither
// `any` nor `error` round-trips faithfully through any store:
//
//   - Data is JSON-marshalled on save and JSON-unmarshalled on load,
//     so a loaded Data is a generic value (map[string]any, []any,
//     float64, string, bool, nil) — NOT the original concrete type.
//   - Errors are persisted as their .Error() string and rehydrated as
//     a flat [errors.New] value. The original error type and any
//     wrap chain are lost — [errors.Is]/[errors.As] against sentinels
//     will NOT match after a round-trip.
//
// This is intentional and sufficient: the durable log's purpose is
// the audit trail and `list-executions` history, not faithful
// resurrection of live Go objects (the in-flight [Coordinator] keeps
// the real values; the SQLite log is the record after the fact).
// Faithful per-step checkpoint-resume is a separate v1.x surface —
// see docs/project/ROADMAP.md "Saga/checkpoint advanced features".
type sqliteLog struct {
	mu sync.Mutex
	db *sql.DB
}

const sqliteLogSchema = `
CREATE TABLE IF NOT EXISTS saga_executions (
	id                TEXT PRIMARY KEY,
	seq               INTEGER NOT NULL,
	name              TEXT NOT NULL,
	status            TEXT NOT NULL,
	data              TEXT NOT NULL,
	steps             TEXT NOT NULL,
	started_at        TEXT NOT NULL,
	ended_at          TEXT NOT NULL,
	error             TEXT NOT NULL DEFAULT '',
	compensate_errors TEXT NOT NULL DEFAULT '[]'
);
CREATE INDEX IF NOT EXISTS idx_saga_executions_seq ON saga_executions(seq);
`

// NewSQLiteLog opens (or creates) a SQLite-backed [Log] at path and
// applies its schema. Pass ":memory:" for an ephemeral database. The
// returned [Log] also satisfies [io.Closer]; callers own its lifetime
// and should Close it.
//
// See the package-internal lossy-round-trip note: Data and error
// fields do not survive a save/load cycle with their concrete types.
func NewSQLiteLog(path string, opts ...dbutil.Option) (Log, error) {
	db, err := dbutil.OpenSQLite(path, opts...)
	if err != nil {
		return nil, fmt.Errorf("saga: open sqlite: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), sqliteLogSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("saga: apply schema: %w", err)
	}
	return &sqliteLog{db: db}, nil
}

// Close releases the underlying database. Safe on a nil receiver.
func (l *sqliteLog) Close() error {
	if l == nil || l.db == nil {
		return nil
	}
	return l.db.Close()
}

// stepRow is the on-disk shape of a [StepResult] — error interfaces
// flattened to strings.
type stepRow struct {
	Name            string        `json:"name"`
	Status          StepStatus    `json:"status"`
	StartedAt       time.Time     `json:"started_at"`
	Duration        time.Duration `json:"duration"`
	Error           string        `json:"error,omitempty"`
	Compensated     bool          `json:"compensated"`
	CompensateError string        `json:"compensate_error,omitempty"`
	CompensateAt    time.Time     `json:"compensate_at"`
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

func toStepRows(steps []StepResult) []stepRow {
	rows := make([]stepRow, len(steps))
	for i, s := range steps {
		rows[i] = stepRow{
			Name:            s.Name,
			Status:          s.Status,
			StartedAt:       s.StartedAt.UTC(),
			Duration:        s.Duration,
			Error:           errString(s.Error),
			Compensated:     s.Compensated,
			CompensateError: errString(s.CompensateError),
			CompensateAt:    s.CompensateAt.UTC(),
		}
	}
	return rows
}

func fromStepRows(rows []stepRow) []StepResult {
	steps := make([]StepResult, len(rows))
	for i, r := range rows {
		steps[i] = StepResult{
			Name:            r.Name,
			Status:          r.Status,
			StartedAt:       r.StartedAt,
			Duration:        r.Duration,
			Error:           errFromString(r.Error),
			Compensated:     r.Compensated,
			CompensateError: errFromString(r.CompensateError),
			CompensateAt:    r.CompensateAt,
		}
	}
	return steps
}

// SaveExecution upserts e by ID. A nil execution or empty ID is a
// no-op (mirrors the in-memory log). The insertion-order sequence is
// assigned once on first insert and preserved across overwrites so
// [ListExecutions] ordering is stable.
func (l *sqliteLog) SaveExecution(ctx context.Context, e *Execution) error {
	if e == nil || e.ID == "" {
		return nil
	}

	dataJSON, err := json.Marshal(e.Data)
	if err != nil {
		return fmt.Errorf("saga: marshal data: %w", err)
	}
	stepsJSON, err := json.Marshal(toStepRows(e.Steps))
	if err != nil {
		return fmt.Errorf("saga: marshal steps: %w", err)
	}
	compErrs := make([]string, len(e.CompensateErrors))
	for i, ce := range e.CompensateErrors {
		compErrs[i] = errString(ce)
	}
	compErrsJSON, err := json.Marshal(compErrs)
	if err != nil {
		return fmt.Errorf("saga: marshal compensate errors: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	const q = `
INSERT INTO saga_executions
	(id, seq, name, status, data, steps, started_at, ended_at, error, compensate_errors)
VALUES
	(?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM saga_executions), ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	name              = excluded.name,
	status            = excluded.status,
	data              = excluded.data,
	steps             = excluded.steps,
	started_at        = excluded.started_at,
	ended_at          = excluded.ended_at,
	error             = excluded.error,
	compensate_errors = excluded.compensate_errors`

	_, err = l.db.ExecContext(ctx, q,
		e.ID,
		e.Name,
		string(e.Status),
		string(dataJSON),
		string(stepsJSON),
		e.StartedAt.UTC().Format(time.RFC3339Nano),
		e.EndedAt.UTC().Format(time.RFC3339Nano),
		errString(e.Error),
		string(compErrsJSON),
	)
	if err != nil {
		return fmt.Errorf("saga: save execution %q: %w", e.ID, err)
	}
	return nil
}

// GetExecution returns the [Execution] for id, or [ErrNotFound].
// Subject to the package's lossy-round-trip note.
func (l *sqliteLog) GetExecution(ctx context.Context, id string) (*Execution, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	const q = `
SELECT name, status, data, steps, started_at, ended_at, error, compensate_errors
FROM saga_executions WHERE id = ?`
	row := l.db.QueryRowContext(ctx, q, id)
	e, err := scanExecution(id, row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return e, err
}

// ListExecutions returns every execution in insertion order
// (oldest first). The returned slice is freshly built and owned by
// the caller.
func (l *sqliteLog) ListExecutions(ctx context.Context) ([]*Execution, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	const q = `
SELECT id, name, status, data, steps, started_at, ended_at, error, compensate_errors
FROM saga_executions ORDER BY seq ASC`
	rows, err := l.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("saga: list executions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*Execution
	for rows.Next() {
		var id string
		var sc rowScanner = func(dest ...any) error {
			return rows.Scan(append([]any{&id}, dest...)...)
		}
		e, err := scanExecutionInto(sc, &id)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("saga: list executions: %w", err)
	}
	return out, nil
}

// rowScanner abstracts *sql.Row / *sql.Rows scanning so the column
// decode logic is shared between GetExecution and ListExecutions.
type rowScanner func(dest ...any) error

// scanExecution decodes a single-row query whose column list does NOT
// include id (id is the known query parameter).
func scanExecution(id string, row *sql.Row) (*Execution, error) {
	return scanExecutionInto(rowScanner(row.Scan), &id)
}

func scanExecutionInto(scan rowScanner, id *string) (*Execution, error) {
	var (
		name, status, dataJSON, stepsJSON string
		startedAt, endedAt, errStr        string
		compErrsJSON                      string
	)
	if err := scan(&name, &status, &dataJSON, &stepsJSON, &startedAt, &endedAt, &errStr, &compErrsJSON); err != nil {
		return nil, err
	}

	e := &Execution{
		ID:     *id,
		Name:   name,
		Status: ExecutionStatus(status),
		Error:  errFromString(errStr),
	}

	if err := json.Unmarshal([]byte(dataJSON), &e.Data); err != nil {
		return nil, fmt.Errorf("saga: unmarshal data for %q: %w", *id, err)
	}

	var rows []stepRow
	if err := json.Unmarshal([]byte(stepsJSON), &rows); err != nil {
		return nil, fmt.Errorf("saga: unmarshal steps for %q: %w", *id, err)
	}
	e.Steps = fromStepRows(rows)

	var compErrs []string
	if err := json.Unmarshal([]byte(compErrsJSON), &compErrs); err != nil {
		return nil, fmt.Errorf("saga: unmarshal compensate errors for %q: %w", *id, err)
	}
	for _, s := range compErrs {
		e.CompensateErrors = append(e.CompensateErrors, errFromString(s))
	}

	var err error
	if e.StartedAt, err = parseTime(startedAt); err != nil {
		return nil, fmt.Errorf("saga: parse started_at for %q: %w", *id, err)
	}
	if e.EndedAt, err = parseTime(endedAt); err != nil {
		return nil, fmt.Errorf("saga: parse ended_at for %q: %w", *id, err)
	}
	return e, nil
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
