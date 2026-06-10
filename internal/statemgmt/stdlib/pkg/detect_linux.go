// SPDX-License-Identifier: Apache-2.0

//go:build linux

package pkg

import (
	"context"
	"os/exec"
)

// defaultProvider auto-detects the host's package manager by
// probing for each backend's primary binary in order of preference.
// Supports apt (Debian/Ubuntu), dnf (RHEL 8+/Rocky/Fedora), zypper
// (openSUSE/SLES), apk (Alpine), and pacman (Arch).
//
// Detection is binary-presence based rather than reading
// /etc/os-release: distros that mix package managers (e.g., RHEL
// with EPEL providing apt) are rare but real, and what matters at
// runtime is which binary is present. apt is probed first so that on
// such a mixed host the native+apt combination resolves to apt rather
// than splitting across two managers.
func defaultProvider() Provider {
	aptGet, aptErr := exec.LookPath("apt-get")
	dpkgQuery, dpkgErr := exec.LookPath("dpkg-query")
	if aptErr == nil && dpkgErr == nil {
		return newAptProvider(aptGet, dpkgQuery)
	}
	dnf, dnfErr := exec.LookPath("dnf")
	rpm, rpmErr := exec.LookPath("rpm")
	if dnfErr == nil && rpmErr == nil {
		return newDnfProvider(dnf, rpm)
	}
	// zypper is the other rpm-based manager (openSUSE / SLES). Probed
	// after dnf — the two are mutually exclusive in practice, but a host
	// with both resolves to dnf.
	if zypper, zErr := exec.LookPath("zypper"); zErr == nil && rpmErr == nil {
		return newZypperProvider(zypper, rpm)
	}
	if apk, err := exec.LookPath("apk"); err == nil {
		return newApkProvider(apk)
	}
	if pacman, err := exec.LookPath("pacman"); err == nil {
		return newPacmanProvider(pacman)
	}
	return &undetectedProvider{}
}

// undetectedProvider is the v1.0 fallback when no supported package
// manager is present. Lookup returns "not installed" so state=absent
// declarations don't drift on hosts where the backend isn't there
// (vacuously true: an unmanaged host can't have packages from a
// missing manager). Mutating operations return ErrNoBackend with a
// pointer at v1.x.
type undetectedProvider struct{}

func (*undetectedProvider) Lookup(name string) (*PkgInfo, error) {
	return &PkgInfo{Name: name, Installed: false}, nil
}

func (*undetectedProvider) Install(_ context.Context, _, _ string) error {
	return ErrNoBackend
}

func (*undetectedProvider) Remove(_ context.Context, _ string) error {
	return ErrNoBackend
}
