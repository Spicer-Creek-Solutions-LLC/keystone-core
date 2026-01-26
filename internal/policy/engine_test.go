package policy

import (
	"context"
	"testing"
)

func TestPolicyEngineEvaluate(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register OPA policy
	opaPolicy := &Policy{
		ID:      "test-opa",
		Name:    "Test OPA Policy",
		Type:    PolicyTypeOPA,
		Enabled: true,
		Policy: `
package test_opa

default allow = false

allow {
    input.action == "read"
}
`,
	}
	registry.RegisterPolicy(opaPolicy)

	// Register CEL policy
	celPolicy := &Policy{
		ID:      "test-cel",
		Name:    "Test CEL Policy",
		Type:    PolicyTypeCEL,
		Enabled: true,
		Policy:  `action == "write"`,
	}
	registry.RegisterPolicy(celPolicy)

	tests := []struct {
		name     string
		policyID string
		input    *EvaluationInput
		allowed  bool
	}{
		{
			name:     "OPA policy allows read",
			policyID: "test-opa",
			input: &EvaluationInput{
				Action: "read",
			},
			allowed: true,
		},
		{
			name:     "OPA policy denies write",
			policyID: "test-opa",
			input: &EvaluationInput{
				Action: "write",
			},
			allowed: false,
		},
		{
			name:     "CEL policy allows write",
			policyID: "test-cel",
			input: &EvaluationInput{
				Action: "write",
			},
			allowed: true,
		},
		{
			name:     "CEL policy denies read",
			policyID: "test-cel",
			input: &EvaluationInput{
				Action: "read",
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := engine.Evaluate(ctx, tt.policyID, tt.input)
			if err != nil {
				t.Fatalf("Evaluate failed: %v", err)
			}

			if result.Allowed != tt.allowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.allowed)
			}
		})
	}
}

func TestPolicyEngineDisabledPolicy(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register disabled policy
	policy := &Policy{
		ID:      "disabled",
		Name:    "Disabled Policy",
		Type:    PolicyTypeOPA,
		Enabled: false,
		Policy:  `package test\ndefault allow = false`,
	}
	registry.RegisterPolicy(policy)

	ctx := context.Background()
	result, err := engine.Evaluate(ctx, "disabled", &EvaluationInput{})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if !result.Allowed {
		t.Error("Disabled policy should allow by default")
	}

	if len(result.Warnings) == 0 {
		t.Error("Expected warning for disabled policy")
	}
}

func TestPolicyEngineEvaluatePolicySet(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register policies
	policy1 := &Policy{
		ID:      "policy1",
		Name:    "Policy 1",
		Type:    PolicyTypeCEL,
		Enabled: true,
		Policy:  `action == "read"`,
	}
	registry.RegisterPolicy(policy1)

	policy2 := &Policy{
		ID:      "policy2",
		Name:    "Policy 2",
		Type:    PolicyTypeCEL,
		Enabled: true,
		Policy:  `user == "admin"`,
	}
	registry.RegisterPolicy(policy2)

	// Register policy set
	set := &PolicySet{
		ID:              "test-set",
		Name:            "Test Set",
		Policies:        []string{"policy1", "policy2"},
		EnforcementMode: ModeEnforce,
		Enabled:         true,
	}
	registry.RegisterPolicySet(set)

	tests := []struct {
		name    string
		input   *EvaluationInput
		allowed bool
	}{
		{
			name: "both policies pass",
			input: &EvaluationInput{
				Action: "read",
				User:   "admin",
			},
			allowed: true,
		},
		{
			name: "one policy fails in enforce mode",
			input: &EvaluationInput{
				Action: "read",
				User:   "guest",
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := engine.EvaluatePolicySet(ctx, "test-set", tt.input)
			if err != nil {
				t.Fatalf("EvaluatePolicySet failed: %v", err)
			}

			if result.Allowed != tt.allowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.allowed)
			}

			if result.Summary.TotalPolicies != 2 {
				t.Errorf("TotalPolicies = %d, want 2", result.Summary.TotalPolicies)
			}
		})
	}
}

func TestPolicyEngineDisabledPolicySet(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register disabled policy set
	set := &PolicySet{
		ID:      "disabled-set",
		Name:    "Disabled Set",
		Enabled: false,
	}
	registry.RegisterPolicySet(set)

	ctx := context.Background()
	result, err := engine.EvaluatePolicySet(ctx, "disabled-set", &EvaluationInput{})
	if err != nil {
		t.Fatalf("EvaluatePolicySet failed: %v", err)
	}

	if !result.Allowed {
		t.Error("Disabled policy set should allow by default")
	}

	if result.Summary.TotalPolicies != 0 {
		t.Errorf("TotalPolicies = %d, want 0", result.Summary.TotalPolicies)
	}
}

