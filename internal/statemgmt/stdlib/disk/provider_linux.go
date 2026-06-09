// SPDX-License-Identifier: Apache-2.0

//go:build linux

package disk

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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
	p.blockdevBin, _ = exec.LookPath("blockdev")
	p.dumpe2fsBin, _ = exec.LookPath("dumpe2fs")
	p.resize2fsBin, _ = exec.LookPath("resize2fs")
	p.xfsInfoBin, _ = exec.LookPath("xfs_info")
	p.xfsGrowfsBin, _ = exec.LookPath("xfs_growfs")
	p.btrfsBin, _ = exec.LookPath("btrfs")
	p.findmntBin, _ = exec.LookPath("findmnt")
	p.resizeF2fsBin, _ = exec.LookPath("resize.f2fs")
	p.readAt = deviceReadAt
	for fstype, name := range mkfsBin {
		p.mkfsPaths[fstype], _ = exec.LookPath(name)
	}
	return p
}

// deviceReadAt reads len(buf) bytes from a block device at off. Used to
// read the f2fs superblock, which has no version-stable userspace size
// tool. Seam so tests inject a fake without a real device.
func deviceReadAt(device string, off int64, buf []byte) (int, error) {
	f, err := os.OpenFile(device, os.O_RDONLY, 0) //nolint:gosec // device is a validated /dev path (devicePathRE)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	return f.ReadAt(buf, off)
}

