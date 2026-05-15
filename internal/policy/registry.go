package policy

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Registry sentinel errors. ErrInvalidPolicy (policy.go) is the
// shape-validation family root; these cover registry-level
// referential + uniqueness failures so call sites can branch.
var (
	// ErrDuplicateID is returned when registering an ID that is
	// already present in the same namespace (policy / set / binding
	// IDs are independent namespaces).
	ErrDuplicateID = errors.New("policy: duplicate id")

	// ErrNotFound is returned by the Get* lookups and wrapped by
	// dangling-reference rejections.
	ErrNotFound = errors.New("policy: not found")

	// ErrDanglingReference is returned when a PolicySet references a
	// missing Policy, or a Binding references a missing Policy/Set.
	ErrDanglingReference = errors.New("policy: dangling reference")
)

// Registry is the in-memory policy store per §4.12. Safe for
// concurrent use — Register* take the write lock, the Get/List hot
// path takes the read lock. Mirrors statemgmt.Registry's shape.
//
// Stored values are deep-cloned on the way in and on the way out:
// a caller mutating a slice/map it passed to RegisterPolicy (or one
// it got back from GetPolicy) cannot corrupt registry state.
//
// Referential integrity is enforced at register time:
//
//   - RegisterPolicySet rejects a set whose PolicyIDs include an
//     unregistered policy.
//   - RegisterBinding rejects a binding whose PolicyID / PolicySetID
//     does not resolve.
//
// There is intentionally no Deregister in v1.0 — policy lifecycle
// CRUD is v1.8 (gRPC CreatePolicy / DeletePolicy are Unimplemented
// in task 12). Removing a policy a set/binding depends on would
// need cascade rules that aren't on the v1.0 critical path.
type Registry struct {
	mu       sync.RWMutex
	policies map[string]*Policy
	sets     map[string]*PolicySet
	bindings map[string]*Binding
}

// NewRegistry returns an empty Registry. Tests use this rather than
// a package global so state doesn't leak between cases.
func NewRegistry() *Registry {
	return &Registry{
		policies: make(map[string]*Policy),
		sets:     make(map[string]*PolicySet),
		bindings: make(map[string]*Binding),
	}
}

// RegisterPolicy validates + stores a deep copy of p. Returns
// ErrInvalidPolicy (wrapped) on shape failure, ErrDuplicateID
// (wrapped with the ID) when p.ID is already registered.
func (r *Registry) RegisterPolicy(p *Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.policies[p.ID]; exists {
		return fmt.Errorf("%w: policy %q", ErrDuplicateID, p.ID)
	}
	r.policies[p.ID] = p.Clone()
	return nil
}

// RegisterPolicySet validates + stores a deep copy of s. Every
// member PolicyID must already be registered (ErrDanglingReference
// otherwise) — register member policies first.
func (r *Registry) RegisterPolicySet(s *PolicySet) error {
	if err := s.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.sets[s.ID]; exists {
		return fmt.Errorf("%w: policy set %q", ErrDuplicateID, s.ID)
	}
	for _, pid := range s.PolicyIDs {
		if _, ok := r.policies[pid]; !ok {
			return fmt.Errorf("%w: policy set %q references unregistered policy %q",
				ErrDanglingReference, s.ID, pid)
		}
	}
	r.sets[s.ID] = s.Clone()
	return nil
}

// RegisterBinding validates + stores a deep copy of b. The bound
// PolicyID / PolicySetID must resolve (ErrDanglingReference
// otherwise).
func (r *Registry) RegisterBinding(b *Binding) error {
	if err := b.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.bindings[b.ID]; exists {
		return fmt.Errorf("%w: binding %q", ErrDuplicateID, b.ID)
	}
	if b.TargetsSet() {
		if _, ok := r.sets[b.PolicySetID]; !ok {
			return fmt.Errorf("%w: binding %q references unregistered policy set %q",
				ErrDanglingReference, b.ID, b.PolicySetID)
		}
	} else {
		if _, ok := r.policies[b.PolicyID]; !ok {
			return fmt.Errorf("%w: binding %q references unregistered policy %q",
				ErrDanglingReference, b.ID, b.PolicyID)
		}
	}
	r.bindings[b.ID] = b.Clone()
	return nil
}

// GetPolicy returns a deep copy of the registered policy, or
// ErrNotFound (wrapped with the ID).
func (r *Registry) GetPolicy(id string) (*Policy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.policies[id]
	if !ok {
		return nil, fmt.Errorf("%w: policy %q", ErrNotFound, id)
	}
	return p.Clone(), nil
}

// GetPolicySet returns a deep copy of the registered set, or
// ErrNotFound.
func (r *Registry) GetPolicySet(id string) (*PolicySet, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sets[id]
	if !ok {
		return nil, fmt.Errorf("%w: policy set %q", ErrNotFound, id)
	}
	return s.Clone(), nil
}

// GetBinding returns a deep copy of the registered binding, or
// ErrNotFound.
func (r *Registry) GetBinding(id string) (*Binding, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.bindings[id]
	if !ok {
		return nil, fmt.Errorf("%w: binding %q", ErrNotFound, id)
	}
	return b.Clone(), nil
}

// ListPolicies returns deep copies of every registered policy,
// sorted by ID for deterministic output.
func (r *Registry) ListPolicies() []*Policy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Policy, 0, len(r.policies))
	for _, p := range r.policies {
		out = append(out, p.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ListPolicySets returns deep copies of every set, sorted by ID.
func (r *Registry) ListPolicySets() []*PolicySet {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*PolicySet, 0, len(r.sets))
	for _, s := range r.sets {
		out = append(out, s.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ListBindings returns deep copies of every binding, sorted by ID.
func (r *Registry) ListBindings() []*Binding {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Binding, 0, len(r.bindings))
	for _, b := range r.bindings {
		out = append(out, b.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// BindingsForResource returns deep copies of every enabled binding
// matching the given resource type / action / labels, sorted by
// binding ID for deterministic evaluation order. A binding with an
// empty Action matches any action; an empty Selector matches any
// labels (see Binding.Matches).
func (r *Registry) BindingsForResource(resourceType, action string, labels map[string]string) []*Binding {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*Binding
	for _, b := range r.bindings {
		if b.Matches(resourceType, action, labels) {
			out = append(out, b.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
