// Package versioning provides API version history tracking and deprecation management
package versioning

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Status represents the status of an API version
type Status string

const (
	// StatusCurrent indicates the current recommended version
	StatusCurrent Status = "current"
	// StatusSupported indicates a supported but not current version
	StatusSupported Status = "supported"
	// StatusDeprecated indicates a deprecated version
	StatusDeprecated Status = "deprecated"
	// StatusRetired indicates a retired version no longer available
	StatusRetired Status = "retired"
	// StatusBeta indicates a beta/preview version
	StatusBeta Status = "beta"
	// StatusAlpha indicates an alpha/experimental version
	StatusAlpha Status = "alpha"
)

// Version represents an API version
type Version struct {
	// Name is the version identifier (e.g., "v1", "v2beta1")
	Name string `json:"name"`

	// Major is the major version number
	Major int `json:"major"`

	// Minor is the minor version number
	Minor int `json:"minor"`

	// Status is the current status of this version
	Status Status `json:"status"`

	// ReleasedAt is when this version was released
	ReleasedAt time.Time `json:"released_at"`

	// DeprecatedAt is when this version was deprecated (if applicable)
	DeprecatedAt *time.Time `json:"deprecated_at,omitempty"`

	// SunsetAt is when this version will be/was retired
	SunsetAt *time.Time `json:"sunset_at,omitempty"`

	// Description describes this version
	Description string `json:"description,omitempty"`

	// Changelog lists changes in this version
	Changelog []ChangeEntry `json:"changelog,omitempty"`

	// DeprecationNotice explains why and how to migrate
	DeprecationNotice *DeprecationNotice `json:"deprecation_notice,omitempty"`

	// BreakingChanges lists breaking changes from previous version
	BreakingChanges []string `json:"breaking_changes,omitempty"`

	// Endpoints lists endpoints available in this version
	Endpoints []Endpoint `json:"endpoints,omitempty"`
}

// ChangeEntry represents a changelog entry
type ChangeEntry struct {
	// Type is the change type (added, changed, deprecated, removed, fixed, security)
	Type ChangeType `json:"type"`

	// Description describes the change
	Description string `json:"description"`

	// Endpoint is the affected endpoint (if applicable)
	Endpoint string `json:"endpoint,omitempty"`

	// IssueRef is a reference to an issue/PR (if applicable)
	IssueRef string `json:"issue_ref,omitempty"`
}

// ChangeType categorizes changelog entries
type ChangeType string

const (
	ChangeTypeAdded      ChangeType = "added"
	ChangeTypeChanged    ChangeType = "changed"
	ChangeTypeDeprecated ChangeType = "deprecated"
	ChangeTypeRemoved    ChangeType = "removed"
	ChangeTypeFixed      ChangeType = "fixed"
	ChangeTypeSecurity   ChangeType = "security"
)

// DeprecationNotice contains information about a deprecation
type DeprecationNotice struct {
	// Reason explains why the version/endpoint is deprecated
	Reason string `json:"reason"`

	// Replacement is the recommended replacement
	Replacement string `json:"replacement,omitempty"`

	// MigrationGuide is a link or text for migration guidance
	MigrationGuide string `json:"migration_guide,omitempty"`

	// SunsetDate is when the deprecated item will be removed
	SunsetDate *time.Time `json:"sunset_date,omitempty"`

	// ContactEmail for questions about migration
	ContactEmail string `json:"contact_email,omitempty"`
}

// Endpoint represents an API endpoint
type Endpoint struct {
	// Method is the HTTP method
	Method string `json:"method"`

	// Path is the endpoint path
	Path string `json:"path"`

	// Description describes what the endpoint does
	Description string `json:"description"`

	// Status is the endpoint status
	Status Status `json:"status"`

	// DeprecationNotice if the endpoint is deprecated
	DeprecationNotice *DeprecationNotice `json:"deprecation_notice,omitempty"`

	// AddedInVersion is when this endpoint was added
	AddedInVersion string `json:"added_in_version,omitempty"`

	// DeprecatedInVersion is when this endpoint was deprecated
	DeprecatedInVersion string `json:"deprecated_in_version,omitempty"`

	// RemovedInVersion is when this endpoint was removed
	RemovedInVersion string `json:"removed_in_version,omitempty"`
}

// Registry tracks API versions and their history
type Registry struct {
	mu       sync.RWMutex
	versions map[string]*Version
	current  string
}

