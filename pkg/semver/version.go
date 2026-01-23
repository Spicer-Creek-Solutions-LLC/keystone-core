package semver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// semverRegex matches semantic version strings.
// Supports: 1.2.3, v1.2.3, 1.2.3-alpha, 1.2.3-alpha.1, 1.2.3+build, 1.2.3-alpha+build
var semverRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

// Version represents a semantic version as defined by semver.org.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
}

// New creates a new Version with the given major, minor, and patch numbers.
func New(major, minor, patch int) Version {
	return Version{
		Major: major,
		Minor: minor,
		Patch: patch,
	}
}

// NewPrerelease creates a new Version with a prerelease tag.
func NewPrerelease(major, minor, patch int, prerelease string) Version {
	return Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: prerelease,
	}
}

// Parse parses a semantic version string.
// It accepts versions with or without a "v" prefix.
func Parse(s string) (Version, error) {
	if s == "" {
		return Version{}, ErrEmptyVersion
	}

	matches := semverRegex.FindStringSubmatch(s)
	if matches == nil {
		return Version{}, fmt.Errorf("%w: %q", ErrInvalidVersion, s)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	return Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: matches[4],
		Build:      matches[5],
	}, nil
}

// MustParse parses a semantic version string and panics if it fails.
// Use this only when you are certain the version string is valid.
func MustParse(s string) Version {
	v, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

// IsValid returns true if the string is a valid semantic version.
func IsValid(s string) bool {
	_, err := Parse(s)
	return err == nil
}

// String returns the version as a string in the format "major.minor.patch[-prerelease][+build]".
func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// IsZero returns true if the version is the zero value (0.0.0).
func (v Version) IsZero() bool {
	return v.Major == 0 && v.Minor == 0 && v.Patch == 0 && v.Prerelease == "" && v.Build == ""
}

// IsPrerelease returns true if the version has a prerelease tag.
func (v Version) IsPrerelease() bool {
	return v.Prerelease != ""
}

// IsStable returns true if the version is stable (no prerelease, major > 0).
func (v Version) IsStable() bool {
	return v.Major > 0 && v.Prerelease == ""
}

// Core returns the version without prerelease or build metadata.
func (v Version) Core() Version {
	return Version{
		Major: v.Major,
		Minor: v.Minor,
		Patch: v.Patch,
	}
}

// NextMajor returns the next major version (e.g., 1.2.3 -> 2.0.0).
func (v Version) NextMajor() Version {
	return Version{
		Major: v.Major + 1,
		Minor: 0,
		Patch: 0,
	}
}

// NextMinor returns the next minor version (e.g., 1.2.3 -> 1.3.0).
func (v Version) NextMinor() Version {
	return Version{
		Major: v.Major,
		Minor: v.Minor + 1,
		Patch: 0,
	}
}

// NextPatch returns the next patch version (e.g., 1.2.3 -> 1.2.4).
func (v Version) NextPatch() Version {
	return Version{
		Major: v.Major,
		Minor: v.Minor,
		Patch: v.Patch + 1,
	}
}

// WithPrerelease returns a copy of the version with the given prerelease tag.
func (v Version) WithPrerelease(prerelease string) Version {
	return Version{
		Major:      v.Major,
		Minor:      v.Minor,
		Patch:      v.Patch,
		Prerelease: prerelease,
		Build:      v.Build,
	}
}

// WithBuild returns a copy of the version with the given build metadata.
func (v Version) WithBuild(build string) Version {
	return Version{
		Major:      v.Major,
		Minor:      v.Minor,
		Patch:      v.Patch,
		Prerelease: v.Prerelease,
		Build:      build,
	}
}

// Compare compares two versions according to semantic versioning rules.
// Returns -1 if v < other, 0 if v == other, 1 if v > other.
// Build metadata is ignored in comparison per semver spec.
func (v Version) Compare(other Version) int {
	// Compare major
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}

	// Compare minor
	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}

	// Compare patch
	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}

	// Compare prerelease
	// A version without prerelease has higher precedence than one with prerelease
	if v.Prerelease == "" && other.Prerelease != "" {
		return 1
	}
	if v.Prerelease != "" && other.Prerelease == "" {
		return -1
	}
	if v.Prerelease != other.Prerelease {
		return comparePrerelease(v.Prerelease, other.Prerelease)
	}

	return 0
}

// LessThan returns true if v < other.
func (v Version) LessThan(other Version) bool {
	return v.Compare(other) < 0
}

// LessThanOrEqual returns true if v <= other.
func (v Version) LessThanOrEqual(other Version) bool {
	return v.Compare(other) <= 0
}

// GreaterThan returns true if v > other.
func (v Version) GreaterThan(other Version) bool {
	return v.Compare(other) > 0
}

// GreaterThanOrEqual returns true if v >= other.
func (v Version) GreaterThanOrEqual(other Version) bool {
	return v.Compare(other) >= 0
}

// Equal returns true if v == other (ignoring build metadata).
func (v Version) Equal(other Version) bool {
	return v.Compare(other) == 0
}

// comparePrerelease compares two prerelease strings according to semver rules.
// Identifiers are compared as follows:
// 1. Numeric identifiers are compared as integers
// 2. Alphanumeric identifiers are compared lexically
// 3. Numeric identifiers always have lower precedence than alphanumeric
// 4. A larger set of identifiers has higher precedence than a smaller set
func comparePrerelease(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	minLen := len(partsA)
	if len(partsB) < minLen {
		minLen = len(partsB)
	}

	for i := 0; i < minLen; i++ {
		result := comparePrereleaseIdentifier(partsA[i], partsB[i])
		if result != 0 {
			return result
		}
	}

	// If all compared parts are equal, the longer prerelease has higher precedence
	if len(partsA) < len(partsB) {
		return -1
	}
	if len(partsA) > len(partsB) {
		return 1
	}

	return 0
}

// comparePrereleaseIdentifier compares two prerelease identifiers.
func comparePrereleaseIdentifier(a, b string) int {
	aNum, aIsNum := parseNumeric(a)
	bNum, bIsNum := parseNumeric(b)

	// Both numeric: compare as integers
	if aIsNum && bIsNum {
		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
		return 0
	}

	// Numeric has lower precedence than alphanumeric
	if aIsNum {
		return -1
	}
	if bIsNum {
		return 1
	}

	// Both alphanumeric: compare lexically
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// parseNumeric attempts to parse a string as a non-negative integer.
func parseNumeric(s string) (int, bool) {
	// Must not have leading zeros (except for "0" itself)
	if len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
