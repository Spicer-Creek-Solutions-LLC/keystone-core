package policy

import (
	"context"
	"testing"
)

func TestOPAEvaluatorAllow(t *testing.T) {
	evaluator := NewOPAEvaluator()

	policy := &Policy{
		ID:       "test-policy",
		Name:     "Test Policy",
		Type:     PolicyTypeOPA,
		Severity: SeverityHigh,
		Policy: `
package test_policy

default allow = false

allow {
    input.action == "read"
}
`,
	}

	tests := []struct {
		name    string
		input   *EvaluationInput
		allowed bool
	}{
		{
			name: "allowed action",
			input: &EvaluationInput{
				Action: "read",
			},
			allowed: true,
		},
		{
			name: "denied action",
			input: &EvaluationInput{
				Action: "write",
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := evaluator.Evaluate(ctx, policy, tt.input)
			if err != nil {
				t.Fatalf("Evaluate failed: %v", err)
			}

			if result.Allowed != tt.allowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.allowed)
			}

			if !tt.allowed && len(result.Violations) == 0 {
				t.Error("Expected violations for denied action")
			}
		})
	}
}

func TestOPAEvaluatorComplex(t *testing.T) {
	evaluator := NewOPAEvaluator()

	policy := &Policy{
		ID:       "resource-policy",
		Name:     "Resource Policy",
		Type:     PolicyTypeOPA,
		Severity: SeverityMedium,
		Policy: `
package resource_policy

default allow = false

allow {
    input.action == "read"
    input.user == "admin"
}

allow {
    input.action == "read"
    input.resource.public == true
}
`,
	}

	tests := []struct {
		name    string
		input   *EvaluationInput
		allowed bool
	}{
		{
			name: "admin read allowed",
			input: &EvaluationInput{
				Action: "read",
				User:   "admin",
			},
			allowed: true,
		},
		{
			name: "public resource read allowed",
			input: &EvaluationInput{
				Action: "read",
				User:   "guest",
				Resource: map[string]interface{}{
					"public": true,
				},
			},
			allowed: true,
		},
		{
			name: "non-admin private resource denied",
			input: &EvaluationInput{
				Action: "read",
				User:   "guest",
				Resource: map[string]interface{}{
					"public": false,
				},
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := evaluator.Evaluate(ctx, policy, tt.input)
			if err != nil {
				t.Fatalf("Evaluate failed: %v", err)
			}

			if result.Allowed != tt.allowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.allowed)
			}
		})
	}
}

func TestOPAEvaluatorWithDeny(t *testing.T) {
	evaluator := NewOPAEvaluator()

	policy := &Policy{
		ID:       "deny-policy",
		Name:     "Deny Policy",
		Type:     PolicyTypeOPA,
		Severity: SeverityCritical,
		Policy: `
package deny_policy

default allow = true
default deny = false

deny {
    input.action == "delete"
    input.resource.protected == true
}

violations[msg] {
    deny
    msg := {"message": "Cannot delete protected resource", "path": "resource.protected"}
}
`,
	}

	tests := []struct {
		name    string
		input   *EvaluationInput
		allowed bool
		wantViolations bool
	}{
		{
			name: "allowed by default",
			input: &EvaluationInput{
				Action: "read",
			},
			allowed: true,
			wantViolations: false,
		},
		{
			name: "denied by explicit rule",
			input: &EvaluationInput{
				Action: "delete",
				Resource: map[string]interface{}{
					"protected": true,
				},
			},
			allowed: false,
			wantViolations: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := evaluator.EvaluateWithDeny(ctx, policy, tt.input)
			if err != nil {
				t.Fatalf("EvaluateWithDeny failed: %v", err)
			}

			if result.Allowed != tt.allowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.allowed)
			}

			if tt.wantViolations && len(result.Violations) == 0 {
				t.Error("Expected violations but got none")
			}
		})
	}
}

func TestOPAEvaluatorInvalidPolicy(t *testing.T) {
	evaluator := NewOPAEvaluator()

	policy := &Policy{
		ID:       "invalid-policy",
		Name:     "Invalid Policy",
		Type:     PolicyTypeOPA,
		Severity: SeverityHigh,
		Policy: `
package invalid_policy

invalid syntax here
`,
	}

	input := &EvaluationInput{
		Action: "read",
	}

	ctx := context.Background()
	_, err := evaluator.Evaluate(ctx, policy, input)
	if err == nil {
		t.Error("Expected error for invalid policy")
	}
}

func TestOPAValidatePolicy(t *testing.T) {
	evaluator := NewOPAEvaluator()
	ctx := context.Background()

	tests := []struct {
		name       string
		policyCode string
		wantErr    bool
	}{
		{
			name: "valid policy",
			policyCode: `
package test

default allow = false
allow {
    input.action == "read"
}
`,
			wantErr: false,
		},
		{
			name: "invalid syntax",
			policyCode: `
package test

this is not valid rego
`,
			wantErr: true,
		},
		{
			name: "empty policy",
			policyCode: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := evaluator.ValidatePolicy(ctx, tt.policyCode)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOPAEvaluatorDuration(t *testing.T) {
	evaluator := NewOPAEvaluator()

	policy := &Policy{
		ID:       "timing-policy",
		Name:     "Timing Policy",
		Type:     PolicyTypeOPA,
		Severity: SeverityLow,
		Policy: `
package timing_policy

default allow = true
`,
	}

	input := &EvaluationInput{
		Action: "test",
	}

	ctx := context.Background()
	result, err := evaluator.Evaluate(ctx, policy, input)
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.Duration == 0 {
		t.Error("Duration should be recorded")
	}

	if result.EvaluatedAt.IsZero() {
		t.Error("EvaluatedAt should be set")
	}
}
