package policy

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPolicyAuditorRecordEvaluation(t *testing.T) {
	auditor := NewPolicyAuditor(100)

	result := &EvaluationResult{
		PolicyID:    "test-policy",
		PolicyName:  "Test Policy",
		Allowed:     true,
		Duration:    10 * time.Millisecond,
		EvaluatedAt: time.Now(),
		Violations:  make([]Violation, 0),
	}

	ctx := context.Background()
	auditor.RecordEvaluation(ctx, result, "deployment", "admin", "create", ModeEnforce)

	entries := auditor.GetEntries(nil)
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	if entry.PolicyID != "test-policy" {
		t.Errorf("PolicyID = %s, want test-policy", entry.PolicyID)
	}
	if entry.ResourceType != "deployment" {
		t.Errorf("ResourceType = %s, want deployment", entry.ResourceType)
	}
	if entry.User != "admin" {
		t.Errorf("User = %s, want admin", entry.User)
	}
	if entry.Action != "create" {
		t.Errorf("Action = %s, want create", entry.Action)
	}
	if entry.EnforcementMode != ModeEnforce {
		t.Errorf("EnforcementMode = %s, want enforce", entry.EnforcementMode)
	}
}

func TestPolicyAuditorCapacity(t *testing.T) {
	auditor := NewPolicyAuditor(5) // Small capacity

	ctx := context.Background()

	// Add more than capacity
	for i := 0; i < 10; i++ {
		result := &EvaluationResult{
			PolicyID:    "policy",
			PolicyName:  "Policy",
			Allowed:     true,
			Duration:    time.Millisecond,
			EvaluatedAt: time.Now(),
			Violations:  make([]Violation, 0),
		}
		auditor.RecordEvaluation(ctx, result, "resource", "user", "action", ModeEnforce)
	}

	entries := auditor.GetEntries(nil)
	if len(entries) != 5 {
		t.Errorf("Expected 5 entries (capacity), got %d", len(entries))
	}
}

func TestPolicyAuditorRecordPolicyResult(t *testing.T) {
	auditor := NewPolicyAuditor(100)

	result := &PolicyResult{
		Allowed:       false,
		EvaluatedAt:   time.Now(),
		TotalDuration: 20 * time.Millisecond,
		Results: []*EvaluationResult{
			{
				PolicyID:    "policy1",
				PolicyName:  "Policy 1",
				Allowed:     true,
				Duration:    10 * time.Millisecond,
				EvaluatedAt: time.Now(),
				Violations:  make([]Violation, 0),
			},
			{
				PolicyID:    "policy2",
				PolicyName:  "Policy 2",
				Allowed:     false,
				Duration:    10 * time.Millisecond,
				EvaluatedAt: time.Now(),
				Violations: []Violation{
					{Rule: "rule1", Message: "Violation", Severity: SeverityHigh},
				},
			},
		},
		Summary: &PolicySummary{},
	}

	ctx := context.Background()
	auditor.RecordPolicyResult(ctx, result, "pod", "admin", "delete", ModeEnforce)

	entries := auditor.GetEntries(nil)
	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}
}

func TestAuditFilterPolicyID(t *testing.T) {
	auditor := NewPolicyAuditor(100)
	ctx := context.Background()

	// Add entries with different policy IDs
	policies := []string{"policy1", "policy2", "policy3"}
	for _, policyID := range policies {
		result := &EvaluationResult{
			PolicyID:    policyID,
			PolicyName:  policyID,
			Allowed:     true,
			Duration:    time.Millisecond,
			EvaluatedAt: time.Now(),
			Violations:  make([]Violation, 0),
		}
		auditor.RecordEvaluation(ctx, result, "resource", "user", "action", ModeEnforce)
	}

	filter := &AuditFilter{
		PolicyID: "policy2",
	}

	entries := auditor.GetEntries(filter)
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}
	if entries[0].PolicyID != "policy2" {
		t.Errorf("PolicyID = %s, want policy2", entries[0].PolicyID)
	}
}

