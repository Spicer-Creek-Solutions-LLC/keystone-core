// SPDX-License-Identifier: Apache-2.0

package bridge

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the bridge module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Network (base)",
		Summary: "Manages a Linux bridge interface at runtime via `ip link`, optionally " +
			"enslaving member ports and persisting the bridge to the host network " +
			"config. Idempotent: re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The bridge interface exists; member ports are enslaved at creation, and (with `persist`) the host network config matches."},
			{Name: "absent", Desc: "The bridge interface does not exist; any persistent config files for it are removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "name", Type: "string", Required: true, Desc: "Bridge interface name, e.g. `br0` (max 15 chars; letters, digits, and `._-`)."},
			{Name: "members", Type: "list", Desc: "Interface names to enslave as bridge ports at creation (state `present` only)."},
			{Name: "stp", Type: "bool", Default: "false", Desc: "Enable Spanning Tree Protocol on the bridge."},
			{Name: "persist", Type: "string", Desc: "Persist the bridge to the host network config: `networkd`, `netplan`, or `auto`. Omit for runtime-only (does not survive reboot)."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Runtime-only bridge with member ports",
				YAML: `bridge:
  br0:
    state: present
    members:
      - eth0
      - eth1
    stp: true`,
			},
			{
				Title: "Bridge persisted via systemd-networkd",
				Desc:  "`persist` also renders the bridge to the host network config so it survives a reboot.",
				YAML: `bridge:
  br0:
    state: present
    members:
      - eth0
    persist: networkd`,
			},
			{
				Title: "Remove a bridge",
				YAML: `bridge:
  br0:
    state: absent`,
			},
		},
		Notes: []string{
			"Linux only; created and removed via `ip link`. Other operating systems are unsupported.",
			"Optional boot persistence backends: `networkd` and `netplan` (`auto` detects). Without `persist` the bridge is runtime-only and does not survive a reboot.",
			"No in-place reconciliation: if a bridge with this name already exists it is considered converged regardless of `stp`/`members`. To change a live bridge, delete it (state `absent`) then re-declare it (planned, #28).",
			"`members` is only valid with state `present`; on `absent` the bridge is deleted and its ports are released automatically.",
		},
	}
}
