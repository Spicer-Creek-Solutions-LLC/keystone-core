// SPDX-License-Identifier: Apache-2.0

package systemd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Options configures Install / Uninstall / Status. Zero values
// fall back to sensible defaults; only Logger is strictly
// required (Install logs every systemctl invocation for audit).
type Options struct {
	// UnitDir is where the .service file lands. Defaults to
	// /etc/systemd/system.
	UnitDir string

	// UnitName is the filename. Defaults to keystone-core-agent.service.
	UnitName string

	// Runner abstracts systemctl. Defaults to NewDefaultRunner()
	// — tests inject a FakeRunner.
	Runner Runner

	// Logger is required.
	Logger *slog.Logger

	// Enable runs `systemctl enable` after writing the unit. Pair
	// with Start to enable + start in one step.
	Enable bool

	// Start runs `systemctl start` after writing (and enabling,
	// if Enable=true). When both are set, we issue
	// `enable --now` to combine the two systemctl round-trips.
	Start bool

	// DryRun renders + reports what would happen but writes no
	// files and runs no systemctl commands.
	DryRun bool
}

func (o Options) fillDefaults() Options {
	if o.UnitDir == "" {
		o.UnitDir = DefaultUnitDir
	}
	if o.UnitName == "" {
		o.UnitName = DefaultUnitName
	}
	if o.Runner == nil {
		o.Runner = NewDefaultRunner()
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return o
}

// InstallResult summarizes what Install did. Created/Updated
// distinguish first-install from converged-already; Reloaded /
// Enabled / Started flag the systemctl side effects.
type InstallResult struct {
	UnitPath  string
	Created   bool
	Updated   bool
	Reloaded  bool
	Enabled   bool
	Started   bool
	BytesOnDisk int
}

// Install renders the unit, atomic-writes it to
// {UnitDir}/{UnitName}, and (if changed) runs daemon-reload +
// optional enable / start. Idempotent — same content on a re-run
// fires zero systemctl commands.
func Install(ctx context.Context, p Params, opts Options) (*InstallResult, error) {
	opts = opts.fillDefaults()
	body, err := Render(p)
	if err != nil {
		return nil, err
	}
	unitPath := filepath.Join(opts.UnitDir, opts.UnitName)

	if opts.DryRun {
		opts.Logger.InfoContext(ctx, "systemd: Install (dry run — no side effects)",
			"unit_path", unitPath,
			"would_enable", opts.Enable,
			"would_start", opts.Start,
		)
		return &InstallResult{UnitPath: unitPath, BytesOnDisk: len(body)}, nil
	}

	existing, statErr := os.ReadFile(unitPath) //nolint:gosec // operator-controlled path
	created := errors.Is(statErr, os.ErrNotExist)
	updated := false
	switch {
	case created:
		// new install — write
	case statErr != nil:
		return nil, fmt.Errorf("systemd: read existing unit %q: %w", unitPath, statErr)
	case bytes.Equal(existing, body):
		// idempotent — content unchanged. Skip daemon-reload.
		// We still enable/start if requested + currently
		// disabled/inactive — but that's covered by the caller
		// re-running with the same Options on subsequent boots.
		opts.Logger.InfoContext(ctx, "systemd: Install (no-op; unit converged)",
			"unit_path", unitPath)
		return &InstallResult{
			UnitPath:    unitPath,
			BytesOnDisk: len(existing),
		}, nil
	default:
		updated = true
	}

	if err := atomicWriteFile(unitPath, body, 0o644); err != nil {
		return nil, fmt.Errorf("systemd: write unit %q: %w", unitPath, err)
	}
	opts.Logger.InfoContext(ctx, "systemd: wrote unit",
		"unit_path", unitPath,
		"bytes", len(body),
		"created", created,
		"updated", updated,
	)

	if _, err := opts.Runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return nil, fmt.Errorf("systemd: daemon-reload: %w", err)
	}
	res := &InstallResult{
		UnitPath:    unitPath,
		Created:     created,
		Updated:     updated,
		Reloaded:    true,
		BytesOnDisk: len(body),
	}

	switch {
	case opts.Enable && opts.Start:
		// Combine: `enable --now` does both.
		if _, err := opts.Runner.Run(ctx, "systemctl", "enable", "--now", opts.UnitName); err != nil {
			return res, fmt.Errorf("systemd: enable --now: %w", err)
		}
		res.Enabled = true
		res.Started = true
	case opts.Enable:
		if _, err := opts.Runner.Run(ctx, "systemctl", "enable", opts.UnitName); err != nil {
			return res, fmt.Errorf("systemd: enable: %w", err)
		}
		res.Enabled = true
	case opts.Start:
		if _, err := opts.Runner.Run(ctx, "systemctl", "start", opts.UnitName); err != nil {
			return res, fmt.Errorf("systemd: start: %w", err)
		}
		res.Started = true
	}
	return res, nil
}

