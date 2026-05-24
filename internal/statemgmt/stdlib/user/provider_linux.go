// SPDX-License-Identifier: Apache-2.0

//go:build linux

package user

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type linuxProvider struct{ osLookup }

func defaultProvider() Provider { return linuxProvider{} }

func (p linuxProvider) Add(ctx context.Context, opts AddOptions) error {
	args := []string{}
	if opts.UID != nil {
		args = append(args, "--uid", strconv.Itoa(*opts.UID))
	}
	switch {
	case opts.GID != nil:
		args = append(args, "--gid", strconv.Itoa(*opts.GID))
	case opts.Group != "":
		// useradd's --gid accepts either GID or group name.
		args = append(args, "--gid", opts.Group)
	}
	if opts.Home != "" {
		args = append(args, "--home-dir", opts.Home)
	}
	if opts.Shell != "" {
		args = append(args, "--shell", opts.Shell)
	}
	if opts.Comment != "" {
		args = append(args, "--comment", opts.Comment)
	}
	if len(opts.Groups) > 0 {
		args = append(args, "--groups", strings.Join(opts.Groups, ","))
	}
	if opts.System {
		args = append(args, "--system")
	}
	if opts.CreateHome {
		args = append(args, "--create-home")
	} else {
		args = append(args, "--no-create-home")
	}
	args = append(args, opts.Name)
	return runManaged(ctx, "useradd", args)
}

func (p linuxProvider) Mod(ctx context.Context, opts ModOptions) error {
	args := []string{}
	if opts.UID != nil {
		args = append(args, "--uid", strconv.Itoa(*opts.UID))
	}
	switch {
	case opts.GID != nil:
		args = append(args, "--gid", strconv.Itoa(*opts.GID))
	case opts.Group != "":
		args = append(args, "--gid", opts.Group)
	}
	if opts.Home != "" {
		args = append(args, "--home", opts.Home)
	}
	if opts.Shell != "" {
		args = append(args, "--shell", opts.Shell)
	}
	if opts.Comment != "" {
		args = append(args, "--comment", opts.Comment)
	}
	if len(args) == 0 {
		// Nothing to change. Refuse rather than invoke a no-op
		// usermod (which exits 0 but burns a syscall).
		return nil
	}
	args = append(args, opts.Name)
	return runManaged(ctx, "usermod", args)
}

func (p linuxProvider) Del(ctx context.Context, name string, removeHome bool) error {
	args := []string{}
	if removeHome {
		args = append(args, "--remove")
	}
	args = append(args, name)
	return runManaged(ctx, "userdel", args)
}

func (p linuxProvider) SetGroups(ctx context.Context, name string, groups []string) error {
	// usermod --groups REPLACES the supplementary group set
	// (without --append). That's the contract the module wants.
	args := []string{"--groups", strings.Join(groups, ",")}
	if len(groups) == 0 {
		// usermod refuses an empty --groups string; pass it
		// explicitly with -G "" to clear the set. Some distros
		// require -s "" via the explicit form.
		args = []string{"--groups", ""}
	}
	args = append(args, name)
	return runManaged(ctx, "usermod", args)
}

// runManaged executes a system-user binary. Mirrors the group
// package's helper but is intentionally local — small duplication
// keeps the package boundary clean (no shared "stdlib internal").
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
