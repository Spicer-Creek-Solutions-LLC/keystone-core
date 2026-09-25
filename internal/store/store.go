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

// EnrollmentToken is one issued token, as S0 records it.
type EnrollmentToken struct {
	TokenID            string
	AgentID            string
	AgentName          string
	BootstrapPublicKey string
	IssuedAt           time.Time
	ExpiresAt          time.Time
	IssuedByUID        uint32
}

// ErrDuplicateEnrollment is returned when a token or agent identifier already
// exists. Both are 128 or more random bits, so it means the generator failed,
// and the caller must not retry with the same values.
var ErrDuplicateEnrollment = errors.New("enrollment token or agent identifier already exists")

// IssueEnrollment records a token and its issuance audit record in one
// transaction. Nothing is returned to the operator until it commits, so a token
// that reached the operator is always on record (ISS-1).
func (s *Store) IssueEnrollment(ctx context.Context, tok EnrollmentToken, audit AuditRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM enrollment WHERE token_id = ? OR agent_id = ?`,
		tok.TokenID, tok.AgentID).Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return ErrDuplicateEnrollment
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO enrollment
		(token_id, agent_id, agent_name, bootstrap_public_key, state, issued_at, expires_at, issued_by_uid)
		VALUES (?, ?, ?, ?, 'issued', ?, ?, ?)`, tok.TokenID, tok.AgentID, tok.AgentName,
		tok.BootstrapPublicKey, tok.IssuedAt.UnixMilli(), tok.ExpiresAt.UnixMilli(), tok.IssuedByUID); err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	return tx.Commit()
}

// EnrollmentRecord is a token's whole record.
type EnrollmentRecord struct {
	EnrollmentToken
	State               string
	NATSPublicKey       string
	SigningPublicKey    []byte
	EncryptionPublicKey []byte
	PermanentJWT        string
	SpentAt             time.Time
}

// Token states.
const (
	TokenIssued = "issued"
	TokenSpent  = "spent"
)

// ErrNoEnrollment is returned for a token or agent with no record.
var ErrNoEnrollment = errors.New("no such enrollment")

const enrollmentColumns = `token_id, agent_id, agent_name, bootstrap_public_key, state, issued_at, expires_at,
	issued_by_uid, nats_public_key, signing_public_key, encryption_public_key, permanent_jwt, spent_at`

func scanEnrollment(row interface{ Scan(...any) error }) (EnrollmentRecord, error) {
	var r EnrollmentRecord
	var issued, expires int64
	var natsKey, jwt sql.NullString
	var spent sql.NullInt64
	err := row.Scan(&r.TokenID, &r.AgentID, &r.AgentName, &r.BootstrapPublicKey, &r.State, &issued, &expires,
		&r.IssuedByUID, &natsKey, &r.SigningPublicKey, &r.EncryptionPublicKey, &jwt, &spent)
	if errors.Is(err, sql.ErrNoRows) {
		return EnrollmentRecord{}, ErrNoEnrollment
	}
	if err != nil {
		return EnrollmentRecord{}, err
	}
	r.IssuedAt, r.ExpiresAt = time.UnixMilli(issued), time.UnixMilli(expires)
	r.NATSPublicKey, r.PermanentJWT = natsKey.String, jwt.String
	if spent.Valid {
		r.SpentAt = time.UnixMilli(spent.Int64)
	}
	return r, nil
}

// Enrollment reads a token's record.
func (s *Store) Enrollment(ctx context.Context, tokenID string) (EnrollmentRecord, error) {
	return scanEnrollment(s.db.QueryRowContext(ctx, `SELECT `+enrollmentColumns+` FROM enrollment WHERE token_id = ?`, tokenID))
}

// EnrollmentByAgent reads the record of the token that assigned an agent's
// identifier.
func (s *Store) EnrollmentByAgent(ctx context.Context, agentID string) (EnrollmentRecord, error) {
	return scanEnrollment(s.db.QueryRowContext(ctx, `SELECT `+enrollmentColumns+` FROM enrollment WHERE agent_id = ?`, agentID))
}

// Credential is what S1 records: the agent's three public halves and the JWT
// minted against them.
type Credential struct {
	NATSPublicKey       string
	SigningPublicKey    []byte
	EncryptionPublicKey []byte
	PermanentJWT        string
	At                  time.Time
}

// ErrAlreadyRecorded is returned when S1 has already recorded halves for the
// token. The caller reads the record and compares (ADR-0003 § 6).
var ErrAlreadyRecorded = errors.New("enrollment credential already recorded")

// RecordCredential is S1's durable write, with its audit record. It succeeds
// once per token: the recorded halves are authoritative from then on.
func (s *Store) RecordCredential(ctx context.Context, tokenID string, c Credential, audit AuditRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE enrollment SET nats_public_key = ?, signing_public_key = ?,
		encryption_public_key = ?, permanent_jwt = ?, credential_issued_at = ?
		WHERE token_id = ? AND state = 'issued' AND nats_public_key IS NULL`,
		c.NATSPublicKey, c.SigningPublicKey, c.EncryptionPublicKey, c.PermanentJWT, c.At.UnixMilli(), tokenID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrAlreadyRecorded
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	return tx.Commit()
}

// ErrNotActivatable is returned when S4 finds the token in no state to
// activate: no credential recorded, or already spent.
var ErrNotActivatable = errors.New("enrollment is not awaiting activation")

// Activate is S4 and S5 in one durable write (RFC 0005 § 1): the agent is
// recorded active with the halves S1 recorded, and the token spent.
func (s *Store) Activate(ctx context.Context, tokenID string, at time.Time, audit AuditRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := scanEnrollment(tx.QueryRowContext(ctx, `SELECT `+enrollmentColumns+` FROM enrollment WHERE token_id = ?`, tokenID))
	if err != nil {
		return err
	}
	if r.State != TokenIssued || r.PermanentJWT == "" {
		return ErrNotActivatable
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agents
		(agent_id, signing_public_key, encryption_public_key, state, created_at, nats_public_key, agent_name)
		VALUES (?, ?, ?, 'active', ?, ?, ?)`, r.AgentID, r.SigningPublicKey, r.EncryptionPublicKey,
		at.UnixMilli(), r.NATSPublicKey, r.AgentName); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE enrollment SET state = 'spent', spent_at = ? WHERE token_id = ?`,
		at.UnixMilli(), tokenID); err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	return tx.Commit()
}

func insertAudit(ctx context.Context, tx *sql.Tx, r AuditRecord) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit
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
