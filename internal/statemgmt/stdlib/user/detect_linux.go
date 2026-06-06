// SPDX-License-Identifier: Apache-2.0

//go:build linux

package user

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// defaultProvider auto-detects the host's user-management toolchain by
// probing for each backend's primary binary in order of preference:
//
//  1. useradd on PATH → shadow-utils (shadowProvider). Preferred —
//     Debian/Ubuntu ship `adduser` too (a Perl wrapper around
//     useradd), so probing useradd first keeps those distros on the
//     full-featured shadow backend rather than the BusyBox one.
//  2. adduser on PATH (and no useradd) → BusyBox (busyboxProvider).
//     This is the Alpine path, where shadow-utils is absent unless the
//     `shadow` package is installed.
//  3. Neither → undetectedProvider (Lookup still works via NSS;
//     mutating ops return ErrNoBackend).
//
// Detection is binary-presence based rather than reading
// /etc/os-release: what matters at runtime is which tool is present.
func defaultProvider() Provider { return detectProvider(exec.LookPath) }

// detectProvider is the testable core of defaultProvider: lookPath is
// exec.LookPath in production and a fake in tests, so detection
// ordering is exercised without mutating the process PATH (which would
// race the shadow backend's parallel exec tests).
func detectProvider(lookPath func(string) (string, error)) Provider {
	if _, err := lookPath("useradd"); err == nil {
		return shadowProvider{}
	}
	if adduser, err := lookPath("adduser"); err == nil {
		return newBusyboxProvider(adduser)
	}
	return undetectedProvider{}
}

// undetectedProvider is the fallback when no supported user toolchain
// is present. Lookup is inherited from osLookup and works via NSS;
// the mutating paths return ErrNoBackend.
type undetectedProvider struct{ osLookup }

func (undetectedProvider) Add(_ context.Context, _ AddOptions) error     { return ErrNoBackend }
func (undetectedProvider) Mod(_ context.Context, _ ModOptions) error     { return ErrNoBackend }
func (undetectedProvider) Del(_ context.Context, _ string, _ bool) error { return ErrNoBackend }
func (undetectedProvider) SetGroups(_ context.Context, _ string, _ []string) error {
	return ErrNoBackend
}

// runManaged executes a system-user binary. Shared by the shadow and
// BusyBox backends; it is the production wiring behind commandRunner.
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
