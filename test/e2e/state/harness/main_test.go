// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRun_FileModuleIdempotent drives the full compile → apply →
// re-apply path against a hermetic file-module fixture in a tempdir
// (no package manager / init system needed), so the harness's own
// logic — including the idempotency assertion — is covered without
// Docker.
func TestRun_FileModuleIdempotent(t *testing.T) {
	root := t.TempDir()
	yaml := "" +
		"metadata:\n" +
		"  name: harness-unit\n" +
		"  version: \"0.1\"\n" +
		"file:\n" +
		"  " + root + "/d:\n" +
		"    state: directory\n" +
		"    mode: \"0755\"\n" +
		"  " + root + "/d/f.txt:\n" +
		"    state: present\n" +
		"    content: \"ok\\n\"\n" +
		"    mode: \"0644\"\n" +
		"    require:\n" +
		"      - file: " + root + "/d\n"

	path := filepath.Join(root, "smoke.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if err := run(path); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "d", "f.txt")); err != nil {
		t.Errorf("expected file applied on disk: %v", err)
	}
}

// TestDiskFixtureCompiles guards the YAML shape smoke.disk.sh generates:
// each resizable fstype gets a mkfs decl (blank device) and a resize
// decl. It compiles only (Parse → Render → Resolve) — no apply — so it
// touches no block device, yet asserts the disk decls parse and resolve.
func TestDiskFixtureCompiles(t *testing.T) {
	yaml := "" +
		"metadata:\n" +
		"  name: harness-disk-unit\n" +
		"  version: \"0.1\"\n" +
		"disk:\n"
	for _, fs := range []string{"ext4", "xfs", "btrfs", "f2fs"} {
		force := "-f"
		if fs == "ext4" {
			force = "-F"
		}
		yaml += "  " + fs + "-mkfs:\n" +
			"    state: present\n" +
			"    device: /dev/loop-" + fs + "-mkfs\n" +
			"    fstype: " + fs + "\n" +
			"    mkfs_options:\n" +
			"      - \"" + force + "\"\n"
		yaml += "  " + fs + "-resize:\n" +
			"    state: present\n" +
			"    device: /dev/loop-" + fs + "-resize\n" +
			"    fstype: " + fs + "\n" +
			"    resize_fs: true\n"
	}
	path := filepath.Join(t.TempDir(), "disk.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	decls, err := compile(path)
	if err != nil {
		t.Fatalf("compile disk fixture: %v", err)
	}
	if len(decls) != 8 {
		t.Fatalf("got %d decls, want 8", len(decls))
	}
	for _, d := range decls {
		if d.Module != "disk" {
			t.Errorf("decl %s module = %q, want disk", d.ID, d.Module)
		}
	}
}

func TestRun_MissingFile(t *testing.T) {
	if err := run(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("expected error for a missing state file")
	}
}

func TestRun_BadYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("\tnot: [valid"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := run(path); err == nil {
		t.Error("expected error for malformed yaml")
	}
}
