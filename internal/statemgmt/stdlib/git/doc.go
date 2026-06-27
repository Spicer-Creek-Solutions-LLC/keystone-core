// SPDX-License-Identifier: Apache-2.0

package git

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the git module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Files & VCS",
		Summary: "Manages a git working tree on the agent: clones a repository to a " +
			"path and optionally tracks a revision on the remote. Idempotent: " +
			"re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "A working tree exists at the declaration name, cloned from `url`, with the named remote pointing at `url`. The checked-out revision is set from `rev` on the initial clone but not enforced afterward."},
			{Name: "latest", Desc: "Like `present`, and HEAD matches `rev` on the remote (default: the remote's default branch). Apply fetches and, with `force`, hard-resets the current ref to the fetched commit."},
			{Name: "absent", Desc: "The path does not exist. A non-repo directory at the path is left in place with an error."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "url", Type: "string", Desc: "Repository URL. Required for `present` and `latest`; rejected for `absent`."},
			{Name: "rev", Type: "string", Default: "HEAD", Desc: "Branch, tag, or SHA to track. `HEAD` means the remote's default branch."},
			{Name: "depth", Type: "int", Default: "0", Desc: "Shallow-clone depth; `0` means a full clone."},
			{Name: "remote", Type: "string", Default: "origin", Desc: "Name of the git remote."},
			{Name: "force", Type: "bool", Desc: "Let `latest` discard local changes during a hard reset. Defaults to `true` for `latest`; opt out with `force: false`."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Clone a repository once",
				Desc:  "`present` clones if the tree is missing and does not enforce the revision thereafter.",
				YAML: `git:
  /opt/myapp/src:
    state: present
    url: https://example.com/myorg/myapp.git
    rev: v1.2.0`,
			},
			{
				Title: "Track a branch on the remote",
				Desc:  "`latest` fetches and hard-resets the current ref to match the remote.",
				YAML: `git:
  /opt/myapp/src:
    state: latest
    url: https://example.com/myorg/myapp.git
    rev: main
    remote: upstream
    depth: 1`,
			},
			{
				Title: "Remove a working tree",
				YAML: `git:
  /opt/myapp/src:
    state: absent`,
			},
		},
		Notes: []string{
			"Distro-agnostic: shells out to the `git` binary via a provider. Mutating operations fail when git is not installed.",
			"`url` is required for `present`/`latest` and rejected (along with `rev`, `depth`, `remote`) for `absent`.",
			"On `latest`, `rev` updates the current ref, not a named local branch; it does not switch branches.",
			"Out of scope in v0.1: authentication (deploy keys, credential helpers, token rotation, SSH host keys), submodules, sparse/partial/bare checkouts, and server-side rev resolution for the `present` up-to-date check (which deliberately does not contact the remote).",
		},
	}
}
