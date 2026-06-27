// SPDX-License-Identifier: Apache-2.0

package at

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the at module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Scheduled tasks",
		Summary: "Manages one-shot scheduled jobs via the Linux `at` toolchain. The " +
			"declaration name tags the job with a marker comment so the module can " +
			"find its own jobs in the queue. Idempotent: re-applying a declaration " +
			"whose tagged job is already queued reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "A job tagged with the declaration name is queued; if none is, one is submitted running `command` at `time`. `at` jobs are one-shot — once a job has run it leaves the queue, so a later run re-queues it."},
			{Name: "absent", Desc: "No job tagged with the declaration name is queued; any matching jobs are removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "command", Type: "string", Desc: "Command to run; required for state `present`."},
			{Name: "time", Type: "string", Desc: "The `at` time spec, passed verbatim (e.g. `now + 1 hour`, `10:30 PM`, `2026-06-01 09:00`). Required for state `present`; must be a single line."},
			{Name: "queue", Type: "string", Default: "a", Desc: "The `at` queue letter (a single ASCII letter `a`–`z`)."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Schedule a one-shot reminder",
				YAML: `at:
  backup-reminder:
    state: present
    command: /usr/bin/remind backup
    time: now + 1 hour`,
			},
			{
				Title: "Queue a job on a specific queue letter",
				YAML: `at:
  nightly-report:
    state: present
    command: /usr/local/bin/report --nightly
    time: midnight tomorrow
    queue: b`,
			},
			{
				Title: "Remove any queued jobs with this tag",
				YAML: `at:
  nightly-report:
    state: absent`,
			},
		},
		Notes: []string{
			"Linux only, via the `at` toolchain; mutating operations fail if the `at` binary is absent.",
			"`at` jobs are fire-once, not recurring — use the cron or systemd_timer modules for recurring schedules.",
			"The command/time of an already-queued tagged job is not re-checked; change the declaration name (or remove it first) to queue a different job.",
			"Not yet supported (planned, #109): replace-on-change re-queue, per-user queues, and `batch` low-load mode.",
		},
	}
}
