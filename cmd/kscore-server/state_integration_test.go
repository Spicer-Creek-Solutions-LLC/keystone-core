//go:build integration

// Package main test — Epic 08 task 13 in-process state integration.
//
// Boots the full Epic 08 server stack — `StateGRPCServer` +
// `stdlib.RegisterAll` + SQLite `StateHistoryStore` — over `bufconn`
// and exercises ApplyState, CheckState (via dry-run), DetectDrift,
// RollbackState, and `Runner.RunSaga` end-to-end using only
// tempdir-safe stdlib modules (file / link / cmd / config).
//
// Modules that touch live system state (package, service, user,
// hostname, mount, ssh, firewall, …) are out of scope here — the
// v0.5 gate covers them through a cross-distro Docker matrix. This
// test stays hermetic so `make test-integration` runs anywhere a
// Go toolchain + /bin/sh exist.
//
// Build-tagged `integration` because it runs real subprocesses via
// the `cmd` module and writes to the filesystem. Not part of
// `make test` (the unit tier). Run with `make test-integration`.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// stateFixture wires the Epic 08 server stack over bufconn with a
// fresh Registry (populated by stdlib.RegisterAll), an in-memory
// SQLite store, and a tempdir root for ${ROOT} substitution.
type stateFixture struct {
	t        *testing.T
	root     string // t.TempDir() — substituted into fixture YAML
	registry *statemgmt.Registry
	store    state.Store // implements state.StateHistoryStore
	server   *controlplane.StateGRPCServer
	grpcSrv  *grpc.Server
	conn     *grpc.ClientConn
	client   v1.StateServiceClient
}

func bootStateFixture(t *testing.T) *stateFixture {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("uses real /bin/sh subprocesses and POSIX symlink semantics; linux only in v0.1")
	}

	store, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: ":memory:"},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	registry := statemgmt.NewRegistry()
	if err := stdlib.RegisterAll(registry); err != nil {
		t.Fatalf("stdlib.RegisterAll: %v", err)
	}

	srv := controlplane.NewStateGRPCServer(registry, store)

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

	root := t.TempDir()

	t.Cleanup(func() {
		_ = conn.Close()
		grpcSrv.Stop()
		_ = listener.Close()
		_ = store.Close()
	})

	return &stateFixture{
		t:        t,
		root:     root,
		registry: registry,
		store:    store,
		server:   srv,
		grpcSrv:  grpcSrv,
		conn:     conn,
		client:   v1.NewStateServiceClient(conn),
	}
}

// loadFixture reads testdata/<name> and substitutes ${ROOT} → f.root.
// Tests can then send the bytes straight to ApplyStateRequest.
func (f *stateFixture) loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata/%s: %v", name, err)
	}
	return []byte(strings.ReplaceAll(string(raw), "${ROOT}", f.root))
}

// applyEvents collects the three event categories an ApplyState /
// RollbackState stream emits so tests can assert on them directly.
type applyEvents struct {
	runID    string
	decls    []*v1.StateDeclarationResult
	terminal *v1.StateRunTerminal
}

func drainApplyStream(t *testing.T, stream grpc.ServerStreamingClient[v1.ApplyStateResponse]) applyEvents {
	t.Helper()
	var ev applyEvents
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return ev
		}
		if err != nil {
			t.Fatalf("stream recv: %v", err)
		}
		switch x := msg.GetEvent().(type) {
		case *v1.ApplyStateResponse_RunId:
			ev.runID = x.RunId
		case *v1.ApplyStateResponse_DeclResult:
			ev.decls = append(ev.decls, x.DeclResult)
		case *v1.ApplyStateResponse_Terminal:
			ev.terminal = x.Terminal
		}
	}
}

