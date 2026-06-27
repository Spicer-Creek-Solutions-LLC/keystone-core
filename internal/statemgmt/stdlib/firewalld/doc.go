// SPDX-License-Identifier: Apache-2.0

package firewalld

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the firewalld module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Firewall (base)",
		Summary: "Manages one item — a service, a port, or a rich rule — in a firewalld " +
			"zone via `firewall-cmd --permanent`, reloading so the change takes effect. " +
			"Idempotent: re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The declared service, port, or rich rule is enabled on the zone; added if missing."},
			{Name: "absent", Desc: "The declared service, port, or rich rule is not enabled on the zone; removed if present."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "zone", Type: "string", Default: "public", Desc: "Target firewalld zone. The zone must already exist — this module does not create zones."},
			{Name: "service", Type: "string", Desc: "A firewalld service name (e.g. `ssh`). Exactly one of `service` / `port` / `rich_rule` is required."},
			{Name: "port", Type: "string", Desc: "A port or port range with protocol, `PORT[-PORT]/{tcp,udp,sctp,dccp}` (e.g. `8080/tcp`). Exactly one of `service` / `port` / `rich_rule` is required."},
			{Name: "rich_rule", Type: "string", Desc: "A firewalld rich rule, compared by canonical form. Exactly one of `service` / `port` / `rich_rule` is required."},
			{Name: "reload", Type: "bool", Default: "true", Desc: "Run `firewall-cmd --reload` after a change to push the permanent change to the running firewalld."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Allow the ssh service on the public zone",
				YAML: `firewalld:
  allow-ssh:
    state: present
    service: ssh`,
			},
			{
				Title: "Open a port on a specific zone",
				YAML: `firewalld:
  app-port:
    state: present
    zone: trusted
    port: 8080/tcp`,
			},
			{
				Title: "Add a rich rule, and remove a service",
				Desc:  "Rich rules are matched by canonical form, so reformatting them does not cause drift.",
				YAML: `firewalld:
  drop-rfc1918:
    state: present
    rich_rule: rule family="ipv4" source address="10.0.0.0/8" drop
  no-cockpit:
    state: absent
    service: cockpit`,
			},
		},
		Notes: []string{
			"Linux only: operates by shelling out to `firewall-cmd`; other operating systems return an unsupported-OS error.",
			"Always operates on the *permanent* configuration; `reload: true` (the default) makes the change active in the running firewalld.",
			"Exactly one of `service` / `port` / `rich_rule` must be set per declaration.",
			"Rich-rule matching is syntactic only — it does not normalise value semantics (e.g. it will not lowercase a MAC or canonicalise a CIDR), so write such values exactly as firewalld stores them.",
			"Out of scope (planned, #19): whole-zone management and zone creation, interface/source binding, default-zone and per-zone target, masquerade/forward-port/ICMP toggles, `--direct` rules, ipset, runtime-only changes, and lockdown/panic mode.",
		},
	}
}
