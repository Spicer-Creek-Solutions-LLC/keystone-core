// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// StateGRPCServer implements v1.StateServiceServer by composing the
// Epic 08 engine pieces with the StateHistoryStore. Each RPC runs
// the full Parse → Render → Validate → Resolve → Run/Check/Drift
// pipeline; results stream (for ApplyState) or roll up (for the
// unary RPCs) and persist along the way so the history store stays
// in lockstep with the wire output.
//
// The server holds engine factories rather than concrete instances:
// every request gets a fresh Renderer/Validator/Resolver/Runner so
// per-request state (variable overrides, facts) doesn't leak across
// callers.
type StateGRPCServer struct {
	v1.UnimplementedStateServiceServer

	Registry *statemgmt.Registry
	Store    state.StateHistoryStore
	// Clock returns "now" — defaults to time.Now in NewStateGRPCServer
	// but tests override it for deterministic timestamps.
	Clock func() time.Time

	// Auditor is the Epic 12 task 4 audit emission seam. When non-nil,
	// ApplyState wraps the per-run streamObserver with a
	// audit.StateApplyObserver so every applied declaration emits one
	// audit entry. Production wiring assigns auditRT.FanOut.
	Auditor audit.Auditor

	// Resolver selects the agents a targeted run applies to, using the
	// same targeting as batch exec rather than a second dialect.
	// Converge reaches them. Both nil means this server can only run
	// state on itself, and a targeted request is refused rather than
	// silently converging the control plane instead of the fleet.
	Resolver AgentTargetResolver
	Converge ConvergeFanout
	// ConvergeTimeout bounds ONE agent's run. Zero uses
	// defaultConvergeTimeout.
	ConvergeTimeout time.Duration
}

// NewStateGRPCServer returns a server backed by registry + store.
// Pass nil for registry to fall back to statemgmt.DefaultRegistry.
func NewStateGRPCServer(registry *statemgmt.Registry, store state.StateHistoryStore) *StateGRPCServer {
	return &StateGRPCServer{
		Registry: registry,
		Store:    store,
		Clock:    time.Now,
	}
}

// ApplyState compiles + applies in one RPC. The stream starts with a
// run_id event so the caller can persist the handle, then emits one
// decl_result per declaration as the runner progresses, then a
// terminal event. If dry_run is true, the server delegates to
// CheckState's path (no Apply) and the stream carries the resulting
// per-decl outcomes the same way.
func (s *StateGRPCServer) ApplyState(req *v1.ApplyStateRequest, stream grpc.ServerStreamingServer[v1.ApplyStateResponse]) error {
	if err := requireYAML(req.GetYamlContent()); err != nil {
		return err
	}

	// A targeted request converges AGENTS; the state file is compiled
	// on each of them, not here. Compiling centrally would render
	// `.Facts` against the control plane and then ship declarations
	// describing the wrong host, so the remote path deliberately never
	// touches s.compile.
	remoteAgents, err := s.resolveRemoteTargets(stream.Context(), req)
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if len(remoteAgents) > 0 {
		return s.applyStateRemote(stream, req, remoteAgents)
	}

	decls, err := s.compile(req.GetYamlContent(), toMap(req.GetVariableOverrides()), toMap(req.GetFacts()))
	if err != nil {
		return err
	}

	mode := state.StateRunModeApply
	if req.GetDryRun() {
		mode = state.StateRunModeCheck
	}
	runID := uuid.NewString()

	if err := s.openRun(stream.Context(), runID, mode, req.GetSource(), req.GetClusterId(), req.GetAgentId(), decls); err != nil {
		return err
	}

	// First stream event: run_id so the caller can persist the handle
	// and reference it later (history queries, rollback).
	if err := stream.Send(&v1.ApplyStateResponse{Event: &v1.ApplyStateResponse_RunId{RunId: runID}}); err != nil {
		return err
	}

	innerObs := &streamObserver{
		stream: stream,
		store:  s.Store,
		runID:  runID,
	}
	var obs statemgmt.RunObserver = innerObs
	if s.Auditor != nil {
		// Wrap the per-request stream observer with audit emission.
		// audit.StateApplyObserver chains through to innerObs on
		// every method so the streamed wire output is unaffected;
		// Done additionally emits one audit entry per declaration.
		obs = audit.NewStateApplyObserver(innerObs, s.Auditor)
	}

	runner := &statemgmt.Runner{Registry: s.resolveRegistry(), Observer: obs}

	var report *statemgmt.RunReport
	var runErr error
	if req.GetDryRun() {
		report, runErr = runner.Check(stream.Context(), decls)
	} else {
		report, runErr = runner.Run(stream.Context(), decls)
	}

	// Persist any observer write failures (the streamObserver's
	// AddStateRunResult errors land in obs.persistErrs). They don't
	// abort the RPC — the wire client sees the per-decl event
	// regardless — but they do show up in the run's error_message.
	finalErrMsg := obsErrors(innerObs, runErr)
	finalStatus := state.StateRunStatusCompleted
	if runErr != nil {
		finalStatus = state.StateRunStatusFailed
	}

	end := reportAggregatesToEnd(finalStatus, nowOr(s.now()), finalErrMsg, report)
	if err := s.Store.FinalizeStateRun(stream.Context(), runID, end); err != nil {
		// Persistence failure is logged via the error return; the
		// stream still gets its terminal event so the client knows
		// the run finished.
		_ = stream.Send(terminalEvent(runID, recordStatusToProto(finalStatus), reportAggregatesToProto(report), fmt.Sprintf("finalize: %v", err)))
		return status.Errorf(codes.Internal, "finalize state run: %v", err)
	}

	return stream.Send(terminalEvent(runID, recordStatusToProto(finalStatus), reportAggregatesToProto(report), finalErrMsg))
}

