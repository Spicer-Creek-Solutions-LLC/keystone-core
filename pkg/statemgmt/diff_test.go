package statemgmt

import (
	"os"
	"strings"
	"testing"
	"time"
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
	testFile := "/tmp/titan-test-nodrift.txt"
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
	testFile := "/tmp/titan-test-contentdrift.txt"
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
					ID:     "/tmp/titan-nonexistent-file.txt",
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
	testFile1 := "/tmp/titan-test-multi1.txt"
	testFile2 := "/tmp/titan-test-multi2.txt"

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