// drainRollbackStream mirrors [drainApplyStream] for the
// RollbackStateResponse oneof, which became a distinct proto type
// after [drainApplyStream] was first written. The two streams emit
// the same RunId / DeclResult / Terminal events; only the wrapper
// type names differ.
func drainRollbackStream(t *testing.T, stream grpc.ServerStreamingClient[v1.RollbackStateResponse]) applyEvents {
	t.Helper()
	var ev applyEvents
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return ev
		}
		if err != nil {
			t.Fatalf("stream recv: %v", err)
		}
		switch x := msg.GetEvent().(type) {
		case *v1.RollbackStateResponse_RunId:
			ev.runID = x.RunId
		case *v1.RollbackStateResponse_DeclResult:
			ev.decls = append(ev.decls, x.DeclResult)
		case *v1.RollbackStateResponse_Terminal:
			ev.terminal = x.Terminal
		}
	}
}

// applyFixture is the convenient one-call form: load + send + drain.
func (f *stateFixture) applyFixture(t *testing.T, name string, dryRun bool) applyEvents {
	t.Helper()
	stream, err := f.client.ApplyState(t.Context(), &v1.ApplyStateRequest{
		YamlContent: f.loadFixture(t, name),
		Source:      name,
		AgentId:     "agent-int",
		DryRun:      dryRun,
	})
	if err != nil {
		t.Fatalf("ApplyState %s: %v", name, err)
	}
	return drainApplyStream(t, stream)
}

// changedCount returns how many decls reported CHANGED in ev. Used
// for idempotency assertions ("first run > 0; second run == 0").
func changedCount(ev applyEvents) int {
	n := 0
	for _, d := range ev.decls {
		if d.GetOutcome() == v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED {
			n++
		}
	}
	return n
}

// ---- Test 1: Apply + idempotency -----------------------------------

// First Apply changes the world; the second Apply on the same fixture
// must report zero CHANGED rows — the epic's idempotency gate.
func TestEpic08_StateIntegration_ApplyCheckIdempotency(t *testing.T) {
	f := bootStateFixture(t)

	first := f.applyFixture(t, "state-integration.yaml", false)
	if first.terminal == nil {
		t.Fatal("first apply: no terminal event")
	}
	if first.terminal.GetStatus() != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
		t.Fatalf("first apply: status=%v err=%q", first.terminal.GetStatus(), first.terminal.GetErrorMessage())
	}
	if got, want := len(first.decls), 10; got != want {
		t.Fatalf("first apply: decls=%d, want %d", got, want)
	}
	if changedCount(first) == 0 {
		t.Fatal("first apply: zero CHANGED decls; expected fresh resources")
	}

	// Spot-check representative artifacts on disk.
	helloPath := filepath.Join(f.root, "dirA", "hello.txt")
	if data, err := os.ReadFile(helloPath); err != nil {
		t.Fatalf("read %s: %v", helloPath, err)
	} else if got, want := string(data), "hello epic 08\n"; got != want {
		t.Errorf("hello.txt = %q, want %q", got, want)
	}
	// Symlink resolves through to the hello content.
	linkPath := filepath.Join(f.root, "dirB", "hello-link")
	if data, err := os.ReadFile(linkPath); err != nil {
		t.Fatalf("read symlink %s: %v", linkPath, err)
	} else if got, want := string(data), "hello epic 08\n"; got != want {
		t.Errorf("symlink target content = %q, want %q", got, want)
	}
	// cmd-emit-marker fired → marker.txt exists, AND the file decl
	// that requires it ran after.
	markerPath := filepath.Join(f.root, "dirA", "marker.txt")
	if _, err := os.Stat(markerPath); err != nil {
		t.Errorf("marker.txt not created by cmd-emit-marker: %v", err)
	}

	// Second apply: idempotent — no CHANGED rows.
	second := f.applyFixture(t, "state-integration.yaml", false)
	if second.terminal.GetStatus() != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
		t.Fatalf("second apply: status=%v err=%q", second.terminal.GetStatus(), second.terminal.GetErrorMessage())
	}
	for _, d := range second.decls {
		if d.GetOutcome() == v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED {
			t.Errorf("idempotency violated: %s reported CHANGED on second apply (diff=%q)", d.GetDeclId(), d.GetApplyDiff())
		}
	}

	// Dry-run (CheckState path) on the converged tree: every decl
	// should be UNCHANGED — no CHANGED, no DRIFT_DETECTED.
	dry := f.applyFixture(t, "state-integration.yaml", true)
	if dry.terminal.GetStatus() != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
		t.Fatalf("dry-run: status=%v err=%q", dry.terminal.GetStatus(), dry.terminal.GetErrorMessage())
	}
	for _, d := range dry.decls {
		switch d.GetOutcome() {
		case v1.StateRunOutcome_STATE_RUN_OUTCOME_UNCHANGED,
			v1.StateRunOutcome_STATE_RUN_OUTCOME_NO_OP:
			// good
		default:
			t.Errorf("dry-run on converged state: %s outcome=%v want UNCHANGED", d.GetDeclId(), d.GetOutcome())
		}
	}
}

