// SPDX-License-Identifier: Apache-2.0

package link

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the link module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Storage",
		Summary: "Manages a symbolic or hard link at a path. Complements the `file` " +
			"module by adding dedicated hard-link support and a focused two-state " +
			"model when the link itself is the resource. Idempotent: re-applying an " +
			"unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "A link exists at the path pointing at `target` — a symlink (`kind: symlink`, the default) or a hard link (`kind: hard`)."},
			{Name: "absent", Desc: "Nothing exists at the path; an existing link or file is removed. A directory is left in place with an error (use the `file` module)."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "target", Type: "string", Required: true, Desc: "What the link points at. Required for `state: present`; rejected for `state: absent`. Symlink targets are stored verbatim and not resolved against the link's directory."},
			{Name: "kind", Type: "string", Default: "symlink", Desc: "`symlink` (default) for a symbolic link, or `hard` for a hard link. A hard-link target must exist and be on the same filesystem. Only meaningful with `state: present`."},
			{Name: "force", Type: "bool", Default: "false", Desc: "Replace an existing non-matching file at the path. A directory is never auto-removed."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Symbolic link",
				YAML: `link:
  /usr/local/bin/myapp:
    state: present
    target: /opt/myapp/bin/myapp`,
			},
			{
				Title: "Hard link, replacing any existing file",
				Desc:  "The target must exist and live on the same filesystem.",
				YAML: `link:
  /var/lib/myapp/current:
    state: present
    target: /var/lib/myapp/releases/v1.0.0
    kind: hard
    force: true`,
			},
			{
				Title: "Remove a link",
				YAML: `link:
  /etc/myapp/old.conf:
    state: absent`,
			},
		},
		Notes: []string{
			"Filesystem-agnostic: Linux and macOS in v1.0; Windows links are out of scope.",
			"`target` is required for `state: present` and rejected for `state: absent`.",
			"Symlink targets are compared and stored verbatim; relative targets are not normalised or canonicalised.",
			"For owner/group on the link itself, use the `file` module's `symlink` state.",
		},
	}
}