func TestAuditFilterResourceType(t *testing.T) {
	auditor := NewPolicyAuditor(100)
	ctx := context.Background()

	// Add entries with different resource types
	resources := []string{"pod", "deployment", "service"}
	for _, resourceType := range resources {
		result := &EvaluationResult{
			PolicyID:    "policy",
			PolicyName:  "Policy",
			Allowed:     true,
			Duration:    time.Millisecond,
			EvaluatedAt: time.Now(),
			Violations:  make([]Violation, 0),
		}
		auditor.RecordEvaluation(ctx, result, resourceType, "user", "action", ModeEnforce)
	}

	filter := &AuditFilter{
		ResourceType: "deployment",
	}

	entries := auditor.GetEntries(filter)
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}
	if entries[0].ResourceType != "deployment" {
		t.Errorf("ResourceType = %s, want deployment", entries[0].ResourceType)
	}
}

func TestAuditFilterAllowed(t *testing.T) {
	auditor := NewPolicyAuditor(100)
	ctx := context.Background()

	// Add allowed and denied entries
	for i := 0; i < 5; i++ {
		result := &EvaluationResult{
			PolicyID:    "policy",
			PolicyName:  "Policy",
			Allowed:     i%2 == 0, // Alternate allowed/denied
			Duration:    time.Millisecond,
			EvaluatedAt: time.Now(),
			Violations:  make([]Violation, 0),
		}
		auditor.RecordEvaluation(ctx, result, "resource", "user", "action", ModeEnforce)
	}

	allowed := true
	filter := &AuditFilter{
		Allowed: &allowed,
	}

	entries := auditor.GetEntries(filter)
	if len(entries) != 3 {
		t.Fatalf("Expected 3 allowed entries, got %d", len(entries))
	}
	for _, entry := range entries {
		if !entry.Allowed {
			t.Error("Expected only allowed entries")
		}
	}
}

func TestAuditFilterTimeRange(t *testing.T) {
	auditor := NewPolicyAuditor(100)
	ctx := context.Background()

	now := time.Now()

	// Add entries at different times
	for i := 0; i < 5; i++ {
		result := &EvaluationResult{
			PolicyID:    "policy",
			PolicyName:  "Policy",
			Allowed:     true,
			Duration:    time.Millisecond,
			EvaluatedAt: now.Add(time.Duration(i) * time.Hour),
			Violations:  make([]Violation, 0),
		}
		auditor.RecordEvaluation(ctx, result, "resource", "user", "action", ModeEnforce)
	}

	filter := &AuditFilter{
		StartTime: now.Add(1 * time.Hour),
		EndTime:   now.Add(3 * time.Hour),
	}

	entries := auditor.GetEntries(filter)
	// Time range is inclusive on both ends, so entries at 1h, 2h, and 3h match
	if len(entries) != 3 {
		t.Fatalf("Expected 3 entries in time range, got %d", len(entries))
	}
}

func TestAuditFilterLimit(t *testing.T) {
	auditor := NewPolicyAuditor(100)
	ctx := context.Background()

	// Add many entries
	for i := 0; i < 20; i++ {
		result := &EvaluationResult{
			PolicyID:    "policy",
			PolicyName:  "Policy",
			Allowed:     true,
			Duration:    time.Millisecond,
			EvaluatedAt: time.Now(),
			Violations:  make([]Violation, 0),
		}
		auditor.RecordEvaluation(ctx, result, "resource", "user", "action", ModeEnforce)
	}

	filter := &AuditFilter{
		Limit: 10,
	}

	entries := auditor.GetEntries(filter)
	if len(entries) != 10 {
		t.Fatalf("Expected 10 entries (limit), got %d", len(entries))
	}
}

