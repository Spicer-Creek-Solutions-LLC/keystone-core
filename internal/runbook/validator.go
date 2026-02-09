package runbook

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ValidationError represents a validation error with context.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return e.Message
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []*ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	if len(e) == 1 {
		return e[0].Error()
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d validation errors:\n", len(e)))
	for _, err := range e {
		sb.WriteString("  - ")
		sb.WriteString(err.Error())
		sb.WriteString("\n")
	}
	return sb.String()
}

// Validator provides methods for validating runbook definitions.
type Validator struct {
	// AllowFutureStepTypes allows step types planned for future phases.
	AllowFutureStepTypes bool
}

// NewValidator creates a new Validator with default settings.
func NewValidator() *Validator {
	return &Validator{
		AllowFutureStepTypes: false,
	}
}

// Validate checks a runbook for validity and returns any validation errors.
func (v *Validator) Validate(rb *Runbook) error {
	// Validate metadata and spec
	metaErrs := v.validateMetadata(&rb.Metadata)
	specErrs := v.validateSpec(&rb.Spec)

	if len(metaErrs) == 0 && len(specErrs) == 0 {
		return nil
	}

	errs := make(ValidationErrors, 0, len(metaErrs)+len(specErrs))
	errs = append(errs, metaErrs...)
	errs = append(errs, specErrs...)
	return errs
}

// validateMetadata validates the runbook metadata.
func (v *Validator) validateMetadata(m *Metadata) ValidationErrors {
	var errs ValidationErrors

	if m.Name == "" {
		errs = append(errs, &ValidationError{
			Field:   "metadata.name",
			Message: "name is required",
		})
	} else if !isValidName(m.Name) {
		errs = append(errs, &ValidationError{
			Field:   "metadata.name",
			Message: "name must match pattern ^[a-z0-9][a-z0-9-]*[a-z0-9]$ or be a single character [a-z0-9]",
		})
	}

	return errs
}

// validateSpec validates the runbook spec.
func (v *Validator) validateSpec(spec *Spec) ValidationErrors {
	var errs ValidationErrors

	// Validate inputs
	inputNames := make(map[string]bool)
	for i, input := range spec.Inputs {
		inputErrs := v.validateInput(&input, i)
		errs = append(errs, inputErrs...)

		if inputNames[input.Name] {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("spec.inputs[%d].name", i),
				Message: fmt.Sprintf("duplicate input name %q", input.Name),
			})
		}
		inputNames[input.Name] = true
	}

	// Validate steps
	if len(spec.Steps) == 0 {
		errs = append(errs, &ValidationError{
			Field:   "spec.steps",
			Message: "at least one step is required",
		})
	}

	stepNames := make(map[string]bool)
	for i := range spec.Steps {
		step := &spec.Steps[i]
		stepErrs := v.validateStep(step, i, "spec.steps")
		errs = append(errs, stepErrs...)

		if stepNames[step.Name] {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("spec.steps[%d].name", i),
				Message: fmt.Sprintf("duplicate step name %q", step.Name),
			})
		}
		stepNames[step.Name] = true
	}

	// Validate step dependencies
	errs = append(errs, v.validateStepDependencies(spec.Steps, stepNames, "spec.steps")...)

	// Validate onSuccess steps
	for i := range spec.OnSuccess {
		stepErrs := v.validateStep(&spec.OnSuccess[i], i, "spec.onSuccess")
		errs = append(errs, stepErrs...)
	}

	// Validate onFailure steps
	for i := range spec.OnFailure {
		stepErrs := v.validateStep(&spec.OnFailure[i], i, "spec.onFailure")
		errs = append(errs, stepErrs...)
	}

	// Validate timeout
	if spec.Timeout != "" {
		if _, err := time.ParseDuration(spec.Timeout); err != nil {
			errs = append(errs, &ValidationError{
				Field:   "spec.timeout",
				Message: fmt.Sprintf("invalid duration format: %v", err),
			})
		}
	}

	// Validate maxRetries
	if spec.MaxRetries < 0 {
		errs = append(errs, &ValidationError{
			Field:   "spec.maxRetries",
			Message: "maxRetries must be non-negative",
		})
	}

	return errs
}

