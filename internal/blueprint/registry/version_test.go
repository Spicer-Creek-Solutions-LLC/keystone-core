package registry

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		major   int
		minor   int
		patch   int
		prerel  string
		build   string
	}{
		{"1.0.0", false, 1, 0, 0, "", ""},
		{"1.2.3", false, 1, 2, 3, "", ""},
		{"0.1.0", false, 0, 1, 0, "", ""},
		{"10.20.30", false, 10, 20, 30, "", ""},
		{"v1.0.0", false, 1, 0, 0, "", ""},
		{"1.0.0-alpha", false, 1, 0, 0, "alpha", ""},
		{"1.0.0-beta.1", false, 1, 0, 0, "beta.1", ""},
		{"1.0.0-rc.1", false, 1, 0, 0, "rc.1", ""},
		{"1.0.0+build", false, 1, 0, 0, "", "build"},
		{"1.0.0-alpha+build", false, 1, 0, 0, "alpha", "build"},
		{"", true, 0, 0, 0, "", ""},
		{"invalid", true, 0, 0, 0, "", ""},
		{"1.0", true, 0, 0, 0, "", ""},
		{"1", true, 0, 0, 0, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := ParseVersion(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if v.Major != tt.major {
				t.Errorf("major = %d, want %d", v.Major, tt.major)
			}
			if v.Minor != tt.minor {
				t.Errorf("minor = %d, want %d", v.Minor, tt.minor)
			}
			if v.Patch != tt.patch {
				t.Errorf("patch = %d, want %d", v.Patch, tt.patch)
			}
			if v.Prerelease != tt.prerel {
				t.Errorf("prerelease = %q, want %q", v.Prerelease, tt.prerel)
			}
			if v.Build != tt.build {
				t.Errorf("build = %q, want %q", v.Build, tt.build)
			}
		})
	}
}

func TestVersion_Compare(t *testing.T) {
	tests := []struct {
		v1   string
		v2   string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "2.0.0", -1},
		{"1.2.0", "1.1.0", 1},
		{"1.1.0", "1.2.0", -1},
		{"1.0.2", "1.0.1", 1},
		{"1.0.1", "1.0.2", -1},
		{"1.0.0", "1.0.0-alpha", 1},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-beta", "1.0.0-alpha", 1},
		{"1.0.0-alpha.1", "1.0.0-alpha.2", -1},
		{"1.0.0-1", "1.0.0-2", -1},
		{"1.0.0-alpha", "1.0.0-1", 1}, // alpha > 1 (alphanumeric > numeric)
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			v1, _ := ParseVersion(tt.v1)
			v2, _ := ParseVersion(tt.v2)

			got := v1.Compare(v2)
			if got != tt.want {
				t.Errorf("Compare(%s, %s) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestVersion_String(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.0.0", "1.0.0"},
		{"1.2.3", "1.2.3"},
		{"1.0.0-alpha", "1.0.0-alpha"},
		{"1.0.0+build", "1.0.0+build"},
		{"1.0.0-alpha+build", "1.0.0-alpha+build"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, _ := ParseVersion(tt.input)
			got := v.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersion_IsNewerThan(t *testing.T) {
	v1, _ := ParseVersion("2.0.0")
	v2, _ := ParseVersion("1.0.0")

	if !v1.IsNewerThan(v2) {
		t.Error("2.0.0 should be newer than 1.0.0")
	}
	if v2.IsNewerThan(v1) {
		t.Error("1.0.0 should not be newer than 2.0.0")
	}
}

func TestVersion_IsOlderThan(t *testing.T) {
	v1, _ := ParseVersion("1.0.0")
	v2, _ := ParseVersion("2.0.0")

	if !v1.IsOlderThan(v2) {
		t.Error("1.0.0 should be older than 2.0.0")
	}
	if v2.IsOlderThan(v1) {
		t.Error("2.0.0 should not be older than 1.0.0")
	}
}

func TestVersion_IsCompatibleWith(t *testing.T) {
	v1, _ := ParseVersion("1.2.0")
	v2, _ := ParseVersion("1.5.0")
	v3, _ := ParseVersion("2.0.0")

	if !v1.IsCompatibleWith(v2) {
		t.Error("1.2.0 should be compatible with 1.5.0")
	}
	if v1.IsCompatibleWith(v3) {
		t.Error("1.2.0 should not be compatible with 2.0.0")
	}
}

func TestVersion_IsStable(t *testing.T) {
	tests := []struct {
		version string
		stable  bool
	}{
		{"1.0.0", true},
		{"1.0.0-alpha", false},
		{"1.0.0-rc.1", false},
		{"1.0.0+build", true}, // Build metadata doesn't affect stability
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			v, _ := ParseVersion(tt.version)
			if v.IsStable() != tt.stable {
				t.Errorf("IsStable() = %v, want %v", v.IsStable(), tt.stable)
			}
		})
	}
}

func TestParseConstraint(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		op      ConstraintOperator
	}{
		{"1.0.0", false, OpEqual},
		{"=1.0.0", false, OpEqual},
		{"!=1.0.0", false, OpNotEqual},
		{">1.0.0", false, OpGreater},
		{">=1.0.0", false, OpGreaterOrEqual},
		{"<1.0.0", false, OpLess},
		{"<=1.0.0", false, OpLessOrEqual},
		{"^1.0.0", false, OpCaret},
		{"~1.0.0", false, OpTilde},
		{"*", false, OpWildcard},
		{"latest", false, OpWildcard},
		{"", true, ""},
		{"invalid", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			c, err := ParseConstraint(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if c.Operator != tt.op {
				t.Errorf("Operator = %q, want %q", c.Operator, tt.op)
			}
		})
	}
}

