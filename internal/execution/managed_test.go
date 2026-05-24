// SPDX-License-Identifier: Apache-2.0

package execution

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// scriptedExecutor returns canned ExecuteResults in order. It records
// the number of Execute calls so retry coverage can assert how far we
// progressed before terminating.
type scriptedExecutor struct {
	mu      sync.Mutex
	results []ExecuteResult
	calls   int
	// preCall, when non-nil, runs before each Execute returns. Used to
	// drive context cancellation mid-attempt.
	preCall func(ctx context.Context, attempt int)
}

func (e *scriptedExecutor) Execute(ctx context.Context, _ ExecuteRequest) ExecuteResult {
	e.mu.Lock()
	idx := e.calls
	e.calls++
	e.mu.Unlock()
	if e.preCall != nil {
		e.preCall(ctx, idx+1)
	}
	if idx >= len(e.results) {
		return ExecuteResult{Error: "scriptedExecutor: out of scripted results"}
	}
	return e.results[idx]
}

// fakeSleep replaces ctx-aware sleep with an immediate return. Tests
// that need to assert ctx-respect override per-test.
func fakeSleep(_ context.Context, _ time.Duration) error { return nil }

// recordedCallbacks captures every callback invocation in a single
// slice for ordering assertions.
type recordedCallbacks struct {
	mu   sync.Mutex
	logs []string
}

func (r *recordedCallbacks) log(s string) {
	r.mu.Lock()
	r.logs = append(r.logs, s)
	r.mu.Unlock()
}

func (r *recordedCallbacks) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.logs))
	copy(out, r.logs)
	return out
}

func (r *recordedCallbacks) callbacks() Callbacks {
	return Callbacks{
		OnStarted:   func() { r.log("started") },
		OnCompleted: func(_ ExecuteResult) { r.log("completed") },
		OnFailed:    func(_ ExecuteResult) { r.log("failed") },
		OnTimeout:   func(_ ExecuteResult) { r.log("timeout") },
		OnCancelled: func() { r.log("cancelled") },
		OnRetrying:  func(n int, _ ExecuteResult, _ time.Duration) { r.log("retrying:" + itoa(n)) },
		OnRetry:     func(n int) { r.log("retry:" + itoa(n)) },
	}
}

func itoa(n int) string {
	switch n {
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	case 4:
		return "4"
	default:
		return "n"
	}
}

func TestRun_HappyPath(t *testing.T) {
	t.Parallel()

	exec := &scriptedExecutor{results: []ExecuteResult{{ExitCode: 0}}}
	rec := &recordedCallbacks{}
	me, err := New(Config{Executor: exec, Callbacks: rec.callbacks(), Sleep: fakeSleep})
	if err != nil {
		t.Fatal(err)
	}
	state, res := me.Run(context.Background(), ExecuteRequest{Command: "true"})
	if state != StateCompleted {
		t.Errorf("state = %v, want COMPLETED", state)
	}
	if !res.Succeeded() {
		t.Errorf("result not succeeded: %+v", res)
	}
	want := []string{"started", "completed"}
	if got := rec.snapshot(); !equal(got, want) {
		t.Errorf("callbacks = %v, want %v", got, want)
	}
}

func TestRun_FailedNoRetry(t *testing.T) {
	t.Parallel()

	exec := &scriptedExecutor{results: []ExecuteResult{{ExitCode: 1}}}
	rec := &recordedCallbacks{}
	me, _ := New(Config{Executor: exec, Callbacks: rec.callbacks(), Sleep: fakeSleep})
	state, res := me.Run(context.Background(), ExecuteRequest{})
	if state != StateFailed {
		t.Errorf("state = %v, want FAILED", state)
	}
	if res.ExitCode != 1 {
		t.Errorf("ExitCode = %d", res.ExitCode)
	}
	want := []string{"started", "failed"}
	if got := rec.snapshot(); !equal(got, want) {
		t.Errorf("callbacks = %v, want %v", got, want)
	}
}

func TestRun_RetryUntilSuccess(t *testing.T) {
	t.Parallel()

	exec := &scriptedExecutor{results: []ExecuteResult{
		{ExitCode: 1},
		{ExitCode: 0},
	}}
	rec := &recordedCallbacks{}
	me, _ := New(Config{
		Executor:  exec,
		Callbacks: rec.callbacks(),
		Retry:     RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Millisecond},
		Sleep:     fakeSleep,
	})
	state, res := me.Run(context.Background(), ExecuteRequest{})
	if state != StateCompleted {
		t.Errorf("state = %v, want COMPLETED", state)
	}
	if exec.calls != 2 {
		t.Errorf("calls = %d, want 2", exec.calls)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d", res.ExitCode)
	}
	want := []string{"started", "failed", "retrying:2", "retry:2", "completed"}
	if got := rec.snapshot(); !equal(got, want) {
		t.Errorf("callbacks = %v, want %v", got, want)
	}
}

