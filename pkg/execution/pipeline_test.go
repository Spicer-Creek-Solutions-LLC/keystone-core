package execution

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewPipelineExecutor(t *testing.T) {
	tests := []struct {
		name string
		opts *PipelineExecutorOptions
	}{
		{
			name: "nil options",
			opts: nil,
		},
		{
			name: "with executor",
			opts: &PipelineExecutorOptions{
				Executor: NewExecutor(nil),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pe := NewPipelineExecutor(tt.opts)
			if pe == nil {
				t.Fatal("expected non-nil pipeline executor")
			}
			if pe.executor == nil {
				t.Fatal("expected non-nil internal executor")
			}
		})
	}
}

func TestPipelineExecutor_Execute_NilPipeline(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	_, err := pe.Execute(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for nil pipeline")
	}
}

func TestPipelineExecutor_Execute_EmptyPipeline(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	pipeline := &Pipeline{
		ID:     "empty",
		Stages: []*PipelineStage{},
	}

	_, err := pe.Execute(context.Background(), pipeline, nil)
	if err == nil {
		t.Fatal("expected error for empty pipeline")
	}
}

func TestPipelineExecutor_Execute_SingleStage(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	pipeline := NewPipelineBuilder("single-stage").
		AddShellCommand("echo", ShellTypeBash, "echo 'hello world'").
		Build()

	result, err := pe.Execute(context.Background(), pipeline, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	if len(result.StageResults) != 1 {
		t.Fatalf("expected 1 stage result, got %d", len(result.StageResults))
	}

	output := strings.TrimSpace(string(result.FinalOutput))
	if output != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", output)
	}
}

