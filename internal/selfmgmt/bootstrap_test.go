// SPDX-License-Identifier: Apache-2.0

package selfmgmt

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.keystone-core.io/keystone-core/pkg/statemachine"
)

// recordingHandler is a configurable [PhaseHandler] test double.
// Each method appends its name to `calls`; if `failAt` matches, the
// method returns failErr instead. rollbackErr is returned from
// Rollback. rollbackFailedAt captures the failedAt argument for
// assertion.
type recordingHandler struct {
	calls            []string
	failAt           string // "detect" | "configure" | "validate" | "install" | "blueprints" | "verify" | ""
	failErr          error
	rollbackErr      error
	rollbackFailedAt BootstrapState
}

func (h *recordingHandler) record(name string) error {
	h.calls = append(h.calls, name)
	if h.failAt == name {
		return h.failErr
	}
	return nil
}

func (h *recordingHandler) Detect(_ context.Context, _ *SeedConfig) error {
	return h.record("detect")
}
func (h *recordingHandler) Configure(_ context.Context, _ *SeedConfig) error {
	return h.record("configure")
}
func (h *recordingHandler) Validate(_ context.Context, _ *SeedConfig) error {
	return h.record("validate")
}
func (h *recordingHandler) Install(_ context.Context, _ *SeedConfig) error {
	return h.record("install")
}
func (h *recordingHandler) ApplyBlueprints(_ context.Context, _ *SeedConfig) error {
	return h.record("blueprints")
}
func (h *recordingHandler) Verify(_ context.Context, _ *SeedConfig) error {
	return h.record("verify")
}
func (h *recordingHandler) Rollback(_ context.Context, _ *SeedConfig, failedAt BootstrapState) error {
	h.calls = append(h.calls, "rollback")
	h.rollbackFailedAt = failedAt
	return h.rollbackErr
}

func goodSeed(t *testing.T) *SeedConfig {
	t.Helper()
	s := &SeedConfig{
		Mode:        SeedModeDevelopment,
		ClusterName: "test-cluster",
		NodeRole:    SeedNodeRoleSeed,
		Storage:     SeedStorage{Driver: SeedStorageSQLite, DSN: "./data/test.db"},
		NATS:        SeedNATS{Mode: SeedNATSEmbedded},
		TLSStrategy: SeedTLSSelfSigned,
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return s
}

func TestWithLogger(t *testing.T) {
	t.Run("nil is ignored", func(t *testing.T) {
		mgr, err := NewBootstrapManager(goodSeed(t), &recordingHandler{}, WithLogger(nil))
		if err != nil {
			t.Fatalf("NewBootstrapManager: %v", err)
		}
		if mgr.logger == nil {
			t.Fatal("logger left nil")
		}
	})
}

func TestNewBootstrapManager_NilArgs(t *testing.T) {
	t.Run("nil seed", func(t *testing.T) {
		_, err := NewBootstrapManager(nil, &recordingHandler{})
		if err == nil {
			t.Fatal("want error")
		}
	})
	t.Run("nil handler", func(t *testing.T) {
		_, err := NewBootstrapManager(goodSeed(t), nil)
		if err == nil {
			t.Fatal("want error")
		}
	})
}

// TestMachineTopology asserts the FSM has every documented edge.
// A regression in newMachine's transition list would silently change
// the run-loop's reachability invariants.
func TestMachineTopology(t *testing.T) {
	type edge struct {
		from  BootstrapState
		event BootstrapEvent
	}
	required := []edge{
		{StateNotStarted, EventStartDetect},
		{StateDetecting, EventDetectDone}, {StateDetecting, EventDetectFail},
		{StateDetected, EventStartConfigure},
		{StateConfiguring, EventConfigureDone}, {StateConfiguring, EventConfigureFail},
		{StateConfigured, EventStartValidate},
		{StateValidating, EventValidateDone}, {StateValidating, EventValidateFail},
		{StateValidated, EventStartInstall},
		{StateInstalling, EventInstallDone}, {StateInstalling, EventInstallFail},
		{StateInstalled, EventStartBlueprints},
		{StateApplyingBlueprints, EventBlueprintsDone}, {StateApplyingBlueprints, EventBlueprintsFail},
		{StateBlueprintsApplied, EventStartVerify},
		{StateVerifying, EventVerifyDone}, {StateVerifying, EventVerifyFail},
		{StateFailed, EventRollback},
	}
	cp := statemachine.NewMemoryCheckpointer[BootstrapState, BootstrapEvent]()
	for _, e := range required {
		m, err := newMachine(e.from, cp)
		if err != nil {
			t.Fatalf("newMachine(%s): %v", e.from, err)
		}
		if !m.Can(e.event) {
			t.Errorf("from %s: event %s missing", e.from, e.event)
		}
	}
}

func TestBootstrapState_IsTerminal(t *testing.T) {
	terminal := map[BootstrapState]bool{
		StateVerified:   true,
		StateRolledBack: true,
	}
	all := []BootstrapState{
		StateNotStarted, StateDetecting, StateDetected, StateConfiguring,
		StateConfigured, StateValidating, StateValidated, StateInstalling,
		StateInstalled, StateApplyingBlueprints, StateBlueprintsApplied,
		StateVerifying, StateVerified, StateFailed, StateRolledBack,
	}
	for _, s := range all {
		if got, want := s.IsTerminal(), terminal[s]; got != want {
			t.Errorf("%s.IsTerminal() = %v, want %v", s, got, want)
		}
	}
}

func TestRun_HappyPath(t *testing.T) {
	h := &recordingHandler{}
	mgr, err := NewBootstrapManager(goodSeed(t), h)
	if err != nil {
		t.Fatalf("NewBootstrapManager: %v", err)
	}
	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := mgr.State(), StateVerified; got != want {
		t.Errorf("State = %s, want %s", got, want)
	}
	wantCalls := []string{"detect", "configure", "validate", "install", "blueprints", "verify"}
	if got := h.calls; !equalStrings(got, wantCalls) {
		t.Errorf("calls = %v, want %v", got, wantCalls)
	}
	// 12 transitions: 6 start_X + 6 X_done.
	if got := len(mgr.History()); got != 12 {
		t.Errorf("history len = %d, want 12", got)
	}
}

func TestRun_IdempotentReentry(t *testing.T) {
	h := &recordingHandler{}
	mgr, err := NewBootstrapManager(goodSeed(t), h)
	if err != nil {
		t.Fatalf("NewBootstrapManager: %v", err)
	}
	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run #1: %v", err)
	}
	firstCalls := len(h.calls)

	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run #2: %v", err)
	}
	if got := len(h.calls); got != firstCalls {
		t.Errorf("handler invoked again on re-run: calls=%v (first=%d)", h.calls, firstCalls)
	}
	if got, want := mgr.State(), StateVerified; got != want {
		t.Errorf("State after re-run = %s, want %s", got, want)
	}
}

