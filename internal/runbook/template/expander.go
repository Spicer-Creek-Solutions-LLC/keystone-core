package template

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// Expander expands template references in runbooks.
type Expander struct {
	registry *Registry
}

// NewExpander creates a new template expander.
func NewExpander(registry *Registry) *Expander {
	return &Expander{registry: registry}
}

// Expand expands all template references in a runbook.
// Returns a new runbook with templates replaced by their steps.
func (e *Expander) Expand(rb *runbook.Runbook) (*runbook.Runbook, error) {
	// Deep copy the runbook
	expanded := &runbook.Runbook{
		APIVersion: rb.APIVersion,
		Kind:       rb.Kind,
		Metadata:   rb.Metadata,
		Spec: runbook.Spec{
			Description: rb.Spec.Description,
			Inputs:      rb.Spec.Inputs,
			Timeout:     rb.Spec.Timeout,
			MaxRetries:  rb.Spec.MaxRetries,
			OnSuccess:   rb.Spec.OnSuccess,
			OnFailure:   rb.Spec.OnFailure,
		},
	}

	// Expand steps
	expandedSteps, err := e.expandSteps(rb.Spec.Steps, "")
	if err != nil {
		return nil, fmt.Errorf("expand steps: %w", err)
	}
	expanded.Spec.Steps = expandedSteps

	return expanded, nil
}

// expandSteps expands template references in a list of steps.
func (e *Expander) expandSteps(steps []runbook.Step, prefix string) ([]runbook.Step, error) {
	var expanded []runbook.Step

	for i := range steps {
		step := &steps[i]
		// Check if this step uses a template
		tmplName, hasTemplate := step.Config["template"].(string)
		if hasTemplate {
			// Get template
			tmplVersion := ""
			if v, ok := step.Config["version"].(string); ok {
				tmplVersion = v
			}

			tmpl, ok := e.registry.Get(tmplName, tmplVersion)
			if !ok {
				return nil, fmt.Errorf("step %d: template %s not found", i, tmplName)
			}

			// Get parameters
			params, _ := step.Config["parameters"].(map[string]interface{})
			if params == nil {
				params = make(map[string]interface{})
			}

			// Apply defaults
			for _, param := range tmpl.Spec.Parameters {
				if _, ok := params[param.Name]; !ok && param.Default != nil {
					params[param.Name] = param.Default
				}
			}

			// Validate required parameters
			for _, param := range tmpl.Spec.Parameters {
				if param.Required {
					if _, ok := params[param.Name]; !ok {
						return nil, fmt.Errorf("step %d: required parameter %q not provided for template %s",
							i, param.Name, tmplName)
					}
				}
			}

			// Expand template steps with parameter substitution
			stepPrefix := fmt.Sprintf("%s%s_", prefix, step.Name)
			for j := range tmpl.Spec.Steps {
				tmplStep := &tmpl.Spec.Steps[j]
				expandedStep := e.substituteParameters(*tmplStep, params, stepPrefix)

				// Prefix step name to avoid collisions
				expandedStep.Name = fmt.Sprintf("%s%s", stepPrefix, tmplStep.Name)

				// Update dependencies to use prefixed names
				for k, dep := range expandedStep.DependsOn {
					expandedStep.DependsOn[k] = fmt.Sprintf("%s%s", stepPrefix, dep)
				}

				// Handle nested templates recursively
				if _, hasNested := expandedStep.Config["template"].(string); hasNested {
					nestedExpanded, err := e.expandSteps([]runbook.Step{expandedStep}, stepPrefix)
					if err != nil {
						return nil, fmt.Errorf("expand nested template at step %d.%d: %w", i, j, err)
					}
					expanded = append(expanded, nestedExpanded...)
				} else {
					expanded = append(expanded, expandedStep)
				}
			}
		} else {
			// Regular step - check for nested step lists (parallel, loop, etc.)
			expandedStep := *step

			// Expand nested steps in parallel/loop configurations
			if nestedSteps, ok := step.Config["steps"]; ok {
				if stepList, ok := nestedSteps.([]interface{}); ok {
					var parsedSteps []runbook.Step
					for _, s := range stepList {
						if stepMap, ok := s.(map[string]interface{}); ok {
							parsed, err := mapToStep(stepMap)
							if err != nil {
								return nil, fmt.Errorf("parse nested step: %w", err)
							}
							parsedSteps = append(parsedSteps, *parsed)
						}
					}

					expandedNested, err := e.expandSteps(parsedSteps, prefix)
					if err != nil {
						return nil, err
					}

					// Convert back to interface slice for config
					expandedStep.Config = copyConfig(step.Config)
					expandedStep.Config["steps"] = stepsToInterface(expandedNested)
				}
			}

			expanded = append(expanded, expandedStep)
		}
	}

	return expanded, nil
}

