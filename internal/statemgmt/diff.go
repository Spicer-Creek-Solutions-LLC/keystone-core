package statemgmt

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
)

// DriftSeverity indicates the severity level of drift
type DriftSeverity string

const (
	DriftNone     DriftSeverity = "none"
	DriftLow      DriftSeverity = "low"
	DriftMedium   DriftSeverity = "medium"
	DriftHigh     DriftSeverity = "high"
	DriftCritical DriftSeverity = "critical"
)

// DriftStatus represents the drift status of a state
type DriftStatus struct {
	// StateID identifies the state
	StateID string

	// Module is the module name
	Module string

	// HasDrift indicates if drift was detected
	HasDrift bool

	// Severity indicates the drift severity
	Severity DriftSeverity

	// Differences contains the detected differences
	Differences []Difference

	// CheckedAt is when drift was checked
	CheckedAt time.Time
}

// Difference represents a single difference between desired and actual state
type Difference struct {
	// Path is the field path (e.g., "mode", "owner", "contents")
	Path string

	// Desired is the expected value
	Desired interface{}

	// Actual is the current value
	Actual interface{}

	// Severity indicates how critical this difference is
	Severity DriftSeverity

	// Message provides a human-readable description
	Message string
}

// DriftReport contains drift detection results for a state file
type DriftReport struct {
	// RunID identifies this drift check
	RunID string

	// StateFiles are the files checked
	StateFiles []string

	// CheckedAt is when the check occurred
	CheckedAt time.Time

	// Duration is how long the check took
	Duration time.Duration

	// States contains drift status for each state
	States []*DriftStatus

	// Summary provides overall drift statistics
	Summary *DriftSummary
}

// DriftSummary provides high-level drift statistics
type DriftSummary struct {
	// Total number of states checked
	Total int

	// NoDrift is the count of states with no drift
	NoDrift int

	// LowDrift is the count of states with low severity drift
	LowDrift int

	// MediumDrift is the count of states with medium severity drift
	MediumDrift int

	// HighDrift is the count of states with high severity drift
	HighDrift int

	// CriticalDrift is the count of states with critical severity drift
	CriticalDrift int

	// OverallSeverity is the highest severity found
	OverallSeverity DriftSeverity
}

// StateDiffer compares desired state vs actual state
type StateDiffer struct {
	// Registry contains available modules
	Registry *ModuleRegistry

	// EventPublisher publishes events (optional)
	EventPublisher events.EventPublisher

	// EventSource identifies the source of events
	EventSource string
}

// NewStateDiffer creates a new state differ
func NewStateDiffer() *StateDiffer {
	return &StateDiffer{
		Registry: DefaultRegistry,
	}
}

