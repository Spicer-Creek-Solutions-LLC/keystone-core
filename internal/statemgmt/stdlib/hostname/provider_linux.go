// SPDX-License-Identifier: Apache-2.0

//go:build linux

package hostname

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
)

// etcHostnamePath is the static-hostname store; variable for tests.
var etcHostnamePath = "/etc/hostname"

// lookPath is the exec.LookPath indirection; variable for tests so
// the "hostnamectl present vs absent" branch is exercisable.
var lookPath = exec.LookPath

type linuxProvider struct {
	runner commandRunner
}

// commandRunner is the injection point for hostnamectl / hostname(1).
type commandRunner func(ctx context.Context, bin string, args []string) error

func defaultProvider() Provider { return &linuxProvider{runner: execRun} }

func (p *linuxProvider) Current() (string, bool, error) {
	data, err := os.ReadFile(etcHostnamePath) //nolint:gosec // fixed path
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", etcHostnamePath, err)
	}
	return strings.TrimSpace(string(data)), true, nil
}

func (p *linuxProvider) Set(ctx context.Context, hostname string) error {
	// Prefer hostnamectl: it writes /etc/hostname, calls
	// sethostname(2), notifies dbus, and handles SELinux contexts.
	if bin, err := lookPath("hostnamectl"); err == nil {
		return p.runner(ctx, bin, []string{"set-hostname", hostname})
	}
	// Fallback: write the static store, then set the running
	// kernel hostname via hostname(1).
	if err := writeFileAtomic(etcHostnamePath, []byte(hostname+"\n")); err != nil {
		return err
	}
	bin, err := lookPath("hostname")
	if err != nil {
		return fmt.Errorf("hostname: neither hostnamectl nor hostname found on PATH")
	}
	return p.runner(ctx, bin, []string{hostname})
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".keystone.tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil { //nolint:gosec // /etc/hostname is world-readable
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

func execRun(ctx context.Context, bin string, args []string) error {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; arg is a validated hostname
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("%s %s: exit %d: %s", bin, strings.Join(args, " "), exitErr.ExitCode(), strings.TrimSpace(string(out)))
	}
	return fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), err)
}
