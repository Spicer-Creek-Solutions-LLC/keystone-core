// SPDX-License-Identifier: Apache-2.0

package route

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

const defaultTable = "main"

const (
	paramDestination = "destination"
	paramGateway     = "gateway"
	paramInterface   = "interface"
	paramMetric      = "metric"
	paramTable       = "table"
	paramPersist     = "persist"
	paramSeverity    = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramDestination: {},
	paramGateway:     {},
	paramInterface:   {},
	paramMetric:      {},
	paramTable:       {},
	paramPersist:     {},
	paramSeverity:    {},
}

// ifaceRE matches a Linux interface name; same charset/length as
// the network module's check.
var ifaceRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,15}$`)

// tableRE matches an rt_tables entry: either a name (alphanumeric +
// `_.-`) or a decimal integer. The kernel's `rt_tables(5)`
// recognises four built-ins plus operator-defined names.
var tableRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type params struct {
	Label       string
	State       string
	Destination string // canonicalised CIDR (host bits preserved for net routes; host route ok)
	Gateway     string // canonicalised IP (no prefix) or "" if unset
	Interface   string
	Metric      int
	HasMetric   bool
	Table       string // "main" by default
	Persist     string // "" = runtime-only; networkd | netplan | auto
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: destination, gateway, interface, metric, table, severity)", k)
		}
	}
	p := &params{Label: decl.Name, State: decl.State, Table: defaultTable}
	if raw, ok := decl.Params[paramDestination]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("destination: expected string, got %T", raw)
		}
		canon, err := canonicalDestination(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("destination: %w", err)
		}
		p.Destination = canon
	}
	if raw, ok := decl.Params[paramGateway]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("gateway: expected string, got %T", raw)
		}
		s = strings.TrimSpace(s)
		if s != "" {
			canon, err := canonicalIP(s)
			if err != nil {
				return nil, fmt.Errorf("gateway: %w", err)
			}
			p.Gateway = canon
		}
	}
	if raw, ok := decl.Params[paramInterface]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("interface: expected string, got %T", raw)
		}
		p.Interface = strings.TrimSpace(s)
	}
	if raw, ok := decl.Params[paramMetric]; ok {
		n, err := coerceInt(raw)
		if err != nil {
			return nil, fmt.Errorf("metric: %w", err)
		}
		p.Metric = n
		p.HasMetric = true
	}
	if raw, ok := decl.Params[paramTable]; ok {
		switch v := raw.(type) {
		case string:
			p.Table = strings.TrimSpace(v)
		case int:
			p.Table = strconv.Itoa(v)
		case int64:
			p.Table = strconv.FormatInt(v, 10)
		case float64:
			if v != float64(int64(v)) {
				return nil, fmt.Errorf("table: expected an integer or string, got %v", v)
			}
			p.Table = strconv.FormatInt(int64(v), 10)
		default:
			return nil, fmt.Errorf("table: expected string or int, got %T", raw)
		}
		if p.Table == "" {
			p.Table = defaultTable
		}
	}
	if raw, ok := decl.Params[paramPersist]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("persist: expected string, got %T", raw)
		}
		p.Persist = strings.ToLower(strings.TrimSpace(s))
	}
	return p, nil
}

// canonicalDestination parses a CIDR (host or network route) or a
// bare IP (treated as a /32 or /128 host route) and returns the
// canonical CIDR form.
func canonicalDestination(s string) (string, error) {
	if pfx, err := netip.ParsePrefix(s); err == nil {
		return pfx.String(), nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return "", fmt.Errorf("not a CIDR or IP (%w)", err)
	}
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	return netip.PrefixFrom(addr, bits).String(), nil
}

func canonicalIP(s string) (string, error) {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return "", fmt.Errorf("not an IP (%w)", err)
	}
	return addr.String(), nil
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
	switch p.State {
	case StatePresent, StateAbsent:
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	if p.Destination == "" {
		return fmt.Errorf("destination: required")
	}
	if p.Interface != "" && !ifaceRE.MatchString(p.Interface) {
		return fmt.Errorf("interface: %q is not a valid Linux interface name", p.Interface)
	}
	if p.HasMetric && (p.Metric < 0 || p.Metric > 0xFFFFFFFF) {
		return fmt.Errorf("metric: out of range; got %d", p.Metric)
	}
	if p.Table != "" && !tableRE.MatchString(p.Table) {
		return fmt.Errorf("table: %q contains unsupported characters", p.Table)
	}
	if p.State == StatePresent {
		if p.Gateway == "" && p.Interface == "" {
			return fmt.Errorf("present requires at least one of gateway / interface")
		}
	}
	if p.Persist != "" {
		if !netpersist.ValidBackend(p.Persist) {
			return fmt.Errorf("persist: must be one of networkd, netplan, auto; got %q", p.Persist)
		}
		// Both renderers key the route to an output interface (the
		// networkd drop-in lives under <iface>.network.d/; the netplan
		// route is nested under the interface). A gateway-only route has
		// no interface to attach to in the persistent config.
		if p.Interface == "" {
			return fmt.Errorf("persist requires interface (the route's output interface; a gateway-only route can't be rendered to networkd/netplan)")
		}
	}
	return nil
}
