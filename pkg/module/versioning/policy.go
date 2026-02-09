package versioning

import (
	"fmt"
	"strings"
	"time"
)

// VersionPolicy defines rules for version selection and usage
type VersionPolicy struct {
	// AllowPrerelease controls whether prerelease versions can be used
	AllowPrerelease bool `json:"allow_prerelease" yaml:"allow_prerelease"`

	// AllowDeprecated controls whether deprecated versions can be used
	AllowDeprecated bool `json:"allow_deprecated" yaml:"allow_deprecated"`

	// AllowYanked controls whether yanked versions can be used (dangerous!)
	AllowYanked bool `json:"allow_yanked" yaml:"allow_yanked"`

	// AllowSecurityVulnerabilities controls whether versions with known vulnerabilities can be used
	AllowSecurityVulnerabilities bool `json:"allow_security_vulnerabilities" yaml:"allow_security_vulnerabilities"`

	// MaxDeprecationSeverity is the maximum deprecation severity allowed
	// Versions with higher severity will be rejected
	MaxDeprecationSeverity DeprecationSeverity `json:"max_deprecation_severity" yaml:"max_deprecation_severity"`

	// EnforceSunsetDates rejects versions past their sunset date
	EnforceSunsetDates bool `json:"enforce_sunset_dates" yaml:"enforce_sunset_dates"`

	// WarnOnDeprecated emits warnings for deprecated versions
	WarnOnDeprecated bool `json:"warn_on_deprecated" yaml:"warn_on_deprecated"`

	// WarnOnPrerelease emits warnings for prerelease versions
	WarnOnPrerelease bool `json:"warn_on_prerelease" yaml:"warn_on_prerelease"`

	// WarnOnSecurityIssues emits warnings for versions with security issues
	WarnOnSecurityIssues bool `json:"warn_on_security_issues" yaml:"warn_on_security_issues"`

	// RequireMinVersion is the minimum Keystone version required
	RequireMinVersion string `json:"require_min_version,omitempty" yaml:"require_min_version,omitempty"`

	// AllowedModules is a whitelist of allowed modules (empty = all allowed)
	AllowedModules []string `json:"allowed_modules,omitempty" yaml:"allowed_modules,omitempty"`

	// BlockedModules is a blacklist of blocked modules
	BlockedModules []string `json:"blocked_modules,omitempty" yaml:"blocked_modules,omitempty"`

	// AllowedRegistries is a whitelist of allowed registries (empty = all allowed)
	AllowedRegistries []string `json:"allowed_registries,omitempty" yaml:"allowed_registries,omitempty"`

	// BlockedRegistries is a blacklist of blocked registries
	BlockedRegistries []string `json:"blocked_registries,omitempty" yaml:"blocked_registries,omitempty"`

	// CustomRules are additional policy rules
	CustomRules []PolicyRule `json:"custom_rules,omitempty" yaml:"custom_rules,omitempty"`
}

// PolicyRule represents a custom version policy rule
type PolicyRule struct {
	// Name is the rule name
	Name string `json:"name" yaml:"name"`

	// Description explains what the rule does
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// ModulePattern is a glob pattern for matching modules
	ModulePattern string `json:"module_pattern,omitempty" yaml:"module_pattern,omitempty"`

	// VersionPattern is a regex pattern for matching versions
	VersionPattern string `json:"version_pattern,omitempty" yaml:"version_pattern,omitempty"`

	// Action is the action to take (allow, deny, warn)
	Action PolicyAction `json:"action" yaml:"action"`

	// Message is the message to show when the rule matches
	Message string `json:"message,omitempty" yaml:"message,omitempty"`
}

// PolicyAction represents the action to take when a rule matches
type PolicyAction string

const (
	// PolicyActionAllow allows the version
	PolicyActionAllow PolicyAction = "allow"

	// PolicyActionDeny denies the version
	PolicyActionDeny PolicyAction = "deny"

	// PolicyActionWarn allows but warns about the version
	PolicyActionWarn PolicyAction = "warn"
)

