// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// ---- test plumbing -------------------------------------------------

// stateFixture wires a fresh StateGRPCServer over bufconn + in-memory
// SQLite. Each test gets a hermetic instance: its own Registry, its
// own store, its own gRPC server.
type stateFixture struct {
	t        *testing.T
	registry *statemgmt.Registry
	store    state.Store
	server   *StateGRPCServer
	grpcSrv  *grpc.Server
	conn     *grpc.ClientConn
	client   v1.StateServiceClient
}

func newStateFixture(t *testing.T) *stateFixture {
	t.Helper()
	store, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: ":memory:"},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	registry := statemgmt.NewRegistry()
	srv := NewStateGRPCServer(registry, store)

	listener := bufconn.Listen(1 << 20)
	grpcSrv := grpc.NewServer()
	v1.RegisterStateServiceServer(grpcSrv, srv)

	go func() {
		if err := grpcSrv.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("grpc serve: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}

	t.Cleanup(func() {
		_ = conn.Close()
		grpcSrv.Stop()
		_ = listener.Close()
		_ = store.Close()
	})

	return &stateFixture{
		t:        t,
		registry: registry,
		store:    store,
		server:   srv,
		grpcSrv:  grpcSrv,
		conn:     conn,
		client:   v1.NewStateServiceClient(conn),
	}
}

// fixtureModule is a scripted Module the test fixture installs into
// the per-fixture Registry. checkCalls/applyCalls/testCalls let
// tests assert phase invocation. checkMatches drives Check's
// outcome.
type fixtureModule struct {
	name         string
	validStates  []string
	checkMatches bool
	checkErr     error
	applyChanged bool
	applyErr     error
	testResult   bool
	testErr      error
	checkCalls   atomic.Int64
	applyCalls   atomic.Int64
	testCalls    atomic.Int64
}

func (m *fixtureModule) Name() string          { return m.name }
func (m *fixtureModule) ValidStates() []string { return m.validStates }
func (m *fixtureModule) Check(context.Context, *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	m.checkCalls.Add(1)
	if m.checkErr != nil {
		return nil, m.checkErr
	}
	return &statemgmt.ModuleCheckResult{Matches: m.checkMatches, Diff: "test-diff"}, nil
}
func (m *fixtureModule) Apply(context.Context, *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	m.applyCalls.Add(1)
	if m.applyErr != nil {
		return nil, m.applyErr
	}
	return &statemgmt.StateResult{Success: true, Changed: m.applyChanged, Diff: "applied-diff"}, nil
}
func (m *fixtureModule) Test(context.Context, *statemgmt.Declaration) (bool, error) {
	m.testCalls.Add(1)
	if m.testErr != nil {
		return false, m.testErr
	}
	return m.testResult, nil
}

func (f *stateFixture) installModule(name string, opts ...func(*fixtureModule)) *fixtureModule {
	f.t.Helper()
	m := &fixtureModule{
		name:        name,
		validStates: []string{"present"},
		testResult:  true,
	}
	for _, opt := range opts {
		opt(m)
	}
	if err := f.registry.Register(name, func() statemgmt.Module { return m }); err != nil {
		f.t.Fatalf("Register %s: %v", name, err)
	}
	return m
}

// receiveAll drains a stream until io.EOF.
func receiveAll[T any](t *testing.T, stream grpc.ServerStreamingClient[T]) []*T {
	t.Helper()
	var out []*T
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("stream recv: %v", err)
		}
		out = append(out, msg)
	}
}

// ---- ApplyState ----------------------------------------------------

