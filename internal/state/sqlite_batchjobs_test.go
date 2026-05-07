package state

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func sampleBatchJob(id string) *BatchJobRecord {
	return &BatchJobRecord{
		ID:          id,
		Target:      map[string]any{"role": "web"},
		Command:     "uptime",
		Args:        []string{"-p"},
		Status:      BatchJobStatusPending,
		Concurrency: 5,
		TotalAgents: 10,
		CreatedAt:   time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC),
	}
}

func TestSQLiteStore_BatchJobCRUD(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	b := sampleBatchJob("b-1")
	if err := s.CreateBatchJob(ctx, b); err != nil {
		t.Fatalf("CreateBatchJob: %v", err)
	}

	got, err := s.GetBatchJob(ctx, "b-1")
	if err != nil {
		t.Fatalf("GetBatchJob: %v", err)
	}
	if got.ID != b.ID || got.Command != b.Command || got.Concurrency != b.Concurrency {
		t.Errorf("scalar fields lost: %+v", got)
	}
	if got.Target["role"] != "web" {
		t.Errorf("Target round-trip: %v", got.Target)
	}
	if len(got.Args) != 1 || got.Args[0] != "-p" {
		t.Errorf("Args round-trip: %v", got.Args)
	}
	if !got.CreatedAt.Equal(b.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, b.CreatedAt)
	}

	if err := s.UpdateBatchJobCounts(ctx, "b-1", 7, 5, 2); err != nil {
		t.Fatalf("UpdateBatchJobCounts: %v", err)
	}
	got, err = s.GetBatchJob(ctx, "b-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.CompletedAgents != 7 || got.SuccessfulAgents != 5 || got.FailedAgents != 2 {
		t.Errorf("counts not applied: %+v", got)
	}
}

func TestSQLiteStore_BatchJob_NotFound(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	if _, err := s.GetBatchJob(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetBatchJob missing: %v, want ErrNotFound", err)
	}
	if err := s.UpdateBatchJobCounts(ctx, "missing", 0, 0, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateBatchJobCounts missing: %v, want ErrNotFound", err)
	}
}

