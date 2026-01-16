package policy

import (
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
)

func TestRegistryRegisterPolicy(t *testing.T) {
	registry := NewRegistry()

	policy := &Policy{
		ID:              "test-policy",
		Name:            "Test Policy",
		Description:     "A test policy",
		Type:            PolicyTypeOPA,
		Category:        CategorySecurity,
		Severity:        SeverityHigh,
		EnforcementMode: ModeEnforce,
		Policy:          "package test\ndefault allow = false",
		Enabled:         true,
	}

	err := registry.RegisterPolicy(policy)
	if err != nil {
		t.Fatalf("RegisterPolicy failed: %v", err)
	}

	retrieved, ok := registry.GetPolicy("test-policy")
	if !ok {
		t.Fatal("Policy not found")
	}

	if retrieved.Name != "Test Policy" {
		t.Errorf("Name = %s, want Test Policy", retrieved.Name)
	}

	if retrieved.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	if retrieved.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestRegistryRegisterPolicyValidation(t *testing.T) {
	registry := NewRegistry()

	tests := []struct {
		name    string
		policy  *Policy
		wantErr bool
	}{
		{
			name: "valid policy",
			policy: &Policy{
				ID:     "valid",
				Name:   "Valid",
				Policy: "code",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			policy: &Policy{
				Name:   "Test",
				Policy: "code",
			},
			wantErr: true,
		},
		{
			name: "missing name",
			policy: &Policy{
				ID:     "test",
				Policy: "code",
			},
			wantErr: true,
		},
		{
			name: "missing policy code",
			policy: &Policy{
				ID:   "test",
				Name: "Test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.RegisterPolicy(tt.policy)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegistryUpdatePolicy(t *testing.T) {
	registry := NewRegistry()

	policy := &Policy{
		ID:              "test-policy",
		Name:            "Test Policy",
		Policy:          "original",
		Type:            PolicyTypeOPA,
		EnforcementMode: ModeAudit,
	}

	err := registry.RegisterPolicy(policy)
	if err != nil {
		t.Fatalf("RegisterPolicy failed: %v", err)
	}

	originalCreatedAt := policy.CreatedAt

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return time.Since(originalCreatedAt) >= 10*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("expected UpdatedAt to have a later timestamp: %v", err)
	}

	updated := &Policy{
		ID:              "test-policy",
		Name:            "Updated Policy",
		Policy:          "updated",
		Type:            PolicyTypeOPA,
		EnforcementMode: ModeEnforce,
	}

	err = registry.UpdatePolicy(updated)
	if err != nil {
		t.Fatalf("UpdatePolicy failed: %v", err)
	}

	retrieved, _ := registry.GetPolicy("test-policy")
	if retrieved.Name != "Updated Policy" {
		t.Errorf("Name = %s, want Updated Policy", retrieved.Name)
	}

	if !retrieved.CreatedAt.Equal(originalCreatedAt) {
		t.Error("CreatedAt should not change on update")
	}

	if retrieved.UpdatedAt.Equal(originalCreatedAt) {
		t.Error("UpdatedAt should change on update")
	}
}

func TestRegistryDeletePolicy(t *testing.T) {
	registry := NewRegistry()

	policy := &Policy{
		ID:     "test-policy",
		Name:   "Test",
		Policy: "code",
	}

	registry.RegisterPolicy(policy)

	err := registry.DeletePolicy("test-policy")
	if err != nil {
		t.Fatalf("DeletePolicy failed: %v", err)
	}

	_, ok := registry.GetPolicy("test-policy")
	if ok {
		t.Error("Policy should be deleted")
	}

	err = registry.DeletePolicy("nonexistent")
	if err == nil {
		t.Error("Expected error for deleting nonexistent policy")
	}
}

func TestRegistryListPolicies(t *testing.T) {
	registry := NewRegistry()

	policies := []*Policy{
		{ID: "policy1", Name: "Policy 1", Policy: "code1", Category: CategorySecurity},
		{ID: "policy2", Name: "Policy 2", Policy: "code2", Category: CategoryCompliance},
		{ID: "policy3", Name: "Policy 3", Policy: "code3", Category: CategorySecurity},
	}

	for _, p := range policies {
		registry.RegisterPolicy(p)
	}

	all := registry.ListPolicies()
	if len(all) != 3 {
		t.Errorf("ListPolicies count = %d, want 3", len(all))
	}

	security := registry.ListPoliciesByCategory(CategorySecurity)
	if len(security) != 2 {
		t.Errorf("Security policies count = %d, want 2", len(security))
	}

	compliance := registry.ListPoliciesByCategory(CategoryCompliance)
	if len(compliance) != 1 {
		t.Errorf("Compliance policies count = %d, want 1", len(compliance))
	}
}

func TestRegistryListPoliciesByType(t *testing.T) {
	registry := NewRegistry()

	policies := []*Policy{
		{ID: "opa1", Name: "OPA 1", Policy: "code1", Type: PolicyTypeOPA},
		{ID: "cel1", Name: "CEL 1", Policy: "code2", Type: PolicyTypeCEL},
		{ID: "opa2", Name: "OPA 2", Policy: "code3", Type: PolicyTypeOPA},
	}

	for _, p := range policies {
		registry.RegisterPolicy(p)
	}

	opa := registry.ListPoliciesByType(PolicyTypeOPA)
	if len(opa) != 2 {
		t.Errorf("OPA policies count = %d, want 2", len(opa))
	}

	cel := registry.ListPoliciesByType(PolicyTypeCEL)
	if len(cel) != 1 {
		t.Errorf("CEL policies count = %d, want 1", len(cel))
	}
}

func TestRegistryPolicySet(t *testing.T) {
	registry := NewRegistry()

	set := &PolicySet{
		ID:              "test-set",
		Name:            "Test Set",
		Description:     "A test policy set",
		Policies:        []string{"policy1", "policy2"},
		EnforcementMode: ModeEnforce,
		Enabled:         true,
	}

	err := registry.RegisterPolicySet(set)
	if err != nil {
		t.Fatalf("RegisterPolicySet failed: %v", err)
	}

	retrieved, ok := registry.GetPolicySet("test-set")
	if !ok {
		t.Fatal("Policy set not found")
	}

	if retrieved.Name != "Test Set" {
		t.Errorf("Name = %s, want Test Set", retrieved.Name)
	}

	if len(retrieved.Policies) != 2 {
		t.Errorf("Policies count = %d, want 2", len(retrieved.Policies))
	}
}

func TestRegistryDeletePolicySet(t *testing.T) {
	registry := NewRegistry()

	set := &PolicySet{
		ID:   "test-set",
		Name: "Test",
	}

	registry.RegisterPolicySet(set)

	err := registry.DeletePolicySet("test-set")
	if err != nil {
		t.Fatalf("DeletePolicySet failed: %v", err)
	}

	_, ok := registry.GetPolicySet("test-set")
	if ok {
		t.Error("Policy set should be deleted")
	}
}

func TestRegistryBinding(t *testing.T) {
	registry := NewRegistry()

	binding := &PolicyBinding{
		ID:           "test-binding",
		PolicyID:     "policy1",
		ResourceType: "deployment",
		Actions:      []string{"create", "update"},
		Enabled:      true,
	}

	err := registry.RegisterBinding(binding)
	if err != nil {
		t.Fatalf("RegisterBinding failed: %v", err)
	}

	retrieved, ok := registry.GetBinding("test-binding")
	if !ok {
		t.Fatal("Binding not found")
	}

	if retrieved.ResourceType != "deployment" {
		t.Errorf("ResourceType = %s, want deployment", retrieved.ResourceType)
	}

	if len(retrieved.Actions) != 2 {
		t.Errorf("Actions count = %d, want 2", len(retrieved.Actions))
	}
}

func TestRegistryBindingValidation(t *testing.T) {
	registry := NewRegistry()

	tests := []struct {
		name    string
		binding *PolicyBinding
		wantErr bool
	}{
		{
			name: "valid with policy ID",
			binding: &PolicyBinding{
				ID:           "binding1",
				PolicyID:     "policy1",
				ResourceType: "deployment",
			},
			wantErr: false,
		},
		{
			name: "valid with policy set ID",
			binding: &PolicyBinding{
				ID:           "binding2",
				PolicySetID:  "set1",
				ResourceType: "deployment",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			binding: &PolicyBinding{
				PolicyID:     "policy1",
				ResourceType: "deployment",
			},
			wantErr: true,
		},
		{
			name: "missing policy reference",
			binding: &PolicyBinding{
				ID:           "binding3",
				ResourceType: "deployment",
			},
			wantErr: true,
		},
		{
			name: "missing resource type",
			binding: &PolicyBinding{
				ID:       "binding4",
				PolicyID: "policy1",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := registry.RegisterBinding(tt.binding)
			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterBinding() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegistryListBindingsForResource(t *testing.T) {
	registry := NewRegistry()

	bindings := []*PolicyBinding{
		{
			ID:           "binding1",
			PolicyID:     "policy1",
			ResourceType: "deployment",
			Enabled:      true,
		},
		{
			ID:           "binding2",
			PolicyID:     "policy2",
			ResourceType: "service",
			Enabled:      true,
		},
		{
			ID:           "binding3",
			PolicyID:     "policy3",
			ResourceType: "deployment",
			Enabled:      false, // Disabled
		},
	}

	for _, b := range bindings {
		registry.RegisterBinding(b)
	}

	deploymentBindings := registry.ListBindingsForResource("deployment")
	if len(deploymentBindings) != 1 {
		t.Errorf("Deployment bindings count = %d, want 1 (only enabled)", len(deploymentBindings))
	}

	serviceBindings := registry.ListBindingsForResource("service")
	if len(serviceBindings) != 1 {
		t.Errorf("Service bindings count = %d, want 1", len(serviceBindings))
	}
}
