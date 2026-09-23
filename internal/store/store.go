package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const (
	StoreFileMode = 0o600
)

var ErrCorrupt = errors.New("store is corrupt")

type Store struct {
	db   *sql.DB
	path string
}

type AcceptedRequest struct {
	JobID         string
	CorrelationID string
	Target        string
	Argv          []string
	ExecutionUser string
	State         string
	AcceptedAt    time.Time
}

type Result struct {
	JobID          string
	State          string
	ExitStatus     int
	Stdout         []byte
	Stderr         []byte
	Truncated      bool
	DiscardedBytes int
	RecordedAt     time.Time
}

type AuditRecord struct {
	JobID                 *string
	CorrelationID         *string
	Actor                 *string
	ActorUID              *uint32
	ActorUsernameSnapshot *string
	Target                *string
	Action                string
	Result                string
	At                    time.Time
}

func Open(path string) (*Store, error) {
	return open(path, 0)
}

// OpenWithMigrationFailure is used by the acceptance contract to demonstrate
// that a failed migration does not commit a partial step.
func OpenWithMigrationFailure(path string, failAt int) (*Store, error) {
	return open(path, failAt)
}

func open(path string, failAt int) (*Store, error) {
	if err := preparePath(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
	if err := s.migrate(failAt); err != nil {
		db.Close()
		if failAt == 0 {
			if info, statErr := os.Stat(path); statErr == nil && info.Size() > 0 {
				return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
			}
		}
		return nil, err
	}
	if err := validate(s.db); err != nil {
		db.Close()
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if err := os.Chmod(path, StoreFileMode); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func preparePath(path string) error {
	if path == "" {
		return errors.New("empty store path")
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.Chmod(path, StoreFileMode); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) RecordAcceptedRequest(ctx context.Context, r AcceptedRequest) error {
	argv, err := json.Marshal(r.Argv)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO jobs
		(job_id, correlation_id, target, argv_json, execution_user, state, accepted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, r.JobID, r.CorrelationID, r.Target, string(argv), r.ExecutionUser, r.State, r.AcceptedAt.UnixMilli())
	return err
}

func (s *Store) RecordResult(ctx context.Context, r Result) error {
	if r.Stdout == nil {
		r.Stdout = []byte{}
	}
	if r.Stderr == nil {
		r.Stderr = []byte{}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	updated, err := tx.ExecContext(ctx, `UPDATE jobs SET state = ? WHERE job_id = ?`, r.State, r.JobID)
	if err != nil {
		return err
	}
	if n, err := updated.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return fmt.Errorf("job %q does not exist", r.JobID)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO results
		(job_id, state, exit_status, stdout, stderr, truncated, discarded_bytes, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, r.JobID, r.State, r.ExitStatus, r.Stdout, r.Stderr, boolInt(r.Truncated), r.DiscardedBytes, r.RecordedAt.UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AppendAudit(ctx context.Context, jobID, correlationID, actor, target, action, result string, at time.Time) error {
	return s.AppendAuditRecord(ctx, AuditRecord{
		JobID: &jobID, CorrelationID: &correlationID, Actor: &actor, Target: &target,
		Action: action, Result: result, At: at,
	})
}

func (s *Store) AppendAuditRecord(ctx context.Context, r AuditRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit
		(timestamp, job_id, correlation_id, actor, actor_uid, actor_username_snapshot, target, action, result)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.At.UnixMilli(), r.JobID, r.CorrelationID,
		r.Actor, r.ActorUID, r.ActorUsernameSnapshot, r.Target, r.Action, r.Result)
	return err
}

func (s *Store) RetainAudit(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM audit WHERE timestamp < ?`, before.UnixMilli())
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
