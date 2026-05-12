package service

import (
	"fmt"
	"regexp"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StateRunning = "running"
	StateStopped = "stopped"
)

const (
	paramEnable   = "enable"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramEnable:   {},
	paramSeverity: {},
}

// unitNameRE matches systemd's permissive unit-name charset
// (alphanumerics, underscore, dot, at-sign for templated units,
// colon, dash). Rejects shell metacharacters / spaces early so a
// hostile name can't reach `systemctl`.
var unitNameRE = regexp.MustCompile(`^[a-zA-Z0-9_.@:-]+$`)

type params struct {
	Name      string
	State     string
	Enable    bool
	HasEnable bool // distinguishes "leave boot-state alone" from "disable at boot"
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for key := range decl.Params {
		if _, ok := allowedKeys[key]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: enable, severity)", key)
		}
	}
	p := &params{Name: decl.Name, State: decl.State}
	if raw, ok := decl.Params[paramEnable]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("enable: expected bool, got %T", raw)
		}
		p.Enable = b
		p.HasEnable = true
	}
	return p, nil
}

func (p *params) validate() error {
	if !unitNameRE.MatchString(p.Name) {
		return fmt.Errorf("invalid service name %q (must match %s)", p.Name, unitNameRE)
	}
	return nil
}