func TestRun_ExhaustRetries(t *testing.T) {
	t.Parallel()

	exec := &scriptedExecutor{results: []ExecuteResult{
		{ExitCode: 1}, {ExitCode: 1}, {ExitCode: 1},
	}}
	rec := &recordedCallbacks{}
	me, _ := New(Config{
		Executor:  exec,
		Callbacks: rec.callbacks(),
		Retry:     RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Millisecond},
		Sleep:     fakeSleep,
	})
	state, _ := me.Run(context.Background(), ExecuteRequest{})
	if state != StateFailed {
		t.Errorf("state = %v, want FAILED", state)
	}
	if exec.calls != 3 {
		t.Errorf("calls = %d, want 3", exec.calls)
	}
	want := []string{
		"started", "failed",
		"retrying:2", "retry:2", "failed",
		"retrying:3", "retry:3", "failed",
	}
	if got := rec.snapshot(); !equal(got, want) {
		t.Errorf("callbacks = %v, want %v", got, want)
	}
}

func TestRun_PerCallTimeout(t *testing.T) {
	t.Parallel()

	exec := &scriptedExecutor{results: []ExecuteResult{{TimedOut: true, ExitCode: -1}}}
	rec := &recordedCallbacks{}
	me, _ := New(Config{Executor: exec, Callbacks: rec.callbacks(), Sleep: fakeSleep})
	state, res := me.Run(context.Background(), ExecuteRequest{})
	if state != StateTimeout {
		t.Errorf("state = %v, want TIMEOUT", state)
	}
	if !res.TimedOut {
		t.Error("result.TimedOut should be true")
	}
	want := []string{"started", "timeout"}
	if got := rec.snapshot(); !equal(got, want) {
		t.Errorf("callbacks = %v, want %v", got, want)
	}
}

func TestRun_CtxDeadlineDuringExec(t *testing.T) {
	t.Parallel()

	// Pre-cancel via DeadlineExceeded so handleCtxFire routes to
	// TIMEOUT regardless of the script's outcome.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	exec := &scriptedExecutor{results: []ExecuteResult{{ExitCode: 0}}}
	rec := &recordedCallbacks{}
	me, _ := New(Config{Executor: exec, Callbacks: rec.callbacks(), Sleep: fakeSleep})
	state, _ := me.Run(ctx, ExecuteRequest{})
	if state != StateTimeout {
		t.Errorf("state = %v, want TIMEOUT", state)
	}
	if got := rec.snapshot(); len(got) != 1 || got[0] != "timeout" {
		t.Errorf("callbacks = %v, want [timeout]", got)
	}
	if exec.calls != 0 {
		t.Errorf("executor called %d times, want 0 (ctx pre-fired)", exec.calls)
	}
}

func TestRun_CancelMidAttempt(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exec := &scriptedExecutor{
		results: []ExecuteResult{{ExitCode: 0}},
		preCall: func(_ context.Context, _ int) { cancel() },
	}
	rec := &recordedCallbacks{}
	me, _ := New(Config{Executor: exec, Callbacks: rec.callbacks(), Sleep: fakeSleep})
	state, _ := me.Run(ctx, ExecuteRequest{})
	if state != StateCancelled {
		t.Errorf("state = %v, want CANCELLED", state)
	}
	if got := rec.snapshot(); len(got) < 2 || got[len(got)-1] != "cancelled" {
		t.Errorf("callbacks = %v, want last=cancelled", got)
	}
}

