package access

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/policy"
)

func TestNewPolicyEngineAdapter(t *testing.T) {
	registry := policy.NewRegistry()
	engine := policy.NewPolicyEngine(registry)

	tests := []struct {
		name    string
		config  *PolicyEngineAdapterConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "nil engine",
			config: &PolicyEngineAdapterConfig{
				Engine: nil,
			},
			wantErr: true,
		},
		{
			name: "valid config with defaults",
			config: &PolicyEngineAdapterConfig{
				Engine: engine,
			},
			wantErr: false,
		},
		{
			name: "valid config with resource type",
			config: &PolicyEngineAdapterConfig{
				Engine:       engine,
				ResourceType: "custom-file",
			},
			wantErr: false,
		},
		{
			name: "valid config with policy ID",
			config: &PolicyEngineAdapterConfig{
				Engine:   engine,
				PolicyID: "test-policy",
			},
			wantErr: false,
		},
		{
			name: "valid config with policy set ID",
			config: &PolicyEngineAdapterConfig{
				Engine:      engine,
				PolicySetID: "test-set",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := NewPolicyEngineAdapter(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPolicyEngineAdapter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && adapter == nil {
				t.Error("NewPolicyEngineAdapter() returned nil adapter")
			}
		})
	}
}

func TestPolicyEngineAdapter_Evaluate(t *testing.T) {
	// Create registry and engine
	registry := policy.NewRegistry()
	engine := policy.NewPolicyEngine(registry)

	// Register a simple CEL policy that allows read access
	celPolicy := &policy.Policy{
		ID:              "test-read-only",
		Name:            "Test Read Only",
		Type:            policy.PolicyTypeCEL,
		Category:        policy.CategorySecurity,
		Severity:        policy.SeverityMedium,
		EnforcementMode: policy.ModeEnforce,
		Policy:          `action == "get" || action == "list" || action == "stat"`,
		Enabled:         true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := registry.RegisterPolicy(celPolicy); err != nil {
		t.Fatalf("Failed to register policy: %v", err)
	}

	// Create adapter with specific policy
	adapter, err := NewPolicyEngineAdapter(&PolicyEngineAdapterConfig{
		Engine:   engine,
		PolicyID: "test-read-only",
	})
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	tests := []struct {
		name        string
		req         *AccessRequest
		wantAllowed bool
	}{
		{
			name: "allow read",
			req: &AccessRequest{
				Identity: &Identity{
					ID:   "user1",
					Type: "user",
				},
				Namespace: "test",
				Path:      "/file.txt",
				Action:    ActionGet,
			},
			wantAllowed: true,
		},
		{
			name: "allow list",
			req: &AccessRequest{
				Identity: &Identity{
					ID:   "user1",
					Type: "user",
				},
				Namespace: "test",
				Path:      "/",
				Action:    ActionList,
			},
			wantAllowed: true,
		},
		{
			name: "deny write",
			req: &AccessRequest{
				Identity: &Identity{
					ID:   "user1",
					Type: "user",
				},
				Namespace: "test",
				Path:      "/file.txt",
				Action:    ActionPut,
			},
			wantAllowed: false,
		},
		{
			name: "deny delete",
			req: &AccessRequest{
				Identity: &Identity{
					ID:   "user1",
					Type: "user",
				},
				Namespace: "test",
				Path:      "/file.txt",
				Action:    ActionDelete,
			},
			wantAllowed: false,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := adapter.Evaluate(ctx, tt.req)
			if err != nil {
				t.Errorf("Evaluate() error = %v", err)
				return
			}
			if result.Allowed != tt.wantAllowed {
				t.Errorf("Evaluate() allowed = %v, want %v", result.Allowed, tt.wantAllowed)
			}
		})
	}
}

