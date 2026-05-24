// SPDX-License-Identifier: Apache-2.0

package verification

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// funcVerifier adapts a func to a Verifier for engine tests.
type funcVerifier struct {
	typ string
	fn  func(ctx context.Context, step Step) Result
}

func (f funcVerifier) Type() string { return f.typ }
func (f funcVerifier) Verify(ctx context.Context, step Step) Result {
	return f.fn(ctx, step)
}

func okResult() Result   { return Result{Success: true, Message: "ok"} }
func failResult() Result { return Result{Success: false, Message: "nope"} }

func regWith(t *testing.T, vs ...Verifier) *Registry {
	t.Helper()
	r := NewRegistry()
	for _, v := range vs {
		if err := r.Register(v); err != nil {
			t.Fatalf("register %s: %v", v.Type(), err)
		}
	}
	return r
}

func TestEngine_Sequential_Success(t *testing.T) {
	t.Parallel()
	reg := regWith(t, funcVerifier{"http", func(context.Context, Step) Result { return okResult() }})
	e := NewEngine(reg)
	wr := e.Run(context.Background(), Workflow{
		Name:  "wf",
		Steps: []Step{{Name: "a", Type: "http"}, {Name: "b", Type: "http"}},
	})
	if !wr.Success || len(wr.Steps) != 2 {
		t.Fatalf("Success=%v steps=%d, want true/2", wr.Success, len(wr.Steps))
	}
	if wr.Steps[0].Name != "a" || wr.Steps[1].Name != "b" {
		t.Errorf("step order not preserved: %+v", wr.Steps)
	}
}

func TestEngine_Sequential_AbortSkipsRest(t *testing.T) {
	t.Parallel()
	reg := regWith(t,
		funcVerifier{"bad", func(context.Context, Step) Result { return failResult() }},
		funcVerifier{"good", func(context.Context, Step) Result { return okResult() }},
	)
	wr := NewEngine(reg).Run(context.Background(), Workflow{
		Steps: []Step{
			{Name: "s1", Type: "bad"},
			{Name: "s2", Type: "good"},
			{Name: "s3", Type: "good"},
		},
	})
	if wr.Success {
		t.Error("Success = true, want false")
	}
	if wr.Steps[0].Result.Success {
		t.Error("s1 should have failed")
	}
	if !wr.Steps[1].Skipped || !wr.Steps[2].Skipped {
		t.Errorf("s2/s3 should be Skipped under FailAbort: %+v", wr.Steps)
	}
}

func TestEngine_Sequential_ContinueRunsAll(t *testing.T) {
	t.Parallel()
	var ran int32
	reg := regWith(t, funcVerifier{"bad", func(context.Context, Step) Result {
		atomic.AddInt32(&ran, 1)
		return failResult()
	}})
	wr := NewEngine(reg).Run(context.Background(), Workflow{
		OnFailure: FailContinue,
		Steps:     []Step{{Name: "s1", Type: "bad"}, {Name: "s2", Type: "bad"}},
	})
	if wr.Success {
		t.Error("Success = true, want false")
	}
	if atomic.LoadInt32(&ran) != 2 {
		t.Errorf("ran %d steps, want 2 (FailContinue runs all)", ran)
	}
	if wr.Steps[0].Skipped || wr.Steps[1].Skipped {
		t.Error("no step should be Skipped under FailContinue")
	}
}

func TestEngine_OptionalFailureTolerated(t *testing.T) {
	t.Parallel()
	reg := regWith(t,
		funcVerifier{"bad", func(context.Context, Step) Result { return failResult() }},
		funcVerifier{"good", func(context.Context, Step) Result { return okResult() }},
	)
	wr := NewEngine(reg).Run(context.Background(), Workflow{
		Steps: []Step{
			{Name: "opt", Type: "bad", Optional: true},
			{Name: "req", Type: "good"},
		},
	})
	if !wr.Success {
		t.Error("Success = false, want true (optional failure ignored)")
	}
	if wr.Steps[1].Skipped {
		t.Error("required step after optional failure must still run")
	}
}

