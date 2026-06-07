// SPDX-License-Identifier: Apache-2.0

package firewall

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

const defaultZone = "public"

const (
	paramService       = "service"
	paramPort          = "port"
	paramBackend       = "backend"
	paramZone          = "zone"
	paramStrictCatalog = "strict_catalog"
	paramSeverity      = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramService:       {},
	paramPort:          {},
	paramBackend:       {},
	paramZone:          {},
	paramStrictCatalog: {},
	paramSeverity:      {},
}

var zoneRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// portRE matches PORT or PORT-PORT followed by /tcp|udp|sctp|dccp.
var portRE = regexp.MustCompile(`^(\d+)(?:-(\d+))?/(tcp|udp|sctp|dccp)$`)

var validBackends = map[string]struct{}{
	BackendIptables:  {},
	BackendNftables:  {},
	BackendFirewalld: {},
}

type params struct {
	Label   string // Declaration.Name — a human label (decl ID; not used for matching)
	State   string
	Backend string // "" means auto-detect
	Zone    string // firewalld backend only — default "public"

	// Resolved port set (from a `service:` name — possibly multi-port —
	// or a single `port:` spec). Each Port is "PORT" or "PORT-PORT"
	// with no protocol suffix; Proto is tcp | udp | sctp | dccp.
	Ports []portProto
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: service, port, backend, zone, severity)", k)
		}
	}
	p := &params{Label: decl.Name, State: decl.State, Zone: defaultZone}
	if raw, ok := decl.Params[paramBackend]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("backend: expected string, got %T", raw)
		}
		p.Backend = strings.TrimSpace(s)
	}
	// strict_catalog (default true): when false, a `service:` name not
	// in the static catalog falls back to the host's /etc/services.
	strict := true
	if raw, ok := decl.Params[paramStrictCatalog]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("strict_catalog: expected bool, got %T", raw)
		}
		strict = b
	}
	if raw, ok := decl.Params[paramZone]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("zone: expected string, got %T", raw)
		}
		if s != "" {
			p.Zone = strings.TrimSpace(s)
		}
	}
	// Exactly one of service / port must be set.
	set := 0
	if raw, ok := decl.Params[paramService]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("service: expected string, got %T", raw)
		}
		name := strings.TrimSpace(s)
		if name == "" {
			return nil, fmt.Errorf("service: empty")
		}
		ports, err := resolveService(name, strict)
		if err != nil {
			return nil, err
		}
		p.Ports = ports
		set++
	}
	if raw, ok := decl.Params[paramPort]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("port: expected string, got %T", raw)
		}
		port, proto, err := parsePortSpec(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("port: %w", err)
		}
		p.Ports = []portProto{{Port: port, Proto: proto}}
		set++
	}
	if set != 1 {
		return nil, fmt.Errorf("exactly one of service / port must be set (got %d)", set)
	}
	return p, nil
}

// resolveService turns a service name into its port set: the static
// catalog first, then — only when strict is false — the host's
// /etc/services. A strict miss, or a loose miss, is an error that
// names the catalog and the escape hatches.
func resolveService(name string, strict bool) ([]portProto, error) {
	if entry, ok := knownServices[name]; ok {
		return entry, nil
	}
	if strict {
		return nil, fmt.Errorf("service %q is not in the catalog (catalog: %s) — use port: PORT/PROTO, or set strict_catalog: false to fall back to /etc/services", name, strings.Join(KnownServiceNames(), ", "))
	}
	ports, err := lookupServicesFile(servicesFilePath, name)
	if err != nil {
		return nil, fmt.Errorf("service %q is not in the catalog and the /etc/services fallback failed: %w", name, err)
	}
	return ports, nil
}

// parsePortSpec parses "PORT/PROTO" or "PORT-PORT/PROTO".
func parsePortSpec(s string) (port, proto string, err error) {
	m := portRE.FindStringSubmatch(s)
	if m == nil {
		return "", "", fmt.Errorf("expected PORT[-PORT]/{tcp,udp,sctp,dccp}, got %q", s)
	}
	low := m[1]
	high := m[2] // may be empty
	proto = m[3]
	lowN, err := strconv.Atoi(low)
	if err != nil || lowN < 1 || lowN > 65535 {
		return "", "", fmt.Errorf("port out of range: %s", low)
	}
	if high == "" {
		return low, proto, nil
	}
	highN, err := strconv.Atoi(high)
	if err != nil || highN < 1 || highN > 65535 {
		return "", "", fmt.Errorf("port out of range: %s", high)
	}
	if highN <= lowN {
		return "", "", fmt.Errorf("port range must be ascending: %s-%s", low, high)
	}
	return low + "-" + high, proto, nil
}

func (p *params) validate() error {
	if p.Backend != "" {
		if _, ok := validBackends[p.Backend]; !ok {
			return fmt.Errorf("backend: must be one of iptables, nftables, firewalld; got %q", p.Backend)
		}
	}
	if p.Zone == "" || !zoneRE.MatchString(p.Zone) {
		return fmt.Errorf("invalid zone %q", p.Zone)
	}
	if len(p.Ports) == 0 {
		return fmt.Errorf("port unresolved (internal: parseParams should have failed)")
	}
	switch p.State {
	case StatePresent, StateAbsent:
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}

// firewalldPortValue formats one port for `firewall-cmd --add-port=…`
// — "22/tcp" or "1000-2000/tcp".
func firewalldPortValue(pp portProto) string {
	return pp.Port + "/" + pp.Proto
}

// iptablesRule formats the rule body for the iptables module for one
// port. The module accepts a list (used as-is) — we pass a list so the
// validated tokens never need quote-aware re-parsing. iptables uses
// ':' (not '-') for `--dport` ranges.
func iptablesRule(pp portProto) []any {
	dport := pp.Port
	if i := strings.IndexByte(dport, '-'); i >= 0 {
		dport = dport[:i] + ":" + dport[i+1:]
	}
	return []any{"-p", pp.Proto, "--dport", dport, "-j", "ACCEPT"}
}

// nftablesRule formats the rule body for the nftables module for one
// port. nft uses '-' for `dport` ranges, matching our internal form.
func nftablesRule(pp portProto) string {
	return fmt.Sprintf("%s dport %s accept", pp.Proto, pp.Port)
}
