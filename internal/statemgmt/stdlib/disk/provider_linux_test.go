//go:build linux

package disk

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type capture struct {
	bin  string
	args []string
}

func newRecordingProvider(out string, runErr error) (*linuxProvider, *[]capture) {
	var calls []capture
	run := func(_ context.Context, bin string, args []string) (string, error) {
		calls = append(calls, capture{bin: bin, args: args})
		return out, runErr
	}
	p := &linuxProvider{
		blkidBin:  "blkid",
		wipefsBin: "wipefs",
		mkfsPaths: map[string]string{},
		run:       run,
	}
	for fstype, name := range mkfsBin {
		p.mkfsPaths[fstype] = name
	}
	return p, &calls
}

// --- GetFilesystem ----------------------------------------------------

func TestLinuxProvider_GetFilesystem_Present(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("ext4\n", nil)
	got, err := p.GetFilesystem(context.Background(), "/dev/sdb1")
	if err != nil || got != "ext4" {
		t.Fatalf("got %q err %v", got, err)
	}
	if (*calls)[0].bin != "blkid" || strings.Join((*calls)[0].args, " ") != "-o value -s TYPE /dev/sdb1" {
		t.Errorf("args: %+v", (*calls)[0])
	}
}

func TestLinuxProvider_GetFilesystem_Empty(t *testing.T) {
	t.Parallel()
	// blkid exits 2 when no signature; execRun renders the error
	// with "exit 2:" in its message
	p, _ := newRecordingProvider("", errors.New("blkid …: exit 2: "))
	got, err := p.GetFilesystem(context.Background(), "/dev/sdb1")
	if err != nil || got != "" {
		t.Errorf("no-signature: got %q,%v", got, err)
	}
}

func TestLinuxProvider_GetFilesystem_OtherError(t *testing.T) {
	t.Parallel()
	p, _ := newRecordingProvider("", errors.New("blkid …: exit 4: permission denied"))
	if _, err := p.GetFilesystem(context.Background(), "/dev/sdb1"); err == nil {
		t.Error("non-2 exit should propagate")
	}
}

func TestLinuxProvider_GetFilesystem_MissingBinary(t *testing.T) {
	t.Parallel()
	p := &linuxProvider{}
	if _, err := p.GetFilesystem(context.Background(), "/dev/sdb1"); !errors.Is(err, ErrNoBlkid) {
		t.Errorf("missing blkid → %v", err)
	}
}

func TestIsNoSignature(t *testing.T) {
	t.Parallel()
	if !isNoSignature(errors.New("…: exit 2: blah")) {
		t.Error("exit 2 should be no-signature")
	}
	if isNoSignature(errors.New("…: exit 4: blah")) {
		t.Error("exit 4 is not no-signature")
	}
	if isNoSignature(errors.New("no exit info")) {
		t.Error("non-exit error is not no-signature")
	}
}

// --- MakeFilesystem ---------------------------------------------------

func TestLinuxProvider_MakeFilesystem(t *testing.T) {
	t.Parallel()
	// ext4 with options
	p, calls := newRecordingProvider("", nil)
	if err := p.MakeFilesystem(context.Background(), "/dev/sdb1", "ext4", []string{"-F", "-L", "mylabel"}); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].bin != "mkfs.ext4" || strings.Join((*calls)[0].args, " ") != "-F -L mylabel /dev/sdb1" {
		t.Errorf("ext4 args: %+v", (*calls)[0])
	}
	// swap uses mkswap, not mkfs.swap
	p, calls = newRecordingProvider("", nil)
	if err := p.MakeFilesystem(context.Background(), "/dev/sdb2", "swap", nil); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].bin != "mkswap" || strings.Join((*calls)[0].args, " ") != "/dev/sdb2" {
		t.Errorf("swap args: %+v", (*calls)[0])
	}
	// xfs
	p, calls = newRecordingProvider("", nil)
	if err := p.MakeFilesystem(context.Background(), "/dev/sdb1", "xfs", []string{"-f"}); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].bin != "mkfs.xfs" || strings.Join((*calls)[0].args, " ") != "-f /dev/sdb1" {
		t.Errorf("xfs args: %+v", (*calls)[0])
	}
	// runner error propagates
	p, _ = newRecordingProvider("", errors.New("denied"))
	if err := p.MakeFilesystem(context.Background(), "/dev/sdb1", "ext4", nil); err == nil {
		t.Error("runner error should propagate")
	}
	// missing per-fstype binary
	p = &linuxProvider{mkfsPaths: map[string]string{"ext4": ""}}
	if err := p.MakeFilesystem(context.Background(), "/dev/sdb1", "ext4", nil); !errors.Is(err, ErrNoMkfs) {
		t.Errorf("missing mkfs.ext4 → %v", err)
	}
}

// --- WipeFilesystem --------------------------------------------------

func TestLinuxProvider_WipeFilesystem(t *testing.T) {
	t.Parallel()
	p, calls := newRecordingProvider("", nil)
	if err := p.WipeFilesystem(context.Background(), "/dev/sdb1"); err != nil {
		t.Fatal(err)
	}
	if (*calls)[0].bin != "wipefs" || strings.Join((*calls)[0].args, " ") != "-a /dev/sdb1" {
		t.Errorf("args: %+v", (*calls)[0])
	}
	// runner error
	p, _ = newRecordingProvider("", errors.New("busy"))
	if err := p.WipeFilesystem(context.Background(), "/dev/sdb1"); err == nil {
		t.Error("runner error should propagate")
	}
	// missing binary
	p = &linuxProvider{}
	if err := p.WipeFilesystem(context.Background(), "/dev/sdb1"); !errors.Is(err, ErrNoWipefs) {
		t.Errorf("missing wipefs → %v", err)
	}
}

// --- exec + defaultProvider ------------------------------------------

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil); err == nil {
		t.Error("expected an error from `false`")
	}
	if _, err := execRun(context.Background(), "/nonexistent/blkid", nil); err == nil {
		t.Error("expected an error from a missing binary")
	}
	out, err := execRun(context.Background(), "echo", []string{"-n", "ok"})
	if err != nil || out != "ok" {
		t.Errorf("echo: %q %v", out, err)
	}
}

func TestDefaultProvider_NonNil(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("defaultProvider returned nil")
	}
}
