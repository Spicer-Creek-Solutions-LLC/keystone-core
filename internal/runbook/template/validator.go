package template

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// ValidationError represents a template validation error.
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

// Validate validates a template.
func Validate(tmpl *Template) error {
	var errs ValidationErrors

	// Validate API version
	if tmpl.APIVersion == "" {
		errs = append(errs, &ValidationError{
			Field:   "apiVersion",
			Message: "apiVersion is required",
		})
	}

	// Validate kind
	if tmpl.Kind != "StepTemplate" {
		errs = append(errs, &ValidationError{
			Field:   "kind",
			Message: "kind must be 'StepTemplate'",
		})
	}

	// Validate metadata
	errs = append(errs, validateMetadata(&tmpl.Metadata)...)

	// Validate spec
	errs = append(errs, validateSpec(&tmpl.Spec)...)

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// validateMetadata validates template metadata.
func validateMetadata(m *Metadata) ValidationErrors {
	var errs ValidationErrors

	if m.Name == "" {
		errs = append(errs, &ValidationError{
			Field:   "metadata.name",
			Message: "name is required",
		})
	} else if !isValidTemplateName(m.Name) {
		errs = append(errs, &ValidationError{
			Field:   "metadata.name",
			Message: "name must match pattern ^[a-z0-9][a-z0-9-]*[a-z0-9]$ or be a single character [a-z0-9]",
		})
	}

	if m.Version == "" {
		errs = append(errs, &ValidationError{
			Field:   "metadata.version",
			Message: "version is required",
		})
	} else if !isValidVersion(m.Version) {
		errs = append(errs, &ValidationError{
			Field:   "metadata.version",
			Message: "version must be a valid semantic version (e.g., 1.0.0)",
		})
	}

	return errs
}

// validateSpec validates template spec.
func validateSpec(s *Spec) ValidationErrors {
	var errs ValidationErrors

	// Validate parameters
	paramNames := make(map[string]bool)
	for i, param := range s.Parameters {
		prefix := fmt.Sprintf("spec.parameters[%d]", i)

		switch {
		case param.Name == "":
			errs = append(errs, &ValidationError{
				Field:   prefix + ".name",
				Message: "name is required",
			})
		case paramNames[param.Name]:
			errs = append(errs, &ValidationError{
				Field:   prefix + ".name",
				Message: fmt.Sprintf("duplicate parameter name %q", param.Name),
			})
		default:
			paramNames[param.Name] = true
		}

		if param.Type == "" {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".type",
				Message: "type is required",
			})
		} else if !isValidParameterType(param.Type) {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".type",
				Message: fmt.Sprintf("invalid parameter type %q (valid: string, int, bool, float, list, map)", param.Type),
			})
		}

		// Validate validation rules
		if param.Validation != nil {
			errs = append(errs, validateParameterValidation(param.Validation, param.Type, prefix)...)
		}
	}

	// Validate steps
	if len(s.Steps) == 0 {
		errs = append(errs, &ValidationError{
			Field:   "spec.steps",
			Message: "at least one step is required",
		})
	}

	stepNames := make(map[string]bool)
	for i := range s.Steps {
		step := &s.Steps[i]
		prefix := fmt.Sprintf("spec.steps[%d]", i)

		switch {
		case step.Name == "":
			errs = append(errs, &ValidationError{
				Field:   prefix + ".name",
				Message: "name is required",
			})
		case stepNames[step.Name]:
			errs = append(errs, &ValidationError{
				Field:   prefix + ".name",
				Message: fmt.Sprintf("duplicate step name %q", step.Name),
			})
		default:
			stepNames[step.Name] = true
		}

		if !step.Type.IsValidExtended() {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".type",
				Message: fmt.Sprintf("invalid step type %q", step.Type),
			})
		}
	}

	// Validate outputs
	outputNames := make(map[string]bool)
	for i, output := range s.Outputs {
		prefix := fmt.Sprintf("spec.outputs[%d]", i)

		switch {
		case output.Name == "":
			errs = append(errs, &ValidationError{
				Field:   prefix + ".name",
				Message: "name is required",
			})
		case outputNames[output.Name]:
			errs = append(errs, &ValidationError{
				Field:   prefix + ".name",
				Message: fmt.Sprintf("duplicate output name %q", output.Name),
			})
		default:
			outputNames[output.Name] = true
		}

		if output.Source == "" {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".source",
				Message: "source is required",
			})
		}
	}

	return errs
}

