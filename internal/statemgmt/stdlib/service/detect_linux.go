//go:build linux

package service

import (
	"context"
	"os"
	"os/exec"
)

// systemdRunDir is the canonical "systemd is the active init"
// marker. Overridable for tests.
var systemdRunDir = "/run/systemd/system"

// defaultProvider auto-detects the init system. Preference order:
//
//  1. /run/systemd/system/ exists → systemd is PID 1 → systemdProvider.
//  2. systemctl is on PATH (chroot / container without /run/systemd/
//     system but the binary still works) → systemdProvider.
//  3. Otherwise → undetectedProvider (returns ErrNoBackend on
//     mutating ops; Lookup reports the unit as not-existing so
//     state=stopped decls don't false-drift).
//
// OpenRC / sysvinit branches land in 11f2 + post-v1.0.
func defaultProvider() Provider {
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
