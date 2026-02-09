package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
)

// CELEvaluator evaluates CEL (Common Expression Language) policies
type CELEvaluator struct {
	// Environment options
	envOptions []cel.EnvOption
}

// NewCELEvaluator creates a new CEL evaluator
func NewCELEvaluator(options ...cel.EnvOption) *CELEvaluator {
	return &CELEvaluator{
		envOptions: options,
	}
}

// Evaluate evaluates a CEL policy against input data
func (e *CELEvaluator) Evaluate(ctx context.Context, policy *Policy, input *EvaluationInput) (*EvaluationResult, error) {
	start := time.Now()

	result := &EvaluationResult{
		PolicyID:    policy.ID,
		PolicyName:  policy.Name,
		EvaluatedAt: start,
		Violations:  make([]Violation, 0),
		Warnings:    make([]string, 0),
	}

	// Create CEL environment with standard declarations
	// Create a new slice to avoid modifying the original envOptions
	envOptions := make([]cel.EnvOption, len(e.envOptions), len(e.envOptions)+1)
	copy(envOptions, e.envOptions)
	envOptions = append(envOptions,
		cel.Declarations( //nolint:staticcheck // SA1019: cel.Declarations is deprecated but requires CEL API migration to cel.Variable
			decls.NewVar("input", decls.NewMapType(decls.String, decls.Dyn)),
			decls.NewVar("resource", decls.Dyn),
			decls.NewVar("action", decls.String),
			decls.NewVar("user", decls.String),
			decls.NewVar("context", decls.NewMapType(decls.String, decls.Dyn)),
		),
	)

	env, err := cel.NewEnv(envOptions...)
	if err != nil {
		result.Allowed = false
		result.Message = fmt.Sprintf("Failed to create CEL environment: %v", err)
		result.Duration = time.Since(start)
		return result, fmt.Errorf("failed to create environment: %w", err)
	}

	// Parse the expression
	ast, issues := env.Compile(policy.Policy)
	if issues != nil && issues.Err() != nil {
		result.Allowed = false
		result.Message = fmt.Sprintf("Failed to compile policy: %v", issues.Err())
		result.Duration = time.Since(start)
		return result, fmt.Errorf("compilation failed: %w", issues.Err())
	}

	// Create program
	prg, err := env.Program(ast)
	if err != nil {
		result.Allowed = false
		result.Message = fmt.Sprintf("Failed to create program: %v", err)
		result.Duration = time.Since(start)
		return result, fmt.Errorf("program creation failed: %w", err)
	}

	// Prepare input variables
	vars := map[string]interface{}{
		"input":    input,
		"resource": input.Resource,
		"action":   input.Action,
		"user":     input.User,
		"context":  input.Context,
	}

	// Evaluate
	out, _, err := prg.Eval(vars)
	if err != nil {
		result.Allowed = false
		result.Message = fmt.Sprintf("Evaluation failed: %v", err)
		result.Duration = time.Since(start)
		return result, fmt.Errorf("evaluation failed: %w", err)
	}

	// Check result type and value
	if out.Type() == cel.BoolType {
		allowed, ok := out.Value().(bool)
		if !ok {
			result.Allowed = false
			result.Message = "Policy did not return a boolean value"
		} else {
			result.Allowed = allowed
			if allowed {
				result.Message = "Policy evaluation passed"
			} else {
				result.Message = "Policy evaluation failed"
				// Create a generic violation
				result.Violations = append(result.Violations, Violation{
					Rule:     policy.ID,
					Message:  "CEL expression evaluated to false",
					Severity: policy.Severity,
				})
			}
		}
	} else {
		result.Allowed = false
		result.Message = fmt.Sprintf("Policy returned non-boolean type: %v", out.Type())
	}

	result.Duration = time.Since(start)
	return result, nil
}

