package semver

import (
	masterminds "github.com/Masterminds/semver/v3"
)

// Constraint matches versions against a SemVer constraint expression
// (caret ^, tilde ~, wildcard *, compound, OR ||).
type Constraint interface {
	Check(Version) bool
	String() string
}

type constraint struct {
	c *masterminds.Constraints
}

func (c constraint) Check(v Version) bool { return c.c.Check(v.v) }
func (c constraint) String() string       { return c.c.String() }

// NewConstraint parses a constraint expression.
func NewConstraint(s string) (Constraint, error) {
	c, err := masterminds.NewConstraint(s)
	if err != nil {
		return nil, err
	}
	return constraint{c: c}, nil
}
