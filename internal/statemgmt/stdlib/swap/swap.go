// Package swap implements the `swap` stdlib state module — managing
// swap space (a swapfile or a swap partition/device), its /etc/fstab
// entry, and its live swapon state, per PROJECT-DETAILS §4.8 (Storage
// category). Inspection is via /proc/swaps; activation via
// mkswap(8)/swapon(8)/swapoff(8); a not-yet-existing swapfile is
// created with dd(1).
//
// Declaration.Name is the swap source — a swapfile path
// ("/swapfile") or a block-device path ("/dev/sda2"). UUID=/LABEL=
// swap sources are not supported in v1.0.
//
// State semantics:
//
//	on      — the source is an active swap area AND there is a
//	          matching fstab entry. Apply upserts the fstab entry;
//	          if not active it creates the swapfile (when Name is a
//	          missing regular-file path and `size:` is set), runs
//	          mkswap, then swapon.
//	present — just the fstab entry matches (no live-activation
//	          requirement; "configure now, enable later").
//	off     — the source is not active (the fstab entry is left
//	          untouched — use `absent` to remove it).
//	absent  — the source is not active AND there is no fstab entry;
//	          when Name is a regular file (a swapfile) it is also
//	          removed (a device is left alone).
//
// v1.0 out of scope (V1X candidates):
//   - UUID=/LABEL= swap sources.
//   - Enforcing/changing the size of an *existing* swapfile (`size`
//     only governs creation).
//   - fallocate-based fast swapfile creation (v1.0 uses dd).
//   - Custom fstab options for swap (`nofail`, `discard`, …);
//     `mkswap -L <label>` / `-f`.
//   - btrfs (NOCOW) swapfiles; not rotating a partition UUID that
//     mkswap would change when re-activating; zram / dphys-swapfile.
package swap

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

func (m *Module) Name() string { return "swap" }