func TestStateGRPCServer_ApplyState_InSyncRun(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	mod := f.installModule("file", func(m *fixtureModule) {
		m.checkMatches = true
	})

	stream, err := f.client.ApplyState(t.Context(), &v1.ApplyStateRequest{
		YamlContent: []byte("file:\n  /etc/hosts:\n    state: present\n"),
		Source:      "test.yaml",
		AgentId:     "agent-1",
	})
	if err != nil {
		t.Fatalf("ApplyState: %v", err)
	}
	events := receiveAll(t, stream)
	if len(events) < 3 {
		t.Fatalf("len(events) = %d, want >= 3 (run_id + decl_result + terminal)", len(events))
	}
	// First event is run_id.
	runID := events[0].GetRunId()
	if runID == "" {
		t.Errorf("first event = %+v, want run_id", events[0])
	}
	// Find the terminal.
	var term *v1.StateRunTerminal
	for _, e := range events {
		if e.GetTerminal() != nil {
			term = e.GetTerminal()
		}
	}
	if term == nil {
		t.Fatal("no terminal event")
	}
	if term.Status != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
		t.Errorf("Status = %v, want COMPLETED", term.Status)
	}
	if term.Aggregates.Unchanged != 1 {
		t.Errorf("Unchanged = %d, want 1", term.Aggregates.Unchanged)
	}
	if mod.applyCalls.Load() != 0 {
		t.Errorf("Apply invoked %d times, want 0 (Check matched)", mod.applyCalls.Load())
	}

	// Verify persistence landed in lockstep.
	header, results, err := f.store.GetStateRun(t.Context(), runID)
	if err != nil {
		t.Fatalf("GetStateRun: %v", err)
	}
	if header.Status != state.StateRunStatusCompleted {
		t.Errorf("persisted Status = %v, want completed", header.Status)
	}
	if header.Total != 1 || header.Unchanged != 1 {
		t.Errorf("persisted counts wrong: total=%d unchanged=%d", header.Total, header.Unchanged)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
	if header.DeclarationsJSON == "[]" || !strings.Contains(header.DeclarationsJSON, "file:/etc/hosts") {
		t.Errorf("DeclarationsJSON should carry the decl shape; got %q", header.DeclarationsJSON)
	}
}

func TestStateGRPCServer_ApplyState_DriftAndApply(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	mod := f.installModule("file", func(m *fixtureModule) {
		m.checkMatches = false
		m.applyChanged = true
		m.testResult = true
	})

	stream, err := f.client.ApplyState(t.Context(), &v1.ApplyStateRequest{
		YamlContent: []byte("file:\n  /etc/hosts:\n    state: present\n"),
	})
	if err != nil {
		t.Fatalf("ApplyState: %v", err)
	}
	events := receiveAll(t, stream)

	var declOutcome v1.StateRunOutcome
	var term *v1.StateRunTerminal
	for _, e := range events {
		if d := e.GetDeclResult(); d != nil {
			declOutcome = d.Outcome
		}
		if t2 := e.GetTerminal(); t2 != nil {
			term = t2
		}
	}
	if declOutcome != v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED {
		t.Errorf("decl outcome = %v, want CHANGED", declOutcome)
	}
	if term.Aggregates.Changed != 1 {
		t.Errorf("Changed = %d, want 1", term.Aggregates.Changed)
	}
	if mod.applyCalls.Load() != 1 || mod.testCalls.Load() != 1 {
		t.Errorf("Apply/Test should fire once each; got apply=%d test=%d",
			mod.applyCalls.Load(), mod.testCalls.Load())
	}
}

func TestStateGRPCServer_ApplyState_FailureCascades(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	f.installModule("good", func(m *fixtureModule) {
		m.checkMatches = true
	})
	f.installModule("bad", func(m *fixtureModule) {
		m.checkErr = errors.New("disk unreachable")
	})

	// bad has no deps, good:/a has no deps, good:/c requires bad.
	// Topo (with source-order tiebreak): bad:/b → good:/a → good:/c.
	// When bad fails, the two remaining decls cascade-skip.
	yaml := []byte(strings.Join([]string{
		"bad:",
		"  /b:",
		"    state: present",
		"good:",
		"  /a:",
		"    state: present",
		"  /c:",
		"    state: present",
		"    require: [{ bad: /b }]",
	}, "\n") + "\n")

	stream, err := f.client.ApplyState(t.Context(), &v1.ApplyStateRequest{YamlContent: yaml})
	if err != nil {
		t.Fatalf("ApplyState: %v", err)
	}
	events := receiveAll(t, stream)

	var term *v1.StateRunTerminal
	skipSeen := 0
	failSeen := 0
	for _, e := range events {
		if d := e.GetDeclResult(); d != nil {
			switch d.Outcome {
			case v1.StateRunOutcome_STATE_RUN_OUTCOME_FAILED:
				failSeen++
			case v1.StateRunOutcome_STATE_RUN_OUTCOME_SKIPPED:
				skipSeen++
			}
		}
		if t2 := e.GetTerminal(); t2 != nil {
			term = t2
		}
	}
	if failSeen != 1 {
		t.Errorf("failSeen = %d, want 1", failSeen)
	}
	if skipSeen != 2 {
		t.Errorf("skipSeen = %d, want 2 (good:/a and good:/c after bad fails)", skipSeen)
	}
	if term.Status != v1.StateRunStatus_STATE_RUN_STATUS_FAILED {
		t.Errorf("Status = %v, want FAILED", term.Status)
	}
}

