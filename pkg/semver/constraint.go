package semver

import (
	"fmt"
	"regexp"
	"strings"
)

// Constraint represents a version constraint that can check if versions match.
type Constraint interface {
	// Check returns true if the version satisfies the constraint.
	Check(v Version) bool

	// String returns the constraint as a string.
	String() string
}

// constraintRegex matches constraint operators and versions.
var constraintRegex = regexp.MustCompile(`^(>=|<=|>|<|=|!=|\^|~)?\s*v?(\d+)(?:\.(\d+|x|\*))?(?:\.(\d+|x|\*))?(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

// ParseConstraint parses a version constraint string.
// Supported formats:
//   - Exact: "1.2.3", "=1.2.3"
//   - Range: ">1.0.0", ">=1.0.0", "<2.0.0", "<=2.0.0", "!=1.5.0"
//   - Caret: "^1.2.3" (compatible with 1.x.x where x >= 2.3)
//   - Tilde: "~1.2.3" (compatible with 1.2.x where x >= 3)
//   - Wildcard: "1.x", "1.2.x", "1.*", "1.2.*"
//   - Compound: ">=1.0.0 <2.0.0", ">=1.0.0, <2.0.0"
func ParseConstraint(s string) (Constraint, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("%w: empty constraint", ErrInvalidConstraint)
	}

	// Check for OR constraints (||) first - they have lowest precedence
	if strings.Contains(s, "||") {
		return parseOrConstraint(s)
	}

	// Check for compound constraints (space or comma separated)
	if strings.Contains(s, " ") || strings.Contains(s, ",") {
		return parseCompoundConstraint(s)
	}

	return parseSingleConstraint(s)
}

// MustParseConstraint parses a constraint and panics if it fails.
func MustParseConstraint(s string) Constraint {
	c, err := ParseConstraint(s)
	if err != nil {
		panic(err)
	}
	return c
}

// parseSingleConstraint parses a single constraint expression.
func parseSingleConstraint(s string) (Constraint, error) {
	s = strings.TrimSpace(s)

	// Handle special case: "*" means any version
	if s == "*" || s == "x" {
		return &anyConstraint{}, nil
	}

	matches := constraintRegex.FindStringSubmatch(s)
	if matches == nil {
		return nil, fmt.Errorf("%w: %q", ErrInvalidConstraint, s)
	}

	op := matches[1]
	major := matches[2]
	minor := matches[3]
	patch := matches[4]
	prerelease := matches[5]

	// Handle wildcards
	hasWildcard := minor == "x" || minor == "*" || patch == "x" || patch == "*"
	if hasWildcard {
		return parseWildcardConstraint(major, minor, patch)
	}

	// Parse the version
	versionStr := major
	if minor != "" {
		versionStr += "." + minor
	} else {
		versionStr += ".0"
	}
	if patch != "" {
		versionStr += "." + patch
	} else {
		versionStr += ".0"
	}
	if prerelease != "" {
		versionStr += "-" + prerelease
	}

	v, err := Parse(versionStr)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrInvalidConstraint, s)
	}

	// Handle operators
	switch op {
	case "", "=":
		return &exactConstraint{version: v, original: s}, nil
	case "!=":
		return &notEqualConstraint{version: v, original: s}, nil
	case ">":
		return &greaterThanConstraint{version: v, original: s}, nil
	case ">=":
		return &greaterThanOrEqualConstraint{version: v, original: s}, nil
	case "<":
		return &lessThanConstraint{version: v, original: s}, nil
	case "<=":
		return &lessThanOrEqualConstraint{version: v, original: s}, nil
	case "^":
		return newCaretConstraint(v, s), nil
	case "~":
		return newTildeConstraint(v, s), nil
	default:
		return nil, fmt.Errorf("%w: unknown operator %q", ErrInvalidConstraint, op)
	}
}

// parseWildcardConstraint parses a wildcard constraint like "1.x" or "1.2.*".
func parseWildcardConstraint(major, minor, patch string) (Constraint, error) {
	majorInt := mustParseInt(major)

	if minor == "x" || minor == "*" || minor == "" {
		// "1.x" or "1.*" means >=1.0.0 <2.0.0
		return &andConstraint{
			constraints: []Constraint{
				&greaterThanOrEqualConstraint{version: New(majorInt, 0, 0)},
				&lessThanConstraint{version: New(majorInt+1, 0, 0)},
			},
			original: fmt.Sprintf("%d.x", majorInt),
		}, nil
	}

	minorInt := mustParseInt(minor)
	// "1.2.x" or "1.2.*" means >=1.2.0 <1.3.0
	return &andConstraint{
		constraints: []Constraint{
			&greaterThanOrEqualConstraint{version: New(majorInt, minorInt, 0)},
			&lessThanConstraint{version: New(majorInt, minorInt+1, 0)},
		},
		original: fmt.Sprintf("%d.%d.x", majorInt, minorInt),
	}, nil
}

// parseCompoundConstraint parses a compound constraint like ">=1.0.0 <2.0.0".
func parseCompoundConstraint(s string) (Constraint, error) {
	// Split by comma or space
	var parts []string
	if strings.Contains(s, ",") {
		parts = strings.Split(s, ",")
	} else {
		parts = strings.Fields(s)
	}

	var constraints []Constraint
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		c, err := parseSingleConstraint(part)
		if err != nil {
			return nil, err
		}
		constraints = append(constraints, c)
	}

	if len(constraints) == 0 {
		return nil, fmt.Errorf("%w: empty compound constraint", ErrInvalidConstraint)
	}
	if len(constraints) == 1 {
		return constraints[0], nil
	}

	return &andConstraint{constraints: constraints, original: s}, nil
}

// parseOrConstraint parses an OR constraint like "^1.0.0 || ^2.0.0".
func parseOrConstraint(s string) (Constraint, error) {
	parts := strings.Split(s, "||")

	var constraints []Constraint
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Each part might be a compound constraint
		var c Constraint
		var err error
		if strings.Contains(part, " ") || strings.Contains(part, ",") {
			c, err = parseCompoundConstraint(part)
		} else {
			c, err = parseSingleConstraint(part)
		}
		if err != nil {
			return nil, err
		}
		constraints = append(constraints, c)
	}

	if len(constraints) == 0 {
		return nil, fmt.Errorf("%w: empty OR constraint", ErrInvalidConstraint)
	}
	if len(constraints) == 1 {
		return constraints[0], nil
	}

	return &orConstraint{constraints: constraints, original: s}, nil
}

// newCaretConstraint creates a caret constraint.
// ^1.2.3 means >=1.2.3 <2.0.0 (for major > 0)
// ^0.2.3 means >=0.2.3 <0.3.0 (for major == 0)
// ^0.0.3 means >=0.0.3 <0.0.4 (for major == 0 && minor == 0)
func newCaretConstraint(v Version, original string) Constraint {
	var upper Version

	switch {
	case v.Major > 0:
		upper = New(v.Major+1, 0, 0)
	case v.Minor > 0:
		upper = New(0, v.Minor+1, 0)
	default:
		upper = New(0, 0, v.Patch+1)
	}

	return &andConstraint{
		constraints: []Constraint{
			&greaterThanOrEqualConstraint{version: v},
			&lessThanConstraint{version: upper},
		},
		original: original,
	}
}

// newTildeConstraint creates a tilde constraint.
// ~1.2.3 means >=1.2.3 <1.3.0
func newTildeConstraint(v Version, original string) Constraint {
	upper := New(v.Major, v.Minor+1, 0)

	return &andConstraint{
		constraints: []Constraint{
			&greaterThanOrEqualConstraint{version: v},
			&lessThanConstraint{version: upper},
		},
		original: original,
	}
}

// Constraint implementations

type anyConstraint struct{}

func (c *anyConstraint) Check(_ Version) bool { return true }
func (c *anyConstraint) String() string       { return "*" }

type exactConstraint struct {
	version  Version
	original string
}

func (c *exactConstraint) Check(v Version) bool { return v.Equal(c.version) }
func (c *exactConstraint) String() string {
	if c.original != "" {
		return c.original
	}
	return "=" + c.version.String()
}

type notEqualConstraint struct {
	version  Version
	original string
}

func (c *notEqualConstraint) Check(v Version) bool { return !v.Equal(c.version) }
func (c *notEqualConstraint) String() string {
	if c.original != "" {
		return c.original
	}
	return "!=" + c.version.String()
}

type greaterThanConstraint struct {
	version  Version
	original string
}

func (c *greaterThanConstraint) Check(v Version) bool { return v.GreaterThan(c.version) }
func (c *greaterThanConstraint) String() string {
	if c.original != "" {
		return c.original
	}
	return ">" + c.version.String()
}

type greaterThanOrEqualConstraint struct {
	version  Version
	original string
}

func (c *greaterThanOrEqualConstraint) Check(v Version) bool { return v.GreaterThanOrEqual(c.version) }
func (c *greaterThanOrEqualConstraint) String() string {
	if c.original != "" {
		return c.original
	}
	return ">=" + c.version.String()
}

type lessThanConstraint struct {
	version  Version
	original string
}

func (c *lessThanConstraint) Check(v Version) bool { return v.LessThan(c.version) }
func (c *lessThanConstraint) String() string {
	if c.original != "" {
		return c.original
	}
	return "<" + c.version.String()
}

type lessThanOrEqualConstraint struct {
	version  Version
	original string
}

func (c *lessThanOrEqualConstraint) Check(v Version) bool { return v.LessThanOrEqual(c.version) }
func (c *lessThanOrEqualConstraint) String() string {
	if c.original != "" {
		return c.original
	}
	return "<=" + c.version.String()
}

type andConstraint struct {
	constraints []Constraint
	original    string
}

func (c *andConstraint) Check(v Version) bool {
	for _, constraint := range c.constraints {
		if !constraint.Check(v) {
			return false
		}
	}
	return true
}

func (c *andConstraint) String() string {
	if c.original != "" {
		return c.original
	}
	var parts []string
	for _, constraint := range c.constraints {
		parts = append(parts, constraint.String())
	}
	return strings.Join(parts, " ")
}

type orConstraint struct {
	constraints []Constraint
	original    string
}

func (c *orConstraint) Check(v Version) bool {
	for _, constraint := range c.constraints {
		if constraint.Check(v) {
			return true
		}
	}
	return false
}

func (c *orConstraint) String() string {
	if c.original != "" {
		return c.original
	}
	var parts []string
	for _, constraint := range c.constraints {
		parts = append(parts, constraint.String())
	}
	return strings.Join(parts, " || ")
}

// mustParseInt parses an integer or returns 0.
func mustParseInt(s string) int {
	n, _ := parseNumeric(s)
	return n
}
