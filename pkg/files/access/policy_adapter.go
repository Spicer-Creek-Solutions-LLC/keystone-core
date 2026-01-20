// Package access provides access control for the file distribution system.
package access

import (
	"context"
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/policy"
)

// PolicyEngineAdapter adapts the policy engine to the file access PolicyEvaluator interface.
// This provides integration between the file distribution system and the policy enforcement
// engine, allowing file access to be controlled by OPA/CEL/builtin policies.
type PolicyEngineAdapter struct {
	engine *policy.PolicyEngine

	// resourceType is the resource type used for policy bindings (e.g., "file")
	resourceType string

	// policySetID is an optional policy set to evaluate
	policySetID string

	// policyID is an optional single policy to evaluate
	policyID string
}

// PolicyEngineAdapterConfig configures the policy engine adapter.
type PolicyEngineAdapterConfig struct {
	// Engine is the policy engine to use for evaluation
	Engine *policy.PolicyEngine

	// ResourceType is the resource type for policy bindings (default: "file")
	ResourceType string

	// PolicySetID is an optional policy set to evaluate instead of resource bindings
	PolicySetID string

	// PolicyID is an optional single policy to evaluate
	PolicyID string
}

// NewPolicyEngineAdapter creates a new policy engine adapter.
func NewPolicyEngineAdapter(config *PolicyEngineAdapterConfig) (*PolicyEngineAdapter, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.Engine == nil {
		return nil, fmt.Errorf("policy engine is required")
	}

	resourceType := config.ResourceType
	if resourceType == "" {
		resourceType = "file"
	}

	return &PolicyEngineAdapter{
		engine:       config.Engine,
		resourceType: resourceType,
		policySetID:  config.PolicySetID,
		policyID:     config.PolicyID,
	}, nil
}

// Evaluate evaluates the policy engine for a file access request.
// This implements the PolicyEvaluator interface for file access control.
func (a *PolicyEngineAdapter) Evaluate(ctx context.Context, req *AccessRequest) (*AccessResult, error) {
	start := time.Now()

	// Build evaluation input from access request
	input := a.buildEvaluationInput(req)

	// Evaluate based on configuration
	var policyResult *policy.PolicyResult
	var singleResult *policy.EvaluationResult
	var err error

	if a.policyID != "" {
		// Evaluate a single specific policy
		singleResult, err = a.engine.Evaluate(ctx, a.policyID, input)
		if err != nil {
			return nil, fmt.Errorf("policy evaluation failed: %w", err)
		}
		policyResult = a.singleResultToPolicyResult(singleResult)
	} else if a.policySetID != "" {
		// Evaluate a policy set
		policyResult, err = a.engine.EvaluatePolicySet(ctx, a.policySetID, input)
		if err != nil {
			return nil, fmt.Errorf("policy set evaluation failed: %w", err)
		}
	} else {
		// Evaluate all policies bound to the resource type
		policyResult, err = a.engine.EvaluateForResource(ctx, a.resourceType, input)
		if err != nil {
			return nil, fmt.Errorf("resource policy evaluation failed: %w", err)
		}
	}

	// Convert policy result to access result
	return a.policyResultToAccessResult(policyResult, time.Since(start)), nil
}

// buildEvaluationInput builds a policy evaluation input from an access request.
func (a *PolicyEngineAdapter) buildEvaluationInput(req *AccessRequest) *policy.EvaluationInput {
	// Build resource representation
	resource := map[string]interface{}{
		"namespace": req.Namespace,
		"path":      req.Path,
		"full_path": req.FullPath(),
	}

	// Add identity information
	if req.Identity != nil {
		resource["identity"] = map[string]interface{}{
			"id":         req.Identity.ID,
			"type":       req.Identity.Type,
			"roles":      req.Identity.Roles,
			"attributes": req.Identity.Attributes,
		}
	}

	// Add request metadata
	if req.Metadata != nil {
		resource["metadata"] = req.Metadata
	}

	// Build context
	context := make(map[string]interface{})
	context["namespace"] = req.Namespace
	context["path"] = req.Path
	context["action"] = string(req.Action)

	if req.Identity != nil {
		context["identity_type"] = req.Identity.Type
		context["roles"] = req.Identity.Roles
	}

	// Get user from identity
	user := ""
	if req.Identity != nil {
		user = req.Identity.ID
	}

	return &policy.EvaluationInput{
		Resource:  resource,
		Action:    string(req.Action),
		User:      user,
		Context:   context,
		Timestamp: time.Now(),
	}
}

