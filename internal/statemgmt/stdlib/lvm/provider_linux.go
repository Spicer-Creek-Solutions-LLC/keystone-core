// SPDX-License-Identifier: Apache-2.0

//go:build linux

package lvm

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// lvmTools is the set of binaries the Provider may invoke. They're
// all installed by the `lvm2` package; defaultProvider resolves each
// independently and the per-op `bin()` helper returns ErrNoLVM with
// the missing-binary name when one is unavailable.
var lvmTools = []string{
	"pvcreate", "pvremove", "pvs",
	"vgcreate", "vgremove", "vgextend", "vgreduce", "vgs",
	"lvcreate", "lvremove", "lvextend", "lvs",
}

func defaultProvider() Provider {
	p := &linuxProvider{bins: map[string]string{}, run: execRun}
	for _, name := range lvmTools {
		p.bins[name], _ = exec.LookPath(name)
	}
	return p
}

type linuxProvider struct {
	bins map[string]string // tool name → resolved path ("" if absent)
	run  commandRunner
}

func (p *linuxProvider) bin(name string) (string, error) {
	if b := p.bins[name]; b != "" {
		return b, nil
	}
	return "", fmt.Errorf("%w (%s missing)", ErrNoLVM, name)
}

// --- PV ----------------------------------------------------------------

func (p *linuxProvider) HasPV(ctx context.Context, device string) (bool, error) {
	bin, err := p.bin("pvs")
	if err != nil {
		return false, err
	}
	out, runErr := p.run(ctx, bin, []string{"--noheadings", "-o", "pv_name", device})
	if runErr != nil {
		// `pvs <missing-device>` exits non-zero; treat as "absent". A
		// real I/O issue surfaces from Create/Remove at apply time.
		return false, nil
	}
	return strings.TrimSpace(out) != "", nil
}

func (p *linuxProvider) CreatePV(ctx context.Context, device string) error {
	bin, err := p.bin("pvcreate")
	if err != nil {
		return err
	}
	_, err = p.run(ctx, bin, []string{device})
	return err
}

func (p *linuxProvider) RemovePV(ctx context.Context, device string) error {
	bin, err := p.bin("pvremove")
	if err != nil {
		return err
	}
	_, err = p.run(ctx, bin, []string{device})
	return err
}

// --- VG ----------------------------------------------------------------

func (p *linuxProvider) HasVG(ctx context.Context, name string) (bool, error) {
	bin, err := p.bin("vgs")
	if err != nil {
		return false, err
	}
	out, runErr := p.run(ctx, bin, []string{"--noheadings", "-o", "vg_name", name})
	if runErr != nil {
		return false, nil
	}
	return strings.TrimSpace(out) != "", nil
}

func (p *linuxProvider) CreateVG(ctx context.Context, name string, pvs []string) error {
	bin, err := p.bin("vgcreate")
	if err != nil {
		return err
	}
	args := append([]string{name}, pvs...)
	_, err = p.run(ctx, bin, args)
	return err
}

func (p *linuxProvider) RemoveVG(ctx context.Context, name string) error {
	bin, err := p.bin("vgremove")
	if err != nil {
		return err
	}
	_, err = p.run(ctx, bin, []string{"-y", name})
	return err
}

func (p *linuxProvider) GetVGPVs(ctx context.Context, name string) ([]string, error) {
	bin, err := p.bin("vgs")
	if err != nil {
		return nil, err
	}
	// vgs expands one row per PV when a pv-level field is requested.
	out, err := p.run(ctx, bin, []string{"--noheadings", "-o", "pv_name", name})
	if err != nil {
		return nil, err
	}
	var pvs []string
	for _, line := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			pvs = append(pvs, s)
		}
	}
	return pvs, nil
}

func (p *linuxProvider) ExtendVG(ctx context.Context, name string, pvs []string) error {
	bin, err := p.bin("vgextend")
	if err != nil {
		return err
	}
	args := append([]string{name}, pvs...)
	_, err = p.run(ctx, bin, args)
	return err
}

