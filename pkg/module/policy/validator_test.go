package policy

import (
	"testing"
	"time"

	policypkg "github.com/shawnbutts/keystone-core/pkg/policy"
)

func TestDefaultCapabilityPolicyConfig(t *testing.T) {
	config := DefaultCapabilityPolicyConfig()

	// Verify defaults
	if config.AllowByDefault {
		t.Error("AllowByDefault should be false by default")
	}

	// Verify require approval capabilities
	expectedApproval := []string{"exec", "http.post", "secrets.write"}
	if len(config.RequireApprovalCapabilities) != len(expectedApproval) {
		t.Errorf("RequireApprovalCapabilities count = %d, want %d",
			len(config.RequireApprovalCapabilities), len(expectedApproval))
	}

	// Verify trust level requirements
	if config.TrustLevelRequirements["exec"] != TrustLevelVerified {
		t.Error("exec should require Verified trust level")
	}
	if config.TrustLevelRequirements["http.post"] != TrustLevelCommunity {
		t.Error("http.post should require Community trust level")
	}
	if config.TrustLevelRequirements["secrets.write"] != TrustLevelVerified {
		t.Error("secrets.write should require Verified trust level")
	}

	// Verify environment restrictions
	prodRestrictions := config.EnvironmentRestrictions["prod"]
	if len(prodRestrictions) != 2 {
		t.Errorf("prod environment restrictions count = %d, want 2", len(prodRestrictions))
	}
}

func TestNewModulePolicyEngine(t *testing.T) {
	registry := policypkg.NewRegistry()
	policyEngine := policypkg.NewPolicyEngine(registry)
	engine := NewModulePolicyEngine(policyEngine, nil)

	if engine == nil {
		t.Fatal("NewModulePolicyEngine returned nil")
	}

	if engine.PolicyEngine != policyEngine {
		t.Error("PolicyEngine not set correctly")
	}

	if engine.CapabilityConfig == nil {
		t.Error("CapabilityConfig should be initialized with defaults")
	}

	if engine.EnforcementMode != policypkg.ModeEnforce {
		t.Error("EnforcementMode should default to Enforce")
	}
}

func TestValidateCapability_Blocked(t *testing.T) {
	config := &CapabilityPolicyConfig{
		AllowByDefault:      true,
		BlockedCapabilities: []string{"dangerous.capability"},
	}

	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), config)

	ctx := &ModulePolicyContext{
		Module: &ModuleInfo{
			Name:    "test/module",
			Version: "1.0.0",
		},
		TrustLevel:  TrustLevelVerified,
		Environment: "dev",
	}

	// Blocked capability should be denied
	allowed, err := engine.ValidateCapability(ctx, "dangerous.capability")
	if err != nil {
		t.Fatalf("ValidateCapability error = %v", err)
	}
	if allowed {
		t.Error("Blocked capability should be denied")
	}
}

func TestValidateCapability_TrustLevel(t *testing.T) {
	config := &CapabilityPolicyConfig{
		AllowByDefault: true,
		TrustLevelRequirements: map[string]TrustLevel{
			"exec":          TrustLevelVerified,
			"http.post":     TrustLevelCommunity,
			"secrets.write": TrustLevelVerified,
		},
	}

	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), config)

	testCases := []struct {
		name       string
		capability string
		trustLevel TrustLevel
		wantAllow  bool
	}{
		{
			name:       "exec with verified trust - allowed",
			capability: "exec",
			trustLevel: TrustLevelVerified,
			wantAllow:  true,
		},
		{
			name:       "exec with community trust - denied",
			capability: "exec",
			trustLevel: TrustLevelCommunity,
			wantAllow:  false,
		},
		{
			name:       "exec with system trust - allowed",
			capability: "exec",
			trustLevel: TrustLevelSystem,
			wantAllow:  true,
		},
		{
			name:       "http.post with community trust - allowed",
			capability: "http.post",
			trustLevel: TrustLevelCommunity,
			wantAllow:  true,
		},
		{
			name:       "http.post with untrusted - denied",
			capability: "http.post",
			trustLevel: TrustLevelUntrusted,
			wantAllow:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &ModulePolicyContext{
				Module: &ModuleInfo{
					Name:    "test/module",
					Version: "1.0.0",
				},
				TrustLevel:  tc.trustLevel,
				Environment: "dev",
			}

			allowed, err := engine.ValidateCapability(ctx, tc.capability)
			if err != nil {
				t.Fatalf("ValidateCapability error = %v", err)
			}
			if allowed != tc.wantAllow {
				t.Errorf("ValidateCapability(%s, %s) = %v, want %v",
					tc.capability, tc.trustLevel, allowed, tc.wantAllow)
			}
		})
	}
}

