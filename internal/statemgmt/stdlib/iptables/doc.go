// SPDX-License-Identifier: Apache-2.0

package iptables

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the iptables module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Firewall (base)",
		Summary: "Manages a single iptables (or ip6tables) rule, identified by its full " +
			"spec (chain + match args + target) rather than a name. Idempotent: a rule " +
			"is only appended/inserted or deleted when its presence differs from the " +
			"declared state.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The rule exists in the chain (`iptables -C` succeeds); Apply appends it (`-A`) or inserts it at `position` (`-I`) if missing. Existing rules are never reordered."},
			{Name: "absent", Desc: "The rule does not exist in the chain; Apply runs `-D` until every matching copy is removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "chain", Type: "string", Required: true, Desc: "Target chain, built-in or custom, e.g. `INPUT`, `POSTROUTING`."},
			{Name: "rule", Type: "string", Required: true, Desc: "The match spec + target only, e.g. `-p tcp --dport 22 -j ACCEPT`. May also be given as a list of args. Must not include `-t`, `-A/-I/-D/-C/-R`, `-N`, or `-P` — the module supplies the table and action."},
			{Name: "table", Type: "string", Default: "filter", Desc: "One of `filter`, `nat`, `mangle`, `raw`, `security`."},
			{Name: "family", Type: "string", Default: "ipv4", Desc: "Address family: `ipv4` (iptables) or `ipv6` (ip6tables)."},
			{Name: "position", Type: "int", Desc: "Insert position for `-I` (1-based); `0` (default) appends with `-A`. Allowed on `present` only."},
			{Name: "save", Type: "string", Desc: "Absolute path to write `iptables-save` output to after a change."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Allow inbound SSH",
				YAML: `iptables:
  allow-ssh:
    state: present
    chain: INPUT
    rule: "-p tcp --dport 22 -m conntrack --ctstate NEW -j ACCEPT"`,
			},
			{
				Title: "Insert an IPv6 NAT rule and persist the ruleset",
				Desc:  "`position: 1` inserts at the head of the chain; `save` writes ip6tables-save output.",
				YAML: `iptables:
  masquerade-v6:
    state: present
    chain: POSTROUTING
    table: nat
    family: ipv6
    rule:
      - -o
      - eth0
      - -j
      - MASQUERADE
    position: 1
    save: /etc/iptables/rules.v6`,
			},
			{
				Title: "Remove a blocked source",
				YAML: `iptables:
  drop-bad-host:
    state: absent
    chain: INPUT
    rule: "-s 10.0.0.5 -j DROP"
    save: /etc/iptables/rules.v4`,
			},
		},
		Notes: []string{
			"Linux only — the rule is checked/applied via the iptables/ip6tables binaries; other operating systems return an unsupported-OS error.",
			"A rule has no name: its identity is the full spec, so the declaration name is just a human label. The module never moves an existing rule (no order management).",
			"Out of scope (v0.x candidates, #114): a structured rule model (proto/dport/jump à la Salt), ordering guarantees, `family: both` in one declaration, distro persistence integration (netfilter-persistent / the iptables service) beyond a plain `save:`, iptables-nft vs iptables-legacy selection, and chain creation/policy. nftables is its own module.",
		},
	}
}
