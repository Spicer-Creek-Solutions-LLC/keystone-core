// SPDX-License-Identifier: Apache-2.0

// Package bond implements the `bond` stdlib state module — creates
// and removes a Linux bonding (link-aggregation) interface at
// runtime via `ip link`, per PROJECT-DETAILS §4.8 (Network base
// category).
//
// **Runtime-only in v1.0** — the bond interface is in the kernel
// right now but does not survive a reboot. Persistent
// configuration is V1X (see the `network` module's V1X entry).
//
// Per declaration the module manages one bond interface:
//
//   - `name: <iface>` — the bond's interface name (e.g. `bond0`).
//   - `mode: <m>` — bonding mode; one of the names (`balance-rr`,
//     `active-backup`, `balance-xor`, `broadcast`, `802.3ad`,
//     `balance-tlb`, `balance-alb`) or their numeric forms (0-6).
//     Default `balance-rr` (kernel default).
//   - `members: [<iface>, …]` — interfaces to enslave at create
//     time; each is set master via `ip link set <member> master
//     <bond>`.
//   - `miimon: <ms>` — link-monitor interval in milliseconds (0
//     disables; default unset = kernel default).
//
// Identity: the bond's interface name. State present (bond exists
// with this name + bond type) / absent (no interface of this name).
//
// **No in-place attribute / member reconciliation in v1.0.** If the
// bond already exists, this module reports converged regardless of
// its current mode / miimon / member set. Operators who need to
// change a live bond's attributes delete it first (state `absent`)
// then declare it again. Reconciling these in-place is V1X — it
// requires destroying and re-creating the bond (members are
// released, traffic interrupted), and that's a deliberate-enough
// action to keep behind an explicit operator step.
//
// DriftSeverity HIGH (a missing or misconfigured aggregation =
// reduced link redundancy / unreachable host). MEDIUM nil.
//
// v0.1 out of scope (v0.x candidates):
//   - **In-place attribute reconciliation**: mode, miimon,
//     xmit_hash_policy, lacp_rate, ad_select, primary, primary_reselect,
//     fail_over_mac, num_grat_arp, all_slaves_active, …
//   - **Member-set reconciliation** on an existing bond (add/remove
//     slaves without destroying the bond).
//   - **Persistent / boot-survive configuration** rendered to the
//     host's network manager.
//   - **Slave-level attributes** (queue id, prio).
//   - **VRRP / track-fail** integration with daemons that maintain
//     active/backup state.
package bond

import (
	"context"
	"fmt"
	"strings"
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

func (m *Module) Name() string { return "bond" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

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
	link, err := m.provider.GetLink(ctx, p.Name)
	if err != nil {
		return nil, err
	}
	loc := fmt.Sprintf("bond %s", p.Name)
	switch p.State {
	case StatePresent:
		if link != nil && link.Kind == "bond" {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		if link == nil {
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s not present → create", loc)}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s exists but kind is %q (not bond) — refusing to clobber", loc, link.Kind)}, nil
	case StateAbsent:
		if link == nil {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s present; want absent", loc)}, nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	link, err := m.provider.GetLink(ctx, p.Name)
	if err != nil {
		return failure(start), err
	}
	loc := fmt.Sprintf("bond %s", p.Name)

	switch p.State {
	case StatePresent:
		if link != nil && link.Kind == "bond" {
			return ok(start, false, "", "already converged"), nil
		}
		if link != nil {
			return failure(start), fmt.Errorf("%s exists as kind %q; refusing to clobber — delete it first", loc, link.Kind)
		}
		if err := m.provider.CreateBond(ctx, p.toSpec()); err != nil {
			return failure(start), fmt.Errorf("create %s: %w", loc, err)
		}
		for _, member := range p.Members {
			if err := m.provider.SetMaster(ctx, member, p.Name); err != nil {
				return failure(start), fmt.Errorf("enslave %s → %s: %w", member, p.Name, err)
			}
		}
		diff := fmt.Sprintf("created %s mode=%s", loc, p.Mode)
		if len(p.Members) > 0 {
			diff += " members=" + strings.Join(p.Members, ",")
		}
		return ok(start, true, diff, "applied"), nil
	case StateAbsent:
		if link == nil {
			return ok(start, false, "", "already converged"), nil
		}
		if err := m.provider.DeleteLink(ctx, p.Name); err != nil {
			return failure(start), fmt.Errorf("delete %s: %w", loc, err)
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

func (p *params) toSpec() BondSpec {
	return BondSpec{
		Name:      p.Name,
		Mode:      p.Mode,
		Miimon:    p.Miimon,
		HasMiimon: p.HasMiimon,
	}
}

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}
