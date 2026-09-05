// SPDX-License-Identifier: Apache-2.0

package saga

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// recordingStep is a Step that records every call (forward + compensate)
// onto a shared journal. Used to assert ordering.
type recordingStep struct {
	name     string
	action   func(ctx context.Context, data any) (any, error)
	compErr  error // if non-nil, Compensate returns this
	skipComp bool  // if true, Compensate is nil (no-op)
}

type journalEntry struct {
	Step    string
	Kind    string // "action" | "compensate"
	DataIn  any
	DataOut any
	Err     error
}

func buildSteps(j *[]journalEntry, ss ...recordingStep) []Step {
	out := make([]Step, len(ss))
	for i, s := range ss {
		s := s
		step := Step{
			Name: s.name,
			Action: func(ctx context.Context, data any) (any, error) {
				out, err := s.action(ctx, data)
				*j = append(*j, journalEntry{Step: s.name, Kind: "action", DataIn: data, DataOut: out, Err: err})
				return out, err
			},
		}
		if !s.skipComp {
			step.Compensate = func(_ context.Context, data any) error {
				*j = append(*j, journalEntry{Step: s.name, Kind: "compensate", DataIn: data, Err: s.compErr})
				return s.compErr
			}
		}
		out[i] = step
	}
	return out
}

// passthroughAction is the no-op forward action: thread data unchanged, no error.
func passthroughAction() func(context.Context, any) (any, error) {
	return func(_ context.Context, d any) (any, error) { return d, nil }
}

// failingAction is a forward action that errors with the given error.
func failingAction(err error) func(context.Context, any) (any, error) {
	return func(_ context.Context, d any) (any, error) { return d, err }
}

// fixedClock returns a deterministic clock that advances by 1s on each call.
func fixedClock() func() time.Time {
	t := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		t = t.Add(time.Second)
		return t
	}
}

// --- happy path -----------------------------------------------------

func TestRun_HappyPath_AllSucceed(t *testing.T) {
	t.Parallel()
	var j []journalEntry
	steps := buildSteps(&j,
		recordingStep{name: "a", action: passthroughAction()},
		recordingStep{name: "b", action: passthroughAction()},
		recordingStep{name: "c", action: passthroughAction()},
	)
	c := &Coordinator{Clock: fixedClock()}
	exec, err := c.Run(context.Background(), "happy", steps, "seed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec.Status != StatusCompleted {
		t.Errorf("Status = %s, want completed", exec.Status)
	}
	if exec.Data != "seed" {
		t.Errorf("Data = %v, want seed (passthrough)", exec.Data)
	}
	if exec.Error != nil {
		t.Errorf("Error = %v, want nil", exec.Error)
	}
	for i, sr := range exec.Steps {
		if sr.Status != StepCompleted {
			t.Errorf("step %d (%q) status = %s, want completed", i, sr.Name, sr.Status)
		}
		if sr.Compensated {
			t.Errorf("step %d compensated unexpectedly", i)
		}
	}
	// No compensation entries in the journal.
	for _, e := range j {
		if e.Kind == "compensate" {
			t.Errorf("compensation ran unexpectedly: %+v", e)
		}
	}
}

func TestRun_DataThreading(t *testing.T) {
	t.Parallel()
	steps := []Step{
		{Name: "+1", Action: func(_ context.Context, d any) (any, error) { return d.(int) + 1, nil }},
		{Name: "*2", Action: func(_ context.Context, d any) (any, error) { return d.(int) * 2, nil }},
		{Name: "+10", Action: func(_ context.Context, d any) (any, error) { return d.(int) + 10, nil }},
	}
	c := &Coordinator{}
	exec, err := c.Run(context.Background(), "math", steps, 3)
	if err != nil {
		t.Fatal(err)
	}
	// 3 → 4 → 8 → 18
	if exec.Data.(int) != 18 {
		t.Errorf("data threading = %v, want 18", exec.Data)
	}
}

func TestRun_EmptyStepsList(t *testing.T) {
	t.Parallel()
	c := &Coordinator{}
	exec, err := c.Run(context.Background(), "empty", nil, "x")
	if err != nil || exec.Status != StatusCompleted {
		t.Fatalf("empty saga: status=%s err=%v", exec.Status, err)
	}
	if exec.Data != "x" {
		t.Errorf("initial data should round-trip: %v", exec.Data)
	}
}

