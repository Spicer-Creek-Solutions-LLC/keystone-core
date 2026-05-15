package policy_test

import (
	"encoding/json"
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

func validPolicy() *policy.Policy {
	return &policy.Policy{
		ID:              "require-labels",
		Name:            "Require Labels",
		Type:            audit.PolicyTypeBuiltin,
		Category:        policy.CategorySecurity,
		Severity:        audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit,
		Code:            `{"rule":"require-labels"}`,
		Enabled:         true,
	}
}

func TestCategory_String_IsKnown(t *testing.T) {
	t.Parallel()
	for _, c := range policy.AllCategories() {
		if !c.IsKnown() {
			t.Errorf("%q not known", c)
		}
		if c.String() == "" {
			t.Errorf("empty String for %v", c)
		}
	}
	if policy.Category("bogus").IsKnown() {
		t.Errorf("bogus reported known")
	}
	if policy.Category("").IsKnown() {
		t.Errorf("empty reported known")
	}
}

func TestParseCategory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    policy.Category
		wantErr bool
	}{
		{"security", policy.CategorySecurity, false},
		{"  Compliance ", policy.CategoryCompliance, false},
		{"COST", policy.CategoryCost, false},
		{"custom", policy.CategoryCustom, false},
		{"operational", policy.CategoryOperational, false},
		{"", "", true},
		{"   ", "", true},
		{"unknown", "", true},
	}
	for _, tt := range tests {
		got, err := policy.ParseCategory(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("ParseCategory(%q): want error", tt.in)
			}
			if !errors.Is(err, policy.ErrInvalidPolicy) {
				t.Errorf("ParseCategory(%q): err not ErrInvalidPolicy family: %v", tt.in, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseCategory(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("ParseCategory(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCategory_MarshalRoundTrip(t *testing.T) {
	t.Parallel()
	type wrap struct {
		C policy.Category `json:"c"`
	}
	in := wrap{C: policy.CategoryCompliance}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out wrap
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.C != policy.CategoryCompliance {
		t.Errorf("round trip = %q", out.C)
	}
}

func TestCategory_MarshalRejectsUnknown(t *testing.T) {
	t.Parallel()
	_, err := json.Marshal(struct {
		C policy.Category `json:"c"`
	}{C: policy.Category("nope")})
	if err == nil {
		t.Errorf("marshal of unknown category succeeded")
	}
}

func TestCategory_UnmarshalRejectsUnknown(t *testing.T) {
	t.Parallel()
	var w struct {
		C policy.Category `json:"c"`
	}
	if err := json.Unmarshal([]byte(`{"c":"bogus"}`), &w); err == nil {
		t.Errorf("unmarshal of bogus category succeeded")
	}
}

func TestPolicy_Validate_OK(t *testing.T) {
	t.Parallel()
	if err := validPolicy().Validate(); err != nil {
		t.Errorf("valid policy rejected: %v", err)
	}
	// Disabled is still valid — Enabled gates evaluation not registration.
	p := validPolicy()
	p.Enabled = false
	if err := p.Validate(); err != nil {
		t.Errorf("disabled policy rejected: %v", err)
	}
}

func TestPolicy_Validate_Rejections(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*policy.Policy)
	}{
		{"nil", nil},
		{"empty id", func(p *policy.Policy) { p.ID = "  " }},
		{"empty name", func(p *policy.Policy) { p.Name = "" }},
		{"unknown type", func(p *policy.Policy) { p.Type = audit.PolicyType("ldap") }},
		{"empty type", func(p *policy.Policy) { p.Type = "" }},
		{"unknown category", func(p *policy.Policy) { p.Category = policy.Category("misc") }},
		{"invalid severity", func(p *policy.Policy) { p.Severity = audit.SeverityUnknown }},
		{"invalid enforcement", func(p *policy.Policy) { p.EnforcementMode = audit.EnforcementModeUnknown }},
		{"empty code", func(p *policy.Policy) { p.Code = "   " }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p *policy.Policy
			if tt.mutate != nil {
				p = validPolicy()
				tt.mutate(p)
			}
			err := p.Validate()
			if err == nil {
				t.Fatalf("%s: expected error", tt.name)
			}
			if !errors.Is(err, policy.ErrInvalidPolicy) {
				t.Errorf("%s: err not ErrInvalidPolicy family: %v", tt.name, err)
			}
		})
	}
}

func TestPolicy_Clone_DeepCopies(t *testing.T) {
	t.Parallel()
	p := validPolicy()
	p.Tags = []string{"a", "b"}
	p.Metadata = map[string]string{"k": "v"}
	cp := p.Clone()
	cp.Tags[0] = "MUT"
	cp.Metadata["k"] = "MUT"
	if p.Tags[0] != "a" {
		t.Errorf("Tags aliased: %v", p.Tags)
	}
	if p.Metadata["k"] != "v" {
		t.Errorf("Metadata aliased: %v", p.Metadata)
	}
}

func TestPolicy_Clone_Nil(t *testing.T) {
	t.Parallel()
	var p *policy.Policy
	if p.Clone() != nil {
		t.Errorf("nil clone non-nil")
	}
}
