package statemgmt

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
)

// Test helper functions
func createTestFile(t *testing.T, path string, contents string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatalf("Failed to create test file %s: %v", path, err)
	}
}

func removeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Logf("Warning: failed to remove test file %s: %v", path, err)
	}
}

func TestStateDiffer_NoDrift(t *testing.T) {
	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					Module: "file",
					ID:     "/tmp/test.txt",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test",
					},
				},
			},
		},
	}

	// Create test file
	testFile := "/tmp/kscore-test-nodrift.txt"
	createTestFile(t, testFile, "test", 0644)
	defer removeTestFile(t, testFile)

	// Update state to use test file
	stateFile.States["file"][0].ID = testFile

	differ := NewStateDiffer()
	report, err := differ.CheckDrift(stateFile)
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}

	if report.Summary.Total != 1 {
		t.Errorf("Expected 1 state, got %d", report.Summary.Total)
	}

	if report.Summary.NoDrift != 1 {
		t.Errorf("Expected no drift, got %d states with drift", report.Summary.Total-report.Summary.NoDrift)
	}

	if report.Summary.OverallSeverity != DriftNone {
		t.Errorf("Expected DriftNone, got %s", report.Summary.OverallSeverity)
	}
}

func TestStateDiffer_ContentDrift(t *testing.T) {
	testFile := "/tmp/kscore-test-contentdrift.txt"
	createTestFile(t, testFile, "old content", 0644)
	defer removeTestFile(t, testFile)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					Module: "file",
					ID:     testFile,
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "new content",
					},
				},
			},
		},
	}

	differ := NewStateDiffer()
	report, err := differ.CheckDrift(stateFile)
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}

	if report.Summary.NoDrift != 0 {
		t.Error("Expected drift to be detected")
	}

	if len(report.States) != 1 {
		t.Fatalf("Expected 1 state, got %d", len(report.States))
	}

	status := report.States[0]
	if !status.HasDrift {
		t.Error("Expected HasDrift to be true")
	}

	if status.Severity == DriftNone {
		t.Error("Expected drift severity to be set")
	}

	if len(status.Differences) == 0 {
		t.Error("Expected differences to be detected")
	}
}

func TestStateDiffer_MissingFile(t *testing.T) {
	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					Module: "file",
					ID:     "/tmp/kscore-nonexistent-file.txt",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test",
					},
				},
			},
		},
	}

	differ := NewStateDiffer()
	report, err := differ.CheckDrift(stateFile)
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}

	if report.Summary.NoDrift != 0 {
		t.Error("Expected drift for missing file")
	}

	status := report.States[0]
	if !status.HasDrift {
		t.Error("Expected HasDrift for missing file")
	}
}

func TestStateDiffer_MultipleStates(t *testing.T) {
	testFile1 := "/tmp/kscore-test-multi1.txt"
	testFile2 := "/tmp/kscore-test-multi2.txt"

	createTestFile(t, testFile1, "correct", 0644)
	createTestFile(t, testFile2, "wrong", 0644)
	defer removeTestFile(t, testFile1)
	defer removeTestFile(t, testFile2)

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					Module: "file",
					ID:     testFile1,
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "correct",
					},
				},
				{
					Module: "file",
					ID:     testFile2,
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "corrected",
					},
				},
			},
		},
	}

	differ := NewStateDiffer()
	report, err := differ.CheckDrift(stateFile)
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}

	if report.Summary.Total != 2 {
		t.Errorf("Expected 2 states, got %d", report.Summary.Total)
	}

	if report.Summary.NoDrift != 1 {
		t.Errorf("Expected 1 state with no drift, got %d", report.Summary.NoDrift)
	}

	// One state should have drift
	driftCount := report.Summary.Total - report.Summary.NoDrift
	if driftCount != 1 {
		t.Errorf("Expected 1 state with drift, got %d", driftCount)
	}
}

