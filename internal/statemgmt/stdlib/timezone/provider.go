// SPDX-License-Identifier: Apache-2.0

package timezone

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms.
var ErrUnsupportedOS = errors.New("timezone: unsupported OS for v0.1 (Linux only)")

// ErrZoneNotFound is returned by Set when the declared zone has no
// /usr/share/zoneinfo/<zone> entry on the agent. (Can't be checked
// at compile time — the engine validator doesn't see the target
// host's zoneinfo tree.)
var ErrZoneNotFound = errors.New("timezone: zone not found in /usr/share/zoneinfo")

// Provider abstracts the OS-level timezone operations.
type Provider interface {
	// Current returns the timezone derived from
	// readlink /etc/localtime (stripping the
	// /usr/share/zoneinfo/ prefix). set=false when /etc/localtime
	// isn't a symlink into the zoneinfo tree.
	Current() (zone string, set bool, err error)
	// Set validates the zone exists, then applies it.
	Set(ctx context.Context, zone string) error
}

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsZoneNotFound(err error) bool  { return errors.Is(err, ErrZoneNotFound) }