func TestPolicyEngineEvaluateForResource(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register policy
	policy := &Policy{
		ID:      "resource-policy",
		Name:    "Resource Policy",
		Type:    PolicyTypeCEL,
		Enabled: true,
		Policy:  `action == "create"`,
	}
	registry.RegisterPolicy(policy)

	// Register binding
	binding := &PolicyBinding{
		ID:           "binding1",
		PolicyID:     "resource-policy",
		ResourceType: "pod",
		Actions:      []string{"create", "update"},
		Enabled:      true,
	}
	registry.RegisterBinding(binding)

	tests := []struct {
		name         string
		resourceType string
		input        *EvaluationInput
		allowed      bool
		evaluated    bool
	}{
		{
			name:         "matching resource and action",
			resourceType: "pod",
			input: &EvaluationInput{
				Action: "create",
			},
			allowed:   true,
			evaluated: true,
		},
		{
			name:         "matching resource, non-matching action in binding",
			resourceType: "pod",
			input: &EvaluationInput{
				Action: "delete",
			},
			allowed:   true,
			evaluated: false, // Policy not evaluated due to action filter
		},
		{
			name:         "non-matching resource",
			resourceType: "service",
			input: &EvaluationInput{
				Action: "create",
			},
			allowed:   true,
			evaluated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := engine.EvaluateForResource(ctx, tt.resourceType, tt.input)
			if err != nil {
				t.Fatalf("EvaluateForResource failed: %v", err)
			}

			if result.Allowed != tt.allowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.allowed)
			}

			if tt.evaluated {
				if result.Summary.TotalPolicies == 0 {
					t.Error("Expected policy to be evaluated")
				}
			} else {
				if result.Summary.TotalPolicies != 0 {
					t.Error("Expected no policies to be evaluated")
				}
			}
		})
	}
}

