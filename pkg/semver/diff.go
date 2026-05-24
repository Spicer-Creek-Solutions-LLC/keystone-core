// SPDX-License-Identifier: Apache-2.0

package semver

// DiffKind is the dimension along which two versions differ.
type DiffKind int

const (
	DiffSame       DiffKind = iota // versions are precedence-equal
	DiffPatch                      // patch component differs
	DiffMinor                      // minor component differs
	DiffMajor                      // major component differs
	DiffPrerelease                 // only prerelease component differs
)

// Direction is the relative direction of the diff.
type Direction int

const (
	DirectionSame Direction = iota
	DirectionUpgrade
	DirectionDowngrade
)

// Diff describes the relationship between two versions.
type Diff struct {
	Kind      DiffKind
	Direction Direction
}

func (d Diff) IsBreaking() bool { return d.Kind == DiffMajor }
func (d Diff) IsFeature() bool  { return d.Kind == DiffMinor }
func (d Diff) IsBugFix() bool   { return d.Kind == DiffPatch }

// DiffOf returns the diff describing the change from `from` to `to`.
// Build metadata is ignored per SemVer 2.0.0 §10, so versions that differ
// only in metadata are reported as DiffSame.
func DiffOf(from, to Version) Diff {
	cmp := from.Compare(to)
	if cmp == 0 {
		return Diff{Kind: DiffSame, Direction: DirectionSame}
	}
	dir := DirectionUpgrade
	if cmp > 0 {
		dir = DirectionDowngrade
	}
	switch {
	case from.Major() != to.Major():
		return Diff{Kind: DiffMajor, Direction: dir}
	case from.Minor() != to.Minor():
		return Diff{Kind: DiffMinor, Direction: dir}
	case from.Patch() != to.Patch():
		return Diff{Kind: DiffPatch, Direction: dir}
	default:
		return Diff{Kind: DiffPrerelease, Direction: dir}
	}
}
