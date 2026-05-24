// SPDX-License-Identifier: Apache-2.0

// Package nftables implements the `nftables` stdlib state module —
// managing a single nftables rule, per PROJECT-DETAILS §4.8 (Firewall
// category). It is the nft-native sibling of the `iptables` module;
// the two manage different rulesets and do not coordinate.
//
// A rule has no name — its identity is its text inside a chain
// (family + table + chain + the rule expression). Declaration.Name is
// therefore just a human label (the decl ID); the rule comes from the
// `family` + `table` + `chain` + `rule` params.
//
// State semantics:
//
//	present — the chain contains at least one rule whose canonical
//	          text equals `rule`. Apply appends it (`nft add rule`)
//	          or, when `index:` is set, inserts it at that 0-based
//	          position (`nft insert rule … index N`) if it isn't
//	          already there; the module never moves an existing rule
//	          (no order management). When `save:` is set, `nft list
//	          ruleset` output is written there after a change.
//	absent  — the chain contains no rule whose text equals `rule`.
//	          Apply deletes matching rules by handle until none
//	          remain (so duplicates are all removed).
//
// `rule` is the rule expression *only* — no `add`/`insert rule`, no
// family/table/chain — e.g. "tcp dport 22 accept" or "ct state
// established,related accept" (a string is whitespace-split; a list
// of args is used as-is). `family` defaults to "inet"; the chain and
// table must already exist (this module does not create them).
//
// Idempotency note: rule matching compares against nft's *canonical*
// rendering of the rule (what `nft list chain` prints). Write `rule`
// in canonical form — nft normalises service names, abbreviations and
// some operators, so e.g. `tcp dport ssh accept` will not match the
// stored `tcp dport 22 accept` and would be re-added on every run.
//
// v0.1 out of scope (v0.x candidates):
//   - Structured rule params (proto/dport/saddr/jump/… à la Salt)
//     instead of a raw `rule`; rule ordering / re-placement; matching
//     by handle or by rule comment instead of by canonical text;
//     quote-aware rule parsing (`comment "with spaces"` needs the
//     list form).
//   - Managing tables / chains (`nft add table|chain`, base-chain
//     hooks/priority/policy), named sets and maps, flowtables, and
//     atomic whole-ruleset file management / `nftables.service`
//     persistence beyond a plain `save: <path>`.
//   - iptables is its own module.
package nftables

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// maxDeleteIterations bounds the `absent` deletion loop so it can
// never spin even if a delete somehow fails to remove a matching
// rule.
const maxDeleteIterations = 100

// New selects the platform's real Provider via auto-detection.
func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

type Module struct {
	provider Provider
}

func (m *Module) Name() string { return "nftables" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a missing firewall rule (e.g. "allow SSH" gone) can
// lock you out; a stray rule that should be absent is a hole. Both
// are HIGH. nil → MEDIUM. Operators override via `severity:`.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
	return statemgmt.DriftSeverityHigh
}

func (m *Module) Check(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	handles, err := m.matchingHandles(ctx, p)
	if err != nil {
		return nil, err
	}
	loc := fmt.Sprintf("%s %s/%s", p.Family, p.Table, p.Chain)
	switch p.State {
	case StatePresent:
		if len(handles) > 0 {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("rule not present in %s → add", loc)}, nil
	case StateAbsent:
		if len(handles) == 0 {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("rule present in %s; want absent", loc)}, nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	handles, err := m.matchingHandles(ctx, p)
	if err != nil {
		return failure(start), err
	}
	loc := fmt.Sprintf("%s %s/%s", p.Family, p.Table, p.Chain)

	switch p.State {
	case StatePresent:
		if len(handles) > 0 {
			return ok(start, false, "", "already converged"), nil
		}
		if err := m.provider.AddRule(ctx, p.Family, p.Table, p.Chain, p.Index, p.Rule); err != nil {
			return failure(start), fmt.Errorf("add rule: %w", err)
		}
		if err := m.maybeSave(ctx, p); err != nil {
			return failure(start), err
		}
		return ok(start, true, fmt.Sprintf("added rule to %s", loc), "applied"), nil

	case StateAbsent:
		if len(handles) == 0 {
			return ok(start, false, "", "already converged"), nil
		}
		for i := 0; ; i++ {
			if i >= maxDeleteIterations {
				return failure(start), fmt.Errorf("rule still matches after %d deletions in %s — giving up", maxDeleteIterations, loc)
			}
			if err := m.provider.DeleteRule(ctx, p.Family, p.Table, p.Chain, handles[0]); err != nil {
				return failure(start), fmt.Errorf("delete rule (handle %d): %w", handles[0], err)
			}
			handles, err = m.matchingHandles(ctx, p)
			if err != nil {
				return failure(start), err
			}
			if len(handles) == 0 {
				break
			}
		}
		if err := m.maybeSave(ctx, p); err != nil {
			return failure(start), err
		}
		return ok(start, true, fmt.Sprintf("removed rule from %s", loc), "applied"), nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

func (m *Module) parsed(decl *statemgmt.Declaration) (*params, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return p, nil
}

// matchingHandles lists the chain's rules and returns the handles of
// those whose canonical text equals the declared rule.
func (m *Module) matchingHandles(ctx context.Context, p *params) ([]int, error) {
	rules, err := m.provider.ListRuleHandles(ctx, p.Family, p.Table, p.Chain)
	if err != nil {
		return nil, err
	}
	want := strings.Join(p.Rule, " ")
	var out []int
	for _, r := range rules {
		if r.Text == want {
			out = append(out, r.Handle)
		}
	}
	return out, nil
}

func (m *Module) maybeSave(ctx context.Context, p *params) error {
	if p.Save == "" {
		return nil
	}
	if err := m.provider.SaveRuleset(ctx, p.Save); err != nil {
		return fmt.Errorf("save ruleset to %s: %w", p.Save, err)
	}
	return nil
}

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}
