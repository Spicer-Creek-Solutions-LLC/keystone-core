// SPDX-License-Identifier: Apache-2.0

package nftables

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the nftables module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Firewall (base)",
		Summary: "Manages a single nftables rule inside an existing chain, identified by its " +
			"canonical text (`family` + `table` + `chain` + `rule`). Idempotent: re-applying " +
			"an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The chain contains a rule whose canonical text equals `rule`; appended, or inserted at `index` when set."},
			{Name: "absent", Desc: "The chain contains no rule whose text equals `rule`; all matching rules are deleted."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "table", Type: "string", Required: true, Desc: "Existing table the chain lives in (the module does not create tables)."},
			{Name: "chain", Type: "string", Required: true, Desc: "Existing chain to manage the rule in (the module does not create chains)."},
			{Name: "rule", Type: "list", Required: true, Desc: "The rule expression only — match + statement, no verb/family/table/chain, e.g. `tcp dport 22 accept`. A string is whitespace-split; a list of args is used as-is. Write it in nft's canonical form."},
			{Name: "family", Type: "string", Default: "inet", Desc: "Address family: one of `ip`, `ip6`, `inet`, `arp`, `bridge`, `netdev`."},
			{Name: "index", Type: "int", Desc: "0-based insert position (`nft insert rule … index N`); omit to append. State `present` only."},
			{Name: "save", Type: "string", Desc: "Absolute path; after a change, `nft list ruleset` output is written there."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Allow inbound SSH",
				YAML: `nftables:
  allow-ssh:
    state: present
    table: filter
    chain: input
    rule: tcp dport 22 accept`,
			},
			{
				Title: "Insert a rule at the top of the chain and persist the ruleset",
				Desc:  "`index: 0` inserts before existing rules; `save` dumps the full ruleset after the change.",
				YAML: `nftables:
  drop-bad-source:
    state: present
    family: ip
    table: filter
    chain: input
    rule: ip saddr 10.0.0.0/8 drop
    index: 0
    save: /etc/nftables.conf`,
			},
			{
				Title: "Remove a rule",
				YAML: `nftables:
  remove-telnet:
    state: absent
    table: filter
    chain: input
    rule: tcp dport 23 accept`,
			},
		},
		Notes: []string{
			"Linux only (nft-native); the nft-native sibling of the `iptables` module — the two manage different rulesets and do not coordinate.",
			"Rule matching compares against nft's canonical rendering (what `nft list chain` prints), so write `rule` in canonical form — e.g. `tcp dport ssh accept` will not match a stored `tcp dport 22 accept` and would be re-added every run.",
			"The chain and table must already exist; the module creates neither, nor manages base-chain hooks/priority/policy, sets, maps, or flowtables.",
			"`index` and order management: the module never moves an existing rule. `index` only affects where a new rule is inserted; it is rejected with state `absent`.",
			"Out of scope (planned, #115): structured rule params, rule ordering/re-placement, table/chain management, and matching by handle or comment.",
		},
	}
}
