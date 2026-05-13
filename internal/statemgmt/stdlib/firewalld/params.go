package firewalld

import (
	"fmt"
	"regexp"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

const defaultZone = "public"

// Item kinds. The string is the suffix in `firewall-cmd
// --add-<kind>=` / `--remove-<kind>=` / `--query-<kind>=`.
const (
	KindService  = "service"
	KindPort     = "port"
	KindRichRule = "rich-rule"
)

const (
	paramZone     = "zone"
	paramService  = "service"
	paramPort     = "port"
	paramRichRule = "rich_rule"
	paramReload   = "reload"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramZone:     {},
	paramService:  {},
	paramPort:     {},
	paramRichRule: {},
	paramReload:   {},
	paramSeverity: {},
}

// zoneRE matches a firewalld zone name (alphanumeric + `_-`).
var zoneRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// serviceRE matches a firewalld service name (alphanumeric + `_-`,
// covering the stock set like `ssh`, `dhcpv6-client`, `mountd`).
var serviceRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// portRE matches PORT or PORT-PORT followed by /tcp|udp|sctp|dccp.
var portRE = regexp.MustCompile(`^\d+(-\d+)?/(tcp|udp|sctp|dccp)$`)

type params struct {
	Label  string // Declaration.Name — a human label (decl ID; not used for matching)
	State  string
	Zone   string
	Item   Item // the one item this declaration manages
	Reload bool // run `firewall-cmd --reload` after a change (default true)
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: zone, service, port, rich_rule, reload, severity)", k)
		}
	}
	p := &params{Label: decl.Name, State: decl.State, Zone: defaultZone, Reload: true}
	if raw, ok := decl.Params[paramZone]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("zone: expected string, got %T", raw)
		}
		if s != "" {
			p.Zone = strings.TrimSpace(s)
		}
	}
	if raw, ok := decl.Params[paramReload]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("reload: expected bool, got %T", raw)
		}
		p.Reload = b
	}
	// item: exactly one of service / port / rich_rule.
	var (
		kind, value string
		setCount    int
	)
	if raw, ok := decl.Params[paramService]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("service: expected string, got %T", raw)
		}
		kind, value, setCount = KindService, strings.TrimSpace(s), setCount+1
	}
	if raw, ok := decl.Params[paramPort]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("port: expected string, got %T", raw)
		}
		kind, value, setCount = KindPort, strings.TrimSpace(s), setCount+1
	}
	if raw, ok := decl.Params[paramRichRule]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("rich_rule: expected string, got %T", raw)
		}
		kind, value, setCount = KindRichRule, strings.TrimSpace(s), setCount+1
	}
	if setCount > 1 {
		return nil, fmt.Errorf("exactly one of service / port / rich_rule must be set (got %d)", setCount)
	}
	p.Item = Item{Kind: kind, Value: value}
	return p, nil
}

func (p *params) validate() error {
	if p.Zone == "" {
		return fmt.Errorf("zone is required")
	}
	if !zoneRE.MatchString(p.Zone) {
		return fmt.Errorf("invalid zone name %q", p.Zone)
	}
	if p.Item.Kind == "" {
		return fmt.Errorf("exactly one of service / port / rich_rule is required")
	}
	if p.Item.Value == "" {
		return fmt.Errorf("%s value is empty", paramKeyFor(p.Item.Kind))
	}
	if strings.ContainsAny(p.Item.Value, "\r\n\x00") {
		return fmt.Errorf("%s value contains a newline or NUL", paramKeyFor(p.Item.Kind))
	}
	switch p.Item.Kind {
	case KindService:
		if !serviceRE.MatchString(p.Item.Value) {
			return fmt.Errorf("invalid service name %q", p.Item.Value)
		}
	case KindPort:
		if !portRE.MatchString(p.Item.Value) {
			return fmt.Errorf("invalid port %q (want PORT[-PORT]/{tcp,udp,sctp,dccp})", p.Item.Value)
		}
	case KindRichRule:
		// Rich rules carry their own grammar; firewalld will reject
		// malformed ones at apply time. We only ensure single-line +
		// non-empty here.
	}
	switch p.State {
	case StatePresent, StateAbsent:
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}

// paramKeyFor maps an Item.Kind back to the decl param key, for
// human-readable errors.
func paramKeyFor(kind string) string {
	switch kind {
	case KindService:
		return paramService
	case KindPort:
		return paramPort
	case KindRichRule:
		return paramRichRule
	}
	return kind
}