// --- failure + compensation -----------------------------------------

func TestRun_StepFailure_CompensatesInReverse(t *testing.T) {
	t.Parallel()
	var j []journalEntry
	boom := errors.New("boom")
	steps := buildSteps(&j,
		recordingStep{name: "a", action: passthroughAction()},
		recordingStep{name: "b", action: passthroughAction()},
		recordingStep{name: "c", action: failingAction(boom)},
		recordingStep{name: "d", action: passthroughAction()},
	)
	c := &Coordinator{}
	exec, err := c.Run(context.Background(), "boom", steps, "seed")
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
	if exec.Status != StatusFailed {
		t.Errorf("Status = %s, want failed", exec.Status)
	}
	if exec.Error != boom {
		t.Errorf("Execution.Error = %v, want %v", exec.Error, boom)
	}
	// Steps: a completed→compensated, b completed→compensated, c failed, d skipped.
	wantStatuses := []StepStatus{StepCompensated, StepCompensated, StepFailed, StepSkipped}
	for i, want := range wantStatuses {
		if exec.Steps[i].Status != want {
			t.Errorf("step %d (%q) status = %s, want %s", i, exec.Steps[i].Name, exec.Steps[i].Status, want)
		}
	}
	// Compensation walked b then a (reverse).
	var compOrder []string
	for _, e := range j {
		if e.Kind == "compensate" {
			compOrder = append(compOrder, e.Step)
		}
	}
	if want := []string{"b", "a"}; !reflect.DeepEqual(compOrder, want) {
		t.Errorf("compensation order = %v, want %v", compOrder, want)
	}
	// Step.Compensated flag is set on every step that got reverted.
	if !exec.Steps[0].Compensated || !exec.Steps[1].Compensated {
		t.Errorf("Compensated flag not set on rolled-back steps")
	}
	if exec.Steps[2].Compensated || exec.Steps[3].Compensated {
		t.Errorf("Compensated flag set on the failing/skipped step")
	}
}

func TestRun_CompensateFailure_AggregatesAndContinues(t *testing.T) {
	t.Parallel()
	var j []journalEntry
	boom := errors.New("boom")
	compErr := errors.New("compensate-bad")
	steps := buildSteps(&j,
		recordingStep{name: "a", action: passthroughAction()},                   // Compensate ok
		recordingStep{name: "b", action: passthroughAction(), compErr: compErr}, // Compensate errors
		recordingStep{name: "c", action: failingAction(boom)},
	)
	c := &Coordinator{}
	exec, err := c.Run(context.Background(), "aborted", steps, "seed")
	if exec.Status != StatusAborted {
		t.Errorf("Status = %s, want aborted (one compensate failed)", exec.Status)
	}
	if !errors.Is(err, boom) {
		t.Errorf("err should wrap %v, got %v", boom, err)
	}
	if !errors.Is(err, compErr) {
		t.Errorf("err should wrap %v, got %v", compErr, err)
	}
	if len(exec.CompensateErrors) != 1 {
		t.Errorf("CompensateErrors len = %d, want 1", len(exec.CompensateErrors))
	}
	// Step a should still have run its Compensate (aggregate-and-continue rule).
	var compOrder []string
	for _, e := range j {
		if e.Kind == "compensate" {
			compOrder = append(compOrder, e.Step)
		}
	}
	if want := []string{"b", "a"}; !reflect.DeepEqual(compOrder, want) {
		t.Errorf("compensation walk = %v, want both b then a (continue past b's failure)", compOrder)
	}
	// Per-step statuses
	if exec.Steps[0].Status != StepCompensated {
		t.Errorf("step a status = %s, want compensated", exec.Steps[0].Status)
	}
	if exec.Steps[1].Status != StepCompensateFailed {
		t.Errorf("step b status = %s, want compensate-failed", exec.Steps[1].Status)
	}
	if exec.Steps[1].CompensateError != compErr {
		t.Errorf("step b CompensateError = %v, want %v", exec.Steps[1].CompensateError, compErr)
	}
}