// PolicyViolationError represents a policy violation error
type PolicyViolationError struct {
	// Module is the module that violated the policy
	Module string `json:"module"`

	// Version is the version that violated the policy
	Version string `json:"version"`

	// Rule is the rule that was violated
	Rule string `json:"rule"`

	// Severity is the violation severity
	Severity ViolationSeverity `json:"severity"`

	// Message describes the violation
	Message string `json:"message"`

	// Suggestion is a suggested fix
	Suggestion string `json:"suggestion,omitempty"`
}

// ViolationSeverity indicates how severe a policy violation is
type ViolationSeverity string

// ViolationSeverity constants define the severity levels.
const (
	ViolationSeverityError   ViolationSeverity = "error"
	ViolationSeverityWarning ViolationSeverity = "warning"
	ViolationSeverityInfo    ViolationSeverity = "info"
)

// Error implements the error interface
func (v PolicyViolationError) Error() string {
	return fmt.Sprintf("[%s] %s@%s: %s", v.Severity, v.Module, v.Version, v.Message)
}

// DefaultPolicy returns a sensible default policy
func DefaultPolicy() *VersionPolicy {
	return &VersionPolicy{
		AllowPrerelease:              false,
		AllowDeprecated:              true, // Allow but warn
		AllowYanked:                  false,
		AllowSecurityVulnerabilities: false,
		MaxDeprecationSeverity:       DeprecationSeverityHigh,
		EnforceSunsetDates:           true,
		WarnOnDeprecated:             true,
		WarnOnPrerelease:             true,
		WarnOnSecurityIssues:         true,
	}
}

// StrictPolicy returns a strict policy that rejects deprecated versions
func StrictPolicy() *VersionPolicy {
	return &VersionPolicy{
		AllowPrerelease:              false,
		AllowDeprecated:              false,
		AllowYanked:                  false,
		AllowSecurityVulnerabilities: false,
		MaxDeprecationSeverity:       DeprecationSeverityLow,
		EnforceSunsetDates:           true,
		WarnOnDeprecated:             true,
		WarnOnPrerelease:             true,
		WarnOnSecurityIssues:         true,
	}
}

// PermissivePolicy returns a permissive policy that allows most versions
func PermissivePolicy() *VersionPolicy {
	return &VersionPolicy{
		AllowPrerelease:              true,
		AllowDeprecated:              true,
		AllowYanked:                  false, // Still block yanked
		AllowSecurityVulnerabilities: true,
		MaxDeprecationSeverity:       DeprecationSeverityCritical,
		EnforceSunsetDates:           false,
		WarnOnDeprecated:             true,
		WarnOnPrerelease:             false,
		WarnOnSecurityIssues:         true,
	}
}

// PolicyChecker checks versions against a policy
type PolicyChecker struct {
	policy *VersionPolicy
}

// NewPolicyChecker creates a new policy checker
func NewPolicyChecker(policy *VersionPolicy) *PolicyChecker {
	if policy == nil {
		policy = DefaultPolicy()
	}
	return &PolicyChecker{policy: policy}
}

// CheckResult contains the result of a policy check
type CheckResult struct {
	// Allowed indicates if the version is allowed
	Allowed bool

	// Violations is the list of policy violations
	Violations []PolicyViolationError

	// Warnings is the list of warnings
	Warnings []string
}