func TestDriftSeverity_Critical(t *testing.T) {
	differ := NewStateDiffer()

	// Test critical severity for permission changes
	severity := differ.calculateDiffSeverity("file", "mode", "0644", "0777")
	if severity != DriftHigh {
		t.Errorf("Expected DriftHigh for mode change, got %s", severity)
	}

	// Test critical severity for owner changes
	severity = differ.calculateDiffSeverity("file", "owner", "root", "user")
	if severity != DriftHigh {
		t.Errorf("Expected DriftHigh for owner change, got %s", severity)
	}
}

func TestDriftSeverity_Medium(t *testing.T) {
	differ := NewStateDiffer()

	// Test medium severity for content changes
	severity := differ.calculateDiffSeverity("file", "contents", "old", "new")
	if severity != DriftMedium {
		t.Errorf("Expected DriftMedium for contents change, got %s", severity)
	}
}

func TestDriftSeverity_Low(t *testing.T) {
	differ := NewStateDiffer()

	// Test low severity for other fields
	severity := differ.calculateDiffSeverity("file", "comment", "old", "new")
	if severity != DriftLow {
		t.Errorf("Expected DriftLow for comment change, got %s", severity)
	}
}

func TestCompareStates_Identical(t *testing.T) {
	state1 := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test",
		State:  "present",
		Parameters: map[string]interface{}{
			"contents": "test",
			"mode":     "0644",
		},
	}

	state2 := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test",
		State:  "present",
		Parameters: map[string]interface{}{
			"contents": "test",
			"mode":     "0644",
		},
	}

	diffs := CompareStates(state1, state2)
	if len(diffs) != 0 {
		t.Errorf("Expected no differences, got %d", len(diffs))
	}
}

func TestCompareStates_DifferentState(t *testing.T) {
	state1 := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test",
		State:  "present",
	}

	state2 := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test",
		State:  "absent",
	}

	diffs := CompareStates(state1, state2)
	if len(diffs) == 0 {
		t.Error("Expected differences to be detected")
	}

	// Should have high severity for state change
	found := false
	for _, diff := range diffs {
		if diff.Path == "state" && diff.Severity == DriftHigh {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected high severity diff for state change")
	}
}

func TestCompareStates_DifferentParameters(t *testing.T) {
	state1 := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test",
		State:  "present",
		Parameters: map[string]interface{}{
			"contents": "old",
			"mode":     "0644",
		},
	}

	state2 := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test",
		State:  "present",
		Parameters: map[string]interface{}{
			"contents": "new",
			"mode":     "0644",
		},
	}

	diffs := CompareStates(state1, state2)
	if len(diffs) == 0 {
		t.Error("Expected differences in contents")
	}

	// Check that contents difference is detected
	found := false
	for _, diff := range diffs {
		if diff.Path == "contents" {
			found = true
			if diff.Desired != "old" || diff.Actual != "new" {
				t.Errorf("Expected desired='old', actual='new', got desired=%v, actual=%v", diff.Desired, diff.Actual)
			}
			break
		}
	}
	if !found {
		t.Error("Expected contents difference to be detected")
	}
}

func TestCompareStates_MissingParameter(t *testing.T) {
	state1 := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test",
		State:  "present",
		Parameters: map[string]interface{}{
			"contents": "test",
			"mode":     "0644",
		},
	}

	state2 := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test",
		State:  "present",
		Parameters: map[string]interface{}{
			"contents": "test",
		},
	}

	diffs := CompareStates(state1, state2)
	if len(diffs) == 0 {
		t.Error("Expected difference for missing mode parameter")
	}

	// Check that mode is reported as missing
	found := false
	for _, diff := range diffs {
		if diff.Path == "mode" && diff.Actual == nil {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected mode to be reported as missing")
	}
}

