package policy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewControlMapping(t *testing.T) {
	cm := NewControlMapping()
	if cm == nil {
		t.Fatal("expected non-nil control mapping")
	}
	if cm.controls == nil {
		t.Error("expected controls map to be initialized")
	}
	if cm.policies == nil {
		t.Error("expected policies map to be initialized")
	}
}

func TestControlMapping_AddControl(t *testing.T) {
	cm := NewControlMapping()

	control := &ComplianceControl{
		ID:          "CIS-1.1.1",
		Framework:   FrameworkCIS,
		Title:       "Test Control",
		Description: "A test control",
		Severity:    SeverityMedium,
		PolicyIDs:   []string{"policy-1", "policy-2"},
	}

	err := cm.AddControl(control)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify control was added
	retrieved, ok := cm.GetControl("CIS-1.1.1")
	if !ok {
		t.Error("expected control to exist")
	}
	if retrieved.Title != "Test Control" {
		t.Errorf("expected title 'Test Control', got '%s'", retrieved.Title)
	}

	// Verify reverse mapping
	controls := cm.GetControlsForPolicy("policy-1")
	if len(controls) != 1 {
		t.Errorf("expected 1 control for policy-1, got %d", len(controls))
	}
}

func TestControlMapping_AddControl_NilError(t *testing.T) {
	cm := NewControlMapping()

	err := cm.AddControl(nil)
	if err == nil {
		t.Error("expected error for nil control")
	}
}

func TestControlMapping_AddControl_EmptyIDError(t *testing.T) {
	cm := NewControlMapping()

	control := &ComplianceControl{
		ID:        "",
		Framework: FrameworkCIS,
	}

	err := cm.AddControl(control)
	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestControlMapping_GetControlsByFramework(t *testing.T) {
	cm := NewControlMapping()

	// Add CIS controls
	cm.AddControl(&ComplianceControl{ID: "CIS-1", Framework: FrameworkCIS})
	cm.AddControl(&ComplianceControl{ID: "CIS-2", Framework: FrameworkCIS})

	// Add SOC2 control
	cm.AddControl(&ComplianceControl{ID: "SOC2-1", Framework: FrameworkSOC2})

	cisControls := cm.GetControlsByFramework(FrameworkCIS)
	if len(cisControls) != 2 {
		t.Errorf("expected 2 CIS controls, got %d", len(cisControls))
	}

	soc2Controls := cm.GetControlsByFramework(FrameworkSOC2)
	if len(soc2Controls) != 1 {
		t.Errorf("expected 1 SOC2 control, got %d", len(soc2Controls))
	}
}

func TestLoadBuiltinControlMappings(t *testing.T) {
	cm := NewControlMapping()
	LoadBuiltinControlMappings(cm)

	// Check CIS controls loaded
	cisControls := cm.GetControlsByFramework(FrameworkCIS)
	if len(cisControls) < 1 {
		t.Error("expected CIS controls to be loaded")
	}

	// Check SOC2 controls loaded
	soc2Controls := cm.GetControlsByFramework(FrameworkSOC2)
	if len(soc2Controls) < 1 {
		t.Error("expected SOC2 controls to be loaded")
	}

	// Check NIST controls loaded
	nistControls := cm.GetControlsByFramework(FrameworkNIST)
	if len(nistControls) < 1 {
		t.Error("expected NIST controls to be loaded")
	}
}

func TestSeverityRank(t *testing.T) {
	tests := []struct {
		severity Severity
		expected int
	}{
		{SeverityCritical, 4},
		{SeverityHigh, 3},
		{SeverityMedium, 2},
		{SeverityLow, 1},
		{Severity("unknown"), 0},
	}

	for _, tt := range tests {
		rank := severityRank(tt.severity)
		if rank != tt.expected {
			t.Errorf("severityRank(%s) = %d, expected %d", tt.severity, rank, tt.expected)
		}
	}
}

func TestNewComplianceReportGenerator(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compliance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "audit.db")
	store, err := NewSQLitePolicyAuditStore(&SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	})
	if err != nil {
		t.Fatalf("failed to create audit store: %v", err)
	}
	defer store.Close()

	registry := NewRegistry()
	generator := NewComplianceReportGenerator(store, registry)

	if generator == nil {
		t.Fatal("expected non-nil generator")
	}
	if generator.auditStore != store {
		t.Error("expected audit store to be set")
	}
	if generator.registry != registry {
		t.Error("expected registry to be set")
	}
}