// NewRegistry creates a new version registry
func NewRegistry() *Registry {
	return &Registry{
		versions: make(map[string]*Version),
	}
}

// Register registers an API version
func (r *Registry) Register(version *Version) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.versions[version.Name] = version

	// Update current if this is marked as current
	if version.Status == StatusCurrent {
		r.current = version.Name
	}
}

// Get returns a version by name
func (r *Registry) Get(name string) (*Version, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	v, ok := r.versions[name]
	return v, ok
}

// Current returns the current recommended version
func (r *Registry) Current() (*Version, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.current == "" {
		return nil, false
	}
	v, ok := r.versions[r.current]
	return v, ok
}

// List returns all versions sorted by release date (newest first)
func (r *Registry) List() []*Version {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions := make([]*Version, 0, len(r.versions))
	for _, v := range r.versions {
		versions = append(versions, v)
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].ReleasedAt.After(versions[j].ReleasedAt)
	})

	return versions
}

// ListByStatus returns versions filtered by status
func (r *Registry) ListByStatus(status Status) []*Version {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var versions []*Version
	for _, v := range r.versions {
		if v.Status == status {
			versions = append(versions, v)
		}
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].ReleasedAt.After(versions[j].ReleasedAt)
	})

	return versions
}

// Supported returns all supported versions (current + supported status)
func (r *Registry) Supported() []*Version {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var versions []*Version
	for _, v := range r.versions {
		if v.Status == StatusCurrent || v.Status == StatusSupported {
			versions = append(versions, v)
		}
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].ReleasedAt.After(versions[j].ReleasedAt)
	})

	return versions
}

// Deprecate marks a version as deprecated
func (r *Registry) Deprecate(name string, notice *DeprecationNotice) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	v, ok := r.versions[name]
	if !ok {
		return fmt.Errorf("version %s not found", name)
	}

	now := time.Now()
	v.Status = StatusDeprecated
	v.DeprecatedAt = &now
	v.DeprecationNotice = notice

	if notice.SunsetDate != nil {
		v.SunsetAt = notice.SunsetDate
	}

	return nil
}

// Retire marks a version as retired
func (r *Registry) Retire(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	v, ok := r.versions[name]
	if !ok {
		return fmt.Errorf("version %s not found", name)
	}

	now := time.Now()
	v.Status = StatusRetired
	v.SunsetAt = &now

	return nil
}

// SetCurrent sets the current recommended version
func (r *Registry) SetCurrent(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.versions[name]; !ok {
		return fmt.Errorf("version %s not found", name)
	}

	// Demote previous current to supported
	if r.current != "" && r.current != name {
		if prev, ok := r.versions[r.current]; ok {
			if prev.Status == StatusCurrent {
				prev.Status = StatusSupported
			}
		}
	}

	r.versions[name].Status = StatusCurrent
	r.current = name

	return nil
}

// IsDeprecated returns true if a version is deprecated
func (r *Registry) IsDeprecated(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	v, ok := r.versions[name]
	if !ok {
		return false
	}
	return v.Status == StatusDeprecated
}

// IsRetired returns true if a version is retired
func (r *Registry) IsRetired(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	v, ok := r.versions[name]
	if !ok {
		return false
	}
	return v.Status == StatusRetired
}

// GetDeprecationNotice returns the deprecation notice for a version
func (r *Registry) GetDeprecationNotice(name string) *DeprecationNotice {
	r.mu.RLock()
	defer r.mu.RUnlock()

	v, ok := r.versions[name]
	if !ok || v.DeprecationNotice == nil {
		return nil
	}
	return v.DeprecationNotice
}

// CheckSunset returns versions that are past their sunset date
func (r *Registry) CheckSunset() []*Version {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	var expired []*Version

	for _, v := range r.versions {
		if v.SunsetAt != nil && now.After(*v.SunsetAt) && v.Status != StatusRetired {
			expired = append(expired, v)
		}
	}

	return expired
}

// VersionHistory returns the complete history for display
func (r *Registry) VersionHistory() *History {
	versions := r.List()

	history := &History{
		Versions:   versions,
		Current:    r.current,
		GeneratedAt: time.Now(),
	}

	// Count by status
	for _, v := range versions {
		switch v.Status {
		case StatusCurrent:
			history.CurrentCount = 1
		case StatusSupported:
			history.SupportedCount++
		case StatusDeprecated:
			history.DeprecatedCount++
		case StatusRetired:
			history.RetiredCount++
		}
	}

	return history
}

