// SPDX-License-Identifier: Apache-2.0

// Package lvm implements the `lvm` stdlib state module — manages
// one LVM object (a Physical Volume, a Volume Group, or a Logical
// Volume) per declaration, per PROJECT-DETAILS §4.8 (Storage
// category).
//
// Operations per declaration (exactly one):
//
//   - **PV** — `pv: <device>` → creates / removes a Physical Volume
//     on `<device>` via `pvcreate` / `pvremove`. Check via
//     `pvs --noheadings -o pv_name <device>`.
//
//   - **VG** — `vg: <name>` + `pvs: [<device>, …]` (for present) →
//     creates / removes a Volume Group via `vgcreate name pvs…` /
//     `vgremove -y name`. Check via
//     `vgs --noheadings -o vg_name <name>`. **The declared PV set is
//     reconciled** on an existing VG: PVs in `pvs:` but not in the live
//     VG are added (`vgextend`), and PVs in the live VG but not in
//     `pvs:` are removed (`vgreduce`). Paths are matched against LVM's
//     `pv_name` after resolving symlinks, so `/dev/disk/by-id/…` works.
//     No `-f`: `vgreduce` refuses to drop a PV that still holds LV
//     extents (operators `pvmove` data off first), and `vgextend`
//     assumes the device is already a PV (a separate `pv:` decl).
//
//   - **LV** — `lv: <name>` + `vg: <vgname>` + exactly one of
//     `size: <human>` (e.g. `10G`, `500M`, `1T`) or
//     `extents: <N%FREE|N%VG|N%PVS|N%ORIGIN>` → creates / removes a
//     Logical Volume via `lvcreate -y -n <lv> {-L size|-l extents}
//     <vg>` / `lvremove -y <vg>/<lv>`. Check via
//     `lvs --noheadings -o lv_name <vg>/<lv>`. A **size-based** LV is
//     also **grown** when the live size is below the declared size:
//     `lvextend -L <size> [--resizefs] <vg>/<lv>` (set `resize_fs:
//     true` to grow the contained filesystem too). Size is a minimum
//     ("at least" semantics) — a declared size ≤ the live size is
//     satisfied, so the module never shrinks (filesystem-dangerous)
//     and stays idempotent across LVM's extent rounding. `extents:`
//     LVs are create-only (the target depends on live free space).
//
// Declaration.Name is just a human label (the decl ID); the
// operation is identified by which of `pv` / `vg` / `lv` is set.
// States `present` / `absent`. DriftSeverity HIGH (LVM objects are
// data-bearing — wrong state is a real outage risk).
//
// Safety: this module relies on LVM's own refusal to clobber. v1.0
// never passes `-f` / `--force` to any of the underlying tools, so a
// `pvcreate` on a device with existing data will fail, a `vgremove`
// on a VG that still has LVs will fail, and an `lvremove` on a
// mounted LV will fail. Operators must clear blockers (unmount,
// remove children) themselves before the relevant Apply.
//
// v0.1 out of scope (v0.x candidates):
//   - `vgchange` allocation policy, missing-PV cleanup, and automatic
//     `pvmove` of data off a PV being removed (the PV-set reconcile
//     itself landed; it relies on LVM refusing an unsafe `vgreduce`).
//   - LV **shrink** (`lvreduce`) — fundamentally dangerous (the
//     filesystem must support shrink first); resize is grow-only.
//   - `extents:`-based LV resize (the percentage target depends on
//     live free space).
//   - LV metadata (tags, allocation policy, stripe / mirror /
//     RAID levels, thin pools, cache, snapshots, naming policy).
//   - PV metadata (`--metadatasize`, allocation tags, restore).
//   - Filesystem creation on an LV — use the `disk` module on the
//     resulting device, or `cmd` with `mkfs.X` for less-common
//     filesystems.
package lvm

