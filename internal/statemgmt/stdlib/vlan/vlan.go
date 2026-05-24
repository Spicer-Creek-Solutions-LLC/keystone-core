// SPDX-License-Identifier: Apache-2.0

// Package vlan implements the `vlan` stdlib state module — creates
// and removes a Linux 802.1Q VLAN interface at runtime via `ip
// link`, per PROJECT-DETAILS §4.8 (Network base category).
//
// **Runtime-only in v1.0** — the VLAN interface is in the kernel
// right now but does not survive a reboot. Persistent configuration
// is V1X.
//
// Per declaration the module manages one VLAN interface:
//
//   - `name: <iface>` — the VLAN interface name (e.g. `eth0.10`).
//   - `parent: <iface>` — the underlying interface (e.g. `eth0`).
//     This is the link the VLAN tags ride on.
//   - `id: <int>` — VLAN ID, 1-4094 (802.1Q reserved 0 and 4095).
//
// **No in-place attribute reconciliation in v1.0.** If a VLAN
// interface already exists with this name, the module considers it
// converged regardless of its actual id / parent. Operators who
// need to change a live VLAN delete it first then declare it again.
//
// DriftSeverity HIGH — a missing tagged interface is a downed L2
// segment. MEDIUM nil.
//
// v0.1 out of scope (v0.x candidates):
//   - In-place reconciliation of the VLAN's `id`, `parent`, ingress
//     / egress QoS maps (`ingress-qos-map` / `egress-qos-map`),
//     `reorder_hdr`, `gvrp`, `mvrp`, `loose_binding`.
//   - **QinQ / 802.1ad** (`proto 802.1ad`) — v1.0 always uses the
//     default 802.1Q proto.
//   - **VLAN ranges** (declare 100-200 in one decl).
//   - **Bridge VLAN filtering** entries (the `bridge` module's
//     V1X scope).
//   - Persistent / boot-survive configuration.
package vlan

import (
	"context"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

type Module struct {
	provider Provider
}

func (m *Module) Name() string { return "vlan" }

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
	loc := fmt.Sprintf("vlan %s", p.Name)
	switch p.State {
	case StatePresent:
		if link != nil && link.Kind == "vlan" {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		if link == nil {
			return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s not present → create (parent=%s id=%d)", loc, p.Parent, p.ID)}, nil
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: fmt.Sprintf("%s exists but kind is %q (not vlan) — refusing to clobber", loc, link.Kind)}, nil
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
	loc := fmt.Sprintf("vlan %s", p.Name)

	switch p.State {
	case StatePresent:
		if link != nil && link.Kind == "vlan" {
			return ok(start, false, "", "already converged"), nil
		}
		if link != nil {
			return failure(start), fmt.Errorf("%s exists as kind %q; refusing to clobber — delete it first", loc, link.Kind)
		}
		if err := m.provider.CreateVLAN(ctx, VLANSpec{Name: p.Name, Parent: p.Parent, ID: p.ID}); err != nil {
			return failure(start), fmt.Errorf("create %s: %w", loc, err)
		}
		return ok(start, true, fmt.Sprintf("created %s (parent=%s id=%d)", loc, p.Parent, p.ID), "applied"), nil
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

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}
