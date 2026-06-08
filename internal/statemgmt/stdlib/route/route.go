// SPDX-License-Identifier: Apache-2.0

// Package route implements the `route` stdlib state module —
// manages one entry in the kernel routing table at *runtime* via the
// iproute2 `ip` tool, per PROJECT-DETAILS §4.8 (Network base
// category).
//
// Runtime + optional boot-survive. Without `persist:` the route is in
// the kernel right now but does not survive a reboot. With
// `persist: networkd|netplan|auto` the module also renders the route to
// the host's network config (see persist.go / render.go):
//
//   - networkd — a `[Route]` drop-in under
//     `<iface>.network.d/<slug>.conf`. networkd *merges* drop-ins, so
//     multiple routes on one interface and the interface's address unit
//     all coexist; a minimal `[Match]` base unit is created if absent.
//   - netplan — a per-route `routes:` document. netplan merges by key,
//     so it coexists with the interface's address document; but netplan
//     *replaces* a list across files, so multiple routes on the same
//     interface via separate netplan files conflict — use networkd for
//     multi-route interfaces. netplan's `table:` is numeric.
//
// `persist:` requires `interface:` (a gateway-only route has no output
// interface to attach the rendered entry to). Other per-distro
// persistence targets (NetworkManager static-routes,
// /etc/sysconfig/network-scripts/route-*, /etc/network/interfaces
// `post-up ip route add …`) remain V1X.
//
// Per declaration the module manages one route, keyed on
// `(destination, table)`:
//
//   - `destination: <CIDR>` — required. Use `0.0.0.0/0` (or `::/0`)
//     for the default route, or a specific network for a static
//     route.
//   - One or both of `gateway: <IP>` and `interface: <name>` —
//     required for state `present`. A `gateway:` alone is the most
//     common form; a `dev`-only route is valid for directly-attached
//     networks. Both means "via this gateway on this interface".
//   - `metric: <int>` — optional. When set, the metric is part of the
//     route's identity (`ip route` allows multiple routes to the same
//     destination at different metrics).
//   - `table: <name|int>` — optional, defaults to `main`. Names
//     follow `rt_tables` (5) (main / local / default / custom);
//     numeric values 1-254 are also accepted (255 is local, 254 is
//     main by convention but we let the kernel decide).
//
// Identity / idempotency: `ip route show <dest> [metric N] [table T]`
// returns the matching route(s); Check compares the gateway and
// interface (modulo `<unset>`-means-don't-care semantics on each
// declared field). Apply runs `ip route replace` for `present` —
// "replace" semantics mean an existing route with the same identity
// (dest+metric+table) is overwritten with the new gateway/interface,
// which is the idempotency-friendly choice (an `add` would fail with
// EEXIST when the operator changed only the gateway). Apply runs
// `ip route del` for `absent`.
//
// DriftSeverity HIGH — a missing default route is no egress; a
// stale route can hairpin traffic into a black hole. MEDIUM nil.
//
// v0.1 out of scope (v0.x candidates):
//   - **Additional persist backends**: NetworkManager static-routes,
//     sysconfig `route-*`, /etc/network/interfaces post-up.
//   - **Route attributes**: `proto`, `scope`, `src`, `mtu`,
//     `advmss`, `pref`, `nexthop` multipath, `onlink`, `realms`,
//     `congctl`.
//   - **Source-routing policy** rules (`ip rule add`) and a separate
//     `rule` module.
//   - **VRF awareness** beyond the `table:` knob.
//   - **IPv6 specific knobs**: `expires`, `pref` med/high/low.
//   - **Default-table inference from metric** (some platforms imply
//     a table from a metric range — v1.0 keeps them orthogonal).
package route

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

