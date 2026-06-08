// SPDX-License-Identifier: Apache-2.0

package bridge

import (
	"fmt"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"
)

const netdevKind = "bridge"

// stpWord renders a networkd boolean.
func stpWord(stp bool) string {
	if stp {
		return "yes"
	}
	return "no"
}

// renderNetdevSection is the bridge-specific `[Bridge]` block. STP is
// emitted explicitly (yes/no) so the declaration pins the value rather
// than relying on the backend default.
func renderNetdevSection(p *params) string {
	return fmt.Sprintf("[Bridge]\nSTP=%s\n", stpWord(p.STP))
}

// renderNetplan is the netplan `bridges:` stanza. netplan's `stp`
// default is true, so it is emitted explicitly to pin a declared false.
func renderNetplan(p *params) string {
	var b strings.Builder
	b.WriteString(netpersist.ManagedHeader)
	b.WriteString("network:\n  version: 2\n  bridges:\n")
	fmt.Fprintf(&b, "    %s:\n", p.Name)
	if len(p.Members) > 0 {
		b.WriteString("      interfaces:\n")
		for _, m := range p.Members {
			fmt.Fprintf(&b, "        - %s\n", m)
		}
	}
	b.WriteString("      parameters:\n")
	fmt.Fprintf(&b, "        stp: %t\n", p.STP)
	return b.String()
}