func TestRun_PhaseFailureAutoRollback(t *testing.T) {
	cases := []struct {
		failAt         string
		wantFailedAt   BootstrapState
		wantPriorCalls []string
	}{
		{failAt: "detect", wantFailedAt: StateDetecting, wantPriorCalls: nil},
		{failAt: "configure", wantFailedAt: StateConfiguring, wantPriorCalls: []string{"detect"}},
		{failAt: "validate", wantFailedAt: StateValidating, wantPriorCalls: []string{"detect", "configure"}},
		{failAt: "install", wantFailedAt: StateInstalling, wantPriorCalls: []string{"detect", "configure", "validate"}},
		{failAt: "blueprints", wantFailedAt: StateApplyingBlueprints, wantPriorCalls: []string{"detect", "configure", "validate", "install"}},
		{failAt: "verify", wantFailedAt: StateVerifying, wantPriorCalls: []string{"detect", "configure", "validate", "install", "blueprints"}},
	}
	for _, tc := range cases {
		t.Run(tc.failAt, func(t *testing.T) {
			phaseErr := fmt.Errorf("simulated %s failure", tc.failAt)
			h := &recordingHandler{failAt: tc.failAt, failErr: phaseErr}
			mgr, err := NewBootstrapManager(goodSeed(t), h)
			if err != nil {
				t.Fatalf("NewBootstrapManager: %v", err)
			}
			err = mgr.Run(context.Background())
			if err == nil {
				t.Fatal("Run: want error, got nil")
			}
			if !errors.Is(err, ErrBootstrap) {
				t.Errorf("err = %v, want errors.Is(ErrBootstrap)", err)
			}
			if !errors.Is(err, phaseErr) {
				t.Errorf("err = %v, want errors.Is(phaseErr)", err)
			}
			if got, want := mgr.State(), StateRolledBack; got != want {
				t.Errorf("State = %s, want %s", got, want)
			}
			if got, want := h.rollbackFailedAt, tc.wantFailedAt; got != want {
				t.Errorf("Rollback failedAt = %s, want %s", got, want)
			}
			want := append(append([]string{}, tc.wantPriorCalls...), tc.failAt, "rollback")
			if !equalStrings(h.calls, want) {
				t.Errorf("calls = %v, want %v", h.calls, want)
			}
		})
	}
}