func TestRun_NilCompensate_IsNoOp(t *testing.T) {
	t.Parallel()
	var j []journalEntry
	boom := errors.New("boom")
	steps := buildSteps(&j,
		recordingStep{name: "a", action: passthroughAction(), skipComp: true}, // Compensate is nil
		recordingStep{name: "b", action: failingAction(boom)},
	)
	c := &Coordinator{}
	exec, err := c.Run(context.Background(), "nil-comp", steps, "seed")
	if exec.Status != StatusFailed {
		t.Errorf("Status = %s, want failed (nil Compensate is no-op success)", exec.Status)
	}
	if err != boom {
		t.Errorf("err = %v, want %v", err, boom)
	}
	if exec.Steps[0].Status != StepCompensated {
		t.Errorf("step a (nil Compensate) status = %s, want compensated", exec.Steps[0].Status)
	}
	if !exec.Steps[0].Compensated {
		t.Errorf("step a Compensated flag should be set even for nil Compensate")
	}
}

func TestRun_FirstStepFails_NoCompensation(t *testing.T) {
	t.Parallel()
	var j []journalEntry
	boom := errors.New("boom")
	steps := buildSteps(&j,
		recordingStep{name: "a", action: failingAction(boom)},
		recordingStep{name: "b", action: passthroughAction()},
	)
	c := &Coordinator{}
	exec, err := c.Run(context.Background(), "first-fail", steps, "seed")
	if err != boom || exec.Status != StatusFailed {
		t.Errorf("first-step-fail: err=%v status=%s", err, exec.Status)
	}
	if exec.Steps[0].Status != StepFailed {
		t.Errorf("step a status = %s, want failed", exec.Steps[0].Status)
	}
	if exec.Steps[1].Status != StepSkipped {
		t.Errorf("step b status = %s, want skipped", exec.Steps[1].Status)
	}
	for _, e := range j {
		if e.Kind == "compensate" {
			t.Errorf("compensation ran when no steps had completed: %+v", e)
		}
	}
}

// --- context cancellation -------------------------------------------

func TestRun_ContextCancelledMidForward_TreatedAsFailure(t *testing.T) {
	t.Parallel()
	var j []journalEntry
	ctx, cancel := context.WithCancel(context.Background())
	steps := buildSteps(&j,
		recordingStep{name: "a", action: passthroughAction()},
		recordingStep{name: "b", action: func(_ context.Context, d any) (any, error) {
			cancel() // cancel inside step b's Action
			return d, nil
		}},
		recordingStep{name: "c", action: passthroughAction()}, // would have been canceled before this runs
	)
	c := &Coordinator{}
	exec, err := c.Run(ctx, "ctx", steps, "seed")
	if err == nil {
		t.Fatal("expected an error from ctx cancel")
	}
	if exec.Status != StatusFailed {
		t.Errorf("Status = %s, want failed", exec.Status)
	}
	if !errors.Is(exec.Error, context.Canceled) {
		t.Errorf("Execution.Error = %v, want context.Canceled", exec.Error)
	}
	// Steps a and b completed; the cancel is detected before c.
	// Compensation walks b then a (both have Compensates from buildSteps).
	var compOrder []string
	for _, e := range j {
		if e.Kind == "compensate" {
			compOrder = append(compOrder, e.Step)
		}
	}
	if want := []string{"b", "a"}; !reflect.DeepEqual(compOrder, want) {
		t.Errorf("compensation order under ctx cancel = %v, want %v", compOrder, want)
	}
}

func TestRun_ContextCancelledDoesntStopCompensation(t *testing.T) {
	t.Parallel()
	// Even when the parent ctx is canceled by the time we get to the
	// compensation walk, every Compensate must still run.
	var compRan []string
	ctx, cancel := context.WithCancel(context.Background())
	boom := errors.New("boom")
	steps := []Step{
		{Name: "a", Action: passthroughAction(), Compensate: func(_ context.Context, _ any) error {
			compRan = append(compRan, "a")
			return nil
		}},
		{Name: "b", Action: passthroughAction(), Compensate: func(_ context.Context, _ any) error {
			compRan = append(compRan, "b")
			return nil
		}},
		{Name: "c", Action: func(_ context.Context, d any) (any, error) {
			cancel() // cancel ctx before failing
			return d, boom
		}},
	}
	c := &Coordinator{}
	exec, _ := c.Run(ctx, "x", steps, nil)
	if exec.Status != StatusFailed {
		t.Errorf("status = %s, want failed", exec.Status)
	}
	if want := []string{"b", "a"}; !reflect.DeepEqual(compRan, want) {
		t.Errorf("compensation under canceled ctx = %v, want %v (must run despite cancel)", compRan, want)
	}
}