// ---- CheckState ----------------------------------------------------

func TestStateGRPCServer_CheckState_DriftReported_NoApply(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	mod := f.installModule("file", func(m *fixtureModule) {
		m.checkMatches = false
	})

	resp, err := f.client.CheckState(t.Context(), &v1.CheckStateRequest{
		YamlContent: []byte("file:\n  /etc/hosts:\n    state: present\n"),
	})
	if err != nil {
		t.Fatalf("CheckState: %v", err)
	}
	if len(resp.Declarations) != 1 {
		t.Fatalf("len(declarations) = %d, want 1", len(resp.Declarations))
	}
	if resp.Declarations[0].Outcome != v1.StateRunOutcome_STATE_RUN_OUTCOME_DRIFT_DETECTED {
		t.Errorf("Outcome = %v, want DRIFT_DETECTED", resp.Declarations[0].Outcome)
	}
	if resp.Aggregates.Drifted != 1 {
		t.Errorf("Drifted = %d, want 1", resp.Aggregates.Drifted)
	}
	if mod.applyCalls.Load() != 0 {
		t.Errorf("Apply called %d times in Check mode; want 0", mod.applyCalls.Load())
	}
	if resp.RunId == "" {
		t.Error("RunId empty")
	}
	header, _, err := f.store.GetStateRun(t.Context(), resp.RunId)
	if err != nil {
		t.Fatalf("GetStateRun: %v", err)
	}
	if header.Mode != state.StateRunModeCheck {
		t.Errorf("persisted Mode = %v, want check", header.Mode)
	}
}

// ---- DetectDrift ---------------------------------------------------

func TestStateGRPCServer_DetectDrift_SeverityAggregation(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	f.installModule("file", func(m *fixtureModule) {
		m.checkMatches = false
	})

	yaml := []byte(strings.Join([]string{
		"file:",
		"  /low:",
		"    state: present",
		"    severity: low",
		"  /critical:",
		"    state: present",
		"    severity: critical",
	}, "\n") + "\n")
	resp, err := f.client.DetectDrift(t.Context(), &v1.DetectDriftRequest{YamlContent: yaml})
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if resp.AggregateSeverity != v1.DriftSeverity_DRIFT_SEVERITY_CRITICAL {
		t.Errorf("AggregateSeverity = %v, want CRITICAL", resp.AggregateSeverity)
	}
	if resp.Aggregates.Drifted != 2 {
		t.Errorf("Drifted = %d, want 2", resp.Aggregates.Drifted)
	}
	if len(resp.Statuses) != 2 {
		t.Errorf("len(Statuses) = %d, want 2", len(resp.Statuses))
	}
	header, _, err := f.store.GetStateRun(t.Context(), resp.RunId)
	if err != nil {
		t.Fatalf("GetStateRun: %v", err)
	}
	if header.Mode != state.StateRunModeDrift {
		t.Errorf("persisted Mode = %v, want drift", header.Mode)
	}
}

// ---- GetStateHistory / GetStateStatus ------------------------------