func TestCompareStates_ExtraParameter(t *testing.T) {
	state1 := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test",
		State:  "present",
		Parameters: map[string]interface{}{
			"contents": "test",
		},
	}

	state2 := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test",
		State:  "present",
		Parameters: map[string]interface{}{
			"contents": "test",
			"extra":    "parameter",
		},
	}

	diffs := CompareStates(state1, state2)
	if len(diffs) == 0 {
		t.Error("Expected difference for extra parameter")
	}

	// Check that extra parameter is reported
	found := false
	for _, diff := range diffs {
		if diff.Path == "extra" && diff.Desired == nil {
			found = true
			if diff.Severity != DriftLow {
				t.Errorf("Expected low severity for extra parameter, got %s", diff.Severity)
			}
			break
		}
	}
	if !found {
		t.Error("Expected extra parameter to be reported")
	}
}

func TestFormatDriftReport(t *testing.T) {
	report := &DriftReport{
		RunID:     "test-123",
		CheckedAt: time.Now(),
		Duration:  100 * time.Millisecond,
		States: []*DriftStatus{
			{
				StateID:  "/tmp/test",
				Module:   "file",
				HasDrift: true,
				Severity: DriftMedium,
				Differences: []Difference{
					{
						Path:     "contents",
						Desired:  "old",
						Actual:   "new",
						Severity: DriftMedium,
						Message:  "contents: expected old, got new",
					},
				},
			},
		},
		Summary: &DriftSummary{
			Total:           1,
			NoDrift:         0,
			MediumDrift:     1,
			OverallSeverity: DriftMedium,
		},
	}

	output := FormatDriftReport(report)

	// Check that output contains expected sections
	if !strings.Contains(output, "Drift Report") {
		t.Error("Expected output to contain 'Drift Report'")
	}

	if !strings.Contains(output, "test-123") {
		t.Error("Expected output to contain run ID")
	}

	if !strings.Contains(output, "Summary") {
		t.Error("Expected output to contain 'Summary'")
	}

	if !strings.Contains(output, "file./tmp/test") {
		t.Error("Expected output to contain state ID")
	}

	if !strings.Contains(output, "contents: expected old, got new") {
		t.Error("Expected output to contain difference message")
	}
}

func TestFormatDriftReport_NoDrift(t *testing.T) {
	report := &DriftReport{
		RunID:     "test-456",
		CheckedAt: time.Now(),
		Duration:  50 * time.Millisecond,
		States: []*DriftStatus{
			{
				StateID:  "/tmp/test",
				Module:   "file",
				HasDrift: false,
				Severity: DriftNone,
			},
		},
		Summary: &DriftSummary{
			Total:           1,
			NoDrift:         1,
			OverallSeverity: DriftNone,
		},
	}

	output := FormatDriftReport(report)

	if !strings.Contains(output, "No drift detected") {
		t.Error("Expected output to contain 'No drift detected'")
	}
}

func TestDriftSummary_OverallSeverity(t *testing.T) {
	differ := NewStateDiffer()

	report := &DriftReport{
		States: []*DriftStatus{
			{Severity: DriftLow},
			{Severity: DriftMedium},
			{Severity: DriftHigh},
			{Severity: DriftNone},
		},
	}

	summary := differ.calculateSummary(report)

	if summary.OverallSeverity != DriftHigh {
		t.Errorf("Expected OverallSeverity to be DriftHigh, got %s", summary.OverallSeverity)
	}

	if summary.Total != 4 {
		t.Errorf("Expected Total to be 4, got %d", summary.Total)
	}

	if summary.LowDrift != 1 {
		t.Errorf("Expected LowDrift to be 1, got %d", summary.LowDrift)
	}

	if summary.MediumDrift != 1 {
		t.Errorf("Expected MediumDrift to be 1, got %d", summary.MediumDrift)
	}

	if summary.HighDrift != 1 {
		t.Errorf("Expected HighDrift to be 1, got %d", summary.HighDrift)
	}

	if summary.NoDrift != 1 {
		t.Errorf("Expected NoDrift to be 1, got %d", summary.NoDrift)
	}
}

