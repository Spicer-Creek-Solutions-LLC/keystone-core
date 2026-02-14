package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/shawnbutts/keystone-core/internal/statemgmt"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// mockStateExecutor implements StateExecutor for testing.
type mockStateExecutor struct {
	run *statemgmt.StateRun
	err error
}

func (m *mockStateExecutor) ExecuteState(ctx context.Context, stateFile *statemgmt.StateFile) (*statemgmt.StateRun, error) {
	return m.run, m.err
}

// mockDriftChecker implements StateDriftChecker for testing.
type mockDriftChecker struct {
	report *statemgmt.DriftReport
	err    error
}

func (m *mockDriftChecker) CheckDrift(stateFile *statemgmt.StateFile) (*statemgmt.DriftReport, error) {
	return m.report, m.err
}

// mockApplyStream implements grpc.ServerStreamingServer[pb.ApplyStateResponse] for testing.
type mockApplyStream struct {
	grpc.ServerStreamingServer[pb.ApplyStateResponse]
	responses []*pb.ApplyStateResponse
	ctx       context.Context
}

func newMockApplyStream() *mockApplyStream {
	return &mockApplyStream{ctx: context.Background()}
}

func (m *mockApplyStream) Send(resp *pb.ApplyStateResponse) error {
	m.responses = append(m.responses, resp)
	return nil
}

func (m *mockApplyStream) Context() context.Context {
	return m.ctx
}