// ---- Test 2: Drift detect after out-of-band mutation ---------------

// Apply the fixture, then delete a file behind the engine's back.
// DetectDrift must report exactly that decl as drifted; a follow-up
// Apply ("fix") must restore it and converge to zero CHANGED on a
// third Apply.
func TestEpic08_StateIntegration_DriftDetectAndFix(t *testing.T) {
	f := bootStateFixture(t)

	first := f.applyFixture(t, "state-integration.yaml", false)
	if first.terminal.GetStatus() != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
		t.Fatalf("first apply: status=%v err=%q", first.terminal.GetStatus(), first.terminal.GetErrorMessage())
	}

	// Out-of-band mutation: blow away one file.
	driftedID := "file:" + filepath.Join(f.root, "dirB", "extra.txt")
	if err := os.Remove(filepath.Join(f.root, "dirB", "extra.txt")); err != nil {
		t.Fatalf("simulate drift: %v", err)
	}

	drift, err := f.client.DetectDrift(t.Context(), &v1.DetectDriftRequest{
		YamlContent: f.loadFixture(t, "state-integration.yaml"),
		Source:      "drift-check.yaml",
		AgentId:     "agent-int",
	})
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if drift.GetStatus() != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
		t.Fatalf("drift: status=%v err=%q", drift.GetStatus(), drift.GetErrorMessage())
	}
	var driftedIDs []string
	for _, ds := range drift.GetStatuses() {
		if ds.GetState() == v1.DriftState_DRIFT_STATE_DRIFTED {
			driftedIDs = append(driftedIDs, ds.GetDeclId())
		}
	}
	if len(driftedIDs) != 1 || driftedIDs[0] != driftedID {
		t.Fatalf("drifted decls = %v, want [%s]", driftedIDs, driftedID)
	}
	if drift.GetAggregateSeverity() == v1.DriftSeverity_DRIFT_SEVERITY_NONE ||
		drift.GetAggregateSeverity() == v1.DriftSeverity_DRIFT_SEVERITY_UNSPECIFIED {
		t.Errorf("aggregate severity = %v, want > NONE for a drifted file", drift.GetAggregateSeverity())
	}

	// Fix: re-Apply the same YAML. Only the drifted decl should
	// report CHANGED; everything else stays UNCHANGED.
	fixed := f.applyFixture(t, "state-integration.yaml", false)
	if fixed.terminal.GetStatus() != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
		t.Fatalf("fix apply: status=%v err=%q", fixed.terminal.GetStatus(), fixed.terminal.GetErrorMessage())
	}
	var changedIDs []string
	for _, d := range fixed.decls {
		if d.GetOutcome() == v1.StateRunOutcome_STATE_RUN_OUTCOME_CHANGED {
			changedIDs = append(changedIDs, d.GetDeclId())
		}
	}
	if len(changedIDs) != 1 || changedIDs[0] != driftedID {
		t.Errorf("fix apply: changed decls = %v, want [%s]", changedIDs, driftedID)
	}

	// Third apply: fully converged again.
	third := f.applyFixture(t, "state-integration.yaml", false)
	if n := changedCount(third); n != 0 {
		t.Errorf("third apply (post-fix): changed = %d, want 0", n)
	}
}