// History represents the complete version history
type History struct {
	Versions        []*Version `json:"versions"`
	Current         string     `json:"current"`
	CurrentCount    int        `json:"current_count"`
	SupportedCount  int        `json:"supported_count"`
	DeprecatedCount int        `json:"deprecated_count"`
	RetiredCount    int        `json:"retired_count"`
	GeneratedAt     time.Time  `json:"generated_at"`
}

// Format returns a human-readable version history
func (h *History) Format() string {
	var sb strings.Builder

	sb.WriteString("API Version History\n")
	sb.WriteString("===================\n\n")

	sb.WriteString(fmt.Sprintf("Current: %s\n", h.Current))
	sb.WriteString(fmt.Sprintf("Supported: %d, Deprecated: %d, Retired: %d\n\n",
		h.SupportedCount, h.DeprecatedCount, h.RetiredCount))

	for _, v := range h.Versions {
		statusIcon := statusIcon(v.Status)
		sb.WriteString(fmt.Sprintf("%s %s (%s)\n", statusIcon, v.Name, v.Status))
		sb.WriteString(fmt.Sprintf("   Released: %s\n", v.ReleasedAt.Format("2006-01-02")))

		if v.Description != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", v.Description))
		}

		if v.DeprecatedAt != nil {
			sb.WriteString(fmt.Sprintf("   Deprecated: %s\n", v.DeprecatedAt.Format("2006-01-02")))
		}

		if v.SunsetAt != nil {
			sb.WriteString(fmt.Sprintf("   Sunset: %s\n", v.SunsetAt.Format("2006-01-02")))
		}

		if v.DeprecationNotice != nil {
			sb.WriteString(fmt.Sprintf("   ⚠ %s\n", v.DeprecationNotice.Reason))
			if v.DeprecationNotice.Replacement != "" {
				sb.WriteString(fmt.Sprintf("   → Use %s instead\n", v.DeprecationNotice.Replacement))
			}
		}

		if len(v.BreakingChanges) > 0 {
			sb.WriteString("   Breaking changes:\n")
			for _, bc := range v.BreakingChanges {
				sb.WriteString(fmt.Sprintf("     - %s\n", bc))
			}
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

func statusIcon(status Status) string {
	switch status {
	case StatusCurrent:
		return "✓"
	case StatusSupported:
		return "●"
	case StatusDeprecated:
		return "⚠"
	case StatusRetired:
		return "✗"
	case StatusBeta:
		return "β"
	case StatusAlpha:
		return "α"
	default:
		return "?"
	}
}

// DeprecationWarning generates a deprecation warning message
func DeprecationWarning(version string, notice *DeprecationNotice) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("WARNING: API version '%s' is deprecated", version))

	if notice != nil {
		if notice.Reason != "" {
			sb.WriteString(fmt.Sprintf(": %s", notice.Reason))
		}
		sb.WriteString(".")

		if notice.Replacement != "" {
			sb.WriteString(fmt.Sprintf(" Please migrate to %s.", notice.Replacement))
		}

		if notice.SunsetDate != nil {
			sb.WriteString(fmt.Sprintf(" This version will be removed on %s.",
				notice.SunsetDate.Format("2006-01-02")))
		}

		if notice.MigrationGuide != "" {
			sb.WriteString(fmt.Sprintf(" Migration guide: %s", notice.MigrationGuide))
		}
	}

	return sb.String()
}

// SunsetWarning generates a sunset warning message
func SunsetWarning(version string, sunsetDate time.Time) string {
	daysUntil := int(time.Until(sunsetDate).Hours() / 24)

	if daysUntil < 0 {
		return fmt.Sprintf("ERROR: API version '%s' has been retired as of %s",
			version, sunsetDate.Format("2006-01-02"))
	}

	if daysUntil == 0 {
		return fmt.Sprintf("CRITICAL: API version '%s' will be retired TODAY", version)
	}

	if daysUntil <= 7 {
		return fmt.Sprintf("CRITICAL: API version '%s' will be retired in %d day(s) on %s",
			version, daysUntil, sunsetDate.Format("2006-01-02"))
	}

	if daysUntil <= 30 {
		return fmt.Sprintf("WARNING: API version '%s' will be retired in %d day(s) on %s",
			version, daysUntil, sunsetDate.Format("2006-01-02"))
	}

	return fmt.Sprintf("NOTICE: API version '%s' will be retired on %s (%d days)",
		version, sunsetDate.Format("2006-01-02"), daysUntil)
}
