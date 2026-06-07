// SPDX-License-Identifier: Apache-2.0

package route

import (
	"fmt"
	"strconv"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"
)

// resolveBackend turns `persist: auto` into a concrete backend; an
// explicit networkd / netplan passes through.
func resolveBackend(p *params) (string, error) {
	if p.Persist == netpersist.Auto {
		return netpersist.DetectBackend()
	}
	return p.Persist, nil
}

// routeSlug is a filesystem-safe, per-route identifier (destination +
// table + metric) used for the drop-in / netplan filenames so each
// route gets its own file.
func routeSlug(p *params) string {
	s := sanitizeSlug(p.Destination) + "-" + sanitizeSlug(p.Table)
	if p.HasMetric {
		s += "-m" + strconv.Itoa(p.Metric)
	}
	return s
}

var slugReplacer = strings.NewReplacer(".", "-", ":", "-", "/", "_")

func sanitizeSlug(s string) string { return slugReplacer.Replace(s) }

// routePersistPath returns the file this route's persist manages on the
// backend: a networkd drop-in or a per-route netplan document.
func routePersistPath(backend string, p *params) (string, error) {
	switch backend {
	case netpersist.Networkd:
		return netpersist.NetworkDropinPath(p.Interface, routeSlug(p)), nil
	case netpersist.Netplan:
		return netpersist.NetplanRoutePath(routeSlug(p)), nil
	default:
		return "", fmt.Errorf("unsupported persist backend %q", backend)
	}
}

func renderRoute(backend string, p *params) (string, error) {
	switch backend {
	case netpersist.Networkd:
		return renderNetworkdRoute(p), nil
	case netpersist.Netplan:
		return renderNetplanRoute(p), nil
	default:
		return "", fmt.Errorf("unsupported persist backend %q", backend)
	}
}

// renderNetworkdRoute is the body of a systemd-networkd `.network`
// drop-in (`<iface>.network.d/<slug>.conf`) — networkd merges it into
// the interface's unit, so it coexists with the interface's address
// config and with other routes' drop-ins.
func renderNetworkdRoute(p *params) string {
	var b strings.Builder
	b.WriteString("# Managed by keystone-core (route module). Do not edit.\n")
	b.WriteString("[Route]\n")
	fmt.Fprintf(&b, "Destination=%s\n", p.Destination)
	if p.Gateway != "" {
		fmt.Fprintf(&b, "Gateway=%s\n", p.Gateway)
	}
	if p.HasMetric {
		fmt.Fprintf(&b, "Metric=%d\n", p.Metric)
	}
	if p.Table != defaultTable {
		fmt.Fprintf(&b, "Table=%s\n", p.Table)
	}
	return b.String()
}

// renderNetplanRoute is a per-route netplan document nesting one route
// under the interface. netplan merges by key, so the route document and
// the interface's address document combine; note that *multiple* routes
// on the same interface across separate netplan files conflict (netplan
// replaces a list rather than appending) — networkd drop-ins are the
// robust multi-route backend. netplan's `table:` is numeric, so a
// non-`main` named table must be given as its rt_tables number.
func renderNetplanRoute(p *params) string {
	var b strings.Builder
	b.WriteString("# Managed by keystone-core (route module). Do not edit.\n")
	b.WriteString("network:\n")
	b.WriteString("  version: 2\n")
	b.WriteString("  ethernets:\n")
	fmt.Fprintf(&b, "    %s:\n", p.Interface)
	b.WriteString("      routes:\n")
	fmt.Fprintf(&b, "        - to: %s\n", p.Destination)
	if p.Gateway != "" {
		fmt.Fprintf(&b, "          via: %s\n", p.Gateway)
	}
	if p.HasMetric {
		fmt.Fprintf(&b, "          metric: %d\n", p.Metric)
	}
	if p.Table != defaultTable {
		fmt.Fprintf(&b, "          table: %s\n", p.Table)
	}
	return b.String()
}

// minimalBase is a content-only `.network` unit ([Match] only) the route
// module writes (create-if-absent) so a route-only interface has a base
// for its drop-ins to merge into. It is never reconciled, so it doesn't
// fight the `network` module's fuller base for the same interface.
func minimalBase(iface string) string {
	return "# Managed by keystone-core (route module — base for drop-ins). Do not edit.\n" +
		"[Match]\n" +
		"Name=" + iface + "\n"
}