func TestPipelineExecutor_Execute_MultiStage(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	// Create a pipeline: echo "hello" | tr 'a-z' 'A-Z'
	pipeline := NewPipelineBuilder("multi-stage").
		AddShellCommand("echo", ShellTypeBash, "echo hello").
		AddShellCommand("upper", ShellTypeBash, "tr 'a-z' 'A-Z'").
		Build()

	result, err := pe.Execute(context.Background(), pipeline, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		for i, sr := range result.StageResults {
			if sr.Error != nil {
				t.Logf("stage %d error: %v", i, sr.Error)
			}
		}
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	if len(result.StageResults) != 2 {
		t.Fatalf("expected 2 stage results, got %d", len(result.StageResults))
	}

	output := strings.TrimSpace(string(result.FinalOutput))
	if output != "HELLO" {
		t.Errorf("expected 'HELLO', got '%s'", output)
	}
}

func TestPipelineExecutor_Execute_WithTransform(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	pipeline := &Pipeline{
		ID: "transform-pipeline",
		Stages: []*PipelineStage{
			{
				ID:        "echo",
				Command:   "echo test",
				ShellType: ShellTypeBash,
				Transform: func(input []byte) ([]byte, error) {
					return []byte(strings.ToUpper(string(input))), nil
				},
			},
			{
				ID:        "cat",
				Command:   "cat",
				ShellType: ShellTypeBash,
			},
		},
	}

	result, err := pe.Execute(context.Background(), pipeline, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimSpace(string(result.FinalOutput))
	if output != "TEST" {
		t.Errorf("expected 'TEST' after transform, got '%s'", output)
	}
}

func TestPipelineExecutor_Execute_StopOnError(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	pipeline := NewPipelineBuilder("error-pipeline").
		AddShellCommand("fail", ShellTypeBash, "exit 1").
		AddShellCommand("never-run", ShellTypeBash, "echo should not run").
		StopOnError(true).
		Build()

	result, err := pe.Execute(context.Background(), pipeline, nil)
	if err == nil {
		t.Fatal("expected error from failed pipeline")
	}

	if result.Success {
		t.Fatal("expected success to be false")
	}

	// Second stage should be skipped
	if len(result.StageResults) != 2 {
		t.Fatalf("expected 2 stage results, got %d", len(result.StageResults))
	}

	if !result.StageResults[1].Skipped {
		t.Error("expected second stage to be skipped")
	}

	if result.SkippedStages() != 1 {
		t.Errorf("expected 1 skipped stage, got %d", result.SkippedStages())
	}
}

func TestPipelineExecutor_Execute_WithTimeout(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	pipeline := NewPipelineBuilder("timeout-pipeline").
		AddShellCommand("slow", ShellTypeBash, "sleep 10").
		WithGlobalTimeout(100 * time.Millisecond).
		Build()

	result, err := pe.Execute(context.Background(), pipeline, nil)
	if err == nil {
		t.Fatal("expected error from timeout")
	}

	if result.Success {
		t.Fatal("expected success to be false")
	}
}

func TestPipelineExecutor_Execute_WithHandler(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	pipeline := NewPipelineBuilder("handler-pipeline").
		AddShellCommand("step1", ShellTypeBash, "echo one").
		AddShellCommand("step2", ShellTypeBash, "echo two").
		Build()

	var handlerCalls []string
	handler := func(stageIndex int, stageID string, result *StageResult) {
		handlerCalls = append(handlerCalls, stageID)
	}

	result, err := pe.Execute(context.Background(), pipeline, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	if len(handlerCalls) != 2 {
		t.Fatalf("expected 2 handler calls, got %d", len(handlerCalls))
	}

	if handlerCalls[0] != "step1" || handlerCalls[1] != "step2" {
		t.Errorf("unexpected handler calls: %v", handlerCalls)
	}
}

func TestPipelineExecutor_Execute_CancelContext(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	ctx, cancel := context.WithCancel(context.Background())

	pipeline := NewPipelineBuilder("cancel-pipeline").
		AddShellCommand("slow", ShellTypeBash, "sleep 10").
		Build()

	// Cancel immediately
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := pe.Execute(ctx, pipeline, nil)
	if err == nil {
		t.Fatal("expected error from cancellation")
	}

	if result.Success {
		t.Fatal("expected success to be false")
	}
}

func TestPipelineExecutor_GetRunningPipelines(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	// Initially empty
	if len(pe.GetRunningPipelines()) != 0 {
		t.Fatal("expected no running pipelines initially")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pipeline := NewPipelineBuilder("long-running").
		AddShellCommand("slow", ShellTypeBash, "sleep 10").
		Build()

	// Start pipeline in goroutine
	done := make(chan struct{})
	go func() {
		pe.Execute(ctx, pipeline, nil)
		close(done)
	}()

	// Wait for it to start
	time.Sleep(100 * time.Millisecond)

	running := pe.GetRunningPipelines()
	if len(running) != 1 {
		t.Fatalf("expected 1 running pipeline, got %d", len(running))
	}

	if running[0] != "long-running" {
		t.Errorf("expected pipeline ID 'long-running', got '%s'", running[0])
	}

	// Cancel and wait for completion
	cancel()
	<-done

	// Should be empty again
	if len(pe.GetRunningPipelines()) != 0 {
		t.Fatal("expected no running pipelines after completion")
	}
}

func TestPipelineExecutor_CancelPipeline(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	// Cancel non-existent pipeline
	err := pe.CancelPipeline("non-existent")
	if err == nil {
		t.Fatal("expected error for non-existent pipeline")
	}

	pipeline := NewPipelineBuilder("to-cancel").
		AddShellCommand("slow", ShellTypeBash, "sleep 10").
		Build()

	// Start pipeline in goroutine
	done := make(chan struct{})
	go func() {
		pe.Execute(context.Background(), pipeline, nil)
		close(done)
	}()

	// Wait for it to start
	time.Sleep(100 * time.Millisecond)

	// Cancel it
	err = pe.CancelPipeline("to-cancel")
	if err != nil {
		t.Fatalf("unexpected error cancelling pipeline: %v", err)
	}

	// Wait for completion with timeout
	select {
	case <-done:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline did not complete after cancellation")
	}
}

func TestPipelineBuilder(t *testing.T) {
	builder := NewPipelineBuilder("test-build")

	pipeline := builder.
		AddCommand("cmd1", "echo hello").
		AddShellCommand("cmd2", ShellTypeBash, "cat").
		AddStage(&PipelineStage{
			ID:      "cmd3",
			Command: "echo done",
		}).
		WithGlobalTimeout(5 * time.Second).
		WithEnv(map[string]string{"FOO": "bar"}).
		StopOnError(true).
		ContinueOnError(false).
		Build()

	if pipeline.ID != "test-build" {
		t.Errorf("expected ID 'test-build', got '%s'", pipeline.ID)
	}

	if len(pipeline.Stages) != 3 {
		t.Fatalf("expected 3 stages, got %d", len(pipeline.Stages))
	}

	if pipeline.GlobalTimeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", pipeline.GlobalTimeout)
	}

	if pipeline.Env["FOO"] != "bar" {
		t.Errorf("expected env FOO=bar, got %s", pipeline.Env["FOO"])
	}

	if !pipeline.StopOnError {
		t.Error("expected StopOnError to be true")
	}
}

func TestPipelineBuilder_AutoStageID(t *testing.T) {
	builder := NewPipelineBuilder("auto-id")

	pipeline := builder.
		AddStage(&PipelineStage{Command: "echo 1"}).
		AddStage(&PipelineStage{Command: "echo 2"}).
		Build()

	if pipeline.Stages[0].ID != "stage-0" {
		t.Errorf("expected 'stage-0', got '%s'", pipeline.Stages[0].ID)
	}
	if pipeline.Stages[1].ID != "stage-1" {
		t.Errorf("expected 'stage-1', got '%s'", pipeline.Stages[1].ID)
	}
}

func TestPipelineResult_Duration(t *testing.T) {
	start := time.Now()
	end := start.Add(100 * time.Millisecond)
	result := &PipelineResult{
		StartTime: start,
		EndTime:   end,
	}

	duration := result.Duration()
	if duration != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", duration)
	}
}

func TestPipelineResult_StageCounts(t *testing.T) {
	result := &PipelineResult{
		StageResults: []*StageResult{
			{StageID: "s1", ExitCode: 0},
			{StageID: "s2", ExitCode: 1},
			{StageID: "s3", Skipped: true},
		},
	}

	if result.SuccessfulStages() != 1 {
		t.Errorf("expected 1 successful stage, got %d", result.SuccessfulStages())
	}

	if result.FailedStages() != 1 {
		t.Errorf("expected 1 failed stage, got %d", result.FailedStages())
	}

	if result.SkippedStages() != 1 {
		t.Errorf("expected 1 skipped stage, got %d", result.SkippedStages())
	}
}

func TestPipelineResult_GetStageResult(t *testing.T) {
	result := &PipelineResult{
		StageResults: []*StageResult{
			{StageID: "s1"},
			{StageID: "s2"},
		},
	}

	sr := result.GetStageResult("s2")
	if sr == nil {
		t.Fatal("expected to find stage s2")
	}
	if sr.StageID != "s2" {
		t.Errorf("expected StageID 's2', got '%s'", sr.StageID)
	}

	sr = result.GetStageResult("non-existent")
	if sr != nil {
		t.Error("expected nil for non-existent stage")
	}
}

func TestPipelineResult_CollectErrors(t *testing.T) {
	result := &PipelineResult{
		Error: context.DeadlineExceeded,
		StageResults: []*StageResult{
			{StageID: "s1"},
			{StageID: "s2", Error: context.Canceled},
		},
	}

	errors := result.CollectErrors()
	if len(errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors))
	}
}

