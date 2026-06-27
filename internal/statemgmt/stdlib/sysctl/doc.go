// SPDX-License-Identifier: Apache-2.0

package sysctl

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the sysctl module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "System config",
		Summary: "Manages a kernel parameter (sysctl key): sets its runtime value and, by " +
			"default, records it in a keystone-managed drop-in under /etc/sysctl.d/ so it " +
			"survives a reboot. Idempotent: re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The kernel parameter (the declaration name) has the declared `value`; with `persist: true` (the default) a drop-in under `/etc/sysctl.d/` also records it."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "value", Type: "string", Required: true, Desc: "The value to set. Multi-field values (e.g. `\"4096 16384 4194304\"`) are whitespace-normalized; ints/bools are accepted and coerced to strings."},
			{Name: "persist", Type: "bool", Default: "true", Desc: "Write a `/etc/sysctl.d/` drop-in so the value survives reboot. Set `false` to change only the running value."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Enable IPv4 forwarding (runtime + persisted)",
				YAML: `sysctl:
  net.ipv4.ip_forward:
    state: present
    value: "1"`,
			},
			{
				Title: "Set a runtime-only value",
				Desc:  "With `persist: false` only the running kernel value changes; no drop-in is written.",
				YAML: `sysctl:
  net.core.somaxconn:
    state: present
    value: "1024"
    persist: false`,
			},
			{
				Title: "Multi-field value (slashed key form)",
				Desc:  "Keys accept slashed notation; it is normalized to the dotted form.",
				YAML: `sysctl:
  net/ipv4/tcp_rmem:
    state: present
    value: "4096 16384 4194304"`,
			},
		},
		Notes: []string{
			"Linux only; BSD/macOS sysctl namespaces differ and are out of scope.",
			"The declaration name is the kernel key, in dotted (`net.ipv4.ip_forward`) or slashed (`net/ipv4/ip_forward`) form — both normalize to the same persistence file.",
			"One drop-in file per key; a single consolidated keystone drop-in is a future enhancement.",
			"v0.1 sets the value directly and does not run `sysctl --system`, so no reload is needed this boot.",
		},
	}
}