func TestSQLiteStore_ListBatchJobs(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	t0 := time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC)
	b1 := sampleBatchJob("b-1")
	b1.CreatedAt = t0
	b2 := sampleBatchJob("b-2")
	b2.Status = BatchJobStatusRunning
	b2.CreatedAt = t0.Add(time.Hour)
	b3 := sampleBatchJob("b-3")
	b3.CreatedAt = t0.Add(2 * time.Hour)

	for _, b := range []*BatchJobRecord{b1, b2, b3} {
		if err := s.CreateBatchJob(ctx, b); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("all", func(t *testing.T) {
		got, err := s.ListBatchJobs(ctx, BatchJobFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 {
			t.Errorf("ListBatchJobs: %d rows", len(got))
		}
	})

	t.Run("by status", func(t *testing.T) {
		got, err := s.ListBatchJobs(ctx, BatchJobFilter{Status: BatchJobStatusRunning})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "b-2" {
			t.Errorf("by status: %d rows", len(got))
		}
	})

	t.Run("by since/until", func(t *testing.T) {
		got, err := s.ListBatchJobs(ctx, BatchJobFilter{
			Since: t0.Add(30 * time.Minute),
			Until: t0.Add(90 * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "b-2" {
			t.Errorf("time-range: %d rows", len(got))
		}
	})

	t.Run("invalid sort rejected", func(t *testing.T) {
		_, err := s.ListBatchJobs(ctx, BatchJobFilter{SortColumn: "bad"})
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestSQLiteStore_BatchJob_NilRecord(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	if err := s.CreateBatchJob(t.Context(), nil); err == nil {
		t.Error("CreateBatchJob(nil): expected error")
	}
	if err := s.CreateBatchAgentResult(t.Context(), nil); err == nil {
		t.Error("CreateBatchAgentResult(nil): expected error")
	}
}

func TestSQLiteStore_BatchAgentResults(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	// Seed parent rows so FKs satisfy.
	if err := s.CreateAgent(ctx, sampleAgent("agent-1")); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAgent(ctx, sampleAgent("agent-2")); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBatchJob(ctx, sampleBatchJob("job-1")); err != nil {
		t.Fatal(err)
	}

	r1 := &BatchAgentResultRecord{
		BatchJobID:  "job-1",
		AgentID:     "agent-1",
		Success:     true,
		ExitCode:    0,
		StartedAt:   time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 5, 6, 14, 0, 1, 0, time.UTC),
	}
	r2 := &BatchAgentResultRecord{
		BatchJobID:  "job-1",
		AgentID:     "agent-2",
		Success:     false,
		ExitCode:    1,
		Error:       "boom",
		StartedAt:   time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 5, 6, 14, 0, 2, 0, time.UTC),
	}

	if err := s.CreateBatchAgentResult(ctx, r1); err != nil {
		t.Fatalf("CreateBatchAgentResult r1: %v", err)
	}
	if err := s.CreateBatchAgentResult(ctx, r2); err != nil {
		t.Fatalf("CreateBatchAgentResult r2: %v", err)
	}

	got, err := s.ListBatchAgentResults(ctx, "job-1")
	if err != nil {
		t.Fatalf("ListBatchAgentResults: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	// Sorted by agent_id
	if got[0].AgentID != "agent-1" || got[1].AgentID != "agent-2" {
		t.Errorf("ordering: %s, %s", got[0].AgentID, got[1].AgentID)
	}
	if !got[0].Success || got[1].Success {
		t.Errorf("Success values: %v, %v", got[0].Success, got[1].Success)
	}
	if got[1].Error != "boom" {
		t.Errorf("Error: %q", got[1].Error)
	}
	if got[1].ExitCode != 1 {
		t.Errorf("ExitCode: %d", got[1].ExitCode)
	}
}

func TestSQLiteStore_BatchJob_Lifecycle(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()

	if err := s.CreateBatchJob(ctx, sampleBatchJob("b-1")); err != nil {
		t.Fatalf("CreateBatchJob: %v", err)
	}

	startedAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	if err := s.MarkBatchJobRunning(ctx, "b-1", startedAt); err != nil {
		t.Fatalf("MarkBatchJobRunning: %v", err)
	}
	got, err := s.GetBatchJob(ctx, "b-1")
	if err != nil {
		t.Fatalf("GetBatchJob: %v", err)
	}
	if got.Status != BatchJobStatusRunning {
		t.Errorf("Status = %q, want running", got.Status)
	}
	if !got.StartedAt.Equal(startedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, startedAt)
	}

	completedAt := startedAt.Add(time.Minute)
	if err := s.FinalizeBatchJob(ctx, "b-1", BatchJobStatusCompleted, completedAt); err != nil {
		t.Fatalf("FinalizeBatchJob: %v", err)
	}
	got, err = s.GetBatchJob(ctx, "b-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != BatchJobStatusCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if !got.CompletedAt.Equal(completedAt) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, completedAt)
	}
}

func TestSQLiteStore_BatchJob_LifecycleValidation(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateBatchJob(ctx, sampleBatchJob("b-1")); err != nil {
		t.Fatal(err)
	}

	if err := s.MarkBatchJobRunning(ctx, "b-1", time.Time{}); err == nil {
		t.Error("zero startedAt should error")
	}
	if err := s.FinalizeBatchJob(ctx, "b-1", BatchJobStatusCompleted, time.Time{}); err == nil {
		t.Error("zero completedAt should error")
	}
	if err := s.FinalizeBatchJob(ctx, "b-1", BatchJobStatusPending, time.Now()); err == nil {
		t.Error("non-terminal status should error")
	}
	if err := s.FinalizeBatchJob(ctx, "b-1", BatchJobStatusRunning, time.Now()); err == nil {
		t.Error("non-terminal status (running) should error")
	}

	if err := s.MarkBatchJobRunning(ctx, "ghost", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("MarkBatchJobRunning missing: %v, want ErrNotFound", err)
	}
	if err := s.FinalizeBatchJob(ctx, "ghost", BatchJobStatusCompleted, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("FinalizeBatchJob missing: %v, want ErrNotFound", err)
	}
}

func TestSQLiteStore_BatchAgentResult_ForeignKeyEnforced(t *testing.T) {
	s := newSQLiteStoreForTest(t)
	r := &BatchAgentResultRecord{
		BatchJobID: "no-such-job",
		AgentID:    "no-such-agent",
		Success:    true,
	}
	err := s.CreateBatchAgentResult(t.Context(), r)
	if err == nil {
		t.Fatal("expected FK violation")
	}
	if !strings.Contains(err.Error(), "FOREIGN KEY") &&
		!strings.Contains(err.Error(), "constraint") {
		t.Errorf("expected FK error; got: %v", err)
	}
}