func TestAuditSummary(t *testing.T) {
	auditor := NewPolicyAuditor(100)
	ctx := context.Background()

	// Add varied entries
	entries := []struct {
		policyID     string
		resourceType string
		allowed      bool
		violations   []Violation
	}{
		{"policy1", "pod", true, []Violation{}},
		{"policy1", "pod", false, []Violation{{Severity: SeverityHigh}}},
		{"policy2", "deployment", true, []Violation{}},
		{"policy2", "deployment", false, []Violation{{Severity: SeverityCritical}, {Severity: SeverityMedium}}},
	}

	for _, e := range entries {
		result := &EvaluationResult{
			PolicyID:    e.policyID,
			PolicyName:  e.policyID,
			Allowed:     e.allowed,
			Duration:    10 * time.Millisecond,
			EvaluatedAt: time.Now(),
			Violations:  e.violations,
		}
		auditor.RecordEvaluation(ctx, result, e.resourceType, "user", "action", ModeEnforce)
	}

	summary := auditor.GetSummary(nil)

	if summary.TotalEvaluations != 4 {
		t.Errorf("TotalEvaluations = %d, want 4", summary.TotalEvaluations)
	}
	if summary.AllowedEvaluations != 2 {
		t.Errorf("AllowedEvaluations = %d, want 2", summary.AllowedEvaluations)
	}
	if summary.DeniedEvaluations != 2 {
		t.Errorf("DeniedEvaluations = %d, want 2", summary.DeniedEvaluations)
	}
	if summary.TotalViolations != 3 {
		t.Errorf("TotalViolations = %d, want 3", summary.TotalViolations)
	}

	if summary.ViolationsBySeverity[SeverityCritical] != 1 {
		t.Errorf("Critical violations = %d, want 1", summary.ViolationsBySeverity[SeverityCritical])
	}
	if summary.ViolationsBySeverity[SeverityHigh] != 1 {
		t.Errorf("High violations = %d, want 1", summary.ViolationsBySeverity[SeverityHigh])
	}
	if summary.ViolationsBySeverity[SeverityMedium] != 1 {
		t.Errorf("Medium violations = %d, want 1", summary.ViolationsBySeverity[SeverityMedium])
	}

	if summary.ViolationsByPolicy["policy1"] != 1 {
		t.Errorf("Policy1 violations = %d, want 1", summary.ViolationsByPolicy["policy1"])
	}
	if summary.ViolationsByPolicy["policy2"] != 2 {
		t.Errorf("Policy2 violations = %d, want 2", summary.ViolationsByPolicy["policy2"])
	}

	if summary.EvaluationsByResource["pod"] != 2 {
		t.Errorf("Pod evaluations = %d, want 2", summary.EvaluationsByResource["pod"])
	}
	if summary.EvaluationsByResource["deployment"] != 2 {
		t.Errorf("Deployment evaluations = %d, want 2", summary.EvaluationsByResource["deployment"])
	}

	if summary.AverageDuration != 10*time.Millisecond {
		t.Errorf("AverageDuration = %v, want 10ms", summary.AverageDuration)
	}
}

func TestAuditorClear(t *testing.T) {
	auditor := NewPolicyAuditor(100)
	ctx := context.Background()

	// Add entries
	for i := 0; i < 5; i++ {
		result := &EvaluationResult{
			PolicyID:    "policy",
			PolicyName:  "Policy",
			Allowed:     true,
			Duration:    time.Millisecond,
			EvaluatedAt: time.Now(),
			Violations:  make([]Violation, 0),
		}
		auditor.RecordEvaluation(ctx, result, "resource", "user", "action", ModeEnforce)
	}

	if len(auditor.GetEntries(nil)) != 5 {
		t.Fatal("Expected 5 entries before clear")
	}

	auditor.Clear()

	if len(auditor.GetEntries(nil)) != 0 {
		t.Error("Expected 0 entries after clear")
	}
}

func TestComplianceReporter(t *testing.T) {
	registry := NewRegistry()
	auditor := NewPolicyAuditor(100)
	reporter := NewComplianceReporter(auditor, registry)

	// Register policies
	registry.RegisterPolicy(&Policy{
		ID:       "policy1",
		Name:     "Policy 1",
		Severity: SeverityHigh,
	})
	registry.RegisterPolicy(&Policy{
		ID:       "policy2",
		Name:     "Policy 2",
		Severity: SeverityCritical,
	})

	ctx := context.Background()
	now := time.Now()

	// Add audit entries
	auditor.RecordEvaluation(ctx, &EvaluationResult{
		PolicyID:    "policy1",
		PolicyName:  "Policy 1",
		Allowed:     true,
		Duration:    time.Millisecond,
		EvaluatedAt: now,
		Violations:  []Violation{},
	}, "pod", "user", "create", ModeEnforce)

	auditor.RecordEvaluation(ctx, &EvaluationResult{
		PolicyID:    "policy2",
		PolicyName:  "Policy 2",
		Allowed:     false,
		Duration:    time.Millisecond,
		EvaluatedAt: now,
		Violations: []Violation{
			{Severity: SeverityCritical, Message: "Critical violation"},
			{Severity: SeverityHigh, Message: "High violation"},
		},
	}, "pod", "user", "delete", ModeEnforce)

	// Generate report
	period := ReportPeriod{
		Start: now.Add(-1 * time.Hour),
		End:   now.Add(1 * time.Hour),
	}

	report := reporter.GenerateReport(period)

	if report.TotalPolicies != 2 {
		t.Errorf("TotalPolicies = %d, want 2", report.TotalPolicies)
	}
	if report.CompliantPolicies != 1 {
		t.Errorf("CompliantPolicies = %d, want 1", report.CompliantPolicies)
	}
	if report.ViolatingPolicies != 1 {
		t.Errorf("ViolatingPolicies = %d, want 1", report.ViolatingPolicies)
	}

	expectedRate := 50.0 // 1 out of 2
	if report.ComplianceRate != expectedRate {
		t.Errorf("ComplianceRate = %.2f, want %.2f", report.ComplianceRate, expectedRate)
	}

	if report.ViolationsBySeverity[SeverityCritical] != 1 {
		t.Errorf("Critical violations = %d, want 1", report.ViolationsBySeverity[SeverityCritical])
	}
	if report.ViolationsBySeverity[SeverityHigh] != 1 {
		t.Errorf("High violations = %d, want 1", report.ViolationsBySeverity[SeverityHigh])
	}
}

