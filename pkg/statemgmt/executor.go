package statemgmt

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Executor executes state declarations
type Executor struct {
	// Registry contains available modules
	Registry *ModuleRegistry

	// DryRun indicates whether to only preview changes
	DryRun bool
}

// NewExecutor creates a new executor
func NewExecutor() *Executor {
	return &Executor{
		Registry: DefaultRegistry,
		DryRun:   false,
	}
}

// ExecuteState executes a state file
func (e *Executor) ExecuteState(ctx context.Context, stateFile *StateFile) (*StateRun, error) {
	run := &StateRun{
		RunID:      uuid.New().String(),
		StateFiles: []string{stateFile.Path},
		DryRun:     e.DryRun,
		StartTime:  time.Now(),
		Results:    make([]*StateResult, 0),
	}

	// Execute all state declarations
	for module, declarations := range stateFile.States {
		for _, decl := range declarations {
			result, err := e.executeDeclaration(ctx, module, &decl)
			if err != nil {
				// Record error but continue
				result = &StateResult{
					StateID:   decl.ID,
					Module:    module,
					Success:   false,
					Error:     err,
					Comment:   fmt.Sprintf("Execution error: %v", err),
					StartTime: time.Now(),
					EndTime:   time.Now(),
				}
			}

			run.Results = append(run.Results, result)

			// Check for fail_hard
			if decl.FailHard && !result.Success {
				run.EndTime = time.Now()
				run.Summary = e.calculateSummary(run)
				return run, fmt.Errorf("state %s.%s failed and fail_hard is set", module, decl.ID)
			}
		}
	}

	run.EndTime = time.Now()
	run.Summary = e.calculateSummary(run)

	return run, nil
}

// executeDeclaration executes a single state declaration
func (e *Executor) executeDeclaration(ctx context.Context, moduleName string, decl *StateDeclaration) (*StateResult, error) {
	// Get the module
	module, err := e.Registry.Get(moduleName)
	if err != nil {
		return nil, fmt.Errorf("module not found: %w", err)
	}

	// Check conditions
	if !e.shouldRun(ctx, decl) {
		return &StateResult{
			StateID:   decl.ID,
			Module:    moduleName,
			Success:   true,
			Changed:   false,
			Comment:   "Skipped due to condition",
			StartTime: time.Now(),
			EndTime:   time.Now(),
		}, nil
	}

	// If dry-run, only check
	if e.DryRun {
		checkResult, err := module.Check(ctx, decl)
		if err != nil {
			return nil, fmt.Errorf("check failed: %w", err)
		}

		return &StateResult{
			StateID:   decl.ID,
			Module:    moduleName,
			Success:   true,
			Changed:   !checkResult.Matches,
			Comment:   "Dry run - would apply changes",
			Changes:   checkResult.Diff,
			StartTime: time.Now(),
			EndTime:   time.Now(),
		}, nil
	}

	// Apply the state with retry logic
	var result *StateResult
	var applyErr error

	attempts := 1
	if decl.Retry != nil && decl.Retry.Attempts > 0 {
		attempts = decl.Retry.Attempts
	}

	delay := time.Second
	if decl.Retry != nil && decl.Retry.Delay > 0 {
		delay = decl.Retry.Delay
	}

	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			// Apply backoff
			if decl.Retry != nil && decl.Retry.BackoffMultiplier > 0 {
				delay = time.Duration(float64(delay) * decl.Retry.BackoffMultiplier)
				if decl.Retry.MaxDelay > 0 && delay > decl.Retry.MaxDelay {
					delay = decl.Retry.MaxDelay
				}
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		result, applyErr = module.Apply(ctx, decl)
		if applyErr == nil && result.Success {
			break
		}
	}

	if applyErr != nil {
		return nil, applyErr
	}

	return result, nil
}

// shouldRun checks if a state should run based on unless/only_if conditions
func (e *Executor) shouldRun(ctx context.Context, decl *StateDeclaration) bool {
	// Check "unless" condition - if true, skip execution
	if decl.Unless != "" {
		if e.evaluateCondition(ctx, decl.Unless) {
			return false
		}
	}

	// Check "only_if" condition - if false, skip execution
	if decl.OnlyIf != "" {
		if !e.evaluateCondition(ctx, decl.OnlyIf) {
			return false
		}
	}

	return true
}

// evaluateCondition evaluates a shell command condition
func (e *Executor) evaluateCondition(ctx context.Context, condition string) bool {
	// Create a temporary cmd module to evaluate the condition
	cmdModule := NewCmdModule()

	decl := &StateDeclaration{
		ID:         condition,
		Module:     "cmd",
		State:      "run",
		Parameters: make(map[string]interface{}),
	}

	result, err := cmdModule.Apply(ctx, decl)
	if err != nil || !result.Success {
		return false
	}

	return true
}

// calculateSummary calculates the run summary
func (e *Executor) calculateSummary(run *StateRun) *RunSummary {
	summary := &RunSummary{
		Total:    len(run.Results),
		Duration: run.EndTime.Sub(run.StartTime),
	}

	for _, result := range run.Results {
		if result.Success {
			summary.Succeeded++
			if result.Changed {
				summary.Changed++
			} else {
				summary.Unchanged++
			}
		} else {
			summary.Failed++
		}
	}

	summary.Success = (summary.Failed == 0)

	return summary
}

// ApplyState is a convenience function to execute a state file
func ApplyState(ctx context.Context, stateFile *StateFile, dryRun bool) (*StateRun, error) {
	executor := NewExecutor()
	executor.DryRun = dryRun
	return executor.ExecuteState(ctx, stateFile)
}

// CheckState checks a state file without applying changes
func CheckState(ctx context.Context, stateFile *StateFile) (*StateRun, error) {
	return ApplyState(ctx, stateFile, true)
}
