package controlplane

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"

	"go.keystone-core.io/keystone-core/internal/state"
)

// GRPCServerConfig wires the operator-facing ControlPlaneService.
// Dispatcher and Store are required; BatchExecutor is optional —
// when nil the streaming RPCs respond with codes.Unavailable so the
// binary can boot before task 12 lands the NATS-backed executor.
type GRPCServerConfig struct {
	Dispatcher *BatchDispatcher
	Store      state.AgentStore
	Executor   BatchExecutor
	Logger     *slog.Logger
}

// GRPCServer is the operator-facing ControlPlaneService implementation.
// Methods outside the Epic 07 scope (ServerStatus, ListAgents,
// GetAgent, GetCommandStatus, ListCommandHistory) stay Unimplemented
// here and are owned by their respective epics.
type GRPCServer struct {
	v1.UnimplementedControlPlaneServiceServer

	dispatcher *BatchDispatcher
	store      state.AgentStore
	executor   BatchExecutor
	log        *slog.Logger
}

// NewGRPCServer validates cfg and returns a ControlPlaneService impl.
func NewGRPCServer(cfg GRPCServerConfig) (*GRPCServer, error) {
	if cfg.Dispatcher == nil {
		return nil, errors.New("controlplane: GRPCServer requires a BatchDispatcher")
	}
	if cfg.Store == nil {
		return nil, errors.New("controlplane: GRPCServer requires an AgentStore")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &GRPCServer{
		dispatcher: cfg.Dispatcher,
		store:      cfg.Store,
		executor:   cfg.Executor,
		log:        cfg.Logger,
	}, nil
}

// ExecuteCommand dispatches a single command to one agent as a batch-
// of-one. Emits {command_id} → {completion}. Output chunks are
// reserved for task 10/12 once the agent's CommandResponse bridges
// into BatchAgentResultRecord with stdout/stderr.
func (s *GRPCServer) ExecuteCommand(
	req *v1.ExecuteCommandRequest,
	stream grpc.ServerStreamingServer[v1.ExecuteCommandResponse],
) error {
	if req.GetAgentId() == "" {
		return status.Error(codes.InvalidArgument, "agent_id is required")
	}
	if req.GetCommand() == "" {
		return status.Error(codes.InvalidArgument, "command is required")
	}
	if s.executor == nil {
		return status.Error(codes.Unavailable, "control plane: BatchExecutor not wired (task 9d / 12)")
	}

	ctx := stream.Context()
	events := make(chan batchEvent, 8)
	dec := &streamingDecorator{inner: s.executor, emit: nonBlockingEmit(ctx, events)}

	progress := make(chan BatchProgressEvent, 8)
	batchReq := BatchRequest{
		Command:     req.GetCommand(),
		Args:        req.GetArgs(),
		Concurrency: 1,
	}
	batchID, err := s.dispatcher.ExecuteBatch(ctx, batchReq, []string{req.GetAgentId()}, dec, progress)
	if err != nil {
		return status.Errorf(codes.Internal, "execute batch-of-one: %v", err)
	}

	if err := stream.Send(&v1.ExecuteCommandResponse{
		Event: &v1.ExecuteCommandResponse_CommandId{CommandId: batchID},
	}); err != nil {
		return err
	}

	// Wait for the single agent's completion, then map to
	// CommandCompletion and return.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-events:
			if ev.kind != eventKindStarted {
				if err := stream.Send(&v1.ExecuteCommandResponse{
					Event: &v1.ExecuteCommandResponse_Completion{
						Completion: agentResultToCompletion(ev),
					},
				}); err != nil {
					return err
				}
				// Drain the orchestrator's Complete event so the
				// ExecuteBatch goroutine cleans up its registration
				// before this RPC returns.
				s.drainUntilComplete(ctx, progress)
				return nil
			}
		}
	}
}

