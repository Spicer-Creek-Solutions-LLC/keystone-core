// SPDX-License-Identifier: Apache-2.0

package timer

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the systemd_timer module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Scheduled tasks",
		Summary: "Manages a systemd `.timer` unit: it generates the unit from " +
			"structured parameters and controls whether the timer is enabled at " +
			"boot and currently armed. Idempotent: re-applying an unchanged " +
			"declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The `/etc/systemd/system/<name>.timer` unit exists with the generated content; with `enable: true` (the default) the timer is also enabled at boot and active, with `enable: false` it is disabled and inactive."},
			{Name: "absent", Desc: "The timer unit is removed (after a best-effort disable+stop) and systemd is reloaded."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "on_calendar", Type: "string", Required: true, Desc: "systemd `OnCalendar=` expression (required for state `present`). Single line; not pre-validated — systemctl rejects malformed expressions at enable time."},
			{Name: "service", Type: "string", Default: "<name>.service", Desc: "Unit the timer triggers (`Unit=`). The paired service is the operator's responsibility."},
			{Name: "persistent", Type: "bool", Default: "false", Desc: "systemd `Persistent=` flag — run the job on next boot if a scheduled run was missed."},
			{Name: "description", Type: "string", Default: "Keystone-managed timer <name>", Desc: "Unit `Description=` value. Single line."},
			{Name: "enable", Type: "bool", Default: "true", Desc: "Enable at boot and activate now. `false` requires the timer disabled and inactive."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Nightly backup timer",
				Desc:  "Triggers backup.service every day at 02:00, catching up after downtime.",
				YAML: `systemd_timer:
  backup:
    state: present
    on_calendar: "*-*-* 02:00:00"
    persistent: true
    description: "Nightly backup"`,
			},
			{
				Title: "Timer paired with its service",
				Desc:  "Compose with the `file` module for the unit; `require` orders the timer after it.",
				YAML: `systemd_timer:
  report:
    state: present
    on_calendar: "Mon *-*-* 08:00:00"
    service: report.service
    require:
      - file: /etc/systemd/system/report.service`,
			},
			{
				Title: "Remove a timer",
				YAML: `systemd_timer:
  backup:
    state: absent`,
			},
		},
		Notes: []string{
			"Backend: systemd only. On a Linux host without `systemctl` operations fail with ErrNoBackend; on non-Linux, ErrUnsupportedOS.",
			"`Declaration.Name` is the timer base name (`backup` → unit `backup.timer`).",
			"The `.service` the timer triggers is the operator's responsibility — compose with the `file` and `service` modules, or point `service:` at an existing unit.",
			"state=absent cannot carry attribute params (`on_calendar`, `service`, `persistent`, `description`).",
			"Out of scope (planned, #106): generating the paired `.service`, `--user` (per-user) timers, and additional `[Timer]` knobs (OnBootSec / OnUnitActiveSec / RandomizedDelaySec). v1.0 takes OnCalendar + Persistent only.",
		},
	}
}