func TestRun_AutoRollbackDisabled(t *testing.T) {
	phaseErr := errors.New("simulated configure failure")
	h := &recordingHandler{failAt: "configure", failErr: phaseErr}
	mgr, err := NewBootstrapManager(goodSeed(t), h, WithAutoRollback(false))
	if err != nil {
		t.Fatalf("NewBootstrapManager: %v", err)
	}
	err = mgr.Run(context.Background())
	if err == nil {
		t.Fatal("Run: want error")
	}
	if !errors.Is(err, ErrBootstrap) || !errors.Is(err, phaseErr) {
		t.Errorf("err = %v, want ErrBootstrap wrapping phaseErr", err)
	}
	if got, want := mgr.State(), StateFailed; got != want {
		t.Errorf("State = %s, want %s", got, want)
	}
	for _, c := range h.calls {
		if c == "rollback" {
			t.Fatalf("Rollback called despite AutoRollback=false: calls=%v", h.calls)
		}
	}

	// Re-running on a Failed machine must not silently retry.
	err = mgr.Run(context.Background())
	if err == nil || !errors.Is(err, ErrBootstrap) {
		t.Errorf("re-run on Failed: err = %v, want ErrBootstrap", err)
	}
}

func TestRun_RollbackHandlerFailure(t *testing.T) {
	phaseErr := errors.New("install failed")
	rollbackErr := errors.New("rollback failed too")
	h := &recordingHandler{failAt: "install", failErr: phaseErr, rollbackErr: rollbackErr}
	mgr, err := NewBootstrapManager(goodSeed(t), h)
	if err != nil {
		t.Fatalf("NewBootstrapManager: %v", err)
	}
	err = mgr.Run(context.Background())
	if err == nil {
		t.Fatal("Run: want error")
	}
	if !errors.Is(err, phaseErr) {
		t.Errorf("err missing phaseErr: %v", err)
	}
	if !errors.Is(err, rollbackErr) {
		t.Errorf("err missing rollbackErr: %v", err)
	}
	// State stays at Failed because the rollback event was not fired.
	if got, want := mgr.State(), StateFailed; got != want {
		t.Errorf("State = %s, want %s", got, want)
	}
}

// TestRun_ResumeFromCheckpoint constructs a manager, drives it
// partway via a shared checkpointer, then builds a SECOND manager
// against the same checkpointer to assert the resume picks up where
// the first stopped without re-invoking already-done phases.
func TestRun_ResumeFromCheckpoint(t *testing.T) {
	cp := statemachine.NewMemoryCheckpointer[BootstrapState, BootstrapEvent]()
	seed := goodSeed(t)

	// First run: fail at validate, AutoRollback=false to freeze at Failed
	// — but that would not test mid-success resume. Instead, fail at
	// validate WITH AutoRollback=false and then build a clean second
	// manager against a fresh checkpoint that we manually advance to
	// Configured.
	firstHandler := &recordingHandler{}
	mgr1, err := NewBootstrapManager(seed, firstHandler, WithCheckpointer(cp))
	if err != nil {
		t.Fatalf("NewBootstrapManager #1: %v", err)
	}
	// Drive manually to Configured by firing the events directly via
	// a helper machine sharing the checkpointer. Simulate a crash
	// just after Configure completed.
	helper, err := newMachine(StateNotStarted, cp)
	if err != nil {
		t.Fatalf("helper machine: %v", err)
	}
	for _, ev := range []BootstrapEvent{
		EventStartDetect, EventDetectDone,
		EventStartConfigure, EventConfigureDone,
	} {
		if err := helper.Fire(context.Background(), ev); err != nil {
			t.Fatalf("helper fire %s: %v", ev, err)
		}
	}
	if err := helper.Checkpoint(context.Background()); err != nil {
		t.Fatalf("helper checkpoint: %v", err)
	}

	_ = mgr1 // first manager not used for the run after checkpoint seeding

	// Second manager: resumes from Configured.
	secondHandler := &recordingHandler{}
	mgr2, err := NewBootstrapManager(seed, secondHandler, WithCheckpointer(cp))
	if err != nil {
		t.Fatalf("NewBootstrapManager #2: %v", err)
	}
	if err := mgr2.Run(context.Background()); err != nil {
		t.Fatalf("Run #2: %v", err)
	}
	if got, want := mgr2.State(), StateVerified; got != want {
		t.Errorf("State = %s, want %s", got, want)
	}
	// Detect + Configure were already done; only validate..verify should run.
	wantCalls := []string{"validate", "install", "blueprints", "verify"}
	if got := secondHandler.calls; !equalStrings(got, wantCalls) {
		t.Errorf("calls = %v, want %v", got, wantCalls)
	}
}

