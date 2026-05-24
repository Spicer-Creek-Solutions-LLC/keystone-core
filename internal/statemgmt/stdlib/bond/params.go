// SPDX-License-Identifier: Apache-2.0

package bond

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

const (
	StatePresent = "present"
	StateAbsent  = "absent"
)

// validModes maps every accepted form (numeric + named) to its
// canonical kernel name. The kernel accepts both forms in `ip link
// add … type bond mode <m>`; we canonicalise to the name for
// readability.
var validModes = map[string]string{
	"0": "balance-rr", "balance-rr": "balance-rr",
	"1": "active-backup", "active-backup": "active-backup",
	"2": "balance-xor", "balance-xor": "balance-xor",
	"3": "broadcast", "broadcast": "broadcast",
	"4": "802.3ad", "802.3ad": "802.3ad",
	"5": "balance-tlb", "balance-tlb": "balance-tlb",
	"6": "balance-alb", "balance-alb": "balance-alb",
}

// KnownModes returns the canonical names in sorted order. Used in
// error messages and exported for docs tooling.
func KnownModes() []string {
	seen := map[string]struct{}{}
	for _, v := range validModes {
		seen[v] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

const defaultMode = "balance-rr"

const (
	paramName     = "name"
	paramMode     = "mode"
	paramMembers  = "members"
	paramMiimon   = "miimon"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramName:     {},
	paramMode:     {},
	paramMembers:  {},
	paramMiimon:   {},
	paramSeverity: {},
}

// ifaceRE matches a Linux interface name (max 15 chars).
var ifaceRE = regexp.MustCompile(`^[A-Za-z0-9._-]{1,15}$`)

type params struct {
	Label     string
	State     string
	Name      string
	Mode      string
	Members   []string
	Miimon    int
	HasMiimon bool
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: name, mode, members, miimon, severity)", k)
		}
	}
	p := &params{Label: decl.Name, State: decl.State, Mode: defaultMode}
	if raw, ok := decl.Params[paramName]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("name: expected string, got %T", raw)
		}
		p.Name = strings.TrimSpace(s)
	}
	if raw, ok := decl.Params[paramMode]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("mode: expected string, got %T", raw)
		}
		s = strings.TrimSpace(s)
		if canon, ok := validModes[s]; ok {
			p.Mode = canon
		} else {
			return nil, fmt.Errorf("mode: %q is not a valid bond mode (allowed: %s, or 0-6)", s, strings.Join(KnownModes(), ", "))
		}
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
	if raw, ok := decl.Params[paramMiimon]; ok {
		n, err := coerceInt(raw)
		if err != nil {
			return nil, fmt.Errorf("miimon: %w", err)
		}
		p.Miimon = n
		p.HasMiimon = true
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
	for i, m := range p.Members {
		if !ifaceRE.MatchString(m) {
			return fmt.Errorf("members[%d]: %q is not a valid Linux interface name", i, m)
		}
	}
	if p.HasMiimon && p.Miimon < 0 {
		return fmt.Errorf("miimon: must be >= 0; got %d", p.Miimon)
	}
	if p.State == StateAbsent {
		if len(p.Members) > 0 {
			return fmt.Errorf("members is only valid with state=present (absent deletes the bond; members are released automatically)")
		}
	}
	return nil
}
