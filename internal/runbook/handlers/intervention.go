package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/internal/runbook"
	"github.com/shawnbutts/keystone-core/internal/runbook/intervention"
)

// InterventionManager defines the interface for intervention operations.
type InterventionManager interface {
	// CreateRequest creates a new intervention request.
	CreateRequest(ctx context.Context, config *intervention.Config, executionID, stepName string, metadata map[string]interface{}) (*intervention.Request, error)

	// WaitForResponse waits for an intervention request to complete.
	WaitForResponse(ctx context.Context, requestID string) (*intervention.Request, error)

	// GetRequestByExecution retrieves an intervention request by execution ID and step name.
	GetRequestByExecution(ctx context.Context, executionID, stepName string) (*intervention.Request, error)

	// Cancel cancels a pending intervention request.
	Cancel(ctx context.Context, requestID string, reason string) (*intervention.Request, error)
}

// InterventionHandler handles intervention step execution (prompt, wait_manual, confirm).
// Configuration varies by intervention type:
//
// For all types:
//   - title: Brief description of what's being requested (required)
//   - description: Detailed context for the operator (optional)
//   - timeout: Duration to wait for response (e.g., "1h", "24h")
//   - notifyChannels: Notification channels for intervention requests
//
// For prompt type:
//   - prompts: List of input field definitions
//     - name: Field identifier (required)
//     - label: Display label (optional)
//     - type: Field type (text, number, boolean, select, multiselect, textarea, password)
//     - required: Whether field is required (default: false)
//     - default: Default value
//     - options: For select/multiselect, list of {value, label} options
//     - validation: Validation rules {pattern, min, max}
//
// For confirm type:
//   - confirmMessage: Message displayed for confirmation (optional)
type InterventionHandler struct {
	manager      InterventionManager
	handlerType  runbook.StepType
}

// NewPromptHandler creates a new prompt step handler.
func NewPromptHandler(manager InterventionManager) *InterventionHandler {
	return &InterventionHandler{
		manager:     manager,
		handlerType: runbook.StepTypePrompt,
	}
}

// NewWaitManualHandler creates a new wait_manual step handler.
func NewWaitManualHandler(manager InterventionManager) *InterventionHandler {
	return &InterventionHandler{
		manager:     manager,
		handlerType: runbook.StepTypeWaitManual,
	}
}

// NewConfirmHandler creates a new confirm step handler.
func NewConfirmHandler(manager InterventionManager) *InterventionHandler {
	return &InterventionHandler{
		manager:     manager,
		handlerType: runbook.StepTypeConfirm,
	}
}

// Type returns the step type.
func (h *InterventionHandler) Type() runbook.StepType {
	return h.handlerType
}

// Validate validates the step configuration.
func (h *InterventionHandler) Validate(step *runbook.Step) error {
	if _, ok := step.Config["title"]; !ok {
		return fmt.Errorf("%s step requires 'title' configuration", step.Type)
	}

	// Type-specific validation
	switch step.Type {
	case runbook.StepTypePrompt:
		return h.validatePromptConfig(step.Config)
	case runbook.StepTypeConfirm, runbook.StepTypeWaitManual:
		// No additional required fields
		return nil
	}

	return nil
}

// validatePromptConfig validates prompt-specific configuration.
func (h *InterventionHandler) validatePromptConfig(config map[string]interface{}) error {
	prompts, ok := config["prompts"]
	if !ok {
		return fmt.Errorf("prompt step requires 'prompts' configuration")
	}

	promptList, ok := prompts.([]interface{})
	if !ok {
		return fmt.Errorf("prompts must be a list")
	}

	if len(promptList) == 0 {
		return fmt.Errorf("prompts list cannot be empty")
	}

	for i, p := range promptList {
		prompt, ok := p.(map[string]interface{})
		if !ok {
			return fmt.Errorf("prompts[%d] must be an object", i)
		}

		if _, ok := prompt["name"]; !ok {
			return fmt.Errorf("prompts[%d].name is required", i)
		}

		if fieldType, ok := prompt["type"].(string); ok {
			ft := intervention.FieldType(fieldType)
			if !ft.IsValid() {
				return fmt.Errorf("prompts[%d].type is invalid: %s", i, fieldType)
			}
		}
	}

	return nil
}

