// SPDX-License-Identifier: Apache-2.0

//go:build linux

package disk

import (
	"context"
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