func TestValidateCapability_EnvironmentRestrictions(t *testing.T) {
	config := &CapabilityPolicyConfig{
		AllowByDefault: true,
		EnvironmentRestrictions: map[string][]string{
			"prod": {"exec", "secrets.write"},
		},
	}

	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), config)

	testCases := []struct {
		name        string
		capability  string
		environment string
		wantAllow   bool
	}{
		{
			name:        "exec in prod - denied",
			capability:  "exec",
			environment: "prod",
			wantAllow:   false,
		},
		{
			name:        "exec in dev - allowed",
			capability:  "exec",
			environment: "dev",
			wantAllow:   true,
		},
		{
			name:        "secrets.write in prod - denied",
			capability:  "secrets.write",
			environment: "prod",
			wantAllow:   false,
		},
		{
			name:        "http.get in prod - allowed",
			capability:  "http.get",
			environment: "prod",
			wantAllow:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &ModulePolicyContext{
				Module: &ModuleInfo{
					Name:    "test/module",
					Version: "1.0.0",
				},
				TrustLevel:  TrustLevelVerified,
				Environment: tc.environment,
			}

			allowed, err := engine.ValidateCapability(ctx, tc.capability)
			if err != nil {
				t.Fatalf("ValidateCapability error = %v", err)
			}
			if allowed != tc.wantAllow {
				t.Errorf("ValidateCapability(%s, %s) = %v, want %v",
					tc.capability, tc.environment, allowed, tc.wantAllow)
			}
		})
	}
}

func TestValidateCapabilities(t *testing.T) {
	config := &CapabilityPolicyConfig{
		AllowByDefault:      false, // Deny by default
		BlockedCapabilities: []string{"dangerous"},
		TrustLevelRequirements: map[string]TrustLevel{
			"exec": TrustLevelVerified,
		},
	}

	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), config)

	ctx := &ModulePolicyContext{
		Module: &ModuleInfo{
			Name:    "test/module",
			Version: "1.0.0",
		},
		TrustLevel:  TrustLevelCommunity,
		Environment: "dev",
		Capabilities: []string{
			"http.get",  // Should be denied (default deny)
			"exec",      // Should be denied (insufficient trust)
			"dangerous", // Should be denied (blocked)
		},
	}

	result, err := engine.ValidateCapabilities(ctx, ctx.Capabilities)
	if err != nil {
		t.Fatalf("ValidateCapabilities error = %v", err)
	}

	// All should be denied
	if len(result.AllowedCapabilities) != 0 {
		t.Errorf("AllowedCapabilities count = %d, want 0", len(result.AllowedCapabilities))
	}

	if len(result.DeniedCapabilities) != 3 {
		t.Errorf("DeniedCapabilities count = %d, want 3", len(result.DeniedCapabilities))
	}

	if len(result.Violations) != 3 {
		t.Errorf("Violations count = %d, want 3", len(result.Violations))
	}
}

