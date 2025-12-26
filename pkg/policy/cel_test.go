package policy

import (
	"context"
	"testing"
)

func TestCELEvaluatorSimple(t *testing.T) {
	evaluator := NewCELEvaluator()

	policy := &Policy{
		ID:       "simple-cel",
		Name:     "Simple CEL Policy",
		Type:     PolicyTypeCEL,
		Severity: SeverityMedium,
		Policy:   `action == "read"`,
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

func TestCELEvaluatorComplex(t *testing.T) {
	evaluator := NewCELEvaluator()

	policy := &Policy{
		ID:       "complex-cel",
		Name:     "Complex CEL Policy",
		Type:     PolicyTypeCEL,
		Severity: SeverityHigh,
		Policy:   `(action == "read" && user == "admin") || (action == "read" && resource.public == true)`,
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

func TestCELEvaluatorResourceAccess(t *testing.T) {
	evaluator := NewCELEvaluator()

	policy := &Policy{
		ID:       "resource-access",
		Name:     "Resource Access Policy",
		Type:     PolicyTypeCEL,
		Severity: SeverityCritical,
		Policy:   `resource.owner == user || resource.shared == true`,
	}

	tests := []struct {
		name    string
		input   *EvaluationInput
		allowed bool
	}{
		{
			name: "owner access allowed",
			input: &EvaluationInput{
				User: "alice",
				Resource: map[string]interface{}{
					"owner":  "alice",
					"shared": false,
				},
			},
			allowed: true,
		},
		{
			name: "shared resource allowed",
			input: &EvaluationInput{
				User: "bob",
				Resource: map[string]interface{}{
					"owner":  "alice",
					"shared": true,
				},
			},
			allowed: true,
		},
		{
			name: "non-owner non-shared denied",
			input: &EvaluationInput{
				User: "bob",
				Resource: map[string]interface{}{
					"owner":  "alice",
					"shared": false,
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

func TestCELEvaluatorWithContext(t *testing.T) {
	evaluator := NewCELEvaluator()

	policy := &Policy{
		ID:       "context-policy",
		Name:     "Context-based Policy",
		Type:     PolicyTypeCEL,
		Severity: SeverityLow,
		Policy:   `context.environment == "production" && user == "admin"`,
	}

	tests := []struct {
		name    string
		input   *EvaluationInput
		allowed bool
	}{
		{
			name: "admin in production allowed",
			input: &EvaluationInput{
				User: "admin",
				Context: map[string]interface{}{
					"environment": "production",
				},
			},
			allowed: true,
		},
		{
			name: "admin in dev denied",
			input: &EvaluationInput{
				User: "admin",
				Context: map[string]interface{}{
					"environment": "development",
				},
			},
			allowed: false,
		},
		{
			name: "non-admin in production denied",
			input: &EvaluationInput{
				User: "guest",
				Context: map[string]interface{}{
					"environment": "production",
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

func TestCELEvaluatorInvalidPolicy(t *testing.T) {
	evaluator := NewCELEvaluator()

	policy := &Policy{
		ID:       "invalid-cel",
		Name:     "Invalid CEL Policy",
		Type:     PolicyTypeCEL,
		Severity: SeverityHigh,
		Policy:   `this is not valid CEL syntax !!!`,
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

func TestCELValidatePolicy(t *testing.T) {
	evaluator := NewCELEvaluator()
	ctx := context.Background()

	tests := []struct {
		name       string
		policyCode string
		wantErr    bool
	}{
		{
			name:       "valid simple expression",
			policyCode: `action == "read"`,
			wantErr:    false,
		},
		{
			name:       "valid complex expression",
			policyCode: `action == "read" && (user == "admin" || resource.public == true)`,
			wantErr:    false,
		},
		{
			name:       "invalid syntax",
			policyCode: `action == `,
			wantErr:    true,
		},
		{
			name:       "empty policy",
			policyCode: "",
			wantErr:    true,
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

func TestCELEvaluatorDuration(t *testing.T) {
	evaluator := NewCELEvaluator()

	policy := &Policy{
		ID:       "timing-policy",
		Name:     "Timing Policy",
		Type:     PolicyTypeCEL,
		Severity: SeverityLow,
		Policy:   `true`,
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

func TestCELEvaluatorWithDetails(t *testing.T) {
	evaluator := NewCELEvaluator()

	policy := &Policy{
		ID:       "details-policy",
		Name:     "Policy with Details",
		Type:     PolicyTypeCEL,
		Severity: SeverityMedium,
		Policy:   `action == "delete" && resource.protected == false`,
	}

	tests := []struct {
		name    string
		input   *EvaluationInput
		allowed bool
	}{
		{
			name: "allowed delete unprotected",
			input: &EvaluationInput{
				Action: "delete",
				Resource: map[string]interface{}{
					"protected": false,
				},
			},
			allowed: true,
		},
		{
			name: "denied delete protected",
			input: &EvaluationInput{
				Action: "delete",
				Resource: map[string]interface{}{
					"protected": true,
				},
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := evaluator.EvaluateWithDetails(ctx, policy, tt.input)
			if err != nil {
				t.Fatalf("EvaluateWithDetails failed: %v", err)
			}

			if result.Allowed != tt.allowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tt.allowed)
			}
		})
	}
}