func TestComplianceReportGenerator_GenerateReport(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compliance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "audit.db")
	store, err := NewSQLitePolicyAuditStore(&SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	})
	if err != nil {
		t.Fatalf("failed to create audit store: %v", err)
	}
	defer store.Close()

	registry := NewRegistry()
	generator := NewComplianceReportGenerator(store, registry)

	// Add some test audit entries
	ctx := context.Background()
	now := time.Now()

	entries := []*AuditEntry{
		{
			ID:           "entry-1",
			Timestamp:    now.Add(-1 * time.Hour),
			PolicyID:     "policy-1",
			PolicyName:   "Test Policy 1",
			ResourceType: "file",
			Allowed:      true,
			Duration:     10 * time.Millisecond,
			Metadata:     map[string]interface{}{"resource_id": "file-1"},
		},
		{
			ID:           "entry-2",
			Timestamp:    now.Add(-30 * time.Minute),
			PolicyID:     "policy-1",
			PolicyName:   "Test Policy 1",
			ResourceType: "file",
			Allowed:      false,
			Duration:     15 * time.Millisecond,
			Violations: []Violation{
				{Rule: "file-permissions", Message: "Permissions too open", Severity: SeverityHigh},
			},
			Metadata: map[string]interface{}{"resource_id": "file-2"},
		},
		{
			ID:           "entry-3",
			Timestamp:    now.Add(-15 * time.Minute),
			PolicyID:     "policy-2",
			PolicyName:   "Test Policy 2",
			ResourceType: "service",
			Allowed:      true,
			Duration:     5 * time.Millisecond,
			Metadata:     map[string]interface{}{"resource_id": "service-1"},
		},
	}

	for _, entry := range entries {
		if err := store.Store(ctx, entry); err != nil {
			t.Fatalf("failed to store entry: %v", err)
		}
	}

	// Generate report
	opts := &ReportOptions{
		Period: ReportPeriod{
			Start: now.Add(-2 * time.Hour),
			End:   now,
		},
		IncludeResourceTrails: true,
		MaxResourceTrails:     100,
		TopViolationsLimit:    10,
	}

	report, err := generator.GenerateReport(ctx, opts)
	if err != nil {
		t.Fatalf("failed to generate report: %v", err)
	}

	// Verify report
	if report.ID == "" {
		t.Error("expected report ID to be set")
	}
	if report.GeneratedAt.IsZero() {
		t.Error("expected generated_at to be set")
	}

	// Check summary
	if report.Summary == nil {
		t.Fatal("expected summary to be set")
	}
	if report.Summary.TotalEvaluations != 3 {
		t.Errorf("expected 3 total evaluations, got %d", report.Summary.TotalEvaluations)
	}
	if report.Summary.PassedEvaluations != 2 {
		t.Errorf("expected 2 passed evaluations, got %d", report.Summary.PassedEvaluations)
	}
	if report.Summary.FailedEvaluations != 1 {
		t.Errorf("expected 1 failed evaluation, got %d", report.Summary.FailedEvaluations)
	}
	if report.Summary.TotalViolations != 1 {
		t.Errorf("expected 1 violation, got %d", report.Summary.TotalViolations)
	}

	// Check policy results
	if len(report.PolicyResults) != 2 {
		t.Errorf("expected 2 policy results, got %d", len(report.PolicyResults))
	}

	// Check top violations
	if len(report.TopViolations) != 1 {
		t.Errorf("expected 1 top violation, got %d", len(report.TopViolations))
	}
	if len(report.TopViolations) > 0 {
		if report.TopViolations[0].Severity != SeverityHigh {
			t.Errorf("expected high severity, got %s", report.TopViolations[0].Severity)
		}
	}
}

