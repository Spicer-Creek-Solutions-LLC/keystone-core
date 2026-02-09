package versioning

import (
	"testing"
	"time"
)

func TestVersionInfo_IsUsable(t *testing.T) {
	tests := []struct {
		name     string
		state    VersionState
		expected bool
	}{
		{"stable", VersionStateStable, true},
		{"prerelease", VersionStatePrerelease, true},
		{"deprecated", VersionStateDeprecated, true},
		{"yanked", VersionStateYanked, false},
		{"retracted", VersionStateRetracted, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &VersionInfo{State: tt.state}
			if got := v.IsUsable(); got != tt.expected {
				t.Errorf("IsUsable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestVersionInfo_IsDeprecated(t *testing.T) {
	tests := []struct {
		name        string
		state       VersionState
		deprecation *DeprecationInfo
		expected    bool
	}{
		{"stable", VersionStateStable, nil, false},
		{"deprecated state", VersionStateDeprecated, nil, true},
		{"with deprecation info", VersionStateStable, &DeprecationInfo{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &VersionInfo{State: tt.state, Deprecation: tt.deprecation}
			if got := v.IsDeprecated(); got != tt.expected {
				t.Errorf("IsDeprecated() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestVersionInfo_IsPrerelease(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		state    VersionState
		expected bool
	}{
		{"stable version", "1.0.0", VersionStateStable, false},
		{"alpha version", "1.0.0-alpha.1", VersionStateStable, true},
		{"beta version", "1.0.0-beta", VersionStateStable, true},
		{"rc version", "1.0.0-rc.1", VersionStateStable, true},
		{"prerelease state", "1.0.0", VersionStatePrerelease, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &VersionInfo{Version: tt.version, State: tt.state}
			if got := v.IsPrerelease(); got != tt.expected {
				t.Errorf("IsPrerelease() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestVersionInfo_IsSunset(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	future := time.Now().Add(24 * time.Hour)

	tests := []struct {
		name        string
		deprecation *DeprecationInfo
		expected    bool
	}{
		{"no deprecation", nil, false},
		{"no sunset date", &DeprecationInfo{}, false},
		{"future sunset", &DeprecationInfo{SunsetDate: &future}, false},
		{"past sunset", &DeprecationInfo{SunsetDate: &past}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &VersionInfo{Deprecation: tt.deprecation}
			if got := v.IsSunset(); got != tt.expected {
				t.Errorf("IsSunset() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestVersionInfo_GetRecommendedReplacement(t *testing.T) {
	tests := []struct {
		name            string
		deprecation     *DeprecationInfo
		retraction      *RetractionInfo
		expectedModule  string
		expectedVersion string
	}{
		{
			name:            "no deprecation or retraction",
			expectedModule:  "",
			expectedVersion: "",
		},
		{
			name: "deprecation with replacement version",
			deprecation: &DeprecationInfo{
				ReplacementVersion: "2.0.0",
			},
			expectedModule:  "test/module",
			expectedVersion: "2.0.0",
		},
		{
			name: "deprecation with replacement module",
			deprecation: &DeprecationInfo{
				ReplacementModule:  "other/module",
				ReplacementVersion: "1.0.0",
			},
			expectedModule:  "other/module",
			expectedVersion: "1.0.0",
		},
		{
			name: "retraction with replacement",
			retraction: &RetractionInfo{
				ReplacementVersion: "1.5.0",
			},
			expectedModule:  "test/module",
			expectedVersion: "1.5.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &VersionInfo{
				Module:      "test/module",
				Deprecation: tt.deprecation,
				Retraction:  tt.retraction,
			}
			gotModule, gotVersion := v.GetRecommendedReplacement()
			if gotModule != tt.expectedModule || gotVersion != tt.expectedVersion {
				t.Errorf("GetRecommendedReplacement() = (%v, %v), want (%v, %v)",
					gotModule, gotVersion, tt.expectedModule, tt.expectedVersion)
			}
		})
	}
}

func TestVersionInfo_Deprecate(t *testing.T) {
	v := NewVersionInfo("test/module", "1.0.0")

	sunset := time.Now().Add(30 * 24 * time.Hour)
	v.Deprecate(
		"Use version 2.0.0 for improved performance",
		WithReplacement("2.0.0"),
		WithSunsetDate(sunset),
		WithSeverity(DeprecationSeverityHigh),
		WithMigrationGuide("https://docs.example.com/migrate"),
		WithBreakingChanges("API changes", "Configuration format changed"),
	)

	if v.State != VersionStateDeprecated {
		t.Errorf("State = %v, want %v", v.State, VersionStateDeprecated)
	}
	if v.Deprecation == nil {
		t.Fatal("Deprecation is nil")
	}
	if v.Deprecation.ReplacementVersion != "2.0.0" {
		t.Errorf("ReplacementVersion = %v, want 2.0.0", v.Deprecation.ReplacementVersion)
	}
	if v.Deprecation.Severity != DeprecationSeverityHigh {
		t.Errorf("Severity = %v, want %v", v.Deprecation.Severity, DeprecationSeverityHigh)
	}
	if len(v.Deprecation.BreakingChanges) != 2 {
		t.Errorf("BreakingChanges count = %d, want 2", len(v.Deprecation.BreakingChanges))
	}
}

func TestVersionInfo_Yank(t *testing.T) {
	v := NewVersionInfo("test/module", "1.0.0")
	v.Yank("Critical bug discovered")

	if v.State != VersionStateYanked {
		t.Errorf("State = %v, want %v", v.State, VersionStateYanked)
	}
	if v.Retraction == nil {
		t.Fatal("Retraction is nil")
	}
	if v.Retraction.Reason != "Critical bug discovered" {
		t.Errorf("Reason = %v, want 'Critical bug discovered'", v.Retraction.Reason)
	}
}

func TestVersionInfo_Retract(t *testing.T) {
	v := NewVersionInfo("test/module", "1.0.0")
	v.Retract("Security vulnerability", "CVE-2024-1234")

	if v.State != VersionStateRetracted {
		t.Errorf("State = %v, want %v", v.State, VersionStateRetracted)
	}
	if v.Retraction == nil {
		t.Fatal("Retraction is nil")
	}
	if v.Retraction.CVE != "CVE-2024-1234" {
		t.Errorf("CVE = %v, want CVE-2024-1234", v.Retraction.CVE)
	}
}

func TestVersionInfo_WarningMessage(t *testing.T) {
	t.Run("yanked version", func(t *testing.T) {
		v := NewVersionInfo("test/module", "1.0.0")
		v.Yank("Critical bug")
		msg := v.WarningMessage()
		if msg == "" {
			t.Error("WarningMessage() returned empty string")
		}
	})

	t.Run("deprecated version", func(t *testing.T) {
		v := NewVersionInfo("test/module", "1.0.0")
		v.Deprecate("Old version", WithReplacement("2.0.0"))
		msg := v.WarningMessage()
		if msg == "" {
			t.Error("WarningMessage() returned empty string")
		}
	})

	t.Run("security issues", func(t *testing.T) {
		v := NewVersionInfo("test/module", "1.0.0")
		v.AddSecurityAdvisory(SecurityAdvisory{
			ID:       "CVE-2024-1234",
			Severity: "high",
			Title:    "Buffer overflow",
		})
		msg := v.WarningMessage()
		if msg == "" {
			t.Error("WarningMessage() returned empty string")
		}
	})
}

func TestPolicyChecker_Check(t *testing.T) {
	t.Run("default policy allows stable versions", func(t *testing.T) {
		checker := NewPolicyChecker(DefaultPolicy())
		v := NewVersionInfo("test/module", "1.0.0")
		result := checker.Check(v)
		if !result.Allowed {
			t.Errorf("Stable version should be allowed: %v", result.Violations)
		}
	})

	t.Run("default policy blocks yanked versions", func(t *testing.T) {
		checker := NewPolicyChecker(DefaultPolicy())
		v := NewVersionInfo("test/module", "1.0.0")
		v.Yank("Critical bug")
		result := checker.Check(v)
		if result.Allowed {
			t.Error("Yanked version should be blocked")
		}
		if len(result.Violations) == 0 {
			t.Error("Expected violations for yanked version")
		}
	})

	t.Run("default policy warns on deprecated", func(t *testing.T) {
		checker := NewPolicyChecker(DefaultPolicy())
		v := NewVersionInfo("test/module", "1.0.0")
		v.Deprecate("Old version")
		result := checker.Check(v)
		if !result.Allowed {
			t.Errorf("Deprecated version should be allowed by default policy: %v", result.Violations)
		}
		if len(result.Warnings) == 0 {
			t.Error("Expected warnings for deprecated version")
		}
	})

	t.Run("strict policy blocks deprecated", func(t *testing.T) {
		checker := NewPolicyChecker(StrictPolicy())
		v := NewVersionInfo("test/module", "1.0.0")
		v.Deprecate("Old version")
		result := checker.Check(v)
		if result.Allowed {
			t.Error("Strict policy should block deprecated versions")
		}
	})

	t.Run("default policy blocks prerelease", func(t *testing.T) {
		checker := NewPolicyChecker(DefaultPolicy())
		v := NewVersionInfo("test/module", "1.0.0-alpha.1")
		result := checker.Check(v)
		if result.Allowed {
			t.Error("Default policy should block prerelease versions")
		}
	})

	t.Run("permissive policy allows prerelease", func(t *testing.T) {
		checker := NewPolicyChecker(PermissivePolicy())
		v := NewVersionInfo("test/module", "1.0.0-alpha.1")
		result := checker.Check(v)
		if !result.Allowed {
			t.Errorf("Permissive policy should allow prerelease: %v", result.Violations)
		}
	})

	t.Run("enforces sunset dates", func(t *testing.T) {
		checker := NewPolicyChecker(DefaultPolicy())
		past := time.Now().Add(-24 * time.Hour)
		v := NewVersionInfo("test/module", "1.0.0")
		v.Deprecate("Old version", WithSunsetDate(past))
		result := checker.Check(v)
		if result.Allowed {
			t.Error("Should block versions past sunset date")
		}
	})

	t.Run("blocks retracted versions", func(t *testing.T) {
		checker := NewPolicyChecker(PermissivePolicy())
		v := NewVersionInfo("test/module", "1.0.0")
		v.Retract("Security issue", "CVE-2024-1234")
		result := checker.Check(v)
		if result.Allowed {
			t.Error("Should always block retracted versions")
		}
	})

	t.Run("blocks modules with security vulnerabilities by default", func(t *testing.T) {
		checker := NewPolicyChecker(DefaultPolicy())
		v := NewVersionInfo("test/module", "1.0.0")
		v.AddSecurityAdvisory(SecurityAdvisory{
			ID:       "CVE-2024-1234",
			Severity: "high",
			Title:    "Buffer overflow",
		})
		result := checker.Check(v)
		if result.Allowed {
			t.Error("Should block versions with security vulnerabilities by default")
		}
	})
}

func TestPolicyChecker_BlockedModules(t *testing.T) {
	policy := DefaultPolicy()
	policy.BlockedModules = []string{"blocked/*", "other/bad"}

	checker := NewPolicyChecker(policy)

	tests := []struct {
		module  string
		allowed bool
	}{
		{"blocked/module", false},
		{"other/bad", false},
		{"allowed/module", true},
	}

	for _, tt := range tests {
		t.Run(tt.module, func(t *testing.T) {
			v := NewVersionInfo(tt.module, "1.0.0")
			result := checker.Check(v)
			if result.Allowed != tt.allowed {
				t.Errorf("Module %s: Allowed = %v, want %v", tt.module, result.Allowed, tt.allowed)
			}
		})
	}
}

func TestPolicyChecker_AllowedModules(t *testing.T) {
	policy := DefaultPolicy()
	policy.AllowedModules = []string{"allowed/*", "specific/module"}

	checker := NewPolicyChecker(policy)

	tests := []struct {
		module  string
		allowed bool
	}{
		{"allowed/module", true},
		{"specific/module", true},
		{"other/module", false},
	}

	for _, tt := range tests {
		t.Run(tt.module, func(t *testing.T) {
			v := NewVersionInfo(tt.module, "1.0.0")
			result := checker.Check(v)
			if result.Allowed != tt.allowed {
				t.Errorf("Module %s: Allowed = %v, want %v", tt.module, result.Allowed, tt.allowed)
			}
		})
	}
}

func TestPolicyChecker_CustomRules(t *testing.T) {
	policy := DefaultPolicy()
	policy.CustomRules = []PolicyRule{
		{
			Name:           "block-old-major",
			ModulePattern:  "*",
			VersionPattern: "0.*",
			Action:         PolicyActionDeny,
			Message:        "v0.x versions are not allowed",
		},
		{
			Name:           "warn-beta",
			VersionPattern: "*-beta*",
			Action:         PolicyActionWarn,
			Message:        "Beta versions should be used with caution",
		},
	}
	policy.AllowPrerelease = true // Allow prerelease for this test

	checker := NewPolicyChecker(policy)

	t.Run("blocks v0.x versions", func(t *testing.T) {
		v := NewVersionInfo("test/module", "0.9.0")
		result := checker.Check(v)
		if result.Allowed {
			t.Error("Should block v0.x versions")
		}
	})

	t.Run("warns on beta versions", func(t *testing.T) {
		v := NewVersionInfo("test/module", "1.0.0-beta.1")
		result := checker.Check(v)
		if !result.Allowed {
			t.Errorf("Should allow beta versions: %v", result.Violations)
		}
		if len(result.Warnings) == 0 {
			t.Error("Should warn on beta versions")
		}
	})
}

func TestPolicyChecker_FilterVersions(t *testing.T) {
	checker := NewPolicyChecker(DefaultPolicy())

	versions := []*VersionInfo{
		NewVersionInfo("test/module", "1.0.0"),
		NewVersionInfo("test/module", "1.1.0"),
		NewVersionInfo("test/module", "2.0.0-alpha.1"),
	}
	versions[1].Yank("Critical bug")

	allowed, violations := checker.FilterVersions(versions)

	// Should filter out yanked and prerelease
	if len(allowed) != 1 {
		t.Errorf("Expected 1 allowed version, got %d", len(allowed))
	}
	if allowed[0].Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", allowed[0].Version)
	}
	if len(violations) != 2 {
		t.Errorf("Expected 2 violations, got %d", len(violations))
	}
}

func TestPolicyChecker_SelectBestVersion(t *testing.T) {
	checker := NewPolicyChecker(DefaultPolicy())

	t.Run("prefers non-deprecated", func(t *testing.T) {
		versions := []*VersionInfo{
			NewVersionInfo("test/module", "1.0.0"),
			NewVersionInfo("test/module", "2.0.0"),
		}
		versions[0].Deprecate("Old version")

		best, _ := checker.SelectBestVersion(versions)
		if best == nil {
			t.Fatal("Expected a best version")
		}
		if best.Version != "2.0.0" {
			t.Errorf("Expected 2.0.0, got %s", best.Version)
		}
	})

	t.Run("returns nil for no allowed versions", func(t *testing.T) {
		versions := []*VersionInfo{
			NewVersionInfo("test/module", "1.0.0"),
		}
		versions[0].Yank("Critical bug")

		best, _ := checker.SelectBestVersion(versions)
		if best != nil {
			t.Error("Expected nil for no allowed versions")
		}
	})
}

func TestDeprecationSeverityOrder(t *testing.T) {
	policy := DefaultPolicy()
	policy.MaxDeprecationSeverity = DeprecationSeverityMedium

	checker := NewPolicyChecker(policy)

	tests := []struct {
		severity DeprecationSeverity
		allowed  bool
	}{
		{DeprecationSeverityLow, true},
		{DeprecationSeverityMedium, true},
		{DeprecationSeverityHigh, false},
		{DeprecationSeverityCritical, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			v := NewVersionInfo("test/module", "1.0.0")
			v.Deprecate("Old version", WithSeverity(tt.severity))
			result := checker.Check(v)
			if result.Allowed != tt.allowed {
				t.Errorf("Severity %s: Allowed = %v, want %v", tt.severity, result.Allowed, tt.allowed)
			}
		})
	}
}

func TestGenerateDeprecationReport(t *testing.T) {
	versions := []*VersionInfo{
		NewVersionInfo("mod1", "1.0.0"),
		NewVersionInfo("mod2", "1.0.0"),
		NewVersionInfo("mod3", "1.0.0"),
	}
	versions[0].Deprecate("Old version", WithReplacement("2.0.0"))
	versions[1].AddSecurityAdvisory(SecurityAdvisory{
		ID:       "CVE-2024-1234",
		Severity: "high",
		Title:    "Security issue",
	})

	report := GenerateDeprecationReport(versions)

	if report.TotalModules != 3 {
		t.Errorf("TotalModules = %d, want 3", report.TotalModules)
	}
	if report.DeprecatedCount != 1 {
		t.Errorf("DeprecatedCount = %d, want 1", report.DeprecatedCount)
	}
	if report.SecurityIssueCount != 1 {
		t.Errorf("SecurityIssueCount = %d, want 1", report.SecurityIssueCount)
	}
	if len(report.UpgradeSuggestions) != 1 {
		t.Errorf("UpgradeSuggestions count = %d, want 1", len(report.UpgradeSuggestions))
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		s        string
		expected bool
	}{
		{"*", "anything", true},
		{"test/*", "test/module", true},
		{"test/*", "other/module", false},
		{"*/module", "test/module", true},
		{"*/module", "test/other", false},
		{"*test*", "my-test-module", true},
		{"exact", "exact", true},
		{"exact", "other", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.s, func(t *testing.T) {
			if got := matchesPattern(tt.pattern, tt.s); got != tt.expected {
				t.Errorf("matchesPattern(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.expected)
			}
		})
	}
}

func TestNewVersionInfo(t *testing.T) {
	t.Run("stable version", func(t *testing.T) {
		v := NewVersionInfo("test/module", "1.0.0")
		if v.State != VersionStateStable {
			t.Errorf("State = %v, want %v", v.State, VersionStateStable)
		}
	})

	t.Run("prerelease version", func(t *testing.T) {
		v := NewVersionInfo("test/module", "1.0.0-alpha.1")
		if v.State != VersionStatePrerelease {
			t.Errorf("State = %v, want %v", v.State, VersionStatePrerelease)
		}
	})
}
