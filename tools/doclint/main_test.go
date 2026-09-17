package main

import (
	"regexp"
	"strings"
	"testing"
)

// The classifiers decide whether a hit on a superseded conclusion is reported.
// A classifier that fires too readily disposes of live hits silently, which is
// the failure mode the whole tool exists to avoid -- so each one is tested in
// both directions, and the negative case is the one that matters.
func TestDescribesRequiresWordBoundaries(t *testing.T) {
	for _, c := range []struct {
		text  string
		match bool
		why   string
	}{
		{"it is not, and G22 corrected that claim", true, "the case the classifier exists for"},
		{"It Is Not the same thing", true, "case-insensitive"},
		{"it is notable that the runner queued", false, "review of #339: `not` is a prefix of `notable`"},
		{"an earlier draft said otherwise", true, ""},
		{"an earlier versioned protocol", false, "`version` is a prefix of `versioned`"},
		{"was carried here from P08", true, ""},
		{"was carried today by the agent", false, "`to` is a prefix of `today`"},
	} {
		if got := describes.MatchString(c.text); got != c.match {
			t.Errorf("describes(%q) = %v, want %v  %s", c.text, got, c.match, c.why)
		}
	}
}

func TestNegatedRequiresWordBoundaries(t *testing.T) {
	for _, c := range []struct {
		text  string
		match bool
	}{
		{"the operator no longer", true},
		{"this is not", true},
		{"a job cannot", false}, // `not` is the tail of `cannot`
		{"the run was never", true},
		{"whenever", false}, // `never` is the tail of `whenever`
	} {
		if got := negated.MatchString(c.text); got != c.match {
			t.Errorf("negated(%q) = %v, want %v", c.text, got, c.match)
		}
	}
}

// G28: bold is assertion in this repository, not quotation. Treating it as
// quotation meant the claims most worth sweeping were the ones no rule could
// fire on.
func TestEmphasisedTreatsBoldAsAssertion(t *testing.T) {
	for _, c := range []struct {
		unit string
		hit  string
		want bool
	}{
		{"read *one file per task* and it is wrong", "one file per task", true},
		{"**No runner available can run containers**, so", "No runner", false},
		{"a **bold** phrase then the claim runs containers here", "runs containers", false},
		{"*quoted* then plain then *quoted again*", "then plain then", false},
	} {
		at := strings.Index(c.unit, c.hit)
		if at < 0 {
			t.Fatalf("test is wrong: %q not in %q", c.hit, c.unit)
		}
		if got := emphasised(c.unit, at); got != c.want {
			t.Errorf("emphasised(%q, hit=%q) = %v, want %v", c.unit, c.hit, got, c.want)
		}
	}
}

// A control character inside a regex literal is invisible in every editor and
// silently kills the alternative it lands in. `describes` carried two literal
// backspaces from P11b until G28. `make whitespace-check` guards the tree; this
// guards the patterns the tool actually compiles.
func TestNoControlCharactersInPatterns(t *testing.T) {
	ctrl := regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
	check := func(name, pat string) {
		if loc := ctrl.FindStringIndex(pat); loc != nil {
			t.Errorf("%s holds a control character at byte %d: %q", name, loc[0], pat)
		}
	}
	check("negated", negated.String())
	check("correction", correction.String())
	check("describes", describes.String())
	for _, r := range rules {
		check(r.ID+".Pattern", r.Pattern)
		check(r.ID+".Stale", r.Stale)
	}
}
