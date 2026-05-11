package controlplane

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"

	"go.keystone-core.io/keystone-core/internal/state"
)

// GetBatchJob returns the persisted state of a batch.
func (s *GRPCServer) GetBatchJob(ctx context.Context, req *v1.GetBatchJobRequest) (*v1.GetBatchJobResponse, error) {
	if req.GetBatchJobId() == "" {
		return nil, status.Error(codes.InvalidArgument, "batch_job_id is required")
	}
	rec, err := s.dispatcher.GetBatch(ctx, req.GetBatchJobId())
	if err != nil {
		if errors.Is(err, ErrBatchNotFound) {
			return nil, status.Errorf(codes.NotFound, "batch %q not found", req.GetBatchJobId())
		}
		return nil, status.Errorf(codes.Internal, "get batch: %v", err)
	}
	return &v1.GetBatchJobResponse{Batch: batchJobToProto(rec)}, nil
}

// ListBatchJobs paginates batches with optional filters. v1.0 uses
// offset/limit semantics — cursor-based paging is v1.x.
func (s *GRPCServer) ListBatchJobs(ctx context.Context, req *v1.ListBatchJobsRequest) (*v1.ListBatchJobsResponse, error) {
	filter := state.BatchJobFilter{
		Limit:  int(req.GetLimit()),
		Offset: int(req.GetOffset()),
	}
	if st := req.GetStatus(); st != v1.BatchJobStatus_BATCH_JOB_STATUS_UNSPECIFIED {
		filter.Status = batchStatusFromProto(st)
	}
	if t := req.GetSince(); t != nil {
		filter.Since = t.AsTime()
	}
	if t := req.GetUntil(); t != nil {
		filter.Until = t.AsTime()
	}
	recs, err := s.dispatcher.ListBatches(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list batches: %v", err)
	}
	out := &v1.ListBatchJobsResponse{Batches: make([]*v1.BatchJob, 0, len(recs))}
	for _, r := range recs {
		out.Batches = append(out.Batches, batchJobToProto(r))
	}
	return out, nil
}

// CancelBatchJob signals an in-flight batch to stop. Returns
// FailedPrecondition when the batch is already finalized.
func (s *GRPCServer) CancelBatchJob(ctx context.Context, req *v1.CancelBatchJobRequest) (*v1.CancelBatchJobResponse, error) {
	if req.GetBatchJobId() == "" {
		return nil, status.Error(codes.InvalidArgument, "batch_job_id is required")
	}
	err := s.dispatcher.Cancel(ctx, req.GetBatchJobId())
	switch {
	case err == nil:
		return &v1.CancelBatchJobResponse{}, nil
	case errors.Is(err, ErrBatchNotFound):
		return nil, status.Errorf(codes.NotFound, "batch %q not found", req.GetBatchJobId())
	case errors.Is(err, ErrBatchFinalized):
		return nil, status.Errorf(codes.FailedPrecondition, "batch %q already finalized", req.GetBatchJobId())
	default:
		return nil, status.Errorf(codes.Internal, "cancel batch: %v", err)
	}
}

// ListBatchAgentResults returns every per-agent result for a batch.
func (s *GRPCServer) ListBatchAgentResults(ctx context.Context, req *v1.ListBatchAgentResultsRequest) (*v1.ListBatchAgentResultsResponse, error) {
	if req.GetBatchJobId() == "" {
		return nil, status.Error(codes.InvalidArgument, "batch_job_id is required")
	}
	recs, err := s.dispatcher.ListAgentResults(ctx, req.GetBatchJobId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list agent results: %v", err)
	}
	out := &v1.ListBatchAgentResultsResponse{Results: make([]*v1.BatchAgentResult, 0, len(recs))}
	for _, r := range recs {
		out.Results = append(out.Results, batchAgentResultToProto(r))
	}
	return out, nil
}

