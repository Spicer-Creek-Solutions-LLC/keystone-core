package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/open-policy-agent/opa/rego"
)

// OPAEvaluator evaluates OPA/Rego policies
type OPAEvaluator struct {
	// Options for OPA evaluation
	options []func(*rego.Rego)
}

// NewOPAEvaluator creates a new OPA evaluator
func NewOPAEvaluator(options ...func(*rego.Rego)) *OPAEvaluator {
	return &OPAEvaluator{
		options: options,
	}
}

// Evaluate evaluates a policy against input data
func (e *OPAEvaluator) Evaluate(ctx context.Context, policy *Policy, input *EvaluationInput) (*EvaluationResult, error) {
	start := time.Now()

	result := &EvaluationResult{
		PolicyID:    policy.ID,
		PolicyName:  policy.Name,
		EvaluatedAt: start,
		Violations:  make([]Violation, 0),
		Warnings:    make([]string, 0),
	}

	// Create Rego query
	pkgName := getPackageName(policy)
	options := append(e.options,
		rego.Query("data."+pkgName+".allow"),
		rego.Module(policy.ID+".rego", policy.Policy),
		rego.Input(input),
	)

	r := rego.New(options...)

	// Prepare query
	query, err := r.PrepareForEval(ctx)
	if err != nil {
		result.Allowed = false
		result.Message = fmt.Sprintf("Failed to prepare policy: %v", err)
		result.Duration = time.Since(start)
		return result, fmt.Errorf("failed to prepare policy: %w", err)
	}

	// Evaluate
	rs, err := query.Eval(ctx)
	if err != nil {
		result.Allowed = false
		result.Message = fmt.Sprintf("Policy evaluation failed: %v", err)
		result.Duration = time.Since(start)
		return result, fmt.Errorf("evaluation failed: %w", err)
	}

	// Check results
	if len(rs) == 0 {
		result.Allowed = false
		result.Message = "Policy returned no results"
	} else if len(rs[0].Expressions) == 0 {
		result.Allowed = false
		result.Message = "Policy returned no expressions"
	} else {
		// Get allow decision
		allowed, ok := rs[0].Expressions[0].Value.(bool)
		if !ok {
			result.Allowed = false
			result.Message = "Policy did not return a boolean value"
		} else {
			result.Allowed = allowed
			if allowed {
				result.Message = "Policy evaluation passed"
			} else {
				result.Message = "Policy evaluation failed"
				// Try to get violations
				violations := e.extractViolations(rs, policy)
				result.Violations = violations
			}
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// extractViolations tries to extract violation details from OPA results
func (e *OPAEvaluator) extractViolations(rs rego.ResultSet, policy *Policy) []Violation {
	violations := make([]Violation, 0)

	// Try to find violations in the result
	// This is a simplified version - real implementation would query for violations specifically
	if len(rs) > 0 && len(rs[0].Bindings) > 0 {
		if violationsData, ok := rs[0].Bindings["violations"]; ok {
			if violationsList, ok := violationsData.([]interface{}); ok {
				for _, v := range violationsList {
					if violationMap, ok := v.(map[string]interface{}); ok {
						violation := Violation{
							Rule:     policy.ID,
							Severity: policy.Severity,
						}
						if msg, ok := violationMap["message"].(string); ok {
							violation.Message = msg
						}
						if path, ok := violationMap["path"].(string); ok {
							violation.Path = path
						}
						violations = append(violations, violation)
					}
				}
			}
		}
	}

	// If no specific violations found, create a generic one
	if len(violations) == 0 {
		violations = append(violations, Violation{
			Rule:     policy.ID,
			Message:  "Policy evaluation denied the operation",
			Severity: policy.Severity,
		})
	}

	return violations
}

// EvaluateWithDeny evaluates a policy with an explicit deny check
func (e *OPAEvaluator) EvaluateWithDeny(ctx context.Context, policy *Policy, input *EvaluationInput) (*EvaluationResult, error) {
	start := time.Now()

	result := &EvaluationResult{
		PolicyID:    policy.ID,
		PolicyName:  policy.Name,
		EvaluatedAt: start,
		Violations:  make([]Violation, 0),
		Warnings:    make([]string, 0),
	}

	// Create Rego query for both allow and deny
	pkgName := getPackageName(policy)
	options := append(e.options,
		rego.Query(fmt.Sprintf("allow = data.%s.allow; deny = data.%s.deny; violations = data.%s.violations", pkgName, pkgName, pkgName)),
		rego.Module(policy.ID+".rego", policy.Policy),
		rego.Input(input),
	)

	r := rego.New(options...)

	// Prepare query
	query, err := r.PrepareForEval(ctx)
	if err != nil {
		result.Allowed = false
		result.Message = fmt.Sprintf("Failed to prepare policy: %v", err)
		result.Duration = time.Since(start)
		return result, fmt.Errorf("failed to prepare policy: %w", err)
	}

	// Evaluate
	rs, err := query.Eval(ctx)
	if err != nil {
		result.Allowed = false
		result.Message = fmt.Sprintf("Policy evaluation failed: %v", err)
		result.Duration = time.Since(start)
		return result, fmt.Errorf("evaluation failed: %w", err)
	}

	// Parse results
	if len(rs) > 0 && len(rs[0].Bindings) > 0 {
		bindings := rs[0].Bindings

		// First check allow value
		if allowVal, ok := bindings["allow"]; ok {
			if allow, ok := allowVal.(bool); ok {
				result.Allowed = allow
				if !allow {
					result.Message = "Policy denied the operation"
				} else {
					result.Message = "Policy allowed the operation"
				}
			}
		}

		// Check deny (explicit denial overrides allow)
		if denyVal, ok := bindings["deny"]; ok {
			if deny, ok := denyVal.(bool); ok && deny {
				result.Allowed = false
				result.Message = "Policy explicitly denied the operation"
			}
		}

		// Extract violations
		if violationsVal, ok := bindings["violations"]; ok {
			violations := e.parseViolationsFromData(violationsVal, policy)
			result.Violations = violations
		}
	} else {
		result.Allowed = false
		result.Message = "Policy returned no results"
	}

	result.Duration = time.Since(start)
	return result, nil
}

// parseViolationsFromData parses violations from OPA result data
func (e *OPAEvaluator) parseViolationsFromData(data interface{}, policy *Policy) []Violation {
	violations := make([]Violation, 0)

	switch v := data.(type) {
	case []interface{}:
		for _, item := range v {
			if violationMap, ok := item.(map[string]interface{}); ok {
				violation := Violation{
					Rule:     policy.ID,
					Severity: policy.Severity,
				}
				if msg, ok := violationMap["message"].(string); ok {
					violation.Message = msg
				}
				if path, ok := violationMap["path"].(string); ok {
					violation.Path = path
				}
				if remediation, ok := violationMap["remediation"].(string); ok {
					violation.Remediation = remediation
				}
				violations = append(violations, violation)
			}
		}
	case map[string]interface{}:
		// Single violation as map
		violation := Violation{
			Rule:     policy.ID,
			Severity: policy.Severity,
		}
		if msg, ok := v["message"].(string); ok {
			violation.Message = msg
		}
		violations = append(violations, violation)
	}

	return violations
}

// getPackageName extracts package name from policy code
func getPackageName(policy *Policy) string {
	// Parse package name from policy code
	// Look for "package <name>" declaration
	packagePrefix := "package "

	startIdx := 0
	for i := range policy.Policy {
		if i+len(packagePrefix) <= len(policy.Policy) &&
		   policy.Policy[i:i+len(packagePrefix)] == packagePrefix {
			startIdx = i + len(packagePrefix)
			break
		}
	}

	if startIdx == 0 {
		// No package declaration found, use policy ID with sanitization
		return sanitizePackageName(policy.ID)
	}

	// Find end of package name (whitespace or newline)
	endIdx := startIdx
	for endIdx < len(policy.Policy) {
		c := policy.Policy[endIdx]
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			break
		}
		endIdx++
	}

	return policy.Policy[startIdx:endIdx]
}

// sanitizePackageName converts a string to a valid Rego package name
func sanitizePackageName(name string) string {
	// Replace hyphens with underscores
	result := ""
	for _, c := range name {
		if c == '-' {
			result += "_"
		} else {
			result += string(c)
		}
	}
	return result
}

// ValidatePolicy validates OPA policy syntax
func (e *OPAEvaluator) ValidatePolicy(ctx context.Context, policyCode string) error {
	_, err := rego.New(
		rego.Query("data"),
		rego.Module("validation.rego", policyCode),
	).PrepareForEval(ctx)

	if err != nil {
		return fmt.Errorf("policy validation failed: %w", err)
	}

	return nil
}