func TestValidateModule(t *testing.T) {
	config := &CapabilityPolicyConfig{
		AllowByDefault: true,
		TrustLevelRequirements: map[string]TrustLevel{
			"exec": TrustLevelVerified,
		},
	}

	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), config)

	ctx := &ModulePolicyContext{
		Module: &ModuleInfo{
			Name:    "test/module",
			Version: "1.0.0",
		},
		TrustLevel:   TrustLevelVerified,
		Environment:  "dev",
		Capabilities: []string{"exec", "http.get"},
		Timestamp:    time.Now(),
	}

	result, err := engine.ValidateModule(ctx)
	if err != nil {
		t.Fatalf("ValidateModule error = %v", err)
	}

	if !result.Allowed {
		t.Errorf("Module should be allowed, reason: %s", result.Reason)
	}

	if len(result.AllowedCapabilities) != 2 {
		t.Errorf("AllowedCapabilities count = %d, want 2", len(result.AllowedCapabilities))
	}

	if len(result.DeniedCapabilities) != 0 {
		t.Errorf("DeniedCapabilities count = %d, want 0", len(result.DeniedCapabilities))
	}

	if result.EvaluationTime == 0 {
		t.Error("EvaluationTime should be recorded")
	}
}

func TestValidateModule_EnforcementMode(t *testing.T) {
	config := &CapabilityPolicyConfig{
		AllowByDefault: false,
	}

	testCases := []struct {
		name            string
		enforcementMode policypkg.EnforcementMode
		wantAllowed     bool
		wantWarnings    bool
	}{
		{
			name:            "enforce mode - deny",
			enforcementMode: policypkg.ModeEnforce,
			wantAllowed:     false,
			wantWarnings:    false,
		},
		{
			name:            "audit mode - allow with warning",
			enforcementMode: policypkg.ModeAudit,
			wantAllowed:     true,
			wantWarnings:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			registry := policypkg.NewRegistry()
			engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), config)
			engine.EnforcementMode = tc.enforcementMode

			ctx := &ModulePolicyContext{
				Module: &ModuleInfo{
					Name:    "test/module",
					Version: "1.0.0",
				},
				TrustLevel:   TrustLevelCommunity,
				Environment:  "dev",
				Capabilities: []string{"exec"}, // Will be denied
			}

			result, err := engine.ValidateModule(ctx)
			if err != nil {
				t.Fatalf("ValidateModule error = %v", err)
			}

			if result.Allowed != tc.wantAllowed {
				t.Errorf("Allowed = %v, want %v", result.Allowed, tc.wantAllowed)
			}

			if tc.wantWarnings && len(result.Warnings) == 0 {
				t.Error("Expected warnings in audit mode")
			}
		})
	}
}

func TestAddRule(t *testing.T) {
	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), nil)

	rule1 := &ModulePolicyRule{
		ID:       "rule1",
		Name:     "Test Rule 1",
		Enabled:  true,
		Priority: 10,
		Conditions: PolicyCondition{
			ModuleNamePattern: "test/*",
		},
		Action: PolicyAction{
			Type: ActionWarn,
			Warn: "Test warning",
		},
	}

	rule2 := &ModulePolicyRule{
		ID:       "rule2",
		Name:     "Test Rule 2",
		Enabled:  true,
		Priority: 20, // Higher priority
		Conditions: PolicyCondition{
			ModuleNamePattern: "test/*",
		},
		Action: PolicyAction{
			Type: ActionAllow,
		},
	}

	engine.AddRule(rule1)
	engine.AddRule(rule2)

	if len(engine.Rules) != 2 {
		t.Errorf("Rules count = %d, want 2", len(engine.Rules))
	}

	// Rules should be sorted by priority (higher first)
	if engine.Rules[0].ID != "rule2" {
		t.Error("Rules not sorted by priority correctly")
	}
}

func TestRemoveRule(t *testing.T) {
	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), nil)

	rule := &ModulePolicyRule{
		ID:      "rule1",
		Name:    "Test Rule",
		Enabled: true,
	}

	engine.AddRule(rule)

	if len(engine.Rules) != 1 {
		t.Fatalf("Rules count = %d, want 1", len(engine.Rules))
	}

	engine.RemoveRule("rule1")

	if len(engine.Rules) != 0 {
		t.Errorf("Rules count after removal = %d, want 0", len(engine.Rules))
	}
}

