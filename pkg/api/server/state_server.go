package server

import (
	"context"
	"fmt"
	"os"
	"time"

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

// StateHistoryStore provides persistent state history and status storage.
type StateHistoryStore interface {
	SaveRun(ctx context.Context, run *StateHistoryRun) error
	ListRuns(ctx context.Context, filter *StateHistoryFilter) ([]*StateHistoryRun, string, error)
	GetStatus(ctx context.Context, agentID, statePath string) ([]*StateStatusEntry, error)
}

// StateHistoryRun represents a historical state run for the server layer.
type StateHistoryRun struct {
	RunID         string
	AgentID       string
	StateFiles    []string
	Target        string
	DryRun        bool
	Success       bool
	Total         int
	Succeeded     int
	Failed        int
	Changed       int
	Unchanged     int
	StartTime     time.Time
	EndTime       time.Time
	DurationMs    int64
	CorrelationID string
	User          string
}

// StateHistoryFilter specifies filters for listing state runs.
type StateHistoryFilter struct {
	AgentID   string
	StatePath string
	Success   *bool
	StartTime *time.Time
	EndTime   *time.Time
	PageSize  int
	PageToken string
}

// StateStatusEntry represents current state status for an agent.
type StateStatusEntry struct {
	AgentID      string
	StateID      string
	Module       string
	CurrentState string
	DesiredState string
	Compliant    bool
	LastApplied  time.Time
	LastChecked  time.Time
}

// StateServer implements the StateService gRPC server.
type StateServer struct {
	pb.UnimplementedStateServiceServer
	executor     StateExecutor
	driftChecker StateDriftChecker
	historyStore StateHistoryStore
}

// NewStateServer creates a new StateServer.
// Any dependency may be nil — RPCs return codes.Unavailable if the required dep is nil.
func NewStateServer(executor StateExecutor, driftChecker StateDriftChecker) *StateServer {
	return &StateServer{
		executor:     executor,
		driftChecker: driftChecker,
	}
}

// SetHistoryStore sets the state history store for GetStateHistory and GetStateStatus RPCs.
func (s *StateServer) SetHistoryStore(store StateHistoryStore) {
	s.historyStore = store
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
	if s.historyStore == nil {
		return nil, status.Error(codes.Unavailable, "state history store not available")
	}

	filter := &StateHistoryFilter{
		AgentID:   req.AgentId,
		StatePath: req.StatePath,
		PageSize:  int(req.PageSize),
		PageToken: req.PageToken,
	}
	if req.Success != nil {
		v := req.Success.Value
		filter.Success = &v
	}
	if req.StartTime != nil {
		t := req.StartTime.AsTime()
		filter.StartTime = &t
	}
	if req.EndTime != nil {
		t := req.EndTime.AsTime()
		filter.EndTime = &t
	}

	runs, nextToken, err := s.historyStore.ListRuns(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list state history: %v", err)
	}

	resp := &pb.GetStateHistoryResponse{
		NextPageToken: nextToken,
		TotalCount:    int32(len(runs)), //nolint:gosec // G115: bounded by page size
	}

	for _, run := range runs {
		pbRun := &pb.StateRun{
			RunId:         run.RunID,
			StateFiles:    run.StateFiles,
			Target:        run.Target,
			DryRun:        run.DryRun,
			CorrelationId: run.CorrelationID,
			User:          run.User,
		}
		if !run.StartTime.IsZero() {
			pbRun.StartTime = timestamppb.New(run.StartTime)
		}
		if !run.EndTime.IsZero() {
			pbRun.EndTime = timestamppb.New(run.EndTime)
		}
		pbRun.Summary = &pb.StateRunSummary{
			RunId:      run.RunID,
			Total:      int32(run.Total),      //nolint:gosec // G115: bounded by state count
			Succeeded:  int32(run.Succeeded),  //nolint:gosec // G115: bounded by state count
			Failed:     int32(run.Failed),     //nolint:gosec // G115: bounded by state count
			Changed:    int32(run.Changed),    //nolint:gosec // G115: bounded by state count
			Unchanged:  int32(run.Unchanged),  //nolint:gosec // G115: bounded by state count
			Success:    run.Success,
			DurationMs: run.DurationMs,
		}
		resp.Runs = append(resp.Runs, pbRun)
	}

	return resp, nil
}

// GetStateStatus retrieves current state status for an agent.
func (s *StateServer) GetStateStatus(ctx context.Context, req *pb.GetStateStatusRequest) (*pb.GetStateStatusResponse, error) {
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}
	if s.historyStore == nil {
		return nil, status.Error(codes.Unavailable, "state history store not available")
	}

	statuses, err := s.historyStore.GetStatus(ctx, req.AgentId, req.StatePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get state status: %v", err)
	}

	resp := &pb.GetStateStatusResponse{
		AgentId: req.AgentId,
	}

	var latestChecked time.Time
	for _, s := range statuses {
		entry := &pb.CurrentStateStatus{
			StateId:      s.StateID,
			Module:       s.Module,
			CurrentState: s.CurrentState,
			DesiredState: s.DesiredState,
			Compliant:    s.Compliant,
		}
		if !s.LastApplied.IsZero() {
			entry.LastApplied = timestamppb.New(s.LastApplied)
		}
		if !s.LastChecked.IsZero() {
			entry.LastChecked = timestamppb.New(s.LastChecked)
			if s.LastChecked.After(latestChecked) {
				latestChecked = s.LastChecked
			}
		}
		resp.States = append(resp.States, entry)
	}

	if !latestChecked.IsZero() {
		resp.LastChecked = timestamppb.New(latestChecked)
	}

	return resp, nil
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
