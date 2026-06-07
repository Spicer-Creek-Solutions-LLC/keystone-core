// SPDX-License-Identifier: Apache-2.0

package disk

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (the v1.0
// disk module is Linux-only).
var ErrUnsupportedOS = errors.New("disk: unsupported OS for v0.1 (Linux only)")

// ErrNoBlkid is returned when `blkid` is not on PATH.
var ErrNoBlkid = errors.New("disk: the blkid binary was not found on PATH")

// ErrNoWipefs is returned when `wipefs` is not on PATH.
var ErrNoWipefs = errors.New("disk: the wipefs binary was not found on PATH")

// ErrNoMkfs is returned when the per-fstype mkfs binary is not on
// PATH. The Error() of a wrapped error mentions the fstype.
var ErrNoMkfs = errors.New("disk: an mkfs binary was not found on PATH")

// ErrNoResizeTool is returned when a filesystem-resize tool
// (`blockdev` / `dumpe2fs` / `resize2fs`) is not on PATH.
var ErrNoResizeTool = errors.New("disk: a filesystem-resize tool was not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoBlkid(err error) bool       { return errors.Is(err, ErrNoBlkid) }
func IsNoWipefs(err error) bool      { return errors.Is(err, ErrNoWipefs) }
func IsNoMkfs(err error) bool        { return errors.Is(err, ErrNoMkfs) }
func IsNoResizeTool(err error) bool  { return errors.Is(err, ErrNoResizeTool) }

// Provider abstracts the operations the disk module performs.
// Production shells out to `blkid` / `mkfs.X` / `wipefs`; tests
// inject a fake.
type Provider interface {
	// GetFilesystem returns the filesystem signature on `device`
	// (one of the names in the v1.0 catalog) or "" when the device
	// has no signature. A missing block device or other I/O failure
	// surfaces as an error.
	GetFilesystem(ctx context.Context, device string) (string, error)
	// MakeFilesystem runs `mkfs.<fstype>` (or `mkswap` for the swap
	// pseudo-fstype) with the operator-supplied options, then the
	// device path.
	MakeFilesystem(ctx context.Context, device, fstype string, mkfsOptions []string) error
	// WipeFilesystem runs `wipefs -a <device>`.
	WipeFilesystem(ctx context.Context, device string) error
	// FilesystemFillsDevice reports whether the filesystem on `device`
	// already occupies the full block device (so a grow is a no-op).
	// v0.5 supports ext2/3/4 only; other fstypes return ErrUnsupportedOS-
	// adjacent guidance from the module's validate(), so this is only
	// called for ext.
	FilesystemFillsDevice(ctx context.Context, device, fstype string) (bool, error)
	// ResizeFilesystem grows the filesystem on `device` to fill the
	// block device (`resize2fs <device>` for ext).
	ResizeFilesystem(ctx context.Context, device, fstype string) error
}

// commandRunner runs `blkid` / `mkfs.X` / `mkswap` / `wipefs`. It
// returns combined stdout+stderr and, on a non-zero exit, an error
// wrapping the exit code and trimmed output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
