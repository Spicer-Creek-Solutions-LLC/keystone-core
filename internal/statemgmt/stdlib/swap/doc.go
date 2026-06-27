// SPDX-License-Identifier: Apache-2.0

package swap

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the swap module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Storage",
		Summary: "Manages swap space — a swapfile or a swap partition/device — together " +
			"with its /etc/fstab entry and live swapon state. Idempotent: re-applying " +
			"an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "on", Desc: "The source is an active swap area and has a matching fstab entry. A missing swapfile is created (when `size` is set), `mkswap`'d, then `swapon`'d."},
			{Name: "present", Desc: "The fstab entry matches; live activation is not required (\"configure now, enable later\")."},
			{Name: "off", Desc: "The source is not an active swap area. The fstab entry is left untouched — use `absent` to remove it."},
			{Name: "absent", Desc: "The source is not active and has no fstab entry; a leftover swapfile (a regular-file source) is also removed (a device is left alone)."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "size", Type: "string", Desc: "Swapfile size with an explicit unit, e.g. `\"2G\"`, `\"512M\"`, `\"1024K\"` (1024-based). State `on` only, and only used to create a not-yet-existing swapfile."},
			{Name: "priority", Type: "int", Desc: "Swap priority, `-1`–`32767` (`-1` = swapon's default). Becomes the `swapon -p` flag and the fstab `pri=` option. Invalid with `off`/`absent`."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Create and activate a swapfile",
				Desc:  "A missing swapfile is created at the declared size, formatted, and enabled.",
				YAML: `swap:
  /swapfile:
    state: on
    size: "2G"
    priority: 10`,
			},
			{
				Title: "Enable a swap partition",
				YAML: `swap:
  /dev/sda2:
    state: on`,
			},
			{
				Title: "Configure now, enable later; deactivate elsewhere",
				Desc:  "`present` writes only the fstab entry; `off` deactivates without touching fstab.",
				YAML: `swap:
  /dev/sdb1:
    state: present
    priority: 5
  /swapfile:
    state: off`,
			},
		},
		Notes: []string{
			"Linux only: inspection via /proc/swaps; activation via mkswap(8)/swapon(8)/swapoff(8); a missing swapfile is created with dd(1). Other operating systems get a no-op provider.",
			"The declaration name is the swap source and must be an absolute path — a swapfile (`/swapfile`) or a block device (`/dev/sda2`). `UUID=`/`LABEL=` sources are not supported (planned).",
			"`size` governs only swapfile creation; it does not resize an existing swapfile.",
			"Out of scope: custom fstab swap options (`nofail`, `discard`), fallocate-based creation, btrfs (NOCOW) swapfiles, zram / dphys-swapfile.",
		},
	}
}
