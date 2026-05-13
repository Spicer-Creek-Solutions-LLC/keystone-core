// Package disk implements the `disk` stdlib state module — manages
// one block device's filesystem signature, per PROJECT-DETAILS §4.8
// (Storage category).
//
// Single operation per declaration: ensure `device:` (an absolute
// `/dev/` path) has the filesystem signature named by `fstype:`
// (states `present`) or has no signature at all (state `absent`).
// v1.0 does not manage partitions, labels, UUIDs, encryption, or
// resize — see the ROADMAP entry.
//
// Idempotency: Check runs `blkid -o value -s TYPE <device>` and
// compares to the declared fstype. Apply only invokes
// `mkfs.<fstype>` when the device has no fstype or — with explicit
// `force: true` — a *different* one. `wipefs -a <device>` runs only
// for state `absent` AND with `force: true`. The `force` gate is the
// data-loss safety: in v1.0, every Apply that would overwrite a
// pre-existing filesystem requires the operator to opt in by name.
//
// Supported fstypes (curated; expansion is V1X):
//
//	ext2, ext3, ext4, xfs, btrfs, f2fs, vfat, exfat, swap
//
// Each maps to its own mkfs-family binary (`mkfs.ext4`, `mkfs.xfs`,
// …, `mkswap` for swap). The binary must be on PATH at apply time;
// missing → `ErrNoMkfs` with the fstype.
//
// `mkfs_options:` is a list of strings passed verbatim to the
// chosen `mkfs.X` (e.g. `-L mylabel`, `-F`, `-f`, `-N inodes`).
// Each element is charset-validated (no shell metacharacters,
// whitespace, or control characters).
//
// v0.1 out of scope (v0.x candidates):
//   - Partition creation / removal / label / flags via parted /
//     sgdisk (GPT or MBR). Partitioning is destructive enough that
//     it deserves its own module.
//   - Filesystem **resize** (`resize2fs`, `xfs_growfs`,
//     `btrfs filesystem resize`).
//   - Filesystem **label** and **UUID** management (without
//     re-formatting): `tune2fs -L`, `xfs_admin -L`, `swaplabel -L`,
//     `tune2fs -U`, `xfs_admin -U`, etc.
//   - **Encryption** (LUKS / cryptsetup), key management, opening /
//     closing volumes.
//   - **Re-format on mismatch** without explicit `force: true` —
//     operators who want unconditional re-format can pass `force:
//     true`, but a per-decl policy / dry-run preview is V1X.
//   - Additional fstypes: ntfs (`mkfs.ntfs` via ntfs-3g), zfs, bcachefs,
//     reiserfs (defunct), jfs (legacy), tmpfs (it's not a real fs in
//     the disk sense — use `mount`).
package disk

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

func (m *Module) Name() string { return "disk" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a filesystem signature mismatch on a managed
// device is a data-bearing concern. HIGH always; MEDIUM nil.
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
	current, err := m.provider.GetFilesystem(ctx, p.Device)
	if err != nil {
		return nil, err
	}
	switch p.State {
	case StatePresent:
		if current == p.Fstype {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s: %s → %s", p.Device, displayFS(current), p.Fstype)}, nil
	case StateAbsent:
		if current == "" {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s: %s → <none>", p.Device, current)}, nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	current, err := m.provider.GetFilesystem(ctx, p.Device)
	if err != nil {
		return failure(start), err
	}

	switch p.State {
	case StatePresent:
		if current == p.Fstype {
			return ok(start, false, "", "already converged"), nil
		}
		if current != "" && !p.Force {
			return failure(start), fmt.Errorf("%s has filesystem %q; want %q — re-formatting would destroy data; set `force: true` to proceed", p.Device, current, p.Fstype)
		}
		if err := m.provider.MakeFilesystem(ctx, p.Device, p.Fstype, p.MkfsOptions); err != nil {
			return failure(start), fmt.Errorf("mkfs.%s %s: %w", p.Fstype, p.Device, err)
		}
		return ok(start, true, fmt.Sprintf("%s: %s → %s", p.Device, displayFS(current), p.Fstype), "applied"), nil
	case StateAbsent:
		if current == "" {
			return ok(start, false, "", "already converged"), nil
		}
		if !p.Force {
			return failure(start), fmt.Errorf("%s has filesystem %q; wiping would destroy data; set `force: true` to proceed", p.Device, current)
		}
		if err := m.provider.WipeFilesystem(ctx, p.Device); err != nil {
			return failure(start), fmt.Errorf("wipefs %s: %w", p.Device, err)
		}
		return ok(start, true, fmt.Sprintf("%s: %s → <none>", p.Device, current), "applied"), nil
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

func displayFS(s string) string {
	if s == "" {
		return "<none>"
	}
	return s
}

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}