func TestRuleMatches_ModuleNamePattern(t *testing.T) {
	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), nil)

	rule := &ModulePolicyRule{
		Conditions: PolicyCondition{
			ModuleNamePattern: "test/*",
		},
	}

	testCases := []struct {
		moduleName string
		wantMatch  bool
	}{
		{"test/module", true},
		{"test/another", true},
		{"prod/module", false},
		{"test", false},
	}

	for _, tc := range testCases {
		t.Run(tc.moduleName, func(t *testing.T) {
			ctx := &ModulePolicyContext{
				Module: &ModuleInfo{
					Name: tc.moduleName,
				},
			}

			matches := engine.ruleMatches(rule, ctx)
			if matches != tc.wantMatch {
				t.Errorf("ruleMatches(%s) = %v, want %v", tc.moduleName, matches, tc.wantMatch)
			}
		})
	}
}

func TestRuleMatches_TrustLevel(t *testing.T) {
	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), nil)

	testCases := []struct {
		name        string
		minTrust    TrustLevel
		maxTrust    TrustLevel
		actualTrust TrustLevel
		wantMatch   bool
	}{
		{
			name:        "meets minimum",
			minTrust:    TrustLevelCommunity,
			actualTrust: TrustLevelVerified,
			wantMatch:   true,
		},
		{
			name:        "below minimum",
			minTrust:    TrustLevelVerified,
			actualTrust: TrustLevelCommunity,
			wantMatch:   false,
		},
		{
			name:        "within max",
			maxTrust:    TrustLevelVerified,
			actualTrust: TrustLevelCommunity,
			wantMatch:   true,
		},
		{
			name:        "above max",
			maxTrust:    TrustLevelCommunity,
			actualTrust: TrustLevelVerified,
			wantMatch:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rule := &ModulePolicyRule{
				Conditions: PolicyCondition{
					MinTrustLevel: tc.minTrust,
					MaxTrustLevel: tc.maxTrust,
				},
			}

			ctx := &ModulePolicyContext{
				Module: &ModuleInfo{
					Name: "test/module",
				},
				TrustLevel: tc.actualTrust,
			}

			matches := engine.ruleMatches(rule, ctx)
			if matches != tc.wantMatch {
				t.Errorf("ruleMatches = %v, want %v", matches, tc.wantMatch)
			}
		})
	}
}

func TestRuleMatches_Environment(t *testing.T) {
	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), nil)

	rule := &ModulePolicyRule{
		Conditions: PolicyCondition{
			Environments: []string{"prod", "staging"},
		},
	}

	testCases := []struct {
		environment string
		wantMatch   bool
	}{
		{"prod", true},
		{"staging", true},
		{"dev", false},
	}

	for _, tc := range testCases {
		t.Run(tc.environment, func(t *testing.T) {
			ctx := &ModulePolicyContext{
				Module: &ModuleInfo{
					Name: "test/module",
				},
				Environment: tc.environment,
			}

			matches := engine.ruleMatches(rule, ctx)
			if matches != tc.wantMatch {
				t.Errorf("ruleMatches(%s) = %v, want %v", tc.environment, matches, tc.wantMatch)
			}
		})
	}
}

func TestRuleMatches_Capabilities(t *testing.T) {
	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), nil)

	testCases := []struct {
		name      string
		required  []string
		forbidden []string
		actual    []string
		wantMatch bool
	}{
		{
			name:      "has required",
			required:  []string{"exec"},
			actual:    []string{"exec", "http.get"},
			wantMatch: true,
		},
		{
			name:      "missing required",
			required:  []string{"exec", "secrets.write"},
			actual:    []string{"exec"},
			wantMatch: false,
		},
		{
			name:      "no forbidden",
			forbidden: []string{"secrets.write"},
			actual:    []string{"exec", "http.get"},
			wantMatch: true,
		},
		{
			name:      "has forbidden",
			forbidden: []string{"secrets.write"},
			actual:    []string{"exec", "secrets.write"},
			wantMatch: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rule := &ModulePolicyRule{
				Conditions: PolicyCondition{
					RequiredCapabilities:  tc.required,
					ForbiddenCapabilities: tc.forbidden,
				},
			}

			ctx := &ModulePolicyContext{
				Module: &ModuleInfo{
					Name: "test/module",
				},
				Capabilities: tc.actual,
			}

			matches := engine.ruleMatches(rule, ctx)
			if matches != tc.wantMatch {
				t.Errorf("ruleMatches = %v, want %v", matches, tc.wantMatch)
			}
		})
	}
}

