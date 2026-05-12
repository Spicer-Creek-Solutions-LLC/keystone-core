package link

import (
	"fmt"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// State string constants.
const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

// Link kind constants. The `kind` param selects between a symbolic
// link (the default) and a hard link.
const (
	KindSymlink = "symlink"
	KindHard    = "hard"
)

const (
	paramTarget   = "target"
	paramKind     = "kind"
	paramForce    = "force"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramTarget:   {},
	paramKind:     {},
	paramForce:    {},
	paramSeverity: {},
}

// params is the parsed view the Check/Apply paths consume. Path is
// Declaration.Name (the link location); Target is what it points at.
type params struct {
	Path   string
	State  string
	Target string
	Kind   string // KindSymlink or KindHard; defaults to KindSymlink
	Force  bool   // replace an existing non-matching file at Path
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: target, kind, force, severity)", k)
		}
	}
	p := &params{
		Path:  decl.Name,
		State: decl.State,
		Kind:  KindSymlink,
	}
	if raw, ok := decl.Params[paramTarget]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("target: expected string, got %T", raw)
		}
		p.Target = s
	}
	if raw, ok := decl.Params[paramKind]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("kind: expected %q or %q, got %T", KindSymlink, KindHard, raw)
		}
		p.Kind = s
	}
	if raw, ok := decl.Params[paramForce]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("force: expected bool, got %T", raw)
		}
		p.Force = b
	}
	return p, nil
}

func (p *params) validate() error {
	switch p.State {
	case StatePresent:
		if p.Target == "" {
			return fmt.Errorf("state=present requires target")
		}
		if p.Kind != KindSymlink && p.Kind != KindHard {
			return fmt.Errorf("kind: must be %q or %q, got %q", KindSymlink, KindHard, p.Kind)
		}
	case StateAbsent:
		var leaked []string
		if p.Target != "" {
			leaked = append(leaked, "target")
		}
		// kind is only meaningful when creating a link; reject it
		// on absent declarations to catch operator confusion. A
		// non-default value is the signal — KindSymlink is the
		// zero-meaning default we set unconditionally.
		if p.Kind != KindSymlink {
			leaked = append(leaked, "kind")
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=absent cannot carry attribute params: %v", leaked)
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}
