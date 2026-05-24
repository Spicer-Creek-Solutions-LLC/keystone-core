// SPDX-License-Identifier: Apache-2.0

package group

import (
	"fmt"
	"regexp"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

const (
	paramGID      = "gid"
	paramSystem   = "system"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramGID:      {},
	paramSystem:   {},
	paramSeverity: {},
}

// Linux group-name convention: 1–32 chars, starts with a lower-case
// letter or underscore, then lowercase / digits / underscore / dash.
// `groupadd` enforces a similar regex; we pre-validate so the error
// surfaces from the module rather than as a confusing `groupadd`
// exit code.
var groupNameRE = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// params is the typed view the Check / Apply paths consume.
type params struct {
	Name   string
	State  string
	GID    *int // nil = unspecified
	System bool
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for key := range decl.Params {
		if _, ok := allowedKeys[key]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: gid, system, severity)", key)
		}
	}
	p := &params{Name: decl.Name, State: decl.State}
	if raw, ok := decl.Params[paramGID]; ok {
		n, err := coerceInt(raw)
		if err != nil {
			return nil, fmt.Errorf("gid: %w", err)
		}
		p.GID = &n
	}
	if raw, ok := decl.Params[paramSystem]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("system: expected bool, got %T", raw)
		}
		p.System = b
	}
	return p, nil
}

// coerceInt accepts the integer shapes yaml.v3 decodes.
func coerceInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("expected integer, got %v", v)
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("expected integer, got %T", v)
	}
}

func (p *params) validate() error {
	if !groupNameRE.MatchString(p.Name) {
		return fmt.Errorf("invalid group name %q (must match %s)", p.Name, groupNameRE)
	}
	if p.GID != nil {
		if *p.GID < 0 {
			return fmt.Errorf("gid: must be non-negative, got %d", *p.GID)
		}
		// 32-bit unsigned upper bound — Linux gids are uint32.
		if *p.GID > (1<<31)-1 {
			return fmt.Errorf("gid: out of range, got %d", *p.GID)
		}
	}
	if p.State == StateAbsent {
		var leaked []string
		if p.GID != nil {
			leaked = append(leaked, "gid")
		}
		if p.System {
			leaked = append(leaked, "system")
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=absent cannot carry attribute params: %v", leaked)
		}
	}
	return nil
}