func TestComplianceReportGenerator_GenerateReport_WithFramework(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compliance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "audit.db")
	store, err := NewSQLitePolicyAuditStore(&SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	})
	if err != nil {
		t.Fatalf("failed to create audit store: %v", err)
	}
	defer store.Close()

	registry := NewRegistry()
	generator := NewComplianceReportGenerator(store, registry)

	// Set up control mapping
	cm := NewControlMapping()
	cm.AddControl(&ComplianceControl{
		ID:        "CIS-1",
		Framework: FrameworkCIS,
		Title:     "Test Control",
		PolicyIDs: []string{"policy-1"},
	})
	generator.SetControlMapping(cm)

	// Add test audit entry
	ctx := context.Background()
	now := time.Now()

	entry := &AuditEntry{
		ID:           "entry-1",
		Timestamp:    now.Add(-1 * time.Hour),
		PolicyID:     "policy-1",
		PolicyName:   "Test Policy",
		Allowed:      true,
		Duration:     10 * time.Millisecond,
		Metadata:     map[string]interface{}{},
	}
	store.Store(ctx, entry)

	// Generate report with framework
	opts := &ReportOptions{
		Period: ReportPeriod{
			Start: now.Add(-2 * time.Hour),
			End:   now,
		},
		Framework:       FrameworkCIS,
		IncludeControls: true,
	}

	report, err := generator.GenerateReport(ctx, opts)
	if err != nil {
		t.Fatalf("failed to generate report: %v", err)
	}

	if report.Framework != FrameworkCIS {
		t.Errorf("expected framework CIS, got %s", report.Framework)
	}

	if len(report.ControlResults) == 0 {
		t.Error("expected control results when framework specified")
	}
}

func TestComplianceReportGenerator_FormatJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compliance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "audit.db")
	store, err := NewSQLitePolicyAuditStore(&SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	})
	if err != nil {
		t.Fatalf("failed to create audit store: %v", err)
	}
	defer store.Close()

	generator := NewComplianceReportGenerator(store, NewRegistry())

	report := &DetailedComplianceReport{
		ID:          "test-report",
		GeneratedAt: time.Now(),
		Summary: &ComplianceSummary{
			TotalEvaluations:     10,
			PassedEvaluations:    8,
			FailedEvaluations:    2,
			ViolationsBySeverity: make(map[Severity]int),
		},
	}

	data, err := generator.FormatReport(report, ReportFormatJSON)
	if err != nil {
		t.Fatalf("failed to format JSON: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}

	// Verify it contains expected content
	jsonStr := string(data)
	if !strContains(jsonStr, "test-report") {
		t.Error("expected JSON to contain report ID")
	}
}

func TestComplianceReportGenerator_FormatCSV(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compliance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "audit.db")
	store, err := NewSQLitePolicyAuditStore(&SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	})
	if err != nil {
		t.Fatalf("failed to create audit store: %v", err)
	}
	defer store.Close()

	generator := NewComplianceReportGenerator(store, NewRegistry())

	report := &DetailedComplianceReport{
		ID:          "test-report",
		GeneratedAt: time.Now(),
		TopViolations: []*DetailedViolation{
			{
				PolicyID:   "policy-1",
				PolicyName: "Test Policy",
				Rule:       "test-rule",
				Severity:   SeverityHigh,
				Count:      5,
				FirstSeen:  time.Now().Add(-1 * time.Hour),
				LastSeen:   time.Now(),
			},
		},
	}

	data, err := generator.FormatReport(report, ReportFormatCSV)
	if err != nil {
		t.Fatalf("failed to format CSV: %v", err)
	}

	csvStr := string(data)
	if !strContains(csvStr, "Policy ID") {
		t.Error("expected CSV to contain header")
	}
	if !strContains(csvStr, "policy-1") {
		t.Error("expected CSV to contain policy ID")
	}
}

