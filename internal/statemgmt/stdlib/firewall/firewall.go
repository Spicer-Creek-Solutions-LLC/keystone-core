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
//	iptables:   table=filter chain=INPUT family=ipv4
//	            rule=["-p", PROTO, "--dport", PORT[:PORT], "-j", "ACCEPT"]
//	nftables:   family=inet table=filter chain=input
//	            rule="PROTO dport PORT[-PORT] accept"
//
// A named service is resolved through a small fixed catalog
// (services.go) so every backend agrees on the port — firewalld's
// own service catalog and `/etc/services` would otherwise diverge.
//
// v0.1 out of scope (v0.x candidates):
//   - `action: deny` (v1.0 is allow-only).
//   - `family: both` for iptables (v1.0 manages IPv4 only on the
//     iptables backend — for IPv6 coverage use `backend: nftables`
//     with `family=inet`, or `backend: firewalld`).
//   - chain / table / family overrides on iptables / nftables
//     backends (v1.0 hard-codes the standard inbound chain).
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
	backend, sub, err := m.prepare(ctx, decl)
	if err != nil {
		return nil, err
	}
	return backend.Check(ctx, sub)
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	backend, sub, err := m.prepare(ctx, decl)
	if err != nil {
		return nil, err
	}
	res, applyErr := backend.Apply(ctx, sub)
	if res != nil && res.Duration == 0 {
		res.Duration = time.Since(start)
	}
	return res, applyErr
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}

// prepare parses, validates, resolves the backend, and builds the
// sub-declaration to hand the backend module.
func (m *Module) prepare(ctx context.Context, decl *statemgmt.Declaration) (statemgmt.Module, *statemgmt.Declaration, error) {
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
	sub, err := buildSubDecl(backendName, p, decl)
	if err != nil {
		return nil, nil, err
	}
	return backend, sub, nil
}

// buildSubDecl translates the abstraction's params into the chosen
// backend's declaration. Backend defaults (chain / table / family /
// zone) are baked in here — v1.0 does not expose them as abstraction
// params; operators who need to customise them use the backend
// module directly.
func buildSubDecl(backendName string, p *params, decl *statemgmt.Declaration) (*statemgmt.Declaration, error) {
	switch backendName {
	case BackendFirewalld:
		return &statemgmt.Declaration{
			ID:     decl.ID,
			Module: BackendFirewalld,
			State:  decl.State,
			Name:   decl.Name,
			Params: map[string]any{
				"zone": p.Zone,
				"port": p.firewalldPortValue(),
			},
		}, nil
	case BackendIptables:
		return &statemgmt.Declaration{
			ID:     decl.ID,
			Module: BackendIptables,
			State:  decl.State,
			Name:   decl.Name,
			Params: map[string]any{
				"table":  "filter",
				"chain":  "INPUT",
				"family": "ipv4",
				"rule":   p.iptablesRule(),
			},
		}, nil
	case BackendNftables:
		return &statemgmt.Declaration{
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
		}, nil
	}
	return nil, fmt.Errorf("unsupported backend %q", backendName)
}
