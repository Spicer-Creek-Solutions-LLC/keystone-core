// Package blueprint provides tests for breaking change detection.
package blueprint

import (
	"strings"
	"testing"
)

func TestNewBreakingChangeDetector(t *testing.T) {
	d := NewBreakingChangeDetector()
	if d == nil {
		t.Fatal("NewBreakingChangeDetector returned nil")
	}
}

func TestGetMajorVersion(t *testing.T) {
	tests := []struct {
		version string
		want    int
	}{
		{"1.0.0", 1},
		{"2.5.3", 2},
		{"v1.0.0", 1},
		{"v3.2.1", 3},
		{"0.1.0", 0},
		{"10.0.0", 10},
		{"", 0},
		{"invalid", 0},
	}

	for _, tt := range tests {
		got := getMajorVersion(tt.version)
		if got != tt.want {
			t.Errorf("getMajorVersion(%q) = %d, want %d", tt.version, got, tt.want)
		}
	}
}

func TestTitleCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"low", "Low"},
		{"medium", "Medium"},
		{"high", "High"},
		{"critical", "Critical"},
		{"", ""},
		{"a", "A"},
	}

	for _, tt := range tests {
		got := titleCase(tt.input)
		if got != tt.want {
			t.Errorf("titleCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBreakingChangeDetector_MajorVersion(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
	}
	new := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "2.0.0"},
	}

	report := detector.Detect(old, new)

	if !report.HasBreakingChanges {
		t.Error("Expected breaking changes for major version bump")
	}

	found := false
	for _, change := range report.Changes {
		if change.Type == BreakingMajorVersion {
			found = true
			if change.Severity != SeverityHigh {
				t.Errorf("Major version change should be high severity, got %s", change.Severity)
			}
		}
	}
	if !found {
		t.Error("Expected BreakingMajorVersion change")
	}
}

func TestBreakingChangeDetector_NoMajorVersion(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
	}
	new := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.1.0"},
	}

	report := detector.Detect(old, new)

	for _, change := range report.Changes {
		if change.Type == BreakingMajorVersion {
			t.Error("No breaking change expected for minor version bump")
		}
	}
}

func TestBreakingChangeDetector_ParameterRemoved(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
		Parameters: map[string]ParameterSchema{
			"old_param": {Type: "string"},
			"kept":      {Type: "string"},
		},
	}
	new := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.1.0"},
		Parameters: map[string]ParameterSchema{
			"kept": {Type: "string"},
		},
	}

	report := detector.Detect(old, new)

	if !report.HasBreakingChanges {
		t.Error("Expected breaking changes for parameter removal")
	}

	found := false
	for _, change := range report.Changes {
		if change.Type == BreakingParameterRemoved && change.AffectedItem == "old_param" {
			found = true
			if !change.AutoFixable {
				t.Error("Parameter removal should be auto-fixable")
			}
		}
	}
	if !found {
		t.Error("Expected BreakingParameterRemoved change for old_param")
	}
}

func TestBreakingChangeDetector_ParameterTypeChanged(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
		Parameters: map[string]ParameterSchema{
			"param": {Type: "string"},
		},
	}
	new := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.1.0"},
		Parameters: map[string]ParameterSchema{
			"param": {Type: "integer"},
		},
	}

	report := detector.Detect(old, new)

	if !report.HasBreakingChanges {
		t.Error("Expected breaking changes for parameter type change")
	}

	found := false
	for _, change := range report.Changes {
		if change.Type == BreakingParameterTypeChanged && change.AffectedItem == "param" {
			found = true
			if change.OldValue != "string" || change.NewValue != "integer" {
				t.Errorf("Expected old=string new=integer, got old=%s new=%s", change.OldValue, change.NewValue)
			}
		}
	}
	if !found {
		t.Error("Expected BreakingParameterTypeChanged change")
	}
}

func TestBreakingChangeDetector_ParameterBecameRequired(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
		Parameters: map[string]ParameterSchema{
			"param": {Type: "string", Required: false},
		},
	}
	new := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.1.0"},
		Parameters: map[string]ParameterSchema{
			"param": {Type: "string", Required: true},
		},
	}

	report := detector.Detect(old, new)

	if !report.HasBreakingChanges {
		t.Error("Expected breaking changes for parameter becoming required")
	}

	found := false
	for _, change := range report.Changes {
		if change.Type == BreakingParameterRequired && change.AffectedItem == "param" {
			found = true
		}
	}
	if !found {
		t.Error("Expected BreakingParameterRequired change")
	}
}

