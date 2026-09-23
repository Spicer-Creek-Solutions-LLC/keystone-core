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
	if err := s.DB().QueryRow(`SELECT actor_uid, actor_username_snapshot, job_id, target FROM audit`).Scan(&gotUID, &gotName, &job, &target); err != nil {
		t.Fatal(err)
	}
	if gotUID != int64(uid) || gotName != name || job.Valid || target.Valid {
		t.Fatalf("audit identity/scope = %d/%q job=%v target=%v", gotUID, gotName, job, target)
	}
}
