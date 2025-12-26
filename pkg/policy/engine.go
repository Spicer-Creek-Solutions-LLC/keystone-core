package policy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PolicyEngine coordinates policy evaluation across different evaluators
type PolicyEngine struct {
	registry     *Registry
	opaEvaluator *OPAEvaluator
	celEvaluator *CELEvaluator
	mu           sync.RWMutex
}

// NewPolicyEngine creates a new policy engine
func NewPolicyEngine(registry *Registry) *PolicyEngine {
	return &PolicyEngine{
		registry:     registry,
		opaEvaluator: NewOPAEvaluator(),
		celEvaluator: NewCELEvaluator(),
	}
}

// Evaluate evaluates a single policy against input
func (e *PolicyEngine) Evaluate(ctx context.Context, policyID string, input *EvaluationInput) (*EvaluationResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Get policy from registry
	policy, ok := e.registry.GetPolicy(policyID)
	if !ok {
		return nil, fmt.Errorf("policy not found: %s", policyID)
	}

	// Check if policy is enabled
	if !policy.Enabled {
		return &EvaluationResult{
			PolicyID:    policyID,
			PolicyName:  policy.Name,
			Allowed:     true, // Skip disabled policies
			Message:     "Policy is disabled",
			EvaluatedAt: time.Now(),
			Violations:  make([]Violation, 0),
			Warnings:    []string{"Policy is disabled"},
		}, nil
	}

	// Select evaluator based on policy type
	return e.evaluatePolicy(ctx, policy, input)
}

// EvaluatePolicySet evaluates all policies in a policy set
func (e *PolicyEngine) EvaluatePolicySet(ctx context.Context, setID string, input *EvaluationInput) (*PolicyResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Get policy set
	set, ok := e.registry.GetPolicySet(setID)
	if !ok {
		return nil, fmt.Errorf("policy set not found: %s", setID)
	}

	// Check if set is enabled
	if !set.Enabled {
		return &PolicyResult{
			Allowed:       true,
			Results:       make([]*EvaluationResult, 0),
			EvaluatedAt:   time.Now(),
			TotalDuration: 0,
			Summary: &PolicySummary{
				TotalPolicies:        0,
				AllowedPolicies:      0,
				DeniedPolicies:       0,
				TotalViolations:      0,
				ViolationsBySeverity: make(map[Severity]int),
			},
		}, nil
	}

	// Evaluate all policies in the set
	results := make([]*EvaluationResult, 0, len(set.Policies))
	for _, policyID := range set.Policies {
		result, err := e.Evaluate(ctx, policyID, input)
		if err != nil {
			// Continue evaluation even if one policy fails
			result = &EvaluationResult{
				PolicyID:    policyID,
				PolicyName:  policyID,
				Allowed:     false,
				Message:     fmt.Sprintf("Evaluation failed: %v", err),
				EvaluatedAt: time.Now(),
				Violations: []Violation{
					{
						Rule:     policyID,
						Message:  err.Error(),
						Severity: SeverityHigh,
					},
				},
				Warnings: make([]string, 0),
			}
		}
		results = append(results, result)
	}

	// Aggregate results
	return e.aggregateResults(setID, results, set.EnforcementMode), nil
}

// EvaluateForResource evaluates all policies bound to a resource type
func (e *PolicyEngine) EvaluateForResource(ctx context.Context, resourceType string, input *EvaluationInput) (*PolicyResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Get bindings for resource
	bindings := e.registry.ListBindingsForResource(resourceType)

	if len(bindings) == 0 {
		return &PolicyResult{
			Allowed:       true,
			Results:       make([]*EvaluationResult, 0),
			EvaluatedAt:   time.Now(),
			TotalDuration: 0,
			Summary: &PolicySummary{
				TotalPolicies:        0,
				AllowedPolicies:      0,
				DeniedPolicies:       0,
				TotalViolations:      0,
				ViolationsBySeverity: make(map[Severity]int),
			},
		}, nil
	}

	// Collect all policies and policy sets
	results := make([]*EvaluationResult, 0)
	enforcementMode := ModeAudit // Default to audit mode

	for _, binding := range bindings {
		// Check if action matches
		if len(binding.Actions) > 0 {
			actionMatches := false
			for _, action := range binding.Actions {
				if action == input.Action {
					actionMatches = true
					break
				}
			}
			if !actionMatches {
				continue
			}
		}

		// Evaluate policy or policy set
		if binding.PolicyID != "" {
			// Get policy to check enforcement mode
			policy, ok := e.registry.GetPolicy(binding.PolicyID)
			if ok && policy.EnforcementMode == ModeEnforce {
				enforcementMode = ModeEnforce
			}

			result, err := e.Evaluate(ctx, binding.PolicyID, input)
			if err != nil {
				result = &EvaluationResult{
					PolicyID:    binding.PolicyID,
					PolicyName:  binding.PolicyID,
					Allowed:     false,
					Message:     fmt.Sprintf("Evaluation failed: %v", err),
					EvaluatedAt: time.Now(),
					Violations: []Violation{
						{
							Rule:     binding.PolicyID,
							Message:  err.Error(),
							Severity: SeverityHigh,
						},
					},
					Warnings: make([]string, 0),
				}
			}
			results = append(results, result)
		} else if binding.PolicySetID != "" {
			// Get policy set to check enforcement mode
			policySet, ok := e.registry.GetPolicySet(binding.PolicySetID)
			if ok && policySet.EnforcementMode == ModeEnforce {
				enforcementMode = ModeEnforce
			}

			setResult, err := e.EvaluatePolicySet(ctx, binding.PolicySetID, input)
			if err != nil {
				continue
			}
			results = append(results, setResult.Results...)
		}
	}

	// Aggregate results
	return e.aggregateResults("", results, enforcementMode), nil
}