func TestConstraint_Matches(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		matches    bool
	}{
		// Exact
		{"1.0.0", "1.0.0", true},
		{"1.0.0", "1.0.1", false},
		{"=1.0.0", "1.0.0", true},

		// Not equal
		{"!=1.0.0", "1.0.0", false},
		{"!=1.0.0", "1.0.1", true},

		// Greater
		{">1.0.0", "2.0.0", true},
		{">1.0.0", "1.0.0", false},
		{">1.0.0", "0.9.0", false},

		// Greater or equal
		{">=1.0.0", "2.0.0", true},
		{">=1.0.0", "1.0.0", true},
		{">=1.0.0", "0.9.0", false},

		// Less
		{"<2.0.0", "1.0.0", true},
		{"<2.0.0", "2.0.0", false},
		{"<2.0.0", "3.0.0", false},

		// Less or equal
		{"<=2.0.0", "1.0.0", true},
		{"<=2.0.0", "2.0.0", true},
		{"<=2.0.0", "3.0.0", false},

		// Caret (compatible)
		{"^1.2.3", "1.2.3", true},
		{"^1.2.3", "1.3.0", true},
		{"^1.2.3", "1.9.9", true},
		{"^1.2.3", "2.0.0", false},
		{"^1.2.3", "1.2.0", false},

		// Tilde (patch-level)
		{"~1.2.3", "1.2.3", true},
		{"~1.2.3", "1.2.9", true},
		{"~1.2.3", "1.3.0", false},
		{"~1.2.3", "2.0.0", false},
		{"~1.2.3", "1.2.0", false},

		// Wildcard
		{"*", "1.0.0", true},
		{"*", "2.5.3", true},
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.version, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("failed to parse constraint: %v", err)
			}

			v, err := ParseVersion(tt.version)
			if err != nil {
				t.Fatalf("failed to parse version: %v", err)
			}

			got := c.Matches(v)
			if got != tt.matches {
				t.Errorf("Matches() = %v, want %v", got, tt.matches)
			}
		})
	}
}

