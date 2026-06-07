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
// A named service is resolved through a small fixed catalog
// (services.go) so every backend agrees on the port — firewalld's
// own service catalog and `/etc/services` would otherwise diverge.
//
// v0.1 out of scope (v0.x candidates):
//   - `action: deny` (v1.0 is allow-only).
//   - chain / table / family overrides on iptables / nftables
//     backends (v1.0 hard-codes the standard inbound chain and, for
//     iptables, dual-stack v4+v6 — an operator who needs a single
//     family or a custom chain uses the backend module directly).
//   - Named-service catalog expansion (firewalld-native names like
//     `dhcpv6-client`, multi-port services like `samba`, an
//     `/etc/services` lookup with a `strict_catalog: false` opt-in).
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

// ipv6SkipReason is the operator-facing note surfaced (loudly, in both
// the Comment and the Diff) when the IPv6 half of a dual-stack iptables
// apply is skipped because ip6tables is absent on the host. The skip is
// graceful — the IPv4 rule still applies — but it must be unmistakable
// that IPv6 was NOT covered.
const ipv6SkipReason = "IPv6 NOT APPLIED — ip6tables not found on host"

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
			diffs = append(diffs, labelled(s.family, res.Diff))
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
				comments = append(comments, ipv6SkipReason)
				diffs = append(diffs, "ipv6: SKIPPED (ip6tables absent)")
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
				diffs = append(diffs, labelled(s.family, res.Diff))
			}
			if res.Comment != "" {
				comments = append(comments, labelled(s.family, res.Comment))
			}
		}
	}
	agg.Diff = strings.Join(diffs, "; ")
	agg.Comment = strings.Join(comments, "; ")
	agg.Duration = time.Since(start)
	return agg, nil
}

// labelled prefixes a per-family Diff/Comment fragment with its family
// (ipv4 / ipv6) so a dual-stack iptables result is legible. Single-
// stack backends (firewalld / nftables) carry no family label, so the
// fragment passes through unchanged.
func labelled(family, s string) string {
	if family == "" {
		return s
	}
	return family + ": " + s
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

// subApply is one backend declaration to dispatch. family is the
// iptables address family label (ipv4 / ipv6) used to prefix the
// aggregated Diff/Comment, or "" for the single-stack backends
// (firewalld / nftables, which cover both families in one rule).
// skippable marks the IPv6 iptables sub: on a host without ip6tables it
// is skipped gracefully rather than failing the whole apply.
type subApply struct {
	family    string
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
// directly. The iptables backend yields two declarations (IPv4 + IPv6)
// for dual-stack coverage; firewalld and nftables yield one each.
func buildSubDecls(backendName string, p *params, decl *statemgmt.Declaration) ([]subApply, error) {
	switch backendName {
	case BackendFirewalld:
		return []subApply{{decl: &statemgmt.Declaration{
			ID:     decl.ID,
			Module: BackendFirewalld,
			State:  decl.State,
			Name:   decl.Name,
			Params: map[string]any{
				"zone": p.Zone,
				"port": p.firewalldPortValue(),
			},
		}}}, nil
	case BackendNftables:
		return []subApply{{decl: &statemgmt.Declaration{
			ID:     decl.ID,
			Module: BackendNftables,
			State:  decl.State,
			Name:   decl.Name,
			Params: map[string]any{
				"family": "inet",
				"table":  "filter",
				"chain":  "input",
				"rule":   p.nftablesRule(),
			},
		}}}, nil
	case BackendIptables:
		return []subApply{
			{family: iptables.FamilyIPv4, decl: iptablesSubDecl(p, decl, iptables.FamilyIPv4)},
			{family: iptables.FamilyIPv6, skippable: true, decl: iptablesSubDecl(p, decl, iptables.FamilyIPv6)},
		}, nil
	}
	return nil, fmt.Errorf("unsupported backend %q", backendName)
}

// iptablesSubDecl builds the iptables backend declaration for one
// address family. The rule body is family-agnostic (-p PROTO --dport
// PORT -j ACCEPT), so the same match works for iptables and ip6tables.
func iptablesSubDecl(p *params, decl *statemgmt.Declaration, family string) *statemgmt.Declaration {
	return &statemgmt.Declaration{
		ID:     decl.ID,
		Module: BackendIptables,
		State:  decl.State,
		Name:   decl.Name,
		Params: map[string]any{
			"table":  "filter",
			"chain":  "INPUT",
			"family": family,
			"rule":   p.iptablesRule(),
		},
	}
}