// UninstallResult summarizes what Uninstall did. NoUnit=true
// means there was nothing to uninstall — Uninstall returns nil
// in that case (idempotent).
type UninstallResult struct {
	UnitPath string
	Stopped  bool
	Disabled bool
	Removed  bool
	Reloaded bool
	NoUnit   bool
}

// Uninstall reverses Install: stop + disable + remove unit file +
// daemon-reload. Idempotent — if no unit file exists, returns
// NoUnit=true with no systemctl invocations.
func Uninstall(ctx context.Context, opts Options) (*UninstallResult, error) {
	opts = opts.fillDefaults()
	unitPath := filepath.Join(opts.UnitDir, opts.UnitName)

	if _, err := os.Stat(unitPath); errors.Is(err, os.ErrNotExist) {
		opts.Logger.InfoContext(ctx, "systemd: Uninstall (no-op; unit not present)",
			"unit_path", unitPath)
		return &UninstallResult{UnitPath: unitPath, NoUnit: true}, nil
	} else if err != nil {
		return nil, fmt.Errorf("systemd: stat %q: %w", unitPath, err)
	}

	if opts.DryRun {
		opts.Logger.InfoContext(ctx, "systemd: Uninstall (dry run — no side effects)",
			"unit_path", unitPath)
		return &UninstallResult{UnitPath: unitPath}, nil
	}

	res := &UninstallResult{UnitPath: unitPath}

	// Stop first; if the service isn't active, systemctl exits
	// non-zero. We tolerate that — caller wants the unit gone
	// regardless. Same logic for disable.
	if _, err := opts.Runner.Run(ctx, "systemctl", "stop", opts.UnitName); err != nil {
		opts.Logger.WarnContext(ctx, "systemd: stop returned error (continuing)",
			"err", err.Error())
	} else {
		res.Stopped = true
	}
	if _, err := opts.Runner.Run(ctx, "systemctl", "disable", opts.UnitName); err != nil {
		opts.Logger.WarnContext(ctx, "systemd: disable returned error (continuing)",
			"err", err.Error())
	} else {
		res.Disabled = true
	}

	if err := os.Remove(unitPath); err != nil {
		return res, fmt.Errorf("systemd: remove %q: %w", unitPath, err)
	}
	res.Removed = true

	if _, err := opts.Runner.Run(ctx, "systemctl", "daemon-reload"); err != nil {
		return res, fmt.Errorf("systemd: daemon-reload: %w", err)
	}
	res.Reloaded = true

	opts.Logger.InfoContext(ctx, "systemd: uninstalled",
		"unit_path", unitPath)
	return res, nil
}

// StatusResult is what Status reports. UnitPresent=false means
// the unit file isn't installed (no systemctl invocation made).
type StatusResult struct {
	UnitName    string
	UnitPath    string
	UnitPresent bool
	Active      bool
	Enabled     bool
	// ActiveState is the raw systemctl is-active output (e.g.
	// "active", "inactive", "failed"). Operators want the
	// unparsed value when triaging.
	ActiveState  string
	EnabledState string
}

// Status wraps `systemctl is-active` + `is-enabled`. systemctl
// returns non-zero exit codes for inactive/disabled — we ignore
// the exit code and use the output line. On a host without the
// unit installed, returns UnitPresent=false with no systemctl
// invocation.
func Status(ctx context.Context, opts Options) (*StatusResult, error) {
	opts = opts.fillDefaults()
	unitPath := filepath.Join(opts.UnitDir, opts.UnitName)
	res := &StatusResult{UnitName: opts.UnitName, UnitPath: unitPath}

	if _, err := os.Stat(unitPath); errors.Is(err, os.ErrNotExist) {
		return res, nil
	} else if err != nil {
		return nil, fmt.Errorf("systemd: stat %q: %w", unitPath, err)
	}
	res.UnitPresent = true

	// is-active and is-enabled both exit non-zero for negative
	// answers but still print the state on stdout.
	out, _ := opts.Runner.Run(ctx, "systemctl", "is-active", opts.UnitName)
	res.ActiveState = strings.TrimSpace(string(out))
	res.Active = res.ActiveState == "active"

	out, _ = opts.Runner.Run(ctx, "systemctl", "is-enabled", opts.UnitName)
	res.EnabledState = strings.TrimSpace(string(out))
	res.Enabled = res.EnabledState == "enabled" || res.EnabledState == "alias" || res.EnabledState == "static" || res.EnabledState == "indirect"
	return res, nil
}

// atomicWriteFile is the temp+rename idiom — duplicated from
// internal/agent/bootstrap/install.go to keep this package
// dependency-free. Tiny helper, lifting it to a shared package
// would be over-engineering for one extra caller. Lift if a
// third use surfaces.
func atomicWriteFile(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(path), err)
	}
	tmp := path + ".tmp." + strconv.Itoa(os.Getpid()) + "." + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, body, mode); err != nil {
		return fmt.Errorf("write temp %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename temp into place: %w", err)
	}
	return nil
}
