// SPDX-License-Identifier: Apache-2.0

package firewall

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// portProto is one port (or PORT-PORT range, though the catalog uses
// single ports) bound to one protocol.
type portProto struct {
	Port  string
	Proto string
}

func (pp portProto) String() string { return pp.Port + "/" + pp.Proto }

// knownServices is the curated service-name catalog. A static map
// keeps the abstraction's semantics identical on every backend —
// firewalld's own catalog and `/etc/services` would otherwise diverge
// across hosts. Each name maps to one or more port/proto pairs, so a
// single declaration can open a multi-port service (e.g. `samba`)
// atomically. Operators who need a name not listed here either pass
// `port: PORT/PROTO` directly or opt into the host's `/etc/services`
// with `strict_catalog: false`.
//
// The set is intentionally curated (high-value services), not a mirror
// of firewalld's ~150 built-ins. Note: port-based rules are an
// approximation of firewalld's richer native services (e.g.
// `dhcpv6-client` is an IPv6 link-local rule in firewalld; here it is
// 546/udp opened on whatever families the backend covers) — operators
// who need firewalld's exact rich semantics use the firewalld backend
// module directly.
var knownServices = map[string][]portProto{
	// --- core (single-port) ---
	"ssh":        {{"22", "tcp"}},
	"http":       {{"80", "tcp"}},
	"https":      {{"443", "tcp"}},
	"smtp":       {{"25", "tcp"}},
	"smtps":      {{"465", "tcp"}},
	"submission": {{"587", "tcp"}},
	"imap":       {{"143", "tcp"}},
	"imaps":      {{"993", "tcp"}},
	"pop3":       {{"110", "tcp"}},
	"pop3s":      {{"995", "tcp"}},
	"ldap":       {{"389", "tcp"}},
	"ldaps":      {{"636", "tcp"}},
	"ntp":        {{"123", "udp"}},
	"dns-tcp":    {{"53", "tcp"}},
	"dns-udp":    {{"53", "udp"}},
	"mysql":      {{"3306", "tcp"}},
	"postgresql": {{"5432", "tcp"}},
	"redis":      {{"6379", "tcp"}},
	"ftp":        {{"21", "tcp"}},

	// --- curated firewalld-native additions ---
	"dhcp":          {{"67", "udp"}},
	"dhcpv6-client": {{"546", "udp"}},
	"cockpit":       {{"9090", "tcp"}},
	"nfs":           {{"2049", "tcp"}},
	"tftp":          {{"69", "udp"}},
	"snmp":          {{"161", "udp"}},
	"mdns":          {{"5353", "udp"}},
	"ipp":           {{"631", "tcp"}},

	// --- multi-port ---
	"dns":      {{"53", "tcp"}, {"53", "udp"}},
	"kerberos": {{"88", "tcp"}, {"88", "udp"}},
	"mountd":   {{"20048", "tcp"}, {"20048", "udp"}},
	"samba":    {{"137", "udp"}, {"138", "udp"}, {"139", "tcp"}, {"445", "tcp"}},
}

// KnownServiceNames returns the catalog in sorted order. Used in
// error messages and exported so tests / docs tooling can enumerate
// the catalog without reaching into the unexported map.
func KnownServiceNames() []string {
	names := make([]string, 0, len(knownServices))
	for n := range knownServices {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// servicesFilePath is the host services database, overridable in tests.
var servicesFilePath = "/etc/services"

// looseProtos is the protocol set the `/etc/services` fallback accepts
// — the same set `port:` accepts. Other protocols on a services line
// (e.g. `ddp`) are skipped.
var looseProtos = map[string]struct{}{
	"tcp": {}, "udp": {}, "sctp": {}, "dccp": {},
}

// lookupServicesFile resolves a service name against an /etc/services-
// format file: lines are `canonical-name port/proto [aliases...]` with
// optional `#` comments. The name matches the canonical name or any
// alias; every matching line contributes its port/proto (so a name
// listed under both tcp and udp resolves to two ports). Returns an
// error if the name resolves to nothing.
func lookupServicesFile(path, name string) ([]portProto, error) {
	f, err := os.Open(path) //nolint:gosec // path is the fixed servicesFilePath seam (/etc/services); test-overridable
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var out []portProto
	seen := map[string]struct{}{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		canonical, portproto, aliases := fields[0], fields[1], fields[2:]
		if !nameMatches(name, canonical, aliases) {
			continue
		}
		pp, ok := parseServicesPortProto(portproto)
		if !ok {
			continue
		}
		key := pp.String()
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, pp)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("service %q not found in %s", name, path)
	}
	// Deterministic order for stable rule generation / test assertions.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			pi, _ := strconv.Atoi(out[i].Port)
			pj, _ := strconv.Atoi(out[j].Port)
			return pi < pj
		}
		return out[i].Proto < out[j].Proto
	})
	return out, nil
}

func nameMatches(want, canonical string, aliases []string) bool {
	if want == canonical {
		return true
	}
	for _, a := range aliases {
		if want == a {
			return true
		}
	}
	return false
}

// parseServicesPortProto parses an /etc/services "port/proto" token.
// ok is false for a malformed token, an out-of-range port, or a
// protocol outside the supported set.
func parseServicesPortProto(tok string) (portProto, bool) {
	slash := strings.IndexByte(tok, '/')
	if slash <= 0 || slash == len(tok)-1 {
		return portProto{}, false
	}
	port, proto := tok[:slash], strings.ToLower(tok[slash+1:])
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return portProto{}, false
	}
	if _, ok := looseProtos[proto]; !ok {
		return portProto{}, false
	}
	return portProto{Port: port, Proto: proto}, true
}
