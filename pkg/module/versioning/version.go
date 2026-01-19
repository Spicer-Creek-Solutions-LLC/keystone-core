// Package versioning provides module version lifecycle management with deprecation support
package versioning

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// VersionState represents the lifecycle state of a version
type VersionState string

const (
	// VersionStateStable indicates a stable, released version
	VersionStateStable VersionState = "stable"

	// VersionStatePrerelease indicates a prerelease version (alpha, beta, rc)
	VersionStatePrerelease VersionState = "prerelease"

	// VersionStateDeprecated indicates the version is deprecated but still usable
	VersionStateDeprecated VersionState = "deprecated"

	// VersionStateYanked indicates the version has been yanked and should not be used
	VersionStateYanked VersionState = "yanked"

	// VersionStateRetracted indicates the version was retracted due to critical issues
	VersionStateRetracted VersionState = "retracted"
)

// VersionInfo contains comprehensive information about a module version
type VersionInfo struct {
	// Module is the module name
	Module string `json:"module" yaml:"module"`

	// Version is the semver version string
	Version string `json:"version" yaml:"version"`

	// State is the lifecycle state
	State VersionState `json:"state" yaml:"state"`

	// PublishedAt is when the version was published
	PublishedAt time.Time `json:"published_at" yaml:"published_at"`

	// Deprecation contains deprecation details (if deprecated)
	Deprecation *DeprecationInfo `json:"deprecation,omitempty" yaml:"deprecation,omitempty"`

	// Retraction contains retraction details (if retracted/yanked)
	Retraction *RetractionInfo `json:"retraction,omitempty" yaml:"retraction,omitempty"`

	// SecurityAdvisories lists any security advisories
	SecurityAdvisories []SecurityAdvisory `json:"security_advisories,omitempty" yaml:"security_advisories,omitempty"`

	// SupportedUntil is the date until which this version is supported
	SupportedUntil *time.Time `json:"supported_until,omitempty" yaml:"supported_until,omitempty"`

	// RequiresMinVersion is the minimum Keystone version required
	RequiresMinVersion string `json:"requires_min_version,omitempty" yaml:"requires_min_version,omitempty"`
}

// DeprecationInfo contains details about a deprecation
type DeprecationInfo struct {
	// DeprecatedAt is when the deprecation was announced
	DeprecatedAt time.Time `json:"deprecated_at" yaml:"deprecated_at"`

	// DeprecatedBy is who deprecated the version
	DeprecatedBy string `json:"deprecated_by,omitempty" yaml:"deprecated_by,omitempty"`

	// Reason explains why the version is deprecated
	Reason string `json:"reason" yaml:"reason"`

	// ReplacementVersion is the recommended replacement version
	ReplacementVersion string `json:"replacement_version,omitempty" yaml:"replacement_version,omitempty"`

	// ReplacementModule is the replacement module (if different)
	ReplacementModule string `json:"replacement_module,omitempty" yaml:"replacement_module,omitempty"`

	// SunsetDate is when the version will be fully removed/unsupported
	SunsetDate *time.Time `json:"sunset_date,omitempty" yaml:"sunset_date,omitempty"`

	// MigrationGuide is a URL to migration documentation
	MigrationGuide string `json:"migration_guide,omitempty" yaml:"migration_guide,omitempty"`

	// BreakingChanges lists the breaking changes in the replacement
	BreakingChanges []string `json:"breaking_changes,omitempty" yaml:"breaking_changes,omitempty"`

	// Severity indicates how urgently users should migrate
	Severity DeprecationSeverity `json:"severity" yaml:"severity"`
}

// DeprecationSeverity indicates the urgency of migrating from a deprecated version
type DeprecationSeverity string

const (
	// DeprecationSeverityLow - migration is recommended but not urgent
	DeprecationSeverityLow DeprecationSeverity = "low"

	// DeprecationSeverityMedium - migration should be planned
	DeprecationSeverityMedium DeprecationSeverity = "medium"

	// DeprecationSeverityHigh - migration should happen soon
	DeprecationSeverityHigh DeprecationSeverity = "high"

	// DeprecationSeverityCritical - immediate migration required
	DeprecationSeverityCritical DeprecationSeverity = "critical"
)