// Check checks if a version is allowed by the policy
func (c *PolicyChecker) Check(info *VersionInfo) *CheckResult {
	result := &CheckResult{
		Allowed:    true,
		Violations: []PolicyViolationError{},
		Warnings:   []string{},
	}

	// Check if module is blocked
	if c.isModuleBlocked(info.Module) {
		result.Allowed = false
		result.Violations = append(result.Violations, PolicyViolationError{
			Module:   info.Module,
			Version:  info.Version,
			Rule:     "blocked_module",
			Severity: ViolationSeverityError,
			Message:  "Module is blocked by policy",
		})
	}

	// Check if module is in allowlist (if allowlist is not empty)
	if len(c.policy.AllowedModules) > 0 && !c.isModuleAllowed(info.Module) {
		result.Allowed = false
		result.Violations = append(result.Violations, PolicyViolationError{
			Module:   info.Module,
			Version:  info.Version,
			Rule:     "allowed_modules",
			Severity: ViolationSeverityError,
			Message:  "Module is not in the allowed modules list",
		})
	}

	// Check yanked versions
	if info.State == VersionStateYanked && !c.policy.AllowYanked {
		result.Allowed = false
		violation := PolicyViolationError{
			Module:   info.Module,
			Version:  info.Version,
			Rule:     "yanked_version",
			Severity: ViolationSeverityError,
			Message:  "Version has been yanked and should not be used",
		}
		if info.Retraction != nil && info.Retraction.ReplacementVersion != "" {
			violation.Suggestion = fmt.Sprintf("Use version %s instead", info.Retraction.ReplacementVersion)
		}
		result.Violations = append(result.Violations, violation)
	}

	// Check retracted versions
	if info.State == VersionStateRetracted {
		result.Allowed = false
		violation := PolicyViolationError{
			Module:   info.Module,
			Version:  info.Version,
			Rule:     "retracted_version",
			Severity: ViolationSeverityError,
			Message:  "Version has been retracted due to critical issues",
		}
		if info.Retraction != nil {
			if info.Retraction.CVE != "" {
				violation.Message += fmt.Sprintf(" (%s)", info.Retraction.CVE)
			}
			if info.Retraction.ReplacementVersion != "" {
				violation.Suggestion = fmt.Sprintf("Use version %s instead", info.Retraction.ReplacementVersion)
			}
		}
		result.Violations = append(result.Violations, violation)
	}

	// Check deprecated versions
	if info.IsDeprecated() {
		if !c.policy.AllowDeprecated {
			result.Allowed = false
			violation := PolicyViolationError{
				Module:   info.Module,
				Version:  info.Version,
				Rule:     "deprecated_version",
				Severity: ViolationSeverityError,
				Message:  "Deprecated versions are not allowed by policy",
			}
			if info.Deprecation != nil && info.Deprecation.ReplacementVersion != "" {
				violation.Suggestion = fmt.Sprintf("Use version %s instead", info.Deprecation.ReplacementVersion)
			}
			result.Violations = append(result.Violations, violation)
		} else if c.policy.WarnOnDeprecated {
			result.Warnings = append(result.Warnings, info.WarningMessage())
		}

		// Check deprecation severity
		if info.Deprecation != nil {
			if !c.severityAllowed(info.Deprecation.Severity) {
				result.Allowed = false
				result.Violations = append(result.Violations, PolicyViolationError{
					Module:   info.Module,
					Version:  info.Version,
					Rule:     "deprecation_severity",
					Severity: ViolationSeverityError,
					Message:  fmt.Sprintf("Deprecation severity %s exceeds maximum allowed %s", info.Deprecation.Severity, c.policy.MaxDeprecationSeverity),
				})
			}
		}
	}

	// Check sunset dates
	if c.policy.EnforceSunsetDates && info.IsSunset() {
		result.Allowed = false
		result.Violations = append(result.Violations, PolicyViolationError{
			Module:   info.Module,
			Version:  info.Version,
			Rule:     "sunset_date",
			Severity: ViolationSeverityError,
			Message:  fmt.Sprintf("Version has passed its sunset date of %s", info.Deprecation.SunsetDate.Format("2006-01-02")),
		})
	}

	// Check prerelease versions
	if info.IsPrerelease() {
		if !c.policy.AllowPrerelease {
			result.Allowed = false
			result.Violations = append(result.Violations, PolicyViolationError{
				Module:   info.Module,
				Version:  info.Version,
				Rule:     "prerelease_version",
				Severity: ViolationSeverityError,
				Message:  "Prerelease versions are not allowed by policy",
			})
		} else if c.policy.WarnOnPrerelease {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Using prerelease version %s@%s", info.Module, info.Version))
		}
	}

	// Check security vulnerabilities
	if info.HasSecurityIssues() {
		if !c.policy.AllowSecurityVulnerabilities {
			result.Allowed = false
			result.Violations = append(result.Violations, PolicyViolationError{
				Module:   info.Module,
				Version:  info.Version,
				Rule:     "security_vulnerability",
				Severity: ViolationSeverityError,
				Message:  fmt.Sprintf("Version has %d known security vulnerability(ies)", len(info.SecurityAdvisories)),
			})
		} else if c.policy.WarnOnSecurityIssues {
			result.Warnings = append(result.Warnings, info.WarningMessage())
		}
	}

	// Check custom rules
	for _, rule := range c.policy.CustomRules {
		if c.ruleMatches(&rule, info) {
			switch rule.Action {
			case PolicyActionDeny:
				result.Allowed = false
				result.Violations = append(result.Violations, PolicyViolationError{
					Module:   info.Module,
					Version:  info.Version,
					Rule:     rule.Name,
					Severity: ViolationSeverityError,
					Message:  rule.Message,
				})
			case PolicyActionWarn:
				msg := rule.Message
				if msg == "" {
					msg = fmt.Sprintf("Custom rule '%s' matched", rule.Name)
				}
				result.Warnings = append(result.Warnings, msg)
			default:
			}
		}
	}

	return result
}

