package resolver

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *Version
		wantErr bool
	}{
		{
			name:  "simple version",
			input: "1.2.3",
			want: &Version{
				Major:    1,
				Minor:    2,
				Patch:    3,
				Original: "1.2.3",
			},
		},
		{
			name:  "version with v prefix",
			input: "v1.2.3",
			want: &Version{
				Major:    1,
				Minor:    2,
				Patch:    3,
				Original: "v1.2.3",
			},
		},
		{
			name:  "version with prerelease",
			input: "1.2.3-alpha.1",
			want: &Version{
				Major:      1,
				Minor:      2,
				Patch:      3,
				Prerelease: "alpha.1",
				Original:   "1.2.3-alpha.1",
			},
		},
		{
			name:  "version with build metadata",
			input: "1.2.3+20230101",
			want: &Version{
				Major:    1,
				Minor:    2,
				Patch:    3,
				Build:    "20230101",
				Original: "1.2.3+20230101",
			},
		},
		{
			name:  "version with prerelease and build",
			input: "1.2.3-beta.2+build.123",
			want: &Version{
				Major:      1,
				Minor:      2,
				Patch:      3,
				Prerelease: "beta.2",
				Build:      "build.123",
				Original:   "1.2.3-beta.2+build.123",
			},
		},
		{
			name:    "invalid version",
			input:   "1.2",
			wantErr: true,
		},
		{
			name:    "empty version",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   "abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Major != tt.want.Major || got.Minor != tt.want.Minor || got.Patch != tt.want.Patch {
					t.Errorf("ParseVersion() = %v, want %v", got, tt.want)
				}
				if got.Prerelease != tt.want.Prerelease || got.Build != tt.want.Build {
					t.Errorf("ParseVersion() metadata = %v+%v, want %v+%v",
						got.Prerelease, got.Build, tt.want.Prerelease, tt.want.Build)
				}
			}
		})
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{"equal versions", "1.2.3", "1.2.3", 0},
		{"major greater", "2.0.0", "1.9.9", 1},
		{"major less", "1.0.0", "2.0.0", -1},
		{"minor greater", "1.3.0", "1.2.9", 1},
		{"minor less", "1.2.0", "1.3.0", -1},
		{"patch greater", "1.2.4", "1.2.3", 1},
		{"patch less", "1.2.3", "1.2.4", -1},
		{"release > prerelease", "1.2.3", "1.2.3-alpha", 1},
		{"prerelease < release", "1.2.3-beta", "1.2.3", -1},
		{"prerelease comparison", "1.2.3-alpha.2", "1.2.3-alpha.1", 1},
		{"prerelease numeric vs alpha", "1.2.3-1", "1.2.3-alpha", -1},
		{"build metadata ignored", "1.2.3+build1", "1.2.3+build2", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			va, _ := ParseVersion(tt.a)
			vb, _ := ParseVersion(tt.b)
			got := va.Compare(vb)
			if got != tt.want {
				t.Errorf("Compare() = %v, want %v (comparing %s to %s)", got, tt.want, tt.a, tt.b)
			}
		})
	}
}

func TestConstraintMatches(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		version    string
		want       bool
	}{
		// Exact matches
		{"exact match", "=1.2.3", "1.2.3", true},
		{"exact no match", "=1.2.3", "1.2.4", false},
		{"double equals", "==1.2.3", "1.2.3", true},

		// Inequality
		{"not equal match", "!=1.2.3", "1.2.4", true},
		{"not equal no match", "!=1.2.3", "1.2.3", false},

		// Greater than
		{">= match", ">=1.2.3", "1.2.3", true},
		{">= match higher", ">=1.2.3", "1.3.0", true},
		{">= no match", ">=1.2.3", "1.2.2", false},
		{"> match", ">1.2.3", "1.2.4", true},
		{"> no match", ">1.2.3", "1.2.3", false},

		// Less than
		{"<= match", "<=1.2.3", "1.2.3", true},
		{"<= match lower", "<=1.2.3", "1.2.0", true},
		{"<= no match", "<=1.2.3", "1.2.4", false},
		{"< match", "<1.2.3", "1.2.2", true},
		{"< no match", "<1.2.3", "1.2.3", false},

		// Caret (^)
		{"^ major match", "^1.2.3", "1.5.0", true},
		{"^ major no match", "^1.2.3", "2.0.0", false},
		{"^ minor match 0.x", "^0.2.3", "0.2.5", true},
		{"^ minor no match 0.x", "^0.2.3", "0.3.0", false},
		{"^ patch match 0.0.x", "^0.0.3", "0.0.3", true},
		{"^ patch no match 0.0.x", "^0.0.3", "0.0.4", false},

		// Tilde (~)
		{"~ match", "~1.2.3", "1.2.5", true},
		{"~ no match minor", "~1.2.3", "1.3.0", false},
		{"~ no match major", "~1.2.3", "2.0.0", false},
	}

	parser := &DefaultConstraintParser{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constraint, err := parser.Parse(tt.constraint)
			if err != nil {
				t.Fatalf("failed to parse constraint: %v", err)
			}

			got := constraint.Matches(tt.version)
			if got != tt.want {
				t.Errorf("Matches() = %v, want %v (constraint %s, version %s)", got, tt.want, tt.constraint, tt.version)
			}
		})
	}
}

