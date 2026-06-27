// SPDX-License-Identifier: Apache-2.0

package vlan

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the vlan module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Network (base)",
		Summary: "Manages a Linux 802.1Q VLAN interface — creates or removes a tagged " +
			"interface on a parent link at runtime via `ip link`, optionally persisting " +
			"it to the host network config. Idempotent: re-applying an unchanged " +
			"declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The VLAN interface exists on `parent` with the declared `id`; when `persist` is set the matching network config is also written."},
			{Name: "absent", Desc: "The VLAN interface (matched by `name`) does not exist; any persisted config for it is removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "name", Type: "string", Required: true, Desc: "VLAN interface name, e.g. `eth0.10` (max 15 chars; letters, digits, `._-` only)."},
			{Name: "parent", Type: "string", Desc: "Underlying interface the VLAN tags ride on, e.g. `eth0`. Required for state `present`; rejected for `absent`."},
			{Name: "id", Type: "int", Desc: "VLAN ID, 1–4094 (802.1Q reserves 0 and 4095). Required for state `present`; rejected for `absent`."},
			{Name: "persist", Type: "string", Desc: "Boot-survive backend: `networkd`, `netplan`, or `auto`. Omit for runtime-only (the VLAN is in the kernel now but does not survive a reboot)."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Runtime-only tagged interface",
				YAML: `vlan:
  eth0.10:
    state: present
    parent: eth0
    id: 10`,
			},
			{
				Title: "Persist across reboots via networkd",
				Desc:  "Renders a `.netdev` plus an enslave drop-in under the parent's `.network.d/`.",
				YAML: `vlan:
  eth0.20:
    state: present
    parent: eth0
    id: 20
    persist: networkd`,
			},
			{
				Title: "Remove a VLAN and its persisted config",
				Desc:  "`absent` deletes by name only — `parent`/`id` must not be set.",
				YAML: `vlan:
  eth0.10:
    state: absent
    persist: networkd`,
			},
		},
		Notes: []string{
			"Linux-only — created at runtime via `ip link`; non-Linux hosts get a reduced no-op provider.",
			"No in-place reconciliation: an existing VLAN of the same name is treated as converged regardless of its live `id`/`parent`. To change a live VLAN, delete it first then declare it again (planned, #29).",
			"Out of scope: QinQ / 802.1ad, VLAN ranges, QoS maps, and persist backends beyond networkd/netplan/auto.",
			"DriftSeverity is HIGH — a missing tagged interface is a downed L2 segment.",
		},
	}
}
