package network

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const StatePresent = "present"

const (
	minMTU = 68    // IPv4 minimum from RFC 791
	maxMTU = 65535 // 16-bit unsigned ceiling
)

const (
	paramInterface = "interface"
	paramAddresses = "addresses"
	paramMTU       = "mtu"
	paramUp        = "up"
	paramSeverity  = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramInterface: {},
	paramAddresses: {},
	paramMTU:       {},
	paramUp:        {},
	paramSeverity:  {},
}

// ifaceRE matches a Linux interface name: letters, digits, plus
// `.`/`_`/`-`. Length 1-15 (IFNAMSIZ-1). VLAN suffix `.<id>` and
// bridge / bond naming conventions all fit.
var ifaceRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,15}$`)

type params struct {
	Label        string
	State        string
	Interface    string
	Addresses    []string // canonicalised CIDR strings
	HasAddresses bool
	MTU          int
	HasMTU       bool
	Up           bool
	HasUp        bool
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: interface, addresses, mtu, up, severity)", k)
		}
	}
	p := &params{Label: decl.Name, State: decl.State}
	if raw, ok := decl.Params[paramInterface]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("interface: expected string, got %T", raw)
		}
		p.Interface = strings.TrimSpace(s)
	}
	if raw, ok := decl.Params[paramAddresses]; ok {
		addrs, err := parseAddressList(raw)
		if err != nil {
			return nil, fmt.Errorf("addresses: %w", err)
		}
		p.Addresses = addrs
		p.HasAddresses = true
	}
	if raw, ok := decl.Params[paramMTU]; ok {
		n, err := coerceInt(raw)
		if err != nil {
			return nil, fmt.Errorf("mtu: %w", err)
		}
		p.MTU = n
		p.HasMTU = true
	}
	if raw, ok := decl.Params[paramUp]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("up: expected bool, got %T", raw)
		}
		p.Up = b
		p.HasUp = true
	}
	return p, nil
}

func parseAddressList(raw any) ([]string, error) {
	v, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a list of CIDR strings, got %T", raw)
	}
	out := make([]string, 0, len(v))
	for i, e := range v {
		s, ok := e.(string)
		if !ok {
			return nil, fmt.Errorf("element %d: expected string, got %T", i, e)
		}
		s = strings.TrimSpace(s)
		canon, err := canonicalCIDR(s)
		if err != nil {
			return nil, fmt.Errorf("element %d (%q): %w", i, s, err)
		}
		out = append(out, canon)
	}
	return out, nil
}

// canonicalCIDR parses an IPv4 / IPv6 CIDR (host bits preserved) and
// returns its canonical form (`netip.Prefix.String()`). Used both at
// parse-time for the operator's input and by the Provider to
// normalise the kernel's response.
func canonicalCIDR(s string) (string, error) {
	pfx, err := netip.ParsePrefix(s)
	if err != nil {
		return "", fmt.Errorf("not a CIDR (%w)", err)
	}
	return pfx.String(), nil
}

// isLinkLocal reports whether a canonical CIDR string covers a
// kernel-auto-assigned link-local address (IPv6 `fe80::/10` or IPv4
// `169.254.0.0/16`). These are never auto-removed by the
// reconciliation.
func isLinkLocal(canonical string) bool {
	pfx, err := netip.ParsePrefix(canonical)
	if err != nil {
		return false
	}
	return pfx.Addr().IsLinkLocalUnicast()
}

func coerceInt(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		if v != float64(int64(v)) {
			return 0, fmt.Errorf("expected a whole number, got %v", v)
		}
		return int(v), nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("expected an integer, got %q", v)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("expected an integer, got %T", raw)
	}
}

func (p *params) validate() error {
	if p.State != StatePresent {
		return fmt.Errorf("state must be %q (got %q) — interface removal is not supported in v1.0", StatePresent, p.State)
	}
	if p.Interface == "" {
		return fmt.Errorf("interface: required")
	}
	if !ifaceRE.MatchString(p.Interface) {
		return fmt.Errorf("interface: %q is not a valid Linux interface name (max 15 chars; letters/digits/`._-` only)", p.Interface)
	}
	if !p.HasAddresses && !p.HasMTU && !p.HasUp {
		return fmt.Errorf("at least one of addresses / mtu / up must be set — a bare `interface:` declaration is a no-op")
	}
	if p.HasMTU && (p.MTU < minMTU || p.MTU > maxMTU) {
		return fmt.Errorf("mtu: must be in [%d, %d]; got %d", minMTU, maxMTU, p.MTU)
	}
	return nil
}