func TestStateDiffer_FormatDiffMessage(t *testing.T) {
	differ := NewStateDiffer()

	tests := []struct {
		name     string
		field    string
		desired  interface{}
		actual   interface{}
		expected string
	}{
		{
			name:     "string values",
			field:    "contents",
			desired:  "expected text",
			actual:   "actual text",
			expected: "contents: expected expected text, got actual text",
		},
		{
			name:     "int values",
			field:    "mode",
			desired:  0644,
			actual:   0755,
			expected: "mode: expected 420, got 493",
		},
		{
			name:     "bool values",
			field:    "enabled",
			desired:  true,
			actual:   false,
			expected: "enabled: expected true, got false",
		},
		{
			name:     "nil values",
			field:    "owner",
			desired:  "root",
			actual:   nil,
			expected: "owner: expected root, got <nil>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := differ.formatDiffMessage(tt.field, tt.desired, tt.actual)
			if result != tt.expected {
				t.Errorf("formatDiffMessage() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestStateDiffer_SeverityRank(t *testing.T) {
	differ := NewStateDiffer()

	tests := []struct {
		name     string
		severity DriftSeverity
		expected int
	}{
		{name: "DriftNone", severity: DriftNone, expected: 0},
		{name: "DriftLow", severity: DriftLow, expected: 1},
		{name: "DriftMedium", severity: DriftMedium, expected: 2},
		{name: "DriftHigh", severity: DriftHigh, expected: 3},
		{name: "DriftCritical", severity: DriftCritical, expected: 4},
		{name: "unknown severity", severity: DriftSeverity("unknown"), expected: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := differ.severityRank(tt.severity)
			if result != tt.expected {
				t.Errorf("severityRank(%s) = %d, want %d", tt.severity, result, tt.expected)
			}
		})
	}
}

func TestStateDiffer_CalculateSummary_CriticalDrift(t *testing.T) {
	differ := NewStateDiffer()

	report := &DriftReport{
		States: []*DriftStatus{
			{Severity: DriftCritical},
			{Severity: DriftLow},
		},
	}

	summary := differ.calculateSummary(report)

	if summary.OverallSeverity != DriftCritical {
		t.Errorf("Expected OverallSeverity to be DriftCritical, got %s", summary.OverallSeverity)
	}

	if summary.CriticalDrift != 1 {
		t.Errorf("Expected CriticalDrift to be 1, got %d", summary.CriticalDrift)
	}
}

func TestStateDiffer_CalculateSummary_Empty(t *testing.T) {
	differ := NewStateDiffer()

	report := &DriftReport{
		States: []*DriftStatus{},
	}

	summary := differ.calculateSummary(report)

	if summary.Total != 0 {
		t.Errorf("Expected Total to be 0, got %d", summary.Total)
	}

	if summary.OverallSeverity != DriftNone {
		t.Errorf("Expected OverallSeverity to be DriftNone, got %s", summary.OverallSeverity)
	}
}

// MockEventPublisher is a mock implementation for testing
type MockEventPublisher struct {
	events       []*events.Event
	publishAsync bool
	publishErr   error
}

func (m *MockEventPublisher) Publish(event *events.Event) error {
	m.events = append(m.events, event)
	return m.publishErr
}

func (m *MockEventPublisher) PublishAsync(event *events.Event) error {
	m.publishAsync = true
	m.events = append(m.events, event)
	return m.publishErr
}

func (m *MockEventPublisher) Close() error {
	return nil
}

func TestEmitDriftEvent_WithPublisher(t *testing.T) {
	mock := &MockEventPublisher{}
	differ := &StateDiffer{
		Registry:       DefaultRegistry,
		EventPublisher: mock,
		EventSource:    "/test-source",
	}

	status := &DriftStatus{
		StateID:   "/tmp/test.txt",
		Module:    "file",
		HasDrift:  true,
		Severity:  DriftMedium,
		CheckedAt: time.Now(),
		Differences: []Difference{
			{
				Path:     "contents",
				Desired:  "expected",
				Actual:   "actual",
				Severity: DriftMedium,
				Message:  "contents changed",
			},
		},
	}

	differ.emitDriftEvent("test-run-id", status)

	if !mock.publishAsync {
		t.Error("Expected PublishAsync to be called")
	}

	if len(mock.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(mock.events))
	}
}

func TestEmitDriftEvent_WithoutPublisher(t *testing.T) {
	differ := &StateDiffer{
		Registry:       DefaultRegistry,
		EventPublisher: nil, // No publisher
	}

	status := &DriftStatus{
		StateID:  "/tmp/test.txt",
		Module:   "file",
		HasDrift: true,
		Severity: DriftMedium,
	}

	// Should not panic when EventPublisher is nil
	differ.emitDriftEvent("test-run-id", status)
}

func TestEmitDriftEvent_DefaultSource(t *testing.T) {
	mock := &MockEventPublisher{}
	differ := &StateDiffer{
		Registry:       DefaultRegistry,
		EventPublisher: mock,
		EventSource:    "", // Empty source should use default
	}

	status := &DriftStatus{
		StateID:  "/tmp/test.txt",
		Module:   "file",
		HasDrift: true,
		Severity: DriftLow,
	}

	differ.emitDriftEvent("test-run-id", status)

	if len(mock.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(mock.events))
	}
}

func TestEmitDriftEvent_AllSeverities(t *testing.T) {
	severities := []DriftSeverity{DriftCritical, DriftHigh, DriftMedium, DriftLow, DriftNone}

	for _, severity := range severities {
		t.Run(string(severity), func(t *testing.T) {
			mock := &MockEventPublisher{}
			differ := &StateDiffer{
				Registry:       DefaultRegistry,
				EventPublisher: mock,
			}

			status := &DriftStatus{
				StateID:  "/tmp/test.txt",
				Module:   "file",
				HasDrift: true,
				Severity: severity,
			}

			differ.emitDriftEvent("test-run-id", status)

			if len(mock.events) != 1 {
				t.Errorf("Expected 1 event for severity %s, got %d", severity, len(mock.events))
			}
		})
	}
}

func TestParseDifferences_StructuredDiff(t *testing.T) {
	differ := NewStateDiffer()

	decl := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test.txt",
		State:  "present",
	}

	diff := map[string]interface{}{
		"contents": map[string]interface{}{
			"desired": "expected content",
			"actual":  "actual content",
		},
		"mode": map[string]interface{}{
			"desired": "0644",
			"actual":  "0755",
		},
	}

	differences := differ.parseDifferences(decl, diff)

	if len(differences) != 2 {
		t.Errorf("Expected 2 differences, got %d", len(differences))
	}

	// Check that both differences are parsed correctly
	foundContents := false
	foundMode := false
	for _, d := range differences {
		if d.Path == "contents" {
			foundContents = true
			if d.Desired != "expected content" {
				t.Errorf("Expected desired='expected content', got %v", d.Desired)
			}
			if d.Actual != "actual content" {
				t.Errorf("Expected actual='actual content', got %v", d.Actual)
			}
		}
		if d.Path == "mode" {
			foundMode = true
			if d.Severity != DriftHigh {
				t.Errorf("Expected DriftHigh for mode, got %s", d.Severity)
			}
		}
	}

	if !foundContents {
		t.Error("Contents difference not found")
	}
	if !foundMode {
		t.Error("Mode difference not found")
	}
}

