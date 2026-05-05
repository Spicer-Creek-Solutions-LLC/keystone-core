package semver

import "testing"

func TestNewConstraint(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		wantErr bool
	}{
		{"caret", "^1.2.3", false},
		{"tilde", "~1.2.3", false},
		{"wildcard", "1.x", false},
		{"compound", ">=1.0.0, <2.0.0", false},
		{"or", "1.x || 2.x", false},
		{"garbage", "not-a-constraint", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewConstraint(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewConstraint(%q) err = %v, wantErr %v", tt.expr, err, tt.wantErr)
			}
		})
	}
}

func TestConstraint_Check(t *testing.T) {
	tests := []struct {
		expr string
		ver  string
		want bool
	}{
		{"^1.2.3", "1.2.3", true},
		{"^1.2.3", "1.2.4", true},
		{"^1.2.3", "1.3.0", true},
		{"^1.2.3", "2.0.0", false},
		{"~1.2.3", "1.2.4", true},
		{"~1.2.3", "1.3.0", false},
		{"1.x", "1.0.0", true},
		{"1.x", "1.99.99", true},
		{"1.x", "2.0.0", false},
		{">=1.0.0, <2.0.0", "1.5.0", true},
		{">=1.0.0, <2.0.0", "2.0.0", false},
		{"1.x || 2.x", "1.5.0", true},
		{"1.x || 2.x", "2.5.0", true},
		{"1.x || 2.x", "3.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.expr+"@"+tt.ver, func(t *testing.T) {
			c, err := NewConstraint(tt.expr)
			if err != nil {
				t.Fatalf("NewConstraint(%q): %v", tt.expr, err)
			}
			if got := c.Check(MustParse(tt.ver)); got != tt.want {
				t.Errorf("Check(%s @ %s) = %v, want %v", tt.expr, tt.ver, got, tt.want)
			}
		})
	}
}

func TestConstraint_String(t *testing.T) {
	c, err := NewConstraint("^1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if c.String() == "" {
		t.Error("Constraint.String() must not be empty")
	}
}
