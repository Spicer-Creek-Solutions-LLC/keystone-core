// SPDX-License-Identifier: Apache-2.0

package network

import (
	"fmt"
	"sort"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"
)

// renderPersist produces the persistent-config file content for the
// given backend from the declared addresses + mtu. `up` is runtime-only
// and is not rendered. The output is deterministic (addresses sorted)
// so Check can compare it byte-for-byte against the on-disk file.
func renderPersist(backend string, p *params) (string, error) {
	switch backend {
	case netpersist.Networkd:
		return renderNetworkd(p), nil
	case netpersist.Netplan:
		return renderNetplan(p), nil
	default:
		return "", fmt.Errorf("unsupported persist backend %q", backend)
	}
}

// persistFilePath is the file this module manages for an interface on
// the given backend.
func persistFilePath(backend, iface string) (string, error) {
	switch backend {
	case netpersist.Networkd:
		return netpersist.NetworkPath(iface), nil
	case netpersist.Netplan:
		return netpersist.NetplanPath(iface), nil
	default:
		return "", fmt.Errorf("unsupported persist backend %q", backend)
	}
}

func sortedAddresses(p *params) []string {
	addrs := append([]string(nil), p.Addresses...)
	sort.Strings(addrs)
	return addrs
}

// renderNetworkd emits a systemd-networkd `.network` unit:
//
//	[Match]
//	Name=<iface>
//
//	[Network]
//	Address=<cidr>
//	…
//
//	[Link]
//	MTUBytes=<mtu>
func renderNetworkd(p *params) string {
	var b strings.Builder
	b.WriteString("# Managed by keystone-core (network module). Do not edit.\n")
	b.WriteString("[Match]\n")
	fmt.Fprintf(&b, "Name=%s\n", p.Interface)
	if p.HasAddresses {
		b.WriteString("\n[Network]\n")
		for _, a := range sortedAddresses(p) {
			fmt.Fprintf(&b, "Address=%s\n", a)
		}
	}
	if p.HasMTU {
		b.WriteString("\n[Link]\n")
		fmt.Fprintf(&b, "MTUBytes=%d\n", p.MTU)
	}
	return b.String()
}

// renderNetplan emits a netplan v2 document under `ethernets:` (the
// `network` module targets an existing interface — bond / bridge / vlan
// have their own modules and sections):
//
//	network:
//	  version: 2
//	  ethernets:
//	    <iface>:
//	      addresses:
//	        - <cidr>
//	      mtu: <mtu>
func renderNetplan(p *params) string {
	var b strings.Builder
	b.WriteString("# Managed by keystone-core (network module). Do not edit.\n")
	b.WriteString("network:\n")
	b.WriteString("  version: 2\n")
	b.WriteString("  ethernets:\n")
	fmt.Fprintf(&b, "    %s:\n", p.Interface)
	if p.HasAddresses {
		b.WriteString("      addresses:\n")
		for _, a := range sortedAddresses(p) {
			fmt.Fprintf(&b, "        - %s\n", a)
		}
	}
	if p.HasMTU {
		fmt.Fprintf(&b, "      mtu: %d\n", p.MTU)
	}
	return b.String()
}
