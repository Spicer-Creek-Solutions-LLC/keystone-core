package handlers

import (
	"context"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// NoopHandler implements a no-operation step for testing and placeholders.
type NoopHandler struct{}

// NewNoopHandler creates a new noop handler.
func NewNoopHandler() *NoopHandler {
	return &NoopHandler{}
}

// Type returns the step type.
func (h *NoopHandler) Type() runbook.StepType {
	return runbook.StepTypeNoop
}

// Validate checks step config.
func (h *NoopHandler) Validate(step *runbook.Step) error {
	// No validation required for noop
	return nil
}

// Execute runs the step.
func (h *NoopHandler) Execute(ctx context.Context, step *runbook.Step, vars VariableContext) (*runbook.StepResult, error) {
	start := time.Now()

	// Extract optional message from config
	message := "no operation performed"
	if msg, ok := step.Config["message"].(string); ok && msg != "" {
		message = msg
	}

	return &runbook.StepResult{
		Success:  true,
		Message:  message,
		Duration: time.Since(start),
		Outputs:  make(map[string]interface{}),
	}, nil
}