func TestParseDifferences_SimpleDiff(t *testing.T) {
	differ := NewStateDiffer()

	decl := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test.txt",
		State:  "present",
	}

	// Simple value diff (not structured map with desired/actual)
	diff := map[string]interface{}{
		"size":    12345,
		"changed": true,
	}

	differences := differ.parseDifferences(decl, diff)

	if len(differences) != 2 {
		t.Errorf("Expected 2 differences, got %d", len(differences))
	}

	// Check that simple values are handled
	for _, d := range differences {
		if d.Severity != DriftMedium {
			t.Errorf("Expected DriftMedium for simple diff, got %s", d.Severity)
		}
		if d.Desired != nil {
			t.Errorf("Expected nil desired for simple diff, got %v", d.Desired)
		}
	}
}

func TestParseDifferences_Empty(t *testing.T) {
	differ := NewStateDiffer()

	decl := &StateDeclaration{
		Module: "file",
		ID:     "/tmp/test.txt",
		State:  "present",
	}

	diff := map[string]interface{}{}

	differences := differ.parseDifferences(decl, diff)

	if len(differences) != 0 {
		t.Errorf("Expected 0 differences, got %d", len(differences))
	}
}

func TestCalculateDiffSeverity_ServiceFields(t *testing.T) {
	differ := NewStateDiffer()

	tests := []struct {
		module   string
		field    string
		expected DriftSeverity
	}{
		// Service critical fields
		{"service", "state", DriftHigh},
		{"service", "enabled", DriftHigh},
		// User critical fields
		{"user", "uid", DriftHigh},
		{"user", "shell", DriftHigh},
		// File critical fields
		{"file", "mode", DriftHigh},
		{"file", "owner", DriftHigh},
		{"file", "group", DriftHigh},
		// Non-critical fields
		{"service", "description", DriftLow},
		{"user", "comment", DriftLow},
		{"unknown", "anything", DriftLow},
	}

	for _, tt := range tests {
		t.Run(tt.module+"_"+tt.field, func(t *testing.T) {
			severity := differ.calculateDiffSeverity(tt.module, tt.field, "old", "new")
			if severity != tt.expected {
				t.Errorf("calculateDiffSeverity(%s, %s) = %s, want %s", tt.module, tt.field, severity, tt.expected)
			}
		})
	}
}

