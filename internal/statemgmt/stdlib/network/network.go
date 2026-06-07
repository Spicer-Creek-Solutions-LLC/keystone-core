// SPDX-License-Identifier: Apache-2.0

// Package network implements the `network` stdlib state module —
// manages one network interface's *runtime* configuration via the
// iproute2 `ip` tool, per PROJECT-DETAILS §4.8 (Network base
// category).
//
// The module reconciles the live runtime config (via `ip`) and, when
// `persist:` is set, **also** renders a boot-survive file so the config
// survives a reboot. `persist: networkd|netplan|auto` writes a
// systemd-networkd `*.network` unit or a netplan YAML document mirroring
// the declared addresses + mtu (`up` is runtime-only — networkd brings a
// matched link up by default and there is no clean persistent `down`).
// `auto` picks netplan when `/etc/netplan` exists, else networkd. The
// file is written for the next boot; the runtime is already live via the
// `ip` ops, so nothing is auto-activated (no `netplan apply` /
// `networkctl reload`). NetworkManager `system-connections/`, Debian
// `/etc/network/interfaces`, and RHEL `ifcfg-*` renderers remain V1X.
//
// Per declaration the module manages one interface (`interface:`)
// and reconciles whichever of:
//
//   - `addresses: [<CIDR>, …]` — the declared list is the full set
//     for that interface (modulo link-local — see below). Apply
//     adds missing entries and removes extras.
//   - `mtu: <int>` — link MTU (68-65535).
//   - `up: <bool>` — link admin state (`ip link set up|down`).
//
// At least one of those three must be set; a bare `interface:` decl
// is a no-op and is rejected at validate. `interface:` itself is the
// identity; the module does not create or remove interfaces (use
// `bond` / `bridge` / `vlan` for virtual interfaces, or a
// provisioning tool for physical ones). A missing interface surfaces
// as `ErrInterfaceNotFound`.
//
// Link-local handling: kernel-auto-assigned IPv6 (`fe80::/10`) and
// IPv4 (`169.254.0.0/16`) link-local addresses are **never removed**
// by the address-set reconciliation — they're operationally required
// for IPv6 ND / autoconf. The operator can still *declare* a
// link-local in `addresses:` (the kernel will reject the add if it
// conflicts), but the reconciler won't strip one that's already
// there.
//
// Declaration.Name is just a human label (the decl ID); the
// interface identity is `interface:`. State `present` only —
// removing an interface is V1X (bring it down via `up: false`).
// DriftSeverity HIGH (wrong network = unreachable host). Nil →
// MEDIUM.
//
// v0.1 out of scope (v0.x candidates):
//   - **Persistent / boot-survive configuration** for the remaining
//     network managers: NetworkManager `system-connections/`, Debian
//     `/etc/network/interfaces`, RHEL `ifcfg-*` (networkd + netplan
//     landed via `persist:`). Each is its own distro-aware renderer.
//   - **Per-family address management** — declare IPv4 and IPv6
//     sets independently (today they're one merged list).
//   - **Address scope, valid_lft, preferred_lft, broadcast, peer**
//     attributes per `ip addr add` options.
//   - **Routing-table per-route attributes** beyond the basic
//     `route` module that ships in the next PR.
//   - **DNS resolvers, search domains, NTP servers** — these live
//     in /etc/resolv.conf / systemd-resolved / NetworkManager and
//     are V1X.
//   - **Wireless (`wpa_supplicant`), 802.1X, IPsec, WireGuard,
//     OpenVPN** — vendor / protocol modules in their own right.
//   - **Interface creation / removal** (use bond / bridge / vlan).
package network