type linuxProvider struct {
	blkidBin      string
	wipefsBin     string
	blockdevBin   string
	dumpe2fsBin   string
	resize2fsBin  string
	xfsInfoBin    string
	xfsGrowfsBin  string
	btrfsBin      string
	findmntBin    string
	resizeF2fsBin string
	mkfsPaths     map[string]string // fstype → resolved mkfs binary path ("" if absent)
	run           commandRunner
	readAt        func(device string, off int64, buf []byte) (int, error)
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

// --- resize ------------------------------------------------------------

// FilesystemFillsDevice dispatches the per-fstype "is the fs already as
// large as the block device" check. ext is device-based; xfs and btrfs
// query their own size and compare to the block device. The query never
// requires a specific mount state (the resize itself does — see
// ResizeFilesystem).
func (p *linuxProvider) FilesystemFillsDevice(ctx context.Context, device, fstype string) (bool, error) {
	deviceBytes, err := p.deviceSize(ctx, device)
	if err != nil {
		return false, err
	}
	switch fstype {
	case "ext2", "ext3", "ext4":
		if p.dumpe2fsBin == "" {
			return false, fmt.Errorf("%w (dumpe2fs missing)", ErrNoResizeTool)
		}
		fsOut, err := p.run(ctx, p.dumpe2fsBin, []string{"-h", device})
		if err != nil {
			return false, fmt.Errorf("dumpe2fs -h %s: %w", device, err)
		}
		return extFillsDevice(fsOut, deviceBytes)
	case "xfs":
		return p.xfsFillsDevice(ctx, device, deviceBytes)
	case "btrfs":
		return p.btrfsFillsDevice(ctx, device, deviceBytes)
	case "f2fs":
		return p.f2fsFillsDevice(device, deviceBytes)
	}
	return false, fmt.Errorf("resize not supported for fstype %q", fstype)
}

// deviceSize returns the block device's size in bytes.
func (p *linuxProvider) deviceSize(ctx context.Context, device string) (uint64, error) {
	if p.blockdevBin == "" {
		return 0, fmt.Errorf("%w (blockdev missing)", ErrNoResizeTool)
	}
	out, err := p.run(ctx, p.blockdevBin, []string{"--getsize64", device})
	if err != nil {
		return 0, fmt.Errorf("blockdev --getsize64 %s: %w", device, err)
	}
	n, err := strconv.ParseUint(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse device size %q: %w", strings.TrimSpace(out), err)
	}
	return n, nil
}

// mountpointOf returns the device's mountpoint, or "" when it is not
// mounted. A findmnt non-zero exit (source not mounted) is "" not an
// error; only a missing binary is an error.
func (p *linuxProvider) mountpointOf(ctx context.Context, device string) (string, error) {
	if p.findmntBin == "" {
		return "", fmt.Errorf("%w (findmnt missing)", ErrNoResizeTool)
	}
	out, err := p.run(ctx, p.findmntBin, []string{"-n", "-o", "TARGET", "--source", device})
	if err != nil {
		return "", nil // not mounted
	}
	mnt := strings.TrimSpace(out)
	if i := strings.IndexByte(mnt, '\n'); i >= 0 {
		mnt = mnt[:i] // first mountpoint if bind-mounted in several places
	}
	return mnt, nil
}

// xfsFillsDevice reports whether the xfs already occupies the device.
// xfs_info needs the mountpoint when the fs is mounted and accepts the
// device when it is not.
func (p *linuxProvider) xfsFillsDevice(ctx context.Context, device string, deviceBytes uint64) (bool, error) {
	if p.xfsInfoBin == "" {
		return false, fmt.Errorf("%w (xfs_info missing)", ErrNoResizeTool)
	}
	target := device
	if mnt, err := p.mountpointOf(ctx, device); err != nil {
		return false, err
	} else if mnt != "" {
		target = mnt
	}
	out, err := p.run(ctx, p.xfsInfoBin, []string{target})
	if err != nil {
		return false, fmt.Errorf("xfs_info %s: %w", target, err)
	}
	fsBytes, bsize, err := parseXfsInfoBytes(out)
	if err != nil {
		return false, err
	}
	// Maximal when one more fs block would not fit on the device.
	return fsBytes+bsize > deviceBytes, nil
}

// btrfsFillsDevice reports whether the btrfs already spans the device.
// `btrfs filesystem show --raw` reports the device size btrfs is using;
// `resize max` grows it to the full block device, so equality is full.
func (p *linuxProvider) btrfsFillsDevice(ctx context.Context, device string, deviceBytes uint64) (bool, error) {
	if p.btrfsBin == "" {
		return false, fmt.Errorf("%w (btrfs missing)", ErrNoResizeTool)
	}
	out, err := p.run(ctx, p.btrfsBin, []string{"filesystem", "show", "--raw", device})
	if err != nil {
		return false, fmt.Errorf("btrfs filesystem show %s: %w", device, err)
	}
	fsBytes, err := parseBtrfsShowBytes(out, device)
	if err != nil {
		return false, err
	}
	return fsBytes >= deviceBytes, nil
}

// f2fs has no version-stable userspace tool that reports its size, so
// the fill check reads the on-disk superblock (a stable ABI) directly.
const (
	f2fsSuperOffset  = 1024 // F2FS_SUPER_OFFSET: superblock starts here
	f2fsSuperReadLen = 64   // enough to cover through block_count (offset 36 + 8)
	f2fsSuperMinLen  = 44
	f2fsMagic        = 0xF2F52010
)

// f2fsFillsDevice reports whether the f2fs already occupies every whole
// section that fits on the device. resize.f2fs grows by whole sections
// (block_count rounds down to a section boundary), so the check is
// section-aware — comparing block-for-block would never converge.
func (p *linuxProvider) f2fsFillsDevice(device string, deviceBytes uint64) (bool, error) {
	if p.readAt == nil {
		return false, fmt.Errorf("%w (no device reader)", ErrNoResizeTool)
	}
	buf := make([]byte, f2fsSuperReadLen)
	n, err := p.readAt(device, f2fsSuperOffset, buf)
	if err != nil && n < f2fsSuperMinLen {
		return false, fmt.Errorf("read f2fs superblock on %s: %w", device, err)
	}
	blockCount, blockSize, secBlocks, err := parseF2fsSuperblock(buf[:n])
	if err != nil {
		return false, err
	}
	deviceBlocks := deviceBytes / blockSize
	maxBlocks := (deviceBlocks / secBlocks) * secBlocks // largest whole-section size that fits
	return blockCount >= maxBlocks, nil
}

// parseF2fsSuperblock reads the block count and section geometry from a
// raw f2fs superblock (read at f2fsSuperOffset). All multi-byte fields
// are little-endian. Pure for testability.
//
// On-disk layout (struct f2fs_super_block), byte offsets within the
// superblock: magic @0 (u32), log_blocksize @16 (u32), log_blocks_per_seg
// @20 (u32), segs_per_sec @24 (u32), block_count @36 (u64).
func parseF2fsSuperblock(b []byte) (blockCount, blockSize, secBlocks uint64, err error) {
	if len(b) < f2fsSuperMinLen {
		return 0, 0, 0, fmt.Errorf("f2fs superblock too short: %d bytes", len(b))
	}
	if magic := binary.LittleEndian.Uint32(b[0:4]); magic != f2fsMagic {
		return 0, 0, 0, fmt.Errorf("f2fs superblock magic = %#x, want %#x", magic, uint32(f2fsMagic))
	}
	logBlocksize := binary.LittleEndian.Uint32(b[16:20])
	logBlocksPerSeg := binary.LittleEndian.Uint32(b[20:24])
	segsPerSec := binary.LittleEndian.Uint32(b[24:28])
	blockCount = binary.LittleEndian.Uint64(b[36:44])
	if logBlocksize > 30 || logBlocksPerSeg > 30 || segsPerSec == 0 {
		return 0, 0, 0, fmt.Errorf("f2fs superblock geometry implausible (log_blocksize=%d log_blocks_per_seg=%d segs_per_sec=%d)", logBlocksize, logBlocksPerSeg, segsPerSec)
	}
	blockSize = uint64(1) << logBlocksize
	secBlocks = (uint64(1) << logBlocksPerSeg) * uint64(segsPerSec)
	return blockCount, blockSize, secBlocks, nil
}

// parseXfsInfoBytes reads the data-section block size and block count
// from `xfs_info` output (the `data =  bsize=4096 blocks=N` line) and
// returns the fs size and block size in bytes. Pure for testability.
func parseXfsInfoBytes(out string) (fsBytes, bsize uint64, err error) {
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "data ") && !strings.HasPrefix(trimmed, "data=") {
			continue
		}
		if !strings.Contains(trimmed, "bsize=") || !strings.Contains(trimmed, "blocks=") {
			continue
		}
		bsize = parseTaggedUint(trimmed, "bsize=")
		blocks := parseTaggedUint(trimmed, "blocks=")
		if bsize == 0 || blocks == 0 {
			return 0, 0, fmt.Errorf("xfs_info: could not parse bsize/blocks from %q", trimmed)
		}
		return bsize * blocks, bsize, nil
	}
	return 0, 0, fmt.Errorf("xfs_info: data section line not found")
}