// Execute executes the intervention step.
func (h *InterventionHandler) Execute(ctx context.Context, step *runbook.Step, varCtx VariableContext) (*runbook.StepResult, error) {
	startTime := time.Now()

	// Check if manager is available
	if h.manager == nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  "intervention manager not configured",
			Duration: time.Since(startTime),
		}, fmt.Errorf("intervention manager not configured")
	}

	// Build intervention config from step configuration
	config, err := h.buildConfig(step, varCtx)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to build intervention config: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	// Build metadata for the request
	metadata := make(map[string]interface{})
	metadata["runbook_name"] = varCtx.RunbookName()
	metadata["execution_id"] = varCtx.ExecutionID()
	metadata["step_name"] = step.Name

	// Check for existing request (resume case)
	existingReq, err := h.manager.GetRequestByExecution(ctx, varCtx.ExecutionID(), step.Name)
	if err != nil {
		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("failed to check for existing request: %v", err),
			Duration: time.Since(startTime),
		}, err
	}

	var req *intervention.Request
	if existingReq != nil {
		// Use existing request
		req = existingReq
	} else {
		// Create new intervention request
		req, err = h.manager.CreateRequest(ctx, config, varCtx.ExecutionID(), step.Name, metadata)
		if err != nil {
			return &runbook.StepResult{
				Success:  false,
				Message:  fmt.Sprintf("failed to create intervention request: %v", err),
				Duration: time.Since(startTime),
				Outputs: map[string]interface{}{
					"error": err.Error(),
				},
			}, err
		}
	}

	// If already terminal, return immediately
	if req.State.IsTerminal() {
		return h.buildResult(req, startTime), nil
	}

	// Store request ID before waiting (WaitForResponse may return nil on error)
	requestID := req.ID

	// Wait for response
	req, err = h.manager.WaitForResponse(ctx, requestID)
	if err != nil {
		// Check if context was cancelled
		if ctx.Err() != nil {
			// Try to cancel the pending request using a background context
			// since the original context is already cancelled
			cancelCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, cancelErr := h.manager.Cancel(cancelCtx, requestID, "execution cancelled")
			if cancelErr != nil {
				_ = cancelErr
			}
		}

		// Build outputs even when we only have the request ID
		outputs := map[string]interface{}{
			"request_id": requestID,
			"error":      err.Error(),
		}
		if req != nil {
			outputs["state"] = string(req.State)
		}

		return &runbook.StepResult{
			Success:  false,
			Message:  fmt.Sprintf("error waiting for intervention response: %v", err),
			Duration: time.Since(startTime),
			Outputs:  outputs,
		}, err
	}

	return h.buildResult(req, startTime), nil
}

// buildConfig builds an intervention config from step configuration.
func (h *InterventionHandler) buildConfig(step *runbook.Step, varCtx VariableContext) (*intervention.Config, error) {
	config := step.Config
	ic := &intervention.Config{}

	// Set intervention type based on step type
	switch step.Type {
	case runbook.StepTypePrompt:
		ic.Type = intervention.InterventionTypePrompt
	case runbook.StepTypeWaitManual:
		ic.Type = intervention.InterventionTypeWaitManual
	case runbook.StepTypeConfirm:
		ic.Type = intervention.InterventionTypeConfirm
	}

	// Title (required)
	if title, ok := config["title"].(string); ok {
		resolved, err := varCtx.Resolve(title)
		if err != nil {
			return nil, fmt.Errorf("resolve title: %w", err)
		}
		ic.Title = resolved
	}

	// Description (optional)
	if desc, ok := config["description"].(string); ok {
		resolved, err := varCtx.Resolve(desc)
		if err != nil {
			return nil, fmt.Errorf("resolve description: %w", err)
		}
		ic.Description = resolved
	}

	// Timeout (optional)
	if timeout, ok := config["timeout"].(string); ok {
		ic.Timeout = timeout
	}

	// NotifyChannels (optional)
	if channels, ok := config["notifyChannels"].([]interface{}); ok {
		for _, c := range channels {
			if s, ok := c.(string); ok {
				ic.NotifyChannels = append(ic.NotifyChannels, s)
			}
		}
	}

	// ConfirmMessage (optional, for confirm type)
	if msg, ok := config["confirmMessage"].(string); ok {
		resolved, err := varCtx.Resolve(msg)
		if err != nil {
			return nil, fmt.Errorf("resolve confirmMessage: %w", err)
		}
		ic.ConfirmMessage = resolved
	}

	// Prompts (for prompt type)
	if prompts, ok := config["prompts"].([]interface{}); ok {
		for _, p := range prompts {
			if promptMap, ok := p.(map[string]interface{}); ok {
				field, err := h.buildPromptField(promptMap, varCtx)
				if err != nil {
					return nil, err
				}
				ic.Prompts = append(ic.Prompts, *field)
			}
		}
	}

	return ic, nil
}