func TestBreakingChangeDetector_NewRequiredParameter(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata:   Metadata{Name: "test", Version: "1.0.0"},
		Parameters: map[string]ParameterSchema{},
	}
	new := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.1.0"},
		Parameters: map[string]ParameterSchema{
			"new_param": {Type: "string", Required: true},
		},
	}

	report := detector.Detect(old, new)

	if !report.HasBreakingChanges {
		t.Error("Expected breaking changes for new required parameter")
	}

	found := false
	for _, change := range report.Changes {
		if change.Type == BreakingParameterRequired && change.AffectedItem == "new_param" {
			found = true
		}
	}
	if !found {
		t.Error("Expected BreakingParameterRequired change for new_param")
	}
}

func TestBreakingChangeDetector_FeatureRemoved(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
		Features: map[string]Feature{
			"feature1": {Description: "Feature 1"},
			"feature2": {Description: "Feature 2"},
		},
	}
	new := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.1.0"},
		Features: map[string]Feature{
			"feature2": {Description: "Feature 2"},
		},
	}

	report := detector.Detect(old, new)

	if !report.HasBreakingChanges {
		t.Error("Expected breaking changes for feature removal")
	}

	found := false
	for _, change := range report.Changes {
		if change.Type == BreakingFeatureRemoved && change.AffectedItem == "feature1" {
			found = true
			if !change.AutoFixable {
				t.Error("Feature removal should be auto-fixable")
			}
		}
	}
	if !found {
		t.Error("Expected BreakingFeatureRemoved change for feature1")
	}
}

func TestBreakingChangeDetector_DependencyRemoved(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
		Dependencies: &Dependencies{
			Requires: []string{"dep1", "dep2"},
		},
	}
	new := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.1.0"},
		Dependencies: &Dependencies{
			Requires: []string{"dep2"},
		},
	}

	report := detector.Detect(old, new)

	if !report.HasBreakingChanges {
		t.Error("Expected breaking changes for dependency removal")
	}

	found := false
	for _, change := range report.Changes {
		if change.Type == BreakingDependencyRemoved && change.AffectedItem == "dep1" {
			found = true
		}
	}
	if !found {
		t.Error("Expected BreakingDependencyRemoved change for dep1")
	}
}

func TestBreakingChangeDetector_EntrypointRemoved(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
		Entrypoints: map[string]string{
			"default": "states/main.yaml",
			"setup":   "states/setup.yaml",
		},
	}
	new := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.1.0"},
		Entrypoints: map[string]string{
			"default": "states/main.yaml",
		},
	}

	report := detector.Detect(old, new)

	if !report.HasBreakingChanges {
		t.Error("Expected breaking changes for entrypoint removal")
	}

	found := false
	for _, change := range report.Changes {
		if change.Type == BreakingEntrypointRemoved && change.AffectedItem == "setup" {
			found = true
			if change.Severity != SeverityHigh {
				t.Errorf("Entrypoint removal should be high severity, got %s", change.Severity)
			}
		}
	}
	if !found {
		t.Error("Expected BreakingEntrypointRemoved change for setup")
	}
}

func TestBreakingChangeDetector_StateFileRemoved(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
		Entrypoints: map[string]string{
			"default": "states/main.yaml",
			"setup":   "states/setup.yaml",
		},
	}
	new := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.1.0"},
		Entrypoints: map[string]string{
			"default": "states/new_main.yaml",
		},
	}

	report := detector.Detect(old, new)

	if !report.HasBreakingChanges {
		t.Error("Expected breaking changes for state file removal")
	}

	// Should detect both states/main.yaml and states/setup.yaml as removed
	removedCount := 0
	for _, change := range report.Changes {
		if change.Type == BreakingStateRemoved {
			removedCount++
		}
	}
	if removedCount != 2 {
		t.Errorf("Expected 2 state file removals, got %d", removedCount)
	}
}

func TestBreakingChangeDetector_NoChanges(t *testing.T) {
	detector := NewBreakingChangeDetector()

	bp := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
		Parameters: map[string]ParameterSchema{
			"param": {Type: "string"},
		},
		Features: map[string]Feature{
			"feature": {Description: "test"},
		},
	}

	report := detector.Detect(bp, bp)

	if report.HasBreakingChanges {
		t.Error("No breaking changes expected for identical blueprints")
	}
	if len(report.Changes) != 0 {
		t.Errorf("Expected 0 changes, got %d", len(report.Changes))
	}
}

func TestBreakingChangeDetector_HighestSeverity(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
		Parameters: map[string]ParameterSchema{
			"param": {Type: "string"},
		},
		Entrypoints: map[string]string{
			"setup": "states/setup.yaml",
		},
	}
	new := &Blueprint{
		Metadata:    Metadata{Name: "test", Version: "2.0.0"},
		Parameters:  map[string]ParameterSchema{},
		Entrypoints: map[string]string{},
	}

	report := detector.Detect(old, new)

	if report.HighestSeverity != SeverityHigh {
		t.Errorf("Expected highest severity to be high, got %s", report.HighestSeverity)
	}
	if !report.RequiresAcknowledgment {
		t.Error("Expected RequiresAcknowledgment to be true for high severity")
	}
}