// isModuleBlocked checks if a module is in the blocklist
func (c *PolicyChecker) isModuleBlocked(module string) bool {
	for _, blocked := range c.policy.BlockedModules {
		if matchesPattern(blocked, module) {
			return true
		}
	}
	return false
}

// isModuleAllowed checks if a module is in the allowlist
func (c *PolicyChecker) isModuleAllowed(module string) bool {
	for _, allowed := range c.policy.AllowedModules {
		if matchesPattern(allowed, module) {
			return true
		}
	}
	return false
}

// severityAllowed checks if a deprecation severity is allowed
func (c *PolicyChecker) severityAllowed(severity DeprecationSeverity) bool {
	severityOrder := map[DeprecationSeverity]int{
		DeprecationSeverityLow:      1,
		DeprecationSeverityMedium:   2,
		DeprecationSeverityHigh:     3,
		DeprecationSeverityCritical: 4,
	}

	maxOrder := severityOrder[c.policy.MaxDeprecationSeverity]
	actualOrder := severityOrder[severity]

	return actualOrder <= maxOrder
}

// ruleMatches checks if a custom rule matches a version
func (c *PolicyChecker) ruleMatches(rule *PolicyRule, info *VersionInfo) bool {
	if rule.ModulePattern != "" && !matchesPattern(rule.ModulePattern, info.Module) {
		return false
	}
	if rule.VersionPattern != "" && !matchesPattern(rule.VersionPattern, info.Version) {
		return false
	}
	return true
}

// matchesPattern checks if a string matches a glob-like pattern
func matchesPattern(pattern, s string) bool {
	// Simple glob matching: * matches any sequence of characters
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		return strings.Contains(s, pattern[1:len(pattern)-1])
	}
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(s, pattern[1:])
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(s, pattern[:len(pattern)-1])
	}
	return pattern == s
}

// CheckVersion is a convenience function to check a version with the default policy
func CheckVersion(info *VersionInfo) *CheckResult {
	return NewPolicyChecker(nil).Check(info)
}

// CheckVersionWithPolicy checks a version with a specific policy
func CheckVersionWithPolicy(info *VersionInfo, policy *VersionPolicy) *CheckResult {
	return NewPolicyChecker(policy).Check(info)
}

// FilterVersions filters a list of versions to only include allowed ones
func (c *PolicyChecker) FilterVersions(versions []*VersionInfo) ([]*VersionInfo, []PolicyViolationError) {
	var allowed []*VersionInfo
	var allViolations []PolicyViolationError

	for _, v := range versions {
		result := c.Check(v)
		if result.Allowed {
			allowed = append(allowed, v)
		} else {
			allViolations = append(allViolations, result.Violations...)
		}
	}

	return allowed, allViolations
}

