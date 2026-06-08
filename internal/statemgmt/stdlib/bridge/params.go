// SPDX-License-Identifier: Apache-2.0

package bridge

import (
	"fmt"
	"regexp"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/netpersist"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

const (
	paramName     = "name"
	paramMembers  = "members"
	paramSTP      = "stp"
	paramPersist  = "persist"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramName:     {},
	paramMembers:  {},
	paramSTP:      {},
	paramPersist:  {},
	paramSeverity: {},
}

var ifaceRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,15}$`)

type params struct {
	Label   string
	State   string
	Name    string
	Members []string
	STP     bool
	Persist string // "" = runtime-only; networkd | netplan | auto
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: name, members, stp, persist, severity)", k)
		}
	}
	p := &params{Label: decl.Name, State: decl.State}
	if raw, ok := decl.Params[paramName]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("name: expected string, got %T", raw)
		}
		p.Name = strings.TrimSpace(s)
	}
	if raw, ok := decl.Params[paramMembers]; ok {
		v, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("members: expected a list of interface names, got %T", raw)
		}
		out := make([]string, 0, len(v))
		for i, e := range v {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("members[%d]: expected string, got %T", i, e)
			}
			s = strings.TrimSpace(s)
			if s == "" {
				return nil, fmt.Errorf("members[%d]: empty", i)
			}
			out = append(out, s)
		}
		p.Members = out
	}
	if raw, ok := decl.Params[paramSTP]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("stp: expected bool, got %T", raw)
		}
		p.STP = b
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

func (p *params) validate() error {
	switch p.State {
	case StatePresent, StateAbsent:
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	if p.Name == "" {
		return fmt.Errorf("name: required")
	}
	if !ifaceRE.MatchString(p.Name) {
		return fmt.Errorf("name: %q is not a valid Linux interface name (max 15 chars; letters/digits/`._-` only)", p.Name)
	}
	for i, m := range p.Members {
		if !ifaceRE.MatchString(m) {
			return fmt.Errorf("members[%d]: %q is not a valid Linux interface name", i, m)
		}
	}
	if p.State == StateAbsent && len(p.Members) > 0 {
		return fmt.Errorf("members is only valid with state=present (absent deletes the bridge; ports are released automatically)")
	}
	if p.Persist != "" && !netpersist.ValidBackend(p.Persist) {
		return fmt.Errorf("persist: must be one of networkd, netplan, auto; got %q", p.Persist)
	}
	return nil
}