func TestApplyRuleAction_Deny(t *testing.T) {
	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), nil)

	rule := &ModulePolicyRule{
		Name: "Deny Rule",
		Action: PolicyAction{
			Type:        ActionDeny,
			BlockReason: "Test deny",
		},
	}

	result := &ModulePolicyResult{
		Allowed: true,
	}

	engine.applyRuleAction(rule, result)

	if result.Allowed {
		t.Error("Result should be denied")
	}

	if result.Reason != "Test deny" {
		t.Errorf("Reason = %s, want 'Test deny'", result.Reason)
	}
}

func TestApplyRuleAction_Warn(t *testing.T) {
	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), nil)

	rule := &ModulePolicyRule{
		Name: "Warn Rule",
		Action: PolicyAction{
			Type: ActionWarn,
			Warn: "Test warning",
		},
	}

	result := &ModulePolicyResult{
		Allowed:  true,
		Warnings: []string{},
	}

	engine.applyRuleAction(rule, result)

	if !result.Allowed {
		t.Error("Result should still be allowed")
	}

	if len(result.Warnings) != 1 {
		t.Errorf("Warnings count = %d, want 1", len(result.Warnings))
	}

	if result.Warnings[0] != "Test warning" {
		t.Errorf("Warning = %s, want 'Test warning'", result.Warnings[0])
	}
}

func TestApplyRuleAction_Modify(t *testing.T) {
	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), nil)

	rule := &ModulePolicyRule{
		Name: "Modify Rule",
		Action: PolicyAction{
			Type:              ActionModify,
			AllowCapabilities: []string{"http.get", "log"},
			DenyCapabilities:  []string{"exec"},
		},
	}

	result := &ModulePolicyResult{
		Allowed:             true,
		AllowedCapabilities: []string{},
		DeniedCapabilities:  []string{},
	}

	engine.applyRuleAction(rule, result)

	if !result.Allowed {
		t.Error("Result should still be allowed")
	}

	if len(result.AllowedCapabilities) != 2 {
		t.Errorf("AllowedCapabilities count = %d, want 2", len(result.AllowedCapabilities))
	}

	if len(result.DeniedCapabilities) != 1 {
		t.Errorf("DeniedCapabilities count = %d, want 1", len(result.DeniedCapabilities))
	}
}

func TestApplyRuleAction_Block(t *testing.T) {
	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), nil)

	rule := &ModulePolicyRule{
		Name: "Block Rule",
		Action: PolicyAction{
			Block:       true,
			BlockReason: "Custom block reason",
		},
	}

	result := &ModulePolicyResult{
		Allowed: true,
	}

	engine.applyRuleAction(rule, result)

	if result.Allowed {
		t.Error("Result should be blocked")
	}

	if result.Reason != "Custom block reason" {
		t.Errorf("Reason = %s, want 'Custom block reason'", result.Reason)
	}
}

