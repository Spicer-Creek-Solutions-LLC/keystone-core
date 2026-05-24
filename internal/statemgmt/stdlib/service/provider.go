// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (v1.0 ships
// systemd only).
var ErrUnsupportedOS = errors.New("service: unsupported OS for v0.1 (Linux only)")

// ErrNoBackend is returned on a Linux host where no supported init
// system was detected. v1.0 ships systemd; OpenRC / sysvinit /
// Upstart are tracked in V1X (11f2 + post-v1.0).
var ErrNoBackend = errors.New("service: no supported init system detected on this host (systemd not found; OpenRC / sysvinit are v1.x)")

// ErrUnitNotFound is returned by the module when the declared
// service unit doesn't exist on the host. The operator should
// install the package first (typically via a `require: [package:
// <name>]` relationship in the state file).
var ErrUnitNotFound = errors.New("service: unit not found on this host (install the package first)")

// ServiceInfo is the on-system shape the module compares against the
// declaration. Exists==false → the unit file isn't present;
// Active/Enabled are meaningless in that case.
type ServiceInfo struct {
	Name    string
	Exists  bool
	Active  bool // currently running
	Enabled bool // starts at boot
}

// Provider abstracts the OS-level service operations.
type Provider interface {
	Lookup(name string) (*ServiceInfo, error)
	Start(ctx context.Context, name string) error
	Stop(ctx context.Context, name string) error
	Enable(ctx context.Context, name string) error
	Disable(ctx context.Context, name string) error
}

// commandRunner is the injection point that lets the systemd
// backend's tests pin arg formation without invoking systemctl.
type commandRunner func(ctx context.Context, bin string, args []string) error

// IsUnsupportedOS / IsNoBackend / IsUnitNotFound expose the
// sentinel matchers so the gRPC server + CLI can render friendlier
// messages on the operator-facing surface.
func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoBackend(err error) bool     { return errors.Is(err, ErrNoBackend) }
func IsUnitNotFound(err error) bool  { return errors.Is(err, ErrUnitNotFound) }