import (
	"context"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// New selects the platform's real Provider via auto-detection.
func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

type Module struct {
	provider Provider
}

func (m *Module) Name() string { return "lvm" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: LVM objects are data-bearing; wrong state is a real
// outage risk. HIGH always; MEDIUM nil.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
	return statemgmt.DriftSeverityHigh
}

func (m *Module) Check(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	has, err := m.has(ctx, p)
	if err != nil {
		return nil, err
	}
	loc := p.locator()
	switch p.State {
	case StatePresent:
		if has {
			// An existing object may still need reconciling: a size-based
			// LV that is below its declared size grows; a VG whose live PV
			// set differs from the declared `pvs:` extends / reduces.
			drift, diff, err := m.existingDrift(ctx, p)
			if err != nil {
				return nil, err
			}
			if drift {
				return &statemgmt.ModuleCheckResult{Matches: false, Diff: diff}, nil
			}
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s not present → create", loc)}, nil
	case StateAbsent:
		if !has {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s present; want absent", loc)}, nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	has, err := m.has(ctx, p)
	if err != nil {
		return failure(start), err
	}
	loc := p.locator()

	switch p.State {
	case StatePresent:
		if has {
			return m.reconcileExisting(ctx, p, start)
		}
		if err := m.create(ctx, p); err != nil {
			return failure(start), fmt.Errorf("create %s: %w", loc, err)
		}
		return ok(start, true, fmt.Sprintf("created %s", loc), "applied"), nil
	case StateAbsent:
		if !has {
			return ok(start, false, "", "already converged"), nil
		}
		if err := m.remove(ctx, p); err != nil {
			return failure(start), fmt.Errorf("remove %s: %w", loc, err)
		}
		return ok(start, true, fmt.Sprintf("removed %s", loc), "applied"), nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

func (m *Module) parsed(decl *statemgmt.Declaration) (*params, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// lvSizeDrift reports whether an existing LV is smaller than the
// declared size and should grow. It applies only to a size-based LV op
// ("at least" semantics: a declared size ≤ the live size is satisfied —
// shrink is never performed, which also keeps the check idempotent
// across LVM's extent rounding). PV / VG ops, and extents-based LVs
// (whose target depends on live free space), never report size drift.
func (m *Module) lvSizeDrift(ctx context.Context, p *params) (bool, string, error) {
	if p.Op != OpLV || p.Size == "" {
		return false, "", nil
	}
	want, err := sizeToBytes(p.Size)
	if err != nil {
		return false, "", err
	}
	live, err := m.provider.GetLVSize(ctx, p.VGName, p.LVName)
	if err != nil {
		return false, "", err
	}
	if live >= want {
		return false, "", nil
	}
	return true, fmt.Sprintf("LV %s/%s: %d → at least %d bytes (grow to %s)", p.VGName, p.LVName, live, want, p.Size), nil
}

// existingDrift reports whether an already-present object differs from
// its declaration in a way the module reconciles in place: a size-based
// LV below its declared size, or a VG whose PV set differs from the
// declared `pvs:`. A PV has no reconcilable attributes.
func (m *Module) existingDrift(ctx context.Context, p *params) (bool, string, error) {
	switch p.Op {
	case OpLV:
		return m.lvSizeDrift(ctx, p)
	case OpVG:
		drift, diff, _, _, err := m.vgPVDrift(ctx, p)
		return drift, diff, err
	}
	return false, "", nil
}

// vgPVDrift compares an existing VG's live PV set against the declared
// `pvs:` and returns the PVs to add (declared − live) and remove (live −
// declared). Both sides are canonicalised (symlinks followed) before
// diffing so a declared /dev/disk/by-id/… path matches LVM's pv_name;
// toAdd carries the declared paths (for vgextend) and toRemove the live
// pv_names (for vgreduce).
func (m *Module) vgPVDrift(ctx context.Context, p *params) (drift bool, diff string, toAdd, toRemove []string, err error) {
	if p.Op != OpVG || len(p.VGPVs) == 0 {
		return false, "", nil, nil, nil
	}
	live, err := m.provider.GetVGPVs(ctx, p.VGName)
	if err != nil {
		return false, "", nil, nil, err
	}
	wantRefs, err := m.canonRefs(ctx, p.VGPVs)
	if err != nil {
		return false, "", nil, nil, err
	}
	liveRefs, err := m.canonRefs(ctx, live)
	if err != nil {
		return false, "", nil, nil, err
	}
	toAdd, toRemove = diffPVSets(wantRefs, liveRefs)
	if len(toAdd) == 0 && len(toRemove) == 0 {
		return false, "", nil, nil, nil
	}
	return true, fmt.Sprintf("VG %s PV set: add %v, remove %v", p.VGName, toAdd, toRemove), toAdd, toRemove, nil
}

// pvRef pairs a device path with its canonical (symlink-resolved) form;
// the diff matches on canon and returns orig.
type pvRef struct{ orig, canon string }

func (m *Module) canonRefs(ctx context.Context, paths []string) ([]pvRef, error) {
	out := make([]pvRef, 0, len(paths))
	for _, path := range paths {
		c, err := m.provider.Canonicalize(ctx, path)
		if err != nil {
			return nil, err
		}
		out = append(out, pvRef{orig: path, canon: c})
	}
	return out, nil
}

// diffPVSets returns the orig paths present in want but not live
// (toAdd), and present in live but not want (toRemove), matching on the
// canonical form and de-duplicating by it while preserving input order.
func diffPVSets(want, live []pvRef) (toAdd, toRemove []string) {
	wantCanon := make(map[string]bool, len(want))
	for _, w := range want {
		wantCanon[w.canon] = true
	}
	liveCanon := make(map[string]bool, len(live))
	for _, l := range live {
		liveCanon[l.canon] = true
	}
	seen := map[string]bool{}
	for _, w := range want {
		if !liveCanon[w.canon] && !seen[w.canon] {
			toAdd = append(toAdd, w.orig)
			seen[w.canon] = true
		}
	}
	seen = map[string]bool{}
	for _, l := range live {
		if !wantCanon[l.canon] && !seen[l.canon] {
			toRemove = append(toRemove, l.orig)
			seen[l.canon] = true
		}
	}
	return toAdd, toRemove
}

// reconcileExisting reconciles an already-present object in place: grow
// a size-based LV, extend/reduce a VG's PV set, or no-op a PV.
func (m *Module) reconcileExisting(ctx context.Context, p *params, start time.Time) (*statemgmt.StateResult, error) {
	loc := p.locator()
	switch p.Op {
	case OpLV:
		drift, diff, err := m.lvSizeDrift(ctx, p)
		if err != nil {
			return failure(start), err
		}
		if !drift {
			return ok(start, false, "", "already converged"), nil
		}
		if err := m.provider.ExtendLV(ctx, p.VGName, p.LVName, p.Size, p.ResizeFS); err != nil {
			return failure(start), fmt.Errorf("resize %s: %w", loc, err)
		}
		return ok(start, true, diff, "resized"), nil
	case OpVG:
		drift, diff, toAdd, toRemove, err := m.vgPVDrift(ctx, p)
		if err != nil {
			return failure(start), err
		}
		if !drift {
			return ok(start, false, "", "already converged"), nil
		}
		// Extend before reduce so the VG's capacity never dips mid-apply.
		if len(toAdd) > 0 {
			if err := m.provider.ExtendVG(ctx, p.VGName, toAdd); err != nil {
				return failure(start), fmt.Errorf("extend %s: %w", loc, err)
			}
		}
		if len(toRemove) > 0 {
			if err := m.provider.ReduceVG(ctx, p.VGName, toRemove); err != nil {
				return failure(start), fmt.Errorf("reduce %s: %w", loc, err)
			}
		}
		return ok(start, true, diff, "reconciled"), nil
	}
	// PV: no reconcilable attributes.
	return ok(start, false, "", "already converged"), nil
}

// has dispatches the per-op existence query.
func (m *Module) has(ctx context.Context, p *params) (bool, error) {
	switch p.Op {
	case OpPV:
		return m.provider.HasPV(ctx, p.Device)
	case OpVG:
		return m.provider.HasVG(ctx, p.VGName)
	case OpLV:
		return m.provider.HasLV(ctx, p.VGName, p.LVName)
	}
	return false, fmt.Errorf("unknown op %v", p.Op)
}

func (m *Module) create(ctx context.Context, p *params) error {
	switch p.Op {
	case OpPV:
		return m.provider.CreatePV(ctx, p.Device)
	case OpVG:
		return m.provider.CreateVG(ctx, p.VGName, p.VGPVs)
	case OpLV:
		return m.provider.CreateLV(ctx, p.VGName, p.LVName, p.Size, p.Extents)
	}
	return fmt.Errorf("unknown op %v", p.Op)
}

func (m *Module) remove(ctx context.Context, p *params) error {
	switch p.Op {
	case OpPV:
		return m.provider.RemovePV(ctx, p.Device)
	case OpVG:
		return m.provider.RemoveVG(ctx, p.VGName)
	case OpLV:
		return m.provider.RemoveLV(ctx, p.VGName, p.LVName)
	}
	return fmt.Errorf("unknown op %v", p.Op)
}

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}
