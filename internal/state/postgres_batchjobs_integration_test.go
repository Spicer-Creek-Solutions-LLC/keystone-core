// SPDX-License-Identifier: Apache-2.0

//go:build integration

package state

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPg_BatchJobCRUD(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

	b := sampleBatchJob("b-1")
	if err := s.CreateBatchJob(ctx, b); err != nil {
		t.Fatalf("CreateBatchJob: %v", err)
	}

	got, err := s.GetBatchJob(ctx, "b-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != b.ID || got.Command != b.Command || got.Concurrency != b.Concurrency {
		t.Errorf("scalar fields lost: %+v", got)
	}
	if got.Target["role"] != "web" {
		t.Errorf("Target round-trip: %v", got.Target)
	}
	if !got.CreatedAt.Equal(b.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", got.CreatedAt, b.CreatedAt)
	}

	if err := s.UpdateBatchJobCounts(ctx, "b-1", 7, 5, 2); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetBatchJob(ctx, "b-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.CompletedAgents != 7 || got.SuccessfulAgents != 5 || got.FailedAgents != 2 {
		t.Errorf("counts not applied: %+v", got)
	}
}

func TestPg_BatchJob_NotFound(t *testing.T) {
	s := newPgStoreForTest(t)
	if _, err := s.GetBatchJob(t.Context(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetBatchJob: %v", err)
	}
	if err := s.UpdateBatchJobCounts(t.Context(), "missing", 0, 0, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("UpdateBatchJobCounts: %v", err)
	}
}

func TestPg_BatchJob_NilRecord(t *testing.T) {
	s := newPgStoreForTest(t)
	if err := s.CreateBatchJob(t.Context(), nil); err == nil {
		t.Error("CreateBatchJob(nil): expected error")
	}
	if err := s.CreateBatchAgentResult(t.Context(), nil); err == nil {
		t.Error("CreateBatchAgentResult(nil): expected error")
	}
}

func TestPg_ListBatchJobs(t *testing.T) {
	s := newPgStoreForTest(t)
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
			t.Errorf("got %d rows", len(got))
		}
	})
	t.Run("by status", func(t *testing.T) {
		got, err := s.ListBatchJobs(ctx, BatchJobFilter{Status: BatchJobStatusRunning})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "b-2" {
			t.Errorf("got %d rows", len(got))
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
			t.Errorf("got %d rows", len(got))
		}
	})
	t.Run("invalid sort rejected", func(t *testing.T) {
		_, err := s.ListBatchJobs(ctx, BatchJobFilter{SortColumn: "bad"})
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestPg_BatchAgentResults(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()

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
		t.Fatal(err)
	}
	if err := s.CreateBatchAgentResult(ctx, r2); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListBatchAgentResults(ctx, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d", len(got))
	}
	if got[0].AgentID != "agent-1" || got[1].AgentID != "agent-2" {
		t.Errorf("ordering: %s, %s", got[0].AgentID, got[1].AgentID)
	}
	if !got[0].Success || got[1].Success {
		t.Errorf("Success: %v, %v", got[0].Success, got[1].Success)
	}
	if got[1].Error != "boom" || got[1].ExitCode != 1 {
		t.Errorf("r2 fields: %+v", got[1])
	}
}

func TestPg_BatchJob_Lifecycle(t *testing.T) {
	s := newPgStoreForTest(t)
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
		t.Fatal(err)
	}
	if got.Status != BatchJobStatusRunning {
		t.Errorf("Status = %q", got.Status)
	}
	if !got.StartedAt.Equal(startedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, startedAt)
	}

	completedAt := startedAt.Add(time.Minute)
	if err := s.FinalizeBatchJob(ctx, "b-1", BatchJobStatusPartial, completedAt); err != nil {
		t.Fatalf("FinalizeBatchJob: %v", err)
	}
	got, err = s.GetBatchJob(ctx, "b-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != BatchJobStatusPartial {
		t.Errorf("Status = %q, want partial", got.Status)
	}
	if !got.CompletedAt.Equal(completedAt) {
		t.Errorf("CompletedAt = %v, want %v", got.CompletedAt, completedAt)
	}

	// Validation matrix.
	if err := s.MarkBatchJobRunning(ctx, "b-1", time.Time{}); err == nil {
		t.Error("zero startedAt should error")
	}
	if err := s.FinalizeBatchJob(ctx, "b-1", BatchJobStatusRunning, time.Now()); err == nil {
		t.Error("non-terminal status should error")
	}
	if err := s.MarkBatchJobRunning(ctx, "ghost", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v, want ErrNotFound", err)
	}
}

func TestPg_BatchAgentResult_OutputRoundTrip(t *testing.T) {
	s := newPgStoreForTest(t)
	ctx := t.Context()
	if err := s.CreateAgent(ctx, sampleAgent("agent-out")); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateBatchJob(ctx, sampleBatchJob("job-out")); err != nil {
		t.Fatal(err)
	}
	stdout := []byte("hello\nworld\n")
	stderr := []byte("warning: x\n")
	r := &BatchAgentResultRecord{
		BatchJobID:      "job-out",
		AgentID:         "agent-out",
		Success:         true,
		Stdout:          stdout,
		Stderr:          stderr,
		StdoutTruncated: false,
		StderrTruncated: true,
		StartedAt:       time.Date(2026, 5, 10, 14, 0, 0, 0, time.UTC),
		CompletedAt:     time.Date(2026, 5, 10, 14, 0, 1, 0, time.UTC),
	}
	if err := s.CreateBatchAgentResult(ctx, r); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetBatchAgentResult(ctx, "job-out", "agent-out")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Stdout) != string(stdout) {
		t.Errorf("Stdout = %q, want %q", got.Stdout, stdout)
	}
	if string(got.Stderr) != string(stderr) {
		t.Errorf("Stderr = %q", got.Stderr)
	}
	if got.StdoutTruncated || !got.StderrTruncated {
		t.Errorf("truncation flags: stdout=%v stderr=%v",
			got.StdoutTruncated, got.StderrTruncated)
	}
}

func TestPg_GetBatchAgentResult_NotFound(t *testing.T) {
	s := newPgStoreForTest(t)
	_, err := s.GetBatchAgentResult(t.Context(), "missing", "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestPg_BatchAgentResult_ForeignKeyEnforced(t *testing.T) {
	s := newPgStoreForTest(t)
	r := &BatchAgentResultRecord{
		BatchJobID: "no-such-job",
		AgentID:    "no-such-agent",
		Success:    true,
	}
	err := s.CreateBatchAgentResult(t.Context(), r)
	if err == nil {
		t.Fatal("expected FK violation")
	}
	if !strings.Contains(err.Error(), "foreign key") &&
		!strings.Contains(err.Error(), "violates") {
		t.Errorf("expected FK error; got: %v", err)
	}
}