// BatchExecuteCommand dispatches one command to the set of agents that
// match the Target. Emits:
//
//	{batch_job_id} → AGENT_START* → AGENT_COMPLETE | AGENT_FAILED*
//	             → BatchTerminal
//
// BatchAgentOutput chunks are reserved for v1.x (see V1X-BACKLOG); the
// v1.0 agent returns stdout/stderr buffered in CommandResponse and
// the bridge into the batch_agent_results row is task 12.
func (s *GRPCServer) BatchExecuteCommand(
	req *v1.BatchExecuteCommandRequest,
	stream grpc.ServerStreamingServer[v1.BatchExecuteCommandResponse],
) error {
	if req.GetCommand() == "" {
		return status.Error(codes.InvalidArgument, "command is required")
	}
	if s.executor == nil {
		return status.Error(codes.Unavailable, "control plane: BatchExecutor not wired (task 9d / 12)")
	}

	ctx := stream.Context()
	target := Target{
		AgentIDs:        req.GetTarget().GetAgentIds(),
		Labels:          req.GetTarget().GetLabels(),
		HostnamePattern: req.GetTarget().GetHostnamePattern(),
	}
	agents, err := ResolveTarget(ctx, s.store, target)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "resolve target: %v", err)
	}
	if len(agents) == 0 {
		// Zero matches is a clean Completed batch-of-zero — emit a
		// synthetic terminal so clients always see a stream close
		// rather than an error.
		return stream.Send(&v1.BatchExecuteCommandResponse{
			Event: &v1.BatchExecuteCommandResponse_Terminal{
				Terminal: &v1.BatchTerminal{
					Status: v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED,
					At:     timestamppb.Now(),
				},
			},
		})
	}

	agentIDs := make([]string, 0, len(agents))
	for _, a := range agents {
		agentIDs = append(agentIDs, a.ID)
	}

	events := make(chan batchEvent, 100) // §4.7 buffer = 100
	dec := &streamingDecorator{inner: s.executor, emit: nonBlockingEmit(ctx, events)}
	progress := make(chan BatchProgressEvent, 8)

	batchReq := BatchRequest{
		Command:     req.GetCommand(),
		Args:        req.GetArgs(),
		Concurrency: int(req.GetConcurrency()),
	}
	batchID, err := s.dispatcher.ExecuteBatch(ctx, batchReq, agentIDs, dec, progress)
	if err != nil {
		return status.Errorf(codes.Internal, "execute batch: %v", err)
	}

	if err := stream.Send(&v1.BatchExecuteCommandResponse{
		Event: &v1.BatchExecuteCommandResponse_BatchJobId{BatchJobId: batchID},
	}); err != nil {
		return err
	}

	return s.pumpBatchStream(stream, events, progress)
}

// pumpBatchStream translates events from the executor decorator and
// progress events from the orchestrator into the stream. Returns when
// the orchestrator's Complete event arrives (after draining remaining
// agent events) or when the context fires.
func (s *GRPCServer) pumpBatchStream(
	stream grpc.ServerStreamingServer[v1.BatchExecuteCommandResponse],
	events <-chan batchEvent,
	progress <-chan BatchProgressEvent,
) error {
	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-events:
			if err := sendBatchEvent(stream, ev); err != nil {
				return err
			}
		case prog, ok := <-progress:
			if !ok {
				return nil
			}
			if prog.Phase != BatchProgressPhaseComplete {
				continue
			}
			// Drain remaining agent events before the terminal.
			for {
				select {
				case ev := <-events:
					if err := sendBatchEvent(stream, ev); err != nil {
						return err
					}
				default:
					return stream.Send(&v1.BatchExecuteCommandResponse{
						Event: &v1.BatchExecuteCommandResponse_Terminal{
							Terminal: &v1.BatchTerminal{
								Status: batchStatusToProto(prog.Status),
								At:     timestamppb.Now(),
							},
						},
					})
				}
			}
		}
	}
}

func (s *GRPCServer) drainUntilComplete(ctx context.Context, progress <-chan BatchProgressEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case prog, ok := <-progress:
			if !ok || prog.Phase == BatchProgressPhaseComplete {
				return
			}
		}
	}
}

// ---- streaming decorator + event plumbing ----------------------------------

type batchEventKind int

const (
	eventKindStarted batchEventKind = iota
	eventKindCompleted
	eventKindFailed
)

type batchEvent struct {
	kind    batchEventKind
	agentID string
	result  state.BatchAgentResultRecord
	err     error
	at      time.Time
}

// streamingDecorator wraps a BatchExecutor and emits before/after
// events for the gRPC layer to translate into AGENT_START /
// AGENT_COMPLETE / AGENT_FAILED messages.
type streamingDecorator struct {
	inner BatchExecutor
	emit  func(batchEvent)
}

