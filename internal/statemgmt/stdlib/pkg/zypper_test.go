// SPDX-License-Identifier: Apache-2.0

//go:build linux

package pkg

import (
	"context"
	"errors"
	"testing"
)

func newZypperForTest(r commandRunner, lookup rpmLookupFn) *zypperProvider {
	return &zypperProvider{
		zypper:    "/usr/bin/zypper",
		rpm:       "/usr/bin/rpm",
		runner:    r,
		rpmLookup: lookup,
	}
}

func TestZypperProvider_Install_NoVersion(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	if err := newZypperForTest(cr.run, nil).Install(context.Background(), "nginx", ""); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got := cr.calls[0]
	if !sliceEq(got.Args, []string{"--non-interactive", "install", "nginx"}) {
		t.Errorf("args = %v", got.Args)
	}
	if got.Bin != "/usr/bin/zypper" {
		t.Errorf("Bin = %q", got.Bin)
	}
}

func TestZypperProvider_Install_WithVersion(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	if err := newZypperForTest(cr.run, nil).Install(context.Background(), "nginx", "1.20.1-1"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	// zypper pins with "name=version" (its exact-edition operator).
	if last := cr.calls[0].Args[len(cr.calls[0].Args)-1]; last != "nginx=1.20.1-1" {
		t.Errorf("pinned arg = %q, want nginx=1.20.1-1", last)
	}
}

func TestZypperProvider_Remove(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	if err := newZypperForTest(cr.run, nil).Remove(context.Background(), "nginx"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !sliceEq(cr.calls[0].Args, []string{"--non-interactive", "remove", "nginx"}) {
		t.Errorf("args = %v", cr.calls[0].Args)
	}
}

func TestZypperProvider_Lookup_DispatchesToRpmParser(t *testing.T) {
	t.Parallel()
	// openSUSE is rpm-based, so Lookup reuses the rpm query + parser.
	fake := func(_ context.Context, _, _ string) (string, int, error) {
		return "1.20.1-150400.1.1\n", 0, nil
	}
	info, err := newZypperForTest(nil, fake).Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !info.Installed || info.Version != "1.20.1-150400.1.1" {
		t.Errorf("Lookup: %+v", info)
	}
}

func TestZypperProvider_Lookup_NotInstalledExit1(t *testing.T) {
	t.Parallel()
	fake := func(_ context.Context, _, _ string) (string, int, error) {
		return "package nginx is not installed\n", 1, errors.New("exit status 1")
	}
	info, err := newZypperForTest(nil, fake).Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup should swallow exit-1 not-installed: %v", err)
	}
	if info.Installed {
		t.Error("expected not installed")
	}
}

func TestZypperProvider_Lookup_RealErrorSurfaces(t *testing.T) {
	t.Parallel()
	fake := func(_ context.Context, _, _ string) (string, int, error) {
		return "", 2, errors.New("rpmdb lock failure")
	}
	if _, err := newZypperForTest(nil, fake).Lookup("nginx"); err == nil {
		t.Error("a non-1 exit must surface as an error")
	}
}
