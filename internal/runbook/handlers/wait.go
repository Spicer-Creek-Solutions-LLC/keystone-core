package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// WaitHandler implements a delay or condition wait step.
type WaitHandler struct{}

// NewWaitHandler creates a new wait handler.
func NewWaitHandler() *WaitHandler {
	return &WaitHandler{}
}

// Type returns the step type.
func (h *WaitHandler) Type() runbook.StepType {
	return runbook.StepTypeWait
}

// Validate checks step config.
func (h *WaitHandler) Validate(step *runbook.Step) error {
	_, hasDuration := step.Config["duration"]
	_, hasCondition := step.Config["condition"]

	if !hasDuration && !hasCondition {
		return errors.New("wait step requires either 'duration' or 'condition' in config")
	}

	if hasDuration {
		dur, ok := step.Config["duration"].(string)
		if !ok {
			return errors.New("duration must be a string")
		}
		if _, err := time.ParseDuration(dur); err != nil {
			return errors.New("invalid duration format")
		}
	}

	return nil
}

// Execute runs the step.
func (h *WaitHandler) Execute(ctx context.Context, step *runbook.Step, vars VariableContext) (*runbook.StepResult, error) {
	start := time.Now()

	// Handle duration-based wait
	if durStr, ok := step.Config["duration"].(string); ok {
		dur, err := time.ParseDuration(durStr)
		if err != nil {
			return &runbook.StepResult{
				Success:  false,
				Message:  "invalid duration: " + err.Error(),
				Duration: time.Since(start),
			}, err
		}

		if err := wait.ForContext(ctx, dur); err != nil {
			return &runbook.StepResult{
				Success:  false,
				Message:  "wait interrupted: " + err.Error(),
				Duration: time.Since(start),
			}, err
		}

		return &runbook.StepResult{
			Success:  true,
			Message:  "waited " + durStr,
			Duration: time.Since(start),
			Outputs:  make(map[string]interface{}),
		}, nil
	}

	// Handle condition-based wait (future implementation)
	if condition, ok := step.Config["condition"].(string); ok {
		// For now, just log that conditions aren't fully implemented
		_ = condition

		// Get polling interval and timeout
		interval := 5 * time.Second
		if intStr, ok := step.Config["interval"].(string); ok {
			if d, err := time.ParseDuration(intStr); err == nil {
				interval = d
			}
		}

		timeout := 5 * time.Minute
		if toStr, ok := step.Config["timeout"].(string); ok {
			if d, err := time.ParseDuration(toStr); err == nil {
				timeout = d
			}
		}

		// For now, just wait a single interval (full condition evaluation in Week 4)
		_ = timeout // Will be used for condition timeout
		if err := wait.ForContext(ctx, interval); err != nil {
			return &runbook.StepResult{
				Success:  false,
				Message:  "condition wait interrupted",
				Duration: time.Since(start),
			}, err
		}

		return &runbook.StepResult{
			Success:  true,
			Message:  "condition wait completed (stub implementation)",
			Duration: time.Since(start),
			Outputs:  make(map[string]interface{}),
		}, nil
	}

	return &runbook.StepResult{
		Success:  false,
		Message:  "no wait configuration found",
		Duration: time.Since(start),
	}, errors.New("no wait configuration found")
}
