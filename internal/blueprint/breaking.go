// Package blueprint provides breaking change detection for blueprint upgrades.
package blueprint

import (
	"fmt"
	"sort"
	"strings"
)

// BreakingChangeType represents the type of breaking change.
type BreakingChangeType string

const (
	// BreakingMajorVersion indicates a major version bump.
	BreakingMajorVersion BreakingChangeType = "major_version"

	// BreakingParameterRemoved indicates a parameter was removed.
	BreakingParameterRemoved BreakingChangeType = "parameter_removed"

	// BreakingParameterTypeChanged indicates a parameter type changed.
	BreakingParameterTypeChanged BreakingChangeType = "parameter_type_changed"

	// BreakingParameterRequired indicates a parameter became required.
	BreakingParameterRequired BreakingChangeType = "parameter_required"

	// BreakingFeatureRemoved indicates a feature was removed.
	BreakingFeatureRemoved BreakingChangeType = "feature_removed"

	// BreakingDependencyRemoved indicates a dependency was removed.
	BreakingDependencyRemoved BreakingChangeType = "dependency_removed"

	// BreakingEntrypointRemoved indicates an entry point was removed.
	BreakingEntrypointRemoved BreakingChangeType = "entrypoint_removed"

	// BreakingStateRemoved indicates a state file was removed.
	BreakingStateRemoved BreakingChangeType = "state_removed"

	// BreakingBehaviorChanged indicates behavior changed significantly.
	BreakingBehaviorChanged BreakingChangeType = "behavior_changed"
)

// BreakingChangeSeverity represents the severity of a breaking change.
type BreakingChangeSeverity string

const (
	// SeverityLow indicates minor impact, usually auto-fixable.
	SeverityLow BreakingChangeSeverity = "low"

	// SeverityMedium indicates moderate impact, may need configuration changes.
	SeverityMedium BreakingChangeSeverity = "medium"

	// SeverityHigh indicates significant impact, requires manual intervention.
	SeverityHigh BreakingChangeSeverity = "high"

	// SeverityCritical indicates critical impact, may cause data loss or outage.
	SeverityCritical BreakingChangeSeverity = "critical"
)

// BreakingChange represents a single breaking change.
type BreakingChange struct {
	// Type is the type of breaking change.
	Type BreakingChangeType `json:"type"`

	// Severity is the severity of the change.
	Severity BreakingChangeSeverity `json:"severity"`

	// Description describes the change.
	Description string `json:"description"`

	// AffectedItem is the item affected (parameter name, feature, etc).
	AffectedItem string `json:"affected_item,omitempty"`

	// OldValue is the previous value (if applicable).
	OldValue string `json:"old_value,omitempty"`

	// NewValue is the new value (if applicable).
	NewValue string `json:"new_value,omitempty"`

	// Migration contains migration guidance.
	Migration string `json:"migration,omitempty"`

	// AutoFixable indicates if this can be auto-fixed.
	AutoFixable bool `json:"auto_fixable"`
}

// BreakingChangeReport is the result of breaking change detection.
type BreakingChangeReport struct {
	// FromVersion is the version being upgraded from.
	FromVersion string `json:"from_version"`

	// ToVersion is the version being upgraded to.
	ToVersion string `json:"to_version"`

	// BlueprintName is the blueprint name.
	BlueprintName string `json:"blueprint_name"`

	// Changes lists all detected breaking changes.
	Changes []BreakingChange `json:"changes"`

	// HasBreakingChanges is true if any breaking changes were detected.
	HasBreakingChanges bool `json:"has_breaking_changes"`

	// HighestSeverity is the highest severity among all changes.
	HighestSeverity BreakingChangeSeverity `json:"highest_severity,omitempty"`

	// RequiresAcknowledgment is true if user must acknowledge changes.
	RequiresAcknowledgment bool `json:"requires_acknowledgment"`
}

// BreakingChangeDetector detects breaking changes between blueprint versions.
type BreakingChangeDetector struct{}

// NewBreakingChangeDetector creates a new breaking change detector.
func NewBreakingChangeDetector() *BreakingChangeDetector {
	return &BreakingChangeDetector{}
}