// applyStateRemote runs a targeted apply across agents.
//
// The run record is opened with no declarations: the control plane has
// not compiled them and, with per-agent facts, different hosts can
// legitimately resolve the same file to different declaration sets.
// Recording one host's view as if it were the run's would be a lie in
// the history table.
func (s *StateGRPCServer) applyStateRemote(
	stream grpc.ServerStreamingServer[v1.ApplyStateResponse],
	req *v1.ApplyStateRequest,
	agents []state.AgentRecord,
) error {
	ctx := stream.Context()
	mode := state.StateRunModeApply
	convergeMode := agent.ConvergeModeApply
	if req.GetDryRun() {
		mode = state.StateRunModeCheck
		convergeMode = agent.ConvergeModeCheck
	}
	runID := uuid.NewString()

	if err := s.openRun(ctx, runID, mode, req.GetSource(), req.GetClusterId(), req.GetAgentId(), nil); err != nil {
		return err
	}
	if err := stream.Send(&v1.ApplyStateResponse{Event: &v1.ApplyStateResponse_RunId{RunId: runID}}); err != nil {
		return err
	}

	terminal, err := s.runRemote(ctx, stream, runID, agents, req, convergeMode, convergePrincipal(ctx))
	if err != nil {
		return err
	}

	finalStatus := state.StateRunStatusCompleted
	if terminal.GetStatus() == v1.StateRunStatus_STATE_RUN_STATUS_FAILED {
		finalStatus = state.StateRunStatusFailed
	}
	end := state.StateRunEnd{
		Status:    finalStatus,
		EndedAt:   nowOr(s.now()).Time(),
		Changed:   int(terminal.GetAggregates().GetChanged()),
		Unchanged: int(terminal.GetAggregates().GetUnchanged()),
		Failed:    int(terminal.GetAggregates().GetFailed()),
		Skipped:   int(terminal.GetAggregates().GetSkipped()),
	}
	end.Total = end.Changed + end.Unchanged + end.Failed + end.Skipped
	if err := s.Store.FinalizeStateRun(ctx, runID, end); err != nil {
		_ = stream.Send(&v1.ApplyStateResponse{Event: &v1.ApplyStateResponse_Terminal{Terminal: terminal}})
		return status.Errorf(codes.Internal, "finalize state run: %v", err)
	}
	return stream.Send(&v1.ApplyStateResponse{Event: &v1.ApplyStateResponse_Terminal{Terminal: terminal}})
}

