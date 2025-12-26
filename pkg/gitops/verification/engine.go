package verification

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Engine orchestrates verification workflows
type Engine struct {
	verifiers map[VerificationType]Verifier
	mu        sync.RWMutex
}

// NewEngine creates a new verification engine
func NewEngine() *Engine {
	return &Engine{
		verifiers: make(map[VerificationType]Verifier),
	}
}

// RegisterVerifier registers a verification module
func (e *Engine) RegisterVerifier(verifier Verifier) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.verifiers[verifier.Type()] = verifier
}

// Execute executes a verification workflow
func (e *Engine) Execute(ctx context.Context, workflow *VerificationWorkflow) (*WorkflowResult, error) {
	result := &WorkflowResult{
		WorkflowName: workflow.Name,
		StartTime:    time.Now(),
		TotalSteps:   len(workflow.Steps),
	}

	// Set workflow timeout
	if workflow.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, workflow.Timeout)
		defer cancel()
	}

	// Execute steps
	if workflow.Parallel {
		result.Steps = e.executeParallel(ctx, workflow.Steps)
	} else {
		result.Steps = e.executeSequential(ctx, workflow.Steps)
	}

	// Calculate statistics
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	for _, step := range result.Steps {
		if step.Success {
			result.PassedSteps++
		} else {
			result.FailedSteps++
		}
	}

	// Overall success if all non-optional steps passed
	result.Success = result.FailedSteps == 0

	return result, nil
}

// executeSequential executes steps one by one
func (e *Engine) executeSequential(ctx context.Context, steps []*VerificationStep) []*VerificationResult {
	results := make([]*VerificationResult, 0, len(steps))

	for _, step := range steps {
		// Check context cancellation
		select {
		case <-ctx.Done():
			// Add skipped result for remaining steps
			results = append(results, &VerificationResult{
				StepName:  step.Name,
				Success:   false,
				Message:   "Workflow cancelled",
				Timestamp: time.Now(),
			})
			continue
		default:
		}

		result := e.executeStep(ctx, step)
		results = append(results, result)

		// Stop on failure unless ContinueOnFailure is set
		if !result.Success && !step.ContinueOnFailure {
			// Mark remaining steps as skipped
			for i := len(results); i < len(steps); i++ {
				results = append(results, &VerificationResult{
					StepName:  steps[i].Name,
					Success:   false,
					Message:   "Skipped due to previous failure",
					Timestamp: time.Now(),
				})
			}
			break
		}
	}

	return results
}

// executeParallel executes steps concurrently
func (e *Engine) executeParallel(ctx context.Context, steps []*VerificationStep) []*VerificationResult {
	results := make([]*VerificationResult, len(steps))
	var wg sync.WaitGroup

	for i, step := range steps {
		wg.Add(1)
		go func(idx int, s *VerificationStep) {
			defer wg.Done()
			results[idx] = e.executeStep(ctx, s)
		}(i, step)
	}

	wg.Wait()
	return results
}

// executeStep executes a single verification step with retries
func (e *Engine) executeStep(ctx context.Context, step *VerificationStep) *VerificationResult {
	start := time.Now()

	// Get verifier for this step type
	e.mu.RLock()
	verifier, ok := e.verifiers[step.Type]
	e.mu.RUnlock()

	if !ok {
		return &VerificationResult{
			StepName:  step.Name,
			Success:   false,
			Message:   fmt.Sprintf("No verifier registered for type: %s", step.Type),
			Timestamp: start,
			Duration:  time.Since(start),
		}
	}

	// Execute with retries
	var result *VerificationResult
	var err error
	attempts := 0
	maxAttempts := step.Retries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempts < maxAttempts {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return &VerificationResult{
				StepName:  step.Name,
				Success:   false,
				Message:   "Step cancelled",
				Timestamp: start,
				Duration:  time.Since(start),
				Error:     ctx.Err(),
			}
		default:
		}

		// Create step context with timeout
		if step.Timeout > 0 {
			var cancel context.CancelFunc
			_, cancel = context.WithTimeout(ctx, step.Timeout)
			defer cancel()
			// TODO: Pass timeout context to verifier.Verify() when API is updated
		}

		// Execute verification
		result, err = verifier.Verify(step)
		attempts++

		if err == nil && result.Success {
			result.Retries = attempts - 1
			result.Duration = time.Since(start)
			return result
		}

		// Retry delay
		if attempts < maxAttempts && step.RetryDelay > 0 {
			select {
			case <-time.After(step.RetryDelay):
			case <-ctx.Done():
				break
			}
		}
	}

	// All retries failed
	if result == nil {
		result = &VerificationResult{
			StepName:  step.Name,
			Success:   false,
			Message:   "Verification failed",
			Error:     err,
		}
	}
	result.Retries = attempts - 1
	result.Duration = time.Since(start)
	result.Timestamp = start

	return result
}

// GetVerifier returns a registered verifier by type
func (e *Engine) GetVerifier(vtype VerificationType) (Verifier, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	verifier, ok := e.verifiers[vtype]
	return verifier, ok
}
