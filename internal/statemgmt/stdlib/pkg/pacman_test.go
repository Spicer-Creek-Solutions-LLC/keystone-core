// SPDX-License-Identifier: Apache-2.0

//go:build linux

package pkg

import (
	"context"
	"errors"
	"testing"
)

func newPacmanForTest(r commandRunner, q pacmanQueryFn) *pacmanProvider {
	return &pacmanProvider{pacman: "/usr/bin/pacman", runner: r, query: q}
}

func TestPacmanProvider_Install_NoVersion(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	if err := newPacmanForTest(cr.run, nil).Install(context.Background(), "nginx", ""); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got := cr.calls[0]
	if !sliceEq(got.Args, []string{"-S", "--noconfirm", "nginx"}) {
		t.Errorf("args = %v", got.Args)
	}
	if got.Bin != "/usr/bin/pacman" {
		t.Errorf("Bin = %q", got.Bin)
	}
}

func TestPacmanProvider_Install_VersionPinErrors(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	err := newPacmanForTest(cr.run, nil).Install(context.Background(), "nginx", "1.20.1")
	if !errors.Is(err, ErrVersionPinUnsupported) {
		t.Fatalf("err = %v, want ErrVersionPinUnsupported", err)
	}
	if len(cr.calls) != 0 {
		t.Errorf("pacman must not run on a version-pinned install: %v", cr.calls)
	}
}

func TestPacmanProvider_Remove(t *testing.T) {
	t.Parallel()
	cr := &capturingRunner{}
	if err := newPacmanForTest(cr.run, nil).Remove(context.Background(), "nginx"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !sliceEq(cr.calls[0].Args, []string{"-R", "--noconfirm", "nginx"}) {
		t.Errorf("args = %v", cr.calls[0].Args)
	}
}

func TestParsePacmanQuery(t *testing.T) {
	t.Parallel()
	// installed
	if info := parsePacmanQuery("nginx", "nginx 1.20.1-1\n", 0); !info.Installed || info.Version != "1.20.1-1" {
		t.Errorf("installed: %+v", info)
	}
	// not installed (exit 1, empty stdout)
	if info := parsePacmanQuery("nginx", "", 1); info.Installed {
		t.Errorf("not installed: %+v", info)
	}
	// installed but version field missing → installed, no version
	if info := parsePacmanQuery("nginx", "nginx\n", 0); !info.Installed || info.Version != "" {
		t.Errorf("no-version line: %+v", info)
	}
}

func TestPacmanProvider_Lookup_DispatchesToParser(t *testing.T) {
	t.Parallel()
	fake := func(_ context.Context, _, _ string) (string, int, error) {
		return "nginx 1.20.1-1\n", 0, nil
	}
	info, err := newPacmanForTest(nil, fake).Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !info.Installed || info.Version != "1.20.1-1" {
		t.Errorf("Lookup: %+v", info)
	}
}

func TestPacmanProvider_Lookup_NotInstalledExit1(t *testing.T) {
	t.Parallel()
	fake := func(_ context.Context, _, _ string) (string, int, error) {
		return "", 1, errors.New("exit status 1")
	}
	info, err := newPacmanForTest(nil, fake).Lookup("nginx")
	if err != nil {
		t.Fatalf("Lookup should swallow exit-1 not-found: %v", err)
	}
	if info.Installed {
		t.Error("expected not installed")
	}
}

func TestPacmanProvider_Lookup_RealErrorSurfaces(t *testing.T) {
	t.Parallel()
	fake := func(_ context.Context, _, _ string) (string, int, error) {
		return "", 2, errors.New("pacman db locked")
	}
	if _, err := newPacmanForTest(nil, fake).Lookup("nginx"); err == nil {
		t.Error("a non-1 exit must surface as an error")
	}
}
