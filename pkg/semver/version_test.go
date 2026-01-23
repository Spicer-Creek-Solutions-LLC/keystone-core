package semver

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input      string
		want       Version
		wantErr    bool
	}{
		// Basic versions
		{"1.2.3", Version{Major: 1, Minor: 2, Patch: 3}, false},
		{"0.0.0", Version{Major: 0, Minor: 0, Patch: 0}, false},
		{"10.20.30", Version{Major: 10, Minor: 20, Patch: 30}, false},

		// With v prefix
		{"v1.2.3", Version{Major: 1, Minor: 2, Patch: 3}, false},
		{"V1.2.3", Version{}, true}, // Only lowercase v supported

		// With prerelease
		{"1.2.3-alpha", Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha"}, false},
		{"1.2.3-alpha.1", Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha.1"}, false},
		{"1.2.3-0.3.7", Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "0.3.7"}, false},
		{"1.2.3-x.7.z.92", Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "x.7.z.92"}, false},
		{"1.0.0-alpha.beta", Version{Major: 1, Minor: 0, Patch: 0, Prerelease: "alpha.beta"}, false},

		// With build metadata
		{"1.2.3+build", Version{Major: 1, Minor: 2, Patch: 3, Build: "build"}, false},
		{"1.2.3+build.123", Version{Major: 1, Minor: 2, Patch: 3, Build: "build.123"}, false},
		{"1.2.3+20130313144700", Version{Major: 1, Minor: 2, Patch: 3, Build: "20130313144700"}, false},

		// With both
		{"1.2.3-alpha+build", Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha", Build: "build"}, false},
		{"1.2.3-beta.1+exp.sha.5114f85", Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "beta.1", Build: "exp.sha.5114f85"}, false},

		// Invalid versions
		{"", Version{}, true},
		{"1", Version{}, true},
		{"1.2", Version{}, true},
		{"1.2.3.4", Version{}, true},
		{"a.b.c", Version{}, true},
		{"-1.2.3", Version{}, true},
		{"1.-2.3", Version{}, true},
		{"1.2.-3", Version{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("Parse(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMustParse(t *testing.T) {
	// Should not panic
	v := MustParse("1.2.3")
	if v.Major != 1 || v.Minor != 2 || v.Patch != 3 {
		t.Errorf("MustParse(1.2.3) = %v", v)
	}

	// Should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParse with invalid version should panic")
		}
	}()
	MustParse("invalid")
}

func TestIsValid(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"1.2.3", true},
		{"v1.2.3", true},
		{"1.2.3-alpha", true},
		{"1.2.3+build", true},
		{"invalid", false},
		{"", false},
		{"1.2", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := IsValid(tt.input); got != tt.want {
				t.Errorf("IsValid(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestVersion_String(t *testing.T) {
	tests := []struct {
		version Version
		want    string
	}{
		{Version{Major: 1, Minor: 2, Patch: 3}, "1.2.3"},
		{Version{Major: 0, Minor: 0, Patch: 0}, "0.0.0"},
		{Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha"}, "1.2.3-alpha"},
		{Version{Major: 1, Minor: 2, Patch: 3, Build: "build"}, "1.2.3+build"},
		{Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha", Build: "build"}, "1.2.3-alpha+build"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.version.String(); got != tt.want {
				t.Errorf("Version.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersion_IsZero(t *testing.T) {
	if !(Version{}).IsZero() {
		t.Error("Zero version should be zero")
	}
	if New(1, 0, 0).IsZero() {
		t.Error("1.0.0 should not be zero")
	}
	if NewPrerelease(0, 0, 0, "alpha").IsZero() {
		t.Error("0.0.0-alpha should not be zero")
	}
}

func TestVersion_IsPrerelease(t *testing.T) {
	if New(1, 0, 0).IsPrerelease() {
		t.Error("1.0.0 should not be prerelease")
	}
	if !NewPrerelease(1, 0, 0, "alpha").IsPrerelease() {
		t.Error("1.0.0-alpha should be prerelease")
	}
}

func TestVersion_IsStable(t *testing.T) {
	if !New(1, 0, 0).IsStable() {
		t.Error("1.0.0 should be stable")
	}
	if New(0, 1, 0).IsStable() {
		t.Error("0.1.0 should not be stable")
	}
	if NewPrerelease(1, 0, 0, "alpha").IsStable() {
		t.Error("1.0.0-alpha should not be stable")
	}
}

func TestVersion_Core(t *testing.T) {
	v := Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "alpha", Build: "build"}
	core := v.Core()
	if core.Major != 1 || core.Minor != 2 || core.Patch != 3 || core.Prerelease != "" || core.Build != "" {
		t.Errorf("Core() = %v, want 1.2.3", core)
	}
}

func TestVersion_Next(t *testing.T) {
	v := New(1, 2, 3)

	if got := v.NextPatch(); got != New(1, 2, 4) {
		t.Errorf("NextPatch() = %v, want 1.2.4", got)
	}
	if got := v.NextMinor(); got != New(1, 3, 0) {
		t.Errorf("NextMinor() = %v, want 1.3.0", got)
	}
	if got := v.NextMajor(); got != New(2, 0, 0) {
		t.Errorf("NextMajor() = %v, want 2.0.0", got)
	}
}

func TestVersion_With(t *testing.T) {
	v := New(1, 2, 3)

	if got := v.WithPrerelease("alpha"); got.Prerelease != "alpha" {
		t.Errorf("WithPrerelease() = %v, want prerelease=alpha", got)
	}
	if got := v.WithBuild("build"); got.Build != "build" {
		t.Errorf("WithBuild() = %v, want build=build", got)
	}
}

func TestVersion_Compare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		// Basic comparisons
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "1.0.0", 0},

		// Minor version
		{"1.0.0", "1.1.0", -1},
		{"1.1.0", "1.0.0", 1},

		// Patch version
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},

		// Prerelease has lower precedence
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0", "1.0.0-alpha", 1},

		// Prerelease comparisons
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1},
		{"1.0.0-alpha.beta", "1.0.0-beta", -1},
		{"1.0.0-beta", "1.0.0-beta.2", -1},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1},
		{"1.0.0-beta.11", "1.0.0-rc.1", -1},

		// Build metadata ignored
		{"1.0.0+build1", "1.0.0+build2", 0},
		{"1.0.0+build", "1.0.0", 0},

		// Real-world examples
		{"1.9.0", "1.10.0", -1},  // Numeric comparison, not string
		{"2.0.0", "10.0.0", -1},  // Numeric comparison
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			a := MustParse(tt.a)
			b := MustParse(tt.b)
			if got := a.Compare(b); got != tt.want {
				t.Errorf("Compare(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestVersion_ComparisonMethods(t *testing.T) {
	v1 := MustParse("1.0.0")
	v2 := MustParse("2.0.0")

	if !v1.LessThan(v2) {
		t.Error("1.0.0 should be less than 2.0.0")
	}
	if !v1.LessThanOrEqual(v2) {
		t.Error("1.0.0 should be less than or equal to 2.0.0")
	}
	if !v2.GreaterThan(v1) {
		t.Error("2.0.0 should be greater than 1.0.0")
	}
	if !v2.GreaterThanOrEqual(v1) {
		t.Error("2.0.0 should be greater than or equal to 1.0.0")
	}
	if !v1.Equal(v1) {
		t.Error("1.0.0 should equal 1.0.0")
	}
	if v1.Equal(v2) {
		t.Error("1.0.0 should not equal 2.0.0")
	}

	// Equal ignores build metadata
	v1b := MustParse("1.0.0+build")
	if !v1.Equal(v1b) {
		t.Error("1.0.0 should equal 1.0.0+build")
	}
}
