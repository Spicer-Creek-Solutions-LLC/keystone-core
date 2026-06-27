// SPDX-License-Identifier: Apache-2.0

package langpkg

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the langpkg module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "Files & VCS",
		Summary: "Manages one language-ecosystem package per declaration via pip, npm, " +
			"or gem — distinct from the OS-level `package` module. Idempotent: " +
			"re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The package is installed. With `version` set, exactly that version must be installed; otherwise any installed version satisfies."},
			{Name: "absent", Desc: "The package is not installed; an installed package is removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "name", Type: "string", Required: true, Desc: "Package name (npm scoped names like `@types/node` are accepted)."},
			{Name: "manager", Type: "string", Required: true, Desc: "Language toolchain: `pip`, `npm`, or `gem`. No cross-ecosystem auto-detect."},
			{Name: "version", Type: "string", Desc: "Strict-equality version pin (state `present` only). Unset means any installed version satisfies. Invalid with `absent`."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Pin a Python package version",
				YAML: `langpkg:
  gunicorn-pin:
    state: present
    name: gunicorn
    manager: pip
    version: "21.2.0"`,
			},
			{
				Title: "Install a global npm tool, remove a gem",
				YAML: `langpkg:
  pm2:
    state: present
    name: pm2
    manager: npm
  old-bundler:
    state: absent
    name: bundler
    manager: gem`,
			},
		},
		Notes: []string{
			"Module maturity: experimental.",
			"Installs are system-wide / global only (`pip install`, `npm install -g`, `gem install`).",
			"Version matching is strict-equality; semver ranges and constraints are out of scope.",
			"Out of scope: per-user / per-project installs, lockfile-driven installs, PEP-668 `--break-system-packages`, manager-option pass-through, and additional ecosystems (cargo, composer, mvn/gradle, go install).",
		},
	}
}
