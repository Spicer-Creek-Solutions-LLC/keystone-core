// SPDX-License-Identifier: Apache-2.0

//go:build linux

package disk

import (
	"context"
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
	"testing"
)

const sampleDumpe2fs = `dumpe2fs 1.47.0
Filesystem volume name:   <none>
Block count:              2621440
Block size:               4096
Reserved block count:     131072
`

func TestExtFillsDevice(t *testing.T) {
	t.Parallel()
	const tenGiB = 2621440 * 4096 // = 10737418240

	// fs exactly fills the device
	if fills, err := extFillsDevice(sampleDumpe2fs, tenGiB); err != nil || !fills {
		t.Errorf("equal sizes → fills; got %v %v", fills, err)
	}
	// device is larger than the fs → needs grow
	if fills, err := extFillsDevice(sampleDumpe2fs, tenGiB*2); err != nil || fills {
		t.Errorf("larger device → not fills; got %v %v", fills, err)
	}
	// within one block of the device → fills (maximal)
	if fills, err := extFillsDevice(sampleDumpe2fs, tenGiB+2048); err != nil || !fills {
		t.Errorf("within one block → fills; got %v %v", fills, err)
	}
	// missing fields → error
	if _, err := extFillsDevice("no fields here", tenGiB); err == nil {
		t.Error("missing Block count/size → error")
	}
	if _, err := extFillsDevice("Block count:              100\n", tenGiB); err == nil {
		t.Error("missing Block size → error")
	}
}

func TestLinuxProvider_FilesystemFillsDevice(t *testing.T) {
	t.Parallel()
	p := &linuxProvider{
		blockdevBin: "/usr/sbin/blockdev",
		dumpe2fsBin: "/usr/sbin/dumpe2fs",
		run: func(_ context.Context, bin string, _ []string) (string, error) {
			if strings.HasSuffix(bin, "blockdev") {
				return "21474836480\n", nil // 20 GiB device
			}
			return sampleDumpe2fs, nil // 10 GiB fs
		},
	}
	fills, err := p.FilesystemFillsDevice(context.Background(), "/dev/sda1", "ext4")
	if err != nil || fills {
		t.Errorf("10G fs on 20G device → not fills; got %v %v", fills, err)
	}
	// missing tools
	if _, err := (&linuxProvider{}).FilesystemFillsDevice(context.Background(), "/dev/sda1", "ext4"); !IsNoResizeTool(err) {
		t.Errorf("missing blockdev/dumpe2fs → ErrNoResizeTool, got %v", err)
	}
}

func TestLinuxProvider_ResizeFilesystem(t *testing.T) {
	t.Parallel()
	var gotBin string
	var gotArgs []string
	p := &linuxProvider{
		resize2fsBin: "/usr/sbin/resize2fs",
		run: func(_ context.Context, bin string, args []string) (string, error) {
			gotBin, gotArgs = bin, args
			return "", nil
		},
	}
	if err := p.ResizeFilesystem(context.Background(), "/dev/sda1", "ext4"); err != nil {
		t.Fatal(err)
	}
	if gotBin != "/usr/sbin/resize2fs" || len(gotArgs) != 1 || gotArgs[0] != "/dev/sda1" {
		t.Errorf("resize2fs call = %s %v", gotBin, gotArgs)
	}
	if err := (&linuxProvider{}).ResizeFilesystem(context.Background(), "/dev/sda1", "ext4"); !IsNoResizeTool(err) {
		t.Errorf("missing resize2fs → ErrNoResizeTool, got %v", err)
	}
}

// --- xfs / btrfs parsers ----------------------------------------------

const sampleXfsInfo = `meta-data=/dev/sdb               isize=512    agcount=4, agsize=655360 blks
         =                       sectsz=512   attr=2, projid32bit=1
data     =                       bsize=4096   blocks=2621440, imaxpct=25
         =                       sunit=0      swidth=0 blks
naming   =version 2              bsize=4096   ascii-ci=0, ftype=1
log      =internal log           bsize=4096   blocks=2560, version=2
`

func TestParseXfsInfoBytes(t *testing.T) {
	t.Parallel()
	fsBytes, bsize, err := parseXfsInfoBytes(sampleXfsInfo)
	if err != nil || bsize != 4096 || fsBytes != 2621440*4096 {
		t.Errorf("parseXfsInfoBytes = %d,%d,%v", fsBytes, bsize, err)
	}
	if _, _, err := parseXfsInfoBytes("no data section here\n"); err == nil {
		t.Error("missing data line → error")
	}
}