// ---- Test 3: History + Rollback round-trip -------------------------

// Apply two versions of a fixture, list history, then RollbackState
// back to the first run. The file on disk must be restored to its
// v1 content.
func TestEpic08_StateIntegration_HistoryAndRollback(t *testing.T) {
	f := bootStateFixture(t)
	target := filepath.Join(f.root, "rollback-target.txt")

	v1apply := func(content string) string {
		t.Helper()
		yaml := fmt.Sprintf("file:\n  %s:\n    state: present\n    content: %q\n", target, content)
		stream, err := f.client.ApplyState(t.Context(), &v1.ApplyStateRequest{
			YamlContent: []byte(yaml),
			Source:      "rollback-" + content + ".yaml",
			AgentId:     "agent-int",
		})
		if err != nil {
			t.Fatalf("ApplyState %q: %v", content, err)
		}
		ev := drainApplyStream(t, stream)
		if ev.terminal.GetStatus() != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
			t.Fatalf("apply %q: status=%v err=%q", content, ev.terminal.GetStatus(), ev.terminal.GetErrorMessage())
		}
		return ev.runID
	}

	v1RunID := v1apply("v1")
	if data, err := os.ReadFile(target); err != nil || string(data) != "v1" {
		t.Fatalf("post-v1 read: data=%q err=%v", string(data), err)
	}
	_ = v1apply("v2")
	if data, err := os.ReadFile(target); err != nil || string(data) != "v2" {
		t.Fatalf("post-v2 read: data=%q err=%v", string(data), err)
	}

	// History should list ≥ 2 completed runs.
	hist, err := f.client.GetStateHistory(t.Context(), &v1.GetStateHistoryRequest{
		AgentId: "agent-int",
		Status:  v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED,
	})
	if err != nil {
		t.Fatalf("GetStateHistory: %v", err)
	}
	if len(hist.GetRuns()) < 2 {
		t.Fatalf("history: %d runs, want >= 2", len(hist.GetRuns()))
	}

	// Rollback to v1.
	rbStream, err := f.client.RollbackState(t.Context(), &v1.RollbackStateRequest{
		RunId: v1RunID,
	})
	if err != nil {
		t.Fatalf("RollbackState: %v", err)
	}
	rb := drainRollbackStream(t, rbStream)
	if rb.terminal.GetStatus() != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
		t.Fatalf("rollback: status=%v err=%q", rb.terminal.GetStatus(), rb.terminal.GetErrorMessage())
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "v1" {
		t.Errorf("post-rollback read: data=%q err=%v; want %q", string(data), err, "v1")
	}
}

// ---- Test 4: Saga compensation re-applies prior state --------------