// buildPromptField builds a prompt field from configuration.
func (h *InterventionHandler) buildPromptField(config map[string]interface{}, varCtx VariableContext) (*intervention.PromptField, error) {
	field := &intervention.PromptField{}

	// Name (required)
	if name, ok := config["name"].(string); ok {
		field.Name = name
	}

	// Label (optional)
	if label, ok := config["label"].(string); ok {
		resolved, err := varCtx.Resolve(label)
		if err != nil {
			return nil, fmt.Errorf("resolve label for %s: %w", field.Name, err)
		}
		field.Label = resolved
	}

	// Type (optional, default: text)
	if fieldType, ok := config["type"].(string); ok {
		field.Type = intervention.FieldType(fieldType)
	} else {
		field.Type = intervention.FieldTypeText
	}

	// Required (optional)
	if required, ok := config["required"].(bool); ok {
		field.Required = required
	}

	// Default (optional)
	if defaultVal, ok := config["default"]; ok {
		field.Default = defaultVal
	}

	// Description (optional)
	if desc, ok := config["description"].(string); ok {
		resolved, err := varCtx.Resolve(desc)
		if err != nil {
			return nil, fmt.Errorf("resolve description for %s: %w", field.Name, err)
		}
		field.Description = resolved
	}

	// Options (for select/multiselect)
	if options, ok := config["options"].([]interface{}); ok {
		for _, opt := range options {
			if optMap, ok := opt.(map[string]interface{}); ok {
				option := intervention.Option{
					Value: optMap["value"],
				}
				if label, ok := optMap["label"].(string); ok {
					resolved, err := varCtx.Resolve(label)
					if err != nil {
						return nil, fmt.Errorf("resolve option label: %w", err)
					}
					option.Label = resolved
				}
				field.Options = append(field.Options, option)
			}
		}
	}

	// Validation (optional)
	if validation, ok := config["validation"].(map[string]interface{}); ok {
		field.Validation = &intervention.FieldValidation{}

		if pattern, ok := validation["pattern"].(string); ok {
			field.Validation.Pattern = pattern
		}
		if min, ok := validation["min"].(float64); ok {
			field.Validation.Min = &min
		}
		if max, ok := validation["max"].(float64); ok {
			field.Validation.Max = &max
		}
	}

	return field, nil
}

// buildResult builds a step result from an intervention request.
func (h *InterventionHandler) buildResult(req *intervention.Request, startTime time.Time) *runbook.StepResult {
	outputs := map[string]interface{}{
		"request_id": req.ID,
		"state":      string(req.State),
		"type":       string(req.Type),
	}

	// Include response details
	if req.Response != nil {
		outputs["operator"] = req.Response.Operator
		outputs["confirmed"] = req.Response.Confirmed
		outputs["comment"] = req.Response.Comment
		outputs["responded_at"] = req.Response.RespondedAt.Format(time.RFC3339)

		// Include values for prompt type
		if len(req.Response.Values) > 0 {
			outputs["values"] = req.Response.Values

			// Also expose values at the top level for easy access
			for k, v := range req.Response.Values {
				outputs[k] = v
			}
		}
	}

	var message string
	var success bool

	switch req.State {
	case intervention.InterventionStateCompleted:
		success = true
		switch req.Type {
		case intervention.InterventionTypeConfirm:
			if req.Response != nil && req.Response.Confirmed {
				message = fmt.Sprintf("confirmed by %s", req.Response.Operator)
			} else {
				message = fmt.Sprintf("declined by %s", req.Response.Operator)
				success = false
			}
		case intervention.InterventionTypeWaitManual:
			message = fmt.Sprintf("acknowledged by %s", req.Response.Operator)
		case intervention.InterventionTypePrompt:
			message = fmt.Sprintf("input provided by %s", req.Response.Operator)
		}
	case intervention.InterventionStateExpired:
		success = false
		message = "intervention request expired"
	case intervention.InterventionStateCancelled:
		success = false
		message = "intervention request cancelled"
	default:
		success = false
		message = fmt.Sprintf("unexpected state: %s", req.State)
	}

	return &runbook.StepResult{
		Success:  success,
		Message:  message,
		Duration: time.Since(startTime),
		Outputs:  outputs,
	}
}
