package kms

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestComplianceReporter_RegisterKey(t *testing.T) {
	reporter := NewComplianceReporter(DefaultComplianceConfig(), nil)

	key := KeyInventoryItem{
		KeyID:     "test-key-1",
		KeyType:   "symmetric",
		Algorithm: "aes-256-gcm",
		KeySize:   256,
		CreatedAt: time.Now(),
		Status:    "active",
	}

	reporter.RegisterKey(key)

	reporter.mu.RLock()
	defer reporter.mu.RUnlock()

	if _, ok := reporter.keyInventory[key.KeyID]; !ok {
		t.Error("key was not registered")
	}
}

func TestComplianceReporter_UpdateKeyStatus(t *testing.T) {
	reporter := NewComplianceReporter(DefaultComplianceConfig(), nil)

	key := KeyInventoryItem{
		KeyID:  "test-key-1",
		Status: "active",
	}
	reporter.RegisterKey(key)

	err := reporter.UpdateKeyStatus("test-key-1", "disabled")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	reporter.mu.RLock()
	status := reporter.keyInventory["test-key-1"].Status
	reporter.mu.RUnlock()

	if status != "disabled" {
		t.Errorf("expected status 'disabled', got '%s'", status)
	}

	err = reporter.UpdateKeyStatus("nonexistent", "disabled")
	if err == nil {
		t.Error("expected error for nonexistent key")
	}
}

func TestComplianceReporter_RecordRotation(t *testing.T) {
	config := DefaultComplianceConfig()
	config.RotationMaxAge = 90 * 24 * time.Hour

	reporter := NewComplianceReporter(config, nil)

	key := KeyInventoryItem{
		KeyID:     "test-key-1",
		CreatedAt: time.Now().Add(-30 * 24 * time.Hour),
		Status:    "active",
	}
	reporter.RegisterKey(key)

	event := RotationEvent{
		KeyID:      "test-key-1",
		RotatedAt:  time.Now(),
		RotatedBy:  "admin",
		OldVersion: 1,
		NewVersion: 2,
		Reason:     "scheduled",
	}
	reporter.RecordRotation(event)

	reporter.mu.RLock()
	defer reporter.mu.RUnlock()

	if len(reporter.rotationLog) != 1 {
		t.Errorf("expected 1 rotation event, got %d", len(reporter.rotationLog))
	}

	updated := reporter.keyInventory["test-key-1"]
	if updated.LastRotated == nil {
		t.Error("LastRotated was not set")
	}
	if updated.NextRotation == nil {
		t.Error("NextRotation was not set")
	}
	if updated.RotationCount != 1 {
		t.Errorf("expected RotationCount 1, got %d", updated.RotationCount)
	}
}

func TestComplianceReporter_RecordAccess(t *testing.T) {
	reporter := NewComplianceReporter(DefaultComplianceConfig(), nil)

	reporter.RecordAccess("user1", "/secrets/db-password", "read", true)
	reporter.RecordAccess("user1", "/secrets/db-password", "read", true)
	reporter.RecordAccess("user1", "/secrets/db-password", "read", false)

	reporter.mu.RLock()
	defer reporter.mu.RUnlock()

	entry, ok := reporter.accessStats["user1:/secrets/db-password"]
	if !ok {
		t.Fatal("access stats not recorded")
	}

	if entry.accessCount != 3 {
		t.Errorf("expected accessCount 3, got %d", entry.accessCount)
	}
	if entry.failureCount != 1 {
		t.Errorf("expected failureCount 1, got %d", entry.failureCount)
	}
}

func TestComplianceReporter_GenerateReport(t *testing.T) {
	config := DefaultComplianceConfig()
	config.RotationMaxAge = 365 * 24 * time.Hour

	reporter := NewComplianceReporter(config, nil)

	// Register some keys
	now := time.Now()
	lastRotated := now.Add(-30 * 24 * time.Hour)
	nextRotation := now.Add(335 * 24 * time.Hour)

	reporter.RegisterKey(KeyInventoryItem{
		KeyID:        "key-1",
		KeyType:      "symmetric",
		Algorithm:    "aes-256-gcm",
		KeySize:      256,
		CreatedAt:    now.Add(-60 * 24 * time.Hour),
		LastRotated:  &lastRotated,
		NextRotation: &nextRotation,
		Status:       "active",
	})

	reporter.RegisterKey(KeyInventoryItem{
		KeyID:     "key-2",
		KeyType:   "asymmetric",
		Algorithm: "rsa-4096",
		KeySize:   4096,
		CreatedAt: now.Add(-90 * 24 * time.Hour),
		Status:    "active",
	})

	// Record some access
	reporter.RecordAccess("admin", "/secrets/prod/db", "read", true)
	reporter.RecordAccess("service-a", "/secrets/prod/api-key", "read", true)

	period := ReportPeriod{
		Start: now.Add(-30 * 24 * time.Hour),
		End:   now,
	}

	report, err := reporter.GenerateReport(context.Background(), FrameworkSOC2, period)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	if report.Framework != FrameworkSOC2 {
		t.Errorf("expected framework %s, got %s", FrameworkSOC2, report.Framework)
	}

	if len(report.Results) == 0 {
		t.Error("expected compliance check results")
	}

	if len(report.KeyInventory) != 2 {
		t.Errorf("expected 2 keys in inventory, got %d", len(report.KeyInventory))
	}

	if report.Summary.TotalRequirements == 0 {
		t.Error("expected summary to have requirements")
	}
}

