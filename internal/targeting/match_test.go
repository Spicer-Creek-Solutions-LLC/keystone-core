package targeting

import "testing"

func TestMatchValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		value   any
		pattern string
		want    bool
	}{
		{name: "literal hit", value: "linux", pattern: "linux", want: true},
		{name: "literal miss", value: "darwin", pattern: "linux", want: false},
		{name: "literal case-sensitive", value: "Linux", pattern: "linux", want: false},
		{name: "glob prefix hit", value: "web-01", pattern: "web-*", want: true},
		{name: "glob prefix miss", value: "db-01", pattern: "web-*", want: false},
		{name: "glob suffix hit", value: "db-prod-01", pattern: "*-01", want: true},
		{name: "glob single-char", value: "a", pattern: "?", want: true},
		{name: "glob single-char miss", value: "ab", pattern: "?", want: false},
		{name: "nil value", value: nil, pattern: "", want: true},
		{name: "nil value vs literal", value: nil, pattern: "linux", want: false},
		{name: "stringer value", value: stubStringer("amd64"), pattern: "amd64", want: true},
		{name: "non-string fallback", value: 42, pattern: "42", want: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := matchValue(tc.value, tc.pattern); got != tc.want {
				t.Errorf("matchValue(%v, %q) = %v, want %v", tc.value, tc.pattern, got, tc.want)
			}
		})
	}
}

func TestMatchValue_BadGlob(t *testing.T) {
	t.Parallel()
	// Unmatched `[` produces a glob-compile error; matchValue must not
	// panic and must report no match.
	if matchValue("anything", "[unclosed") {
		t.Error("unclosed bracket pattern should not match")
	}
}

func TestGlobCache_Reuse(t *testing.T) {
	t.Parallel()
	g1, err1 := getOrCompileGlob("svc-*")
	if err1 != nil {
		t.Fatalf("first compile: %v", err1)
	}
	g2, err2 := getOrCompileGlob("svc-*")
	if err2 != nil {
		t.Fatalf("second compile: %v", err2)
	}
	// Same pattern must return the cached entry, not a fresh compile.
	if g1 != g2 {
		t.Error("cache miss: getOrCompileGlob returned a different glob for the same pattern")
	}
}

func TestMatchAny_Adapter(t *testing.T) {
	t.Parallel()

	got, err := matchAny("web-01", "web-*")
	if err != nil {
		t.Fatalf("matchAny: %v", err)
	}
	if got != true {
		t.Errorf("matchAny(web-01, web-*) = %v, want true", got)
	}

	if _, err := matchAny("only-one"); err == nil {
		t.Error("matchAny with 1 arg: expected error")
	}
	if _, err := matchAny("v", 7); err == nil {
		t.Error("matchAny with non-string pattern: expected error")
	}
}

type stubStringer string

func (s stubStringer) String() string { return string(s) }
