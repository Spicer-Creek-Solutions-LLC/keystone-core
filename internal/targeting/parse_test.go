// SPDX-License-Identifier: Apache-2.0

package targeting

import (
	"strings"
	"testing"
)

func TestTranslate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "builtin literal",
			in:   "os:linux",
			want: `match(os, "linux")`,
		},
		{
			name: "builtin glob",
			in:   "id:web-*",
			want: `match(id, "web-*")`,
		},
		{
			name: "label sugar",
			in:   "role:web",
			want: `match(labels.role, "web")`,
		},
		{
			name: "explicit labels prefix",
			in:   "labels.env:prod",
			want: `match(labels.env, "prod")`,
		},
		{
			name: "AND compound",
			in:   "role:web AND env:prod",
			want: `match(labels.role, "web") and match(labels.env, "prod")`,
		},
		{
			name: "OR lowercase",
			in:   "role:db or role:cache",
			want: `match(labels.role, "db") or match(labels.role, "cache")`,
		},
		{
			name: "NOT",
			in:   "NOT role:legacy",
			want: `not match(labels.role, "legacy")`,
		},
		{
			name: "symbolic operators",
			in:   "role:web && env:prod || role:cache",
			want: `match(labels.role, "web") and match(labels.env, "prod") or match(labels.role, "cache")`,
		},
		{
			name: "bang operator",
			in:   "!role:legacy",
			want: `not match(labels.role, "legacy")`,
		},
		{
			name: "parens",
			in:   "(role:web OR role:cache) AND env:prod",
			want: `(match(labels.role, "web") or match(labels.role, "cache")) and match(labels.env, "prod")`,
		},
		{
			name: "quoted value with space",
			in:   `hostname:"db prod 1"`,
			want: `match(hostname, "db prod 1")`,
		},
		{
			name: "single-quoted value",
			in:   `role:'web tier'`,
			want: `match(labels.role, "web tier")`,
		},
		{
			name: "value with dot (ip-shaped)",
			in:   "ip:10.0.0.1",
			want: `match(ip, "10.0.0.1")`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := translate(tc.in)
			if err != nil {
				t.Fatalf("translate(%q) error: %v", tc.in, err)
			}
			if normalize(got) != normalize(tc.want) {
				t.Errorf("translate(%q)\n  got:  %s\n  want: %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestTranslate_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string // substring expected in the error message
	}{
		{name: "missing colon", in: "role web", want: "expected ':'"},
		{name: "dangling colon", in: "role:", want: "after ':'"},
		{name: "empty quoted", in: `role:""`, want: ""}, // empty quoted is allowed; this case asserts no error
		{name: "unterminated quote", in: `role:"web`, want: "unterminated"},
		{name: "leading colon", in: ":web", want: "unexpected character"},
		{name: "stray digit field", in: "1role:web", want: "unexpected character"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := translate(tc.in)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("translate(%q) unexpected error: %v", tc.in, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("translate(%q) = nil error, want substring %q", tc.in, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("translate(%q) error %q, want substring %q", tc.in, err.Error(), tc.want)
			}
		})
	}
}

// normalize collapses runs of whitespace to a single space so the
// translator's spacing decisions don't make assertions brittle.
func normalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