// EvaluateBatch evaluates multiple inputs against a policy
func (e *PolicyEngine) EvaluateBatch(ctx context.Context, policyID string, inputs []*EvaluationInput) ([]*EvaluationResult, error) {
	results := make([]*EvaluationResult, len(inputs))

	for i, input := range inputs {
		result, err := e.Evaluate(ctx, policyID, input)
		if err != nil {
			return nil, fmt.Errorf("batch evaluation failed at index %d: %w", i, err)
		}
		results[i] = result
	}

	return results, nil
}

// EvaluateBatchParallel evaluates multiple inputs in parallel
func (e *PolicyEngine) EvaluateBatchParallel(ctx context.Context, policyID string, inputs []*EvaluationInput) ([]*EvaluationResult, error) {
	results := make([]*EvaluationResult, len(inputs))
	errChan := make(chan error, len(inputs))
	var wg sync.WaitGroup

	for i, input := range inputs {
		wg.Add(1)
		go func(idx int, inp *EvaluationInput) {
			defer wg.Done()
			result, err := e.Evaluate(ctx, policyID, inp)
			if err != nil {
				errChan <- fmt.Errorf("evaluation failed at index %d: %w", idx, err)
				return
			}
			results[idx] = result
		}(i, input)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	if len(errChan) > 0 {
		return nil, <-errChan
	}

	return results, nil
}

// ValidatePolicy validates a policy without storing it
func (e *PolicyEngine) ValidatePolicy(ctx context.Context, policy *Policy) error {
	// Basic validation
	if policy.ID == "" {
		return fmt.Errorf("policy ID is required")
	}
	if policy.Name == "" {
		return fmt.Errorf("policy name is required")
	}
	if policy.Policy == "" {
		return fmt.Errorf("policy code is required")
	}

	// Validate based on type
	switch policy.Type {
	case PolicyTypeOPA:
		return e.opaEvaluator.ValidatePolicy(ctx, policy.Policy)
	case PolicyTypeCEL:
		return e.celEvaluator.ValidatePolicy(ctx, policy.Policy)
	case PolicyTypeBuiltin:
		return nil // Built-in policies are always valid
	default:
		return fmt.Errorf("unknown policy type: %s", policy.Type)
	}
}

// evaluatePolicy evaluates a policy using the appropriate evaluator
func (e *PolicyEngine) evaluatePolicy(ctx context.Context, policy *Policy, input *EvaluationInput) (*EvaluationResult, error) {
	switch policy.Type {
	case PolicyTypeOPA:
		return e.opaEvaluator.Evaluate(ctx, policy, input)
	case PolicyTypeCEL:
		return e.celEvaluator.Evaluate(ctx, policy, input)
	case PolicyTypeBuiltin:
		return e.evaluateBuiltinPolicy(ctx, policy, input)
	default:
		return nil, fmt.Errorf("unsupported policy type: %s", policy.Type)
	}
}

// evaluateBuiltinPolicy evaluates built-in policies
func (e *PolicyEngine) evaluateBuiltinPolicy(ctx context.Context, policy *Policy, input *EvaluationInput) (*EvaluationResult, error) {
	// Built-in policies would be implemented here
	// For now, return a placeholder
	return &EvaluationResult{
		PolicyID:    policy.ID,
		PolicyName:  policy.Name,
		Allowed:     true,
		Message:     "Built-in policy evaluation not yet implemented",
		EvaluatedAt: time.Now(),
		Violations:  make([]Violation, 0),
		Warnings:    []string{"Built-in policy type not yet implemented"},
	}, nil
}

// aggregateResults aggregates multiple evaluation results
func (e *PolicyEngine) aggregateResults(id string, results []*EvaluationResult, mode EnforcementMode) *PolicyResult {
	summary := &PolicySummary{
		TotalPolicies:        len(results),
		AllowedPolicies:      0,
		DeniedPolicies:       0,
		TotalViolations:      0,
		ViolationsBySeverity: make(map[Severity]int),
	}

	allowed := true
	totalDuration := time.Duration(0)

	for _, result := range results {
		totalDuration += result.Duration

		if result.Allowed {
			summary.AllowedPolicies++
		} else {
			summary.DeniedPolicies++
			if mode == ModeEnforce {
				allowed = false
			}
		}

		// Count violations by severity
		for _, violation := range result.Violations {
			summary.TotalViolations++
			summary.ViolationsBySeverity[violation.Severity]++
		}
	}

	policyResult := &PolicyResult{
		Allowed:       allowed,
		Results:       results,
		Summary:       summary,
		TotalDuration: totalDuration,
		EvaluatedAt:   time.Now(),
	}

	return policyResult
}