func (d *streamingDecorator) Execute(
	ctx context.Context, batchID, agentID, command string, args []string,
) (state.BatchAgentResultRecord, error) {
	d.emit(batchEvent{kind: eventKindStarted, agentID: agentID, at: time.Now()})
	res, err := d.inner.Execute(ctx, batchID, agentID, command, args)
	switch {
	case err != nil:
		d.emit(batchEvent{kind: eventKindFailed, agentID: agentID, result: res, err: err, at: time.Now()})
	case !res.Success:
		d.emit(batchEvent{kind: eventKindFailed, agentID: agentID, result: res, at: time.Now()})
	default:
		d.emit(batchEvent{kind: eventKindCompleted, agentID: agentID, result: res, at: time.Now()})
	}
	return res, err
}

// nonBlockingEmit returns an emit fn that drops events when the
// channel is full or ctx is done (back-pressure boundary).
func nonBlockingEmit(ctx context.Context, ch chan<- batchEvent) func(batchEvent) {
	return func(ev batchEvent) {
		select {
		case ch <- ev:
		case <-ctx.Done():
		default:
		}
	}
}

// sendBatchEvent translates one batchEvent into the wire format and
// sends it on the gRPC stream.
func sendBatchEvent(
	stream grpc.ServerStreamingServer[v1.BatchExecuteCommandResponse],
	ev batchEvent,
) error {
	at := timestamppb.New(ev.at)
	switch ev.kind {
	case eventKindStarted:
		return stream.Send(&v1.BatchExecuteCommandResponse{
			Event: &v1.BatchExecuteCommandResponse_Lifecycle{
				Lifecycle: &v1.BatchAgentLifecycle{
					AgentId: ev.agentID,
					Kind:    v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_START,
					At:      at,
				},
			},
		})
	case eventKindCompleted:
		return stream.Send(&v1.BatchExecuteCommandResponse{
			Event: &v1.BatchExecuteCommandResponse_Lifecycle{
				Lifecycle: &v1.BatchAgentLifecycle{
					AgentId: ev.agentID,
					Kind:    v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_COMPLETE,
					At:      at,
				},
			},
		})
	case eventKindFailed:
		msg := ev.result.Error
		if ev.err != nil {
			msg = ev.err.Error()
		}
		return stream.Send(&v1.BatchExecuteCommandResponse{
			Event: &v1.BatchExecuteCommandResponse_Lifecycle{
				Lifecycle: &v1.BatchAgentLifecycle{
					AgentId: ev.agentID,
					Kind:    v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_FAILED,
					At:      at,
					Error:   msg,
				},
			},
		})
	}
	return fmt.Errorf("controlplane: unknown batch event kind %d", ev.kind)
}

// agentResultToCompletion translates the single-agent terminal event
// into the ExecuteCommand stream's CommandCompletion.
func agentResultToCompletion(ev batchEvent) *v1.CommandCompletion {
	completedAt := ev.at
	if !ev.result.CompletedAt.IsZero() {
		completedAt = ev.result.CompletedAt
	}
	if ev.kind == eventKindCompleted {
		return &v1.CommandCompletion{
			Status:      v1.CommandStatus_COMMAND_STATUS_COMPLETED,
			ExitCode:    int32(ev.result.ExitCode),
			CompletedAt: timestamppb.New(completedAt),
		}
	}
	msg := ev.result.Error
	if ev.err != nil {
		msg = ev.err.Error()
	}
	return &v1.CommandCompletion{
		Status:      v1.CommandStatus_COMMAND_STATUS_FAILED,
		ExitCode:    int32(ev.result.ExitCode),
		CompletedAt: timestamppb.New(completedAt),
		Error:       msg,
	}
}

func batchStatusToProto(s state.BatchJobStatus) v1.BatchJobStatus {
	switch s {
	case state.BatchJobStatusPending:
		return v1.BatchJobStatus_BATCH_JOB_STATUS_PENDING
	case state.BatchJobStatusRunning:
		return v1.BatchJobStatus_BATCH_JOB_STATUS_RUNNING
	case state.BatchJobStatusCompleted:
		return v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED
	case state.BatchJobStatusFailed:
		return v1.BatchJobStatus_BATCH_JOB_STATUS_FAILED
	case state.BatchJobStatusPartial:
		return v1.BatchJobStatus_BATCH_JOB_STATUS_PARTIAL
	case state.BatchJobStatusCancelled:
		return v1.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED
	default:
		return v1.BatchJobStatus_BATCH_JOB_STATUS_UNSPECIFIED
	}
}
