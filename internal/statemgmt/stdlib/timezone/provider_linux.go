// SPDX-License-Identifier: Apache-2.0

//go:build linux

package timezone

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Paths are variables for tests.
var (
	localtimePath = "/etc/localtime"
	zoneinfoRoot  = "/usr/share/zoneinfo"
	etcTimezone   = "/etc/timezone"
)

// lookPath indirection for the timedatectl-present-vs-absent branch.
var lookPath = exec.LookPath

type linuxProvider struct {
	runner commandRunner
}

type commandRunner func(ctx context.Context, bin string, args []string) error

func defaultProvider() Provider { return &linuxProvider{runner: execRun} }

func (p *linuxProvider) Current() (string, bool, error) {
	target, err := os.Readlink(localtimePath)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		// Not a symlink (some setups copy the zoneinfo file
		// directly) — we can't derive the zone name, so report
		// "unknown" which the module treats as drift.
		return "", false, nil
	}
	// target may be relative ("../usr/share/zoneinfo/UTC") or
	// absolute. Resolve relative paths against /etc, then strip the
	// zoneinfo prefix.
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(localtimePath), target)
	}
	target = filepath.Clean(target)
	root := filepath.Clean(zoneinfoRoot) + string(filepath.Separator)
	if !strings.HasPrefix(target, root) {
		return "", false, nil
	}
	return strings.TrimPrefix(target, root), true, nil
}

func (p *linuxProvider) Set(ctx context.Context, zone string) error {
	// Validate the zone exists in the zoneinfo tree before doing
	// anything — both for the timedatectl path (it validates too,
	// but a clean error message is nicer) and the fallback path
	// (which would otherwise create a dangling symlink).
	zonePath := filepath.Join(zoneinfoRoot, zone)
	if _, err := os.Stat(zonePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrZoneNotFound, zone)
		}
		return fmt.Errorf("stat %s: %w", zonePath, err)
	}

	if bin, err := lookPath("timedatectl"); err == nil {
		return p.runner(ctx, bin, []string{"set-timezone", zone})
	}

	// Fallback: symlink /etc/localtime → the zoneinfo file, and
	// write /etc/timezone (Debian-family reads it).
	if err := symlinkAtomic(zonePath, localtimePath); err != nil {
		return err
	}
	if err := os.WriteFile(etcTimezone, []byte(zone+"\n"), 0o644); err != nil { //nolint:gosec // /etc/timezone is world-readable
		return fmt.Errorf("write %s: %w", etcTimezone, err)
	}
	return nil
}

// symlinkAtomic creates link → target, replacing any existing link.
// Writes a temp symlink then renames it so a crash mid-operation
// doesn't leave /etc/localtime missing.
func symlinkAtomic(target, link string) error {
	tmp := link + ".keystone.tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return fmt.Errorf("symlink tmp: %w", err)
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", link, err)
	}
	return nil
}

func execRun(ctx context.Context, bin string, args []string) error {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin resolved via exec.LookPath; arg is a validated zone name
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
