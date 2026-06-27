// SPDX-License-Identifier: Apache-2.0

package system

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the system module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "System & core",
		Summary: "Manages system-level settings that don't fit the other stdlib " +
			"modules — login banners (`/etc/motd`, `/etc/issue`, `/etc/issue.net`), " +
			"marker-gated reboots, and the system locale. Each declaration performs " +
			"exactly one operation, selected by which param is set. Idempotent: " +
			"re-applying a converged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The banner has the declared `content`, the locale equals `locale`, or (for `reboot`) a reboot is scheduled when the marker indicates one is needed."},
			{Name: "absent", Desc: "The banner file is emptied. Valid for the `banner` op only — `reboot` and `locale` reject `absent`."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "banner", Type: "string", Desc: "Banner to manage: `motd`, `issue`, or `issue_net`. Selects the banner op."},
			{Name: "content", Type: "string", Required: true, Desc: "Banner contents (required with `banner`; use `\"\"` for an empty file). Banner op only."},
			{Name: "reboot", Type: "bool", Desc: "Set `true` to select the reboot op (schedules a reboot only when the marker indicates one is needed). `false` is rejected."},
			{Name: "when_file", Type: "string", Default: "/var/run/reboot-required", Desc: "Reboot marker file checked before scheduling. Reboot op only."},
			{Name: "delay", Type: "int", Default: "1", Desc: "Minutes before the scheduled reboot (0–60; `0` reboots now). Reboot op only."},
			{Name: "locale", Type: "string", Desc: "POSIX locale identifier (e.g. `en_US.UTF-8`). Selects the locale op."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Set the login banner",
				YAML: `system:
  login-motd:
    state: present
    banner: motd
    content: |
      Authorized access only.
      All activity is logged.`,
			},
			{
				Title: "Reboot when a kernel update flags one",
				Desc:  "Schedules a reboot only when the marker file is present; otherwise no-op.",
				YAML: `system:
  apply-pending-reboot:
    state: present
    reboot: true
    delay: 5
    when_file: /var/run/reboot-required`,
			},
			{
				Title: "Set the system locale",
				YAML: `system:
  system-locale:
    state: present
    locale: en_US.UTF-8`,
			},
		},
		Notes: []string{
			"Behaviour is distribution-agnostic; Linux has the full provider, other operating systems get a reduced one.",
			"Exactly one of `banner` / `reboot` / `locale` must be set per declaration.",
			"`reboot` is marker-gated: the `when_file` marker is checked first, then a host reboot-hint tool (`needs-restarting -r` on RHEL/Rocky/Fedora); hosts with neither (e.g. Alpine) rely on the marker alone.",
			"`reboot` and `locale` support `state: present` only; `absent` applies to `banner` (empties the file).",
			"Out of scope (planned, #22): reboot disconnect-tolerance, Arch reboot-hint detection, Debian's `/etc/default/locale` dual-file, per-`LC_*` overrides, and `absent` semantics for reboot/locale.",
		},
	}
}
