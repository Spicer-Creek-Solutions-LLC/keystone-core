// SPDX-License-Identifier: Apache-2.0

package user

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the user module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "System & core",
		Summary: "Manages a Linux user account — its UID/GID, primary group, home " +
			"directory, login shell, GECOS comment, and supplementary groups. " +
			"Idempotent: re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The user exists with the declared scalar fields and supplementary groups."},
			{Name: "absent", Desc: "The user does not exist; an existing account is removed (and its home directory too when `remove_home` is set)."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "uid", Type: "int", Desc: "Numeric UID (state `present`)."},
			{Name: "gid", Type: "int", Desc: "Numeric primary GID (state `present`). Mutually exclusive with `group`."},
			{Name: "group", Type: "string", Desc: "Primary group name (state `present`). Mutually exclusive with `gid`."},
			{Name: "home", Type: "string", Desc: "Home directory; must be an absolute path (state `present`)."},
			{Name: "shell", Type: "string", Desc: "Login shell; must be an absolute path (state `present`)."},
			{Name: "comment", Type: "string", Desc: "GECOS comment field (state `present`)."},
			{Name: "groups", Type: "list", Desc: "Supplementary group names; the declared set replaces the live set (state `present`)."},
			{Name: "system", Type: "bool", Default: "false", Desc: "Create the account as a system user (state `present`)."},
			{Name: "create_home", Type: "bool", Default: "false", Desc: "Create the home directory on account creation (state `present`)."},
			{Name: "remove_home", Type: "bool", Default: "false", Desc: "Delete the home directory when removing the account (state `absent` only)."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Create a user with a shell and supplementary groups",
				YAML: `user:
  alice:
    state: present
    uid: 1500
    home: /home/alice
    shell: /bin/bash
    comment: Alice
    groups:
      - wheel
      - docker
    create_home: true`,
			},
			{
				Title: "System user with an explicit primary group",
				Desc:  "Use `group` or `gid` for the primary group, never both.",
				YAML: `user:
  svc-myapp:
    state: present
    system: true
    group: myapp
    home: /var/lib/myapp
    shell: /usr/sbin/nologin`,
			},
			{
				Title: "Remove a user and its home directory",
				YAML: `user:
  alice:
    state: absent
    remove_home: true`,
			},
		},
		Notes: []string{
			"Linux only for the full Check/Apply pipeline; mutating Apply on other operating systems returns ErrUnsupportedOS.",
			"On Linux the backend is auto-detected: shadow-utils (useradd/usermod/userdel) by default, BusyBox (adduser/deluser) on Alpine.",
			"BusyBox ships no usermod, so changing an existing account's scalar fields is unsupported there; creation and removal work.",
			"`gid` and `group` are mutually exclusive on a `present` user.",
			"`state: absent` may carry only `remove_home` (plus `severity`); attribute params are rejected.",
			"Out of scope for v0.1: macOS via dscl, password / SSH-key management, account expiration (chage), sudoers, and skeleton-dir overrides.",
		},
	}
}