func TestComplianceReporter_AllFrameworks(t *testing.T) {
	reporter := NewComplianceReporter(DefaultComplianceConfig(), nil)

	frameworks := []ComplianceFramework{
		FrameworkSOC2,
		FrameworkPCIDSS,
		FrameworkHIPAA,
		FrameworkGDPR,
		FrameworkFedRAMP,
		FrameworkNIST,
	}

	period := ReportPeriod{
		Start: time.Now().Add(-30 * 24 * time.Hour),
		End:   time.Now(),
	}

	for _, framework := range frameworks {
		t.Run(string(framework), func(t *testing.T) {
			report, err := reporter.GenerateReport(context.Background(), framework, period)
			if err != nil {
				t.Fatalf("GenerateReport(%s) failed: %v", framework, err)
			}

			if report.Framework != framework {
				t.Errorf("expected framework %s, got %s", framework, report.Framework)
			}

			if len(report.Results) == 0 {
				t.Error("expected compliance check results")
			}
		})
	}
}

func TestComplianceReporter_CryptographicControls(t *testing.T) {
	config := DefaultComplianceConfig()
	config.MinKeySize = 256

	reporter := NewComplianceReporter(config, nil)

	// Register a weak key
	reporter.RegisterKey(KeyInventoryItem{
		KeyID:     "weak-key",
		Algorithm: "aes-128-gcm",
		KeySize:   128,
		CreatedAt: time.Now(),
		Status:    "active",
	})

	period := ReportPeriod{
		Start: time.Now().Add(-24 * time.Hour),
		End:   time.Now(),
	}

	report, err := reporter.GenerateReport(context.Background(), FrameworkPCIDSS, period)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	// Should have at least one non-compliant result for weak key
	found := false
	for _, result := range report.Results {
		if result.Requirement.Category == "Cryptographic Keys" && result.Status == StatusNonCompliant {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected non-compliant result for weak key")
	}
}

func TestComplianceReporter_KeyRotationOverdue(t *testing.T) {
	config := DefaultComplianceConfig()
	config.RotationMaxAge = 90 * 24 * time.Hour

	reporter := NewComplianceReporter(config, nil)

	// Register an overdue key
	overdue := time.Now().Add(-100 * 24 * time.Hour)
	overdueNext := time.Now().Add(-10 * 24 * time.Hour)

	reporter.RegisterKey(KeyInventoryItem{
		KeyID:        "overdue-key",
		Algorithm:    "aes-256-gcm",
		KeySize:      256,
		CreatedAt:    overdue,
		LastRotated:  &overdue,
		NextRotation: &overdueNext,
		Status:       "active",
	})

	period := ReportPeriod{
		Start: time.Now().Add(-30 * 24 * time.Hour),
		End:   time.Now(),
	}

	report, err := reporter.GenerateReport(context.Background(), FrameworkSOC2, period)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	if report.Rotation.KeysOverdue != 1 {
		t.Errorf("expected 1 overdue key, got %d", report.Rotation.KeysOverdue)
	}
}

func TestComplianceReport_ExportJSON(t *testing.T) {
	reporter := NewComplianceReporter(DefaultComplianceConfig(), nil)

	reporter.RegisterKey(KeyInventoryItem{
		KeyID:     "test-key",
		Algorithm: "aes-256-gcm",
		KeySize:   256,
		CreatedAt: time.Now(),
		Status:    "active",
	})

	period := ReportPeriod{
		Start: time.Now().Add(-24 * time.Hour),
		End:   time.Now(),
	}

	report, err := reporter.GenerateReport(context.Background(), FrameworkSOC2, period)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	data, err := report.ExportJSON()
	if err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}
}