// parseBtrfsShowBytes reads the device size from `btrfs filesystem show
// --raw` output. With a single-device filesystem it uses the only devid
// line; with several it matches the requested device. Multi-device
// resize (per-devid) is out of scope.
func parseBtrfsShowBytes(out, device string) (uint64, error) {
	var sizes []uint64
	var matched uint64
	found := false
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		// devid  1  size  <bytes>  used  <bytes>  path  <dev>
		if len(f) >= 8 && f[0] == "devid" && f[2] == "size" && f[6] == "path" {
			n, err := strconv.ParseUint(f[3], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("btrfs show: bad size %q: %w", f[3], err)
			}
			sizes = append(sizes, n)
			if f[7] == device {
				matched, found = n, true
			}
		}
	}
	switch {
	case found:
		return matched, nil
	case len(sizes) == 1:
		return sizes[0], nil
	case len(sizes) == 0:
		return 0, fmt.Errorf("btrfs filesystem show: no devid line for %s", device)
	default:
		return 0, fmt.Errorf("btrfs filesystem show: %d devices, none matching %s (multi-device resize is V1X)", len(sizes), device)
	}
}

// parseTaggedUint extracts the run of digits immediately following
// `key` (e.g. "bsize=") in s. Returns 0 when absent.
func parseTaggedUint(s, key string) uint64 {
	i := strings.Index(s, key)
	if i < 0 {
		return 0
	}
	rest := s[i+len(key):]
	j := 0
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	n, _ := strconv.ParseUint(rest[:j], 10, 64)
	return n
}

