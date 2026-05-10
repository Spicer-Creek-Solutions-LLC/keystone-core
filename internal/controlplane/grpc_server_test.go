package controlplane_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// fakeStream is a minimal in-process grpc.ServerStreamingServer[T] for
// testing. Send copies the proto pointer and records it; Context()
// returns a parent ctx cancellable by the test.
type fakeStream[T any] struct {
	ctx context.Context

	mu   sync.Mutex
	sent []*T
}

func newFakeStream[T any](ctx context.Context) *fakeStream[T] {
	return &fakeStream[T]{ctx: ctx}
}

func (s *fakeStream[T]) Send(msg *T) error {
	s.mu.Lock()
	s.sent = append(s.sent, msg)
	s.mu.Unlock()
	return nil
}

func (s *fakeStream[T]) Sent() []*T {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*T, len(s.sent))
	copy(out, s.sent)
	return out
}

func (s *fakeStream[T]) Context() context.Context           { return s.ctx }
func (s *fakeStream[T]) SetHeader(metadata.MD) error        { return nil }
func (s *fakeStream[T]) SendHeader(metadata.MD) error       { return nil }
func (s *fakeStream[T]) SetTrailer(metadata.MD)             {}
func (s *fakeStream[T]) SendMsg(_ any) error                { return nil }
func (s *fakeStream[T]) RecvMsg(_ any) error                { return nil }

func newGRPCFixture(t *testing.T, exec controlplane.BatchExecutor) (*controlplane.GRPCServer, state.Store) {
	t.Helper()
	store := newTestStore(t)
	disp, err := controlplane.NewBatchDispatcher(controlplane.BatchDispatcherConfig{Store: store})
	if err != nil {
		t.Fatalf("NewBatchDispatcher: %v", err)
	}
	srv, err := controlplane.NewGRPCServer(controlplane.GRPCServerConfig{
		Dispatcher: disp,
		Store:      store,
		Executor:   exec,
	})
	if err != nil {
		t.Fatalf("NewGRPCServer: %v", err)
	}
	return srv, store
}

func TestNewGRPCServer_Validation(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	disp, _ := controlplane.NewBatchDispatcher(controlplane.BatchDispatcherConfig{Store: store})
	if _, err := controlplane.NewGRPCServer(controlplane.GRPCServerConfig{Store: store}); err == nil {
		t.Error("nil Dispatcher should error")
	}
	if _, err := controlplane.NewGRPCServer(controlplane.GRPCServerConfig{Dispatcher: disp}); err == nil {
		t.Error("nil Store should error")
	}
}

func TestExecuteCommand_NoExecutorReturnsUnavailable(t *testing.T) {
	t.Parallel()
	srv, _ := newGRPCFixture(t, nil)
	stream := newFakeStream[v1.ExecuteCommandResponse](context.Background())
	err := srv.ExecuteCommand(
		&v1.ExecuteCommandRequest{AgentId: "x", Command: "uptime"}, stream)
	if status.Code(err) != codes.Unavailable {
		t.Errorf("err code = %v, want Unavailable", status.Code(err))
	}
}

