// Package iptables implements the `iptables` stdlib state module —
// managing a single iptables (or ip6tables) rule, per
// PROJECT-DETAILS §4.8 (Firewall category).
//
// A rule has no name — its identity is the full spec (chain + match
// args + target), checked via `iptables -C`. Declaration.Name is
// therefore just a human label (the decl ID); the rule comes from
// the `chain` + `rule` params.
//
// State semantics:
//
//	present — `iptables -t <table> -C <chain> <rule...>` succeeds
//	          (the rule exists in the chain). Apply appends it
//	          (`-A`, the default) or inserts it at `position` (`-I`)
//	          if it isn't already there; the module never moves an
//	          existing rule (no order management). When `save:` is
//	          set, `iptables-save` output is written there after a
//	          change.
//	absent  — `iptables -C` does not succeed (the rule isn't in the
//	          chain). Apply runs `-D` until it is gone (so duplicates
//	          are all removed).
//
// `rule` is the match spec + target *only* — no `-t`, no `-A/-I/-D`,
// no chain name — e.g. "-p tcp --dport 22 -m conntrack --ctstate NEW
// -j ACCEPT" (a string is whitespace-split; a list of args is used
// as-is). `table` defaults to "filter"; `family` to "ipv4".
//
// v0.1 out of scope (v0.x candidates):
//   - `family: both`; structured rule params (proto/dport/jump/… à
//     la Salt) instead of a raw `rule`; rule position / ordering
//     management; quote-aware rule parsing (--comment "with spaces").
//   - Managing the whole rules file / distro-aware persistence
//     (netfilter-persistent, the iptables service) beyond a plain
//     `save: <path>`; iptables-nft vs iptables-legacy selection;
//     chain creation (`-N`) / policy (`-P`).
//   - nftables is its own module.
package iptables

import (
	"context"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// maxDeleteIterations bounds the `absent` deletion loop so it can
// never spin even if -D somehow fails to remove a matching rule.
const maxDeleteIterations = 100

// New selects the platform's real Provider via auto-detection.
func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

type Module struct {
	provider Provider
}

func (m *Module) Name() string { return "iptables" }

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
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	has, err := m.provider.HasRule(ctx, p.Family, p.Table, p.Chain, p.Rule)
	if err != nil {
		return nil, err
	}
	switch p.State {
	case StatePresent:
		if has {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("rule not present in %s/%s → add", p.Table, p.Chain)}, nil
	case StateAbsent:
		if !has {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("rule present in %s/%s; want absent", p.Table, p.Chain)}, nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	has, err := m.provider.HasRule(ctx, p.Family, p.Table, p.Chain, p.Rule)
	if err != nil {
		return failure(start), err
	}

	switch p.State {
	case StatePresent:
		if has {
			return ok(start, false, "", "already converged"), nil
		}
		if err := m.provider.AddRule(ctx, p.Family, p.Table, p.Chain, p.Position, p.Rule); err != nil {
			return failure(start), fmt.Errorf("add rule: %w", err)
		}
		if err := m.maybeSave(ctx, p); err != nil {
			return failure(start), err
		}
		return ok(start, true, fmt.Sprintf("added rule to %s/%s", p.Table, p.Chain), "applied"), nil

	case StateAbsent:
		if !has {
			return ok(start, false, "", "already converged"), nil
		}
		for i := 0; ; i++ {
			if i >= maxDeleteIterations {
				return failure(start), fmt.Errorf("rule still matches after %d deletions of %s/%s — giving up", maxDeleteIterations, p.Table, p.Chain)
			}
			if err := m.provider.DeleteRule(ctx, p.Family, p.Table, p.Chain, p.Rule); err != nil {
				return failure(start), fmt.Errorf("delete rule: %w", err)
			}
			still, err := m.provider.HasRule(ctx, p.Family, p.Table, p.Chain, p.Rule)
			if err != nil {
				return failure(start), err
			}
			if !still {
				break
			}
		}
		if err := m.maybeSave(ctx, p); err != nil {
			return failure(start), err
		}
		return ok(start, true, fmt.Sprintf("removed rule from %s/%s", p.Table, p.Chain), "applied"), nil
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

func (m *Module) maybeSave(ctx context.Context, p *params) error {
	if p.Save == "" {
		return nil
	}
	if err := m.provider.Save(ctx, p.Family, p.Save); err != nil {
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