func TestStateServer_ApplyState_NilExecutor(t *testing.T) {
	srv := NewStateServer(nil, nil)
	stream := newMockApplyStream()

	err := srv.ApplyState(&pb.ApplyStateRequest{StateContent: "file:\n  /tmp/test:\n    state: present\n"}, stream)
	if err == nil {
		t.Fatal("expected error for nil executor")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestStateServer_ApplyState_NoInput(t *testing.T) {
	exec := &mockStateExecutor{}
	srv := NewStateServer(exec, nil)
	stream := newMockApplyStream()

	err := srv.ApplyState(&pb.ApplyStateRequest{}, stream)
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestStateServer_ApplyState_Success(t *testing.T) {
	now := time.Now()
	run := &statemgmt.StateRun{
		RunID:     "run-123",
		StartTime: now,
		EndTime:   now.Add(time.Second),
		Results: []*statemgmt.StateResult{
			{
				StateID:   "/tmp/test",
				Module:    "file",
				Success:   true,
				Changed:   true,
				Comment:   "File created",
				Duration:  500 * time.Millisecond,
				StartTime: now,
				EndTime:   now.Add(500 * time.Millisecond),
				Changes:   map[string]interface{}{"state": "created"},
			},
		},
		Summary: &statemgmt.RunSummary{
			Total:     1,
			Succeeded: 1,
			Changed:   1,
			Success:   true,
			Duration:  time.Second,
		},
	}

	exec := &mockStateExecutor{run: run}
	srv := NewStateServer(exec, nil)
	stream := newMockApplyStream()

	// Use inline content to avoid file I/O
	err := srv.ApplyState(&pb.ApplyStateRequest{
		StateContent: "file:\n  /tmp/test:\n    state: present\n",
	}, stream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: RUN_START + STATE_RESULT + RUN_COMPLETE = 3
	if len(stream.responses) != 3 {
		t.Fatalf("got %d responses, want 3", len(stream.responses))
	}

	if stream.responses[0].Type != pb.StateResponseType_STATE_RESPONSE_TYPE_RUN_START {
		t.Errorf("first response type = %v, want RUN_START", stream.responses[0].Type)
	}

	if stream.responses[1].Type != pb.StateResponseType_STATE_RESPONSE_TYPE_STATE_RESULT {
		t.Errorf("second response type = %v, want STATE_RESULT", stream.responses[1].Type)
	}
	if stream.responses[1].StateResult.StateId != "/tmp/test" {
		t.Errorf("state_id = %q, want %q", stream.responses[1].StateResult.StateId, "/tmp/test")
	}
	if !stream.responses[1].StateResult.Success {
		t.Error("expected success=true")
	}
	if !stream.responses[1].StateResult.Changed {
		t.Error("expected changed=true")
	}
	if stream.responses[1].StateResult.Changes["state"] != "created" {
		t.Errorf("changes[state] = %q, want %q", stream.responses[1].StateResult.Changes["state"], "created")
	}

	if stream.responses[2].Type != pb.StateResponseType_STATE_RESPONSE_TYPE_RUN_COMPLETE {
		t.Errorf("third response type = %v, want RUN_COMPLETE", stream.responses[2].Type)
	}
	if stream.responses[2].Summary == nil {
		t.Fatal("expected summary in last response")
	}
	if stream.responses[2].Summary.Total != 1 {
		t.Errorf("summary.total = %d, want 1", stream.responses[2].Summary.Total)
	}
	if !stream.responses[2].Summary.Success {
		t.Error("expected summary.success=true")
	}
}

func TestStateServer_ApplyState_ExecutionError(t *testing.T) {
	now := time.Now()
	run := &statemgmt.StateRun{
		RunID:     "run-err",
		StartTime: now,
		EndTime:   now.Add(time.Second),
		Results: []*statemgmt.StateResult{
			{
				StateID: "/tmp/test",
				Module:  "file",
				Success: false,
				Error:   errors.New("permission denied"),
				Comment: "Failed to create file",
			},
		},
		Summary: &statemgmt.RunSummary{
			Total:   1,
			Failed:  1,
			Success: false,
		},
	}

	exec := &mockStateExecutor{run: run, err: errors.New("state failed")}
	srv := NewStateServer(exec, nil)
	stream := newMockApplyStream()

	err := srv.ApplyState(&pb.ApplyStateRequest{
		StateContent: "file:\n  /tmp/test:\n    state: present\n",
	}, stream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still stream results, with RUN_FAILED at the end
	last := stream.responses[len(stream.responses)-1]
	if last.Type != pb.StateResponseType_STATE_RESPONSE_TYPE_RUN_FAILED {
		t.Errorf("last response type = %v, want RUN_FAILED", last.Type)
	}
	if last.Error == "" {
		t.Error("expected error message in last response")
	}
}

func TestStateServer_CheckState_NilExecutor(t *testing.T) {
	srv := NewStateServer(nil, nil)

	_, err := srv.CheckState(context.Background(), &pb.CheckStateRequest{
		StateContent: "file:\n  /tmp/test:\n    state: present\n",
	})
	if err == nil {
		t.Fatal("expected error for nil executor")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestStateServer_CheckState_NoInput(t *testing.T) {
	exec := &mockStateExecutor{}
	srv := NewStateServer(exec, nil)

	_, err := srv.CheckState(context.Background(), &pb.CheckStateRequest{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestStateServer_CheckState_Success(t *testing.T) {
	run := &statemgmt.StateRun{
		RunID: "check-123",
		Results: []*statemgmt.StateResult{
			{
				StateID: "/tmp/test",
				Module:  "file",
				Success: true,
				Changed: true,
				Comment: "Would create file",
				Changes: map[string]interface{}{"state": "created"},
			},
			{
				StateID: "/tmp/existing",
				Module:  "file",
				Success: true,
				Changed: false,
				Comment: "Already present",
			},
		},
	}

	exec := &mockStateExecutor{run: run}
	srv := NewStateServer(exec, nil)

	resp, err := srv.CheckState(context.Background(), &pb.CheckStateRequest{
		StateContent: "file:\n  /tmp/test:\n    state: present\n",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.RunId != "check-123" {
		t.Errorf("run_id = %q, want %q", resp.RunId, "check-123")
	}
	if resp.WouldChange != 1 {
		t.Errorf("would_change = %d, want 1", resp.WouldChange)
	}
	if resp.WouldFail != 0 {
		t.Errorf("would_fail = %d, want 0", resp.WouldFail)
	}
	if len(resp.AgentResults) != 1 {
		t.Fatalf("got %d agent results, want 1", len(resp.AgentResults))
	}
	if resp.AgentResults[0].Total != 2 {
		t.Errorf("agent_results[0].total = %d, want 2", resp.AgentResults[0].Total)
	}
}

func TestStateServer_DetectDrift_NilChecker(t *testing.T) {
	srv := NewStateServer(nil, nil)

	_, err := srv.DetectDrift(context.Background(), &pb.DetectDriftRequest{
		StateContent: "file:\n  /tmp/test:\n    state: present\n",
	})
	if err == nil {
		t.Fatal("expected error for nil drift checker")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestStateServer_DetectDrift_NoInput(t *testing.T) {
	checker := &mockDriftChecker{}
	srv := NewStateServer(nil, checker)

	_, err := srv.DetectDrift(context.Background(), &pb.DetectDriftRequest{})
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestStateServer_DetectDrift_Success(t *testing.T) {
	now := time.Now()
	report := &statemgmt.DriftReport{
		RunID:     "drift-123",
		CheckedAt: now,
		Duration:  time.Second,
		States: []*statemgmt.DriftStatus{
			{
				StateID:  "/tmp/test",
				Module:   "file",
				HasDrift: true,
				Severity: statemgmt.DriftHigh,
				Differences: []statemgmt.Difference{
					{
						Path:     "mode",
						Desired:  "0644",
						Actual:   "0777",
						Severity: statemgmt.DriftHigh,
						Message:  "mode: expected 0644, got 0777",
					},
				},
				CheckedAt: now,
			},
			{
				StateID:   "/tmp/ok",
				Module:    "file",
				HasDrift:  false,
				Severity:  statemgmt.DriftNone,
				CheckedAt: now,
			},
		},
		Summary: &statemgmt.DriftSummary{
			Total:           2,
			NoDrift:         1,
			HighDrift:       1,
			OverallSeverity: statemgmt.DriftHigh,
		},
	}

	checker := &mockDriftChecker{report: report}
	srv := NewStateServer(nil, checker)

	resp, err := srv.DetectDrift(context.Background(), &pb.DetectDriftRequest{
		StateContent: "file:\n  /tmp/test:\n    state: present\n",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.RunId != "drift-123" {
		t.Errorf("run_id = %q, want %q", resp.RunId, "drift-123")
	}
	if resp.Summary == nil {
		t.Fatal("expected summary")
	}
	if resp.Summary.Total != 2 {
		t.Errorf("summary.total = %d, want 2", resp.Summary.Total)
	}
	if resp.Summary.NoDrift != 1 {
		t.Errorf("summary.no_drift = %d, want 1", resp.Summary.NoDrift)
	}
	if resp.Summary.HighDrift != 1 {
		t.Errorf("summary.high_drift = %d, want 1", resp.Summary.HighDrift)
	}
	if !resp.Summary.HasDrift {
		t.Error("expected has_drift=true")
	}
	if resp.Summary.MaxSeverity != pb.DriftSeverity_DRIFT_SEVERITY_HIGH {
		t.Errorf("max_severity = %v, want HIGH", resp.Summary.MaxSeverity)
	}

	if len(resp.AgentResults) != 1 {
		t.Fatalf("got %d agent results, want 1", len(resp.AgentResults))
	}
	ar := resp.AgentResults[0]
	if !ar.HasDrift {
		t.Error("expected agent result has_drift=true")
	}
	if len(ar.DriftStatuses) != 2 {
		t.Fatalf("got %d drift statuses, want 2", len(ar.DriftStatuses))
	}
	if ar.DriftStatuses[0].StateId != "/tmp/test" {
		t.Errorf("drift_statuses[0].state_id = %q, want %q", ar.DriftStatuses[0].StateId, "/tmp/test")
	}
	if !ar.DriftStatuses[0].HasDrift {
		t.Error("expected drift_statuses[0].has_drift=true")
	}
	if len(ar.DriftStatuses[0].Differences) != 1 {
		t.Fatalf("got %d differences, want 1", len(ar.DriftStatuses[0].Differences))
	}
	if ar.DriftStatuses[0].Differences[0].Path != "mode" {
		t.Errorf("difference path = %q, want %q", ar.DriftStatuses[0].Differences[0].Path, "mode")
	}
}

func TestStateServer_DetectDrift_CheckerError(t *testing.T) {
	checker := &mockDriftChecker{err: errors.New("check failed")}
	srv := NewStateServer(nil, checker)

	_, err := srv.DetectDrift(context.Background(), &pb.DetectDriftRequest{
		StateContent: "file:\n  /tmp/test:\n    state: present\n",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("got code %v, want Internal", st.Code())
	}
}

func TestStateServer_GetStateHistory_NilStore(t *testing.T) {
	srv := NewStateServer(nil, nil)

	_, err := srv.GetStateHistory(context.Background(), &pb.GetStateHistoryRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestStateServer_GetStateHistory_WithStore(t *testing.T) {
	srv := NewStateServer(nil, nil)
	now := time.Now()
	store := &mockHistoryStore{
		runs: []*StateHistoryRun{
			{
				RunID:     "run-1",
				AgentID:   "agent-1",
				StateFiles: []string{"/etc/state/web.yaml"},
				Target:    "os:linux",
				Success:   true,
				Total:     5,
				Succeeded: 4,
				Failed:    1,
				Changed:   3,
				Unchanged: 2,
				StartTime: now,
				EndTime:   now.Add(10 * time.Second),
				DurationMs: 10000,
			},
		},
	}
	srv.SetHistoryStore(store)

	resp, err := srv.GetStateHistory(context.Background(), &pb.GetStateHistoryRequest{AgentId: "agent-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(resp.Runs))
	}
	if resp.Runs[0].RunId != "run-1" {
		t.Errorf("run_id = %q, want %q", resp.Runs[0].RunId, "run-1")
	}
	if resp.Runs[0].Summary == nil {
		t.Fatal("expected summary")
	}
	if resp.Runs[0].Summary.Total != 5 {
		t.Errorf("summary.total = %d, want 5", resp.Runs[0].Summary.Total)
	}
}

func TestStateServer_GetStateStatus_EmptyAgentID(t *testing.T) {
	srv := NewStateServer(nil, nil)

	_, err := srv.GetStateStatus(context.Background(), &pb.GetStateStatusRequest{AgentId: ""})
	if err == nil {
		t.Fatal("expected error for empty agent_id")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestStateServer_GetStateStatus_NilStore(t *testing.T) {
	srv := NewStateServer(nil, nil)

	_, err := srv.GetStateStatus(context.Background(), &pb.GetStateStatusRequest{AgentId: "agent-1"})
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestStateServer_GetStateStatus_WithStore(t *testing.T) {
	srv := NewStateServer(nil, nil)
	now := time.Now()
	store := &mockHistoryStore{
		statuses: []*StateStatusEntry{
			{
				AgentID:      "agent-1",
				StateID:      "pkg_nginx",
				Module:       "pkg",
				CurrentState: "installed",
				DesiredState: "installed",
				Compliant:    true,
				LastApplied:  now,
				LastChecked:  now,
			},
			{
				AgentID:      "agent-1",
				StateID:      "file_config",
				Module:       "file",
				CurrentState: "present",
				DesiredState: "present",
				Compliant:    true,
				LastApplied:  now.Add(-time.Hour),
				LastChecked:  now,
			},
		},
	}
	srv.SetHistoryStore(store)

	resp, err := srv.GetStateStatus(context.Background(), &pb.GetStateStatusRequest{AgentId: "agent-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AgentId != "agent-1" {
		t.Errorf("agent_id = %q, want %q", resp.AgentId, "agent-1")
	}
	if len(resp.States) != 2 {
		t.Fatalf("expected 2 states, got %d", len(resp.States))
	}
	if !resp.States[0].Compliant {
		t.Error("expected states[0].compliant = true")
	}
	if resp.LastChecked == nil {
		t.Error("expected non-nil last_checked")
	}
}

// mockHistoryStore implements StateHistoryStore for testing.
type mockHistoryStore struct {
	runs     []*StateHistoryRun
	statuses []*StateStatusEntry
	err      error
}

func (m *mockHistoryStore) SaveRun(_ context.Context, _ *StateHistoryRun) error {
	return m.err
}

func (m *mockHistoryStore) ListRuns(_ context.Context, _ *StateHistoryFilter) ([]*StateHistoryRun, string, error) {
	return m.runs, "", m.err
}

func (m *mockHistoryStore) GetStatus(_ context.Context, _, _ string) ([]*StateStatusEntry, error) {
	return m.statuses, m.err
}

func TestDriftSeverityToProto(t *testing.T) {
	tests := []struct {
		input statemgmt.DriftSeverity
		want  pb.DriftSeverity
	}{
		{statemgmt.DriftNone, pb.DriftSeverity_DRIFT_SEVERITY_NONE},
		{statemgmt.DriftLow, pb.DriftSeverity_DRIFT_SEVERITY_LOW},
		{statemgmt.DriftMedium, pb.DriftSeverity_DRIFT_SEVERITY_MEDIUM},
		{statemgmt.DriftHigh, pb.DriftSeverity_DRIFT_SEVERITY_HIGH},
		{statemgmt.DriftCritical, pb.DriftSeverity_DRIFT_SEVERITY_CRITICAL},
		{"unknown", pb.DriftSeverity_DRIFT_SEVERITY_UNSPECIFIED},
	}

	for _, tt := range tests {
		got := driftSeverityToProto(tt.input)
		if got != tt.want {
			t.Errorf("driftSeverityToProto(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
