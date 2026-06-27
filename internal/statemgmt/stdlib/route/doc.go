// SPDX-License-Identifier: Apache-2.0

package route

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the route module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Network (base)",
		Summary: "Manages one entry in the kernel routing table at runtime via iproute2, " +
			"keyed on `(destination, table)` (plus `metric` when set). Optionally renders " +
			"the route to networkd or netplan so it survives a reboot. Idempotent: " +
			"re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The route exists with the declared gateway and/or interface (added or `ip route replace`d to converge)."},
			{Name: "absent", Desc: "No route for the declared destination/metric/table; an existing one is removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "destination", Type: "string", Required: true, Desc: "Target as CIDR or a bare host IP (a host IP becomes a /32 or /128). Use `0.0.0.0/0` or `::/0` for the default route."},
			{Name: "gateway", Type: "string", Desc: "Next-hop IP. For state `present`, at least one of `gateway` / `interface` is required."},
			{Name: "interface", Type: "string", Desc: "Output interface (`dev`). For state `present`, at least one of `gateway` / `interface` is required; `persist` additionally requires it."},
			{Name: "metric", Type: "int", Desc: "Route metric. When set, it is part of the route's identity (multiple routes to one destination at different metrics)."},
			{Name: "table", Type: "string", Default: "main", Desc: "Routing table by `rt_tables` name (main/local/default/custom) or numeric value 1-254."},
			{Name: "persist", Type: "string", Desc: "Boot-survive backend: `networkd`, `netplan`, or `auto`. Omit for runtime-only. Requires `interface`."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Default route via a gateway",
				YAML: `route:
  default-route:
    state: present
    destination: 0.0.0.0/0
    gateway: 192.168.1.1`,
			},
			{
				Title: "Static network route, persisted across reboots",
				Desc:  "`persist` renders the route to networkd so it survives a reboot; it requires `interface`.",
				YAML: `route:
  vpn-net:
    state: present
    destination: 10.0.0.0/24
    gateway: 192.168.1.254
    interface: eth0
    metric: 100
    table: vpn
    persist: networkd`,
			},
			{
				Title: "Remove a stale route",
				YAML: `route:
  drop-old:
    state: absent
    destination: 10.0.0.0/24`,
			},
		},
		Notes: []string{
			"Linux-only; manages routes via the iproute2 `ip` tool. `present` uses `ip route replace` (idempotent against an existing entry at the same dest/metric/table).",
			"Without `persist`, the route is in the kernel immediately but does not survive a reboot.",
			"`persist` supports `networkd` and `netplan` (or `auto`) and requires `interface`. networkd merges drop-ins; netplan replaces a route list across files, so use networkd for multiple routes on one interface.",
			"Out of scope (v0.x candidates): additional persist backends (NetworkManager, sysconfig route-*, /etc/network/interfaces), richer route attributes (proto/scope/src/mtu/onlink/nexthop multipath), and source-routing policy rules.",
		},
	}
}
