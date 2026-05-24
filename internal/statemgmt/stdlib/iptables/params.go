// SPDX-License-Identifier: Apache-2.0

package iptables

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

// Address families.
const (
	FamilyIPv4 = "ipv4"
	FamilyIPv6 = "ipv6"
)

const defaultTable = "filter"

const (
	paramChain    = "chain"
	paramRule     = "rule"
	paramTable    = "table"
	paramFamily   = "family"
	paramPosition = "position"
	paramSave     = "save"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramChain:    {},
	paramRule:     {},
	paramTable:    {},
	paramFamily:   {},
	paramPosition: {},
	paramSave:     {},
	paramSeverity: {},
}

var knownTables = map[string]struct{}{
	"filter": {}, "nat": {}, "mangle": {}, "raw": {}, "security": {},
}

// chainRE matches an iptables chain name (built-in or custom).
var chainRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// bannedRuleArgs are flags the `rule` must not include — the module
// supplies the table and the append/insert/delete/check action
// itself, so seeing one here is a double-spec mistake.
var bannedRuleArgs = map[string]struct{}{
	"-t": {}, "--table": {},
	"-A": {}, "--append": {},
	"-I": {}, "--insert": {},
	"-D": {}, "--delete": {},
	"-C": {}, "--check": {},
	"-R": {}, "--replace": {},
	"-N": {}, "--new-chain": {},
	"-P": {}, "--policy": {},
}

type params struct {
	Label    string // Declaration.Name — a human label (decl ID; not used for matching)
	State    string
	Chain    string
	Rule     []string
	Table    string // filter|nat|mangle|raw|security
	Family   string // ipv4|ipv6
	Position int    // 0 = append (-A); >=1 = insert at this position (-I)
	Save     string // optional path for iptables-save output

	seen map[string]struct{}
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	seen := make(map[string]struct{}, len(decl.Params))
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: chain, rule, table, family, position, save, severity)", k)
		}
		seen[k] = struct{}{}
	}
	p := &params{Label: decl.Name, State: decl.State, Table: defaultTable, Family: FamilyIPv4, seen: seen}
	if raw, ok := decl.Params[paramChain]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("chain: expected string, got %T", raw)
		}
		p.Chain = s
	}
	if raw, ok := decl.Params[paramRule]; ok {
		args, err := parseRule(raw)
		if err != nil {
			return nil, fmt.Errorf("rule: %w", err)
		}
		p.Rule = args
	}
	if raw, ok := decl.Params[paramTable]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("table: expected string, got %T", raw)
		}
		if s != "" {
			p.Table = s
		}
	}
	if raw, ok := decl.Params[paramFamily]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("family: expected %q or %q, got %T", FamilyIPv4, FamilyIPv6, raw)
		}
		if s != "" {
			p.Family = strings.ToLower(s)
		}
	}
	if raw, ok := decl.Params[paramPosition]; ok {
		n, err := parsePosition(raw)
		if err != nil {
			return nil, fmt.Errorf("position: %w", err)
		}
		p.Position = n
	}
	if raw, ok := decl.Params[paramSave]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("save: expected string, got %T", raw)
		}
		p.Save = s
	}
	return p, nil
}

// parseRule accepts the rule spec as a whitespace-split string or as
// a list of args. The result is a non-empty list of non-empty,
// single-line tokens.
func parseRule(raw any) ([]string, error) {
	var args []string
	switch v := raw.(type) {
	case string:
		args = strings.Fields(v)
	case []any:
		for i, e := range v {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("element %d: expected string, got %T", i, e)
			}
			args = append(args, s)
		}
	default:
		return nil, fmt.Errorf("expected a string or a list of strings, got %T", raw)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("empty")
	}
	for _, a := range args {
		if strings.TrimSpace(a) == "" {
			return nil, fmt.Errorf("empty argument")
		}
		if strings.ContainsAny(a, "\r\n") {
			return nil, fmt.Errorf("argument %q contains a newline", a)
		}
	}
	return args, nil
}

func parsePosition(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return checkPos(v)
	case int64:
		return checkPos(int(v))
	case float64:
		if v != float64(int64(v)) {
			return 0, fmt.Errorf("expected a whole number, got %v", v)
		}
		return checkPos(int(v))
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("expected an integer, got %q", v)
		}
		return checkPos(n)
	default:
		return 0, fmt.Errorf("expected an integer, got %T", raw)
	}
}

func checkPos(n int) (int, error) {
	if n < 0 {
		return 0, fmt.Errorf("must be >= 0, got %d", n)
	}
	return n, nil
}

func (p *params) validate() error {
	if strings.TrimSpace(p.Chain) == "" {
		return fmt.Errorf("chain is required")
	}
	if !chainRE.MatchString(p.Chain) {
		return fmt.Errorf("invalid chain name %q", p.Chain)
	}
	if _, ok := knownTables[p.Table]; !ok {
		return fmt.Errorf("table: must be one of filter, nat, mangle, raw, security; got %q", p.Table)
	}
	if p.Family != FamilyIPv4 && p.Family != FamilyIPv6 {
		return fmt.Errorf("family: must be %q or %q, got %q", FamilyIPv4, FamilyIPv6, p.Family)
	}
	if len(p.Rule) == 0 {
		return fmt.Errorf("rule is required (the match spec + target, e.g. \"-p tcp --dport 22 -j ACCEPT\")")
	}
	for _, a := range p.Rule {
		if _, banned := bannedRuleArgs[a]; banned {
			return fmt.Errorf("rule must not contain %q (the module supplies the table and the action; rule is the match spec + target only)", a)
		}
	}
	if strings.ContainsAny(p.Save, "\r\n") {
		return fmt.Errorf("save must be a single line")
	}
	if p.Save != "" && !strings.HasPrefix(p.Save, "/") {
		return fmt.Errorf("save %q must be an absolute path", p.Save)
	}
	switch p.State {
	case StatePresent:
		// position allowed.
	case StateAbsent:
		if _, ok := p.seen[paramPosition]; ok {
			return fmt.Errorf("state=absent cannot carry position (it only affects adding a rule)")
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}
