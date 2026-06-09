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

// defaultOpenrcRunDir is the canonical "OpenRC is the active init"
// marker (created when OpenRC boots). Threaded through defaultProvider
// like defaultSystemdRunDir so tests can point it at a path of their
// choosing without a package-level mutable global.
const defaultOpenrcRunDir = "/run/openrc"

// defaultProvider auto-detects the init system. systemdRunDir /
// openrcRunDir are the paths to test for "<init> is the active init";
// pass the default* consts for production, or test-controlled paths
// from a unit test. Preference order:
//
//  1. systemdRunDir exists → systemd is PID 1 → systemdProvider.
//  2. systemctl is on PATH (chroot / container without
//     /run/systemd/system but the binary still works) →
//     systemdProvider.
//  3. openrcRunDir exists + rc-service/rc-update on PATH → openrc.
//  4. rc-service/rc-update on PATH (container without /run/openrc) →
//     openrc.
//  5. `service` + (chkconfig | update-rc.d) on PATH → sysvinit.
//  6. Otherwise → undetectedProvider (returns ErrNoBackend on
//     mutating ops; Lookup reports the unit as not-existing so
//     state=stopped decls don't false-drift).
//
// systemd is preferred over OpenRC over sysvinit, so a host that somehow
// has several resolves to the most modern. The launchd (macOS) branch is
// post-v1.0.
func defaultProvider(systemdRunDir, openrcRunDir string) Provider {
	if fi, err := os.Stat(systemdRunDir); err == nil && fi.IsDir() {
		if sc, err := exec.LookPath("systemctl"); err == nil {
			return newSystemdProvider(sc)
		}
	}
	if sc, err := exec.LookPath("systemctl"); err == nil {
		return newSystemdProvider(sc)
	}
	if fi, err := os.Stat(openrcRunDir); err == nil && fi.IsDir() {
		if rs, ru, ok := lookOpenRC(); ok {
			return newOpenrcProvider(rs, ru)
		}
	}
	if rs, ru, ok := lookOpenRC(); ok {
		return newOpenrcProvider(rs, ru)
	}
	if p := lookSysvinit(); p != nil {
		return p
	}
	return &undetectedProvider{}
}

// lookSysvinit resolves a classic SysV-init host: the `service` runtime
// wrapper plus a boot-enable tool (chkconfig on RHEL/CentOS, update-rc.d
// on Debian/Devuan). chkconfig is preferred when both are somehow
// present. Returns nil when there is no usable sysvinit toolchain (the
// caller then falls through to undetectedProvider). Only reached on a
// non-systemd, non-OpenRC host — systemd boxes ship `service`/`chkconfig`
// shims too but resolve to systemd earlier.
func lookSysvinit() Provider {
	svc, err := exec.LookPath("service")
	if err != nil {
		return nil
	}
	if chk, err := exec.LookPath("chkconfig"); err == nil {
		return newSysvinitProvider(svc, chk, sysvChkconfig)
	}
	if urc, err := exec.LookPath("update-rc.d"); err == nil {
		return newSysvinitProvider(svc, urc, sysvUpdateRcd)
	}
	return nil
}

// lookOpenRC resolves the OpenRC control binaries; ok is true only
// when both are present (the backend needs rc-service for status/
// start/stop and rc-update for the runlevel/enable operations).
func lookOpenRC() (rcService, rcUpdate string, ok bool) {
	rs, err1 := exec.LookPath("rc-service")
	ru, err2 := exec.LookPath("rc-update")
	if err1 != nil || err2 != nil {
		return "", "", false
	}
	return rs, ru, true
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