// validateInput validates an input definition.
func (v *Validator) validateInput(input *InputDef, index int) ValidationErrors {
	var errs ValidationErrors
	prefix := fmt.Sprintf("spec.inputs[%d]", index)

	if input.Name == "" {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".name",
			Message: "name is required",
		})
	} else if !isValidIdentifier(input.Name) {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".name",
			Message: "name must be a valid identifier (alphanumeric and underscores, starting with a letter)",
		})
	}

	if !input.Type.IsValid() {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".type",
			Message: fmt.Sprintf("invalid type %q", input.Type),
		})
	}

	// Validate default value matches type
	if input.Default != nil {
		if err := v.validateInputValue(input.Default, input.Type); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".default",
				Message: fmt.Sprintf("default value type mismatch: %v", err),
			})
		}
	}

	// Validate validation rules
	if input.Validation != nil {
		errs = append(errs, v.validateInputValidation(input.Validation, input.Type, prefix)...)
	}

	return errs
}

// validateInputValue checks if a value matches the expected input type.
func (v *Validator) validateInputValue(value interface{}, inputType InputType) error {
	switch inputType {
	case InputTypeString:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case InputTypeInt:
		switch value.(type) {
		case int, int64, float64:
			// YAML parses integers as int, but JSON may parse as float64
		default:
			return fmt.Errorf("expected int, got %T", value)
		}
	case InputTypeBool:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected bool, got %T", value)
		}
	case InputTypeFloat:
		switch value.(type) {
		case float64, float32, int, int64:
			// Accept any numeric type
		default:
			return fmt.Errorf("expected float, got %T", value)
		}
	case InputTypeList:
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("expected list, got %T", value)
		}
	case InputTypeMap:
		if _, ok := value.(map[string]interface{}); !ok {
			// YAML may parse as map[interface{}]interface{}
			if _, ok := value.(map[interface{}]interface{}); !ok {
				return fmt.Errorf("expected map, got %T", value)
			}
		}
	}
	return nil
}

// validateInputValidation validates input validation rules.
func (v *Validator) validateInputValidation(val *InputValidation, inputType InputType, prefix string) ValidationErrors {
	var errs ValidationErrors

	if val.Pattern != "" {
		if inputType != InputTypeString {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".validation.pattern",
				Message: "pattern validation only applies to string inputs",
			})
		} else if _, err := regexp.Compile(val.Pattern); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".validation.pattern",
				Message: fmt.Sprintf("invalid regex pattern: %v", err),
			})
		}
	}

	if val.Min != nil || val.Max != nil {
		switch inputType {
		case InputTypeInt, InputTypeFloat, InputTypeString, InputTypeList:
			// Valid for numeric types (value) and string/list (length)
		default:
			errs = append(errs, &ValidationError{
				Field:   prefix + ".validation",
				Message: "min/max validation only applies to numeric, string, or list inputs",
			})
		}
	}

	if val.Min != nil && val.Max != nil && *val.Min > *val.Max {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".validation",
			Message: "min cannot be greater than max",
		})
	}

	return errs
}

// validateStep validates a step definition.
func (v *Validator) validateStep(step *Step, index int, prefix string) ValidationErrors {
	var errs ValidationErrors
	stepPrefix := fmt.Sprintf("%s[%d]", prefix, index)

	if step.Name == "" {
		errs = append(errs, &ValidationError{
			Field:   stepPrefix + ".name",
			Message: "name is required",
		})
	} else if !isValidIdentifier(step.Name) {
		errs = append(errs, &ValidationError{
			Field:   stepPrefix + ".name",
			Message: "name must be a valid identifier",
		})
	}

	// Validate step type
	if v.AllowFutureStepTypes {
		if !step.Type.IsValidExtended() {
			errs = append(errs, &ValidationError{
				Field:   stepPrefix + ".type",
				Message: fmt.Sprintf("invalid step type %q", step.Type),
			})
		}
	} else {
		if !step.Type.IsValid() {
			errs = append(errs, &ValidationError{
				Field:   stepPrefix + ".type",
				Message: fmt.Sprintf("unsupported step type %q (supported: command, api, notification, wait, noop, fail, if, switch, loop, parallel, runbook)", step.Type),
			})
		}
	}

	// Validate timeout
	if step.Timeout != "" {
		if _, err := time.ParseDuration(step.Timeout); err != nil {
			errs = append(errs, &ValidationError{
				Field:   stepPrefix + ".timeout",
				Message: fmt.Sprintf("invalid duration format: %v", err),
			})
		}
	}

	// Validate retries
	if step.Retries != nil {
		errs = append(errs, v.validateRetryConfig(step.Retries, stepPrefix)...)
	}

	// Validate outputs
	outputNames := make(map[string]bool)
	for i, output := range step.Outputs {
		outputErrs := v.validateOutput(&output, i, stepPrefix)
		errs = append(errs, outputErrs...)

		if outputNames[output.Name] {
			errs = append(errs, &ValidationError{
				Field:   fmt.Sprintf("%s.outputs[%d].name", stepPrefix, i),
				Message: fmt.Sprintf("duplicate output name %q", output.Name),
			})
		}
		outputNames[output.Name] = true
	}

	// Validate step-type-specific config
	errs = append(errs, v.validateStepConfig(step, stepPrefix)...)

	return errs
}