func TestEngine_Retries(t *testing.T) {
	t.Parallel()
	var attempts int32
	reg := regWith(t, funcVerifier{"flaky", func(context.Context, Step) Result {
		if atomic.AddInt32(&attempts, 1) < 3 {
			return failResult()
		}
		return okResult()
	}})
	e := &Engine{Registry: reg, BackoffBase: time.Millisecond, BackoffCap: 2 * time.Millisecond}
	wr := e.Run(context.Background(), Workflow{
		Steps: []Step{{Name: "s", Type: "flaky", Retries: 3}},
	})
	if !wr.Success {
		t.Fatalf("Success = false, want true after retries")
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
	if wr.Steps[0].Result.Retries != 2 {
		t.Errorf("Result.Retries = %d, want 2", wr.Steps[0].Result.Retries)
	}
}

func TestEngine_PerStepTimeout(t *testing.T) {
	t.Parallel()
	reg := regWith(t, funcVerifier{"slow", func(ctx context.Context, _ Step) Result {
		<-ctx.Done() // respect the per-step deadline
		return Result{Success: false, Message: "ctx done", Error: ctx.Err()}
	}})
	wr := NewEngine(reg).Run(context.Background(), Workflow{
		Steps: []Step{{Name: "s", Type: "slow", Timeout: 20 * time.Millisecond}},
	})
	if wr.Success {
		t.Error("Success = true, want false (step timed out)")
	}
	if wr.Steps[0].Result.Error == nil {
		t.Error("expected ctx error recorded")
	}
}

func TestEngine_UnknownVerifier(t *testing.T) {
	t.Parallel()
	wr := NewEngine(NewRegistry()).Run(context.Background(), Workflow{
		Steps: []Step{{Name: "s", Type: "mystery"}},
	})
	if wr.Success {
		t.Error("Success = true, want false")
	}
	if wr.Steps[0].Result.Error == nil {
		t.Error("unknown verifier must set Result.Error")
	}
}

func TestEngine_Parallel_AllRun_OrderPreserved(t *testing.T) {
	t.Parallel()
	reg := regWith(t, funcVerifier{"http", func(context.Context, Step) Result {
		time.Sleep(5 * time.Millisecond)
		return okResult()
	}})
	steps := make([]Step, 6)
	for i := range steps {
		steps[i] = Step{Name: string(rune('a' + i)), Type: "http"}
	}
	wr := NewEngine(reg).Run(context.Background(), Workflow{Parallel: true, Steps: steps})
	if !wr.Success || len(wr.Steps) != 6 {
		t.Fatalf("Success=%v len=%d", wr.Success, len(wr.Steps))
	}
	for i := range wr.Steps {
		if wr.Steps[i].Name != string(rune('a'+i)) {
			t.Errorf("parallel result order broken at %d: %q", i, wr.Steps[i].Name)
		}
	}
}

func TestEngine_Parallel_ConcurrencyCap(t *testing.T) {
	t.Parallel()
	var inFlight, peak int32
	var mu sync.Mutex
	reg := regWith(t, funcVerifier{"http", func(context.Context, Step) Result {
		n := atomic.AddInt32(&inFlight, 1)
		mu.Lock()
		if n > peak {
			peak = n
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return okResult()
	}})
	steps := make([]Step, 8)
	for i := range steps {
		steps[i] = Step{Name: string(rune('a' + i)), Type: "http"}
	}
	wr := NewEngine(reg).Run(context.Background(), Workflow{
		Parallel: true, MaxParallel: 2, Steps: steps,
	})
	if !wr.Success {
		t.Fatal("Success = false")
	}
	if peak > 2 {
		t.Errorf("peak concurrency = %d, want <= 2 (MaxParallel)", peak)
	}
}

func TestEngine_OverallTimeout(t *testing.T) {
	t.Parallel()
	reg := regWith(t, funcVerifier{"slow", func(ctx context.Context, _ Step) Result {
		<-ctx.Done()
		return Result{Success: false, Error: ctx.Err()}
	}})
	start := time.Now()
	wr := NewEngine(reg).Run(context.Background(), Workflow{
		Timeout: 30 * time.Millisecond,
		Steps:   []Step{{Name: "s", Type: "slow"}},
	})
	if wr.Success {
		t.Error("Success = true, want false (overall timeout)")
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("workflow did not honour overall timeout (took %v)", time.Since(start))
	}
}

func TestEngine_ExternalCancel(t *testing.T) {
	t.Parallel()
	reg := regWith(t, funcVerifier{"slow", func(ctx context.Context, _ Step) Result {
		<-ctx.Done()
		return Result{Success: false, Error: ctx.Err()}
	}})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(15 * time.Millisecond); cancel() }()
	wr := NewEngine(reg).Run(ctx, Workflow{Steps: []Step{{Name: "s", Type: "slow"}}})
	if wr.Success {
		t.Error("Success = true, want false (external cancel)")
	}
}
