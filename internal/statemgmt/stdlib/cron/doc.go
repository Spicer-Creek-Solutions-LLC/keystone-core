// SPDX-License-Identifier: Apache-2.0

package cron

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the cron module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Scheduled tasks",
		Summary: "Manages a single per-user crontab entry, identified by the declaration " +
			"name and tagged with a marker comment so only that entry is owned. " +
			"Idempotent: re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The user's crontab has an entry tagged with the declaration name, carrying the declared `schedule` and `command`."},
			{Name: "absent", Desc: "The user's crontab has no entry tagged with the declaration name; an existing tagged entry is removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "command", Type: "string", Required: true, Desc: "The command line to run (single line). Required for state `present`; rejected for `absent`."},
			{Name: "schedule", Type: "string", Required: true, Desc: "A five-field cron spec (`*/5 * * * *`) or an `@`-shortcut (`@daily`, `@reboot`, …). Required for state `present`; rejected for `absent`."},
			{Name: "user", Type: "string", Default: "root", Desc: "Owner of the crontab to manage."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Run a nightly backup",
				YAML: `cron:
  nightly-backup:
    state: present
    command: /usr/bin/backup
    schedule: "0 2 * * *"
    user: root`,
			},
			{
				Title: "Per-user job with an @-shortcut",
				YAML: `cron:
  app-cache-warm:
    state: present
    command: /opt/myapp/bin/warm-cache
    schedule: "@hourly"
    user: myapp`,
			},
			{
				Title: "Remove a job",
				Desc:  "Only the entry tagged with this name is removed; other crontab lines are left alone.",
				YAML: `cron:
  old-cleanup:
    state: absent
    user: root`,
			},
		},
		Notes: []string{
			"Linux only; the module shells out to `crontab(1)` and fails when that binary is absent.",
			"Manages exactly the entry tagged `# keystone-cron: <name>`; other crontab lines are never touched.",
			"`command` and `schedule` are required for `present` and rejected for `absent`.",
			"`schedule` field contents are not deeply validated — only field count and known `@`-shortcuts.",
			"Out of scope (planned, #105): per-field schedule params, `/etc/cron.d` drop-ins, environment lines.",
		},
	}
}
