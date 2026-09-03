// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFragments(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".changes", "unreleased")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func TestLoadDemoTags(t *testing.T) {
	root := writeFragments(t, map[string]string{
		"tagged.yaml":    "kind: Added\nbody: |-\n  **Drift detection.** Reports per-resource drift.\ncustom:\n  Demo: state-drift\n",
		"untagged.yaml":  "kind: Fixed\nbody: |-\n  **A quiet fix.** Not demoable.\n",
		"empty-tag.yaml": "kind: Added\nbody: |-\n  **Blank.** Tag present but empty.\ncustom:\n  Demo: \"  \"\n",
		"notes.md":       "not a fragment",
	})

	tags, err := LoadDemoTags(root)
	if err != nil {
		t.Fatalf("LoadDemoTags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("LoadDemoTags() = %d tags, want 1: %+v", len(tags), tags)
	}
	if tags[0].Feature != "state-drift" {
		t.Errorf("Feature = %q, want state-drift", tags[0].Feature)
	}
	if tags[0].Title != "Drift detection." {
		t.Errorf("Title = %q, want %q", tags[0].Title, "Drift detection.")
	}
}

func TestLoadDemoTags_NoChangesDir(t *testing.T) {
	tags, err := LoadDemoTags(t.TempDir())
	if err != nil {
		t.Fatalf("LoadDemoTags() = %v, want nil error for an absent dir", err)
	}
	if len(tags) != 0 {
		t.Errorf("LoadDemoTags() = %v, want none", tags)
	}
}

func TestLoadDemoTags_MalformedFragment(t *testing.T) {
	root := writeFragments(t, map[string]string{"bad.yaml": "kind: [unterminated\n"})
	if _, err := LoadDemoTags(root); err == nil {
		t.Error("LoadDemoTags(malformed) = nil error, want error")
	}
}

func TestFragmentTitle(t *testing.T) {
	tests := []struct{ body, want string }{
		{"**Bold title.** Then prose.", "Bold title."},
		{"**Multi word title** and more\nsecond line", "Multi word title"},
		{"No bold marker here", "No bold marker here"},
		{"", ""},
		{"**unterminated bold", "**unterminated bold"},
	}
	for _, tt := range tests {
		if got := fragmentTitle(tt.body); got != tt.want {
			t.Errorf("fragmentTitle(%q) = %q, want %q", tt.body, got, tt.want)
		}
	}
}

func TestReconcile(t *testing.T) {
	m, err := LoadManifest(writeManifest(t, validManifest, "a.tape", "b.tape"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	tags := []DemoTag{
		{File: "one.yaml", Feature: "state-apply", Title: "Covered."},
		{File: "two.yaml", Feature: "brand-new-thing", Title: "Not shot."},
	}
	rep := Reconcile(m, tags)

	if len(rep.Covered) != 1 || rep.Covered[0].Feature != "state-apply" {
		t.Errorf("Covered = %+v, want the state-apply tag", rep.Covered)
	}
	if len(rep.Unshot) != 1 || rep.Unshot[0].Feature != "brand-new-thing" {
		t.Errorf("Unshot = %+v, want the brand-new-thing tag", rep.Unshot)
	}
	if len(rep.Orphaned) != 0 {
		t.Errorf("Orphaned = %v, want none", rep.Orphaned)
	}
}

// A shot whose feature matches nothing in the current cycle is normal
// — shots outlive the release that introduced them — so it must be
// reported separately from the actionable Unshot set.
func TestReconcile_OrphanedShotIsInformational(t *testing.T) {
	m, err := LoadManifest(writeManifest(t, validManifest, "a.tape", "b.tape"))
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	rep := Reconcile(m, nil)
	if len(rep.Unshot) != 0 {
		t.Errorf("Unshot = %+v, want none", rep.Unshot)
	}
	if len(rep.Orphaned) != 1 || rep.Orphaned[0] != "state-apply" {
		t.Errorf("Orphaned = %v, want [state-apply]", rep.Orphaned)
	}
}
