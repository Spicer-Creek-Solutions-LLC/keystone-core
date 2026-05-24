// SPDX-License-Identifier: Apache-2.0

// Package mount implements the `mount` stdlib state module —
// managing a filesystem's /etc/fstab entry and its live mount state,
// per PROJECT-DETAILS §4.8 (Storage category). Inspection is via
// /proc/mounts; mounting/unmounting via mount(8)/umount(8).
//
// Declaration.Name is the mount point (an absolute path — fstab is
// keyed by mount point).
//
// State semantics:
//
//	mounted   — the fstab entry for the mount point matches the
//	            declaration (device / fstype / opts-as-set / dump /
//	            pass) AND a filesystem is currently mounted there.
//	            Apply upserts the fstab entry, creates the mount
//	            point dir when `mkmnt`, and mounts if not already.
//	present   — just the fstab entry matches the declaration (no
//	            requirement that it's currently mounted — for
//	            `noauto` mounts, or "configure now, mount later").
//	unmounted — nothing is mounted at the mount point. The fstab
//	            entry is left untouched (use `absent` to remove it).
//	absent    — nothing is mounted at the mount point AND there is
//	            no fstab entry (Apply unmounts then removes the
//	            line).
//
// The live mount's device is not re-verified against the declaration
// (the kernel resolves UUID=/LABEL= to a real device), so a live
// device change isn't detected — only the fstab entry is. If
// /etc/fstab is missing it is created on first write.
//
// v0.1 out of scope (v0.x candidates):
//   - Remount-on-change (`mount -o remount` when fstab opts change
//     for a mounted fs; reconcile a live device change).
//   - fstab `\040` escaping for whitespace in paths / options.
//   - findmnt-based inspection (richer than /proc/mounts);
//     `noauto` / `nofail` awareness for the `mounted` live check.
//   - swap-type fstab entries (the `swap` module's job).
//   - loop-device / encrypted (crypttab) coordination.
//   - `unmounted` with `persist: true` (also drop the fstab entry —
//     v1.0: use `absent`).
package mount

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// fstabPath is /etc/fstab; a package var for tests. Tests overriding
// it must not run in parallel.
var fstabPath = "/etc/fstab"

// New selects the platform's real Provider via auto-detection.
func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

type Module struct {
	provider Provider
}

func (m *Module) Name() string { return "mount" }

func (m *Module) ValidStates() []string {
	return []string{StateMounted, StatePresent, StateUnmounted, StateAbsent}
}

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a filesystem that should be mounted but isn't (or a
// stale fstab entry the system will act on at boot, or a mount
// declared absent but still present) is a data-availability concern
// — HIGH. A `present`-only fstab entry or a temporary `unmounted`
// state is config-level — MEDIUM. Operators override via `severity:`.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
	switch decl.State {
	case StateMounted, StateAbsent:
		return statemgmt.DriftSeverityHigh
	default:
		return statemgmt.DriftSeverityMedium
	}
}

func (m *Module) Check(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	matches, diff, err := m.check(ctx, p)
	if err != nil {
		return nil, err
	}
	if matches {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: diff}, nil
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	changed, diff, err := m.apply(ctx, p)
	if err != nil {
		return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}, err
	}
	if !changed {
		return &statemgmt.StateResult{Success: true, Changed: false, Comment: "already converged", Duration: time.Since(start)}, nil
	}
	return &statemgmt.StateResult{Success: true, Changed: true, Diff: diff, Comment: "applied", Duration: time.Since(start)}, nil
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

