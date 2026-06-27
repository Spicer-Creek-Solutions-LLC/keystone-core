// SPDX-License-Identifier: Apache-2.0

package ssh

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the ssh module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "SSH & security",
		Summary: "Manages a single entry in a user's ~/.ssh/authorized_keys — the " +
			"classic authorized_key.present / .absent. The entry is identified by its " +
			"key material (keytype + blob); its options and comment are rewritten to " +
			"match the declaration. Idempotent: re-applying an unchanged declaration " +
			"reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The user's authorized_keys contains the line for `key`; `~/.ssh` (0700) and authorized_keys (0600) are ensured, and a matching entry's options/comment are rewritten in place."},
			{Name: "absent", Desc: "No line with that key material is present; all matching lines are removed (the file/dir are otherwise left alone)."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "key", Type: "string", Required: true, Desc: "The public key as `\"<keytype> <blob>[ comment]\"` (no options prefix — use `options`)."},
			{Name: "user", Type: "string", Required: true, Desc: "Target user whose authorized_keys is managed. Managing another user's keys needs root."},
			{Name: "options", Type: "list", Desc: "authorized_keys options prefix, as a comma-joined string or a list of option strings (state `present` only)."},
			{Name: "comment", Type: "string", Desc: "Comment for the managed line; overrides any comment carried in `key` (state `present` only)."},
			{Name: "severity", Type: "string", Desc: "Override the drift severity reported for this entry (defaults to high)."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Authorize a deploy bot's key",
				YAML: `ssh:
  deploy-bot-key:
    state: present
    user: deploy
    key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAID build-bot"`,
			},
			{
				Title: "Restrict a key with options and a comment",
				Desc:  "`options` may be a list; it is joined into the comma-separated prefix.",
				YAML: `ssh:
  ci-runner-key:
    state: present
    user: ci
    key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAID"
    comment: ci-runner
    options:
      - no-pty
      - no-agent-forwarding`,
			},
			{
				Title: "Revoke a key",
				Desc:  "Only the key material identifies the entry; `options`/`comment` are not allowed with `absent`.",
				YAML: `ssh:
  retire-old-key:
    state: absent
    user: deploy
    key: "ssh-rsa AAAAB3NzaC1yc2EAAAA"`,
			},
		},
		Notes: []string{
			"OS-agnostic: managing authorized_keys lines works on any POSIX host.",
			"Managing another user's keys needs root (or running the agent as that user); the chown step surfaces an EPERM hint otherwise.",
			"Out of scope (planned, #113): validating the key blob as a well-formed SSH public key, options-set comparison, and whole-file management; the key blob is only charset-checked.",
			"State `absent` cannot carry `options` or `comment` — only the key material identifies the entry.",
		},
	}
}
