// SPDX-License-Identifier: Apache-2.0

package kmod

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the kernel_module module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "System config",
		Summary: "Manages a Linux kernel module: ensures it is loaded (or unloaded) and, " +
			"by default, that a keystone-managed `/etc/modules-load.d` entry persists the " +
			"choice across reboots. Idempotent: re-applying an unchanged declaration reports " +
			"no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The named module is loaded; with `persist: true` (the default) a keystone-managed `/etc/modules-load.d` entry ensures it loads at boot."},
			{Name: "absent", Desc: "The named module is not loaded; with `persist: true` the keystone-managed modules-load entry is removed so a reboot does not bring it back."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "persist", Type: "bool", Default: "true", Desc: "Whether to manage a boot-load entry under `/etc/modules-load.d`. When `false`, only the live loaded/unloaded state is reconciled."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Load a module and persist it across reboots",
				Desc:  "The declaration name is the module name; dashes and underscores are both accepted.",
				YAML: `kernel_module:
  br_netfilter:
    state: present`,
			},
			{
				Title: "Load for the current boot only",
				Desc:  "With persist disabled, no `/etc/modules-load.d` entry is written.",
				YAML: `kernel_module:
  overlay:
    state: present
    persist: false`,
			},
			{
				Title: "Ensure a module is unloaded and stays out at boot",
				YAML: `kernel_module:
  pcspkr:
    state: absent`,
			},
		},
		Notes: []string{
			"Linux only — there is no kernel-module concept on other operating systems.",
			"Module names accept dashed (`br-netfilter`) or underscored (`br_netfilter`) forms; dashes are normalised to underscores (the kernel's internal form) so `/proc/modules` comparisons and persist filenames stay stable.",
			"Out of scope for v0.1: modprobe options / load-time module parameters, and `/etc/modprobe.d` blacklist management.",
		},
	}
}
