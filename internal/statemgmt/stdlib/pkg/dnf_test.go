// SPDX-License-Identifier: Apache-2.0

//go:build linux

package pkg

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Shared test helpers (capturingRunner, capturedCall, sliceEq,
// containsEnv) live in apt_test.go — same package.

// ---- parseRpmQuery -----------------------------------------------

func TestParseRpmQuery_Installed(t *testing.T) {
	t.Parallel()
	// rpm -q --qf '%{VERSION}-%{RELEASE}\n' prints version-release on
	// exit 0 for an installed package.
	info, err := parseRpmQuery("nginx", "1.20.1-14.el9_2.1\n")
	if err != nil {
		t.Fatalf("parseRpmQuery: %v", err)
	}
	if !info.Installed {
		t.Errorf("Installed = false, want true")
	}
	if info.Version != "1.20.1-14.el9_2.1" {
		t.Errorf("Version = %q, want 1.20.1-14.el9_2.1", info.Version)
	}
}

func TestParseRpmQuery_NotInstalledMessage(t *testing.T) {
	t.Parallel()
	// rpm prints this to stdout (not stderr) with exit 1; cmd.Output()
	// captures it, so the parser must recognise it as not-installed.
	info, err := parseRpmQuery("nginx", "package nginx is not installed\n")
	if err != nil {
		t.Fatalf("parseRpmQuery: %v", err)
	}
	if info.Installed {
		t.Error("not-installed message should report Installed=false")
	}
	if info.Version != "" {
		t.Errorf("Version should be empty when not installed; got %q", info.Version)
	}
}

func TestParseRpmQuery_EmptyOutput(t *testing.T) {
	t.Parallel()
	// Defensive: empty stdout maps to not-installed.
	info, err := parseRpmQuery("nginx", "")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info.Installed {
		t.Error("empty stdout should mean not installed")
	}
}

func TestParseRpmQuery_TrailingNoise(t *testing.T) {
	t.Parallel()
	// Only the first line is the version; any trailing scriptlet noise
	// is ignored.
	info, err := parseRpmQuery("nginx", "1.20.1-14.el9\nwarning: stray output\n")
	if err != nil {
		t.Fatalf("parseRpmQuery: %v", err)
	}
	if !info.Installed || info.Version != "1.20.1-14.el9" {
		t.Errorf("got %+v, want installed nginx 1.20.1-14.el9", info)
	}
}

func TestParseRpmQuery_WhitespaceOnly(t *testing.T) {
	t.Parallel()
	// Whitespace-only output trims to empty → not installed (not a
	// spurious installed-with-empty-version).
	info, err := parseRpmQuery("nginx", "   \n")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if info.Installed {
		t.Error("whitespace-only output should mean not installed")
	}
}

// ---- dnfProvider arg formation -----------------------------------

func newDnfForTest(r commandRunner, lookup rpmLookupFn) *dnfProvider {
	return &dnfProvider{
		dnf:       "/usr/bin/dnf",
		rpm:       "/usr/bin/rpm",
		runner:    r,
		rpmLookup: lookup,
	}
}

func TestDnfProvider_Install_NoVersion(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newDnfForTest(cr.run, nil)
	if err := p.Install(context.Background(), "nginx", ""); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(cr.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(cr.calls))
	}
	got := cr.calls[0]
	wantArgs := []string{"install", "-y", "nginx"}
	if !sliceEq(got.Args, wantArgs) {
		t.Errorf("args = %v, want %v", got.Args, wantArgs)
	}
	if got.Bin != "/usr/bin/dnf" {
		t.Errorf("Bin = %q, want /usr/bin/dnf", got.Bin)
	}
}

func TestDnfProvider_Install_WithVersion(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newDnfForTest(cr.run, nil)
	if err := p.Install(context.Background(), "nginx", "1.20.1"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got := cr.calls[0]
	// dnf pins with "name-version", not apt's "name=version".
	if got.Args[len(got.Args)-1] != "nginx-1.20.1" {
		t.Errorf("pinned-version arg wrong; got %v, want trailing nginx-1.20.1", got.Args)
	}
}

func TestDnfProvider_Remove(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	p := newDnfForTest(cr.run, nil)
	if err := p.Remove(context.Background(), "nginx"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	wantArgs := []string{"remove", "-y", "nginx"}
	if !sliceEq(cr.calls[0].Args, wantArgs) {
		t.Errorf("args = %v, want %v", cr.calls[0].Args, wantArgs)
	}
}

func TestDnfProvider_RunnerErrorPropagates(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{err: errors.New("dnf: nothing provides nginx")}
	p := newDnfForTest(cr.run, nil)
	err := p.Install(context.Background(), "nginx", "")
	if err == nil || !strings.Contains(err.Error(), "nothing provides") {
		t.Errorf("err = %v, want runner's underlying error", err)
	}
}

// ---- dnfProvider Lookup via injected rpmLookup --------------------

func TestDnfProvider_Lookup_DispatchesToParser(t *testing.T) {
	t.Parallel()
	fake := func(_ context.Context, _, name string) (string, int, error) {
		return "1.20.1-14.el9\n", 0, nil
	}
	p := newDnfForTest(nil, fake)
	info, err := p.Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !info.Installed || info.Version != "1.20.1-14.el9" {
		t.Errorf("Lookup: %+v", info)
	}
}

func TestDnfProvider_Lookup_NotInstalledExit1(t *testing.T) {
	t.Parallel()
	// rpm exit 1 with the not-installed message is the canonical
	// "absent" signal — Lookup must not surface it as an error.
	fake := func(_ context.Context, _, name string) (string, int, error) {
		return "package nginx is not installed\n", 1, errors.New("exit status 1")
	}
	p := newDnfForTest(nil, fake)
	info, err := p.Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup should swallow exit-1 not-installed: %v", err)
	}
	if info.Installed {
		t.Error("expected not installed")
	}
}

func TestDnfProvider_Lookup_RealErrorSurfaces(t *testing.T) {
	t.Parallel()
	// A non-1 exit (e.g. rpm binary IO error) must surface, not be
	// swallowed as not-installed.
	fake := func(_ context.Context, _, name string) (string, int, error) {
		return "", 2, errors.New("rpmdb: BDB0113 lock failure")
	}
	p := newDnfForTest(nil, fake)
	if _, err := p.Lookup("nginx"); err == nil {
		t.Error("expected non-exit-1 rpm error to surface")
	}
}
