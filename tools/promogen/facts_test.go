// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// fakeRepo builds the minimum tree DeriveFacts reads.
func fakeRepo(t *testing.T, modules, binaries int, runSH string) string {
	t.Helper()
	root := fakeRepoWithChangelog(t, modules, binaries, runSH, defaultTestChangelog)
	return root
}

func writeVanity(t *testing.T, root, body string) {
	t.Helper()
	dir := filepath.Join(root, "deploy", "vanity")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir vanity: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vangen.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write vangen.json: %v", err)
	}
}

const defaultTestChangelog = `# Changelog

## [Unreleased]

## [v1.0.0] — Planned

## [v0.5.0] — 2026-06-27

### Added

- something
`

func fakeRepoWithChangelog(t *testing.T, modules, binaries int, runSH, changelog string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "CHANGELOG.md"), []byte(changelog), 0o600); err != nil {
		t.Fatalf("write changelog: %v", err)
	}
	writeVanity(t, root, `{"repositories":[{"url":"https://codeberg.org/Example-Org/keystone-core"}]}`)

	for i := range modules {
		dir := filepath.Join(root, "internal/statemgmt/stdlib", string(rune('a'+i)))
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "doc.go"), []byte("package x\n"), 0o600); err != nil {
			t.Fatalf("write doc.go: %v", err)
		}
	}
	for i := range binaries {
		if err := os.MkdirAll(filepath.Join(root, "cmd", "bin"+string(rune('a'+i))), 0o750); err != nil {
			t.Fatalf("mkdir cmd: %v", err)
		}
	}
	stateDir := filepath.Join(root, "test/e2e/state")
	if err := os.MkdirAll(stateDir, 0o750); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "run.sh"), []byte(runSH), 0o600); err != nil {
		t.Fatalf("write run.sh: %v", err)
	}
	return root
}

const threeDistros = `#!/usr/bin/env bash
# preamble that mentions debian and should not be counted
DISTROS=(
  "debian-12|debian:12|systemd|apt|cron|cron|exec /lib/systemd/systemd"
  "rocky-9|rockylinux:9|systemd|dnf|cronie|crond|exec /usr/lib/systemd/systemd"
  "alpine-3-19|alpine:3.19|openrc|apk|dcron|dcron|exec sleep infinity"
)
# a trailing "quoted|string" outside the array must not be counted
echo "debian-12|nope"
`

func TestDeriveFacts(t *testing.T) {
	root := fakeRepo(t, 4, 3, threeDistros)
	f, err := DeriveFacts(root)
	if err != nil {
		t.Fatalf("DeriveFacts: %v", err)
	}
	if f.ModuleCount != 4 {
		t.Errorf("ModuleCount = %d, want 4", f.ModuleCount)
	}
	if f.BinaryCount != 3 {
		t.Errorf("BinaryCount = %d, want 3", f.BinaryCount)
	}
	if f.DistroCount != 3 {
		t.Errorf("DistroCount = %d, want 3", f.DistroCount)
	}
	if f.RepoURL != "codeberg.org/Example-Org/keystone-core" {
		t.Errorf("RepoURL = %q, want the scheme-stripped vanity url", f.RepoURL)
	}
	if f.Version != "v0.5.0" {
		t.Errorf("Version = %q, want v0.5.0 from the changelog", f.Version)
	}
	if !f.PreRelease {
		t.Error("PreRelease = false, want true on the v0.x line")
	}
}

// The end card's status string is the one piece of copy where being
// wrong actively misleads, so pin the mapping.
func TestReleaseLabel(t *testing.T) {
	tests := []struct {
		version   string
		wantPre   bool
		wantLabel string
	}{
		{"v0.5.0", true, "v0.5.0 — pre-release"},
		{"v0.1.0", true, "v0.1.0 — pre-release"},
		{"v1.0.0", false, "v1.0.0"},
		{"v2.3.1", false, "v2.3.1"},
		{"dev", true, "dev — pre-release"},
		{"garbage", true, "garbage — pre-release"},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			pre := true
			if m := semverTag.FindStringSubmatch(tt.version); m != nil && m[1] != "0" {
				pre = false
			}
			if pre != tt.wantPre {
				t.Errorf("PreRelease(%q) = %v, want %v", tt.version, pre, tt.wantPre)
			}
			label := tt.version + " — pre-release"
			if !pre {
				label = tt.version
			}
			if label != tt.wantLabel {
				t.Errorf("ReleaseLabel(%q) = %q, want %q", tt.version, label, tt.wantLabel)
			}
		})
	}
}

