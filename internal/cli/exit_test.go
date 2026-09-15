package cli

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// The codes are read out of PRODUCT-CHARTER.md rather than repeated here. A
// test listing them again would agree with a mistake it shared with the source.
func charterCodes(t *testing.T) map[int]bool {
	t.Helper()
	b, err := os.ReadFile("../../docs/project/PRODUCT-CHARTER.md")
	if err != nil {
		t.Fatalf("reading the charter: %v", err)
	}
	got := map[int]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `(\\d+)` \\| ").FindAllSubmatch(b, -1) {
		n, err := strconv.Atoi(string(m[1]))
		if err != nil {
			t.Fatalf("unparseable code %q", m[1])
		}
		got[n] = true
	}
	if len(got) < 5 {
		t.Fatalf("read %d codes from the charter; the pattern no longer matches its table", len(got))
	}
	return got
}

func TestEveryCharterCodeIsDefinedAndNoneIsInvented(t *testing.T) {
	charter := charterCodes(t)

	defined := map[int]bool{}
	for _, c := range All() {
		if defined[int(c)] {
			t.Errorf("code %d is listed twice by All()", c)
		}
		defined[int(c)] = true
	}

	for n := range charter {
		if !defined[n] {
			t.Errorf("the charter defines exit code %d and this package does not", n)
		}
	}
	for n := range defined {
		if !charter[n] {
			t.Errorf("this package defines exit code %d, which the charter does not", n)
		}
	}
}
