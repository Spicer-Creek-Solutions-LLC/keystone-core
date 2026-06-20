// SPDX-License-Identifier: Apache-2.0

//go:build linux

package pkg

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// mkExec creates an executable stub named bin inside dir so
// exec.LookPath finds it. The contents are irrelevant — detection is
// presence-based and never runs the binary.
func mkExec(t *testing.T, dir, bin string) {
	t.Helper()
	p := filepath.Join(dir, bin)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

// TestDefaultProvider_DetectsByPath drives defaultProvider through
// each backend branch by controlling which package-manager binaries
// are visible on PATH. This covers the detection logic and, with it,
// every backend constructor (newApt/newDnf/newZypper/newApk/newPacman)
// plus the undetected fallback — all of which only fire on a host
// whose package manager differs from the one running the test.
func TestDefaultProvider_DetectsByPath(t *testing.T) {
	tests := []struct {
		name    string
		bins    []string
		wantFn  func(Provider) bool
		wantStr string
	}{
		{"apt", []string{"apt-get", "dpkg-query"}, func(p Provider) bool { _, ok := p.(*aptProvider); return ok }, "*aptProvider"},
		{"dnf", []string{"dnf", "rpm"}, func(p Provider) bool { _, ok := p.(*dnfProvider); return ok }, "*dnfProvider"},
		{"zypper", []string{"zypper", "rpm"}, func(p Provider) bool { _, ok := p.(*zypperProvider); return ok }, "*zypperProvider"},
		{"apk", []string{"apk"}, func(p Provider) bool { _, ok := p.(*apkProvider); return ok }, "*apkProvider"},
		{"pacman", []string{"pacman"}, func(p Provider) bool { _, ok := p.(*pacmanProvider); return ok }, "*pacmanProvider"},
		{"none", nil, func(p Provider) bool { _, ok := p.(*undetectedProvider); return ok }, "*undetectedProvider"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, b := range tt.bins {
				mkExec(t, dir, b)
			}
			// Scope PATH to the stub dir only — deterministic regardless
			// of what the host running the test actually has installed.
			t.Setenv("PATH", dir)
			got := defaultProvider()
			if !tt.wantFn(got) {
				t.Errorf("defaultProvider() = %T, want %s", got, tt.wantStr)
			}
		})
	}
}

// TestDefaultProvider_DnfWinsOverZypper guards the documented order:
// a host carrying both dnf and zypper (plus rpm) resolves to dnf.
func TestDefaultProvider_DnfWinsOverZypper(t *testing.T) {
	dir := t.TempDir()
	for _, b := range []string{"dnf", "zypper", "rpm"} {
		mkExec(t, dir, b)
	}
	t.Setenv("PATH", dir)
	if _, ok := defaultProvider().(*dnfProvider); !ok {
		t.Errorf("defaultProvider() = %T, want *dnfProvider (dnf precedes zypper)", defaultProvider())
	}
}

// TestDefaultProvider_AptWinsOverEverything guards apt-first precedence:
// even alongside dnf/rpm, an apt host resolves to apt.
func TestDefaultProvider_AptWinsOverEverything(t *testing.T) {
	dir := t.TempDir()
	for _, b := range []string{"apt-get", "dpkg-query", "dnf", "rpm"} {
		mkExec(t, dir, b)
	}
	t.Setenv("PATH", dir)
	if _, ok := defaultProvider().(*aptProvider); !ok {
		t.Errorf("defaultProvider() = %T, want *aptProvider (apt probed first)", defaultProvider())
	}
}

// TestRealRpmLookup_BinaryNotFound exercises the non-ExitError error
// branch of the dnf backend's real query path (mirrors the dpkg case).
func TestRealRpmLookup_BinaryNotFound(t *testing.T) {
	t.Parallel()
	if _, _, err := realRpmLookup(context.Background(), "/no/such/rpm", "nginx"); err == nil {
		t.Fatal("expected lookup error for a missing rpm binary")
	}
}

// TestRealPacmanQuery_BinaryNotFound exercises the non-ExitError error
// branch of the pacman backend's real query path.
func TestRealPacmanQuery_BinaryNotFound(t *testing.T) {
	t.Parallel()
	if _, _, err := realPacmanQuery(context.Background(), "/no/such/pacman", "nginx"); err == nil {
		t.Fatal("expected query error for a missing pacman binary")
	}
}