// Detect detects breaking changes between two blueprint versions.
func (d *BreakingChangeDetector) Detect(oldBlueprint, newBlueprint *Blueprint) *BreakingChangeReport {
	report := &BreakingChangeReport{
		FromVersion:   oldBlueprint.Metadata.Version,
		ToVersion:     newBlueprint.Metadata.Version,
		BlueprintName: newBlueprint.Metadata.Name,
	}

	// Check major version bump
	d.checkMajorVersion(oldBlueprint, newBlueprint, report)

	// Check parameter changes
	d.checkParameterChanges(oldBlueprint, newBlueprint, report)

	// Check feature changes
	d.checkFeatureChanges(oldBlueprint, newBlueprint, report)

	// Check dependency changes
	d.checkDependencyChanges(oldBlueprint, newBlueprint, report)

	// Check entry point changes
	d.checkEntrypointChanges(oldBlueprint, newBlueprint, report)

	// Check state file changes
	d.checkStateChanges(oldBlueprint, newBlueprint, report)

	// Calculate summary
	report.HasBreakingChanges = len(report.Changes) > 0
	if report.HasBreakingChanges {
		report.HighestSeverity = d.calculateHighestSeverity(report.Changes)
		report.RequiresAcknowledgment = report.HighestSeverity == SeverityHigh ||
			report.HighestSeverity == SeverityCritical
	}

	return report
}

// checkMajorVersion checks for major version bumps.
func (d *BreakingChangeDetector) checkMajorVersion(old, updated *Blueprint, report *BreakingChangeReport) {
	oldMajor := getMajorVersion(old.Metadata.Version)
	newMajor := getMajorVersion(updated.Metadata.Version)

	if newMajor > oldMajor {
		report.Changes = append(report.Changes, BreakingChange{
			Type:        BreakingMajorVersion,
			Severity:    SeverityHigh,
			Description: fmt.Sprintf("Major version upgrade from %s to %s", old.Metadata.Version, updated.Metadata.Version),
			OldValue:    old.Metadata.Version,
			NewValue:    updated.Metadata.Version,
			Migration:   "Review the changelog and migration guide for this major version.",
			AutoFixable: false,
		})
	}
}

// checkParameterChanges checks for parameter schema changes.
func (d *BreakingChangeDetector) checkParameterChanges(old, updated *Blueprint, report *BreakingChangeReport) {
	// Check for removed parameters
	for name := range old.Parameters {
		if _, exists := updated.Parameters[name]; !exists {
			report.Changes = append(report.Changes, BreakingChange{
				Type:         BreakingParameterRemoved,
				Severity:     SeverityMedium,
				Description:  fmt.Sprintf("Parameter '%s' was removed", name),
				AffectedItem: name,
				OldValue:     fmt.Sprintf("type: %s", old.Parameters[name].Type),
				Migration:    "Remove this parameter from your configuration.",
				AutoFixable:  true,
			})
		}
	}

	// Check for type changes and new required parameters
	for name := range updated.Parameters {
		newParam := updated.Parameters[name]
		oldParam, exists := old.Parameters[name]
		if !exists {
			// New parameter
			if newParam.Required && newParam.Default == nil {
				report.Changes = append(report.Changes, BreakingChange{
					Type:         BreakingParameterRequired,
					Severity:     SeverityMedium,
					Description:  fmt.Sprintf("New required parameter '%s' without default", name),
					AffectedItem: name,
					NewValue:     fmt.Sprintf("type: %s", newParam.Type),
					Migration:    fmt.Sprintf("Add parameter '%s' to your configuration.", name),
					AutoFixable:  false,
				})
			}
			continue
		}

		// Type change
		if oldParam.Type != newParam.Type {
			report.Changes = append(report.Changes, BreakingChange{
				Type:         BreakingParameterTypeChanged,
				Severity:     SeverityMedium,
				Description:  fmt.Sprintf("Parameter '%s' type changed from '%s' to '%s'", name, oldParam.Type, newParam.Type),
				AffectedItem: name,
				OldValue:     oldParam.Type,
				NewValue:     newParam.Type,
				Migration:    fmt.Sprintf("Update the value of '%s' to match the new type '%s'.", name, newParam.Type),
				AutoFixable:  false,
			})
		}

		// Became required
		if newParam.Required && !oldParam.Required && newParam.Default == nil {
			report.Changes = append(report.Changes, BreakingChange{
				Type:         BreakingParameterRequired,
				Severity:     SeverityLow,
				Description:  fmt.Sprintf("Parameter '%s' is now required", name),
				AffectedItem: name,
				Migration:    fmt.Sprintf("Ensure parameter '%s' is set in your configuration.", name),
				AutoFixable:  false,
			})
		}
	}
}

