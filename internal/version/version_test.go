package version

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The version must be derived from the repository, not typed. A test comparing
// String() against a literal would pass on a build that reports the wrong
// commit, which is the defect this exists to prevent.
func TestStringCarriesTheRepositoryCommit(t *testing.T) {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Skipf("no git available: %v", err)
	}
	want := strings.TrimSpace(string(out))
	if want == "" {
		t.Fatal("git reported an empty HEAD")
	}

	old := commit
	t.Cleanup(func() { commit = old })

	commit = want
	if got := String(); got != Dev+"+g"+want {
		t.Errorf("String() = %q, want %q", got, Dev+"+g"+want)
	}
	if got := Commit(); got != want {
		t.Errorf("Commit() = %q, want %q", got, want)
	}
}

// An unknown commit is reported as absence, not as a placeholder a caller would
// have to recognise by string comparison.
func TestUnknownCommitIsEmptyRatherThanASentinel(t *testing.T) {
	old := commit
	t.Cleanup(func() { commit = old })
	commit = ""

	// go test builds carry build info, so this asserts the shape rather than
	// forcing the empty branch: whatever is reported must never be a sentinel.
	for _, bad := range []string{"unknown", "none", "dev", "HEAD", "?"} {
		if Commit() == bad {
			t.Errorf("Commit() returned the sentinel %q; unknown must be empty", bad)
		}
	}
	if s := String(); !strings.HasPrefix(s, Dev) {
		t.Errorf("String() = %q, want a %s prefix", s, Dev)
	}
}

// The commit must not be written into the source. A literal initialiser would
// make String() report a build that does not exist, and every test that sets
// `commit` itself would still pass -- which is how the first version of this
// case missed it.
func TestCommitHasNoLiteralInitialiser(t *testing.T) {
	b, err := os.ReadFile("version.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || !strings.HasPrefix(trimmed, "var commit") {
			continue
		}
		if strings.Contains(trimmed, "=") {
			t.Errorf("commit carries a literal initialiser: %q", trimmed)
		}
		return
	}
	t.Fatal("no `var commit` declaration found; this test no longer checks anything")
}