func TestComplianceReport_GetNonCompliantItems(t *testing.T) {
	report := &ComplianceReport{
		Results: []ComplianceCheckResult{
			{Requirement: ComplianceRequirement{ID: "1"}, Status: StatusCompliant},
			{Requirement: ComplianceRequirement{ID: "2"}, Status: StatusNonCompliant},
			{Requirement: ComplianceRequirement{ID: "3"}, Status: StatusNonCompliant},
			{Requirement: ComplianceRequirement{ID: "4"}, Status: StatusPartiallyCompliant},
		},
	}

	nonCompliant := report.GetNonCompliantItems()
	if len(nonCompliant) != 2 {
		t.Errorf("expected 2 non-compliant items, got %d", len(nonCompliant))
	}
}

func TestComplianceReport_GetCriticalIssues(t *testing.T) {
	report := &ComplianceReport{
		Results: []ComplianceCheckResult{
			{Requirement: ComplianceRequirement{ID: "1", Severity: "critical"}, Status: StatusNonCompliant},
			{Requirement: ComplianceRequirement{ID: "2", Severity: "high"}, Status: StatusNonCompliant},
			{Requirement: ComplianceRequirement{ID: "3", Severity: "critical"}, Status: StatusCompliant},
		},
	}

	critical := report.GetCriticalIssues()
	if len(critical) != 1 {
		t.Errorf("expected 1 critical issue, got %d", len(critical))
	}
}

func TestComplianceReport_IsCompliant(t *testing.T) {
	tests := []struct {
		name       string
		percentage float64
		threshold  float64
		want       bool
	}{
		{"above threshold", 85.0, 80.0, true},
		{"at threshold", 80.0, 80.0, true},
		{"below threshold", 75.0, 80.0, false},
		{"zero threshold", 50.0, 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &ComplianceReport{
				Summary: ComplianceSummary{
					CompliancePercentage: tt.percentage,
				},
			}

			got := report.IsCompliant(tt.threshold)
			if got != tt.want {
				t.Errorf("IsCompliant(%v) = %v, want %v", tt.threshold, got, tt.want)
			}
		})
	}
}

func TestComplianceSummary_RiskLevel(t *testing.T) {
	reporter := NewComplianceReporter(DefaultComplianceConfig(), nil)

	tests := []struct {
		name     string
		results  []ComplianceCheckResult
		wantRisk string
	}{
		{
			name: "critical issues",
			results: []ComplianceCheckResult{
				{Requirement: ComplianceRequirement{Severity: "critical"}, Status: StatusNonCompliant},
			},
			wantRisk: "critical",
		},
		{
			name: "high issues only",
			results: []ComplianceCheckResult{
				{Requirement: ComplianceRequirement{Severity: "high"}, Status: StatusNonCompliant},
			},
			wantRisk: "high",
		},
		{
			name: "medium issues only",
			results: []ComplianceCheckResult{
				{Requirement: ComplianceRequirement{Severity: "medium"}, Status: StatusNonCompliant},
			},
			wantRisk: "medium",
		},
		{
			name: "all compliant",
			results: []ComplianceCheckResult{
				{Requirement: ComplianceRequirement{Severity: "critical"}, Status: StatusCompliant},
			},
			wantRisk: "low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := reporter.calculateSummary(tt.results)
			if summary.RiskLevel != tt.wantRisk {
				t.Errorf("expected risk level %s, got %s", tt.wantRisk, summary.RiskLevel)
			}
		})
	}
}

func TestComplianceReporter_AccessAuditSummary(t *testing.T) {
	reporter := NewComplianceReporter(DefaultComplianceConfig(), nil)

	// Record various access patterns
	for i := 0; i < 10; i++ {
		reporter.RecordAccess("user1", "/secrets/prod/db", "read", true)
	}
	for i := 0; i < 5; i++ {
		reporter.RecordAccess("user2", "/secrets/prod/api", "read", true)
	}
	reporter.RecordAccess("user3", "/secrets/dev/test", "read", false)

	period := ReportPeriod{
		Start: time.Now().Add(-24 * time.Hour),
		End:   time.Now(),
	}

	report, err := reporter.GenerateReport(context.Background(), FrameworkSOC2, period)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	audit := report.AccessAudit
	if audit.TotalAccesses != 16 {
		t.Errorf("expected 16 total accesses, got %d", audit.TotalAccesses)
	}
	if audit.UniquePrincipals != 3 {
		t.Errorf("expected 3 unique principals, got %d", audit.UniquePrincipals)
	}
	if audit.FailedAttempts != 1 {
		t.Errorf("expected 1 failed attempt, got %d", audit.FailedAttempts)
	}
	if len(audit.TopAccessors) == 0 {
		t.Error("expected top accessors")
	}
}