func (m *Module) Name() string { return "route" }

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
	current, err := m.provider.GetRoute(ctx, p.toQuery())
	if err != nil {
		return nil, err
	}
	loc := p.locator()
	switch p.State {
	case StatePresent:
		runtimeMatch := current != nil && routeMatches(p, current)
		persistDrift := false
		if p.Persist != "" {
			d, err := persistPresentDrift(p)
			if err != nil {
				return nil, err
			}
			persistDrift = d
		}
		if runtimeMatch && !persistDrift {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		var parts []string
		if !runtimeMatch {
			if current == nil {
				parts = append(parts, fmt.Sprintf("%s not present → add", loc))
			} else {
				parts = append(parts, fmt.Sprintf("%s differs: %s → %s", loc, current.summary(), p.summary()))
			}
		}
		if persistDrift {
			parts = append(parts, fmt.Sprintf("%s persist (%s) out of date", loc, p.Persist))
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: strings.Join(parts, "; ")}, nil
	case StateAbsent:
		persistLeftover := false
		if p.Persist != "" {
			d, err := persistAbsentDrift(p)
			if err != nil {
				return nil, err
			}
			persistLeftover = d
		}
		if current == nil && !persistLeftover {
			return &statemgmt.ModuleCheckResult{Matches: true}, nil
		}
		var parts []string
		if current != nil {
			parts = append(parts, fmt.Sprintf("%s present (%s); want absent", loc, current.summary()))
		}
		if persistLeftover {
			parts = append(parts, fmt.Sprintf("%s persist file present; want removed", loc))
		}
		return &statemgmt.ModuleCheckResult{Matches: false, Diff: strings.Join(parts, "; ")}, nil
	}
	return nil, fmt.Errorf("unknown state %q", p.State)
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	current, err := m.provider.GetRoute(ctx, p.toQuery())
	if err != nil {
		return failure(start), err
	}
	loc := p.locator()

	switch p.State {
	case StatePresent:
		runtimeMatch := current != nil && routeMatches(p, current)
		persistDrift := false
		if p.Persist != "" {
			d, err := persistPresentDrift(p)
			if err != nil {
				return failure(start), err
			}
			persistDrift = d
		}
		if runtimeMatch && !persistDrift {
			return ok(start, false, "", "already converged"), nil
		}
		var diffs []string
		if !runtimeMatch {
			// `ip route replace` is idempotent against an existing entry
			// at the same (dest, metric, table) — overwriting it with the
			// new gateway/interface is the right behaviour for a state
			// module (operators don't want EEXIST when only the gateway
			// changed).
			if err := m.provider.ReplaceRoute(ctx, p.toSpec()); err != nil {
				return failure(start), fmt.Errorf("replace route %s: %w", loc, err)
			}
			if current == nil {
				diffs = append(diffs, fmt.Sprintf("added %s (%s)", loc, p.summary()))
			} else {
				diffs = append(diffs, fmt.Sprintf("replaced %s: %s → %s", loc, current.summary(), p.summary()))
			}
		}
		if persistDrift {
			if err := writePersist(p); err != nil {
				return failure(start), fmt.Errorf("persist route %s: %w", loc, err)
			}
			diffs = append(diffs, fmt.Sprintf("wrote %s persist", p.Persist))
		}
		return ok(start, true, strings.Join(diffs, "; "), "applied"), nil
	case StateAbsent:
		persistLeftover := false
		if p.Persist != "" {
			d, err := persistAbsentDrift(p)
			if err != nil {
				return failure(start), err
			}
			persistLeftover = d
		}
		if current == nil && !persistLeftover {
			return ok(start, false, "", "already converged"), nil
		}
		var diffs []string
		if current != nil {
			if err := m.provider.DelRoute(ctx, p.toQuery()); err != nil {
				return failure(start), fmt.Errorf("delete route %s: %w", loc, err)
			}
			diffs = append(diffs, fmt.Sprintf("removed %s (%s)", loc, current.summary()))
		}
		if persistLeftover {
			if err := removePersist(p); err != nil {
				return failure(start), fmt.Errorf("remove persist %s: %w", loc, err)
			}
			diffs = append(diffs, fmt.Sprintf("removed %s persist", p.Persist))
		}
		return ok(start, true, strings.Join(diffs, "; "), "applied"), nil
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

// routeMatches reports whether the live route matches the
// declaration. Each declared field is compared; an unset declared
// field is "don't care". Destination + metric + table are part of
// the lookup, so they always match by definition.
func routeMatches(p *params, live *RouteEntry) bool {
	if p.Gateway != "" && live.Gateway != p.Gateway {
		return false
	}
	if p.Interface != "" && live.Interface != p.Interface {
		return false
	}
	return true
}

// summary renders a RouteEntry in a compact human form for diff
// messages.
func (r *RouteEntry) summary() string {
	var parts []string
	if r.Gateway != "" {
		parts = append(parts, "via "+r.Gateway)
	}
	if r.Interface != "" {
		parts = append(parts, "dev "+r.Interface)
	}
	if len(parts) == 0 {
		parts = append(parts, "<nexthop unset>")
	}
	return strings.Join(parts, " ")
}

func (p *params) summary() string {
	var parts []string
	if p.Gateway != "" {
		parts = append(parts, "via "+p.Gateway)
	}
	if p.Interface != "" {
		parts = append(parts, "dev "+p.Interface)
	}
	return strings.Join(parts, " ")
}

func (p *params) locator() string {
	loc := "route " + p.Destination
	if p.HasMetric {
		loc += fmt.Sprintf(" metric %d", p.Metric)
	}
	if p.Table != "" && p.Table != "main" {
		loc += " table " + p.Table
	}
	return loc
}

// toQuery shapes the params for a routing-table lookup (Get / Del).
func (p *params) toQuery() RouteQuery {
	q := RouteQuery{Destination: p.Destination, Table: p.Table}
	if p.HasMetric {
		q.Metric = p.Metric
		q.HasMetric = true
	}
	return q
}

// toSpec shapes the params for `ip route replace`.
func (p *params) toSpec() RouteSpec {
	s := RouteSpec{Destination: p.Destination, Gateway: p.Gateway, Interface: p.Interface, Table: p.Table}
	if p.HasMetric {
		s.Metric = p.Metric
		s.HasMetric = true
	}
	return s
}

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}