func TestStageResult_Methods(t *testing.T) {
	sr := &StageResult{
		StageID:   "test",
		Input:     []byte("input"),
		Output:    []byte("output"),
		Stderr:    []byte("error"),
		StartTime: time.Now(),
		EndTime:   time.Now().Add(50 * time.Millisecond),
	}

	if sr.InputString() != "input" {
		t.Errorf("expected 'input', got '%s'", sr.InputString())
	}

	if sr.OutputString() != "output" {
		t.Errorf("expected 'output', got '%s'", sr.OutputString())
	}

	if sr.StderrString() != "error" {
		t.Errorf("expected 'error', got '%s'", sr.StderrString())
	}

	if sr.Duration() != 50*time.Millisecond {
		t.Errorf("expected 50ms, got %v", sr.Duration())
	}
}

func TestPipelineExecutor_Execute_WithEnv(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	pipeline := NewPipelineBuilder("env-pipeline").
		AddStage(&PipelineStage{
			ID:        "env-echo",
			Command:   "echo $TEST_VAR",
			ShellType: ShellTypeBash,
			Env:       map[string]string{"TEST_VAR": "hello"},
		}).
		Build()

	result, err := pe.Execute(context.Background(), pipeline, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimSpace(string(result.FinalOutput))
	if output != "hello" {
		t.Errorf("expected 'hello', got '%s'", output)
	}
}

func TestPipelineExecutor_Execute_GlobalEnv(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	pipeline := NewPipelineBuilder("global-env-pipeline").
		AddShellCommand("env-echo", ShellTypeBash, "echo $GLOBAL_VAR").
		WithEnv(map[string]string{"GLOBAL_VAR": "global"}).
		Build()

	result, err := pe.Execute(context.Background(), pipeline, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimSpace(string(result.FinalOutput))
	if output != "global" {
		t.Errorf("expected 'global', got '%s'", output)
	}
}

func TestPipelineExecutor_Execute_ThreeStages(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	// echo "hello world" | wc -w | tr -d ' '
	pipeline := NewPipelineBuilder("three-stages").
		AddShellCommand("echo", ShellTypeBash, "echo hello world").
		AddShellCommand("wc", ShellTypeBash, "wc -w").
		AddShellCommand("trim", ShellTypeBash, "tr -d ' '").
		Build()

	result, err := pe.Execute(context.Background(), pipeline, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	output := strings.TrimSpace(string(result.FinalOutput))
	if output != "2" {
		t.Errorf("expected '2', got '%s'", output)
	}
}

func TestPipelineExecutor_Execute_InputToOutput(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	// Test that output flows correctly between stages
	pipeline := NewPipelineBuilder("input-output").
		AddShellCommand("numbers", ShellTypeBash, "echo -e '3\\n1\\n2'").
		AddShellCommand("sort", ShellTypeBash, "sort -n").
		Build()

	result, err := pe.Execute(context.Background(), pipeline, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	output := strings.TrimSpace(string(result.FinalOutput))
	if output != "1\n2\n3" {
		t.Errorf("expected '1\\n2\\n3', got '%s'", output)
	}
}

func TestPipelineExecutor_Execute_StageTimeout(t *testing.T) {
	pe := NewPipelineExecutor(nil)

	pipeline := &Pipeline{
		ID: "stage-timeout",
		Stages: []*PipelineStage{
			{
				ID:        "slow",
				Command:   "sleep 10",
				ShellType: ShellTypeBash,
				Timeout:   100 * time.Millisecond,
			},
		},
	}

	result, _ := pe.Execute(context.Background(), pipeline, nil)

	if result.Success {
		t.Fatal("expected failure due to timeout")
	}
}
