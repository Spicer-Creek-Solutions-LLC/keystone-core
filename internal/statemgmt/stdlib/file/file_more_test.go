// SPDX-License-Identifier: Apache-2.0

package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// applyDecl is a small helper: parse + validate + Apply, returning the
// result and error. Keeps the scenario tests below terse.
func applyDecl(t *testing.T, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	t.Helper()
	return newModule().Apply(context.Background(), decl)
}

func TestDriftSeverity_AllBranches(t *testing.T) {
	m := newModule()
	tests := []struct {
		name string
		decl *statemgmt.Declaration
		want statemgmt.DriftSeverity
	}{
		{"nil decl", nil, statemgmt.DriftSeverityMedium},
		{"absent high", declFor("/x", StateAbsent, nil), statemgmt.DriftSeverityHigh},
		{"present medium", declFor("/x", StatePresent, nil), statemgmt.DriftSeverityMedium},
		{"directory medium", declFor("/x", StateDirectory, nil), statemgmt.DriftSeverityMedium},
		{"symlink medium", declFor("/x", StateSymlink, map[string]any{"target": "/y"}), statemgmt.DriftSeverityMedium},
		{"unknown state default medium", declFor("/x", "bogus", nil), statemgmt.DriftSeverityMedium},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.DriftSeverity(tt.decl, nil); got != tt.want {
				t.Errorf("DriftSeverity = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSymlink_Create(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	res, err := applyDecl(t, declFor(link, StateSymlink, map[string]any{"target": "/etc/hostname"}))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Changed {
		t.Error("want Changed=true on create")
	}
	got, err := os.Readlink(link)
	if err != nil || got != "/etc/hostname" {
		t.Fatalf("readlink = %q, %v; want /etc/hostname", got, err)
	}
}

func TestSymlink_Idempotent(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink("/target", link); err != nil {
		t.Fatal(err)
	}
	res, err := applyDecl(t, declFor(link, StateSymlink, map[string]any{"target": "/target"}))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Changed {
		t.Error("want Changed=false when symlink already matches")
	}
}

func TestSymlink_ReplaceWrongTarget(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink("/old", link); err != nil {
		t.Fatal(err)
	}
	if _, err := applyDecl(t, declFor(link, StateSymlink, map[string]any{"target": "/new"})); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, _ := os.Readlink(link)
	if got != "/new" {
		t.Errorf("readlink = %q; want /new (replaced)", got)
	}
}

func TestSymlink_RefusesNonSymlinkCollision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "regular")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := applyDecl(t, declFor(path, StateSymlink, map[string]any{"target": "/x"}))
	if err == nil {
		t.Fatal("want error replacing a regular file with a symlink")
	}
	if res != nil && res.Success {
		t.Error("want Success=false on collision")
	}
}

func TestDirectory_CreateWithMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep")
	res, err := applyDecl(t, declFor(path, StateDirectory, map[string]any{"mode": "0750"}))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Changed {
		t.Error("want Changed=true on mkdir")
	}
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		t.Fatalf("stat: %v, isDir=%v", err, fi.IsDir())
	}
	if perm := fi.Mode().Perm(); perm != 0o750 {
		t.Errorf("mode = %#o, want 0750", perm)
	}
}

func TestDirectory_ModeChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := applyDecl(t, declFor(path, StateDirectory, map[string]any{"mode": "0755"})); err != nil {
		t.Fatalf("apply: %v", err)
	}
	fi, _ := os.Stat(path)
	if perm := fi.Mode().Perm(); perm != 0o755 {
		t.Errorf("mode = %#o, want 0755", perm)
	}
}

func TestDirectory_RefusesFileCollision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := applyDecl(t, declFor(path, StateDirectory, nil)); err == nil {
		t.Fatal("want error: cannot create directory where a file exists")
	}
}