func (p *linuxProvider) ReduceVG(ctx context.Context, name string, pvs []string) error {
	bin, err := p.bin("vgreduce")
	if err != nil {
		return err
	}
	// No -f: vgreduce refuses to remove a PV that still holds allocated
	// extents, so data on a still-in-use PV is never silently orphaned.
	args := append([]string{name}, pvs...)
	_, err = p.run(ctx, bin, args)
	return err
}

// Canonicalize resolves a device path to LVM's pv_name form by
// following symlinks (e.g. /dev/disk/by-id/… → /dev/sdb). Best-effort:
// an unresolvable path (not present / EvalSymlinks error) is returned
// unchanged so the diff can still proceed.
func (p *linuxProvider) Canonicalize(_ context.Context, device string) (string, error) {
	resolved, err := filepath.EvalSymlinks(device)
	if err != nil {
		return device, nil
	}
	return resolved, nil
}

// --- LV ----------------------------------------------------------------

func (p *linuxProvider) HasLV(ctx context.Context, vg, lv string) (bool, error) {
	bin, err := p.bin("lvs")
	if err != nil {
		return false, err
	}
	out, runErr := p.run(ctx, bin, []string{"--noheadings", "-o", "lv_name", vg + "/" + lv})
	if runErr != nil {
		return false, nil
	}
	return strings.TrimSpace(out) != "", nil
}

func (p *linuxProvider) CreateLV(ctx context.Context, vg, lv, size, extents string) error {
	bin, err := p.bin("lvcreate")
	if err != nil {
		return err
	}
	args := []string{"-y", "-n", lv}
	switch {
	case size != "":
		args = append(args, "-L", size)
	case extents != "":
		args = append(args, "-l", extents)
	default:
		return fmt.Errorf("internal: CreateLV called with neither size nor extents")
	}
	args = append(args, vg)
	_, err = p.run(ctx, bin, args)
	return err
}

func (p *linuxProvider) RemoveLV(ctx context.Context, vg, lv string) error {
	bin, err := p.bin("lvremove")
	if err != nil {
		return err
	}
	_, err = p.run(ctx, bin, []string{"-y", vg + "/" + lv})
	return err
}

func (p *linuxProvider) GetLVSize(ctx context.Context, vg, lv string) (uint64, error) {
	bin, err := p.bin("lvs")
	if err != nil {
		return 0, err
	}
	out, err := p.run(ctx, bin, []string{"--noheadings", "--nosuffix", "--units", "b", "-o", "lv_size", vg + "/" + lv})
	if err != nil {
		return 0, err
	}
	return parseLVSizeBytes(out)
}

// parseLVSizeBytes reads the byte count from `lvs --units b --nosuffix`
// output (a single whitespace-padded integer, e.g. "  10737418240").
func parseLVSizeBytes(out string) (uint64, error) {
	s := strings.TrimSpace(out)
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("unexpected lvs size output %q: %w", s, err)
	}
	return n, nil
}

func (p *linuxProvider) ExtendLV(ctx context.Context, vg, lv, size string, resizeFS bool) error {
	bin, err := p.bin("lvextend")
	if err != nil {
		return err
	}
	args := []string{"-L", size}
	if resizeFS {
		args = append(args, "--resizefs")
	}
	args = append(args, vg+"/"+lv)
	_, err = p.run(ctx, bin, args)
	return err
}

// --- exec --------------------------------------------------------------

// execRun is the production commandRunner. Captures combined output
// so the LVM tool's complaint reaches the operator.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed LVM flags + validated device paths / names / sizes / extent specs from a validated declaration
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "", fmt.Errorf("%s %s: exit %d: %s", bin, strings.Join(args, " "), exitErr.ExitCode(), strings.TrimSpace(string(out)))
	}
	return "", fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
}
