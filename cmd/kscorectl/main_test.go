// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewCommand(t *testing.T) {
	cmd := newCommand()
	if cmd.Use != "kscorectl" {
		t.Errorf("Use = %q, want %q", cmd.Use, "kscorectl")
	}
	if cmd.Short == "" {
		t.Error("Short is empty")
	}
}

// Run is a no-op for the v1.0 hello-world; verify it returns nil immediately
// without blocking on ctx.
func TestRun_ReturnsImmediately(t *testing.T) {
	if err := run(context.Background(), nil, nil); err != nil {
		t.Fatalf("run returned error: %v", err)
	}
}

func TestRunCLI(t *testing.T) {
	// --help → cobra prints usage, exit 0.
	var out, errb bytes.Buffer
	if code := runCLI([]string{"--help"}, nil, &out, &errb); code != 0 {
		t.Fatalf("--help exit = %d, want 0", code)
	}
	if !strings.Contains(out.String()+errb.String(), "kscorectl") {
		t.Fatalf("--help output missing usage: %q %q", out.String(), errb.String())
	}

	// Unknown subcommand with no plugin → cobra error, exit 1, and the
	// error is surfaced to stderr rather than silently swallowed.
	t.Setenv("PATH", t.TempDir()) // no kscore-* here
	var ferr bytes.Buffer
	if code := runCLI([]string{"ghost"}, nil, &bytes.Buffer{}, &ferr); code != 1 {
		t.Fatalf("unknown subcommand exit = %d, want 1", code)
	}
	if !strings.Contains(ferr.String(), "error:") {
		t.Fatalf("unknown subcommand stderr = %q, want it to contain %q", ferr.String(), "error:")
	}

	// Git-style plugin dispatch: kscore-demo on PATH → exit code
	// forwarded.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kscore-demo"),
		[]byte("#!/bin/sh\necho \"demo:$1\"\nexit 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	var pout bytes.Buffer
	code := runCLI([]string{"demo", "arg1"}, nil, &pout, &errb)
	if code != 5 {
		t.Fatalf("plugin exit = %d, want 5 (forwarded)", code)
	}
	if strings.TrimSpace(pout.String()) != "demo:arg1" {
		t.Fatalf("plugin stdout = %q", pout.String())
	}
}
