// SPDX-License-Identifier: Apache-2.0

//go:build linux

package disk

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// mkfsBin maps a v1.0 catalog fstype to the binary that creates it.
// `swap` is the odd one — the binary is `mkswap`, not `mkfs.swap`.
var mkfsBin = map[string]string{
	"ext2":  "mkfs.ext2",
	"ext3":  "mkfs.ext3",
	"ext4":  "mkfs.ext4",
	"xfs":   "mkfs.xfs",
	"btrfs": "mkfs.btrfs",
	"f2fs":  "mkfs.f2fs",
	"vfat":  "mkfs.vfat",
	"exfat": "mkfs.exfat",
	"swap":  "mkswap",
}

func defaultProvider() Provider {
	p := &linuxProvider{
		mkfsPaths: map[string]string{},
		run:       execRun,
	}
	p.blkidBin, _ = exec.LookPath("blkid")
	p.wipefsBin, _ = exec.LookPath("wipefs")
	for fstype, name := range mkfsBin {
		p.mkfsPaths[fstype], _ = exec.LookPath(name)
	}
	return p
}

type linuxProvider struct {
	blkidBin  string
	wipefsBin string
	mkfsPaths map[string]string // fstype → resolved mkfs binary path ("" if absent)
	run       commandRunner
}

func (p *linuxProvider) GetFilesystem(ctx context.Context, device string) (string, error) {
	if p.blkidBin == "" {
		return "", ErrNoBlkid
	}
	out, runErr := p.run(ctx, p.blkidBin, []string{"-o", "value", "-s", "TYPE", device})
	if runErr != nil {
		// `blkid` exits 2 when the device has no recognised
		// signature — treat as "no filesystem". Any other failure
		// (missing device, EACCES) we report as an error.
		if isNoSignature(runErr) {
			return "", nil
		}
		return "", runErr
	}
	return strings.TrimSpace(out), nil
}

// isNoSignature reports whether a blkid failure means "device has no
// signature" rather than a real I/O failure. blkid exits 2 in that
// case; execRun's error string carries the exit code.
func isNoSignature(err error) bool {
	return strings.Contains(err.Error(), "exit 2:")
}

func (p *linuxProvider) MakeFilesystem(ctx context.Context, device, fstype string, mkfsOptions []string) error {
	bin := p.mkfsPaths[fstype]
	if bin == "" {
		return fmt.Errorf("%w (%s missing)", ErrNoMkfs, mkfsBin[fstype])
	}
	args := append([]string(nil), mkfsOptions...)
	args = append(args, device)
	_, err := p.run(ctx, bin, args)
	return err
}

func (p *linuxProvider) WipeFilesystem(ctx context.Context, device string) error {
	if p.wipefsBin == "" {
		return ErrNoWipefs
	}
	_, err := p.run(ctx, p.wipefsBin, []string{"-a", device})
	return err
}

// execRun is the production commandRunner. Captures combined output
// so the underlying tool's complaint reaches the operator.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath (or per-fstype mkfsBin map); args are the operator-supplied mkfs_options (charset-checked at validate time) + a validated /dev path
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
