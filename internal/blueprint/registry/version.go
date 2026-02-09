package registry

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Version represents a semantic version for blueprints.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
	Original   string
}

// semVerRegex matches semantic version strings.
var semVerRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z\-\.]+))?(?:\+([0-9A-Za-z\-\.]+))?$`)

// ParseVersion parses a semantic version string.
func ParseVersion(s string) (*Version, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty version string")
	}

	matches := semVerRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid version format: %s", s)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	return &Version{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: matches[4],
		Build:      matches[5],
		Original:   s,
	}, nil
}

// String returns the string representation of the version.
func (v *Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease != "" {
		s += "-" + v.Prerelease
	}
	if v.Build != "" {
		s += "+" + v.Build
	}
	return s
}

// Compare compares two versions.
// Returns: -1 if v < other, 0 if v == other, 1 if v > other
func (v *Version) Compare(other *Version) int {
	// Compare major, minor, patch
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}

	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}
		return 1
	}

	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}
		return 1
	}

	// Handle prerelease versions
	// Version without prerelease > version with prerelease
	if v.Prerelease == "" && other.Prerelease != "" {
		return 1
	}
	if v.Prerelease != "" && other.Prerelease == "" {
		return -1
	}

	// Both have prerelease, compare them
	if v.Prerelease != other.Prerelease {
		return comparePrerelease(v.Prerelease, other.Prerelease)
	}

	// Build metadata doesn't affect precedence
	return 0
}

// IsNewerThan returns true if v is newer than other.
func (v *Version) IsNewerThan(other *Version) bool {
	return v.Compare(other) > 0
}

// IsOlderThan returns true if v is older than other.
func (v *Version) IsOlderThan(other *Version) bool {
	return v.Compare(other) < 0
}

// IsCompatibleWith returns true if v is compatible with other (same major version).
func (v *Version) IsCompatibleWith(other *Version) bool {
	return v.Major == other.Major
}

// IsStable returns true if this is not a prerelease version.
func (v *Version) IsStable() bool {
	return v.Prerelease == ""
}

// comparePrerelease compares prerelease strings according to SemVer rules.
func comparePrerelease(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	for i := 0; i < len(partsA) && i < len(partsB); i++ {
		partA := partsA[i]
		partB := partsB[i]

		// Try to parse as numbers
		numA, errA := strconv.Atoi(partA)
		numB, errB := strconv.Atoi(partB)

		switch {
		case errA == nil && errB == nil:
			// Both are numbers
			if numA != numB {
				if numA < numB {
					return -1
				}
				return 1
			}
		case errA == nil:
			// A is numeric, B is alphanumeric - numeric < alphanumeric
			return -1
		case errB == nil:
			// A is alphanumeric, B is numeric - alphanumeric > numeric
			return 1
		default:
			// Both are alphanumeric
			if partA < partB {
				return -1
			}
			if partA > partB {
				return 1
			}
		}
	}

	// More parts = greater version
	if len(partsA) < len(partsB) {
		return -1
	}
	if len(partsA) > len(partsB) {
		return 1
	}

	return 0
}

// ConstraintOperator represents a version constraint operator.
type ConstraintOperator string

// OpEqual constants define the operators.
const (
	OpEqual          ConstraintOperator = "="
	OpNotEqual       ConstraintOperator = "!="
	OpGreater        ConstraintOperator = ">"
	OpGreaterOrEqual ConstraintOperator = ">="
	OpLess           ConstraintOperator = "<"
	OpLessOrEqual    ConstraintOperator = "<="
	OpCaret          ConstraintOperator = "^" // Compatible with (same major)
	OpTilde          ConstraintOperator = "~" // Patch-level changes
	OpWildcard       ConstraintOperator = "*" // Any version
)

// Constraint represents a version constraint.
type Constraint struct {
	Operator ConstraintOperator
	Version  *Version
	Raw      string
}

// constraintRegex matches version constraint expressions.
var constraintRegex = regexp.MustCompile(`^\s*(>=|<=|>|<|!=|=|\^|~)?\s*(v?[\d\.]+(?:-[0-9A-Za-z\-\.]+)?(?:\+[0-9A-Za-z\-\.]+)?|\*)\s*$`)

// ParseConstraint parses a version constraint string.
func ParseConstraint(s string) (*Constraint, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty constraint")
	}

	// Handle special cases
	if s == "*" || strings.EqualFold(s, "latest") {
		return &Constraint{
			Operator: OpWildcard,
			Raw:      s,
		}, nil
	}

	matches := constraintRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("invalid constraint format: %s", s)
	}

	op := ConstraintOperator(matches[1])
	if op == "" {
		op = OpEqual // Default to exact match
	}

	// Don't parse version for wildcard
	if matches[2] == "*" {
		return &Constraint{
			Operator: OpWildcard,
			Raw:      s,
		}, nil
	}

	version, err := ParseVersion(matches[2])
	if err != nil {
		return nil, fmt.Errorf("invalid version in constraint: %w", err)
	}

	return &Constraint{
		Operator: op,
		Version:  version,
		Raw:      s,
	}, nil
}

// Matches returns true if the version satisfies the constraint.
func (c *Constraint) Matches(v *Version) bool {
	if c.Operator == OpWildcard {
		return true
	}

	cmp := v.Compare(c.Version)

	switch c.Operator {
	case OpEqual, "":
		return cmp == 0
	case OpNotEqual:
		return cmp != 0
	case OpGreater:
		return cmp > 0
	case OpGreaterOrEqual:
		return cmp >= 0
	case OpLess:
		return cmp < 0
	case OpLessOrEqual:
		return cmp <= 0
	case OpCaret:
		// ^1.2.3 means >=1.2.3 <2.0.0 (same major)
		if cmp < 0 {
			return false
		}
		return v.Major == c.Version.Major
	case OpTilde:
		// ~1.2.3 means >=1.2.3 <1.3.0 (same major.minor)
		if cmp < 0 {
			return false
		}
		return v.Major == c.Version.Major && v.Minor == c.Version.Minor
	default:
		return false
	}
}

// String returns the string representation of the constraint.
func (c *Constraint) String() string {
	if c.Raw != "" {
		return c.Raw
	}
	if c.Operator == OpWildcard {
		return "*"
	}
	return string(c.Operator) + c.Version.String()
}

// ConstraintSet represents multiple constraints that must all be satisfied.
type ConstraintSet struct {
	Constraints []*Constraint
	Raw         string
}

// ParseConstraintSet parses a constraint set string (space or comma separated).
func ParseConstraintSet(s string) (*ConstraintSet, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return &ConstraintSet{
			Constraints: []*Constraint{{Operator: OpWildcard}},
			Raw:         "*",
		}, nil
	}

	// Split by comma or space
	parts := regexp.MustCompile(`[,\s]+`).Split(s, -1)
	var constraints []*Constraint

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		constraint, err := ParseConstraint(part)
		if err != nil {
			return nil, err
		}
		constraints = append(constraints, constraint)
	}

	if len(constraints) == 0 {
		return &ConstraintSet{
			Constraints: []*Constraint{{Operator: OpWildcard}},
			Raw:         "*",
		}, nil
	}

	return &ConstraintSet{
		Constraints: constraints,
		Raw:         s,
	}, nil
}

// Matches returns true if the version satisfies all constraints in the set.
func (cs *ConstraintSet) Matches(v *Version) bool {
	for _, c := range cs.Constraints {
		if !c.Matches(v) {
			return false
		}
	}
	return true
}

// String returns the string representation of the constraint set.
func (cs *ConstraintSet) String() string {
	if cs.Raw != "" {
		return cs.Raw
	}
	var parts []string
	for _, c := range cs.Constraints {
		parts = append(parts, c.String())
	}
	return strings.Join(parts, " ")
}

// VersionResolver resolves version constraints against available versions.
type VersionResolver struct {
	client Client
}

// NewVersionResolver creates a new VersionResolver.
func NewVersionResolver(client Client) *VersionResolver {
	return &VersionResolver{client: client}
}

// ResolveVersion resolves a version constraint to the best matching version.
func (r *VersionResolver) ResolveVersion(blueprintName, constraint string) (string, error) {
	// Parse constraint
	cs, err := ParseConstraintSet(constraint)
	if err != nil {
		return "", fmt.Errorf("invalid constraint: %w", err)
	}

	// Get available versions
	versions, err := r.client.ListVersions(blueprintName)
	if err != nil {
		return "", fmt.Errorf("failed to list versions: %w", err)
	}

	if len(versions) == 0 {
		return "", ErrVersionNotFound
	}

	// Find best matching version
	return r.selectBestVersion(versions, cs)
}

// ResolveVersionFromList resolves a version constraint from a list of available versions.
func (r *VersionResolver) ResolveVersionFromList(versions []string, constraint string) (string, error) {
	cs, err := ParseConstraintSet(constraint)
	if err != nil {
		return "", fmt.Errorf("invalid constraint: %w", err)
	}

	return r.selectBestVersion(versions, cs)
}

// selectBestVersion selects the best (highest) version matching the constraint.
func (r *VersionResolver) selectBestVersion(versions []string, cs *ConstraintSet) (string, error) {
	var matching []*Version

	for _, vStr := range versions {
		v, err := ParseVersion(vStr)
		if err != nil {
			continue // Skip invalid versions
		}

		if cs.Matches(v) {
			matching = append(matching, v)
		}
	}

	if len(matching) == 0 {
		return "", ErrVersionNotFound
	}

	// Sort by version (highest first)
	sort.Slice(matching, func(i, j int) bool {
		return matching[i].Compare(matching[j]) > 0
	})

	// Return highest matching version
	return matching[0].String(), nil
}

// GetMatchingVersions returns all versions matching the constraint.
func (r *VersionResolver) GetMatchingVersions(blueprintName, constraint string) ([]string, error) {
	cs, err := ParseConstraintSet(constraint)
	if err != nil {
		return nil, fmt.Errorf("invalid constraint: %w", err)
	}

	versions, err := r.client.ListVersions(blueprintName)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions: %w", err)
	}

	var matching []string
	for _, vStr := range versions {
		v, err := ParseVersion(vStr)
		if err != nil {
			continue
		}

		if cs.Matches(v) {
			matching = append(matching, vStr)
		}
	}

	return matching, nil
}

// IsVersionCompatible checks if two versions are compatible.
func IsVersionCompatible(v1, v2 string) (bool, error) {
	ver1, err := ParseVersion(v1)
	if err != nil {
		return false, err
	}

	ver2, err := ParseVersion(v2)
	if err != nil {
		return false, err
	}

	return ver1.IsCompatibleWith(ver2), nil
}

// CompareVersions compares two version strings.
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func CompareVersions(v1, v2 string) (int, error) {
	ver1, err := ParseVersion(v1)
	if err != nil {
		return 0, err
	}

	ver2, err := ParseVersion(v2)
	if err != nil {
		return 0, err
	}

	return ver1.Compare(ver2), nil
}

// SortVersions sorts version strings in descending order (newest first).
func SortVersions(versions []string) []string {
	// Parse all versions
	type versionEntry struct {
		str string
		ver *Version
	}

	var entries []versionEntry
	for _, vStr := range versions {
		v, err := ParseVersion(vStr)
		if err != nil {
			// Put invalid versions at the end
			entries = append(entries, versionEntry{str: vStr, ver: nil})
		} else {
			entries = append(entries, versionEntry{str: vStr, ver: v})
		}
	}

	// Sort
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ver == nil {
			return false
		}
		if entries[j].ver == nil {
			return true
		}
		return entries[i].ver.Compare(entries[j].ver) > 0
	})

	// Extract strings
	result := make([]string, len(entries))
	for i, e := range entries {
		result[i] = e.str
	}

	return result
}

// FilterStableVersions filters to only stable (non-prerelease) versions.
func FilterStableVersions(versions []string) []string {
	var stable []string
	for _, vStr := range versions {
		v, err := ParseVersion(vStr)
		if err != nil {
			continue
		}
		if v.IsStable() {
			stable = append(stable, vStr)
		}
	}
	return stable
}