import (
	"context"
	"fmt"
	"sort"
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

func (m *Module) Name() string { return "network" }

func (m *Module) ValidStates() []string { return []string{StatePresent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: a wrong-network interface is unreachability. HIGH;
// MEDIUM nil. Operators override via `severity:`.
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
	state, err := m.provider.GetInterface(ctx, p.Interface)
	if err != nil {
		return nil, err
	}
	diffs := m.diff(p, state)
	if p.Persist != "" {
		backend, err := m.resolveBackend(ctx, p)
		if err != nil {
			return nil, err
		}
		drift, _, err := m.persistDrift(ctx, backend, p)
		if err != nil {
			return nil, err
		}
		if drift {
			diffs = append(diffs, fmt.Sprintf("persist(%s) file out of date", backend))
		}
	}
	if len(diffs) == 0 {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: strings.Join(diffs, "; ")}, nil
}

// resolveBackend turns `persist: auto` into a concrete backend; an
// explicit networkd / netplan passes through.
func (m *Module) resolveBackend(ctx context.Context, p *params) (string, error) {
	if p.Persist == PersistAuto {
		return m.provider.DetectBackend(ctx)
	}
	return p.Persist, nil
}

// persistDrift reports whether the on-disk persistent file differs from
// the desired render (or is absent). It returns the desired content so
// Apply can write it without re-rendering.
func (m *Module) persistDrift(ctx context.Context, backend string, p *params) (bool, string, error) {
	desired, err := renderPersist(backend, p)
	if err != nil {
		return false, "", err
	}
	current, exists, err := m.provider.GetPersisted(ctx, backend, p.Interface)
	if err != nil {
		return false, "", err
	}
	return !exists || current != desired, desired, nil
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := m.parsed(decl)
	if err != nil {
		return nil, err
	}
	state, err := m.provider.GetInterface(ctx, p.Interface)
	if err != nil {
		return failure(start), err
	}
	diffs := m.diff(p, state)

	// Resolve the persistent-file drift up front so a runtime-converged
	// interface with a stale (or missing) persist file still applies.
	var (
		backend      string
		persistDrift bool
		desired      string
	)
	if p.Persist != "" {
		backend, err = m.resolveBackend(ctx, p)
		if err != nil {
			return failure(start), err
		}
		persistDrift, desired, err = m.persistDrift(ctx, backend, p)
		if err != nil {
			return failure(start), err
		}
	}

	if len(diffs) == 0 && !persistDrift {
		return ok(start, false, "", "already converged"), nil
	}

	// Order: MTU first (some changes require it), then addresses,
	// then admin state last (so any address / MTU change is in
	// place before bringing the link up).
	if p.HasMTU && state.MTU != p.MTU {
		if err := m.provider.SetMTU(ctx, p.Interface, p.MTU); err != nil {
			return failure(start), fmt.Errorf("set mtu: %w", err)
		}
	}
	if p.HasAddresses {
		toAdd, toRemove := m.addressDelta(p, state)
		for _, cidr := range toAdd {
			if err := m.provider.AddAddress(ctx, p.Interface, cidr); err != nil {
				return failure(start), fmt.Errorf("add %s: %w", cidr, err)
			}
		}
		for _, cidr := range toRemove {
			if err := m.provider.DelAddress(ctx, p.Interface, cidr); err != nil {
				return failure(start), fmt.Errorf("remove %s: %w", cidr, err)
			}
		}
	}
	if p.HasUp && state.Up != p.Up {
		if err := m.provider.SetLinkUp(ctx, p.Interface, p.Up); err != nil {
			return failure(start), fmt.Errorf("set link up=%v: %w", p.Up, err)
		}
	}
	if persistDrift {
		if err := m.provider.SetPersisted(ctx, backend, p.Interface, desired); err != nil {
			return failure(start), fmt.Errorf("write persist(%s): %w", backend, err)
		}
		diffs = append(diffs, fmt.Sprintf("persist(%s) written", backend))
	}
	return ok(start, true, strings.Join(diffs, "; "), "applied"), nil
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

// diff produces the human-readable change list. Empty list = no
// drift. Order matches Apply (MTU → addresses → up).
func (m *Module) diff(p *params, state *InterfaceState) []string {
	var diffs []string
	if p.HasMTU && state.MTU != p.MTU {
		diffs = append(diffs, fmt.Sprintf("mtu %d → %d", state.MTU, p.MTU))
	}
	if p.HasAddresses {
		add, remove := m.addressDelta(p, state)
		if len(add) > 0 {
			diffs = append(diffs, fmt.Sprintf("addr +%s", strings.Join(add, ",")))
		}
		if len(remove) > 0 {
			diffs = append(diffs, fmt.Sprintf("addr -%s", strings.Join(remove, ",")))
		}
	}
	if p.HasUp && state.Up != p.Up {
		diffs = append(diffs, fmt.Sprintf("link %s → %s", upStr(state.Up), upStr(p.Up)))
	}
	return diffs
}

// addressDelta returns (toAdd, toRemove) as sorted CIDR string
// slices. toRemove excludes kernel-auto-assigned link-local
// addresses — those are never stripped.
func (m *Module) addressDelta(p *params, state *InterfaceState) (toAdd, toRemove []string) {
	have := make(map[string]struct{}, len(state.Addresses))
	for _, a := range state.Addresses {
		have[a] = struct{}{}
	}
	want := make(map[string]struct{}, len(p.Addresses))
	for _, a := range p.Addresses {
		want[a] = struct{}{}
	}
	for a := range want {
		if _, ok := have[a]; !ok {
			toAdd = append(toAdd, a)
		}
	}
	for a := range have {
		if _, ok := want[a]; ok {
			continue
		}
		if isLinkLocal(a) {
			continue
		}
		toRemove = append(toRemove, a)
	}
	sort.Strings(toAdd)
	sort.Strings(toRemove)
	return toAdd, toRemove
}

func upStr(b bool) string {
	if b {
		return "up"
	}
	return "down"
}

func ok(start time.Time, changed bool, diff, comment string) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: true, Changed: changed, Diff: diff, Comment: comment, Duration: time.Since(start)}
}
func failure(start time.Time) *statemgmt.StateResult {
	return &statemgmt.StateResult{Success: false, Changed: false, Duration: time.Since(start)}
}