func TestRun_CancelDuringBackoff(t *testing.T) {
	t.Parallel()

	exec := &scriptedExecutor{results: []ExecuteResult{{ExitCode: 1}}}
	rec := &recordedCallbacks{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Sleep cancels the parent ctx, simulating "operator cancelled
	// while we waited to retry."
	me, err := New(Config{
		Executor:  exec,
		Callbacks: rec.callbacks(),
		Retry:     RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Millisecond},
		Sleep: func(_ context.Context, _ time.Duration) error {
			cancel()
			return context.Canceled
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	state, _ := me.Run(ctx, ExecuteRequest{})
	if state != StateCancelled {
		t.Errorf("state = %v, want CANCELLED", state)
	}
	logs := rec.snapshot()
	if !contains(logs, "failed") || !contains(logs, "cancelled") {
		t.Errorf("callbacks = %v, want failed + cancelled", logs)
	}
}

func TestRun_PreCancelledCtx(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exec := &scriptedExecutor{}
	rec := &recordedCallbacks{}
	me, _ := New(Config{Executor: exec, Callbacks: rec.callbacks(), Sleep: fakeSleep})
	state, _ := me.Run(ctx, ExecuteRequest{})
	if state != StateCancelled {
		t.Errorf("state = %v, want CANCELLED", state)
	}
	if exec.calls != 0 {
		t.Errorf("executor called %d times on pre-cancelled ctx, want 0", exec.calls)
	}
	if got := rec.snapshot(); len(got) != 1 || got[0] != "cancelled" {
		t.Errorf("callbacks = %v, want [cancelled]", got)
	}
}

func TestRun_NilCallbacksSafe(t *testing.T) {
	t.Parallel()

	exec := &scriptedExecutor{results: []ExecuteResult{{ExitCode: 1}, {ExitCode: 0}}}
	me, err := New(Config{
		Executor: exec,
		Retry:    RetryPolicy{MaxAttempts: 2, InitialBackoff: time.Millisecond},
		Sleep:    fakeSleep,
	})
	if err != nil {
		t.Fatal(err)
	}
	state, _ := me.Run(context.Background(), ExecuteRequest{})
	if state != StateCompleted {
		t.Errorf("state = %v, want COMPLETED", state)
	}
}

func TestRun_NonZeroExitCountsAsFailure(t *testing.T) {
	t.Parallel()

	exec := &scriptedExecutor{results: []ExecuteResult{{ExitCode: 2}}}
	me, _ := New(Config{Executor: exec, Sleep: fakeSleep})
	state, _ := me.Run(context.Background(), ExecuteRequest{})
	if state != StateFailed {
		t.Errorf("state = %v, want FAILED", state)
	}
}

func TestRun_SystemErrorCountsAsFailure(t *testing.T) {
	t.Parallel()

	exec := &scriptedExecutor{results: []ExecuteResult{{ExitCode: 0, Error: "lookup: no such file"}}}
	me, _ := New(Config{Executor: exec, Sleep: fakeSleep})
	state, _ := me.Run(context.Background(), ExecuteRequest{})
	if state != StateFailed {
		t.Errorf("state = %v, want FAILED (system error overrides ExitCode==0)", state)
	}
}

func TestNew_NilExec(t *testing.T) {
	t.Parallel()
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error for nil executor")
	}
	if !strings.Contains(err.Error(), "nil executor") {
		t.Errorf("error %q should mention nil executor", err)
	}
}

func TestRetryPolicy_Backoff(t *testing.T) {
	t.Parallel()

	p := RetryPolicy{
		InitialBackoff:    100 * time.Millisecond,
		BackoffMultiplier: 2,
		MaxBackoff:        500 * time.Millisecond,
	}.effective()

	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 0},
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 500 * time.Millisecond}, // capped
		{10, 500 * time.Millisecond}, // still capped
	}
	for _, tc := range cases {
		if got := p.backoffFor(tc.attempt); got != tc.want {
			t.Errorf("backoffFor(%d) = %v, want %v", tc.attempt, got, tc.want)
		}
	}
}

func TestRetryPolicy_Defaults(t *testing.T) {
	t.Parallel()
	p := RetryPolicy{}.effective()
	if p.MaxAttempts != 1 {
		t.Errorf("MaxAttempts = %d, want 1", p.MaxAttempts)
	}
	if p.InitialBackoff != 100*time.Millisecond {
		t.Errorf("InitialBackoff = %v", p.InitialBackoff)
	}
	if p.BackoffMultiplier != 2 {
		t.Errorf("BackoffMultiplier = %v", p.BackoffMultiplier)
	}
	if p.MaxBackoff != 10*time.Second {
		t.Errorf("MaxBackoff = %v", p.MaxBackoff)
	}
}

func TestState_String(t *testing.T) {
	t.Parallel()
	cases := map[State]string{
		StatePending:   "PENDING",
		StateRunning:   "RUNNING",
		StateCompleted: "COMPLETED",
		StateFailed:    "FAILED",
		StateTimeout:   "TIMEOUT",
		StateCancelled: "CANCELLED",
		StateRetrying:  "RETRYING",
		State(99):      "State(99)",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(s), got, want)
		}
	}
}

func TestExecutorFunc(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	var f Executor = ExecutorFunc(func(_ context.Context, _ ExecuteRequest) ExecuteResult {
		calls.Add(1)
		return ExecuteResult{ExitCode: 0}
	})
	if !f.Execute(context.Background(), ExecuteRequest{}).Succeeded() {
		t.Error("ExecutorFunc.Execute should return success")
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

func TestCtxSleep_DeadlineFires(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ctxSleep(ctx, 10*time.Millisecond); !errors.Is(err, context.Canceled) {
		t.Errorf("ctxSleep cancelled = %v, want Canceled", err)
	}
	if err := ctxSleep(context.Background(), 0); err != nil {
		t.Errorf("ctxSleep d=0 = %v, want nil", err)
	}
}

// equal compares two string slices for ordering and content.
func equal(a, b []string) bool {
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

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
