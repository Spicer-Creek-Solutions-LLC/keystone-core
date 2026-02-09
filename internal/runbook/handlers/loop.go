package handlers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// LoopHandler executes steps for each item in a collection.
// Configuration:
//   - items: Expression returning a collection to iterate over (required)
//   - as: Name of the loop variable (default: "item")
//   - indexAs: Name of the index variable (default: "index")
//   - steps: Steps to execute for each item (required)
//   - maxParallel: Maximum concurrent executions (0 = sequential, default: 0)
//   - continueOnError: Continue iteration even if a step fails (default: false)
type LoopHandler struct {
	stepExecutor StepExecutor
}

// NewLoopHandler creates a new loop handler.
func NewLoopHandler(executor StepExecutor) *LoopHandler {
	return &LoopHandler{stepExecutor: executor}
}

// Type returns the step type.
func (h *LoopHandler) Type() runbook.StepType {
	return runbook.StepTypeLoop
}

// Validate validates the step configuration.
func (h *LoopHandler) Validate(step *runbook.Step) error {
	if _, ok := step.Config["items"]; !ok {
		return fmt.Errorf("loop step requires 'items' configuration")
	}

	if _, ok := step.Config["steps"]; !ok {
		return fmt.Errorf("loop step requires 'steps' configuration")
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

// Execute executes the loop step.
func (h *LoopHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	startTime := time.Now()

	// Get items - can be a direct array or a string expression
	var items []interface{}
	var err error

	switch v := step.Config["items"].(type) {
	case string:
		// Resolve items expression
		itemsValue, resolveErr := varCtx.ResolveValue(v)
		if resolveErr != nil {
			return &runbook.StepResult{
				Success:  false,
				Message:  fmt.Sprintf("items resolution failed: %v", resolveErr),
				Duration: time.Since(startTime),
			}, resolveErr
		}
		items, err = toSlice(itemsValue)
	case []interface{}:
		items = v
	case []string:
		items = make([]interface{}, len(v))
		for i, s := range v {
			items[i] = s
		}
	default:
		items, err = toSlice(v)
	}

	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("items must be a list: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	// Get loop variable names
	itemVar := "item"
	if v, ok := step.Config["as"].(string); ok && v != "" {
		itemVar = v
	}

	indexVar := "index"
	if v, ok := step.Config["indexAs"].(string); ok && v != "" {
		indexVar = v
	}

	// Get maxParallel
	maxParallel := 0
	switch v := step.Config["maxParallel"].(type) {
	case int:
		maxParallel = v
	case float64:
		maxParallel = int(v)
	}

	// Get continueOnError
	continueOnError := false
	if v, ok := step.Config["continueOnError"].(bool); ok {
		continueOnError = v
	}

	// Parse steps
	loopSteps, err := parseStepList(step.Config["steps"])
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("invalid loop steps: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	// Create results tracking
	var loopResults []LoopIterationResult
	var failedCount int
	var mu sync.Mutex

	if maxParallel <= 0 || maxParallel == 1 {
		// Sequential execution
		for i, item := range items {
			select {
			case <-ctx.Done():
				return &runbook.StepResult{
					Success:  false,
					Message:  "loop cancelled",
					Duration: time.Since(startTime),
					Outputs: map[string]interface{}{
						"completed_count": i,
						"total_count":     len(items),
						"failed_count":    failedCount,
					},
				}, ctx.Err()
			default:
			}

			iterResult := h.executeIteration(ctx, loopSteps, varCtx, item, i, len(items), itemVar, indexVar)
			loopResults = append(loopResults, iterResult)

			if !iterResult.Success {
				failedCount++
				if !continueOnError {
					return &runbook.StepResult{
						Success:  false,
						Message:  fmt.Sprintf("loop iteration %d failed: %v", i, iterResult.Error),
						Duration: time.Since(startTime),
						Outputs: map[string]interface{}{
							"completed_count": i + 1,
							"total_count":     len(items),
							"failed_count":    failedCount,
							"iterations":      loopResults,
						},
					}, iterResult.Error
				}
			}
		}
	} else {
		// Parallel execution with concurrency limit
		sem := make(chan struct{}, maxParallel)
		var wg sync.WaitGroup
		results := make([]LoopIterationResult, len(items))
		var firstErr error

		for i, item := range items {
			select {
			case <-ctx.Done():
				return &runbook.StepResult{
					Success:  false,
					Message:  "loop cancelled",
					Duration: time.Since(startTime),
				}, ctx.Err()
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(idx int, itm interface{}) {
				defer wg.Done()
				defer func() { <-sem }()

				result := h.executeIteration(ctx, loopSteps, varCtx, itm, idx, len(items), itemVar, indexVar)

				mu.Lock()
				results[idx] = result
				if !result.Success {
					failedCount++
					if firstErr == nil {
						firstErr = result.Error
					}
				}
				mu.Unlock()
			}(i, item)
		}

		wg.Wait()
		loopResults = results

		if firstErr != nil && !continueOnError {
			return &runbook.StepResult{
				Success:  false,
				Message:  fmt.Sprintf("loop failed: %v", firstErr),
				Duration: time.Since(startTime),
				Outputs: map[string]interface{}{
					"completed_count": len(items),
					"total_count":     len(items),
					"failed_count":    failedCount,
					"iterations":      loopResults,
				},
			}, firstErr
		}
	}

	return &runbook.StepResult{
		Success:  failedCount == 0 || continueOnError,
		Message:  fmt.Sprintf("completed %d iterations (%d failed)", len(items), failedCount),
		Duration: time.Since(startTime),
		Outputs: map[string]interface{}{
			"completed_count": len(items),
			"total_count":     len(items),
			"failed_count":    failedCount,
			"iterations":      loopResults,
		},
	}, nil
}

// LoopIterationResult holds the result of a single loop iteration.
type LoopIterationResult struct {
	Index   int                    `json:"index"`
	Item    interface{}            `json:"item"`
	Success bool                   `json:"success"`
	Error   error                  `json:"error,omitempty"`
	Outputs map[string]interface{} `json:"outputs,omitempty"`
}

// executeIteration executes a single loop iteration.
func (h *LoopHandler) executeIteration(
	ctx context.Context,
	steps []runbook.Step,
	varCtx VariableContext,
	item interface{},
	index int,
	total int,
	itemVar string,
	indexVar string,
) LoopIterationResult {
	result := LoopIterationResult{
		Index:   index,
		Item:    item,
		Success: true,
	}

	if h.stepExecutor == nil {
		return result
	}

	// Create a loop context wrapper that adds loop variables
	loopCtx := &loopVariableContext{
		parent:   varCtx,
		itemVar:  itemVar,
		item:     item,
		indexVar: indexVar,
		index:    index,
		total:    total,
	}

	if err := h.stepExecutor.ExecuteSteps(ctx, steps, loopCtx); err != nil {
		result.Success = false
		result.Error = err
	}

	return result
}

// loopVariableContext wraps a VariableContext and adds loop variables.
type loopVariableContext struct {
	parent   VariableContext
	itemVar  string
	item     interface{}
	indexVar string
	index    int
	total    int
}

func (c *loopVariableContext) GetInput(name string) (interface{}, bool) {
	// Check loop variables first
	switch name {
	case c.itemVar:
		return c.item, true
	case c.indexVar:
		return c.index, true
	case "loop_first":
		return c.index == 0, true
	case "loop_last":
		return c.index == c.total-1, true
	case "loop_total":
		return c.total, true
	}
	return c.parent.GetInput(name)
}

func (c *loopVariableContext) GetStepOutput(stepName, outputName string) (interface{}, bool) {
	return c.parent.GetStepOutput(stepName, outputName)
}

func (c *loopVariableContext) ExecutionID() string {
	return c.parent.ExecutionID()
}

func (c *loopVariableContext) RunbookName() string {
	return c.parent.RunbookName()
}

func (c *loopVariableContext) Resolve(template string) (string, error) {
	return c.parent.Resolve(template)
}

func (c *loopVariableContext) ResolveValue(template string) (interface{}, error) {
	return c.parent.ResolveValue(template)
}

func (c *loopVariableContext) EvaluateCondition(expr string) (bool, error) {
	return c.parent.EvaluateCondition(expr)
}

// toSlice converts an interface{} to a slice.
func toSlice(v interface{}) ([]interface{}, error) {
	switch val := v.(type) {
	case []interface{}:
		return val, nil
	case []string:
		result := make([]interface{}, len(val))
		for i, s := range val {
			result[i] = s
		}
		return result, nil
	case []int:
		result := make([]interface{}, len(val))
		for i, n := range val {
			result[i] = n
		}
		return result, nil
	case []float64:
		result := make([]interface{}, len(val))
		for i, n := range val {
			result[i] = n
		}
		return result, nil
	case map[string]interface{}:
		// Convert map to list of key-value pairs
		result := make([]interface{}, 0, len(val))
		for k, v := range val {
			result = append(result, map[string]interface{}{
				"key":   k,
				"value": v,
			})
		}
		return result, nil
	case string:
		// Convert string to list of characters
		result := make([]interface{}, len(val))
		for i, c := range val {
			result[i] = string(c)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to slice", v)
	}
}
