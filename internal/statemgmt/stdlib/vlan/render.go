// SPDX-License-Identifier: Apache-2.0

package vlan

import (
	"fmt"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"
)

const netdevKind = "vlan"

// renderNetdevSection is the VLAN-specific `[VLAN]` block (the tag id).
func renderNetdevSection(p *params) string {
	return fmt.Sprintf("[VLAN]\nId=%d\n", p.ID)
}

// renderNetplan is the netplan `vlans:` stanza: the tag id and the
// parent link.
func renderNetplan(p *params) string {
	var b strings.Builder
	b.WriteString(netpersist.ManagedHeader)
	b.WriteString("network:\n  version: 2\n  vlans:\n")
	fmt.Fprintf(&b, "    %s:\n", p.Name)
	fmt.Fprintf(&b, "      id: %d\n", p.ID)
	fmt.Fprintf(&b, "      link: %s\n", p.Parent)
	return b.String()
}