// Apply v1 (records history), then RunSaga directly on a [v2, fail]
// pair. The cmd step fails → compensation walks back, finds the
// prior decl in history, and re-applies it. The on-disk file must
// be restored to v1.
func TestEpic08_StateIntegration_SagaCompensation(t *testing.T) {
	f := bootStateFixture(t)
	target := filepath.Join(f.root, "saga-target.txt")

	// Step 1: apply v1 through the gRPC server so the SQLite history
	// store captures the prior state.
	v1yaml := fmt.Sprintf("file:\n  %s:\n    state: present\n    content: %q\n", target, "v1")
	v1Stream, err := f.client.ApplyState(t.Context(), &v1.ApplyStateRequest{
		YamlContent: []byte(v1yaml),
		Source:      "saga-v1.yaml",
		AgentId:     "agent-int",
	})
	if err != nil {
		t.Fatalf("ApplyState v1: %v", err)
	}
	if ev := drainApplyStream(t, v1Stream); ev.terminal.GetStatus() != v1.StateRunStatus_STATE_RUN_STATUS_COMPLETED {
		t.Fatalf("v1 apply: status=%v err=%q", ev.terminal.GetStatus(), ev.terminal.GetErrorMessage())
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "v1" {
		t.Fatalf("post-v1: data=%q err=%v", string(data), err)
	}

	// Step 2: construct the v2-then-fail batch and run it through
	// the saga path directly — ApplyState has no saga toggle on the
	// wire (it stays unary Run); the saga coordinator is the
	// programmatic surface.
	v2yaml := fmt.Sprintf(`file:
  %s:
    state: present
    content: "v2"
cmd:
  saga-fail:
    state: run
    command: "exit 1"
    creates: %s
    require:
      - file: %s
`, target, filepath.Join(f.root, "saga-never-created"), target)

	parsed, err := statemgmt.Parse([]byte(v2yaml))
	if err != nil {
		t.Fatalf("Parse v2: %v", err)
	}
	if err := statemgmt.NewValidator(f.registry).Validate(parsed); err != nil {
		t.Fatalf("Validate v2: %v", err)
	}
	ordered, err := statemgmt.NewResolver().Resolve(parsed)
	if err != nil {
		t.Fatalf("Resolve v2: %v", err)
	}

	runner := &statemgmt.Runner{Registry: f.registry}
	report, sagaErr := runner.RunSaga(t.Context(), ordered, statemgmt.SagaConfig{
		History: f.store,
		AgentID: "agent-int",
	})
	if sagaErr == nil {
		t.Fatal("RunSaga: want error from failing cmd, got nil")
	}
	if report == nil {
		t.Fatal("RunSaga: nil report")
	}

	// The file decl must have been compensated. With a prior state
	// in history, the saga re-applies it — the file content snaps
	// back to "v1".
	if data, err := os.ReadFile(target); err != nil {
		t.Fatalf("post-saga read %s: %v", target, err)
	} else if got := string(data); got != "v1" {
		t.Errorf("post-saga compensation: target=%q, want %q (rollback should have re-applied prior state)", got, "v1")
	}

	// And: report carries Compensated=true for at least one decl.
	var compCount int
	for _, r := range report.Results {
		if r.Compensated {
			compCount++
		}
	}
	if compCount == 0 {
		t.Error("saga report: no decls reported Compensated=true; expected >= 1")
	}
}

// ---- Test 5: Requisite cycle error carries the full path -----------

// A cycle YAML must be rejected at compile time with the full A → B
// → C → A path in the error so an operator can fix it. The proto
// surface is InvalidArgument from grpc_state_server.compile's
// resolver branch.
func TestEpic08_StateIntegration_RequisiteCycleErrorMessage(t *testing.T) {
	f := bootStateFixture(t)

	stream, err := f.client.ApplyState(t.Context(), &v1.ApplyStateRequest{
		YamlContent: f.loadFixture(t, "state-integration-cycle.yaml"),
		Source:      "cycle.yaml",
		AgentId:     "agent-int",
	})
	if err != nil {
		t.Fatalf("ApplyState (cycle): %v", err)
	}

	// The stream surfaces the InvalidArgument error on the first
	// Recv — the server rejects the compile before opening the run.
	_, recvErr := stream.Recv()
	if recvErr == nil {
		t.Fatal("cycle apply: want compile error, got nil")
	}
	msg := recvErr.Error()
	if !strings.Contains(msg, "cycle") {
		t.Errorf("cycle error message = %q; want to contain %q", msg, "cycle")
	}
	// The three decl IDs in the cycle must all appear so an operator
	// can grep them out of their YAML.
	cycleDecls := []string{
		"file:" + filepath.Join(f.root, "cycle-a"),
		"file:" + filepath.Join(f.root, "cycle-b"),
		"file:" + filepath.Join(f.root, "cycle-c"),
	}
	for _, id := range cycleDecls {
		if !strings.Contains(msg, id) {
			t.Errorf("cycle error %q missing decl %q", msg, id)
		}
	}
}
