// Package variables provides variable management for runbook execution.
package variables

import (
	"fmt"
	"sync"
	"time"
)

// Context provides a hierarchical variable context for runbook execution.
// Variables are resolved in the following order:
// 1. Built-in variables (execution.id, runbook.name, now, etc.)
// 2. Step outputs (steps.<stepName>.<outputName>)
// 3. Global inputs (inputs.<inputName>)
type Context struct {
	mu sync.RWMutex

	// Execution metadata
	executionID    string
	runbookName    string
	runbookVersion string
	startTime      time.Time

	// Input values provided at execution time
	inputs map[string]interface{}

	// Step outputs collected during execution
	stepOutputs map[string]map[string]interface{}

	// Template engine for resolving expressions
	template *TemplateEngine
}

// NewContext creates a new variable context.
func NewContext(executionID, runbookName, runbookVersion string, inputs map[string]interface{}) *Context {
	if inputs == nil {
		inputs = make(map[string]interface{})
	}

	ctx := &Context{
		executionID:    executionID,
		runbookName:    runbookName,
		runbookVersion: runbookVersion,
		startTime:      time.Now(),
		inputs:         inputs,
		stepOutputs:    make(map[string]map[string]interface{}),
	}

	ctx.template = NewTemplateEngine(ctx)
	return ctx
}

// ExecutionID returns the execution ID.
func (c *Context) ExecutionID() string {
	return c.executionID
}

// RunbookName returns the runbook name.
func (c *Context) RunbookName() string {
	return c.runbookName
}

// RunbookVersion returns the runbook version.
func (c *Context) RunbookVersion() string {
	return c.runbookVersion
}

// StartTime returns the execution start time.
func (c *Context) StartTime() time.Time {
	return c.startTime
}

// GetInput returns an input value by name.
func (c *Context) GetInput(name string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	v, ok := c.inputs[name]
	return v, ok
}

// SetInput sets an input value.
func (c *Context) SetInput(name string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.inputs[name] = value
}

// GetStepOutput returns an output value from a completed step.
func (c *Context) GetStepOutput(stepName, outputName string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	outputs, ok := c.stepOutputs[stepName]
	if !ok {
		return nil, false
	}

	v, ok := outputs[outputName]
	return v, ok
}

// SetStepOutput sets an output value for a step.
func (c *Context) SetStepOutput(stepName, outputName string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stepOutputs[stepName] == nil {
		c.stepOutputs[stepName] = make(map[string]interface{})
	}

	c.stepOutputs[stepName][outputName] = value
}

// SetStepOutputs sets all outputs for a step.
func (c *Context) SetStepOutputs(stepName string, outputs map[string]interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.stepOutputs[stepName] = outputs
}

// GetStepOutputs returns all outputs for a step.
func (c *Context) GetStepOutputs(stepName string) (map[string]interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	outputs, ok := c.stepOutputs[stepName]
	if !ok {
		return nil, false
	}

	// Return a copy to prevent modification
	result := make(map[string]interface{}, len(outputs))
	for k, v := range outputs {
		result[k] = v
	}
	return result, true
}

// Resolve resolves a template string against the current variable context.
func (c *Context) Resolve(template string) (string, error) {
	return c.template.Execute(template)
}

// ResolveValue resolves a template and returns the typed value.
// This is useful when the template resolves to a non-string value.
func (c *Context) ResolveValue(template string) (interface{}, error) {
	return c.template.ExecuteValue(template)
}

// EvaluateCondition evaluates a condition expression and returns a boolean.
// This is used for step conditions and other boolean expressions.
func (c *Context) EvaluateCondition(expr string) (bool, error) {
	evaluator := NewExpressionEvaluator(c)
	return evaluator.Evaluate(expr)
}

// EvaluateConditionResult evaluates a condition and returns a detailed result.
func (c *Context) EvaluateConditionResult(expr string) *ConditionResult {
	evaluator := NewExpressionEvaluator(c)
	return evaluator.EvaluateCondition(expr)
}

// ResolveMap resolves all string values in a map recursively.
func (c *Context) ResolveMap(m map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(m))

	for k, v := range m {
		resolved, err := c.resolveValue(v)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve %q: %w", k, err)
		}
		result[k] = resolved
	}

	return result, nil
}

// resolveValue recursively resolves template values.
func (c *Context) resolveValue(v interface{}) (interface{}, error) {
	switch val := v.(type) {
	case string:
		// Check if string contains template syntax
		if hasTemplateSyntax(val) {
			return c.template.ExecuteValue(val)
		}
		return val, nil

	case map[string]interface{}:
		return c.ResolveMap(val)

	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			resolved, err := c.resolveValue(item)
			if err != nil {
				return nil, err
			}
			result[i] = resolved
		}
		return result, nil

	default:
		return v, nil
	}
}

// ToData returns the context data for template rendering.
func (c *Context) ToData() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Build inputs copy
	inputsCopy := make(map[string]interface{}, len(c.inputs))
	for k, v := range c.inputs {
		inputsCopy[k] = v
	}

	// Build steps copy
	stepsCopy := make(map[string]interface{}, len(c.stepOutputs))
	for stepName, outputs := range c.stepOutputs {
		outputsCopy := make(map[string]interface{}, len(outputs))
		for k, v := range outputs {
			outputsCopy[k] = v
		}
		stepsCopy[stepName] = outputsCopy
	}

	return map[string]interface{}{
		"inputs": inputsCopy,
		"steps":  stepsCopy,
		"runbook": map[string]interface{}{
			"name":    c.runbookName,
			"version": c.runbookVersion,
		},
		"execution": map[string]interface{}{
			"id":         c.executionID,
			"start_time": c.startTime,
		},
		"now": time.Now(),
	}
}

// hasTemplateSyntax checks if a string contains Go template syntax.
func hasTemplateSyntax(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '{' && s[i+1] == '{' {
			return true
		}
	}
	return false
}
