// SPDX-License-Identifier: Apache-2.0

package security

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the security module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "SSH & security",
		Summary: "Manages SELinux settings (global mode and named booleans) and AppArmor " +
			"per-profile modes. Each declaration performs exactly one operation, " +
			"selected by which params are set. Idempotent: re-applying an unchanged " +
			"declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The declared setting holds: the SELinux mode, the SELinux boolean value, or the AppArmor profile mode is converged. These are settings, not items — `absent` is not supported."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "mode", Type: "string", Desc: "SELinux global mode (op selector): `enforcing`, `permissive`, or `disabled`. Sets `SELINUX=` in `/etc/selinux/config` persistently and, when feasible, `setenforce` at runtime. Mutually exclusive with `boolean` and `apparmor.profile`."},
			{Name: "boolean", Type: "string", Desc: "SELinux boolean name to toggle (op selector), e.g. `httpd_can_network_connect`. Requires `value`. Mutually exclusive with `mode` and `apparmor.profile`."},
			{Name: "value", Type: "bool", Desc: "Desired state for `boolean`. Accepts `on`/`off`, `true`/`false`, `yes`/`no`, `1`/`0`. Only valid with `boolean`."},
			{Name: "apparmor.profile", Type: "string", Desc: "AppArmor profile name as `aa-status` reports it — usually the confined program's path, e.g. `/usr/bin/foo` (op selector). Requires `apparmor.profile_mode`. Mutually exclusive with `mode` and `boolean`."},
			{Name: "apparmor.profile_mode", Type: "string", Desc: "AppArmor profile mode: `enforce`, `complain`, or `disable` (`disable` converges to the profile being unloaded). Only valid with `apparmor.profile`."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Set the global SELinux mode",
				YAML: `security:
  set-selinux-enforcing:
    state: present
    mode: enforcing`,
			},
			{
				Title: "Toggle a SELinux boolean",
				YAML: `security:
  allow-httpd-network:
    state: present
    boolean: httpd_can_network_connect
    value: on`,
			},
			{
				Title: "Put an AppArmor profile into complain mode",
				YAML: `security:
  nginx-apparmor-complain:
    state: present
    apparmor.profile: /usr/sbin/nginx
    apparmor.profile_mode: complain`,
			},
		},
		Notes: []string{
			"Experimental, partial support; `present` only (see docs/project/STATE-SUPPORT-MATRIX.md).",
			"SELinux `mode: disabled` cannot transition at runtime — the persistent edit applies, but a reboot is required for the runtime to go disabled. The Apply comment says so.",
			"Exactly one of `mode`, `boolean`, or `apparmor.profile` must be set per declaration; the operation is identified by which params are present.",
			"Out of scope in v0.1: SELinux file contexts (`semanage fcontext` / `restorecon`), port labels, module install (`semodule`), login/user mappings; AppArmor whole-subsystem enable/disable and profile load/reload; runtime-only mode sets without touching the persistent config.",
		},
	}
}
