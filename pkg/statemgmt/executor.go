package statemgmt

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/titananvil/titan-anvil/pkg/events"
)

// Executor executes state declarations
type Executor struct {
	// Registry contains available modules
	Registry *ModuleRegistry

	// DryRun indicates whether to only preview changes
	DryRun bool

	// EventPublisher publishes events (optional)
	EventPublisher events.EventPublisher

	// EventSource identifies the source of events
	EventSource string
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

	// Emit state.apply.start event
	e.emitEvent(events.EventTypeStateApplyStart, events.SeverityInfo, map[string]interface{}{
		"run_id":     run.RunID,
		"state_file": stateFile.Path,
		"dry_run":    e.DryRun,
	})

	// Resolve execution order based on dependencies
	executionOrder, err := ResolveExecutionOrder(stateFile)
	if err != nil {
		run.EndTime = time.Now()
		run.Summary = &RunSummary{
			Total:   0,
			Failed:  1,
			Success: false,
		}

		// Emit state.apply.fail event
		e.emitEvent(events.EventTypeStateApplyFail, events.SeverityError, map[string]interface{}{
			"run_id":     run.RunID,
			"state_file": stateFile.Path,
			"error":      err.Error(),
			"reason":     "dependency resolution failed",
		})

		return run, fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	// Track state results for watch/onchanges
	stateResults := make(map[string]*StateResult)

	// Execute states in dependency order
	for _, decl := range executionOrder {
		result, err := e.executeDeclaration(ctx, decl.Module, decl)
		if err != nil {
			// Record error but continue
			result = &StateResult{
				StateID:   decl.ID,
				Module:    decl.Module,
				Success:   false,
				Error:     err,
				Comment:   fmt.Sprintf("Execution error: %v", err),
				StartTime: time.Now(),
				EndTime:   time.Now(),
			}
		}

		run.Results = append(run.Results, result)
		stateKey := decl.Module + ":" + decl.ID
		stateResults[stateKey] = result

		// Emit state.change event if state was changed
		if result.Changed {
			e.emitEvent(events.EventTypeStateChange, events.SeverityInfo, map[string]interface{}{
				"run_id":   run.RunID,
				"state_id": result.StateID,
				"module":   result.Module,
				"success":  result.Success,
				"changed":  result.Changed,
				"comment":  result.Comment,
			})
		}

		// Check for fail_hard
		if decl.FailHard && !result.Success {
			run.EndTime = time.Now()
			run.Summary = e.calculateSummary(run)

			// Emit state.apply.fail event
			e.emitEvent(events.EventTypeStateApplyFail, events.SeverityError, map[string]interface{}{
				"run_id":     run.RunID,
				"state_file": stateFile.Path,
				"failed_state": result.StateID,
				"error":      result.Error.Error(),
				"reason":     "fail_hard triggered",
			})

			return run, fmt.Errorf("state %s.%s failed and fail_hard is set", decl.Module, decl.ID)
		}
	}

	run.EndTime = time.Now()
	run.Summary = e.calculateSummary(run)

	// Emit state.apply.done event
	severity := events.SeverityInfo
	if run.Summary.Failed > 0 {
		severity = events.SeverityWarning
	}

	e.emitEvent(events.EventTypeStateApplyDone, severity, map[string]interface{}{
		"run_id":       run.RunID,
		"state_file":   stateFile.Path,
		"total":        run.Summary.Total,
		"succeeded":    run.Summary.Succeeded,
		"failed":       run.Summary.Failed,
		"changed":      run.Summary.Changed,
		"unchanged":    run.Summary.Unchanged,
		"duration_ms":  run.EndTime.Sub(run.StartTime).Milliseconds(),
		"dry_run":      e.DryRun,
	})

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

// emitEvent emits an event if EventPublisher is configured
func (e *Executor) emitEvent(eventType events.EventType, severity events.Severity, data map[string]interface{}) {
	if e.EventPublisher == nil {
		return
	}

	source := e.EventSource
	if source == "" {
		source = "/state-manager"
	}

	// Extract correlation ID from data if present
	correlationID := ""
	if cid, ok := data["run_id"].(string); ok {
		correlationID = cid
	}

	eventBuilder := events.NewEvent(eventType).
		Source(source).
		Severity(severity).
		DataMap(data)

	// Set correlation ID if available
	if correlationID != "" {
		eventBuilder = eventBuilder.CorrelationID(correlationID)
	}

	event := eventBuilder.Build()

	// Use async publish to avoid blocking state execution
	if err := e.EventPublisher.PublishAsync(event); err != nil {
		// Log error but don't fail state execution
		fmt.Printf("Warning: failed to emit event %s: %v\n", eventType, err)
	}
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
