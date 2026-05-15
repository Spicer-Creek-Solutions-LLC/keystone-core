package policy_test

import (
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/policy"
)

func TestBinding_Validate_OK(t *testing.T) {
	t.Parallel()
	pol := &policy.Binding{ID: "b1", PolicyID: "p1", ResourceType: "secret", Enabled: true}
	if err := pol.Validate(); err != nil {
		t.Errorf("policy binding rejected: %v", err)
	}
	set := &policy.Binding{ID: "b2", PolicySetID: "s1", ResourceType: "secret"}
	if err := set.Validate(); err != nil {
		t.Errorf("set binding rejected: %v", err)
	}
}

func TestBinding_Validate_Rejections(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		b    *policy.Binding
	}{
		{"nil", nil},
		{"empty id", &policy.Binding{PolicyID: "p", ResourceType: "secret"}},
		{"both refs", &policy.Binding{ID: "b", PolicyID: "p", PolicySetID: "s", ResourceType: "secret"}},
		{"neither ref", &policy.Binding{ID: "b", ResourceType: "secret"}},
		{"empty resource type", &policy.Binding{ID: "b", PolicyID: "p"}},
		{"empty selector key", &policy.Binding{ID: "b", PolicyID: "p", ResourceType: "secret", Selector: map[string]string{" ": "v"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.b.Validate()
			if err == nil {
				t.Fatalf("expected error")
			}
			if !errors.Is(err, policy.ErrInvalidPolicy) {
				t.Errorf("err not ErrInvalidPolicy family: %v", err)
			}
		})
	}
}

func TestBinding_TargetsSet(t *testing.T) {
	t.Parallel()
	if (&policy.Binding{PolicySetID: "s"}).TargetsSet() != true {
		t.Errorf("set binding not reported as TargetsSet")
	}
	if (&policy.Binding{PolicyID: "p"}).TargetsSet() != false {
		t.Errorf("policy binding reported as TargetsSet")
	}
}

func TestBinding_Matches(t *testing.T) {
	t.Parallel()
	base := func() *policy.Binding {
		return &policy.Binding{
			ID: "b", PolicyID: "p", ResourceType: "secret",
			Action: "write", Enabled: true,
			Selector: map[string]string{"env": "prod"},
		}
	}
	tests := []struct {
		name      string
		mutate    func(*policy.Binding)
		rt        string
		action    string
		labels    map[string]string
		wantMatch bool
	}{
		{"exact match", nil, "secret", "write", map[string]string{"env": "prod"}, true},
		{"wrong resource type", nil, "lease", "write", map[string]string{"env": "prod"}, false},
		{"wrong action", nil, "secret", "read", map[string]string{"env": "prod"}, false},
		{"selector mismatch", nil, "secret", "write", map[string]string{"env": "dev"}, false},
		{"selector missing key", nil, "secret", "write", map[string]string{"region": "us"}, false},
		{"extra labels ok (subset)", nil, "secret", "write", map[string]string{"env": "prod", "x": "y"}, true},
		{"empty action matches any", func(b *policy.Binding) { b.Action = "" }, "secret", "anything", map[string]string{"env": "prod"}, true},
		{"empty selector matches any", func(b *policy.Binding) { b.Selector = nil }, "secret", "write", nil, true},
		{"disabled never matches", func(b *policy.Binding) { b.Enabled = false }, "secret", "write", map[string]string{"env": "prod"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := base()
			if tt.mutate != nil {
				tt.mutate(b)
			}
			if got := b.Matches(tt.rt, tt.action, tt.labels); got != tt.wantMatch {
				t.Errorf("Matches = %v, want %v", got, tt.wantMatch)
			}
		})
	}
}

func TestBinding_Matches_NilReceiver(t *testing.T) {
	t.Parallel()
	var b *policy.Binding
	if b.Matches("secret", "write", nil) {
		t.Errorf("nil binding matched")
	}
}

func TestBinding_Clone_DeepCopies(t *testing.T) {
	t.Parallel()
	b := &policy.Binding{ID: "b", PolicyID: "p", ResourceType: "secret", Selector: map[string]string{"env": "prod"}}
	cp := b.Clone()
	cp.Selector["env"] = "MUT"
	if b.Selector["env"] != "prod" {
		t.Errorf("Selector aliased: %v", b.Selector)
	}
	var nilB *policy.Binding
	if nilB.Clone() != nil {
		t.Errorf("nil clone non-nil")
	}
}