// TestRun_ResumeMidPhase covers the mid-phase resume case: machine
// checkpointed inside a transient state (Configuring) before the
// done/fail event fired. The handler MUST be called once on resume —
// this is the idempotency contract documented on PhaseHandler.
func TestRun_ResumeMidPhase(t *testing.T) {
	cp := statemachine.NewMemoryCheckpointer[BootstrapState, BootstrapEvent]()
	seed := goodSeed(t)

	helper, err := newMachine(StateNotStarted, cp)
	if err != nil {
		t.Fatalf("helper machine: %v", err)
	}
	for _, ev := range []BootstrapEvent{
		EventStartDetect, EventDetectDone, EventStartConfigure,
	} {
		if err := helper.Fire(context.Background(), ev); err != nil {
			t.Fatalf("helper fire %s: %v", ev, err)
		}
	}
	if err := helper.Checkpoint(context.Background()); err != nil {
		t.Fatalf("helper checkpoint: %v", err)
	}
	if got, want := helper.Current(), StateConfiguring; got != want {
		t.Fatalf("helper Current = %s, want %s", got, want)
	}

	h := &recordingHandler{}
	mgr, err := NewBootstrapManager(seed, h, WithCheckpointer(cp))
	if err != nil {
		t.Fatalf("NewBootstrapManager: %v", err)
	}
	if err := mgr.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got, want := mgr.State(), StateVerified; got != want {
		t.Errorf("State = %s, want %s", got, want)
	}
	// Mid-phase resume must NOT re-call Detect; must call Configure
	// (idempotent contract); then run remaining phases.
	wantCalls := []string{"configure", "validate", "install", "blueprints", "verify"}
	if !equalStrings(h.calls, wantCalls) {
		t.Errorf("calls = %v, want %v", h.calls, wantCalls)
	}
}

// faultyCheckpointer fails Save after the Nth call. Lets a test
// exercise the fireAndCheckpoint failure path without needing a real
// durable backend.
type faultyCheckpointer struct {
	inner    statemachine.Checkpointer[BootstrapState, BootstrapEvent]
	failAfter int
	saves     int
	saveErr   error
	loadErr   error
}

func (c *faultyCheckpointer) Save(ctx context.Context, snap statemachine.Snapshot[BootstrapState, BootstrapEvent]) error {
	c.saves++
	if c.saves > c.failAfter {
		return c.saveErr
	}
	return c.inner.Save(ctx, snap)
}

func (c *faultyCheckpointer) Load(ctx context.Context) (statemachine.Snapshot[BootstrapState, BootstrapEvent], bool, error) {
	if c.loadErr != nil {
		return statemachine.Snapshot[BootstrapState, BootstrapEvent]{}, false, c.loadErr
	}
	return c.inner.Load(ctx)
}

func TestRun_CheckpointSaveError(t *testing.T) {
	saveErr := errors.New("disk full")
	cp := &faultyCheckpointer{
		inner:     statemachine.NewMemoryCheckpointer[BootstrapState, BootstrapEvent](),
		failAfter: 1, // first save succeeds (after start_detect); second errors
		saveErr:   saveErr,
	}
	h := &recordingHandler{}
	mgr, err := NewBootstrapManager(goodSeed(t), h, WithCheckpointer(cp))
	if err != nil {
		t.Fatalf("NewBootstrapManager: %v", err)
	}
	err = mgr.Run(context.Background())
	if err == nil {
		t.Fatal("Run: want error")
	}
	if !errors.Is(err, saveErr) {
		t.Errorf("err = %v, want errors.Is(saveErr)", err)
	}
}

func TestRun_RestoreError(t *testing.T) {
	loadErr := errors.New("checkpoint corrupted")
	cp := &faultyCheckpointer{
		inner:   statemachine.NewMemoryCheckpointer[BootstrapState, BootstrapEvent](),
		loadErr: loadErr,
	}
	mgr, err := NewBootstrapManager(goodSeed(t), &recordingHandler{}, WithCheckpointer(cp))
	if err != nil {
		t.Fatalf("NewBootstrapManager: %v", err)
	}
	err = mgr.Run(context.Background())
	if err == nil {
		t.Fatal("Run: want error")
	}
	if !errors.Is(err, loadErr) {
		t.Errorf("err = %v, want errors.Is(loadErr)", err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