func TestMeetsMinimumTrust(t *testing.T) {
	testCases := []struct {
		actual  TrustLevel
		minimum TrustLevel
		want    bool
	}{
		{TrustLevelUnknown, TrustLevelUnknown, true},
		{TrustLevelUntrusted, TrustLevelUnknown, true},
		{TrustLevelCommunity, TrustLevelUntrusted, true},
		{TrustLevelVerified, TrustLevelCommunity, true},
		{TrustLevelInternal, TrustLevelVerified, true},
		{TrustLevelSystem, TrustLevelInternal, true},
		{TrustLevelCommunity, TrustLevelVerified, false},
		{TrustLevelUntrusted, TrustLevelCommunity, false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.actual)+"_vs_"+string(tc.minimum), func(t *testing.T) {
			result := meetsMinimumTrust(tc.actual, tc.minimum)
			if result != tc.want {
				t.Errorf("meetsMinimumTrust(%s, %s) = %v, want %v",
					tc.actual, tc.minimum, result, tc.want)
			}
		})
	}
}

func TestMeetsMaximumTrust(t *testing.T) {
	testCases := []struct {
		actual  TrustLevel
		maximum TrustLevel
		want    bool
	}{
		{TrustLevelUnknown, TrustLevelSystem, true},
		{TrustLevelCommunity, TrustLevelVerified, true},
		{TrustLevelVerified, TrustLevelVerified, true},
		{TrustLevelSystem, TrustLevelInternal, false},
		{TrustLevelInternal, TrustLevelVerified, false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.actual)+"_vs_"+string(tc.maximum), func(t *testing.T) {
			result := meetsMaximumTrust(tc.actual, tc.maximum)
			if result != tc.want {
				t.Errorf("meetsMaximumTrust(%s, %s) = %v, want %v",
					tc.actual, tc.maximum, result, tc.want)
			}
		})
	}
}

func TestValidateModule_WithCustomRules(t *testing.T) {
	config := &CapabilityPolicyConfig{
		AllowByDefault: true,
	}

	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), config)

	// Add a rule that denies modules with "dangerous" in the name
	engine.AddRule(&ModulePolicyRule{
		ID:       "deny-dangerous",
		Name:     "Deny Dangerous Modules",
		Enabled:  true,
		Priority: 100,
		Conditions: PolicyCondition{
			ModuleNamePattern: "*/*dangerous*",
		},
		Action: PolicyAction{
			Type:        ActionDeny,
			BlockReason: "Module name contains 'dangerous'",
		},
	})

	ctx := &ModulePolicyContext{
		Module: &ModuleInfo{
			Name:    "test/dangerous-module",
			Version: "1.0.0",
		},
		TrustLevel:   TrustLevelVerified,
		Environment:  "dev",
		Capabilities: []string{"http.get"},
	}

	result, err := engine.ValidateModule(ctx)
	if err != nil {
		t.Fatalf("ValidateModule error = %v", err)
	}

	if result.Allowed {
		t.Error("Dangerous module should be denied by custom rule")
	}

	if result.Reason != "Module name contains 'dangerous'" {
		t.Errorf("Reason = %s, want 'Module name contains 'dangerous''", result.Reason)
	}
}

func TestValidateModule_DisabledRule(t *testing.T) {
	config := &CapabilityPolicyConfig{
		AllowByDefault: true,
	}

	registry := policypkg.NewRegistry()
	engine := NewModulePolicyEngine(policypkg.NewPolicyEngine(registry), config)

	// Add a disabled rule
	engine.AddRule(&ModulePolicyRule{
		ID:       "deny-all",
		Name:     "Deny All",
		Enabled:  false, // Disabled
		Priority: 100,
		Conditions: PolicyCondition{
			ModuleNamePattern: "*",
		},
		Action: PolicyAction{
			Type:        ActionDeny,
			BlockReason: "Should not apply",
		},
	})

	ctx := &ModulePolicyContext{
		Module: &ModuleInfo{
			Name:    "test/module",
			Version: "1.0.0",
		},
		TrustLevel:   TrustLevelVerified,
		Environment:  "dev",
		Capabilities: []string{"http.get"},
	}

	result, err := engine.ValidateModule(ctx)
	if err != nil {
		t.Fatalf("ValidateModule error = %v", err)
	}

	if !result.Allowed {
		t.Error("Module should be allowed (disabled rule should not apply)")
	}
}
