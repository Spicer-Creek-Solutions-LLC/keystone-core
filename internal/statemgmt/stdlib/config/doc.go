// SPDX-License-Identifier: Apache-2.0

package config

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the config module. Rendered
// into the docs-site "State Modules" section by tools/gendocs/modules
// (regenerated via `make docs-sync`). Keep States in sync with
// ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Files & VCS",
		Summary: "Manages a single key/value entry inside a config file — a flat " +
			"`keyvalue` file or an INI file — touching only the line that defines " +
			"the key and leaving every comment, blank line, and other key untouched. " +
			"Idempotent: re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The key equals `value` in the given section. An existing key's value is replaced in place; a missing key (or section, or file) is created."},
			{Name: "absent", Desc: "Every line defining the key in the given section is removed; the section header is left in place even if it empties."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "key", Type: "string", Required: true, Desc: "Config key to manage. Must not contain `=`, newlines, or start with `#`, `;`, or `[`."},
			{Name: "value", Type: "string", Desc: "Desired value (required for state `present`; rejected for `absent`). Numbers and bools are coerced to their string form; must be a single line."},
			{Name: "format", Type: "string", Default: "keyvalue", Desc: "File shape: `keyvalue` (flat `key=value` lines) or `ini` (with `[section]` headers)."},
			{Name: "section", Type: "string", Desc: "INI section header the key belongs to (`format: ini` only); `\"\"` is the implicit top section before any header."},
			{Name: "space_around_separator", Type: "bool", Default: "false", Desc: "For newly written lines, emit `key = value` instead of `key=value`. Rejected for state `absent`."},
			{Name: "create", Type: "bool", Default: "true", Desc: "For state `present`, create the file if it is missing. With `false`, a missing file is an error."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Set a key in a flat keyvalue file",
				YAML: `config:
  /etc/default/myapp:
    state: present
    key: LOG_LEVEL
    value: info`,
			},
			{
				Title: "Set a key in an INI section",
				Desc:  "The section header is created if it does not exist.",
				YAML: `config:
  /etc/myapp/app.ini:
    state: present
    format: ini
    section: server
    key: port
    value: "8080"
    space_around_separator: true`,
			},
			{
				Title: "Remove a key, refusing to create a missing file",
				YAML: `config:
  /etc/default/myapp:
    state: absent
    key: DEBUG
  /etc/myapp/optional.conf:
    state: present
    key: enabled
    value: "true"
    create: false`,
			},
		},
		Notes: []string{
			"Distro-agnostic: operates on file text, not on any package or service.",
			"Complements the `file` module — `file` owns whole-file content; `config` touches only one key's line, preserving everything else.",
			"Key matching is case-sensitive; `present` updates the first occurrence, `absent` removes all occurrences.",
			"Out of scope in v0.1: TOML/YAML/JSON/XML formats, configurable separators, inline/trailing comments, uncomment-aware updates, repeated-key directives, and creating parent directories (planned, #107).",
		},
	}
}
