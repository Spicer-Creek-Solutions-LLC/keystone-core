package semver

import "fmt"

// ChangeType represents the type of change between two versions.
type ChangeType int

const (
	// ChangeNone indicates no change between versions.
	ChangeNone ChangeType = iota

	// ChangeBuild indicates only build metadata changed.
	// Per semver spec, build metadata does not affect version precedence.
	ChangeBuild

	// ChangePatch indicates a patch version change (bug fix, backwards compatible).
	ChangePatch

	// ChangeMinor indicates a minor version change (new feature, backwards compatible).
	ChangeMinor

	// ChangeMajor indicates a major version change (breaking change).
	ChangeMajor

	// ChangePrerelease indicates only the prerelease tag changed.
	ChangePrerelease
)

// String returns a human-readable name for the change type.
func (c ChangeType) String() string {
	switch c {
	case ChangeNone:
		return "none"
	case ChangeBuild:
		return "build"
	case ChangePatch:
		return "patch"
	case ChangeMinor:
		return "minor"
	case ChangeMajor:
		return "major"
	case ChangePrerelease:
		return "prerelease"
	default:
		return "unknown"
	}
}

// Description returns a semantic description of the change type.
func (c ChangeType) Description() string {
	switch c {
	case ChangeNone:
		return "no change"
	case ChangeBuild:
		return "build metadata change"
	case ChangePatch:
		return "bug fix"
	case ChangeMinor:
		return "feature addition"
	case ChangeMajor:
		return "breaking change"
	case ChangePrerelease:
		return "prerelease change"
	default:
		return "unknown change"
	}
}

// Direction represents the direction of a version change.
type Direction int

const (
	// DirectionNone indicates no change or only build metadata change.
	DirectionNone Direction = iota

	// DirectionUpgrade indicates the version increased.
	DirectionUpgrade

	// DirectionDowngrade indicates the version decreased.
	DirectionDowngrade
)

// String returns a human-readable name for the direction.
func (d Direction) String() string {
	switch d {
	case DirectionNone:
		return "none"
	case DirectionUpgrade:
		return "upgrade"
	case DirectionDowngrade:
		return "downgrade"
	default:
		return "unknown"
	}
}

// Diff represents the difference between two versions.
type Diff struct {
	// From is the original version.
	From Version

	// To is the target version.
	To Version

	// Type indicates what kind of change occurred.
	Type ChangeType

	// Direction indicates whether this is an upgrade or downgrade.
	Direction Direction
}

// Compare calculates the difference between two versions.
// This is the primary way to get a rich comparison result.
func Compare(from, to Version) Diff {
	diff := Diff{
		From: from,
		To:   to,
	}

	// Determine change type and direction
	cmp := from.Compare(to)

	if cmp == 0 {
		// Versions are equal (ignoring build metadata)
		if from.Build != to.Build {
			diff.Type = ChangeBuild
			diff.Direction = DirectionNone
		} else {
			diff.Type = ChangeNone
			diff.Direction = DirectionNone
		}
		return diff
	}

	// Set direction
	if cmp < 0 {
		diff.Direction = DirectionUpgrade
	} else {
		diff.Direction = DirectionDowngrade
	}

	// Determine change type based on what changed
	diff.Type = determineChangeType(from, to)

	return diff
}

// determineChangeType determines the type of change between two versions.
func determineChangeType(from, to Version) ChangeType {
	// Major version change
	if from.Major != to.Major {
		return ChangeMajor
	}

	// Minor version change
	if from.Minor != to.Minor {
		return ChangeMinor
	}

	// Patch version change
	if from.Patch != to.Patch {
		return ChangePatch
	}

	// Only prerelease changed
	if from.Prerelease != to.Prerelease {
		return ChangePrerelease
	}

	// Only build changed (shouldn't reach here due to Compare logic, but be safe)
	return ChangeBuild
}

// String returns a human-readable description of the diff.
func (d Diff) String() string {
	if d.Type == ChangeNone {
		return "no change"
	}
	if d.Type == ChangeBuild {
		return "build metadata change only"
	}

	return fmt.Sprintf("%s %s (%s)", d.Type, d.Direction, d.Type.Description())
}

// Summary returns a brief summary suitable for logs or reports.
func (d Diff) Summary() string {
	if d.Type == ChangeNone {
		return fmt.Sprintf("%s (no change)", d.From)
	}
	return fmt.Sprintf("%s → %s (%s %s)", d.From, d.To, d.Type, d.Direction)
}

// IsBreaking returns true if this change may break backwards compatibility.
// Per semver, only major version changes are breaking (for stable versions).
// For 0.x.y versions, minor changes may also be breaking.
func (d Diff) IsBreaking() bool {
	if d.Direction != DirectionUpgrade {
		return false
	}

	// Major version change is always breaking
	if d.Type == ChangeMajor {
		return true
	}

	// For 0.x.y versions, minor changes may be breaking
	if d.From.Major == 0 && d.Type == ChangeMinor {
		return true
	}

	return false
}

// IsFeature returns true if this change adds new features (backwards compatible).
func (d Diff) IsFeature() bool {
	if d.Direction != DirectionUpgrade {
		return false
	}

	// Minor version upgrade (for stable versions) indicates new features
	if d.Type == ChangeMinor && d.From.Major > 0 {
		return true
	}

	return false
}

// IsBugFix returns true if this change is a bug fix (backwards compatible).
func (d Diff) IsBugFix() bool {
	return d.Direction == DirectionUpgrade && d.Type == ChangePatch
}

// IsUpgrade returns true if this is an upgrade (version increased).
func (d Diff) IsUpgrade() bool {
	return d.Direction == DirectionUpgrade
}

// IsDowngrade returns true if this is a downgrade (version decreased).
func (d Diff) IsDowngrade() bool {
	return d.Direction == DirectionDowngrade
}

// IsCompatible returns true if upgrading from From to To is backwards compatible.
// This means the change is a patch or minor update (for stable versions).
func (d Diff) IsCompatible() bool {
	if d.Direction != DirectionUpgrade {
		return d.Type == ChangeNone || d.Type == ChangeBuild
	}

	// Patch and prerelease changes are compatible
	if d.Type == ChangePatch || d.Type == ChangePrerelease {
		return true
	}

	// Minor changes are compatible only for stable versions (major > 0)
	if d.Type == ChangeMinor && d.From.Major > 0 {
		return true
	}

	return false
}

// MajorDelta returns the difference in major versions (To.Major - From.Major).
func (d Diff) MajorDelta() int {
	return d.To.Major - d.From.Major
}

// MinorDelta returns the difference in minor versions (To.Minor - From.Minor).
func (d Diff) MinorDelta() int {
	return d.To.Minor - d.From.Minor
}

// PatchDelta returns the difference in patch versions (To.Patch - From.Patch).
func (d Diff) PatchDelta() int {
	return d.To.Patch - d.From.Patch
}