// validateRetryConfig validates retry configuration.
func (v *Validator) validateRetryConfig(r *RetryConfig, prefix string) ValidationErrors {
	var errs ValidationErrors

	if r.MaxAttempts < 1 {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".retries.maxAttempts",
			Message: "maxAttempts must be at least 1",
		})
	}

	if r.Delay != "" {
		if _, err := time.ParseDuration(r.Delay); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".retries.delay",
				Message: fmt.Sprintf("invalid duration format: %v", err),
			})
		}
	}

	if r.MaxDelay != "" {
		if _, err := time.ParseDuration(r.MaxDelay); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".retries.maxDelay",
				Message: fmt.Sprintf("invalid duration format: %v", err),
			})
		}
	}

	if !r.Backoff.IsValid() {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".retries.backoff",
			Message: fmt.Sprintf("invalid backoff type %q", r.Backoff),
		})
	}

	return errs
}

// validateOutput validates an output definition.
func (v *Validator) validateOutput(output *OutputDef, index int, prefix string) ValidationErrors {
	var errs ValidationErrors
	outputPrefix := fmt.Sprintf("%s.outputs[%d]", prefix, index)

	if output.Name == "" {
		errs = append(errs, &ValidationError{
			Field:   outputPrefix + ".name",
			Message: "name is required",
		})
	} else if !isValidIdentifier(output.Name) {
		errs = append(errs, &ValidationError{
			Field:   outputPrefix + ".name",
			Message: "name must be a valid identifier",
		})
	}

	if !output.Source.IsValid() {
		errs = append(errs, &ValidationError{
			Field:   outputPrefix + ".source",
			Message: fmt.Sprintf("invalid output source %q", output.Source),
		})
	}

	if !output.Parser.IsValid() {
		errs = append(errs, &ValidationError{
			Field:   outputPrefix + ".parser",
			Message: fmt.Sprintf("invalid output parser %q", output.Parser),
		})
	}

	// Validate parser-specific requirements
	switch output.Parser {
	case OutputParserRegex:
		if output.Path == "" {
			errs = append(errs, &ValidationError{
				Field:   outputPrefix + ".path",
				Message: "path (regex pattern) is required for regex parser",
			})
		} else if _, err := regexp.Compile(output.Path); err != nil {
			errs = append(errs, &ValidationError{
				Field:   outputPrefix + ".path",
				Message: fmt.Sprintf("invalid regex pattern: %v", err),
			})
		}
	case OutputParserJSONPath:
		if output.Path == "" {
			errs = append(errs, &ValidationError{
				Field:   outputPrefix + ".path",
				Message: "path (JSONPath expression) is required for jsonpath parser",
			})
		}
	case OutputParserJSON:
		if output.Path == "" {
			errs = append(errs, &ValidationError{
				Field:   outputPrefix + ".path",
				Message: "path (JSON key) is required for json parser",
			})
		}
	default:
	}

	return errs
}