func TestGenerateMigrationGuide_NoChanges(t *testing.T) {
	report := &BreakingChangeReport{
		FromVersion:        "1.0.0",
		ToVersion:          "1.1.0",
		BlueprintName:      "test",
		HasBreakingChanges: false,
	}

	guide := GenerateMigrationGuide(report)

	if !strings.Contains(guide, "No breaking changes detected") {
		t.Error("Expected 'No breaking changes' message")
	}
	if !strings.Contains(guide, "Safe to upgrade") {
		t.Error("Expected 'Safe to upgrade' message")
	}
}

func TestGenerateMigrationGuide_WithChanges(t *testing.T) {
	report := &BreakingChangeReport{
		FromVersion:            "1.0.0",
		ToVersion:              "2.0.0",
		BlueprintName:          "test",
		HasBreakingChanges:     true,
		HighestSeverity:        SeverityHigh,
		RequiresAcknowledgment: true,
		Changes: []BreakingChange{
			{
				Type:         BreakingMajorVersion,
				Severity:     SeverityHigh,
				Description:  "Major version upgrade",
				Migration:    "Review changelog",
				AutoFixable:  false,
			},
			{
				Type:         BreakingParameterRemoved,
				Severity:     SeverityMedium,
				Description:  "Parameter removed",
				AffectedItem: "old_param",
				Migration:    "Remove parameter",
				AutoFixable:  true,
			},
		},
	}

	guide := GenerateMigrationGuide(report)

	if !strings.Contains(guide, "Migration Guide") {
		t.Error("Expected 'Migration Guide' header")
	}
	if !strings.Contains(guide, "High Severity") {
		t.Error("Expected 'High Severity' section")
	}
	if !strings.Contains(guide, "Medium Severity") {
		t.Error("Expected 'Medium Severity' section")
	}
	if !strings.Contains(guide, "--accept-breaking-changes") {
		t.Error("Expected '--accept-breaking-changes' mention")
	}
	if !strings.Contains(guide, "Recommended Steps") {
		t.Error("Expected 'Recommended Steps' section")
	}
}

func TestSortChangesBySeverity(t *testing.T) {
	changes := []BreakingChange{
		{Severity: SeverityLow},
		{Severity: SeverityCritical},
		{Severity: SeverityMedium},
		{Severity: SeverityHigh},
	}

	SortChangesBySeverity(changes)

	expected := []BreakingChangeSeverity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow}
	for i, change := range changes {
		if change.Severity != expected[i] {
			t.Errorf("At index %d: expected %s, got %s", i, expected[i], change.Severity)
		}
	}
}

func TestBreakingChangeDetector_NilDependencies(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
		// Dependencies is nil
	}
	new := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.1.0"},
		Dependencies: &Dependencies{
			Requires: []string{"new-dep"},
		},
	}

	// Should not panic
	report := detector.Detect(old, new)
	if report == nil {
		t.Fatal("Report should not be nil")
	}
}

func TestBreakingChangeDetector_EmptyBlueprints(t *testing.T) {
	detector := NewBreakingChangeDetector()

	old := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.0"},
	}
	new := &Blueprint{
		Metadata: Metadata{Name: "test", Version: "1.0.1"},
	}

	report := detector.Detect(old, new)

	if report.HasBreakingChanges {
		t.Error("No breaking changes expected for empty blueprints")
	}
	if report.BlueprintName != "test" {
		t.Errorf("Expected blueprint name 'test', got '%s'", report.BlueprintName)
	}
}

func TestBreakingChangeTypes(t *testing.T) {
	// Verify constants are defined correctly
	types := []BreakingChangeType{
		BreakingMajorVersion,
		BreakingParameterRemoved,
		BreakingParameterTypeChanged,
		BreakingParameterRequired,
		BreakingFeatureRemoved,
		BreakingDependencyRemoved,
		BreakingEntrypointRemoved,
		BreakingStateRemoved,
		BreakingBehaviorChanged,
	}

	for _, ct := range types {
		if ct == "" {
			t.Error("Breaking change type should not be empty")
		}
	}
}

func TestBreakingChangeSeverities(t *testing.T) {
	// Verify severity levels are defined correctly
	severities := []BreakingChangeSeverity{
		SeverityLow,
		SeverityMedium,
		SeverityHigh,
		SeverityCritical,
	}

	for _, s := range severities {
		if s == "" {
			t.Error("Severity should not be empty")
		}
	}
}
