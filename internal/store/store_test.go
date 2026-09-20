package store

import (
	"context"
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
