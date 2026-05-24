// SPDX-License-Identifier: Apache-2.0

package firewall

import "sort"

// knownServices is the v1.0 service-name catalog. The static map
// keeps the abstraction's semantics identical on every backend —
// firewalld's own catalog and `/etc/services` would otherwise diverge
// across hosts (firewalld-native names like `dhcpv6-client`,
// distro-dependent `/etc/services` content, multi-port services like
// `samba`). Operators who need a service not listed here pass `port:
// PORT/PROTO` directly. Catalog expansion is V1X.
//
// Each entry is one port + one proto. Services that naturally need
// multiple — DNS (53/tcp + 53/udp) — are split into separate names
// (`dns-tcp` / `dns-udp`).
var knownServices = map[string]struct {
	Port  string
	Proto string
}{
	"ssh":        {"22", "tcp"},
	"http":       {"80", "tcp"},
	"https":      {"443", "tcp"},
	"smtp":       {"25", "tcp"},
	"smtps":      {"465", "tcp"},
	"submission": {"587", "tcp"},
	"imap":       {"143", "tcp"},
	"imaps":      {"993", "tcp"},
	"pop3":       {"110", "tcp"},
	"pop3s":      {"995", "tcp"},
	"ldap":       {"389", "tcp"},
	"ldaps":      {"636", "tcp"},
	"ntp":        {"123", "udp"},
	"dns-tcp":    {"53", "tcp"},
	"dns-udp":    {"53", "udp"},
	"mysql":      {"3306", "tcp"},
	"postgresql": {"5432", "tcp"},
	"redis":      {"6379", "tcp"},
	"ftp":        {"21", "tcp"},
}

// KnownServiceNames returns the catalog in sorted order. Used in
// error messages and exported so tests / docs tooling can enumerate
// the v1.0 catalog without reaching into the unexported map.
func KnownServiceNames() []string {
	names := make([]string, 0, len(knownServices))
	for n := range knownServices {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
