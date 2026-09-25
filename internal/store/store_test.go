package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordResultRollsBackForUnknownJob(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	err = s.RecordResult(context.Background(), Result{JobID: "missing", State: "Completed", RecordedAt: time.Now()})
	if err == nil {
		t.Fatal("unknown job accepted")
	}
	var results int
	if err := s.DB().QueryRow("SELECT COUNT(*) FROM results").Scan(&results); err != nil {
		t.Fatal(err)
	}
	if results != 0 {
		t.Fatalf("results = %d, want 0 after rollback", results)
	}
}

func TestAppendConnectionAuditCarriesKernelIdentityAndNullScope(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	uid, name := uint32(1234), "removed-user"
	if err := s.AppendAuditRecord(context.Background(), AuditRecord{
		ActorUID: &uid, ActorUsernameSnapshot: &name,
		Action: "operator.authorization.denied", Result: "denied", At: time.Unix(10, 0),
	}); err != nil {
		t.Fatal(err)
	}
	var gotUID int64
	var gotName string
	var job, target sql.NullString
	var actor sql.NullString
	if err := s.DB().QueryRow(`SELECT actor, actor_uid, actor_username_snapshot, job_id, target FROM audit`).Scan(&actor, &gotUID, &gotName, &job, &target); err != nil {
		t.Fatal(err)
	}
	if actor.Valid || gotUID != int64(uid) || gotName != name || job.Valid || target.Valid {
		t.Fatalf("audit identity/scope = actor:%v %d/%q job=%v target=%v", actor, gotUID, gotName, job, target)
	}
}

func TestAppendGeneralAuditPreservesNonOperatorActor(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.AppendAudit(context.Background(), "job", "correlation", "deadline", "agent", "job.expired", "expired", time.Now()); err != nil {
		t.Fatal(err)
	}
	var actor string
	var uid sql.NullInt64
	var username sql.NullString
	if err := s.DB().QueryRow(`SELECT actor, actor_uid, actor_username_snapshot FROM audit`).Scan(&actor, &uid, &username); err != nil {
		t.Fatal(err)
	}
	if actor != "deadline" || uid.Valid || username.Valid {
		t.Fatalf("general actor = %q, uid=%v username=%v", actor, uid, username)
	}
}

func TestIssueEnrollmentRecordsTokenAndAuditTogether(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	uid := uint32(1501)
	tok := EnrollmentToken{
		TokenID: "t1", AgentID: "a1", AgentName: "web-1", BootstrapPublicKey: "UBOOT",
		IssuedAt: time.Unix(100, 0), ExpiresAt: time.Unix(1000, 0), IssuedByUID: uid,
	}
	audit := AuditRecord{ActorUID: &uid, Action: "enrollment.token.issued", Result: "issued", At: time.Unix(100, 0)}
	if err := s.IssueEnrollment(context.Background(), tok, audit); err != nil {
		t.Fatal(err)
	}
	var state string
	var expires int64
	if err := s.DB().QueryRow(`SELECT state, expires_at FROM enrollment WHERE token_id = 't1'`).Scan(&state, &expires); err != nil {
		t.Fatal(err)
	}
	if state != "issued" || expires != 1000_000 {
		t.Fatalf("state %q expires %d", state, expires)
	}

	// A duplicate agent identifier is refused, and its audit record is not
	// written without the token it would describe.
	dup := tok
	dup.TokenID = "t2"
	if err := s.IssueEnrollment(context.Background(), dup, audit); !errors.Is(err, ErrDuplicateEnrollment) {
		t.Fatalf("duplicate agent identifier: %v", err)
	}
	var audits int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM audit WHERE action = 'enrollment.token.issued'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("%d issuance records for one token", audits)
	}
}

// The token state and its spent time move together; the schema refuses one
// without the other.
func TestEnrollmentStateAndSpentTimeAreOneFact(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, q := range []string{
		`INSERT INTO enrollment (token_id, agent_id, agent_name, bootstrap_public_key, state, issued_at, expires_at, issued_by_uid) VALUES ('t', 'a', 'n', 'k', 'spent', 1, 2, 0)`,
		`INSERT INTO enrollment (token_id, agent_id, agent_name, bootstrap_public_key, state, issued_at, expires_at, issued_by_uid, spent_at) VALUES ('t', 'a', 'n', 'k', 'issued', 1, 2, 0, 3)`,
		`INSERT INTO enrollment (token_id, agent_id, agent_name, bootstrap_public_key, state, issued_at, expires_at, issued_by_uid, nats_public_key) VALUES ('t', 'a', 'n', 'k', 'issued', 1, 2, 0, 'U')`,
	} {
		if _, err := s.DB().Exec(q); err == nil {
			t.Errorf("accepted: %s", q)
		}
	}
}
