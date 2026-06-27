// SPDX-License-Identifier: Apache-2.0

package hostname

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the hostname module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "System & core",
		Summary: "Manages the system static hostname. The declaration name IS the " +
			"desired hostname. Idempotent: re-applying when the static hostname " +
			"already matches reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The system static hostname equals the declaration name."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "severity", Type: "string", Desc: "Overrides the reported drift severity (defaults to `medium` for this module)."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Set a short hostname",
				YAML: `hostname:
  web-1:
    state: present`,
			},
			{
				Title: "Set a fully-qualified hostname",
				Desc:  "The declaration name carries the desired value, including any FQDN.",
				YAML: `hostname:
  web1.us-east.internal:
    state: present`,
			},
		},
		Notes: []string{
			"The declaration name is the desired hostname; there are no parameters beyond `severity`.",
			"Linux is fully supported: `hostnamectl` is used when present (handling dbus / SELinux), otherwise the module writes `/etc/hostname` and execs `hostname(1)`. Non-Linux returns ErrUnsupportedOS.",
			"v0.1 out of scope: pretty hostname (`/etc/machine-info`), transient-only mode, divergent running-vs-static repair, and macOS.",
		},
	}
}
