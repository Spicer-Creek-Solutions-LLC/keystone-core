package semver

import (
	"testing"
)

func TestCompare(t *testing.T) {
	tests := []struct {
		name      string
		from      string
		to        string
		wantType  ChangeType
		wantDir   Direction
	}{
		// No change
		{"no change", "1.0.0", "1.0.0", ChangeNone, DirectionNone},

		// Build metadata only
		{"build change only", "1.0.0+build1", "1.0.0+build2", ChangeBuild, DirectionNone},
		{"add build", "1.0.0", "1.0.0+build", ChangeBuild, DirectionNone},

		// Major upgrades
		{"major upgrade", "1.0.0", "2.0.0", ChangeMajor, DirectionUpgrade},
		{"major upgrade with reset", "1.5.3", "2.0.0", ChangeMajor, DirectionUpgrade},

		// Minor upgrades
		{"minor upgrade", "1.0.0", "1.1.0", ChangeMinor, DirectionUpgrade},
		{"minor upgrade with reset", "1.2.5", "1.3.0", ChangeMinor, DirectionUpgrade},

		// Patch upgrades
		{"patch upgrade", "1.0.0", "1.0.1", ChangePatch, DirectionUpgrade},
		{"patch upgrade large", "1.0.0", "1.0.100", ChangePatch, DirectionUpgrade},

		// Prerelease changes
		{"prerelease to release", "1.0.0-alpha", "1.0.0", ChangePrerelease, DirectionUpgrade},
		{"release to prerelease", "1.0.0", "1.0.0-alpha", ChangePrerelease, DirectionDowngrade},
		{"prerelease to prerelease", "1.0.0-alpha", "1.0.0-beta", ChangePrerelease, DirectionUpgrade},

		// Downgrades
		{"major downgrade", "2.0.0", "1.0.0", ChangeMajor, DirectionDowngrade},
		{"minor downgrade", "1.5.0", "1.4.0", ChangeMinor, DirectionDowngrade},
		{"patch downgrade", "1.0.5", "1.0.4", ChangePatch, DirectionDowngrade},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			from := MustParse(tt.from)
			to := MustParse(tt.to)
			diff := Compare(from, to)

			if diff.Type != tt.wantType {
				t.Errorf("Compare(%s, %s).Type = %v, want %v", tt.from, tt.to, diff.Type, tt.wantType)
			}
			if diff.Direction != tt.wantDir {
				t.Errorf("Compare(%s, %s).Direction = %v, want %v", tt.from, tt.to, diff.Direction, tt.wantDir)
			}
		})
	}
}

