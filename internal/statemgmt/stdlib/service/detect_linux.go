// SPDX-License-Identifier: Apache-2.0

//go:build linux

package service

import (
	"context"
	"os"
	"os/exec"
)

// defaultSystemdRunDir is the canonical "systemd is the active init"
// marker. Production callers pass this through defaultProvider;
// tests pass a path of their choosing. v0.x finding (B5-tier
// follow-up + the v0.x ROADMAP entry "`service` stdlib module —
// make systemdRunDir test-mutable without a package-level global"):
// the previous shape was a package-level `var`, which the race
// detector caught when parallel tests mutated it. Threading the
// run-dir through the parameter removes the global.
const defaultSystemdRunDir = "/run/systemd/system"

// defaultProvider auto-detects the init system. systemdRunDir is the
// path to test for "systemd is the active init"; pass
// defaultSystemdRunDir for production, or a test-controlled path
// from a unit test. Preference order:
//
//  1. systemdRunDir exists → systemd is PID 1 → systemdProvider.
//  2. systemctl is on PATH (chroot / container without
//     /run/systemd/system but the binary still works) →
//     systemdProvider.
//  3. Otherwise → undetectedProvider (returns ErrNoBackend on
//     mutating ops; Lookup reports the unit as not-existing so
//     state=stopped decls don't false-drift).
//
// OpenRC / sysvinit branches land in 11f2 + post-v1.0.
func defaultProvider(systemdRunDir string) Provider {
	if fi, err := os.Stat(systemdRunDir); err == nil && fi.IsDir() {
		if sc, err := exec.LookPath("systemctl"); err == nil {
			return newSystemdProvider(sc)
		}
	}
	if sc, err := exec.LookPath("systemctl"); err == nil {
		return newSystemdProvider(sc)
	}
	return &undetectedProvider{}
}

type undetectedProvider struct{}

func (*undetectedProvider) Lookup(name string) (*ServiceInfo, error) {
	// No init system → no managed services. Report not-exists so
	// state=stopped declarations vacuously match and state=running
	// declarations drift (and Apply returns ErrNoBackend, which is
	// the honest answer).
	return &ServiceInfo{Name: name, Exists: false}, nil
}
func (*undetectedProvider) Start(_ context.Context, _ string) error   { return ErrNoBackend }
func (*undetectedProvider) Stop(_ context.Context, _ string) error    { return ErrNoBackend }
func (*undetectedProvider) Enable(_ context.Context, _ string) error  { return ErrNoBackend }
func (*undetectedProvider) Disable(_ context.Context, _ string) error { return ErrNoBackend }
