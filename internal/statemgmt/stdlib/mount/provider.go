// SPDX-License-Identifier: Apache-2.0

package mount

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (v1.0 manages
// mounts via /proc/mounts + mount(8)/umount(8)).
var ErrUnsupportedOS = errors.New("mount: unsupported OS for v0.1 (Linux only)")

// ErrNoMountTools is returned by mount/unmount operations on a host
// where the `mount` or `umount` binary is not on PATH.
var ErrNoMountTools = errors.New("mount: the `mount`/`umount` binaries were not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoMountTools(err error) bool  { return errors.Is(err, ErrNoMountTools) }

// MountInfo is the live state of a mount point. Mounted==false → the
// other fields are meaningless.
type MountInfo struct {
	MountPoint string
	Device     string // the source as the kernel reports it (UUID/LABEL are resolved to a device)
	FSType     string
	Mounted    bool
}

// Provider abstracts the live-mount operations. Production reads
// /proc/mounts and shells out to mount(8)/umount(8); tests inject a
// fake.
type Provider interface {
	// Lookup reports whether something is mounted at mountPoint.
	Lookup(ctx context.Context, mountPoint string) (*MountInfo, error)
	// Mount mounts device at mountPoint with the given fstype + opts.
	Mount(ctx context.Context, device, mountPoint, fstype, opts string) error
	// Unmount unmounts whatever is at mountPoint.
	Unmount(ctx context.Context, mountPoint string) error
}

// commandRunner runs mount(8)/umount(8). It returns combined
// stdout+stderr and, on a non-zero exit, an error wrapping the exit
// code and trimmed output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
