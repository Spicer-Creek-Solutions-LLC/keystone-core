package timer

import (
	"fmt"
	"strings"
)

// managedHeader marks generated unit files so an operator (or a
// future drift check) can tell at a glance the file is keystone's.
const managedHeader = "# Managed by keystone-core. Do not edit.\n"

// renderTimerUnit produces the .timer unit file content for p. The
// output is fully determined by p, so Check compares it byte-for-byte
// against what is on disk — no parsing required.
func renderTimerUnit(p *params) string {
	var b strings.Builder
	b.WriteString(managedHeader)
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=%s\n", p.Description)
	b.WriteString("\n[Timer]\n")
	fmt.Fprintf(&b, "OnCalendar=%s\n", p.OnCalendar)
	if p.Persistent {
		b.WriteString("Persistent=true\n")
	}
	fmt.Fprintf(&b, "Unit=%s\n", p.Service)
	b.WriteString("\n[Install]\nWantedBy=timers.target\n")
	return b.String()
}

// timerUnitName is the systemd unit name for the declaration —
// "<name>.timer".
func timerUnitName(p *params) string { return p.Name + ".timer" }
