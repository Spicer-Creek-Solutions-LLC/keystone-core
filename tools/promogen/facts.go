// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Facts are the volatile numbers the promo cards quote. Every one is
// derived from the repository at render time rather than typed into a
// card, so a card cannot drift from what the project actually ships.
type Facts struct {
	// Version is the most recently released version, read from
	// CHANGELOG.md, or "dev" before the first release.
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
	// RepoURL is the canonical repository URL with the scheme stripped,
	// for the end card's call to action.
	RepoURL string
}

var semverTag = regexp.MustCompile(`^v(\d+)\.`)

// DeriveFacts collects the card facts from repoRoot.
func DeriveFacts(repoRoot string) (*Facts, error) {
	f := &Facts{Version: "dev"}

	v, err := latestReleasedVersion(repoRoot)
	if err != nil {
		return nil, err
	}
	if v != "" {
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

	if f.ModuleCount, err = countDocumentedModules(repoRoot); err != nil {
		return nil, err
	}
	if f.BinaryCount, err = countBinaries(repoRoot); err != nil {
		return nil, err
	}
	if f.DistroCount, err = countMatrixDistros(repoRoot); err != nil {
		return nil, err
	}
	if f.RepoURL, err = canonicalRepoURL(repoRoot); err != nil {
		return nil, err
	}
	return f, nil
}

// canonicalRepoURL reads the repository URL out of the Go vanity-import
// config and strips the scheme for display.
//
// Typed by hand, this went out as "codeberg.org/keystone-core/keystone-core",
// which is not the repository -- exactly the class of error the rest of
// this tool exists to prevent, so it is derived too. vangen.json is the
// in-tree source of truth for the canonical URL (deploy/vanity/), and
// unlike `git remote` it does not vary with how the tree was cloned.
func canonicalRepoURL(repoRoot string) (string, error) {
	path := filepath.Join(repoRoot, "deploy", "vanity", "vangen.json")
	// #nosec G304 G703 -- fixed path under the developer's own checkout.
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("promogen: read vanity config: %w", err)
	}
	var cfg struct {
		Repositories []struct {
			URL string `json:"url"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("promogen: parse vanity config: %w", err)
	}
	if len(cfg.Repositories) == 0 || cfg.Repositories[0].URL == "" {
		return "", fmt.Errorf("promogen: no repository url in %s", path)
	}
	u := cfg.Repositories[0].URL
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	return strings.TrimSuffix(u, "/"), nil
}

// releasedHeading matches a shipped CHANGELOG version heading:
// `## [v0.5.0] — 2026-06-27`. The date is required, which is what
// excludes `## [Unreleased]` and `## [v1.0.0] — Planned`.
var releasedHeading = regexp.MustCompile(`^## \[(v\d+\.\d+\.\d+)\][^\d]+(\d{4}-\d{2}-\d{2})\s*$`)

// latestReleasedVersion reads the newest shipped version out of
// CHANGELOG.md, or "" when nothing has shipped yet.
//
// Deliberately NOT `git describe`: this value is baked into a
// generated tape that is committed and diff-checked, so it has to be a
// function of the tree's content and nothing else. Deriving it from
// tags made the generated output depend on clone depth — CI checks out
// with fetch-depth 1 and no tags, so the card regenerated as "dev" and
// the drift gate failed on every PR while passing on every developer
// machine. CHANGELOG.md is in the tree at any depth.
func latestReleasedVersion(repoRoot string) (string, error) {
	path := filepath.Join(repoRoot, "CHANGELOG.md")
	// #nosec G304 G703 -- fixed path under the developer's own checkout.
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("promogen: read changelog: %w", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if m := releasedHeading.FindStringSubmatch(line); m != nil {
			return m[1], nil
		}
	}
	return "", nil
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
