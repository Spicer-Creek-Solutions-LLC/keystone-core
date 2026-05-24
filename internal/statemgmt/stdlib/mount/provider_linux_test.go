// SPDX-License-Identifier: Apache-2.0

//go:build linux

package mount

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withProcMounts points procMountsPath at a tempdir file. Callers
// must NOT t.Parallel().
func withProcMounts(t *testing.T, content string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mounts")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	old := procMountsPath
	procMountsPath = p
	t.Cleanup(func() { procMountsPath = old })
}

func TestUnescapeMount(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		`/mnt/plain`:      `/mnt/plain`,
		`/mnt/my\040disk`: `/mnt/my disk`,
		`/a\011b`:         "/a\tb",
		`/a\134b`:         `/a\b`,
		`trailing\`:       `trailing\`, // a bare trailing backslash is left as-is
	}
	for in, want := range cases {
		if got := unescapeMount(in); got != want {
			t.Errorf("unescapeMount(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLookupProcMounts(t *testing.T) {
	withProcMounts(t, "proc /proc proc rw,nosuid 0 0\n"+
		"/dev/sda1 / ext4 rw,relatime 0 0\n"+
		"/dev/sdb1 /mnt/my\\040disk ext4 rw 0 0\n")

	mi, err := lookupProcMounts("/")
	if err != nil {
		t.Fatal(err)
	}
	if !mi.Mounted || mi.Device != "/dev/sda1" || mi.FSType != "ext4" {
		t.Errorf("/ → %+v", mi)
	}
	// escaped mount point
	mi, _ = lookupProcMounts("/mnt/my disk")
	if !mi.Mounted || mi.Device != "/dev/sdb1" {
		t.Errorf("escaped mount point → %+v", mi)
	}
	// not a mount point
	mi, _ = lookupProcMounts("/not/mounted")
	if mi.Mounted {
		t.Errorf("/not/mounted should not be reported mounted: %+v", mi)
	}
	// a parent mount must not satisfy a child path
	mi, _ = lookupProcMounts("/proc/sys")
	if mi.Mounted {
		t.Errorf("a child of /proc should not match /proc's line: %+v", mi)
	}
	// unreadable → error
	procMountsPath = filepath.Join(t.TempDir(), "missing")
	if _, err := lookupProcMounts("/"); err == nil {
		t.Error("missing /proc/mounts should error")
	}
}

func TestLinuxProvider_MountUnmountArgs(t *testing.T) {
	t.Parallel()
	var calls [][]string
	run := func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		return "", nil
	}
	p := &linuxProvider{mount: "mount", umount: "umount", run: run}
	if err := p.Mount(context.Background(), "/dev/sdb1", "/data", "ext4", "rw,noatime"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls[0], " "); got != "-t ext4 -o rw,noatime /dev/sdb1 /data" {
		t.Errorf("Mount args = %q", got)
	}
	// empty opts → no -o; empty fstype → no -t
	calls = nil
	if err := p.Mount(context.Background(), "/dev/sdb1", "/data", "", ""); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls[0], " "); got != "/dev/sdb1 /data" {
		t.Errorf("Mount args (no fstype/opts) = %q", got)
	}
	calls = nil
	if err := p.Unmount(context.Background(), "/data"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls[0], " "); got != "/data" {
		t.Errorf("Unmount args = %q", got)
	}

	// runner error propagates
	p2 := &linuxProvider{mount: "mount", umount: "umount", run: func(context.Context, string, []string) (string, error) {
		return "", errors.New("mount: bad")
	}}
	if err := p2.Mount(context.Background(), "/d", "/m", "ext4", ""); err == nil {
		t.Error("Mount should propagate a runner error")
	}
}

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil); err == nil {
		t.Error("expected an error from `false`")
	}
	if _, err := execRun(context.Background(), "/nonexistent/mount", nil); err == nil {
		t.Error("expected an error from a missing binary")
	}
	out, err := execRun(context.Background(), "echo", []string{"-n", "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Errorf("echo = %q", out)
	}
}

func TestNoMountProvider(t *testing.T) {
	withProcMounts(t, "/dev/sda1 / ext4 rw 0 0\n")
	p := &noMountProvider{}
	if mi, err := p.Lookup(context.Background(), "/"); err != nil || !mi.Mounted {
		t.Errorf("Lookup still works: %+v %v", mi, err)
	}
	if err := p.Mount(context.Background(), "/d", "/m", "ext4", ""); !errors.Is(err, ErrNoMountTools) {
		t.Errorf("Mount err = %v", err)
	}
	if err := p.Unmount(context.Background(), "/m"); !errors.Is(err, ErrNoMountTools) {
		t.Errorf("Unmount err = %v", err)
	}
}

func TestDefaultProvider_NonNil(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("defaultProvider returned nil")
	}
}
