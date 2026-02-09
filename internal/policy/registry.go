package policy

import (
	"fmt"
	"sync"
	"time"
)

// Registry manages policy storage and retrieval
type Registry struct {
	policies   map[string]*Policy
	policySets map[string]*PolicySet
	bindings   map[string]*PolicyBinding
	mu         sync.RWMutex
	setsMu     sync.RWMutex
	bindingsMu sync.RWMutex
}

// NewRegistry creates a new policy registry
func NewRegistry() *Registry {
	return &Registry{
		policies:   make(map[string]*Policy),
		policySets: make(map[string]*PolicySet),
		bindings:   make(map[string]*PolicyBinding),
	}
}

// RegisterPolicy registers a new policy
func (r *Registry) RegisterPolicy(policy *Policy) error {
	if policy.ID == "" {
		return fmt.Errorf("policy ID is required")
	}
	if policy.Name == "" {
		return fmt.Errorf("policy name is required")
	}
	if policy.Policy == "" {
		return fmt.Errorf("policy code is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if policy.CreatedAt.IsZero() {
		policy.CreatedAt = now
	}
	policy.UpdatedAt = now

	r.policies[policy.ID] = policy
	return nil
}

// GetPolicy retrieves a policy by ID
func (r *Registry) GetPolicy(id string) (*Policy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	policy, ok := r.policies[id]
	return policy, ok
}

// ListPolicies returns all policies
func (r *Registry) ListPolicies() []*Policy {
	r.mu.RLock()
	defer r.mu.RUnlock()

	policies := make([]*Policy, 0, len(r.policies))
	for _, policy := range r.policies {
		policies = append(policies, policy)
	}
	return policies
}

// ListPoliciesByCategory returns policies in a category
func (r *Registry) ListPoliciesByCategory(category PolicyCategory) []*Policy {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var policies []*Policy
	for _, policy := range r.policies {
		if policy.Category == category {
			policies = append(policies, policy)
		}
	}
	return policies
}

// ListPoliciesByType returns policies of a specific type
func (r *Registry) ListPoliciesByType(policyType PolicyType) []*Policy {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var policies []*Policy
	for _, policy := range r.policies {
		if policy.Type == policyType {
			policies = append(policies, policy)
		}
	}
	return policies
}

// UpdatePolicy updates an existing policy
func (r *Registry) UpdatePolicy(policy *Policy) error {
	if policy.ID == "" {
		return fmt.Errorf("policy ID is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.policies[policy.ID]
	if !ok {
		return fmt.Errorf("policy not found: %s", policy.ID)
	}

	policy.CreatedAt = existing.CreatedAt
	policy.UpdatedAt = time.Now()
	r.policies[policy.ID] = policy
	return nil
}

// DeletePolicy removes a policy
func (r *Registry) DeletePolicy(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.policies[id]; !ok {
		return fmt.Errorf("policy not found: %s", id)
	}

	delete(r.policies, id)
	return nil
}

// RegisterPolicySet registers a policy set
func (r *Registry) RegisterPolicySet(set *PolicySet) error {
	if set.ID == "" {
		return fmt.Errorf("policy set ID is required")
	}
	if set.Name == "" {
		return fmt.Errorf("policy set name is required")
	}

	r.setsMu.Lock()
	defer r.setsMu.Unlock()

	now := time.Now()
	if set.CreatedAt.IsZero() {
		set.CreatedAt = now
	}
	set.UpdatedAt = now

	r.policySets[set.ID] = set
	return nil
}

// GetPolicySet retrieves a policy set by ID
func (r *Registry) GetPolicySet(id string) (*PolicySet, bool) {
	r.setsMu.RLock()
	defer r.setsMu.RUnlock()
	set, ok := r.policySets[id]
	return set, ok
}

// ListPolicySets returns all policy sets
func (r *Registry) ListPolicySets() []*PolicySet {
	r.setsMu.RLock()
	defer r.setsMu.RUnlock()

	sets := make([]*PolicySet, 0, len(r.policySets))
	for _, set := range r.policySets {
		sets = append(sets, set)
	}
	return sets
}

// DeletePolicySet removes a policy set
func (r *Registry) DeletePolicySet(id string) error {
	r.setsMu.Lock()
	defer r.setsMu.Unlock()

	if _, ok := r.policySets[id]; !ok {
		return fmt.Errorf("policy set not found: %s", id)
	}

	delete(r.policySets, id)
	return nil
}

// RegisterBinding registers a policy binding
func (r *Registry) RegisterBinding(binding *PolicyBinding) error {
	if binding.ID == "" {
		return fmt.Errorf("binding ID is required")
	}
	if binding.PolicyID == "" && binding.PolicySetID == "" {
		return fmt.Errorf("either policy_id or policy_set_id is required")
	}
	if binding.ResourceType == "" {
		return fmt.Errorf("resource_type is required")
	}

	r.bindingsMu.Lock()
	defer r.bindingsMu.Unlock()

	now := time.Now()
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = now
	}
	binding.UpdatedAt = now

	r.bindings[binding.ID] = binding
	return nil
}

// GetBinding retrieves a binding by ID
func (r *Registry) GetBinding(id string) (*PolicyBinding, bool) {
	r.bindingsMu.RLock()
	defer r.bindingsMu.RUnlock()
	binding, ok := r.bindings[id]
	return binding, ok
}

// ListBindings returns all bindings
func (r *Registry) ListBindings() []*PolicyBinding {
	r.bindingsMu.RLock()
	defer r.bindingsMu.RUnlock()

	bindings := make([]*PolicyBinding, 0, len(r.bindings))
	for _, binding := range r.bindings {
		bindings = append(bindings, binding)
	}
	return bindings
}

// ListBindingsForResource returns bindings for a resource type
func (r *Registry) ListBindingsForResource(resourceType string) []*PolicyBinding {
	r.bindingsMu.RLock()
	defer r.bindingsMu.RUnlock()

	var bindings []*PolicyBinding
	for _, binding := range r.bindings {
		if binding.ResourceType == resourceType && binding.Enabled {
			bindings = append(bindings, binding)
		}
	}
	return bindings
}

// DeleteBinding removes a binding
func (r *Registry) DeleteBinding(id string) error {
	r.bindingsMu.Lock()
	defer r.bindingsMu.Unlock()

	if _, ok := r.bindings[id]; !ok {
		return fmt.Errorf("binding not found: %s", id)
	}

	delete(r.bindings, id)
	return nil
}
