// SPDX-License-Identifier: Apache-2.0

package mount

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the mount module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Storage",
		Summary: "Manages a filesystem's `/etc/fstab` entry and its live mount state. " +
			"The declaration name is the mount point. Idempotent: re-applying an " +
			"unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "mounted", Desc: "The fstab entry matches the declaration (device, fstype, opts-as-set, dump, pass) and a filesystem is currently mounted at the mount point."},
			{Name: "present", Desc: "The fstab entry matches the declaration, with no requirement that it is currently mounted (for `noauto` mounts, or configure-now-mount-later)."},
			{Name: "unmounted", Desc: "Nothing is mounted at the mount point. The fstab entry is left untouched (use `absent` to remove it)."},
			{Name: "absent", Desc: "Nothing is mounted at the mount point and there is no fstab entry; Apply unmounts then removes the line."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "device", Type: "string", Desc: "Source device for the fstab entry, e.g. `/dev/sdb1`, `UUID=...`, `LABEL=...`. Required for `mounted`/`present`; rejected on `unmounted`/`absent`."},
			{Name: "fstype", Type: "string", Desc: "Filesystem type, e.g. `ext4`, `xfs`, `tmpfs`. Required for `mounted`/`present`; rejected on `unmounted`/`absent`."},
			{Name: "opts", Type: "string", Default: "defaults", Desc: "Comma-separated mount options. Compared as a set, so option order is insignificant."},
			{Name: "dump", Type: "int", Default: "0", Desc: "fstab dump field (the fifth column)."},
			{Name: "pass", Type: "int", Default: "0", Desc: "fstab fsck pass field (the sixth column)."},
			{Name: "mkmnt", Type: "bool", Default: "true", Desc: "Create the mount point directory before mounting (state `mounted`)."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Mount a data volume and persist it in fstab",
				YAML: `mount:
  /data:
    state: mounted
    device: /dev/sdb1
    fstype: ext4
    opts: rw,noatime`,
			},
			{
				Title: "Configure an fstab entry without mounting now",
				Desc:  "A `noauto` entry: written to fstab, not mounted until later.",
				YAML: `mount:
  /backups:
    state: present
    device: UUID=xyz
    fstype: xfs
    opts: noauto`,
			},
			{
				Title: "Unmount and remove an fstab entry",
				YAML: `mount:
  /data:
    state: absent`,
			},
		},
		Notes: []string{
			"Linux only: inspection via `/proc/mounts`, mounting/unmounting via mount(8)/umount(8).",
			"`device`/`fstype`/`opts`/`dump`/`pass` describe the fstab entry and may only appear on `mounted`/`present`; an `unmounted`/`absent` declaration that carries them is rejected.",
			"The live mount's device is not re-verified against the declaration (the kernel resolves `UUID=`/`LABEL=` to a real device), so a live device change is not detected — only the fstab entry is.",
			"Not yet supported (planned, #111): remount-on-change, crypttab/encrypted coordination, and swap-type entries (use the `swap` module).",
		},
	}
}
