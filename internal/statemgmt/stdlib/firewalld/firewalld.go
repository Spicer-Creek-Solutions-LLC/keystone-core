// SPDX-License-Identifier: Apache-2.0

// Package firewalld implements the `firewalld` stdlib state module —
// managing one item (a service, a port, or a rich rule) in a
// firewalld zone via `firewall-cmd --permanent`, per
// PROJECT-DETAILS §4.8 (Firewall category).
//
// Declaration.Name is just a human label (the decl ID); the item's
// identity is `zone` + the kind/value of whichever of `service`,
// `port`, `rich_rule` is set.
//
// State semantics:
//
//	present — `firewall-cmd --permanent --zone=Z --query-<kind>=<v>`
//	          exits 0 (the item is enabled on the zone). Apply runs
//	          `--add-<kind>=<v>` if it isn't, then (unless `reload:
//	          false`) `firewall-cmd --reload` to push the permanent
//	          change to runtime.
//	absent  — the matching `--query-…` does not exit 0. Apply runs
//	          `--remove-<kind>=<v>` if needed, then reloads.
//
// The module always operates on the *permanent* configuration (so
// changes survive a reboot); `--reload` is what makes a permanent
// change active in the running firewalld. The zone must already
// exist — this module does not create zones.
//
// Rich-rule idempotency: rich rules are compared by *canonical form*
// (see canon.go), so a declared rule that differs from firewalld's
// stored form only in whitespace, attribute quoting, or the order of
// attributes within an element still matches. Check lists the zone's
// stored rich rules (`--list-rich-rules`) and compares each — and the
// declared rule — through the same canonicaliser. The canonicaliser is
// *syntactic* only: it does not normalise value semantics (e.g. it
// won't lowercase a MAC or canonicalise a CIDR), so a value that
// firewalld itself rewrites won't match — write such values as
// firewalld stores them.
//
// v0.1 out of scope (v0.x candidates):
//   - Whole-zone management (declare the complete service / port /
//     rich-rule set and prune the rest) and zone creation; binding
//     interfaces or source addresses to a zone; default-zone
//     management; per-zone target (ACCEPT / REJECT / DROP).
//   - Toggles for masquerade, ICMP block, forward ports, ICMP types,
//     and protocol items; `--direct` rules; ipset management;
//     runtime-only (non-permanent) changes; lockdown / panic mode.
//   - `firewall` is its own (planned) abstraction module that
//     dispatches across iptables / nftables / firewalld; firewalld
//     is its own backend here.
package firewalld

import (
	"context"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

// New selects the platform's real Provider via auto-detection.
func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

type Module struct {
	provider Provider
}

func (m *Module) Name() string { return "firewalld" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a missing firewall item (e.g. the `ssh` service
// gone from `public`) can lock you out; a stray one is a hole. Both
// HIGH. nil → MEDIUM. Operators override via `severity:`.
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
	has, err := m.has(ctx, p)
	if err != nil {
		return nil, err
	}
	loc := fmt.Sprintf("%s %q on zone %q", p.Item.Kind, p.Item.Value, p.Zone)
	switch p.State {
	case StatePresent:
		if has {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s not enabled → add", loc)}, nil
	case StateAbsent:
		if !has {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s enabled; want absent", loc)}, nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	has, err := m.has(ctx, p)
	if err != nil {
		return failure(start), err
	}
	loc := fmt.Sprintf("%s %q on zone %q", p.Item.Kind, p.Item.Value, p.Zone)

	switch p.State {
	case StatePresent:
		if has {
			return ok(start, false, "", "already converged"), nil
		}
		if err := m.provider.Add(ctx, p.Zone, p.Item); err != nil {
			return failure(start), fmt.Errorf("add %s: %w", loc, err)
		}
		if err := m.maybeReload(ctx, p); err != nil {
			return failure(start), err
		}
		return ok(start, true, fmt.Sprintf("added %s", loc), "applied"), nil

	case StateAbsent:
		if !has {
			return ok(start, false, "", "already converged"), nil
		}
		if err := m.provider.Remove(ctx, p.Zone, p.Item); err != nil {
			return failure(start), fmt.Errorf("remove %s: %w", loc, err)
		}
		if err := m.maybeReload(ctx, p); err != nil {
			return failure(start), err
		}
		return ok(start, true, fmt.Sprintf("removed %s", loc), "applied"), nil
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

// has reports whether the declared item is present on the zone. Rich
// rules are compared by canonical form (firewalld stores a normalised
// version, so a re-formatted rule must still match); service / port are
// atomic values queried directly via the provider.
func (m *Module) has(ctx context.Context, p *params) (bool, error) {
	if p.Item.Kind != KindRichRule {
		return m.provider.Has(ctx, p.Zone, p.Item)
	}
	stored, err := m.provider.ListRichRules(ctx, p.Zone)
	if err != nil {
		return false, err
	}
	want := canonicalizeRichRule(p.Item.Value)
	for _, s := range stored {
		if canonicalizeRichRule(s) == want {
			return true, nil
		}
	}
	return false, nil
}

func (m *Module) maybeReload(ctx context.Context, p *params) error {
	if !p.Reload {
		return nil
	}
	if err := m.provider.Reload(ctx); err != nil {
		return fmt.Errorf("reload firewalld: %w", err)
	}
	return nil
}

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}
