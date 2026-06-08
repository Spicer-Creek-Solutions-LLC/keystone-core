// SPDX-License-Identifier: Apache-2.0

// Package bridge implements the `bridge` stdlib state module —
// creates and removes a Linux bridge interface at runtime via `ip
// link`, per PROJECT-DETAILS §4.8 (Network base category).
//
// Runtime + optional boot-survive. Without `persist:` the bridge is in
// the kernel right now but does not survive a reboot. With
// `persist: networkd|netplan|auto` the module also renders the bridge to
// the host network config: a `<bridge>.netdev` (`Kind=bridge` + `[Bridge]
// STP=`) plus a `[Network] Bridge=<bridge>` enslave drop-in under each
// port's `.network.d/` (networkd, absent cleans up by glob), or a single
// `bridges:` document (netplan).
//
// Per declaration the module manages one bridge:
//
//   - `name: <iface>` — the bridge's interface name (e.g. `br0`).
//   - `members: [<iface>, …]` — interfaces to add as bridge ports
//     at create time; each is set master via `ip link set <member>
//     master <bridge>`.
//   - `stp: <bool>` — enable Spanning Tree Protocol (default false).
//
// **No in-place attribute / member reconciliation in v1.0.** If a
// bridge already exists with this name, the module considers it
// converged regardless of stp / members. Operators who need to
// change a live bridge delete it first (state `absent`) then
// declare it again.
//
// DriftSeverity HIGH — a missing bridge typically means VM / container
// traffic can't reach its uplink. MEDIUM nil.
//
// v0.1 out of scope (v0.x candidates):
//   - In-place reconciliation of stp + members + other bridge attrs
//     (forward_delay, hello_time, max_age, vlan_filtering,
//     vlan_default_pvid, mcast_snooping, …).
//   - Per-port bridge attributes (state, priority, cost, pvid).
//   - Additional persist backends (NetworkManager, ifupdown).
package bridge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

type Module struct {
	provider Provider
}

func (m *Module) Name() string { return "bridge" }

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
	loc := fmt.Sprintf("bridge %s", p.Name)
	switch p.State {
	case StatePresent:
		if link != nil && link.Kind != "bridge" {
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s exists but kind is %q (not bridge) — refusing to clobber", loc, link.Kind)}, nil
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

// persistDrift reports whether the persistent config for a present
// bridge is missing or stale (false when persist is not requested).
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

// persistLeftover reports whether persistent files for an absent bridge
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
	loc := fmt.Sprintf("bridge %s", p.Name)

	switch p.State {
	case StatePresent:
		if link != nil && link.Kind != "bridge" {
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
			if err := m.provider.CreateBridge(ctx, BridgeSpec{Name: p.Name, STP: p.STP}); err != nil {
				return failure(start), fmt.Errorf("create %s: %w", loc, err)
			}
			for _, member := range p.Members {
				if err := m.provider.SetMaster(ctx, member, p.Name); err != nil {
					return failure(start), fmt.Errorf("attach %s → %s: %w", member, p.Name, err)
				}
			}
			diff := fmt.Sprintf("created %s stp=%v", loc, p.STP)
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

// writePersist renders the present bridge to the host network config.
func (m *Module) writePersist(p *params) error {
	d, err := devicePersist(p)
	if err != nil {
		return err
	}
	return d.Write()
}

// removePersist deletes the bridge's persistent files.
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

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}