func TestDiff_IsBreaking(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want bool
	}{
		// Major upgrade is breaking
		{"1.0.0", "2.0.0", true},

		// For 0.x, minor upgrade is breaking
		{"0.1.0", "0.2.0", true},

		// But 0.x patch is not breaking
		{"0.1.0", "0.1.1", false},

		// Minor upgrade for stable is not breaking
		{"1.0.0", "1.1.0", false},

		// Patch upgrade is not breaking
		{"1.0.0", "1.0.1", false},

		// Downgrades are not considered "breaking" by this method
		{"2.0.0", "1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.from+"_to_"+tt.to, func(t *testing.T) {
			diff := Compare(MustParse(tt.from), MustParse(tt.to))
			if got := diff.IsBreaking(); got != tt.want {
				t.Errorf("Diff{%s → %s}.IsBreaking() = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestDiff_IsFeature(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want bool
	}{
		// Minor upgrade on stable version is feature
		{"1.0.0", "1.1.0", true},

		// Major is not "feature" (it's breaking)
		{"1.0.0", "2.0.0", false},

		// Patch is not feature (it's bug fix)
		{"1.0.0", "1.0.1", false},

		// 0.x minor is not feature (it may be breaking)
		{"0.1.0", "0.2.0", false},

		// Downgrade is not feature
		{"1.5.0", "1.4.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.from+"_to_"+tt.to, func(t *testing.T) {
			diff := Compare(MustParse(tt.from), MustParse(tt.to))
			if got := diff.IsFeature(); got != tt.want {
				t.Errorf("Diff{%s → %s}.IsFeature() = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestDiff_IsBugFix(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want bool
	}{
		// Patch upgrade is bug fix
		{"1.0.0", "1.0.1", true},
		{"0.1.0", "0.1.1", true},

		// Minor/major are not bug fixes
		{"1.0.0", "1.1.0", false},
		{"1.0.0", "2.0.0", false},

		// Downgrade is not bug fix
		{"1.0.5", "1.0.4", false},
	}

	for _, tt := range tests {
		t.Run(tt.from+"_to_"+tt.to, func(t *testing.T) {
			diff := Compare(MustParse(tt.from), MustParse(tt.to))
			if got := diff.IsBugFix(); got != tt.want {
				t.Errorf("Diff{%s → %s}.IsBugFix() = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestDiff_IsUpgradeDowngrade(t *testing.T) {
	tests := []struct {
		from        string
		to          string
		wantUp      bool
		wantDown    bool
	}{
		{"1.0.0", "2.0.0", true, false},
		{"2.0.0", "1.0.0", false, true},
		{"1.0.0", "1.0.0", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.from+"_to_"+tt.to, func(t *testing.T) {
			diff := Compare(MustParse(tt.from), MustParse(tt.to))
			if got := diff.IsUpgrade(); got != tt.wantUp {
				t.Errorf("IsUpgrade() = %v, want %v", got, tt.wantUp)
			}
			if got := diff.IsDowngrade(); got != tt.wantDown {
				t.Errorf("IsDowngrade() = %v, want %v", got, tt.wantDown)
			}
		})
	}
}

func TestDiff_IsCompatible(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want bool
	}{
		// Patch is compatible
		{"1.0.0", "1.0.1", true},

		// Minor on stable is compatible
		{"1.0.0", "1.1.0", true},

		// Major is not compatible
		{"1.0.0", "2.0.0", false},

		// 0.x minor is not compatible (may be breaking)
		{"0.1.0", "0.2.0", false},

		// No change is compatible
		{"1.0.0", "1.0.0", true},

		// Downgrade - only no change is compatible
		{"2.0.0", "1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.from+"_to_"+tt.to, func(t *testing.T) {
			diff := Compare(MustParse(tt.from), MustParse(tt.to))
			if got := diff.IsCompatible(); got != tt.want {
				t.Errorf("IsCompatible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiff_Deltas(t *testing.T) {
	diff := Compare(MustParse("1.2.3"), MustParse("3.5.8"))

	if got := diff.MajorDelta(); got != 2 {
		t.Errorf("MajorDelta() = %d, want 2", got)
	}
	if got := diff.MinorDelta(); got != 3 {
		t.Errorf("MinorDelta() = %d, want 3", got)
	}
	if got := diff.PatchDelta(); got != 5 {
		t.Errorf("PatchDelta() = %d, want 5", got)
	}
}

func TestDiff_String(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want string
	}{
		{"1.0.0", "1.0.0", "no change"},
		{"1.0.0+b1", "1.0.0+b2", "build metadata change only"},
		{"1.0.0", "2.0.0", "major upgrade (breaking change)"},
		{"1.0.0", "1.1.0", "minor upgrade (feature addition)"},
		{"1.0.0", "1.0.1", "patch upgrade (bug fix)"},
		{"2.0.0", "1.0.0", "major downgrade (breaking change)"},
	}

	for _, tt := range tests {
		t.Run(tt.from+"_to_"+tt.to, func(t *testing.T) {
			diff := Compare(MustParse(tt.from), MustParse(tt.to))
			if got := diff.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDiff_Summary(t *testing.T) {
	diff := Compare(MustParse("1.0.0"), MustParse("2.0.0"))
	summary := diff.Summary()

	// Should contain both versions and change type
	if summary != "1.0.0 → 2.0.0 (major upgrade)" {
		t.Errorf("Summary() = %q", summary)
	}

	// No change case
	diff2 := Compare(MustParse("1.0.0"), MustParse("1.0.0"))
	if got := diff2.Summary(); got != "1.0.0 (no change)" {
		t.Errorf("Summary() for no change = %q", got)
	}
}

func TestChangeType_String(t *testing.T) {
	tests := []struct {
		ct   ChangeType
		want string
	}{
		{ChangeNone, "none"},
		{ChangeBuild, "build"},
		{ChangePatch, "patch"},
		{ChangeMinor, "minor"},
		{ChangeMajor, "major"},
		{ChangePrerelease, "prerelease"},
	}

	for _, tt := range tests {
		if got := tt.ct.String(); got != tt.want {
			t.Errorf("ChangeType(%d).String() = %q, want %q", tt.ct, got, tt.want)
		}
	}
}

func TestChangeType_Description(t *testing.T) {
	tests := []struct {
		ct   ChangeType
		want string
	}{
		{ChangeNone, "no change"},
		{ChangeBuild, "build metadata change"},
		{ChangePatch, "bug fix"},
		{ChangeMinor, "feature addition"},
		{ChangeMajor, "breaking change"},
		{ChangePrerelease, "prerelease change"},
	}

	for _, tt := range tests {
		if got := tt.ct.Description(); got != tt.want {
			t.Errorf("ChangeType(%d).Description() = %q, want %q", tt.ct, got, tt.want)
		}
	}
}

func TestDirection_String(t *testing.T) {
	tests := []struct {
		d    Direction
		want string
	}{
		{DirectionNone, "none"},
		{DirectionUpgrade, "upgrade"},
		{DirectionDowngrade, "downgrade"},
	}

	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("Direction(%d).String() = %q, want %q", tt.d, got, tt.want)
		}
	}
}