func TestComplianceReportGenerator_FormatText(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compliance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "audit.db")
	store, err := NewSQLitePolicyAuditStore(&SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	})
	if err != nil {
		t.Fatalf("failed to create audit store: %v", err)
	}
	defer store.Close()

	generator := NewComplianceReportGenerator(store, NewRegistry())

	report := &DetailedComplianceReport{
		ID:          "test-report",
		GeneratedAt: time.Now(),
		Period: ReportPeriod{
			Start: time.Now().Add(-24 * time.Hour),
			End:   time.Now(),
		},
		Summary: &ComplianceSummary{
			TotalEvaluations:     100,
			PassedEvaluations:    90,
			FailedEvaluations:    10,
			EvaluationPassRate:   90.0,
			TotalViolations:      10,
			ViolationsBySeverity: make(map[Severity]int),
		},
	}

	data, err := generator.FormatReport(report, ReportFormatText)
	if err != nil {
		t.Fatalf("failed to format text: %v", err)
	}

	textStr := string(data)
	if !strContains(textStr, "COMPLIANCE REPORT") {
		t.Error("expected text to contain title")
	}
	if !strContains(textStr, "SUMMARY") {
		t.Error("expected text to contain summary section")
	}
}

func TestComplianceReportGenerator_FormatMarkdown(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compliance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "audit.db")
	store, err := NewSQLitePolicyAuditStore(&SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	})
	if err != nil {
		t.Fatalf("failed to create audit store: %v", err)
	}
	defer store.Close()

	generator := NewComplianceReportGenerator(store, NewRegistry())

	report := &DetailedComplianceReport{
		ID:          "test-report",
		GeneratedAt: time.Now(),
		Period: ReportPeriod{
			Start: time.Now().Add(-24 * time.Hour),
			End:   time.Now(),
		},
		Summary: &ComplianceSummary{
			TotalEvaluations:     100,
			PassedEvaluations:    90,
			FailedEvaluations:    10,
			EvaluationPassRate:   90.0,
			TotalViolations:      10,
			ViolationsBySeverity: make(map[Severity]int),
		},
	}

	data, err := generator.FormatReport(report, ReportFormatMarkdown)
	if err != nil {
		t.Fatalf("failed to format markdown: %v", err)
	}

	mdStr := string(data)
	if !strContains(mdStr, "# Compliance Report") {
		t.Error("expected markdown to contain title")
	}
	if !strContains(mdStr, "## Summary") {
		t.Error("expected markdown to contain summary section")
	}
	if !strContains(mdStr, "|") {
		t.Error("expected markdown to contain tables")
	}
}

func TestComplianceReportGenerator_FormatHTML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compliance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "audit.db")
	store, err := NewSQLitePolicyAuditStore(&SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	})
	if err != nil {
		t.Fatalf("failed to create audit store: %v", err)
	}
	defer store.Close()

	generator := NewComplianceReportGenerator(store, NewRegistry())

	report := &DetailedComplianceReport{
		ID:          "test-report",
		GeneratedAt: time.Now(),
		Period: ReportPeriod{
			Start: time.Now().Add(-24 * time.Hour),
			End:   time.Now(),
		},
		Summary: &ComplianceSummary{
			TotalEvaluations:     100,
			PassedEvaluations:    90,
			FailedEvaluations:    10,
			EvaluationPassRate:   90.0,
			TotalViolations:      10,
			ViolationsBySeverity: make(map[Severity]int),
		},
		TopViolations: []*DetailedViolation{
			{
				PolicyName: "Test Policy",
				Rule:       "test-rule",
				Severity:   SeverityCritical,
				Count:      5,
			},
		},
	}

	data, err := generator.FormatReport(report, ReportFormatHTML)
	if err != nil {
		t.Fatalf("failed to format HTML: %v", err)
	}

	htmlStr := string(data)
	if !strContains(htmlStr, "<!DOCTYPE html>") {
		t.Error("expected HTML doctype")
	}
	if !strContains(htmlStr, "<h1>Compliance Report</h1>") {
		t.Error("expected HTML title")
	}
	if !strContains(htmlStr, "severity-critical") {
		t.Error("expected HTML to contain severity class")
	}
}

