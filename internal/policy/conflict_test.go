package policy

import (
	"testing"
)

func TestDefaultConflictDetectorConfig(t *testing.T) {
	config := DefaultConflictDetectorConfig()

	if !config.EnableOverlapDetection {
		t.Error("Expected overlap detection to be enabled")
	}
	if !config.EnableContradictionDetection {
		t.Error("Expected contradiction detection to be enabled")
	}
	if !config.EnableDuplicateDetection {
		t.Error("Expected duplicate detection to be enabled")
	}
	if config.SeverityThreshold != ConflictInfo {
		t.Errorf("Expected threshold Info, got %s", config.SeverityThreshold)
	}
}

func TestNewConflictDetector(t *testing.T) {
	registry := NewRegistry()
	detector := NewConflictDetector(registry, nil)

	if detector == nil {
		t.Fatal("Expected detector to be created")
	}
	if detector.registry != registry {
		t.Error("Expected registry to be set")
	}
}

func TestConflictDetector_DetectOverlap(t *testing.T) {
	registry := NewRegistry()

	// Add policies with same category but different enforcement modes
	registry.RegisterPolicy(&Policy{
		ID:              "policy-1",
		Name:            "Policy 1",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		EnforcementMode: ModeEnforce,
	})

	registry.RegisterPolicy(&Policy{
		ID:              "policy-2",
		Name:            "Policy 2",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'write'",
		Category:        CategorySecurity,
		EnforcementMode: ModeAudit,
	})

	detector := NewConflictDetector(registry, nil)
	conflicts := detector.DetectAll()

	if len(conflicts) == 0 {
		t.Error("Expected overlap conflict to be detected")
	}

	found := false
	for _, c := range conflicts {
		if c.Type == ConflictTypeOverlap {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected overlap conflict type")
	}
}

func TestConflictDetector_DetectContradiction(t *testing.T) {
	registry := NewRegistry()

	// Add policies with same category but significantly different severities
	registry.RegisterPolicy(&Policy{
		ID:              "policy-1",
		Name:            "Policy 1",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		Severity:        SeverityLow,
		EnforcementMode: ModeEnforce,
	})

	registry.RegisterPolicy(&Policy{
		ID:              "policy-2",
		Name:            "Policy 2",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'write'",
		Category:        CategorySecurity,
		Severity:        SeverityCritical,
		EnforcementMode: ModeEnforce,
	})

	detector := NewConflictDetector(registry, nil)
	conflicts := detector.DetectAll()

	found := false
	for _, c := range conflicts {
		if c.Type == ConflictTypeContradiction {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected contradiction conflict to be detected")
	}
}

func TestConflictDetector_DetectDuplicate(t *testing.T) {
	registry := NewRegistry()

	// Add very similar policies
	registry.RegisterPolicy(&Policy{
		ID:              "policy-1",
		Name:            "Policy 1",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read' && user == 'admin'",
		Category:        CategorySecurity,
		Severity:        SeverityMedium,
		EnforcementMode: ModeEnforce,
	})

	registry.RegisterPolicy(&Policy{
		ID:              "policy-2",
		Name:            "Policy 2",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read' && user == 'admin'",
		Category:        CategorySecurity,
		Severity:        SeverityMedium,
		EnforcementMode: ModeEnforce,
	})

	detector := NewConflictDetector(registry, nil)
	conflicts := detector.DetectAll()

	found := false
	for _, c := range conflicts {
		if c.Type == ConflictTypeDuplicate {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected duplicate conflict to be detected")
	}
}

func TestConflictDetector_NoConflicts(t *testing.T) {
	registry := NewRegistry()

	// Add policies with different categories
	registry.RegisterPolicy(&Policy{
		ID:              "policy-1",
		Name:            "Policy 1",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		EnforcementMode: ModeEnforce,
	})

	registry.RegisterPolicy(&Policy{
		ID:              "policy-2",
		Name:            "Policy 2",
		Type:            PolicyTypeOPA,
		Policy:          "package test\ndefault allow := false",
		Category:        CategoryCompliance,
		EnforcementMode: ModeEnforce,
	})

	detector := NewConflictDetector(registry, nil)
	conflicts := detector.DetectAll()

	if len(conflicts) != 0 {
		t.Errorf("Expected no conflicts, got %d", len(conflicts))
	}
}

func TestConflictDetector_SeverityThreshold(t *testing.T) {
	registry := NewRegistry()

	// Add policies that would create an info-level conflict
	registry.RegisterPolicy(&Policy{
		ID:              "policy-1",
		Name:            "Policy 1",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		Severity:        SeverityMedium,
		EnforcementMode: ModeEnforce,
	})

	registry.RegisterPolicy(&Policy{
		ID:              "policy-2",
		Name:            "Policy 2",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		Severity:        SeverityMedium,
		EnforcementMode: ModeEnforce,
	})

	// With warning threshold, should not see info-level duplicates
	config := &ConflictDetectorConfig{
		EnableDuplicateDetection: true,
		SeverityThreshold:        ConflictWarning,
	}

	detector := NewConflictDetector(registry, config)
	conflicts := detector.DetectAll()

	// Duplicate detection creates info-level conflicts, should be filtered
	for _, c := range conflicts {
		if c.Type == ConflictTypeDuplicate && c.Severity == ConflictInfo {
			t.Error("Expected info-level conflicts to be filtered out")
		}
	}
}

func TestConflictDetector_GetConflictsByType(t *testing.T) {
	registry := NewRegistry()

	registry.RegisterPolicy(&Policy{
		ID:              "policy-1",
		Name:            "Policy 1",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		EnforcementMode: ModeEnforce,
	})

	registry.RegisterPolicy(&Policy{
		ID:              "policy-2",
		Name:            "Policy 2",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		EnforcementMode: ModeAudit, // Different mode = overlap
	})

	detector := NewConflictDetector(registry, nil)
	detector.DetectAll()

	overlaps := detector.GetConflictsByType(ConflictTypeOverlap)
	if len(overlaps) == 0 {
		t.Error("Expected overlap conflicts")
	}

	duplicates := detector.GetConflictsByType(ConflictTypeDuplicate)
	// May or may not have duplicates depending on threshold
	_ = duplicates
}

func TestConflictDetector_GetConflictsBySeverity(t *testing.T) {
	registry := NewRegistry()

	registry.RegisterPolicy(&Policy{
		ID:              "policy-1",
		Name:            "Policy 1",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		EnforcementMode: ModeEnforce,
	})

	registry.RegisterPolicy(&Policy{
		ID:              "policy-2",
		Name:            "Policy 2",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		EnforcementMode: ModeAudit,
	})

	detector := NewConflictDetector(registry, nil)
	detector.DetectAll()

	warnings := detector.GetConflictsBySeverity(ConflictWarning)
	if len(warnings) == 0 {
		t.Error("Expected warning-level conflicts")
	}
}

func TestConflictDetector_Clear(t *testing.T) {
	registry := NewRegistry()

	registry.RegisterPolicy(&Policy{
		ID:              "policy-1",
		Name:            "Policy 1",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		EnforcementMode: ModeEnforce,
	})

	registry.RegisterPolicy(&Policy{
		ID:              "policy-2",
		Name:            "Policy 2",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		EnforcementMode: ModeAudit,
	})

	detector := NewConflictDetector(registry, nil)
	detector.DetectAll()

	if len(detector.GetConflicts()) == 0 {
		t.Error("Expected conflicts before clear")
	}

	detector.Clear()

	if len(detector.GetConflicts()) != 0 {
		t.Error("Expected no conflicts after clear")
	}
}

func TestConflictDetector_GenerateReport(t *testing.T) {
	registry := NewRegistry()

	registry.RegisterPolicy(&Policy{
		ID:              "policy-1",
		Name:            "Policy 1",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		EnforcementMode: ModeEnforce,
	})

	registry.RegisterPolicy(&Policy{
		ID:              "policy-2",
		Name:            "Policy 2",
		Type:            PolicyTypeCEL,
		Policy:          "action == 'read'",
		Category:        CategorySecurity,
		EnforcementMode: ModeAudit,
	})

	detector := NewConflictDetector(registry, nil)
	detector.DetectAll()

	report := detector.GenerateReport()

	if report.GeneratedAt.IsZero() {
		t.Error("Expected GeneratedAt to be set")
	}
	if report.TotalConflicts == 0 {
		t.Error("Expected conflicts in report")
	}
	if len(report.ConflictsByType) == 0 {
		t.Error("Expected conflicts by type")
	}
	if len(report.ConflictsBySeverity) == 0 {
		t.Error("Expected conflicts by severity")
	}
}

func TestNewConflictResolver(t *testing.T) {
	resolver := NewConflictResolver(StrategyPriority)

	if resolver == nil {
		t.Fatal("Expected resolver to be created")
	}
	if resolver.defaultStrategy != StrategyPriority {
		t.Errorf("Expected default strategy Priority, got %s", resolver.defaultStrategy)
	}
}

func TestConflictResolver_SetGetPriority(t *testing.T) {
	resolver := NewConflictResolver(StrategyPriority)

	resolver.SetPolicyPriority("policy-1", 100)
	resolver.SetPolicyPriority("policy-2", 50)

	if resolver.GetPolicyPriority("policy-1") != 100 {
		t.Error("Expected priority 100 for policy-1")
	}
	if resolver.GetPolicyPriority("policy-2") != 50 {
		t.Error("Expected priority 50 for policy-2")
	}
	if resolver.GetPolicyPriority("unknown") != 0 {
		t.Error("Expected priority 0 for unknown policy")
	}
}

func TestConflictResolver_Resolve_Priority(t *testing.T) {
	resolver := NewConflictResolver(StrategyPriority)
	resolver.SetPolicyPriority("policy-1", 100)
	resolver.SetPolicyPriority("policy-2", 50)

	conflict := &PolicyConflict{
		ID:       "test-conflict",
		Type:     ConflictTypeOverlap,
		Policies: []string{"policy-1", "policy-2"},
	}

	policies := []*Policy{
		{ID: "policy-1", Name: "Policy 1"},
		{ID: "policy-2", Name: "Policy 2"},
	}

	resolution := resolver.Resolve(conflict, policies)

	if resolution.WinningPolicy != "policy-1" {
		t.Errorf("Expected policy-1 to win, got %s", resolution.WinningPolicy)
	}
	if resolution.Strategy != StrategyPriority {
		t.Errorf("Expected strategy Priority, got %s", resolution.Strategy)
	}
}

func TestConflictResolver_Resolve_MostRestrictive(t *testing.T) {
	resolver := NewConflictResolver(StrategyMostRestrictive)

	conflict := &PolicyConflict{
		ID:       "test-conflict",
		Type:     ConflictTypeOverlap,
		Policies: []string{"policy-1", "policy-2", "policy-3"},
	}

	policies := []*Policy{
		{ID: "policy-1", Name: "Policy 1", EnforcementMode: ModeAudit, Severity: SeverityLow},
		{ID: "policy-2", Name: "Policy 2", EnforcementMode: ModeEnforce, Severity: SeverityCritical},
		{ID: "policy-3", Name: "Policy 3", EnforcementMode: ModeWarn, Severity: SeverityMedium},
	}

	resolution := resolver.Resolve(conflict, policies)

	if resolution.WinningPolicy != "policy-2" {
		t.Errorf("Expected policy-2 (most restrictive) to win, got %s", resolution.WinningPolicy)
	}
}

func TestConflictResolver_Resolve_LeastRestrictive(t *testing.T) {
	resolver := NewConflictResolver(StrategyLeastRestrictive)

	conflict := &PolicyConflict{
		ID:       "test-conflict",
		Type:     ConflictTypeOverlap,
		Policies: []string{"policy-1", "policy-2"},
	}

	policies := []*Policy{
		{ID: "policy-1", Name: "Policy 1", EnforcementMode: ModeEnforce, Severity: SeverityCritical},
		{ID: "policy-2", Name: "Policy 2", EnforcementMode: ModeAudit, Severity: SeverityLow},
	}

	resolution := resolver.Resolve(conflict, policies)

	if resolution.WinningPolicy != "policy-2" {
		t.Errorf("Expected policy-2 (least restrictive) to win, got %s", resolution.WinningPolicy)
	}
}

func TestConflictResolver_Resolve_Deny(t *testing.T) {
	resolver := NewConflictResolver(StrategyDeny)

	conflict := &PolicyConflict{
		ID:       "test-conflict",
		Type:     ConflictTypeOverlap,
		Policies: []string{"policy-1", "policy-2"},
	}

	policies := []*Policy{
		{ID: "policy-1", Name: "Policy 1"},
		{ID: "policy-2", Name: "Policy 2"},
	}

	resolution := resolver.Resolve(conflict, policies)

	if resolution.Outcome != "Action denied due to policy conflict" {
		t.Error("Expected deny outcome")
	}
}

func TestConflictResolver_Resolve_Allow(t *testing.T) {
	resolver := NewConflictResolver(StrategyAllow)

	conflict := &PolicyConflict{
		ID:       "test-conflict",
		Type:     ConflictTypeOverlap,
		Policies: []string{"policy-1", "policy-2"},
	}

	policies := []*Policy{
		{ID: "policy-1", Name: "Policy 1"},
		{ID: "policy-2", Name: "Policy 2"},
	}

	resolution := resolver.Resolve(conflict, policies)

	if resolution.Outcome != "Action allowed despite policy conflict" {
		t.Error("Expected allow outcome")
	}
}

func TestConflictResolver_RegisterResolver(t *testing.T) {
	resolver := NewConflictResolver(StrategyDeny)

	customCalled := false
	resolver.RegisterResolver(ConflictTypeOverlap, func(c *PolicyConflict, p []*Policy) *ConflictResolution {
		customCalled = true
		return &ConflictResolution{
			ConflictID: c.ID,
			Strategy:   StrategyCustom,
			Outcome:    "Custom resolution",
		}
	})

	conflict := &PolicyConflict{
		ID:       "test-conflict",
		Type:     ConflictTypeOverlap,
		Policies: []string{"policy-1"},
	}

	policies := []*Policy{{ID: "policy-1", Name: "Policy 1"}}

	resolution := resolver.Resolve(conflict, policies)

	if !customCalled {
		t.Error("Expected custom resolver to be called")
	}
	if resolution.Strategy != StrategyCustom {
		t.Error("Expected custom strategy")
	}
}

func TestConflictResolver_ResolveWithStrategy(t *testing.T) {
	resolver := NewConflictResolver(StrategyDeny) // Default is deny

	conflict := &PolicyConflict{
		ID:       "test-conflict",
		Type:     ConflictTypeOverlap,
		Policies: []string{"policy-1", "policy-2"},
	}

	policies := []*Policy{
		{ID: "policy-1", Name: "Policy 1"},
		{ID: "policy-2", Name: "Policy 2"},
	}

	// Use different strategy than default
	resolution := resolver.ResolveWithStrategy(conflict, policies, StrategyAllow)

	if resolution.Strategy != StrategyAllow {
		t.Errorf("Expected strategy Allow, got %s", resolution.Strategy)
	}
	if resolution.Outcome != "Action allowed despite policy conflict" {
		t.Error("Expected allow outcome")
	}
}

func TestSeverityDistance(t *testing.T) {
	tests := []struct {
		s1       Severity
		s2       Severity
		expected int
	}{
		{SeverityLow, SeverityLow, 0},
		{SeverityLow, SeverityMedium, 1},
		{SeverityLow, SeverityHigh, 2},
		{SeverityLow, SeverityCritical, 3},
		{SeverityCritical, SeverityLow, 3},
		{SeverityMedium, SeverityHigh, 1},
	}

	for _, tt := range tests {
		result := severityDistance(tt.s1, tt.s2)
		if result != tt.expected {
			t.Errorf("severityDistance(%s, %s) = %d, want %d", tt.s1, tt.s2, result, tt.expected)
		}
	}
}

func TestCalculateSimilarity(t *testing.T) {
	tests := []struct {
		s1       string
		s2       string
		expected float64
	}{
		{"hello", "hello", 1.0},
		{"", "", 1.0}, // Two empty strings are identical
		{"hello", "", 0.0},
		{"", "hello", 0.0},
		{"hello", "hella", 0.8}, // 4/5 match
		{"abc", "xyz", 0.0},
	}

	for _, tt := range tests {
		result := calculateSimilarity(tt.s1, tt.s2)
		if result != tt.expected {
			t.Errorf("calculateSimilarity(%q, %q) = %f, want %f", tt.s1, tt.s2, result, tt.expected)
		}
	}
}
