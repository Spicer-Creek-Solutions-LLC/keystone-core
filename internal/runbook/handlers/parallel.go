package handlers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// ParallelHandler executes multiple steps concurrently.
// Configuration:
//   - steps: List of steps to execute in parallel (required)
//   - maxParallel: Maximum concurrent executions (0 = all at once, default: 0)
//   - failFast: Stop all parallel executions on first failure (default: false)
type ParallelHandler struct {
	stepExecutor StepExecutor
}

// NewParallelHandler creates a new parallel handler.
func NewParallelHandler(executor StepExecutor) *ParallelHandler {
	return &ParallelHandler{stepExecutor: executor}
}

// Type returns the step type.
func (h *ParallelHandler) Type() runbook.StepType {
	return runbook.StepTypeParallel
}

// Validate validates the step configuration.
func (h *ParallelHandler) Validate(step *runbook.Step) error {
	if _, ok := step.Config["steps"]; !ok {
		return fmt.Errorf("parallel step requires 'steps' configuration")
	}

	// Validate maxParallel if provided
	if maxParallel, ok := step.Config["maxParallel"]; ok {
		switch v := maxParallel.(type) {
		case int:
			if v < 0 {
				return fmt.Errorf("maxParallel must be non-negative")
			}
		case float64:
			if v < 0 {
				return fmt.Errorf("maxParallel must be non-negative")
			}
		default:
			return fmt.Errorf("maxParallel must be an integer")
		}
	}

	return nil
}

// Execute executes steps in parallel.
func (h *ParallelHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	startTime := time.Now()

	// Parse steps
	parallelSteps, err := parseStepList(step.Config["steps"])
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("invalid parallel steps: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	if len(parallelSteps) == 0 {
		return &runbook.StepResult{
			Success:  true,
			Message:  "no steps to execute",
			Duration: time.Since(startTime),
			Outputs: map[string]interface{}{
				"completed_count": 0,
				"failed_count":    0,
			},
		}, nil
	}

	// Get maxParallel
	maxParallel := 0
	if v, ok := step.Config["maxParallel"].(int); ok {
		maxParallel = v
	} else if v, ok := step.Config["maxParallel"].(float64); ok {
		maxParallel = int(v)
	}

	// Get failFast
	failFast := false
	if v, ok := step.Config["failFast"].(bool); ok {
		failFast = v
	}

	// Create cancellable context for failFast
	execCtx := ctx
	var cancel context.CancelFunc
	if failFast {
		execCtx, cancel = context.WithCancel(ctx)
		defer cancel()
	}

	// Track results
	results := make([]ParallelStepResult, len(parallelSteps))
	var failedCount int
	var mu sync.Mutex
	var firstErr error

	// Execute steps in parallel with optional concurrency limit
	var sem chan struct{}
	if maxParallel > 0 {
		sem = make(chan struct{}, maxParallel)
	}

	var wg sync.WaitGroup
	for i, pStep := range parallelSteps {
		// Check for cancellation
		select {
		case <-execCtx.Done():
			return &runbook.StepResult{
				Success:  false,
				Message:  "parallel execution cancelled",
				Duration: time.Since(startTime),
				Outputs: map[string]interface{}{
					"results": results,
				},
			}, execCtx.Err()
		default:
		}

		// Acquire semaphore if limited
		if sem != nil {
			sem <- struct{}{}
		}

		wg.Add(1)
		go func(idx int, s runbook.Step) {
			defer wg.Done()
			if sem != nil {
				defer func() { <-sem }()
			}

			stepResult := ParallelStepResult{
				StepName: s.Name,
				Index:    idx,
				Success:  true,
			}

			if h.stepExecutor != nil {
				// Execute single step as a list
				execErr := h.stepExecutor.ExecuteSteps(execCtx, []runbook.Step{s}, varCtx)
				if execErr != nil {
					stepResult.Success = false
					stepResult.Error = execErr
				}
			}

			mu.Lock()
			results[idx] = stepResult
			if !stepResult.Success {
				failedCount++
				if firstErr == nil {
					firstErr = stepResult.Error
				}
				if failFast && cancel != nil {
					cancel()
				}
			}
			mu.Unlock()
		}(i, pStep)
	}

	wg.Wait()

	success := failedCount == 0
	message := fmt.Sprintf("completed %d steps (%d failed)", len(parallelSteps), failedCount)

	return &runbook.StepResult{
		Success:  success,
		Message:  message,
		Duration: time.Since(startTime),
		Outputs: map[string]interface{}{
			"completed_count": len(parallelSteps),
			"failed_count":    failedCount,
			"results":         results,
		},
	}, firstErr
}

// ParallelStepResult holds the result of a parallel step execution.
type ParallelStepResult struct {
	StepName string `json:"step_name"`
	Index    int    `json:"index"`
	Success  bool   `json:"success"`
	Error    error  `json:"error,omitempty"`
}
