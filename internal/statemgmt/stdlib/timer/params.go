// SPDX-License-Identifier: Apache-2.0

package timer

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

const (
	paramOnCalendar  = "on_calendar"
	paramService     = "service"
	paramPersistent  = "persistent"
	paramDescription = "description"
	paramEnable      = "enable"
	paramSeverity    = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramOnCalendar:  {},
	paramService:     {},
	paramPersistent:  {},
	paramDescription: {},
	paramEnable:      {},
	paramSeverity:    {},
}

// unitNameRE matches systemd's permissive unit-name charset
// (alphanumerics, underscore, dot, '@' for templated units, ':' and
// dash). It rejects whitespace, slashes and shell metacharacters so
// a hostile name can't reach `systemctl`.
var unitNameRE = regexp.MustCompile(`^[a-zA-Z0-9_.@:-]+$`)

// descriptionRE keeps the Description= value on a single line — it is
// written verbatim into the generated unit file.
func validDescription(s string) bool { return !strings.ContainsAny(s, "\r\n") }

type params struct {
	Name        string // Declaration.Name — the timer base name (→ "<name>.timer")
	State       string
	OnCalendar  string
	Service     string // unit the timer triggers; default "<name>.service"
	Persistent  bool
	Description string // default "Keystone-managed timer <name>"
	Enable      bool   // default true

	// seen records which params keys the declaration actually set,
	// so absent-state declarations can be rejected for carrying
	// attribute params without false positives on defaulted values.
	seen map[string]struct{}
}

func defaultDescription(name string) string { return "Keystone-managed timer " + name }
func defaultService(name string) string     { return name + ".service" }

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	seen := make(map[string]struct{}, len(decl.Params))
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: on_calendar, service, persistent, description, enable, severity)", k)
		}
		seen[k] = struct{}{}
	}
	p := &params{
		Name:        decl.Name,
		State:       decl.State,
		Service:     defaultService(decl.Name),
		Description: defaultDescription(decl.Name),
		Enable:      true,
		seen:        seen,
	}
	if raw, ok := decl.Params[paramOnCalendar]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("on_calendar: expected string, got %T", raw)
		}
		p.OnCalendar = s
	}
	if raw, ok := decl.Params[paramService]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("service: expected string, got %T", raw)
		}
		if s != "" {
			p.Service = s
		}
	}
	if raw, ok := decl.Params[paramPersistent]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("persistent: expected bool, got %T", raw)
		}
		p.Persistent = b
	}
	if raw, ok := decl.Params[paramDescription]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("description: expected string, got %T", raw)
		}
		if s != "" {
			p.Description = s
		}
	}
	if raw, ok := decl.Params[paramEnable]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("enable: expected bool, got %T", raw)
		}
		p.Enable = b
	}
	return p, nil
}

func (p *params) validate() error {
	if !unitNameRE.MatchString(p.Name) {
		return fmt.Errorf("invalid timer name %q (must match %s)", p.Name, unitNameRE)
	}
	switch p.State {
	case StatePresent:
		if strings.TrimSpace(p.OnCalendar) == "" {
			return fmt.Errorf("state=present requires on_calendar")
		}
		if strings.ContainsAny(p.OnCalendar, "\r\n") {
			return fmt.Errorf("on_calendar must be a single line")
		}
		if !unitNameRE.MatchString(p.Service) {
			return fmt.Errorf("invalid service unit %q (must match %s)", p.Service, unitNameRE)
		}
		if !validDescription(p.Description) {
			return fmt.Errorf("description must be a single line")
		}
	case StateAbsent:
		var leaked []string
		for _, k := range []string{paramOnCalendar, paramService, paramPersistent, paramDescription} {
			if _, ok := p.seen[k]; ok {
				leaked = append(leaked, k)
			}
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=absent cannot carry attribute params: %v", leaked)
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}
