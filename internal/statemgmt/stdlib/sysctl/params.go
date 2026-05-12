package sysctl

import (
	"fmt"
	"regexp"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const StatePresent = "present"

const (
	paramValue    = "value"
	paramPersist  = "persist"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramValue:    {},
	paramPersist:  {},
	paramSeverity: {},
}

// keyRE matches sysctl key charsets in either the dotted
// (net.ipv4.ip_forward) or slashed (net/ipv4/ip_forward) form. Both
// are valid; the module normalises slashes → dots.
var keyRE = regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`)

type params struct {
	Key     string // normalised: slashes → dots
	Value   string
	Persist bool
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: value, persist, severity)", k)
		}
	}
	p := &params{Key: normalizeKey(decl.Name), Persist: true}
	if raw, ok := decl.Params[paramValue]; ok {
		s, ok := raw.(string)
		if !ok {
			// Accept ints/bools for ergonomics — sysctl values are
			// often "1" / "0" and YAML decodes those as int/bool.
			p.Value = fmt.Sprintf("%v", raw)
		} else {
			p.Value = s
		}
	}
	if raw, ok := decl.Params[paramPersist]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("persist: expected bool, got %T", raw)
		}
		p.Persist = b
	}
	return p, nil
}

// normalizeKey converts the slashed form to the dotted form so the
// two notations map to the same persistence file and compare equal.
func normalizeKey(name string) string {
	return strings.ReplaceAll(name, "/", ".")
}

func (p *params) validate() error {
	if !keyRE.MatchString(p.Key) {
		return fmt.Errorf("invalid sysctl key %q (must match %s)", p.Key, keyRE)
	}
	if p.Value == "" {
		return fmt.Errorf("value is required")
	}
	return nil
}
