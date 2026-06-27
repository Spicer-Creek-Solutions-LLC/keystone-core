// SPDX-License-Identifier: Apache-2.0

package pkg

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the package module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "System & core",
		Summary: "Manages a Linux system package via the host's native package manager — " +
			"present at any version or pinned to an exact one, or removed. Idempotent: " +
			"re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "installed", Desc: "The package is installed, at any version or — if `version` is set — at that exact version."},
			{Name: "absent", Desc: "The package is not installed; an installed package is removed."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "version", Type: "string", Desc: "Exact version pin (state `installed` only); install/upgrade/downgrade to it. Omit for any version. Invalid with state `absent`."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Ensure a package is installed",
				YAML: `package:
  nginx:
    state: installed`,
			},
			{
				Title: "Pin to an exact version",
				Desc:  "The resource name is the package name; `version` pins it.",
				YAML: `package:
  nginx:
    state: installed
    version: "1.20.1"`,
			},
			{
				Title: "Remove a package",
				YAML: `package:
  telnet:
    state: absent`,
			},
		},
		Notes: []string{
			"Backend is auto-detected. apt, dnf, and apk are stable (verified on the live Debian/Ubuntu, Rocky, and Alpine matrix); zypper and pacman are experimental (implemented and unit-tested, but no SUSE/Arch distro is in the live matrix yet).",
			"`version` is invalid with state `absent`.",
			"Out of scope for v0.1: a `latest` state, explicit cache refresh, package holds/pinning, purge (vs remove), repository/source management, and macOS (Homebrew) / Windows (Chocolatey, winget).",
		},
	}
}
