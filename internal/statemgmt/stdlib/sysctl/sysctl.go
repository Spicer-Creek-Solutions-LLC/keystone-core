// Package sysctl implements the `sysctl` stdlib state module —
// kernel parameter management per PROJECT-DETAILS §4.8.
//
// State semantics:
//
//	present — the kernel parameter <Name> has value <value>, and
//	          (when persist:true, the default) a keystone-managed
//	          drop-in under /etc/sysctl.d/ records it so the value
//	          survives a reboot.
//
// Name accepts the dotted (net.ipv4.ip_forward) or slashed
// (net/ipv4/ip_forward) notation; the module normalises slashes →
// dots so both map to the same persistence file.
//
// v0.1 out of scope (v0.x candidates):
//   - One consolidated keystone /etc/sysctl.d file instead of one
//     file per key
//   - `sysctl --system` reload after writing config (v1.0 sets the
//     value directly, so a reload isn't needed this boot)
//   - Non-Linux (BSD/macOS sysctl namespaces differ)
package sysctl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

type Module struct {
	provider Provider
}

func New() statemgmt.Module { return &Module{provider: defaultProvider()} }

// NewWithProvider is the test injection point.
func NewWithProvider(p Provider) statemgmt.Module { return &Module{provider: p} }

func (m *Module) Name() string { return "sysctl" }

func (m *Module) ValidStates() []string { return []string{StatePresent} }

func (m *Module) Validate(decl *statemgmt.Declaration) error {
	p, err := parseParams(decl)
	if err != nil {
		return err
	}
	return p.validate()
}

// DriftSeverity: sysctl drift is config-level. Some keys are
// security-relevant (e.g., net.ipv4.conf.all.rp_filter) but the
// module can't tell which — operator overrides via severity:.
func (m *Module) DriftSeverity(*statemgmt.Declaration, *statemgmt.ModuleCheckResult) statemgmt.DriftSeverity {
	return statemgmt.DriftSeverityMedium
}

func (m *Module) Check(_ context.Context, decl *statemgmt.Declaration) (*statemgmt.ModuleCheckResult, error) {
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	return m.check(p)
}

func (m *Module) check(p *params) (*statemgmt.ModuleCheckResult, error) {
	runtimeVal, exists, err := m.provider.Get(p.Key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrKeyNotFound, p.Key)
	}
	want := normalizeValue(p.Value)
	var diffs []string
	if runtimeVal != want {
		diffs = append(diffs, fmt.Sprintf("runtime %q → %q", runtimeVal, want))
	}
	if p.Persist {
		persisted, ok, err := m.provider.ReadPersist(p.Key)
		if err != nil {
			return nil, err
		}
		if !ok {
			diffs = append(diffs, "persist file missing")
		} else if persisted != want {
			diffs = append(diffs, fmt.Sprintf("persist %q → %q", persisted, want))
		}
	}
	if len(diffs) == 0 {
		return &statemgmt.ModuleCheckResult{Matches: true}, nil
	}
	return &statemgmt.ModuleCheckResult{Matches: false, Diff: strings.Join(diffs, "; ")}, nil
}

func (m *Module) Apply(ctx context.Context, decl *statemgmt.Declaration) (*statemgmt.StateResult, error) {
	start := time.Now()
	p, err := parseParams(decl)
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	pre, err := m.check(p)
	if err != nil {
		return &statemgmt.StateResult{Success: false, Duration: time.Since(start)}, err
	}
	if pre.Matches {
		return &statemgmt.StateResult{Success: true, Comment: "already converged", Duration: time.Since(start)}, nil
	}
	want := normalizeValue(p.Value)
	if err := m.provider.Set(ctx, p.Key, want); err != nil {
		return &statemgmt.StateResult{Success: false, Diff: pre.Diff, Duration: time.Since(start)}, err
	}
	if p.Persist {
		if err := m.provider.WritePersist(p.Key, want); err != nil {
			return &statemgmt.StateResult{Success: false, Diff: pre.Diff, Duration: time.Since(start)}, err
		}
	}
	return &statemgmt.StateResult{Success: true, Changed: true, Diff: pre.Diff, Comment: "applied", Duration: time.Since(start)}, nil
}

func (m *Module) Test(ctx context.Context, decl *statemgmt.Declaration) (bool, error) {
	res, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return res.Matches, nil
}
