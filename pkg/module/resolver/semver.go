package resolver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version represents a semantic version
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Build      string
	Original   string
}

// semVerRegex matches semantic version strings
var semVerRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z\-\.]+))?(?:\+([0-9A-Za-z\-\.]+))?$`)

// ParseVersion parses a semantic version string
func ParseVersion(s string) (*Version, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("%w: empty version string", ErrInvalidVersion)
	}

	matches := semVerRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidVersion, s)
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

// String returns the string representation of the version
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

// Compare compares two versions
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

// comparePrerelease compares prerelease versions according to SemVer 2.0.0
func comparePrerelease(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	minLen := len(aParts)
	if len(bParts) < minLen {
		minLen = len(bParts)
	}

	for i := 0; i < minLen; i++ {
		aNum, aIsNum := tryParseInt(aParts[i])
		bNum, bIsNum := tryParseInt(bParts[i])

		// Numeric identifiers are compared numerically
		if aIsNum && bIsNum {
			if aNum < bNum {
				return -1
			}
			if aNum > bNum {
				return 1
			}
			continue
		}

		// Numeric identifier is always less than non-numeric
		if aIsNum {
			return -1
		}
		if bIsNum {
			return 1
		}

		// Both are non-numeric, compare lexically
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}

	// Longer prerelease is greater
	if len(aParts) < len(bParts) {
		return -1
	}
	if len(aParts) > len(bParts) {
		return 1
	}

	return 0
}

// tryParseInt attempts to parse a string as an integer
func tryParseInt(s string) (int, bool) {
	i, err := strconv.Atoi(s)
	return i, err == nil
}

// IsPrerelease returns true if this is a prerelease version
func (v *Version) IsPrerelease() bool {
	return v.Prerelease != ""
}

// Less returns true if v < other
func (v *Version) Less(other *Version) bool {
	return v.Compare(other) < 0
}

// Equal returns true if v == other
func (v *Version) Equal(other *Version) bool {
	return v.Compare(other) == 0
}

// Greater returns true if v > other
func (v *Version) Greater(other *Version) bool {
	return v.Compare(other) > 0
}

// Constraint represents a version constraint
type Constraint struct {
	operator string
	version  *Version
	original string
}

// NewConstraint creates a new constraint
func NewConstraint(operator string, version *Version) *Constraint {
	return &Constraint{
		operator: operator,
		version:  version,
		original: operator + version.String(),
	}
}

// Matches returns true if the version satisfies the constraint
func (c *Constraint) Matches(v string) bool {
	version, err := ParseVersion(v)
	if err != nil {
		return false
	}

	switch c.operator {
	case "=", "==":
		return version.Equal(c.version)
	case "!=":
		return !version.Equal(c.version)
	case ">":
		return version.Greater(c.version)
	case ">=":
		return version.Greater(c.version) || version.Equal(c.version)
	case "<":
		return version.Less(c.version)
	case "<=":
		return version.Less(c.version) || version.Equal(c.version)
	case "^":
		// Caret: allows changes that do not modify left-most non-zero digit
		// ^1.2.3 := >=1.2.3 <2.0.0
		// ^0.2.3 := >=0.2.3 <0.3.0
		// ^0.0.3 := >=0.0.3 <0.0.4
		if version.Less(c.version) {
			return false
		}
		if c.version.Major > 0 {
			return version.Major == c.version.Major
		}
		if c.version.Minor > 0 {
			return version.Major == c.version.Major && version.Minor == c.version.Minor
		}
		return version.Major == c.version.Major && version.Minor == c.version.Minor && version.Patch == c.version.Patch
	case "~":
		// Tilde: allows patch-level changes
		// ~1.2.3 := >=1.2.3 <1.3.0
		// ~1.2 := >=1.2.0 <1.3.0
		if version.Less(c.version) {
			return false
		}
		return version.Major == c.version.Major && version.Minor == c.version.Minor
	default:
		return false
	}
}

// String returns the constraint as a string
func (c *Constraint) String() string {
	return c.original
}

// IsExact returns true if this is an exact version (not a range)
func (c *Constraint) IsExact() bool {
	return c.operator == "=" || c.operator == "=="
}

// MultiConstraint represents multiple constraints AND'd together
type MultiConstraint struct {
	constraints []*Constraint
	original    string
}

// Matches returns true if the version satisfies all constraints
func (m *MultiConstraint) Matches(v string) bool {
	for _, c := range m.constraints {
		if !c.Matches(v) {
			return false
		}
	}
	return true
}

// String returns the constraint as a string
func (m *MultiConstraint) String() string {
	return m.original
}

// IsExact returns true if this is an exact version
func (m *MultiConstraint) IsExact() bool {
	if len(m.constraints) == 1 {
		return m.constraints[0].IsExact()
	}
	return false
}

// DefaultConstraintParser is the default constraint parser
type DefaultConstraintParser struct{}

// Parse parses a constraint string
func (p *DefaultConstraintParser) Parse(constraint string) (VersionConstraint, error) {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		return nil, fmt.Errorf("%w: empty constraint", ErrInvalidConstraint)
	}

	// Handle wildcard
	if constraint == "*" || constraint == "latest" {
		// * matches any version - represented as >=0.0.0
		v, _ := ParseVersion("0.0.0")
		return NewConstraint(">=", v), nil
	}

	// Detect operator
	operator := "="
	versionStr := constraint

	// Check longer operators first to avoid partial matches
	for _, op := range []string{">=", "<=", "!=", "==", "^", "~", ">", "<", "="} {
		if strings.HasPrefix(constraint, op) {
			operator = op
			versionStr = strings.TrimSpace(constraint[len(op):])
			break
		}
	}

	// Parse version
	version, err := ParseVersion(versionStr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConstraint, err)
	}

	return NewConstraint(operator, version), nil
}

// ParseMultiple parses multiple constraints (space or comma separated, AND'd together)
func (p *DefaultConstraintParser) ParseMultiple(constraints []string) (VersionConstraint, error) {
	if len(constraints) == 0 {
		return nil, fmt.Errorf("%w: no constraints provided", ErrInvalidConstraint)
	}

	if len(constraints) == 1 {
		return p.Parse(constraints[0])
	}

	parsed := make([]*Constraint, 0, len(constraints))
	for _, c := range constraints {
		vc, err := p.Parse(c)
		if err != nil {
			return nil, err
		}
		if constraint, ok := vc.(*Constraint); ok {
			parsed = append(parsed, constraint)
		} else {
			return nil, fmt.Errorf("%w: nested multi-constraints not supported", ErrInvalidConstraint)
		}
	}

	return &MultiConstraint{
		constraints: parsed,
		original:    strings.Join(constraints, " "),
	}, nil
}

// DefaultVersionSelector is the default version selector
type DefaultVersionSelector struct{}

// Select selects the best version matching the constraint
func (s *DefaultVersionSelector) Select(constraint VersionConstraint, available []string) (string, error) {
	if len(available) == 0 {
		return "", ErrNoVersionsAvailable
	}

	// Filter versions that match the constraint
	matching := make([]string, 0)
	for _, v := range available {
		if constraint.Matches(v) {
			matching = append(matching, v)
		}
	}

	if len(matching) == 0 {
		return "", fmt.Errorf("%w: no versions match %s", ErrConstraintUnsatisfiable, constraint.String())
	}

	// Select the highest matching version
	return s.SelectHighest(matching)
}

// SelectHighest selects the highest available version
func (s *DefaultVersionSelector) SelectHighest(available []string) (string, error) {
	if len(available) == 0 {
		return "", ErrNoVersionsAvailable
	}

	highest := available[0]
	highestVer, err := ParseVersion(highest)
	if err != nil {
		return "", err
	}

	for i := 1; i < len(available); i++ {
		ver, err := ParseVersion(available[i])
		if err != nil {
			continue
		}
		if ver.Greater(highestVer) {
			highest = available[i]
			highestVer = ver
		}
	}

	return highest, nil
}

// SelectLowest selects the lowest available version
func (s *DefaultVersionSelector) SelectLowest(available []string) (string, error) {
	if len(available) == 0 {
		return "", ErrNoVersionsAvailable
	}

	lowest := available[0]
	lowestVer, err := ParseVersion(lowest)
	if err != nil {
		return "", err
	}

	for i := 1; i < len(available); i++ {
		ver, err := ParseVersion(available[i])
		if err != nil {
			continue
		}
		if ver.Less(lowestVer) {
			lowest = available[i]
			lowestVer = ver
		}
	}

	return lowest, nil
}