// substituteParameters replaces parameter placeholders in a step.
func (e *Expander) substituteParameters(step runbook.Step, params map[string]interface{}, prefix string) runbook.Step {
	// Deep copy the step
	result := runbook.Step{
		Name:      step.Name,
		Type:      step.Type,
		DependsOn: append([]string{}, step.DependsOn...),
		Condition: substituteString(step.Condition, params),
		Timeout:   step.Timeout,
		Retries:   step.Retries,
		Outputs:   step.Outputs,
		Config:    substituteConfig(step.Config, params),
	}

	return result
}

// substituteString replaces parameter placeholders in a string.
func substituteString(s string, params map[string]interface{}) string {
	result := s
	for name, value := range params {
		placeholder := fmt.Sprintf("{{ .params.%s }}", name)
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))

		// Also support alternate syntax
		placeholder2 := fmt.Sprintf("{{.params.%s}}", name)
		result = strings.ReplaceAll(result, placeholder2, fmt.Sprintf("%v", value))
	}
	return result
}

// substituteConfig replaces parameter placeholders in config.
func substituteConfig(config, params map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range config {
		result[k] = substituteValue(v, params)
	}
	return result
}

// substituteValue replaces parameter placeholders in a value.
func substituteValue(value interface{}, params map[string]interface{}) interface{} {
	switch v := value.(type) {
	case string:
		return substituteString(v, params)
	case map[string]interface{}:
		return substituteConfig(v, params)
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = substituteValue(item, params)
		}
		return result
	default:
		return value
	}
}

// copyConfig deep copies a config map.
func copyConfig(config map[string]interface{}) map[string]interface{} {
	// Use JSON marshaling for deep copy
	data, _ := json.Marshal(config)
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result
}

// mapToStep converts a map to a Step.
func mapToStep(m map[string]interface{}) (*runbook.Step, error) {
	step := &runbook.Step{}

	if name, ok := m["name"].(string); ok {
		step.Name = name
	}
	if stepType, ok := m["type"].(string); ok {
		step.Type = runbook.StepType(stepType)
	}
	if config, ok := m["config"].(map[string]interface{}); ok {
		step.Config = config
	}
	if dependsOn, ok := m["dependsOn"].([]interface{}); ok {
		for _, d := range dependsOn {
			if ds, ok := d.(string); ok {
				step.DependsOn = append(step.DependsOn, ds)
			}
		}
	}
	if condition, ok := m["condition"].(string); ok {
		step.Condition = condition
	}
	if timeout, ok := m["timeout"].(string); ok {
		step.Timeout = timeout
	}

	return step, nil
}

// stepsToInterface converts steps to interface slice for config.
func stepsToInterface(steps []runbook.Step) []interface{} {
	result := make([]interface{}, len(steps))
	for i := range steps {
		step := &steps[i]
		result[i] = map[string]interface{}{
			"name":      step.Name,
			"type":      string(step.Type),
			"config":    step.Config,
			"dependsOn": step.DependsOn,
			"condition": step.Condition,
			"timeout":   step.Timeout,
		}
	}
	return result
}
