// SPDX-License-Identifier: Apache-2.0

//go:build linux

package pkg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Shared test helpers (capturingRunner, capturedCall, sliceEq,
// containsEnv) live in apt_test.go — same package.

// ---- parseApkList ------------------------------------------------

func TestParseApkList_Installed(t *testing.T) {
	t.Parallel()
	out := "nginx-1.20.1-r0 x86_64 {nginx} (BSD-2-Clause) [installed]\n"
	info := parseApkList("nginx", out)
	if !info.Installed {
		t.Errorf("Installed = false, want true")
	}
	// apk version is "<pkgver>-r<pkgrel>"; the whole thing after the
	// "nginx-" prefix is the version.
	if info.Version != "1.20.1-r0" {
		t.Errorf("Version = %q, want 1.20.1-r0", info.Version)
	}
}

func TestParseApkList_NotInstalledEmpty(t *testing.T) {
	t.Parallel()
	// apk list returns exit 0 with empty output when nothing matches.
	info := parseApkList("nginx", "")
	if info.Installed {
		t.Error("empty output should mean not installed")
	}
	if info.Version != "" {
		t.Errorf("Version should be empty when not installed; got %q", info.Version)
	}
}

func TestParseApkList_HyphenatedName(t *testing.T) {
	t.Parallel()
	// A package name containing a hyphen (py3-yaml). Stripping the
	// known "py3-yaml-" prefix yields the correct version; naive
	// last-dash splitting would mangle it.
	out := "py3-yaml-6.0-r0 x86_64 {py3-yaml} (MIT) [installed]\n"
	info := parseApkList("py3-yaml", out)
	if !info.Installed || info.Version != "6.0-r0" {
		t.Errorf("got %+v, want installed py3-yaml 6.0-r0", info)
	}
}

func TestParseApkList_GlobOvermatchSkipped(t *testing.T) {
	t.Parallel()
	// `apk list` can return extra packages matched by the name glob;
	// only the token whose remainder after "<name>-" is digit-led (a
	// real apk pkgver) counts. The query is "nginx" but a
	// "nginx-module-foo" line (strips to "module-foo-…", not digit-led)
	// precedes the real one and must be skipped.
	out := "nginx-module-foo-1.0-r0 x86_64 {nginx-module-foo} (BSD) [installed]\n" +
		"nginx-1.20.1-r0 x86_64 {nginx} (BSD-2-Clause) [installed]\n"
	info := parseApkList("nginx", out)
	if !info.Installed || info.Version != "1.20.1-r0" {
		t.Errorf("got %+v, want the exact nginx line (1.20.1-r0)", info)
	}
}

func TestParseApkList_GlobOvermatchOnly(t *testing.T) {
	t.Parallel()
	// Only a glob over-match is present (no real "nginx" line) →
	// reported as not installed, not a false positive on the longer
	// package's version.
	out := "nginx-module-foo-1.0-r0 x86_64 {nginx-module-foo} (BSD) [installed]\n"
	info := parseApkList("nginx", out)
	if info.Installed {
		t.Errorf("glob over-match alone should be not installed; got %+v", info)
	}
}

func TestParseApkList_NonInstalledMarkerSkipped(t *testing.T) {
	t.Parallel()
	// A line without the [installed] marker (e.g. an available-but-not
	// -installed upgrade row) must not count as installed.
	out := "nginx-1.21.0-r0 x86_64 {nginx} (BSD-2-Clause) [upgradable from: nginx-1.20.1-r0]\n"
	info := parseApkList("nginx", out)
	if info.Installed {
		t.Error("line without [installed] should not count as installed")
	}
}

// ---- apkProvider arg formation -----------------------------------

func newApkForTest(r commandRunner, lookup apkListFn) *apkProvider {
	return &apkProvider{
		apk:        "/sbin/apk",
		runner:     r,
		listLookup: lookup,
	}
}

func TestApkProvider_Install_NoVersion(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newApkForTest(cr.run, nil)
	if err := p.Install(context.Background(), "nginx", ""); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(cr.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(cr.calls))
	}
	got := cr.calls[0]
	wantArgs := []string{"add", "nginx"}
	if !sliceEq(got.Args, wantArgs) {
		t.Errorf("args = %v, want %v", got.Args, wantArgs)
	}
	if got.Bin != "/sbin/apk" {
		t.Errorf("Bin = %q, want /sbin/apk", got.Bin)
	}
}

