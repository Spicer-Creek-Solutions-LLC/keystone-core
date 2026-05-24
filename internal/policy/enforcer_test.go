// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"context"
	"testing"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

func allowResults() []policy.EvaluationResult {
	return []policy.EvaluationResult{{PolicyID: "p1", Allowed: true}}
}

func denyResults() []policy.EvaluationResult {
	return []policy.EvaluationResult{
		{PolicyID: "p1", Allowed: true},
		{PolicyID: "p2", Allowed: false,
			Violations: []audit.Violation{{Rule: "r", Message: "no", Severity: audit.SeverityHigh}}},
	}
}

func TestEnforcer_DefaultDisabled(t *testing.T) {
	t.Parallel()
	e := policy.NewEnforcer()
	if e.Enabled() {
		t.Errorf("default Enforcer should be disabled (v1.0 audit-mode-only)")
	}
}

func TestEnforcer_V1_0_AlwaysAllows(t *testing.T) {
	t.Parallel()
	e := policy.NewEnforcer() // disabled

	// Denying verdict + Enforce mode → STILL allowed in v1.0.
	d := e.Enforce(context.Background(), audit.EnforcementModeEnforce, denyResults())
	if !d.Allowed {
		t.Errorf("v1.0 must always allow; got Allowed=false")
	}
	if !d.WouldDeny {
		t.Errorf("WouldDeny should record the real verdict (true) for reporting")
	}
	if d.Mode != audit.EnforcementModeEnforce {
		t.Errorf("Mode passthrough = %v, want Enforce", d.Mode)
	}
	if len(d.Results) != 2 {
		t.Errorf("Results passthrough = %d, want 2", len(d.Results))
	}
}

func TestEnforcer_V1_0_AllowingVerdict(t *testing.T) {
	t.Parallel()
	e := policy.NewEnforcer()
	d := e.Enforce(context.Background(), audit.EnforcementModeEnforce, allowResults())
	if !d.Allowed || d.WouldDeny {
		t.Errorf("allow verdict: Allowed=%v WouldDeny=%v, want true/false", d.Allowed, d.WouldDeny)
	}
}

func TestEnforcer_V1_0_EmptyResultsVacuouslyAllowed(t *testing.T) {
	t.Parallel()
	e := policy.NewEnforcer()
	d := e.Enforce(context.Background(), audit.EnforcementModeAudit, nil)
	if !d.Allowed || d.WouldDeny {
		t.Errorf("empty results: Allowed=%v WouldDeny=%v, want true/false (vacuous allow)", d.Allowed, d.WouldDeny)
	}
}

func TestEnforcer_V1_8Seam_GateSwitch(t *testing.T) {
	t.Parallel()
	e := policy.NewEnforcer(policy.WithEnforcementEnabled(true))
	if !e.Enabled() {
		t.Fatalf("WithEnforcementEnabled(true) not honored")
	}

	tests := []struct {
		name        string
		mode        audit.EnforcementMode
		results     []policy.EvaluationResult
		wantAllowed bool
	}{
		{"audit deny → allow (log only)", audit.EnforcementModeAudit, denyResults(), true},
		{"warn deny → allow", audit.EnforcementModeWarn, denyResults(), true},
		{"enforce deny → BLOCK", audit.EnforcementModeEnforce, denyResults(), false},
		{"enforce allow → allow", audit.EnforcementModeEnforce, allowResults(), true},
		{"unknown mode deny → allow (fail-open on unknown)", audit.EnforcementModeUnknown, denyResults(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := e.Enforce(context.Background(), tt.mode, tt.results)
			if d.Allowed != tt.wantAllowed {
				t.Errorf("Allowed = %v, want %v", d.Allowed, tt.wantAllowed)
			}
			// WouldDeny always reflects the real verdict, regardless
			// of mode or enabled state.
			wantWouldDeny := !policy.AllowedAll(tt.results)
			if d.WouldDeny != wantWouldDeny {
				t.Errorf("WouldDeny = %v, want %v", d.WouldDeny, wantWouldDeny)
			}
		})
	}
}

func TestEnforcer_WithEnforcementEnabledFalseStaysDisabled(t *testing.T) {
	t.Parallel()
	e := policy.NewEnforcer(policy.WithEnforcementEnabled(false))
	if e.Enabled() {
		t.Errorf("explicit false should stay disabled")
	}
	d := e.Enforce(context.Background(), audit.EnforcementModeEnforce, denyResults())
	if !d.Allowed {
		t.Errorf("disabled enforcer must allow even with Enforce mode + deny verdict")
	}
}