func TestComplianceReporterTopViolations(t *testing.T) {
	registry := NewRegistry()
	auditor := NewPolicyAuditor(100)
	reporter := NewComplianceReporter(auditor, registry)

	// Register policies
	for i := 1; i <= 5; i++ {
		registry.RegisterPolicy(&Policy{
			ID:       fmt.Sprintf("policy%d", i),
			Name:     fmt.Sprintf("Policy %d", i),
			Severity: SeverityHigh,
		})
	}

	ctx := context.Background()
	now := time.Now()

	// Add entries with different violation counts
	// policy1: 5 violations, policy2: 3, policy3: 2, policy4: 1, policy5: 0
	for i := 1; i <= 5; i++ {
		policyID := fmt.Sprintf("policy%d", i)
		violationCount := 6 - i // 5, 4, 3, 2, 1

		for j := 0; j < violationCount; j++ {
			auditor.RecordEvaluation(ctx, &EvaluationResult{
				PolicyID:    policyID,
				PolicyName:  policyID,
				Allowed:     false,
				Duration:    time.Millisecond,
				EvaluatedAt: now,
				Violations: []Violation{
					{Severity: SeverityHigh, Message: "Violation"},
				},
			}, "resource", "user", "action", ModeEnforce)
		}
	}

	period := ReportPeriod{
		Start: now.Add(-1 * time.Hour),
		End:   now.Add(1 * time.Hour),
	}

	report := reporter.GenerateReport(period)

	if len(report.TopViolations) > 5 {
		t.Errorf("Expected at most 5 top violations, got %d", len(report.TopViolations))
	}

	// Check ordering (descending by count)
	if len(report.TopViolations) > 1 {
		for i := 0; i < len(report.TopViolations)-1; i++ {
			if report.TopViolations[i].Count < report.TopViolations[i+1].Count {
				t.Error("Top violations not sorted by count (descending)")
			}
		}
	}
}

func TestAuditFilterUserAndAction(t *testing.T) {
	auditor := NewPolicyAuditor(100)
	ctx := context.Background()

	users := []string{"alice", "bob", "charlie"}
	actions := []string{"create", "update", "delete"}

	// Add entries with different users and actions
	for _, user := range users {
		for _, action := range actions {
			result := &EvaluationResult{
				PolicyID:    "policy",
				PolicyName:  "Policy",
				Allowed:     true,
				Duration:    time.Millisecond,
				EvaluatedAt: time.Now(),
				Violations:  make([]Violation, 0),
			}
			auditor.RecordEvaluation(ctx, result, "resource", user, action, ModeEnforce)
		}
	}

	// Filter by user
	filter := &AuditFilter{
		User: "alice",
	}
	entries := auditor.GetEntries(filter)
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries for user alice, got %d", len(entries))
	}

	// Filter by action
	filter = &AuditFilter{
		Action: "delete",
	}
	entries = auditor.GetEntries(filter)
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries for action delete, got %d", len(entries))
	}

	// Filter by both
	filter = &AuditFilter{
		User:   "bob",
		Action: "update",
	}
	entries = auditor.GetEntries(filter)
	if len(entries) != 1 {
		t.Errorf("Expected 1 entry for bob+update, got %d", len(entries))
	}
}
