// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Facts are the volatile numbers the promo cards quote. Every one is
// derived from the repository at render time rather than typed into a
// card, so a card cannot drift from what the project actually ships.
type Facts struct {
	// Version is the most recent release tag (e.g. "v0.5.0"), or
	// "dev" outside a tagged checkout.
	Version string
	// PreRelease is true while the line is v0.x — the end card says so
	// out loud rather than implying GA.
	PreRelease bool
	// ReleaseLabel is the end-card status string.
	ReleaseLabel string
	// ModuleCount is the number of documented state stdlib modules.
	ModuleCount int
	// BinaryCount is the number of cmd/* binaries shipped.
	BinaryCount int
	// DistroCount is the size of the live cross-distro matrix.
	DistroCount int
}

var semverTag = regexp.MustCompile(`^v(\d+)\.`)

// DeriveFacts collects the card facts from repoRoot.
func DeriveFacts(repoRoot string) (*Facts, error) {
	f := &Facts{Version: "dev"}

	if v := gitDescribe(repoRoot); v != "" {
		f.Version = v
	}
	// Anything on the v0.x line is pre-release by the VERSIONING.md
	// ladder; "dev" is likewise not a release.
	f.PreRelease = true
	if m := semverTag.FindStringSubmatch(f.Version); m != nil && m[1] != "0" {
		f.PreRelease = false
	}
	f.ReleaseLabel = f.Version + " — pre-release"
	if !f.PreRelease {
		f.ReleaseLabel = f.Version
	}

	var err error
	if f.ModuleCount, err = countDocumentedModules(repoRoot); err != nil {
		return nil, err
	}
	if f.BinaryCount, err = countBinaries(repoRoot); err != nil {
		return nil, err
	}
	if f.DistroCount, err = countMatrixDistros(repoRoot); err != nil {
		return nil, err
	}
	return f, nil
}

// gitDescribe returns the nearest tag, or "" when git is unavailable
// or the checkout carries no tags (a shallow CI clone, say). Callers
// fall back to "dev" rather than failing the render.
func gitDescribe(repoRoot string) string {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// countDocumentedModules counts state stdlib modules carrying a
// doc.go, which is the repo's existing "this module is real and
// documented" marker (enforced by tools/gendocs/modules -strict).
func countDocumentedModules(repoRoot string) (int, error) {
	matches, err := filepath.Glob(filepath.Join(repoRoot, "internal/statemgmt/stdlib/*/doc.go"))
	if err != nil {
		return 0, fmt.Errorf("promogen: glob stdlib modules: %w", err)
	}
	return len(matches), nil
}

// countBinaries counts cmd/* directories.
func countBinaries(repoRoot string) (int, error) {
	entries, err := os.ReadDir(filepath.Join(repoRoot, "cmd"))
	if err != nil {
		return 0, fmt.Errorf("promogen: read cmd/: %w", err)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	return n, nil
}

// distroEntry matches one "name|image|init|pkgmgr|..." row of the
// DISTROS bash array in test/e2e/state/run.sh. Counting the array
// beats hard-coding the number in a card that nobody re-checks when a
// distro joins the matrix.
var distroEntry = regexp.MustCompile(`^\s*"[a-z0-9.-]+\|[^"]*"\s*$`)

func countMatrixDistros(repoRoot string) (int, error) {
	path := filepath.Join(repoRoot, "test/e2e/state/run.sh")
	// #nosec G304 G703 -- fixed path under the developer's own checkout.
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("promogen: read cross-distro harness: %w", err)
	}
	n, inArray := 0, false
	for _, line := range strings.Split(string(raw), "\n") {
		switch {
		case strings.HasPrefix(strings.TrimSpace(line), "DISTROS=("):
			inArray = true
		case inArray && strings.TrimSpace(line) == ")":
			inArray = false
		case inArray && distroEntry.MatchString(line):
			n++
		}
	}
	if n == 0 {
		return 0, fmt.Errorf("promogen: found no DISTROS entries in %s "+
			"(harness format changed?)", path)
	}
	return n, nil
}
