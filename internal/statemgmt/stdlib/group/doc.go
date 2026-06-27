// SPDX-License-Identifier: Apache-2.0

package group

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the group module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "System & core",
		Summary: "Manages a Unix group, optionally pinning its numeric GID. Idempotent: " +
			"re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The group exists with the declared GID (when `gid` is set); otherwise any existing GID is left untouched."},
			{Name: "absent", Desc: "The group does not exist; an existing group of that name is removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "gid", Type: "int", Desc: "Numeric GID to pin. When set, an existing group with a different GID is modified to match. Not allowed with state `absent`."},
			{Name: "system", Type: "bool", Default: "false", Desc: "Create as a system group (allocates from the system GID range). Honored only on group creation; not allowed with state `absent`."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Ensure a group exists",
				YAML: `group:
  developers:
    state: present`,
			},
			{
				Title: "Pin a numeric GID",
				YAML: `group:
  developers:
    state: present
    gid: 1500`,
			},
			{
				Title: "System group, then removal of another",
				YAML: `group:
  app-runtime:
    state: present
    system: true
  legacy:
    state: absent`,
			},
		},
		Notes: []string{
			"Linux has the full Check / Apply / Test pipeline. On macOS / BSD only the read-only lookup works; mutating applies return ErrUnsupportedOS.",
			"On Linux the backend is auto-detected: shadow-utils (groupadd/groupmod/groupdel) when `groupadd` is present, otherwise BusyBox (addgroup/delgroup) on Alpine.",
			"BusyBox ships no groupmod, so changing an existing group's GID is unsupported on the BusyBox backend (ErrModUnsupported); creation and removal work.",
			"`gid` and `system` cannot be set when state is `absent`.",
			"Out of scope for v0.1: macOS via `dscl`, forced deletion of a user's primary group, `members:` (use `user.groups:`), NIS/LDAP/SSSD mutations, and Windows.",
		},
	}
}