// validateStepConfig validates step-type-specific configuration.
func (v *Validator) validateStepConfig(step *Step, prefix string) ValidationErrors {
	var errs ValidationErrors

	switch step.Type {
	case StepTypeCommand:
		errs = append(errs, v.validateCommandConfig(step.Config, prefix)...)
	case StepTypeAPI:
		errs = append(errs, v.validateAPIConfig(step.Config, prefix)...)
	case StepTypeNotification:
		errs = append(errs, v.validateNotificationConfig(step.Config, prefix)...)
	case StepTypeWait:
		errs = append(errs, v.validateWaitConfig(step.Config, prefix)...)
	case StepTypeFail:
		errs = append(errs, v.validateFailConfig(step.Config, prefix)...)
	case StepTypeIf:
		errs = append(errs, v.validateIfConfig(step.Config, prefix)...)
	case StepTypeSwitch:
		errs = append(errs, v.validateSwitchConfig(step.Config, prefix)...)
	case StepTypeLoop:
		errs = append(errs, v.validateLoopConfig(step.Config, prefix)...)
	case StepTypeParallel:
		errs = append(errs, v.validateParallelConfig(step.Config, prefix)...)
	case StepTypeSubRunbook:
		errs = append(errs, v.validateSubRunbookConfig(step.Config, prefix)...)
	case StepTypeApproval:
		errs = append(errs, v.validateApprovalConfig(step.Config, prefix)...)
	case StepTypePrompt:
		errs = append(errs, v.validatePromptConfig(step.Config, prefix)...)
	case StepTypeConfirm:
		errs = append(errs, v.validateConfirmConfig(step.Config, prefix)...)
	case StepTypeWaitManual:
		errs = append(errs, v.validateWaitManualConfig(step.Config, prefix)...)
	case StepTypeState:
		errs = append(errs, v.validateStateConfig(step.Config, prefix)...)
	case StepTypeDeploy:
		errs = append(errs, v.validateDeployConfig(step.Config, prefix)...)
	case StepTypeRollback:
		errs = append(errs, v.validateRollbackConfig(step.Config, prefix)...)
	case StepTypeScript:
		errs = append(errs, v.validateScriptConfig(step.Config, prefix)...)
	case StepTypeNoop, StepTypeQuery:
		// No config required for noop or query
	default:
	}

	return errs
}

// validateIfConfig validates if step configuration.
func (v *Validator) validateIfConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	if _, ok := config["condition"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.condition",
			Message: "condition is required for if step",
		})
	}

	if _, ok := config["then"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.then",
			Message: "then branch is required for if step",
		})
	}

	return errs
}

// validateSwitchConfig validates switch step configuration.
func (v *Validator) validateSwitchConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	if _, ok := config["value"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.value",
			Message: "value is required for switch step",
		})
	}

	if _, ok := config["cases"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.cases",
			Message: "cases is required for switch step",
		})
	}

	return errs
}

// validateLoopConfig validates loop step configuration.
func (v *Validator) validateLoopConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	if _, ok := config["items"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.items",
			Message: "items is required for loop step",
		})
	}

	if _, ok := config["steps"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.steps",
			Message: "steps is required for loop step",
		})
	}

	return errs
}

// validateParallelConfig validates parallel step configuration.
func (v *Validator) validateParallelConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	if _, ok := config["steps"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.steps",
			Message: "steps is required for parallel step",
		})
	}

	return errs
}

// validateSubRunbookConfig validates sub-runbook step configuration.
func (v *Validator) validateSubRunbookConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	if _, ok := config["runbook"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.runbook",
			Message: "runbook name is required for runbook step",
		})
	}

	return errs
}

// validateCommandConfig validates command step configuration.
func (v *Validator) validateCommandConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	if _, ok := config["command"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.command",
			Message: "command is required",
		})
	}

	return errs
}

// validateAPIConfig validates API step configuration.
func (v *Validator) validateAPIConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	if _, ok := config["url"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.url",
			Message: "url is required",
		})
	}

	if method, ok := config["method"]; ok {
		validMethods := map[string]bool{
			"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true, "HEAD": true, "OPTIONS": true,
		}
		if methodStr, ok := method.(string); !ok || !validMethods[strings.ToUpper(methodStr)] {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.method",
				Message: "method must be a valid HTTP method",
			})
		}
	}

	return errs
}

// validateNotificationConfig validates notification step configuration.
func (v *Validator) validateNotificationConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	if _, ok := config["channel"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.channel",
			Message: "channel is required",
		})
	}

	if _, ok := config["message"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.message",
			Message: "message is required",
		})
	}

	return errs
}

