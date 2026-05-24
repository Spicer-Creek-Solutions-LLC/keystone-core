// SPDX-License-Identifier: Apache-2.0

//go:build linux

package mount

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// procMountsPath is the kernel mount table; a package var for tests.
var procMountsPath = "/proc/mounts"

func defaultProvider() Provider {
	mountBin, e1 := exec.LookPath("mount")
	umountBin, e2 := exec.LookPath("umount")
	if e1 != nil || e2 != nil {
		return &noMountProvider{}
	}
	return &linuxProvider{mount: mountBin, umount: umountBin, run: execRun}
}

type linuxProvider struct {
	mount  string
	umount string
	run    commandRunner
}

func (p *linuxProvider) Lookup(_ context.Context, mountPoint string) (*MountInfo, error) {
	return lookupProcMounts(mountPoint)
}

func (p *linuxProvider) Mount(ctx context.Context, device, mountPoint, fstype, opts string) error {
	args := []string{}
	if fstype != "" {
		args = append(args, "-t", fstype)
	}
	if strings.TrimSpace(opts) != "" {
		args = append(args, "-o", opts)
	}
	args = append(args, device, mountPoint)
	_, err := p.run(ctx, p.mount, args)
	return err
}

func (p *linuxProvider) Unmount(ctx context.Context, mountPoint string) error {
	_, err := p.run(ctx, p.umount, []string{mountPoint})
	return err
}

// lookupProcMounts scans procMountsPath for a line whose (unescaped)
// mount-point field equals mountPoint.
func lookupProcMounts(mountPoint string) (*MountInfo, error) {
	data, err := os.ReadFile(procMountsPath) //nolint:gosec // procMountsPath is a fixed kernel path (overridable only in tests)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", procMountsPath, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		if unescapeMount(f[1]) == mountPoint {
			return &MountInfo{
				MountPoint: mountPoint,
				Device:     unescapeMount(f[0]),
				FSType:     f[2],
				Mounted:    true,
			}, nil
		}
	}
	return &MountInfo{MountPoint: mountPoint, Mounted: false}, nil
}

// unescapeMount decodes the octal escapes (\040 space, \011 tab,
// \012 newline, \134 backslash, …) that /proc/mounts uses for
// whitespace and backslashes in device / mount-point fields.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// execRun is the production commandRunner. Captures combined output
// so mount/umount complaints reach the operator.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed flags + operator-supplied device / mount point / fstype / opts from a validated state declaration
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

// noMountProvider stands in when the mount/umount binaries are
// absent. Lookup still works (it only reads /proc/mounts); the
// mutating ops fail with ErrNoMountTools.
type noMountProvider struct{}

func (*noMountProvider) Lookup(_ context.Context, mountPoint string) (*MountInfo, error) {
	return lookupProcMounts(mountPoint)
}
func (*noMountProvider) Mount(context.Context, string, string, string, string) error {
	return ErrNoMountTools
}
func (*noMountProvider) Unmount(context.Context, string) error { return ErrNoMountTools }
