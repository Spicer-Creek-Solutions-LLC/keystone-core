package semver

import "errors"

var (
	// ErrEmptyVersion is returned when parsing an empty version string.
	ErrEmptyVersion = errors.New("empty version string")

	// ErrInvalidVersion is returned when parsing an invalid version string.
	ErrInvalidVersion = errors.New("invalid semantic version")

	// ErrInvalidConstraint is returned when parsing an invalid constraint string.
	ErrInvalidConstraint = errors.New("invalid version constraint")
)
