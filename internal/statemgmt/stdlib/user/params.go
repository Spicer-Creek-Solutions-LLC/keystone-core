package user

import (
	"fmt"
	"path/filepath"
	"regexp"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

const (
	paramUID        = "uid"
	paramGID        = "gid"
	paramGroup      = "group"
	paramHome       = "home"
	paramShell      = "shell"
	paramComment    = "comment"
	paramGroups     = "groups"
	paramSystem     = "system"
	paramCreateHome = "create_home"
	paramRemoveHome = "remove_home"
	paramSeverity   = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramUID:        {},
	paramGID:        {},
	paramGroup:      {},
	paramHome:       {},
	paramShell:      {},
	paramComment:    {},
	paramGroups:     {},
	paramSystem:     {},
	paramCreateHome: {},
	paramRemoveHome: {},
	paramSeverity:   {},
}

// Linux username convention. Matches the group module's rule so a
// user-named-after-group invariant doesn't get tripped up by
// inconsistent regexes.
var userNameRE = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// params is the typed view the Check / Apply paths consume.
type params struct {
	Name       string
	State      string
	UID        *int // nil = unspecified
	GID        *int // nil = unspecified (mutex with Group)
	Group      string
	Home       string
	Shell      string
	Comment    string
	Groups     []string // supplementary; nil = "no opinion"
	HasGroups  bool     // distinguishes "leave alone" (nil) from "make empty" ([])
	System     bool
	CreateHome bool
	RemoveHome bool
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for key := range decl.Params {
		if _, ok := allowedKeys[key]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: uid, gid, group, home, shell, comment, groups, system, create_home, remove_home, severity)", key)
		}
	}
	p := &params{Name: decl.Name, State: decl.State}
	if raw, ok := decl.Params[paramUID]; ok {
		n, err := coerceInt(raw)
		if err != nil {
			return nil, fmt.Errorf("uid: %w", err)
		}
		p.UID = &n
	}
	if raw, ok := decl.Params[paramGID]; ok {
		n, err := coerceInt(raw)
		if err != nil {
			return nil, fmt.Errorf("gid: %w", err)
		}
		p.GID = &n
	}
	if raw, ok := decl.Params[paramGroup]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("group: expected string, got %T", raw)
		}
		p.Group = s
	}
	if raw, ok := decl.Params[paramHome]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("home: expected string, got %T", raw)
		}
		p.Home = s
	}
	if raw, ok := decl.Params[paramShell]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("shell: expected string, got %T", raw)
		}
		p.Shell = s
	}
	if raw, ok := decl.Params[paramComment]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("comment: expected string, got %T", raw)
		}
		p.Comment = s
	}
	if raw, ok := decl.Params[paramGroups]; ok {
		list, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("groups: expected list, got %T", raw)
		}
		p.HasGroups = true
		p.Groups = make([]string, 0, len(list))
		for i, v := range list {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("groups[%d]: expected string, got %T", i, v)
			}
			p.Groups = append(p.Groups, s)
		}
	}
	if raw, ok := decl.Params[paramSystem]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("system: expected bool, got %T", raw)
		}
		p.System = b
	}
	if raw, ok := decl.Params[paramCreateHome]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("create_home: expected bool, got %T", raw)
		}
		p.CreateHome = b
	}
	if raw, ok := decl.Params[paramRemoveHome]; ok {
		b, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("remove_home: expected bool, got %T", raw)
		}
		p.RemoveHome = b
	}
	return p, nil
}

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
	if !userNameRE.MatchString(p.Name) {
		return fmt.Errorf("invalid user name %q (must match %s)", p.Name, userNameRE)
	}

	if p.State == StateAbsent {
		// Allow only remove_home + severity on absent; everything
		// else is a sign of operator confusion.
		var leaked []string
		if p.UID != nil {
			leaked = append(leaked, "uid")
		}
		if p.GID != nil {
			leaked = append(leaked, "gid")
		}
		if p.Group != "" {
			leaked = append(leaked, "group")
		}
		if p.Home != "" {
			leaked = append(leaked, "home")
		}
		if p.Shell != "" {
			leaked = append(leaked, "shell")
		}
		if p.Comment != "" {
			leaked = append(leaked, "comment")
		}
		if p.HasGroups {
			leaked = append(leaked, "groups")
		}
		if p.System {
			leaked = append(leaked, "system")
		}
		if p.CreateHome {
			leaked = append(leaked, "create_home")
		}
		if len(leaked) > 0 {
			return fmt.Errorf("state=absent cannot carry attribute params: %v (remove_home + severity are allowed)", leaked)
		}
		return nil
	}

	// state=present rules.
	if p.UID != nil {
		if *p.UID < 0 || *p.UID > (1<<31)-1 {
			return fmt.Errorf("uid: out of range, got %d", *p.UID)
		}
	}
	if p.GID != nil {
		if *p.GID < 0 || *p.GID > (1<<31)-1 {
			return fmt.Errorf("gid: out of range, got %d", *p.GID)
		}
	}
	if p.GID != nil && p.Group != "" {
		return fmt.Errorf("gid and group are mutually exclusive; pick one")
	}
	if p.Home != "" && !filepath.IsAbs(p.Home) {
		return fmt.Errorf("home must be an absolute path; got %q", p.Home)
	}
	if p.Shell != "" && !filepath.IsAbs(p.Shell) {
		return fmt.Errorf("shell must be an absolute path; got %q", p.Shell)
	}
	if p.RemoveHome {
		return fmt.Errorf("remove_home is only valid with state=absent")
	}
	return nil
}
