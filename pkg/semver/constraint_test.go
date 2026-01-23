package semver

import (
	"testing"
)

func TestParseConstraint_Exact(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		want       bool
	}{
		{"1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", false},
		{"=1.2.3", "1.2.3", true},
		{"=1.2.3", "1.2.4", false},
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.version, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q) error = %v", tt.constraint, err)
			}
			if got := c.Check(MustParse(tt.version)); got != tt.want {
				t.Errorf("Check(%s) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestParseConstraint_Range(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		want       bool
	}{
		// Greater than
		{">1.0.0", "1.0.1", true},
		{">1.0.0", "1.0.0", false},
		{">1.0.0", "0.9.9", false},
		{">1.0.0", "2.0.0", true},

		// Greater than or equal
		{">=1.0.0", "1.0.0", true},
		{">=1.0.0", "1.0.1", true},
		{">=1.0.0", "0.9.9", false},

		// Less than
		{"<2.0.0", "1.9.9", true},
		{"<2.0.0", "2.0.0", false},
		{"<2.0.0", "2.0.1", false},

		// Less than or equal
		{"<=2.0.0", "2.0.0", true},
		{"<=2.0.0", "1.9.9", true},
		{"<=2.0.0", "2.0.1", false},

		// Not equal
		{"!=1.5.0", "1.4.0", true},
		{"!=1.5.0", "1.5.0", false},
		{"!=1.5.0", "1.6.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.version, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q) error = %v", tt.constraint, err)
			}
			if got := c.Check(MustParse(tt.version)); got != tt.want {
				t.Errorf("Check(%s) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestParseConstraint_Caret(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		want       bool
	}{
		// ^1.2.3 means >=1.2.3 <2.0.0
		{"^1.2.3", "1.2.3", true},
		{"^1.2.3", "1.2.4", true},
		{"^1.2.3", "1.9.9", true},
		{"^1.2.3", "2.0.0", false},
		{"^1.2.3", "1.2.2", false},

		// ^0.2.3 means >=0.2.3 <0.3.0
		{"^0.2.3", "0.2.3", true},
		{"^0.2.3", "0.2.9", true},
		{"^0.2.3", "0.3.0", false},
		{"^0.2.3", "0.2.2", false},

		// ^0.0.3 means >=0.0.3 <0.0.4
		{"^0.0.3", "0.0.3", true},
		{"^0.0.3", "0.0.4", false},
		{"^0.0.3", "0.0.2", false},
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.version, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q) error = %v", tt.constraint, err)
			}
			if got := c.Check(MustParse(tt.version)); got != tt.want {
				t.Errorf("Check(%s) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestParseConstraint_Tilde(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		want       bool
	}{
		// ~1.2.3 means >=1.2.3 <1.3.0
		{"~1.2.3", "1.2.3", true},
		{"~1.2.3", "1.2.9", true},
		{"~1.2.3", "1.3.0", false},
		{"~1.2.3", "1.2.2", false},
		{"~1.2.3", "2.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.version, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q) error = %v", tt.constraint, err)
			}
			if got := c.Check(MustParse(tt.version)); got != tt.want {
				t.Errorf("Check(%s) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestParseConstraint_Wildcard(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		want       bool
	}{
		// 1.x means >=1.0.0 <2.0.0
		{"1.x", "1.0.0", true},
		{"1.x", "1.9.9", true},
		{"1.x", "2.0.0", false},
		{"1.x", "0.9.9", false},

		// 1.* same as 1.x
		{"1.*", "1.5.0", true},
		{"1.*", "2.0.0", false},

		// 1.2.x means >=1.2.0 <1.3.0
		{"1.2.x", "1.2.0", true},
		{"1.2.x", "1.2.9", true},
		{"1.2.x", "1.3.0", false},
		{"1.2.x", "1.1.9", false},

		// 1.2.* same as 1.2.x
		{"1.2.*", "1.2.5", true},
		{"1.2.*", "1.3.0", false},

		// * means any
		{"*", "1.0.0", true},
		{"*", "0.0.1", true},
		{"*", "999.999.999", true},
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.version, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q) error = %v", tt.constraint, err)
			}
			if got := c.Check(MustParse(tt.version)); got != tt.want {
				t.Errorf("Check(%s) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestParseConstraint_Compound(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		want       bool
	}{
		// Space-separated AND
		{">=1.0.0 <2.0.0", "1.0.0", true},
		{">=1.0.0 <2.0.0", "1.5.0", true},
		{">=1.0.0 <2.0.0", "2.0.0", false},
		{">=1.0.0 <2.0.0", "0.9.9", false},

		// Comma-separated AND
		{">=1.0.0, <2.0.0", "1.5.0", true},
		{">=1.0.0, <2.0.0", "2.0.0", false},

		// Multiple conditions
		{">=1.0.0 <2.0.0 !=1.5.0", "1.4.0", true},
		{">=1.0.0 <2.0.0 !=1.5.0", "1.5.0", false},
		{">=1.0.0 <2.0.0 !=1.5.0", "1.6.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.version, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q) error = %v", tt.constraint, err)
			}
			if got := c.Check(MustParse(tt.version)); got != tt.want {
				t.Errorf("Check(%s) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestParseConstraint_Or(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		want       bool
	}{
		// OR constraint
		{"^1.0.0 || ^2.0.0", "1.5.0", true},
		{"^1.0.0 || ^2.0.0", "2.5.0", true},
		{"^1.0.0 || ^2.0.0", "3.0.0", false},
		{"^1.0.0 || ^2.0.0", "0.9.0", false},

		// Complex OR
		{">=1.0.0 <1.5.0 || >=2.0.0 <2.5.0", "1.2.0", true},
		{">=1.0.0 <1.5.0 || >=2.0.0 <2.5.0", "2.2.0", true},
		{">=1.0.0 <1.5.0 || >=2.0.0 <2.5.0", "1.7.0", false},
		{">=1.0.0 <1.5.0 || >=2.0.0 <2.5.0", "2.7.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.version, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q) error = %v", tt.constraint, err)
			}
			if got := c.Check(MustParse(tt.version)); got != tt.want {
				t.Errorf("Check(%s) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestParseConstraint_WithPrerelease(t *testing.T) {
	tests := []struct {
		constraint string
		version    string
		want       bool
	}{
		{">=1.0.0-alpha", "1.0.0-alpha", true},
		{">=1.0.0-alpha", "1.0.0-beta", true},
		{">=1.0.0-alpha", "1.0.0", true},
		{"<1.0.0", "1.0.0-alpha", true}, // Prerelease is less than release
	}

	for _, tt := range tests {
		t.Run(tt.constraint+"_"+tt.version, func(t *testing.T) {
			c, err := ParseConstraint(tt.constraint)
			if err != nil {
				t.Fatalf("ParseConstraint(%q) error = %v", tt.constraint, err)
			}
			if got := c.Check(MustParse(tt.version)); got != tt.want {
				t.Errorf("Check(%s) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestParseConstraint_String(t *testing.T) {
	tests := []string{
		"1.2.3",
		">=1.0.0",
		"^1.2.3",
		"~1.2.3",
		"1.x",
		"*",
	}

	for _, s := range tests {
		c, err := ParseConstraint(s)
		if err != nil {
			t.Fatalf("ParseConstraint(%q) error = %v", s, err)
		}
		// String() should return something meaningful
		if c.String() == "" {
			t.Errorf("Constraint.String() is empty for %q", s)
		}
	}
}

func TestParseConstraint_Invalid(t *testing.T) {
	tests := []string{
		"",
		"invalid",
		"abc.def.ghi",
		">>1.0.0",
		"1.2.3.4",
	}

	for _, s := range tests {
		t.Run(s, func(t *testing.T) {
			_, err := ParseConstraint(s)
			if err == nil {
				t.Errorf("ParseConstraint(%q) should return error", s)
			}
		})
	}
}

func TestMustParseConstraint(t *testing.T) {
	// Should not panic
	c := MustParseConstraint("^1.2.3")
	if !c.Check(MustParse("1.5.0")) {
		t.Error("MustParseConstraint returned invalid constraint")
	}

	// Should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParseConstraint with invalid constraint should panic")
		}
	}()
	MustParseConstraint("invalid")
}
