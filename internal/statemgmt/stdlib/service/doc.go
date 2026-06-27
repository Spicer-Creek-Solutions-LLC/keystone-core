// SPDX-License-Identifier: Apache-2.0

package service

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the service module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "System & core",
		Summary: "Manages a system service's running state and, optionally, its " +
			"boot-enablement. Running and enabled-at-boot are orthogonal axes. " +
			"Idempotent: re-applying an unchanged declaration reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "running", Desc: "The service is active (started if it was stopped)."},
			{Name: "stopped", Desc: "The service is inactive (stopped if it was running)."},
		},
		Params: []statemgmt.ParamDoc{
			{Name: "enable", Type: "bool", Desc: "Boot-enablement: `true` enables the unit at boot, `false` disables it. Omit to leave the boot-state untouched."},
		},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Run and enable a service at boot",
				YAML: `service:
  nginx:
    state: running
    enable: true`,
			},
			{
				Title: "Install the package, then ensure its service runs",
				Desc:  "The `require` requisite orders the service after the package that provides the unit.",
				YAML: `service:
  nginx:
    state: running
    enable: true
    require:
      - package: nginx`,
			},
			{
				Title: "Stop and disable a service",
				YAML: `service:
  telnet:
    state: stopped
    enable: false`,
			},
		},
		Notes: []string{
			"Linux only; no init system on other operating systems is supported.",
			"systemd and OpenRC backends are stable (verified on the live distro matrix); the sysvinit backend is experimental (implemented and fixture-tested, but no sysvinit-default distro is in the live matrix).",
			"The unit must already exist on the host — install the providing package first, typically via a `require: [package: <name>]` relationship.",
			"Out of scope for v0.1: separate mask/unmask, `reload`, restart-on-change, and unit listing/inspection.",
		},
	}
}