// CheckState is the unary form of dry-run. Same compile pipeline, but
// every per-decl result lands in the response slice rather than the
// stream. Useful for CLI consumers that want one round-trip.
func (s *StateGRPCServer) CheckState(ctx context.Context, req *v1.CheckStateRequest) (*v1.CheckStateResponse, error) {
	if err := requireYAML(req.GetYamlContent()); err != nil {
		return nil, err
	}
	decls, err := s.compile(req.GetYamlContent(), toMap(req.GetVariableOverrides()), toMap(req.GetFacts()))
	if err != nil {
		return nil, err
	}
	runID := uuid.NewString()
	if err := s.openRun(ctx, runID, state.StateRunModeCheck, req.GetSource(), req.GetClusterId(), req.GetAgentId(), decls); err != nil {
		return nil, err
	}

	runner := &statemgmt.Runner{Registry: s.resolveRegistry()}
	report, runErr := runner.Check(ctx, decls)
	resp := &v1.CheckStateResponse{
		RunId:      runID,
		Aggregates: reportAggregatesToProto(report),
	}
	resp.Status = v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED
	if runErr != nil {
		resp.Status = v1.StateRunStatus_STATE_RUN_STATUS_FAILED
		resp.ErrorMessage = runErr.Error()
	}
	// Persist per-decl rows + finalize.
	persistErr := s.persistResults(ctx, runID, report)
	finalStatus := state.StateRunStatusCompleted
	if runErr != nil {
		finalStatus = state.StateRunStatusFailed
	}
	end := reportAggregatesToEnd(finalStatus, nowOr(s.now()), joinErrs(runErr, persistErr), report)
	if err := s.Store.FinalizeStateRun(ctx, runID, end); err != nil {
		return nil, status.Errorf(codes.Internal, "finalize state run: %v", err)
	}

	for _, r := range report.Results {
		resp.Declarations = append(resp.Declarations, declResultToProto(&r))
	}
	return resp, nil
}

// DetectDrift runs the Check phase + severity classification.
// Persists like the other RPCs.
func (s *StateGRPCServer) DetectDrift(ctx context.Context, req *v1.DetectDriftRequest) (*v1.DetectDriftResponse, error) {
	if err := requireYAML(req.GetYamlContent()); err != nil {
		return nil, err
	}
	decls, err := s.compile(req.GetYamlContent(), toMap(req.GetVariableOverrides()), toMap(req.GetFacts()))
	if err != nil {
		return nil, err
	}
	runID := uuid.NewString()
	if err := s.openRun(ctx, runID, state.StateRunModeDrift, req.GetSource(), req.GetClusterId(), req.GetAgentId(), decls); err != nil {
		return nil, err
	}

	detector := &statemgmt.Detector{Registry: s.resolveRegistry()}
	report, runErr := detector.Detect(ctx, decls)

	resp := &v1.DetectDriftResponse{
		RunId:             runID,
		AggregateSeverity: severityToProto(report.AggregateSeverity),
		Aggregates:        driftReportAggregatesToProto(report),
		Status:            v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED,
	}
	if runErr != nil {
		resp.Status = v1.StateRunStatus_STATE_RUN_STATUS_FAILED
		resp.ErrorMessage = runErr.Error()
	}
	for _, s := range report.Statuses {
		resp.Statuses = append(resp.Statuses, driftStatusToProto(s))
	}

	// Persist as a drift-mode run. Reuse the runner-shaped result
	// rows so GetStateStatus on a drift run looks consistent with
	// the apply/check paths.
	if err := s.persistDriftResults(ctx, runID, report); err != nil {
		return nil, status.Errorf(codes.Internal, "persist drift results: %v", err)
	}
	finalStatus := state.StateRunStatusCompleted
	if runErr != nil {
		finalStatus = state.StateRunStatusFailed
	}
	end := state.StateRunEnd{
		Status:       finalStatus,
		EndedAt:      s.now(),
		ErrorMessage: errMsg(runErr),
		Total:        report.TotalChecked,
		Unchanged:    report.InSync,
		Failed:       report.Errors,
		Skipped:      report.Skipped,
		Drifted:      report.Drifted,
	}
	if err := s.Store.FinalizeStateRun(ctx, runID, end); err != nil {
		return nil, status.Errorf(codes.Internal, "finalize state run: %v", err)
	}
	return resp, nil
}

