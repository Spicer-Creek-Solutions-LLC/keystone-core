package controlplane

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	rb "go.keystone-core.io/keystone-core/internal/runbook"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// Runbook provider interfaces — independently nilable; an RPC whose
// provider is missing returns codes.Unavailable. Boot wiring is the
// deferred server composition (see ROADMAP "Durable runbook
// execution store").
type (
	runbookCatalog interface {
		List(ctx context.Context) ([]*rb.Runbook, error)
		Get(ctx context.Context, id string) (*rb.Runbook, error)
	}
	runbookRunner interface {
		Execute(ctx context.Context, id string, inputs map[string]any) (*rb.Execution, error)
	}
	runbookExecStore interface {
		Get(ctx context.Context, id string) (*rb.Execution, error)
	}
)

// RunbookGRPCServer implements v1.RunbookServiceServer (Epic 15
// task 12) — minimal query + execute + execution status.
type RunbookGRPCServer struct {
	v1.UnimplementedRunbookServiceServer

	Catalog runbookCatalog
	Runner  runbookRunner
	Store   runbookExecStore
}

func runbookToProto(r *rb.Runbook) *v1.RunbookSummary {
	steps := make([]string, 0, len(r.Spec.Steps))
	for _, st := range r.Spec.Steps {
		steps = append(steps, st.Name)
	}
	return &v1.RunbookSummary{
		Name:      r.Metadata.Name,
		Namespace: r.Metadata.Namespace,
		Version:   r.Metadata.Version,
		Steps:     steps,
	}
}

func executionToProto(e *rb.Execution) *v1.RunbookExecutionView {
	steps := make([]*v1.RunbookStepView, 0, len(e.Steps))
	for _, s := range e.Steps {
		sv := &v1.RunbookStepView{
			Name:     s.Name,
			Type:     s.Type,
			Status:   string(s.Status),
			Attempts: int32(s.Attempts),
		}
		if s.Error != nil {
			sv.Error = s.Error.Error()
		}
		steps = append(steps, sv)
	}
	view := &v1.RunbookExecutionView{
		Id:      e.ID,
		Runbook: e.Runbook,
		Status:  string(e.Status),
		Steps:   steps,
	}
	if e.Error != nil {
		view.Error = e.Error.Error()
	}
	return view
}

// ListRunbooks returns every runbook's summary.
func (s *RunbookGRPCServer) ListRunbooks(ctx context.Context, _ *v1.ListRunbooksRequest) (*v1.ListRunbooksResponse, error) {
	if s.Catalog == nil {
		return nil, status.Error(codes.Unavailable, "runbook: catalog not wired")
	}
	rbs, err := s.Catalog.List(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	out := make([]*v1.RunbookSummary, 0, len(rbs))
	for _, r := range rbs {
		out = append(out, runbookToProto(r))
	}
	return &v1.ListRunbooksResponse{Runbooks: out, TotalCount: int32(len(out))}, nil
}

// GetRunbook returns one runbook's summary.
func (s *RunbookGRPCServer) GetRunbook(ctx context.Context, req *v1.GetRunbookRequest) (*v1.GetRunbookResponse, error) {
	if s.Catalog == nil {
		return nil, status.Error(codes.Unavailable, "runbook: catalog not wired")
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	r, err := s.Catalog.Get(ctx, req.GetId())
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	return &v1.GetRunbookResponse{Runbook: runbookToProto(r)}, nil
}

// ExecuteRunbook runs a runbook. A run that completes but ends failed
// is returned with status="failed" (not a gRPC error); only a setup
// error with no execution is codes.Internal.
func (s *RunbookGRPCServer) ExecuteRunbook(ctx context.Context, req *v1.ExecuteRunbookRequest) (*v1.ExecuteRunbookResponse, error) {
	if s.Runner == nil {
		return nil, status.Error(codes.Unavailable, "runbook: runner not wired")
	}
	if req.GetRunbook() == "" {
		return nil, status.Error(codes.InvalidArgument, "runbook is required")
	}
	inputs := make(map[string]any, len(req.GetInputs()))
	for k, v := range req.GetInputs() {
		inputs[k] = v
	}
	exec, err := s.Runner.Execute(ctx, req.GetRunbook(), inputs)
	if exec == nil {
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return nil, status.Error(codes.Internal, "runbook: execute returned no execution")
	}
	if err != nil && !errors.Is(err, rb.ErrExecutionFailed) {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &v1.ExecuteRunbookResponse{Execution: executionToProto(exec)}, nil
}

// GetRunbookExecution fetches a recorded execution.
func (s *RunbookGRPCServer) GetRunbookExecution(ctx context.Context, req *v1.GetRunbookExecutionRequest) (*v1.GetRunbookExecutionResponse, error) {
	if s.Store == nil {
		return nil, status.Error(codes.Unavailable, "runbook: execution store not wired")
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	e, err := s.Store.Get(ctx, req.GetId())
	if err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	return &v1.GetRunbookExecutionResponse{Execution: executionToProto(e)}, nil
}
