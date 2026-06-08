// SPDX-License-Identifier: Apache-2.0

// Package bond implements the `bond` stdlib state module — creates
// and removes a Linux bonding (link-aggregation) interface at
// runtime via `ip link`, per PROJECT-DETAILS §4.8 (Network base
// category).
//
// Runtime + optional boot-survive. Without `persist:` the bond is in
// the kernel right now but does not survive a reboot. With
// `persist: networkd|netplan|auto` the module also renders the bond to
// the host network config (via the shared netpersist helper):
//
//   - networkd — a `<bond>.netdev` (`[NetDev] Kind=bond` + `[Bond]
//     Mode=…`) plus a `[Network] Bond=<bond>` enslave drop-in under each
//     member's `<member>.network.d/` (with a create-if-absent member
//     base). Enslaving is member-side, so `absent` removes the drop-ins
//     by glob (the absent declaration carries no member list).
//   - netplan — a single `bonds:` document listing the members inline.
//
// Persistence renders the *declared* config; consistent with the
// no-in-place-reconcile rule below, an already-present bond's running
// attributes are left untouched while its boot config is corrected.
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
//   - **Additional persist backends**: NetworkManager, ifupdown.
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
		if link != nil && link.Kind != "bond" {
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s exists but kind is %q (not bond) — refusing to clobber", loc, link.Kind)}, nil
		}
		runtimeMatch := link != nil
		persistDrift, err := m.persistDrift(p)
		if err != nil {
			return nil, err
		}
		if runtimeMatch && !persistDrift {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		var parts []string
		if !runtimeMatch {
			parts = append(parts, fmt.Sprintf("%s not present → create", loc))
		}
		if persistDrift {
			parts = append(parts, fmt.Sprintf("%s persist (%s) out of date", loc, p.Persist))
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: strings.Join(parts, "; ")}, nil
	case StateAbsent:
		persistLeftover, err := m.persistLeftover(p)
		if err != nil {
			return nil, err
		}
		if link == nil && !persistLeftover {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		var parts []string
		if link != nil {
			parts = append(parts, fmt.Sprintf("%s present; want absent", loc))
		}
		if persistLeftover {
			parts = append(parts, fmt.Sprintf("%s persist file present; want removed", loc))
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: strings.Join(parts, "; ")}, nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

// persistDrift reports whether the persistent config for a present bond
// is missing or stale (false when persist is not requested).
func (m *Module) persistDrift(p *params) (bool, error) {
	if p.Persist == "" {
		return false, nil
	}
	d, err := devicePersist(p)
	if err != nil {
		return false, err
	}
	return d.PresentDrift()
}

// persistLeftover reports whether persistent files for an absent bond
// still exist (false when persist is not requested).
func (m *Module) persistLeftover(p *params) (bool, error) {
	if p.Persist == "" {
		return false, nil
	}
	d, err := devicePersist(p)
	if err != nil {
		return false, err
	}
	return d.AbsentDrift()
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
		if link != nil && link.Kind != "bond" {
			return failure(start), fmt.Errorf("%s exists as kind %q; refusing to clobber — delete it first", loc, link.Kind)
		}
		runtimeMatch := link != nil
		persistDrift, err := m.persistDrift(p)
		if err != nil {
			return failure(start), err
		}
		if runtimeMatch && !persistDrift {
			return ok(start, false, "", "already converged"), nil
		}
		var diffs []string
		if !runtimeMatch {
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
			diffs = append(diffs, diff)
		}
		if persistDrift {
			if err := m.writePersist(p); err != nil {
				return failure(start), fmt.Errorf("persist %s: %w", loc, err)
			}
			diffs = append(diffs, fmt.Sprintf("wrote %s persist", p.Persist))
		}
		return ok(start, true, strings.Join(diffs, "; "), "applied"), nil
	case StateAbsent:
		persistLeftover, err := m.persistLeftover(p)
		if err != nil {
			return failure(start), err
		}
		if link == nil && !persistLeftover {
			return ok(start, false, "", "already converged"), nil
		}
		var diffs []string
		if link != nil {
			if err := m.provider.DeleteLink(ctx, p.Name); err != nil {
				return failure(start), fmt.Errorf("delete %s: %w", loc, err)
			}
			diffs = append(diffs, fmt.Sprintf("removed %s", loc))
		}
		if persistLeftover {
			if err := m.removePersist(p); err != nil {
				return failure(start), fmt.Errorf("remove persist %s: %w", loc, err)
			}
			diffs = append(diffs, fmt.Sprintf("removed %s persist", p.Persist))
		}
		return ok(start, true, strings.Join(diffs, "; "), "applied"), nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

// writePersist renders the present bond to the host network config.
func (m *Module) writePersist(p *params) error {
	d, err := devicePersist(p)
	if err != nil {
		return err
	}
	return d.Write()
}

// removePersist deletes the bond's persistent files.
func (m *Module) removePersist(p *params) error {
	d, err := devicePersist(p)
	if err != nil {
		return err
	}
	return d.Remove()
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
