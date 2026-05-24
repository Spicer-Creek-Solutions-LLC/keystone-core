// SPDX-License-Identifier: Apache-2.0

package nftables

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

const defaultFamily = "inet"

// nft address families.
var knownFamilies = map[string]struct{}{
	"ip": {}, "ip6": {}, "inet": {}, "arp": {}, "bridge": {}, "netdev": {},
}

const (
	paramFamily   = "family"
	paramTable    = "table"
	paramChain    = "chain"
	paramRule     = "rule"
	paramIndex    = "index"
	paramSave     = "save"
	paramSeverity = statemgmt.ReservedSeverityParamKey
)

var allowedKeys = map[string]struct{}{
	paramFamily:   {},
	paramTable:    {},
	paramChain:    {},
	paramRule:     {},
	paramIndex:    {},
	paramSave:     {},
	paramSeverity: {},
}

// identRE matches an nft table / chain name.
var identRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// bannedRuleHeadWords are keywords the `rule` must not start with —
// the module supplies the `add rule` / `insert rule` verb and the
// family/table/chain itself, so a rule beginning with one of these is
// a double-spec mistake (or an attempt to smuggle a second command).
var bannedRuleHeadWords = map[string]struct{}{
	"add": {}, "insert": {}, "delete": {}, "replace": {}, "create": {},
	"destroy": {}, "rule": {}, "table": {}, "chain": {}, "set": {},
	"map": {}, "flush": {}, "list": {}, "reset": {}, "rename": {},
	"nft": {}, "include": {}, "define": {}, "import": {}, "export": {},
}

type params struct {
	Label  string // Declaration.Name — a human label (decl ID; not used for matching)
	State  string
	Family string   // ip|ip6|inet|arp|bridge|netdev
	Table  string   // an existing table
	Chain  string   // an existing chain in that table
	Rule   []string // the rule expression: match + statement, no verb/family/table/chain
	Index  int      // <0 = append (`nft add rule`); >=0 = insert at this 0-based index (`nft insert rule … index N`)
	Save   string   // optional absolute path for `nft list ruleset` output

	indexSet bool
}

func parseParams(decl *statemgmt.Declaration) (*params, error) {
	if decl == nil {
		return nil, fmt.Errorf("nil declaration")
	}
	for k := range decl.Params {
		if _, ok := allowedKeys[k]; !ok {
			return nil, fmt.Errorf("unknown param %q (allowed: family, table, chain, rule, index, save, severity)", k)
		}
	}
	p := &params{Label: decl.Name, State: decl.State, Family: defaultFamily, Table: "", Chain: "", Index: -1}
	if raw, ok := decl.Params[paramFamily]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("family: expected string, got %T", raw)
		}
		if s != "" {
			p.Family = strings.ToLower(strings.TrimSpace(s))
		}
	}
	if raw, ok := decl.Params[paramTable]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("table: expected string, got %T", raw)
		}
		p.Table = strings.TrimSpace(s)
	}
	if raw, ok := decl.Params[paramChain]; ok {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("chain: expected string, got %T", raw)
		}
		p.Chain = strings.TrimSpace(s)
	}
	if raw, ok := decl.Params[paramRule]; ok {
		args, err := parseRule(raw)
		if err != nil {
			return nil, fmt.Errorf("rule: %w", err)
		}
		p.Rule = args
	}
	if raw, ok := decl.Params[paramIndex]; ok {
		n, err := parseIndex(raw)
		if err != nil {
			return nil, fmt.Errorf("index: %w", err)
		}
		p.Index = n
		p.indexSet = true
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

// parseRule accepts the rule expression as a whitespace-split string
// or as a list of args. The result is a non-empty list of non-empty,
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
		if strings.Contains(a, ";") {
			return nil, fmt.Errorf("argument %q contains %q (statement separator not allowed in a single-rule spec)", a, ";")
		}
	}
	return args, nil
}

func parseIndex(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return checkIndex(v)
	case int64:
		return checkIndex(int(v))
	case float64:
		if v != float64(int64(v)) {
			return 0, fmt.Errorf("expected a whole number, got %v", v)
		}
		return checkIndex(int(v))
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("expected an integer, got %q", v)
		}
		return checkIndex(n)
	default:
		return 0, fmt.Errorf("expected an integer, got %T", raw)
	}
}

func checkIndex(n int) (int, error) {
	if n < 0 {
		return 0, fmt.Errorf("must be >= 0, got %d", n)
	}
	return n, nil
}

func (p *params) validate() error {
	if _, ok := knownFamilies[p.Family]; !ok {
		return fmt.Errorf("family: must be one of ip, ip6, inet, arp, bridge, netdev; got %q", p.Family)
	}
	if p.Table == "" {
		return fmt.Errorf("table is required")
	}
	if !identRE.MatchString(p.Table) {
		return fmt.Errorf("invalid table name %q", p.Table)
	}
	if p.Chain == "" {
		return fmt.Errorf("chain is required")
	}
	if !identRE.MatchString(p.Chain) {
		return fmt.Errorf("invalid chain name %q", p.Chain)
	}
	if len(p.Rule) == 0 {
		return fmt.Errorf("rule is required (the rule expression, e.g. \"tcp dport 22 accept\")")
	}
	if _, banned := bannedRuleHeadWords[strings.ToLower(p.Rule[0])]; banned {
		return fmt.Errorf("rule must not begin with %q (the module supplies the add/insert verb and the family/table/chain; rule is the match + statement only)", p.Rule[0])
	}
	if strings.ContainsAny(p.Save, "\r\n") {
		return fmt.Errorf("save must be a single line")
	}
	if p.Save != "" && !strings.HasPrefix(p.Save, "/") {
		return fmt.Errorf("save %q must be an absolute path", p.Save)
	}
	switch p.State {
	case StatePresent:
		// index allowed.
	case StateAbsent:
		if p.indexSet {
			return fmt.Errorf("state=absent cannot carry index (it only affects adding a rule)")
		}
	default:
		return fmt.Errorf("invalid state %q", p.State)
	}
	return nil
}
