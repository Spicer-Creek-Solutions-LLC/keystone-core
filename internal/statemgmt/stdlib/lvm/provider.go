// SPDX-License-Identifier: Apache-2.0

package lvm

import (
	"context"
	"errors"
)

// ErrUnsupportedOS is returned on non-Linux platforms (LVM is
// Linux-only).
var ErrUnsupportedOS = errors.New("lvm: unsupported OS for v0.1 (Linux only)")

// ErrNoLVM is returned when one of the LVM tools required by an
// operation (`pvcreate` / `pvremove` / `pvs` / `vgcreate` / `vgremove`
// / `vgs` / `lvcreate` / `lvremove` / `lvs`) is not on PATH.
var ErrNoLVM = errors.New("lvm: an LVM tool was not found on PATH")

func IsUnsupportedOS(err error) bool { return errors.Is(err, ErrUnsupportedOS) }
func IsNoLVM(err error) bool         { return errors.Is(err, ErrNoLVM) }

// Provider abstracts the LVM operations. Production shells out to
// the `lvm2`-provided tools; tests inject a fake.
//
// v1.0 deliberately does not pass `-f` / `--force` to any tool, so
// LVM's own safety refusals (overwrite-existing-PV, non-empty-VG-on-
// remove, mounted-LV-on-remove) reach the operator as errors. The
// `-y` / `--yes` flag is used on remove + create paths only to
// suppress interactive confirmation prompts that would otherwise
// hang the agent.
type Provider interface {
	HasPV(ctx context.Context, device string) (bool, error)
	CreatePV(ctx context.Context, device string) error
	RemovePV(ctx context.Context, device string) error

	HasVG(ctx context.Context, name string) (bool, error)
	CreateVG(ctx context.Context, name string, pvs []string) error
	RemoveVG(ctx context.Context, name string) error
	// GetVGPVs returns the device paths of the PVs currently in the VG
	// (`vgs --noheadings -o pv_name <vg>`), one per element.
	GetVGPVs(ctx context.Context, name string) ([]string, error)
	// ExtendVG adds PVs to an existing VG (`vgextend <vg> <pvs…>`). The
	// devices must already be PVs (created via a separate `pv:` decl).
	ExtendVG(ctx context.Context, name string, pvs []string) error
	// ReduceVG removes PVs from an existing VG (`vgreduce <vg> <pvs…>`).
	// No `-f`: LVM refuses to remove a PV that still holds LV extents, so
	// the operator must `pvmove` data off it first.
	ReduceVG(ctx context.Context, name string, pvs []string) error
	// Canonicalize resolves a device path to the canonical form LVM
	// reports as pv_name (symlinks followed), so a declared
	// /dev/disk/by-id/… path matches the live PV set. Best-effort: an
	// unresolvable path is returned unchanged.
	Canonicalize(ctx context.Context, device string) (string, error)

	HasLV(ctx context.Context, vg, lv string) (bool, error)
	// CreateLV is called with exactly one of size / extents
	// non-empty; the other is "". The Module enforces this.
	CreateLV(ctx context.Context, vg, lv, size, extents string) error
	RemoveLV(ctx context.Context, vg, lv string) error
	// GetLVSize returns the live size of an existing LV in bytes
	// (`lvs --units b --nosuffix -o lv_size`).
	GetLVSize(ctx context.Context, vg, lv string) (uint64, error)
	// ExtendLV grows an existing LV to size (`lvextend -L <size>`), and
	// when resizeFS is true also grows the contained filesystem via
	// `--resizefs` (fsadm). Grow-only: callers never invoke this to
	// shrink.
	ExtendLV(ctx context.Context, vg, lv, size string, resizeFS bool) error
}

// commandRunner runs an LVM tool. It returns combined stdout+stderr
// and, on a non-zero exit, an error wrapping the exit code and
// trimmed output.
type commandRunner func(ctx context.Context, bin string, args []string) (string, error)
