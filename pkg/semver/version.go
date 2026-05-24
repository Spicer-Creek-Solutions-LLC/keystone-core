// SPDX-License-Identifier: Apache-2.0

// Package semver wraps github.com/Masterminds/semver/v3 to provide the
// project-facing semver API. Callers do not need to import Masterminds.
package semver

import (
	"sort"

	masterminds "github.com/Masterminds/semver/v3"
)

// Version is a parsed SemVer 2.0.0 version.
type Version struct {
	v *masterminds.Version
}

// Parse parses a SemVer 2.0.0 version string. A leading "v" is permitted
// and preserved by Original.
func Parse(s string) (Version, error) {
	v, err := masterminds.NewVersion(s)
	if err != nil {
		return Version{}, err
	}
	return Version{v: v}, nil
}

// MustParse is like Parse but panics on invalid input.
func MustParse(s string) Version {
	v, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

func (v Version) String() string     { return v.v.String() }
func (v Version) Original() string   { return v.v.Original() }
func (v Version) Major() uint64      { return v.v.Major() }
func (v Version) Minor() uint64      { return v.v.Minor() }
func (v Version) Patch() uint64      { return v.v.Patch() }
func (v Version) Prerelease() string { return v.v.Prerelease() }
func (v Version) Metadata() string   { return v.v.Metadata() }

func (v Version) Compare(other Version) int      { return v.v.Compare(other.v) }
func (v Version) LessThan(other Version) bool    { return v.v.LessThan(other.v) }
func (v Version) GreaterThan(other Version) bool { return v.v.GreaterThan(other.v) }

// Equal reports precedence equality. Build metadata is ignored per SemVer 2.0.0 §10.
func (v Version) Equal(other Version) bool { return v.v.Equal(other.v) }

// NextMajor returns the next major version (X+1.0.0), clearing prerelease and metadata.
func (v Version) NextMajor() Version { r := v.v.IncMajor(); return Version{v: &r} }

// NextMinor returns the next minor version (X.Y+1.0), clearing prerelease and metadata.
func (v Version) NextMinor() Version { r := v.v.IncMinor(); return Version{v: &r} }

// NextPatch returns the next patch version (X.Y.Z+1), clearing prerelease and metadata.
func (v Version) NextPatch() Version { r := v.v.IncPatch(); return Version{v: &r} }

// Sort sorts vs in ascending order.
func Sort(vs []Version) {
	sort.Slice(vs, func(i, j int) bool { return vs[i].LessThan(vs[j]) })
}