func TestPolicyEngineEvaluateBatch(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	policy := &Policy{
		ID:      "batch-policy",
		Name:    "Batch Policy",
		Type:    PolicyTypeCEL,
		Enabled: true,
		Policy:  `action == "read"`,
	}
	registry.RegisterPolicy(policy)

	inputs := []*EvaluationInput{
		{Action: "read"},
		{Action: "write"},
		{Action: "read"},
	}

	ctx := context.Background()
	results, err := engine.EvaluateBatch(ctx, "batch-policy", inputs)
	if err != nil {
		t.Fatalf("EvaluateBatch failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	if !results[0].Allowed {
		t.Error("First result should be allowed")
	}
	if results[1].Allowed {
		t.Error("Second result should be denied")
	}
	if !results[2].Allowed {
		t.Error("Third result should be allowed")
	}
}

func TestPolicyEngineEvaluateBatchParallel(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	policy := &Policy{
		ID:      "parallel-policy",
		Name:    "Parallel Policy",
		Type:    PolicyTypeCEL,
		Enabled: true,
		Policy:  `user == "admin"`,
	}
	registry.RegisterPolicy(policy)

	inputs := []*EvaluationInput{
		{User: "admin"},
		{User: "guest"},
		{User: "admin"},
		{User: "user"},
	}

	ctx := context.Background()
	results, err := engine.EvaluateBatchParallel(ctx, "parallel-policy", inputs)
	if err != nil {
		t.Fatalf("EvaluateBatchParallel failed: %v", err)
	}

	if len(results) != 4 {
		t.Fatalf("Expected 4 results, got %d", len(results))
	}

	expected := []bool{true, false, true, false}
	for i, expectedAllowed := range expected {
		if results[i].Allowed != expectedAllowed {
			t.Errorf("Result %d: Allowed = %v, want %v", i, results[i].Allowed, expectedAllowed)
		}
	}
}

func TestPolicyEngineValidatePolicy(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	tests := []struct {
		name    string
		policy  *Policy
		wantErr bool
	}{
		{
			name: "valid OPA policy",
			policy: &Policy{
				ID:   "valid-opa",
				Name: "Valid OPA",
				Type: PolicyTypeOPA,
				Policy: `package test

default allow = false`,
			},
			wantErr: false,
		},
		{
			name: "valid CEL policy",
			policy: &Policy{
				ID:     "valid-cel",
				Name:   "Valid CEL",
				Type:   PolicyTypeCEL,
				Policy: `action == "read"`,
			},
			wantErr: false,
		},
		{
			name: "invalid OPA policy",
			policy: &Policy{
				ID:     "invalid-opa",
				Name:   "Invalid OPA",
				Type:   PolicyTypeOPA,
				Policy: `invalid syntax`,
			},
			wantErr: true,
		},
		{
			name: "invalid CEL policy",
			policy: &Policy{
				ID:     "invalid-cel",
				Name:   "Invalid CEL",
				Type:   PolicyTypeCEL,
				Policy: `action ==`,
			},
			wantErr: true,
		},
		{
			name: "missing ID",
			policy: &Policy{
				Name:   "No ID",
				Type:   PolicyTypeCEL,
				Policy: `action == "read"`,
			},
			wantErr: true,
		},
		{
			name: "missing name",
			policy: &Policy{
				ID:     "no-name",
				Type:   PolicyTypeCEL,
				Policy: `action == "read"`,
			},
			wantErr: true,
		},
		{
			name: "missing policy code",
			policy: &Policy{
				ID:   "no-code",
				Name: "No Code",
				Type: PolicyTypeCEL,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := engine.ValidatePolicy(ctx, tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPolicyEngineEnforcementModes(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register a policy that will fail
	policy := &Policy{
		ID:      "deny-policy",
		Name:    "Deny Policy",
		Type:    PolicyTypeCEL,
		Enabled: true,
		Policy:  `action == "allowed_action"`, // Will fail for other actions
	}
	registry.RegisterPolicy(policy)

	tests := []struct {
		name            string
		enforcementMode EnforcementMode
		expectedAllowed bool
	}{
		{
			name:            "enforce mode denies violations",
			enforcementMode: ModeEnforce,
			expectedAllowed: false,
		},
		{
			name:            "audit mode allows violations",
			enforcementMode: ModeAudit,
			expectedAllowed: true, // Violations recorded but allowed
		},
		{
			name:            "warn mode allows violations",
			enforcementMode: ModeWarn,
			expectedAllowed: true, // Warnings generated but allowed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Update policy enforcement mode
			policy.EnforcementMode = tt.enforcementMode
			registry.UpdatePolicy(policy)

			// Create binding
			bindingID := "binding-" + string(tt.enforcementMode)
			binding := &PolicyBinding{
				ID:           bindingID,
				PolicyID:     "deny-policy",
				ResourceType: "test-resource",
				Enabled:      true,
			}
			registry.RegisterBinding(binding)

			ctx := context.Background()
			input := &EvaluationInput{
				Action: "forbidden_action", // Will be denied
			}

			result, err := engine.EvaluateForResource(ctx, "test-resource", input)
			if err != nil {
				t.Fatalf("EvaluateForResource failed: %v", err)
			}

			if result.Allowed != tt.expectedAllowed {
				t.Errorf("Allowed = %v, want %v for mode %s", result.Allowed, tt.expectedAllowed, tt.enforcementMode)
			}

			// Cleanup
			registry.DeleteBinding(bindingID)
		})
	}
}

func TestPolicyEngineAggregation(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	// Register multiple policies with different severities
	policies := []*Policy{
		{
			ID:       "critical-policy",
			Name:     "Critical Policy",
			Type:     PolicyTypeCEL,
			Severity: SeverityCritical,
			Enabled:  true,
			Policy:   `action == "never_allow"`,
		},
		{
			ID:       "high-policy",
			Name:     "High Policy",
			Type:     PolicyTypeCEL,
			Severity: SeverityHigh,
			Enabled:  true,
			Policy:   `user == "never_allow"`,
		},
	}

	for _, p := range policies {
		registry.RegisterPolicy(p)
	}

	// Register policy set
	set := &PolicySet{
		ID:              "severity-test",
		Name:            "Severity Test",
		Policies:        []string{"critical-policy", "high-policy"},
		EnforcementMode: ModeEnforce,
		Enabled:         true,
	}
	registry.RegisterPolicySet(set)

	ctx := context.Background()
	input := &EvaluationInput{
		Action: "some_action",
		User:   "some_user",
	}

	result, err := engine.EvaluatePolicySet(ctx, "severity-test", input)
	if err != nil {
		t.Fatalf("EvaluatePolicySet failed: %v", err)
	}

	if result.Allowed {
		t.Error("Expected violations to deny in enforce mode")
	}

	if result.Summary.DeniedPolicies != 2 {
		t.Errorf("DeniedPolicies = %d, want 2", result.Summary.DeniedPolicies)
	}

	if result.Summary.ViolationsBySeverity[SeverityCritical] != 1 {
		t.Errorf("CriticalViolations = %d, want 1", result.Summary.ViolationsBySeverity[SeverityCritical])
	}

	if result.Summary.ViolationsBySeverity[SeverityHigh] != 1 {
		t.Errorf("HighViolations = %d, want 1", result.Summary.ViolationsBySeverity[SeverityHigh])
	}
}

func TestPolicyEngineNotFound(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)

	ctx := context.Background()

	// Test non-existent policy
	_, err := engine.Evaluate(ctx, "nonexistent", &EvaluationInput{})
	if err == nil {
		t.Error("Expected error for nonexistent policy")
	}

	// Test non-existent policy set
	_, err = engine.EvaluatePolicySet(ctx, "nonexistent-set", &EvaluationInput{})
	if err == nil {
		t.Error("Expected error for nonexistent policy set")
	}
}