func TestComplianceReporter_RotationSummary(t *testing.T) {
	config := DefaultComplianceConfig()
	config.RotationMaxAge = 90 * 24 * time.Hour

	reporter := NewComplianceReporter(config, nil)

	now := time.Now()
	lastRotated := now.Add(-30 * 24 * time.Hour)
	pendingRotation := now.Add(15 * 24 * time.Hour) // within 30 days

	// Key 1: will be rotated
	reporter.RegisterKey(KeyInventoryItem{
		KeyID:        "key-1",
		Algorithm:    "aes-256-gcm",
		KeySize:      256,
		CreatedAt:    now.Add(-120 * 24 * time.Hour),
		LastRotated:  &lastRotated,
		NextRotation: &pendingRotation,
		Status:       "active",
	})

	// Key 2: pending rotation (within 30 days)
	pendingRotation2 := now.Add(20 * 24 * time.Hour)
	reporter.RegisterKey(KeyInventoryItem{
		KeyID:        "key-2",
		Algorithm:    "aes-256-gcm",
		KeySize:      256,
		CreatedAt:    now.Add(-70 * 24 * time.Hour),
		LastRotated:  &lastRotated,
		NextRotation: &pendingRotation2,
		Status:       "active",
	})

	reporter.RecordRotation(RotationEvent{
		KeyID:      "key-1",
		RotatedAt:  now.Add(-2 * 24 * time.Hour),
		RotatedBy:  "admin",
		OldVersion: 1,
		NewVersion: 2,
		Reason:     "scheduled",
	})

	period := ReportPeriod{
		Start: now.Add(-7 * 24 * time.Hour),
		End:   now,
	}

	report, err := reporter.GenerateReport(context.Background(), FrameworkSOC2, period)
	if err != nil {
		t.Fatalf("GenerateReport failed: %v", err)
	}

	rotation := report.Rotation
	if rotation.TotalKeys != 2 {
		t.Errorf("expected 2 total keys, got %d", rotation.TotalKeys)
	}
	if rotation.KeysRotatedInPeriod != 1 {
		t.Errorf("expected 1 key rotated in period, got %d", rotation.KeysRotatedInPeriod)
	}
	if rotation.KeysPendingRotation != 1 {
		t.Errorf("expected 1 key pending rotation, got %d", rotation.KeysPendingRotation)
	}
}

func TestDefaultComplianceConfig(t *testing.T) {
	config := DefaultComplianceConfig()

	if len(config.Frameworks) == 0 {
		t.Error("expected default frameworks")
	}
	if config.RotationMaxAge == 0 {
		t.Error("expected non-zero rotation max age")
	}
	if config.MinKeySize == 0 {
		t.Error("expected non-zero min key size")
	}
	if config.AuditRetention == 0 {
		t.Error("expected non-zero audit retention")
	}
}

func BenchmarkComplianceReporter_GenerateReport(b *testing.B) {
	config := DefaultComplianceConfig()
	reporter := NewComplianceReporter(config, nil)

	// Register many keys
	now := time.Now()
	for i := 0; i < 100; i++ {
		lastRotated := now.Add(-time.Duration(i) * 24 * time.Hour)
		nextRotation := now.Add(time.Duration(365-i) * 24 * time.Hour)

		reporter.RegisterKey(KeyInventoryItem{
			KeyID:        fmt.Sprintf("key-%d", i),
			KeyType:      "symmetric",
			Algorithm:    "aes-256-gcm",
			KeySize:      256,
			CreatedAt:    now.Add(-time.Duration(i+30) * 24 * time.Hour),
			LastRotated:  &lastRotated,
			NextRotation: &nextRotation,
			Status:       "active",
		})
	}

	// Record many accesses
	for i := 0; i < 1000; i++ {
		reporter.RecordAccess(
			fmt.Sprintf("user-%d", i%10),
			fmt.Sprintf("/secrets/path-%d", i%50),
			"read",
			i%20 != 0,
		)
	}

	period := ReportPeriod{
		Start: now.Add(-30 * 24 * time.Hour),
		End:   now,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := reporter.GenerateReport(ctx, FrameworkSOC2, period)
		if err != nil {
			b.Fatalf("GenerateReport failed: %v", err)
		}
	}
}

func BenchmarkComplianceReporter_RecordAccess(b *testing.B) {
	reporter := NewComplianceReporter(DefaultComplianceConfig(), nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reporter.RecordAccess(
			fmt.Sprintf("user-%d", i%100),
			fmt.Sprintf("/secrets/path-%d", i%500),
			"read",
			true,
		)
	}
}