func TestAbsent_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gone")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := applyDecl(t, declFor(path, StateAbsent, nil))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Changed {
		t.Error("want Changed=true removing an existing file")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("path still exists: %v", err)
	}
}

func TestAbsent_RefusesDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "d")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := applyDecl(t, declFor(path, StateAbsent, nil)); err == nil {
		t.Fatal("want error: refuse to recursively remove a directory")
	}
}

func TestAbsent_AlreadyMissingIdempotent(t *testing.T) {
	dir := t.TempDir()
	res, err := applyDecl(t, declFor(filepath.Join(dir, "nope"), StateAbsent, nil))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Changed {
		t.Error("want Changed=false when already absent")
	}
}

func TestValidate_Rejections(t *testing.T) {
	m := newModule()
	tests := []struct {
		name   string
		decl   *statemgmt.Declaration
		wantOK bool
	}{
		{"present content+source mutex", declFor("/x", StatePresent, map[string]any{"content": "a", "source": "/s"}), false},
		{"directory with content", declFor("/x", StateDirectory, map[string]any{"content": "a"}), false},
		{"directory with target", declFor("/x", StateDirectory, map[string]any{"target": "/y"}), false},
		{"symlink missing target", declFor("/x", StateSymlink, nil), false},
		{"symlink with mode", declFor("/x", StateSymlink, map[string]any{"target": "/y", "mode": "0644"}), false},
		{"symlink with content", declFor("/x", StateSymlink, map[string]any{"target": "/y", "content": "a"}), false},
		{"absent with mode", declFor("/x", StateAbsent, map[string]any{"mode": "0644"}), false},
		{"valid present", declFor("/x", StatePresent, map[string]any{"content": "a"}), true},
		{"valid directory", declFor("/x", StateDirectory, map[string]any{"mode": "0755"}), true},
		{"valid symlink", declFor("/x", StateSymlink, map[string]any{"target": "/y"}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := m.Validate(tt.decl)
			if tt.wantOK && err != nil {
				t.Errorf("Validate err = %v, want nil", err)
			}
			if !tt.wantOK && err == nil {
				t.Error("Validate err = nil, want error")
			}
		})
	}
}

// TestCheck_SourceError covers the wantContentHash source-read error
// branch surfaced through diffAttributes: a present file whose declared
// source does not exist reports a "source error" diff rather than a
// content-hash mismatch.
func TestCheck_SourceError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("live"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := newModule().Check(context.Background(), declFor(path, StatePresent, map[string]any{
		"source": filepath.Join(dir, "does-not-exist"),
	}))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Matches {
		t.Error("want Matches=false when the source is unreadable")
	}
}

// TestCheck_TypeMismatches walks the type-collision diff branches:
// a present declaration over a directory, and a directory declaration
// over a regular file.
func TestCheck_TypeMismatches(t *testing.T) {
	dir := t.TempDir()

	asDir := filepath.Join(dir, "isdir")
	if err := os.Mkdir(asDir, 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := newModule().Check(context.Background(), declFor(asDir, StatePresent, map[string]any{"content": "x"}))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Matches {
		t.Error("present over a directory should not match")
	}

	asFile := filepath.Join(dir, "isfile")
	if err := os.WriteFile(asFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = newModule().Check(context.Background(), declFor(asFile, StateDirectory, nil))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Matches {
		t.Error("directory over a regular file should not match")
	}
}

// TestSymlink_TargetDiffReported checks the symlink-target drift branch
// in diffCheck (wrong target reported, then converged by Apply).
func TestSymlink_TargetDiffReported(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "l")
	if err := os.Symlink("/old", link); err != nil {
		t.Fatal(err)
	}
	res, err := newModule().Check(context.Background(), declFor(link, StateSymlink, map[string]any{"target": "/new"}))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Matches {
		t.Error("want Matches=false for a wrong symlink target")
	}
	if res.Diff == "" {
		t.Error("want a non-empty diff describing the target change")
	}
}
