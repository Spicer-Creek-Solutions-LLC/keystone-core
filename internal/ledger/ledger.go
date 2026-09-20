package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const (
	LedgerFileMode = 0o600
	LedgerDirMode  = 0o700
)

var ErrCorrupt = errors.New("ledger is corrupt")

type Ledger struct {
	db   *sql.DB
	path string
}

type Receipt struct {
	JobID      string
	Metadata   []byte
	State      string
	ReceivedAt time.Time
}

type Start struct {
	JobID     string
	StartedAt time.Time
}

type TerminalResult struct {
	JobID          string
	State          string
	ExitStatus     int
	Output         []byte
	Truncated      bool
	DiscardedBytes int
	RecordedAt     time.Time
}

func Open(path string) (*Ledger, error) {
	if path == "" {
		return nil, errors.New("empty ledger path")
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	l := &Ledger{db: db, path: path}
	if err := l.migrate(); err != nil {
		db.Close()
		if info, statErr := os.Stat(path); statErr == nil && info.Size() > 0 {
			return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
		}
		return nil, err
	}
	if err := validate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if err := os.Chmod(path, LedgerFileMode); err != nil {
		db.Close()
		return nil, err
	}
	return l, nil
}

func (l *Ledger) Close() error { return l.db.Close() }
func (l *Ledger) DB() *sql.DB  { return l.db }

func (l *Ledger) RecordReceipt(ctx context.Context, r Receipt) error {
	_, err := l.db.ExecContext(ctx, `INSERT INTO receipts(job_id, metadata, state, received_at) VALUES (?, ?, ?, ?)`, r.JobID, r.Metadata, r.State, r.ReceivedAt.UnixMilli())
	return err
}

func (l *Ledger) RecordStart(ctx context.Context, s Start) error {
	_, err := l.db.ExecContext(ctx, `INSERT INTO starts(job_id, started_at) VALUES (?, ?)`, s.JobID, s.StartedAt.UnixMilli())
	return err
}

func (l *Ledger) RecordTerminalResult(ctx context.Context, r TerminalResult) error {
	_, err := l.db.ExecContext(ctx, `INSERT INTO terminal_results(job_id, state, exit_status, output, truncated, discarded_bytes, recorded_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, r.JobID, r.State, r.ExitStatus, r.Output, boolInt(r.Truncated), r.DiscardedBytes, r.RecordedAt.UnixMilli())
	return err
}

func (l *Ledger) RecordResultPubAck(ctx context.Context, jobID string, ack []byte, at time.Time) error {
	_, err := l.db.ExecContext(ctx, `INSERT INTO result_pubacks(job_id, pub_ack, acked_at) VALUES (?, ?, ?)`, jobID, ack, at.UnixMilli())
	return err
}

func (l *Ledger) Count(ctx context.Context, table string) (int, error) {
	switch table {
	case "receipts", "starts", "terminal_results", "result_pubacks", "schema_migrations":
	default:
		return 0, errors.New("invalid table")
	}
	var n int
	err := l.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n)
	return n, err
}

func (l *Ledger) RetainReceipts(ctx context.Context, before time.Time) error {
	_, err := l.db.ExecContext(ctx, `DELETE FROM receipts WHERE received_at < ?`, before.UnixMilli())
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func validate(db *sql.DB) error {
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return errors.New(integrity)
	}
	return nil
}

func (l *Ledger) migrate() error {
	if _, err := l.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY CHECK(version > 0), applied_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	if _, err := l.db.Exec(`CREATE TABLE IF NOT EXISTS receipts (job_id TEXT PRIMARY KEY, metadata BLOB NOT NULL, state TEXT NOT NULL, received_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	if _, err := l.db.Exec(`CREATE TABLE IF NOT EXISTS starts (job_id TEXT PRIMARY KEY REFERENCES receipts(job_id), started_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	if _, err := l.db.Exec(`CREATE TABLE IF NOT EXISTS terminal_results (job_id TEXT PRIMARY KEY REFERENCES receipts(job_id), state TEXT NOT NULL, exit_status INTEGER NOT NULL, output BLOB NOT NULL, truncated INTEGER NOT NULL, discarded_bytes INTEGER NOT NULL, recorded_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	if _, err := l.db.Exec(`CREATE TABLE IF NOT EXISTS result_pubacks (job_id TEXT PRIMARY KEY REFERENCES terminal_results(job_id), pub_ack BLOB NOT NULL, acked_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	_, err := l.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, strftime('%s','now') * 1000)`)
	return err
}