func TestCalculateDiffSeverity_ContentField(t *testing.T) {
	differ := NewStateDiffer()

	// Test both "contents" and "content" variations
	severity1 := differ.calculateDiffSeverity("file", "contents", "old", "new")
	if severity1 != DriftMedium {
		t.Errorf("Expected DriftMedium for contents, got %s", severity1)
	}

	severity2 := differ.calculateDiffSeverity("file", "content", "old", "new")
	if severity2 != DriftMedium {
		t.Errorf("Expected DriftMedium for content, got %s", severity2)
	}
}

func TestCalculateStateSeverity_AllBranches(t *testing.T) {
	differ := NewStateDiffer()

	tests := []struct {
		name        string
		decl        *StateDeclaration
		differences []Difference
		expected    DriftSeverity
	}{
		{
			name:        "empty differences",
			decl:        &StateDeclaration{Module: "file"},
			differences: []Difference{},
			expected:    DriftNone,
		},
		{
			name: "critical severity returns immediately",
			decl: &StateDeclaration{Module: "file"},
			differences: []Difference{
				{Severity: DriftLow},
				{Severity: DriftCritical},
				{Severity: DriftMedium},
			},
			expected: DriftCritical,
		},
		{
			name: "high severity with medium and low",
			decl: &StateDeclaration{Module: "file"},
			differences: []Difference{
				{Severity: DriftLow},
				{Severity: DriftMedium},
				{Severity: DriftHigh},
			},
			expected: DriftHigh,
		},
		{
			name: "medium severity upgrade from low",
			decl: &StateDeclaration{Module: "file"},
			differences: []Difference{
				{Severity: DriftLow},
				{Severity: DriftMedium},
			},
			expected: DriftMedium,
		},
		{
			name: "only low severity",
			decl: &StateDeclaration{Module: "file"},
			differences: []Difference{
				{Severity: DriftLow},
				{Severity: DriftLow},
			},
			expected: DriftLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := differ.calculateStateSeverity(tt.decl, tt.differences)
			if result != tt.expected {
				t.Errorf("calculateStateSeverity() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestGenerateRunID(t *testing.T) {
	id1 := generateRunID()
	if id1 == "" {
		t.Error("generateRunID returned empty string")
	}

	if !strings.HasPrefix(id1, "drift-") {
		t.Errorf("generateRunID should start with 'drift-', got %s", id1)
	}

	// Verify it contains a timestamp
	if len(id1) < 10 {
		t.Errorf("generateRunID seems too short: %s", id1)
	}
}

func TestCheckStateDrift_ModuleNotFound(t *testing.T) {
	differ := NewStateDiffer()

	decl := &StateDeclaration{
		Module: "nonexistent-module",
		ID:     "/tmp/test",
		State:  "present",
	}

	status, err := differ.checkStateDrift(decl)

	if err == nil {
		t.Error("Expected error for non-existent module")
	}

	if status != nil {
		t.Error("Expected nil status on error")
	}

	if !strings.Contains(err.Error(), "module not found") {
		t.Errorf("Expected 'module not found' error, got: %v", err)
	}
}

func TestCheckDrift_DependencyResolutionError(t *testing.T) {
	// Create a state file with circular dependencies
	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					Module: "file",
					ID:     "/tmp/a",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "file", ID: "/tmp/b"},
						},
					},
				},
				{
					Module: "file",
					ID:     "/tmp/b",
					State:  "present",
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "file", ID: "/tmp/a"},
						},
					},
				},
			},
		},
	}

	differ := NewStateDiffer()
	_, err := differ.CheckDrift(stateFile)

	if err == nil {
		t.Error("Expected error for circular dependencies")
	}

	if !strings.Contains(err.Error(), "circular") && !strings.Contains(err.Error(), "dependencies") {
		t.Errorf("Expected circular dependency error, got: %v", err)
	}
}