func TestParseBtrfsShowBytes(t *testing.T) {
	t.Parallel()
	single := "Label: none  uuid: 1b3\n\tTotal devices 1 FS bytes used 196608\n\tdevid    1 size 10737418240 used 2172649472 path /dev/sdb\n"
	if n, err := parseBtrfsShowBytes(single, "/dev/sdb"); err != nil || n != 10737418240 {
		t.Errorf("single match = %d,%v", n, err)
	}
	// single device, path not matched literally → fall back to the only devid
	if n, err := parseBtrfsShowBytes(single, "/dev/disk/by-id/x"); err != nil || n != 10737418240 {
		t.Errorf("single fallback = %d,%v", n, err)
	}
	multi := "\tdevid 1 size 100 used 0 path /dev/sdb\n\tdevid 2 size 200 used 0 path /dev/sdc\n"
	if n, err := parseBtrfsShowBytes(multi, "/dev/sdc"); err != nil || n != 200 {
		t.Errorf("multi match = %d,%v", n, err)
	}
	if _, err := parseBtrfsShowBytes(multi, "/dev/sdd"); err == nil {
		t.Error("multi-device, no match → error")
	}
	if _, err := parseBtrfsShowBytes("no devid lines\n", "/dev/sdb"); err == nil {
		t.Error("no devid line → error")
	}
}

// --- xfs / btrfs fill checks ------------------------------------------

func TestXfsFillsDevice(t *testing.T) {
	t.Parallel()
	mounted := &linuxProvider{
		blockdevBin: "blockdev", findmntBin: "findmnt", xfsInfoBin: "xfs_info",
		run: func(_ context.Context, bin string, args []string) (string, error) {
			switch {
			case strings.HasSuffix(bin, "blockdev"):
				return "21474836480\n", nil // 20 GiB device
			case strings.HasSuffix(bin, "findmnt"):
				return "/mnt/data\n", nil // mounted
			default: // xfs_info
				if len(args) != 1 || args[0] != "/mnt/data" {
					t.Errorf("xfs_info should target the mountpoint, got %v", args)
				}
				return sampleXfsInfo, nil
			}
		},
	}
	if fills, err := mounted.FilesystemFillsDevice(context.Background(), "/dev/sdb", "xfs"); err != nil || fills {
		t.Errorf("10G xfs on 20G device → not fills; got %v %v", fills, err)
	}

	// unmounted: findmnt fails → xfs_info targets the device
	unmounted := &linuxProvider{
		blockdevBin: "blockdev", findmntBin: "findmnt", xfsInfoBin: "xfs_info",
		run: func(_ context.Context, bin string, args []string) (string, error) {
			switch {
			case strings.HasSuffix(bin, "blockdev"):
				return "10737418240\n", nil // 10 GiB — equals fs
			case strings.HasSuffix(bin, "findmnt"):
				return "", errors.New("exit 1") // not mounted
			default:
				if args[0] != "/dev/sdb" {
					t.Errorf("unmounted xfs_info should target the device, got %v", args)
				}
				return sampleXfsInfo, nil
			}
		},
	}
	if fills, err := unmounted.FilesystemFillsDevice(context.Background(), "/dev/sdb", "xfs"); err != nil || !fills {
		t.Errorf("10G xfs on 10G device → fills; got %v %v", fills, err)
	}
}

func TestBtrfsFillsDevice(t *testing.T) {
	t.Parallel()
	p := &linuxProvider{
		blockdevBin: "blockdev", btrfsBin: "btrfs",
		run: func(_ context.Context, bin string, _ []string) (string, error) {
			if strings.HasSuffix(bin, "blockdev") {
				return "10737418240\n", nil // 10 GiB device == fs
			}
			return "\tdevid 1 size 10737418240 used 0 path /dev/sdb\n", nil
		},
	}
	if fills, err := p.FilesystemFillsDevice(context.Background(), "/dev/sdb", "btrfs"); err != nil || !fills {
		t.Errorf("btrfs spanning device → fills; got %v %v", fills, err)
	}
}

func TestFillsDevice_MissingTools(t *testing.T) {
	t.Parallel()
	withBlockdev := func() *linuxProvider {
		return &linuxProvider{blockdevBin: "blockdev", run: func(context.Context, string, []string) (string, error) { return "100\n", nil }}
	}
	if _, err := withBlockdev().FilesystemFillsDevice(context.Background(), "/dev/sdb", "xfs"); !IsNoResizeTool(err) {
		t.Errorf("missing xfs_info → ErrNoResizeTool, got %v", err)
	}
	if _, err := withBlockdev().FilesystemFillsDevice(context.Background(), "/dev/sdb", "btrfs"); !IsNoResizeTool(err) {
		t.Errorf("missing btrfs → ErrNoResizeTool, got %v", err)
	}
	if _, err := (&linuxProvider{}).FilesystemFillsDevice(context.Background(), "/dev/sdb", "xfs"); !IsNoResizeTool(err) {
		t.Errorf("missing blockdev → ErrNoResizeTool, got %v", err)
	}
}