func TestExecuteCommand_ValidationErrors(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{defaultSuccess: true}
	srv, _ := newGRPCFixture(t, exec)

	stream := newFakeStream[v1.ExecuteCommandResponse](context.Background())
	if err := srv.ExecuteCommand(&v1.ExecuteCommandRequest{Command: "x"}, stream); status.Code(err) != codes.InvalidArgument {
		t.Errorf("empty agent_id err code = %v, want InvalidArgument", status.Code(err))
	}
	stream2 := newFakeStream[v1.ExecuteCommandResponse](context.Background())
	if err := srv.ExecuteCommand(&v1.ExecuteCommandRequest{AgentId: "x"}, stream2); status.Code(err) != codes.InvalidArgument {
		t.Errorf("empty command err code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestExecuteCommand_SingleAgentStreamShape(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{
		outcomes: map[string]agentOutcome{
			"agent-1": {success: true, exit: 0},
		},
	}
	srv, store := newGRPCFixture(t, exec)
	seedAgent(t, store, "agent-1")

	stream := newFakeStream[v1.ExecuteCommandResponse](context.Background())
	err := srv.ExecuteCommand(
		&v1.ExecuteCommandRequest{AgentId: "agent-1", Command: "uptime"}, stream)
	if err != nil {
		t.Fatalf("ExecuteCommand: %v", err)
	}

	sent := stream.Sent()
	if len(sent) != 2 {
		t.Fatalf("sent = %d messages, want 2 (command_id + completion)", len(sent))
	}
	if id := sent[0].GetCommandId(); id == "" {
		t.Errorf("first message should carry command_id; got %+v", sent[0].Event)
	}
	comp := sent[1].GetCompletion()
	if comp == nil {
		t.Fatalf("second message should be completion; got %+v", sent[1].Event)
	}
	if comp.Status != v1.CommandStatus_COMMAND_STATUS_COMPLETED {
		t.Errorf("completion status = %v, want COMPLETED", comp.Status)
	}
}

func TestExecuteCommand_FailureMapsToFailedCompletion(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{
		outcomes: map[string]agentOutcome{
			"agent-2": {success: false, exit: 7, errorStr: "boom"},
		},
	}
	srv, store := newGRPCFixture(t, exec)
	seedAgent(t, store, "agent-2")

	stream := newFakeStream[v1.ExecuteCommandResponse](context.Background())
	if err := srv.ExecuteCommand(&v1.ExecuteCommandRequest{AgentId: "agent-2", Command: "x"}, stream); err != nil {
		t.Fatal(err)
	}
	sent := stream.Sent()
	comp := sent[len(sent)-1].GetCompletion()
	if comp == nil || comp.Status != v1.CommandStatus_COMMAND_STATUS_FAILED {
		t.Errorf("completion = %+v, want FAILED", comp)
	}
	if comp.Error == "" {
		t.Error("failed completion must carry an Error string")
	}
}

func TestBatchExecuteCommand_NoExecutorReturnsUnavailable(t *testing.T) {
	t.Parallel()
	srv, _ := newGRPCFixture(t, nil)
	stream := newFakeStream[v1.BatchExecuteCommandResponse](context.Background())
	err := srv.BatchExecuteCommand(
		&v1.BatchExecuteCommandRequest{Command: "x", Target: &v1.Target{AgentIds: []string{"a"}}},
		stream)
	if status.Code(err) != codes.Unavailable {
		t.Errorf("err code = %v, want Unavailable", status.Code(err))
	}
}

func TestBatchExecuteCommand_EmptyCommand(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{defaultSuccess: true}
	srv, _ := newGRPCFixture(t, exec)
	stream := newFakeStream[v1.BatchExecuteCommandResponse](context.Background())
	err := srv.BatchExecuteCommand(&v1.BatchExecuteCommandRequest{}, stream)
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("err code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestBatchExecuteCommand_ZeroMatches_TerminalCompleted(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{defaultSuccess: true}
	srv, _ := newGRPCFixture(t, exec)
	stream := newFakeStream[v1.BatchExecuteCommandResponse](context.Background())
	err := srv.BatchExecuteCommand(
		&v1.BatchExecuteCommandRequest{
			Command: "x",
			Target:  &v1.Target{Labels: map[string]string{"role": "nonexistent"}},
		}, stream)
	if err != nil {
		t.Fatalf("BatchExecuteCommand: %v", err)
	}
	sent := stream.Sent()
	if len(sent) != 1 {
		t.Fatalf("sent = %d, want 1 (terminal only)", len(sent))
	}
	term := sent[0].GetTerminal()
	if term == nil || term.Status != v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED {
		t.Errorf("terminal = %+v, want COMPLETED", term)
	}
}

func TestBatchExecuteCommand_ThreeAgents_StreamShape(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{
		outcomes: map[string]agentOutcome{
			"web-1": {success: true},
			"web-2": {success: true},
			"web-3": {success: false, exit: 1, errorStr: "kaboom"},
		},
		delay: 10 * time.Millisecond,
	}
	srv, store := newGRPCFixture(t, exec)
	for _, id := range []string{"web-1", "web-2", "web-3"} {
		seedAgentLabeled(t, store, id, map[string]string{"role": "web"})
	}

	stream := newFakeStream[v1.BatchExecuteCommandResponse](context.Background())
	err := srv.BatchExecuteCommand(
		&v1.BatchExecuteCommandRequest{
			Command: "uptime",
			Target:  &v1.Target{Labels: map[string]string{"role": "web"}},
		}, stream)
	if err != nil {
		t.Fatalf("BatchExecuteCommand: %v", err)
	}

	sent := stream.Sent()
	if len(sent) == 0 {
		t.Fatal("no stream messages")
	}
	if sent[0].GetBatchJobId() == "" {
		t.Errorf("first message should carry batch_job_id; got %+v", sent[0].Event)
	}
	terminal := sent[len(sent)-1].GetTerminal()
	if terminal == nil {
		t.Fatalf("last message should be terminal; got %+v", sent[len(sent)-1].Event)
	}
	if terminal.Status != v1.BatchJobStatus_BATCH_JOB_STATUS_PARTIAL {
		t.Errorf("terminal.Status = %v, want PARTIAL", terminal.Status)
	}

	starts, completes, failures := 0, 0, 0
	for _, m := range sent[1 : len(sent)-1] {
		l := m.GetLifecycle()
		if l == nil {
			continue
		}
		switch l.Kind {
		case v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_START:
			starts++
		case v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_COMPLETE:
			completes++
		case v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_FAILED:
			failures++
		}
	}
	if starts != 3 {
		t.Errorf("AGENT_START count = %d, want 3", starts)
	}
	if completes != 2 {
		t.Errorf("AGENT_COMPLETE count = %d, want 2", completes)
	}
	if failures != 1 {
		t.Errorf("AGENT_FAILED count = %d, want 1", failures)
	}
}

func TestBatchExecuteCommand_AgentIDs(t *testing.T) {
	t.Parallel()
	exec := &scriptedBatchExecutor{defaultSuccess: true}
	srv, store := newGRPCFixture(t, exec)
	seedAgent(t, store, "explicit-1")
	seedAgent(t, store, "explicit-2")

	stream := newFakeStream[v1.BatchExecuteCommandResponse](context.Background())
	err := srv.BatchExecuteCommand(
		&v1.BatchExecuteCommandRequest{
			Command: "x",
			Target:  &v1.Target{AgentIds: []string{"explicit-1", "explicit-2"}},
		}, stream)
	if err != nil {
		t.Fatalf("BatchExecuteCommand: %v", err)
	}
	sent := stream.Sent()
	term := sent[len(sent)-1].GetTerminal()
	if term == nil || term.Status != v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED {
		t.Errorf("terminal = %+v, want COMPLETED", term)
	}
}

func seedAgent(t *testing.T, store state.Store, id string) {
	t.Helper()
	now := time.Now()
	if err := store.CreateAgent(context.Background(), &state.AgentRecord{
		ID: id, Hostname: id + ".example", OS: "linux", Architecture: "amd64",
		Status: state.AgentStatusConnected, RegisteredAt: now, LastHeartbeatAt: now,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func seedAgentLabeled(t *testing.T, store state.Store, id string, labels map[string]string) {
	t.Helper()
	now := time.Now()
	if err := store.CreateAgent(context.Background(), &state.AgentRecord{
		ID: id, Hostname: id + ".example", OS: "linux", Architecture: "amd64",
		Labels: labels, Status: state.AgentStatusConnected, RegisteredAt: now, LastHeartbeatAt: now,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}
