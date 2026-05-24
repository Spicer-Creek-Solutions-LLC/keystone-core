// SPDX-License-Identifier: Apache-2.0

//go:build linux

package group

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// linuxProvider shells out to groupadd / groupmod / groupdel. These
// binaries live in /usr/sbin on every mainstream distro; we don't
// pin a path so PATH resolution finds whichever wrapper the OS
// ships (some distros symlink groupadd → adduser-shim etc).
type linuxProvider struct{ osLookup }

func defaultProvider() Provider { return linuxProvider{} }

func (p linuxProvider) Add(ctx context.Context, name string, gid *int, system bool) error {
	args := []string{}
	if gid != nil {
		args = append(args, "--gid", strconv.Itoa(*gid))
	}
	if system {
		args = append(args, "--system")
	}
	args = append(args, name)
	return runManaged(ctx, "groupadd", args)
}

func (p linuxProvider) Mod(ctx context.Context, name string, gid int) error {
	return runManaged(ctx, "groupmod", []string{"--gid", strconv.Itoa(gid), name})
}

func (p linuxProvider) Del(ctx context.Context, name string) error {
	return runManaged(ctx, "groupdel", []string{name})
}

// runManaged executes a system-group binary with the given args.
// Captures stderr and surfaces it in the wrapped error so the
// operator sees `groupadd`'s actual complaint rather than a bare
// "exit 9".
func runManaged(ctx context.Context, bin string, args []string) error {
	cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // bin is a fixed module-internal constant
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