// checkFeatureChanges checks for feature changes.
func (d *BreakingChangeDetector) checkFeatureChanges(old, updated *Blueprint, report *BreakingChangeReport) {
	// Check for removed features
	for name := range old.Features {
		if _, exists := updated.Features[name]; !exists {
			report.Changes = append(report.Changes, BreakingChange{
				Type:         BreakingFeatureRemoved,
				Severity:     SeverityMedium,
				Description:  fmt.Sprintf("Feature '%s' was removed", name),
				AffectedItem: name,
				Migration:    fmt.Sprintf("Remove feature '%s' from your configuration if enabled.", name),
				AutoFixable:  true,
			})
		}
	}
}

// checkDependencyChanges checks for dependency changes.
func (d *BreakingChangeDetector) checkDependencyChanges(old, updated *Blueprint, report *BreakingChangeReport) {
	if old.Dependencies == nil || updated.Dependencies == nil {
		return
	}

	// Combine all old dependencies
	oldDeps := make(map[string]bool)
	for _, dep := range old.Dependencies.Requires {
		oldDeps[dep] = true
	}
	for _, dep := range old.Dependencies.RequiresBefore {
		oldDeps[dep] = true
	}

	// Combine all new dependencies
	newDeps := make(map[string]bool)
	for _, dep := range updated.Dependencies.Requires {
		newDeps[dep] = true
	}
	for _, dep := range updated.Dependencies.RequiresBefore {
		newDeps[dep] = true
	}

	// Check for removed dependencies
	for dep := range oldDeps {
		if !newDeps[dep] {
			report.Changes = append(report.Changes, BreakingChange{
				Type:         BreakingDependencyRemoved,
				Severity:     SeverityLow,
				Description:  fmt.Sprintf("Dependency '%s' was removed", dep),
				AffectedItem: dep,
				Migration:    "This dependency is no longer included. You may need to add it separately.",
				AutoFixable:  false,
			})
		}
	}
}

// checkEntrypointChanges checks for entry point changes.
func (d *BreakingChangeDetector) checkEntrypointChanges(old, updated *Blueprint, report *BreakingChangeReport) {
	// Check for removed entry points
	for name := range old.Entrypoints {
		if _, exists := updated.Entrypoints[name]; !exists {
			report.Changes = append(report.Changes, BreakingChange{
				Type:         BreakingEntrypointRemoved,
				Severity:     SeverityHigh,
				Description:  fmt.Sprintf("Entry point '%s' was removed", name),
				AffectedItem: name,
				Migration:    fmt.Sprintf("Update your configuration to use a different entry point. '%s' no longer exists.", name),
				AutoFixable:  false,
			})
		}
	}
}

// checkStateChanges checks for state file changes by examining entrypoint values.
// Since state files are resolved dynamically via entrypoints and features,
// we check if any entrypoint now points to a different or removed state file.
func (d *BreakingChangeDetector) checkStateChanges(old, updated *Blueprint, report *BreakingChangeReport) {
	// Collect all state files referenced by entrypoints in the old blueprint
	oldStateFiles := make(map[string]bool)
	for _, stateFile := range old.Entrypoints {
		oldStateFiles[stateFile] = true
	}

	// Collect all state files referenced by entrypoints in the new blueprint
	newStateFiles := make(map[string]bool)
	for _, stateFile := range updated.Entrypoints {
		newStateFiles[stateFile] = true
	}

	// Check for removed state files (no longer referenced by any entrypoint)
	for stateFile := range oldStateFiles {
		if !newStateFiles[stateFile] {
			// Check if the file is still referenced but via a different entrypoint name
			stillExists := false
			for _, sf := range updated.Entrypoints {
				if sf == stateFile {
					stillExists = true
					break
				}
			}
			if !stillExists {
				report.Changes = append(report.Changes, BreakingChange{
					Type:         BreakingStateRemoved,
					Severity:     SeverityMedium,
					Description:  fmt.Sprintf("State file '%s' is no longer referenced by any entrypoint", stateFile),
					AffectedItem: stateFile,
					Migration:    "The functionality from this state file may have been moved or consolidated.",
					AutoFixable:  false,
				})
			}
		}
	}
}

