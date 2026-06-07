// SPDX-License-Identifier: Apache-2.0

// Package firewall implements the `firewall` stdlib state module —
// the v1.0 cross-backend abstraction over iptables, nftables and
// firewalld, per PROJECT-DETAILS §4.8 (Firewall category).
//
// One declaration says "allow this service (or port) inbound" and is
// dispatched to whichever backend is in use on the host. The three
// concrete backend modules (`iptables`, `nftables`, `firewalld`) are
// still available directly when the operator needs anything outside
// this abstraction's narrow scope.
//
// Backend selection:
//
//   - `backend:` (operator override): one of `iptables`, `nftables`,
//     `firewalld`.
//   - Otherwise auto-detect (in order):
//     1. `firewall-cmd` on PATH AND `firewall-cmd --state` exits 0
//     → firewalld.
//     2. `iptables` on PATH → iptables.
//     3. `nft` on PATH → nftables.
//     4. else → ErrNoFirewall.
//
// `iptables` is preferred over `nft` because on most Linux systems
// `iptables` is the iptables-nft shim — the iptables CLI is what
// operators expect, even when the kernel backend is nft. Pure-nft
// setups should pin `backend: nftables`.
//
// Translation (operator declaration → backend declaration):
//
//	firewalld:  zone=<zone> port=<PORT[-PORT]/PROTO>
//	iptables:   table=filter chain=INPUT family={ipv4,ipv6}  (dual-stack)
//	            rule=["-p", PROTO, "--dport", PORT[:PORT], "-j", "ACCEPT"]
//	nftables:   family=inet table=filter chain=input
//	            rule="PROTO dport PORT[-PORT] accept"
//
// The iptables backend runs as two sub-applies — one per address family
// — so a single `firewall` declaration opens both IPv4 and IPv6 by
// default, matching firewalld and nftables (which cover both families
// in one rule). When ip6tables is absent (IPv6-disabled host) the IPv6
// half is skipped gracefully: the IPv4 rule still applies and the
// StateResult Comment + Diff say loudly that IPv6 was NOT applied.
//
// A named service is resolved through the catalog (services.go) so
// every backend agrees on the port. The catalog is multi-port (e.g.
// `samba` → 137/udp + 138/udp + 139/tcp + 445/tcp, expanded to one
// rule per port per family); names not in the static catalog can fall
// back to the host's `/etc/services` with `strict_catalog: false`.
//
// v0.1 out of scope (v0.x candidates):
//   - `action: deny` (v1.0 is allow-only).
//   - chain / table / family overrides on iptables / nftables
//     backends (v1.0 hard-codes the standard inbound chain and, for
//     iptables, dual-stack v4+v6 — an operator who needs a single
//     family or a custom chain uses the backend module directly).
//   - Per-source filtering and richer rich-rule-style matches.
//   - nftables backend chain creation (v1.0 requires `inet filter
//     input` to already exist — see the nftables module's V1X
//     entry).
package firewall

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/firewalld"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/iptables"
	"go.keystone-core.io/keystone-core/internal/statemgmt/stdlib/nftables"
)

// New selects the platform's real detector and wires the three
// backend modules. Each backend is a fresh instance per Module so
// they don't share provider state across runs.
func New() statemgmt.Module {
	return &Module{
		detector: defaultDetector(),
		backends: map[string]statemgmt.Module{
			BackendIptables:  iptables.New(),
			BackendNftables:  nftables.New(),
			BackendFirewalld: firewalld.New(),
		},
	}
}

// NewWithBackends is the test injection point.
func NewWithBackends(d BackendDetector, backends map[string]statemgmt.Module) statemgmt.Module {
	return &Module{detector: d, backends: backends}
}

type Module struct {
	detector BackendDetector
	backends map[string]statemgmt.Module
}

func (m *Module) Name() string { return "firewall" }