func TestStateDiffer_CheckDrift_WithEventPublisher(t *testing.T) {
	// Create a file with drift to trigger event emission
	testFile := "/tmp/kscore-test-event-drift.txt"
	createTestFile(t, testFile, "old content", 0644)
	defer removeTestFile(t, testFile)

	mock := &MockEventPublisher{}
	differ := &StateDiffer{
		Registry:       DefaultRegistry,
		EventPublisher: mock,
		EventSource:    "/test-differ",
	}

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					Module: "file",
					ID:     testFile,
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "new content",
					},
				},
			},
		},
	}

	report, err := differ.CheckDrift(stateFile)
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}

	// Should have drift
	if report.Summary.NoDrift == report.Summary.Total {
		t.Error("Expected drift to be detected")
	}

	// Should have emitted an event
	if len(mock.events) == 0 {
		t.Error("Expected drift event to be emitted")
	}
}

func TestCheckStateDrift_StateMismatchNoDiff(t *testing.T) {
	// Test the case where state doesn't match but diff is empty
	// This triggers the state mismatch fallback in checkStateDrift
	testFile := "/tmp/kscore-test-state-mismatch.txt"
	// Remove the file to create a "absent vs present" mismatch
	removeTestFile(t, testFile)

	differ := NewStateDiffer()

	stateFile := &StateFile{
		Path: "test.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					Module: "file",
					ID:     testFile,
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "test",
					},
				},
			},
		},
	}

	report, err := differ.CheckDrift(stateFile)
	if err != nil {
		t.Fatalf("CheckDrift failed: %v", err)
	}

	if len(report.States) != 1 {
		t.Fatalf("Expected 1 state, got %d", len(report.States))
	}

	status := report.States[0]
	if !status.HasDrift {
		t.Error("Expected HasDrift to be true")
	}

	// Should have at least one difference for state mismatch
	if len(status.Differences) == 0 {
		t.Error("Expected at least one difference")
	}
}