func (s *StateGRPCServer) GetStateHistory(ctx context.Context, req *v1.GetStateHistoryRequest) (*v1.GetStateHistoryResponse, error) {
	filter := state.StateRunFilter{
		AgentID: req.GetAgentId(),
		Mode:    protoModeToRecord(req.GetMode()),
		Status:  protoStatusToRecord(req.GetStatus()),
		Limit:   int(req.GetPageSize()),
		Offset:  int(req.GetPageOffset()),
	}
	if t := req.GetSince(); t != nil {
		filter.After = t.AsTime()
	}
	if t := req.GetUntil(); t != nil {
		filter.Before = t.AsTime()
	}
	records, err := s.Store.ListStateRuns(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list state runs: %v", err)
	}
	out := &v1.GetStateHistoryResponse{}
	for _, r := range records {
		out.Runs = append(out.Runs, runRecordToProto(r))
	}
	return out, nil
}

// RollbackState re-applies the declarations from a previously-stored
// run. The proto path is server-driven so the client doesn't have to
// reconstruct YAML from per-decl history. New run_id; new state_runs
// row; the stream shape matches ApplyState's so the CLI's drain loop
// is identical.
func (s *StateGRPCServer) RollbackState(req *v1.RollbackStateRequest, rbStream grpc.ServerStreamingServer[v1.RollbackStateResponse]) error {
	// RollbackState streams RollbackStateResponse (distinct from
	// ApplyStateResponse for buf RPC_REQUEST_RESPONSE_UNIQUE) but the
	// internal apply machinery (streamObserver, send helpers) is typed
	// to ApplyStateResponse. The adapter presents the ApplyState shape
	// and translates each message on Send — keeping this body and the
	// shared observer untouched.
	stream := rollbackStreamAdapter{rbStream}
	if req.GetRunId() == "" {
		return status.Error(codes.InvalidArgument, "run_id is required")
	}

	// Load the historical record; surface NotFound cleanly.
	header, _, err := s.Store.GetStateRun(stream.Context(), req.GetRunId())
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return status.Errorf(codes.NotFound, "state run %q not found", req.GetRunId())
		}
		return status.Errorf(codes.Internal, "get state run: %v", err)
	}
	decls, err := unmarshalDeclarations(header.DeclarationsJSON)
	if err != nil {
		return status.Errorf(codes.Internal, "decode declarations: %v", err)
	}
	if len(decls) == 0 {
		return status.Errorf(codes.FailedPrecondition, "state run %q has no declarations to roll back", req.GetRunId())
	}

	// Re-validate against the current Registry — guards against
	// modules removed/renamed since the original run. Skip render
	// (decls are already rendered) but redo Resolver to ensure topo
	// order is current.
	reg := s.resolveRegistry()
	fake := &statemgmt.StateFile{Declarations: decls}
	if err := statemgmt.NewValidator(reg).Validate(fake); err != nil {
		return status.Errorf(codes.FailedPrecondition, "validate: %v", err)
	}
	ordered, err := statemgmt.NewResolver().Resolve(fake)
	if err != nil {
		return status.Errorf(codes.FailedPrecondition, "resolve: %v", err)
	}

	// Choose source/agent/cluster: request overrides win; otherwise
	// inherit from the original run.
	source := req.GetSource()
	if source == "" {
		source = "rollback-of-" + req.GetRunId()
	}
	agentID := req.GetAgentId()
	if agentID == "" {
		agentID = header.AgentID
	}
	clusterID := req.GetClusterId()
	if clusterID == "" {
		clusterID = header.ClusterID
	}

	mode := state.StateRunModeApply
	if req.GetDryRun() {
		mode = state.StateRunModeCheck
	}
	runID := uuid.NewString()
	if err := s.openRun(stream.Context(), runID, mode, source, clusterID, agentID, ordered); err != nil {
		return err
	}

	if err := stream.Send(&v1.ApplyStateResponse{Event: &v1.ApplyStateResponse_RunId{RunId: runID}}); err != nil {
		return err
	}

	obs := &streamObserver{stream: stream, store: s.Store, runID: runID}
	runner := &statemgmt.Runner{Registry: reg, Observer: obs}

	var report *statemgmt.RunReport
	var runErr error
	if req.GetDryRun() {
		report, runErr = runner.Check(stream.Context(), ordered)
	} else {
		report, runErr = runner.Run(stream.Context(), ordered)
	}

	finalErrMsg := obsErrors(obs, runErr)
	finalStatus := state.StateRunStatusCompleted
	if runErr != nil {
		finalStatus = state.StateRunStatusFailed
	}
	end := reportAggregatesToEnd(finalStatus, nowOr(s.now()), finalErrMsg, report)
	if err := s.Store.FinalizeStateRun(stream.Context(), runID, end); err != nil {
		_ = stream.Send(terminalEvent(runID, recordStatusToProto(finalStatus), reportAggregatesToProto(report), fmt.Sprintf("finalize: %v", err)))
		return status.Errorf(codes.Internal, "finalize state run: %v", err)
	}
	return stream.Send(terminalEvent(runID, recordStatusToProto(finalStatus), reportAggregatesToProto(report), finalErrMsg))
}