func TestPolicyEngineAdapter_EvaluateForResource(t *testing.T) {
	// Create registry and engine
	registry := policy.NewRegistry()
	engine := policy.NewPolicyEngine(registry)

	// Register a policy
	celPolicy := &policy.Policy{
		ID:              "file-namespace-check",
		Name:            "File Namespace Check",
		Type:            policy.PolicyTypeCEL,
		Category:        policy.CategorySecurity,
		Severity:        policy.SeverityMedium,
		EnforcementMode: policy.ModeEnforce,
		Policy:          `context.namespace == "allowed"`,
		Enabled:         true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := registry.RegisterPolicy(celPolicy); err != nil {
		t.Fatalf("Failed to register policy: %v", err)
	}

	// Bind policy to file resource type
	binding := &policy.PolicyBinding{
		ID:           "file-binding",
		PolicyID:     "file-namespace-check",
		ResourceType: "file",
		Actions:      []string{"get", "put"},
		Enabled:      true,
	}
	if err := registry.RegisterBinding(binding); err != nil {
		t.Fatalf("Failed to register binding: %v", err)
	}

	// Create adapter using resource type (no specific policy)
	adapter, err := NewPolicyEngineAdapter(&PolicyEngineAdapterConfig{
		Engine:       engine,
		ResourceType: "file",
	})
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	ctx := context.Background()

	// Test allowed namespace
	allowedReq := &AccessRequest{
		Identity: &Identity{
			ID:   "user1",
			Type: "user",
		},
		Namespace: "allowed",
		Path:      "/file.txt",
		Action:    ActionGet,
	}
	result, err := adapter.Evaluate(ctx, allowedReq)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Allowed {
		t.Errorf("Expected allowed for 'allowed' namespace, got denied: %s", result.Reason)
	}

	// Test denied namespace
	deniedReq := &AccessRequest{
		Identity: &Identity{
			ID:   "user1",
			Type: "user",
		},
		Namespace: "denied",
		Path:      "/file.txt",
		Action:    ActionGet,
	}
	result, err = adapter.Evaluate(ctx, deniedReq)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if result.Allowed {
		t.Errorf("Expected denied for 'denied' namespace, got allowed")
	}
}

func TestBuildEvaluationInput(t *testing.T) {
	registry := policy.NewRegistry()
	engine := policy.NewPolicyEngine(registry)

	adapter, err := NewPolicyEngineAdapter(&PolicyEngineAdapterConfig{
		Engine: engine,
	})
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	req := &AccessRequest{
		Identity: &Identity{
			ID:         "spiffe://example.com/agent/1",
			Type:       "agent",
			Roles:      []string{"reader", "writer"},
			Attributes: map[string]string{"env": "prod"},
		},
		Namespace: "packages",
		Path:      "/myapp/v1.0.0.tar.gz",
		Action:    ActionGet,
		Metadata:  map[string]string{"source": "cli"},
	}

	input := adapter.buildEvaluationInput(req)

	// Verify resource
	resource, ok := input.Resource.(map[string]interface{})
	if !ok {
		t.Fatal("Expected resource to be a map")
	}
	if resource["namespace"] != "packages" {
		t.Errorf("Expected namespace 'packages', got %v", resource["namespace"])
	}
	if resource["path"] != "/myapp/v1.0.0.tar.gz" {
		t.Errorf("Expected path '/myapp/v1.0.0.tar.gz', got %v", resource["path"])
	}
	if resource["full_path"] != "/packages/myapp/v1.0.0.tar.gz" {
		t.Errorf("Expected full_path '/packages/myapp/v1.0.0.tar.gz', got %v", resource["full_path"])
	}

	// Verify identity in resource
	identity, ok := resource["identity"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected identity to be a map")
	}
	if identity["id"] != "spiffe://example.com/agent/1" {
		t.Errorf("Expected identity ID, got %v", identity["id"])
	}
	if identity["type"] != "agent" {
		t.Errorf("Expected identity type 'agent', got %v", identity["type"])
	}

	// Verify action
	if input.Action != "get" {
		t.Errorf("Expected action 'get', got %v", input.Action)
	}

	// Verify user
	if input.User != "spiffe://example.com/agent/1" {
		t.Errorf("Expected user to be identity ID, got %v", input.User)
	}

	// Verify context
	if input.Context["namespace"] != "packages" {
		t.Errorf("Expected context namespace 'packages', got %v", input.Context["namespace"])
	}
}

func TestFileAccessPolicy(t *testing.T) {
	p := FileAccessPolicy(
		"test-policy",
		"Test Policy",
		`resource.path.startsWith("/allowed/")`,
		policy.SeverityHigh,
	)

	if p.ID != "test-policy" {
		t.Errorf("Expected ID 'test-policy', got %s", p.ID)
	}
	if p.Name != "Test Policy" {
		t.Errorf("Expected Name 'Test Policy', got %s", p.Name)
	}
	if p.Type != policy.PolicyTypeCEL {
		t.Errorf("Expected Type CEL, got %v", p.Type)
	}
	if p.Severity != policy.SeverityHigh {
		t.Errorf("Expected Severity High, got %v", p.Severity)
	}
	if !p.Enabled {
		t.Error("Expected policy to be enabled")
	}
}

func TestDefaultFileAccessPolicies(t *testing.T) {
	policies := DefaultFileAccessPolicies()

	if len(policies) < 4 {
		t.Errorf("Expected at least 4 default policies, got %d", len(policies))
	}

	// Check policy IDs exist
	expectedIDs := []string{
		"file-require-auth",
		"file-block-secrets",
		"file-readonly-non-admin",
		"file-block-executables",
	}

	policyIDs := make(map[string]bool)
	for _, p := range policies {
		policyIDs[p.ID] = true
	}

	for _, id := range expectedIDs {
		if !policyIDs[id] {
			t.Errorf("Expected default policy with ID '%s'", id)
		}
	}
}

func TestPolicyEngineAdapter_NoPolicies(t *testing.T) {
	// Create empty registry and engine
	registry := policy.NewRegistry()
	engine := policy.NewPolicyEngine(registry)

	adapter, err := NewPolicyEngineAdapter(&PolicyEngineAdapterConfig{
		Engine:       engine,
		ResourceType: "file",
	})
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	ctx := context.Background()
	req := &AccessRequest{
		Identity: &Identity{
			ID:   "user1",
			Type: "user",
		},
		Namespace: "test",
		Path:      "/file.txt",
		Action:    ActionGet,
	}

	// With no policies, should be allowed
	result, err := adapter.Evaluate(ctx, req)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !result.Allowed {
		t.Errorf("Expected allowed when no policies, got denied: %s", result.Reason)
	}
}
