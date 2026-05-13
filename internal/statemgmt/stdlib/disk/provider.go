package disk

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (the v1.0
// disk module is Linux-only).
var ErrUnsupportedOS = errors.New("disk: unsupported OS for v1.0 (Linux only)")

// ErrNoBlkid is returned when `blkid` is not on PATH.
var ErrNoBlkid = errors.New("disk: the blkid binary was not found on PATH")

// ErrNoWipefs is returned when `wipefs` is not on PATH.
var ErrNoWipefs = errors.New("disk: the wipefs binary was not found on PATH")

// ErrNoMkfs is returned when the per-fstype mkfs binary is not on
// PATH. The Error() of a wrapped error mentions the fstype.
var ErrNoMkfs = errors.New("disk: an mkfs binary was not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoBlkid(err error) bool       { return errors.Is(err, ErrNoBlkid) }
func IsNoWipefs(err error) bool      { return errors.Is(err, ErrNoWipefs) }
func IsNoMkfs(err error) bool        { return errors.Is(err, ErrNoMkfs) }

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
}

// commandRunner runs `blkid` / `mkfs.X` / `mkswap` / `wipefs`. It
// returns combined stdout+stderr and, on a non-zero exit, an error
// wrapping the exit code and trimmed output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
