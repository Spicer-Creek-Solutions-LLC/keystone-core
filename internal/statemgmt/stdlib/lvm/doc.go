// SPDX-License-Identifier: Apache-2.0

package lvm

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the lvm module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Storage",
		Summary: "Manages one LVM object per declaration — a Physical Volume (`pv`), a " +
			"Volume Group (`vg`), or a Logical Volume (`lv`). Idempotent: re-applying an " +
			"unchanged declaration reports no change, and a size-based LV or a VG's PV set " +
			"is reconciled in place.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The object exists. An existing size-based LV below its declared size is grown; an existing VG whose live PV set differs from `pvs:` is extended/reduced."},
			{Name: "absent", Desc: "The object does not exist; an existing PV, VG, or LV is removed (LVM refuses to remove one that still holds data/children)."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "pv", Type: "string", Desc: "Device path (e.g. `/dev/sdb1`) for a Physical Volume operation. Mutually exclusive with `vg`/`lv`."},
			{Name: "vg", Type: "string", Desc: "Volume Group name. On its own it is a VG operation; combined with `lv` it names the parent VG of a Logical Volume."},
			{Name: "pvs", Type: "list", Desc: "Member device paths for a VG (required when a VG is `present`). Reconciled on an existing VG: devices added/removed to match."},
			{Name: "lv", Type: "string", Desc: "Logical Volume name for an LV operation. Requires `vg` (the parent volume group)."},
			{Name: "size", Type: "string", Desc: "LV size, e.g. `10G`, `500M`, `1T` (no suffix = MiB). Grow-only \"at least\" semantics. Mutually exclusive with `extents`."},
			{Name: "extents", Type: "string", Desc: "LV size as a percentage, e.g. `100%FREE`, `50%VG`. Create-only. Mutually exclusive with `size`."},
			{Name: "resize_fs", Type: "bool", Default: "false", Desc: "Pass `--resizefs` to `lvextend` so the contained filesystem grows with a size-based LV."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Build a stack: PV, then VG, then LV",
				Desc:  "The `require` requisites order each object after its dependency.",
				YAML: `lvm:
  pv-sdb1:
    state: present
    pv: /dev/sdb1
  vg-data:
    state: present
    vg: data
    pvs:
      - /dev/sdb1
    require:
      - lvm: pv-sdb1
  lv-home:
    state: present
    lv: home
    vg: data
    size: 10G
    require:
      - lvm: vg-data`,
			},
			{
				Title: "Grow a logical volume and its filesystem",
				YAML: `lvm:
  lv-home-grow:
    state: present
    lv: home
    vg: data
    size: 20G
    resize_fs: true`,
			},
			{
				Title: "Consume all free space, and remove an old PV",
				YAML: `lvm:
  lv-scratch:
    state: present
    lv: scratch
    vg: data
    extents: 100%FREE
  pv-old:
    state: absent
    pv: /dev/sdc1`,
			},
		},
		Notes: []string{
			"Linux only; other operating systems get a no-op provider that reports the LVM tools as unavailable.",
			"Exactly one of `pv` / `vg` / `lv` must be set per declaration; the operation is implied by which one.",
			"A VG that is `present` requires `pvs:` (at least one device); the live PV set is reconciled (add via `vgextend`, remove via `vgreduce`).",
			"`size` and `extents` are mutually exclusive on an LV. `size` is grow-only (never shrinks); `extents` LVs are create-only.",
			"DriftSeverity is HIGH — LVM objects are data-bearing. The module never passes `-f`/`--force`, so LVM refuses to clobber existing data, non-empty VGs, or mounted LVs; operators must clear blockers first.",
			"Out of scope (planned, #23): LV shrink, extents-based resize, thin/cache/snapshot, RAID, PV metadata, and allocation policy. Use the `disk` module to create a filesystem on the resulting device.",
		},
	}
}