// GetBatchAgentResult fetches a single (batch, agent) result row.
func (s *GRPCServer) GetBatchAgentResult(ctx context.Context, req *v1.GetBatchAgentResultRequest) (*v1.GetBatchAgentResultResponse, error) {
	if req.GetBatchJobId() == "" || req.GetAgentId() == "" {
		return nil, status.Error(codes.InvalidArgument, "batch_job_id and agent_id are required")
	}
	rec, err := s.dispatcher.GetAgentResult(ctx, req.GetBatchJobId(), req.GetAgentId())
	if err != nil {
		if errors.Is(err, ErrBatchNotFound) {
			return nil, status.Errorf(codes.NotFound, "result for (%q, %q) not found",
				req.GetBatchJobId(), req.GetAgentId())
		}
		return nil, status.Errorf(codes.Internal, "get agent result: %v", err)
	}
	return &v1.GetBatchAgentResultResponse{Result: batchAgentResultToProto(rec)}, nil
}

// ---- proto / record converters --------------------------------------------

func batchJobToProto(r *state.BatchJobRecord) *v1.BatchJob {
	if r == nil {
		return nil
	}
	out := &v1.BatchJob{
		Id:               r.ID,
		Command:          r.Command,
		Args:             r.Args,
		Status:           batchStatusToProto(r.Status),
		Concurrency:      int32(r.Concurrency),
		TotalAgents:      int32(r.TotalAgents),
		CompletedAgents:  int32(r.CompletedAgents),
		SuccessfulAgents: int32(r.SuccessfulAgents),
		FailedAgents:     int32(r.FailedAgents),
		CreatedAt:        timestampOrNil(r.CreatedAt),
		StartedAt:        timestampOrNil(r.StartedAt),
		CompletedAt:      timestampOrNil(r.CompletedAt),
	}
	// Target on state.BatchJobRecord is an opaque map[string]any.
	// Surface only the three fields the gRPC Target type carries; the
	// rich form lands with the V1X server-side expression compile.
	out.Target = targetMapToProto(r.Target)
	return out
}

func targetMapToProto(m map[string]any) *v1.Target {
	if m == nil {
		return nil
	}
	t := &v1.Target{}
	if v, ok := m["agent_ids"].([]string); ok {
		t.AgentIds = append([]string(nil), v...)
	}
	if v, ok := m["labels"].(map[string]string); ok {
		t.Labels = copyStringMap(v)
	}
	if v, ok := m["hostname_pattern"].(string); ok {
		t.HostnamePattern = v
	}
	return t
}

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func batchAgentResultToProto(r *state.BatchAgentResultRecord) *v1.BatchAgentResult {
	if r == nil {
		return nil
	}
	return &v1.BatchAgentResult{
		BatchJobId:      r.BatchJobID,
		AgentId:         r.AgentID,
		Success:         r.Success,
		ExitCode:        int32(r.ExitCode),
		Error:           r.Error,
		Stdout:          r.Stdout,
		Stderr:          r.Stderr,
		StdoutTruncated: r.StdoutTruncated,
		StderrTruncated: r.StderrTruncated,
		StartedAt:       timestampOrNil(r.StartedAt),
		CompletedAt:     timestampOrNil(r.CompletedAt),
	}
}

func batchStatusFromProto(s v1.BatchJobStatus) state.BatchJobStatus {
	switch s {
	case v1.BatchJobStatus_BATCH_JOB_STATUS_PENDING:
		return state.BatchJobStatusPending
	case v1.BatchJobStatus_BATCH_JOB_STATUS_RUNNING:
		return state.BatchJobStatusRunning
	case v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED:
		return state.BatchJobStatusCompleted
	case v1.BatchJobStatus_BATCH_JOB_STATUS_FAILED:
		return state.BatchJobStatusFailed
	case v1.BatchJobStatus_BATCH_JOB_STATUS_PARTIAL:
		return state.BatchJobStatusPartial
	case v1.BatchJobStatus_BATCH_JOB_STATUS_CANCELLED:
		return state.BatchJobStatusCancelled
	default:
		return ""
	}
}

func timestampOrNil(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
