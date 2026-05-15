package policy_test

import (
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

func validSet() *policy.PolicySet {
	return &policy.PolicySet{
		ID:        "baseline",
		Name:      "Baseline Security",
		PolicyIDs: []string{"require-labels", "deny-privileged"},
		Enabled:   true,
	}
}

func TestPolicySet_Validate_OK(t *testing.T) {
	t.Parallel()
	if err := validSet().Validate(); err != nil {
		t.Errorf("valid set rejected: %v", err)
	}
	s := validSet()
	mode := audit.EnforcementModeEnforce
	s.EnforcementOverride = &mode
	if err := s.Validate(); err != nil {
		t.Errorf("set with valid override rejected: %v", err)
	}
}

func TestPolicySet_Validate_Rejections(t *testing.T) {
	t.Parallel()
	bad := audit.EnforcementModeUnknown
	tests := []struct {
		name   string
		mutate func(*policy.PolicySet)
	}{
		{"nil", nil},
		{"empty id", func(s *policy.PolicySet) { s.ID = "" }},
		{"empty name", func(s *policy.PolicySet) { s.Name = " " }},
		{"no members", func(s *policy.PolicySet) { s.PolicyIDs = nil }},
		{"empty member id", func(s *policy.PolicySet) { s.PolicyIDs = []string{"a", " "} }},
		{"duplicate member", func(s *policy.PolicySet) { s.PolicyIDs = []string{"a", "a"} }},
		{"invalid override", func(s *policy.PolicySet) { s.EnforcementOverride = &bad }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s *policy.PolicySet
			if tt.mutate != nil {
				s = validSet()
				tt.mutate(s)
			}
			err := s.Validate()
			if err == nil {
				t.Fatalf("expected error")
			}
			if !errors.Is(err, policy.ErrInvalidPolicy) {
				t.Errorf("err not ErrInvalidPolicy family: %v", err)
			}
		})
	}
}

func TestPolicySet_EffectiveMode(t *testing.T) {
	t.Parallel()
	s := validSet()
	// No override: policy's own mode wins.
	if got := s.EffectiveMode(audit.EnforcementModeAudit); got != audit.EnforcementModeAudit {
		t.Errorf("no override: got %v, want Audit", got)
	}
	// Override forces all members.
	mode := audit.EnforcementModeEnforce
	s.EnforcementOverride = &mode
	if got := s.EffectiveMode(audit.EnforcementModeAudit); got != audit.EnforcementModeEnforce {
		t.Errorf("override: got %v, want Enforce", got)
	}
	// Nil receiver returns the passed mode.
	var nilSet *policy.PolicySet
	if got := nilSet.EffectiveMode(audit.EnforcementModeWarn); got != audit.EnforcementModeWarn {
		t.Errorf("nil receiver: got %v, want Warn", got)
	}
}

func TestPolicySet_Clone_DeepCopies(t *testing.T) {
	t.Parallel()
	s := validSet()
	mode := audit.EnforcementModeEnforce
	s.EnforcementOverride = &mode
	s.Tags = []string{"t"}
	s.Metadata = map[string]string{"k": "v"}
	cp := s.Clone()

	cp.PolicyIDs[0] = "MUT"
	cp.Tags[0] = "MUT"
	cp.Metadata["k"] = "MUT"
	*cp.EnforcementOverride = audit.EnforcementModeAudit

	if s.PolicyIDs[0] == "MUT" {
		t.Errorf("PolicyIDs aliased")
	}
	if s.Tags[0] == "MUT" {
		t.Errorf("Tags aliased")
	}
	if s.Metadata["k"] == "MUT" {
		t.Errorf("Metadata aliased")
	}
	if *s.EnforcementOverride != audit.EnforcementModeEnforce {
		t.Errorf("EnforcementOverride pointer aliased")
	}
}

func TestPolicySet_Clone_Nil(t *testing.T) {
	t.Parallel()
	var s *policy.PolicySet
	if s.Clone() != nil {
		t.Errorf("nil clone non-nil")
	}
}
