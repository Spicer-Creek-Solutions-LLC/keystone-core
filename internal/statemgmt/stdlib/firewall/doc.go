// SPDX-License-Identifier: Apache-2.0

package firewall

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the firewall module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Firewall (base)",
		Summary: "Manages a single inbound allow rule for a named service or a " +
			"port, dispatched to whichever firewall backend (iptables, nftables, " +
			"or firewalld) is in use on the host. Idempotent: re-applying an " +
			"unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The inbound allow rule for the declared service/port exists on the active backend."},
			{Name: "absent", Desc: "The inbound allow rule for the declared service/port is removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "service", Type: "string", Desc: "Catalog service name (e.g. `ssh`, `https`, `samba`) resolved to its port set. Mutually exclusive with `port`; exactly one of the two is required."},
			{Name: "port", Type: "string", Desc: "Explicit port spec `PORT[-PORT]/{tcp,udp,sctp,dccp}` (e.g. `8080/tcp`, `1000-2000/udp`). Mutually exclusive with `service`; exactly one of the two is required."},
			{Name: "backend", Type: "string", Desc: "Force a backend — one of `iptables`, `nftables`, `firewalld`. Auto-detected from the host when omitted."},
			{Name: "zone", Type: "string", Default: "public", Desc: "firewalld zone (firewalld backend only)."},
			{Name: "strict_catalog", Type: "bool", Default: "true", Desc: "When false, a `service:` name not in the static catalog falls back to the host's `/etc/services`."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Allow SSH inbound",
				Desc:  "Backend is auto-detected; the SSH service resolves to 22/tcp.",
				YAML: `firewall:
  allow-ssh:
    state: present
    service: ssh`,
			},
			{
				Title: "Allow an explicit port on a pinned backend",
				YAML: `firewall:
  allow-app:
    state: present
    port: 8080/tcp
    backend: nftables`,
			},
			{
				Title: "firewalld zone and removal",
				YAML: `firewall:
  allow-https-dmz:
    state: present
    service: https
    backend: firewalld
    zone: dmz
  drop-old-port:
    state: absent
    port: 9000/tcp`,
			},
		},
		Notes: []string{
			"Backends: iptables, nftables, and firewalld. Backend is selected by `backend:` or auto-detected (firewalld → iptables → nftables).",
			"The iptables backend applies dual-stack (IPv4 + IPv6) by default; on a host without ip6tables the IPv6 half is skipped gracefully and the result says so.",
			"Exactly one of `service` and `port` must be set.",
			"v0.1 out of scope (planned, #20): `action: deny` (allow-only today), chain/table/family overrides on the iptables/nftables backends, per-source filtering, and nftables backend chain creation. Use a backend module directly for those.",
		},
	}
}
