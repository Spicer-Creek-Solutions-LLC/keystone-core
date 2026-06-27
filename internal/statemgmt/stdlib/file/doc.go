// SPDX-License-Identifier: Apache-2.0

package file

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the file module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "System & core",
		Summary: "Manages a single filesystem path — a regular file (content from " +
			"inline text or a source), a directory, or a symlink — along with its " +
			"owner, group, and mode. Idempotent: re-applying an unchanged declaration " +
			"reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "A regular file exists with the declared content (or copied from `source`), owner, group, and mode."},
			{Name: "directory", Desc: "A directory exists with the declared owner, group, and mode."},
			{Name: "symlink", Desc: "A symlink exists at the declared path pointing at `target`."},
			{Name: "absent", Desc: "The path does not exist; a file, directory, or symlink there is removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "content", Type: "string", Desc: "Inline file content (state `present`). Mutually exclusive with `source`."},
			{Name: "source", Type: "string", Desc: "Path to copy content from (state `present`). Mutually exclusive with `content`."},
			{Name: "mode", Type: "string", Desc: "Permission bits in octal, quoted, e.g. `\"0644\"`."},
			{Name: "owner", Type: "string", Desc: "Owning user (name or numeric uid)."},
			{Name: "group", Type: "string", Desc: "Owning group (name or numeric gid)."},
			{Name: "target", Type: "string", Required: true, Desc: "Symlink target (state `symlink` only)."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Write a config file",
				YAML: `file:
  /etc/myapp/config.toml:
    state: present
    content: |
      [server]
      port = 8080
    owner: myapp
    group: myapp
    mode: "0640"`,
			},
			{
				Title: "Ensure a directory, then a file inside it",
				Desc:  "The `require` requisite orders the file after the directory.",
				YAML: `file:
  /opt/myapp:
    state: directory
    mode: "0750"
  /opt/myapp/VERSION:
    state: present
    content: "1.0.0\n"
    require:
      - file: /opt/myapp`,
			},
			{
				Title: "Symlink and removal",
				YAML: `file:
  /usr/local/bin/myapp:
    state: symlink
    target: /opt/myapp/bin/myapp
  /etc/myapp/old.conf:
    state: absent`,
			},
		},
		Notes: []string{
			"Linux has the full provider; other operating systems get a reduced one.",
			"`content` and `source` are mutually exclusive on a `present` file.",
		},
	}
}