// A silently-zero distro count would put "0-distro matrix" on a card,
// so a harness whose format changed must be an error, not a default.
func TestCountMatrixDistros_FormatChange(t *testing.T) {
	root := fakeRepo(t, 1, 1, "#!/usr/bin/env bash\necho no array here\n")
	if _, err := DeriveFacts(root); err == nil {
		t.Fatal("DeriveFacts() = nil error, want an error when DISTROS is unparseable")
	}
}

func TestDeriveFacts_MissingTree(t *testing.T) {
	if _, err := DeriveFacts(t.TempDir()); err == nil {
		t.Error("DeriveFacts(empty) = nil error, want error")
	}
}

// The real repo must always yield sane facts; this catches a harness
// or layout change that would otherwise only surface at render time.
func TestRepoFacts(t *testing.T) {
	f, err := DeriveFacts(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("DeriveFacts(repo): %v", err)
	}
	if f.ModuleCount <= 0 || f.BinaryCount <= 0 || f.DistroCount <= 0 {
		t.Errorf("facts = %+v, want all counts > 0", f)
	}
}

// The version is baked into a committed, diff-checked tape, so it must
// be a function of tree content alone. Deriving it from git tags made
// the generated output depend on clone depth: CI checks out with
// fetch-depth 1 and no tags, regenerated the card as "dev", and failed
// the drift gate on every PR while passing locally.
func TestLatestReleasedVersion(t *testing.T) {
	tests := []struct {
		name      string
		changelog string
		want      string
	}{
		{
			name:      "skips Unreleased and Planned",
			changelog: defaultTestChangelog,
			want:      "v0.5.0",
		},
		{
			name:      "picks the newest of several releases",
			changelog: "# Changelog\n\n## [Unreleased]\n\n## [v0.5.0] — 2026-06-27\n\n## [v0.1.0] — 2026-05-28\n",
			want:      "v0.5.0",
		},
		{
			name:      "nothing released yet",
			changelog: "# Changelog\n\n## [Unreleased]\n\n## [v1.0.0] — Planned\n",
			want:      "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := fakeRepoWithChangelog(t, 1, 1, threeDistros, tt.changelog)
			got, err := latestReleasedVersion(root)
			if err != nil {
				t.Fatalf("latestReleasedVersion: %v", err)
			}
			if got != tt.want {
				t.Errorf("latestReleasedVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A tree with no shipped release must still render, as "dev".
func TestDeriveFacts_NoReleaseYet(t *testing.T) {
	root := fakeRepoWithChangelog(t, 1, 1, threeDistros,
		"# Changelog\n\n## [Unreleased]\n")
	f, err := DeriveFacts(root)
	if err != nil {
		t.Fatalf("DeriveFacts: %v", err)
	}
	if f.Version != "dev" || f.ReleaseLabel != "dev — pre-release" {
		t.Errorf("facts = %+v, want dev/pre-release", f)
	}
}

// Facts must not vary with git state. This is the regression guard for
// the shallow-checkout failure.
func TestDeriveFacts_IndependentOfGit(t *testing.T) {
	root := fakeRepo(t, 2, 2, threeDistros)
	// A temp dir is not a git repository at all; if DeriveFacts still
	// consulted git this would degrade to "dev".
	f, err := DeriveFacts(root)
	if err != nil {
		t.Fatalf("DeriveFacts: %v", err)
	}
	if f.Version != "v0.5.0" {
		t.Errorf("Version = %q outside a git checkout, want v0.5.0 — "+
			"facts must derive from tree content, not git", f.Version)
	}
}

// The end card's URL was hand-typed once and named a repository that
// does not exist. It is derived now, so pin the derivation.
func TestCanonicalRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{
			name: "strips https",
			body: `{"repositories":[{"url":"https://codeberg.org/Org/repo"}]}`,
			want: "codeberg.org/Org/repo",
		},
		{
			name: "strips http and a trailing slash",
			body: `{"repositories":[{"url":"http://example.com/org/repo/"}]}`,
			want: "example.com/org/repo",
		},
		{
			name:    "no repositories",
			body:    `{"repositories":[]}`,
			wantErr: true,
		},
		{
			name:    "empty url",
			body:    `{"repositories":[{"url":""}]}`,
			wantErr: true,
		},
		{
			name:    "malformed json",
			body:    `{"repositories":`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeVanity(t, root, tt.body)
			got, err := canonicalRepoURL(root)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("canonicalRepoURL() = %q, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("canonicalRepoURL: %v", err)
			}
			if got != tt.want {
				t.Errorf("canonicalRepoURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanonicalRepoURL_Missing(t *testing.T) {
	if _, err := canonicalRepoURL(t.TempDir()); err == nil {
		t.Error("canonicalRepoURL(missing) = nil error, want error")
	}
}