func (m *Module) check(ctx context.Context, p *params) (matches bool, diff string, err error) {
	content, err := readFstab()
	if err != nil {
		return false, "", err
	}
	switch p.State {
	case StateMounted:
		want := desiredEntry(p)
		e, found := findEntry(content, p.MountPoint)
		if !found {
			return false, fmt.Sprintf("no fstab entry for %s → add", p.MountPoint), nil
		}
		if !e.matchesDesired(want) {
			return false, fmt.Sprintf("fstab entry differs: %q → %q", e.render(), want.render()), nil
		}
		info, err := m.provider.Lookup(ctx, p.MountPoint)
		if err != nil {
			return false, "", err
		}
		if !info.Mounted {
			return false, fmt.Sprintf("%s not mounted → mount %s", p.MountPoint, p.Device), nil
		}
		return true, "", nil

	case StatePresent:
		want := desiredEntry(p)
		e, found := findEntry(content, p.MountPoint)
		if !found {
			return false, fmt.Sprintf("no fstab entry for %s → add", p.MountPoint), nil
		}
		if !e.matchesDesired(want) {
			return false, fmt.Sprintf("fstab entry differs: %q → %q", e.render(), want.render()), nil
		}
		return true, "", nil

	case StateUnmounted:
		info, err := m.provider.Lookup(ctx, p.MountPoint)
		if err != nil {
			return false, "", err
		}
		if info.Mounted {
			return false, fmt.Sprintf("%s is mounted; want unmounted", p.MountPoint), nil
		}
		return true, "", nil

	case StateAbsent:
		info, err := m.provider.Lookup(ctx, p.MountPoint)
		if err != nil {
			return false, "", err
		}
		_, found := findEntry(content, p.MountPoint)
		if !info.Mounted && !found {
			return true, "", nil
		}
		var parts []string
		if info.Mounted {
			parts = append(parts, "mounted")
		}
		if found {
			parts = append(parts, "in fstab")
		}
		return false, fmt.Sprintf("%s present (%s); want absent", p.MountPoint, strings.Join(parts, " + ")), nil
	}
	return false, "", fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) apply(ctx context.Context, p *params) (changed bool, diff string, err error) {
	content, err := readFstab()
	if err != nil {
		return false, "", err
	}
	switch p.State {
	case StateMounted:
		want := desiredEntry(p)
		e, found := findEntry(content, p.MountPoint)
		fstabChanged := false
		if !found || !e.matchesDesired(want) {
			if err := writeFstab(upsertEntry(content, want)); err != nil {
				return false, "", err
			}
			fstabChanged = true
		}
		info, err := m.provider.Lookup(ctx, p.MountPoint)
		if err != nil {
			return false, "", err
		}
		didMount := false
		if !info.Mounted {
			if p.Mkmnt {
				if err := os.MkdirAll(p.MountPoint, 0o755); err != nil { //nolint:gosec // operator-supplied mount point; 0755 is the conventional default for a mount point
					return false, "", fmt.Errorf("mkdir %s: %w", p.MountPoint, err)
				}
			}
			if err := m.provider.Mount(ctx, p.Device, p.MountPoint, p.FSType, p.Opts); err != nil {
				return false, "", fmt.Errorf("mount %s: %w", p.MountPoint, err)
			}
			didMount = true
		}
		switch {
		case fstabChanged && didMount:
			return true, fmt.Sprintf("updated fstab entry and mounted %s", p.MountPoint), nil
		case fstabChanged:
			return true, fmt.Sprintf("updated fstab entry for %s", p.MountPoint), nil
		case didMount:
			return true, fmt.Sprintf("mounted %s", p.MountPoint), nil
		default:
			return false, "", nil
		}

	case StatePresent:
		want := desiredEntry(p)
		e, found := findEntry(content, p.MountPoint)
		if found && e.matchesDesired(want) {
			return false, "", nil
		}
		if err := writeFstab(upsertEntry(content, want)); err != nil {
			return false, "", err
		}
		if found {
			return true, fmt.Sprintf("updated fstab entry for %s", p.MountPoint), nil
		}
		return true, fmt.Sprintf("added fstab entry for %s", p.MountPoint), nil

	case StateUnmounted:
		info, err := m.provider.Lookup(ctx, p.MountPoint)
		if err != nil {
			return false, "", err
		}
		if !info.Mounted {
			return false, "", nil
		}
		if err := m.provider.Unmount(ctx, p.MountPoint); err != nil {
			return false, "", fmt.Errorf("umount %s: %w", p.MountPoint, err)
		}
		return true, fmt.Sprintf("unmounted %s", p.MountPoint), nil

	case StateAbsent:
		info, err := m.provider.Lookup(ctx, p.MountPoint)
		if err != nil {
			return false, "", err
		}
		_, found := findEntry(content, p.MountPoint)
		if !info.Mounted && !found {
			return false, "", nil
		}
		if info.Mounted {
			if err := m.provider.Unmount(ctx, p.MountPoint); err != nil {
				return false, "", fmt.Errorf("umount %s: %w", p.MountPoint, err)
			}
		}
		if found {
			if err := writeFstab(removeEntry(content, p.MountPoint)); err != nil {
				return false, "", err
			}
		}
		switch {
		case info.Mounted && found:
			return true, fmt.Sprintf("unmounted %s and removed its fstab entry", p.MountPoint), nil
		case info.Mounted:
			return true, fmt.Sprintf("unmounted %s", p.MountPoint), nil
		default:
			return true, fmt.Sprintf("removed fstab entry for %s", p.MountPoint), nil
		}
	}
	return false, "", fmt.Errorf("unknown state %q", p.State)
}

// readFstab returns /etc/fstab's content; a missing file is ("", nil).
func readFstab() (string, error) {
	data, err := os.ReadFile(fstabPath) //nolint:gosec // fstabPath is /etc/fstab (overridable only in tests)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", fstabPath, err)
	}
	return string(data), nil
}

// writeFstab atomically writes /etc/fstab. An existing file's
// permission bits are preserved; a new file gets 0644.
func writeFstab(content string) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(fstabPath); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp := fstabPath + ".keystone.tmp"
	if err := os.WriteFile(tmp, []byte(content), mode); err != nil { //nolint:gosec // /etc/fstab is world-readable; mode mirrors the existing file or is 0644
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod tmp: %w", err)
	}
	if err := os.Rename(tmp, fstabPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s → %s: %w", tmp, fstabPath, err)
	}
	return nil
}