// --- xfs / btrfs resize (by mountpoint) -------------------------------

func TestResizeFilesystem_XFS(t *testing.T) {
	t.Parallel()
	var gotBin string
	var gotArgs []string
	mounted := &linuxProvider{
		findmntBin: "findmnt", xfsGrowfsBin: "xfs_growfs",
		run: func(_ context.Context, bin string, args []string) (string, error) {
			if strings.HasSuffix(bin, "findmnt") {
				return "/mnt/data\n", nil
			}
			gotBin, gotArgs = bin, args
			return "", nil
		},
	}
	if err := mounted.ResizeFilesystem(context.Background(), "/dev/sdb", "xfs"); err != nil {
		t.Fatal(err)
	}
	if gotBin != "xfs_growfs" || len(gotArgs) != 1 || gotArgs[0] != "/mnt/data" {
		t.Errorf("xfs_growfs call = %s %v", gotBin, gotArgs)
	}

	// unmounted → clear error, no growfs
	unmounted := &linuxProvider{
		findmntBin: "findmnt", xfsGrowfsBin: "xfs_growfs",
		run: func(_ context.Context, bin string, _ []string) (string, error) {
			if strings.HasSuffix(bin, "findmnt") {
				return "", errors.New("exit 1")
			}
			t.Error("xfs_growfs must not run on an unmounted device")
			return "", nil
		},
	}
	if err := unmounted.ResizeFilesystem(context.Background(), "/dev/sdb", "xfs"); err == nil || !strings.Contains(err.Error(), "not mounted") {
		t.Errorf("unmounted xfs resize should error 'not mounted'; got %v", err)
	}
	// missing tool
	if err := (&linuxProvider{}).ResizeFilesystem(context.Background(), "/dev/sdb", "xfs"); !IsNoResizeTool(err) {
		t.Errorf("missing xfs_growfs → ErrNoResizeTool, got %v", err)
	}
}

func TestResizeFilesystem_Btrfs(t *testing.T) {
	t.Parallel()
	var gotArgs []string
	p := &linuxProvider{
		findmntBin: "findmnt", btrfsBin: "btrfs",
		run: func(_ context.Context, bin string, args []string) (string, error) {
			if strings.HasSuffix(bin, "findmnt") {
				return "/srv\n", nil
			}
			gotArgs = args
			return "", nil
		},
	}
	if err := p.ResizeFilesystem(context.Background(), "/dev/sdb", "btrfs"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(gotArgs, " ") != "filesystem resize max /srv" {
		t.Errorf("btrfs resize call = %v", gotArgs)
	}
}

func TestResizeFilesystem_UnsupportedFstype(t *testing.T) {
	t.Parallel()
	// blockdev present so FilesystemFillsDevice reaches the fstype switch.
	p := &linuxProvider{blockdevBin: "blockdev", run: func(context.Context, string, []string) (string, error) { return "100\n", nil }}
	if err := p.ResizeFilesystem(context.Background(), "/dev/sdb", "vfat"); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("vfat resize should error 'not supported'; got %v", err)
	}
	if _, err := p.FilesystemFillsDevice(context.Background(), "/dev/sdb", "vfat"); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Errorf("vfat fill-check should error 'not supported'; got %v", err)
	}
}

// --- f2fs (superblock fill-check + offline resize) --------------------

// f2fsSuper builds a 64-byte f2fs superblock image with the geometry
// fields parseF2fsSuperblock reads. logBlocksize 12 → 4 KiB blocks;
// logBlocksPerSeg 9 + segsPerSec 1 → 512-block (2 MiB) sections.
func f2fsSuper(blockCount uint64, logBlocksize, logBlocksPerSeg, segsPerSec uint32) []byte {
	b := make([]byte, f2fsSuperReadLen)
	binary.LittleEndian.PutUint32(b[0:4], f2fsMagic)
	binary.LittleEndian.PutUint32(b[16:20], logBlocksize)
	binary.LittleEndian.PutUint32(b[20:24], logBlocksPerSeg)
	binary.LittleEndian.PutUint32(b[24:28], segsPerSec)
	binary.LittleEndian.PutUint64(b[36:44], blockCount)
	return b
}

