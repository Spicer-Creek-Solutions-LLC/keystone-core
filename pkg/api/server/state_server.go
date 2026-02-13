package server

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/shawnbutts/keystone-core/internal/statemgmt"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// StateExecutor executes state declarations.
type StateExecutor interface {
	ExecuteState(ctx context.Context, stateFile *statemgmt.StateFile) (*statemgmt.StateRun, error)
}

// StateDriftChecker detects configuration drift.
type StateDriftChecker interface {
	CheckDrift(stateFile *statemgmt.StateFile) (*statemgmt.DriftReport, error)
}

// StateServer implements the StateService gRPC server.
type StateServer struct {
	pb.UnimplementedStateServiceServer
	executor     StateExecutor
	driftChecker StateDriftChecker
}

// NewStateServer creates a new StateServer.
// Any dependency may be nil — RPCs return codes.Unavailable if the required dep is nil.
func NewStateServer(executor StateExecutor, driftChecker StateDriftChecker) *StateServer {
	return &StateServer{
		executor:     executor,
		driftChecker: driftChecker,
	}
}

// ApplyState applies state declarations and streams progress.
func (s *StateServer) ApplyState(req *pb.ApplyStateRequest, stream grpc.ServerStreamingServer[pb.ApplyStateResponse]) error {
	if s.executor == nil {
		return status.Error(codes.Unavailable, "state executor not available")
	}

	stateFile, err := loadStateFromRequest(req.StateContent, req.StatePath)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "failed to load state: %v", err)
	}

	// Send run start
	if err := stream.Send(&pb.ApplyStateResponse{
		Type:      pb.StateResponseType_STATE_RESPONSE_TYPE_RUN_START,
		Timestamp: timestamppb.Now(),
	}); err != nil {
		return err
	}

	// Create executor with dry-run setting
	exec, ok := s.executor.(*statemgmt.Executor)
	if ok && req.DryRun {
		exec.DryRun = true
		defer func() { exec.DryRun = false }()
	}

	run, execErr := s.executor.ExecuteState(stream.Context(), stateFile)
	if run == nil {
		return status.Errorf(codes.Internal, "state execution failed: %v", execErr)
	}

	// Stream individual results
	for _, result := range run.Results {
		resp := &pb.ApplyStateResponse{
			RunId: run.RunID,
			Type:  pb.StateResponseType_STATE_RESPONSE_TYPE_STATE_RESULT,
			StateResult: &pb.StateResult{
				StateId:    result.StateID,
				Module:     result.Module,
				Success:    result.Success,
				Changed:    result.Changed,
				Comment:    result.Comment,
				DurationMs: result.Duration.Milliseconds(),
				Timestamp:  timestamppb.Now(),
			},
			Timestamp: timestamppb.Now(),
		}
		if result.Error != nil {
			resp.StateResult.Error = result.Error.Error()
		}
		if result.Changes != nil {
			resp.StateResult.Changes = make(map[string]string, len(result.Changes))
			for k, v := range result.Changes {
				resp.StateResult.Changes[k] = fmt.Sprintf("%v", v)
			}
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}

	// Send run summary
	summaryResp := &pb.ApplyStateResponse{
		RunId:     run.RunID,
		Type:      pb.StateResponseType_STATE_RESPONSE_TYPE_RUN_COMPLETE,
		Timestamp: timestamppb.Now(),
	}
	if run.Summary != nil {
		summaryResp.Summary = &pb.StateRunSummary{
			RunId:      run.RunID,
			Total:      int32(run.Summary.Total),    //nolint:gosec // G115: bounded by state count
			Succeeded:  int32(run.Summary.Succeeded), //nolint:gosec // G115: bounded by state count
			Failed:     int32(run.Summary.Failed),    //nolint:gosec // G115: bounded by state count
			Changed:    int32(run.Summary.Changed),   //nolint:gosec // G115: bounded by state count
			Unchanged:  int32(run.Summary.Unchanged), //nolint:gosec // G115: bounded by state count
			Success:    run.Summary.Success,
			DurationMs: run.Summary.Duration.Milliseconds(),
		}
		if !run.StartTime.IsZero() {
			summaryResp.Summary.StartTime = timestamppb.New(run.StartTime)
		}
		if !run.EndTime.IsZero() {
			summaryResp.Summary.EndTime = timestamppb.New(run.EndTime)
		}
	}
	if execErr != nil {
		summaryResp.Type = pb.StateResponseType_STATE_RESPONSE_TYPE_RUN_FAILED
		summaryResp.Error = execErr.Error()
	}
	return stream.Send(summaryResp)
}

