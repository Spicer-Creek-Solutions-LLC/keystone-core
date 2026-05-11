package controlplane_test

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func TestGetBatchJob_Validation(t *testing.T) {
	t.Parallel()
	srv, _ := newGRPCFixture(t, nil)
	if _, err := srv.GetBatchJob(context.Background(), &v1.GetBatchJobRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("empty id code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestGetBatchJob_NotFound(t *testing.T) {
	t.Parallel()
	srv, _ := newGRPCFixture(t, nil)
	_, err := srv.GetBatchJob(context.Background(), &v1.GetBatchJobRequest{BatchJobId: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("err code = %v, want NotFound", status.Code(err))
	}
}

func TestGetBatchJob_HappyPath(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{defaultSuccess: true}
	srv, store := newGRPCFixture(t, exec)
	seedAgent(t, store, "a-1")

	id, err := mustExecuteBatch(t, srv, store, exec, []string{"a-1"})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := srv.GetBatchJob(context.Background(), &v1.GetBatchJobRequest{BatchJobId: id})
	if err != nil {
		t.Fatalf("GetBatchJob: %v", err)
	}
	if resp.Batch.Id != id {
		t.Errorf("Batch.Id = %q, want %q", resp.Batch.Id, id)
	}
}

func TestListBatchJobs_Filter(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{defaultSuccess: true}
	srv, store := newGRPCFixture(t, exec)
	seedAgent(t, store, "a-1")
	if _, err := mustExecuteBatch(t, srv, store, exec, []string{"a-1"}); err != nil {
		t.Fatal(err)
	}

	// No filter → at least 1 row.
	resp, err := srv.ListBatchJobs(context.Background(), &v1.ListBatchJobsRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Batches) == 0 {
		t.Error("expected ≥1 batch")
	}

	// Cancelled-status filter on a completed batch → empty.
	resp, err = srv.ListBatchJobs(context.Background(), &v1.ListBatchJobsRequest{
		Status: v1.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED,
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Batches) != 0 {
		t.Errorf("cancelled-only filter = %d batches, want 0", len(resp.Batches))
	}
}

func TestCancelBatchJob_Validation(t *testing.T) {
	t.Parallel()
	srv, _ := newGRPCFixture(t, nil)
	if _, err := srv.CancelBatchJob(context.Background(), &v1.CancelBatchJobRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("empty id code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestCancelBatchJob_NotFound(t *testing.T) {
	t.Parallel()
	srv, _ := newGRPCFixture(t, nil)
	_, err := srv.CancelBatchJob(context.Background(), &v1.CancelBatchJobRequest{BatchJobId: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("err code = %v, want NotFound", status.Code(err))
	}
}

func TestCancelBatchJob_AlreadyFinalized(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{defaultSuccess: true}
	srv, store := newGRPCFixture(t, exec)
	seedAgent(t, store, "a-1")
	id, _ := mustExecuteBatch(t, srv, store, exec, []string{"a-1"})

	_, err := srv.CancelBatchJob(context.Background(), &v1.CancelBatchJobRequest{BatchJobId: id})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("err code = %v, want FailedPrecondition", status.Code(err))
	}
}

func TestGetBatchAgentResult_NotFound(t *testing.T) {
	t.Parallel()
	srv, _ := newGRPCFixture(t, nil)
	_, err := srv.GetBatchAgentResult(context.Background(),
		&v1.GetBatchAgentResultRequest{BatchJobId: "missing", AgentId: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("err code = %v, want NotFound", status.Code(err))
	}
}

func TestGetBatchAgentResult_HappyPath(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{defaultSuccess: true}
	srv, store := newGRPCFixture(t, exec)
	seedAgent(t, store, "a-1")
	id, _ := mustExecuteBatch(t, srv, store, exec, []string{"a-1"})

	resp, err := srv.GetBatchAgentResult(context.Background(),
		&v1.GetBatchAgentResultRequest{BatchJobId: id, AgentId: "a-1"})
	if err != nil {
		t.Fatalf("GetBatchAgentResult: %v", err)
	}
	if resp.Result.AgentId != "a-1" || !resp.Result.Success {
		t.Errorf("result = %+v", resp.Result)
	}
}

func TestListBatchAgentResults(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{defaultSuccess: true}
	srv, store := newGRPCFixture(t, exec)
	for _, id := range []string{"a-1", "a-2"} {
		seedAgent(t, store, id)
	}
	id, _ := mustExecuteBatch(t, srv, store, exec, []string{"a-1", "a-2"})

	resp, err := srv.ListBatchAgentResults(context.Background(),
		&v1.ListBatchAgentResultsRequest{BatchJobId: id})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 2 {
		t.Errorf("results = %d, want 2", len(resp.Results))
	}
}

func TestBatchExecuteCommand_DryRun(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{defaultSuccess: true}
	srv, store := newGRPCFixture(t, exec)
	for _, id := range []string{"w-1", "w-2"} {
		seedAgentLabeled(t, store, id, map[string]string{"role": "web"})
	}

	stream := newFakeStream[v1.BatchExecuteCommandResponse](context.Background())
	err := srv.BatchExecuteCommand(&v1.BatchExecuteCommandRequest{
		Command: "uptime",
		Target:  &v1.Target{Labels: map[string]string{"role": "web"}},
		DryRun:  true,
	}, stream)
	if err != nil {
		t.Fatal(err)
	}

	sent := stream.Sent()
	if len(sent) != 2 {
		t.Fatalf("dry-run sent = %d, want 2 (preview + terminal); got %+v", len(sent), sent)
	}
	preview := sent[0].GetPreview()
	if preview == nil {
		t.Fatalf("first message should be Preview; got %+v", sent[0].Event)
	}
	if len(preview.AgentIds) != 2 {
		t.Errorf("preview agents = %v, want 2", preview.AgentIds)
	}
	term := sent[1].GetTerminal()
	if term == nil || term.Status != v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED {
		t.Errorf("terminal = %+v, want Completed", term)
	}

	// Critical: no batch row created.
	if exec.calls != 0 {
		t.Errorf("dry-run called executor %d times, want 0", exec.calls)
	}
	jobs, _ := store.ListBatchJobs(context.Background(), state.BatchJobFilter{Limit: 10})
	if len(jobs) != 0 {
		t.Errorf("dry-run created %d batch jobs, want 0", len(jobs))
	}
}

// mustExecuteBatch dispatches a one-shot batch through the GRPCServer
// and waits for its terminal. Returns the batch ID.
func mustExecuteBatch(t *testing.T, srv *controlplane.GRPCServer, _ state.Store, exec *scriptedBatchExecutor, agentIDs []string) (string, error) {
	t.Helper()
	_ = exec
	stream := newFakeStream[v1.BatchExecuteCommandResponse](context.Background())
	if err := srv.BatchExecuteCommand(&v1.BatchExecuteCommandRequest{
		Command: "uptime",
		Target:  &v1.Target{AgentIds: append([]string(nil), agentIDs...)},
	}, stream); err != nil {
		return "", err
	}
	for _, m := range stream.Sent() {
		if id := m.GetBatchJobId(); id != "" {
			// Wait briefly for orchestrator to drain.
			time.Sleep(50 * time.Millisecond)
			return id, nil
		}
	}
	t.Fatal("no batch_job_id event")
	return "", nil
}