func TestParseF2fsSuperblock(t *testing.T) {
	t.Parallel()
	bc, bs, sec, err := parseF2fsSuperblock(f2fsSuper(2560, 12, 9, 1))
	if err != nil || bc != 2560 || bs != 4096 || sec != 512 {
		t.Errorf("parse = %d,%d,%d,%v", bc, bs, sec, err)
	}
	// bad magic
	if _, _, _, err := parseF2fsSuperblock(make([]byte, f2fsSuperReadLen)); err == nil {
		t.Error("zero superblock (bad magic) should error")
	}
	// too short
	if _, _, _, err := parseF2fsSuperblock(f2fsSuper(1, 12, 9, 1)[:10]); err == nil {
		t.Error("short superblock should error")
	}
	// implausible geometry
	if _, _, _, err := parseF2fsSuperblock(f2fsSuper(1, 99, 9, 1)); err == nil {
		t.Error("implausible log_blocksize should error")
	}
	if _, _, _, err := parseF2fsSuperblock(f2fsSuper(1, 12, 9, 0)); err == nil {
		t.Error("segs_per_sec=0 should error")
	}
}

func f2fsProvider(deviceBytes, blockCount uint64) *linuxProvider {
	return &linuxProvider{
		blockdevBin: "blockdev",
		run: func(_ context.Context, _ string, _ []string) (string, error) {
			return strconv.FormatUint(deviceBytes, 10) + "\n", nil
		},
		readAt: func(_ string, off int64, buf []byte) (int, error) {
			if off != f2fsSuperOffset {
				return 0, errors.New("unexpected offset")
			}
			return copy(buf, f2fsSuper(blockCount, 12, 9, 1)), nil
		},
	}
}

func TestF2fsFillsDevice(t *testing.T) {
	t.Parallel()
	const blk, mib = 4096, 1024 * 1024
	cases := []struct {
		name        string
		deviceBytes uint64
		blockCount  uint64
		wantFills   bool
	}{
		{"exactly full", 10 * mib, 10 * mib / blk, true},
		{"needs grow", 10 * mib, 8 * mib / blk, false},
		// 11 MiB device, 10 MiB fs: the extra 1 MiB is less than a 2 MiB
		// section, so resize.f2fs can't grow — section-aware = already full.
		{"sub-section slack is full", 11 * mib, 10 * mib / blk, true},
		// 12 MiB device, 10 MiB fs: a whole 2 MiB section fits → grow.
		{"whole section available", 12 * mib, 10 * mib / blk, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fills, err := f2fsProvider(c.deviceBytes, c.blockCount).
				FilesystemFillsDevice(context.Background(), "/dev/sdb", "f2fs")
			if err != nil || fills != c.wantFills {
				t.Errorf("fills = %v (err %v), want %v", fills, err, c.wantFills)
			}
		})
	}

	// a bad superblock surfaces as an error
	bad := &linuxProvider{
		blockdevBin: "blockdev",
		run:         func(context.Context, string, []string) (string, error) { return "1048576\n", nil },
		readAt:      func(_ string, _ int64, buf []byte) (int, error) { return len(buf), nil }, // zeroed → bad magic
	}
	if _, err := bad.FilesystemFillsDevice(context.Background(), "/dev/sdb", "f2fs"); err == nil {
		t.Error("bad superblock should error")
	}
}

func TestResizeFilesystem_F2fs(t *testing.T) {
	t.Parallel()
	var gotBin string
	var gotArgs []string
	unmounted := &linuxProvider{
		findmntBin: "findmnt", resizeF2fsBin: "resize.f2fs",
		run: func(_ context.Context, bin string, args []string) (string, error) {
			if strings.HasSuffix(bin, "findmnt") {
				return "", errors.New("exit 1") // not mounted
			}
			gotBin, gotArgs = bin, args
			return "", nil
		},
	}
	if err := unmounted.ResizeFilesystem(context.Background(), "/dev/sdb", "f2fs"); err != nil {
		t.Fatal(err)
	}
	if gotBin != "resize.f2fs" || len(gotArgs) != 1 || gotArgs[0] != "/dev/sdb" {
		t.Errorf("resize.f2fs call = %s %v", gotBin, gotArgs)
	}

	// mounted → clear error, no resize
	mounted := &linuxProvider{
		findmntBin: "findmnt", resizeF2fsBin: "resize.f2fs",
		run: func(_ context.Context, bin string, _ []string) (string, error) {
			if strings.HasSuffix(bin, "findmnt") {
				return "/mnt/f\n", nil
			}
			t.Error("resize.f2fs must not run on a mounted device")
			return "", nil
		},
	}
	if err := mounted.ResizeFilesystem(context.Background(), "/dev/sdb", "f2fs"); err == nil || !strings.Contains(err.Error(), "mounted") {
		t.Errorf("mounted f2fs resize should error 'mounted'; got %v", err)
	}
	// missing tool
	if err := (&linuxProvider{}).ResizeFilesystem(context.Background(), "/dev/sdb", "f2fs"); !IsNoResizeTool(err) {
		t.Errorf("missing resize.f2fs → ErrNoResizeTool, got %v", err)
	}
}