// extFillsDevice parses `dumpe2fs -h` output for the block count and
// block size, then reports whether the ext filesystem already occupies
// the device — i.e. growing it by one more block would exceed the
// device. Pure for testability.
func extFillsDevice(dumpe2fsOut string, deviceBytes uint64) (bool, error) {
	blockCount, ok := parseDumpe2fsField(dumpe2fsOut, "Block count:")
	if !ok {
		return false, fmt.Errorf("dumpe2fs: Block count not found")
	}
	blockSize, ok := parseDumpe2fsField(dumpe2fsOut, "Block size:")
	if !ok || blockSize == 0 {
		return false, fmt.Errorf("dumpe2fs: Block size not found")
	}
	fsBytes := blockCount * blockSize
	// Maximal when one more block would not fit on the device.
	return fsBytes+blockSize > deviceBytes, nil
}

func parseDumpe2fsField(out, field string) (uint64, bool) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, field) {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(line, field))
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// ResizeFilesystem grows the filesystem to fill the block device,
// dispatching per fstype. ext resizes the device directly; xfs and
// btrfs grow a *mounted* filesystem by its mountpoint and error clearly
// when the device is not mounted.
func (p *linuxProvider) ResizeFilesystem(ctx context.Context, device, fstype string) error {
	switch fstype {
	case "ext2", "ext3", "ext4":
		if p.resize2fsBin == "" {
			return fmt.Errorf("%w (resize2fs missing)", ErrNoResizeTool)
		}
		_, err := p.run(ctx, p.resize2fsBin, []string{device})
		return err
	case "xfs":
		return p.growByMountpoint(ctx, device, "xfs", p.xfsGrowfsBin, "xfs_growfs",
			func(mnt string) []string { return []string{mnt} })
	case "btrfs":
		return p.growByMountpoint(ctx, device, "btrfs", p.btrfsBin, "btrfs",
			func(mnt string) []string { return []string{"filesystem", "resize", "max", mnt} })
	case "f2fs":
		return p.resizeF2fs(ctx, device)
	}
	return fmt.Errorf("resize not supported for fstype %q", fstype)
}

// resizeF2fs grows an f2fs to fill the device. resize.f2fs is an offline
// resize, so — the mirror of the mounted fstypes — a *mounted* device is
// a clear, actionable error.
func (p *linuxProvider) resizeF2fs(ctx context.Context, device string) error {
	if p.resizeF2fsBin == "" {
		return fmt.Errorf("%w (resize.f2fs missing)", ErrNoResizeTool)
	}
	mnt, err := p.mountpointOf(ctx, device)
	if err != nil {
		return err
	}
	if mnt != "" {
		return fmt.Errorf("f2fs resize: %s is mounted at %s (resize.f2fs is offline — unmount it first)", device, mnt)
	}
	_, err = p.run(ctx, p.resizeF2fsBin, []string{device})
	return err
}

// growByMountpoint resolves the device's mountpoint and runs a grow tool
// against it. xfs_growfs and btrfs both grow a mounted filesystem by
// mountpoint, so an unmounted device is a clear, actionable error.
func (p *linuxProvider) growByMountpoint(ctx context.Context, device, fstype, bin, name string, args func(mnt string) []string) error {
	if bin == "" {
		return fmt.Errorf("%w (%s missing)", ErrNoResizeTool, name)
	}
	mnt, err := p.mountpointOf(ctx, device)
	if err != nil {
		return err
	}
	if mnt == "" {
		return fmt.Errorf("%s resize: %s is not mounted (%s grows a mounted filesystem by mountpoint)", fstype, device, name)
	}
	_, err = p.run(ctx, bin, args(mnt))
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