// --- log integration ------------------------------------------------

func TestRun_LogReceivesPendingAndTerminalStates(t *testing.T) {
	t.Parallel()
	log := NewInMemoryLog()
	c := &Coordinator{Log: log}
	steps := []Step{
		{Name: "ok", Action: passthroughAction()},
	}
	exec, err := c.Run(context.Background(), "logged", steps, "x")
	if err != nil {
		t.Fatal(err)
	}
	got, err := log.GetExecution(context.Background(), exec.ID)
	if err != nil || got.Status != StatusCompleted {
		t.Errorf("log GetExecution: %+v err=%v", got, err)
	}
	list, err := log.ListExecutions(context.Background())
	if err != nil || len(list) != 1 {
		t.Errorf("ListExecutions: len=%d err=%v", len(list), err)
	}
}

func TestStepStatus_String(t *testing.T) {
	t.Parallel()
	for s, want := range map[StepStatus]string{
		StepPending:          "pending",
		StepRunning:          "running",
		StepCompleted:        "completed",
		StepFailed:           "failed",
		StepCompensated:      "compensated",
		StepCompensateFailed: "compensate-failed",
		StepSkipped:          "skipped",
		StepStatus(99):       "StepStatus(99)",
	} {
		if got := s.String(); got != want {
			t.Errorf("StepStatus(%d).String() = %q, want %q", int(s), got, want)
		}
	}
}

func TestCoordinator_NilClock_FallsBackToTimeNow(t *testing.T) {
	t.Parallel()
	c := &Coordinator{} // Clock = nil
	before := time.Now()
	exec, _ := c.Run(context.Background(), "now", []Step{{Name: "a", Action: passthroughAction()}}, nil)
	after := time.Now()
	if exec.StartedAt.Before(before) || exec.StartedAt.After(after) {
		t.Errorf("StartedAt %v outside [%v, %v]", exec.StartedAt, before, after)
	}
}

// --- API conformance / smoke ----------------------------------------

func TestExecution_IDIsAssigned(t *testing.T) {
	t.Parallel()
	c := &Coordinator{}
	exec, _ := c.Run(context.Background(), "id", nil, nil)
	if exec.ID == "" {
		t.Error("Execution.ID should be auto-assigned")
	}
}

func TestRun_DurationRecorded(t *testing.T) {
	t.Parallel()
	c := &Coordinator{}
	exec, _ := c.Run(context.Background(), "dur",
		[]Step{{Name: "wait", Action: func(_ context.Context, d any) (any, error) {
			time.Sleep(2 * time.Millisecond)
			return d, nil
		}}}, nil)
	if exec.Steps[0].Duration <= 0 {
		t.Errorf("Duration should be positive, got %v", exec.Steps[0].Duration)
	}
}

func TestRun_ErrorJoinedFromCompensationFailures(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	c1 := errors.New("c1")
	c2 := errors.New("c2")
	steps := []Step{
		{Name: "a", Action: passthroughAction(), Compensate: func(_ context.Context, _ any) error { return c1 }},
		{Name: "b", Action: passthroughAction(), Compensate: func(_ context.Context, _ any) error { return c2 }},
		{Name: "c", Action: failingAction(boom)},
	}
	c := &Coordinator{}
	_, err := c.Run(context.Background(), "joined", steps, nil)
	for _, want := range []error{boom, c1, c2} {
		if !errors.Is(err, want) {
			t.Errorf("returned err should wrap %v, got %v", want, err)
		}
	}
}

// Sanity: status strings are stable for documentation cross-refs.
func TestExecutionStatus_Constants(t *testing.T) {
	t.Parallel()
	for s, want := range map[ExecutionStatus]string{
		StatusPending:   "pending",
		StatusRunning:   "running",
		StatusCompleted: "completed",
		StatusFailed:    "failed",
		StatusAborted:   "aborted",
	} {
		if string(s) != want {
			t.Errorf("status %q != %q", s, want)
		}
	}
	// Avoid unused-import warnings in this file's edge cases.
	_ = fmt.Sprint
}
