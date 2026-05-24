// SPDX-License-Identifier: Apache-2.0

package semver

import "testing"

func TestDiffOf(t *testing.T) {
	tests := []struct {
		name string
		from string
		to   string
		want Diff
	}{
		{"same", "1.2.3", "1.2.3", Diff{DiffSame, DirectionSame}},
		{"patch up", "1.2.3", "1.2.4", Diff{DiffPatch, DirectionUpgrade}},
		{"patch down", "1.2.4", "1.2.3", Diff{DiffPatch, DirectionDowngrade}},
		{"minor up", "1.2.3", "1.3.0", Diff{DiffMinor, DirectionUpgrade}},
		{"minor down", "1.3.0", "1.2.3", Diff{DiffMinor, DirectionDowngrade}},
		{"major up", "1.2.3", "2.0.0", Diff{DiffMajor, DirectionUpgrade}},
		{"major down", "2.0.0", "1.2.3", Diff{DiffMajor, DirectionDowngrade}},
		{"prerelease up", "1.2.3-alpha", "1.2.3-beta", Diff{DiffPrerelease, DirectionUpgrade}},
		{"prerelease release", "1.2.3-rc.1", "1.2.3", Diff{DiffPrerelease, DirectionUpgrade}},
		{"metadata-only same", "1.2.3+a", "1.2.3+b", Diff{DiffSame, DirectionSame}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DiffOf(MustParse(tt.from), MustParse(tt.to))
			if got != tt.want {
				t.Errorf("DiffOf(%q, %q) = %+v, want %+v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestDiff_Predicates(t *testing.T) {
	tests := []struct {
		kind     DiffKind
		breaking bool
		feature  bool
		bugfix   bool
	}{
		{DiffSame, false, false, false},
		{DiffPatch, false, false, true},
		{DiffMinor, false, true, false},
		{DiffMajor, true, false, false},
		{DiffPrerelease, false, false, false},
	}
	for _, tt := range tests {
		d := Diff{Kind: tt.kind}
		if d.IsBreaking() != tt.breaking {
			t.Errorf("Kind=%d IsBreaking=%v, want %v", tt.kind, d.IsBreaking(), tt.breaking)
		}
		if d.IsFeature() != tt.feature {
			t.Errorf("Kind=%d IsFeature=%v, want %v", tt.kind, d.IsFeature(), tt.feature)
		}
		if d.IsBugFix() != tt.bugfix {
			t.Errorf("Kind=%d IsBugFix=%v, want %v", tt.kind, d.IsBugFix(), tt.bugfix)
		}
	}
}