func TestComplianceReportGenerator_FormatUnsupported(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compliance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "audit.db")
	store, err := NewSQLitePolicyAuditStore(&SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	})
	if err != nil {
		t.Fatalf("failed to create audit store: %v", err)
	}
	defer store.Close()

	generator := NewComplianceReportGenerator(store, NewRegistry())

	report := &DetailedComplianceReport{ID: "test"}

	_, err = generator.FormatReport(report, ReportFormat("pdf"))
	if err == nil {
		t.Error("expected error for unsupported format")
	}
}

func TestResourceAuditTrail(t *testing.T) {
	trail := &ResourceAuditTrail{
		ResourceID:   "resource-1",
		ResourceType: "file",
		Evaluations:  make([]*ResourceEvaluation, 0),
		FirstSeen:    time.Now().Add(-1 * time.Hour),
		LastSeen:     time.Now(),
	}

	trail.Evaluations = append(trail.Evaluations, &ResourceEvaluation{
		EvaluationID: "eval-1",
		PolicyID:     "policy-1",
		Allowed:      true,
		Timestamp:    time.Now(),
	})

	if len(trail.Evaluations) != 1 {
		t.Errorf("expected 1 evaluation, got %d", len(trail.Evaluations))
	}
}

func TestDefaultReportOptions(t *testing.T) {
	period := ReportPeriod{
		Start: time.Now().Add(-24 * time.Hour),
		End:   time.Now(),
	}

	opts := DefaultReportOptions(period)

	if opts == nil {
		t.Fatal("expected non-nil options")
	}
	if !opts.IncludeResourceTrails {
		t.Error("expected IncludeResourceTrails to be true")
	}
	if !opts.IncludeControls {
		t.Error("expected IncludeControls to be true")
	}
	if opts.MaxResourceTrails != 1000 {
		t.Errorf("expected MaxResourceTrails 1000, got %d", opts.MaxResourceTrails)
	}
	if opts.TopViolationsLimit != 20 {
		t.Errorf("expected TopViolationsLimit 20, got %d", opts.TopViolationsLimit)
	}
}

func TestGenerateRecommendations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "compliance-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "audit.db")
	store, err := NewSQLitePolicyAuditStore(&SQLitePolicyAuditStoreConfig{
		Path:          dbPath,
		AutoRetention: false,
	})
	if err != nil {
		t.Fatalf("failed to create audit store: %v", err)
	}
	defer store.Close()

	generator := NewComplianceReportGenerator(store, NewRegistry())

	// Test with critical violations
	report := &DetailedComplianceReport{
		Summary: &ComplianceSummary{
			TotalEvaluations:   100,
			PassedEvaluations:  80,
			EvaluationPassRate: 80.0,
			ViolationsBySeverity: map[Severity]int{
				SeverityCritical: 5,
				SeverityHigh:     10,
			},
			NonCompliantResources: 3,
		},
	}

	recommendations := generator.generateRecommendations(report)

	if len(recommendations) == 0 {
		t.Error("expected recommendations to be generated")
	}

	// Should have recommendation for critical violations
	hasCriticalRec := false
	for _, rec := range recommendations {
		if strContains(rec, "critical") {
			hasCriticalRec = true
			break
		}
	}
	if !hasCriticalRec {
		t.Error("expected recommendation for critical violations")
	}
}

func TestAppendUnique(t *testing.T) {
	slice := []string{"a", "b", "c"}

	// Adding new item
	result := appendUnique(slice, "d")
	if len(result) != 4 {
		t.Errorf("expected 4 items, got %d", len(result))
	}

	// Adding existing item
	result = appendUnique(result, "b")
	if len(result) != 4 {
		t.Errorf("expected 4 items (no duplicate), got %d", len(result))
	}
}

// Helper function
func strContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