func TestParseConstraintSet(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
		count   int
	}{
		{">=1.0.0 <2.0.0", false, 2},
		{">=1.0.0, <2.0.0", false, 2},
		{"^1.0.0", false, 1},
		{"", false, 1}, // Empty becomes wildcard
		{">=1.0.0 <=2.0.0 !=1.5.0", false, 3},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cs, err := ParseConstraintSet(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(cs.Constraints) != tt.count {
				t.Errorf("constraint count = %d, want %d", len(cs.Constraints), tt.count)
			}
		})
	}
}

func TestConstraintSet_Matches(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		matches    bool
	}{
		{">=1.0.0 <2.0.0", "1.5.0", true},
		{">=1.0.0 <2.0.0", "0.9.0", false},
		{">=1.0.0 <2.0.0", "2.0.0", false},
		{">=1.0.0 <2.0.0 !=1.5.0", "1.5.0", false},
		{">=1.0.0 <2.0.0 !=1.5.0", "1.6.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.version, func(t *testing.T) {
			cs, _ := ParseConstraintSet(tt.constraint)
			v, _ := ParseVersion(tt.version)

			got := cs.Matches(v)
			if got != tt.matches {
				t.Errorf("Matches() = %v, want %v", got, tt.matches)
			}
		})
	}
}

func TestVersionResolver_ResolveVersionFromList(t *testing.T) {
	resolver := NewVersionResolver(nil)

	versions := []string{"1.0.0", "1.1.0", "1.2.0", "2.0.0", "2.1.0"}

	tests := []struct {
		constraint string
		want       string
		wantErr    bool
	}{
		{">=1.0.0 <2.0.0", "1.2.0", false},
		{"^1.0.0", "1.2.0", false},
		{"~1.1.0", "1.1.0", false},
		{">=2.0.0", "2.1.0", false},
		{"*", "2.1.0", false},
		{">=3.0.0", "", true}, // No matching version
	}

	for _, tt := range tests {
		t.Run(tt.constraint, func(t *testing.T) {
			got, err := resolver.ResolveVersionFromList(versions, tt.constraint)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("resolved = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSortVersions(t *testing.T) {
	versions := []string{"1.0.0", "2.0.0", "1.5.0", "0.9.0", "1.2.3"}
	sorted := SortVersions(versions)

	expected := []string{"2.0.0", "1.5.0", "1.2.3", "1.0.0", "0.9.0"}
	for i, v := range sorted {
		if v != expected[i] {
			t.Errorf("sorted[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

func TestFilterStableVersions(t *testing.T) {
	versions := []string{"1.0.0", "1.1.0-alpha", "1.2.0", "2.0.0-rc.1"}
	stable := FilterStableVersions(versions)

	if len(stable) != 2 {
		t.Errorf("expected 2 stable versions, got %d", len(stable))
	}

	expected := map[string]bool{"1.0.0": true, "1.2.0": true}
	for _, v := range stable {
		if !expected[v] {
			t.Errorf("unexpected stable version: %s", v)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cmp, err := CompareVersions("2.0.0", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmp != 1 {
		t.Errorf("CompareVersions(2.0.0, 1.0.0) = %d, want 1", cmp)
	}

	cmp, err = CompareVersions("1.0.0", "2.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmp != -1 {
		t.Errorf("CompareVersions(1.0.0, 2.0.0) = %d, want -1", cmp)
	}

	cmp, err = CompareVersions("1.0.0", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmp != 0 {
		t.Errorf("CompareVersions(1.0.0, 1.0.0) = %d, want 0", cmp)
	}
}

func TestIsVersionCompatible(t *testing.T) {
	compatible, err := IsVersionCompatible("1.2.0", "1.5.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !compatible {
		t.Error("1.2.0 and 1.5.0 should be compatible")
	}

	compatible, err = IsVersionCompatible("1.2.0", "2.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if compatible {
		t.Error("1.2.0 and 2.0.0 should not be compatible")
	}
}