// validateParameterValidation validates parameter validation rules.
func validateParameterValidation(v *ParameterValidation, paramType, prefix string) ValidationErrors {
	var errs ValidationErrors

	// Pattern only applies to strings
	if v.Pattern != "" {
		if paramType != "string" {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".validation.pattern",
				Message: "pattern validation only applies to string parameters",
			})
		} else if _, err := regexp.Compile(v.Pattern); err != nil {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".validation.pattern",
				Message: fmt.Sprintf("invalid regex pattern: %v", err),
			})
		}
	}

	// Min/max apply to numeric types and string/list lengths
	if v.Min != nil || v.Max != nil {
		validTypes := map[string]bool{"int": true, "float": true, "string": true, "list": true}
		if !validTypes[paramType] {
			errs = append(errs, &ValidationError{
				Field:   prefix + ".validation",
				Message: "min/max validation only applies to numeric, string, or list parameters",
			})
		}
	}

	if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
		errs = append(errs, &ValidationError{
			Field:   prefix + ".validation",
			Message: "min cannot be greater than max",
		})
	}

	return errs
}

// isValidTemplateName checks if a name is valid.
func isValidTemplateName(name string) bool {
	if name == "" {
		return false
	}
	if len(name) == 1 {
		return (name[0] >= 'a' && name[0] <= 'z') || (name[0] >= '0' && name[0] <= '9')
	}
	matched, _ := regexp.MatchString(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`, name)
	return matched
}

// isValidVersion checks if a version string is valid semver.
func isValidVersion(version string) bool {
	// Basic semver pattern: major.minor.patch with optional pre-release
	matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$`, version)
	return matched
}

// isValidParameterType checks if a parameter type is valid.
func isValidParameterType(t string) bool {
	validTypes := map[string]bool{
		"string": true,
		"int":    true,
		"bool":   true,
		"float":  true,
		"list":   true,
		"map":    true,
	}
	return validTypes[t]
}

// ValidateRef validates a template reference against a registry.
func ValidateRef(ref *Ref, registry *Registry) error {
	if ref.Name == "" {
		return &ValidationError{
			Field:   "name",
			Message: "template name is required",
		}
	}

	// Get template from registry
	tmpl, ok := registry.Get(ref.Name, ref.Version)
	if !ok {
		if ref.Version != "" {
			return &ValidationError{
				Field:   "name",
				Message: fmt.Sprintf("template %s version %s not found", ref.Name, ref.Version),
			}
		}
		return &ValidationError{
			Field:   "name",
			Message: fmt.Sprintf("template %s not found", ref.Name),
		}
	}

	// Validate required parameters are provided
	for _, param := range tmpl.Spec.Parameters {
		if param.Required {
			if _, ok := ref.Parameters[param.Name]; !ok && param.Default == nil {
				return &ValidationError{
					Field:   "parameters." + param.Name,
					Message: fmt.Sprintf("required parameter %q not provided", param.Name),
				}
			}
		}
	}

	return nil
}

// Validator provides template validation.
type Validator struct {
	registry *Registry
}

// NewValidator creates a new template validator.
func NewValidator(registry *Registry) *Validator {
	return &Validator{registry: registry}
}

// ValidateRunbookWithTemplates validates a runbook, resolving any template references.
func (v *Validator) ValidateRunbookWithTemplates(rb *runbook.Runbook) error {
	rbValidator := runbook.NewValidator()
	rbValidator.AllowFutureStepTypes = true

	// Basic validation first
	if err := rbValidator.Validate(rb); err != nil {
		return err
	}

	// Validate template references in steps
	// (Templates are referenced via "template" step type or config)
	for i := range rb.Spec.Steps {
		step := &rb.Spec.Steps[i]
		if tmplName, ok := step.Config["template"].(string); ok {
			tmplVersion := ""
			if v, ok := step.Config["version"].(string); ok {
				tmplVersion = v
			}

			if _, ok := v.registry.Get(tmplName, tmplVersion); !ok {
				return &ValidationError{
					Field:   fmt.Sprintf("spec.steps[%d].config.template", i),
					Message: fmt.Sprintf("template %s not found", tmplName),
				}
			}
		}
	}

	return nil
}
