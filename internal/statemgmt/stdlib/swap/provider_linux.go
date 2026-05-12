//go:build linux

package swap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// procSwapsPath is the kernel swap table; a package var for tests.
var procSwapsPath = "/proc/swaps"

func defaultProvider() Provider {
	mkswap, e1 := exec.LookPath("mkswap")
	swapon, e2 := exec.LookPath("swapon")
	swapoff, e3 := exec.LookPath("swapoff")
	if e1 != nil || e2 != nil || e3 != nil {
		return &noSwapProvider{}
	}
	dd, _ := exec.LookPath("dd") // may be absent; only needed to create swapfiles
	return &linuxProvider{mkswap: mkswap, swapon: swapon, swapoff: swapoff, dd: dd, run: execRun}
}

type linuxProvider struct {
	mkswap  string
	swapon  string
	swapoff string
	dd      string
	run     commandRunner
}

func (p *linuxProvider) Lookup(_ context.Context, source string) (*SwapInfo, error) {
	return lookupProcSwaps(source)
}

func (p *linuxProvider) MakeSwap(ctx context.Context, path string) error {
	_, err := p.run(ctx, p.mkswap, []string{path})
	return err
}

func (p *linuxProvider) SwapOn(ctx context.Context, path string, priority int) error {
	args := []string{}
	if priority >= 0 {
		args = append(args, "-p", strconv.Itoa(priority))
	}
	args = append(args, path)
	_, err := p.run(ctx, p.swapon, args)
	return err
}

func (p *linuxProvider) SwapOff(ctx context.Context, path string) error {
	_, err := p.run(ctx, p.swapoff, []string{path})
	return err
}

func (p *linuxProvider) CreateSwapfile(ctx context.Context, path string, sizeBytes int64) error {
	if p.dd == "" {
		return fmt.Errorf("swap: `dd` not found on PATH (needed to create a swapfile)")
	}
	if sizeBytes <= 0 || sizeBytes%1024 != 0 {
		return fmt.Errorf("swap: invalid swapfile size %d bytes (must be a positive whole number of KiB)", sizeBytes)
	}
	kib := sizeBytes / 1024
	if _, err := p.run(ctx, p.dd, []string{"if=/dev/zero", "of=" + path, "bs=1024", "count=" + strconv.FormatInt(kib, 10)}); err != nil {
		return fmt.Errorf("create swapfile %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}

// lookupProcSwaps scans procSwapsPath for a line whose first field
// (Filename) equals source.
func lookupProcSwaps(source string) (*SwapInfo, error) {
	data, err := os.ReadFile(procSwapsPath) //nolint:gosec // procSwapsPath is a fixed kernel path (overridable only in tests)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", procSwapsPath, err)
	}
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 { // header: "Filename Type Size Used Priority"
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		if f[0] == source {
			prio, _ := strconv.Atoi(f[4])
			return &SwapInfo{Source: source, Active: true, Priority: prio}, nil
		}
	}
	return &SwapInfo{Source: source, Active: false}, nil
}

// execRun is the production commandRunner. Captures combined output
// so mkswap/swapon/swapoff/dd complaints reach the operator.
func execRun(ctx context.Context, bin string, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; args are fixed flags + an operator-supplied swap-source path / numeric priority / size from a validated state declaration
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

// noSwapProvider stands in when the mkswap/swapon/swapoff binaries
// are absent. Lookup still works (it only reads /proc/swaps); the
// mutating ops fail with ErrNoSwapTools.
type noSwapProvider struct{}

func (*noSwapProvider) Lookup(_ context.Context, source string) (*SwapInfo, error) {
	return lookupProcSwaps(source)
}
func (*noSwapProvider) MakeSwap(context.Context, string) error              { return ErrNoSwapTools }
func (*noSwapProvider) SwapOn(context.Context, string, int) error           { return ErrNoSwapTools }
func (*noSwapProvider) SwapOff(context.Context, string) error               { return ErrNoSwapTools }
func (*noSwapProvider) CreateSwapfile(context.Context, string, int64) error { return ErrNoSwapTools }