func (m *Module) ValidStates() []string {
	return []string{StateOn, StatePresent, StateOff, StateAbsent}
}

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a swap area that should be active but isn't (or a
// stale fstab entry the system acts on at boot, or a leftover
// swapfile that should be gone) is a memory-pressure / cleanliness
// concern — HIGH. A `present`-only fstab entry or a temporary `off`
// state is config-level — MEDIUM. Operators override via `severity:`.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
	switch decl.State {
	case StateOn, StateAbsent:
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
	case StateOn:
		opts, found := findSwapEntry(content, p.Source)
		if !found {
			return false, fmt.Sprintf("no fstab swap entry for %s → add", p.Source), nil
		}
		if !optsSetEqual(opts, desiredOpts(p)) {
			return false, fmt.Sprintf("fstab swap opts differ: %q → %q", opts, desiredOpts(p)), nil
		}
		info, err := m.provider.Lookup(ctx, p.Source)
		if err != nil {
			return false, "", err
		}
		if !info.Active {
			return false, fmt.Sprintf("%s not active → swapon", p.Source), nil
		}
		return true, "", nil

	case StatePresent:
		opts, found := findSwapEntry(content, p.Source)
		if !found {
			return false, fmt.Sprintf("no fstab swap entry for %s → add", p.Source), nil
		}
		if !optsSetEqual(opts, desiredOpts(p)) {
			return false, fmt.Sprintf("fstab swap opts differ: %q → %q", opts, desiredOpts(p)), nil
		}
		return true, "", nil

	case StateOff:
		info, err := m.provider.Lookup(ctx, p.Source)
		if err != nil {
			return false, "", err
		}
		if info.Active {
			return false, fmt.Sprintf("%s is active; want off", p.Source), nil
		}
		return true, "", nil

	case StateAbsent:
		info, err := m.provider.Lookup(ctx, p.Source)
		if err != nil {
			return false, "", err
		}
		_, inFstab := findSwapEntry(content, p.Source)
		leftoverFile := isRegularFile(p.Source)
		if !info.Active && !inFstab && !leftoverFile {
			return true, "", nil
		}
		var parts []string
		if info.Active {
			parts = append(parts, "active")
		}
		if inFstab {
			parts = append(parts, "in fstab")
		}
		if leftoverFile {
			parts = append(parts, "swapfile present")
		}
		return false, fmt.Sprintf("%s present (%s); want absent", p.Source, strings.Join(parts, " + ")), nil
	}
	return false, "", fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) apply(ctx context.Context, p *params) (changed bool, diff string, err error) {
	content, err := readFstab()
	if err != nil {
		return false, "", err
	}
	switch p.State {
	case StateOn:
		opts, found := findSwapEntry(content, p.Source)
		fstabChanged := false
		if !found || !optsSetEqual(opts, desiredOpts(p)) {
			if err := writeFstab(upsertSwapEntry(content, p)); err != nil {
				return false, "", err
			}
			fstabChanged = true
		}
		info, err := m.provider.Lookup(ctx, p.Source)
		if err != nil {
			return false, "", err
		}
		activated := false
		if !info.Active {
			if err := m.ensureSwapArea(ctx, p); err != nil {
				return false, "", err
			}
			if err := m.provider.SwapOn(ctx, p.Source, p.Priority); err != nil {
				return false, "", fmt.Errorf("swapon %s: %w", p.Source, err)
			}
			activated = true
		}
		switch {
		case fstabChanged && activated:
			return true, fmt.Sprintf("added fstab entry and activated swap %s", p.Source), nil
		case fstabChanged:
			return true, fmt.Sprintf("updated fstab swap entry for %s", p.Source), nil
		case activated:
			return true, fmt.Sprintf("activated swap %s", p.Source), nil
		default:
			return false, "", nil
		}

	case StatePresent:
		opts, found := findSwapEntry(content, p.Source)
		if found && optsSetEqual(opts, desiredOpts(p)) {
			return false, "", nil
		}
		if err := writeFstab(upsertSwapEntry(content, p)); err != nil {
			return false, "", err
		}
		if found {
			return true, fmt.Sprintf("updated fstab swap entry for %s", p.Source), nil
		}
		return true, fmt.Sprintf("added fstab swap entry for %s", p.Source), nil

	case StateOff:
		info, err := m.provider.Lookup(ctx, p.Source)
		if err != nil {
			return false, "", err
		}
		if !info.Active {
			return false, "", nil
		}
		if err := m.provider.SwapOff(ctx, p.Source); err != nil {
			return false, "", fmt.Errorf("swapoff %s: %w", p.Source, err)
		}
		return true, fmt.Sprintf("deactivated swap %s", p.Source), nil

	case StateAbsent:
		info, err := m.provider.Lookup(ctx, p.Source)
		if err != nil {
			return false, "", err
		}
		_, inFstab := findSwapEntry(content, p.Source)
		leftoverFile := isRegularFile(p.Source)
		if !info.Active && !inFstab && !leftoverFile {
			return false, "", nil
		}
		if info.Active {
			if err := m.provider.SwapOff(ctx, p.Source); err != nil {
				return false, "", fmt.Errorf("swapoff %s: %w", p.Source, err)
			}
		}
		if inFstab {
			if err := writeFstab(removeSwapEntry(content, p.Source)); err != nil {
				return false, "", err
			}
		}
		if leftoverFile {
			if err := os.Remove(p.Source); err != nil {
				return false, "", fmt.Errorf("remove swapfile %s: %w", p.Source, err)
			}
		}
		var did []string
		if info.Active {
			did = append(did, "deactivated")
		}
		if inFstab {
			did = append(did, "removed fstab entry")
		}
		if leftoverFile {
			did = append(did, "removed swapfile")
		}
		return true, fmt.Sprintf("%s for %s", strings.Join(did, " + "), p.Source), nil
	}
	return false, "", fmt.Errorf("unknown state %q", p.State)
}

// ensureSwapArea makes sure p.Source is a swap area ready to swapon:
// when it is a regular-file path that doesn't exist, the swapfile is
// created (size required); then mkswap is run (which also covers a
// pre-existing not-active area).
func (m *Module) ensureSwapArea(ctx context.Context, p *params) error {
	switch fi, statErr := os.Lstat(p.Source); {
	case errors.Is(statErr, fs.ErrNotExist):
		if !p.HasSize {
			return fmt.Errorf("%w: %s", ErrSwapfileSizeRequired, p.Source)
		}
		if err := m.provider.CreateSwapfile(ctx, p.Source, p.SizeBytes); err != nil {
			return err
		}
	case statErr != nil:
		return fmt.Errorf("stat %s: %w", p.Source, statErr)
	case !fi.Mode().IsRegular() && fi.Mode()&os.ModeDevice == 0:
		// not a regular file and not a device — refuse rather than
		// mkswap something unexpected.
		return fmt.Errorf("swap source %s is neither a regular file nor a device", p.Source)
	}
	if err := m.provider.MakeSwap(ctx, p.Source); err != nil {
		return fmt.Errorf("mkswap %s: %w", p.Source, err)
	}
	return nil
}

func isRegularFile(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode().IsRegular()
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
