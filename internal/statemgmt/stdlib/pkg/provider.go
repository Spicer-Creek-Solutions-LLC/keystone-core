// SPDX-License-Identifier: Apache-2.0

package pkg

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned by detect on platforms where v1.0
// doesn't ship any package-manager backend (i.e., not Linux).
var ErrUnsupportedOS = errors.New("pkg: unsupported OS for v0.1 (Linux only)")

// ErrNoBackend is returned by Apply paths when no supported
// package manager binary was detected on the host. Supported: apt
// (Debian/Ubuntu), dnf (RHEL/Rocky/Fedora), and apk (Alpine);
// zypper / pacman are tracked for post-v1.0 follow-ups.
var ErrNoBackend = errors.New("pkg: no supported package manager detected on this host (none of apt-get, dnf, apk found; zypper / pacman are v1.x)")

// PkgInfo is the on-system shape the module compares against the
// declaration. Version is empty when Installed is false; a present
// Version with Installed=false would be a parser bug.
type PkgInfo struct {
	Name      string
	Installed bool
	Version   string
}

// Provider abstracts the OS-level package operations. Production
// code uses the platform-specific real impl returned by
// defaultProvider(); tests inject a fake.
type Provider interface {
	Lookup(name string) (*PkgInfo, error)
	Install(ctx context.Context, name, version string) error // version "" → latest available
	Remove(ctx context.Context, name string) error
}

// commandRunner is the injection point that lets per-backend tests
// pin arg formation without invoking apt-get / dnf / etc. for real.
// Each platform backend exposes a runner field; production
// defaultProvider sets it to execRun; tests inject a capturing
// shim that asserts the args.
type commandRunner func(ctx context.Context, bin string, args []string, env []string) error

// IsUnsupportedOS reports whether err is the package sentinel for
// non-Linux platforms.
func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }

// IsNoBackend reports whether err is the package sentinel for "no
// supported package manager detected." Exposed so the gRPC server
// can render a friendlier message on the operator-facing surface.
func IsNoBackend(err error) bool { return errors.Is(err, ErrNoBackend) }