func (m *Module) ValidStates() []string { return []string{StatePresent, StateAbsent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a missing allow rule (e.g. the SSH allow gone) can
// lock you out; a stray allow that should be absent is a hole. Both
// HIGH. nil → MEDIUM. Operators override via `severity:`.
func (m *Module) DriftSeverity(decl *statemgmt.Declaration, _ *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	if decl == nil {
		return statemgmt.DriftSeverityMedium
	}
	return statemgmt.DriftSeverityHigh
}

func (m *Module) Check(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	backend, subs, err := m.prepare(ctx, decl)
	if err != nil {
		return nil, err
	}
	matches := true
	var diffs []string
	for _, s := range subs {
		res, err := backend.Check(ctx, s.decl)
		if err != nil {
			// A skippable (IPv6) sub on a host without ip6tables can't
			// be checked; treat it as converged so an IPv4-only host
			// doesn't report perpetual, unfixable drift.
			if s.skippable && iptables.IsNoIptables(err) {
				continue
			}
			return nil, err
		}
		if !res.Matches {
			matches = false
		}
		if res.Diff != "" {
			diffs = append(diffs, labelled(s.label, res.Diff))
		}
	}
	return &statemgmt.ModuleCheckResult{Matches: matches, Diff: strings.Join(diffs, "; ")}, nil
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	backend, subs, err := m.prepare(ctx, decl)
	if err != nil {
		return nil, err
	}
	agg := &statemgmt.StateResult{Success: true}
	var diffs, comments []string
	for _, s := range subs {
		res, applyErr := backend.Apply(ctx, s.decl)
		if applyErr != nil {
			// Graceful skip: IPv6 sub on a host without ip6tables. The
			// IPv4 rule still applies; surface the skip loudly in both
			// the Comment and the Diff so it's unmistakable that IPv6
			// was not covered.
			if s.skippable && iptables.IsNoIptables(applyErr) {
				comments = append(comments, s.label+" NOT APPLIED — ip6tables not found on host")
				diffs = append(diffs, s.label+": SKIPPED (ip6tables absent)")
				continue
			}
			agg.Success = false
			agg.Diff = strings.Join(diffs, "; ")
			agg.Comment = strings.Join(comments, "; ")
			agg.Duration = time.Since(start)
			return agg, applyErr
		}
		if res != nil {
			agg.Success = agg.Success && res.Success
			agg.Changed = agg.Changed || res.Changed
			if res.Diff != "" {
				diffs = append(diffs, labelled(s.label, res.Diff))
			}
			if res.Comment != "" {
				comments = append(comments, labelled(s.label, res.Comment))
			}
		}
	}
	agg.Diff = strings.Join(diffs, "; ")
	agg.Comment = strings.Join(comments, "; ")
	agg.Duration = time.Since(start)
	return agg, nil
}

// labelled prefixes a per-sub Diff/Comment fragment with its label
// (e.g. "ipv4 137/udp" for a dual-stack iptables port, or "137/udp" for
// a multi-port firewalld/nftables rule) so an aggregated result is
// legible. A single, unambiguous sub carries no label and passes
// through unchanged.
func labelled(label, s string) string {
	if label == "" {
		return s
	}
	return label + ": " + s
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

// subApply is one backend declaration to dispatch. label disambiguates
// the sub in the aggregated Diff/Comment when there is more than one
// (e.g. "ipv4 137/udp" for a dual-stack iptables port, "137/udp" for a
// multi-port firewalld/nftables rule); it is "" for a single,
// unambiguous sub. skippable marks an IPv6 iptables sub: on a host
// without ip6tables it is skipped gracefully rather than failing the
// whole apply.
type subApply struct {
	label     string
	skippable bool
	decl      *statemgmt.Declaration
}

// prepare parses, validates, resolves the backend, and builds the
// sub-declaration(s) to hand the backend module. The iptables backend
// yields two — IPv4 and IPv6 — so `firewall` opens both families by
// default (firewalld and nftables already cover both in one rule).
func (m *Module) prepare(ctx context.Context, decl *statemgmt.Declaration) (statemgmt.Module, []subApply, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, nil, err
	}
	if err := p.validate(); err != nil {
		return nil, nil, err
	}
	backendName := p.Backend
	if backendName == "" {
		n, err := m.detector.Detect(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("detect firewall backend: %w", err)
		}
		backendName = n
	}
	backend, ok := m.backends[backendName]
	if !ok {
		return nil, nil, fmt.Errorf("no backend module wired for %q", backendName)
	}
	subs, err := buildSubDecls(backendName, p, decl)
	if err != nil {
		return nil, nil, err
	}
	return backend, subs, nil
}

// buildSubDecls translates the abstraction's params into the chosen
// backend's declaration(s). Backend defaults (chain / table / family /
// zone) are baked in here — v1.0 does not expose them as abstraction
// params; operators who need to customise them use the backend module
// directly. Each resolved port becomes one rule per family, so a
// multi-port service (e.g. `samba` → 4 ports) and the iptables backend's
// dual-stack (IPv4 + IPv6) multiply out: `samba` on iptables is 8 subs,
// on firewalld / nftables 4. A label disambiguates each sub in the
// aggregated result when there is more than one.
func buildSubDecls(backendName string, p *params, decl *statemgmt.Declaration) ([]subApply, error) {
	multiPort := len(p.Ports) > 1
	switch backendName {
	case BackendFirewalld:
		var subs []subApply
		for _, pp := range p.Ports {
			subs = append(subs, subApply{label: portLabel(multiPort, "", pp), decl: &statemgmt.Declaration{
				ID:     decl.ID,
				Module: BackendFirewalld,
				State:  decl.State,
				Name:   decl.Name,
				Params: map[string]any{
					"zone": p.Zone,
					"port": firewalldPortValue(pp),
				},
			}})
		}
		return subs, nil
	case BackendNftables:
		var subs []subApply
		for _, pp := range p.Ports {
			subs = append(subs, subApply{label: portLabel(multiPort, "", pp), decl: &statemgmt.Declaration{
				ID:     decl.ID,
				Module: BackendNftables,
				State:  decl.State,
				Name:   decl.Name,
				Params: map[string]any{
					"family": "inet",
					"table":  "filter",
					"chain":  "input",
					"rule":   nftablesRule(pp),
				},
			}})
		}
		return subs, nil
	case BackendIptables:
		// iptables always splits per family, so every sub is labelled
		// (at minimum by family) for a legible aggregated result.
		var subs []subApply
		for _, pp := range p.Ports {
			subs = append(subs,
				subApply{label: portLabel(true, iptables.FamilyIPv4, pp), decl: iptablesSubDecl(decl, iptables.FamilyIPv4, pp)},
				subApply{label: portLabel(true, iptables.FamilyIPv6, pp), skippable: true, decl: iptablesSubDecl(decl, iptables.FamilyIPv6, pp)},
			)
		}
		return subs, nil
	}
	return nil, fmt.Errorf("unsupported backend %q", backendName)
}

// portLabel builds a sub's aggregation label. With a family it is
// "<family> <port>/<proto>" (iptables); without, "<port>/<proto>" when
// the rule is one of several ports; "" for a single unambiguous sub.
func portLabel(needed bool, family string, pp portProto) string {
	switch {
	case family != "":
		return family + " " + pp.String()
	case needed:
		return pp.String()
	default:
		return ""
	}
}

// iptablesSubDecl builds the iptables backend declaration for one
// address family and port. The rule body is family-agnostic (-p PROTO
// --dport PORT -j ACCEPT), so the same match works for iptables and
// ip6tables.
func iptablesSubDecl(decl *statemgmt.Declaration, family string, pp portProto) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     decl.ID,
		Module: BackendIptables,
		State:  decl.State,
		Name:   decl.Name,
		Params: map[string]any{
			"table":  "filter",
			"chain":  "INPUT",
			"family": family,
			"rule":   iptablesRule(pp),
		},
	}
}