// RetractionInfo contains details about a version retraction
type RetractionInfo struct {
	// RetractedAt is when the version was retracted
	RetractedAt time.Time `json:"retracted_at" yaml:"retracted_at"`

	// RetractedBy is who retracted the version
	RetractedBy string `json:"retracted_by,omitempty" yaml:"retracted_by,omitempty"`

	// Reason explains why the version was retracted
	Reason string `json:"reason" yaml:"reason"`

	// ReplacementVersion is the safe replacement version
	ReplacementVersion string `json:"replacement_version,omitempty" yaml:"replacement_version,omitempty"`

	// AffectedVersions lists version ranges affected by the same issue
	AffectedVersions []string `json:"affected_versions,omitempty" yaml:"affected_versions,omitempty"`

	// CVE is the CVE identifier if security-related
	CVE string `json:"cve,omitempty" yaml:"cve,omitempty"`
}

// SecurityAdvisory represents a security advisory for a version
type SecurityAdvisory struct {
	// ID is the advisory identifier (CVE, GHSA, etc.)
	ID string `json:"id" yaml:"id"`

	// Severity is the advisory severity (low, medium, high, critical)
	Severity string `json:"severity" yaml:"severity"`

	// Title is a short title
	Title string `json:"title" yaml:"title"`

	// Description is a detailed description
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// URL is a link to the advisory
	URL string `json:"url,omitempty" yaml:"url,omitempty"`

	// FixedIn lists versions that fix this issue
	FixedIn []string `json:"fixed_in,omitempty" yaml:"fixed_in,omitempty"`

	// PublishedAt is when the advisory was published
	PublishedAt time.Time `json:"published_at" yaml:"published_at"`
}

// IsUsable returns true if the version can be used (not yanked/retracted)
func (v *VersionInfo) IsUsable() bool {
	return v.State != VersionStateYanked && v.State != VersionStateRetracted
}

// IsDeprecated returns true if the version is deprecated
func (v *VersionInfo) IsDeprecated() bool {
	return v.State == VersionStateDeprecated || v.Deprecation != nil
}

// IsPrerelease returns true if the version is a prerelease
func (v *VersionInfo) IsPrerelease() bool {
	return v.State == VersionStatePrerelease || isPrereleaseVersion(v.Version)
}

// HasSecurityIssues returns true if there are security advisories
func (v *VersionInfo) HasSecurityIssues() bool {
	return len(v.SecurityAdvisories) > 0
}

// IsSunset returns true if the version has passed its sunset date
func (v *VersionInfo) IsSunset() bool {
	if v.Deprecation == nil || v.Deprecation.SunsetDate == nil {
		return false
	}
	return time.Now().After(*v.Deprecation.SunsetDate)
}

// GetRecommendedReplacement returns the recommended replacement version
func (v *VersionInfo) GetRecommendedReplacement() (module, version string) {
	if v.Deprecation != nil {
		if v.Deprecation.ReplacementModule != "" {
			return v.Deprecation.ReplacementModule, v.Deprecation.ReplacementVersion
		}
		return v.Module, v.Deprecation.ReplacementVersion
	}
	if v.Retraction != nil {
		return v.Module, v.Retraction.ReplacementVersion
	}
	return "", ""
}

// WarningMessage returns a warning message for deprecated/problematic versions
func (v *VersionInfo) WarningMessage() string {
	var warnings []string

	if v.State == VersionStateYanked {
		warnings = append(warnings, fmt.Sprintf("WARNING: %s@%s has been yanked", v.Module, v.Version))
		if v.Retraction != nil && v.Retraction.Reason != "" {
			warnings = append(warnings, fmt.Sprintf("  Reason: %s", v.Retraction.Reason))
		}
	}

	if v.State == VersionStateRetracted {
		warnings = append(warnings, fmt.Sprintf("WARNING: %s@%s has been retracted", v.Module, v.Version))
		if v.Retraction != nil {
			if v.Retraction.Reason != "" {
				warnings = append(warnings, fmt.Sprintf("  Reason: %s", v.Retraction.Reason))
			}
			if v.Retraction.CVE != "" {
				warnings = append(warnings, fmt.Sprintf("  CVE: %s", v.Retraction.CVE))
			}
		}
	}

	if v.IsDeprecated() {
		warnings = append(warnings, fmt.Sprintf("DEPRECATED: %s@%s is deprecated", v.Module, v.Version))
		if v.Deprecation != nil {
			if v.Deprecation.Reason != "" {
				warnings = append(warnings, fmt.Sprintf("  Reason: %s", v.Deprecation.Reason))
			}
			if v.Deprecation.ReplacementVersion != "" {
				replacement := v.Deprecation.ReplacementVersion
				if v.Deprecation.ReplacementModule != "" {
					replacement = v.Deprecation.ReplacementModule + "@" + replacement
				}
				warnings = append(warnings, fmt.Sprintf("  Use: %s", replacement))
			}
			if v.Deprecation.SunsetDate != nil {
				warnings = append(warnings, fmt.Sprintf("  Sunset: %s", v.Deprecation.SunsetDate.Format("2006-01-02")))
			}
			if v.Deprecation.MigrationGuide != "" {
				warnings = append(warnings, fmt.Sprintf("  Migration guide: %s", v.Deprecation.MigrationGuide))
			}
		}
	}

	if len(v.SecurityAdvisories) > 0 {
		warnings = append(warnings, fmt.Sprintf("SECURITY: %s@%s has %d security advisory(ies)",
			v.Module, v.Version, len(v.SecurityAdvisories)))
		for _, adv := range v.SecurityAdvisories {
			warnings = append(warnings, fmt.Sprintf("  - [%s] %s: %s", adv.Severity, adv.ID, adv.Title))
		}
	}

	return strings.Join(warnings, "\n")
}