// SelectBestVersion selects the best version from a list, considering the policy
func (c *PolicyChecker) SelectBestVersion(versions []*VersionInfo) (best *VersionInfo, warnings []string) {
	allowed, _ := c.FilterVersions(versions)
	if len(allowed) == 0 {
		return nil, nil
	}

	// Prefer stable, non-deprecated versions
	for _, v := range allowed {
		if best == nil {
			best = v
			continue
		}

		// Prefer stable over prerelease
		if !v.IsPrerelease() && best.IsPrerelease() {
			best = v
			continue
		}

		// Prefer non-deprecated over deprecated
		if !v.IsDeprecated() && best.IsDeprecated() {
			best = v
			continue
		}

		// Prefer versions without security issues
		if !v.HasSecurityIssues() && best.HasSecurityIssues() {
			best = v
			continue
		}
	}

	if best != nil {
		result := c.Check(best)
		warnings = result.Warnings
	}

	return best, warnings
}

// DeprecationReport generates a deprecation report for a set of modules
type DeprecationReport struct {
	// GeneratedAt is when the report was generated
	GeneratedAt time.Time `json:"generated_at"`

	// TotalModules is the total number of modules analyzed
	TotalModules int `json:"total_modules"`

	// DeprecatedCount is the number of deprecated modules
	DeprecatedCount int `json:"deprecated_count"`

	// SecurityIssueCount is the number of modules with security issues
	SecurityIssueCount int `json:"security_issue_count"`

	// Deprecated lists deprecated modules
	Deprecated []DeprecatedModuleEntry `json:"deprecated,omitempty"`

	// SecurityIssues lists modules with security issues
	SecurityIssues []SecurityIssueEntry `json:"security_issues,omitempty"`

	// UpgradeSuggestions provides upgrade suggestions
	UpgradeSuggestions []UpgradeSuggestion `json:"upgrade_suggestions,omitempty"`
}

// DeprecatedModuleEntry represents a deprecated module in the report
type DeprecatedModuleEntry struct {
	Module             string              `json:"module"`
	Version            string              `json:"version"`
	Severity           DeprecationSeverity `json:"severity"`
	Reason             string              `json:"reason"`
	ReplacementVersion string              `json:"replacement_version,omitempty"`
	SunsetDate         *time.Time          `json:"sunset_date,omitempty"`
}

// SecurityIssueEntry represents a module with security issues in the report
type SecurityIssueEntry struct {
	Module     string             `json:"module"`
	Version    string             `json:"version"`
	Advisories []SecurityAdvisory `json:"advisories"`
}

// UpgradeSuggestion provides a suggested upgrade path
type UpgradeSuggestion struct {
	Module           string `json:"module"`
	CurrentVersion   string `json:"current_version"`
	SuggestedVersion string `json:"suggested_version"`
	Reason           string `json:"reason"`
	MigrationGuide   string `json:"migration_guide,omitempty"`
}

// GenerateDeprecationReport creates a deprecation report for a set of modules
func GenerateDeprecationReport(versions []*VersionInfo) *DeprecationReport {
	report := &DeprecationReport{
		GeneratedAt:  time.Now(),
		TotalModules: len(versions),
	}

	for _, v := range versions {
		if v.IsDeprecated() {
			report.DeprecatedCount++
			entry := DeprecatedModuleEntry{
				Module:  v.Module,
				Version: v.Version,
			}
			if v.Deprecation != nil {
				entry.Severity = v.Deprecation.Severity
				entry.Reason = v.Deprecation.Reason
				entry.ReplacementVersion = v.Deprecation.ReplacementVersion
				entry.SunsetDate = v.Deprecation.SunsetDate
			}
			report.Deprecated = append(report.Deprecated, entry)

			// Add upgrade suggestion
			if v.Deprecation != nil && v.Deprecation.ReplacementVersion != "" {
				report.UpgradeSuggestions = append(report.UpgradeSuggestions, UpgradeSuggestion{
					Module:           v.Module,
					CurrentVersion:   v.Version,
					SuggestedVersion: v.Deprecation.ReplacementVersion,
					Reason:           v.Deprecation.Reason,
					MigrationGuide:   v.Deprecation.MigrationGuide,
				})
			}
		}

		if v.HasSecurityIssues() {
			report.SecurityIssueCount++
			report.SecurityIssues = append(report.SecurityIssues, SecurityIssueEntry{
				Module:     v.Module,
				Version:    v.Version,
				Advisories: v.SecurityAdvisories,
			})
		}
	}

	return report
}
