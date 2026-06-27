// SPDX-License-Identifier: Apache-2.0

package bond

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the bond module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Network (base)",
		Summary: "Manages a Linux bonding (link-aggregation) interface — creates it at " +
			"runtime via `ip link` with the declared mode, members, and link-monitor " +
			"interval, with optional boot persistence to networkd or netplan. Idempotent: " +
			"once the bond exists with the right kind, re-applying reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "A bond interface with the declared name exists (created with `mode`, `members`, and `miimon` when absent); boot config is written when `persist` is set."},
			{Name: "absent", Desc: "No interface of this name exists; an existing bond is deleted (members are released) and any persistent boot config is removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "name", Type: "string", Required: true, Desc: "Bond interface name, e.g. `bond0` (max 15 chars; letters, digits, and `._-`)."},
			{Name: "mode", Type: "string", Default: "balance-rr", Desc: "Bonding mode: a name (`balance-rr`, `active-backup`, `balance-xor`, `broadcast`, `802.3ad`, `balance-tlb`, `balance-alb`) or its numeric form `0`-`6`."},
			{Name: "members", Type: "list", Desc: "Interfaces to enslave at creation time (state `present` only); each is set master on the bond."},
			{Name: "miimon", Type: "int", Desc: "Link-monitor interval in milliseconds; `0` disables. Unset uses the kernel default."},
			{Name: "persist", Type: "string", Desc: "Boot-persistence backend: `networkd`, `netplan`, or `auto`. Unset = runtime-only (does not survive a reboot)."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Active-backup bond over two NICs",
				YAML: `bond:
  bond0:
    state: present
    name: bond0
    mode: active-backup
    members:
      - eth0
      - eth1
    miimon: 100`,
			},
			{
				Title: "802.3ad LACP bond persisted via networkd",
				Desc:  "With `persist` the bond is also written to the host network config so it survives a reboot.",
				YAML: `bond:
  bond0:
    state: present
    name: bond0
    mode: "802.3ad"
    members:
      - eth0
      - eth1
    persist: networkd`,
			},
			{
				Title: "Remove a bond",
				YAML: `bond:
  bond0:
    state: absent
    name: bond0`,
			},
		},
		Notes: []string{
			"Linux only — bonds are created via `ip link`; other operating systems are unsupported.",
			"No in-place attribute or member reconciliation: once the bond exists with kind `bond`, `mode`, `miimon`, and `members` are not reconciled. To change a live bond, declare it `absent` then re-declare it (members are released, traffic interrupted).",
			"`members` is only valid with state `present`; `absent` deletes the bond and releases members automatically.",
			"Persistence backends are `networkd` and `netplan` (or `auto`); NetworkManager and ifupdown are out of scope.",
			"An interface that exists with a non-`bond` kind is left untouched (the module refuses to clobber it).",
		},
	}
}