// CheckState checks state without applying (dry-run mode).
func (s *StateServer) CheckState(ctx context.Context, req *pb.CheckStateRequest) (*pb.CheckStateResponse, error) {
	if s.executor == nil {
		return nil, status.Error(codes.Unavailable, "state executor not available")
	}

	stateFile, err := loadStateFromRequest(req.StateContent, req.StatePath)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to load state: %v", err)
	}

	// Validate the state file
	validator := statemgmt.NewValidator()
	result := validator.Validate(stateFile)

	// Also do a dry-run execution
	exec, ok := s.executor.(*statemgmt.Executor)
	if ok {
		exec.DryRun = true
		defer func() { exec.DryRun = false }()
	}

	run, _ := s.executor.ExecuteState(ctx, stateFile)

	resp := &pb.CheckStateResponse{
		Success: result.Valid,
	}

	if run != nil {
		resp.RunId = run.RunID

		agentResult := &pb.AgentCheckResult{
			Success: true,
		}
		for _, r := range run.Results {
			checkResult := &pb.StateCheckResult{
				StateId:     r.StateID,
				Module:      r.Module,
				WouldChange: r.Changed,
				WouldFail:   !r.Success,
				Comment:     r.Comment,
			}
			if r.Changes != nil {
				checkResult.ProposedChanges = make(map[string]string, len(r.Changes))
				for k, v := range r.Changes {
					checkResult.ProposedChanges[k] = fmt.Sprintf("%v", v)
				}
			}
			agentResult.States = append(agentResult.States, checkResult)
			agentResult.Total++
			if r.Changed {
				agentResult.WouldChange++
				resp.WouldChange++
			}
			if !r.Success {
				agentResult.WouldFail++
				agentResult.Success = false
				resp.WouldFail++
			}
		}

		resp.AgentResults = append(resp.AgentResults, agentResult)
	}

	return resp, nil
}

// DetectDrift detects configuration drift from desired state.
func (s *StateServer) DetectDrift(ctx context.Context, req *pb.DetectDriftRequest) (*pb.DetectDriftResponse, error) {
	if s.driftChecker == nil {
		return nil, status.Error(codes.Unavailable, "drift checker not available")
	}

	stateFile, err := loadStateFromRequest(req.StateContent, req.StatePath)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to load state: %v", err)
	}

	report, err := s.driftChecker.CheckDrift(stateFile)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "drift detection failed: %v", err)
	}

	resp := &pb.DetectDriftResponse{
		RunId:      report.RunID,
		CheckedAt:  timestamppb.New(report.CheckedAt),
		DurationMs: report.Duration.Milliseconds(),
	}

	// Convert drift statuses
	agentResult := &pb.AgentDriftResult{}
	for _, ds := range report.States {
		driftStatus := &pb.DriftStatus{
			StateId:   ds.StateID,
			Module:    ds.Module,
			HasDrift:  ds.HasDrift,
			Severity:  driftSeverityToProto(ds.Severity),
			CheckedAt: timestamppb.New(ds.CheckedAt),
		}
		for _, diff := range ds.Differences {
			driftStatus.Differences = append(driftStatus.Differences, &pb.DriftDifference{
				Path:     diff.Path,
				Desired:  fmt.Sprintf("%v", diff.Desired),
				Actual:   fmt.Sprintf("%v", diff.Actual),
				Severity: driftSeverityToProto(diff.Severity),
				Message:  diff.Message,
			})
		}
		agentResult.DriftStatuses = append(agentResult.DriftStatuses, driftStatus)
		if ds.HasDrift {
			agentResult.HasDrift = true
			if driftSeverityToProto(ds.Severity) > agentResult.MaxSeverity {
				agentResult.MaxSeverity = driftSeverityToProto(ds.Severity)
			}
		}
	}
	resp.AgentResults = append(resp.AgentResults, agentResult)

	// Convert summary
	if report.Summary != nil {
		resp.Summary = &pb.DriftSummary{
			Total:        int32(report.Summary.Total),         //nolint:gosec // G115: bounded by state count
			NoDrift:      int32(report.Summary.NoDrift),       //nolint:gosec // G115: bounded by state count
			LowDrift:     int32(report.Summary.LowDrift),      //nolint:gosec // G115: bounded by state count
			MediumDrift:  int32(report.Summary.MediumDrift),   //nolint:gosec // G115: bounded by state count
			HighDrift:    int32(report.Summary.HighDrift),      //nolint:gosec // G115: bounded by state count
			CriticalDrift: int32(report.Summary.CriticalDrift), //nolint:gosec // G115: bounded by state count
			HasDrift:     report.Summary.Total > report.Summary.NoDrift,
			MaxSeverity:  driftSeverityToProto(report.Summary.OverallSeverity),
		}
	}

	return resp, nil
}

