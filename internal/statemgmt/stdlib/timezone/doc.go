// SPDX-License-Identifier: Apache-2.0

package timezone

import "go.keystone-core.io/keystone-core/internal/statemgmt"

// Doc returns the reference documentation for the timezone module.
// Rendered into the docs-site "State Modules" section by
// tools/gendocs/modules (regenerated via `make docs-sync`). Keep States
// in sync with ValidStates() — the generator enforces it.
func (m *Module) Doc() statemgmt.ModuleDoc {
	return statemgmt.ModuleDoc{
		Category: "System config",
		Summary: "Manages the system timezone. The declaration name is the desired IANA " +
			"zone (e.g. `America/New_York`, `UTC`, `Etc/GMT+5`); it takes no module " +
			"parameters. Idempotent: re-applying an already-set zone reports no change.",
		States: []statemgmt.StateDoc{
			{Name: "present", Desc: "The system timezone is set to the declared zone (`/etc/localtime` symlinks into the zoneinfo tree)."},
		},
		Params: []statemgmt.ParamDoc{},
		Examples: []statemgmt.ModuleExample{
			{
				Title: "Set the system timezone to UTC",
				YAML: `timezone:
  UTC:
    state: present`,
			},
			{
				Title: "Set a region/city zone",
				Desc:  "The declaration name carries the IANA zone name.",
				YAML: `timezone:
  America/New_York:
    state: present`,
			},
			{
				Title: "Order after a package install",
				Desc:  "The `require` requisite applies the zone after tzdata is present.",
				YAML: `timezone:
  Europe/London:
    state: present
    require:
      - package: tzdata`,
			},
		},
		Notes: []string{
			"Linux only; non-Linux platforms return an unsupported-OS error.",
			"`timedatectl` is used when present (validates the zone, updates the symlink and running offset); otherwise the module symlinks `/etc/localtime` to the zoneinfo file and writes `/etc/timezone`, which is what Alpine and systemd-less Debian expect.",
			"The zone must exist as `/usr/share/zoneinfo/<zone>` on the target host; a missing zone fails at apply time.",
			"Takes no module parameters beyond the reserved `severity` override; the zone is the declaration name.",
			"Out of scope for v0.1: NTP-sync coupling (`timedatectl set-ntp`), localtime copy mode (copying the zoneinfo file instead of symlinking), and macOS.",
		},
	}
}