// calculateHighestSeverity calculates the highest severity in a list of changes.
func (d *BreakingChangeDetector) calculateHighestSeverity(changes []BreakingChange) BreakingChangeSeverity {
	severityOrder := map[BreakingChangeSeverity]int{
		SeverityLow:      1,
		SeverityMedium:   2,
		SeverityHigh:     3,
		SeverityCritical: 4,
	}

	highest := SeverityLow
	for _, change := range changes {
		if severityOrder[change.Severity] > severityOrder[highest] {
			highest = change.Severity
		}
	}
	return highest
}

// getMajorVersion extracts the major version from a semver string.
func getMajorVersion(version string) int {
	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	// Split by '.'
	parts := strings.Split(version, ".")
	if len(parts) == 0 {
		return 0
	}

	var major int
	fmt.Sscanf(parts[0], "%d", &major)
	return major
}

// titleCase capitalizes the first letter of a string.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// GenerateMigrationGuide generates a migration guide from a breaking change report.
func GenerateMigrationGuide(report *BreakingChangeReport) string {
	if !report.HasBreakingChanges {
		return fmt.Sprintf("# Migration Guide: %s to %s\n\nNo breaking changes detected. Safe to upgrade.\n",
			report.FromVersion, report.ToVersion)
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Migration Guide: %s %s -> %s\n\n", report.BlueprintName, report.FromVersion, report.ToVersion))
	sb.WriteString(fmt.Sprintf("**Highest Severity:** %s\n\n", report.HighestSeverity))

	if report.RequiresAcknowledgment {
		sb.WriteString("> **Warning:** This upgrade contains breaking changes that require manual intervention.\n")
		sb.WriteString("> Use `--accept-breaking-changes` to acknowledge and proceed.\n\n")
	}

	sb.WriteString("## Breaking Changes\n\n")

	// Group by severity
	bySeverity := make(map[BreakingChangeSeverity][]BreakingChange)
	for _, change := range report.Changes {
		bySeverity[change.Severity] = append(bySeverity[change.Severity], change)
	}

	severities := []BreakingChangeSeverity{SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow}
	for _, severity := range severities {
		changes := bySeverity[severity]
		if len(changes) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s Severity\n\n", titleCase(string(severity))))

		for _, change := range changes {
			sb.WriteString(fmt.Sprintf("#### %s\n\n", change.Description))
			sb.WriteString(fmt.Sprintf("- **Type:** %s\n", change.Type))
			if change.AffectedItem != "" {
				sb.WriteString(fmt.Sprintf("- **Affected:** %s\n", change.AffectedItem))
			}
			if change.OldValue != "" {
				sb.WriteString(fmt.Sprintf("- **Old Value:** %s\n", change.OldValue))
			}
			if change.NewValue != "" {
				sb.WriteString(fmt.Sprintf("- **New Value:** %s\n", change.NewValue))
			}
			if change.AutoFixable {
				sb.WriteString("- **Auto-fixable:** Yes\n")
			}
			sb.WriteString(fmt.Sprintf("\n**Migration:** %s\n\n", change.Migration))
		}
	}

	sb.WriteString("## Recommended Steps\n\n")
	sb.WriteString("1. Review all breaking changes above\n")
	sb.WriteString("2. Backup your current configuration\n")
	sb.WriteString("3. Create a state snapshot before upgrading\n")
	sb.WriteString("4. Apply necessary configuration changes\n")
	sb.WriteString("5. Test in a non-production environment first\n")
	sb.WriteString("6. Upgrade with `--accept-breaking-changes` flag\n")
	sb.WriteString("7. Verify the upgrade was successful\n")

	return sb.String()
}

// SortChangesBySeverity sorts breaking changes by severity (highest first).
func SortChangesBySeverity(changes []BreakingChange) {
	severityOrder := map[BreakingChangeSeverity]int{
		SeverityCritical: 1,
		SeverityHigh:     2,
		SeverityMedium:   3,
		SeverityLow:      4,
	}

	sort.Slice(changes, func(i, j int) bool {
		return severityOrder[changes[i].Severity] < severityOrder[changes[j].Severity]
	})
}
