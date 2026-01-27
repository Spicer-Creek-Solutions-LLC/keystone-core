package execution

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// BenchmarkExecutorSingleStep benchmarks single step execution.
func BenchmarkExecutorSingleStep(b *testing.B) {
	executor := NewExecutor()

	rb := &runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata: runbook.Metadata{
			Name:      "benchmark-single",
			Namespace: "test",
		},
		Spec: runbook.RunbookSpec{
			Description: "Single step benchmark",
			Steps: []runbook.Step{
				{
					Name:   "noop",
					Type:   runbook.StepTypeNoop,
					Config: map[string]interface{}{},
				},
			},
		},
	}

	ctx := context.Background()
	inputs := map[string]interface{}{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.Execute(ctx, rb, inputs)
		if err != nil {
			b.Fatalf("execution failed: %v", err)
		}
	}
}

// BenchmarkExecutorMultipleSteps benchmarks runbooks with multiple steps.
func BenchmarkExecutorMultipleSteps(b *testing.B) {
	stepCounts := []int{5, 10, 20, 50}

	for _, count := range stepCounts {
		b.Run(fmt.Sprintf("steps=%d", count), func(b *testing.B) {
			executor := NewExecutor()

			steps := make([]runbook.Step, count)
			for i := 0; i < count; i++ {
				steps[i] = runbook.Step{
					Name:   fmt.Sprintf("step_%d", i),
					Type:   runbook.StepTypeNoop,
					Config: map[string]interface{}{},
				}
			}

			rb := &runbook.Runbook{
				APIVersion: "runbook.keystone.io/v1",
				Kind:       "Runbook",
				Metadata: runbook.Metadata{
					Name:      "benchmark-multi",
					Namespace: "test",
				},
				Spec: runbook.RunbookSpec{
					Description: "Multi-step benchmark",
					Steps:       steps,
				},
			}

			ctx := context.Background()
			inputs := map[string]interface{}{}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := executor.Execute(ctx, rb, inputs)
				if err != nil {
					b.Fatalf("execution failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkExecutorWithDependencies benchmarks dependency resolution.
func BenchmarkExecutorWithDependencies(b *testing.B) {
	executor := NewExecutor()

	// Create a chain of dependent steps
	steps := []runbook.Step{
		{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
		{Name: "step2", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}, DependsOn: []string{"step1"}},
		{Name: "step3", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}, DependsOn: []string{"step2"}},
		{Name: "step4", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}, DependsOn: []string{"step3"}},
		{Name: "step5", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}, DependsOn: []string{"step4"}},
	}

	rb := &runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "benchmark-deps", Namespace: "test"},
		Spec: runbook.RunbookSpec{
			Description: "Dependency benchmark",
			Steps:       steps,
		},
	}

	ctx := context.Background()
	inputs := map[string]interface{}{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.Execute(ctx, rb, inputs)
		if err != nil {
			b.Fatalf("execution failed: %v", err)
		}
	}
}

// BenchmarkTopologicalSort benchmarks the dependency sorting algorithm.
func BenchmarkTopologicalSort(b *testing.B) {
	sizes := []int{10, 50, 100, 500}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("nodes=%d", size), func(b *testing.B) {
			steps := make([]runbook.Step, size)
			for i := 0; i < size; i++ {
				step := runbook.Step{
					Name:   fmt.Sprintf("step_%d", i),
					Config: map[string]interface{}{},
				}
				if i > 0 {
					// Each step depends on the previous one
					step.DependsOn = []string{fmt.Sprintf("step_%d", i-1)}
				}
				steps[i] = step
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				graph := buildDependencyGraph(steps)
				_, err := topologicalSort(graph)
				if err != nil {
					b.Fatalf("sort failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkConcurrentExecutions benchmarks many concurrent runbook executions.
func BenchmarkConcurrentExecutions(b *testing.B) {
	concurrencyLevels := []int{10, 50, 100}

	for _, concurrent := range concurrencyLevels {
		b.Run(fmt.Sprintf("concurrent=%d", concurrent), func(b *testing.B) {
			executor := NewExecutor()

			rb := &runbook.Runbook{
				APIVersion: "runbook.keystone.io/v1",
				Kind:       "Runbook",
				Metadata:   runbook.Metadata{Name: "benchmark-concurrent", Namespace: "test"},
				Spec: runbook.RunbookSpec{
					Description: "Concurrent benchmark",
					Steps: []runbook.Step{
						{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
						{Name: "step2", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
						{Name: "step3", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
					},
				},
			}

			ctx := context.Background()
			inputs := map[string]interface{}{}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				wg.Add(concurrent)

				for j := 0; j < concurrent; j++ {
					go func() {
						defer wg.Done()
						_, _ = executor.Execute(ctx, rb, inputs)
					}()
				}

				wg.Wait()
			}
		})
	}
}

// BenchmarkVariableResolution benchmarks template variable resolution.
func BenchmarkVariableResolution(b *testing.B) {
	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
	}
	inputs := map[string]interface{}{
		"server": "web-server-01",
		"port":   8080,
		"config": map[string]interface{}{
			"debug":   true,
			"timeout": 30,
		},
	}
	execCtx := NewExecutionContext("exec-123", rb, inputs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = execCtx.Resolve("Deploying to {{ .inputs.server }}:{{ .inputs.port }}")
	}
}

// BenchmarkConditionEvaluation benchmarks condition expression evaluation.
func BenchmarkConditionEvaluation(b *testing.B) {
	rb := &runbook.Runbook{
		Metadata: runbook.Metadata{Name: "test-runbook"},
	}
	inputs := map[string]interface{}{
		"environment": "production",
		"enabled":     true,
		"count":       42,
	}
	execCtx := NewExecutionContext("exec-123", rb, inputs)

	conditions := []string{
		"{{ eq .inputs.environment \"production\" }}",
		"{{ .inputs.enabled }}",
		"{{ gt .inputs.count 10 }}",
	}

	for _, cond := range conditions {
		b.Run(cond, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = execCtx.EvaluateCondition(cond)
			}
		})
	}
}

// BenchmarkExecutorWithStorage benchmarks execution with storage operations.
func BenchmarkExecutorWithStorage(b *testing.B) {
	storage := newMockStorage()
	executor := NewExecutor(WithStorage(storage))

	rb := &runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "benchmark-storage", Namespace: "test"},
		Spec: runbook.RunbookSpec{
			Description: "Storage benchmark",
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
				{Name: "step2", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
				{Name: "step3", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	ctx := context.Background()
	inputs := map[string]interface{}{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.Execute(ctx, rb, inputs)
		if err != nil {
			b.Fatalf("execution failed: %v", err)
		}
	}
}

// mockStorage is a simple in-memory storage for benchmarks.
type mockStorage struct {
	mu         sync.RWMutex
	executions map[string]*runbook.Execution
	steps      map[string]*runbook.StepExecution
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		executions: make(map[string]*runbook.Execution),
		steps:      make(map[string]*runbook.StepExecution),
	}
}

func (s *mockStorage) SaveExecution(ctx context.Context, exec *runbook.Execution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.executions[exec.ID] = exec
	return nil
}

func (s *mockStorage) GetExecution(ctx context.Context, id string) (*runbook.Execution, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	exec, ok := s.executions[id]
	if !ok {
		return nil, fmt.Errorf("execution not found: %s", id)
	}
	return exec, nil
}

func (s *mockStorage) SaveStepExecution(ctx context.Context, executionID string, step *runbook.StepExecution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps[step.Name] = step
	return nil
}

// TestConcurrentExecution100 tests 100+ concurrent executions.
func TestConcurrentExecution100(t *testing.T) {
	executor := NewExecutor()

	rb := &runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "concurrent-test", Namespace: "test"},
		Spec: runbook.RunbookSpec{
			Description: "Concurrent execution test",
			Steps: []runbook.Step{
				{Name: "step1", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
				{Name: "step2", Type: runbook.StepTypeNoop, Config: map[string]interface{}{}},
			},
		},
	}

	const numExecutions = 150
	ctx := context.Background()
	inputs := map[string]interface{}{}

	var wg sync.WaitGroup
	wg.Add(numExecutions)

	var successCount int64
	var errorCount int64
	var mu sync.Mutex
	var sampleError error

	startTime := time.Now()

	for i := 0; i < numExecutions; i++ {
		go func() {
			defer wg.Done()
			exec, err := executor.Execute(ctx, rb, inputs)
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				mu.Lock()
				if sampleError == nil {
					sampleError = err
				}
				mu.Unlock()
				return
			}
			if exec.State == runbook.ExecutionStateCompleted {
				atomic.AddInt64(&successCount, 1)
			} else {
				atomic.AddInt64(&errorCount, 1)
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(startTime)

	t.Logf("Executed %d runbooks in %v", numExecutions, elapsed)
	t.Logf("Success: %d, Errors: %d", successCount, errorCount)
	t.Logf("Average: %v per execution", elapsed/time.Duration(numExecutions))
	if sampleError != nil {
		t.Logf("Sample error: %v", sampleError)
	}

	if errorCount > 0 {
		t.Errorf("Expected no errors, got %d", errorCount)
	}

	if successCount != numExecutions {
		t.Errorf("Expected %d successful executions, got %d", numExecutions, successCount)
	}
}