// EvaluateWithDetails evaluates a CEL policy and extracts detailed violations
func (e *CELEvaluator) EvaluateWithDetails(ctx context.Context, policy *Policy, input *EvaluationInput) (*EvaluationResult, error) {
	start := time.Now()

	result := &EvaluationResult{
		PolicyID:    policy.ID,
		PolicyName:  policy.Name,
		EvaluatedAt: start,
		Violations:  make([]Violation, 0),
		Warnings:    make([]string, 0),
	}

	// Create CEL environment with extended declarations
	// Create a new slice to avoid modifying the original envOptions
	envOptions := make([]cel.EnvOption, len(e.envOptions), len(e.envOptions)+1)
	copy(envOptions, e.envOptions)
	envOptions = append(envOptions,
		cel.Declarations( //nolint:staticcheck // SA1019: cel.Declarations is deprecated but requires CEL API migration to cel.Variable
			decls.NewVar("input", decls.NewMapType(decls.String, decls.Dyn)),
			decls.NewVar("resource", decls.Dyn),
			decls.NewVar("action", decls.String),
			decls.NewVar("user", decls.String),
			decls.NewVar("context", decls.NewMapType(decls.String, decls.Dyn)),
		),
	)

	env, err := cel.NewEnv(envOptions...)
	if err != nil {
		result.Allowed = false
		result.Message = fmt.Sprintf("Failed to create CEL environment: %v", err)
		result.Duration = time.Since(start)
		return result, fmt.Errorf("failed to create environment: %w", err)
	}

	// Parse the expression
	ast, issues := env.Compile(policy.Policy)
	if issues != nil && issues.Err() != nil {
		result.Allowed = false
		result.Message = fmt.Sprintf("Failed to compile policy: %v", issues.Err())
		result.Duration = time.Since(start)
		return result, fmt.Errorf("compilation failed: %w", issues.Err())
	}

	// Create program
	prg, err := env.Program(ast)
	if err != nil {
		result.Allowed = false
		result.Message = fmt.Sprintf("Failed to create program: %v", err)
		result.Duration = time.Since(start)
		return result, fmt.Errorf("program creation failed: %w", err)
	}

	// Prepare input variables
	vars := map[string]interface{}{
		"input":    input,
		"resource": input.Resource,
		"action":   input.Action,
		"user":     input.User,
		"context":  input.Context,
	}

	// Evaluate
	out, details, err := prg.Eval(vars)
	if err != nil {
		result.Allowed = false
		result.Message = fmt.Sprintf("Evaluation failed: %v", err)
		result.Duration = time.Since(start)
		return result, fmt.Errorf("evaluation failed: %w", err)
	}

	// Check result
	if out.Type() == cel.BoolType {
		allowed, ok := out.Value().(bool)
		if !ok {
			result.Allowed = false
			result.Message = "Policy did not return a boolean value"
		} else {
			result.Allowed = allowed
			if allowed {
				result.Message = "Policy evaluation passed"
			} else {
				result.Message = "Policy evaluation failed"
				// Try to extract violation details from evaluation state
				violations := e.extractViolationsFromDetails(details, policy)
				result.Violations = violations
			}
		}
	} else {
		result.Allowed = false
		result.Message = fmt.Sprintf("Policy returned non-boolean type: %v", out.Type())
	}

	result.Duration = time.Since(start)
	return result, nil
}

// extractViolationsFromDetails tries to extract violation information
func (e *CELEvaluator) extractViolationsFromDetails(details *cel.EvalDetails, policy *Policy) []Violation {
	violations := make([]Violation, 0)

	// If we have details, we could potentially extract more information
	// For now, create a basic violation
	if details != nil {
		violation := Violation{
			Rule:     policy.ID,
			Message:  "CEL expression evaluated to false",
			Severity: policy.Severity,
		}
		violations = append(violations, violation)
	}

	if len(violations) == 0 {
		violations = append(violations, Violation{
			Rule:     policy.ID,
			Message:  "Policy evaluation denied the operation",
			Severity: policy.Severity,
		})
	}

	return violations
}

// ValidatePolicy validates CEL policy syntax
func (e *CELEvaluator) ValidatePolicy(ctx context.Context, policyCode string) error {
	// Create a new slice to avoid modifying the original envOptions
	envOptions := make([]cel.EnvOption, len(e.envOptions), len(e.envOptions)+1)
	copy(envOptions, e.envOptions)
	envOptions = append(envOptions,
		cel.Declarations( //nolint:staticcheck // SA1019: cel.Declarations is deprecated but requires CEL API migration to cel.Variable
			decls.NewVar("input", decls.NewMapType(decls.String, decls.Dyn)),
			decls.NewVar("resource", decls.Dyn),
			decls.NewVar("action", decls.String),
			decls.NewVar("user", decls.String),
			decls.NewVar("context", decls.NewMapType(decls.String, decls.Dyn)),
		),
	)

	env, err := cel.NewEnv(envOptions...)
	if err != nil {
		return fmt.Errorf("failed to create environment: %w", err)
	}

	_, issues := env.Compile(policyCode)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("policy validation failed: %w", issues.Err())
	}

	return nil
}
