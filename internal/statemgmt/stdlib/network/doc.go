// SPDX-License-Identifier: Apache-2.0

package network

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the network module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Network (base)",
		Summary: "Manages one network interface's runtime configuration — IP addresses, " +
			"MTU, and admin up/down — via the iproute2 `ip` tool, optionally rendering a " +
			"boot-survive file (systemd-networkd or netplan) so the config persists across " +
			"reboot. Idempotent: re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The interface carries the declared addresses, MTU, and admin state; when `persist` is set, a matching boot-survive file is written. The module does not create or remove interfaces."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "interface", Type: "string", Required: true, Desc: "Interface name to reconcile (≤15 chars; letters/digits/`._-`). The interface must already exist."},
			{Name: "addresses", Type: "list", Desc: "Full set of CIDR addresses for the interface; extras are removed (kernel link-local addresses are never stripped). An empty list means \"no addresses\"."},
			{Name: "mtu", Type: "int", Desc: "Link MTU, in the range 68–65535."},
			{Name: "up", Type: "bool", Desc: "Link admin state (`ip link set up|down`). Runtime-only; not rendered to the persistent file."},
			{Name: "persist", Type: "string", Desc: "Boot-survive renderer: `networkd`, `netplan`, or `auto` (netplan if `/etc/netplan` exists, else networkd). Requires `addresses` or `mtu`. Omit for runtime-only config."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Set addresses and MTU on an interface",
				YAML: `network:
  configure-eth0:
    state: present
    interface: eth0
    addresses:
      - 192.168.1.10/24
      - 10.0.0.1/24
    mtu: 1500`,
			},
			{
				Title: "Persist config across reboot",
				Desc:  "`persist: auto` also writes a networkd or netplan file for the next boot.",
				YAML: `network:
  configure-eth1:
    state: present
    interface: eth1
    addresses:
      - 10.20.0.5/24
    mtu: 9000
    persist: auto`,
			},
			{
				Title: "Bring a link up",
				YAML: `network:
  bring-up-eth2:
    state: present
    interface: eth2
    up: true`,
			},
		},
		Notes: []string{
			"At least one of `addresses` / `mtu` / `up` must be set — a bare `interface:` declaration is a no-op and is rejected.",
			"`persist` renderers are limited to systemd-networkd and netplan; NetworkManager, Debian `/etc/network/interfaces`, and RHEL `ifcfg-*` are planned for v0.6+ (#25).",
			"`persist` writes the file for the next boot only — the runtime is already live via `ip`, so nothing is auto-activated (no `netplan apply` / `networkctl reload`).",
			"`up` is runtime-only and is not rendered to the persistent file (networkd brings a matched link up by default).",
			"Kernel-auto-assigned link-local addresses (IPv6 `fe80::/10`, IPv4 `169.254.0.0/16`) are never removed by address reconciliation.",
			"The module does not create or remove interfaces (use `bond` / `bridge` / `vlan` for virtual interfaces). Linux-only; other operating systems get a no-op provider.",
			"Out of scope (v0.x candidates): per-family address sets, address scope/lifetime attributes, and DNS / NTP / search-domain management.",
		},
	}
}