func TestParseConstraint(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"exact version", "1.2.3", false},
		{"with operator", ">=1.2.3", false},
		{"caret", "^1.2.3", false},
		{"tilde", "~1.2.3", false},
		{"wildcard", "*", false},
		{"latest", "latest", false},
		{"empty", "", true},
		{"invalid version", ">=abc", true},
	}

	parser := &DefaultConstraintParser{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseMultipleConstraints(t *testing.T) {
	parser := &DefaultConstraintParser{}

	// Test multiple constraints (AND)
	constraints := []string{">=1.0.0", "<2.0.0"}
	mc, err := parser.ParseMultiple(constraints)
	if err != nil {
		t.Fatalf("ParseMultiple() error = %v", err)
	}

	tests := []struct {
		version string
		want    bool
	}{
		{"0.9.0", false},
		{"1.0.0", true},
		{"1.5.0", true},
		{"2.0.0", false},
		{"2.1.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := mc.Matches(tt.version)
			if got != tt.want {
				t.Errorf("Matches(%s) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestVersionSelector(t *testing.T) {
	selector := &DefaultVersionSelector{}
	parser := &DefaultConstraintParser{}

	available := []string{"1.0.0", "1.1.0", "1.2.0", "2.0.0", "2.1.0"}

	tests := []struct {
		name       string
		constraint string
		want       string
	}{
		{"select highest matching ^1.0.0", "^1.0.0", "1.2.0"},
		{"select highest matching ^2.0.0", "^2.0.0", "2.1.0"},
		{"select highest matching >=1.1.0 <2.0.0", ">=1.1.0", "2.1.0"}, // Note: single constraint
		{"select exact", "=1.1.0", "1.1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			constraint, err := parser.Parse(tt.constraint)
			if err != nil {
				t.Fatalf("failed to parse constraint: %v", err)
			}

			got, err := selector.Select(constraint, available)
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}

			if got != tt.want {
				t.Errorf("Select() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectHighest(t *testing.T) {
	selector := &DefaultVersionSelector{}

	available := []string{"1.0.0", "2.1.0", "1.5.0", "2.0.0"}
	got, err := selector.SelectHighest(available)
	if err != nil {
		t.Fatalf("SelectHighest() error = %v", err)
	}

	want := "2.1.0"
	if got != want {
		t.Errorf("SelectHighest() = %v, want %v", got, want)
	}
}

func TestSelectLowest(t *testing.T) {
	selector := &DefaultVersionSelector{}

	available := []string{"2.0.0", "1.5.0", "1.0.0", "2.1.0"}
	got, err := selector.SelectLowest(available)
	if err != nil {
		t.Fatalf("SelectLowest() error = %v", err)
	}

	want := "1.0.0"
	if got != want {
		t.Errorf("SelectLowest() = %v, want %v", got, want)
	}
}

func TestVersionString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "1.2.3", "1.2.3"},
		{"with prerelease", "1.2.3-alpha.1", "1.2.3-alpha.1"},
		{"with build", "1.2.3+build", "1.2.3+build"},
		{"with both", "1.2.3-beta+build", "1.2.3-beta+build"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := ParseVersion(tt.input)
			if err != nil {
				t.Fatalf("ParseVersion() error = %v", err)
			}

			got := v.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsPrerelease(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"1.2.3", false},
		{"1.2.3-alpha", true},
		{"1.2.3-beta.1", true},
		{"1.2.3+build", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			v, _ := ParseVersion(tt.version)
			got := v.IsPrerelease()
			if got != tt.want {
				t.Errorf("IsPrerelease() = %v, want %v", got, tt.want)
			}
		})
	}
}