func (s *StateGRPCServer) GetStateStatus(ctx context.Context, req *v1.GetStateStatusRequest) (*v1.GetStateStatusResponse, error) {
	if req.GetRunId() == "" {
		return nil, status.Error(codes.InvalidArgument, "run_id is required")
	}
	header, results, err := s.Store.GetStateRun(ctx, req.GetRunId())
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "state run %q not found", req.GetRunId())
		}
		return nil, status.Errorf(codes.Internal, "get state run: %v", err)
	}
	out := &v1.GetStateStatusResponse{Run: runRecordToProto(header)}
	for _, r := range results {
		out.Declarations = append(out.Declarations, resultRecordToProto(r))
	}
	return out, nil
}

// ---- helpers --------------------------------------------------------

func (s *StateGRPCServer) resolveRegistry() *statemgmt.Registry {
	if s.Registry != nil {
		return s.Registry
	}
	return statemgmt.DefaultRegistry
}

func (s *StateGRPCServer) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

// compile runs the YAML through Parse → Render → Validate → Resolve.
// Includes inside the YAML are rejected — v1.0 ships without a
// server-side state library directory (V1X candidate). The compiled
// declarations are returned in topo execution order.
func (s *StateGRPCServer) compile(yaml []byte, vars, facts map[string]any) ([]*statemgmt.Declaration, error) {
	sf, err := statemgmt.Parse(yaml)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "parse: %v", err)
	}
	if len(sf.Includes) > 0 {
		return nil, status.Errorf(codes.InvalidArgument, "includes not supported in v0.1 yaml_content path; submit a fully-resolved state file")
	}
	if len(vars) > 0 {
		if sf.Variables == nil {
			sf.Variables = map[string]any{}
		}
		for k, v := range vars {
			sf.Variables[k] = v
		}
	}
	renderer := statemgmt.NewRenderer()
	rendered, err := renderer.RenderStateFile(sf, facts)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "render: %v", err)
	}
	if err := statemgmt.NewValidator(s.resolveRegistry()).Validate(rendered); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "validate: %v", err)
	}
	ordered, err := statemgmt.NewResolver().Resolve(rendered)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "resolve: %v", err)
	}
	return ordered, nil
}

