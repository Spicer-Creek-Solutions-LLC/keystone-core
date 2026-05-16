package policy_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

func TestFramework_Surface(t *testing.T) {
	t.Parallel()
	for _, f := range policy.AllFrameworks() {
		if !f.IsKnown() || f.String() == "" {
			t.Errorf("framework %q: IsKnown/String broken", f)
		}
	}
	if policy.Framework("bogus").IsKnown() || policy.Framework("").IsKnown() {
		t.Errorf("bogus/empty reported known")
	}
}

func TestParseFramework(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    policy.Framework
		wantErr bool
	}{
		{"soc2", policy.FrameworkSOC2, false},
		{"  NIST-800-53 ", policy.FrameworkNIST80053, false},
		{"PCI-DSS", policy.FrameworkPCIDSS, false},
		{"custom", policy.FrameworkCustom, false},
		{"", "", true},
		{"sarbanes", "", true},
	}
	for _, tt := range tests {
		got, err := policy.ParseFramework(tt.in)
		if tt.wantErr {
			if err == nil || !errors.Is(err, policy.ErrInvalidPolicy) {
				t.Errorf("ParseFramework(%q): err=%v", tt.in, err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("ParseFramework(%q) = %q,%v want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestFramework_MarshalRoundTrip(t *testing.T) {
	t.Parallel()
	type w struct {
		F policy.Framework `json:"f"`
	}
	b, err := json.Marshal(w{F: policy.FrameworkISO27001})
	if err != nil {
		t.Fatalf("%v", err)
	}
	var out w
	if err := json.Unmarshal(b, &out); err != nil || out.F != policy.FrameworkISO27001 {
		t.Errorf("round trip: %q %v", out.F, err)
	}
	if _, err := json.Marshal(w{F: policy.Framework("nope")}); err == nil {
		t.Errorf("marshal of unknown framework succeeded")
	}
	if err := json.Unmarshal([]byte(`{"f":"nope"}`), &out); err == nil {
		t.Errorf("unmarshal of unknown framework succeeded")
	}
}

func validControl() *policy.ComplianceControl {
	return &policy.ComplianceControl{
		ID:        "CIS-1.1",
		Framework: policy.FrameworkCIS,
		Title:     "Ensure labels present",
		Severity:  audit.SeverityHigh,
		PolicyIDs: []string{"require-labels"},
	}
}

func TestComplianceControl_Validate(t *testing.T) {
	t.Parallel()
	if err := validControl().Validate(); err != nil {
		t.Errorf("valid control rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*policy.ComplianceControl)
	}{
		{"nil", nil},
		{"empty id", func(c *policy.ComplianceControl) { c.ID = " " }},
		{"unknown framework", func(c *policy.ComplianceControl) { c.Framework = "x" }},
		{"empty title", func(c *policy.ComplianceControl) { c.Title = "" }},
		{"invalid severity", func(c *policy.ComplianceControl) { c.Severity = audit.SeverityUnknown }},
		{"no policies", func(c *policy.ComplianceControl) { c.PolicyIDs = nil }},
		{"empty policy id", func(c *policy.ComplianceControl) { c.PolicyIDs = []string{"a", " "} }},
		{"dup policy id", func(c *policy.ComplianceControl) { c.PolicyIDs = []string{"a", "a"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c *policy.ComplianceControl
			if tt.mutate != nil {
				c = validControl()
				tt.mutate(c)
			}
			if err := c.Validate(); err == nil || !errors.Is(err, policy.ErrInvalidPolicy) {
				t.Errorf("err = %v, want ErrInvalidPolicy", err)
			}
		})
	}
}

func TestControlMapping_RegisterAndLookups(t *testing.T) {
	t.Parallel()
	m := policy.NewControlMapping()
	c1 := &policy.ComplianceControl{ID: "SOC2-CC6.1", Framework: policy.FrameworkSOC2,
		Title: "Access control", Severity: audit.SeverityHigh,
		PolicyIDs: []string{"require-labels", "deny-privileged"}}
	c2 := &policy.ComplianceControl{ID: "SOC2-CC7.2", Framework: policy.FrameworkSOC2,
		Title: "Monitoring", Severity: audit.SeverityMedium,
		PolicyIDs: []string{"deny-privileged"}}
	c3 := &policy.ComplianceControl{ID: "CIS-1.1", Framework: policy.FrameworkCIS,
		Title: "Labels", Severity: audit.SeverityLow,
		PolicyIDs: []string{"require-labels"}}
	for _, c := range []*policy.ComplianceControl{c1, c2, c3} {
		if err := m.RegisterControl(c); err != nil {
			t.Fatalf("register %s: %v", c.ID, err)
		}
	}

	if err := m.RegisterControl(c1); !errors.Is(err, policy.ErrDuplicateID) {
		t.Errorf("dup err = %v", err)
	}
	if _, err := m.GetControl("nope"); !errors.Is(err, policy.ErrNotFound) {
		t.Errorf("missing get err = %v", err)
	}
	if err := m.RegisterControl(&policy.ComplianceControl{ID: ""}); !errors.Is(err, policy.ErrInvalidPolicy) {
		t.Errorf("invalid err = %v", err)
	}

	// ControlsForPolicy (policy → controls).
	cs := m.ControlsForPolicy("deny-privileged")
	if len(cs) != 2 || cs[0].ID != "SOC2-CC6.1" || cs[1].ID != "SOC2-CC7.2" {
		t.Errorf("ControlsForPolicy(deny-privileged) = %+v", controlIDs(cs))
	}
	// ControlsForFramework.
	if cf := m.ControlsForFramework(policy.FrameworkSOC2); len(cf) != 2 {
		t.Errorf("SOC2 controls = %d, want 2", len(cf))
	}
	// PoliciesForFramework (framework → de-duped sorted policies).
	pf := m.PoliciesForFramework(policy.FrameworkSOC2)
	if len(pf) != 2 || pf[0] != "deny-privileged" || pf[1] != "require-labels" {
		t.Errorf("PoliciesForFramework(SOC2) = %v", pf)
	}
	if got := m.PoliciesForFramework(policy.FrameworkGDPR); got != nil {
		t.Errorf("PoliciesForFramework(GDPR) = %v, want nil", got)
	}
	if len(m.ListControls()) != 3 {
		t.Errorf("ListControls = %d, want 3", len(m.ListControls()))
	}
}

func TestControlMapping_StoresAndReturnsClones(t *testing.T) {
	t.Parallel()
	m := policy.NewControlMapping()
	c := validControl()
	c.PolicyIDs = []string{"a"}
	_ = m.RegisterControl(c)
	c.PolicyIDs[0] = "MUT" // mutate after register
	got, _ := m.GetControl(c.ID)
	if got.PolicyIDs[0] != "a" {
		t.Errorf("registry stored a shared slice header: %v", got.PolicyIDs)
	}
	got.PolicyIDs[0] = "MUT2" // mutate returned copy
	again, _ := m.GetControl(c.ID)
	if again.PolicyIDs[0] != "a" {
		t.Errorf("GetControl returned a shared slice header")
	}
}

func TestControlMapping_ConcurrentRegisterAndList(t *testing.T) {
	t.Parallel()
	m := policy.NewControlMapping()
	const n = 40
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = m.RegisterControl(&policy.ComplianceControl{
				ID: fmt.Sprintf("C-%03d", i), Framework: policy.FrameworkCustom,
				Title: "t", Severity: audit.SeverityLow, PolicyIDs: []string{"p"},
			})
			_ = m.ListControls()
			_ = m.ControlsForPolicy("p")
		}(i)
	}
	wg.Wait()
	if len(m.ListControls()) != n {
		t.Errorf("registered %d, want %d", len(m.ListControls()), n)
	}
}

func controlIDs(cs []*policy.ComplianceControl) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}
