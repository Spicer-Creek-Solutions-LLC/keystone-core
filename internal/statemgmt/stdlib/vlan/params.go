package vlan

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

const (
	minVLANID = 1
	maxVLANID = 4094
)

const (
	paramName     = "name"
	paramParent   = "parent"
	paramID       = "id"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramName:     {},
	paramParent:   {},
	paramID:       {},
	paramSeverity: {},
}

var ifaceRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,15}$`)

type params struct {
	Label  string
	State  string
	Name   string
	Parent string
	ID     int
	HasID  bool
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: name, parent, id, severity)", k)
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
	if raw, ok := decl.Params[paramParent]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("parent: expected string, got %T", raw)
		}
		p.Parent = strings.TrimSpace(s)
	}
	if raw, ok := decl.Params[paramID]; ok {
		n, err := coerceInt(raw)
		if err != nil {
			return nil, fmt.Errorf("id: %w", err)
		}
		p.ID = n
		p.HasID = true
	}
	return p, nil
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
	if p.Name == "" {
		return fmt.Errorf("name: required")
	}
	if !ifaceRE.MatchString(p.Name) {
		return fmt.Errorf("name: %q is not a valid Linux interface name (max 15 chars; letters/digits/`._-` only)", p.Name)
	}
	if p.State == StatePresent {
		if p.Parent == "" {
			return fmt.Errorf("parent: required for state=present")
		}
		if !ifaceRE.MatchString(p.Parent) {
			return fmt.Errorf("parent: %q is not a valid Linux interface name", p.Parent)
		}
		if !p.HasID {
			return fmt.Errorf("id: required for state=present")
		}
		if p.ID < minVLANID || p.ID > maxVLANID {
			return fmt.Errorf("id: must be in [%d, %d]; got %d", minVLANID, maxVLANID, p.ID)
		}
	} else {
		// state=absent: parent / id ignored (only need to delete by name).
		// Reject explicit values to keep declarations meaningful.
		if p.Parent != "" {
			return fmt.Errorf("parent is only valid with state=present (absent deletes by name)")
		}
		if p.HasID {
			return fmt.Errorf("id is only valid with state=present (absent deletes by name)")
		}
	}
	return nil
}