// openRun creates the state_runs header so per-decl rows have a
// foreign-key target as the run progresses.
func (s *StateGRPCServer) openRun(ctx context.Context, runID string, mode state.StateRunMode, source, clusterID, agentID string, decls []*statemgmt.Declaration) error {
	declJSON, err := declarationsToJSON(decls)
	if err != nil {
		return status.Errorf(codes.Internal, "encode declarations: %v", err)
	}
	rec := &state.StateRunRecord{
		ID:               runID,
		Mode:             mode,
		Source:           source,
		ClusterID:        clusterID,
		AgentID:          agentID,
		StartedAt:        s.now(),
		Status:           state.StateRunStatusRunning,
		DeclarationsJSON: declJSON,
	}
	if err := s.Store.CreateStateRun(ctx, rec); err != nil {
		return status.Errorf(codes.Internal, "create state run: %v", err)
	}
	return nil
}

// persistResults inserts per-decl rows from a finished RunReport.
// Used by CheckState (unary) — ApplyState's stream observer persists
// inline as each decl completes.
func (s *StateGRPCServer) persistResults(ctx context.Context, runID string, rep *statemgmt.RunReport) error {
	if rep == nil {
		return nil
	}
	for _, r := range rep.Results {
		if err := s.Store.AddStateRunResult(ctx, runID, declResultToRecord(runID, &r)); err != nil {
			return err
		}
	}
	return nil
}

// persistDriftResults inserts per-decl rows from a DriftReport.
// Drift statuses don't map 1-to-1 to DeclarationResults (drift has
// no apply/test) but we shape them into the same result rows so
// GetStateStatus stays uniform across modes.
func (s *StateGRPCServer) persistDriftResults(ctx context.Context, runID string, rep *statemgmt.DriftReport) error {
	if rep == nil {
		return nil
	}
	for _, ds := range rep.Statuses {
		rec := &state.StateRunResultRecord{
			RunID:      runID,
			DeclID:     ds.DeclID,
			Module:     ds.Module,
			Outcome:    driftStateToRecordOutcome(ds.State),
			CheckDiff:  ds.Diff,
			StartedAt:  ds.StartedAt,
			DurationMS: ds.Duration.Milliseconds(),
		}
		if ds.Error != nil {
			rec.ErrorMessage = ds.Error.Error()
		}
		if err := s.Store.AddStateRunResult(ctx, runID, rec); err != nil {
			return err
		}
	}
	return nil
}

func driftStateToRecordOutcome(s statemgmt.DriftState) state.StateRunOutcome {
	switch s {
	case statemgmt.DriftStateInSync:
		return state.StateRunOutcomeUnchanged
	case statemgmt.DriftStateDrifted:
		return state.StateRunOutcomeDriftDetected
	case statemgmt.DriftStateError:
		return state.StateRunOutcomeFailed
	case statemgmt.DriftStateSkipped:
		return state.StateRunOutcomeSkipped
	default:
		return state.StateRunOutcome("unknown")
	}
}

// streamObserver is the RunObserver wired during ApplyState. Each
// observer call (Start / Drift / Change / Done / Skip) translates to
// either a stream send, a persistence write, or both. Persistence
// errors accumulate in persistErrs so they can roll up into the run's
// final ErrorMessage rather than aborting the stream.
// rollbackStreamAdapter presents a RollbackStateResponse server
// stream as an ApplyStateResponse one. The embedded interface
// promotes Context/SetHeader/etc.; the declared Send shadows the
// embedded Send and translates the (identical-shape) oneof so the
// RollbackState body and streamObserver remain ApplyState-typed.
type rollbackStreamAdapter struct {
	grpc.ServerStreamingServer[v1.RollbackStateResponse]
}