// validateWaitConfig validates wait step configuration.
func (v *Validator) validateWaitConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	duration, hasDuration := config["duration"]
	_, hasCondition := config["condition"]

	if !hasDuration && !hasCondition {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config",
			Message: "either duration or condition is required",
		})
	}

	if hasDuration {
		if durStr, ok := duration.(string); ok {
			if _, err := time.ParseDuration(durStr); err != nil {
				errs = append(errs, &ValidationError{
					Field:   prefix + ".config.duration",
					Message: fmt.Sprintf("invalid duration format: %v", err),
				})
			}
		} else {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.duration",
				Message: "duration must be a string",
			})
		}
	}

	return errs
}

// validateFailConfig validates fail step configuration.
func (v *Validator) validateFailConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	if _, ok := config["message"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.message",
			Message: "message is required",
		})
	}

	return errs
}

// validateApprovalConfig validates approval step configuration.
func (v *Validator) validateApprovalConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	// Title is required
	if _, ok := config["title"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.title",
			Message: "title is required for approval step",
		})
	}

	// Approvers is required
	approvers, ok := config["approvers"]
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.approvers",
			Message: "approvers is required for approval step",
		})
	} else {
		// Validate approvers is a non-empty list of strings
		switch a := approvers.(type) {
		case []interface{}:
			if len(a) == 0 {
				errs = append(errs, &ValidationError{
					Field:   prefix + ".config.approvers",
					Message: "approvers list cannot be empty",
				})
			}
			for i, item := range a {
				if _, ok := item.(string); !ok {
					errs = append(errs, &ValidationError{
						Field:   fmt.Sprintf("%s.config.approvers[%d]", prefix, i),
						Message: "approver must be a string",
					})
				}
			}
		case []string:
			if len(a) == 0 {
				errs = append(errs, &ValidationError{
					Field:   prefix + ".config.approvers",
					Message: "approvers list cannot be empty",
				})
			}
		default:
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.approvers",
				Message: "approvers must be a list of strings",
			})
		}
	}

	// Validate mode if specified
	if mode, ok := config["mode"].(string); ok {
		validModes := map[string]bool{"any": true, "all": true, "count": true}
		if !validModes[mode] {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.mode",
				Message: fmt.Sprintf("invalid approval mode %q (valid: any, all, count)", mode),
			})
		}

		// Require requiredCount for count mode
		if mode == "count" {
			if _, ok := config["requiredCount"]; !ok {
				errs = append(errs, &ValidationError{
					Field:   prefix + ".config.requiredCount",
					Message: "requiredCount is required when mode is 'count'",
				})
			}
		}
	}

	// Validate timeout format if specified
	if timeout, ok := config["timeout"].(string); ok {
		if _, err := time.ParseDuration(timeout); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.timeout",
				Message: fmt.Sprintf("invalid timeout format: %v", err),
			})
		}
	}

	// Validate reminderInterval format if specified
	if interval, ok := config["reminderInterval"].(string); ok {
		if _, err := time.ParseDuration(interval); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.reminderInterval",
				Message: fmt.Sprintf("invalid reminderInterval format: %v", err),
			})
		}
	}

	// Validate escalateAfter format if specified
	if after, ok := config["escalateAfter"].(string); ok {
		if _, err := time.ParseDuration(after); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.escalateAfter",
				Message: fmt.Sprintf("invalid escalateAfter format: %v", err),
			})
		}
	}

	return errs
}

// validatePromptConfig validates prompt step configuration.
func (v *Validator) validatePromptConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	// Title is required
	if _, ok := config["title"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.title",
			Message: "title is required for prompt step",
		})
	}

	// Prompts is required
	prompts, ok := config["prompts"]
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.prompts",
			Message: "prompts is required for prompt step",
		})
	} else {
		promptList, ok := prompts.([]interface{})
		switch {
		case !ok:
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.prompts",
				Message: "prompts must be a list",
			})
		case len(promptList) == 0:
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.prompts",
				Message: "prompts list cannot be empty",
			})
		default:
			// Validate each prompt field
			validFieldTypes := map[string]bool{
				"text": true, "number": true, "boolean": true,
				"select": true, "multiselect": true, "textarea": true, "password": true,
			}
			for i, p := range promptList {
				prompt, ok := p.(map[string]interface{})
				if !ok {
					errs = append(errs, &ValidationError{
						Field:   fmt.Sprintf("%s.config.prompts[%d]", prefix, i),
						Message: "prompt must be an object",
					})
					continue
				}

				// Name is required
				if _, ok := prompt["name"]; !ok {
					errs = append(errs, &ValidationError{
						Field:   fmt.Sprintf("%s.config.prompts[%d].name", prefix, i),
						Message: "name is required",
					})
				}

				// Type must be valid if specified
				if fieldType, ok := prompt["type"].(string); ok {
					if !validFieldTypes[fieldType] {
						errs = append(errs, &ValidationError{
							Field:   fmt.Sprintf("%s.config.prompts[%d].type", prefix, i),
							Message: fmt.Sprintf("invalid field type %q (valid: text, number, boolean, select, multiselect, textarea, password)", fieldType),
						})
					}
				}
			}
		}
	}

	// Validate timeout format if specified
	if timeout, ok := config["timeout"].(string); ok {
		if _, err := time.ParseDuration(timeout); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.timeout",
				Message: fmt.Sprintf("invalid timeout format: %v", err),
			})
		}
	}

	return errs
}

