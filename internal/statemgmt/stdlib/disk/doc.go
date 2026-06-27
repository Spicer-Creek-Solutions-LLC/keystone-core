// SPDX-License-Identifier: Apache-2.0

package disk

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the disk module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Storage",
		Summary: "Manages the filesystem signature on a single block device: ensures a " +
			"`/dev/` path carries the declared `fstype` (formatting it with `mkfs.<fstype>`) " +
			"or has no signature at all. Idempotent: re-applying an unchanged declaration " +
			"reports no change, and overwriting any pre-existing filesystem requires explicit " +
			"`force: true`.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The device carries the declared `fstype`; it is formatted with `mkfs.<fstype>` only when blank, or — with `force: true` — when it holds a different filesystem. With `resize_fs: true`, an existing matching filesystem is grown to fill the device."},
			{Name: "absent", Desc: "The device has no filesystem signature; an existing one is removed with `wipefs -a`, which requires `force: true`."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "device", Type: "string", Required: true, Desc: "Absolute `/dev/` path of the block device to manage."},
			{Name: "fstype", Type: "string", Desc: "Filesystem to create; required for state `present`. Catalog: `ext2`, `ext3`, `ext4`, `xfs`, `btrfs`, `f2fs`, `vfat`, `exfat`, `swap`."},
			{Name: "mkfs_options", Type: "list", Desc: "Flags passed verbatim to `mkfs.<fstype>`, e.g. `[\"-L\", \"data\"]`. Each element is charset-validated (no whitespace or shell metacharacters). State `present` only."},
			{Name: "force", Type: "bool", Default: "false", Desc: "Opt in to data-destroying operations: re-format a device that holds a different filesystem (`present`), or wipe an existing signature (`absent`)."},
			{Name: "resize_fs", Type: "bool", Default: "false", Desc: "Grow an existing matching filesystem to fill the device (ext2/3/4, xfs, btrfs, f2fs). State `present` only."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Format a device ext4",
				YAML: `disk:
  /dev/sdb1:
    state: present
    device: /dev/sdb1
    fstype: ext4
    mkfs_options:
      - -L
      - data`,
			},
			{
				Title: "Grow a filesystem after the device expanded",
				Desc:  "After the underlying device grows (e.g. an `lvextend`), grow the fs to fill it.",
				YAML: `disk:
  /dev/vg0/data:
    state: present
    device: /dev/vg0/data
    fstype: xfs
    resize_fs: true`,
			},
			{
				Title: "Wipe a device's filesystem signature",
				Desc:  "`force: true` is required for any operation that destroys data.",
				YAML: `disk:
  /dev/sdb2:
    state: absent
    device: /dev/sdb2
    force: true`,
			},
		},
		Notes: []string{
			"Linux-only: the full provider runs `blkid`, `mkfs.*`, `wipefs`, and the per-fstype resize tools; other operating systems get a no-op provider.",
			"The matching `mkfs.<fstype>` binary (or `mkswap` for swap) must be on PATH at apply time.",
			"Every Apply that would overwrite or wipe a pre-existing filesystem requires `force: true` — the data-loss safety gate.",
			"`resize_fs` is grow-only and per-fstype: ext is device-based, xfs and btrfs require the device be mounted, f2fs requires it be unmounted.",
			"Out of scope (planned, #24): partition creation/removal, filesystem label/UUID management, encryption (LUKS), and a broader fstype catalog.",
		},
	}
}
