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
