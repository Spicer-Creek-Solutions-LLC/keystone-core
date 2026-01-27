package handlers

import (
	"context"
	"errors"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// FailHandler implements an intentional failure step for testing error handling.
type FailHandler struct{}

// NewFailHandler creates a new fail handler.
func NewFailHandler() *FailHandler {
	return &FailHandler{}
}

// Type returns the step type.
func (h *FailHandler) Type() runbook.StepType {
	return runbook.StepTypeFail
}

// Validate checks step config.
func (h *FailHandler) Validate(step *runbook.Step) error {
	if _, ok := step.Config["message"]; !ok {
		return errors.New("fail step requires 'message' in config")
	}
	return nil
}

// Execute runs the step (always fails).
func (h *FailHandler) Execute(ctx context.Context, step *runbook.Step, vars VariableContext) (*runbook.StepResult, error) {
	start := time.Now()

	// Get message from config
	message := "intentional failure"
	if msg, ok := step.Config["message"].(string); ok && msg != "" {
		// Resolve template in message
		resolved, err := vars.Resolve(msg)
		if err == nil {
			message = resolved
		} else {
			message = msg
		}
	}

	return &runbook.StepResult{
		Success:  false,
		Message:  message,
		Duration: time.Since(start),
		Outputs:  make(map[string]interface{}),
	}, errors.New(message)
}
