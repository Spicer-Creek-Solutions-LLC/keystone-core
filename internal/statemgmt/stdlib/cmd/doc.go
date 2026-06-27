// SPDX-License-Identifier: Apache-2.0

package cmd

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the cmd module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "System & core",
		Summary: "Executes an arbitrary shell command via /bin/sh, gated by an " +
			"idempotency guard (`creates`, `onlyif`, or `unless`). The mandatory " +
			"guard keeps the declaration idempotent: once the command's work is " +
			"done the guard flips to skip and re-applying reports no change.",
		States: []statemgmt.StateDoc{
			{Name: StateRun, Desc: "Run `command` through /bin/sh unless its guard (`creates`/`onlyif`/`unless`) says the work is already done."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "command", Type: "string", Required: true, Desc: "The shell command to execute via `/bin/sh -c`."},
			{Name: "creates", Type: "string", Desc: "Absolute path; skip the command if this path already exists. Acts as the idempotency guard."},
			{Name: "onlyif", Type: "string", Desc: "Guard command; run `command` only if this exits zero."},
			{Name: "unless", Type: "string", Desc: "Guard command; run `command` only if this exits non-zero."},
			{Name: "cwd", Type: "string", Desc: "Working directory the command runs in."},
			{Name: "env", Type: "map", Desc: "Environment variables (string keys to string values) added to the command's environment."},
			{Name: "timeout_seconds", Type: "int", Default: "60", Desc: "Kill the command after this many seconds. Range [0, 3600]."},
			{Name: "shell", Type: "string", Default: "/bin/sh", Desc: "Shell interpreter. Only `/bin/sh` is supported in v1.0; alternate shells are v1.x."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Run once, guarded by a marker file",
				Desc:  "`creates` makes the declaration idempotent: the command is skipped once the path exists.",
				YAML: `cmd:
  bootstrap-db:
    state: run
    command: /opt/myapp/bin/init-db && touch /var/lib/myapp/.initialized
    creates: /var/lib/myapp/.initialized`,
			},
			{
				Title: "Guard with onlyif and a working directory",
				YAML: `cmd:
  rebuild-cache:
    state: run
    command: make cache
    cwd: /opt/myapp
    onlyif: test -f /opt/myapp/cache.dirty
    timeout_seconds: 300`,
			},
			{
				Title: "Run after a package install, with env",
				Desc:  "The `require` requisite orders this command after the package is present.",
				YAML: `cmd:
  migrate:
    state: run
    command: myapp migrate
    unless: myapp migrate --check
    env:
      MYAPP_ENV: production
    require:
      - package: myapp`,
			},
		},
		Notes: []string{
			"Linux and macOS only; the module is `/bin/sh`-based, so Windows / cmd.exe is v1.x.",
			"`state: run` requires at least one guard (`creates` / `onlyif` / `unless`) for idempotency; use `onlyif: /bin/true` to opt into always-run.",
			"`creates` must be an absolute path.",
			"A successful Apply always counts as a change; the engine only invokes Apply when Check reports the guards say \"run\".",
			"Out of scope for v0.1: `runas` (run as another user), non-POSIX shells, sandboxing (seccomp / namespaces), and command-policy integration.",
		},
	}
}