// prereleasePattern matches prerelease version suffixes
var prereleasePattern = regexp.MustCompile(`-(?:alpha|beta|rc|dev|pre)`)

// isPrereleaseVersion checks if a version string indicates a prerelease
func isPrereleaseVersion(version string) bool {
	return prereleasePattern.MatchString(strings.ToLower(version))
}

// NewVersionInfo creates a new VersionInfo with the given module and version
func NewVersionInfo(module, version string) *VersionInfo {
	state := VersionStateStable
	if isPrereleaseVersion(version) {
		state = VersionStatePrerelease
	}

	return &VersionInfo{
		Module:      module,
		Version:     version,
		State:       state,
		PublishedAt: time.Now(),
	}
}

// Deprecate marks a version as deprecated
func (v *VersionInfo) Deprecate(reason string, opts ...DeprecationOption) {
	v.State = VersionStateDeprecated
	v.Deprecation = &DeprecationInfo{
		DeprecatedAt: time.Now(),
		Reason:       reason,
		Severity:     DeprecationSeverityMedium,
	}

	for _, opt := range opts {
		opt(v.Deprecation)
	}
}

// DeprecationOption configures deprecation details
type DeprecationOption func(*DeprecationInfo)

// WithReplacement sets the replacement version
func WithReplacement(version string) DeprecationOption {
	return func(d *DeprecationInfo) {
		d.ReplacementVersion = version
	}
}

// WithReplacementModule sets the replacement module (if different)
func WithReplacementModule(module, version string) DeprecationOption {
	return func(d *DeprecationInfo) {
		d.ReplacementModule = module
		d.ReplacementVersion = version
	}
}

// WithSunsetDate sets the sunset date
func WithSunsetDate(date time.Time) DeprecationOption {
	return func(d *DeprecationInfo) {
		d.SunsetDate = &date
	}
}

// WithMigrationGuide sets the migration guide URL
func WithMigrationGuide(url string) DeprecationOption {
	return func(d *DeprecationInfo) {
		d.MigrationGuide = url
	}
}

// WithSeverity sets the deprecation severity
func WithSeverity(severity DeprecationSeverity) DeprecationOption {
	return func(d *DeprecationInfo) {
		d.Severity = severity
	}
}

// WithBreakingChanges sets the list of breaking changes
func WithBreakingChanges(changes ...string) DeprecationOption {
	return func(d *DeprecationInfo) {
		d.BreakingChanges = changes
	}
}

// Yank marks a version as yanked (should not be used)
func (v *VersionInfo) Yank(reason string) {
	v.State = VersionStateYanked
	v.Retraction = &RetractionInfo{
		RetractedAt: time.Now(),
		Reason:      reason,
	}
}

// Retract marks a version as retracted due to critical issues
func (v *VersionInfo) Retract(reason string, cve string) {
	v.State = VersionStateRetracted
	v.Retraction = &RetractionInfo{
		RetractedAt: time.Now(),
		Reason:      reason,
		CVE:         cve,
	}
}

// AddSecurityAdvisory adds a security advisory
func (v *VersionInfo) AddSecurityAdvisory(adv SecurityAdvisory) {
	v.SecurityAdvisories = append(v.SecurityAdvisories, adv)
}