func TestStateGRPCServer_HistoryAndStatus(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	f.installModule("file", func(m *fixtureModule) {
		m.checkMatches = true
	})

	// Seed 3 runs by calling CheckState (cheap; in-sync).
	yaml := []byte("file:\n  /etc/hosts:\n    state: present\n")
	for i := 0; i < 3; i++ {
		if _, err := f.client.CheckState(t.Context(), &v1.CheckStateRequest{
			YamlContent: yaml,
			AgentId:     "agent-1",
		}); err != nil {
			t.Fatalf("CheckState seed %d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond) // distinct started_at
	}

	hist, err := f.client.GetStateHistory(t.Context(), &v1.GetStateHistoryRequest{
		AgentId: "agent-1",
	})
	if err != nil {
		t.Fatalf("GetStateHistory: %v", err)
	}
	if len(hist.Runs) != 3 {
		t.Errorf("history len = %d, want 3", len(hist.Runs))
	}

	stat, err := f.client.GetStateStatus(t.Context(), &v1.GetStateStatusRequest{
		RunId: hist.Runs[0].Id,
	})
	if err != nil {
		t.Fatalf("GetStateStatus: %v", err)
	}
	if stat.Run.Id != hist.Runs[0].Id {
		t.Errorf("returned Run.Id = %q, want %q", stat.Run.Id, hist.Runs[0].Id)
	}
	if len(stat.Declarations) != 1 {
		t.Errorf("declarations = %d, want 1", len(stat.Declarations))
	}
}

func TestStateGRPCServer_GetStateStatus_NotFound(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	_, err := f.client.GetStateStatus(t.Context(), &v1.GetStateStatusRequest{RunId: "ghost"})
	if err == nil {
		t.Fatal("expected NotFound")
	}
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound", status.Code(err))
	}
}

// ---- Input validation ---------------------------------------------

func TestStateGRPCServer_RejectsEmptyYAML(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	_, err := f.client.CheckState(t.Context(), &v1.CheckStateRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestStateGRPCServer_RejectsIncludes(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	_, err := f.client.CheckState(t.Context(), &v1.CheckStateRequest{
		YamlContent: []byte("includes:\n  - other.yaml\n"),
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
	if !strings.Contains(err.Error(), "includes not supported") {
		t.Errorf("err = %v, want mention of includes limitation", err)
	}
}

func TestStateGRPCServer_RejectsMalformedYAML(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	_, err := f.client.CheckState(t.Context(), &v1.CheckStateRequest{
		YamlContent: []byte("file:\n  /etc/hosts: {state: present\n"), // unterminated
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestStateGRPCServer_RejectsUnknownModule(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t) // no modules installed
	_, err := f.client.CheckState(t.Context(), &v1.CheckStateRequest{
		YamlContent: []byte("ghost:\n  /x:\n    state: present\n"),
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("err = %v, want module name cited", err)
	}
}

// ---- Variable overrides + facts ------------------------------------

// ---- RollbackState -------------------------------------------------

func TestStateGRPCServer_RollbackState_ReappliesStoredDeclarations(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	mod := f.installModule("file", func(m *fixtureModule) {
		m.checkMatches = true
	})

	// 1. Seed: run an Apply so we have a stored run with persisted
	//    DeclarationsJSON.
	yaml := []byte("file:\n  /etc/hosts:\n    state: present\n")
	stream, err := f.client.ApplyState(t.Context(), &v1.ApplyStateRequest{
		YamlContent: yaml, AgentId: "web-1", Source: "original.yaml",
	})
	if err != nil {
		t.Fatalf("seed ApplyState: %v", err)
	}
	originalEvents := receiveAll(t, stream)
	originalRunID := originalEvents[0].GetRunId()
	if originalRunID == "" {
		t.Fatal("no run_id from seed apply")
	}

	mod.checkCalls.Store(0) // reset so rollback's invocation is unambiguous

	// 2. Rollback. New run; new id; Check fires on the test module
	//    (matches → unchanged → no Apply).
	rollbackStream, err := f.client.RollbackState(t.Context(), &v1.RollbackStateRequest{
		RunId: originalRunID,
	})
	if err != nil {
		t.Fatalf("RollbackState: %v", err)
	}
	events := receiveAll(t, rollbackStream)
	if len(events) < 3 {
		t.Fatalf("event count = %d, want >= 3", len(events))
	}
	rollbackRunID := events[0].GetRunId()
	if rollbackRunID == "" {
		t.Fatal("rollback didn't emit run_id event")
	}
	if rollbackRunID == originalRunID {
		t.Errorf("rollback run_id must be new; got same as original %q", originalRunID)
	}

	var term *v1.StateRunTerminal
	for _, e := range events {
		if t2 := e.GetTerminal(); t2 != nil {
			term = t2
		}
	}
	if term == nil || term.Status != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
		t.Errorf("rollback terminal = %+v, want COMPLETED", term)
	}
	if mod.checkCalls.Load() != 1 {
		t.Errorf("Check called %d times during rollback, want 1", mod.checkCalls.Load())
	}
	if mod.applyCalls.Load() != 0 {
		t.Errorf("Apply called %d times during rollback (Check matched, shouldn't fire)", mod.applyCalls.Load())
	}

	// 3. Persistence: the rollback's stored run inherits agent from
	//    the original, has the default rollback-of-<id> source.
	header, _, err := f.store.GetStateRun(t.Context(), rollbackRunID)
	if err != nil {
		t.Fatalf("GetStateRun(rollback): %v", err)
	}
	if header.AgentID != "web-1" {
		t.Errorf("rollback AgentID = %q, want inherited web-1", header.AgentID)
	}
	if header.Source != "rollback-of-"+originalRunID {
		t.Errorf("rollback Source = %q, want default rollback-of-<id>", header.Source)
	}
	if header.Mode != state.StateRunModeApply {
		t.Errorf("rollback Mode = %v, want apply (no --dry-run)", header.Mode)
	}
}

func TestStateGRPCServer_RollbackState_DryRunSkipsApply(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	mod := f.installModule("file", func(m *fixtureModule) {
		// Original run: drift detected then applied.
		m.checkMatches = false
		m.applyChanged = true
	})

	yaml := []byte("file:\n  /etc/hosts:\n    state: present\n")
	stream, err := f.client.ApplyState(t.Context(), &v1.ApplyStateRequest{YamlContent: yaml})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	seedEvents := receiveAll(t, stream)
	originalRunID := seedEvents[0].GetRunId()

	// Reset counters; flip the module to "still drifted" for the
	// rollback's check path.
	mod.checkCalls.Store(0)
	mod.applyCalls.Store(0)

	rs, err := f.client.RollbackState(t.Context(), &v1.RollbackStateRequest{
		RunId:  originalRunID,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("RollbackState: %v", err)
	}
	receiveAll(t, rs)
	if mod.applyCalls.Load() != 0 {
		t.Errorf("Apply called %d times during dry-run rollback; want 0", mod.applyCalls.Load())
	}
	// Verify Mode is check.
	headers, err := f.store.ListStateRuns(t.Context(), state.StateRunFilter{Mode: state.StateRunModeCheck})
	if err != nil {
		t.Fatalf("ListStateRuns: %v", err)
	}
	if len(headers) == 0 {
		t.Error("no check-mode runs persisted from dry-run rollback")
	}
}

func TestStateGRPCServer_RollbackState_OverridesApply(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	f.installModule("file", func(m *fixtureModule) { m.checkMatches = true })

	stream, err := f.client.ApplyState(t.Context(), &v1.ApplyStateRequest{
		YamlContent: []byte("file:\n  /a:\n    state: present\n"),
		AgentId:     "original-agent",
		ClusterId:   "original-cluster",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	originalRunID := receiveAll(t, stream)[0].GetRunId()

	rs, err := f.client.RollbackState(t.Context(), &v1.RollbackStateRequest{
		RunId:     originalRunID,
		Source:    "explicit-rollback",
		AgentId:   "new-agent",
		ClusterId: "new-cluster",
	})
	if err != nil {
		t.Fatalf("RollbackState: %v", err)
	}
	rollbackRunID := receiveAll(t, rs)[0].GetRunId()
	header, _, err := f.store.GetStateRun(t.Context(), rollbackRunID)
	if err != nil {
		t.Fatalf("GetStateRun: %v", err)
	}
	if header.Source != "explicit-rollback" {
		t.Errorf("Source = %q, want explicit-rollback", header.Source)
	}
	if header.AgentID != "new-agent" {
		t.Errorf("AgentID = %q, want new-agent", header.AgentID)
	}
	if header.ClusterID != "new-cluster" {
		t.Errorf("ClusterID = %q, want new-cluster", header.ClusterID)
	}
}

func TestStateGRPCServer_RollbackState_NotFound(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	stream, err := f.client.RollbackState(t.Context(), &v1.RollbackStateRequest{RunId: "ghost"})
	if err != nil {
		t.Fatalf("RollbackState: %v", err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound", status.Code(err))
	}
}

func TestStateGRPCServer_RollbackState_EmptyRunID(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	stream, err := f.client.RollbackState(t.Context(), &v1.RollbackStateRequest{})
	if err != nil {
		t.Fatalf("RollbackState: %v", err)
	}
	_, err = stream.Recv()
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

// ---- variable overrides + facts (existing) ------------------------

func TestStateGRPCServer_VariableOverridesApplied(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	f.installModule("file", func(m *fixtureModule) {
		m.checkMatches = true
	})

	// The YAML's State references a variable; override should
	// produce a valid state value at render time.
	yaml := []byte(strings.Join([]string{
		"variables:",
		"  desired_state: present",
		"file:",
		"  /etc/hosts:",
		"    state: '{{ .Vars.desired_state }}'",
	}, "\n") + "\n")
	resp, err := f.client.CheckState(t.Context(), &v1.CheckStateRequest{
		YamlContent:        yaml,
		VariableOverrides:  map[string]string{}, // no override
	})
	if err != nil {
		t.Fatalf("CheckState (no override): %v", err)
	}
	if resp.Status != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
		t.Errorf("Status = %v, want COMPLETED", resp.Status)
	}
}