func (a rollbackStreamAdapter) Send(m *v1.ApplyStateResponse) error {
	out := &v1.RollbackStateResponse{}
	switch e := m.GetEvent().(type) {
	case *v1.ApplyStateResponse_RunId:
		out.Event = &v1.RollbackStateResponse_RunId{RunId: e.RunId}
	case *v1.ApplyStateResponse_DeclResult:
		out.Event = &v1.RollbackStateResponse_DeclResult{DeclResult: e.DeclResult}
	case *v1.ApplyStateResponse_Terminal:
		out.Event = &v1.RollbackStateResponse_Terminal{Terminal: e.Terminal}
	}
	return a.ServerStreamingServer.Send(out)
}

type streamObserver struct {
	stream      grpc.ServerStreamingServer[v1.ApplyStateResponse]
	store       state.StateHistoryStore
	runID       string
	persistErrs []error
}

func (o *streamObserver) Start(_ context.Context, _ *statemgmt.Declaration) {
	// state.apply.start is observed but not surfaced on the wire in
	// v1.0 — the per-decl event arrives at Done/Skip time. Epic 11
	// will wire it to the NATS event subject.
}

func (o *streamObserver) Drift(_ context.Context, _ *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) {
	// Same — state.drift will be a separate event subject in
	// Epic 11. The per-decl event below carries the drift diff.
}

func (o *streamObserver) Change(_ context.Context, _ *statemgmt.Declaration, _ *statemgmt.StateResult) {
	// Same. The Done event carries Apply.Changed/Diff.
}

func (o *streamObserver) Done(ctx context.Context, res *statemgmt.DeclarationResult) {
	o.emit(ctx, res)
}

func (o *streamObserver) Skip(ctx context.Context, decl *statemgmt.Declaration, reason error) {
	// Map cascade-skip to a synthetic DeclarationResult so wire +
	// persistence stay uniform.
	res := &statemgmt.DeclarationResult{
		DeclID:    decl.ID,
		Module:    decl.Module,
		Outcome:   statemgmt.OutcomeSkipped,
		Error:     reason,
		StartedAt: time.Now(),
	}
	o.emit(ctx, res)
}

func (o *streamObserver) emit(ctx context.Context, res *statemgmt.DeclarationResult) {
	// Wire: per-decl result message.
	if err := o.stream.Send(&v1.ApplyStateResponse{
		Event: &v1.ApplyStateResponse_DeclResult{DeclResult: declResultToProto(res)},
	}); err != nil {
		o.persistErrs = append(o.persistErrs, fmt.Errorf("stream send %s: %w", res.DeclID, err))
		return
	}
	// Persist: per-decl row.
	if err := o.store.AddStateRunResult(ctx, o.runID, declResultToRecord(o.runID, res)); err != nil {
		o.persistErrs = append(o.persistErrs, fmt.Errorf("persist %s: %w", res.DeclID, err))
	}
}

// requireYAML guards against empty payloads. Empty bytes is a
// frequent client mistake; surface it with a clean InvalidArgument
// rather than letting the parser produce a less actionable error.
func requireYAML(b []byte) error {
	if len(b) == 0 {
		return status.Error(codes.InvalidArgument, "yaml_content is required")
	}
	return nil
}

func terminalEvent(runID string, status v1.StateRunStatus, aggr *v1.StateRunAggregates, errMsg string) *v1.ApplyStateResponse {
	return &v1.ApplyStateResponse{
		Event: &v1.ApplyStateResponse_Terminal{
			Terminal: &v1.StateRunTerminal{
				RunId:        runID,
				Status:       status,
				Aggregates:   aggr,
				ErrorMessage: errMsg,
			},
		},
	}
}

func obsErrors(obs *streamObserver, runErr error) string {
	parts := make([]string, 0, len(obs.persistErrs)+1)
	if runErr != nil {
		parts = append(parts, runErr.Error())
	}
	for _, e := range obs.persistErrs {
		parts = append(parts, e.Error())
	}
	return strings.Join(parts, "; ")
}

func joinErrs(a, b error) string {
	switch {
	case a == nil && b == nil:
		return ""
	case a == nil:
		return b.Error()
	case b == nil:
		return a.Error()
	default:
		return a.Error() + "; " + b.Error()
	}
}

func errMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func toMap(in map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
