package store

import (
	"context"
	"database/sql"
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