// CheckDrift checks for drift in a state file
func (d *StateDiffer) CheckDrift(stateFile *StateFile) (*DriftReport, error) {
	startTime := time.Now()

	// Resolve execution order
	executionOrder, err := ResolveExecutionOrder(stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	report := &DriftReport{
		RunID:      generateRunID(),
		StateFiles: []string{stateFile.Path},
		CheckedAt:  startTime,
		States:     make([]*DriftStatus, 0),
	}

	// Check each state for drift
	for _, decl := range executionOrder {
		status, err := d.checkStateDrift(decl)
		if err != nil {
			// Record error but continue
			status = &DriftStatus{
				StateID:   decl.ID,
				Module:    decl.Module,
				HasDrift:  true,
				Severity:  DriftCritical,
				CheckedAt: time.Now(),
				Differences: []Difference{
					{
						Path:     "error",
						Message:  fmt.Sprintf("Failed to check drift: %v", err),
						Severity: DriftCritical,
					},
				},
			}
		}

		report.States = append(report.States, status)

		// Emit drift event if drift detected
		if status.HasDrift {
			d.emitDriftEvent(report.RunID, status)
		}
	}

	report.Duration = time.Since(startTime)
	report.Summary = d.calculateSummary(report)

	return report, nil
}

// checkStateDrift checks a single state for drift
func (d *StateDiffer) checkStateDrift(decl *StateDeclaration) (*DriftStatus, error) {
	// Get the module
	module, err := d.Registry.Get(decl.Module)
	if err != nil {
		return nil, fmt.Errorf("module not found: %w", err)
	}

	// Check current state
	checkResult, err := module.Check(nil, decl)
	if err != nil {
		return nil, fmt.Errorf("check failed: %w", err)
	}

	status := &DriftStatus{
		StateID:     decl.ID,
		Module:      decl.Module,
		HasDrift:    !checkResult.Matches,
		CheckedAt:   time.Now(),
		Differences: make([]Difference, 0),
	}

	// If state matches, no drift
	if checkResult.Matches {
		status.Severity = DriftNone
		return status, nil
	}

	// Parse differences from check result
	if checkResult.Diff != nil && len(checkResult.Diff) > 0 {
		status.Differences = d.parseDifferences(decl, checkResult.Diff)
	}

	// If no specific differences but state doesn't match, it's a state mismatch
	if len(status.Differences) == 0 {
		status.Differences = []Difference{
			{
				Path:     "state",
				Desired:  decl.State,
				Actual:   checkResult.CurrentState,
				Severity: DriftHigh,
				Message:  fmt.Sprintf("state mismatch: expected %s, got %s", decl.State, checkResult.CurrentState),
			},
		}
	}

	// Calculate overall severity
	status.Severity = d.calculateStateSeverity(decl, status.Differences)

	return status, nil
}

// parseDifferences converts check result diff into structured differences
func (d *StateDiffer) parseDifferences(decl *StateDeclaration, diff map[string]interface{}) []Difference {
	differences := make([]Difference, 0)

	for key, value := range diff {
		if diffMap, ok := value.(map[string]interface{}); ok {
			// Structured diff with desired/actual
			desired := diffMap["desired"]
			actual := diffMap["actual"]

			differences = append(differences, Difference{
				Path:     key,
				Desired:  desired,
				Actual:   actual,
				Severity: d.calculateDiffSeverity(decl.Module, key, desired, actual),
				Message:  d.formatDiffMessage(key, desired, actual),
			})
		} else {
			// Simple value diff
			differences = append(differences, Difference{
				Path:     key,
				Actual:   value,
				Severity: DriftMedium,
				Message:  fmt.Sprintf("%s has changed", key),
			})
		}
	}

	return differences
}

// calculateDiffSeverity determines the severity of a single difference
func (d *StateDiffer) calculateDiffSeverity(module, field string, desired, actual interface{}) DriftSeverity {
	// Critical fields that should never drift
	criticalFields := map[string][]string{
		"file":    {"mode", "owner", "group"},
		"service": {"state", "enabled"},
		"user":    {"uid", "shell"},
	}

	if fields, ok := criticalFields[module]; ok {
		for _, f := range fields {
			if f == field {
				return DriftHigh
			}
		}
	}

	// Content changes are medium severity
	if field == "contents" || field == "content" {
		return DriftMedium
	}

	// Default to low severity
	return DriftLow
}

// calculateStateSeverity determines the overall severity for a state
func (d *StateDiffer) calculateStateSeverity(decl *StateDeclaration, differences []Difference) DriftSeverity {
	if len(differences) == 0 {
		return DriftNone
	}

	// Return the highest severity found
	highest := DriftLow
	for _, diff := range differences {
		if diff.Severity == DriftCritical {
			return DriftCritical
		}
		if diff.Severity == DriftHigh && (highest == DriftLow || highest == DriftMedium) {
			highest = DriftHigh
		}
		if diff.Severity == DriftMedium && highest == DriftLow {
			highest = DriftMedium
		}
	}

	return highest
}

// formatDiffMessage creates a human-readable diff message
func (d *StateDiffer) formatDiffMessage(field string, desired, actual interface{}) string {
	return fmt.Sprintf("%s: expected %v, got %v", field, desired, actual)
}

// calculateSummary calculates drift summary statistics
func (d *StateDiffer) calculateSummary(report *DriftReport) *DriftSummary {
	summary := &DriftSummary{
		Total:           len(report.States),
		OverallSeverity: DriftNone,
	}

	for _, status := range report.States {
		switch status.Severity {
		case DriftNone:
			summary.NoDrift++
		case DriftLow:
			summary.LowDrift++
		case DriftMedium:
			summary.MediumDrift++
		case DriftHigh:
			summary.HighDrift++
		case DriftCritical:
			summary.CriticalDrift++
		}

		// Track highest severity
		if d.severityRank(status.Severity) > d.severityRank(summary.OverallSeverity) {
			summary.OverallSeverity = status.Severity
		}
	}

	return summary
}

// severityRank converts severity to a numeric rank for comparison
func (d *StateDiffer) severityRank(severity DriftSeverity) int {
	switch severity {
	case DriftNone:
		return 0
	case DriftLow:
		return 1
	case DriftMedium:
		return 2
	case DriftHigh:
		return 3
	case DriftCritical:
		return 4
	default:
		return 0
	}
}

// FormatDriftReport formats a drift report as a string
func FormatDriftReport(report *DriftReport) string {
	var sb strings.Builder

	sb.WriteString("=== Drift Report ===\n")
	sb.WriteString(fmt.Sprintf("Run ID: %s\n", report.RunID))
	sb.WriteString(fmt.Sprintf("Checked: %s\n", report.CheckedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Duration: %s\n\n", report.Duration))

	sb.WriteString("=== Summary ===\n")
	sb.WriteString(fmt.Sprintf("Total states: %d\n", report.Summary.Total))
	sb.WriteString(fmt.Sprintf("No drift: %d\n", report.Summary.NoDrift))
	sb.WriteString(fmt.Sprintf("Low drift: %d\n", report.Summary.LowDrift))
	sb.WriteString(fmt.Sprintf("Medium drift: %d\n", report.Summary.MediumDrift))
	sb.WriteString(fmt.Sprintf("High drift: %d\n", report.Summary.HighDrift))
	sb.WriteString(fmt.Sprintf("Critical drift: %d\n", report.Summary.CriticalDrift))
	sb.WriteString(fmt.Sprintf("Overall severity: %s\n\n", report.Summary.OverallSeverity))

	// Show states with drift
	driftStates := 0
	for _, status := range report.States {
		if status.HasDrift {
			driftStates++
			sb.WriteString(fmt.Sprintf("--- %s.%s [%s] ---\n", status.Module, status.StateID, status.Severity))
			for _, diff := range status.Differences {
				sb.WriteString(fmt.Sprintf("  - %s\n", diff.Message))
			}
			sb.WriteString("\n")
		}
	}

	if driftStates == 0 {
		sb.WriteString("No drift detected!\n")
	}

	return sb.String()
}

// CompareStates compares two state declarations
func CompareStates(desired, actual *StateDeclaration) []Difference {
	differences := make([]Difference, 0)

	// Compare basic fields
	if desired.State != actual.State {
		differences = append(differences, Difference{
			Path:     "state",
			Desired:  desired.State,
			Actual:   actual.State,
			Severity: DriftHigh,
			Message:  fmt.Sprintf("state: expected %s, got %s", desired.State, actual.State),
		})
	}

	// Compare parameters
	for key, desiredVal := range desired.Parameters {
		actualVal, exists := actual.Parameters[key]
		if !exists {
			differences = append(differences, Difference{
				Path:     key,
				Desired:  desiredVal,
				Actual:   nil,
				Severity: DriftMedium,
				Message:  fmt.Sprintf("%s: missing in actual state", key),
			})
			continue
		}

		if !reflect.DeepEqual(desiredVal, actualVal) {
			differences = append(differences, Difference{
				Path:     key,
				Desired:  desiredVal,
				Actual:   actualVal,
				Severity: DriftMedium,
				Message:  fmt.Sprintf("%s: expected %v, got %v", key, desiredVal, actualVal),
			})
		}
	}

	// Check for extra parameters in actual
	for key, actualVal := range actual.Parameters {
		if _, exists := desired.Parameters[key]; !exists {
			differences = append(differences, Difference{
				Path:     key,
				Desired:  nil,
				Actual:   actualVal,
				Severity: DriftLow,
				Message:  fmt.Sprintf("%s: unexpected parameter in actual state", key),
			})
		}
	}

	return differences
}

// emitDriftEvent emits a drift event if EventPublisher is configured
func (d *StateDiffer) emitDriftEvent(runID string, status *DriftStatus) {
	if d.EventPublisher == nil {
		return
	}

	source := d.EventSource
	if source == "" {
		source = "/state-manager"
	}

	// Map drift severity to event severity
	eventSeverity := events.SeverityInfo
	switch status.Severity {
	case DriftCritical:
		eventSeverity = events.SeverityCritical
	case DriftHigh:
		eventSeverity = events.SeverityError
	case DriftMedium:
		eventSeverity = events.SeverityWarning
	case DriftLow:
		eventSeverity = events.SeverityInfo
	}

	// Build differences data
	diffs := make([]map[string]interface{}, 0, len(status.Differences))
	for _, diff := range status.Differences {
		diffs = append(diffs, map[string]interface{}{
			"path":     diff.Path,
			"desired":  diff.Desired,
			"actual":   diff.Actual,
			"severity": string(diff.Severity),
			"message":  diff.Message,
		})
	}

	event := events.NewEvent(events.EventTypeStateDrift).
		Source(source).
		Severity(eventSeverity).
		Tag("state_id", status.StateID).
		Tag("module", status.Module).
		Tag("drift_severity", string(status.Severity)).
		Data("run_id", runID).
		Data("state_id", status.StateID).
		Data("module", status.Module).
		Data("has_drift", status.HasDrift).
		Data("severity", string(status.Severity)).
		Data("differences", diffs).
		Data("checked_at", status.CheckedAt).
		Build()

	// Use async publish to avoid blocking drift detection
	if err := d.EventPublisher.PublishAsync(event); err != nil {
		fmt.Printf("Warning: failed to emit drift event: %v\n", err)
	}
}

// generateRunID generates a unique run ID
func generateRunID() string {
	return fmt.Sprintf("drift-%d", time.Now().Unix())
}
