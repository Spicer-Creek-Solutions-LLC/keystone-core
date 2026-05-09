package targeting

import (
	"strings"
	"testing"

	"github.com/expr-lang/expr"
)

func TestCompile_OK(t *testing.T) {
	t.Parallel()

	cases := []string{
		"os:linux",
		"id:web-*",
		"role:web AND env:prod",
		"(role:db OR role:cache) AND status:online",
		"NOT role:legacy",
		`hostname:"db prod 1"`,
	}

	for _, in := range cases {
		in := in
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			te, err := Compile(in)
			if err != nil {
				t.Fatalf("Compile(%q): %v", in, err)
			}
			if te.Raw != in {
				t.Errorf("Raw = %q, want %q", te.Raw, in)
			}
			if te.Translated == "" {
				t.Error("Translated is empty")
			}
			if te.Program == nil {
				t.Error("Program is nil")
			}
		})
	}
}

func TestCompile_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{in: "", want: "empty expression"},
		{in: "   ", want: "empty expression"},
		{in: "role web", want: "expected ':'"},
		{in: `role:"web`, want: "unterminated"},
		{in: ":web", want: "unexpected character"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			_, err := Compile(tc.in)
			if err == nil {
				t.Fatalf("Compile(%q) = nil error, want substring %q", tc.in, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Compile(%q) error = %q, want substring %q", tc.in, err.Error(), tc.want)
			}
		})
	}
}

// TestCompile_Eval confirms that the compiled program produces correct
// boolean results when run against a stub env. Task 3 owns the full
// Matcher; this verifies wire-up only.
func TestCompile_Eval(t *testing.T) {
	t.Parallel()

	env := map[string]any{
		"id":       "web-01",
		"hostname": "web-prod-01",
		"os":       "linux",
		"arch":     "amd64",
		"status":   "online",
		"ip":       "10.0.1.5",
		"labels": map[string]string{
			"role": "web",
			"env":  "prod",
		},
	}

	cases := []struct {
		in   string
		want bool
	}{
		{in: "os:linux", want: true},
		{in: "os:darwin", want: false},
		{in: "id:web-*", want: true},
		{in: "id:db-*", want: false},
		{in: "role:web AND env:prod", want: true},
		{in: "role:web AND env:dev", want: false},
		{in: "role:db OR role:web", want: true},
		{in: "NOT role:legacy", want: true},
		{in: "NOT role:web", want: false},
		{in: "(role:db OR role:cache) AND status:online", want: false},
		{in: "(role:web OR role:cache) AND status:online", want: true},
		{in: `hostname:"web-prod-01"`, want: true},
		{in: "labels.env:prod", want: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			te, err := Compile(tc.in)
			if err != nil {
				t.Fatalf("Compile(%q): %v", tc.in, err)
			}
			out, err := expr.Run(te.Program, env)
			if err != nil {
				t.Fatalf("Run(%q): %v", tc.in, err)
			}
			got, ok := out.(bool)
			if !ok {
				t.Fatalf("Run(%q) returned %T, want bool", tc.in, out)
			}
			if got != tc.want {
				t.Errorf("Run(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
