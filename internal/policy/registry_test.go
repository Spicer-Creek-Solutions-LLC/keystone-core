// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

func regPolicy(id string) *policy.Policy {
	return &policy.Policy{
		ID: id, Name: id, Type: audit.PolicyTypeBuiltin,
		Category: policy.CategorySecurity, Severity: audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit, Code: "{}", Enabled: true,
	}
}

func TestRegistry_RegisterPolicy(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	if err := r.RegisterPolicy(regPolicy("p1")); err != nil {
		t.Fatalf("register: %v", err)
	}
	// Duplicate.
	err := r.RegisterPolicy(regPolicy("p1"))
	if !errors.Is(err, policy.ErrDuplicateID) {
		t.Errorf("dup err = %v, want ErrDuplicateID", err)
	}
	// Invalid shape.
	if err := r.RegisterPolicy(&policy.Policy{ID: ""}); !errors.Is(err, policy.ErrInvalidPolicy) {
		t.Errorf("invalid err = %v", err)
	}
	got, err := r.GetPolicy("p1")
	if err != nil || got.ID != "p1" {
		t.Errorf("get: %v / %+v", err, got)
	}
	if _, err := r.GetPolicy("nope"); !errors.Is(err, policy.ErrNotFound) {
		t.Errorf("missing get err = %v", err)
	}
}

func TestRegistry_RegisterPolicy_StoresClone(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	p := regPolicy("p1")
	p.Tags = []string{"a"}
	_ = r.RegisterPolicy(p)
	p.Tags[0] = "MUT" // mutate after register
	got, _ := r.GetPolicy("p1")
	if got.Tags[0] != "a" {
		t.Errorf("registry stored a shared slice header: %v", got.Tags)
	}
	got.Name = "MUT" // mutate the returned copy
	again, _ := r.GetPolicy("p1")
	if again.Name == "MUT" {
		t.Errorf("GetPolicy returned a shared pointer")
	}
}

func TestRegistry_RegisterPolicySet_DanglingRejected(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	set := &policy.PolicySet{ID: "s1", Name: "s", PolicyIDs: []string{"p1"}, Enabled: true}
	err := r.RegisterPolicySet(set)
	if !errors.Is(err, policy.ErrDanglingReference) {
		t.Fatalf("dangling set err = %v, want ErrDanglingReference", err)
	}
	// Register the member, then the set succeeds.
	if err := r.RegisterPolicy(regPolicy("p1")); err != nil {
		t.Fatalf("register p1: %v", err)
	}
	if err := r.RegisterPolicySet(set); err != nil {
		t.Errorf("set register after member present: %v", err)
	}
	// Duplicate set ID.
	if err := r.RegisterPolicySet(set); !errors.Is(err, policy.ErrDuplicateID) {
		t.Errorf("dup set err = %v", err)
	}
}

func TestRegistry_RegisterBinding_DanglingRejected(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	b := &policy.Binding{ID: "b1", PolicyID: "p1", ResourceType: "secret", Enabled: true}
	if err := r.RegisterBinding(b); !errors.Is(err, policy.ErrDanglingReference) {
		t.Fatalf("dangling binding err = %v", err)
	}
	_ = r.RegisterPolicy(regPolicy("p1"))
	if err := r.RegisterBinding(b); err != nil {
		t.Errorf("binding after policy present: %v", err)
	}

	// Set-targeting binding with missing set.
	sb := &policy.Binding{ID: "b2", PolicySetID: "s1", ResourceType: "secret", Enabled: true}
	if err := r.RegisterBinding(sb); !errors.Is(err, policy.ErrDanglingReference) {
		t.Errorf("dangling set-binding err = %v", err)
	}
}

func TestRegistry_BindingsForResource(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(regPolicy("p1"))
	_ = r.RegisterPolicy(regPolicy("p2"))
	mustBind := func(b *policy.Binding) {
		if err := r.RegisterBinding(b); err != nil {
			t.Fatalf("bind %s: %v", b.ID, err)
		}
	}
	mustBind(&policy.Binding{ID: "z-secret-write", PolicyID: "p1", ResourceType: "secret", Action: "write", Enabled: true})
	mustBind(&policy.Binding{ID: "a-secret-any", PolicyID: "p2", ResourceType: "secret", Enabled: true})
	mustBind(&policy.Binding{ID: "lease-only", PolicyID: "p1", ResourceType: "lease", Enabled: true})
	mustBind(&policy.Binding{ID: "disabled", PolicyID: "p1", ResourceType: "secret", Enabled: false})

	got := r.BindingsForResource("secret", "write", nil)
	if len(got) != 2 {
		t.Fatalf("got %d bindings, want 2", len(got))
	}
	// Sorted by ID: a-secret-any before z-secret-write.
	if got[0].ID != "a-secret-any" || got[1].ID != "z-secret-write" {
		t.Errorf("order = %s, %s", got[0].ID, got[1].ID)
	}
	// "read" only matches the any-action binding.
	if rd := r.BindingsForResource("secret", "read", nil); len(rd) != 1 || rd[0].ID != "a-secret-any" {
		t.Errorf("read bindings = %+v", rd)
	}
}

func TestRegistry_Lists_Sorted_And_Cloned(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	_ = r.RegisterPolicy(regPolicy("b"))
	_ = r.RegisterPolicy(regPolicy("a"))
	ps := r.ListPolicies()
	if len(ps) != 2 || ps[0].ID != "a" || ps[1].ID != "b" {
		t.Fatalf("ListPolicies not sorted: %+v", ps)
	}
	ps[0].Name = "MUT"
	if again := r.ListPolicies(); again[0].Name == "MUT" {
		t.Errorf("ListPolicies returned shared pointers")
	}
}

func TestRegistry_ConcurrentRegisterAndList(t *testing.T) {
	t.Parallel()
	r := policy.NewRegistry()
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = r.RegisterPolicy(regPolicy(fmt.Sprintf("p-%03d", i)))
			_ = r.ListPolicies()
			_, _ = r.GetPolicy(fmt.Sprintf("p-%03d", i))
		}(i)
	}
	wg.Wait()
	if got := r.ListPolicies(); len(got) != n {
		t.Errorf("registered %d, want %d", len(got), n)
	}
}

func TestRegistry_NewRegistryIsolation(t *testing.T) {
	t.Parallel()
	r1 := policy.NewRegistry()
	r2 := policy.NewRegistry()
	_ = r1.RegisterPolicy(regPolicy("p1"))
	if _, err := r2.GetPolicy("p1"); !errors.Is(err, policy.ErrNotFound) {
		t.Errorf("registry state leaked across NewRegistry instances")
	}
}
