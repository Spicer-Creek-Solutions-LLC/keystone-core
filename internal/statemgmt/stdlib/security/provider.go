// SPDX-License-Identifier: Apache-2.0

package security

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (the v1.0
// security backends are Linux-only).
var ErrUnsupportedOS = errors.New("security: unsupported OS for v0.1 (Linux only)")

// ErrSELinuxUnavailable is returned when the SELinux user-space tools
// (`getenforce` / `setenforce` / `getsebool` / `setsebool`) or the
// `/etc/selinux/config` file are not present — typically because
// SELinux is not installed on this host.
var ErrSELinuxUnavailable = errors.New("security: SELinux tools or config not available")

func IsUnsupportedOS(err error) bool      { return errors.Is(err, ErrUnsupportedOS) }
func IsSELinuxUnavailable(err error) bool { return errors.Is(err, ErrSELinuxUnavailable) }

// Provider abstracts the SELinux operations the v1.0 security module
// performs. Production shells out to `getenforce` / `setenforce` /
// `getsebool` / `setsebool` and edits `/etc/selinux/config`; tests
// inject a fake.
//
// A v1.x extension that adds AppArmor will add a second interface
// (e.g. `AppArmorProvider`) and a backend selector; the Provider
// shape stays the same.
type Provider interface {
	// GetPersistentMode reads /etc/selinux/config and returns the
	// `SELINUX=` value: "enforcing" | "permissive" | "disabled".
	GetPersistentMode(ctx context.Context) (string, error)
	// GetRuntimeMode runs `getenforce` and normalises the output
	// ("Enforcing"/"Permissive"/"Disabled") to lower-case.
	GetRuntimeMode(ctx context.Context) (string, error)
	// SetPersistentMode edits /etc/selinux/config in place,
	// preserving file mode.
	SetPersistentMode(ctx context.Context, mode string) error
	// SetRuntimeMode runs `setenforce 1` (enforcing) or `setenforce
	// 0` (permissive). Disabling SELinux at runtime is not possible
	// — the kernel must be re-init'd — and is reported as an error
	// here; the module deals with this before calling Set.
	SetRuntimeMode(ctx context.Context, mode string) error
	// GetBoolean returns the current value of a named SELinux
	// boolean (`getsebool name`).
	GetBoolean(ctx context.Context, name string) (bool, error)
	// SetBoolean sets a named SELinux boolean persistently + at
	// runtime (`setsebool -P name=on|off`).
	SetBoolean(ctx context.Context, name string, value bool) error
}

// commandRunner runs `getenforce` / `setenforce` / `getsebool` /
// `setsebool`. It returns combined stdout+stderr and, on a non-zero
// exit, an error wrapping the exit code and trimmed output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
