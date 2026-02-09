package lockfile

import (
	"fmt"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/semver"
)

// NotFoundError indicates the lock file doesn't exist
type NotFoundError struct {
	Path string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("lock file not found: %s", e.Path)
}

// UnsupportedSchemaError indicates an unsupported schema version
type UnsupportedSchemaError struct {
	Version        int
	LatestVersion  int
	SupportedRange string
}

func (e *UnsupportedSchemaError) Error() string {
	return fmt.Sprintf(
		"unsupported lock file schema version %d (latest: %d, supported: %s). "+
			"Please upgrade Keystone Core or use 'kscore module lock --migrate' to migrate the lock file",
		e.Version, e.LatestVersion, e.SupportedRange,
	)
}

// ValidationError indicates a lock file validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("lock file validation error in %s: %s", e.Field, e.Message)
}

// ChecksumMismatch indicates a checksum mismatch
type ChecksumMismatch struct {
	Module   string
	Version  string
	Expected string
	Actual   string
	Missing  bool
}

func (c *ChecksumMismatch) String() string {
	if c.Missing {
		return fmt.Sprintf("%s@%s: checksum missing from lock file", c.Module, c.Version)
	}
	return fmt.Sprintf("%s@%s: checksum mismatch (expected %s, got %s)",
		c.Module, c.Version, c.Expected, c.Actual)
}

// MigrationResult contains the result of a lock file migration
type MigrationResult struct {
	// FromVersion is the original schema version
	FromVersion int `json:"from_version"`

	// ToVersion is the target schema version
	ToVersion int `json:"to_version"`

	// Skipped is true if migration was not needed
	Skipped bool `json:"skipped"`

	// Steps contains details of each migration step
	Steps []MigrationStep `json:"steps"`
}

// MigrationStep describes a single migration step
type MigrationStep struct {
	// FromVersion is the starting version for this step
	FromVersion int `json:"from_version"`

	// ToVersion is the ending version for this step
	ToVersion int `json:"to_version"`

	// StartedAt is when the step started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the step completed
	CompletedAt time.Time `json:"completed_at,omitempty"`

	// Changes describes what changed
	Changes []string `json:"changes,omitempty"`

	// Error if the step failed
	Error string `json:"error,omitempty"`
}

// Diff represents differences between two lock files
type Diff struct {
	// Added modules
	Added map[string]*LockedModule `json:"added"`

	// Removed modules
	Removed map[string]*LockedModule `json:"removed"`

	// Changed modules
	Changed map[string]*ModuleChange `json:"changed"`

	// Unchanged module names
	Unchanged []string `json:"unchanged"`
}

// ModuleChange describes how a module changed
type ModuleChange struct {
	// OldVersion is the previous version
	OldVersion string `json:"old_version"`

	// NewVersion is the new version
	NewVersion string `json:"new_version"`

	// OldHash is the previous hash
	OldHash string `json:"old_hash"`

	// NewHash is the new hash
	NewHash string `json:"new_hash"`
}

// IsEmpty returns true if there are no changes
func (d *Diff) IsEmpty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

// Summary returns a human-readable summary of changes
func (d *Diff) Summary() string {
	if d.IsEmpty() {
		return "No changes"
	}

	var parts []string
	if len(d.Added) > 0 {
		parts = append(parts, fmt.Sprintf("%d added", len(d.Added)))
	}
	if len(d.Removed) > 0 {
		parts = append(parts, fmt.Sprintf("%d removed", len(d.Removed)))
	}
	if len(d.Changed) > 0 {
		parts = append(parts, fmt.Sprintf("%d changed", len(d.Changed)))
	}

	return fmt.Sprintf("%s (%d unchanged)", joinParts(parts), len(d.Unchanged))
}

func joinParts(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		return parts[0] + ", " + joinParts(parts[1:])
	}
}

// UpdateType classifies the type of version change
type UpdateType string

const (
	// UpdateTypeMajor indicates a major version change
	UpdateTypeMajor UpdateType = "major"
	// UpdateTypeMinor indicates a minor version change
	UpdateTypeMinor UpdateType = "minor"
	// UpdateTypePatch indicates a patch version change
	UpdateTypePatch UpdateType = "patch"
	// UpdateTypePrerelease indicates a prerelease change
	UpdateTypePrerelease UpdateType = "prerelease"
	// UpdateTypeDowngrade indicates a version downgrade
	UpdateTypeDowngrade UpdateType = "downgrade"
	// UpdateTypeUnknown indicates an unknown change type
	UpdateTypeUnknown UpdateType = "unknown"
)

// GetUpdateType returns the type of update from old to new version
func (c *ModuleChange) GetUpdateType() UpdateType {
	// Parse versions using semver package
	oldVer, oldErr := semver.Parse(c.OldVersion)
	newVer, newErr := semver.Parse(c.NewVersion)

	// If either version fails to parse, fall back to string comparison
	if oldErr != nil || newErr != nil {
		if c.NewVersion < c.OldVersion {
			return UpdateTypeDowngrade
		}
		if c.NewVersion == c.OldVersion {
			return UpdateTypeUnknown
		}
		return UpdateTypeUnknown
	}

	// Use semver comparison to determine change type
	diff := semver.Compare(oldVer, newVer)

	switch diff.Direction {
	case semver.DirectionNone:
		return UpdateTypeUnknown
	case semver.DirectionDowngrade:
		return UpdateTypeDowngrade
	case semver.DirectionUpgrade:
		switch diff.Type {
		case semver.ChangeMajor:
			return UpdateTypeMajor
		case semver.ChangeMinor:
			return UpdateTypeMinor
		case semver.ChangePatch:
			return UpdateTypePatch
		case semver.ChangePrerelease:
			return UpdateTypePrerelease
		default:
			return UpdateTypeUnknown
		}
	}

	return UpdateTypeUnknown
}
