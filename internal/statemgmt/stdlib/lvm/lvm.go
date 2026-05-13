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
//     `vgs --noheadings -o vg_name <name>`. **The PV set is
//     immutable post-creation in v1.0** — extending or reducing an
//     existing VG is V1X; v1.0 only verifies the VG exists by name.
//
//   - **LV** — `lv: <name>` + `vg: <vgname>` + exactly one of
//     `size: <human>` (e.g. `10G`, `500M`, `1T`) or
//     `extents: <N%FREE|N%VG|N%PVS|N%ORIGIN>` → creates / removes a
//     Logical Volume via `lvcreate -y -n <lv> {-L size|-l extents}
//     <vg>` / `lvremove -y <vg>/<lv>`. Check via
//     `lvs --noheadings -o lv_name <vg>/<lv>`. **Existing-LV size
//     mismatch is not reconciled in v1.0** — extending / shrinking
//     an existing LV is V1X; v1.0 only verifies the LV exists by
//     name.
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
// v1.0 out of scope (V1X candidates):
//   - Existing-VG PV-set management (`vgextend` / `vgreduce`,
//     `vgchange` allocation policy, missing-PV cleanup).
//   - Existing-LV resize (`lvextend` / `lvreduce` /
//     `lvresize --resizefs`); shrink is fundamentally dangerous
//     (filesystem must support it).
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
			return ok(start, false, "", "already converged"), nil
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
