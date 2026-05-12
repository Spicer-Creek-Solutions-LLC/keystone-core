//go:build linux

package swap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withProcSwaps points procSwapsPath at a tempdir file. Callers must
// NOT t.Parallel().
func withProcSwaps(t *testing.T, content string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "swaps")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	old := procSwapsPath
	procSwapsPath = p
	t.Cleanup(func() { procSwapsPath = old })
}

func TestLookupProcSwaps(t *testing.T) {
	withProcSwaps(t, "Filename\t\t\t\tType\t\tSize\t\tUsed\t\tPriority\n"+
		"/dev/sda2                               partition\t2097148\t\t0\t\t-2\n"+
		"/swapfile                               file\t\t1048572\t\t1024\t\t10\n")
	mi, err := lookupProcSwaps("/dev/sda2")
	if err != nil {
		t.Fatal(err)
	}
	if !mi.Active || mi.Priority != -2 {
		t.Errorf("/dev/sda2 → %+v", mi)
	}
	mi, _ = lookupProcSwaps("/swapfile")
	if !mi.Active || mi.Priority != 10 {
		t.Errorf("/swapfile → %+v", mi)
	}
	mi, _ = lookupProcSwaps("/dev/nope")
	if mi.Active {
		t.Errorf("/dev/nope should not be active: %+v", mi)
	}
	// the header line must not be mistaken for an entry
	mi, _ = lookupProcSwaps("Filename")
	if mi.Active {
		t.Error("the header should not match")
	}
	// unreadable → error
	procSwapsPath = filepath.Join(t.TempDir(), "missing")
	if _, err := lookupProcSwaps("/dev/sda2"); err == nil {
		t.Error("missing /proc/swaps should error")
	}
}

func TestLinuxProvider_Args(t *testing.T) {
	t.Parallel()
	var calls [][]string
	run := func(_ context.Context, _ string, args []string) (string, error) {
		calls = append(calls, args)
		return "", nil
	}
	p := &linuxProvider{mkswap: "mkswap", swapon: "swapon", swapoff: "swapoff", dd: "dd", run: run}

	if err := p.MakeSwap(context.Background(), "/swapfile"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls[0], " ") != "/swapfile" {
		t.Errorf("MakeSwap args = %v", calls[0])
	}
	// swapon with priority
	calls = nil
	if err := p.SwapOn(context.Background(), "/swapfile", 7); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls[0], " ") != "-p 7 /swapfile" {
		t.Errorf("SwapOn -p args = %v", calls[0])
	}
	// swapon without priority (negative)
	calls = nil
	if err := p.SwapOn(context.Background(), "/swapfile", -1); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls[0], " ") != "/swapfile" {
		t.Errorf("SwapOn no-prio args = %v", calls[0])
	}
	// swapoff
	calls = nil
	if err := p.SwapOff(context.Background(), "/swapfile"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls[0], " ") != "/swapfile" {
		t.Errorf("SwapOff args = %v", calls[0])
	}

	// runner error propagates
	p2 := &linuxProvider{mkswap: "mkswap", run: func(context.Context, string, []string) (string, error) {
		return "", errors.New("mkswap: busy")
	}}
	if err := p2.MakeSwap(context.Background(), "/x"); err == nil {
		t.Error("MakeSwap should propagate a runner error")
	}
}

func TestLinuxProvider_CreateSwapfile(t *testing.T) {
	t.Parallel()
	var ddArgs []string
	run := func(_ context.Context, bin string, args []string) (string, error) {
		ddArgs = args
		return "", nil
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sf")
	// the fake `dd` doesn't write the file, so create it ourselves so
	// the os.Chmod step has something to act on.
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &linuxProvider{dd: "dd", run: run}
	if err := p.CreateSwapfile(context.Background(), path, 2*1024*1024); err != nil { // 2 MiB → 2048 KiB
		t.Fatal(err)
	}
	if strings.Join(ddArgs, " ") != "if=/dev/zero of="+path+" bs=1024 count=2048" {
		t.Errorf("dd args = %v", ddArgs)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("swapfile mode = %o, want 0600", fi.Mode().Perm())
	}
	// bad size (not a whole KiB)
	if err := p.CreateSwapfile(context.Background(), path, 1500); err == nil {
		t.Error("a non-KiB size should error")
	}
	// no dd binary
	p2 := &linuxProvider{dd: "", run: run}
	if err := p2.CreateSwapfile(context.Background(), path, 1024); err == nil {
		t.Error("missing dd should error")
	}
}

func TestExecRun(t *testing.T) {
	t.Parallel()
	if _, err := execRun(context.Background(), "false", nil); err == nil {
		t.Error("expected an error from `false`")
	}
	if _, err := execRun(context.Background(), "/nonexistent/mkswap", nil); err == nil {
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

func TestNoSwapProvider(t *testing.T) {
	withProcSwaps(t, "Filename Type Size Used Priority\n/dev/sda2 partition 1 0 -2\n")
	p := &noSwapProvider{}
	if mi, err := p.Lookup(context.Background(), "/dev/sda2"); err != nil || !mi.Active {
		t.Errorf("Lookup still works: %+v %v", mi, err)
	}
	if err := p.MakeSwap(context.Background(), "/x"); !errors.Is(err, ErrNoSwapTools) {
		t.Errorf("MakeSwap err = %v", err)
	}
	if err := p.SwapOn(context.Background(), "/x", -1); !errors.Is(err, ErrNoSwapTools) {
		t.Errorf("SwapOn err = %v", err)
	}
	if err := p.SwapOff(context.Background(), "/x"); !errors.Is(err, ErrNoSwapTools) {
		t.Errorf("SwapOff err = %v", err)
	}
	if err := p.CreateSwapfile(context.Background(), "/x", 1024); !errors.Is(err, ErrNoSwapTools) {
		t.Errorf("CreateSwapfile err = %v", err)
	}
}

func TestDefaultProvider_NonNil(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("defaultProvider returned nil")
	}
}
