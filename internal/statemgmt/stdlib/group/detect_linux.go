// SPDX-License-Identifier: Apache-2.0

//go:build linux

package group

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// defaultProvider auto-detects the host's group-management toolchain
// by probing for each backend's primary binary in order of
// preference:
//
//  1. groupadd on PATH → shadow-utils (shadowProvider). Preferred —
//     Debian/Ubuntu ship `addgroup` too (a Perl wrapper), so probing
//     groupadd first keeps those distros on the shadow backend.
//  2. addgroup on PATH (and no groupadd) → BusyBox (busyboxProvider).
//     The Alpine path, where shadow-utils is absent unless the
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
	if _, err := lookPath("groupadd"); err == nil {
		return shadowProvider{}
	}
	if addgroup, err := lookPath("addgroup"); err == nil {
		return newBusyboxProvider(addgroup)
	}
	return undetectedProvider{}
}

// undetectedProvider is the fallback when no supported group toolchain
// is present. Lookup is inherited from osLookup and works via NSS;
// the mutating paths return ErrNoBackend.
type undetectedProvider struct{ osLookup }

func (undetectedProvider) Add(_ context.Context, _ string, _ *int, _ bool) error { return ErrNoBackend }
func (undetectedProvider) Mod(_ context.Context, _ string, _ int) error          { return ErrNoBackend }
func (undetectedProvider) Del(_ context.Context, _ string) error                 { return ErrNoBackend }

// runManaged executes a system-group binary with the given args.
// Shared by the shadow and BusyBox backends; it is the production
// wiring behind commandRunner. Captures stderr and surfaces it in the
// wrapped error so the operator sees the tool's actual complaint
// rather than a bare "exit 9".
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
