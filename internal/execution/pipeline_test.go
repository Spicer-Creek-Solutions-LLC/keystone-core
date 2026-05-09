package execution

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// pipingExecutor records the StdinInput it sees on each call (so tests
// can assert that piping connected stage N's stdout to stage N+1's
// stdin) and returns a scripted result keyed by call index.
type pipingExecutor struct {
	mu      sync.Mutex
	results []ExecuteResult
	seen    [][]byte // copy of req.StdinInput at each call
	calls   int
	// onCall, if non-nil, runs at the start of each Execute. Used to
	// trip context cancellation between stages.
	onCall func(ctx context.Context, callIndex int)
}

func (e *pipingExecutor) Execute(ctx context.Context, req ExecuteRequest) ExecuteResult {
	e.mu.Lock()
	idx := e.calls
	e.calls++
	stdin := append([]byte(nil), req.StdinInput...)
	e.seen = append(e.seen, stdin)
	e.mu.Unlock()
	if e.onCall != nil {
		e.onCall(ctx, idx)
	}
	if idx >= len(e.results) {
		return ExecuteResult{Error: "pipingExecutor: out of scripted results"}
	}
	return e.results[idx]
}

func TestPipeline_ThreeStageSuccessPipes(t *testing.T) {
	t.Parallel()

	exec := &pipingExecutor{
		results: []ExecuteResult{
			{ExitCode: 0, Stdout: []byte("from-1")},
			{ExitCode: 0, Stdout: []byte("from-2")},
			{ExitCode: 0, Stdout: []byte("from-3")},
		},
	}
	p, err := NewPipeline(PipelineConfig{
		Executor: exec,
		Stages: []Stage{
			{Request: ExecuteRequest{Command: "step1"}},
			{Request: ExecuteRequest{Command: "step2"}},
			{Request: ExecuteRequest{Command: "step3"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	results, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	// Stage 1 sees nil/empty stdin; stage 2 sees "from-1"; stage 3 sees "from-2".
	if len(exec.seen[0]) != 0 {
		t.Errorf("stage 1 stdin = %q, want empty", exec.seen[0])
	}
	if !bytes.Equal(exec.seen[1], []byte("from-1")) {
		t.Errorf("stage 2 stdin = %q, want %q", exec.seen[1], "from-1")
	}
	if !bytes.Equal(exec.seen[2], []byte("from-2")) {
		t.Errorf("stage 3 stdin = %q, want %q", exec.seen[2], "from-2")
	}
}

func TestPipeline_TransformMutates(t *testing.T) {
	t.Parallel()

	exec := &pipingExecutor{
		results: []ExecuteResult{
			{ExitCode: 0, Stdout: []byte("hello")},
			{ExitCode: 0, Stdout: []byte("world")},
		},
	}
	p, _ := NewPipeline(PipelineConfig{
		Executor: exec,
		Stages: []Stage{
			{Request: ExecuteRequest{Command: "say"}},
			{
				Request:   ExecuteRequest{Command: "shout"},
				Transform: func(in []byte) []byte { return bytes.ToUpper(in) },
			},
		},
	})
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !bytes.Equal(exec.seen[1], []byte("HELLO")) {
		t.Errorf("stage 2 stdin = %q, want HELLO (transform applied)", exec.seen[1])
	}
}

func TestPipeline_FailFast(t *testing.T) {
	t.Parallel()

	exec := &pipingExecutor{
		results: []ExecuteResult{
			{ExitCode: 0, Stdout: []byte("ok")},
			{ExitCode: 7, Stderr: []byte("boom")},
			{ExitCode: 0}, // never reached
		},
	}
	p, _ := NewPipeline(PipelineConfig{
		Executor: exec,
		FailFast: true,
		Stages: []Stage{
			{Request: ExecuteRequest{Command: "ok"}},
			{Request: ExecuteRequest{Command: "fail"}},
			{Request: ExecuteRequest{Command: "never"}},
		},
	})
	results, err := p.Run(context.Background())
	if !errors.Is(err, ErrPipelineFailed) {
		t.Fatalf("err = %v, want ErrPipelineFailed", err)
	}
	if !strings.Contains(err.Error(), "stage 1") {
		t.Errorf("err = %q, want stage index in message", err)
	}
	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2 (stage 3 must not run)", len(results))
	}
	if exec.calls != 2 {
		t.Errorf("executor calls = %d, want 2", exec.calls)
	}
}

func TestPipeline_NoFailFastRunsEverything(t *testing.T) {
	t.Parallel()

	exec := &pipingExecutor{
		results: []ExecuteResult{
			{ExitCode: 0, Stdout: []byte("ok1")},
			{ExitCode: 7},
			{ExitCode: 0, Stdout: []byte("ok3")},
		},
	}
	p, _ := NewPipeline(PipelineConfig{
		Executor: exec,
		FailFast: false,
		Stages: []Stage{
			{Request: ExecuteRequest{Command: "a"}},
			{Request: ExecuteRequest{Command: "b"}},
			{Request: ExecuteRequest{Command: "c"}},
		},
	})
	results, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("len(results) = %d, want 3", len(results))
	}
}

func TestPipeline_Empty(t *testing.T) {
	t.Parallel()

	p, _ := NewPipeline(PipelineConfig{Executor: &pipingExecutor{}})
	results, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if results != nil {
		t.Errorf("results = %v, want nil", results)
	}
}

func TestPipeline_SingleStageNoPiping(t *testing.T) {
	t.Parallel()

	exec := &pipingExecutor{results: []ExecuteResult{{ExitCode: 0}}}
	p, _ := NewPipeline(PipelineConfig{
		Executor: exec,
		Stages: []Stage{
			{Request: ExecuteRequest{Command: "only", StdinInput: []byte("preset")}},
		},
	})
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Single-stage pipelines must not overwrite caller-supplied stdin
	// since there's no previous stage to pipe from.
	if !bytes.Equal(exec.seen[0], []byte("preset")) {
		t.Errorf("stage 1 stdin = %q, want preset (no piping on first stage)", exec.seen[0])
	}
}

func TestPipeline_CtxCancelBetweenStages(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	exec := &pipingExecutor{
		results: []ExecuteResult{
			{ExitCode: 0},
			{ExitCode: 0}, // never reached
		},
		onCall: func(_ context.Context, idx int) {
			if idx == 0 {
				cancel()
			}
		},
	}
	defer cancel()

	p, _ := NewPipeline(PipelineConfig{
		Executor: exec,
		Stages: []Stage{
			{Request: ExecuteRequest{Command: "a"}},
			{Request: ExecuteRequest{Command: "b"}},
		},
	})
	results, err := p.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1", len(results))
	}
}

func TestPipeline_PreCancelledCtx(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	exec := &pipingExecutor{}
	p, _ := NewPipeline(PipelineConfig{
		Executor: exec,
		Stages:   []Stage{{Request: ExecuteRequest{Command: "x"}}},
	})
	_, err := p.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if exec.calls != 0 {
		t.Errorf("executor calls = %d, want 0 (ctx pre-fired)", exec.calls)
	}
}

func TestPipeline_NilExecutor(t *testing.T) {
	t.Parallel()
	if _, err := NewPipeline(PipelineConfig{}); err == nil {
		t.Error("expected error for nil executor")
	}
}

func TestPipeline_NilStagesSafe(t *testing.T) {
	t.Parallel()

	exec := &pipingExecutor{}
	p, err := NewPipeline(PipelineConfig{Executor: exec, Stages: nil})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Run(context.Background()); err != nil {
		t.Errorf("Run with nil stages = %v, want nil", err)
	}
}

func TestPipeline_NilTransformIsIdentity(t *testing.T) {
	t.Parallel()

	exec := &pipingExecutor{
		results: []ExecuteResult{
			{ExitCode: 0, Stdout: []byte("payload")},
			{ExitCode: 0},
		},
	}
	p, _ := NewPipeline(PipelineConfig{
		Executor: exec,
		Stages: []Stage{
			{Request: ExecuteRequest{Command: "a"}},
			{Request: ExecuteRequest{Command: "b"}}, // nil Transform
		},
	})
	if _, err := p.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(exec.seen[1], []byte("payload")) {
		t.Errorf("stage 2 stdin = %q, want payload (nil transform = identity)", exec.seen[1])
	}
}