func TestApkProvider_Install_WithVersion(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newApkForTest(cr.run, nil)
	if err := p.Install(context.Background(), "nginx", "1.20.1-r0"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got := cr.calls[0]
	// apk pins with "name=version", like apt.
	if got.Args[len(got.Args)-1] != "nginx=1.20.1-r0" {
		t.Errorf("pinned-version arg wrong; got %v, want trailing nginx=1.20.1-r0", got.Args)
	}
}

func TestApkProvider_Remove(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newApkForTest(cr.run, nil)
	if err := p.Remove(context.Background(), "nginx"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	wantArgs := []string{"del", "nginx"}
	if !sliceEq(cr.calls[0].Args, wantArgs) {
		t.Errorf("args = %v, want %v", cr.calls[0].Args, wantArgs)
	}
}

func TestApkProvider_RunnerErrorPropagates(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{err: errors.New("apk: unable to select packages")}
	p := newApkForTest(cr.run, nil)
	err := p.Install(context.Background(), "nginx", "")
	if err == nil || !strings.Contains(err.Error(), "unable to select") {
		t.Errorf("err = %v, want runner's underlying error", err)
	}
}

// ---- apkProvider Lookup via injected listLookup ------------------

func TestApkProvider_Lookup_DispatchesToParser(t *testing.T) {
	t.Parallel()
	fake := func(_ context.Context, _, name string) (string, error) {
		return "nginx-1.20.1-r0 x86_64 {nginx} (BSD-2-Clause) [installed]\n", nil
	}
	p := newApkForTest(nil, fake)
	info, err := p.Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !info.Installed || info.Version != "1.20.1-r0" {
		t.Errorf("Lookup: %+v", info)
	}
}

func TestApkProvider_Lookup_NotInstalled(t *testing.T) {
	t.Parallel()
	// Empty stdout (apk's no-match) → not installed, no error.
	fake := func(_ context.Context, _, name string) (string, error) {
		return "", nil
	}
	p := newApkForTest(nil, fake)
	info, err := p.Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if info.Installed {
		t.Error("expected not installed")
	}
}

func TestApkProvider_Lookup_ErrorSurfaces(t *testing.T) {
	t.Parallel()
	// Unlike apt/dnf there is no exit-1-as-absent convention; any
	// error from apk list is a genuine failure and must surface.
	fake := func(_ context.Context, _, name string) (string, error) {
		return "", errors.New("apk: database is locked")
	}
	p := newApkForTest(nil, fake)
	if _, err := p.Lookup("nginx"); err == nil {
		t.Error("expected apk list error to surface")
	}
}

// ---- realApkList (binary side) -----------------------------------

// writeStub writes an executable shell stub and returns its path.
func writeStub(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "apk")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func TestRealApkList_Success(t *testing.T) {
	t.Parallel()
	stub := writeStub(t, "echo 'nginx-1.20.1-r0 x86_64 {nginx} (BSD) [installed]'")
	out, err := realApkList(context.Background(), stub, "nginx")
	if err != nil {
		t.Fatalf("realApkList: %v", err)
	}
	if !strings.Contains(out, "nginx-1.20.1-r0") {
		t.Errorf("out = %q, want the stub's line", out)
	}
}

func TestRealApkList_ExitError(t *testing.T) {
	t.Parallel()
	// Exercises the ExitError branch (apk returns non-zero).
	stub := writeStub(t, "echo 'boom' >&2\nexit 2")
	_, err := realApkList(context.Background(), stub, "nginx")
	if err == nil || !strings.Contains(err.Error(), "exit 2") {
		t.Errorf("err = %v, want exit-2 error carrying stderr", err)
	}
}

func TestRealApkList_BinaryNotFound(t *testing.T) {
	t.Parallel()
	// Exercises the non-ExitError branch.
	_, err := realApkList(context.Background(), "/no/such/apk", "nginx")
	if err == nil {
		t.Fatal("expected lookup error")
	}
}