// GetStateHistory retrieves state application history.
func (s *StateServer) GetStateHistory(ctx context.Context, req *pb.GetStateHistoryRequest) (*pb.GetStateHistoryResponse, error) {
	// State history requires a persistent store that tracks runs over time.
	// This is not yet backed by a store — will be wired in a future phase.
	return nil, status.Error(codes.Unimplemented, "state history store not yet available")
}

// GetStateStatus retrieves current state status for an agent.
func (s *StateServer) GetStateStatus(ctx context.Context, req *pb.GetStateStatusRequest) (*pb.GetStateStatusResponse, error) {
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}

	// State status requires a persistent store that tracks per-agent state.
	// This is not yet backed by a store — will be wired in a future phase.
	return nil, status.Error(codes.Unimplemented, "state status store not yet available")
}

// loadStateFromRequest loads a StateFile from request content or path.
func loadStateFromRequest(content, path string) (*statemgmt.StateFile, error) {
	parser := statemgmt.NewParser(".")

	if content != "" {
		tmpFile, err := os.CreateTemp("", "kscore-state-*.yaml")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		if _, err := tmpFile.WriteString(content); err != nil {
			return nil, fmt.Errorf("failed to write temp file: %w", err)
		}
		tmpFile.Close()

		return parser.ParseFile(tmpFile.Name())
	}

	if path != "" {
		return parser.ParseFile(path)
	}

	return nil, fmt.Errorf("either state_content or state_path is required")
}

// driftSeverityToProto converts internal DriftSeverity to proto enum.
func driftSeverityToProto(severity statemgmt.DriftSeverity) pb.DriftSeverity {
	switch severity {
	case statemgmt.DriftNone:
		return pb.DriftSeverity_DRIFT_SEVERITY_NONE
	case statemgmt.DriftLow:
		return pb.DriftSeverity_DRIFT_SEVERITY_LOW
	case statemgmt.DriftMedium:
		return pb.DriftSeverity_DRIFT_SEVERITY_MEDIUM
	case statemgmt.DriftHigh:
		return pb.DriftSeverity_DRIFT_SEVERITY_HIGH
	case statemgmt.DriftCritical:
		return pb.DriftSeverity_DRIFT_SEVERITY_CRITICAL
	default:
		return pb.DriftSeverity_DRIFT_SEVERITY_UNSPECIFIED
	}
}

// Ensure StateServer satisfies the interface at compile time.
var _ pb.StateServiceServer = (*StateServer)(nil)

// Ensure Executor satisfies StateExecutor at compile time.
var _ StateExecutor = (*statemgmt.Executor)(nil)

// Ensure StateDiffer satisfies StateDriftChecker at compile time.
var _ StateDriftChecker = (*statemgmt.StateDiffer)(nil)
