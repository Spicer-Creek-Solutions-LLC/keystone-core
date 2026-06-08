// SPDX-License-Identifier: Apache-2.0

package bond

import (
	"fmt"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"
)

const netdevKind = "bond"

// renderNetdevSection is the bond-specific `[Bond]` block of the
// `.netdev` unit.
func renderNetdevSection(p *params) string {
	var b strings.Builder
	b.WriteString("[Bond]\n")
	fmt.Fprintf(&b, "Mode=%s\n", p.Mode)
	if p.HasMiimon {
		fmt.Fprintf(&b, "MIIMonitorSec=%dms\n", p.Miimon)
	}
	return b.String()
}

// renderNetplan is the netplan `bonds:` stanza: the device, its member
// interfaces inline, and its parameters.
func renderNetplan(p *params) string {
	var b strings.Builder
	b.WriteString(netpersist.ManagedHeader)
	b.WriteString("network:\n  version: 2\n  bonds:\n")
	fmt.Fprintf(&b, "    %s:\n", p.Name)
	if len(p.Members) > 0 {
		b.WriteString("      interfaces:\n")
		for _, m := range p.Members {
			fmt.Fprintf(&b, "        - %s\n", m)
		}
	}
	b.WriteString("      parameters:\n")
	fmt.Fprintf(&b, "        mode: %s\n", p.Mode)
	if p.HasMiimon {
		fmt.Fprintf(&b, "        mii-monitor-interval: %d\n", p.Miimon)
	}
	return b.String()
}
