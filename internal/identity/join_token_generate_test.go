// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"strings"
	"testing"
)

func TestRandomBase62_Length(t *testing.T) {
	t.Parallel()
	for _, n := range []int{1, 8, 40, 100} {
		got, err := randomBase62(n)
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		if len(got) != n {
			t.Errorf("n=%d: got len %d", n, len(got))
		}
	}
}

func TestRandomBase62_Alphabet(t *testing.T) {
	t.Parallel()
	got, err := randomBase62(1024)
	if err != nil {
		t.Fatalf("randomBase62: %v", err)
	}
	for _, c := range got {
		if !strings.ContainsRune(base62Alphabet, c) {
			t.Errorf("character %q outside base62 alphabet", c)
		}
	}
}

func TestRandomBase62_RejectsNonPositive(t *testing.T) {
	t.Parallel()
	for _, n := range []int{0, -1, -100} {
		if _, err := randomBase62(n); err == nil {
			t.Errorf("n=%d accepted; want error", n)
		}
	}
}

func TestRandomBase62_Distinct(t *testing.T) {
	t.Parallel()
	// Statistically near-zero collision risk at 40 chars × 62
	// alphabet (~ 2^238 entropy). 100 samples is a noise-floor
	// sanity check.
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		s, err := randomBase62(40)
		if err != nil {
			t.Fatalf("randomBase62: %v", err)
		}
		if _, dup := seen[s]; dup {
			t.Errorf("collision after %d samples: %q", i, s)
		}
		seen[s] = struct{}{}
	}
}

func TestRandomBase62_Unbiased(t *testing.T) {
	t.Parallel()
	// Regression for the old byte%62 skew (256 mod 62 = 8 made the
	// first 8 symbols ~25% more likely). Rejection sampling makes every
	// symbol equiprobable; assert each appears within ±13% of its
	// expected share over a large sample. Random noise here is <6%, so
	// the ~25% old bias is caught without false failures.
	const perChar = 3000
	s, err := randomBase62(len(base62Alphabet) * perChar)
	if err != nil {
		t.Fatalf("randomBase62: %v", err)
	}
	counts := make(map[rune]int, len(base62Alphabet))
	for _, c := range s {
		counts[c]++
	}
	if len(counts) != len(base62Alphabet) {
		t.Fatalf("only %d distinct symbols, want %d", len(counts), len(base62Alphabet))
	}
	lo, hi := int(perChar*0.87), int(perChar*1.13)
	for _, c := range base62Alphabet {
		if n := counts[c]; n < lo || n > hi {
			t.Errorf("symbol %q count %d outside [%d,%d] — biased distribution", string(c), n, lo, hi)
		}
	}
}

func TestRandomSalt(t *testing.T) {
	t.Parallel()
	a, err := randomSalt()
	if err != nil {
		t.Fatalf("randomSalt: %v", err)
	}
	if len(a) != joinTokenSaltLen {
		t.Errorf("len = %d, want %d", len(a), joinTokenSaltLen)
	}
	b, _ := randomSalt()
	if string(a) == string(b) {
		t.Error("two salts identical — random source not actually random?")
	}
}