// validateConfirmConfig validates confirm step configuration.
func (v *Validator) validateConfirmConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	// Title is required
	if _, ok := config["title"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.title",
			Message: "title is required for confirm step",
		})
	}

	// Validate timeout format if specified
	if timeout, ok := config["timeout"].(string); ok {
		if _, err := time.ParseDuration(timeout); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.timeout",
				Message: fmt.Sprintf("invalid timeout format: %v", err),
			})
		}
	}

	return errs
}

// validateWaitManualConfig validates wait_manual step configuration.
func (v *Validator) validateWaitManualConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	// Title is required
	if _, ok := config["title"]; !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.title",
			Message: "title is required for wait_manual step",
		})
	}

	// Validate timeout format if specified
	if timeout, ok := config["timeout"].(string); ok {
		if _, err := time.ParseDuration(timeout); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.timeout",
				Message: fmt.Sprintf("invalid timeout format: %v", err),
			})
		}
	}

	return errs
}

// validateStateConfig validates state step configuration.
func (v *Validator) validateStateConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	// Either inline or file is required, but not both
	_, hasInline := config["inline"]
	_, hasFile := config["file"]

	if !hasInline && !hasFile {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config",
			Message: "either 'inline' or 'file' is required for state step",
		})
	}

	if hasInline && hasFile {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config",
			Message: "cannot specify both 'inline' and 'file' for state step",
		})
	}

	// Validate inline is a string if specified
	if inline, ok := config["inline"]; ok {
		if _, ok := inline.(string); !ok {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.inline",
				Message: "inline must be a string",
			})
		}
	}

	// Validate file is a string if specified
	if file, ok := config["file"]; ok {
		if _, ok := file.(string); !ok {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.file",
				Message: "file must be a string",
			})
		}
	}

	return errs
}

// validateDeployConfig validates deploy step configuration.
func (v *Validator) validateDeployConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	// Plan is required
	plan, ok := config["plan"]
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.plan",
			Message: "plan is required for deploy step",
		})
	} else if _, ok := plan.(string); !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.plan",
			Message: "plan must be a string",
		})
	}

	// Validate revisions if specified
	if revisions, ok := config["revisions"]; ok {
		if _, ok := revisions.(map[string]interface{}); !ok {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.revisions",
				Message: "revisions must be a map",
			})
		}
	}

	// Validate groups_to_skip if specified
	if groups, ok := config["groups_to_skip"]; ok {
		if _, ok := groups.([]interface{}); !ok {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.groups_to_skip",
				Message: "groups_to_skip must be a list",
			})
		}
	}

	// Validate timeout format if specified
	if timeout, ok := config["timeout"].(string); ok {
		if _, err := time.ParseDuration(timeout); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.timeout",
				Message: fmt.Sprintf("invalid timeout format: %v", err),
			})
		}
	}

	return errs
}

// validateRollbackConfig validates rollback step configuration.
func (v *Validator) validateRollbackConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	// orchestration_id is required
	orchID, ok := config["orchestration_id"]
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.orchestration_id",
			Message: "orchestration_id is required for rollback step",
		})
	} else if _, ok := orchID.(string); !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.orchestration_id",
			Message: "orchestration_id must be a string",
		})
	}

	// Validate timeout format if specified
	if timeout, ok := config["timeout"].(string); ok {
		if _, err := time.ParseDuration(timeout); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.timeout",
				Message: fmt.Sprintf("invalid timeout format: %v", err),
			})
		}
	}

	return errs
}

