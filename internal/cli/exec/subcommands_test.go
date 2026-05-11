package exec

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

func TestStatusCmd(t *testing.T) {
	t.Parallel()
	now := timestamppb.New(time.Date(2026, 5, 11, 10, 0, 0, 0, time.UTC))
	fc := &fakeClient{
		GetBatchJobFn: func(_ context.Context, in *v1.GetBatchJobRequest) (*v1.GetBatchJobResponse, error) {
			return &v1.GetBatchJobResponse{Batch: &v1.BatchJob{
				Id:               in.BatchJobId,
				Status:           v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED,
				Command:          "uptime",
				TotalAgents:      3,
				SuccessfulAgents: 3,
				CreatedAt:        now,
			}}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"status", "batch-7"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"batch: batch-7", "status: completed", "command: uptime", "agents: 3 total"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestListCmd(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		ListBatchJobsFn: func(_ context.Context, in *v1.ListBatchJobsRequest) (*v1.ListBatchJobsResponse, error) {
			if in.Limit != 5 {
				t.Errorf("Limit = %d, want 5", in.Limit)
			}
			return &v1.ListBatchJobsResponse{Batches: []*v1.BatchJob{
				{Id: "b-1", Status: v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED, TotalAgents: 2, SuccessfulAgents: 2},
				{Id: "b-2", Status: v1.BatchJobStatus_BATCH_JOB_STATUS_PARTIAL, TotalAgents: 3, SuccessfulAgents: 2, FailedAgents: 1},
			}}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"list", "--limit", "5"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"b-1", "completed", "b-2", "partial"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestListCmd_StatusFilter(t *testing.T) {
	t.Parallel()
	called := false
	fc := &fakeClient{
		ListBatchJobsFn: func(_ context.Context, in *v1.ListBatchJobsRequest) (*v1.ListBatchJobsResponse, error) {
			called = true
			if in.Status != v1.BatchJobStatus_BATCH_JOB_STATUS_FAILED {
				t.Errorf("Status = %v, want FAILED", in.Status)
			}
			return &v1.ListBatchJobsResponse{}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"list", "--status", "failed"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("ListBatchJobs not called")
	}
}

func TestListCmd_BadStatus(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"list", "--status", "nope"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown --status") {
		t.Errorf("err = %v, want unknown-status error", err)
	}
}

func TestCancelCmd(t *testing.T) {
	t.Parallel()
	called := false
	fc := &fakeClient{
		CancelBatchJobFn: func(_ context.Context, in *v1.CancelBatchJobRequest) (*v1.CancelBatchJobResponse, error) {
			called = true
			if in.BatchJobId != "b-9" {
				t.Errorf("id = %q, want b-9", in.BatchJobId)
			}
			return &v1.CancelBatchJobResponse{}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"cancel", "b-9"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("CancelBatchJob not called")
	}
	if !strings.Contains(buf.String(), "cancelled: b-9") {
		t.Errorf("out = %q", buf.String())
	}
}

func TestOutputCmd_SingleAgent(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		GetBatchAgentResultFn: func(_ context.Context, in *v1.GetBatchAgentResultRequest) (*v1.GetBatchAgentResultResponse, error) {
			return &v1.GetBatchAgentResultResponse{Result: &v1.BatchAgentResult{
				BatchJobId: in.BatchJobId,
				AgentId:    in.AgentId,
				ExitCode:   0,
				Stdout:     []byte("hello\n"),
			}}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"output", "b-1", "--agent", "a-1"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"=== agent a-1 stdout (exit 0) ===", "hello"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in:\n%s", want, buf.String())
		}
	}
}

func TestOutputCmd_Raw_SingleAgent(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		ListBatchAgentResultsFn: func(_ context.Context, _ *v1.ListBatchAgentResultsRequest) (*v1.ListBatchAgentResultsResponse, error) {
			return &v1.ListBatchAgentResultsResponse{Results: []*v1.BatchAgentResult{
				{AgentId: "a-1", Stdout: []byte("raw-bytes-only")},
			}}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"output", "b-1", "--raw"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "raw-bytes-only" {
		t.Errorf("out = %q, want raw bytes only", buf.String())
	}
}

func TestOutputCmd_Raw_RejectsMultipleAgents(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		ListBatchAgentResultsFn: func(_ context.Context, _ *v1.ListBatchAgentResultsRequest) (*v1.ListBatchAgentResultsResponse, error) {
			return &v1.ListBatchAgentResultsResponse{Results: []*v1.BatchAgentResult{
				{AgentId: "a-1"}, {AgentId: "a-2"},
			}}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"output", "b-1", "--raw"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--raw requires a single agent") {
		t.Errorf("err = %v, want --raw-requires-single-agent", err)
	}
}

func TestOutputCmd_Raw_RejectsAll(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		GetBatchAgentResultFn: func(_ context.Context, _ *v1.GetBatchAgentResultRequest) (*v1.GetBatchAgentResultResponse, error) {
			return &v1.GetBatchAgentResultResponse{Result: &v1.BatchAgentResult{AgentId: "a-1"}}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"output", "b-1", "--agent", "a-1", "--raw", "--all"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--raw is incompatible") {
		t.Errorf("err = %v, want --raw-incompatible-with-all", err)
	}
}

func TestOutputCmd_AllPrintsBoth(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		GetBatchAgentResultFn: func(_ context.Context, _ *v1.GetBatchAgentResultRequest) (*v1.GetBatchAgentResultResponse, error) {
			return &v1.GetBatchAgentResultResponse{Result: &v1.BatchAgentResult{
				AgentId: "a-1", Stdout: []byte("out\n"), Stderr: []byte("err\n"),
			}}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"output", "b-1", "--agent", "a-1", "--all"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"stdout", "stderr", "out", "err"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in:\n%s", want, buf.String())
		}
	}
}

func TestRunCmd_DryRun(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{
		BatchExecuteCommandFn: func(ctx context.Context, in *v1.BatchExecuteCommandRequest) (v1.ControlPlaneService_BatchExecuteCommandClient, error) {
			if !in.DryRun {
				t.Error("DryRun should be true")
			}
			return &fakeBatchStream{ctx: ctx, events: []*v1.BatchExecuteCommandResponse{
				{Event: &v1.BatchExecuteCommandResponse_Preview{
					Preview: &v1.BatchPreview{AgentIds: []string{"web-1", "web-2"}},
				}},
				{Event: &v1.BatchExecuteCommandResponse_Terminal{
					Terminal: &v1.BatchTerminal{
						Status: v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED,
						At:     timestamppb.Now(),
					},
				}},
			}}, nil
		},
	}
	cmd := NewCommand(Deps{Dial: fakeDial(fc)})
	cmd.SetArgs([]string{"run", "--target", "role:web", "--dry-run", "uptime"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"matched 2 agent(s)", "web-1", "web-2"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("missing %q in:\n%s", want, buf.String())
		}
	}
}