// policyResultToAccessResult converts a policy result to an access result.
func (a *PolicyEngineAdapter) policyResultToAccessResult(pr *policy.PolicyResult, duration time.Duration) *AccessResult {
	result := &AccessResult{
		Allowed:  pr.Allowed,
		Duration: duration,
	}

	// Build reason from results
	if !pr.Allowed && pr.Summary != nil {
		result.Reason = fmt.Sprintf("policy denied: %d violation(s)", pr.Summary.TotalViolations)

		// Find the first violation for details
		for _, evalResult := range pr.Results {
			if !evalResult.Allowed && len(evalResult.Violations) > 0 {
				result.Reason = evalResult.Violations[0].Message
				result.MatchedRule = evalResult.PolicyID
				break
			}
		}
	} else if pr.Allowed {
		result.Reason = "policy allowed"
		if pr.Summary != nil && pr.Summary.TotalPolicies > 0 {
			result.Reason = fmt.Sprintf("policy allowed: %d policy(ies) evaluated", pr.Summary.TotalPolicies)
		}
	}

	return result
}

// singleResultToPolicyResult wraps a single evaluation result in a PolicyResult.
func (a *PolicyEngineAdapter) singleResultToPolicyResult(er *policy.EvaluationResult) *policy.PolicyResult {
	summary := &policy.PolicySummary{
		TotalPolicies:        1,
		AllowedPolicies:      0,
		DeniedPolicies:       0,
		TotalViolations:      len(er.Violations),
		ViolationsBySeverity: make(map[policy.Severity]int),
	}

	if er.Allowed {
		summary.AllowedPolicies = 1
	} else {
		summary.DeniedPolicies = 1
	}

	for _, v := range er.Violations {
		summary.ViolationsBySeverity[v.Severity]++
	}

	return &policy.PolicyResult{
		Allowed:       er.Allowed,
		Results:       []*policy.EvaluationResult{er},
		Summary:       summary,
		TotalDuration: er.Duration,
		EvaluatedAt:   er.EvaluatedAt,
	}
}

// FileAccessPolicy creates a simple CEL policy for file access control.
// This is a helper function to create common file access policies.
func FileAccessPolicy(id, name string, celExpression string, severity policy.Severity) *policy.Policy {
	return &policy.Policy{
		ID:              id,
		Name:            name,
		Description:     fmt.Sprintf("File access policy: %s", name),
		Type:            policy.PolicyTypeCEL,
		Category:        policy.CategorySecurity,
		Severity:        severity,
		EnforcementMode: policy.ModeEnforce,
		Policy:          celExpression,
		Enabled:         true,
		Tags:            []string{"file-access", "security"},
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

// DefaultFileAccessPolicies returns a set of default file access policies.
// These can be registered with the policy engine for common access control scenarios.
func DefaultFileAccessPolicies() []*policy.Policy {
	return []*policy.Policy{
		// Require authentication for all file operations
		FileAccessPolicy(
			"file-require-auth",
			"Require Authentication",
			`resource.identity != null && resource.identity.id != ""`,
			policy.SeverityHigh,
		),

		// Block access to sensitive paths
		FileAccessPolicy(
			"file-block-secrets",
			"Block Secrets Access",
			`!resource.path.startsWith("/secrets/") || resource.identity.type == "admin"`,
			policy.SeverityCritical,
		),

		// Read-only for non-admin users
		FileAccessPolicy(
			"file-readonly-non-admin",
			"Read-Only for Non-Admin",
			`context.action == "get" || context.action == "list" || context.action == "stat" || has(resource.identity.roles) && resource.identity.roles.exists(r, r == "admin" || r == "writer")`,
			policy.SeverityMedium,
		),

		// Block executable files
		FileAccessPolicy(
			"file-block-executables",
			"Block Executable Uploads",
			`context.action != "put" || !(resource.path.endsWith(".exe") || resource.path.endsWith(".sh") || resource.path.endsWith(".bat"))`,
			policy.SeverityHigh,
		),
	}
}