// validateScriptConfig validates script step configuration.
func (v *Validator) validateScriptConfig(config map[string]interface{}, prefix string) ValidationErrors {
	var errs ValidationErrors

	// Script is required
	script, ok := config["script"]
	if !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.script",
			Message: "script is required for script step",
		})
	} else if _, ok := script.(string); !ok {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".config.script",
			Message: "script must be a string",
		})
	}

	// Validate language if specified
	if lang, ok := config["language"].(string); ok {
		validLanguages := map[string]bool{
			"bash": true, "python": true, "powershell": true, "shell": true,
		}
		if !validLanguages[lang] {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.language",
				Message: fmt.Sprintf("invalid script language %q (valid: bash, python, powershell, shell)", lang),
			})
		}
	}

	// Validate args if specified
	if args, ok := config["args"]; ok {
		switch args.(type) {
		case []interface{}, []string:
			// Valid
		default:
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.args",
				Message: "args must be a list of strings",
			})
		}
	}

	// Validate env if specified
	if env, ok := config["env"]; ok {
		if _, ok := env.(map[string]interface{}); !ok {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".config.env",
				Message: "env must be a map",
			})
		}
	}

	return errs
}

// validateStepDependencies validates that step dependencies form a valid DAG.
func (v *Validator) validateStepDependencies(steps []Step, stepNames map[string]bool, prefix string) ValidationErrors {
	var errs ValidationErrors

	// Check that all dependencies exist
	for i := range steps {
		step := &steps[i]
		for _, dep := range step.DependsOn {
			if !stepNames[dep] {
				errs = append(errs, &ValidationError{
					Field:   fmt.Sprintf("%s[%d].dependsOn", prefix, i),
					Message: fmt.Sprintf("dependency %q does not exist", dep),
				})
			}
			if dep == step.Name {
				errs = append(errs, &ValidationError{
					Field:   fmt.Sprintf("%s[%d].dependsOn", prefix, i),
					Message: "step cannot depend on itself",
				})
			}
		}
	}

	// Check for cycles using DFS
	if cycle := detectCycle(steps); cycle != nil {
		errs = append(errs, &ValidationError{
			Field:   prefix,
			Message: fmt.Sprintf("circular dependency detected: %s", strings.Join(cycle, " -> ")),
		})
	}

	return errs
}

// detectCycle uses DFS to detect cycles in step dependencies.
// Returns the cycle path if found, nil otherwise.
func detectCycle(steps []Step) []string {
	// Build adjacency list
	deps := make(map[string][]string)
	for i := range steps {
		deps[steps[i].Name] = steps[i].DependsOn
	}

	// Track visited and recursion stack
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	path := []string{}

	var dfs func(name string) []string
	dfs = func(name string) []string {
		visited[name] = true
		recStack[name] = true
		path = append(path, name)

		for _, dep := range deps[name] {
			if !visited[dep] {
				if cycle := dfs(dep); cycle != nil {
					return cycle
				}
			} else if recStack[dep] {
				// Found cycle, extract the cycle path
				cycleStart := -1
				for i, n := range path {
					if n == dep {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := append([]string{}, path[cycleStart:]...)
					cycle = append(cycle, dep)
					return cycle
				}
			}
		}

		path = path[:len(path)-1]
		recStack[name] = false
		return nil
	}

	for i := range steps {
		if !visited[steps[i].Name] {
			if cycle := dfs(steps[i].Name); cycle != nil {
				return cycle
			}
		}
	}

	return nil
}

// isValidName checks if a name is valid (lowercase alphanumeric with hyphens).
func isValidName(name string) bool {
	if name == "" {
		return false
	}
	if len(name) == 1 {
		return (name[0] >= 'a' && name[0] <= 'z') || (name[0] >= '0' && name[0] <= '9')
	}
	matched, _ := regexp.MatchString(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`, name)
	return matched
}

// isValidIdentifier checks if a name is a valid identifier (alphanumeric with underscores).
func isValidIdentifier(name string) bool {
	if name == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_]*$`, name)
	return matched
}

// Validate is a convenience function that validates a runbook.
func Validate(rb *Runbook) error {
	return NewValidator().Validate(rb)
}
